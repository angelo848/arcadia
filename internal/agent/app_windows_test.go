//go:build windows

package agent

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

const pipeExistsMsg = "pipe já existe: outra instância ou outro processo"

func TestStartAgentPipeExists(t *testing.T) {
	cases := []struct {
		name string
		code windows.Errno
	}{
		{"ERROR_ALREADY_EXISTS", windows.ERROR_ALREADY_EXISTS},
		// Code observed in practice (Windows 11 26200, go-winio v0.6.4; design.md, T08 notes).
		{"ERROR_ACCESS_DENIED", windows.ERROR_ACCESS_DENIED},
	}
	for _, tc := range cases {
		t.Run("IPC-019/listen com nome existente loga Error explícito/"+tc.name, func(t *testing.T) {
			cfg := testConfig(t, "")
			listenErr := &os.PathError{Op: "open", Path: `\\.\pipe\arcadia-agent-test`, Err: tc.code}
			listen := func() (net.Listener, error) { return nil, listenErr }

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a, err := startAgent(ctx, cfg, testClock(), emptyEnv, listen)
			if err == nil {
				if a != nil {
					sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
					defer scancel()
					_ = a.Shutdown(sctx)
					_ = a.CloseLog()
				}
				t.Fatal("startAgent with failing listen: want error, got nil")
			}
			if !errors.Is(err, tc.code) {
				t.Errorf("startAgent error = %v, want errors.Is(err, %s)", err, tc.name)
			}

			var found bool
			for _, rec := range logLines(t, agentLogPath(cfg)) {
				if rec["msg"] != pipeExistsMsg {
					continue
				}
				found = true
				if rec["level"] != "ERROR" {
					t.Errorf("level = %v, want ERROR", rec["level"])
				}
				if _, ok := rec["pipe"]; !ok {
					t.Errorf("record has no %q attribute: %v", "pipe", rec)
				}
				if _, ok := rec["error"]; !ok {
					t.Errorf("record has no %q attribute: %v", "error", rec)
				}
			}
			if !found {
				t.Errorf("agent.log has no record with msg %q", pipeExistsMsg)
			}

			// A failed startAgent must release what it opened (store and log),
			// otherwise these files stay locked on Windows.
			for _, p := range []string{agentLogPath(cfg), dbPath(cfg), dbPath(cfg) + "-wal", dbPath(cfg) + "-shm"} {
				if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Errorf("remove %s after failed startAgent: %v (handle still open)", p, err)
				}
			}
		})
	}
}
