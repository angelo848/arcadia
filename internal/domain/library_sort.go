package domain

import "fmt"

// Sort is the order of the library listing (LIB-001).
type Sort string

const (
	SortTitle       Sort = "title"
	SortLastPlayed  Sort = "lastPlayed"
	SortTotalPlayed Sort = "totalPlayed"
	SortYear        Sort = "year"

	// DefaultSort is applied by the agent when the request omits sort (LIB-001).
	DefaultSort = SortTitle
)

// ParseSort accepts exactly title, lastPlayed, totalPlayed or year. The empty
// string is an error.
func ParseSort(s string) (Sort, error) {
	switch srt := Sort(s); srt {
	case SortTitle, SortLastPlayed, SortTotalPlayed, SortYear:
		return srt, nil
	}
	return "", fmt.Errorf("domain: sort %q: %w", s, ErrInvalidField)
}
