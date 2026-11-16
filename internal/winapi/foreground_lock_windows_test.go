//go:build windows

package winapi

import (
	"runtime"
	"slices"
	"testing"

	"golang.org/x/sys/windows"
)

// T51 (CON-023): ForceForegroundWith fixa a thread do SO. Teste de caixa branca (package winapi) porque
// lockThread/unlockThread sao seams nao exportados:
//
//	var ( lockThread = runtime.LockOSThread; unlockThread = runtime.UnlockOSThread )
//
// Fica em arquivo proprio porque foreground_windows_test.go e package winapi_test.

type lockFG struct {
	log      *[]string
	fg       uintptr
	fgThread uint32
	self     uint32
	attachOK bool
	tids     map[string]uint32 // thread ids seen per call, when readTID
	readTID  bool
}

func (f *lockFG) rec(s string) {
	*f.log = append(*f.log, s)
	if f.readTID {
		f.tids[s] = windows.GetCurrentThreadId()
	}
}
func (f *lockFG) IsIconic(uintptr) bool            { return false }
func (f *lockFG) ShowWindow(uintptr, int)          {}
func (f *lockFG) SetWindowPos(_, _ uintptr)        {}
func (f *lockFG) SetForegroundWindow(uintptr) bool { f.rec("sfw"); return false }
func (f *lockFG) GetForegroundWindow() uintptr     { return f.fg }
func (f *lockFG) ThreadOf(uintptr) uint32          { return f.fgThread }
func (f *lockFG) CurrentThread() uint32            { f.rec("current"); return f.self }
func (f *lockFG) AttachThreadInput(_, _ uint32, attach bool) bool {
	if attach {
		f.rec("attach")
		return f.attachOK
	}
	f.rec("detach")
	return true
}
func (f *lockFG) BringWindowToTop(uintptr) bool { f.rec("bring"); return false }

func withSeams(t *testing.T, log *[]string) {
	t.Helper()
	oldL, oldU := lockThread, unlockThread
	lockThread = func() { *log = append(*log, "lock") }
	unlockThread = func() { *log = append(*log, "unlock") }
	t.Cleanup(func() { lockThread, unlockThread = oldL, oldU })
}

func TestForceForegroundLocksThread(t *testing.T) { // not parallel: swaps package seams
	const h = uintptr(0x1234)

	t.Run("CON-023/ForceForegroundWith fixa a thread antes de CurrentThread e a solta depois do AttachThreadInput(..., false)", func(t *testing.T) {
		var log []string
		withSeams(t, &log)
		f := &lockFG{log: &log, fg: 0x99, fgThread: 7, self: 3, attachOK: true}
		ForceForegroundWith(f, h)
		want := []string{"lock", "current", "sfw", "attach", "bring", "sfw", "detach", "unlock"}
		if !slices.Equal(log, want) {
			t.Fatalf("ordem = %v, want %v", log, want)
		}
	})

	t.Run("CON-023/o desanexar e o soltar rodam mesmo quando BringWindowToTop ou SetForegroundWindow falham", func(t *testing.T) {
		var log []string
		withSeams(t, &log)
		f := &lockFG{log: &log, fg: 0x99, fgThread: 7, self: 3, attachOK: true} // bring and sfw return false
		if ForceForegroundWith(f, h) {
			t.Fatal("want false")
		}
		n := len(log)
		if n < 2 || log[n-2] != "detach" || log[n-1] != "unlock" {
			t.Fatalf("ordem = %v, want terminar em detach, unlock", log)
		}
	})

	t.Run("CON-023/foreground ja e a janela: lock e unlock em par, nenhum AttachThreadInput", func(t *testing.T) {
		var log []string
		withSeams(t, &log)
		f := &lockFG{log: &log, fg: h}
		if !ForceForegroundWith(f, h) {
			t.Fatal("want true")
		}
		if slices.Contains(log, "attach") || slices.Contains(log, "detach") {
			t.Fatalf("attach inesperado: %v", log)
		}
		locks, unlocks := 0, 0
		for _, c := range log {
			switch c {
			case "lock":
				locks++
			case "unlock":
				unlocks++
			}
		}
		if locks != unlocks {
			t.Fatalf("lock/unlock desemparelhados: %v", log)
		}
	})

	t.Run("CON-023/attach falho: lock e unlock em par e nenhum detach", func(t *testing.T) {
		var log []string
		withSeams(t, &log)
		f := &lockFG{log: &log, fg: 0x99, fgThread: 7, self: 3, attachOK: false}
		ForceForegroundWith(f, h)
		if slices.Contains(log, "detach") || log[0] != "lock" || log[len(log)-1] != "unlock" {
			t.Fatalf("ordem = %v", log)
		}
	})

	t.Run("CON-023/os ids de thread do SO vistos por CurrentThread, pelo anexar e pelo desanexar sao iguais", func(t *testing.T) {
		var log []string
		f := &lockFG{log: &log, fg: 0x99, fgThread: 7, self: 3, attachOK: true, tids: map[string]uint32{}, readTID: true}
		ForceForegroundWith(f, h) // real seams
		a, b, c := f.tids["current"], f.tids["attach"], f.tids["detach"]
		if a == 0 || a != b || b != c {
			t.Fatalf("thread ids: current=%d attach=%d detach=%d", a, b, c)
		}
	})

	t.Run("CON-023/chamada de uma goroutine ja fixa continua fixa depois (contagem aninhada)", func(t *testing.T) {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		before := windows.GetCurrentThreadId()
		var log []string
		f := &lockFG{log: &log, fg: 0x99, fgThread: 7, self: 3, attachOK: true}
		ForceForegroundWith(f, h) // real seams
		for range 200 {
			runtime.Gosched()
			if windows.GetCurrentThreadId() != before {
				t.Fatal("a goroutine mudou de thread: a fixacao do chamador foi desfeita")
			}
		}
	})
}
