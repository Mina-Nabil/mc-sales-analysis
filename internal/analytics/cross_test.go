package analytics

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The pivot folds everything past each axis's cut into "Others" instead of
// dropping it, so its grand total must equal the unfiltered period total — the
// same "never clean totals" property the migration holds to (§8.1). Row and
// column totals must also agree with the cells.
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
			res, err := Cross(ctx, pool, pair[0], pair[1], nil, 0, 5, 4)
			if err != nil {
				t.Fatalf("Cross(%s, %s): %v", pair[0], pair[1], err)
			}
			if res.Grand != total {
				t.Errorf("grand total %d, want %d (Others must absorb the tail)", res.Grand, total)
			}
			var summed int64
			for _, row := range res.Rows {
				var rt int64
				for _, v := range row.Cells {
					rt += v
				}
				if rt != row.Total {
					t.Errorf("row %q cells sum to %d, row total says %d", row.Key, rt, row.Total)
				}
				summed += rt
			}
			if summed != res.Grand {
				t.Errorf("cells sum to %d, grand is %d", summed, res.Grand)
			}
			var colSum int64
			for _, v := range res.ColTotals {
				colSum += v
			}
			if colSum != res.Grand {
				t.Errorf("column totals sum to %d, grand is %d", colSum, res.Grand)
			}
		})
	}
}
