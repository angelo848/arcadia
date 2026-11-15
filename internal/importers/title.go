package importers

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// Title rules of the folder and file sources (phase 11, import-rules.md §2 and §4). Pure: no clock, no disk.
var (
	reGroupParen   = regexp.MustCompile(`\([^()]*\)`)
	reGroupBracket = regexp.MustCompile(`\[[^\[\]]*\]`)
	reOrderPrefix  = regexp.MustCompile(`^\d{1,3}\.\s+(\D)`)
	// reArticle matches "<principal>, <article>" followed by the end, " - " or ": " [IMV-004].
	reArticle = regexp.MustCompile(`(?i)^(.+?),\s+(the|an|a|les|le|la|el|los|las|il|der|die|das)(\s+-\s+|:\s+|\s*$)`)
	// reStartsArticle matches a title that already starts with an article; such a title is not moved [IMV-004].
	reStartsArticle = regexp.MustCompile(`(?i)^(the|an|a|les|le|la|el|los|las|il|der|die|das)\s`)
	// reDisc matches the first disc tag "(Disc N)", "(Disk N)", "(CD N)", "(Disc N of M)" in () or [] [IMV-021].
	reDisc = regexp.MustCompile(`(?i)[\(\[]\s*(?:disc|disk|cd)\s*(\d{1,2})(?:\s*(?:of|de|/)\s*\d{1,2})?\s*[\)\]]`)
)

// SplitExt splits a file name into stem and extension [IMV-001]. The extension is the text after the last dot
// when it has 1..5 [A-Za-z0-9] characters with at least one letter; it is returned in lower case without the dot.
func SplitExt(name string) (stem, ext string) {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return name, ""
	}
	cand := name[i+1:]
	if len(cand) < 1 || len(cand) > 5 {
		return name, ""
	}
	hasLetter := false
	for j := 0; j < len(cand); j++ {
		c := cand[j]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			hasLetter = true
		case c >= '0' && c <= '9':
		default:
			return name, ""
		}
	}
	if !hasLetter {
		return name, ""
	}
	return name[:i], strings.ToLower(cand)
}

// NormalizeTitle returns the proposed title of a file stem or folder name [IMV-001..IMV-006, IMV-008].
// fromFolder enables the removal of the "NN. " ordering prefix [IMV-006]. It is pure: no clock, no disk.
// It returns "" only when stem is blank after trimming [IMV-005].
func NormalizeTitle(stem string, fromFolder bool) string {
	s := stem
	if fromFolder {
		s = reOrderPrefix.ReplaceAllString(s, "$1")
	}
	s = removeGroups(s)
	s = strings.ReplaceAll(s, "_", " ")
	// Decisions below depend on the text, so they are taken on the cleaned and cut text [IMV-008].
	s = cutRunes(s)
	if !strings.Contains(s, " ") {
		s = cleanSpaces(dotsToSpaces(s))
	}
	if !reStartsArticle.MatchString(s) {
		s = cleanSpaces(moveArticle(s))
	}
	if s == "" {
		// Only groups or punctuation: keep the raw stem with its spaces normalized [IMV-005].
		s = cutRunes(stem)
	}
	return s
}

// DiscNumber returns N of the first "(Disc N)", "(Disk N)", "(CD N)" or "(Disc N of M)" tag, in () or [], or 0 [IMV-021].
func DiscNumber(stem string) int {
	m := reDisc.FindStringSubmatch(stem)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// titleKey is the grouping key of a file: the normalized title in lower case [IMV-020, IMV-021].
func titleKey(stem string) string {
	return strings.ToLower(NormalizeTitle(stem, false))
}

// removeGroups removes (...) and [...] groups until none is left; each pass removes the innermost ones [IMV-002].
func removeGroups(s string) string {
	for range len(s) + 1 {
		next := reGroupBracket.ReplaceAllString(reGroupParen.ReplaceAllString(s, ""), "")
		if next == s {
			return s
		}
		s = next
	}
	return s
}

// dotsToSpaces turns each dot that is not between two digits into a space [IMV-003].
func dotsToSpaces(s string) string {
	rs := []rune(s)
	for i, r := range rs {
		if r != '.' {
			continue
		}
		if i > 0 && i+1 < len(rs) && isDigitRune(rs[i-1]) && isDigitRune(rs[i+1]) {
			continue
		}
		rs[i] = ' '
	}
	return string(rs)
}

func isDigitRune(r rune) bool { return r >= '0' && r <= '9' }

// cleanSpaces collapses runs of white space (tabs included) into one space and trims spaces, '-' and ',' from
// both ends [IMV-005].
func cleanSpaces(s string) string {
	return strings.Trim(strings.Join(strings.Fields(s), " "), " -,")
}

// moveArticle moves a trailing article before the principal title [IMV-004]. Only the first match moves.
func moveArticle(s string) string {
	m := reArticle.FindStringSubmatchIndex(s)
	if m == nil {
		return s
	}
	principal := s[m[2]:m[3]]
	article := s[m[4]:m[5]]
	article = strings.ToUpper(article[:1]) + article[1:]
	return article + " " + principal + s[m[5]:]
}

// cutRunes cleans s, keeps at most domain.MaxTitleRunes runes and cleans again [IMV-005].
func cutRunes(s string) string {
	s = cleanSpaces(s)
	rs := []rune(s)
	if len(rs) > domain.MaxTitleRunes {
		rs = rs[:domain.MaxTitleRunes]
	}
	return cleanSpaces(string(rs))
}
