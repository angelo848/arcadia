package domain

import (
	"fmt"
	"unicode/utf8"
)

// ValidateTitle trims spaces at both ends. An empty result or one longer than
// MaxTitleRunes runes is ErrInvalidField (EDT-001).
func ValidateTitle(s string) (string, error) {
	return trimRequired("title", s, MaxTitleRunes)
}

// ValidateDescription does not trim. An empty description is allowed; one
// longer than MaxDescriptionRunes runes is ErrInvalidField (EDT-001).
func ValidateDescription(s string) (string, error) {
	if n := utf8.RuneCountInString(s); n > MaxDescriptionRunes {
		return "", fmt.Errorf("domain: description has %d runes, max %d: %w", n, MaxDescriptionRunes, ErrInvalidField)
	}
	return s, nil
}
