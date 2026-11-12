package domain

import (
	"fmt"
	"regexp"
	"strconv"
)

// releaseDatePattern accepts exactly YYYY, YYYY-MM and YYYY-MM-DD (DOM-013).
// It matches the SQL filter in design.md, so "1998abc" or "1998-6-12" never
// give a year, neither here nor in the store.
var releaseDatePattern = regexp.MustCompile(`^([0-9]{4})(?:-[0-9]{2}(?:-[0-9]{2})?)?$`)

// ReleaseYear returns the year of YYYY, YYYY-MM or YYYY-MM-DD. Any other text
// gives no year (ok = false).
func ReleaseYear(releaseDate string) (year int, ok bool) {
	m := releaseDatePattern.FindStringSubmatch(releaseDate)
	if m == nil {
		return 0, false
	}
	y, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return y, true
}

// WithYear sets the year of releaseDate (DOM-013, EDT-003). If releaseDate
// already has the year, it is returned unchanged; otherwise the result is the
// four-digit year. A year outside YearMin..YearMax is ErrInvalidField.
func WithYear(releaseDate string, year int) (string, error) {
	if year < YearMin || year > YearMax {
		return "", fmt.Errorf("domain: year %d outside %d..%d: %w", year, YearMin, YearMax, ErrInvalidField)
	}
	if y, ok := ReleaseYear(releaseDate); ok && y == year {
		return releaseDate, nil
	}
	return fmt.Sprintf("%04d", year), nil
}
