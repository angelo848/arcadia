package domain

import (
	"sort"
	"time"
)

// TTBState is the state of a time-to-beat row (TTB-002).
type TTBState string

const (
	TTBFound    TTBState = "found"
	TTBDoubt    TTBState = "doubt"
	TTBNotFound TTBState = "not-found"
)

// Sources of a time-to-beat row.
const (
	TTBSourceHLTB = "hltb"
	TTBSourceIGDB = "igdb"
)

// Validity of each kind of row (TTB-006).
const (
	TTBFoundTTL     = 90 * 24 * time.Hour
	TTBIGDBFoundTTL = 30 * 24 * time.Hour
	TTBDoubtTTL     = 30 * 24 * time.Hour
	TTBNotFoundTTL  = 30 * 24 * time.Hour
)

// TTBResult is one HowLongToBeat search result reduced to what the match needs.
type TTBResult struct {
	ID      int64
	Name    string
	Aliases []string
	Year    int
}

// TTBRanked is a scored result; Index is its position in the input slice.
type TTBRanked struct {
	Index  int
	Scored Scored
}

// RankTimeToBeat scores and orders the results (score descending, ties by the smaller ID)
// and classifies the match [TTB-004].
func RankTimeToBeat(title string, year int, results []TTBResult) (TTBState, []TTBRanked) {
	q := Query{Title: title, Year: year}
	ranked := make([]TTBRanked, 0, len(results))
	for i, r := range results {
		members := []Member{{Provider: TTBSourceHLTB, Title: r.Name, Year: r.Year}}
		for _, a := range r.Aliases {
			members = append(members, Member{Provider: TTBSourceHLTB, Title: a, Year: r.Year})
		}
		ranked = append(ranked, TTBRanked{Index: i, Scored: ScoreGroup(q, members)})
	}
	sort.SliceStable(ranked, func(a, b int) bool {
		if ranked[a].Scored.Score != ranked[b].Scored.Score {
			return ranked[a].Scored.Score > ranked[b].Scored.Score
		}
		return results[ranked[a].Index].ID < results[ranked[b].Index].ID
	})
	if len(ranked) == 0 {
		return TTBNotFound, ranked
	}
	var second *Scored
	if len(ranked) > 1 {
		second = &ranked[1].Scored
	}
	switch ClassifyMatch(&ranked[0].Scored, second) {
	case ConfidenceMatch:
		return TTBFound, ranked
	case ConfidenceDoubt:
		return TTBDoubt, ranked
	}
	return TTBNotFound, ranked
}

// TTBExpiresAt returns when a row fetched at fetchedAt expires [TTB-006].
func TTBExpiresAt(source string, state TTBState, fetchedAt time.Time) time.Time {
	switch {
	case state == TTBFound && source == TTBSourceIGDB:
		return fetchedAt.Add(TTBIGDBFoundTTL)
	case state == TTBFound:
		return fetchedAt.Add(TTBFoundTTL)
	case state == TTBDoubt:
		return fetchedAt.Add(TTBDoubtTTL)
	}
	return fetchedAt.Add(TTBNotFoundTTL)
}

// TTBStale reports whether a row must be fetched again [TTB-006]: expired, or unconfirmed
// and the game title changed since the fetch (exact comparison).
func TTBStale(source string, confirmed bool, titleAtFetch, currentTitle string, expiresAt, now time.Time) bool {
	_ = source
	if !now.Before(expiresAt) {
		return true
	}
	return !confirmed && titleAtFetch != currentTitle
}

// TTBBackoff is the wait after the n-th consecutive failure of a source: 1, 5, 30, then 60 minutes.
func TTBBackoff(failures int) time.Duration {
	ladder := [...]time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 60 * time.Minute}
	if failures < 1 {
		failures = 1
	}
	return ladder[min(failures, len(ladder))-1]
}

// HLTBOutcome is what the HowLongToBeat lookup produced.
type HLTBOutcome int

const (
	HLTBFound HLTBOutcome = iota
	HLTBDoubt
	HLTBNotFound
	HLTBFailed // temporary failure or backoff
	HLTBEmptyTerm
)

// IGDBOutcome is what the IGDB fallback produced.
type IGDBOutcome int

const (
	IGDBNotAsked IGDBOutcome = iota
	IGDBValues
	IGDBNoMatch
	IGDBUnavailable
	IGDBNoKey
)

// TTBResolve applies the source table of hltb-rules §10.1 [TTB-033, TTB-034].
func TTBResolve(h HLTBOutcome, i IGDBOutcome) (write bool, source string, state TTBState) {
	switch h {
	case HLTBFound:
		return true, TTBSourceHLTB, TTBFound
	case HLTBDoubt:
		return true, TTBSourceHLTB, TTBDoubt
	case HLTBNotFound:
		if i == IGDBValues {
			return true, TTBSourceIGDB, TTBFound
		}
		return true, TTBSourceHLTB, TTBNotFound
	case HLTBFailed:
		if i == IGDBValues {
			return true, TTBSourceIGDB, TTBFound
		}
		return false, "", ""
	case HLTBEmptyTerm:
		return true, TTBSourceHLTB, TTBNotFound
	}
	return false, "", ""
}
