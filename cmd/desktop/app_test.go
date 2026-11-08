package main

// Tests for T12 (docs/specs/phase-2-skeleton/tasks.md): desktop bindings and the
// shared helpers used by the other cmd/desktop test files.
//
// Signature assumptions beyond tasks.md (the implementer must follow them):
//   - App has an unexported field `client pinger` (not *ipc.Client) so tests can
//     inject a fake; a nil client means "not connected yet" and AgentStatus
//     returns Connected=false without panicking.
//   - App has an unexported field `log *slog.Logger`.
//   - App has an unexported field `gate *libraryGate`.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
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

// fakeIPCClient is the long-lived client handed out by platform.newClient. The
// test feeds notifications through notes and closes it to end forwarding.
type fakeIPCClient struct {
	statusPinger
	notes chan ipc.Notification
}

func (c *fakeIPCClient) Notifications() <-chan ipc.Notification { return c.notes }
func (c *fakeIPCClient) Close() error                           { return nil }

func (c *fakeIPCClient) calls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.methods...)
}

type emitted struct {
	ctx   context.Context
	event string
	data  any
}

// forwardHarness wires an App with fake platform pieces around connectAndForward.
type forwardHarness struct {
	app       *App
	client    *fakeIPCClient
	gate      *libraryGate
	logBuf    *syncBuffer
	emits     chan emitted
	clientNew chan struct{} // closed when newClient was called
	done      chan struct{} // connectAndForward returned
}

func gateConnected(g *libraryGate) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.connected
}

func startForward(t *testing.T, wailsCtx context.Context, probe pinger, start func() error,
	clientStatus ipc.AgentStatus, root string) *forwardHarness {
	t.Helper()
	log, buf := captureLog()
	h := &forwardHarness{
		client:    &fakeIPCClient{statusPinger: statusPinger{status: clientStatus}, notes: make(chan ipc.Notification)},
		gate:      newLibraryGate((&fakeOpener{}).open),
		logBuf:    buf,
		emits:     make(chan emitted, 16),
		clientNew: make(chan struct{}),
		done:      make(chan struct{}),
	}
	var once sync.Once
	h.app = &App{
		log:   log,
		clock: domain.NewFakeClock(time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)),
		platform: platform{
			newProbe: func() (pinger, error) { return probe, nil },
			newClient: func(*slog.Logger) (ipcClient, error) {
				once.Do(func() { close(h.clientNew) })
				return h.client, nil
			},
			startAgent: func(string, string, *slog.Logger) error { return start() },
			emit: func(ctx context.Context, event string, data any) {
				h.emits <- emitted{ctx, event, data}
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(h.done)
		h.app.connectAndForward(ctx, wailsCtx, h.gate, start, root, log)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(testTimeout):
			t.Errorf("connectAndForward did not return within %v", testTimeout)
		}
		_ = h.gate.Close()
	})
	return h
}

func (h *forwardHarness) waitClient(t *testing.T) {
	t.Helper()
	select {
	case <-h.clientNew:
	case <-time.After(testTimeout):
		t.Fatalf("newClient was not called within %v", testTimeout)
	}
}

func (h *forwardHarness) send(t *testing.T, n ipc.Notification) {
	t.Helper()
	select {
	case h.client.notes <- n:
	case <-time.After(testTimeout):
		t.Fatalf("notification %q not consumed within %v", n.Method, testTimeout)
	}
}

func (h *forwardHarness) nextEmit(t *testing.T) emitted {
	t.Helper()
	select {
	case e := <-h.emits:
		return e
	case <-time.After(testTimeout):
		t.Fatalf("no event emitted within %v", testTimeout)
		return emitted{}
	}
}

// finish closes the notification channel and waits for connectAndForward.
func (h *forwardHarness) finish(t *testing.T) {
	t.Helper()
	close(h.client.notes)
	select {
	case <-h.done:
	case <-time.After(testTimeout):
		t.Fatalf("connectAndForward did not return after the notification channel closed")
	}
}

func heartbeatNote(t *testing.T, uptime int64) ipc.Notification {
	t.Helper()
	raw, err := json.Marshal(ipc.HeartbeatParams{UptimeSeconds: uptime})
	if err != nil {
		t.Fatalf("marshal heartbeat: %v", err)
	}
	return ipc.Notification{Method: ipc.MethodHeartbeat, Params: raw}
}

func logHasMsg(t *testing.T, buf *syncBuffer, level, msg string) bool {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		if fmt.Sprint(rec["level"]) == level && fmt.Sprint(rec["msg"]) == msg {
			return true
		}
	}
	return false
}

type ctxKey struct{}

func TestConnectAndForward(t *testing.T) {
	t.Parallel()

	t.Run("DSK-007/heartbeat após falha do ensureAgent marca conectado e checa dataDir", func(t *testing.T) {
		t.Parallel()
		// The probe fails at once and start fails, so ensureAgent gives up
		// without waiting on the clock.
		probe := &statusPinger{err: errors.New("pipe not found")}
		start := func() error { return errors.New("spawn failed") }
		agentStatus := sampleStatus() // DataDir differs from root below
		root := `D:\Outra\Pasta`
		wailsCtx := context.WithValue(context.Background(), ctxKey{}, "wails")
		h := startForward(t, wailsCtx, probe, start, agentStatus, root)

		h.waitClient(t)
		if gateConnected(h.gate) {
			t.Fatalf("gate connected before any heartbeat, want not connected after ensureAgent failed")
		}
		if !logHasMsg(t, h.logBuf, "ERROR", "agente não respondeu") {
			t.Errorf("missing Error log for ensureAgent failure; log:\n%s", h.logBuf.String())
		}

		h.send(t, heartbeatNote(t, 10))
		e := h.nextEmit(t)
		if !gateConnected(h.gate) {
			t.Errorf("gate not connected after the first heartbeat")
		}
		if e.event != heartbeatEvent {
			t.Errorf("emitted event %q, want %q", e.event, heartbeatEvent)
		}
		if got, want := h.client.calls(), []string{ipc.MethodStatus}; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("client calls after first heartbeat = %q, want %q", got, want)
		}
		h.client.mu.Lock()
		if len(h.client.hadDeadline) != 1 || !h.client.hadDeadline[0] ||
			h.client.remaining[0] <= 0 || h.client.remaining[0] > statusTimeout {
			t.Errorf("agent.status deadline = %v / %v, want one call within (0, %v]",
				h.client.hadDeadline, h.client.remaining, statusTimeout)
		}
		h.client.mu.Unlock()
		if !logHasMsg(t, h.logBuf, "WARN", "agente usa outra pasta de dados") {
			t.Errorf("missing DSK-006 data dir warning; log:\n%s", h.logBuf.String())
		}
		if !logHasMsg(t, h.logBuf, "INFO", "agente conectado") {
			t.Errorf("missing Info \"agente conectado\"; log:\n%s", h.logBuf.String())
		}

		// A second heartbeat is forwarded but does not query agent.status again.
		h.send(t, heartbeatNote(t, 11))
		_ = h.nextEmit(t)
		h.finish(t)
		if n := len(h.client.calls()); n != 1 {
			t.Errorf("client calls after two heartbeats = %d, want 1", n)
		}
		if n := countLevel(t, h.logBuf, "WARN"); n != 1 {
			t.Errorf("Warn records = %d, want 1 (data dir checked once); log:\n%s", n, h.logBuf.String())
		}
	})

	t.Run("DSK-003/heartbeat com params inválidos não é repassado", func(t *testing.T) {
		t.Parallel()
		st := sampleStatus()
		probe := &statusPinger{status: st} // agent answers at once
		start := func() error { return errors.New("start must not be called") }
		root := st.DataDir // same dir: no DSK-006 warning
		wailsCtx := context.WithValue(context.Background(), ctxKey{}, "wails")
		h := startForward(t, wailsCtx, probe, start, st, root)

		h.waitClient(t)
		if !gateConnected(h.gate) {
			t.Errorf("gate not connected after ensureAgent succeeded")
		}
		h.send(t, ipc.Notification{Method: ipc.MethodHeartbeat}) // no params
		h.send(t, ipc.Notification{Method: ipc.MethodHeartbeat, Params: json.RawMessage(`"x"`)})
		h.send(t, ipc.Notification{Method: "agent.other", Params: json.RawMessage(`{}`)})
		h.send(t, heartbeatNote(t, 7))

		e := h.nextEmit(t)
		h.finish(t)

		if e.event != heartbeatEvent {
			t.Errorf("emitted event %q, want %q", e.event, heartbeatEvent)
		}
		if e.ctx != wailsCtx {
			t.Errorf("emit got a different context, want the Wails context")
		}
		hb, ok := e.data.(ipc.HeartbeatParams)
		if !ok || hb.UptimeSeconds != 7 {
			t.Errorf("emitted data = %#v, want ipc.HeartbeatParams{UptimeSeconds: 7}", e.data)
		}
		select {
		case extra := <-h.emits:
			t.Errorf("unexpected extra event emitted: %#v", extra)
		default:
		}
		if n := countLevel(t, h.logBuf, "WARN"); n != 2 {
			t.Errorf("Warn records = %d, want 2 (one per invalid heartbeat); log:\n%s", n, h.logBuf.String())
		}
		if n := len(h.client.calls()); n != 0 {
			t.Errorf("client calls = %d, want 0 (already connected through ensureAgent)", n)
		}
	})
}

func TestAppLibrarySummary(t *testing.T) {
	t.Parallel()

	t.Run("DSK-007/binding antes de conectar retorna ErrAgentNotReady sem abrir o banco", func(t *testing.T) {
		t.Parallel()
		op := &fakeOpener{}
		a := &App{gate: newLibraryGate(op.open), log: discardLog()}

		_, err := a.LibrarySummary()

		if !errors.Is(err, ErrAgentNotReady) {
			t.Errorf("LibrarySummary() err = %v, want ErrAgentNotReady", err)
		}
		if n := op.count(); n != 0 {
			t.Errorf("open called %d times, want 0", n)
		}
	})

	t.Run("DSK-007/binding após MarkConnected repassa o resumo", func(t *testing.T) {
		t.Parallel()
		fs := &fakeStore{summary: summaryOf(7, 2)}
		op := &fakeOpener{results: []openResult{{st: fs}}}
		g := newLibraryGate(op.open)
		t.Cleanup(func() { _ = g.Close() })
		a := &App{gate: g, log: discardLog()}
		g.MarkConnected()

		got, err := a.LibrarySummary()

		if err != nil {
			t.Fatalf("LibrarySummary() err = %v", err)
		}
		if want := (LibrarySummaryView{Games: 7, Platforms: 2}); got != want {
			t.Errorf("LibrarySummary() = %+v, want %+v", got, want)
		}
	})
}
