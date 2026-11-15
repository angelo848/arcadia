package importers

import (
	"strings"
)

// naturalLess compares relative paths segment by segment (separator \ or /), case-insensitively,
// with digit runs compared as numbers; ties fall back to the original runes [IMV-010].
func naturalLess(a, b string) bool {
	if c := naturalCompare(a, b); c != 0 {
		return c < 0
	}
	return a < b
}

// natToken is one run of a segment: a digit run (num) or a run of other bytes (text, lower case).
type natToken struct {
	num  bool
	text string // lower-cased text, or the raw digits of a digit run
}

// naturalCompare orders two paths; 0 means the same natural key (the raw strings may still differ).
func naturalCompare(a, b string) int {
	sa, sb := splitSegments(a), splitSegments(b)
	for i := 0; i < len(sa) && i < len(sb); i++ {
		if c := compareTokens(tokenize(sa[i]), tokenize(sb[i])); c != 0 {
			return c
		}
	}
	switch {
	case len(sa) < len(sb):
		return -1
	case len(sa) > len(sb):
		return 1
	}
	return 0
}

// splitSegments splits on \ and /.
func splitSegments(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == '\\' || r == '/' })
}

// tokenize splits a segment into alternating digit and non-digit runs. Text is lower-cased; digits are kept raw.
// Splitting happens on ASCII bytes only, so multi-byte runes are never cut.
func tokenize(s string) []natToken {
	var out []natToken
	start := 0
	for i := 0; i <= len(s); i++ {
		if i < len(s) && i > start && isDigitByte(s[i]) != isDigitByte(s[start]) {
			out = append(out, makeToken(s[start:i]))
			start = i
		}
		if i == len(s) && i > start {
			out = append(out, makeToken(s[start:i]))
		}
	}
	return out
}

func makeToken(run string) natToken {
	if isDigitByte(run[0]) {
		return natToken{num: true, text: run}
	}
	return natToken{text: strings.ToLower(run)}
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

// compareTokens compares two segments token by token; a prefix sorts first.
func compareTokens(a, b []natToken) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareToken(a[i], b[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// compareToken puts numbers before text. Numbers compare by value without parsing (no overflow): first the length
// without leading zeros, then the digits; then the shorter raw run comes first ("2" before "02").
func compareToken(a, b natToken) int {
	switch {
	case a.num && !b.num:
		return -1
	case !a.num && b.num:
		return 1
	case !a.num:
		return strings.Compare(a.text, b.text)
	}
	ta := strings.TrimLeft(a.text, "0")
	tb := strings.TrimLeft(b.text, "0")
	if len(ta) != len(tb) {
		if len(ta) < len(tb) {
			return -1
		}
		return 1
	}
	if c := strings.Compare(ta, tb); c != 0 {
		return c
	}
	switch {
	case len(a.text) < len(b.text):
		return -1
	case len(a.text) > len(b.text):
		return 1
	}
	return 0
}
