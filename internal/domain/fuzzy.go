package domain

import (
	"regexp"
	"sort"
	"strings"
)

// Fuzzy matching for tier 3 (TECH §3.3). Two guards make it safe on this data:
//
//   - Digit guard: X70 vs X90, Tiggo 7 vs Tiggo 8 score high on edit distance but
//     are different cars. If the digit sequences differ, the pair may never
//     auto-link (DigitsDiffer → caller routes to review at most).
//   - Variant guard: x70 vs x70 Plus differ only by a variant token. The classic
//     token-set ratio returns 1.0 for such a subset — exactly the merge §3.3
//     forbids — so we use token-SORT ratio and additionally flag variant-token
//     differences (VariantDiffer) so they can never auto-link.

var digitRun = regexp.MustCompile(`\d+`)

// variant tokens that mark a distinct model, not a typo (§3.3).
var variantTokens = map[string]bool{
	"plus": true, "pro": true, "max": true, "fl": true, "l": true,
	"ev": true, "hev": true, "phev": true, "idm": true, "i-dm": true,
	"gt": true, "s": true, "x": true, "premium": true, "lux": true,
	"luxury": true, "sport": true, "turbo": true, "hybrid": true,
}

// FuzzyScore returns max(normalized-Levenshtein, token-sort) over the two
// strings' normalized forms. 0..1.
func FuzzyScore(a, b string) float64 {
	na, nb := Normalize(a), Normalize(b)
	r := levRatio([]rune(na), []rune(nb))
	if t := tokenSortRatio(na, nb); t > r {
		r = t
	}
	return r
}

// DigitsDiffer reports whether the ordered digit runs of the two strings differ.
func DigitsDiffer(a, b string) bool {
	return strings.Join(digitRun.FindAllString(Normalize(a), -1), " ") !=
		strings.Join(digitRun.FindAllString(Normalize(b), -1), " ")
}

// VariantDiffer reports whether the two strings' token sets differ by a variant
// token (plus/pro/max/…) — i.e. one names a variant the other does not.
func VariantDiffer(a, b string) bool {
	sa, sb := lightTokenSet(a), lightTokenSet(b)
	for t := range sa {
		if !sb[t] && variantTokens[t] {
			return true
		}
	}
	for t := range sb {
		if !sa[t] && variantTokens[t] {
			return true
		}
	}
	return false
}

// CanAutoLink reports whether a fuzzy pair is safe to auto-link at the given
// score: at/above threshold AND neither guard trips.
func CanAutoLink(a, b string, score, threshold float64) bool {
	return score >= threshold && !DigitsDiffer(a, b) && !VariantDiffer(a, b)
}

// lightTokenSet lowercases and splits on whitespace/-/_/. keeping tokens intact
// (unlike Normalize, which fuses letter⇄digit boundaries).
func lightTokenSet(s string) map[string]bool {
	s = strings.ToLower(s)
	s = strings.NewReplacer("-", " ", "_", " ", ".", " ", "/", " ").Replace(s)
	out := map[string]bool{}
	for _, t := range strings.Fields(s) {
		out[t] = true
	}
	return out
}

func tokenSortRatio(a, b string) float64 {
	sa, sb := strings.Fields(a), strings.Fields(b)
	sort.Strings(sa)
	sort.Strings(sb)
	return levRatio([]rune(strings.Join(sa, " ")), []rune(strings.Join(sb, " ")))
}

func levRatio(a, b []rune) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	d := levenshtein(a, b)
	m := len(a)
	if len(b) > m {
		m = len(b)
	}
	return 1 - float64(d)/float64(m)
}

func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
