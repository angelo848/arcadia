//go:build !windows

package input

import (
	"errors"
	"log/slog"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

type hotkey struct{}

// NewHotkey returns a Source that is unavailable on this platform.
func NewHotkey(Hotkey, domain.Clock, *slog.Logger) Source { return &hotkey{} }

func (*hotkey) Name() string               { return "hotkey" }
func (*hotkey) Start(chan<- Trigger) error { return errors.ErrUnsupported }
func (*hotkey) Close() error               { return nil }
