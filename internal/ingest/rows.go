package ingest

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AnnotatedRow is one parsed feed row exactly as the sheet carries it, plus how
// it resolved against the car tree — this is what the dry-run preview shows.
type AnnotatedRow struct {
	Gov   string `json:"gov"`
	Unit  string `json:"unit"`
	Brand string `json:"brand"`
	Model string `json:"model"`
	// ModelYear is 0 when the feed carried none (rendered as "—").
	ModelYear int `json:"model_year"`
	Volume    int `json:"volume"`
	// resolved | new_model | new_brand | motorcycle
	Status     string `json:"status"`
	CanonBrand string `json:"canon_brand,omitempty"`
	CanonModel string `json:"canon_model,omitempty"`
}

// AnnotateRows runs the same resolution ladder as DryRun, but keeps the
// per-row outcome instead of only the totals.
func AnnotateRows(ctx context.Context, pool *pgxpool.Pool, pf *ParsedFeed) ([]AnnotatedRow, error) {
	res, err := loadResolver(ctx, pool)
	if err != nil {
		return nil, err
	}
	exclude := settingBool(ctx, pool, "ingest.exclude_motorcycles", true)

	brandNames := map[int64]string{}
	modelNames := map[int64]string{}
	if rows, err := pool.Query(ctx, `SELECT id, name FROM brands`); err == nil {
		for rows.Next() {
			var id int64
			var n string
			if rows.Scan(&id, &n) == nil {
				brandNames[id] = n
			}
		}
		rows.Close()
	}
	if rows, err := pool.Query(ctx, `SELECT id, name FROM models`); err == nil {
		for rows.Next() {
			var id int64
			var n string
			if rows.Scan(&id, &n) == nil {
				modelNames[id] = n
			}
		}
		rows.Close()
	}

	out := make([]AnnotatedRow, 0, len(pf.Rows))
	for _, row := range pf.Rows {
		a := AnnotatedRow{
			Gov: row.RawGov, Unit: row.RawUnit,
			Brand: row.RawBrand, Model: row.RawModel,
			ModelYear: row.ModelYear, Volume: row.Volume,
		}
		if exclude && res.isMotorcycle(row.RawBrand, row.RawModel) {
			a.Status = "motorcycle"
			out = append(out, a)
			continue
		}
		bid, _ := res.brandT(row.RawBrand)
		if bid == 0 {
			a.Status = "new_brand"
			out = append(out, a)
			continue
		}
		a.CanonBrand = brandNames[bid]
		mid, _ := res.modelT(bid, row.RawModel)
		if mid == 0 {
			a.Status = "new_model"
			out = append(out, a)
			continue
		}
		a.CanonModel = modelNames[mid]
		a.Status = "resolved"
		out = append(out, a)
	}
	return out, nil
}
