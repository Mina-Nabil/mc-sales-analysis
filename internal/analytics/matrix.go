// Package analytics implements the one parameterized matrix query that powers
// every dashboard (TECH §5.1): rows = a dimension, columns = Jan..Dec, plus
// total, share %, YTD, YoY growth, share-point delta, and rank vs a prior year.
// The measure is always SUM(facts.volume); every dimension is JOIN-derived, so
// nothing is stored on the fact (§2.5).
package analytics

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// dimension → SQL expression over the standard join set. COALESCE keeps NULLs
// (unresolved facts, no-distributor brands) as visible buckets so totals
// reconcile (§5.4).
var dimensions = map[string]string{
	"brand":        "COALESCE(b.name,'Unknown')",
	"model":        "CASE WHEN m.id IS NULL THEN 'Unknown' ELSE b.name||' '||m.name END",
	"segment":      "COALESCE(seg.name,'Unknown')",
	"tier":         "COALESCE(m.tier,'Unknown')",
	"car_type":     "COALESCE(m.car_type,'Unknown')",
	"engine":       "COALESCE(f.engine_type,'Unknown')",
	"origin":       "COALESCE(b.origin,'Unknown')",
	"supply":       "COALESCE(f.supply,'Unknown')",
	"region":       "COALESCE(rg.name,'Unknown')",
	"governorate":  "COALESCE(g.name,'Unknown')",
	"traffic_unit": "COALESCE(tu.name,'Unknown')",
	"distributor":  "COALESCE(d.name,'No distributor')",
}

// filter key → SQL expression compared with = ANY($n).
var filters = map[string]string{
	"brand":        "b.name",
	"model":        "m.name",
	"segment":      "seg.name",
	"tier":         "m.tier",
	"car_type":     "m.car_type",
	"engine":       "f.engine_type",
	"origin":       "b.origin",
	"supply":       "f.supply",
	"region":       "rg.name",
	"governorate":  "g.name",
	"traffic_unit": "tu.name",
	"distributor":  "d.name",
}

// Params configures a matrix query. The primary period is (Year, Month) and the
// comparison period is (CompareYear, CompareMonth); Month/CompareMonth are 0 for
// a whole year or 1..12 for a single month, so the matrix can compare year-over-
// year, month-over-month, or any two periods.
type Params struct {
	Dimension    string
	Year         int
	Month        int // 0 = whole year
	CompareYear  int
	CompareMonth int // 0 = whole year (or mirror the primary month, see below)
	Filters      map[string][]string
	Limit        int
}

// Row is one dimension value with its full measure set.
type Row struct {
	Key             string    `json:"key"`
	Months          [12]int64 `json:"months"`
	Total           int64     `json:"total"`
	SharePct        float64   `json:"share_pct"`
	YTDCurrent      int64     `json:"ytd_current"`
	YTDPrior        int64     `json:"ytd_prior"`
	GrowthPct       *float64  `json:"growth_pct"` // null = "new" (no prior volume)
	SharePctPrior   float64   `json:"share_pct_prior"`
	SharePointDelta float64   `json:"share_point_delta"`
	Rank            int       `json:"rank"`
	RankPrior       int       `json:"rank_prior"`
}

// Result is the full matrix payload.
type Result struct {
	Dimension        string `json:"dimension"`
	Year             int    `json:"year"`
	CompareYear      int    `json:"compare_year"`
	Denominator      int64  `json:"denominator"` // grand total for Year (filter context)
	DenominatorPrior int64  `json:"denominator_prior"`
	Rows             []Row  `json:"rows"`
}

// AvailableDimensions lists the valid dimension keys.
func AvailableDimensions() []string {
	out := make([]string, 0, len(dimensions))
	for k := range dimensions {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Matrix runs the parameterized query and computes shares, growth, and ranks.
func Matrix(ctx context.Context, pool *pgxpool.Pool, p Params) (Result, error) {
	res := Result{Dimension: p.Dimension, Year: p.Year, CompareYear: p.CompareYear}
	dimExpr, ok := dimensions[p.Dimension]
	if !ok {
		return res, fmt.Errorf("unknown dimension %q", p.Dimension)
	}
	needDist := p.Dimension == "distributor"
	if _, ok := p.Filters["distributor"]; ok {
		needDist = true
	}

	joins := `
		LEFT JOIN brands b        ON b.id = f.brand_id
		LEFT JOIN models m        ON m.id = f.model_id
		LEFT JOIN segments seg    ON seg.id = m.segment_id
		LEFT JOIN governorates g  ON g.id = f.governorate_id
		LEFT JOIN regions rg      ON rg.id = g.region_id
		LEFT JOIN traffic_units tu ON tu.id = f.traffic_unit_id`
	if needDist {
		joins += `
		LEFT JOIN LATERAL (
			SELECT dd.name FROM distributor_assignments da
			  JOIN distributors dd ON dd.id = da.distributor_id
			 WHERE da.brand_id = f.brand_id AND da.car_type = m.car_type
			   AND da.valid_from <= make_date(f.period_year, f.period_month, 1)
			   AND (da.valid_to IS NULL OR da.valid_to > make_date(f.period_year, f.period_month, 1))
			 LIMIT 1
		) d ON true`
	}

	// Resolve the two comparison windows as month sets:
	//   S1 = primary period's months, S2 = compare period's months.
	// A single month → [that month]; a whole year → the months actually present
	// in that year (so a partial current year compares fairly). If the compare
	// month is unset but the primary is a single month, mirror it into the
	// compare year (e.g. "Jul 2026 vs 2025" ⇒ Jul-over-Jul).
	s1 := monthSet(ctx, pool, p.Year, p.Month)
	var s2 []int
	if p.CompareMonth > 0 {
		s2 = []int{p.CompareMonth}
	} else if p.Month > 0 {
		s2 = []int{p.Month}
	} else {
		s2 = s1
	}

	args := []any{p.Year, p.CompareYear}
	where := []string{"f.period_year IN ($1,$2)"}
	add := func(expr string, vals any) int {
		args = append(args, vals)
		return len(args)
	}
	for key, vals := range p.Filters {
		if expr, ok := filters[key]; ok && len(vals) > 0 {
			where = append(where, fmt.Sprintf("%s = ANY($%d)", expr, add(expr, vals)))
		}
	}
	s1Idx := add("", s1)
	s2Idx := add("", s2)

	q := fmt.Sprintf(`
		WITH base AS (
			SELECT %s AS key, f.period_year AS yr, f.period_month AS mo, f.volume AS vol
			  FROM facts f %s
			 WHERE %s
		)
		SELECT key,
		  %s,
		  COALESCE(SUM(vol) FILTER (WHERE yr=$1 AND mo = ANY($%d::int[])),0) AS total_primary,
		  COALESCE(SUM(vol) FILTER (WHERE yr=$2 AND mo = ANY($%d::int[])),0) AS total_prior
		FROM base GROUP BY key`,
		dimExpr, joins, strings.Join(where, " AND "), monthFilters(), s1Idx, s2Idx)

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return res, fmt.Errorf("matrix query: %w", err)
	}
	defer rows.Close()

	type rec struct {
		row        Row
		totalPrior int64
	}
	var recs []rec
	for rows.Next() {
		var r Row
		var totalPrior int64
		dst := []any{&r.Key}
		for i := range r.Months {
			dst = append(dst, &r.Months[i])
		}
		dst = append(dst, &r.Total, &totalPrior)
		if err := rows.Scan(dst...); err != nil {
			return res, err
		}
		r.YTDCurrent, r.YTDPrior = r.Total, totalPrior // period totals (aligned windows)
		recs = append(recs, rec{r, totalPrior})
	}
	if err := rows.Err(); err != nil {
		return res, err
	}

	// grand totals for share denominators (filter context)
	for _, x := range recs {
		res.Denominator += x.row.Total
		res.DenominatorPrior += x.totalPrior
	}
	den := float64(res.Denominator)
	denP := float64(res.DenominatorPrior)

	// ranks: current by Total, prior by totalPrior
	byCur := make([]int, len(recs))
	byPrior := make([]int, len(recs))
	for i := range recs {
		byCur[i], byPrior[i] = i, i
	}
	sort.SliceStable(byCur, func(a, b int) bool { return recs[byCur[a]].row.Total > recs[byCur[b]].row.Total })
	sort.SliceStable(byPrior, func(a, b int) bool { return recs[byPrior[a]].totalPrior > recs[byPrior[b]].totalPrior })
	rankCur := make([]int, len(recs))
	rankPrior := make([]int, len(recs))
	for pos, idx := range byCur {
		rankCur[idx] = pos + 1
	}
	for pos, idx := range byPrior {
		rankPrior[idx] = pos + 1
	}

	for i := range recs {
		r := recs[i].row
		prior := recs[i].totalPrior
		if den > 0 {
			r.SharePct = float64(r.Total) / den * 100
		}
		if denP > 0 {
			r.SharePctPrior = float64(prior) / denP * 100
		}
		r.SharePointDelta = r.SharePct - r.SharePctPrior
		// The two windows are aligned (same month count), so growth is a direct
		// period-over-period comparison (§5.2).
		if prior > 0 {
			g := (float64(r.Total) - float64(prior)) / float64(prior) * 100
			r.GrowthPct = &g
		} // else nil → "new"
		r.Rank = rankCur[i]
		r.RankPrior = rankPrior[i]
		recs[i].row = r
	}

	sort.SliceStable(recs, func(a, b int) bool { return recs[a].row.Total > recs[b].row.Total })
	limit := p.Limit
	if limit <= 0 || limit > len(recs) {
		limit = len(recs)
	}
	res.Rows = make([]Row, 0, limit)
	for i := 0; i < limit; i++ {
		res.Rows = append(res.Rows, recs[i].row)
	}
	return res, nil
}

// monthSet returns [month] for a single month, or the months actually present
// in the year (fallback: all 12) for a whole-year window.
func monthSet(ctx context.Context, pool *pgxpool.Pool, year, month int) []int {
	if month > 0 {
		return []int{month}
	}
	rows, err := pool.Query(ctx,
		`SELECT DISTINCT period_month FROM facts WHERE period_year=$1 ORDER BY period_month`, year)
	if err != nil {
		return allMonths()
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var m int
		if rows.Scan(&m) == nil {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return allMonths()
	}
	return out
}

func allMonths() []int { return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12} }

func monthFilters() string {
	parts := make([]string, 12)
	for i := 0; i < 12; i++ {
		parts[i] = fmt.Sprintf("COALESCE(SUM(vol) FILTER (WHERE yr=$1 AND mo=%d),0) AS m%d", i+1, i+1)
	}
	return strings.Join(parts, ",\n		  ")
}
