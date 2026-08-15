// Package domain holds pure, I/O-free logic: normalization and the resolution
// engine. Everything here must be exhaustively unit-testable (TECH §9).
package domain

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var (
	reLetterDigit = regexp.MustCompile(`(\p{L})\s(\p{Nd})`) // letter <sp> digit
	reDigitLetter = regexp.MustCompile(`(\p{Nd})\s(\p{L})`) // digit  <sp> letter
	sepReplacer   = strings.NewReplacer("-", "", "_", "", ".", "")
)

// core applies the Unicode/Arabic normalization common to both keys (TECH §3.2
// steps 1–9): NFKC, strip tatweel & diacritics, unify alef/yeh/teh/waw, convert
// Arabic-Indic & Eastern digits to ASCII, lowercase Latin, collapse whitespace.
func core(s string) string {
	s = norm.NFKC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		// step 2: drop tatweel and harakat/diacritics
		if r == 0x0640 || (r >= 0x064B && r <= 0x065F) || r == 0x0670 {
			continue
		}
		switch r {
		case 0x0623, 0x0625, 0x0622, 0x0671: // أ إ آ ٱ
			r = 0x0627 // ا  (step 3)
		case 0x0649, 0x0626: // ى ئ
			r = 0x064A // ي  (step 4)
		case 0x0629: // ة
			r = 0x0647 // ه  (step 5)
		case 0x0624: // ؤ
			r = 0x0648 // و  (step 6)
		}
		switch { // step 7: digits → ASCII
		case r >= 0x0660 && r <= 0x0669:
			r = '0' + (r - 0x0660)
		case r >= 0x06F0 && r <= 0x06F9:
			r = '0' + (r - 0x06F0)
		}
		b.WriteRune(r)
	}
	s = strings.ToLower(b.String())             // step 8 (Arabic is caseless)
	return strings.Join(strings.Fields(s), " ") // step 9: collapse ws + trim
}

// Normalize produces the primary tier-2 key (TECH §3.2, through step 10):
// removes -, _, . and any space sitting on a letter⇄digit boundary, so
// "ZX 7", "ZX-7", "ZX7", "T- 2" all collapse to "zx7" / "t2".
func Normalize(s string) string {
	s = core(s)
	s = sepReplacer.Replace(s)
	s = reLetterDigit.ReplaceAllString(s, "$1$2")
	s = reDigitLetter.ReplaceAllString(s, "$1$2")
	return s
}

// NormalizeNoSpace is the tier-2b key (TECH §3.2 step 11): the primary key with
// all remaining whitespace removed, unifying intra-word Arabic spacing like
// "فور تشنر" / "فورتشنر". Tested to add zero ambiguity over the historical data.
func NormalizeNoSpace(s string) string {
	return strings.ReplaceAll(Normalize(s), " ", "")
}
