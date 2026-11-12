package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ErrInvalidField is wrapped by every field validator of the domain (DOM-015,
// DOM-016, EDT-001, EDT-003).
var ErrInvalidField = errors.New("domain: invalid field")

// NormalizeName trims spaces at both ends and keeps the capitalization. An
// empty result or one longer than MaxNameRunes runes is an error (DOM-015, DOM-016).
func NormalizeName(s string) (string, error) {
	return trimRequired("name", s, MaxNameRunes)
}

// trimRequired trims spaces at both ends; the result must have between 1 and
// max runes.
func trimRequired(field, s string, max int) (string, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", fmt.Errorf("domain: %s is empty: %w", field, ErrInvalidField)
	}
	if n := utf8.RuneCountInString(t); n > max {
		return "", fmt.Errorf("domain: %s has %d runes, max %d: %w", field, n, max, ErrInvalidField)
	}
	return t, nil
}
