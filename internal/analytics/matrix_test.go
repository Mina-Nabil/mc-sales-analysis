package analytics

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Every value the filter picker offers has to be filterable. The picker is fed
// by Values(), which returns DIMENSION values, so a filter must compare against
// the dimension expression. When the two were separate maps they drifted: the
// model dimension rendered "Peugeot 408" while the model filter compared against
// m.name ("408"), and every COALESCE'd bucket ('Unknown', 'No distributor') was
// unreachable because the filter expressions dropped the COALESCE.
//
// This is the round trip: take a dimension's own top value, filter by it, and
// require that value back.
func TestEveryDimensionValueIsFilterable(t *testing.T) {
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

	for _, dim := range AvailableDimensions() {
		t.Run(dim, func(t *testing.T) {
			buckets, err := Aggregate(ctx, pool, dim, nil, 0)
			if err != nil {
				t.Fatalf("Aggregate(%s): %v", dim, err)
			}
			if len(buckets) == 0 {
				t.Skipf("no data for dimension %q", dim)
			}
			for _, b := range buckets {
				if b.Volume == 0 {
					continue
				}
				filtered, err := Aggregate(ctx, pool, dim, map[string][]string{dim: {b.Key}}, 0)
				if err != nil {
					t.Fatalf("Aggregate(%s, filter %s=%q): %v", dim, dim, b.Key, err)
				}
				if len(filtered) == 0 {
					t.Fatalf("filtering %s=%q returned nothing, but the picker offers that value", dim, b.Key)
				}
				var got int64
				for _, fb := range filtered {
					if fb.Key == b.Key {
						got = fb.Volume
					}
				}
				if got != b.Volume {
					t.Errorf("filtering %s=%q gave %d units, dimension reports %d", dim, b.Key, got, b.Volume)
				}
				return // one representative value per dimension is enough
			}
		})
	}
}

// The specific regression: the model dimension is brand-qualified, so the model
// filter must be too.
func TestModelFilterUsesTheBrandQualifiedName(t *testing.T) {
	expr, ok := filterExpr("model")
	if !ok {
		t.Fatal("model is not a filterable key")
	}
	if expr != dimensions["model"] {
		t.Errorf("model filter expression %q != model dimension expression %q", expr, dimensions["model"])
	}
	// and the one deliberate non-dimension filter still resolves
	if e, ok := filterExpr("model_id"); !ok || e != "f.model_id::text" {
		t.Errorf("model_id filter = %q (%v), want f.model_id::text", e, ok)
	}
	if _, ok := filterExpr("nonsense"); ok {
		t.Error("unknown filter keys must not resolve")
	}
}
