package analytics

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Cross is the two-dimension pivot behind the report pages' 2-dimension table:
// rows are one dimension's values, columns another's, cells are SUM(volume).
//
// Both axes are trimmed to their top values by volume and everything past the
// cut is folded into an "Others" bucket rather than dropped, so the grand total
// still equals the filtered period total (§8.1 — never clean totals).
const othersKey = "Others"

// CrossRow is one row of the pivot: cells align with CrossResult.Cols.
type CrossRow struct {
	Key   string  `json:"key"`
	Cells []int64 `json:"cells"`
	Total int64   `json:"total"`
}

// CrossResult is the full pivot payload.
type CrossResult struct {
	Dimension1 string     `json:"dimension1"`
	Dimension2 string     `json:"dimension2"`
	Cols       []string   `json:"cols"`
	Rows       []CrossRow `json:"rows"`
	ColTotals  []int64    `json:"col_totals"`
	Grand      int64      `json:"grand"`
}

// Cross runs the pivot for a single year (year>0) or all time (year==0).
// rowLimit/colLimit cap each axis; anything beyond is summed into "Others".
func Cross(ctx context.Context, pool *pgxpool.Pool, dim1, dim2 string, f map[string][]string, year, rowLimit, colLimit int) (CrossResult, error) {
	res := CrossResult{Dimension1: dim1, Dimension2: dim2, Cols: []string{}, Rows: []CrossRow{}, ColTotals: []int64{}}
	rExpr, ok := dimensions[dim1]
	if !ok {
		return res, fmt.Errorf("unknown dimension %q", dim1)
	}
	cExpr, ok := dimensions[dim2]
	if !ok {
		return res, fmt.Errorf("unknown dimension %q", dim2)
	}
	if rowLimit <= 0 || rowLimit > 300 {
		rowLimit = 60
	}
	if colLimit <= 0 || colLimit > 120 {
		colLimit = 30
	}

	var args []any
	where := append([]string{notExcluded}, filterWhere(f, &args)...)
	if year > 0 {
		args = append(args, year)
		where = append(where, fmt.Sprintf("f.period_year = $%d", len(args)))
	}
	args = append(args, rowLimit)
	rLim := len(args)
	args = append(args, colLimit)
	cLim := len(args)

	joins := joinBlock(needsDist(dim1, f) || needsDist(dim2, f))
	q := fmt.Sprintf(`
		WITH base AS (
			SELECT %s AS r, %s AS c, f.volume AS v
			  FROM facts f %s
			 WHERE %s
		),
		rt AS (SELECT r FROM base GROUP BY r ORDER BY sum(v) DESC LIMIT $%d),
		ct AS (SELECT c FROM base GROUP BY c ORDER BY sum(v) DESC LIMIT $%d)
		SELECT CASE WHEN r IN (SELECT r FROM rt) THEN r ELSE $%d END AS r,
		       CASE WHEN c IN (SELECT c FROM ct) THEN c ELSE $%d END AS c,
		       COALESCE(sum(v),0) AS v
		  FROM base GROUP BY 1, 2`,
		rExpr, cExpr, joins, strings.Join(where, " AND "), rLim, cLim, len(args)+1, len(args)+2)
	args = append(args, othersKey, othersKey)

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return res, fmt.Errorf("cross query: %w", err)
	}
	defer rows.Close()

	type cell struct{ r, c string }
	cells := map[cell]int64{}
	rowTotal, colTotal := map[string]int64{}, map[string]int64{}
	for rows.Next() {
		var r, c string
		var v int64
		if err := rows.Scan(&r, &c, &v); err != nil {
			return res, err
		}
		cells[cell{r, c}] += v
		rowTotal[r] += v
		colTotal[c] += v
		res.Grand += v
	}
	if err := rows.Err(); err != nil {
		return res, err
	}

	res.Cols = rankedKeys(colTotal, dim2)
	for _, k := range rankedKeys(rowTotal, dim1) {
		row := CrossRow{Key: k, Cells: make([]int64, len(res.Cols)), Total: rowTotal[k]}
		for i, c := range res.Cols {
			row.Cells[i] = cells[cell{k, c}]
		}
		res.Rows = append(res.Rows, row)
	}
	res.ColTotals = make([]int64, len(res.Cols))
	for i, c := range res.Cols {
		res.ColTotals[i] = colTotal[c]
	}
	return res, nil
}

// rankedKeys orders an axis by volume (chronological dimensions by key instead,
// like the matrix does for model_year), always pushing "Others" to the end.
func rankedKeys(totals map[string]int64, dim string) []string {
	keys := make([]string, 0, len(totals))
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if (a == othersKey) != (b == othersKey) {
			return b == othersKey
		}
		if orderedByKey[dim] {
			ai, aok := keyAsInt(a)
			bi, bok := keyAsInt(b)
			if aok && bok {
				return ai < bi
			}
			if aok != bok {
				return aok // 'Unknown' and friends sort last
			}
			return a < b
		}
		if totals[a] != totals[b] {
			return totals[a] > totals[b]
		}
		return a < b
	})
	return keys
}
