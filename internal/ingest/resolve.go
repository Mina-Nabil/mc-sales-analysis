package ingest

import (
	"context"
	"fmt"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// resolver holds the seeded alias tables in memory for the deterministic
// resolution ladder (TECH §3.1 tiers 1–2b). No fuzzy, no AI — the migration
// uses only what the alias library already knows.
type resolver struct {
	brandExact, brandNorm, brandNoSpace map[string]int64
	// model maps are keyed by "brandID\x00key" (aliases are per-brand, §2.2).
	modelExact, modelNorm, modelNoSpace map[string]int64
	govNorm, unitNorm                   map[string]int64
	// motorcycle recognition (§0.1, §11.6), keyed on NormalizeNoSpace
	motoBrand      map[string]bool
	motoBrandModel map[string]bool
}

func loadResolver(ctx context.Context, pool *pgxpool.Pool) (*resolver, error) {
	r := &resolver{
		brandExact: map[string]int64{}, brandNorm: map[string]int64{}, brandNoSpace: map[string]int64{},
		modelExact: map[string]int64{}, modelNorm: map[string]int64{}, modelNoSpace: map[string]int64{},
		govNorm: map[string]int64{}, unitNorm: map[string]int64{},
		motoBrand: map[string]bool{}, motoBrandModel: map[string]bool{},
	}

	brandRows, err := pool.Query(ctx,
		`SELECT raw, raw_normalized, raw_normalized_nospace, brand_id
		   FROM brand_aliases WHERE brand_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for brandRows.Next() {
		var raw, norm, nospace string
		var id int64
		if err := brandRows.Scan(&raw, &norm, &nospace, &id); err != nil {
			brandRows.Close()
			return nil, err
		}
		put(r.brandExact, raw, id)
		put(r.brandNorm, norm, id)
		put(r.brandNoSpace, nospace, id)
	}
	brandRows.Close()
	if err := brandRows.Err(); err != nil {
		return nil, err
	}

	modelRows, err := pool.Query(ctx,
		`SELECT brand_id, raw, raw_normalized, raw_normalized_nospace, model_id
		   FROM model_aliases WHERE model_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for modelRows.Next() {
		var bid, mid int64
		var raw, norm, nospace string
		if err := modelRows.Scan(&bid, &raw, &norm, &nospace, &mid); err != nil {
			modelRows.Close()
			return nil, err
		}
		put(r.modelExact, mkey(bid, raw), mid)
		put(r.modelNorm, mkey(bid, norm), mid)
		put(r.modelNoSpace, mkey(bid, nospace), mid)
	}
	modelRows.Close()
	if err := modelRows.Err(); err != nil {
		return nil, err
	}

	geoRows, err := pool.Query(ctx,
		`SELECT raw_normalized, kind, target_id FROM geo_aliases WHERE target_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	for geoRows.Next() {
		var norm, kind string
		var id int64
		if err := geoRows.Scan(&norm, &kind, &id); err != nil {
			geoRows.Close()
			return nil, err
		}
		switch kind {
		case "governorate":
			put(r.govNorm, norm, id)
		case "traffic_unit":
			put(r.unitNorm, norm, id)
		}
	}
	geoRows.Close()
	if err := geoRows.Err(); err != nil {
		return nil, err
	}

	mkRows, err := pool.Query(ctx, `SELECT kind, brand_key, model_key FROM motorcycle_keys`)
	if err != nil {
		return nil, err
	}
	for mkRows.Next() {
		var kind, bk, mk string
		if err := mkRows.Scan(&kind, &bk, &mk); err != nil {
			mkRows.Close()
			return nil, err
		}
		switch kind {
		case "brand":
			r.motoBrand[bk] = true
		case "brand_model":
			r.motoBrandModel[bk+"\x00"+mk] = true
		}
	}
	mkRows.Close()
	if err := mkRows.Err(); err != nil {
		return nil, err
	}

	fmt.Printf("resolver loaded: %d brand keys, %d model keys, %d gov + %d unit geo keys\n",
		len(r.brandExact)+len(r.brandNorm)+len(r.brandNoSpace),
		len(r.modelExact)+len(r.modelNorm)+len(r.modelNoSpace),
		len(r.govNorm), len(r.unitNorm))
	return r, nil
}

// brand resolves a raw brand string through tiers 1 → 2 → 2b. Returns 0 if none.
func (r *resolver) brand(raw string) int64 { id, _ := r.brandT(raw); return id }

// brandT is brand() but also reports which tier matched (for dry-run stats).
func (r *resolver) brandT(raw string) (int64, string) {
	if id, ok := r.brandExact[raw]; ok {
		return id, "exact"
	}
	if id, ok := r.brandNorm[domain.Normalize(raw)]; ok {
		return id, "normalized"
	}
	if id, ok := r.brandNoSpace[domain.NormalizeNoSpace(raw)]; ok {
		return id, "nospace"
	}
	return 0, ""
}

// model resolves a raw model string within an already-resolved brand.
func (r *resolver) model(brandID int64, raw string) int64 { id, _ := r.modelT(brandID, raw); return id }

func (r *resolver) modelT(brandID int64, raw string) (int64, string) {
	if id, ok := r.modelExact[mkey(brandID, raw)]; ok {
		return id, "exact"
	}
	if id, ok := r.modelNorm[mkey(brandID, domain.Normalize(raw))]; ok {
		return id, "normalized"
	}
	if id, ok := r.modelNoSpace[mkey(brandID, domain.NormalizeNoSpace(raw))]; ok {
		return id, "nospace"
	}
	return 0, ""
}

// isMotorcycle reports whether a raw (brand, model) is a motorcycle, using the
// no-space normalized recognition keys derived from history (§0.1, §11.6).
func (r *resolver) isMotorcycle(rawBrand, rawModel string) bool {
	bk := domain.NormalizeNoSpace(rawBrand)
	if r.motoBrand[bk] {
		return true
	}
	return r.motoBrandModel[bk+"\x00"+domain.NormalizeNoSpace(rawModel)]
}

// geo resolves a raw governorate/unit string via its normalized key.
func (r *resolver) geo(m map[string]int64, raw string) int64 {
	if id, ok := m[domain.Normalize(raw)]; ok {
		return id
	}
	if id, ok := m[domain.NormalizeNoSpace(raw)]; ok {
		return id
	}
	return 0
}

// put records the first mapping for a key; a later conflicting mapping is
// ignored (the seed is confirmed and unambiguous, but guard anyway).
func put(m map[string]int64, key string, id int64) {
	if key == "" {
		return
	}
	if _, exists := m[key]; !exists {
		m[key] = id
	}
}

func mkey(brandID int64, key string) string {
	return fmt.Sprintf("%d\x00%s", brandID, key)
}

// ── deterministic replay over already-stored facts ──────────────────────────

// ReresolveResult reports what a replay linked.
type ReresolveResult struct {
	BrandRows   int `json:"brand_rows"`
	BrandVolume int `json:"brand_volume"`
	ModelRows   int `json:"model_rows"`
	ModelVolume int `json:"model_volume"`
}

// ReresolveUnresolved replays the deterministic ladder (tiers 1, 2, 2b) over
// facts that are still unresolved, against the alias tables as they stand now.
//
// Confirming a review item writes an alias for one exact raw spelling and
// re-derives only that spelling's facts. A sibling spelling already stored —
// differing just by spacing, a separator or tatweel — is covered by the new
// alias's normalized keys on the NEXT import, but the rows already in the table
// stay unresolved until something replays the ladder over them. This is that
// replay: no thresholds, no fuzzy, no guesses, so it is safe to run after every
// review decision and at the end of every import.
//
// Pairs whose alias was rejected are skipped, for the same reason Resolve skips
// them: a reviewer detached those facts deliberately.
func ReresolveUnresolved(ctx context.Context, pool *pgxpool.Pool) (ReresolveResult, error) {
	return reresolve(ctx, pool, false)
}

// ReresolveDryRun reports exactly what ReresolveUnresolved would link, without
// keeping any of it: the same work runs inside a transaction that is rolled back.
func ReresolveDryRun(ctx context.Context, pool *pgxpool.Pool) (ReresolveResult, error) {
	return reresolve(ctx, pool, true)
}

func reresolve(ctx context.Context, pool *pgxpool.Pool, dryRun bool) (ReresolveResult, error) {
	var out ReresolveResult
	res, err := loadResolver(ctx, pool)
	if err != nil {
		return out, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)

	// ── brands first: a fact that gains a brand becomes eligible for a model ──
	type brandItem struct {
		raw string
		id  int64
	}
	var brandHits []brandItem
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT raw_brand FROM facts WHERE brand_id IS NULL`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return out, err
		}
		if id := res.brand(raw); id != 0 {
			brandHits = append(brandHits, brandItem{raw, id})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	for _, b := range brandHits {
		var units int
		_ = tx.QueryRow(ctx,
			`SELECT COALESCE(sum(volume),0) FROM facts WHERE raw_brand=$1 AND brand_id IS NULL`,
			b.raw).Scan(&units)
		ct, err := tx.Exec(ctx,
			`UPDATE facts SET brand_id=$2 WHERE raw_brand=$1 AND brand_id IS NULL`, b.raw, b.id)
		if err != nil {
			return out, err
		}
		out.BrandRows += int(ct.RowsAffected())
		out.BrandVolume += units
	}

	// ── then models, within the (now possibly larger) resolved-brand set ──────
	type modelItem struct {
		brandID int64
		raw     string
		modelID int64
	}
	var modelHits []modelItem
	rows, err = tx.Query(ctx, `
		SELECT DISTINCT f.brand_id, f.raw_model
		  FROM facts f
		 WHERE f.brand_id IS NOT NULL AND f.model_id IS NULL AND f.status = 'unresolved'
		   AND NOT EXISTS (
		         SELECT 1 FROM model_aliases a
		          WHERE a.brand_id = f.brand_id AND a.raw = f.raw_model
		            AND a.status = 'rejected')`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var bid int64
		var raw string
		if err := rows.Scan(&bid, &raw); err != nil {
			rows.Close()
			return out, err
		}
		if mid := res.model(bid, raw); mid != 0 {
			modelHits = append(modelHits, modelItem{bid, raw, mid})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	for _, m := range modelHits {
		var units int
		_ = tx.QueryRow(ctx, `
			SELECT COALESCE(sum(volume),0) FROM facts
			 WHERE brand_id=$1 AND raw_model=$2 AND model_id IS NULL AND status='unresolved'`,
			m.brandID, m.raw).Scan(&units)
		ct, err := tx.Exec(ctx, `
			UPDATE facts SET model_id=$3, status='confirmed'
			 WHERE brand_id=$1 AND raw_model=$2 AND model_id IS NULL AND status='unresolved'`,
			m.brandID, m.raw, m.modelID)
		if err != nil {
			return out, err
		}
		out.ModelRows += int(ct.RowsAffected())
		out.ModelVolume += units
	}

	if dryRun {
		return out, tx.Rollback(ctx)
	}
	if out.BrandRows > 0 || out.ModelRows > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO change_log (entity_type, action, actor_kind, volume_impact)
			VALUES ('facts','deterministic_reresolve','agent',$1)`,
			out.BrandVolume+out.ModelVolume); err != nil {
			return out, err
		}
	}
	return out, tx.Commit(ctx)
}
