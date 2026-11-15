// Package input turns hardware input (hotkey, gamepad combo) into pause triggers.
package input

import (
	"fmt"
	"strings"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// Action is what a trigger asks for.
type Action int

const (
	// ActionPause toggles the pause (the zero value).
	ActionPause Action = iota
	// ActionClose asks to close the running game.
	ActionClose
)

// Trigger is one request coming from an input source.
type Trigger struct {
	Source domain.PauseReason // PauseHotkey | PauseGamepad
	At     time.Time
	Action Action
}

// Source is an input source that delivers triggers to a channel.
type Source interface {
	Name() string
	// Start begins delivering triggers to out. An error means the source is
	// unavailable; nothing is left running.
	Start(out chan<- Trigger) error
	// Close is idempotent and waits for the source goroutine to finish.
	Close() error
}

// Hotkey modifier flags (same values as Win32 MOD_*).
const (
	ModAlt      = 0x1
	ModControl  = 0x2
	ModShift    = 0x4
	ModNoRepeat = 0x4000
)

// Hotkey is a global keyboard shortcut.
type Hotkey struct {
	Modifiers uint32
	VK        uint32
}

// DefaultHotkey is Ctrl+Alt+P.
var DefaultHotkey = Hotkey{Modifiers: ModControl | ModAlt, VK: 'P'}

// String renders the hotkey like "Ctrl+Alt+P".
func (h Hotkey) String() string {
	var parts []string
	if h.Modifiers&ModControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if h.Modifiers&ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if h.Modifiers&ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if h.VK >= 0x70 && h.VK <= 0x87 {
		parts = append(parts, fmt.Sprintf("F%d", h.VK-0x70+1))
	} else {
		parts = append(parts, string(rune(h.VK)))
	}
	return strings.Join(parts, "+")
}
