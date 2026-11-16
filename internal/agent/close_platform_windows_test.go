//go:build windows

package agent

import (
	"io"
	"log/slog"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/input"
)

func TestRealPlatformHotkeys(t *testing.T) {
	t.Run("CON-003/o único atalho de teclado registrado pelo agente continua sendo Ctrl+Alt+P", func(t *testing.T) {
		if input.DefaultHotkey.Modifiers != input.ModControl|input.ModAlt || input.DefaultHotkey.VK != 'P' {
			t.Fatalf("DefaultHotkey = %+v, want Ctrl+Alt+P", input.DefaultHotkey)
		}
		parts := realPlatform(domain.NewFakeClock(pauseT0), slog.New(slog.NewTextHandler(io.Discard, nil)))
		hotkeys := 0
		for _, s := range parts.Sources {
			if s.Name() == "hotkey" {
				hotkeys++
			}
		}
		if hotkeys != 1 {
			t.Errorf("keyboard hotkey sources = %d, want exactly 1 (Ctrl+Alt+P)", hotkeys)
		}
	})
}
