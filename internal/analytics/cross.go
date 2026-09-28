package analytics

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Cross is the two-dimension monthly table behind the report pages:
//
//	Model | Model year | Jan | Feb | … | Total
//
// Rows are (dimension1, dimension2) pairs — dimension1 spans its group, and
// dimension2 is the nested breakdown inside it. Dimension 1 shows every value
// it has; dimension 2 is trimmed to a chosen set (top N by volume unless the
// caller names values) with the remainder folded into "Others" rather than
// dropped, so the table still reconciles with the period total (§8.1).
const othersKey = "Others"

// CrossRow is one (dimension1, dimension2) pair across the twelve months.
type CrossRow struct {
	Key1   string    `json:"key1"`
	Key2   string    `json:"key2"`
	Months [12]int64 `json:"months"`
	Total  int64     `json:"total"`
}

// CrossResult is the full table payload.
type CrossResult struct {
	Dimension1 string     `json:"dimension1"`
	Dimension2 string     `json:"dimension2"`
	Values2    []string   `json:"values2"` // the dimension-2 rows in display order
	Rows       []CrossRow `json:"rows"`    // grouped by Key1, ordered by group total
	Months     [12]int64  `json:"months"`
	Grand      int64      `json:"grand"`
}

// Cross runs the table for one year (year>0) or all time (year==0, months are
// then summed across years). values2 pins the dimension-2 rows; when empty the
// top topN2 values by volume are used. rowLimit caps dimension 1 (its tail also
// folds into "Others") purely to protect the browser — it is deliberately high.
func Cross(ctx context.Context, pool *pgxpool.Pool, dim1, dim2 string, f map[string][]string, year int, values2 []string, topN2, rowLimit int) (CrossResult, error) {
	res := CrossResult{Dimension1: dim1, Dimension2: dim2, Values2: []string{}, Rows: []CrossRow{}}
	expr1, ok := dimensions[dim1]
	if !ok {
		return res, fmt.Errorf("unknown dimension %q", dim1)
	}
	expr2, ok := dimensions[dim2]
	if !ok {
		return res, fmt.Errorf("unknown dimension %q", dim2)
	}
	if topN2 <= 0 || topN2 > 50 {
		topN2 = 4
	}
	if rowLimit <= 0 || rowLimit > 2000 {
		rowLimit = 500
	}

	var args []any
	where := append([]string{notExcluded}, filterWhere(f, &args)...)
	if year > 0 {
		args = append(args, year)
		where = append(where, fmt.Sprintf("f.period_year = $%d", len(args)))
	}
	joins := joinBlock(needsDist(dim1, f) || needsDist(dim2, f))
	q := fmt.Sprintf(`
		SELECT %s AS k1, %s AS k2, f.period_month AS mo, COALESCE(sum(f.volume),0) AS v
		  FROM facts f %s
		 WHERE %s
		 GROUP BY 1, 2, 3`,
		expr1, expr2, joins, strings.Join(where, " AND "))

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return res, fmt.Errorf("cross query: %w", err)
	}
	defer rows.Close()

	type cell struct{ k1, k2 string }
	months := map[cell]*[12]int64{}
	total1, total2 := map[string]int64{}, map[string]int64{}
	for rows.Next() {
		var k1, k2 string
		var mo int
		var v int64
		if err := rows.Scan(&k1, &k2, &mo, &v); err != nil {
			return res, err
		}
		if mo < 1 || mo > 12 {
			continue
		}
		m, ok := months[cell{k1, k2}]
		if !ok {
			m = &[12]int64{}
			months[cell{k1, k2}] = m
		}
		m[mo-1] += v
		total1[k1] += v
		total2[k2] += v
		res.Months[mo-1] += v
		res.Grand += v
	}
	if err := rows.Err(); err != nil {
		return res, err
	}

	// Which dimension-2 values get their own row; everything else is "Others".
	keep := map[string]bool{}
	if len(values2) > 0 {
		for _, v := range values2 {
			if _, ok := total2[v]; ok {
				keep[v] = true
			}
		}
	} else {
		ranked := rankedKeys(total2, dim2)
		for i, k := range ranked {
			if i >= topN2 {
				break
			}
			keep[k] = true
		}
	}
	keys1 := rankedKeys(total1, dim1)
	if len(keys1) > rowLimit {
		keys1 = keys1[:rowLimit]
	}
	in1 := map[string]bool{}
	for _, k := range keys1 {
		in1[k] = true
	}

	// Re-bucket into the kept (k1, k2) pairs, folding both tails into "Others".
	folded := map[cell]*[12]int64{}
	kept2 := map[string]int64{}
	for c, m := range months {
		k1, k2 := c.k1, c.k2
		if !in1[k1] {
			k1 = othersKey
		}
		if !keep[k2] {
			k2 = othersKey
		}
		fc := cell{k1, k2}
		acc, ok := folded[fc]
		if !ok {
			acc = &[12]int64{}
			folded[fc] = acc
		}
		var sum int64
		for i, v := range m {
			acc[i] += v
			sum += v
		}
		kept2[k2] += sum
	}
	if !in1[othersKey] {
		// "Others" is a real group only when something was folded into it.
		if _, ok := total1[othersKey]; !ok {
			for c := range folded {
				if c.k1 == othersKey {
					keys1 = append(keys1, othersKey)
					break
				}
			}
		}
	}
	res.Values2 = rankedKeys(kept2, dim2)

	for _, k1 := range keys1 {
		for _, k2 := range res.Values2 {
			m, ok := folded[cell{k1, k2}]
			if !ok {
				continue // a pair with no volume at all is not worth a row
			}
			row := CrossRow{Key1: k1, Key2: k2, Months: *m}
			for _, v := range m {
				row.Total += v
			}
			res.Rows = append(res.Rows, row)
		}
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
				return ai > bi // newest model year first
			}
			if aok != bok {
				return aok // 'Unknown' sorts last
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
