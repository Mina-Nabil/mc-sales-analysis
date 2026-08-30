package review

import (
	"context"
	"os"
	"testing"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Exclude takes a review item's volume out of the measures without deleting a
// single row: facts move to 'rejected' (which analytics filters out) and stay
// countable for reconciliation (§0.1, §8.1).
func TestExcludeMarksFactsWithoutDeleting(t *testing.T) {
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

	const scratch = "ZZ Exclude Test"
	cleanup := func() {
		pool.Exec(ctx, `DELETE FROM change_log WHERE entity_type='model_alias' AND entity_id IN
			(SELECT id FROM model_aliases WHERE brand_id IN (SELECT id FROM brands WHERE name=$1))`, scratch)
		pool.Exec(ctx, `DELETE FROM facts WHERE raw_brand=$1`, scratch)
		pool.Exec(ctx, `DELETE FROM model_aliases WHERE brand_id IN (SELECT id FROM brands WHERE name=$1)`, scratch)
		pool.Exec(ctx, `DELETE FROM brands WHERE name=$1`, scratch)
		pool.Exec(ctx, `DELETE FROM import_batches WHERE period_year=2099`)
	}
	cleanup()
	defer cleanup()

	var brandID, batchID, aliasID int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO brands (name, status) VALUES ($1,'confirmed') RETURNING id`, scratch).Scan(&brandID))
	must(pool.QueryRow(ctx, `INSERT INTO import_batches (feed_role, period_year, period_month, state)
		 VALUES ('primary', 2099, 3, 'committed') RETURNING id`).Scan(&batchID))

	must(func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO facts (raw_governorate, raw_unit, raw_brand, raw_model,
			                   period_year, period_month, volume, brand_id,
			                   import_batch_id, status)
			VALUES ('g','u',$1,'SCRAP',2099,3,42,$2,$3,'needs_review')`, scratch, brandID, batchID)
		return err
	}())
	must(pool.QueryRow(ctx, `
		INSERT INTO model_aliases (brand_id, raw, raw_normalized, raw_normalized_nospace, status, method)
		VALUES ($1,'SCRAP',$2,$3,'needs_review','fuzzy') RETURNING id`,
		brandID, domain.Normalize("SCRAP"), domain.NormalizeNoSpace("SCRAP")).Scan(&aliasID))

	units, err := Exclude(ctx, pool, aliasID, "not a car")
	if err != nil {
		t.Fatalf("Exclude: %v", err)
	}
	if units != 42 {
		t.Errorf("expected 42 units excluded, got %d", units)
	}

	var rows, vol int
	var st string
	must(pool.QueryRow(ctx,
		`SELECT count(*), COALESCE(sum(volume),0), COALESCE(max(status::text),'')
		   FROM facts WHERE raw_brand=$1`, scratch).Scan(&rows, &vol, &st))
	if rows != 1 || vol != 42 {
		t.Errorf("facts were deleted or altered: rows=%d vol=%d (must stay countable)", rows, vol)
	}
	if st != "rejected" {
		t.Errorf("fact status = %q, want rejected", st)
	}

	// a second decision on the same item must be refused
	if _, err := Exclude(ctx, pool, aliasID, ""); err == nil {
		t.Error("Exclude on an already-decided item should fail")
	}
}
