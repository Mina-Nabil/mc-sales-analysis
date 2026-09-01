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
	"strconv"
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
	"car_type":     "COALESCE(m.car_type,'Unknown')",
	"engine":       "COALESCE(m.engine_type,'Unknown')", // model-level spec (§2.8→tree)
	"origin":       "COALESCE(b.origin,'Unknown')",
	"supply":       "COALESCE(m.supply,'Unknown')", // model-level spec (§2.8→tree)
	"region":       "COALESCE(rg.name,'Unknown')",
	"governorate":  "COALESCE(g.name,'Unknown')",
	"traffic_unit": "COALESCE(tu.name,'Unknown')",
	"distributor":  "COALESCE(d.name,'No distributor')",
	// Fact-level, not JOIN-derived: model year is observed source data, part of
	// the row's own identity like volume, so §2.5 still holds.
	"model_year": "COALESCE(f.model_year::text,'Unknown')",
}

// chronological dimensions read badly ranked by volume — order them by key.
var orderedByKey = map[string]bool{"model_year": true}

// extraFilters are filter keys that are NOT dimensions. Everything else filters
// on the dimension expression itself — see filterExpr.
var extraFilters = map[string]string{
	// model_id is the unambiguous, comma-safe way to pin one exact model; it also
	// hits the facts(model_id, period_year, period_month) index.
	"model_id": "f.model_id::text",
}

// filterExpr returns the SQL a filter key compares against.
//
// It is deliberately the DIMENSION expression, not a hand-written twin. The
// filter picker is populated by Values(), which returns dimension values, so a
// filter must compare against whatever produced them. Maintaining a second map
// meant the two silently drifted: the model dimension renders "Peugeot 408"
// (b.name||' '||m.name) while the model filter compared against m.name ("408"),
// so every value the picker offered matched nothing. The same drift made every
// COALESCE'd bucket — 'Unknown', 'No distributor' — impossible to filter on,
// because the filter expressions dropped the COALESCE.
func filterExpr(key string) (string, bool) {
	if expr, ok := extraFilters[key]; ok {
		return expr, true
	}
	expr, ok := dimensions[key]
	return expr, ok
}

// notExcluded keeps facts a reviewer marked out of scope out of every measure.
// They are never deleted (§8.1) — the rows stay countable and the excluded total
// is reported separately — but they must not enter any share or growth figure.
const notExcluded = "f.status <> 'rejected'"

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
	joins := joinBlock(needsDist(p.Dimension, p.Filters))

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
	where := []string{"f.period_year IN ($1,$2)", notExcluded}
	add := func(expr string, vals any) int {
		args = append(args, vals)
		return len(args)
	}
	for key, vals := range p.Filters {
		if expr, ok := filterExpr(key); ok && len(vals) > 0 {
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
		// A brand with no prior volume has no prior rank: every zero ties, so a
		// position among them would be arbitrary (and reads as a huge climb).
		if prior > 0 {
			r.RankPrior = rankPrior[i]
		}
		recs[i].row = r
	}

	if orderedByKey[p.Dimension] {
		// numeric-aware, with the 'Unknown' bucket pinned to the end
		sort.SliceStable(recs, func(a, b int) bool {
			ka, oka := keyAsInt(recs[a].row.Key)
			kb, okb := keyAsInt(recs[b].row.Key)
			if oka != okb {
				return oka // a real year/age sorts before 'Unknown'
			}
			if !oka {
				return recs[a].row.Key < recs[b].row.Key
			}
			return ka < kb
		})
	} else {
		sort.SliceStable(recs, func(a, b int) bool { return recs[a].row.Total > recs[b].row.Total })
	}
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

// needsDist reports whether the distributor LATERAL join is required.
func needsDist(dimension string, f map[string][]string) bool {
	if dimension == "distributor" {
		return true
	}
	_, ok := f["distributor"]
	return ok
}

// joinBlock is the standard set of LEFT JOINs that make every dimension and
// filter expression resolvable; the distributor LATERAL is added only when needed.
func joinBlock(withDist bool) string {
	j := `
		LEFT JOIN brands b        ON b.id = f.brand_id
		LEFT JOIN models m        ON m.id = f.model_id
		LEFT JOIN segments seg    ON seg.id = m.segment_id
		LEFT JOIN governorates g  ON g.id = f.governorate_id
		LEFT JOIN regions rg      ON rg.id = g.region_id
		LEFT JOIN traffic_units tu ON tu.id = f.traffic_unit_id`
	if withDist {
		j += `
		LEFT JOIN LATERAL (
			SELECT dd.name FROM distributor_assignments da
			  JOIN distributors dd ON dd.id = da.distributor_id
			 WHERE da.brand_id = f.brand_id AND da.car_type = m.car_type
			   AND da.valid_from <= make_date(f.period_year, f.period_month, 1)
			   AND (da.valid_to IS NULL OR da.valid_to > make_date(f.period_year, f.period_month, 1))
			 LIMIT 1
		) d ON true`
	}
	return j
}

// filterWhere builds "expr = ANY($n)" conditions from p.Filters, appending to args.
func filterWhere(f map[string][]string, args *[]any) []string {
	var where []string
	for key, vals := range f {
		if expr, ok := filterExpr(key); ok && len(vals) > 0 {
			*args = append(*args, vals)
			where = append(where, fmt.Sprintf("%s = ANY($%d)", expr, len(*args)))
		}
	}
	return where
}

// Values returns the distinct values of a dimension (for filter option lists),
// respecting any active filters, capped at 1000.
func Values(ctx context.Context, pool *pgxpool.Pool, dimension string, f map[string][]string) ([]string, error) {
	dimExpr, ok := dimensions[dimension]
	if !ok {
		return nil, fmt.Errorf("unknown dimension %q", dimension)
	}
	var args []any
	where := append([]string{notExcluded}, filterWhere(f, &args)...)
	whereSQL := "WHERE " + strings.Join(where, " AND ")
	q := fmt.Sprintf(`SELECT DISTINCT %s AS v FROM facts f %s %s ORDER BY v LIMIT 1000`,
		dimExpr, joinBlock(needsDist(dimension, f)), whereSQL)
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Bucket is one dimension value's total (for filtered, all-time-or-year charts).
type Bucket struct {
	Key    string `json:"key"`
	Volume int64  `json:"volume"`
}

// Aggregate returns SUM(volume) by dimension over the filtered set, for a single
// year (year>0) or all time (year==0), ranked descending.
func Aggregate(ctx context.Context, pool *pgxpool.Pool, dimension string, f map[string][]string, year int) ([]Bucket, error) {
	dimExpr, ok := dimensions[dimension]
	if !ok {
		return nil, fmt.Errorf("unknown dimension %q", dimension)
	}
	var args []any
	where := append([]string{notExcluded}, filterWhere(f, &args)...)
	if year > 0 {
		args = append(args, year)
		where = append(where, fmt.Sprintf("f.period_year = $%d", len(args)))
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")
	q := fmt.Sprintf(`SELECT %s AS k, COALESCE(sum(f.volume),0) AS v
		FROM facts f %s %s GROUP BY k ORDER BY v DESC LIMIT 200`,
		dimExpr, joinBlock(needsDist(dimension, f)), whereSQL)
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Bucket{}
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.Key, &b.Volume); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Point is one period's total in a time series.
type Point struct {
	Year   int   `json:"year"`
	Month  int   `json:"month"`
	Volume int64 `json:"volume"`
}

// Timeseries returns per-(year,month) totals for the filtered set, all periods
// (or a single year when p.Year>0 and p.Month==0 is not enough — callers pass
// year via filters). Ordered chronologically.
func Timeseries(ctx context.Context, pool *pgxpool.Pool, f map[string][]string, year int) ([]Point, error) {
	var args []any
	where := append([]string{notExcluded}, filterWhere(f, &args)...)
	if year > 0 {
		args = append(args, year)
		where = append(where, fmt.Sprintf("f.period_year = $%d", len(args)))
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")
	q := fmt.Sprintf(`
		SELECT f.period_year, f.period_month, COALESCE(sum(f.volume),0)
		  FROM facts f %s %s
		 GROUP BY f.period_year, f.period_month
		 ORDER BY f.period_year, f.period_month`,
		joinBlock(needsDist("", f)), whereSQL)
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Point{}
	for rows.Next() {
		var pt Point
		if err := rows.Scan(&pt.Year, &pt.Month, &pt.Volume); err != nil {
			return nil, err
		}
		out = append(out, pt)
	}
	return out, rows.Err()
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

// keyAsInt parses a dimension key that should be numeric ("2027", "-1"),
// reporting false for 'Unknown' and anything else non-numeric.
func keyAsInt(k string) (int, bool) {
	n, err := strconv.Atoi(k)
	return n, err == nil
}

func monthFilters() string {
	parts := make([]string, 12)
	for i := 0; i < 12; i++ {
		parts[i] = fmt.Sprintf("COALESCE(SUM(vol) FILTER (WHERE yr=$1 AND mo=%d),0) AS m%d", i+1, i+1)
	}
	return strings.Join(parts, ",\n		  ")
}
