package ingest

import (
	"context"
	"os"
	"testing"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Normalize must collapse the sibling spellings that make the replay necessary:
// a reviewer confirms one, and the other has to resolve without a second review.
func TestSiblingSpellingsShareAKey(t *testing.T) {
	for _, pair := range [][2]string{
		{"IX 35", "IX35"},
		{"H 2", "H2"},
		{"NKR-12 E", "NKR-12E"},
		{"LML 150", "lml150"},
	} {
		if a, b := domain.Normalize(pair[0]), domain.Normalize(pair[1]); a != b {
			t.Errorf("Normalize(%q)=%q != Normalize(%q)=%q", pair[0], a, pair[1], b)
		}
	}
}

// TestReresolvePicksUpSiblingSpelling is the regression test for the gap the
// replay closes: confirming an alias for one raw spelling must also resolve the
// facts already stored under an equivalent spelling. Uses a scratch brand that
// is torn down afterwards; skipped when no database is reachable.
func TestReresolvePicksUpSiblingSpelling(t *testing.T) {
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

	const scratch = "ZZ Reresolve Test"
	cleanup := func() {
		pool.Exec(ctx, `DELETE FROM facts WHERE raw_brand=$1`, scratch)
		pool.Exec(ctx, `DELETE FROM model_aliases WHERE brand_id IN (SELECT id FROM brands WHERE name=$1)`, scratch)
		pool.Exec(ctx, `DELETE FROM brand_aliases WHERE raw=$1`, scratch)
		pool.Exec(ctx, `DELETE FROM models WHERE brand_id IN (SELECT id FROM brands WHERE name=$1)`, scratch)
		pool.Exec(ctx, `DELETE FROM brands WHERE name=$1`, scratch)
		pool.Exec(ctx, `DELETE FROM import_batches WHERE period_year=2099`)
	}
	cleanup()
	defer cleanup()

	var brandID, modelID, batchID int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx,
		`INSERT INTO brands (name, status) VALUES ($1,'confirmed') RETURNING id`, scratch).Scan(&brandID))
	must(pool.QueryRow(ctx,
		`INSERT INTO models (brand_id, name, status) VALUES ($1,'QQ7','confirmed') RETURNING id`, brandID).Scan(&modelID))
	must(pool.QueryRow(ctx,
		`INSERT INTO import_batches (feed_role, period_year, period_month, state)
		 VALUES ('primary', 2099, 1, 'committed') RETURNING id`).Scan(&batchID))

	// Two spellings of the same model, both unresolved.
	for _, raw := range []string{"QQ 7", "QQ7"} {
		must(func() error {
			_, err := pool.Exec(ctx, `
				INSERT INTO facts (raw_governorate, raw_unit, raw_brand, raw_model,
				                   period_year, period_month, volume, brand_id,
				                   import_batch_id, status)
				VALUES ('g','u',$1,$2,2099,1,10,$3,$4,'unresolved')`,
				scratch, raw, brandID, batchID)
			return err
		}())
	}

	// A reviewer confirms only "QQ7" — this is what Confirm/CreateModelForReview write.
	must(func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO model_aliases (brand_id, raw, raw_normalized, raw_normalized_nospace,
			                           model_id, status, method)
			VALUES ($1,'QQ7',$2,$3,$4,'confirmed','human')`,
			brandID, domain.Normalize("QQ7"), domain.NormalizeNoSpace("QQ7"), modelID)
		return err
	}())

	res, err := ReresolveUnresolved(ctx, pool)
	must(err)

	var linked int
	must(pool.QueryRow(ctx,
		`SELECT count(*) FROM facts WHERE raw_brand=$1 AND raw_model='QQ 7' AND model_id=$2`,
		scratch, modelID).Scan(&linked))
	if linked != 1 {
		t.Fatalf("sibling spelling %q was not linked by the replay (linked=%d, result=%+v)", "QQ 7", linked, res)
	}
	if res.ModelVolume < 10 {
		t.Errorf("expected at least the sibling's 10 units in ModelVolume, got %d", res.ModelVolume)
	}
}

// A rejected alias must keep its facts unresolved — the replay must not relink
// what a reviewer deliberately detached.
func TestReresolveSkipsRejectedPairs(t *testing.T) {
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

	const scratch = "ZZ Rejected Test"
	cleanup := func() {
		pool.Exec(ctx, `DELETE FROM facts WHERE raw_brand=$1`, scratch)
		pool.Exec(ctx, `DELETE FROM model_aliases WHERE brand_id IN (SELECT id FROM brands WHERE name=$1)`, scratch)
		pool.Exec(ctx, `DELETE FROM models WHERE brand_id IN (SELECT id FROM brands WHERE name=$1)`, scratch)
		pool.Exec(ctx, `DELETE FROM brands WHERE name=$1`, scratch)
		pool.Exec(ctx, `DELETE FROM import_batches WHERE period_year=2099`)
	}
	cleanup()
	defer cleanup()

	var brandID, modelID, batchID int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO brands (name, status) VALUES ($1,'confirmed') RETURNING id`, scratch).Scan(&brandID))
	must(pool.QueryRow(ctx, `INSERT INTO models (brand_id, name, status) VALUES ($1,'RR9','confirmed') RETURNING id`, brandID).Scan(&modelID))
	must(pool.QueryRow(ctx, `INSERT INTO import_batches (feed_role, period_year, period_month, state)
		 VALUES ('primary', 2099, 2, 'committed') RETURNING id`).Scan(&batchID))

	must(func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO facts (raw_governorate, raw_unit, raw_brand, raw_model,
			                   period_year, period_month, volume, brand_id,
			                   import_batch_id, status)
			VALUES ('g','u',$1,'RR 9',2099,2,7,$2,$3,'unresolved')`, scratch, brandID, batchID)
		return err
	}())
	// The alias exists but was rejected — and an equivalent confirmed alias also
	// exists, so without the guard the replay would happily relink it.
	must(func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO model_aliases (brand_id, raw, raw_normalized, raw_normalized_nospace, model_id, status, method)
			VALUES ($1,'RR 9',$2,$3,NULL,'rejected','human'),
			       ($1,'RR9',$4,$5,$6,'confirmed','human')`,
			brandID, domain.Normalize("RR 9"), domain.NormalizeNoSpace("RR 9"),
			domain.Normalize("RR9"), domain.NormalizeNoSpace("RR9"), modelID)
		return err
	}())

	if _, err := ReresolveUnresolved(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var stillNull int
	must(pool.QueryRow(ctx,
		`SELECT count(*) FROM facts WHERE raw_brand=$1 AND raw_model='RR 9' AND model_id IS NULL AND status='unresolved'`,
		scratch).Scan(&stillNull))
	if stillNull != 1 {
		t.Fatalf("replay relinked a rejected pair — it must stay unresolved")
	}
}
