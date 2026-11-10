//go:build windows

// White-box tests for Overlay (OVL-002, OVL-005, OVL-006). Show/Hide are
// asynchronous (they post to the window's thread), so visibility is awaited
// with a bounded poll; no behaviour is driven by wall-clock time.
//
// Internal contract required from the implementation:
//
//	func (o *Overlay) windowHandle() windows.HWND
//
// It returns the notice window created by Start (0 when Start failed or after
// Close). The window class is "ArcadiaNotice-<N>" with N taken from classSeq.
package winapi

import (
	"fmt"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"go.uber.org/goleak"
	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/feedback"
)

const (
	ovlWsPopup = 0x80000000

	ovlWsExTopmost     = 0x00000008
	ovlWsExTransparent = 0x00000020
	ovlWsExToolWindow  = 0x00000080
	ovlWsExLayered     = 0x00080000
	ovlWsExNoActivate  = 0x08000000

	ovlGwlStyle   = -16
	ovlGwlExStyle = -20

	ovlDesktopSwitchDesktop = 0x0100
)

var (
	ovlProcGetFocusInfo     = user32.NewProc("GetGUIThreadInfo")
	ovlProcOpenInputDesktop = user32.NewProc("OpenInputDesktop")
	ovlProcCloseDesktop     = user32.NewProc("CloseDesktop")
	ovlProcGetClassNameW    = user32.NewProc("GetClassNameW")
	ovlProcGetClassInfoExW  = user32.NewProc("GetClassInfoExW")
)

type ovlGuiThreadInfo struct {
	size        uint32
	flags       uint32
	hwndActive  uintptr
	hwndFocus   uintptr
	hwndCapture uintptr
	hwndMenu    uintptr
	hwndMove    uintptr
	hwndCaret   uintptr
	rcCaret     [4]int32
}

// requireInteractiveDesktop skips the test when there is no interactive desktop
// (service session, locked workstation, CI).
func requireInteractiveDesktop(t *testing.T) {
	t.Helper()
	h, _, _ := ovlProcOpenInputDesktop.Call(0, 0, ovlDesktopSwitchDesktop)
	if h == 0 {
		t.Skip("sem desktop interativo (OpenInputDesktop falhou)")
	}
	ovlProcCloseDesktop.Call(h)
}

func ovlWindowLong(h windows.HWND, index int) uintptr {
	r, _, _ := procGetWindowLongPtrW.Call(uintptr(h), uintptr(index))
	return r
}

func ovlVisible(h windows.HWND) bool {
	r, _, _ := procIsWindowVisible.Call(uintptr(h))
	return r != 0
}

// ovlWaitVisible polls until the window's visibility equals want.
func ovlWaitVisible(t *testing.T, h windows.HWND, want bool) {
	t.Helper()
	deadline := time.After(waitTimeout)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for ovlVisible(h) != want {
		select {
		case <-deadline:
			t.Fatalf("visibilidade da janela %#x não chegou a %v", h, want)
		case <-tick.C:
		}
	}
}

func ovlForeground() (fg, focus uintptr) {
	fg, _, _ = procGetForegroundWindow.Call()
	info := ovlGuiThreadInfo{}
	info.size = uint32(unsafe.Sizeof(info))
	if r, _, _ := ovlProcGetFocusInfo.Call(0, uintptr(unsafe.Pointer(&info))); r != 0 {
		focus = info.hwndFocus
	}
	return fg, focus
}

func ovlClassName(h windows.HWND) string {
	buf := make([]uint16, 256)
	n, _, _ := ovlProcGetClassNameW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func ovlClassRegistered(name string) bool {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &instance); err != nil {
		return false
	}
	wc := wndClassExW{}
	wc.size = uint32(unsafe.Sizeof(wc))
	r, _, _ := ovlProcGetClassInfoExW.Call(uintptr(instance), uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&wc)))
	return r != 0
}

// startOverlay registers the goroutine-leak check first, then Close, so that at
// cleanup (LIFO) Close runs before goleak verifies.
func startOverlay(t *testing.T) *Overlay {
	t.Helper()
	ignore := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, ignore) })

	o := NewOverlay(testLogger())
	if err := o.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := o.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if h := o.windowHandle(); h == 0 || !isWindow(h) {
		t.Fatalf("windowHandle() = %#x não é uma janela válida após Start", h)
	}
	return o
}

func TestOverlayWindow(t *testing.T) {
	t.Run("OVL-002/estilos WS_POPUP e estendidos TOPMOST, NOACTIVATE, TOOLWINDOW, LAYERED e TRANSPARENT; invisível antes do primeiro Show", func(t *testing.T) {
		o := startOverlay(t)
		h := o.windowHandle()

		style := ovlWindowLong(h, ovlGwlStyle)
		if style&ovlWsPopup == 0 {
			t.Errorf("estilo %#x sem WS_POPUP", style)
		}
		ex := ovlWindowLong(h, ovlGwlExStyle)
		for _, f := range []struct {
			name string
			bit  uintptr
		}{
			{"WS_EX_TOPMOST", ovlWsExTopmost},
			{"WS_EX_NOACTIVATE", ovlWsExNoActivate},
			{"WS_EX_TOOLWINDOW", ovlWsExToolWindow},
			{"WS_EX_LAYERED", ovlWsExLayered},
			{"WS_EX_TRANSPARENT", ovlWsExTransparent},
		} {
			if ex&f.bit == 0 {
				t.Errorf("estilo estendido %#x sem %s", ex, f.name)
			}
		}
		if ovlVisible(h) {
			t.Error("janela visível antes do primeiro Show")
		}
	})

	t.Run("OVL-002/Show deixa a janela visível e a janela em primeiro plano e a do foco continuam as mesmas", func(t *testing.T) {
		requireInteractiveDesktop(t)
		o := startOverlay(t)
		h := o.windowHandle()

		fg0, focus0 := ovlForeground()
		o.Show(feedback.NoticePaused)
		ovlWaitVisible(t, h, true)
		fg1, focus1 := ovlForeground()

		if fg1 != fg0 {
			t.Errorf("janela em primeiro plano mudou de %#x para %#x", fg0, fg1)
		}
		if focus1 != focus0 {
			t.Errorf("janela com foco mudou de %#x para %#x", focus0, focus1)
		}
		if fg1 == uintptr(h) || focus1 == uintptr(h) {
			t.Errorf("o aviso %#x foi ativado (primeiro plano %#x, foco %#x)", h, fg1, focus1)
		}
	})

	t.Run("OVL-002/Hide esconde a janela", func(t *testing.T) {
		o := startOverlay(t)
		h := o.windowHandle()

		o.Show(feedback.NoticeResumed)
		ovlWaitVisible(t, h, true)
		o.Hide()
		ovlWaitVisible(t, h, false)
	})

	t.Run("OVL-005/Close esconde e destrói a janela, desregistra a classe e espera a goroutine (goleak)", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()

		o := NewOverlay(testLogger())
		if err := o.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		h := o.windowHandle()
		if h == 0 || !isWindow(h) {
			t.Fatalf("windowHandle() = %#x não é uma janela válida após Start", h)
		}
		class := ovlClassName(h)
		if class == "" || !ovlClassRegistered(class) {
			t.Fatalf("classe %q da janela não está registrada após Start", class)
		}
		o.Show(feedback.NoticePaused)
		ovlWaitVisible(t, h, true)

		if err := o.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if isWindow(h) {
			t.Errorf("janela %#x ainda existe após Close", h)
		}
		if ovlClassRegistered(class) {
			t.Errorf("classe %q ainda registrada após Close", class)
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("OVL-005/Show depois do Close não tem efeito nem erro", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()

		o := NewOverlay(testLogger())
		if err := o.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		h := o.windowHandle()
		if err := o.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			o.Show(feedback.NoticePaused)
			o.Hide()
		}()
		select {
		case <-done:
		case <-time.After(waitTimeout):
			t.Fatal("Show/Hide depois do Close bloquearam")
		}
		if isWindow(h) || ovlVisible(h) {
			t.Errorf("janela %#x existe/visível após Show depois do Close", h)
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("OVL-005/segunda chamada a Close não tem efeito", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()

		o := NewOverlay(testLogger())
		if err := o.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if err := o.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := o.Close(); err != nil {
			t.Fatalf("segundo Close: %v", err)
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("OVL-005/100 ciclos de Show/Hide: GetGuiResources(USEROBJECTS e GDIOBJECTS) cresce <= 2 e handles <= h0 + 2", func(t *testing.T) {
		o := startOverlay(t)
		h := o.windowHandle()

		cycle := func(i int) {
			n := feedback.NoticePaused
			if i%2 == 1 {
				n = feedback.NoticeResumed
			}
			o.Show(n)
			ovlWaitVisible(t, h, true)
			o.Hide()
			ovlWaitVisible(t, h, false)
		}
		settle := func() uint32 {
			deadline := time.Now().Add(3 * time.Second)
			runtime.GC()
			last := powerProcessHandleCount(t)
			stable := 0
			for stable < 5 && time.Now().Before(deadline) {
				time.Sleep(20 * time.Millisecond)
				runtime.GC()
				cur := powerProcessHandleCount(t)
				if cur == last {
					stable++
				} else {
					stable, last = 0, cur
				}
			}
			return last
		}

		// Leak detector: block A (100 cycles) absorbs one-off growth (lazy loads,
		// runtime OS threads); block B (100 more cycles) must grow ~nothing. A
		// leak of even 1 object per cycle would add ~100 in block B.
		cycle(0) // warm-up: lazy DLL loads, first paint, font/brush selection
		for i := 0; i < 100; i++ {
			cycle(i)
		}
		// The OS/runtime warm-up (per-thread caches, USER/handle bookkeeping) is
		// bounded but takes a few hundred cycles to plateau. Measure successive
		// blocks of 100 cycles: the process must reach a block whose growth is
		// within tolerance within maxBlocks. A real leak (>= 1 object per cycle)
		// adds ~100 per block, never plateaus, and fails every block.
		const maxBlocks = 8
		var lastMsg string
		plateau := false
		for b := 0; b < maxBlocks && !plateau; b++ {
			h1 := settle()
			threads1 := powerThreadsCreated()
			gdi1 := guiResources(t, grGdiObjects)
			user1 := guiResources(t, grUserObjects)
			for i := 0; i < 100; i++ {
				cycle(i)
			}
			hLimit := func() uint32 {
				return h1 + 2 + powerHandlesPerThread*uint32(powerThreadsCreated()-threads1)
			}
			h2 := settle()
			gdi2 := guiResources(t, grGdiObjects)
			user2 := guiResources(t, grUserObjects)
			if h2 <= hLimit() && gdi2 <= gdi1+2 && user2 <= user1+2 {
				plateau = true
				break
			}
			lastMsg = fmt.Sprintf("bloco %d: handles %d -> %d (limite %d), GDI %d -> %d, USER %d -> %d",
				b, h1, h2, hLimit(), gdi1, gdi2, user1, user2)
		}
		if !plateau {
			t.Errorf("recursos nao estabilizam em %d blocos de 100 ciclos (vazamento?); ultimo %s", maxBlocks, lastMsg)
		}
	})

	t.Run("OVL-006/Start com falha injetada (classe já registrada) devolve erro, nada fica rodando e Show/Hide/Close seguintes são no-op sem erro", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()

		var instance windows.Handle
		if err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &instance); err != nil {
			t.Fatalf("GetModuleHandleEx: %v", err)
		}
		// Pre-register the class name the overlay is about to pick, so its own
		// RegisterClassExW fails with ERROR_CLASS_ALREADY_EXISTS.
		name, err := windows.UTF16PtrFromString(fmt.Sprintf("ArcadiaNotice-%d", classSeq.Load()+1))
		if err != nil {
			t.Fatal(err)
		}
		wc := wndClassExW{wndProc: procDefWindowProcW.Addr(), instance: instance, className: name}
		wc.size = uint32(unsafe.Sizeof(wc))
		if r, _, e := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			t.Fatalf("RegisterClassExW de preparo: %v", e)
		}
		t.Cleanup(func() {
			procUnregisterClassW.Call(uintptr(unsafe.Pointer(name)), uintptr(instance))
		})

		o := NewOverlay(testLogger())
		if err := o.Start(); err == nil {
			_ = o.Close()
			t.Fatal("Start devolveu nil apesar da classe duplicada")
		}
		if h := o.windowHandle(); h != 0 {
			t.Errorf("windowHandle() = %#x após Start com erro, quer 0", h)
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			o.Show(feedback.NoticePaused)
			o.Hide()
		}()
		select {
		case <-done:
		case <-time.After(waitTimeout):
			t.Fatal("Show/Hide após Start com erro bloquearam")
		}
		if err := o.Close(); err != nil {
			t.Fatalf("Close após Start com erro: %v", err)
		}
		if err := o.Close(); err != nil {
			t.Fatalf("segundo Close após Start com erro: %v", err)
		}
		goleak.VerifyNone(t, ignore)
	})
}
