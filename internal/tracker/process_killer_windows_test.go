//go:build windows

package tracker

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetHandleCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")

func handleCount(t *testing.T) uint32 {
	t.Helper()
	var n uint32
	if r, _, err := procGetHandleCount.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&n))); r == 0 {
		t.Fatalf("GetProcessHandleCount: %v", err)
	}
	return n
}

// startSleeper starts a harmless long-lived system process (loopback ping,
// no real network) and returns it with its full image path.
func startSleeper(t *testing.T) (*exec.Cmd, string) {
	t.Helper()
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatalf("GetSystemDirectory: %v", err)
	}
	path := filepath.Join(sys, "PING.EXE")
	cmd := exec.Command(path, "-n", "60", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleeper: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd, path
}

func exited(t *testing.T, cmd *exec.Cmd) bool {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return true
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, 10_000)
	return ev == windows.WAIT_OBJECT_0
}

func alive(t *testing.T, cmd *exec.Cmd) bool {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, 0)
	return ev == uint32(windows.WAIT_TIMEOUT)
}

func TestProcessKiller(t *testing.T) {
	// Not parallel: it measures the handle count of the whole test process.

	t.Run("CON-008/processKiller real: caminho errado não mata", func(t *testing.T) {
		cmd, _ := startSleeper(t)
		if err := newProcessKiller().Kill(uint32(cmd.Process.Pid), `C:\Windows\notepad.exe`); err == nil {
			t.Error("Kill with a different image = nil, want a refusal error")
		}
		if !alive(t, cmd) {
			t.Error("process died although the image did not match")
		}
	})

	t.Run("CON-008/processKiller real: caminho certo mata (sem diferença de maiúsculas) e o handle é fechado", func(t *testing.T) {
		k := newProcessKiller()
		killOne := func() {
			cmd, path := startSleeper(t)
			if err := k.Kill(uint32(cmd.Process.Pid), upper(path)); err != nil {
				t.Fatalf("Kill = %v, want nil", err)
			}
			if !exited(t, cmd) {
				t.Fatal("process still alive after Kill with the right image")
			}
			_, _ = cmd.Process.Wait()
		}
		const rounds = 50
		// settled runs the GC in a bounded loop until the count stops dropping.
		settled := func() uint32 {
			last := ^uint32(0)
			for i := 0; i < 20; i++ {
				runtime.GC()
				cur := handleCount(t)
				if cur >= last {
					return cur
				}
				last = cur
				runtime.Gosched()
			}
			return last
		}
		round := func() uint32 {
			for i := 0; i < rounds; i++ {
				killOne()
			}
			return settled()
		}
		// Round 1 absorbs the one-off startup cost (lazy DLLs, runtime threads);
		// rounds 2 and 3 are compared. A real leak grows >= 50 handles per
		// round; the runtime's own thread handles cost about 0-18.
		h1 := round()
		h2 := round()
		h3 := round()
		if growth := int64(h3) - int64(h2); growth > 25 {
			t.Errorf("handle count %d -> %d -> %d over rounds of %d kills (last growth %d): processKiller leaks handles", h1, h2, h3, rounds, growth)
		}
	})
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
