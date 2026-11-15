package domain

import (
	"math"
	"slices"
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

// Thresholds of the match classification, in thousandths so they compare as integers [MET-009].
const (
	MatchAuto          = 850 // minimum score of a "match" (0,85)
	MatchMinSimilarity = 800 // minimum title similarity of a "match": bonuses cannot make up for a weak title (0,80)
	MatchMargin        = 50  // minimum lead of the first item over the second (0,05)
	MatchDoubt         = 600 // minimum score of a "doubt" (0,60)
	MatchAlternative   = 400 // floor of any listed item (0,40)
	MatchCap           = 790 // ceiling of a trapped score [MET-007] (0,79)
)

// Bonuses and limits of ScoreGroup, in thousandths [MET-006].
const (
	bonusPlatform      = 50 // 0,05 when the platform is confirmed
	bonusYear          = 50 // 0,05 when the year is equal
	bonusExtraProvider = 20 // 0,02 per extra provider ...
	maxExtraProviders  = 2  // ... up to two
)

// Same-game rules [MET-005].
const (
	sameGameDice = 900 // minimum Dice of two keys that are not equal, with equal marks (0,90)
	maxYearGap   = 1   // years further apart than this are different games
)

// Confidence is how sure the search is that the first candidate is the game [MET-009].
type Confidence string

const (
	ConfidenceMatch Confidence = "match"
	ConfidenceDoubt Confidence = "doubt"
	ConfidenceNone  Confidence = "none"
)

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

// Member is one provider's candidate inside a group.
type Member struct {
	Provider          string // "igdb", "steamstore", "steamgriddb"
	Title             string
	Year              int      // 0 = unknown
	PlatformKeys      []string // seed keys the provider lists; empty = no information
	PlatformConfirmed bool     // false for the IGDB candidates of the unfiltered second query [MET-003]
}

// Query is what the user searched for. LibraryKey is the seed key of the library platform, "" when it has none.
type Query struct {
	Title      string
	Year       int
	LibraryKey string
}

// Scored is the result of scoring a group; Score and Sim are in thousandths, so thresholds compare as integers.
// Capped is true when the score was trapped at MatchCap [MET-007].
type Scored struct {
	Score, Sim int
	Capped     bool
}

// SameGame says whether two members of different providers are the same game [MET-005]: the
// years are not more than one apart (when both are known) and the keys are equal, or the keys have
// equal marks and a Dice of at least 0,90.
func SameGame(a, b Member) bool {
	if a.Provider == b.Provider {
		return false
	}
	if a.Year != 0 && b.Year != 0 {
		gap := a.Year - b.Year
		if gap < 0 {
			gap = -gap
		}
		if gap > maxYearGap {
			return false
		}
	}
	ka, kb := MatchKey(a.Title), MatchKey(b.Title)
	if ka == "" || kb == "" {
		return false
	}
	if ka == kb {
		return true
	}
	if !slices.Equal(Marks(ka), Marks(kb)) {
		return false
	}
	return thousandths(diceKeys(ka, kb)) >= sameGameDice
}

// ScoreGroup scores a group of members [MET-006, MET-007]: the similarity of the query key to the
// key of the member closest to it, plus the bonuses for platform, year and extra providers, capped
// at 1 and trapped at MatchCap when the marks differ or the platform does not match.
func ScoreGroup(q Query, members []Member) Scored {
	if len(members) == 0 {
		return Scored{}
	}
	qKey := MatchKey(q.Title)

	bestSim, bestKey := -1, ""
	for _, m := range members {
		k := MatchKey(m.Title)
		if s := thousandths(diceKeys(qKey, k)); s > bestSim {
			bestSim, bestKey = s, k
		}
	}

	score := bestSim
	lib := q.LibraryKey

	// Platform bonus: a confirmed member lists the library platform. Only IGDB can be unconfirmed.
	if lib != "" {
		for _, m := range members {
			if confersPlatform(m, lib) {
				score += bonusPlatform
				break
			}
		}
	}
	if q.Year != 0 {
		for _, m := range members {
			if m.Year == q.Year {
				score += bonusYear
				break
			}
		}
	}
	providers := make(map[string]bool, len(members))
	for _, m := range members {
		providers[m.Provider] = true
	}
	extra := min(len(providers)-1, maxExtraProviders)
	if extra > 0 {
		score += bonusExtraProvider * extra
	}
	score = min(score, 1000)

	capped := !slices.Equal(Marks(qKey), Marks(bestKey))
	if lib != "" && platformTrap(members, lib) {
		capped = true
	}
	if capped {
		score = min(score, MatchCap)
	}
	return Scored{Score: score, Sim: bestSim, Capped: capped}
}

// ClassifyMatch applies the table of match-rules.md §3 to the best and the second best group
// [MET-009]. A missing first item is ConfidenceNone.
func ClassifyMatch(first, second *Scored) Confidence {
	if first == nil {
		return ConfidenceNone
	}
	if first.Score >= MatchAuto && first.Sim >= MatchMinSimilarity && !first.Capped &&
		(second == nil || first.Score-second.Score >= MatchMargin) {
		return ConfidenceMatch
	}
	if first.Score >= MatchDoubt {
		return ConfidenceDoubt
	}
	return ConfidenceNone
}

// confersPlatform reports whether a member confirms that the game exists on the platform key.
func confersPlatform(m Member, key string) bool {
	if m.Provider == "igdb" && !m.PlatformConfirmed {
		return false
	}
	return slices.Contains(m.PlatformKeys, key)
}

// platformTrap is the platform rule of MET-007: some member lists platforms, none confirms the
// library platform; or the group came from the unfiltered second IGDB query, which traps it whatever
// the other members say (match-rules §1).
func platformTrap(members []Member, lib string) bool {
	for _, m := range members {
		if m.Provider == "igdb" && !m.PlatformConfirmed {
			return true
		}
	}
	listed := false
	for _, m := range members {
		if len(m.PlatformKeys) == 0 {
			continue
		}
		listed = true
		if confersPlatform(m, lib) {
			return false
		}
	}
	return listed
}

// GameSimilarity is the Dice of the match keys of two members in thousandths. buildGroups uses it to
// pick, among the groups a candidate can join, the closest one (the oldest wins a tie) [MET-005].
func GameSimilarity(a, b Member) int {
	return thousandths(diceKeys(MatchKey(a.Title), MatchKey(b.Title)))
}

// thousandths converts a ratio to thousandths, rounded, so thresholds compare as integers.
func thousandths(x float64) int {
	return int(math.Round(x * 1000))
}

// diceKeys is the Dice coefficient over the rune pairs of two match keys: 1 for equal keys, 0 when
// one has fewer than two runes.
func diceKeys(a, b string) float64 {
	if a == b {
		return 1
	}
	ra, rb := []rune(a), []rune(b)
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
