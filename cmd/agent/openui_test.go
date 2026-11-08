package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
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

type fakeSpawn struct {
	mu    sync.Mutex
	calls []spawnCall
}

func (f *fakeSpawn) spawn(exe string, args []string, flags uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, spawnCall{exe: exe, args: args, flags: flags})
	return nil
}

// captureLog returns a JSON slog logger at Debug and the buffer it writes to.
func captureLog() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func levelsIn(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	var levels []string
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		levels = append(levels, fmt.Sprint(rec["level"]))
	}
	return levels
}

func TestOpenUI(t *testing.T) {
	t.Parallel()
	exeDir := filepath.Join(t.TempDir(), "Arcadia")
	wantExe := filepath.Join(exeDir, "arcadia.exe")

	t.Run("AGT-007/arcadia.exe existe: spawn com flags desacopladas", func(t *testing.T) {
		t.Parallel()
		var statted []string
		stat := func(p string) error { statted = append(statted, p); return nil }
		sp := &fakeSpawn{}
		log, buf := captureLog()

		openUI(exeDir, stat, sp.spawn, log)

		if len(statted) != 1 || statted[0] != wantExe {
			t.Errorf("stat calls = %q, want [%q]", statted, wantExe)
		}
		if len(sp.calls) != 1 {
			t.Fatalf("spawn called %d times, want 1", len(sp.calls))
		}
		got := sp.calls[0]
		if got.exe != wantExe {
			t.Errorf("spawn exe = %q, want %q", got.exe, wantExe)
		}
		if len(got.args) != 0 {
			t.Errorf("spawn args = %q, want none", got.args)
		}
		if want := detachedProcess | createNewProcessGroup; got.flags != want {
			t.Errorf("spawn flags = %#x, want %#x (DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP)", got.flags, want)
		}
		for _, lvl := range levelsIn(t, buf) {
			if lvl == "ERROR" {
				t.Errorf("unexpected ERROR log on successful open: %s", buf.String())
			}
		}
	})

	t.Run("AGT-008/arcadia.exe ausente: loga erro e não chama spawn", func(t *testing.T) {
		t.Parallel()
		var statted []string
		stat := func(p string) error {
			statted = append(statted, p)
			return &fs.PathError{Op: "CreateFile", Path: p, Err: os.ErrNotExist}
		}
		sp := &fakeSpawn{}
		log, buf := captureLog()

		openUI(exeDir, stat, sp.spawn, log)

		if len(statted) != 1 || statted[0] != wantExe {
			t.Errorf("stat calls = %q, want [%q]", statted, wantExe)
		}
		if len(sp.calls) != 0 {
			t.Errorf("spawn called %d times, want 0", len(sp.calls))
		}
		var errs int
		for _, lvl := range levelsIn(t, buf) {
			if lvl == "ERROR" {
				errs++
			}
		}
		if errs == 0 {
			t.Errorf("no ERROR record logged, got: %q", buf.String())
		}
	})
}
