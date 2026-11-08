//go:build windows

// White-box tests: they need the watcher's hidden window handle to deliver
// WM_QUERYENDSESSION / WM_ENDSESSION with SendMessageW, as Windows would.
//
// Internal contract required from the implementation:
//
//	func (w *SessionEndWatcher) windowHandle() windows.HWND
//
// It returns the hidden top-level window created by Start; it must be valid (and
// safe to call from any goroutine) once Start has returned a nil error.
package winapi

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

const (
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmClose           = 0x0010
	gaParent          = 1 // GA_PARENT for GetAncestor

	// waitTimeout bounds every wait on a real event in these tests. It is only a
	// safety net against hangs; behaviour is driven by FakeClock.
	waitTimeout = 5 * time.Second

	endSessionTimeout = 2 * time.Second
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	procSendMessageW    = user32.NewProc("SendMessageW")
	procIsWindow        = user32.NewProc("IsWindow")
	procIsWindowVisible = user32.NewProc("IsWindowVisible")
	procGetAncestor     = user32.NewProc("GetAncestor")
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func isWindow(h windows.HWND) bool {
	r, _, _ := procIsWindow.Call(uintptr(h))
	return r != 0
}

func sendMessage(h windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(uintptr(h), uintptr(msg), wParam, lParam)
	return r
}

// captureHandler is a slog.Handler that records every record it receives.
type captureHandler struct {
	mu      *sync.Mutex
	records *[]slog.Record
}

func newCaptureLogger() (*slog.Logger, *captureHandler) {
	h := &captureHandler{mu: &sync.Mutex{}, records: &[]slog.Record{}}
	return slog.New(h), h
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.records = append(*h.records, r.Clone())
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) count(level slog.Level) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range *h.records {
		if r.Level == level {
			n++
		}
	}
	return n
}

// startWatcher starts a watcher on clock and registers Close at cleanup.
func startWatcher(t *testing.T, clock *domain.FakeClock) (*SessionEndWatcher, <-chan struct{}) {
	t.Helper()
	return startWatcherLog(t, clock, testLogger())
}

func startWatcherLog(t *testing.T, clock *domain.FakeClock, log *slog.Logger) (*SessionEndWatcher, <-chan struct{}) {
	t.Helper()
	w := NewSessionEndWatcher(clock, log)
	ended, err := w.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	h := w.windowHandle()
	if h == 0 || !isWindow(h) {
		t.Fatalf("windowHandle() = %#x não é uma janela válida após Start", h)
	}
	return w, ended
}

// pendingSend is a SendMessageW call running in a goroutine owned by the test.
// done is closed when the window procedure has returned.
type pendingSend struct {
	done chan struct{}
}

// sendAsync delivers msg with SendMessageW (synchronous) from a goroutine owned
// by the test. At cleanup (before the watcher's Close) the handler is released
// (ShutdownDone + clock far ahead) and the goroutine is awaited.
func sendAsync(t *testing.T, w *SessionEndWatcher, clock *domain.FakeClock, msg uint32, wParam uintptr) *pendingSend {
	t.Helper()
	h := w.windowHandle()
	p := &pendingSend{done: make(chan struct{})}
	go func() {
		defer close(p.done)
		sendMessage(h, msg, wParam, 0)
	}()
	t.Cleanup(func() {
		w.ShutdownDone()
		clock.Advance(time.Hour)
		select {
		case <-p.done:
		case <-time.After(waitTimeout):
			t.Errorf("SendMessage(%#x) não retornou após liberar o handler", msg)
		}
	})
	return p
}

func waitSignal(t *testing.T, ended <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ended:
	case <-time.After(waitTimeout):
		t.Fatalf("%s: canal de fim de sessão não sinalizou", what)
	}
}

func waitReturn(t *testing.T, p *pendingSend, what string) {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(waitTimeout):
		t.Fatalf("%s: handler não retornou", what)
	}
}

// notReturned is a non-blocking check that the handler is still running.
func notReturned(t *testing.T, p *pendingSend, what string) {
	t.Helper()
	select {
	case <-p.done:
		t.Fatalf("%s: handler de WM_ENDSESSION retornou cedo demais", what)
	default:
	}
}

func newClock() *domain.FakeClock {
	return domain.NewFakeClock(time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC))
}

// Not parallel: the goleak case must observe the package's goroutines alone.
func TestSessionEndWatcher(t *testing.T) {
	t.Run("AGT-006/janela top-level oculta (não message-only)", func(t *testing.T) {
		clock := newClock()
		w, _ := startWatcher(t, clock)
		h := w.windowHandle()
		if r, _, _ := procIsWindowVisible.Call(uintptr(h)); r != 0 {
			t.Fatal("janela do watcher está visível")
		}
		parent, _, _ := procGetAncestor.Call(uintptr(h), gaParent)
		if windows.HWND(parent) != windows.GetDesktopWindow() {
			t.Fatalf("pai da janela = %#x, quer a área de trabalho %#x (HWND_MESSAGE não recebe WM_ENDSESSION)",
				parent, windows.GetDesktopWindow())
		}
	})

	t.Run("AGT-006/WM_ENDSESSION TRUE sinaliza o canal", func(t *testing.T) {
		clock := newClock()
		w, ended := startWatcher(t, clock)

		// Windows asks first; the watcher must not veto the session end.
		if r := sendMessage(w.windowHandle(), wmQueryEndSession, 0, 0); r == 0 {
			t.Fatal("WM_QUERYENDSESSION retornou FALSE (vetou o fim da sessão)")
		}
		done := sendAsync(t, w, clock, wmEndSession, 1)
		// The signal must arrive while the handler is still blocked (before
		// ShutdownDone), otherwise the agent would never start its shutdown.
		waitSignal(t, ended, "WM_ENDSESSION TRUE")

		w.ShutdownDone()
		waitReturn(t, done, "após ShutdownDone")
	})

	t.Run("AGT-006/WM_ENDSESSION FALSE não sinaliza", func(t *testing.T) {
		clock := newClock()
		w, ended := startWatcher(t, clock)

		done := sendAsync(t, w, clock, wmEndSession, 0)
		// wParam = FALSE: the session is not ending; the handler returns without
		// waiting for ShutdownDone or the clock.
		waitReturn(t, done, "WM_ENDSESSION FALSE")
		select {
		case <-ended:
			t.Fatal("canal sinalizou com WM_ENDSESSION wParam = FALSE")
		default:
		}
	})

	t.Run("AGT-006/handler espera ShutdownDone", func(t *testing.T) {
		clock := newClock()
		w, ended := startWatcher(t, clock)

		done := sendAsync(t, w, clock, wmEndSession, 1)
		waitSignal(t, ended, "WM_ENDSESSION TRUE")

		ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
		defer cancel()
		if err := clock.BlockUntilWaiters(ctx, 1); err != nil {
			t.Fatalf("handler não registrou o timeout de 2 s no relógio: %v", err)
		}
		notReturned(t, done, "sem ShutdownDone e sem avanço do relógio")

		w.ShutdownDone() // clock never advanced
		waitReturn(t, done, "após ShutdownDone")
	})

	t.Run("AGT-006/handler retorna após 2 s do relógio sem ShutdownDone", func(t *testing.T) {
		clock := newClock()
		w, ended := startWatcher(t, clock)

		done := sendAsync(t, w, clock, wmEndSession, 1)
		waitSignal(t, ended, "WM_ENDSESSION TRUE")

		ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
		defer cancel()
		if err := clock.BlockUntilWaiters(ctx, 1); err != nil {
			t.Fatalf("handler não registrou o timeout de 2 s no relógio: %v", err)
		}

		clock.Advance(endSessionTimeout - time.Nanosecond)
		notReturned(t, done, "em 2 s - 1 ns")

		clock.Advance(time.Nanosecond)
		waitReturn(t, done, "após 2 s do relógio")
	})

	t.Run("AGT-006/Close encerra a goroutine (goleak)", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()

		clock := newClock()
		w := NewSessionEndWatcher(clock, testLogger())
		if _, err := w.Start(context.Background()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		h := w.windowHandle()
		if err := w.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if isWindow(h) {
			t.Fatalf("janela %#x ainda existe após Close (DestroyWindow não chamado?)", h)
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("AGT-006/Close idempotente", func(t *testing.T) {
		clock := newClock()
		w := NewSessionEndWatcher(clock, testLogger())
		if _, err := w.Start(context.Background()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("primeiro Close: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("segundo Close: %v", err)
		}
		w.ShutdownDone() // no-op without a pending WM_ENDSESSION, also after Close
		w.ShutdownDone()
	})

	t.Run("AGT-006/WM_CLOSE externo não derruba o watcher", func(t *testing.T) {
		clock := newClock()
		log, logs := newCaptureLogger()
		w, ended := startWatcherLog(t, clock, log)
		h := w.windowHandle()

		// E.g. taskkill without /F. SendMessageW is synchronous: the handler has
		// run (and logged) when it returns.
		sendMessage(h, wmClose, 0, 0)
		if n := logs.count(slog.LevelWarn); n == 0 {
			t.Error("WM_CLOSE externo não gerou log Warn")
		}

		// Still alive: a session end after the stray WM_CLOSE must be seen.
		done := sendAsync(t, w, clock, wmEndSession, 1)
		waitSignal(t, ended, "WM_ENDSESSION TRUE após WM_CLOSE externo")
		w.ShutdownDone()
		waitReturn(t, done, "após ShutdownDone")
		if !isWindow(h) {
			t.Fatalf("janela %#x destruída por WM_CLOSE externo", h)
		}
	})

	t.Run("AGT-006/ctx cancelado encerra o loop e Close retorna nil", func(t *testing.T) {
		clock := newClock()
		w := NewSessionEndWatcher(clock, testLogger())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if _, err := w.Start(ctx); err != nil {
			t.Fatalf("Start: %v", err)
		}
		h := w.windowHandle()
		cancel()

		// Nothing observable signals the loop exit besides the window going
		// away, so poll IsWindow with a large guard.
		guard := time.After(waitTimeout)
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		for isWindow(h) {
			select {
			case <-tick.C:
			case <-guard:
				_ = w.Close()
				t.Fatalf("janela %#x ainda existe %v após cancelar o ctx", h, waitTimeout)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatalf("Close após ctx cancelado: %v", err)
		}
	})

	t.Run("AGT-006/Close com handler pendente retorna após ShutdownDone", func(t *testing.T) {
		clock := newClock()
		w, ended := startWatcher(t, clock)

		done := sendAsync(t, w, clock, wmEndSession, 1)
		waitSignal(t, ended, "WM_ENDSESSION TRUE")
		ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
		defer cancel()
		if err := clock.BlockUntilWaiters(ctx, 1); err != nil {
			t.Fatalf("handler não registrou o timeout de 2 s no relógio: %v", err)
		}

		closed := make(chan error, 1)
		go func() { closed <- w.Close() }()
		// Absence check: a short real window, never used to drive behaviour.
		select {
		case err := <-closed:
			t.Fatalf("Close retornou (%v) com handler de WM_ENDSESSION pendente", err)
		case <-time.After(50 * time.Millisecond):
		}
		notReturned(t, done, "Close pendente, sem ShutdownDone")

		w.ShutdownDone() // clock never advanced
		select {
		case err := <-closed:
			if err != nil {
				t.Fatalf("Close: %v", err)
			}
		case <-time.After(waitTimeout):
			t.Fatal("Close não retornou após ShutdownDone")
		}
		waitReturn(t, done, "após ShutdownDone")
	})
}
