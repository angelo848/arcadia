//go:build windows

package agent

// Tests for T05 of docs/specs/single-exe/tasks.md: Run rejects bad flags with
// exit code 2 before the mutex, the data directory or any file (EXE-003,
// AGT-005). Run is called in a goroutine with a timeout so that an
// implementation that gets past the flag checks (mutex, tray) fails the test
// instead of hanging it.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// runWithTimeout calls Run and returns its exit code, failing if it blocks.
func runWithTimeout(t *testing.T, opts Options) int {
	t.Helper()
	done := make(chan int, 1)
	go func() { done <- Run(opts) }()
	select {
	case code := <-done:
		return code
	case <-time.After(testTimeout):
		t.Fatalf("Run(%q) did not return within %v (went past the flag checks?)", opts.Args, testTimeout)
		return -1
	}
}

func TestRunFlags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args func(dataDir string) []string
	}{
		{
			name: "EXE-003/Run com --agent --foo --data-dir <tmp>/x retorna 2 e não cria <tmp>/x",
			args: func(d string) []string { return []string{"--agent", "--foo", "--data-dir", d} },
		},
		{
			name: "AGT-005/Run com --log-level inválido retorna 2 e não cria a pasta",
			args: func(d string) []string { return []string{"--agent", "--log-level", "verbose", "--data-dir", d} },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dataDir := filepath.Join(t.TempDir(), "x")
			args := tc.args(dataDir)

			if code := runWithTimeout(t, Options{Args: args, Version: "test"}); code != 2 {
				t.Errorf("Run(%q) = %d, want 2", args, code)
			}
			if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("data dir stat = %v, want not exist (Run must fail before creating it)", err)
			}
		})
	}
}
