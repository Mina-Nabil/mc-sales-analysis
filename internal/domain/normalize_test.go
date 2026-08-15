package domain

import "testing"

// Table-driven tests for the normalization core. Includes every real collision
// called out in TECH §9 and the Phase 0 regression suite (9 must-merge,
// 5 must-stay-apart).
func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		// letter/digit spacing (rule 10)
		{"ZX7", "zx7"}, {"ZX 7", "zx7"}, {"ZX-7", "zx7"},
		{"T2", "t2"}, {"T- 2", "t2"},
		{"تيجو7", "تيجو7"}, {"تيجو 7", "تيجو7"},
		// engine vocabulary collisions
		{"ICE", "ice"}, {"ICe", "ice"}, {"Ice", "ice"},
		{"HYBRID", "hybrid"}, {"Hybrid", "hybrid"},
		// Arabic letter unification + diacritics + tatweel
		{"رينـو", "رينو"}, {"رينو", "رينو"},
		{"اودى", "اودي"}, {"اودي", "اودي"},
		// Arabic-Indic digits
		{"موديل ٧", "موديل7"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeMustMerge(t *testing.T) {
	// Pairs that must produce the SAME key (some only under the no-space key).
	merge := [][2]string{
		{"ZX7", "ZX 7"}, {"T2", "T- 2"}, {"تيجو7", "تيجو 7"},
		{"ICE", "ICe"}, {"HYBRID", "Hybrid"},
	}
	for _, p := range merge {
		if Normalize(p[0]) != Normalize(p[1]) {
			t.Errorf("Normalize: %q and %q should merge (%q vs %q)", p[0], p[1], Normalize(p[0]), Normalize(p[1]))
		}
	}
	// These merge only after whitespace stripping (rule 11).
	nospaceMerge := [][2]string{
		{"فور تشنر", "فورتشنر"}, {"لاند كروزر", "لاندكروزر"},
		{"hr v", "hrv"}, {"c s35", "cs35"}, {"d fsk", "dfsk"}, {"t f r", "tfr"},
	}
	for _, p := range nospaceMerge {
		if NormalizeNoSpace(p[0]) != NormalizeNoSpace(p[1]) {
			t.Errorf("NormalizeNoSpace: %q and %q should merge (%q vs %q)",
				p[0], p[1], NormalizeNoSpace(p[0]), NormalizeNoSpace(p[1]))
		}
	}
}

func TestNormalizeMustStayApart(t *testing.T) {
	// Different cars that must NOT collapse together, even under the no-space key.
	apart := [][2]string{
		{"X70", "X90"}, {"Tiggo 7", "Tiggo 8"},
		{"x70", "x70 plus"}, {"x70", "x70 fl"}, {"MG5", "MG6"},
	}
	for _, p := range apart {
		if NormalizeNoSpace(p[0]) == NormalizeNoSpace(p[1]) {
			t.Errorf("%q and %q must stay apart, both = %q", p[0], p[1], NormalizeNoSpace(p[0]))
		}
	}
}
