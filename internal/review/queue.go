// Package review implements tier-3 fuzzy resolution of the unresolved backlog
// and the review-queue operations (TECH §3.3, §4.3, §5, §6.2). Confirming an
// item creates a confirmed alias (tier-1 forever) and re-derives every fact
// pointing at that raw string — the "fix one node, correct all history" mechanic.
package review

import (
	"context"
	"fmt"
	"strings"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type candidate struct {
	text    string
	modelID int64
}

// ResolveResult summarises a fuzzy resolve pass.
type ResolveResult struct {
	Distinct          int `json:"distinct"`
	AutoResolved      int `json:"auto_resolved"`
	Proposed          int `json:"proposed"`            // needs_review with a proposal
	NewModelCandidate int `json:"new_model_candidate"` // needs_review, no proposal
	AutoVolume        int `json:"auto_volume"`
}

// Resolve runs tier-3 fuzzy over every distinct (brand, raw_model) that is
// brand-resolved but model-unresolved, materialising review items as aliases and
// auto-linking the safe high-confidence ones.
func Resolve(ctx context.Context, pool *pgxpool.Pool, threshold, floor float64) (ResolveResult, error) {
	var out ResolveResult
	cands, err := loadCandidates(ctx, pool)
	if err != nil {
		return out, err
	}

	// Pairs whose alias was rejected are deliberately left alone: re-selecting
	// them would re-link the very facts a reviewer just detached (the alias
	// itself is skipped by upsertAlias's ON CONFLICT guard, so the item would
	// never come back to the queue to be noticed). They stay 'unresolved' and
	// are surfaced as unresolved volume until a human resolves them explicitly.
	rows, err := pool.Query(ctx, `
		SELECT f.brand_id, f.raw_model, count(*), sum(f.volume)
		  FROM facts f
		 WHERE f.brand_id IS NOT NULL AND f.model_id IS NULL AND f.status = 'unresolved'
		   AND NOT EXISTS (
		         SELECT 1 FROM model_aliases a
		          WHERE a.brand_id = f.brand_id AND a.raw = f.raw_model
		            AND a.status = 'rejected')
		 GROUP BY f.brand_id, f.raw_model
		 ORDER BY sum(f.volume) DESC`)
	if err != nil {
		return out, err
	}
	type item struct {
		brandID int64
		raw     string
		vol     int
	}
	var items []item
	for rows.Next() {
		var it item
		var cnt int
		if err := rows.Scan(&it.brandID, &it.raw, &cnt, &it.vol); err != nil {
			rows.Close()
			return out, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)

	for _, it := range items {
		out.Distinct++
		bestID, bestScore := int64(0), 0.0
		bestText := ""
		for _, c := range cands[it.brandID] {
			s := domain.FuzzyScore(it.raw, c.text)
			if s > bestScore {
				bestScore, bestID, bestText = s, c.modelID, c.text
			}
		}

		auto := bestID != 0 && domain.CanAutoLink(it.raw, bestText, bestScore, threshold)
		propose := bestID != 0 && bestScore >= floor

		var status, reasoning string
		var modelID *int64
		switch {
		case auto:
			status = "auto_resolved"
			modelID = &bestID
			reasoning = fmt.Sprintf("fuzzy auto-link to %q (%.2f)", bestText, bestScore)
			out.AutoResolved++
			out.AutoVolume += it.vol
		case propose:
			status = "needs_review"
			modelID = &bestID
			reasoning = fmt.Sprintf("fuzzy match to %q (%.2f) — confirm?", bestText, bestScore)
			out.Proposed++
		default:
			status = "needs_review"
			modelID = nil
			reasoning = "no candidate above floor — likely a new model"
			out.NewModelCandidate++
		}

		var conf *float64
		if bestID != 0 {
			conf = &bestScore
		}
		if err := upsertAlias(ctx, tx, it.brandID, it.raw, modelID, status, conf, reasoning); err != nil {
			return out, err
		}
		// Reflect the resolution onto the facts (proposal counts in reports, §4.3).
		newStatus := status
		if _, err := tx.Exec(ctx, `
			UPDATE facts SET model_id=$1, status=$2
			 WHERE brand_id=$3 AND raw_model=$4 AND status='unresolved'`,
			modelID, newStatus, it.brandID, it.raw); err != nil {
			return out, err
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO change_log (entity_type, action, actor_kind) VALUES ('facts','fuzzy_resolve','agent')`); err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
}

// Item is one review-queue entry as shown to a reviewer (§6.2).
type Item struct {
	AliasID    int64    `json:"alias_id"`
	BrandID    int64    `json:"brand_id"`
	Brand      string   `json:"brand"`
	RawModel   string   `json:"raw_model"`
	Proposal   string   `json:"proposal"` // proposed model name, or "" (new model)
	ProposalID *int64   `json:"proposal_id"`
	Confidence *float64 `json:"confidence"`
	Method     string   `json:"method"`
	Reasoning  string   `json:"reasoning"`
	Volume     int      `json:"volume"`
	Periods    int      `json:"periods"`
}

// List returns the queue, ranked by volume impact descending (§6.2).
func List(ctx context.Context, pool *pgxpool.Pool, limit int) ([]Item, error) {
	rows, err := pool.Query(ctx, `
		SELECT a.id, a.brand_id, b.name, a.raw, m.name, a.model_id, a.confidence, a.method,
		       COALESCE(a.reasoning,''),
		       COALESCE(v.vol,0), COALESCE(v.periods,0)
		  FROM model_aliases a
		  JOIN brands b ON b.id = a.brand_id
		  LEFT JOIN models m ON m.id = a.model_id
		  LEFT JOIN LATERAL (
		        SELECT sum(volume) AS vol, count(DISTINCT (period_year,period_month)) AS periods
		          FROM facts f
		         WHERE f.brand_id = a.brand_id AND f.raw_model = a.raw
		           AND f.status IN ('needs_review','unresolved')
		  ) v ON true
		 WHERE a.status = 'needs_review'
		 ORDER BY COALESCE(v.vol,0) DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		var proposal *string
		if err := rows.Scan(&it.AliasID, &it.BrandID, &it.Brand, &it.RawModel, &proposal,
			&it.ProposalID, &it.Confidence, &it.Method, &it.Reasoning,
			&it.Volume, &it.Periods); err != nil {
			return nil, err
		}
		if proposal != nil {
			it.Proposal = *proposal
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Confirm accepts a review item, optionally overriding the proposed model, and
// re-derives all facts for that raw string. Optimistic: fails if the item was
// already decided (§6.2). Returns the volume moved.
func Confirm(ctx context.Context, pool *pgxpool.Pool, aliasID int64, modelID *int64) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var brandID int64
	var raw string
	var proposed *int64
	var status string
	if err := tx.QueryRow(ctx,
		`SELECT brand_id, raw, model_id, status FROM model_aliases WHERE id=$1 FOR UPDATE`,
		aliasID).Scan(&brandID, &raw, &proposed, &status); err != nil {
		return 0, fmt.Errorf("alias %d: %w", aliasID, err)
	}
	if status != "needs_review" {
		return 0, fmt.Errorf("item was already %s — refresh the queue", status)
	}
	target := modelID
	if target == nil {
		target = proposed
	}
	if target == nil {
		return 0, fmt.Errorf("no model to confirm to; pass a model id (this is a new-model candidate)")
	}

	if _, err := tx.Exec(ctx, `
		UPDATE model_aliases
		   SET model_id=$1, status='confirmed', method='human', confidence=NULL,
		       decided_at=now()
		 WHERE id=$2`, *target, aliasID); err != nil {
		return 0, err
	}
	var units int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(sum(volume),0) FROM facts
		 WHERE brand_id=$1 AND raw_model=$2
		   AND status IN ('needs_review','unresolved','auto_resolved')`,
		brandID, raw).Scan(&units); err != nil {
		return 0, err
	}
	ct, err := tx.Exec(ctx, `
		UPDATE facts SET model_id=$1, status='confirmed'
		 WHERE brand_id=$2 AND raw_model=$3
		   AND status IN ('needs_review','unresolved','auto_resolved')`,
		*target, brandID, raw)
	if err != nil {
		return 0, err
	}
	if err := logChange(ctx, tx, "model_alias", aliasID, "confirm", units); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(ct.RowsAffected()), nil
}

// Reject marks a proposal wrong; its facts return to unresolved and the alias
// becomes a negative example (§4.5).
func Reject(ctx context.Context, pool *pgxpool.Pool, aliasID int64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var brandID int64
	var raw, status string
	var rejected *string
	if err := tx.QueryRow(ctx, `
		SELECT a.brand_id, a.raw, a.status, m.name
		  FROM model_aliases a
		  LEFT JOIN models m ON m.id = a.model_id
		 WHERE a.id=$1 FOR UPDATE OF a`,
		aliasID).Scan(&brandID, &raw, &status, &rejected); err != nil {
		return err
	}
	if status != "needs_review" {
		return fmt.Errorf("item was already %s — refresh the queue", status)
	}
	// model_id is cleared so the alias can never resolve, but the model that was
	// rejected is kept in reasoning — that is the negative example (§4.5).
	note := "rejected by reviewer"
	if rejected != nil {
		note = fmt.Sprintf("rejected by reviewer — not %q", *rejected)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE model_aliases SET status='rejected', model_id=NULL, reasoning=$2, decided_at=now() WHERE id=$1`,
		aliasID, note); err != nil {
		return err
	}
	var units int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(sum(volume),0) FROM facts
		 WHERE brand_id=$1 AND raw_model=$2 AND status IN ('needs_review','auto_resolved')`,
		brandID, raw).Scan(&units); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE facts SET model_id=NULL, status='unresolved'
		 WHERE brand_id=$1 AND raw_model=$2 AND status IN ('needs_review','auto_resolved')`,
		brandID, raw); err != nil {
		return err
	}
	if err := logChange(ctx, tx, "model_alias", aliasID, "reject", units); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Exclude marks a review item's volume as out of scope — not a car sale this
// product tracks. The alias is rejected so the string never proposes again, and
// its facts move to status='rejected', which the analytics layer filters out.
//
// Nothing is deleted. The rows stay, the units stay countable, and the excluded
// total is reported beside the motorcycle drop — §0.1 ("excluded, but always
// counted & reported") and §8.1 ("never clean totals") both apply here.
// Returns the units excluded.
func Exclude(ctx context.Context, pool *pgxpool.Pool, aliasID int64, reason string) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var brandID int64
	var raw, status string
	if err := tx.QueryRow(ctx,
		`SELECT brand_id, raw, status FROM model_aliases WHERE id=$1 FOR UPDATE`,
		aliasID).Scan(&brandID, &raw, &status); err != nil {
		return 0, fmt.Errorf("alias %d: %w", aliasID, err)
	}
	if status != "needs_review" {
		return 0, fmt.Errorf("item was already %s — refresh the queue", status)
	}

	note := "excluded by reviewer — volume out of scope"
	if strings.TrimSpace(reason) != "" {
		note = "excluded by reviewer — " + strings.TrimSpace(reason)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE model_aliases
		   SET status='rejected', model_id=NULL, method='human', confidence=NULL,
		       reasoning=$2, decided_at=now()
		 WHERE id=$1`, aliasID, note); err != nil {
		return 0, err
	}

	var units int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(sum(volume),0) FROM facts
		 WHERE brand_id=$1 AND raw_model=$2
		   AND status IN ('needs_review','unresolved','auto_resolved')`,
		brandID, raw).Scan(&units); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE facts SET model_id=NULL, status='rejected'
		 WHERE brand_id=$1 AND raw_model=$2
		   AND status IN ('needs_review','unresolved','auto_resolved')`,
		brandID, raw); err != nil {
		return 0, err
	}
	if err := logChange(ctx, tx, "model_alias", aliasID, "exclude", units); err != nil {
		return 0, err
	}
	return units, tx.Commit(ctx)
}

// BulkConfirm confirms every proposed (non-null) item at or above minConfidence
// (§6.2 "all N above 95%"). Returns items confirmed and total volume moved.
func BulkConfirm(ctx context.Context, pool *pgxpool.Pool, minConfidence float64) (int, int, error) {
	rows, err := pool.Query(ctx,
		`SELECT id FROM model_aliases
		  WHERE status='needs_review' AND model_id IS NOT NULL AND confidence >= $1
		  ORDER BY confidence DESC`, minConfidence)
	if err != nil {
		return 0, 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	n, vol := 0, 0
	for _, id := range ids {
		moved, err := Confirm(ctx, pool, id, nil)
		if err != nil {
			continue // was concurrently decided; skip
		}
		n++
		vol += moved
	}
	return n, vol, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func loadCandidates(ctx context.Context, pool *pgxpool.Pool) (map[int64][]candidate, error) {
	out := map[int64][]candidate{}
	// model names
	r1, err := pool.Query(ctx, `SELECT brand_id, id, name FROM models`)
	if err != nil {
		return nil, err
	}
	for r1.Next() {
		var bid, mid int64
		var name string
		if err := r1.Scan(&bid, &mid, &name); err != nil {
			r1.Close()
			return nil, err
		}
		out[bid] = append(out[bid], candidate{text: name, modelID: mid})
	}
	r1.Close()
	// confirmed/auto aliases as extra candidates
	r2, err := pool.Query(ctx,
		`SELECT brand_id, model_id, raw FROM model_aliases
		  WHERE model_id IS NOT NULL AND status IN ('confirmed','auto_resolved')`)
	if err != nil {
		return nil, err
	}
	for r2.Next() {
		var bid, mid int64
		var raw string
		if err := r2.Scan(&bid, &mid, &raw); err != nil {
			r2.Close()
			return nil, err
		}
		out[bid] = append(out[bid], candidate{text: raw, modelID: mid})
	}
	r2.Close()
	return out, r2.Err()
}

func upsertAlias(ctx context.Context, tx pgx.Tx, brandID int64, raw string, modelID *int64, status string, conf *float64, reasoning string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO model_aliases
		  (brand_id, raw, raw_normalized, raw_normalized_nospace, model_id, status, confidence, method, reasoning)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'fuzzy',$8)
		ON CONFLICT (brand_id, raw) DO UPDATE
		   SET model_id=EXCLUDED.model_id, status=EXCLUDED.status,
		       confidence=EXCLUDED.confidence, method=EXCLUDED.method,
		       reasoning=EXCLUDED.reasoning
		 WHERE model_aliases.status NOT IN ('confirmed','rejected')`,
		brandID, raw, domain.Normalize(raw), domain.NormalizeNoSpace(raw),
		modelID, status, conf, reasoning)
	return err
}

func logChange(ctx context.Context, tx pgx.Tx, entity string, id int64, action string, volumeImpact int) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO change_log (entity_type, entity_id, action, actor_kind, volume_impact)
		 VALUES ($1,$2,$3,'human',$4)`, entity, id, action, volumeImpact)
	return err
}
