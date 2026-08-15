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
