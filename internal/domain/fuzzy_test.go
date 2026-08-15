package domain

import "testing"

func TestFuzzyTyposScoreHigh(t *testing.T) {
	// Genuine typos of the same model should score high (auto or review band).
	cases := [][2]string{
		{"كورلا", "كورولا"},    // Corolla missing a letter
		{"فرتشنر", "فورتشنر"},  // Fortuner typo
		{"سبورتاج", "سبورتاچ"}, // Sportage spelling variant
	}
	for _, c := range cases {
		if s := FuzzyScore(c[0], c[1]); s < 0.75 {
			t.Errorf("FuzzyScore(%q,%q)=%.2f, want ≥0.75", c[0], c[1], s)
		}
	}
}

func TestDigitGuard(t *testing.T) {
	if !DigitsDiffer("Tiggo 7", "Tiggo 8") {
		t.Error("Tiggo 7 vs Tiggo 8 must be flagged as digits-differ")
	}
	if !DigitsDiffer("X70", "X90") {
		t.Error("X70 vs X90 must be flagged as digits-differ")
	}
	if DigitsDiffer("كورلا", "كورولا") {
		t.Error("Corolla typo pair has no digits; must not be flagged")
	}
	// even if they score high, the guard must forbid auto-linking
	if CanAutoLink("Tiggo 7", "Tiggo 8", 0.95, 0.92) {
		t.Error("Tiggo 7/8 must never auto-link despite a high score")
	}
}

func TestVariantGuard(t *testing.T) {
	if !VariantDiffer("x70", "x70 plus") {
		t.Error("x70 vs x70 plus must be flagged as variant-differ")
	}
	if !VariantDiffer("Tiggo 7", "Tiggo 7 Pro") {
		t.Error("Tiggo 7 vs Tiggo 7 Pro must be flagged variant-differ")
	}
	if CanAutoLink("x70", "x70 plus", 0.99, 0.92) {
		t.Error("x70/x70 plus must never auto-link")
	}
	if VariantDiffer("كورلا", "كورولا") {
		t.Error("Corolla typo pair shares tokens; must not be variant-differ")
	}
}
