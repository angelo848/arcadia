package domain

import (
	"strings"
	"unicode"
)

// MatchThreshold is the score a candidate needs to be applied alone; below it the UI lists the
// candidates and the user chooses (MID-002).
const MatchThreshold = 0.80

// Candidate is a game as a provider or the library names it, for matching (MID-002). Year 0
// means unknown and gives no bonus.
type Candidate struct {
	Title    string
	Platform string
	Year     int
}

// editionWords are removed from titles before comparing them (MID-002).
var editionWords = map[string]bool{
	"edition": true, "edicao": true, "goty": true, "remastered": true, "remaster": true,
}

// accentFold maps the accented letters of Portuguese and of common Western titles to plain ones.
var accentFold = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n',
}

// NormalizeTitle returns the comparable form of a title: lowercase, no trademark signs, no
// region or disc tags in brackets or parentheses, no edition words, no accents, and single spaces
// (MID-002).
func NormalizeTitle(s string) string {
	s = stripGroups(strings.ToLower(s), '(', ')')
	s = stripGroups(s, '[', ']')
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '™' || r == '®' || r == '©':
			b.WriteRune(' ')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if f, ok := accentFold[r]; ok {
				r = f
			}
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	words := strings.Fields(b.String())
	kept := words[:0]
	for _, w := range words {
		if !editionWords[w] {
			kept = append(kept, w)
		}
	}
	return strings.Join(kept, " ")
}

// stripGroups removes every open/close group, nested or not, such as "(USA)" or "[Disc 1]".
func stripGroups(s string, open, end rune) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == open:
			depth++
		case r == end && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TitleSimilarity is the Dice coefficient over the letter pairs of the normalized titles: 1 for
// equal titles, 0 when they share no pair (MID-002).
func TitleSimilarity(a, b string) float64 {
	na, nb := NormalizeTitle(a), NormalizeTitle(b)
	if na == nb {
		return 1
	}
	ra, rb := []rune(na), []rune(nb)
	if len(ra) < 2 || len(rb) < 2 {
		return 0
	}
	counts := make(map[string]int, len(ra))
	for i := 0; i+1 < len(ra); i++ {
		counts[string(ra[i:i+2])]++
	}
	shared := 0
	for i := 0; i+1 < len(rb); i++ {
		key := string(rb[i : i+2])
		if counts[key] > 0 {
			counts[key]--
			shared++
		}
	}
	return 2 * float64(shared) / float64(len(ra)-1+len(rb)-1)
}

// MatchScore scores a candidate against the game: the title similarity, plus 0.05 when the
// platform is the same and 0.05 when the year is the same, capped at 1 (MID-002).
func MatchScore(game, cand Candidate) float64 {
	score := TitleSimilarity(game.Title, cand.Title)
	if strings.EqualFold(strings.TrimSpace(game.Platform), strings.TrimSpace(cand.Platform)) && game.Platform != "" {
		score += 0.05
	}
	if game.Year != 0 && game.Year == cand.Year {
		score += 0.05
	}
	if score > 1 {
		score = 1
	}
	return score
}

// MatchAccepted reports whether a score is enough to apply the candidate without asking (MID-002).
func MatchAccepted(score float64) bool {
	return score >= MatchThreshold
}
