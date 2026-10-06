package tree

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// scratchBrand creates an isolated brand and returns it with a cleanup func.
// scratchPrefix covers every brand/distributor these tests create, so cleanup
// also removes the extra brands a test makes beyond the one it is named for.
const scratchPrefix = "ZZ EditBrand"

func scratchBrand(t *testing.T, name string) (context.Context, *pgxpool.Pool, int64) {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	clean := func() {
		name := scratchPrefix
		pool.Exec(ctx, `DELETE FROM change_log WHERE entity_type='distributor_assignment' AND entity_id IN
			(SELECT id FROM distributor_assignments WHERE brand_id IN (SELECT id FROM brands WHERE name LIKE $1))`, name+"%")
		pool.Exec(ctx, `DELETE FROM distributor_assignments WHERE brand_id IN (SELECT id FROM brands WHERE name LIKE $1)`, name+"%")
		pool.Exec(ctx, `DELETE FROM change_log WHERE entity_type='model' AND entity_id IN
			(SELECT id FROM models WHERE brand_id IN (SELECT id FROM brands WHERE name LIKE $1))`, name+"%")
		pool.Exec(ctx, `DELETE FROM models WHERE brand_id IN (SELECT id FROM brands WHERE name LIKE $1)`, name+"%")
		pool.Exec(ctx, `DELETE FROM change_log WHERE entity_type='brand' AND entity_id IN
			(SELECT id FROM brands WHERE name LIKE $1)`, name+"%")
		pool.Exec(ctx, `UPDATE brands SET parent_brand_id=NULL WHERE name LIKE $1`, name+"%")
		pool.Exec(ctx, `DELETE FROM brands WHERE name LIKE $1`, name+"%")
		pool.Exec(ctx, `DELETE FROM change_log WHERE entity_type='distributor' AND entity_id IN
			(SELECT id FROM distributors WHERE name LIKE $1)`, name+"%")
		pool.Exec(ctx, `DELETE FROM distributors WHERE name LIKE $1`, name+"%")
	}
	clean()
	t.Cleanup(func() { clean(); pool.Close() })

	id, _, err := CreateBrand(ctx, pool, name, "China", nil, 0)
	if err != nil {
		t.Fatalf("CreateBrand: %v", err)
	}
	return ctx, pool, id
}

func TestEditBrand(t *testing.T) {
	ctx, pool, id := scratchBrand(t, "ZZ EditBrand Alpha")

	// origin and notes are set, then cleared — the point of full-replace semantics.
	if err := EditBrand(ctx, pool, id, "ZZ EditBrand Alpha", "Korea", "an imported note", nil, 0); err != nil {
		t.Fatalf("EditBrand: %v", err)
	}
	var origin, notes *string
	if err := pool.QueryRow(ctx, `SELECT origin, notes FROM brands WHERE id=$1`, id).Scan(&origin, &notes); err != nil {
		t.Fatal(err)
	}
	if origin == nil || *origin != "Korea" {
		t.Errorf("origin not updated: %v", origin)
	}
	if notes == nil || *notes != "an imported note" {
		t.Errorf("notes not updated: %v", notes)
	}

	if err := EditBrand(ctx, pool, id, "ZZ EditBrand Alpha", "", "", nil, 0); err != nil {
		t.Fatalf("EditBrand clearing: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT origin, notes FROM brands WHERE id=$1`, id).Scan(&origin, &notes); err != nil {
		t.Fatal(err)
	}
	if origin != nil || notes != nil {
		t.Errorf("origin/notes should be NULL, got %v / %v", origin, notes)
	}

	// an edit is logged with the brand's unit impact
	var logged int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM change_log WHERE entity_type='brand' AND entity_id=$1 AND action='edit'`,
		id).Scan(&logged); err != nil {
		t.Fatal(err)
	}
	if logged != 2 {
		t.Errorf("expected 2 edit log rows, got %d", logged)
	}

	if err := EditBrand(ctx, pool, id, "", "China", "", nil, 0); err == nil {
		t.Error("empty name should be rejected")
	}
	if err := EditBrand(ctx, pool, 0, "ZZ Missing", "", "", nil, 0); err == nil {
		t.Error("editing a missing brand should fail")
	}
}

func TestEditBrandNameConflict(t *testing.T) {
	ctx, pool, id := scratchBrand(t, "ZZ EditBrand Alpha")
	other, _, err := CreateBrand(ctx, pool, "ZZ EditBrand Beta", "Japan", nil, 0)
	if err != nil {
		t.Fatalf("CreateBrand other: %v", err)
	}
	_ = other
	if err := EditBrand(ctx, pool, id, "ZZ EditBrand Beta", "China", "", nil, 0); err == nil {
		t.Error("renaming onto an existing brand name should fail")
	} else if !strings.Contains(err.Error(), "already named") {
		t.Errorf("unhelpful error: %v", err)
	}
}

func TestEditBrandParentCycles(t *testing.T) {
	ctx, pool, a := scratchBrand(t, "ZZ EditBrand Alpha")
	b, _, err := CreateBrand(ctx, pool, "ZZ EditBrand Beta", "Japan", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := EditBrand(ctx, pool, a, "ZZ EditBrand Alpha", "China", "", &a, 0); err == nil {
		t.Error("self-parent should be rejected")
	}
	// A under B is fine…
	if err := EditBrand(ctx, pool, a, "ZZ EditBrand Alpha", "China", "", &b, 0); err != nil {
		t.Fatalf("setting a parent: %v", err)
	}
	// …but B under A would close the loop.
	if err := EditBrand(ctx, pool, b, "ZZ EditBrand Beta", "Japan", "", &a, 0); err == nil {
		t.Error("parent cycle should be rejected")
	}
	// detaching works
	if err := EditBrand(ctx, pool, a, "ZZ EditBrand Alpha", "China", "", nil, 0); err != nil {
		t.Fatalf("detaching parent: %v", err)
	}
	var parent *int64
	if err := pool.QueryRow(ctx, `SELECT parent_brand_id FROM brands WHERE id=$1`, a).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	if parent != nil {
		t.Errorf("parent should be NULL, got %v", *parent)
	}
}

// EditModel used to use COALESCE(NULLIF($n,”), col), so a wrongly-set spec could
// never be reset to NULL through the API.
func TestEditModelClearsFields(t *testing.T) {
	ctx, pool, brandID := scratchBrand(t, "ZZ EditBrand Alpha")
	id, _, err := CreateModel(ctx, pool, brandID, "scratch model", "Passenger", "HYBRID", "CKD", nil, 0)
	if err != nil {
		t.Fatalf("CreateModel: %v", err)
	}
	if err := EditModel(ctx, pool, id, "scratch model", "", "", "", nil, 0); err != nil {
		t.Fatalf("EditModel: %v", err)
	}
	var carType, engine, supply *string
	var seg *int64
	if err := pool.QueryRow(ctx,
		`SELECT car_type, engine_type, supply, segment_id FROM models WHERE id=$1`, id).
		Scan(&carType, &engine, &supply, &seg); err != nil {
		t.Fatal(err)
	}
	if carType != nil || engine != nil || supply != nil || seg != nil {
		t.Errorf("fields should all be NULL: %v %v %v %v", carType, engine, supply, seg)
	}
	if err := EditModel(ctx, pool, id, "", "", "", "", nil, 0); err == nil {
		t.Error("empty name should be rejected")
	}
}

func TestAssignments(t *testing.T) {
	ctx, pool, brandID := scratchBrand(t, "ZZ EditBrand Alpha")
	d1, _, err := CreateDistributor(ctx, pool, "ZZ EditBrand Alpha Motors", 0)
	if err != nil {
		t.Fatalf("CreateDistributor: %v", err)
	}
	d2, _, err := CreateDistributor(ctx, pool, "ZZ EditBrand Alpha Trading", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateDistributor(ctx, pool, "ZZ EditBrand Alpha Motors", 0); err == nil {
		t.Error("duplicate distributor name should fail")
	}

	day := func(s string) time.Time {
		tm, _ := time.Parse("2006-01-02", s)
		return tm
	}

	if err := SetAssignment(ctx, pool, brandID, "Passenger", d1, day("2021-02-01"), nil, 0); err != nil {
		t.Fatalf("first assignment: %v", err)
	}
	// A later start must close the open range instead of violating the EXCLUDE.
	if err := SetAssignment(ctx, pool, brandID, "Passenger", d2, day("2024-01-01"), nil, 0); err != nil {
		t.Fatalf("second assignment: %v", err)
	}
	items, err := BrandAssignments(ctx, pool, brandID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(items))
	}
	// newest range first
	if items[0].DistributorID != d2 || items[0].ValidTo != nil {
		t.Errorf("current assignment wrong: %+v", items[0])
	}
	if items[1].DistributorID != d1 || items[1].ValidTo == nil || !items[1].ValidTo.Equal(day("2024-01-01")) {
		t.Errorf("prior range should be closed at 2024-01-01: %+v", items[1])
	}

	// Same valid_from corrects in place rather than adding a row.
	if err := SetAssignment(ctx, pool, brandID, "Passenger", d1, day("2024-01-01"), nil, 0); err != nil {
		t.Fatalf("in-place correction: %v", err)
	}
	items, _ = BrandAssignments(ctx, pool, brandID)
	if len(items) != 2 {
		t.Fatalf("correction should not add a row, got %d", len(items))
	}
	if items[0].DistributorID != d1 {
		t.Errorf("correction did not take: %+v", items[0])
	}

	// Guards
	if err := SetAssignment(ctx, pool, brandID, "", d1, day("2025-01-01"), nil, 0); err == nil {
		t.Error("empty car type should be rejected")
	}
	if err := SetAssignment(ctx, pool, brandID, "Bus", 0, day("2025-01-01"), nil, 0); err == nil {
		t.Error("missing distributor should be rejected")
	}
	to := day("2020-01-01")
	if err := SetAssignment(ctx, pool, brandID, "Bus", d1, day("2025-01-01"), &to, 0); err == nil {
		t.Error("valid_to before valid_from should be rejected")
	}

	// Delete returns to 'No distributor' (§2.4 — absence, not a sentinel).
	if _, err := DeleteAssignment(ctx, pool, items[0].ID, 0); err != nil {
		t.Fatalf("DeleteAssignment: %v", err)
	}
	items, _ = BrandAssignments(ctx, pool, brandID)
	if len(items) != 1 {
		t.Errorf("expected 1 assignment after delete, got %d", len(items))
	}

	var logged int
	pool.QueryRow(ctx, `SELECT count(*) FROM change_log WHERE entity_type='distributor_assignment'
		AND action IN ('set','delete')`).Scan(&logged)
	if logged == 0 {
		t.Error("assignment mutations must write change_log")
	}
}
