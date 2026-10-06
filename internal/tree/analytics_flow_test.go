package tree

import (
	"strconv"
	"testing"
	"time"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/analytics"
)

// The point of managing distributor_assignments rather than storing a distributor
// on a brand, a model or a fact: the analytics SQL needs no change at all. The
// (brand, car_type) LATERAL in analytics.joinBlock already resolves whatever rows
// exist, per fact period, so an assignment written here is live immediately —
// nothing is re-derived and nothing is propagated (§2.5).
//
// This builds a scratch brand with real facts, then watches the distributor
// dimension follow the assignment: no distributor → one distributor → a handover
// mid-history that splits the units → back to none.
func TestAssignmentFlowsIntoAnalytics(t *testing.T) {
	ctx, pool, brandID := scratchBrand(t, scratchPrefix+" Analytics")

	modelID, _, err := CreateModel(ctx, pool, brandID, "scratch car", "Passenger", "", "", nil, 0)
	if err != nil {
		t.Fatalf("CreateModel: %v", err)
	}

	// One fact in 2021 and one in 2024, so a handover between them is visible.
	var batchID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO import_batches (source_filename, state, period_year, period_month)
		VALUES ('zz-assignment-test', 'committed', 2021, 6) RETURNING id`).Scan(&batchID); err != nil {
		t.Fatalf("import_batch: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM facts WHERE import_batch_id=$1`, batchID)
		pool.Exec(ctx, `DELETE FROM import_batches WHERE id=$1`, batchID)
	})
	for _, f := range []struct{ year, month, vol int }{{2021, 6, 70}, {2024, 6, 30}} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO facts (raw_governorate, raw_unit, raw_brand, raw_model,
			                   period_year, period_month, volume,
			                   brand_id, model_id, import_batch_id, status)
			VALUES ('zz','zz','zz-brand','zz-model',$1,$2,$3,$4,$5,$6,'confirmed')`,
			f.year, f.month, f.vol, brandID, modelID, batchID); err != nil {
			t.Fatalf("insert fact: %v", err)
		}
	}

	// units by distributor for this brand only, using the real dimension.
	byDistributor := func() map[string]int64 {
		t.Helper()
		buckets, err := analytics.Aggregate(ctx, pool, "distributor",
			map[string][]string{"model_id": {itoa(modelID)}}, 0)
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		out := map[string]int64{}
		for _, b := range buckets {
			out[b.Key] = b.Volume
		}
		return out
	}

	// 1. no assignment → the COALESCE bucket, with every unit accounted for
	if got := byDistributor(); got["No distributor"] != 100 {
		t.Fatalf("expected 100 units under 'No distributor', got %v", got)
	}

	d1, _, err := CreateDistributor(ctx, pool, scratchPrefix+" Analytics Motors", 0)
	if err != nil {
		t.Fatal(err)
	}
	d2, _, err := CreateDistributor(ctx, pool, scratchPrefix+" Analytics Trading", 0)
	if err != nil {
		t.Fatal(err)
	}
	day := func(s string) time.Time { tm, _ := time.Parse("2006-01-02", s); return tm }

	// 2. one open-ended assignment → all units move, with no re-derivation step
	if err := SetAssignment(ctx, pool, brandID, "Passenger", d1, day("2021-01-01"), nil, 0); err != nil {
		t.Fatalf("SetAssignment: %v", err)
	}
	got := byDistributor()
	if got[scratchPrefix+" Analytics Motors"] != 100 || got["No distributor"] != 0 {
		t.Fatalf("assignment did not reach analytics: %v", got)
	}

	// 3. a handover in 2023 splits history at the fact's own period — the whole
	//    reason the assignment is effective-dated instead of a plain column.
	if err := SetAssignment(ctx, pool, brandID, "Passenger", d2, day("2023-01-01"), nil, 0); err != nil {
		t.Fatalf("handover: %v", err)
	}
	got = byDistributor()
	if got[scratchPrefix+" Analytics Motors"] != 70 || got[scratchPrefix+" Analytics Trading"] != 30 {
		t.Fatalf("handover did not split by period: %v", got)
	}
	if total := got[scratchPrefix+" Analytics Motors"] + got[scratchPrefix+" Analytics Trading"] + got["No distributor"]; total != 100 {
		t.Errorf("units must still reconcile to 100, got %d (%v)", total, got)
	}

	// 4. removing every assignment returns the units to 'No distributor' (§2.4 —
	//    absence of a row, never a sentinel)
	items, err := BrandAssignments(ctx, pool, brandID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range items {
		if _, err := DeleteAssignment(ctx, pool, a.ID, 0); err != nil {
			t.Fatalf("DeleteAssignment: %v", err)
		}
	}
	if got := byDistributor(); got["No distributor"] != 100 {
		t.Fatalf("expected 100 back under 'No distributor', got %v", got)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
