package main

import (
	"context"
	"errors"
	"testing"
)

// TestPickGameExecutable checks the executable dialog of a game's page (EDT-010).
func TestPickGameExecutable(t *testing.T) {
	t.Parallel()

	t.Run("EDT-010/diálogo de executável do jogo com filtro *.exe", func(t *testing.T) {
		t.Parallel()
		calls := make(chan openFileCall, 1)
		a := &App{log: discardLog(), wailsCtx: context.Background(),
			platform: platform{openFile: func(ctx context.Context, title string, filters []fileFilter) (string, error) {
				calls <- openFileCall{ctx, title, filters}
				return `C:\Games\Celeste\Celeste.exe`, nil
			}}}
		got, err := a.PickGameExecutable()
		if err != nil || got != `C:\Games\Celeste\Celeste.exe` {
			t.Fatalf("PickGameExecutable() = %q, %v", got, err)
		}
		c := <-calls
		if c.title != pickGameTitle || len(c.filters) != 1 || c.filters[0].Pattern != "*.exe" {
			t.Errorf("dialog = %q / %+v", c.title, c.filters)
		}
	})

	t.Run("EDT-010/cancelar devolve vazio", func(t *testing.T) {
		t.Parallel()
		a := &App{log: discardLog(), wailsCtx: context.Background(),
			platform: platform{openFile: func(context.Context, string, []fileFilter) (string, error) { return "", nil }}}
		if got, err := a.PickGameExecutable(); err != nil || got != "" {
			t.Errorf("PickGameExecutable() = %q, %v; want empty and nil", got, err)
		}
	})

	t.Run("EDT-010/falha do diálogo devolve erro", func(t *testing.T) {
		t.Parallel()
		a := &App{log: discardLog(), wailsCtx: context.Background(),
			platform: platform{openFile: func(context.Context, string, []fileFilter) (string, error) { return "", errors.New("boom") }}}
		if _, err := a.PickGameExecutable(); err == nil {
			t.Error("err = nil, want an error")
		}
	})
}
