package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

// The private-plate twin shares the by-model-year header signature exactly, so
// detection has to reject it on the title — and on the NORMALIZED title, since
// these sheets carry tatweel elongation.
func TestRejectPrivatePlateVariant(t *testing.T) {
	allVehicles := "تقرير بماركات وطرازات المركبات المرخصة لأول مرة (الزيرو) وفقاً لتوزيعها الجغرافي علي مستوى الجمهورية ووفقاً لسنة الصنع عن الفترة من 2026/03/01 وحتى 2026/03/31"
	private := "تقرير بماركات وطرازات المركبات الملاكي المرخصة لأول مرة (الزيرو) وفقاً لتوزيعها الجغرافي علي مستوى الجمهورية ووفقاً لسنة الصنع عن الفترة من 2026/07/01 وحتى 2026/07/31"

	if err := rejectPrivatePlateVariant(allVehicles); err != nil {
		t.Errorf("all-vehicles report was rejected: %v", err)
	}
	if err := rejectPrivatePlateVariant(private); err == nil {
		t.Error("private-plate report was accepted — it covers only part of the market")
	}
	// tatweel in the middle of the phrase must not defeat the check
	if err := rejectPrivatePlateVariant("تقرير المركبات الملاكــي المرخصة"); err == nil {
		t.Error("tatweel-elongated private-plate title was accepted")
	}
}

// Parse the real March 2026 report: the month missing from the by-status feeds.
func TestParseModelYearFeed(t *testing.T) {
	path := filepath.Join("..", "..", "feeds-year", "2026-03.xlsx")
	if _, err := os.Stat(path); err != nil {
		t.Skip("feeds-year/2026-03.xlsx not present — run scripts/extract-feeds.py")
	}
	pf, err := DetectAndParse(path)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if pf.Signature != "brands_models_by_year" {
		t.Errorf("signature = %q, want brands_models_by_year", pf.Signature)
	}
	if pf.Year != 2026 || pf.Month != 3 {
		t.Errorf("period = %d-%02d, want 2026-03", pf.Year, pf.Month)
	}
	want := []int{2023, 2024, 2025, 2026, 2027}
	if len(pf.ModelYears) != len(want) {
		t.Fatalf("model years = %v, want %v", pf.ModelYears, want)
	}
	for i, y := range want {
		if pf.ModelYears[i] != y {
			t.Fatalf("model years = %v, want %v", pf.ModelYears, want)
		}
	}
	// the sheet's own grand-total row, and the reconciliation target
	if pf.TotalVolume != 58655 {
		t.Errorf("total volume = %d, want 58655 (the sheet's الإجمالي العام row)", pf.TotalVolume)
	}
	// every emitted row must carry a year, since no row in this file is short
	for _, r := range pf.Rows {
		if r.ModelYear == 0 {
			t.Fatalf("row %+v has no model year, but this file's rows all reconcile", r)
			break
		}
		if r.Volume <= 0 {
			t.Fatalf("row %+v has non-positive volume", r)
			break
		}
	}
	t.Logf("parsed %d facts from %d source rows, %d units", len(pf.Rows), 13065, pf.TotalVolume)
}

// The by-status feed must still parse, with no model year.
func TestParseByStatusFeedStillWorks(t *testing.T) {
	path := filepath.Join("..", "..", "feeds", "2026-07.xlsx")
	if _, err := os.Stat(path); err != nil {
		t.Skip("feeds/2026-07.xlsx not present")
	}
	pf, err := DetectAndParse(path)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if pf.Signature != "brands_models_by_status" {
		t.Errorf("signature = %q, want brands_models_by_status", pf.Signature)
	}
	if len(pf.ModelYears) != 0 {
		t.Errorf("by-status feed reported model years %v", pf.ModelYears)
	}
	if pf.TotalVolume != 63219 {
		t.Errorf("Zero-column total = %d, want 63219", pf.TotalVolume)
	}
	for _, r := range pf.Rows {
		if r.ModelYear != 0 {
			t.Fatalf("by-status row carried a model year: %+v", r)
			break
		}
	}
}

// Both feeds describe the same measure, so the by-model-year report must sum to
// the by-status feed's Zero column, key for key.
func TestFeedsReconcileExactly(t *testing.T) {
	for _, mm := range []string{"01", "02", "04", "05", "06", "07"} {
		statusPath := filepath.Join("..", "..", "feeds", "2026-"+mm+".xlsx")
		yearPath := filepath.Join("..", "..", "feeds-year", "2026-"+mm+".xlsx")
		if _, err := os.Stat(statusPath); err != nil {
			t.Skip("feeds/ not present")
		}
		if _, err := os.Stat(yearPath); err != nil {
			t.Skip("feeds-year/ not present")
		}
		st, err := DetectAndParse(statusPath)
		if err != nil {
			t.Fatalf("2026-%s status: %v", mm, err)
		}
		yr, err := DetectAndParse(yearPath)
		if err != nil {
			t.Fatalf("2026-%s year: %v", mm, err)
		}
		if st.TotalVolume != yr.TotalVolume {
			t.Errorf("2026-%s totals differ: status=%d year=%d", mm, st.TotalVolume, yr.TotalVolume)
		}
		type key struct{ g, u, b, m string }
		agg := func(rows []FeedRow) map[key]int {
			out := map[key]int{}
			for _, r := range rows {
				out[key{r.RawGov, r.RawUnit, r.RawBrand, r.RawModel}] += r.Volume
			}
			return out
		}
		a, b := agg(st.Rows), agg(yr.Rows)
		var mismatched int
		for k, v := range b {
			if a[k] != v {
				mismatched++
			}
		}
		for k, v := range a {
			if v != 0 && b[k] != v {
				mismatched++
			}
		}
		if mismatched != 0 {
			t.Errorf("2026-%s: %d keys disagree between the two feeds", mm, mismatched)
		}
	}
}
