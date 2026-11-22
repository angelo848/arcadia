package agent

// Tests for T19 (docs/specs/phase-4-pause/tasks.md): the pause router
// [PAU-001..PAU-005, PAU-008] and the power bridge [PWR-005]. The router is
// driven through its input channel with explicit Trigger.At instants; the
// tracker is a fake that records every call. Each router test registers the
// goleak check before the router starts.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/input"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// pauseT0 is the instant of the first trigger of every pause test.
var pauseT0 = time.Date(2026, 11, 10, 20, 0, 0, 0, time.UTC)

// pauseWait bounds every wait for an asynchronous router or bridge event. It is
// only a failure timeout: the tests never rely on it to pass.
const pauseWait = 2 * time.Second

// pauseCall is one call received by the fake pauseTracker.
type pauseCall struct {
	method      string // "TogglePause" | "Suspend" | "Wake"
	reason      domain.PauseReason
	at          time.Time
	userPresent bool
}

// pauseFake is a programmable pauseTracker. It embeds the games_test fakeTracker
// so the same value also serves as the sessionTracker of the UI handlers
// [PAU-008].
type pauseFake struct {
	*fakeTracker

	pmu     sync.Mutex
	calls   []pauseCall
	toggles chan pauseCall // one entry per TogglePause call, in order

	// Set before the router starts; nil means the default answer.
	toggleFn  func(ctx context.Context, reason domain.PauseReason, at time.Time) (tracker.PauseResult, error)
	suspendFn func(ctx context.Context, at time.Time) error
	wakeFn    func(ctx context.Context, at time.Time, userPresent bool) error

	// CloseByHotkey (T13): closes has one entry per call, in order; closeFn nil
	// answers Closed for game 1.
	closes  chan closeCall
	closeFn func(ctx context.Context, at time.Time) (tracker.CloseResult, error)
}

func newPauseFake() *pauseFake {
	return &pauseFake{
		fakeTracker: &fakeTracker{},
		toggles:     make(chan pauseCall, 64),
		closes:      make(chan closeCall, 64),
	}
}

func (f *pauseFake) record(c pauseCall) {
	f.pmu.Lock()
	defer f.pmu.Unlock()
	f.calls = append(f.calls, c)
}

// recorded returns a copy of every pauseTracker call so far.
func (f *pauseFake) recorded() []pauseCall {
	f.pmu.Lock()
	defer f.pmu.Unlock()
	return append([]pauseCall(nil), f.calls...)
}

// uiInstants returns the instants of the Pause and Resume calls received as
// the UI sessionTracker.
func (f *pauseFake) uiInstants() (pauses, resumes []time.Time) {
	f.fakeTracker.mu.Lock()
	defer f.fakeTracker.mu.Unlock()
	return append([]time.Time(nil), f.pauseAts...), append([]time.Time(nil), f.resumeAts...)
}

func (f *pauseFake) TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (tracker.PauseResult, error) {
	c := pauseCall{method: "TogglePause", reason: reason, at: at}
	f.record(c)
	f.toggles <- c
	if f.toggleFn != nil {
		return f.toggleFn(ctx, reason, at)
	}
	return tracker.PausePaused, nil
}

func (f *pauseFake) Suspend(ctx context.Context, at time.Time) error {
	f.record(pauseCall{method: "Suspend", at: at})
	if f.suspendFn != nil {
		return f.suspendFn(ctx, at)
	}
	return nil
}

func (f *pauseFake) Wake(ctx context.Context, at time.Time, userPresent bool) error {
	f.record(pauseCall{method: "Wake", at: at, userPresent: userPresent})
	if f.wakeFn != nil {
		return f.wakeFn(ctx, at, userPresent)
	}
	return nil
}

// pauseLogBuf is a log sink safe to read while the router goroutine writes.
type pauseLogBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *pauseLogBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// logLine is the level and message of one JSON log record.
type logLine struct {
	level string
	msg   string
}

// lines parses the records written so far.
func (b *pauseLogBuf) lines(t *testing.T) []logLine {
	t.Helper()
	b.mu.Lock()
	data := b.buf.String()
	b.mu.Unlock()
	var out []logLine
	for _, raw := range strings.Split(data, "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var rec struct {
			Level string `json:"level"`
			Msg   string `json:"msg"`
		}
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			t.Fatalf("log record %q is not JSON: %v", raw, err)
		}
		out = append(out, logLine{level: rec.Level, msg: rec.Msg})
	}
	return out
}

// countLog counts the records with the level and, when msg is not empty, the
// message.
func countLog(lines []logLine, level, msg string) int {
	n := 0
	for _, l := range lines {
		if l.level == level && (msg == "" || l.msg == msg) {
			n++
		}
	}
	return n
}

// pauseLogger writes JSON records at Debug into buf.
func pauseLogger(buf *pauseLogBuf) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// pauseRig is a router wired to a pauseFake and a captured log. fb and sink
// are the fakes of the feedback sender and of the close notification (T13).
type pauseRig struct {
	tr   *pauseFake
	in   chan input.Trigger
	log  *pauseLogBuf
	r    *triggerRouter
	fb   *fbFake
	sink *sinkFake

	// alterado em 2026-11-22 (fase 15): opener and shuttingDown of the Big Box action [ATB-004, ATB-007].
	opener *openerFake
	down   *atomic.Bool

	sentinels int // used by flush (close_router_test.go)
}

// startPauseRig registers the goleak check first, then starts the router and
// registers its Close, so Close runs before the leak check [PAU-005].
func startPauseRig(t *testing.T, tr *pauseFake) *pauseRig {
	t.Helper()
	fb, sink := newFBFake(), newSinkFake()
	return startRigWith(t, tr, fb, fb, sink, sink)
}

// startRigWith is startPauseRig with the feedback sender and the sink given by
// the caller (fb and sink are the fakes the rig exposes for assertions; fbS and
// sinkS are what the router receives).
func startRigWith(t *testing.T, tr *pauseFake, fb *fbFake, fbS feedbackSender, sink *sinkFake, sinkS closeSink) *pauseRig {
	t.Helper()
	verifyNoLeaks(t)
	buf := &pauseLogBuf{}
	// alterado em 2026-11-22 (fase 15): newTriggerRouter takes the opener and shuttingDown.
	op, down := &openerFake{}, &atomic.Bool{}
	rig := &pauseRig{tr: tr, in: make(chan input.Trigger, triggerQueueSize), log: buf, fb: fb, sink: sink, opener: op, down: down}
	rig.r = newTriggerRouter(tr, rig.in, pauseLogger(buf), fbS, sinkS, op, down.Load)
	rig.r.Start()
	t.Cleanup(rig.r.Close)
	return rig
}

// nextToggles waits for n TogglePause calls and returns them in arrival order.
// A missing call fails the test instead of hanging it.
func (rig *pauseRig) nextToggles(t *testing.T, n int) []pauseCall {
	t.Helper()
	got := make([]pauseCall, 0, n)
	for len(got) < n {
		select {
		case c := <-rig.tr.toggles:
			got = append(got, c)
		case <-time.After(pauseWait):
			t.Fatalf("TogglePause calls = %d, want %d (timeout)", len(got), n)
		}
	}
	return got
}

// wantToggles compares the reason and the instant of each call.
func wantToggles(t *testing.T, got []pauseCall, want ...pauseCall) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("TogglePause calls = %+v, want %d calls", got, len(want))
	}
	for i := range want {
		if got[i].reason != want[i].reason || !got[i].at.Equal(want[i].at) {
			t.Errorf("call %d = %s@%v, want %s@%v", i, got[i].reason, got[i].at, want[i].reason, want[i].at)
		}
	}
}

// pauseTrig is a trigger of the given source at the given instant.
func pauseTrig(src domain.PauseReason, at time.Time) input.Trigger {
	return input.Trigger{Source: src, At: at}
}

// pauseGapCases are the source combinations of the anti-bounce cases.
var pauseGapCases = []struct {
	name          string
	first, second domain.PauseReason
}{
	{"mesma fonte", domain.PauseHotkey, domain.PauseHotkey},
	{"fontes diferentes", domain.PauseHotkey, domain.PauseGamepad},
	{"fontes diferentes, controle primeiro", domain.PauseGamepad, domain.PauseHotkey},
}

func TestPauseRouter(t *testing.T) {
	t.Run("PAU-001/Trigger hotkey chama TogglePause com motivo hotkey e At do gatilho", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- pauseTrig(domain.PauseHotkey, pauseT0)
		wantToggles(t, rig.nextToggles(t, 1), pauseCall{reason: domain.PauseHotkey, at: pauseT0})
		rig.r.Close()
		if n := countLog(rig.log.lines(t), "WARN", ""); n != 0 {
			t.Errorf("Warn records = %d, want 0 for a paused result", n)
		}
	})

	t.Run("PAU-002/Trigger gamepad chama TogglePause com motivo gamepad", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- pauseTrig(domain.PauseGamepad, pauseT0)
		wantToggles(t, rig.nextToggles(t, 1), pauseCall{reason: domain.PauseGamepad, at: pauseT0})
		rig.r.Close()
		if n := countLog(rig.log.lines(t), "WARN", ""); n != 0 {
			t.Errorf("Warn records = %d, want 0 for a paused result", n)
		}
	})

	t.Run("PAU-003/PauseIgnored loga Debug gatilho de pausa ignorado e nada mais acontece", func(t *testing.T) {
		tr := newPauseFake()
		tr.toggleFn = func(context.Context, domain.PauseReason, time.Time) (tracker.PauseResult, error) {
			return tracker.PauseIgnored, nil
		}
		rig := startPauseRig(t, tr)
		rig.in <- pauseTrig(domain.PauseHotkey, pauseT0)
		wantToggles(t, rig.nextToggles(t, 1), pauseCall{reason: domain.PauseHotkey, at: pauseT0})
		rig.r.Close()
		lines := rig.log.lines(t)
		if n := countLog(lines, "DEBUG", "gatilho de pausa ignorado"); n != 1 {
			t.Errorf("Debug \"gatilho de pausa ignorado\" records = %d, want 1; log = %+v", n, lines)
		}
		if len(lines) != 1 {
			t.Errorf("log records = %+v, want only the Debug record", lines)
		}
		if n := len(tr.recorded()); n != 1 {
			t.Errorf("tracker calls = %d, want 1", n)
		}
	})

	t.Run("PAU-004/gatilhos com At separados por 299 ms: só o primeiro é encaminhado (mesma fonte e fontes diferentes)", func(t *testing.T) {
		for _, c := range pauseGapCases {
			t.Run("PAU-004/"+c.name, func(t *testing.T) {
				rig := startPauseRig(t, newPauseFake())
				rig.in <- pauseTrig(c.first, pauseT0)
				rig.in <- pauseTrig(c.second, pauseT0.Add(299*time.Millisecond))
				// Sentinel: 1 s after the first forwarded trigger, so it is always
				// forwarded and marks the end of the queue.
				rig.in <- pauseTrig(domain.PauseHotkey, pauseT0.Add(time.Second))
				wantToggles(t, rig.nextToggles(t, 2),
					pauseCall{reason: c.first, at: pauseT0},
					pauseCall{reason: domain.PauseHotkey, at: pauseT0.Add(time.Second)})
			})
		}
	})

	t.Run("PAU-004/separados por exatamente 300 ms o segundo também é encaminhado", func(t *testing.T) {
		for _, c := range pauseGapCases {
			t.Run("PAU-004/"+c.name, func(t *testing.T) {
				rig := startPauseRig(t, newPauseFake())
				rig.in <- pauseTrig(c.first, pauseT0)
				rig.in <- pauseTrig(c.second, pauseT0.Add(300*time.Millisecond))
				wantToggles(t, rig.nextToggles(t, 2),
					pauseCall{reason: c.first, at: pauseT0},
					pauseCall{reason: c.second, at: pauseT0.Add(300 * time.Millisecond)})
			})
		}
	})

	t.Run("PAU-004/a janela é medida a partir do último encaminhado: At 0, 200 (descartado) e 300 são encaminhados 0 e 300", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- pauseTrig(domain.PauseHotkey, pauseT0)
		rig.in <- pauseTrig(domain.PauseGamepad, pauseT0.Add(200*time.Millisecond))
		rig.in <- pauseTrig(domain.PauseHotkey, pauseT0.Add(300*time.Millisecond))
		wantToggles(t, rig.nextToggles(t, 2),
			pauseCall{reason: domain.PauseHotkey, at: pauseT0},
			pauseCall{reason: domain.PauseHotkey, at: pauseT0.Add(300 * time.Millisecond)})
	})

	t.Run("PAU-005/ErrShuttingDown é descartado com Debug, sem pânico", func(t *testing.T) {
		tr := newPauseFake()
		tr.toggleFn = func(context.Context, domain.PauseReason, time.Time) (tracker.PauseResult, error) {
			return tracker.PauseIgnored, fmt.Errorf("tracker: %w", tracker.ErrShuttingDown)
		}
		rig := startPauseRig(t, tr)
		rig.in <- pauseTrig(domain.PauseHotkey, pauseT0)
		wantToggles(t, rig.nextToggles(t, 1), pauseCall{reason: domain.PauseHotkey, at: pauseT0})
		rig.r.Close()
		lines := rig.log.lines(t)
		if n := countLog(lines, "DEBUG", ""); n == 0 {
			t.Errorf("no Debug record for ErrShuttingDown; log = %+v", lines)
		}
		for _, l := range lines {
			if l.level == "WARN" || l.level == "ERROR" {
				t.Errorf("ErrShuttingDown logged %s %q, want Debug only", l.level, l.msg)
			}
		}
	})

	t.Run("PAU-005/Close espera a goroutine (goleak) e é idempotente", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.r.Close()
		rig.r.Close()
		rig.r.Close()
		if n := len(rig.tr.recorded()); n != 0 {
			t.Errorf("tracker calls after Close without triggers = %d, want 0", n)
		}
	})
}

func TestPauseRouterUIRequests(t *testing.T) {
	t.Run("PAU-008/pedido da UI (session.pause) 100 ms depois de um gatilho é aplicado, não atualiza o último encaminhado, e um gatilho 100 ms depois do pedido da UI é encaminhado", func(t *testing.T) {
		tr := newPauseFake()
		rig := startPauseRig(t, tr)
		clock := domain.NewFakeClock(pauseT0)
		uiLog := &pauseLogBuf{}
		handlers := gameHandlers(nil, tr, clock, nil, pauseLogger(uiLog))
		pause := handlers[ipc.MethodSessionPause]
		resume := handlers[ipc.MethodSessionResume]
		if pause == nil || resume == nil {
			t.Fatal("games handlers lack session.pause or session.resume")
		}
		ctx := context.Background()

		// A hotkey at T0 is forwarded and becomes the last forwarded trigger.
		rig.in <- pauseTrig(domain.PauseHotkey, pauseT0)
		first := rig.nextToggles(t, 1)

		// A UI pause 100 ms later is applied, and it is idempotent: repeating
		// it is applied again.
		clock.Advance(100 * time.Millisecond)
		if _, err := pause(ctx, nil); err != nil {
			t.Fatalf("session.pause at T0+100ms = %v, want applied", err)
		}
		if _, err := pause(ctx, nil); err != nil {
			t.Fatalf("repeated session.pause = %v, want applied", err)
		}

		// A gamepad 350 ms after T0 is forwarded: the UI pause did not move
		// the window (measured from T0, 250 ms would be discarded).
		rig.in <- pauseTrig(domain.PauseGamepad, pauseT0.Add(350*time.Millisecond))
		second := rig.nextToggles(t, 1)

		// Much later, a UI resume is applied and a hotkey 100 ms after it is
		// forwarded; the UI request did not move the window either.
		clock.Advance(9900 * time.Millisecond) // now T0+10s
		t1 := clock.Now()
		if _, err := resume(ctx, nil); err != nil {
			t.Fatalf("session.resume = %v, want applied", err)
		}
		clock.Advance(100 * time.Millisecond)
		rig.in <- pauseTrig(domain.PauseHotkey, t1.Add(100*time.Millisecond))
		third := rig.nextToggles(t, 1)

		wantToggles(t, first, pauseCall{reason: domain.PauseHotkey, at: pauseT0})
		wantToggles(t, second, pauseCall{reason: domain.PauseGamepad, at: pauseT0.Add(350 * time.Millisecond)})
		wantToggles(t, third, pauseCall{reason: domain.PauseHotkey, at: t1.Add(100 * time.Millisecond)})

		pauses, resumes := tr.uiInstants()
		wantInstants(t, "session.pause", pauses, pauseT0.Add(100*time.Millisecond), pauseT0.Add(100*time.Millisecond))
		wantInstants(t, "session.resume", resumes, t1)

		rig.r.Close()
		if n := countLog(uiLog.lines(t), "ERROR", ""); n != 0 {
			t.Errorf("UI handler logged %d Error records, want 0", n)
		}
	})
}

// wantInstants compares a list of instants with the expected ones, in order.
func wantInstants(t *testing.T, what string, got []time.Time, want ...time.Time) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s instants = %v, want %d instants", what, got, len(want))
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("%s instant %d = %v, want %v", what, i, got[i], want[i])
		}
	}
}

func TestPowerBridge(t *testing.T) {
	t.Run("PWR-005/OnSuspend repassa Suspend com o At do evento", func(t *testing.T) {
		verifyNoLeaks(t)
		if PowerHandlerTimeout != 1500*time.Millisecond {
			t.Errorf("PowerHandlerTimeout = %v, want 1.5 s", PowerHandlerTimeout)
		}
		tr := newPauseFake()
		buf := &pauseLogBuf{}
		b := &powerBridge{tr: tr, timeout: pauseWait, log: pauseLogger(buf)}
		b.OnSuspend(pauseT0)
		calls := tr.recorded()
		if len(calls) != 1 || calls[0].method != "Suspend" || !calls[0].at.Equal(pauseT0) {
			t.Fatalf("tracker calls = %+v, want one Suspend at %v", calls, pauseT0)
		}
		if n := countLog(buf.lines(t), "WARN", ""); n != 0 {
			t.Errorf("Warn records = %d, want 0 for an answer in time", n)
		}
	})

	t.Run("PWR-005/tracker lento: OnSuspend volta em no máximo o timeout e loga Warn tracker não respondeu à suspensão a tempo", func(t *testing.T) {
		verifyNoLeaks(t)
		// Unblocks a fake that is still waiting if the bridge never gives up.
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })

		tr := newPauseFake()
		ctxErr := make(chan error, 1)
		tr.suspendFn = func(ctx context.Context, at time.Time) error {
			// A slow tracker: it answers only when the bridge's context ends.
			select {
			case <-ctx.Done():
				ctxErr <- ctx.Err()
				return ctx.Err()
			case <-release:
				ctxErr <- nil
				return nil
			}
		}
		buf := &pauseLogBuf{}
		b := &powerBridge{tr: tr, timeout: 20 * time.Millisecond, log: pauseLogger(buf)}

		done := make(chan struct{})
		go func() {
			b.OnSuspend(pauseT0)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(pauseWait):
			t.Fatal("OnSuspend still waiting for the slow tracker after its timeout")
		}

		select {
		case err := <-ctxErr:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("tracker context ended with %v, want DeadlineExceeded (the bridge's timeout)", err)
			}
		default:
			t.Error("the slow tracker was never asked to suspend")
		}
		if n := countLog(buf.lines(t), "WARN", "tracker não respondeu à suspensão a tempo"); n != 1 {
			t.Errorf("Warn \"tracker não respondeu à suspensão a tempo\" records = %d, want 1", n)
		}
		if calls := tr.recorded(); len(calls) != 1 || !calls[0].at.Equal(pauseT0) {
			t.Errorf("tracker calls = %+v, want one Suspend at %v", calls, pauseT0)
		}
	})

	t.Run("PWR-005/OnWake repassa userPresent e At; erro loga Debug", func(t *testing.T) {
		verifyNoLeaks(t)
		wakeAt := pauseT0.Add(25 * time.Second)

		for _, present := range []bool{true, false} {
			t.Run(fmt.Sprintf("PWR-005/userPresent=%t", present), func(t *testing.T) {
				tr := newPauseFake()
				buf := &pauseLogBuf{}
				b := &powerBridge{tr: tr, timeout: pauseWait, log: pauseLogger(buf)}
				b.OnWake(wakeAt, present)
				calls := tr.recorded()
				if len(calls) != 1 || calls[0].method != "Wake" || !calls[0].at.Equal(wakeAt) || calls[0].userPresent != present {
					t.Fatalf("tracker calls = %+v, want one Wake at %v with userPresent %t", calls, wakeAt, present)
				}
			})
		}

		for _, tc := range []struct {
			name string
			err  error
		}{
			{"erro comum", errors.New("falha de teste")},
			{"ErrShuttingDown", tracker.ErrShuttingDown},
		} {
			t.Run("PWR-005/erro loga Debug ("+tc.name+")", func(t *testing.T) {
				tr := newPauseFake()
				tr.wakeFn = func(context.Context, time.Time, bool) error { return tc.err }
				buf := &pauseLogBuf{}
				b := &powerBridge{tr: tr, timeout: pauseWait, log: pauseLogger(buf)}
				b.OnWake(wakeAt, true)
				if n := countLog(buf.lines(t), "DEBUG", ""); n == 0 {
					t.Errorf("no Debug record for a Wake error; log = %+v", buf.lines(t))
				}
			})
		}
	})
}
