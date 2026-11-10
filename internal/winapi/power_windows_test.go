//go:build windows

// White-box tests for PowerWatcher (PWR-001..PWR-004, PWR-006). They deliver
// WM_POWERBROADCAST to the watcher's hidden window with SendMessageW, as Windows
// would; no interactive desktop is needed.
//
// Internal contract required from the implementation:
//
//	func (w *PowerWatcher) windowHandle() windows.HWND
//
// plus: the window class is named "ArcadiaPowerWatcher-<N>" with N taken from the
// package-level classSeq counter (same scheme as SessionEndWatcher), which the
// duplicate-class case uses to provoke a RegisterClassExW failure.
package winapi

import (
	"fmt"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"time"
	"unsafe"

	"go.uber.org/goleak"
	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

const (
	wmPowerBroadcast = 0x0218

	pbtAPMSuspend           = 0x0004
	pbtAPMResumeSuspend     = 0x0007
	pbtAPMResumeAutomatic   = 0x0012
	pbtAPMPowerStatusChange = 0x000A
	pbtAPMBatteryLow        = 0x0009
	pbtAPMResumeCritical    = 0x0006
	pbtPowerSettingChange   = 0x8013

	grGdiObjects  = 0
	grUserObjects = 1

	// powerHandlesPerThread mirrors internal/tracker/main_windows_test.go: the Go
	// runtime costs ~6 process handles per OS thread it creates.
	powerHandlesPerThread = 6
)

type powerEvent struct {
	kind        string // "suspend" or "wake"
	at          time.Time
	userPresent bool
}

type powerRecorder struct {
	mu     sync.Mutex
	events []powerEvent
}

func (r *powerRecorder) callbacks() PowerCallbacks {
	return PowerCallbacks{
		OnSuspend: func(at time.Time) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.events = append(r.events, powerEvent{kind: "suspend", at: at})
		},
		OnWake: func(at time.Time, userPresent bool) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.events = append(r.events, powerEvent{kind: "wake", at: at, userPresent: userPresent})
		},
	}
}

func (r *powerRecorder) snapshot() []powerEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]powerEvent(nil), r.events...)
}

// startPower registers the goroutine-leak check first, then Close, so that at
// cleanup (LIFO) Close runs before goleak verifies.
func startPower(t *testing.T, cb PowerCallbacks) (*PowerWatcher, *domain.FakeClock) {
	t.Helper()
	ignore := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, ignore) })

	clock := newClock()
	w := NewPowerWatcher(clock, testLogger(), cb)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if h := w.windowHandle(); h == 0 || !isWindow(h) {
		t.Fatalf("windowHandle() = %#x não é uma janela válida após Start", h)
	}
	return w, clock
}

func guiResources(t *testing.T, flag uintptr) uint32 {
	t.Helper()
	r, _, _ := procGetGuiResources.Call(uintptr(windows.CurrentProcess()), flag)
	return uint32(r)
}

var powerProcGetProcessHandleCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")

func powerProcessHandleCount(t *testing.T) uint32 {
	t.Helper()
	var n uint32
	r, _, err := powerProcGetProcessHandleCount.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		t.Fatalf("GetProcessHandleCount: %v", err)
	}
	return n
}

func powerThreadsCreated() int { return pprof.Lookup("threadcreate").Count() }

func TestPowerWatcher(t *testing.T) {
	t.Run("PWR-001/PBT_APMSUSPEND chama OnSuspend uma vez com clock.Now() do recebimento e o procedimento devolve TRUE", func(t *testing.T) {
		rec := &powerRecorder{}
		w, c := startPower(t, rec.callbacks())

		c.Advance(37 * time.Second)
		want := c.Now()
		if r := sendMessage(w.windowHandle(), wmPowerBroadcast, pbtAPMSuspend, 0); r != 1 {
			t.Fatalf("procedimento devolveu %d, quer TRUE (1)", r)
		}
		ev := rec.snapshot()
		if len(ev) != 1 || ev[0].kind != "suspend" {
			t.Fatalf("eventos = %+v, quer exatamente um OnSuspend", ev)
		}
		if !ev[0].at.Equal(want) {
			t.Fatalf("at = %v, quer clock.Now() do recebimento %v", ev[0].at, want)
		}
	})

	t.Run("PWR-002/PBT_APMRESUMESUSPEND chama OnWake(at, true) uma vez", func(t *testing.T) {
		rec := &powerRecorder{}
		w, c := startPower(t, rec.callbacks())

		c.Advance(5 * time.Minute)
		want := c.Now()
		if r := sendMessage(w.windowHandle(), wmPowerBroadcast, pbtAPMResumeSuspend, 0); r != 1 {
			t.Fatalf("procedimento devolveu %d, quer TRUE (1)", r)
		}
		ev := rec.snapshot()
		if len(ev) != 1 || ev[0].kind != "wake" || !ev[0].userPresent {
			t.Fatalf("eventos = %+v, quer exatamente um OnWake(userPresent=true)", ev)
		}
		if !ev[0].at.Equal(want) {
			t.Fatalf("at = %v, quer %v", ev[0].at, want)
		}
	})

	t.Run("PWR-003/PBT_APMRESUMEAUTOMATIC chama OnWake(at, false) uma vez", func(t *testing.T) {
		rec := &powerRecorder{}
		w, c := startPower(t, rec.callbacks())

		c.Advance(90 * time.Second)
		want := c.Now()
		if r := sendMessage(w.windowHandle(), wmPowerBroadcast, pbtAPMResumeAutomatic, 0); r != 1 {
			t.Fatalf("procedimento devolveu %d, quer TRUE (1)", r)
		}
		ev := rec.snapshot()
		if len(ev) != 1 || ev[0].kind != "wake" || ev[0].userPresent {
			t.Fatalf("eventos = %+v, quer exatamente um OnWake(userPresent=false)", ev)
		}
		if !ev[0].at.Equal(want) {
			t.Fatalf("at = %v, quer %v", ev[0].at, want)
		}
	})

	t.Run("PWR-004/POWERSTATUSCHANGE, BATTERYLOW, RESUMECRITICAL e POWERSETTINGCHANGE não chamam callback e devolvem TRUE", func(t *testing.T) {
		rec := &powerRecorder{}
		w, _ := startPower(t, rec.callbacks())

		for _, tc := range []struct {
			name   string
			wParam uintptr
		}{
			{"PBT_APMPOWERSTATUSCHANGE", pbtAPMPowerStatusChange},
			{"PBT_APMBATTERYLOW", pbtAPMBatteryLow},
			{"PBT_APMRESUMECRITICAL", pbtAPMResumeCritical},
			{"PBT_POWERSETTINGCHANGE", pbtPowerSettingChange},
		} {
			if r := sendMessage(w.windowHandle(), wmPowerBroadcast, tc.wParam, 0); r != 1 {
				t.Errorf("%s: procedimento devolveu %d, quer TRUE (1)", tc.name, r)
			}
		}
		if ev := rec.snapshot(); len(ev) != 0 {
			t.Fatalf("callbacks chamados para eventos ignorados: %+v", ev)
		}
	})

	t.Run("PWR-006/Close termina o laço, destrói janela e classe antes de retornar (goleak) e a segunda chamada não tem efeito", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()

		w := NewPowerWatcher(newClock(), testLogger(), PowerCallbacks{})
		if err := w.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		h := w.windowHandle()
		if h == 0 || !isWindow(h) {
			t.Fatalf("windowHandle() = %#x não é uma janela válida após Start", h)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if isWindow(h) {
			t.Fatalf("janela %#x ainda existe após Close", h)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("segundo Close: %v", err)
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("PWR-006/20 ciclos de Start/Close: GetProcessHandleCount <= h0 + 2 e GetGuiResources não cresce mais que 2", func(t *testing.T) {
		cycle := func() {
			w := NewPowerWatcher(newClock(), testLogger(), PowerCallbacks{})
			if err := w.Start(); err != nil {
				t.Fatalf("Start: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
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

		cycle() // warm-up: lazy DLL loads, shared wndProc callback, first locked thread
		h0 := settle()
		threads0 := powerThreadsCreated()
		gdi0 := guiResources(t, grGdiObjects)
		user0 := guiResources(t, grUserObjects)

		for i := 0; i < 20; i++ {
			cycle()
		}

		deadline := time.Now().Add(5 * time.Second)
		for {
			runtime.GC()
			limit := h0 + 2 + powerHandlesPerThread*uint32(powerThreadsCreated()-threads0)
			got := powerProcessHandleCount(t)
			if got <= limit {
				break
			}
			if time.Now().After(deadline) {
				t.Errorf("handles = %d, quer <= %d (h0 %d + 2 + %d por nova thread do SO, %d novas)",
					got, limit, h0, powerHandlesPerThread, powerThreadsCreated()-threads0)
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if g := guiResources(t, grGdiObjects); g > gdi0+2 {
			t.Errorf("objetos GDI = %d, quer <= %d", g, gdi0+2)
		}
		if u := guiResources(t, grUserObjects); u > user0+2 {
			t.Errorf("objetos USER = %d, quer <= %d", u, user0+2)
		}
	})

	t.Run("PWR-006/Start com erro injetado (classe duplicada) não deixa goroutine viva", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()

		var instance windows.Handle
		if err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &instance); err != nil {
			t.Fatalf("GetModuleHandleEx: %v", err)
		}
		// Pre-register the class name the watcher is about to pick, so its own
		// RegisterClassExW fails with ERROR_CLASS_ALREADY_EXISTS.
		name, err := windows.UTF16PtrFromString(fmt.Sprintf("ArcadiaPowerWatcher-%d", classSeq.Load()+1))
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

		w := NewPowerWatcher(newClock(), testLogger(), PowerCallbacks{})
		if err := w.Start(); err == nil {
			_ = w.Close()
			t.Fatal("Start devolveu nil apesar da classe duplicada")
		}
		if err := w.Close(); err != nil {
			t.Fatalf("Close após Start com erro: %v", err)
		}
		goleak.VerifyNone(t, ignore)
	})
}
