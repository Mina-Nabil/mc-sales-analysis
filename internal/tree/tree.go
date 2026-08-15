// Package tree implements mutations on the car tree: creating brands and models
// (including from the review queue), editing model attributes, merging models,
// and detaching aliases. Every mutation writes change_log and, where it moves
// resolved references, re-derives the affected facts (TECH §2.5, §2.6, §6.3).
package tree

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// applyCasing enforces the §0.1 naming rule: names of ≤4 letters → ALL CAPS;
// longer all-caps names → Title case; anything else is left as typed.
func applyCasing(name string) string {
	name = strings.TrimSpace(name)
	letters := 0
	hasLower := false
	hasUpper := false
	for _, r := range name {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsLower(r) {
				hasLower = true
			}
			if unicode.IsUpper(r) {
				hasUpper = true
			}
		}
	}
	if letters == 0 {
		return name // e.g. Arabic or numeric — no case
	}
	if letters <= 4 {
		return strings.ToUpper(name)
	}
	if hasUpper && !hasLower {
		return titleCase(name)
	}
	return name
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(strings.ToLower(w))
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
		}
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// CreateBrand adds a new brand (a business event, §4.2). Returns its id.
func CreateBrand(ctx context.Context, pool *pgxpool.Pool, name, origin string, parentID *int64, actorID int64) (int64, string, error) {
	name = applyCasing(name)
	if name == "" {
		return 0, "", fmt.Errorf("brand name required")
	}
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO brands (name, origin, parent_brand_id, status)
		VALUES ($1,$2,$3,'confirmed') RETURNING id`,
		name, nullIf(origin), parentID).Scan(&id)
	if err != nil {
		return 0, "", fmt.Errorf("create brand: %w", err)
	}
	_ = logChange(ctx, pool, "brand", id, "create", actorID, 0)
	return id, name, nil
}

// CreateModel adds a new model under a brand. Returns its id.
func CreateModel(ctx context.Context, pool *pgxpool.Pool, brandID int64, name, carType string, segmentID *int64, tier string, actorID int64) (int64, string, error) {
	name = applyCasing(name)
	if name == "" {
		return 0, "", fmt.Errorf("model name required")
	}
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO models (brand_id, name, car_type, segment_id, tier, status)
		VALUES ($1,$2,$3,$4,$5,'confirmed') RETURNING id`,
		brandID, name, nullIf(carType), segmentID, nullIf(tier)).Scan(&id)
	if err != nil {
		return 0, "", fmt.Errorf("create model: %w", err)
	}
	_ = logChange(ctx, pool, "model", id, "create", actorID, 0)
	return id, name, nil
}

// EditModel updates model attributes. Nothing is stored on facts, so changing a
// segment/tier re-derives all history automatically via joins; we still record
// the affected volume in change_log for the impact trail (§6.3).
func EditModel(ctx context.Context, pool *pgxpool.Pool, id int64, name, carType, tier string, segmentID *int64, actorID int64) error {
	var units int
	_ = pool.QueryRow(ctx, `SELECT COALESCE(sum(volume),0) FROM facts WHERE model_id=$1`, id).Scan(&units)
	ct, err := pool.Exec(ctx, `
		UPDATE models SET
		  name = COALESCE(NULLIF($2,''), name),
		  car_type = COALESCE(NULLIF($3,''), car_type),
		  tier = COALESCE(NULLIF($4,''), tier),
		  segment_id = COALESCE($5, segment_id),
		  updated_at = now()
		WHERE id=$1`, id, applyCasing(name), carType, tier, segmentID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("model %d not found", id)
	}
	return logChange(ctx, pool, "model", id, "edit", actorID, units)
}

// MergeInfo previews a merge.
type MergeInfo struct {
	SourceModel string `json:"source_model"`
	IntoModel   string `json:"into_model"`
	Facts       int    `json:"facts"`
	Units       int    `json:"units"`
	Aliases     int    `json:"aliases"`
}

// MergePreview reports what a merge would move (§6.3 impact preview).
func MergePreview(ctx context.Context, pool *pgxpool.Pool, srcID, intoID int64) (MergeInfo, error) {
	var mi MergeInfo
	if err := pool.QueryRow(ctx, `SELECT name FROM models WHERE id=$1`, srcID).Scan(&mi.SourceModel); err != nil {
		return mi, fmt.Errorf("source model %d: %w", srcID, err)
	}
	if err := pool.QueryRow(ctx, `SELECT name FROM models WHERE id=$1`, intoID).Scan(&mi.IntoModel); err != nil {
		return mi, fmt.Errorf("target model %d: %w", intoID, err)
	}
	_ = pool.QueryRow(ctx,
		`SELECT count(*), COALESCE(sum(volume),0) FROM facts WHERE model_id=$1`, srcID).Scan(&mi.Facts, &mi.Units)
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM model_aliases WHERE model_id=$1`, srcID).Scan(&mi.Aliases)
	return mi, nil
}

// MergeModels moves all aliases and facts from src into the survivor, leaving a
// redirect (merged_into). Both models must belong to the same brand.
func MergeModels(ctx context.Context, pool *pgxpool.Pool, srcID, intoID int64, actorID int64) (MergeInfo, error) {
	mi, err := MergePreview(ctx, pool, srcID, intoID)
	if err != nil {
		return mi, err
	}
	if srcID == intoID {
		return mi, fmt.Errorf("cannot merge a model into itself")
	}
	var b1, b2 int64
	if err := pool.QueryRow(ctx, `SELECT brand_id FROM models WHERE id=$1`, srcID).Scan(&b1); err != nil {
		return mi, err
	}
	if err := pool.QueryRow(ctx, `SELECT brand_id FROM models WHERE id=$1`, intoID).Scan(&b2); err != nil {
		return mi, err
	}
	if b1 != b2 {
		return mi, fmt.Errorf("models belong to different brands; move one first")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return mi, err
	}
	defer tx.Rollback(ctx)

	// Re-point aliases, skipping any that would collide with an existing (brand,raw).
	if _, err := tx.Exec(ctx, `
		UPDATE model_aliases a SET model_id=$2
		 WHERE a.model_id=$1
		   AND NOT EXISTS (SELECT 1 FROM model_aliases b
		                    WHERE b.brand_id=a.brand_id AND b.raw=a.raw AND b.model_id=$2)`,
		srcID, intoID); err != nil {
		return mi, err
	}
	if _, err := tx.Exec(ctx, `UPDATE facts SET model_id=$2 WHERE model_id=$1`, srcID, intoID); err != nil {
		return mi, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE models SET merged_into=$2, status='rejected', updated_at=now() WHERE id=$1`, srcID, intoID); err != nil {
		return mi, err
	}
	if err := logChangeTx(ctx, tx, "model", srcID, "merge", actorID, mi.Units); err != nil {
		return mi, err
	}
	return mi, tx.Commit(ctx)
}

// DeleteAlias detaches an alias and returns its facts to unresolved.
func DeleteAlias(ctx context.Context, pool *pgxpool.Pool, aliasID int64, actorID int64) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var brandID int64
	var raw string
	if err := tx.QueryRow(ctx,
		`SELECT brand_id, raw FROM model_aliases WHERE id=$1`, aliasID).Scan(&brandID, &raw); err != nil {
		return 0, fmt.Errorf("alias %d: %w", aliasID, err)
	}
	var units int
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(sum(volume),0) FROM facts
		 WHERE brand_id=$1 AND raw_model=$2 AND model_id IS NOT NULL`, brandID, raw).Scan(&units)
	if _, err := tx.Exec(ctx, `
		UPDATE facts SET model_id=NULL, status='unresolved'
		 WHERE brand_id=$1 AND raw_model=$2`, brandID, raw); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM model_aliases WHERE id=$1`, aliasID); err != nil {
		return 0, err
	}
	if err := logChangeTx(ctx, tx, "model_alias", aliasID, "delete", actorID, units); err != nil {
		return 0, err
	}
	return units, tx.Commit(ctx)
}

// ── brand-level resolution (for facts with no brand) ────────────────────────

// BrandQueueItem is a distinct unresolved raw brand string awaiting a brand.
type BrandQueueItem struct {
	RawBrand string `json:"raw_brand"`
	Volume   int    `json:"volume"`
	Rows     int    `json:"rows"`
}

// BrandQueue lists raw brand strings that resolve to no brand, volume-ranked.
func BrandQueue(ctx context.Context, pool *pgxpool.Pool, limit int) ([]BrandQueueItem, error) {
	rows, err := pool.Query(ctx, `
		SELECT raw_brand, COALESCE(sum(volume),0), count(*)
		  FROM facts WHERE brand_id IS NULL
		 GROUP BY raw_brand ORDER BY sum(volume) DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BrandQueueItem{}
	for rows.Next() {
		var it BrandQueueItem
		if err := rows.Scan(&it.RawBrand, &it.Volume, &it.Rows); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ResolveBrand links a raw brand string to a brand (existing or newly created),
// writing a confirmed alias and setting brand_id on every matching fact. The
// facts stay model-unresolved, so the fuzzy/model queue then handles them.
func ResolveBrand(ctx context.Context, pool *pgxpool.Pool, rawBrand string, brandID int64, actorID int64) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO brand_aliases (raw, raw_normalized, raw_normalized_nospace, brand_id, status, method, decided_by, decided_at)
		VALUES ($1,$2,$3,$4,'confirmed','human',$5,now())
		ON CONFLICT (raw) DO UPDATE SET brand_id=EXCLUDED.brand_id, status='confirmed', method='human'`,
		rawBrand, domain.Normalize(rawBrand), domain.NormalizeNoSpace(rawBrand), brandID, actorID); err != nil {
		return 0, err
	}
	var units int
	_ = tx.QueryRow(ctx, `SELECT COALESCE(sum(volume),0) FROM facts WHERE raw_brand=$1 AND brand_id IS NULL`, rawBrand).Scan(&units)
	if _, err := tx.Exec(ctx,
		`UPDATE facts SET brand_id=$2 WHERE raw_brand=$1 AND brand_id IS NULL`, rawBrand, brandID); err != nil {
		return 0, err
	}
	if err := logChangeTx(ctx, tx, "brand_alias", brandID, "resolve_brand", actorID, units); err != nil {
		return 0, err
	}
	return units, tx.Commit(ctx)
}

// CreateModelForReview creates a model and confirms a review alias to it,
// re-deriving that alias's facts. Closes a "new model" review item in one step.
func CreateModelForReview(ctx context.Context, pool *pgxpool.Pool, aliasID int64, name, carType string, segmentID *int64, tier string, actorID int64) (int64, int, error) {
	var brandID int64
	var raw, status string
	if err := pool.QueryRow(ctx,
		`SELECT brand_id, raw, status FROM model_aliases WHERE id=$1`, aliasID).Scan(&brandID, &raw, &status); err != nil {
		return 0, 0, fmt.Errorf("alias %d: %w", aliasID, err)
	}
	if status != "needs_review" {
		return 0, 0, fmt.Errorf("item was already %s — refresh the queue", status)
	}
	modelID, _, err := CreateModel(ctx, pool, brandID, name, carType, segmentID, tier, actorID)
	if err != nil {
		return 0, 0, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE model_aliases SET model_id=$1, status='confirmed', method='human', confidence=NULL, decided_at=now()
		 WHERE id=$2 AND status='needs_review'`, modelID, aliasID); err != nil {
		return 0, 0, err
	}
	var units int
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(sum(volume),0) FROM facts
		 WHERE brand_id=$1 AND raw_model=$2 AND status IN ('needs_review','unresolved','auto_resolved')`,
		brandID, raw).Scan(&units)
	if _, err := tx.Exec(ctx, `
		UPDATE facts SET model_id=$1, status='confirmed'
		 WHERE brand_id=$2 AND raw_model=$3 AND status IN ('needs_review','unresolved','auto_resolved')`,
		modelID, brandID, raw); err != nil {
		return 0, 0, err
	}
	if err := logChangeTx(ctx, tx, "model", modelID, "create_from_review", actorID, units); err != nil {
		return 0, 0, err
	}
	return modelID, units, tx.Commit(ctx)
}

// ── helpers ─────────────────────────────────────────────────────────────────

func nullIf(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func logChange(ctx context.Context, pool *pgxpool.Pool, entity string, id int64, action string, actorID int64, units int) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO change_log (entity_type, entity_id, action, actor_id, actor_kind, volume_impact)
		 VALUES ($1,$2,$3,$4,'human',$5)`, entity, id, action, actorID, units)
	return err
}

func logChangeTx(ctx context.Context, tx pgx.Tx, entity string, id int64, action string, actorID int64, units int) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO change_log (entity_type, entity_id, action, actor_id, actor_kind, volume_impact)
		 VALUES ($1,$2,$3,$4,'human',$5)`, entity, id, action, actorID, units)
	return err
}
