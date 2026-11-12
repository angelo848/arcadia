package domain

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// FilterVersion is the only FilterSpec version accepted (FLT-001).
const FilterVersion = 1

// ErrInvalidFilter wraps every FilterSpec validation failure (FLT-001).
var ErrInvalidFilter = errors.New("domain: invalid filter")

// Values of FilterSpec.Played (FLT-006).
const (
	playedAny       = "any"
	playedPlayed    = "played"
	playedNotPlayed = "notPlayed"
)

// FilterSpec is the library filter shared by the grid, the counter and future
// playlists (FLT-001). The JSON names are those of contract v0.6.0. Absent or
// null members do not filter.
type FilterSpec struct {
	Version        int           `json:"version"`
	PlatformIDs    []int64       `json:"platformIds,omitempty"`
	GenreIDs       []int64       `json:"genreIds,omitempty"`
	DeveloperIDs   []int64       `json:"developerIds,omitempty"`
	PublisherIDs   []int64       `json:"publisherIds,omitempty"`
	PlayMode       PlayModeGroup `json:"playMode,omitempty"`
	YearFrom       *int          `json:"yearFrom,omitempty"`
	YearTo         *int          `json:"yearTo,omitempty"`
	Statuses       []GameStatus  `json:"statuses,omitempty"`
	Favorite       *bool         `json:"favorite,omitempty"`
	Played         string        `json:"played,omitempty"` // any | played | notPlayed
	PlaySecondsMin *int64        `json:"playSecondsMin,omitempty"`
	PlaySecondsMax *int64        `json:"playSecondsMax,omitempty"`
	Text           string        `json:"text,omitempty"`
}

// Validate reports the first rule of FLT-001..FLT-007 that f breaks. The error
// wraps ErrInvalidFilter. Empty PlayMode and Played are accepted (Normalize
// fills them in).
func (f FilterSpec) Validate() error {
	if f.Version != FilterVersion {
		return filterErr("version must be %d, got %d", FilterVersion, f.Version)
	}
	facets := []struct {
		name string
		ids  []int64
	}{
		{"platformIds", f.PlatformIDs},
		{"genreIds", f.GenreIDs},
		{"developerIds", f.DeveloperIDs},
		{"publisherIds", f.PublisherIDs},
	}
	for _, facet := range facets {
		if err := validateIDs(facet.name, facet.ids); err != nil {
			return err
		}
	}

	switch f.PlayMode {
	case "", PlayGroupAll, PlayGroupSingle, PlayGroupMulti, PlayGroupCoop:
	default:
		return filterErr("playMode %q is not all, single, multi or coop", f.PlayMode)
	}

	if err := validateYear("yearFrom", f.YearFrom); err != nil {
		return err
	}
	if err := validateYear("yearTo", f.YearTo); err != nil {
		return err
	}
	if f.YearFrom != nil && f.YearTo != nil && *f.YearFrom > *f.YearTo {
		return filterErr("yearFrom %d is after yearTo %d", *f.YearFrom, *f.YearTo)
	}

	if err := validateStatuses(f.Statuses); err != nil {
		return err
	}

	switch f.Played {
	case "", playedAny, playedPlayed, playedNotPlayed:
	default:
		return filterErr("played %q is not any, played or notPlayed", f.Played)
	}

	if f.PlaySecondsMin != nil && *f.PlaySecondsMin < 0 {
		return filterErr("playSecondsMin %d is negative", *f.PlaySecondsMin)
	}
	if f.PlaySecondsMax != nil && *f.PlaySecondsMax < 0 {
		return filterErr("playSecondsMax %d is negative", *f.PlaySecondsMax)
	}
	if f.PlaySecondsMin != nil && f.PlaySecondsMax != nil && *f.PlaySecondsMin > *f.PlaySecondsMax {
		return filterErr("playSecondsMin %d is after playSecondsMax %d", *f.PlaySecondsMin, *f.PlaySecondsMax)
	}

	if n := utf8.RuneCountInString(strings.TrimSpace(f.Text)); n > MaxFilterTextRunes {
		return filterErr("text has %d runes, max %d", n, MaxFilterTextRunes)
	}
	return nil
}

// Normalize trims Text, sorts and de-duplicates the id and status lists, and
// fills PlayMode with PlayGroupAll and Played with "any" when they are empty.
// It returns a copy; the receiver's slices are not changed.
func (f FilterSpec) Normalize() FilterSpec {
	f.Text = strings.TrimSpace(f.Text)
	f.PlatformIDs = sortedUnique(f.PlatformIDs)
	f.GenreIDs = sortedUnique(f.GenreIDs)
	f.DeveloperIDs = sortedUnique(f.DeveloperIDs)
	f.PublisherIDs = sortedUnique(f.PublisherIDs)
	f.Statuses = sortedUnique(f.Statuses)
	if f.PlayMode == "" {
		f.PlayMode = PlayGroupAll
	}
	if f.Played == "" {
		f.Played = playedAny
	}
	return f
}

func filterErr(format string, args ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrInvalidFilter)
}

func validateIDs(facet string, ids []int64) error {
	if len(ids) > MaxFilterIDs {
		return filterErr("%s has %d ids, max %d", facet, len(ids), MaxFilterIDs)
	}
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return filterErr("%s has id %d, must be positive", facet, id)
		}
		if _, dup := seen[id]; dup {
			return filterErr("%s repeats id %d", facet, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateYear(name string, y *int) error {
	if y != nil && (*y < YearMin || *y > YearMax) {
		return filterErr("%s %d outside %d..%d", name, *y, YearMin, YearMax)
	}
	return nil
}

func validateStatuses(statuses []GameStatus) error {
	if len(statuses) > MaxFilterStatuses {
		return filterErr("statuses has %d items, max %d", len(statuses), MaxFilterStatuses)
	}
	seen := make(map[GameStatus]struct{}, len(statuses))
	for _, st := range statuses {
		if _, err := ParseGameStatus(string(st)); err != nil {
			return filterErr("statuses has %q, not a game status", st)
		}
		if _, dup := seen[st]; dup {
			return filterErr("statuses repeats %q", st)
		}
		seen[st] = struct{}{}
	}
	return nil
}

// sortedUnique returns a sorted copy of s without repeated values. nil stays nil.
func sortedUnique[T cmp.Ordered](s []T) []T {
	if s == nil {
		return nil
	}
	out := slices.Clone(s)
	slices.Sort(out)
	return slices.Compact(out)
}
