package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
)

func TestLaunchFocusWrap(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cases := []struct {
		name    string
		err     error
		wantRun int
	}{
		{"launch ok focuses the window", nil, 1},
		{"launch failed does not focus", errors.New("boom"), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLaunchFocus(context.Background(), log)
			runs := 0
			f.run = func(context.Context, *slog.Logger) { runs++ }
			h := f.wrap(func(context.Context, json.RawMessage) (any, error) { return "ok", tc.err })
			if _, err := h(context.Background(), nil); !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want %v", err, tc.err)
			}
			f.Stop() // waits for the goroutine, so runs is safe to read
			if runs != tc.wantRun {
				t.Errorf("runs = %d, want %d", runs, tc.wantRun)
			}
		})
	}
}
