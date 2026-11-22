package agent

// Tests for T23 of docs/specs/phase-15-big-box-v2/tasks.md: the opener of the Big Box [ATB-004, ATB-006].

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

func TestOpenBigBox(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	exe := filepath.Join(base, "Arcadia", "arcadia.exe")
	root := filepath.Join(base, "Minha Biblioteca")

	t.Run("ATB-004/spawn com o exe, --bigbox --data-dir R (sem --agent) e DETACHED|NEW_PROCESS_GROUP", func(t *testing.T) {
		t.Parallel()
		sp := &fakeSpawn{}
		if err := openBigBox(exe, root, sp.spawn); err != nil {
			t.Fatalf("openBigBox: %v", err)
		}
		calls := sp.snapshot()
		if len(calls) != 1 {
			t.Fatalf("spawn called %d times, want 1", len(calls))
		}
		if calls[0].exe != exe {
			t.Errorf("exe = %q, want %q", calls[0].exe, exe)
		}
		if want := []string{"--bigbox", "--data-dir", root}; !slices.Equal(calls[0].args, want) {
			t.Errorf("args = %q, want %q", calls[0].args, want)
		}
		if slices.Contains(calls[0].args, "--agent") {
			t.Errorf("args %q contain --agent", calls[0].args)
		}
		if want := detachedProcess | createNewProcessGroup; calls[0].flags != want {
			t.Errorf("flags = %#x, want %#x", calls[0].flags, want)
		}
	})

	t.Run("ATB-006/o erro do spawn é devolvido (com contexto)", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("access denied")
		sp := &fakeSpawn{err: boom}
		if err := openBigBox(exe, root, sp.spawn); !errors.Is(err, boom) {
			t.Fatalf("openBigBox error = %v, want it to wrap %v", err, boom)
		}
	})

	t.Run("ATB-004/uiOpener.OpenBigBox usa exe, root e spawn", func(t *testing.T) {
		t.Parallel()
		sp := &fakeSpawn{}
		if err := (uiOpener{exe: exe, root: root, spawn: sp.spawn}).OpenBigBox(); err != nil {
			t.Fatalf("OpenBigBox: %v", err)
		}
		calls := sp.snapshot()
		if len(calls) != 1 || calls[0].exe != exe || !slices.Equal(calls[0].args, []string{"--bigbox", "--data-dir", root}) {
			t.Errorf("spawn calls = %+v", calls)
		}
	})
}
