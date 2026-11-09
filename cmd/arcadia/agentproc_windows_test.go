package main

// Tests for the agent start command (EXE-007) and the breakaway fallback (DSK-005).

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"golang.org/x/sys/windows"
)

// Win32 process creation flags (values from the Windows SDK).
const (
	detachedProcess        uint32 = 0x00000008 // DETACHED_PROCESS
	createNewProcessGroup  uint32 = 0x00000200 // CREATE_NEW_PROCESS_GROUP
	createBreakawayFromJob uint32 = 0x01000000 // CREATE_BREAKAWAY_FROM_JOB
)

const testExe = `C:\Program Files\Arcadia\arcadia.exe`
const testRoot = `D:\ArcadiaData`

func TestAgentCommand(t *testing.T) {
	t.Parallel()
	exe, args, flags := agentCommand(testExe, testRoot)

	t.Run("EXE-007/exe é o caminho recebido, com qualquer nome de arquivo", func(t *testing.T) {
		t.Parallel()
		if exe != testExe {
			t.Errorf("exe = %q, want %q", exe, testExe)
		}
		for _, e := range []string{
			`D:\Tools\meu launcher (v2).exe`,
			`C:\Users\x\Downloads\Arcadia-1.2.3-portable.EXE`,
			`E:\renamed.exe`,
		} {
			if got, _, _ := agentCommand(e, testRoot); got != e {
				t.Errorf("agentCommand(%q, ...) exe = %q, want %q (no rename, no sibling binary)", e, got, e)
			}
		}
	})
	t.Run("EXE-007/args são --agent --data-dir R", func(t *testing.T) {
		t.Parallel()
		if want := []string{"--agent", "--data-dir", testRoot}; !slices.Equal(args, want) {
			t.Errorf("args = %q, want %q", args, want)
		}
	})
	t.Run("EXE-007/flags DETACHED|NEW_PROCESS_GROUP|BREAKAWAY", func(t *testing.T) {
		t.Parallel()
		if want := detachedProcess | createNewProcessGroup | createBreakawayFromJob; flags != want {
			t.Errorf("flags = %#x, want %#x", flags, want)
		}
	})
	t.Run("EXE-007/root com espaços é um único argumento", func(t *testing.T) {
		t.Parallel()
		root := `D:\My Games\Arcadia Data`
		_, args, _ := agentCommand(testExe, root)
		if want := []string{"--agent", "--data-dir", root}; !slices.Equal(args, want) {
			t.Errorf("args = %q, want %q", args, want)
		}
	})
}

type spawnCall struct {
	exe   string
	args  []string
	flags uint32
}

// scriptedSpawn returns errs[i] on the i-th call (nil once errs is exhausted).
type scriptedSpawn struct {
	errs []error

	mu    sync.Mutex
	calls []spawnCall
}

func (s *scriptedSpawn) spawn(exe string, args []string, flags uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := len(s.calls)
	s.calls = append(s.calls, spawnCall{exe: exe, args: slices.Clone(args), flags: flags})
	if i < len(s.errs) {
		return s.errs[i]
	}
	return nil
}

func TestStartAgentProcess(t *testing.T) {
	t.Parallel()
	wantExe, wantArgs, wantFlags := agentCommand(testExe, testRoot)
	errOther := windows.ERROR_FILE_NOT_FOUND

	retry := []struct {
		name   string
		denied error
	}{
		{"DSK-005/ACCESS_DENIED tenta de novo sem BREAKAWAY e loga aviso", windows.ERROR_ACCESS_DENIED},
		{"DSK-005/ACCESS_DENIED embrulhado tenta de novo sem BREAKAWAY e loga aviso",
			fmt.Errorf("winapi: create detached: %w", windows.ERROR_ACCESS_DENIED)},
	}
	for _, tc := range retry {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sp := &scriptedSpawn{errs: []error{tc.denied}}
			log, buf := captureLog()

			err := startAgentProcess(sp.spawn, testExe, testRoot, log)

			if err != nil {
				t.Errorf("startAgentProcess err = %v, want nil (second attempt succeeded)", err)
			}
			if len(sp.calls) != 2 {
				t.Fatalf("spawn called %d times, want 2", len(sp.calls))
			}
			first, second := sp.calls[0], sp.calls[1]
			if first.flags != wantFlags {
				t.Errorf("first flags = %#x, want %#x", first.flags, wantFlags)
			}
			if want := wantFlags &^ createBreakawayFromJob; second.flags != want {
				t.Errorf("second flags = %#x, want %#x (same flags without CREATE_BREAKAWAY_FROM_JOB)", second.flags, want)
			}
			for i, c := range sp.calls {
				if c.exe != wantExe || !slices.Equal(c.args, wantArgs) {
					t.Errorf("call %d = %q %q, want %q %q", i+1, c.exe, c.args, wantExe, wantArgs)
				}
			}
			if n := countLevel(t, buf, "WARN"); n != 1 {
				t.Errorf("WARN records = %d, want 1; log: %s", n, buf.String())
			}
		})
	}

	t.Run("EXE-007/fallback sem BREAKAWAY usa o mesmo exe e args", func(t *testing.T) {
		t.Parallel()
		exe := `D:\Portable Apps\arcadia-renomeado.exe`
		root := `E:\Dados do Arcadia`
		sp := &scriptedSpawn{errs: []error{windows.ERROR_ACCESS_DENIED}}
		log, _ := captureLog()

		if err := startAgentProcess(sp.spawn, exe, root, log); err != nil {
			t.Errorf("startAgentProcess err = %v, want nil (second attempt succeeded)", err)
		}
		if len(sp.calls) != 2 {
			t.Fatalf("spawn called %d times, want 2", len(sp.calls))
		}
		wantArgs := []string{"--agent", "--data-dir", root}
		wantFlags := []uint32{
			detachedProcess | createNewProcessGroup | createBreakawayFromJob,
			detachedProcess | createNewProcessGroup,
		}
		for i, c := range sp.calls {
			if c.exe != exe || !slices.Equal(c.args, wantArgs) || c.flags != wantFlags[i] {
				t.Errorf("call %d = %q %q %#x, want %q %q %#x", i+1, c.exe, c.args, c.flags, exe, wantArgs, wantFlags[i])
			}
		}
	})

	t.Run("DSK-005/ACCESS_DENIED e segunda tentativa falha retorna erro", func(t *testing.T) {
		t.Parallel()
		sp := &scriptedSpawn{errs: []error{windows.ERROR_ACCESS_DENIED, errOther}}
		log, _ := captureLog()

		err := startAgentProcess(sp.spawn, testExe, testRoot, log)

		if !errors.Is(err, errOther) {
			t.Errorf("startAgentProcess err = %v, want errors.Is(err, second spawn error)", err)
		}
		if len(sp.calls) != 2 {
			t.Errorf("spawn called %d times, want 2 (no third attempt)", len(sp.calls))
		}
	})

	t.Run("DSK-005/outro erro não tenta de novo", func(t *testing.T) {
		t.Parallel()
		sp := &scriptedSpawn{errs: []error{fmt.Errorf("create: %w", errOther)}}
		log, buf := captureLog()

		err := startAgentProcess(sp.spawn, testExe, testRoot, log)

		if !errors.Is(err, errOther) {
			t.Errorf("startAgentProcess err = %v, want errors.Is(err, spawn error)", err)
		}
		if len(sp.calls) != 1 {
			t.Errorf("spawn called %d times, want 1", len(sp.calls))
		}
		if n := countLevel(t, buf, "WARN"); n != 0 {
			t.Errorf("WARN records = %d, want 0 (no fallback); log: %s", n, buf.String())
		}
	})

	t.Run("DSK-005/sucesso na primeira não tenta de novo", func(t *testing.T) {
		t.Parallel()
		sp := &scriptedSpawn{}
		log, buf := captureLog()

		err := startAgentProcess(sp.spawn, testExe, testRoot, log)

		if err != nil {
			t.Errorf("startAgentProcess err = %v, want nil", err)
		}
		if len(sp.calls) != 1 {
			t.Fatalf("spawn called %d times, want 1", len(sp.calls))
		}
		if c := sp.calls[0]; c.exe != wantExe || !slices.Equal(c.args, wantArgs) || c.flags != wantFlags {
			t.Errorf("spawn call = %q %q %#x, want %q %q %#x", c.exe, c.args, c.flags, wantExe, wantArgs, wantFlags)
		}
		if n := countLevel(t, buf, "WARN"); n != 0 {
			t.Errorf("WARN records = %d, want 0; log: %s", n, buf.String())
		}
	})
}
