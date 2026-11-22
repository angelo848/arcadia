package agent

// Tests for T05 of docs/specs/single-exe/tasks.md: "Abrir Arcadia" launches the
// running executable itself (EXE-009, EXE-010) and the agent FlagSet (EXE-003,
// AGT-005).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

// Win32 process creation flags (values from the Windows SDK), written as
// literals so this file stays free of Windows-only imports.
const (
	detachedProcess       uint32 = 0x00000008 // DETACHED_PROCESS
	createNewProcessGroup uint32 = 0x00000200 // CREATE_NEW_PROCESS_GROUP
)

type spawnCall struct {
	exe   string
	args  []string
	flags uint32
}

// fakeSpawn records every call and returns err (nil = success).
type fakeSpawn struct {
	mu    sync.Mutex
	err   error
	calls []spawnCall
}

func (f *fakeSpawn) spawn(exe string, args []string, flags uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, spawnCall{exe: exe, args: slices.Clone(args), flags: flags})
	return f.err
}

func (f *fakeSpawn) snapshot() []spawnCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// captureLog returns a JSON slog logger at Debug and the buffer it writes to.
func captureLog() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

// records parses every JSON log line in buf.
func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

func TestOpenUI(t *testing.T) {
	t.Parallel()
	// The exe does not exist on disk: openUI must not stat it (design.md, EXE-009).
	base := t.TempDir()
	exe := filepath.Join(base, "Arcadia", "arcadia.exe")
	root := filepath.Join(base, "Minha Biblioteca")

	t.Run("EXE-009/spawn com o exe recebido, --data-dir R e flags DETACHED|NEW_PROCESS_GROUP", func(t *testing.T) {
		t.Parallel()
		sp := &fakeSpawn{}
		log, buf := captureLog()

		openUI(exe, root, sp.spawn, log)

		calls := sp.snapshot()
		if len(calls) != 1 {
			t.Fatalf("spawn called %d times, want 1", len(calls))
		}
		got := calls[0]
		if got.exe != exe {
			t.Errorf("spawn exe = %q, want %q", got.exe, exe)
		}
		if want := []string{"--data-dir", root}; !slices.Equal(got.args, want) {
			t.Errorf("spawn args = %q, want %q", got.args, want)
		}
		if want := detachedProcess | createNewProcessGroup; got.flags != want {
			t.Errorf("spawn flags = %#x, want %#x (DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP)", got.flags, want)
		}
		for _, rec := range records(t, buf) {
			if rec["level"] == "ERROR" {
				t.Errorf("unexpected ERROR log on successful open: %v", rec)
			}
		}
	})

	t.Run("EXE-009/sem --agent nos argumentos", func(t *testing.T) {
		t.Parallel()
		sp := &fakeSpawn{}
		log, _ := captureLog()

		openUI(exe, root, sp.spawn, log)

		calls := sp.snapshot()
		if len(calls) != 1 {
			t.Fatalf("spawn called %d times, want 1", len(calls))
		}
		for _, a := range calls[0].args {
			if a == "--agent" || a == "-agent" || a == "--agent=true" || a == "-agent=true" {
				t.Errorf("spawn args = %q contain %q, want UI mode (no --agent)", calls[0].args, a)
			}
		}
	})

	t.Run("EXE-010/falha do spawn loga Error falha ao abrir o Arcadia com path e error", func(t *testing.T) {
		t.Parallel()
		spawnErr := errors.New("CreateProcess: access denied")
		sp := &fakeSpawn{err: spawnErr}
		log, buf := captureLog()

		openUI(exe, root, sp.spawn, log)

		var found bool
		for _, rec := range records(t, buf) {
			if rec["msg"] != "falha ao abrir o Arcadia" {
				continue
			}
			found = true
			if rec["level"] != "ERROR" {
				t.Errorf("level = %v, want ERROR", rec["level"])
			}
			if got := fmt.Sprint(rec["path"]); got != exe {
				t.Errorf("path = %q, want %q", got, exe)
			}
			if got := fmt.Sprint(rec["error"]); got != spawnErr.Error() {
				t.Errorf("error = %q, want %q", got, spawnErr.Error())
			}
		}
		if !found {
			t.Errorf("no record with msg %q, got: %s", "falha ao abrir o Arcadia", buf.String())
		}
	})

	t.Run("EXE-010/falha do spawn não interrompe", func(t *testing.T) {
		t.Parallel()
		sp := &fakeSpawn{err: errors.New("spawn failed")}
		log, buf := captureLog()

		// openUI returns normally after a failure, and a later click tries again.
		openUI(exe, root, sp.spawn, log)
		openUI(exe, root, sp.spawn, log)

		if n := len(sp.snapshot()); n != 2 {
			t.Errorf("spawn called %d times after two failed opens, want 2", n)
		}
		var errs int
		for _, rec := range records(t, buf) {
			if rec["level"] == "ERROR" {
				errs++
			}
		}
		if errs != 2 {
			t.Errorf("%d ERROR records, want 2 (one per failed open)", errs)
		}
	})
}

func TestParseFlags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		args     []string
		wantErr  bool
		dataDir  string
		logLevel string
	}{
		{
			name:    "EXE-003/--agent --foo retorna erro",
			args:    []string{"--agent", "--foo"},
			wantErr: true,
		},
		{
			name:    "EXE-003/--agent --data-dir X é aceito",
			args:    []string{"--agent", "--data-dir", "X"},
			dataDir: "X",
		},
		{
			name: "EXE-003/-agent é aceito",
			args: []string{"-agent"},
		},
		// alterado em 2026-11-22 (fase 15): the agent tolerates --bigbox.
		{
			name:    "ATB-008/--agent --bigbox: o flag é tolerado e segue no modo do agente",
			args:    []string{"--agent", "--bigbox"},
			dataDir: "",
		},
		{
			name:    "ATB-008/--agent --bigbox --data-dir X é aceito",
			args:    []string{"--agent", "--bigbox", "--data-dir", "X"},
			dataDir: "X",
		},
		{
			name:     "AGT-005/--log-level debug é lido",
			args:     []string{"--agent", "--log-level", "debug"},
			logLevel: "debug",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := parseFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseFlags(%q) = %+v, nil; want error", tc.args, cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags(%q): %v", tc.args, err)
			}
			if cfg.DataDir != tc.dataDir {
				t.Errorf("DataDir = %q, want %q", cfg.DataDir, tc.dataDir)
			}
			if tc.logLevel != "" && cfg.LogLevel != tc.logLevel {
				t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, tc.logLevel)
			}
		})
	}
}
