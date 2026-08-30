package tree

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://mc:mc@localhost:5433/mcsales?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil || pool.Ping(context.Background()) != nil {
		t.Skip("no database reachable — skipping integration test")
	}
	return pool
}

// CreateModel's INSERT listed five columns against six value expressions, so
// every "new model" decision — from the review queue and from the Tree page —
// failed at the database. This is the regression test.
func TestCreateModelInsertsSuccessfully(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	defer pool.Close()

	const scratch = "ZZ CreateModel Test"
	cleanup := func() {
		pool.Exec(ctx, `DELETE FROM change_log WHERE entity_type='model' AND entity_id IN
			(SELECT id FROM models WHERE brand_id IN (SELECT id FROM brands WHERE name=$1))`, scratch)
		pool.Exec(ctx, `DELETE FROM models WHERE brand_id IN (SELECT id FROM brands WHERE name=$1)`, scratch)
		pool.Exec(ctx, `DELETE FROM change_log WHERE entity_type='brand' AND entity_id IN
			(SELECT id FROM brands WHERE name=$1)`, scratch)
		pool.Exec(ctx, `DELETE FROM brands WHERE name=$1`, scratch)
	}
	cleanup()
	defer cleanup()

	brandID, _, err := CreateBrand(ctx, pool, scratch, "China", nil, 0)
	if err != nil {
		t.Fatalf("CreateBrand: %v", err)
	}

	// with an optional segment omitted (the common review-queue case)
	id, name, err := CreateModel(ctx, pool, brandID, "tiggo 8 pro", "Passenger", nil, 0)
	if err != nil {
		t.Fatalf("CreateModel: %v", err)
	}
	if id == 0 {
		t.Fatal("CreateModel returned id 0")
	}
	if name != "tiggo 8 pro" { // mixed case is left as typed by §0.1
		t.Errorf("casing changed unexpectedly: %q", name)
	}

	// and the ≤4-letter ALL CAPS rule still applies
	if _, n2, err := CreateModel(ctx, pool, brandID, "mg5", "Passenger", nil, 0); err != nil {
		t.Fatalf("CreateModel short name: %v", err)
	} else if n2 != "MG5" {
		t.Errorf("expected MG5, got %q", n2)
	}

	var carType string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(car_type,'') FROM models WHERE id=$1`, id).Scan(&carType); err != nil {
		t.Fatal(err)
	}
	if carType != "Passenger" {
		t.Errorf("car_type not stored: %q", carType)
	}
}
