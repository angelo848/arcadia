package domain

import "fmt"

// PlatformCategory classifies a platform (DOM-015, ADR 0012).
type PlatformCategory string

const (
	CategoryPC       PlatformCategory = "pc"
	CategoryConsole  PlatformCategory = "console"
	CategoryArcade   PlatformCategory = "arcade"
	CategoryHandheld PlatformCategory = "handheld"
)

// ParsePlatformCategory accepts exactly pc, console, arcade or handheld.
func ParsePlatformCategory(s string) (PlatformCategory, error) {
	switch c := PlatformCategory(s); c {
	case CategoryPC, CategoryConsole, CategoryArcade, CategoryHandheld:
		return c, nil
	}
	return "", fmt.Errorf("domain: platform category %q: %w", s, ErrInvalidField)
}
