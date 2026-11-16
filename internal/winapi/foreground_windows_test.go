//go:build windows

package winapi_test

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

type fakeFG struct {
	calls     []string
	hwnds     []uintptr
	iconic    bool
	fgResults []bool // SetForegroundWindow results in order; default false
	fg        uintptr
	fgThread  uint32
	self      uint32
	attachOK  bool
	bringOK   bool
	sfwN      int
}

func (f *fakeFG) rec(s string, h uintptr) {
	f.calls = append(f.calls, s)
	f.hwnds = append(f.hwnds, h)
}
func (f *fakeFG) IsIconic(h uintptr) bool     { f.rec("iconic", h); return f.iconic }
func (f *fakeFG) ShowWindow(h uintptr, c int) { f.rec("show", h) }
func (f *fakeFG) SetWindowPos(h, after uintptr) {
	if after == ^uintptr(0) {
		f.rec("topmost", h)
	} else {
		f.rec("notopmost", h)
	}
}
func (f *fakeFG) SetForegroundWindow(h uintptr) bool {
	f.rec("sfw", h)
	r := false
	if f.sfwN < len(f.fgResults) {
		r = f.fgResults[f.sfwN]
	}
	f.sfwN++
	return r
}
func (f *fakeFG) GetForegroundWindow() uintptr { f.rec("getfg", 0); return f.fg }
func (f *fakeFG) ThreadOf(h uintptr) uint32    { f.rec("threadof", h); return f.fgThread }
func (f *fakeFG) CurrentThread() uint32        { return f.self }
func (f *fakeFG) AttachThreadInput(a, b uint32, attach bool) bool {
	if attach {
		f.rec("attach", 0)
		return f.attachOK
	}
	f.rec("detach", 0)
	return true
}
func (f *fakeFG) BringWindowToTop(h uintptr) bool { f.rec("bring", h); return f.bringOK }

func seq(f *fakeFG) string {
	var out []string
	for _, c := range f.calls {
		if c != "getfg" && c != "threadof" {
			out = append(out, c)
		}
	}
	return strings.Join(out, ",")
}

func TestForceForegroundWith(t *testing.T) {
	const h = uintptr(0x1234)
	t.Run("CON-023/minimizada restaura antes do topmost", func(t *testing.T) {
		f := &fakeFG{iconic: true, fgResults: []bool{true}}
		if !winapi.ForceForegroundWith(f, h) {
			t.Fatal("want true")
		}
		if got := seq(f); got != "iconic,show,topmost,notopmost,sfw" {
			t.Fatalf("order: %s", got)
		}
	})
	t.Run("CON-023/nao minimizada nao restaura", func(t *testing.T) {
		f := &fakeFG{fgResults: []bool{true}}
		winapi.ForceForegroundWith(f, h)
		if got := seq(f); got != "iconic,topmost,notopmost,sfw" {
			t.Fatalf("order: %s", got)
		}
	})
	t.Run("CON-023/recusa com attach: attach, bring, sfw, detach", func(t *testing.T) {
		f := &fakeFG{fg: 0x99, fgThread: 7, self: 3, attachOK: true, fgResults: []bool{false, true}}
		if !winapi.ForceForegroundWith(f, h) {
			t.Fatal("want true")
		}
		if got := seq(f); got != "iconic,topmost,notopmost,sfw,attach,bring,sfw,detach" {
			t.Fatalf("order: %s", got)
		}
	})
	t.Run("CON-023/detach sempre executado mesmo se Windows recusa", func(t *testing.T) {
		f := &fakeFG{fg: 0x99, fgThread: 7, self: 3, attachOK: true}
		if winapi.ForceForegroundWith(f, h) {
			t.Fatal("want false (refused, no error)")
		}
		if !strings.HasSuffix(seq(f), "attach,bring,sfw,detach") {
			t.Fatalf("order: %s", seq(f))
		}
	})
	t.Run("CON-023/attach falho devolve false sem detach", func(t *testing.T) {
		f := &fakeFG{fg: 0x99, fgThread: 7, self: 3, attachOK: false}
		if winapi.ForceForegroundWith(f, h) {
			t.Fatal("want false")
		}
		if strings.Contains(seq(f), "detach") {
			t.Fatalf("detach without attach: %s", seq(f))
		}
	})
	t.Run("CON-023/sem janela em primeiro plano devolve false", func(t *testing.T) {
		f := &fakeFG{}
		if winapi.ForceForegroundWith(f, h) {
			t.Fatal("want false")
		}
		if strings.Contains(seq(f), "attach") {
			t.Fatalf("unexpected attach: %s", seq(f))
		}
	})
	t.Run("CON-023/ja em primeiro plano devolve true", func(t *testing.T) {
		f := &fakeFG{fg: h}
		if !winapi.ForceForegroundWith(f, h) {
			t.Fatal("want true")
		}
	})
	t.Run("CON-023/mesma thread dispensa attach", func(t *testing.T) {
		f := &fakeFG{fg: 0x99, fgThread: 3, self: 3, bringOK: true, fgResults: []bool{false, true}}
		if !winapi.ForceForegroundWith(f, h) {
			t.Fatal("want true")
		}
		if strings.Contains(seq(f), "attach") {
			t.Fatalf("unexpected attach: %s", seq(f))
		}
	})
	t.Run("CON-023/nenhuma chamada com hwnd 0 exceto consultas e attach", func(t *testing.T) {
		f := &fakeFG{iconic: true, fg: 0x99, fgThread: 7, self: 3, attachOK: true, fgResults: []bool{false, true}}
		winapi.ForceForegroundWith(f, h)
		for i, c := range f.calls {
			if c == "getfg" || c == "attach" || c == "detach" {
				continue
			}
			if f.hwnds[i] == 0 {
				t.Fatalf("call %s with hwnd 0", c)
			}
		}
	})
}

var (
	u32              = windows.NewLazySystemDLL("user32.dll")
	pCreateWindowExW = u32.NewProc("CreateWindowExW")
	pDestroyWindow   = u32.NewProc("DestroyWindow")
)

func mkWin(t *testing.T, w, h int, style uintptr) uintptr {
	cls, _ := windows.UTF16PtrFromString("STATIC")
	r, _, e := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), 0, style, 0, 0, uintptr(w), uintptr(h), 0, 0, 0, 0)
	if r == 0 {
		t.Fatalf("CreateWindowEx: %v", e)
	}
	t.Cleanup(func() { pDestroyWindow.Call(r) })
	return r
}

func TestMainWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const wsVisible, wsPopup = 0x10000000, 0x80000000

	t.Run("CON-023/sem janela visivel devolve erro e zero", func(t *testing.T) {
		mkWin(t, 900, 900, wsPopup) // invisible
		h, err := winapi.MainWindow()
		if err == nil || h != 0 {
			t.Fatalf("want error and 0, got %#x, %v", h, err)
		}
	})
	t.Run("CON-023/escolhe a visivel de maior area e ignora invisivel", func(t *testing.T) {
		mkWin(t, 1000, 1000, wsPopup) // invisible, biggest
		small := mkWin(t, 100, 100, wsPopup|wsVisible)
		big := mkWin(t, 300, 200, wsPopup|wsVisible)
		h, err := winapi.MainWindow()
		if err != nil || h != big || h == small {
			t.Fatalf("got %#x err=%v want %#x", h, err, big)
		}
	})
	t.Run("CON-023/chamadas repetidas usam o mesmo callback", func(t *testing.T) {
		big := mkWin(t, 300, 200, wsPopup|wsVisible)
		for i := 0; i < 3000; i++ {
			h, err := winapi.MainWindow()
			if err != nil || h != big {
				t.Fatalf("iter %d: %#x %v", i, h, err)
			}
		}
	})
}
