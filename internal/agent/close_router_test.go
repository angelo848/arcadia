package agent

// Tests for T13 of docs/specs/phase-13-modo-console/tasks.md: the trigger router with two actions
// (ActionPause and ActionClose) and the game.closedByHotkey notification [CON-004, CON-012, CON-014,
// CON-015, CON-019, CON-021]. The tracker is the pauseFake of pause_test.go (now also with
// CloseByHotkey); the feedback sender and the close sink are fakes; the IPC case uses a real server and
// two raw clients. (alterado em 2026-11-22 (fase 15): the rig of pause_test.go now builds the router with
// the opener and shuttingDown of the Big Box action; no case of this file changed.) No test sleeps: waits are channels with pauseWait, and the "nothing happened"
// checks run after a flush (a later trigger that the single router goroutine only handles once every
// earlier one is done).

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/feedback"
	"github.com/arcadia-launcher/arcadia/internal/input"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// closeCall is one CloseByHotkey call received by the fake tracker.
type closeCall struct {
	at          time.Time
	deadline    time.Time
	hasDeadline bool
	seenAt      time.Time // real instant of the call, to measure the deadline
}

func (f *pauseFake) CloseByHotkey(ctx context.Context, at time.Time) (tracker.CloseResult, error) {
	dl, ok := ctx.Deadline()
	f.record(pauseCall{method: "CloseByHotkey", at: at})
	f.closes <- closeCall{at: at, deadline: dl, hasDeadline: ok, seenAt: time.Now()}
	if f.closeFn != nil {
		return f.closeFn(ctx, at)
	}
	return tracker.CloseResult{Kind: tracker.CloseClosed, GameID: 1}, nil
}

// fbFake records the feedback events sent by the router.
type fbFake struct {
	mu  sync.Mutex
	evs []feedback.Event
	ch  chan feedback.Event
}

func newFBFake() *fbFake { return &fbFake{ch: make(chan feedback.Event, 32)} }

func (f *fbFake) Send(e feedback.Event) {
	f.mu.Lock()
	f.evs = append(f.evs, e)
	f.mu.Unlock()
	select {
	case f.ch <- e:
	default:
	}
}

func (f *fbFake) events() []feedback.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]feedback.Event(nil), f.evs...)
}

// sinkFake records the game ids of the close notifications.
type sinkFake struct {
	mu  sync.Mutex
	ids []int64
	ch  chan int64
}

func newSinkFake() *sinkFake { return &sinkFake{ch: make(chan int64, 32)} }

func (s *sinkFake) GameClosedByHotkey(id int64) {
	s.mu.Lock()
	s.ids = append(s.ids, id)
	s.mu.Unlock()
	select {
	case s.ch <- id:
	default:
	}
}

func (s *sinkFake) got() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int64(nil), s.ids...)
}

// closeTrig is a close trigger (Back+B) at the given instant.
func closeTrig(at time.Time) input.Trigger {
	return input.Trigger{Source: domain.PauseGamepad, Action: input.ActionClose, At: at}
}

// nextCloses waits for n CloseByHotkey calls and returns them in arrival order.
func (rig *pauseRig) nextCloses(t *testing.T, n int) []closeCall {
	t.Helper()
	got := make([]closeCall, 0, n)
	for len(got) < n {
		select {
		case c := <-rig.tr.closes:
			got = append(got, c)
		case <-time.After(pauseWait):
			t.Fatalf("CloseByHotkey calls = %d, want %d (timeout)", len(got), n)
		}
	}
	return got
}

// flush sends a pause trigger far from every other one and waits for it: when it returns the router
// has finished every earlier trigger (the router is a single goroutine, in order).
func (rig *pauseRig) flush(t *testing.T) {
	t.Helper()
	rig.sentinels++
	rig.in <- pauseTrig(domain.PauseHotkey, pauseT0.Add(time.Duration(rig.sentinels)*1000*time.Hour))
	rig.nextToggles(t, 1)
}

func wantCloseAts(t *testing.T, got []closeCall, want ...time.Time) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("CloseByHotkey calls = %+v, want %d calls", got, len(want))
	}
	for i := range want {
		if !got[i].at.Equal(want[i]) {
			t.Errorf("close call %d at %v, want %v", i, got[i].at, want[i])
		}
	}
}

func (rig *pauseRig) wantNoFeedback(t *testing.T) {
	t.Helper()
	if evs := rig.fb.events(); len(evs) != 0 {
		t.Errorf("feedback events = %v, want none", evs)
	}
}

func (rig *pauseRig) waitRefused(t *testing.T) {
	t.Helper()
	select {
	case e := <-rig.fb.ch:
		if e != feedback.EventRefused {
			t.Errorf("feedback event = %v, want EventRefused", e)
		}
	case <-time.After(pauseWait):
		t.Fatal("no EventRefused within the timeout")
	}
}

func resultFn(r tracker.CloseResult, err error) func(context.Context, time.Time) (tracker.CloseResult, error) {
	return func(context.Context, time.Time) (tracker.CloseResult, error) { return r, err }
}

func TestTriggerRouterClose(t *testing.T) {
	t.Run("CON-004/Closed chama CloseByHotkey com o At do gatilho e prazo de 6 s", func(t *testing.T) {
		if closeTimeout != 6*time.Second {
			t.Fatalf("closeTimeout = %v, want 6 s", closeTimeout)
		}
		rig := startPauseRig(t, newPauseFake())
		rig.in <- closeTrig(pauseT0)
		c := rig.nextCloses(t, 1)
		wantCloseAts(t, c, pauseT0)
		if !c[0].hasDeadline {
			t.Fatal("CloseByHotkey context has no deadline")
		}
		if left := c[0].deadline.Sub(c[0].seenAt); left > 6*time.Second || left < 5*time.Second {
			t.Errorf("deadline in %v, want ~6 s", left)
		}
		if n := len(rig.tr.recorded()); n != 1 {
			t.Errorf("tracker calls = %d, want only the CloseByHotkey", n)
		}
		rig.r.Close()
	})

	t.Run("CON-004/dois gatilhos de fechar com menos de 300 ms entre os At: o segundo é descartado sem som", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- closeTrig(pauseT0)
		rig.in <- closeTrig(pauseT0.Add(299 * time.Millisecond))
		rig.in <- closeTrig(pauseT0.Add(time.Second)) // sentinel
		wantCloseAts(t, rig.nextCloses(t, 2), pauseT0, pauseT0.Add(time.Second))
		rig.flush(t)
		rig.wantNoFeedback(t)
		if len(rig.tr.closes) != 0 {
			t.Errorf("extra CloseByHotkey calls queued: %d", len(rig.tr.closes))
		}
	})

	t.Run("CON-004/300 ms exatos passam e o descartado não move a janela", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- closeTrig(pauseT0)
		rig.in <- closeTrig(pauseT0.Add(200 * time.Millisecond)) // dropped
		rig.in <- closeTrig(pauseT0.Add(300 * time.Millisecond))
		wantCloseAts(t, rig.nextCloses(t, 2), pauseT0, pauseT0.Add(300*time.Millisecond))
	})

	t.Run("CON-004/as janelas são por ação: pausa e fechar no mesmo instante vão os dois, na ordem de chegada", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			first input.Trigger
			order []string
		}{
			{"pausa primeiro", pauseTrig(domain.PauseGamepad, pauseT0), []string{"TogglePause", "CloseByHotkey"}},
			{"fechar primeiro", closeTrig(pauseT0), []string{"CloseByHotkey", "TogglePause"}},
		} {
			t.Run("CON-004/"+tc.name, func(t *testing.T) {
				rig := startPauseRig(t, newPauseFake())
				rig.in <- tc.first
				if tc.first.Action == input.ActionClose {
					rig.in <- pauseTrig(domain.PauseGamepad, pauseT0)
				} else {
					rig.in <- closeTrig(pauseT0)
				}
				rig.nextToggles(t, 1)
				rig.nextCloses(t, 1)
				var got []string
				for _, c := range rig.tr.recorded() {
					got = append(got, c.method)
				}
				if fmt.Sprint(got) != fmt.Sprint(tc.order) {
					t.Errorf("call order = %v, want %v", got, tc.order)
				}
			})
		}
	})

	t.Run("CON-004/PAU-004 continua valendo entre gatilhos de pausa, e fechar no meio não move a janela da pausa", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- pauseTrig(domain.PauseHotkey, pauseT0)
		rig.in <- closeTrig(pauseT0.Add(100 * time.Millisecond))
		rig.in <- pauseTrig(domain.PauseGamepad, pauseT0.Add(200*time.Millisecond)) // dropped: 200 ms after the last pause
		rig.in <- pauseTrig(domain.PauseHotkey, pauseT0.Add(300*time.Millisecond))
		wantToggles(t, rig.nextToggles(t, 2),
			pauseCall{reason: domain.PauseHotkey, at: pauseT0},
			pauseCall{reason: domain.PauseHotkey, at: pauseT0.Add(300 * time.Millisecond)})
		wantCloseAts(t, rig.nextCloses(t, 1), pauseT0.Add(100*time.Millisecond))
	})

	t.Run("CON-004/Trigger sem Action (valor zero) continua sendo pausa", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- input.Trigger{Source: domain.PauseHotkey, At: pauseT0}
		wantToggles(t, rig.nextToggles(t, 1), pauseCall{reason: domain.PauseHotkey, at: pauseT0})
		if len(rig.tr.closes) != 0 {
			t.Error("a pause trigger reached CloseByHotkey")
		}
	})

	t.Run("CON-012/refused no-game toca EventRefused", func(t *testing.T) {
		tr := newPauseFake()
		tr.closeFn = resultFn(tracker.CloseResult{Kind: tracker.CloseRefused, Reason: tracker.CloseReasonNoGame}, nil)
		rig := startPauseRig(t, tr)
		rig.in <- closeTrig(pauseT0)
		rig.nextCloses(t, 1)
		rig.waitRefused(t)
		rig.flush(t)
		if got := rig.fb.events(); len(got) != 1 {
			t.Errorf("feedback events = %v, want exactly one EventRefused", got)
		}
		if ids := rig.sink.got(); len(ids) != 0 {
			t.Errorf("sink got %v for a refused close, want none", ids)
		}
	})

	t.Run("CON-015/refused not-adopted e failed enviam EventRefused; refused closing, descarte do antirrepique e ErrShuttingDown não tocam nada", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			res    tracker.CloseResult
			err    error
			refuse bool
		}{
			{"not-adopted", tracker.CloseResult{Kind: tracker.CloseRefused, Reason: tracker.CloseReasonNotAdopted}, nil, true},
			{"failed", tracker.CloseResult{Kind: tracker.CloseFailed, GameID: 5}, nil, true},
			{"failed sem contar tempo", tracker.CloseResult{Kind: tracker.CloseFailed, GameID: 5, Untracked: true}, nil, true},
			{"closing", tracker.CloseResult{Kind: tracker.CloseRefused, Reason: tracker.CloseReasonClosing}, nil, false},
			{"ErrShuttingDown", tracker.CloseResult{}, fmt.Errorf("tracker: %w", tracker.ErrShuttingDown), false},
		} {
			t.Run("CON-015/"+tc.name, func(t *testing.T) {
				tr := newPauseFake()
				tr.closeFn = resultFn(tc.res, tc.err)
				rig := startPauseRig(t, tr)
				rig.in <- closeTrig(pauseT0)
				rig.nextCloses(t, 1)
				if tc.refuse {
					rig.waitRefused(t)
				}
				rig.flush(t)
				want := 0
				if tc.refuse {
					want = 1
				}
				if got := rig.fb.events(); len(got) != want {
					t.Errorf("feedback events = %v, want %d", got, want)
				}
				if ids := rig.sink.got(); len(ids) != 0 {
					t.Errorf("sink got %v, want none", ids)
				}
				for _, l := range rig.log.lines(t) {
					if !tc.refuse && (l.level == "WARN" || l.level == "ERROR") {
						t.Errorf("logged %s %q, want Debug at most", l.level, l.msg)
					}
				}
			})
		}

		t.Run("CON-015/gatilho descartado pelo antirrepique não toca nada", func(t *testing.T) {
			tr := newPauseFake()
			tr.closeFn = resultFn(tracker.CloseResult{Kind: tracker.CloseRefused, Reason: tracker.CloseReasonClosing}, nil)
			rig := startPauseRig(t, tr)
			rig.in <- closeTrig(pauseT0)
			rig.in <- closeTrig(pauseT0.Add(10 * time.Millisecond))
			rig.nextCloses(t, 1)
			rig.flush(t)
			if len(tr.closes) != 0 {
				t.Error("the debounced trigger reached the tracker")
			}
			rig.wantNoFeedback(t)
		})
	})

	t.Run("CON-014/closed (sessão e sem contar tempo) não envia feedback e notifica o gameId", func(t *testing.T) {
		for _, untracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("CON-014/untracked=%t", untracked), func(t *testing.T) {
				tr := newPauseFake()
				tr.closeFn = resultFn(tracker.CloseResult{Kind: tracker.CloseClosed, GameID: 77, Untracked: untracked}, nil)
				rig := startPauseRig(t, tr)
				rig.in <- closeTrig(pauseT0)
				select {
				case id := <-rig.sink.ch:
					if id != 77 {
						t.Errorf("notified game %d, want 77", id)
					}
				case <-time.After(pauseWait):
					t.Fatal("no close notification")
				}
				rig.flush(t)
				rig.wantNoFeedback(t)
				if ids := rig.sink.got(); len(ids) != 1 {
					t.Errorf("notifications = %v, want exactly one", ids)
				}
			})
		}
	})

	t.Run("CON-021/ErrShuttingDown: Debug, sem som e sem notificação", func(t *testing.T) {
		tr := newPauseFake()
		tr.closeFn = resultFn(tracker.CloseResult{}, tracker.ErrShuttingDown)
		rig := startPauseRig(t, tr)
		rig.in <- closeTrig(pauseT0)
		rig.nextCloses(t, 1)
		rig.flush(t)
		rig.r.Close()
		lines := rig.log.lines(t)
		if countLog(lines, "DEBUG", "") == 0 {
			t.Errorf("no Debug record; log = %+v", lines)
		}
		if countLog(lines, "WARN", "")+countLog(lines, "ERROR", "") != 0 {
			t.Errorf("Warn/Error logged for ErrShuttingDown: %+v", lines)
		}
		rig.wantNoFeedback(t)
		if ids := rig.sink.got(); len(ids) != 0 {
			t.Errorf("sink got %v, want none", ids)
		}
	})

	t.Run("CON-021/Close do roteador durante uma chamada pendente retorna sem esperar os 6 s e sem tocar nada", func(t *testing.T) {
		tr := newPauseFake()
		tr.closeFn = func(ctx context.Context, _ time.Time) (tracker.CloseResult, error) {
			<-ctx.Done() // a close wait that only ends with the context
			return tracker.CloseResult{}, ctx.Err()
		}
		rig := startPauseRig(t, tr)
		rig.in <- closeTrig(pauseT0)
		rig.nextCloses(t, 1)
		done := make(chan struct{})
		go func() { rig.r.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(pauseWait):
			t.Fatal("Close still waiting for the pending CloseByHotkey")
		}
		rig.r.Close() // idempotent
		rig.wantNoFeedback(t)
		if ids := rig.sink.got(); len(ids) != 0 {
			t.Errorf("sink got %v, want none", ids)
		}
		if n := countLog(rig.log.lines(t), "WARN", "tracker não respondeu ao fechar a tempo"); n != 0 {
			t.Errorf("a cancelled call logged the timeout Warn %d time(s), want 0", n)
		}
	})

	t.Run("CON-021/prazo vencido: Warn sem som", func(t *testing.T) {
		tr := newPauseFake()
		tr.closeFn = resultFn(tracker.CloseResult{}, fmt.Errorf("tracker: %w", context.DeadlineExceeded))
		rig := startPauseRig(t, tr)
		rig.in <- closeTrig(pauseT0)
		rig.nextCloses(t, 1)
		rig.flush(t)
		rig.r.Close()
		if n := countLog(rig.log.lines(t), "WARN", "tracker não respondeu ao fechar a tempo"); n != 1 {
			t.Errorf("Warn \"tracker não respondeu ao fechar a tempo\" records = %d, want 1; log = %+v", n, rig.log.lines(t))
		}
		rig.wantNoFeedback(t)
		if ids := rig.sink.got(); len(ids) != 0 {
			t.Errorf("sink got %v, want none", ids)
		}
	})

	t.Run("CON-019/a notificação sai depois da resposta do tracker, com gameId", func(t *testing.T) {
		release := make(chan struct{})
		tr := newPauseFake()
		tr.closeFn = func(context.Context, time.Time) (tracker.CloseResult, error) {
			<-release
			return tracker.CloseResult{Kind: tracker.CloseClosed, GameID: 9}, nil
		}
		rig := startPauseRig(t, tr)
		rig.in <- closeTrig(pauseT0)
		rig.nextCloses(t, 1)
		if ids := rig.sink.got(); len(ids) != 0 {
			t.Fatalf("notified %v before the tracker answered", ids)
		}
		close(release)
		select {
		case id := <-rig.sink.ch:
			if id != 9 {
				t.Errorf("notified game %d, want 9", id)
			}
		case <-time.After(pauseWait):
			t.Fatal("no notification after the tracker answered")
		}
	})

	t.Run("CON-019/não sai para refused nem failed", func(t *testing.T) {
		for _, res := range []tracker.CloseResult{
			{Kind: tracker.CloseRefused, Reason: tracker.CloseReasonNoGame, GameID: 3},
			{Kind: tracker.CloseFailed, GameID: 3},
		} {
			tr := newPauseFake()
			tr.closeFn = resultFn(res, nil)
			rig := startPauseRig(t, tr)
			rig.in <- closeTrig(pauseT0)
			rig.nextCloses(t, 1)
			rig.flush(t)
			if ids := rig.sink.got(); len(ids) != 0 {
				t.Errorf("%+v notified %v, want none", res, ids)
			}
		}
	})

	t.Run("CON-019/dois clientes IPC reais recebem game.closedByHotkey {gameId} e, com sessão, depois do session.changed", func(t *testing.T) {
		notes := &gameClosedNotifier{}
		tr := newPauseFake()
		sink := newSinkFake()
		rig := startRigWith(t, tr, newFBFake(), rigFB(), sink, notes)
		srv, addr := startNotifyServer(t)
		notes.set(srv)
		tr.closeFn = func(context.Context, time.Time) (tracker.CloseResult, error) {
			// The tracker notifies the end of the session before it answers (CON-019).
			srv.Notify(ipc.MethodSessionChanged, ipc.SessionChangedParams{Session: ipc.SessionFromSnapshot(domain.SessionSnapshot{
				ID: 1, GameID: 42, Title: "x", State: domain.SessionEnded, StartedAt: pauseT0, AsOf: pauseT0,
			})})
			return tracker.CloseResult{Kind: tracker.CloseClosed, GameID: 42}, nil
		}
		clients := []*rawClient{dialRaw(t, addr), dialRaw(t, addr)}
		for _, c := range clients {
			c.rpc(t, "x.register", nil) // any response proves the connection is registered
		}

		rig.in <- closeTrig(pauseT0)
		for i, c := range clients {
			all := c.waitAll(t, "game.closedByHotkey", func(all []rawMsg) bool {
				return countMethod(all, ipc.MethodGameClosedByHotkey) >= 1
			})
			sessAt, noteAt := -1, -1
			for j, m := range all {
				switch m.method() {
				case ipc.MethodSessionChanged:
					sessAt = j
				case ipc.MethodGameClosedByHotkey:
					noteAt = j
					if _, hasID := m["id"]; hasID {
						t.Errorf("client %d: game.closedByHotkey has an id, want a notification", i+1)
					}
					var p struct {
						GameID int64 `json:"gameId"`
					}
					if err := json.Unmarshal(m["params"], &p); err != nil || p.GameID != 42 {
						t.Errorf("client %d: params %s (err %v), want {gameId: 42}", i+1, m["params"], err)
					}
					if string(m["params"]) != `{"gameId":42}` {
						t.Errorf("client %d: params %s, want exactly {\"gameId\":42}", i+1, m["params"])
					}
				}
			}
			if sessAt < 0 || noteAt < 0 || sessAt > noteAt {
				t.Errorf("client %d: session.changed at %d, game.closedByHotkey at %d; want session.changed first", i+1, sessAt, noteAt)
			}
			if n := countMethod(all, ipc.MethodGameClosedByHotkey); n != 1 {
				t.Errorf("client %d: %d game.closedByHotkey, want 1", i+1, n)
			}
		}
	})

	t.Run("CON-019/fila cheia de uma conexão descarta sem bloquear (IPC-017)", func(t *testing.T) {
		verifyNoLeaks(t)
		srv, addr := startNotifyServer(t)
		notes := &gameClosedNotifier{}
		notes.set(srv)

		// A client that never reads: its queue fills up.
		slow, err := net.DialTimeout("tcp", addr, testTimeout)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		t.Cleanup(func() { _ = slow.Close() })
		if _, err := slow.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"x.register"}` + "\n")); err != nil {
			t.Fatalf("write: %v", err)
		}
		good := dialRaw(t, addr)
		good.rpc(t, "x.register", nil)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := 0; i < 20000; i++ {
				notes.GameClosedByHotkey(int64(i + 1))
			}
		}()
		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Fatal("GameClosedByHotkey blocked on a connection with a full queue")
		}
		good.waitAll(t, "at least one game.closedByHotkey on the reading client", func(all []rawMsg) bool {
			return countMethod(all, ipc.MethodGameClosedByHotkey) >= 1
		})
	})

	t.Run("CON-004/x-console.closeHandlerTimeoutMs é igual a closeTimeout", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "phase-13-modo-console", "contracts", "ipc.openrpc.json"))
		if err != nil {
			t.Skipf("contract not found (moved?): %v", err)
		}
		var doc struct {
			Info struct {
				Console struct {
					CloseHandlerTimeoutMs float64 `json:"closeHandlerTimeoutMs"`
				} `json:"x-console"`
			} `json:"info"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse contract: %v", err)
		}
		if got := time.Duration(doc.Info.Console.CloseHandlerTimeoutMs) * time.Millisecond; got != closeTimeout {
			t.Errorf("x-console.closeHandlerTimeoutMs = %v, closeTimeout = %v; want equal", got, closeTimeout)
		}
	})
}

// rigFB returns a no-op feedback sender for rigs whose test does not look at it.
func rigFB() feedbackSender { return nopFB{} }

type nopFB struct{}

func (nopFB) Send(feedback.Event) {}

// startNotifyServer serves an IPC server without handlers on TCP and registers its shutdown. It must
// be called after the goleak check of the test (startRigWith / verifyNoLeaks).
func startNotifyServer(t *testing.T) (*ipc.Server, string) {
	t.Helper()
	srv := ipc.NewServer(testClock(), map[string]ipc.Handler{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = srv.Serve(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		_ = srv.Close()
		wg.Wait()
	})
	return srv, ln.Addr().String()
}
