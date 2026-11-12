package main

import (
	"context"
	"errors"
	"testing"
)

// TestPickEmulatorExe checks the executable dialog of an emulator (EMU-006).
func TestPickEmulatorExe(t *testing.T) {
	t.Parallel()

	t.Run("EMU-006/diálogo de executável com título próprio e filtro *.exe", func(t *testing.T) {
		t.Parallel()
		calls := make(chan openFileCall, 1)
		a := &App{
			log:      discardLog(),
			wailsCtx: context.Background(),
			platform: platform{openFile: func(ctx context.Context, title string, filters []fileFilter) (string, error) {
				calls <- openFileCall{ctx, title, filters}
				return `C:\Emu\snes9x.exe`, nil
			}},
		}
		got, err := a.PickEmulatorExe()
		if err != nil || got != `C:\Emu\snes9x.exe` {
			t.Fatalf("PickEmulatorExe() = %q, %v", got, err)
		}
		c := <-calls
		if c.title != pickEmulatorTitle || len(c.filters) != 1 || c.filters[0].Pattern != "*.exe" {
			t.Errorf("dialog = %q / %+v", c.title, c.filters)
		}
	})

	t.Run("EMU-006/cancelar devolve vazio", func(t *testing.T) {
		t.Parallel()
		a := &App{
			log:      discardLog(),
			wailsCtx: context.Background(),
			platform: platform{openFile: func(context.Context, string, []fileFilter) (string, error) { return "", nil }},
		}
		if got, err := a.PickEmulatorExe(); err != nil || got != "" {
			t.Errorf("PickEmulatorExe() = %q, %v; want empty and nil", got, err)
		}
	})

	t.Run("EMU-006/falha do diálogo devolve erro", func(t *testing.T) {
		t.Parallel()
		a := &App{
			log:      discardLog(),
			wailsCtx: context.Background(),
			platform: platform{openFile: func(context.Context, string, []fileFilter) (string, error) {
				return "", errors.New("boom")
			}},
		}
		if _, err := a.PickEmulatorExe(); err == nil {
			t.Error("PickEmulatorExe() err = nil, want an error")
		}
	})
}
