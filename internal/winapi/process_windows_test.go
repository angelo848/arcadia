//go:build windows

package winapi_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// helperMarker is the first test argument (after "--") that switches the test
// binary into helper-process mode (see TestHelperProcess).
const helperMarker = "arcadia-winapi-helper"

// detachedFlags are the creation flags the agent uses for "Abrir Arcadia".
const detachedFlags = windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP

var procGetProcessHandleCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")

// processHandleCount returns the number of open handles of the current process,
// read independently of the code under test.
func processHandleCount(t *testing.T) uint32 {
	t.Helper()
	var n uint32
	r, _, err := procGetProcessHandleCount.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		t.Fatalf("GetProcessHandleCount: %v", err)
	}
	return n
}

func cmdExe(t *testing.T) string {
	t.Helper()
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatalf("GetSystemDirectory: %v", err)
	}
	return filepath.Join(dir, "cmd.exe")
}

// TestHelperProcess is not a real test: when the test binary is started by
// CreateDetached with helperMarker after "--", it writes the arguments it
// received (after the marker, event name and output file) as JSON to the output
// file and signals the named event. Otherwise it returns immediately.
func TestHelperProcess(t *testing.T) {
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	if len(args) < 3 || args[0] != helperMarker {
		return
	}
	eventName, outFile, rest := args[1], args[2], args[3:]
	if rest == nil {
		rest = []string{}
	}
	data, err := json.Marshal(rest)
	if err != nil {
		os.Exit(3)
	}
	tmp := outFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		os.Exit(4)
	}
	if err := os.Rename(tmp, outFile); err != nil {
		os.Exit(5)
	}
	name, err := windows.UTF16PtrFromString(eventName)
	if err != nil {
		os.Exit(6)
	}
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		os.Exit(7)
	}
	_ = windows.SetEvent(ev)
	_ = windows.CloseHandle(ev)
	os.Exit(0)
}

func TestCreateDetached(t *testing.T) {
	// Not parallel: the handle-count case measures the whole process.

	t.Run("AGT-007/inicia processo sem aumentar a contagem de handles", func(t *testing.T) {
		exe := cmdExe(t)
		args := []string{"/c", "exit", "0"}

		// Warm-up: the first CreateProcess may lazily load DLLs / open handles
		// that stay open for the life of the process.
		if err := winapi.CreateDetached(exe, args, detachedFlags); err != nil {
			t.Fatalf("CreateDetached (aquecimento): %v", err)
		}

		// A leak of hProcess+hThread adds 2 handles per call, i.e. 2*n. The Go
		// runtime and the test framework may open/close a few handles meanwhile,
		// so the tolerance is "growth < n": noise below n passes, any per-call
		// leak (>= 2*n) fails.
		const n = 10
		before := processHandleCount(t)
		for i := range n {
			if err := winapi.CreateDetached(exe, args, detachedFlags); err != nil {
				t.Fatalf("CreateDetached #%d: %v", i, err)
			}
		}
		after := processHandleCount(t)
		if after > before && after-before >= n {
			t.Fatalf("contagem de handles cresceu de %d para %d após %d CreateDetached (tolerância < %d): hProcess/hThread não fechados?",
				before, after, n, n)
		}
	})

	t.Run("DSK-005/exe inexistente preserva o errno via errors.Is", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nao-existe.exe")
		err := winapi.CreateDetached(missing, nil, detachedFlags)
		if err == nil {
			t.Fatal("CreateDetached de exe inexistente retornou nil")
		}
		if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			t.Fatalf("errors.Is(err, ERROR_FILE_NOT_FOUND) = false; err = %#v (%v)", err, err)
		}
		var errno windows.Errno
		if !errors.As(err, &errno) {
			t.Fatalf("errors.As(err, *windows.Errno) = false; err = %v", err)
		}
	})

	t.Run("DSK-005/ERROR_ACCESS_DENIED preservado via errors.Is", func(t *testing.T) {
		// CreateProcessW on a directory fails with ERROR_ACCESS_DENIED (observed
		// on Windows 11 26200), the same errno the UI uses to drop
		// CREATE_BREAKAWAY_FROM_JOB.
		err := winapi.CreateDetached(t.TempDir(), nil, detachedFlags)
		if err == nil {
			t.Fatal("CreateDetached de diretório retornou nil")
		}
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			t.Fatalf("errors.Is(err, ERROR_ACCESS_DENIED) = false; err = %#v (%v)", err, err)
		}
	})

	t.Run("DSK-004/argumento com espaço chega como um só", func(t *testing.T) {
		self, err := os.Executable()
		if err != nil {
			t.Fatalf("os.Executable: %v", err)
		}
		dir := t.TempDir()
		outFile := filepath.Join(dir, "args.json")
		eventName := fmt.Sprintf(`Local\arcadia-winapi-test-%016x`, rand.Uint64())
		namePtr, err := windows.UTF16PtrFromString(eventName)
		if err != nil {
			t.Fatal(err)
		}
		ev, err := windows.CreateEvent(nil, 1, 0, namePtr)
		if err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
		defer windows.CloseHandle(ev)

		want := []string{"--data-dir", `C:\Arcadia dados\com espaço\`, "simples", ""}
		args := append([]string{"-test.run=^TestHelperProcess$", "--", helperMarker, eventName, outFile}, want...)
		if err := winapi.CreateDetached(self, args, detachedFlags); err != nil {
			t.Fatalf("CreateDetached(helper): %v", err)
		}

		ev2, err := windows.WaitForSingleObject(ev, 30_000)
		if err != nil || ev2 != windows.WAIT_OBJECT_0 {
			t.Fatalf("processo auxiliar não sinalizou em 30 s: wait=%#x err=%v", ev2, err)
		}
		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("ler args do processo auxiliar: %v", err)
		}
		var got []string
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("decodificar %q: %v", data, err)
		}
		if len(got) != len(want) {
			t.Fatalf("args recebidos = %q, quer %q", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("arg[%d] = %q, quer %q (todos: %q)", i, got[i], want[i], got)
			}
		}
	})
}
