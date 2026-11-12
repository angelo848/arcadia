package domain

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Launch types a game can have (EDT-010).
const (
	LaunchTypeExe = "exe"
	LaunchTypeURI = "uri"
)

// Launch field limits (EDT-010), in runes.
const (
	MaxLaunchTargetRunes = 2000
	MaxLaunchArgsRunes   = 2000
)

// ValidateLaunch checks the launch type, target and arguments of a game (EDT-010).
// launchType is exe or uri; the target is not empty, at most MaxLaunchTargetRunes
// long, and an absolute path for exe games; the arguments are at most
// MaxLaunchArgsRunes long and may be empty. The error wraps ErrInvalidField.
func ValidateLaunch(launchType, target, args string) error {
	if launchType != LaunchTypeExe && launchType != LaunchTypeURI {
		return fmt.Errorf("launch type %q: %w", launchType, ErrInvalidField)
	}
	target = strings.TrimSpace(target)
	if target == "" || utf8.RuneCountInString(target) > MaxLaunchTargetRunes {
		return fmt.Errorf("launch target: %w", ErrInvalidField)
	}
	if launchType == LaunchTypeExe && !filepath.IsAbs(target) {
		return fmt.Errorf("launch target %q is not an absolute path: %w", target, ErrInvalidField)
	}
	if utf8.RuneCountInString(args) > MaxLaunchArgsRunes {
		return fmt.Errorf("launch args: %w", ErrInvalidField)
	}
	return nil
}
