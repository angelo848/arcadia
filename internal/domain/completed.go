package domain

import (
	"fmt"
	"time"
)

const completedDateLayout = "2006-01-02"

// ParseCompletedDate reads a real calendar date in YYYY-MM-DD and returns
// midnight UTC of that day (EDT-001, ADR 0009). Any other text, or a date that
// does not exist (such as 2026-02-30), is ErrInvalidField.
func ParseCompletedDate(s string) (time.Time, error) {
	t, err := time.Parse(completedDateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("domain: completed date %q: %w", s, ErrInvalidField)
	}
	return t, nil
}

// FormatCompletedDate returns the YYYY-MM-DD of t in UTC.
func FormatCompletedDate(t time.Time) string {
	return t.UTC().Format(completedDateLayout)
}
