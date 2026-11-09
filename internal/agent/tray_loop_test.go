package agent

// Moved from cmd/agent in T05 of docs/specs/single-exe/tasks.md (names kept).
// Tests for the T10 review (design.md "## cmd/agent", revisão da T10 de
// 2026-11-08): the portable tray loop must end on Ctrl+C / end of session even
// when the systray onReady never runs, and log an Error if the tray does not
// initialize within 30 s.
//
// T06 of docs/specs/single-exe/tasks.md: trayLoop takes trayLoopDeps and
// returns the ShutdownReason (EXE-016) and also ends on IPCFailed (EXE-026).

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// recordHandler is a concurrency-safe slog.Handler that keeps every record and
// signals each one on notify (non-blocking).
type recordHandler struct {
	mu      sync.Mutex
	records []slog.Record
	notify  chan slog.Level
}

func newRecordHandler() *recordHandler {
	return &recordHandler{notify: make(chan slog.Level, 64)}
}

func (h *recordHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r.Clone())
	h.mu.Unlock()
	select {
	case h.notify <- r.Level:
	default:
	}
	return nil
}

func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordHandler) count(level slog.Level) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.Level == level {
			n++
		}
	}
	return n
}

// waitLevel waits until a record at level is logged.
func (h *recordHandler) waitLevel(t *testing.T, level slog.Level) {
	t.Helper()
	timeout := time.After(testTimeout)
	for {
		if h.count(level) > 0 {
			return
		}
		select {
		case <-h.notify:
		case <-timeout:
			t.Fatalf("no %s record logged within %v", level, testTimeout)
		}
	}
}

// trayHarness runs trayLoop in a goroutine with fake callbacks that count calls.
type trayHarness struct {
	clock  *domain.FakeClock
	logs   *recordHandler
	cancel context.CancelFunc

	// ready is unbuffered: a send returns only once the loop took the items,
	// so after deliverReady the loop is in its post-ready select.
	ready      chan trayItems
	sessionEnd chan struct{}
	stop       chan struct{}
	open       chan struct{}
	quitItem   chan struct{}
	// autostartItem is the "Iniciar com o Windows" click channel (T07).
	autostartItem chan struct{}

	ipcFailed chan struct{} // closed by the test to signal EXE-026

	quitCalls, quitBeforeReadyCalls, openCalls atomic.Int32
	openedCh                                   chan struct{}

	// T07: OnToggle returns toggleResult; calls are recorded in order in
	// toggleEvents ("toggle", "checked=true", "checked=false") and each
	// SetChecked is signalled on checkedCh.
	toggleResult bool
	toggleMu     sync.Mutex
	toggleEvents []string
	checkedCh    chan bool

	// reason is written by the loop goroutine before done is closed; read it
	// only after waitDone.
	reason ShutdownReason
	done   chan struct{}
}

type trayOpts struct {
	nilSessionEnd bool
	nilIPCFailed  bool
	cancelFirst   bool // cancel ctx before trayLoop starts
	toggleResult  bool // value returned by OnToggle (T07)
}

func startTray(t *testing.T, o trayOpts) *trayHarness {
	t.Helper()
	clock := testClock()
	h := &trayHarness{
		clock:      clock,
		logs:       newRecordHandler(),
		ready:      make(chan trayItems),
		sessionEnd: make(chan struct{}, 1),
		stop:       make(chan struct{}),
		open:       make(chan struct{}),
		quitItem:   make(chan struct{}),
		ipcFailed:  make(chan struct{}),
		openedCh:   make(chan struct{}, 8),
		done:       make(chan struct{}),

		autostartItem: make(chan struct{}),
		toggleResult:  o.toggleResult,
		checkedCh:     make(chan bool, 8),
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	if o.cancelFirst {
		cancel()
	}
	var sessionEnd <-chan struct{} = h.sessionEnd
	if o.nilSessionEnd {
		sessionEnd = nil
	}
	var ipcFailed <-chan struct{} = h.ipcFailed
	if o.nilIPCFailed {
		ipcFailed = nil
	}
	log := slog.New(h.logs)

	deps := trayLoopDeps{
		Ready:           h.ready,
		SessionEnd:      sessionEnd,
		IPCFailed:       ipcFailed,
		Stop:            h.stop,
		Quit:            func() { h.quitCalls.Add(1) },
		QuitBeforeReady: func() { h.quitBeforeReadyCalls.Add(1) },
		OnOpen: func() {
			h.openCalls.Add(1)
			h.openedCh <- struct{}{}
		},
		OnToggle: func() bool { // T07
			h.recordToggle("toggle")
			return h.toggleResult
		},
		SetChecked: func(checked bool) { // T07
			h.recordToggle(fmt.Sprintf("checked=%v", checked))
			h.checkedCh <- checked
		},
	}

	go func() {
		defer close(h.done)
		h.reason = trayLoop(ctx, clock, log, deps)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(testTimeout):
			t.Error("trayLoop goroutine did not end at cleanup")
		}
	})

	if !o.cancelFirst {
		// The loop registered its 30 s ready timeout: it is inside the select.
		bctx, bcancel := context.WithTimeout(context.Background(), testTimeout)
		defer bcancel()
		if err := clock.BlockUntilWaiters(bctx, 1); err != nil {
			t.Fatalf("BlockUntilWaiters: %v", err)
		}
	}
	return h
}

// deliverReady hands the menu items to the loop (onReady ran).
func (h *trayHarness) deliverReady(t *testing.T) {
	t.Helper()
	select {
	case h.ready <- trayItems{open: h.open, quit: h.quitItem, autostart: h.autostartItem}:
	case <-time.After(testTimeout):
		t.Fatalf("trayLoop did not receive ready within %v", testTimeout)
	}
}

func (h *trayHarness) waitDone(t *testing.T) {
	t.Helper()
	select {
	case <-h.done:
	case <-time.After(testTimeout):
		t.Fatalf("trayLoop did not return within %v", testTimeout)
	}
}

func (h *trayHarness) assertRunning(t *testing.T) {
	t.Helper()
	select {
	case <-h.done:
		t.Fatal("trayLoop returned, want still running")
	default:
	}
}

func (h *trayHarness) waitOpened(t *testing.T) {
	t.Helper()
	select {
	case <-h.openedCh:
	case <-time.After(testTimeout):
		t.Fatal("onOpen not called")
	}
}

// send delivers on an unbuffered channel the loop is selecting on.
func send(t *testing.T, ch chan<- struct{}, what string) {
	t.Helper()
	select {
	case ch <- struct{}{}:
	case <-time.After(testTimeout):
		t.Fatalf("trayLoop did not receive %s within %v", what, testTimeout)
	}
}

func (h *trayHarness) assertCalls(t *testing.T, quit, quitBeforeReady, open int32) {
	t.Helper()
	if got := h.quitCalls.Load(); got != quit {
		t.Errorf("quit called %d times, want %d", got, quit)
	}
	if got := h.quitBeforeReadyCalls.Load(); got != quitBeforeReady {
		t.Errorf("quitBeforeReady called %d times, want %d", got, quitBeforeReady)
	}
	if got := h.openCalls.Load(); got != open {
		t.Errorf("onOpen called %d times, want %d", got, open)
	}
}

func TestTrayLoop(t *testing.T) {
	t.Run("AGT-003/Ctrl+C antes da bandeja inicializar encerra o loop", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{})
		h.cancel()
		h.waitDone(t)
		h.assertCalls(t, 0, 1, 0)
		if n := h.logs.count(slog.LevelError); n != 0 {
			t.Errorf("%d Error records, want 0", n)
		}
	})

	t.Run("AGT-003/Ctrl+C antes de o loop começar encerra o loop", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{cancelFirst: true})
		h.waitDone(t)
		h.assertCalls(t, 0, 1, 0)
	})

	t.Run("AGT-006/fim de sessão antes da bandeja inicializar encerra o loop", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{})
		h.sessionEnd <- struct{}{}
		h.waitDone(t)
		h.assertCalls(t, 0, 1, 0)
	})

	t.Run("AGT-001/bandeja não inicializou em 30 s loga Error e segue esperando", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{})

		h.clock.Advance(trayReadyTimeout - time.Nanosecond)
		if n := h.logs.count(slog.LevelError); n != 0 {
			t.Fatalf("%d Error records before 30 s, want 0", n)
		}
		h.assertRunning(t)

		h.clock.Advance(time.Nanosecond)
		h.logs.waitLevel(t, slog.LevelError)
		h.assertRunning(t)
		h.assertCalls(t, 0, 0, 0)

		// Advancing further does not log again (the Error is logged once).
		h.clock.Advance(time.Hour)

		h.deliverReady(t)
		send(t, h.quitItem, "Sair")
		h.waitDone(t)
		h.assertCalls(t, 1, 0, 0)
		if n := h.logs.count(slog.LevelError); n != 1 {
			t.Errorf("%d Error records, want exactly 1", n)
		}
	})

	t.Run("AGT-003/Sair depois da bandeja pronta chama quit", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{})
		h.deliverReady(t)
		send(t, h.quitItem, "Sair")
		h.waitDone(t)
		h.assertCalls(t, 1, 0, 0)
	})

	t.Run("AGT-003/Ctrl+C depois da bandeja pronta chama quit", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{})
		h.deliverReady(t)
		h.cancel()
		h.waitDone(t)
		h.assertCalls(t, 1, 0, 0)
	})

	t.Run("AGT-007/Abrir Arcadia chama onOpen", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{})
		h.deliverReady(t)
		send(t, h.open, "Abrir Arcadia")
		h.waitOpened(t)
		send(t, h.open, "Abrir Arcadia (2)")
		h.waitOpened(t)
		h.assertRunning(t)
		h.assertCalls(t, 0, 0, 2)

		send(t, h.quitItem, "Sair")
		h.waitDone(t)
		h.assertCalls(t, 1, 0, 2)
	})

	t.Run("AGT-006/fim de sessão depois da bandeja pronta chama quit", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{})
		h.deliverReady(t)
		h.sessionEnd <- struct{}{}
		h.waitDone(t)
		h.assertCalls(t, 1, 0, 0)
	})

	t.Run("AGT-003/loop do systray encerrado (stop) retorna sem chamar nada", func(t *testing.T) {
		t.Parallel()
		t.Run("antes da bandeja pronta", func(t *testing.T) {
			t.Parallel()
			h := startTray(t, trayOpts{})
			close(h.stop)
			h.waitDone(t)
			h.assertCalls(t, 0, 0, 0)
		})
		t.Run("depois da bandeja pronta", func(t *testing.T) {
			t.Parallel()
			h := startTray(t, trayOpts{})
			h.deliverReady(t)
			close(h.stop)
			h.waitDone(t)
			h.assertCalls(t, 0, 0, 0)
		})
	})

	t.Run("AGT-003/sessionEnd nil nunca dispara", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{nilSessionEnd: true})

		// The loop keeps iterating (timeout fires and is logged) and does not
		// treat the nil channel as an end of session.
		h.clock.Advance(trayReadyTimeout)
		h.logs.waitLevel(t, slog.LevelError)
		h.assertRunning(t)
		h.assertCalls(t, 0, 0, 0)

		h.deliverReady(t)
		send(t, h.open, "Abrir Arcadia")
		h.waitOpened(t)
		h.assertRunning(t)
		h.assertCalls(t, 0, 0, 1)

		close(h.stop)
		h.waitDone(t)
		h.assertCalls(t, 0, 0, 1)
	})
}

func (h *trayHarness) assertReason(t *testing.T, want ShutdownReason) {
	t.Helper()
	if h.reason != want {
		t.Errorf("trayLoop returned reason %q, want %q", h.reason, want)
	}
}

func TestTrayLoopReason(t *testing.T) {
	t.Run("EXE-016/Sair devolve quit", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{})
		h.deliverReady(t)
		send(t, h.quitItem, "Sair")
		h.waitDone(t)
		h.assertCalls(t, 1, 0, 0)
		h.assertReason(t, ReasonQuit)
	})

	t.Run("EXE-016/fim de sessão antes e depois da bandeja devolve session-end", func(t *testing.T) {
		t.Parallel()
		t.Run("antes da bandeja pronta", func(t *testing.T) {
			t.Parallel()
			h := startTray(t, trayOpts{})
			h.sessionEnd <- struct{}{}
			h.waitDone(t)
			h.assertCalls(t, 0, 1, 0)
			h.assertReason(t, ReasonSessionEnd)
		})
		t.Run("depois da bandeja pronta", func(t *testing.T) {
			t.Parallel()
			h := startTray(t, trayOpts{})
			h.deliverReady(t)
			h.sessionEnd <- struct{}{}
			h.waitDone(t)
			h.assertCalls(t, 1, 0, 0)
			h.assertReason(t, ReasonSessionEnd)
		})
	})

	t.Run("EXE-016/Ctrl+C antes e depois da bandeja devolve signal", func(t *testing.T) {
		t.Parallel()
		t.Run("antes de o loop começar", func(t *testing.T) {
			t.Parallel()
			h := startTray(t, trayOpts{cancelFirst: true})
			h.waitDone(t)
			h.assertCalls(t, 0, 1, 0)
			h.assertReason(t, ReasonSignal)
		})
		t.Run("antes da bandeja pronta", func(t *testing.T) {
			t.Parallel()
			h := startTray(t, trayOpts{})
			h.cancel()
			h.waitDone(t)
			h.assertCalls(t, 0, 1, 0)
			h.assertReason(t, ReasonSignal)
		})
		t.Run("depois da bandeja pronta", func(t *testing.T) {
			t.Parallel()
			h := startTray(t, trayOpts{})
			h.deliverReady(t)
			h.cancel()
			h.waitDone(t)
			h.assertCalls(t, 1, 0, 0)
			h.assertReason(t, ReasonSignal)
		})
	})

	t.Run("EXE-016/stop devolve other sem chamar nada", func(t *testing.T) {
		t.Parallel()
		t.Run("antes da bandeja pronta", func(t *testing.T) {
			t.Parallel()
			h := startTray(t, trayOpts{})
			close(h.stop)
			h.waitDone(t)
			h.assertCalls(t, 0, 0, 0)
			h.assertReason(t, ReasonOther)
		})
		t.Run("depois da bandeja pronta", func(t *testing.T) {
			t.Parallel()
			h := startTray(t, trayOpts{})
			h.deliverReady(t)
			close(h.stop)
			h.waitDone(t)
			h.assertCalls(t, 0, 0, 0)
			h.assertReason(t, ReasonOther)
		})
	})

	t.Run("EXE-026/ipcFailed depois do ready chama Quit e devolve other", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{})
		h.deliverReady(t)
		close(h.ipcFailed)
		h.waitDone(t)
		h.assertCalls(t, 1, 0, 0)
		h.assertReason(t, ReasonOther)
	})

	t.Run("EXE-026/ipcFailed antes do ready chama QuitBeforeReady e devolve other", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{})
		close(h.ipcFailed)
		h.waitDone(t)
		h.assertCalls(t, 0, 1, 0)
		h.assertReason(t, ReasonOther)
	})

	t.Run("EXE-026/IPCFailed nil nunca dispara", func(t *testing.T) {
		t.Parallel()
		h := startTray(t, trayOpts{nilIPCFailed: true})

		// Before ready: the 30 s timeout fires and is logged; the nil channel
		// is not treated as an IPC failure.
		h.clock.Advance(trayReadyTimeout)
		h.logs.waitLevel(t, slog.LevelError)
		h.assertRunning(t)
		h.assertCalls(t, 0, 0, 0)

		// After ready: the loop keeps serving the menu.
		h.deliverReady(t)
		send(t, h.open, "Abrir Arcadia")
		h.waitOpened(t)
		h.assertRunning(t)
		h.assertCalls(t, 0, 0, 1)

		send(t, h.quitItem, "Sair")
		h.waitDone(t)
		h.assertCalls(t, 1, 0, 1)
		h.assertReason(t, ReasonQuit)
	})
}

func (h *trayHarness) recordToggle(ev string) {
	h.toggleMu.Lock()
	h.toggleEvents = append(h.toggleEvents, ev)
	h.toggleMu.Unlock()
}

func (h *trayHarness) toggleTrace() []string {
	h.toggleMu.Lock()
	defer h.toggleMu.Unlock()
	return append([]string(nil), h.toggleEvents...)
}

// waitChecked waits for the SetChecked call that follows a click.
func (h *trayHarness) waitChecked(t *testing.T) bool {
	t.Helper()
	select {
	case v := <-h.checkedCh:
		return v
	case <-time.After(testTimeout):
		t.Fatalf("SetChecked not called within %v", testTimeout)
		return false
	}
}

func TestTrayLoopToggle(t *testing.T) {
	cases := []struct {
		name   string
		result bool
	}{
		{"AST-007/clique chama OnToggle e SetChecked(false)", false},
		{"AST-008/clique chama OnToggle e SetChecked(true)", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			h := startTray(t, trayOpts{toggleResult: c.result})
			h.deliverReady(t)

			send(t, h.autostartItem, "Iniciar com o Windows")
			if got := h.waitChecked(t); got != c.result {
				t.Errorf("SetChecked(%v), want SetChecked(%v) with OnToggle's result", got, c.result)
			}
			want := []string{"toggle", fmt.Sprintf("checked=%v", c.result)}
			if got := h.toggleTrace(); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("calls = %v, want %v (OnToggle then SetChecked)", got, want)
			}

			// The click does not end the loop nor call the other callbacks.
			h.assertRunning(t)
			h.assertCalls(t, 0, 0, 0)

			// A second click toggles again.
			send(t, h.autostartItem, "Iniciar com o Windows (2)")
			h.waitChecked(t)
			if got := len(h.toggleTrace()); got != 4 {
				t.Errorf("%d toggle events after two clicks, want 4 (%v)", got, h.toggleTrace())
			}

			send(t, h.quitItem, "Sair")
			h.waitDone(t)
			h.assertCalls(t, 1, 0, 0)
			h.assertReason(t, ReasonQuit)
		})
	}
}
