package domain

import (
	"errors"
	"fmt"
	"time"
)

// PauseReason records why a play session is paused (ADR 0012: text value
// validated by the application).
type PauseReason string

// Pause reasons.
const (
	PauseHotkey  PauseReason = "hotkey"
	PauseGamepad PauseReason = "gamepad"
	PauseUI      PauseReason = "ui"
	PauseSleep   PauseReason = "sleep"
)

// errInvalidPauseReason is returned when a text value is not a known PauseReason.
var errInvalidPauseReason = errors.New("invalid pause reason")

// ParsePauseReason converts a text value into a PauseReason. The match is exact
// (case-sensitive, no trimming).
func ParsePauseReason(s string) (PauseReason, error) {
	switch r := PauseReason(s); r {
	case PauseHotkey, PauseGamepad, PauseUI, PauseSleep:
		return r, nil
	default:
		return "", fmt.Errorf("parse pause reason %q: %w", s, errInvalidPauseReason)
	}
}

// UserInitiated reports whether the pause was requested by the user (as opposed
// to the system, e.g. sleep).
func (r PauseReason) UserInitiated() bool {
	switch r {
	case PauseHotkey, PauseGamepad, PauseUI:
		return true
	default:
		return false
	}
}

// PauseToggleDebounce is the minimum interval between accepted pause toggles.
const PauseToggleDebounce = 300 * time.Millisecond

// CloseDebounce is the minimum interval between two close-game requests (CON-004).
const CloseDebounce = 300 * time.Millisecond

// BigBoxDebounce is the minimum interval between two open-Big-Box requests (ATB-003).
const BigBoxDebounce = 300 * time.Millisecond
