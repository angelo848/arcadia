package main

// Tests for T12 (docs/specs/phase-2-skeleton/tasks.md): desktop bindings and the
// shared helpers used by the other cmd/arcadia test files. TestAppShutdown and
// TestSecondInstance are from T09 (docs/specs/single-exe/tasks.md).
//
// T09 assumptions (the implementer must follow them):
//   - platform gains `quit func(ctx context.Context)` and
//     `showWindow func(ctx context.Context)`; both receive the Wails ctx given
//     to startup (like emit) and are never called with a nil ctx.
//   - App.startup starts the supervisor on a goroutine owned by App.wg and
//     returns without waiting for Ensure; its Ensure is ensureAgent over
//     platform.newProbe / platform.startAgent with App.clock.
//
// Signature assumptions beyond tasks.md (the implementer must follow them):
//   - App has an unexported field `client pinger` (not *ipc.Client) so tests can
//     inject a fake; a nil client means "not connected yet" and AgentStatus
//     returns Connected=false without panicking.
//   - App has an unexported field `log *slog.Logger`.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// lumberjackMillRun is the lumberjack cleanup goroutine that Close does not stop
// (design.md, internal/logging).
const lumberjackMillRun = "gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"

// testTimeout bounds every real-time wait in these tests (safety net only).
const testTimeout = 5 * time.Second

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction(lumberjackMillRun))
}

// captureLog returns a JSON slog logger at Debug and the buffer it writes to.
// The buffer is guarded so it can be written from other goroutines.
func captureLog() (*slog.Logger, *syncBuffer) {
	buf := &syncBuffer{}
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// countLevel returns how many JSON log records in buf have the given level.
func countLevel(t *testing.T, buf *syncBuffer, level string) int {
	t.Helper()
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		if fmt.Sprint(rec["level"]) == level {
			n++
		}
	}
	return n
}

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

// statusPinger answers agent.status with a fixed status or a fixed error and
// records the calls it received.
type statusPinger struct {
	status ipc.AgentStatus
	err    error

	mu          sync.Mutex
	methods     []string
	hadDeadline []bool
	remaining   []time.Duration
}

func (p *statusPinger) Call(ctx context.Context, method string, params, result any) error {
	dl, ok := ctx.Deadline()
	p.mu.Lock()
	p.methods = append(p.methods, method)
	p.hadDeadline = append(p.hadDeadline, ok)
	if ok {
		p.remaining = append(p.remaining, time.Until(dl))
	}
	p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	return fillResult(method, p.status, result)
}

// fillResult emulates the JSON decoding the real ipc.Client does into result.
func fillResult(method string, status ipc.AgentStatus, result any) error {
	var v any
	switch method {
	case ipc.MethodStatus:
		v = status
	case ipc.MethodPing:
		v = ipc.PingResult{Pong: true}
	default:
		return &ipc.Error{Code: ipc.CodeMethodNotFound, Message: "Method not found"}
	}
	if result == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, result)
}

func sampleStatus() ipc.AgentStatus {
	return ipc.AgentStatus{
		Version:       "1.2.3",
		PID:           4242,
		StartedAt:     time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
		DataDir:       `C:\Arcadia\data`,
		UptimeSeconds: 3725,
	}
}

func TestAgentStatusView(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		pinger pinger // nil => App without client
	}{
		{"DSK-003/erro de Call vira Connected=false", &statusPinger{err: ipc.ErrConnectionLost}},
		{"DSK-003/erro de contexto vira Connected=false", &statusPinger{err: fmt.Errorf("ipc: call: %w", context.DeadlineExceeded)}},
		{"DSK-003/erro JSON-RPC vira Connected=false", &statusPinger{err: &ipc.Error{Code: ipc.CodeInternalError, Message: "boom"}}},
		{"DSK-003/sem client ainda vira Connected=false", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := &App{log: discardLog()}
			if tc.pinger != nil {
				a.client = tc.pinger
			}
			got := a.AgentStatus()
			if got != (StatusView{}) {
				t.Errorf("AgentStatus() = %+v, want zero StatusView (Connected=false)", got)
			}
		})
	}

	t.Run("DSK-003/Call bem-sucedido preenche os campos de agent.status", func(t *testing.T) {
		t.Parallel()
		st := sampleStatus()
		p := &statusPinger{status: st}
		a := &App{client: p, log: discardLog()}

		got := a.AgentStatus()

		if !got.Connected {
			t.Errorf("Connected = false, want true")
		}
		if got.Version != st.Version {
			t.Errorf("Version = %q, want %q", got.Version, st.Version)
		}
		if got.PID != st.PID {
			t.Errorf("PID = %d, want %d", got.PID, st.PID)
		}
		if got.DataDir != st.DataDir {
			t.Errorf("DataDir = %q, want %q", got.DataDir, st.DataDir)
		}
		if got.UptimeSeconds != st.UptimeSeconds {
			t.Errorf("UptimeSeconds = %d, want %d", got.UptimeSeconds, st.UptimeSeconds)
		}
		at, err := time.Parse(time.RFC3339, got.StartedAt)
		if err != nil {
			t.Errorf("StartedAt = %q, not RFC 3339: %v", got.StartedAt, err)
		} else if !at.Equal(st.StartedAt) {
			t.Errorf("StartedAt = %q, want instant %s", got.StartedAt, st.StartedAt.Format(time.RFC3339))
		}

		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.methods) != 1 || p.methods[0] != ipc.MethodStatus {
			t.Errorf("Call methods = %q, want [%q]", p.methods, ipc.MethodStatus)
		}
	})

	t.Run("DSK-003/Call usa timeout de 500 ms", func(t *testing.T) {
		t.Parallel()
		p := &statusPinger{status: sampleStatus()}
		a := &App{client: p, log: discardLog()}

		_ = a.AgentStatus()

		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.hadDeadline) != 1 || !p.hadDeadline[0] {
			t.Fatalf("Call context deadlines = %v, want one call with a deadline", p.hadDeadline)
		}
		if r := p.remaining[0]; r <= 0 || r > 500*time.Millisecond {
			t.Errorf("Call deadline %v ahead, want within (0, 500ms]", r)
		}
	})
}

// fakeAgent emulates ipc.Client.Call against an agent that may not be running:
// while not running, Call waits until ctx is done (like the real client waiting
// for a connection). start "launches" the agent; once running, pending and new
// calls are answered, unless mute is set (agent never answers).
type fakeAgent struct {
	status ipc.AgentStatus
	mute   bool

	running   chan struct{} // closed when the agent runs
	startOnce sync.Once

	mu         sync.Mutex
	startCalls int
	callCount  int
	startedCh  chan struct{} // closed on the first start call
	startedSet sync.Once
}

func newFakeAgent(running bool, mute bool) *fakeAgent {
	a := &fakeAgent{status: sampleStatus(), mute: mute, running: make(chan struct{}), startedCh: make(chan struct{})}
	if running {
		close(a.running)
	}
	return a
}

func (a *fakeAgent) Call(ctx context.Context, method string, params, result any) error {
	a.mu.Lock()
	a.callCount++
	a.mu.Unlock()
	if !a.mute {
		select {
		case <-a.running:
			return fillResult(method, a.status, result)
		default:
		}
	}
	running := a.running
	if a.mute {
		running = nil // never answers
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("ipc: call %s: waiting for connection: %w", method, ctx.Err())
	case <-running:
		return fillResult(method, a.status, result)
	}
}

func (a *fakeAgent) start() error {
	a.mu.Lock()
	a.startCalls++
	a.mu.Unlock()
	a.startedSet.Do(func() { close(a.startedCh) })
	a.startOnce.Do(func() { close(a.running) })
	return nil
}

func (a *fakeAgent) starts() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.startCalls
}

type ensureResult struct {
	st  ipc.AgentStatus
	err error
}

func runEnsure(ctx context.Context, p pinger, start func() error, clock *domain.FakeClock, timeout time.Duration) <-chan ensureResult {
	ch := make(chan ensureResult, 1)
	go func() {
		st, err := ensureAgent(ctx, p, start, clock, timeout)
		ch <- ensureResult{st, err}
	}()
	return ch
}

func waitEnsure(t *testing.T, ch <-chan ensureResult) ensureResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(testTimeout):
		t.Fatalf("ensureAgent did not return within %v (real time)", testTimeout)
		return ensureResult{}
	}
}

// driveClock advances clock in small steps while waiters are pending, until stop
// is closed, ctx is done, or (when limit > 0) the total advanced would reach limit.
// It returns a function that stops the driver and waits for it.
func driveClock(clock *domain.FakeClock, step, limit time.Duration, stop <-chan struct{}) (wait func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		var advanced time.Duration
		for limit <= 0 || advanced+step < limit {
			if err := clock.BlockUntilWaiters(ctx, 1); err != nil {
				return
			}
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			default:
			}
			clock.Advance(step)
			advanced += step
		}
	}()
	return func() { cancel(); <-done }
}

func TestEnsureAgent(t *testing.T) {
	t.Parallel()
	const timeout = 5 * time.Second
	const step = 100 * time.Millisecond
	t0 := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

	t.Run("DSK-001/agente rodando não chama start", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(t0)
		ag := newFakeAgent(true, false)

		r := waitEnsure(t, runEnsure(context.Background(), ag, ag.start, clock, timeout))

		if r.err != nil {
			t.Fatalf("ensureAgent err = %v, want nil", r.err)
		}
		if n := ag.starts(); n != 0 {
			t.Errorf("start called %d times, want 0", n)
		}
		assertStatus(t, r.st, ag.status)
	})

	t.Run("DSK-001/agente parado chama start uma vez e conecta", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(t0)
		ag := newFakeAgent(false, false)
		// Advance at most to just under the overall timeout, and only until start
		// was called, so the 5 s deadline never fires in this case.
		stopDriver := driveClock(clock, step, timeout, ag.startedCh)
		defer stopDriver()

		r := waitEnsure(t, runEnsure(context.Background(), ag, ag.start, clock, timeout))

		if r.err != nil {
			t.Fatalf("ensureAgent err = %v, want nil", r.err)
		}
		if n := ag.starts(); n != 1 {
			t.Errorf("start called %d times, want 1", n)
		}
		assertStatus(t, r.st, ag.status)
		if el := clock.Now().Sub(t0); el >= timeout {
			t.Errorf("fake time elapsed %v, want < %v", el, timeout)
		}
	})

	t.Run("DSK-001/sem resposta em 5 s retorna erro", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(t0)
		ag := newFakeAgent(false, true)
		stopDriver := driveClock(clock, step, 0, nil)
		defer stopDriver()

		r := waitEnsure(t, runEnsure(context.Background(), ag, ag.start, clock, timeout))
		stopDriver()

		if r.err == nil {
			t.Fatalf("ensureAgent err = nil, want error after %v without answer", timeout)
		}
		if n := ag.starts(); n != 1 {
			t.Errorf("start called %d times, want 1", n)
		}
		if el := clock.Now().Sub(t0); el < timeout {
			t.Errorf("ensureAgent gave up after %v of fake time, want >= %v", el, timeout)
		}
	})

	t.Run("DSK-001/start falha retorna erro", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(t0)
		ag := newFakeAgent(false, false)
		boom := errors.New("spawn failed")
		var mu sync.Mutex
		calls := 0
		start := func() error { mu.Lock(); calls++; mu.Unlock(); return boom }
		stopDriver := driveClock(clock, step, 0, nil)
		defer stopDriver()

		r := waitEnsure(t, runEnsure(context.Background(), ag, start, clock, timeout))
		stopDriver()

		if !errors.Is(r.err, boom) {
			t.Errorf("ensureAgent err = %v, want errors.Is(err, start error)", r.err)
		}
		mu.Lock()
		defer mu.Unlock()
		if calls != 1 {
			t.Errorf("start called %d times, want 1", calls)
		}
	})

	t.Run("DSK-001/contexto cancelado retorna erro", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(t0)
		ag := newFakeAgent(false, true)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		r := waitEnsure(t, runEnsure(ctx, ag, ag.start, clock, timeout))

		if r.err == nil {
			t.Fatalf("ensureAgent err = nil, want error for cancelled context")
		}
		if !errors.Is(r.err, context.Canceled) {
			t.Errorf("ensureAgent err = %v, want errors.Is(err, context.Canceled)", r.err)
		}
	})
}

func assertStatus(t *testing.T, got, want ipc.AgentStatus) {
	t.Helper()
	if got.Version != want.Version || got.PID != want.PID || got.DataDir != want.DataDir ||
		got.UptimeSeconds != want.UptimeSeconds || !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("status = %+v, want %+v", got, want)
	}
}

func TestAgentStatusZeroStartedAt(t *testing.T) {
	t.Parallel()

	t.Run("DSK-003/StartedAt zero vira string vazia", func(t *testing.T) {
		t.Parallel()
		st := sampleStatus()
		st.StartedAt = time.Time{}
		a := &App{client: &statusPinger{status: st}, log: discardLog()}

		got := a.AgentStatus()

		if !got.Connected {
			t.Errorf("Connected = false, want true")
		}
		if got.StartedAt != "" {
			t.Errorf("StartedAt = %q, want empty string for zero time", got.StartedAt)
		}
	})
}

// ---------------------------------------------------------------------------
// T09: App wiring of the supervisor (fakes from supervisor_test.go).

type ctxKey struct{}

// appHarness is an App with fake platform hooks and a FakeClock; its data
// directory is a fresh t.TempDir.
type appHarness struct {
	app      *App
	clock    *domain.FakeClock
	client   *fakeClient
	dir      string
	wailsCtx context.Context

	clients chan struct{}        // one value per newClient call
	starts  chan struct{}        // one value per startAgent call
	startN  atomic.Int32         // startAgent calls
	shows   chan context.Context // showWindow calls
	emits   chan emitted         // emit calls
}

type emitted struct {
	ctx   context.Context
	event string
	data  any
}

// newAppHarness wires probe as the agent.status probe and start as the body of
// platform.startAgent. Cleanup runs OnShutdown (idempotent).
func newAppHarness(t *testing.T, probe pinger, start func() error) *appHarness {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	h := &appHarness{
		clock:    domain.NewFakeClock(supT0),
		client:   newFakeClient(sampleStatus(), 1),
		dir:      dir,
		wailsCtx: context.WithValue(context.Background(), ctxKey{}, "wails"),
		clients:  make(chan struct{}, 8),
		starts:   make(chan struct{}, 64),
		shows:    make(chan context.Context, 8),
		emits:    make(chan emitted, 64),
	}
	h.app = &App{
		cfg:   desktopConfig{DataDir: dir, Exe: `C:\Arcadia\arcadia.exe`, ExeDir: `C:\Arcadia`},
		clock: h.clock,
		platform: platform{
			newProbe: func() (pinger, error) { return probe, nil },
			newClient: func(*slog.Logger) (ipcClient, error) {
				h.clients <- struct{}{}
				return h.client, nil
			},
			startAgent: func(string, string, *slog.Logger) error {
				h.startN.Add(1)
				h.starts <- struct{}{}
				return start()
			},
			emit: func(ctx context.Context, event string, data any) {
				select {
				case h.emits <- emitted{ctx, event, data}:
				default:
				}
			},
			quit:       func(context.Context) {},
			showWindow: func(ctx context.Context) { h.shows <- ctx },
		},
	}
	t.Cleanup(func() { h.app.shutdown(context.Background()) })
	return h
}

func (h *appHarness) waitStart(t *testing.T) {
	t.Helper()
	select {
	case <-h.starts:
	case <-time.After(testTimeout):
		t.Fatalf("startAgent not called within %v (calls so far %d)", testTimeout, h.startN.Load())
	}
}

// shutdownWithin runs OnShutdown and fails if it does not return in testTimeout.
func (h *appHarness) shutdownWithin(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.app.shutdown(context.Background())
	}()
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatalf("OnShutdown did not return within %v", testTimeout)
	}
}

// readDesktopLog returns the JSON records of R/logs/desktop.log.
func readDesktopLog(t *testing.T, root string) []map[string]any {
	t.Helper()
	file := filepath.Join(root, "logs", desktopLogFile)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("desktop.log line is not JSON: %q: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

// blockingPinger blocks every Call until ctx is done (an agent that never
// answers) and signals the first call.
type blockingPinger struct {
	entered chan struct{}
	once    sync.Once
	mu      sync.Mutex
	methods []string
}

func newBlockingPinger() *blockingPinger { return &blockingPinger{entered: make(chan struct{})} }

func (p *blockingPinger) Call(ctx context.Context, method string, _, _ any) error {
	p.mu.Lock()
	p.methods = append(p.methods, method)
	p.mu.Unlock()
	p.once.Do(func() { close(p.entered) })
	<-ctx.Done()
	return fmt.Errorf("probe %s: %w", method, ctx.Err())
}

func TestAppShutdown(t *testing.T) {
	t.Parallel()

	t.Run("EXE-025/OnShutdown só retorna depois de o supervisor terminar", func(t *testing.T) {
		t.Parallel()
		entered := make(chan struct{})
		release := make(chan struct{})
		var enterOnce, releaseOnce sync.Once
		defer releaseOnce.Do(func() { close(release) })
		// The probe fails at once; start blocks (it does not take a ctx), so
		// the supervisor goroutine is busy until the test releases it.
		h := newAppHarness(t, &statusPinger{err: errors.New("pipe not found")}, func() error {
			enterOnce.Do(func() { close(entered) })
			<-release
			return errors.New("spawn failed")
		})
		h.app.startup(h.wailsCtx)
		select {
		case <-entered:
		case <-time.After(testTimeout):
			t.Fatalf("startAgent not called within %v", testTimeout)
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			h.app.shutdown(context.Background())
		}()
		select {
		case <-done:
			t.Fatalf("OnShutdown returned while the supervisor goroutine was still running")
		case <-time.After(quietWindow):
		}
		releaseOnce.Do(func() { close(release) })
		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Fatalf("OnShutdown did not return within %v after the supervisor could finish", testTimeout)
		}
		if n := h.startN.Load(); n != 1 {
			t.Errorf("startAgent calls = %d, want 1", n)
		}
	})

	t.Run("EXE-025/nenhum start depois do início do OnShutdown", func(t *testing.T) {
		t.Parallel()
		probe := newBlockingPinger() // ensureAgent is in its first probe
		h := newAppHarness(t, probe, func() error { return nil })
		h.app.startup(h.wailsCtx)
		select {
		case <-probe.entered:
		case <-time.After(testTimeout):
			t.Fatalf("agent probe not called within %v", testTimeout)
		}

		h.shutdownWithin(t)
		h.clock.Advance(time.Hour)
		if n := h.startN.Load(); n != 0 {
			t.Errorf("startAgent called %d times after OnShutdown began, want 0", n)
		}
	})

	t.Run("DSK-002/OnShutdown não encerra o agente", func(t *testing.T) {
		t.Parallel()
		probe := &statusPinger{status: sampleStatus()} // the agent answers at once
		h := newAppHarness(t, probe, func() error { return errors.New("start must not be called") })
		h.app.startup(h.wailsCtx)
		select {
		case <-h.clients:
		case <-time.After(testTimeout):
			t.Fatalf("newClient not called within %v", testTimeout)
		}

		h.shutdownWithin(t)
		if !h.client.isClosed() {
			t.Errorf("IPC client not closed by OnShutdown")
		}
		probe.mu.Lock()
		probeCalls := append([]string(nil), probe.methods...)
		probe.mu.Unlock()
		for _, m := range append(probeCalls, h.client.methodCalls()...) {
			if m != ipc.MethodStatus {
				t.Errorf("call %q sent to the agent; the UI only reads agent.status and never stops the agent", m)
			}
		}
		if n := h.startN.Load(); n != 0 {
			t.Errorf("startAgent calls = %d, want 0", n)
		}
	})

	t.Run("EXE-025/start recusado depois do OnShutdown marcar stopped", func(t *testing.T) {
		t.Parallel()
		// OnShutdown has no hook between marking stopped and cancelling ctx, so
		// the test performs that first step itself (under a.mu, like shutdown)
		// while ensureAgent is in its first probe, then lets the probe fail
		// with ctx still alive: ensureAgent calls start, which must refuse.
		probe := newGatedPinger(errors.New("pipe not found"))
		h := newAppHarness(t, probe, func() error { return nil })
		h.app.startup(h.wailsCtx)
		select {
		case <-probe.entered:
		case <-time.After(testTimeout):
			t.Fatalf("agent probe not called within %v", testTimeout)
		}

		h.app.mu.Lock()
		h.app.stopped = true
		cancel, logCloser := h.app.cancel, h.app.logCloser
		h.app.mu.Unlock()
		close(probe.release)

		// Ensure returned (refused start) and the supervisor went on to create
		// the client; Publish refuses it, so run returns on its own.
		select {
		case <-h.clients:
		case <-time.After(testTimeout):
			t.Fatalf("newClient not called within %v", testTimeout)
		}
		waitDone := make(chan struct{})
		go func() {
			defer close(waitDone)
			h.app.wg.Wait()
		}()
		select {
		case <-waitDone:
		case <-time.After(testTimeout):
			t.Fatalf("supervisor goroutine did not return within %v", testTimeout)
		}

		if n := h.startN.Load(); n != 0 {
			t.Errorf("startAgent called %d times after stopped was set, want 0", n)
		}
		if !h.client.isClosed() {
			t.Errorf("IPC client not closed after Publish was refused")
		}

		// The rest of OnShutdown (it returns early since stopped is set).
		cancel()
		if err := logCloser.Close(); err != nil {
			t.Errorf("log close: %v", err)
		}
		var refused bool
		for _, rec := range readDesktopLog(t, h.dir) {
			if rec["msg"] == msgRelaunchFail && strings.Contains(fmt.Sprint(rec["error"]), "UI encerrando") {
				refused = true
			}
		}
		if !refused {
			t.Errorf("desktop.log has no %q record with the refused start", msgRelaunchFail)
		}
	})
}

// gatedPinger blocks every Call until release is closed (then fails with err)
// or ctx is done, and signals the first call.
type gatedPinger struct {
	err     error
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newGatedPinger(err error) *gatedPinger {
	return &gatedPinger{err: err, entered: make(chan struct{}), release: make(chan struct{})}
}

func (p *gatedPinger) Call(ctx context.Context, method string, _, _ any) error {
	p.once.Do(func() { close(p.entered) })
	select {
	case <-p.release:
		return fmt.Errorf("probe %s: %w", method, p.err)
	case <-ctx.Done():
		return fmt.Errorf("probe %s: %w", method, ctx.Err())
	}
}

// alterado em 2026-11-22 (fase 15): secondInstance passou a receber bigBox (ATB-009); estes casos
// usam false, ou seja, EXE-011 sem mudança.
func TestSecondInstance(t *testing.T) {
	t.Parallel()

	t.Run("EXE-012/secondInstance chama showWindow, loga Info e Kick", func(t *testing.T) {
		t.Parallel()
		// The probe and start fail at once: startup + 2 attempts = 3 failures,
		// then the supervisor gives up (EXE-022).
		h := newAppHarness(t, &statusPinger{err: errors.New("pipe not found")}, func() error {
			return errors.New("spawn failed")
		})
		h.app.startup(h.wailsCtx)
		stopDriver := driveClock(h.clock, 100*time.Millisecond, 0, nil)
		for range relaunchMaxFailures {
			h.waitStart(t)
		}
		stopDriver() // the clock no longer moves: only Kick can cause a new start

		h.app.secondInstance(false)
		select {
		case ctx := <-h.shows:
			if ctx != h.wailsCtx {
				t.Errorf("showWindow got a different context, want the Wails context")
			}
		case <-time.After(testTimeout):
			t.Fatalf("showWindow not called within %v", testTimeout)
		}
		h.waitStart(t) // Kick in GaveUp: immediate attempt (EXE-012)

		h.shutdownWithin(t)
		var found bool
		for _, rec := range readDesktopLog(t, h.dir) {
			if rec["msg"] == msgSecondInstance {
				found = true
				if rec["level"] != "INFO" {
					t.Errorf("%q level = %v, want INFO", msgSecondInstance, rec["level"])
				}
			}
		}
		if !found {
			t.Errorf("desktop.log has no %q record", msgSecondInstance)
		}
	})

	t.Run("EXE-012/secondInstance antes do startup não falha", func(t *testing.T) {
		t.Parallel()
		h := newAppHarness(t, &statusPinger{status: sampleStatus()}, func() error { return nil })

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("secondInstance before startup panicked: %v", r)
				}
			}()
			h.app.secondInstance(false)
		}()
		select {
		case ctx := <-h.shows:
			if ctx == nil {
				t.Errorf("showWindow called with a nil context before startup")
			}
		default:
		}
		if n := h.startN.Load(); n != 0 {
			t.Errorf("startAgent calls = %d, want 0", n)
		}
	})

	t.Run("EXE-012/secondInstance depois do OnShutdown não mostra janela, não loga e não faz Kick", func(t *testing.T) {
		t.Parallel()
		h := newAppHarness(t, &statusPinger{status: sampleStatus()}, func() error { return nil })
		h.app.startup(h.wailsCtx)
		select {
		case <-h.clients:
		case <-time.After(testTimeout):
			t.Fatalf("newClient not called within %v", testTimeout)
		}
		h.shutdownWithin(t)

		h.app.secondInstance(false)
		select {
		case <-h.shows:
			t.Errorf("showWindow called after OnShutdown")
		default:
		}
		h.app.mu.Lock()
		sup := h.app.sup
		h.app.mu.Unlock()
		if sup == nil {
			t.Fatalf("supervisor not created by startup")
		}
		if n := len(sup.kick); n != 0 {
			t.Errorf("Kick queued after OnShutdown (kick channel len %d, want 0)", n)
		}
		for _, rec := range readDesktopLog(t, h.dir) {
			if rec["msg"] == msgSecondInstance {
				t.Errorf("%q logged after OnShutdown", msgSecondInstance)
			}
		}
		if n := h.startN.Load(); n != 0 {
			t.Errorf("startAgent calls = %d, want 0", n)
		}
	})
}
