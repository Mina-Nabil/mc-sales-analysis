package analytics

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The table folds the dimension-2 tail into "Others" instead of dropping it, so
// its grand total must equal the unfiltered period total — the same "never
// clean totals" property the migration holds to (§8.1). Row totals, month
// totals and the grand total must all agree.
func TestCrossReconcilesWithTheFilteredTotal(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://mc:mc@localhost:5433/mcsales?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil || pool.Ping(ctx) != nil {
		t.Skip("no database reachable — skipping integration test")
	}
	defer pool.Close()

	total, err := seriesTotal(ctx, pool, nil)
	if err != nil {
		t.Fatalf("Timeseries: %v", err)
	}
	if total == 0 {
		t.Skip("no facts loaded")
	}

	for _, pair := range [][2]string{
		{"brand", "region"},
		{"model", "traffic_unit"},
		{"segment", "model_year"},
		{"distributor", "governorate"},
	} {
		t.Run(pair[0]+"×"+pair[1], func(t *testing.T) {
			// A deliberately tight cut, so "Others" is actually exercised.
			res, err := Cross(ctx, pool, pair[0], pair[1], nil, 0, nil, 4, 5)
			if err != nil {
				t.Fatalf("Cross(%s, %s): %v", pair[0], pair[1], err)
			}
			if res.Grand != total {
				t.Errorf("grand total %d, want %d (Others must absorb the tail)", res.Grand, total)
			}
			var summed int64
			for _, row := range res.Rows {
				var rt int64
				for _, v := range row.Months {
					rt += v
				}
				if rt != row.Total {
					t.Errorf("row %q/%q months sum to %d, row total says %d", row.Key1, row.Key2, rt, row.Total)
				}
				summed += rt
			}
			if summed != res.Grand {
				t.Errorf("rows sum to %d, grand is %d", summed, res.Grand)
			}
			var monthSum int64
			for _, v := range res.Months {
				monthSum += v
			}
			if monthSum != res.Grand {
				t.Errorf("month totals sum to %d, grand is %d", monthSum, res.Grand)
			}
			// Dimension 2 must be trimmed to the requested top-N plus "Others";
			// dimension 1 keeps every value it has.
			if len(res.Values2) > 5 {
				t.Errorf("dimension 2 has %d rows, want at most top-4 + Others", len(res.Values2))
			}
			groups := map[string]bool{}
			for _, row := range res.Rows {
				groups[row.Key1] = true
			}
			all, err := Aggregate(ctx, pool, pair[0], nil, 0)
			if err != nil {
				t.Fatalf("Aggregate(%s): %v", pair[0], err)
			}
			for _, b := range all {
				if b.Volume > 0 && !groups[b.Key] && !groups[othersKey] {
					t.Errorf("dimension 1 value %q is missing from the table", b.Key)
					break
				}
			}
		})
	}
}
