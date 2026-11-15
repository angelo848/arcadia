package domain

import (
	"strings"
	"unicode"
)

// articleWords are dropped from match keys so "The Cat" and "Cat" compare equal (MET-006).
var articleWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true,
}

// romanWords maps the roman numerals of two or more letters that are converted to numbers in
// match keys (MET-006). Single "i", "v" and "x" are kept as letters ("Mega Man X").
var romanWords = map[string]string{
	"ii": "2", "iii": "3", "iv": "4", "vi": "6", "vii": "7", "viii": "8", "ix": "9",
	"xi": "11", "xii": "12", "xiii": "13", "xiv": "14", "xv": "15", "xvi": "16",
	"xvii": "17", "xviii": "18", "xix": "19", "xx": "20",
}

// markExcluded are single letters left by apostrophes ("Link's" -> "link s"); they distinguish
// nothing and are not marks (MET-007).
var markExcluded = map[string]bool{
	"s": true, "t": true, "d": true, "m": true,
}

// SearchTerm returns the text sent to the providers for a title: region and tag groups in
// parentheses or brackets, trademark signs and extra spaces are removed, and the first " - "
// becomes ": " (MET-004). It may return an empty string; the caller treats that as an error.
func SearchTerm(title string) string {
	s := stripGroups(title, '(', ')')
	s = stripGroups(s, '[', ']')
	var b strings.Builder
	for _, r := range s {
		if r == '™' || r == '®' || r == '©' {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	s = strings.Join(strings.Fields(b.String()), " ")
	return strings.Replace(s, " - ", ": ", 1)
}

// MatchKey returns the comparison key of a title: NormalizeTitle, without articles and "and",
// with roman numerals of two or more letters turned into numbers (MET-006).
func MatchKey(title string) string {
	words := strings.Fields(NormalizeTitle(title))
	kept := make([]string, 0, len(words))
	for _, w := range words {
		if articleWords[w] {
			continue
		}
		if n, ok := romanWords[w]; ok {
			w = n
		}
		kept = append(kept, w)
	}
	return strings.Join(kept, " ")
}

// Marks returns, in order, the terms of a match key that tell games of the same series apart:
// digits-only terms and single letters, except the apostrophe leftovers s, t, d and m (MET-007).
func Marks(key string) []string {
	marks := []string{}
	for _, w := range strings.Fields(key) {
		if markExcluded[w] {
			continue
		}
		if len(w) == 1 && (unicode.IsLetter(rune(w[0])) || unicode.IsDigit(rune(w[0]))) {
			marks = append(marks, w)
			continue
		}
		if isAllDigits(w) {
			marks = append(marks, w)
		}
	}
	return marks
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
