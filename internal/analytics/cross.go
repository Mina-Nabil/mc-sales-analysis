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
// it has; dimension 2 is trimmed per group (that group's top N by volume unless
// the caller names values) with the remainder folded into "Others" rather than
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
// then summed across years). values2 pins the dimension-2 rows globally; when
// empty each dimension-1 group keeps its own top topN2 values by volume.
// rowLimit caps dimension 1 (its tail also folds into "Others") purely to
// protect the browser — it is deliberately high.
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
	total1 := map[string]int64{}
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
		res.Months[mo-1] += v
		res.Grand += v
	}
	if err := rows.Err(); err != nil {
		return res, err
	}

	keys1 := rankedKeys(total1, dim1)
	if len(keys1) > rowLimit {
		keys1 = keys1[:rowLimit]
	}
	in1 := map[string]bool{}
	for _, k := range keys1 {
		in1[k] = true
	}

	// Fold the dimension-1 tail into "Others" first, keeping k2 intact, so a
	// leftover brand still contributes to the Others group rather than vanishing.
	merged := map[cell]*[12]int64{}
	add := func(k1, k2 string, m *[12]int64) {
		fc := cell{k1, k2}
		acc, ok := merged[fc]
		if !ok {
			acc = &[12]int64{}
			merged[fc] = acc
		}
		for i, v := range m {
			acc[i] += v
		}
	}
	for c, m := range months {
		k1 := c.k1
		if !in1[k1] {
			k1 = othersKey
		}
		add(k1, c.k2, m)
	}
	if !in1[othersKey] {
		for c := range merged {
			if c.k1 == othersKey {
				keys1 = append(keys1, othersKey)
				in1[othersKey] = true
				break
			}
		}
	}

	// Dimension-2 keep set: pinned values are global; otherwise each dim1 group
	// keeps its own top N. A global top-N made Brand × Model collapse every
	// brand that didn't own one of the four biggest models worldwide into
	// Unknown + Others.
	keep2 := map[string]map[string]bool{} // k1 → k2
	if len(values2) > 0 {
		pin := map[string]bool{}
		for _, v := range values2 {
			pin[v] = true
		}
		for _, k1 := range keys1 {
			keep2[k1] = pin
		}
	} else {
		vol := map[string]map[string]int64{}
		for c, m := range merged {
			var s int64
			for _, v := range m {
				s += v
			}
			if vol[c.k1] == nil {
				vol[c.k1] = map[string]int64{}
			}
			vol[c.k1][c.k2] += s
		}
		for _, k1 := range keys1 {
			ranked := rankedKeys(vol[k1], dim2)
			keep := map[string]bool{}
			for i, k := range ranked {
				if i >= topN2 {
					break
				}
				keep[k] = true
			}
			keep2[k1] = keep
		}
	}

	folded := map[cell]*[12]int64{}
	kept2 := map[string]int64{}
	for c, m := range merged {
		k2 := c.k2
		if keep := keep2[c.k1]; keep == nil || !keep[k2] {
			k2 = othersKey
		}
		fc := cell{c.k1, k2}
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
