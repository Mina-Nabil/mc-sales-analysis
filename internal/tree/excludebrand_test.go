package tree

import (
	"strings"
	"testing"
)

// ExcludeBrand is the brand-queue twin of review.Exclude: the volume leaves the
// measures but nothing is deleted, so period totals still reconcile (§8.1).
func TestExcludeBrand(t *testing.T) {
	ctx, pool, brandID := scratchBrand(t, scratchPrefix+" Exclude")
	_ = brandID

	const raw = "ZZ EditBrand unresolved raw"
	var batchID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO import_batches (source_filename, state, period_year, period_month)
		VALUES ('zz-exclude-brand-test','committed',2024,5) RETURNING id`).Scan(&batchID); err != nil {
		t.Fatalf("import_batch: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM facts WHERE import_batch_id=$1`, batchID)
		pool.Exec(ctx, `DELETE FROM import_batches WHERE id=$1`, batchID)
		pool.Exec(ctx, `DELETE FROM brand_aliases WHERE raw=$1`, raw)
	})
	// two brand-unresolved facts, exactly what the brand queue surfaces
	for _, vol := range []int{40, 60} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO facts (raw_governorate, raw_unit, raw_brand, raw_model,
			                   period_year, period_month, volume, import_batch_id, status)
			VALUES ('zz','zz',$1,'zz-model',2024,5,$2,$3,'unresolved')`, raw, vol, batchID); err != nil {
			t.Fatalf("insert fact: %v", err)
		}
	}

	inQueue := func() *BrandQueueItem {
		t.Helper()
		items, err := BrandQueue(ctx, pool, 500)
		if err != nil {
			t.Fatalf("BrandQueue: %v", err)
		}
		for i := range items {
			if items[i].RawBrand == raw {
				return &items[i]
			}
		}
		return nil
	}

	it := inQueue()
	if it == nil || it.Volume != 100 || it.Rows != 2 {
		t.Fatalf("expected the raw brand in the queue with 100 units / 2 rows, got %+v", it)
	}

	units, err := ExcludeBrand(ctx, pool, raw, "motorcycle brand, not a car", 0)
	if err != nil {
		t.Fatalf("ExcludeBrand: %v", err)
	}
	if units != 100 {
		t.Errorf("expected 100 units excluded, got %d", units)
	}
	if it := inQueue(); it != nil {
		t.Errorf("raw brand should have left the queue, still present: %+v", it)
	}

	// Nothing deleted — the rows stay on record, only their status moved (§8.1).
	var rows, vol int
	if err := pool.QueryRow(ctx,
		`SELECT count(*), COALESCE(sum(volume),0) FROM facts WHERE raw_brand=$1`, raw).Scan(&rows, &vol); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || vol != 100 {
		t.Errorf("facts must survive intact: %d rows / %d units", rows, vol)
	}
	var rejected int
	pool.QueryRow(ctx,
		`SELECT count(*) FROM facts WHERE raw_brand=$1 AND status='rejected'`, raw).Scan(&rejected)
	if rejected != 2 {
		t.Errorf("expected both facts rejected, got %d", rejected)
	}

	// The alias records the decision but must stay inert for resolution: the
	// resolver only loads brand_aliases WHERE brand_id IS NOT NULL.
	var status, reasoning string
	var aliasBrand *int64
	if err := pool.QueryRow(ctx,
		`SELECT status::text, COALESCE(reasoning,''), brand_id FROM brand_aliases WHERE raw=$1`, raw).
		Scan(&status, &reasoning, &aliasBrand); err != nil {
		t.Fatalf("expected a brand_alias record: %v", err)
	}
	if status != "rejected" || aliasBrand != nil {
		t.Errorf("alias should be rejected with a NULL brand, got %q / %v", status, aliasBrand)
	}
	if !strings.Contains(reasoning, "motorcycle brand") {
		t.Errorf("reason not recorded: %q", reasoning)
	}

	// Excluding twice is not a silent no-op.
	if _, err := ExcludeBrand(ctx, pool, raw, "", 0); err == nil {
		t.Error("re-excluding with nothing left should report that")
	}

	// New volume under the same raw string must come back for a decision rather
	// than being silently rejected (§0.1 counted & reported).
	if _, err := pool.Exec(ctx, `
		INSERT INTO facts (raw_governorate, raw_unit, raw_brand, raw_model,
		                   period_year, period_month, volume, import_batch_id, status)
		VALUES ('zz','zz',$1,'zz-model',2024,6,7,$2,'unresolved')`, raw, batchID); err != nil {
		t.Fatal(err)
	}
	if it := inQueue(); it == nil || it.Volume != 7 {
		t.Errorf("new unresolved volume should reappear in the queue, got %+v", it)
	}

	var logged int
	pool.QueryRow(ctx,
		`SELECT count(*) FROM change_log WHERE action='exclude_brand' AND volume_impact=100`).Scan(&logged)
	if logged == 0 {
		t.Error("exclusion must be logged with its unit impact")
	}
}
