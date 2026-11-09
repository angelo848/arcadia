package main

// Tests for T09 (docs/specs/single-exe/tasks.md): the agent supervisor
// (design.md, "Supervisor do agente", transition table).
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - run returns when ctx is done or when the client's Notifications() channel
//     is closed (design.md). Tests close only the notification channel as a
//     barrier: notifications already buffered are handled before run returns.
//   - In its main loop the supervisor receives from StateChanged() in every
//     state. The fake channel is unbuffered by default, so a test send returns
//     only once the supervisor took the signal; the signal carries no data and
//     the supervisor reads Connected() after it (EXE-019).
//   - Ensure runs on the supervisor goroutine with a ctx derived from run's ctx.
//   - upSince of the startup connection is the instant Ensure returned ok, so
//     the decision-4 drop (no connection 5 s after Ensure) is a short connection.
//   - Log attrs: "attempt" and "max" are integers; "reason" and "error" hold
//     the text (a string or an error value).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// supT0 is the fake instant every supervisor test starts at.
var supT0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// quietWindow is how long (real time) a negative check waits for something
// that must not happen.
const quietWindow = 50 * time.Millisecond

// Log messages fixed by requirements.md / design.md.
const (
	msgRelaunchFail   = "falha ao relançar o agente"
	msgRelaunching    = "relançando o agente"
	msgGiveUp         = "desistindo de relançar o agente"
	msgReconnected    = "agente reconectado"
	msgDropped        = "agente desconectado sem aviso de encerramento"
	msgShortConn      = "agente caiu logo após subir"
	msgAgentQuit      = "agente encerrado; fechando a UI"
	msgSecondInstance = "segunda instância da UI; trazendo a janela à frente"
	msgDataDir        = "agente usa outra pasta de dados"
)

// ---------------------------------------------------------------------------
// Recording logger: every record is kept and waiters are woken on each one.

type logRec struct {
	level slog.Level
	msg   string
	attrs map[string]string
}

type recLog struct {
	mu      sync.Mutex
	recs    []logRec
	changed chan struct{}
}

func newRecLog() *recLog { return &recLog{changed: make(chan struct{})} }

func (l *recLog) logger() *slog.Logger { return slog.New(&recHandler{store: l}) }

func (l *recLog) add(r logRec) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recs = append(l.recs, r)
	close(l.changed)
	l.changed = make(chan struct{})
}

// len returns how many records exist; use it as the "from" of find and wait.
func (l *recLog) len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recs)
}

// find returns the records at index >= from with the given level and message.
func (l *recLog) find(from int, level slog.Level, msg string) []logRec {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []logRec
	for _, r := range l.recs[min(from, len(l.recs)):] {
		if r.level == level && r.msg == msg {
			out = append(out, r)
		}
	}
	return out
}

func (l *recLog) countLevel(level slog.Level) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, r := range l.recs {
		if r.level == level {
			n++
		}
	}
	return n
}

func (l *recLog) dump() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var b strings.Builder
	for _, r := range l.recs {
		fmt.Fprintf(&b, "  %s %q %v\n", r.level, r.msg, r.attrs)
	}
	return b.String()
}

// wait blocks until a record at index >= from has the given level and message
// and returns the first such record.
func (l *recLog) wait(t *testing.T, from int, level slog.Level, msg string) logRec {
	t.Helper()
	deadline := time.After(testTimeout)
	for {
		l.mu.Lock()
		for _, r := range l.recs[min(from, len(l.recs)):] {
			if r.level == level && r.msg == msg {
				l.mu.Unlock()
				return r
			}
		}
		ch := l.changed
		l.mu.Unlock()
		select {
		case <-ch:
		case <-deadline:
			t.Fatalf("no %s log %q within %v; log:\n%s", level, msg, testTimeout, l.dump())
			return logRec{}
		}
	}
}

type recHandler struct {
	store *recLog
	attrs []slog.Attr
}

func (h *recHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recHandler) Handle(_ context.Context, r slog.Record) error {
	rec := logRec{level: r.Level, msg: r.Message, attrs: map[string]string{}}
	add := func(a slog.Attr) { rec.attrs[a.Key] = fmt.Sprint(a.Value.Resolve().Any()) }
	for _, a := range h.attrs {
		add(a)
	}
	r.Attrs(func(a slog.Attr) bool { add(a); return true })
	h.store.add(rec)
	return nil
}

func (h *recHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &recHandler{store: h.store, attrs: append(slices.Clone(h.attrs), as...)}
}

func (h *recHandler) WithGroup(string) slog.Handler { return h }

func wantAttr(t *testing.T, r logRec, key, want string) {
	t.Helper()
	got, ok := r.attrs[key]
	if !ok {
		t.Errorf("log %q has no attr %q (attrs %v)", r.msg, key, r.attrs)
		return
	}
	if got != want {
		t.Errorf("log %q attr %s = %q, want %q", r.msg, key, got, want)
	}
}

// ---------------------------------------------------------------------------
// FakeClock helpers.

// pendingWaiters returns the exact number of pending FakeClock waiters without
// blocking (BlockUntilWaiters with a cancelled ctx checks the count first).
func pendingWaiters(c *domain.FakeClock) int {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n := 0
	for c.BlockUntilWaiters(ctx, n+1) == nil {
		n++
	}
	return n
}

// waitWaiters blocks until at least n waiters are pending.
func waitWaiters(t *testing.T, c *domain.FakeClock, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if err := c.BlockUntilWaiters(ctx, n); err != nil {
		t.Fatalf("FakeClock waiters did not reach %d within %v (pending %d)", n, testTimeout, pendingWaiters(c))
	}
}

// noNewWaiters checks that no waiter beyond n is registered within quietWindow.
func noNewWaiters(t *testing.T, c *domain.FakeClock, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), quietWindow)
	defer cancel()
	if err := c.BlockUntilWaiters(ctx, n+1); err == nil {
		t.Errorf("a new FakeClock waiter was registered (pending %d, want %d)", pendingWaiters(c), n)
	}
}

// ---------------------------------------------------------------------------
// Fake ipcClient with controlled channels.

type fakeClient struct {
	statusPinger // Call: answers agent.status and records the calls

	notes     chan ipc.Notification
	state     chan struct{}
	connected atomic.Bool

	closed    chan struct{}
	closeOnce sync.Once
	notesOnce sync.Once

	// connProbe, when set before the supervisor starts, receives one value
	// (without blocking) per Connected() call.
	connProbe chan struct{}
}

// newFakeClient returns a disconnected client. stateCap is the StateChanged
// buffer: 0 makes deliver a rendezvous with the supervisor.
func newFakeClient(status ipc.AgentStatus, stateCap int) *fakeClient {
	return &fakeClient{
		statusPinger: statusPinger{status: status},
		notes:        make(chan ipc.Notification, 16), // like ipc.Client (IPC-017)
		state:        make(chan struct{}, stateCap),
		closed:       make(chan struct{}),
	}
}

func (c *fakeClient) Notifications() <-chan ipc.Notification { return c.notes }
func (c *fakeClient) StateChanged() <-chan struct{}          { return c.state }

func (c *fakeClient) Connected() bool {
	up := c.connected.Load()
	if c.connProbe != nil {
		select {
		case c.connProbe <- struct{}{}:
		default:
		}
	}
	return up
}

// waitConnCall waits for one Connected() call recorded by connProbe.
func (c *fakeClient) waitConnCall(t *testing.T) {
	t.Helper()
	select {
	case <-c.connProbe:
	case <-time.After(testTimeout):
		t.Fatalf("Connected() not called within %v", testTimeout)
	}
}

// Close closes the notification channel (StateChanged is left open so a test
// send can never panic; run must exit on the notification channel).
func (c *fakeClient) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.finishNotes()
	})
	return nil
}

func (c *fakeClient) finishNotes() { c.notesOnce.Do(func() { close(c.notes) }) }

func (c *fakeClient) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

// deliver sets the connection state and sends one StateChanged signal, waiting
// until it is taken (or buffered, when stateCap > 0).
func (c *fakeClient) deliver(t *testing.T, up bool) {
	t.Helper()
	c.connected.Store(up)
	select {
	case c.state <- struct{}{}:
	case <-time.After(testTimeout):
		t.Fatalf("StateChanged signal (connected=%v) not taken within %v", up, testTimeout)
	}
}

// signal sets the state and sends a signal without blocking, like ipc.Client.
func (c *fakeClient) signal(up bool) {
	c.connected.Store(up)
	select {
	case c.state <- struct{}{}:
	default:
	}
}

func (c *fakeClient) send(t *testing.T, n ipc.Notification) {
	t.Helper()
	select {
	case c.notes <- n:
	case <-time.After(testTimeout):
		t.Fatalf("notification %q not accepted within %v", n.Method, testTimeout)
	}
}

func (c *fakeClient) methodCalls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.methods...)
}

func shutdownNote(t *testing.T, reason string) ipc.Notification {
	t.Helper()
	raw, err := json.Marshal(ipc.ShutdownParams{Reason: reason})
	if err != nil {
		t.Fatalf("marshal shutdown params: %v", err)
	}
	return ipc.Notification{Method: ipc.MethodShutdown, Params: raw}
}

func heartbeatNote(t *testing.T, uptime int64) ipc.Notification {
	t.Helper()
	raw, err := json.Marshal(ipc.HeartbeatParams{UptimeSeconds: uptime})
	if err != nil {
		t.Fatalf("marshal heartbeat: %v", err)
	}
	return ipc.Notification{Method: ipc.MethodHeartbeat, Params: raw}
}

// ---------------------------------------------------------------------------
// Fake Ensure: every call is handed to the test, which answers it.

type ensureReply struct {
	st  ipc.AgentStatus
	err error
}

type ensureCall struct {
	ctx   context.Context
	at    time.Time // fake Now when Ensure was called
	reply chan ensureReply
}

func (c ensureCall) ok(st ipc.AgentStatus) { c.reply <- ensureReply{st: st} }
func (c ensureCall) fail(err error)        { c.reply <- ensureReply{err: err} }

type fakeEnsure struct {
	clock *domain.FakeClock
	calls chan ensureCall
	n     atomic.Int32
}

func (f *fakeEnsure) ensure(ctx context.Context) (ipc.AgentStatus, error) {
	f.n.Add(1)
	c := ensureCall{ctx: ctx, at: f.clock.Now(), reply: make(chan ensureReply, 1)}
	f.calls <- c
	select {
	case r := <-c.reply:
		return r.st, r.err
	case <-ctx.Done():
		// Like ensureAgent: a cancelled ctx returns without starting anything.
		return ipc.AgentStatus{}, fmt.Errorf("ensure agent: %w", ctx.Err())
	}
}

// fakeGate records MarkConnected.
type fakeGate struct {
	n  atomic.Int32
	ch chan struct{}
}

func (g *fakeGate) MarkConnected() {
	g.n.Add(1)
	select {
	case g.ch <- struct{}{}:
	default:
	}
}

func (g *fakeGate) wait(t *testing.T) {
	t.Helper()
	select {
	case <-g.ch:
	case <-time.After(testTimeout):
		t.Fatalf("gate.MarkConnected not called within %v", testTimeout)
	}
}

// ---------------------------------------------------------------------------
// Harness around newSupervisor / run.

type supEmit struct {
	event string
	data  any
}

type supOptions struct {
	root     string                       // default: sampleStatus().DataDir (no DSK-006 warning)
	stateCap int                          // StateChanged buffer of the fake client
	publish  func(c ipcClient) bool       // default: accepts
	emit     func(event string, data any) // default: records into emits
}

type supHarness struct {
	clock     *domain.FakeClock
	ens       *fakeEnsure
	client    *fakeClient
	clients   chan struct{} // one value per NewClient call
	published atomic.Int32
	gate      *fakeGate
	logs      *recLog
	emits     chan supEmit
	quits     chan struct{}
	quitN     atomic.Int32
	sup       *supervisor

	cancel context.CancelFunc
	done   chan struct{}
}

func newSupHarness(t *testing.T, o supOptions) *supHarness {
	t.Helper()
	if o.root == "" {
		o.root = sampleStatus().DataDir
	}
	h := &supHarness{
		clock:   domain.NewFakeClock(supT0),
		client:  newFakeClient(sampleStatus(), o.stateCap),
		clients: make(chan struct{}, 8),
		gate:    &fakeGate{ch: make(chan struct{}, 8)},
		logs:    newRecLog(),
		emits:   make(chan supEmit, 64),
		quits:   make(chan struct{}, 8),
		done:    make(chan struct{}),
	}
	h.ens = &fakeEnsure{clock: h.clock, calls: make(chan ensureCall, 64)}
	publish := o.publish
	if publish == nil {
		publish = func(ipcClient) bool { return true }
	}
	emit := o.emit
	if emit == nil {
		emit = func(event string, data any) { h.emits <- supEmit{event, data} }
	}
	h.sup = newSupervisor(supervisorConfig{
		Clock:  h.clock,
		Ensure: h.ens.ensure,
		NewClient: func() (ipcClient, error) {
			h.clients <- struct{}{}
			return h.client, nil
		},
		Publish: func(c ipcClient) bool {
			h.published.Add(1)
			return publish(c)
		},
		Gate: h.gate,
		Root: o.root,
		Emit: emit,
		Quit: func() {
			h.quitN.Add(1)
			select {
			case h.quits <- struct{}{}:
			default:
			}
		},
		Log: h.logs.logger(),
	})
	return h
}

// start runs the supervisor on its own goroutine; cleanup cancels and waits.
func (h *supHarness) start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		defer close(h.done)
		h.sup.run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(testTimeout):
			t.Errorf("supervisor run did not return within %v after cancel", testTimeout)
		}
	})
}

func (h *supHarness) stop(t *testing.T) {
	t.Helper()
	h.cancel()
	h.waitDone(t)
}

func (h *supHarness) waitDone(t *testing.T) {
	t.Helper()
	select {
	case <-h.done:
	case <-time.After(testTimeout):
		t.Fatalf("supervisor run did not return within %v", testTimeout)
	}
}

func (h *supHarness) isDone() bool {
	select {
	case <-h.done:
		return true
	default:
		return false
	}
}

func (h *supHarness) nextEnsure(t *testing.T) ensureCall {
	t.Helper()
	select {
	case c := <-h.ens.calls:
		return c
	case <-time.After(testTimeout):
		t.Fatalf("Ensure not called within %v (fake now %v); log:\n%s", testTimeout, h.clock.Now().Sub(supT0), h.logs.dump())
		return ensureCall{}
	}
}

// noEnsure checks that Ensure is not called within quietWindow.
func (h *supHarness) noEnsure(t *testing.T) {
	t.Helper()
	select {
	case c := <-h.ens.calls:
		t.Errorf("unexpected Ensure call at fake +%v", c.at.Sub(supT0))
		c.fail(errors.New("unexpected call"))
	case <-time.After(quietWindow):
	}
}

func (h *supHarness) waitClient(t *testing.T) {
	t.Helper()
	select {
	case <-h.clients:
	case <-time.After(testTimeout):
		t.Fatalf("NewClient not called within %v", testTimeout)
	}
}

func (h *supHarness) waitQuit(t *testing.T) {
	t.Helper()
	select {
	case <-h.quits:
	case <-time.After(testTimeout):
		t.Fatalf("Quit not called within %v", testTimeout)
	}
}

// connectOK answers the startup Ensure with success at supT0 and brings the
// client connection up.
func (h *supHarness) connectOK(t *testing.T) {
	t.Helper()
	h.nextEnsure(t).ok(sampleStatus())
	h.waitClient(t)
	h.client.deliver(t, true)
}

// failStart fails the startup Ensure and waits for the client and for the
// relaunch wait timer.
func (h *supHarness) failStart(t *testing.T, err error) {
	t.Helper()
	h.nextEnsure(t).fail(err)
	h.waitClient(t)
	waitWaiters(t, h.clock, 1)
}

// connectAndDrop connects at supT0, keeps the connection for age and drops it
// without agent.shutdown. It returns once the drop was logged and its wait
// timer is pending.
func (h *supHarness) connectAndDrop(t *testing.T, age time.Duration) {
	t.Helper()
	h.connectOK(t)
	h.clock.Advance(age)
	p := pendingWaiters(h.clock)
	from := h.logs.len()
	h.client.deliver(t, false)
	h.logs.wait(t, from, slog.LevelWarn, msgDropped)
	waitWaiters(t, h.clock, p+1)
}

// giveUpFromStart drives the supervisor to GaveUp: the startup Ensure fails
// (1st failure), then the attempts after 2 s and 4 s fail too.
func (h *supHarness) giveUpFromStart(t *testing.T) {
	t.Helper()
	h.failStart(t, errors.New("start boom"))
	for i, w := range relaunchWaits[1:] {
		h.clock.Advance(w)
		c := h.nextEnsure(t)
		q := pendingWaiters(h.clock)
		c.fail(fmt.Errorf("attempt %d boom", i+2))
		if i == 0 {
			waitWaiters(t, h.clock, q+1)
		}
	}
	h.logs.wait(t, 0, slog.LevelError, msgGiveUp)
}

// ---------------------------------------------------------------------------

func TestSupervisorStartup(t *testing.T) {
	t.Parallel()

	t.Run("EXE-024/ensure inicial falha: conta 1 falha, loga falha ao relançar o agente e espera 2 s", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)

		c1 := h.nextEnsure(t)
		if !c1.at.Equal(supT0) {
			t.Errorf("startup Ensure at fake +%v, want +0", c1.at.Sub(supT0))
		}
		c1.fail(errors.New("spawn boom"))
		r := h.logs.wait(t, 0, slog.LevelError, msgRelaunchFail)
		wantAttr(t, r, "attempt", "1")
		if !strings.Contains(r.attrs["error"], "spawn boom") {
			t.Errorf("log %q error = %q, want it to contain %q", r.msg, r.attrs["error"], "spawn boom")
		}
		h.waitClient(t) // the client is created after a failed Ensure too
		waitWaiters(t, h.clock, 1)

		from := h.logs.len()
		h.clock.Advance(relaunchWaits[1])
		c2 := h.nextEnsure(t)
		if want := supT0.Add(2 * time.Second); !c2.at.Equal(want) {
			t.Errorf("second Ensure at fake +%v, want +2s", c2.at.Sub(supT0))
		}
		info := h.logs.wait(t, from, slog.LevelInfo, msgRelaunching)
		wantAttr(t, info, "attempt", "2")
		wantAttr(t, info, "max", "3")
		c2.fail(errors.New("again"))
	})

	t.Run("EXE-024/nada dispara 1ns antes de 2 s", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.failStart(t, errors.New("spawn boom"))
		p := pendingWaiters(h.clock)

		h.clock.Advance(2*time.Second - time.Nanosecond)
		if got := pendingWaiters(h.clock); got != p {
			t.Errorf("a wait timer fired before 2 s (pending %d -> %d)", p, got)
		}
		h.noEnsure(t)
		if n := len(h.logs.find(0, slog.LevelInfo, msgRelaunching)); n != 0 {
			t.Errorf("%q logged %d times before 2 s, want 0", msgRelaunching, n)
		}

		h.clock.Advance(time.Nanosecond)
		c := h.nextEnsure(t)
		if want := supT0.Add(2 * time.Second); !c.at.Equal(want) {
			t.Errorf("Ensure at fake +%v, want +2s", c.at.Sub(supT0))
		}
		c.fail(errors.New("again"))
	})

	t.Run("DSK-007/ensure ok marca o gate e checa dataDir", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{root: `D:\Outra\Pasta`}) // differs from sampleStatus().DataDir
		h.start(t)

		h.nextEnsure(t).ok(sampleStatus())
		h.gate.wait(t)
		r := h.logs.wait(t, 0, slog.LevelWarn, msgDataDir)
		wantAttr(t, r, "agent_data_dir", sampleStatus().DataDir)
		h.waitClient(t)
		h.client.deliver(t, true)
		h.client.send(t, heartbeatNote(t, 1))
		h.client.finishNotes()
		h.waitDone(t)

		if n := len(h.logs.find(0, slog.LevelWarn, msgDataDir)); n != 1 {
			t.Errorf("%q logged %d times, want 1", msgDataDir, n)
		}
		for _, m := range h.client.methodCalls() {
			if m == ipc.MethodStatus {
				t.Errorf("client agent.status called although Ensure already returned the status")
			}
		}
	})

	t.Run("DSK-007/após ensure falho, primeiro heartbeat ou Connected marca o gate e checa dataDir", func(t *testing.T) {
		t.Parallel()
		triggers := []struct {
			name string
			fire func(t *testing.T, h *supHarness)
		}{
			{"DSK-007/primeiro heartbeat", func(t *testing.T, h *supHarness) {
				h.client.connected.Store(true) // the StateChanged signal may come later
				h.client.send(t, heartbeatNote(t, 10))
			}},
			{"DSK-007/Connected verdadeiro", func(t *testing.T, h *supHarness) {
				h.client.deliver(t, true)
			}},
		}
		for _, tc := range triggers {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				h := newSupHarness(t, supOptions{root: `D:\Outra\Pasta`})
				h.start(t)
				h.failStart(t, errors.New("spawn boom"))
				if n := h.gate.n.Load(); n != 0 {
					t.Fatalf("gate marked %d times after a failed Ensure, want 0", n)
				}

				tc.fire(t, h)
				h.gate.wait(t)
				h.logs.wait(t, 0, slog.LevelWarn, msgDataDir)

				// Later heartbeats are forwarded but do not query agent.status again.
				h.client.send(t, heartbeatNote(t, 11))
				h.client.finishNotes()
				h.waitDone(t)

				calls := h.client.methodCalls()
				if len(calls) != 1 || calls[0] != ipc.MethodStatus {
					t.Errorf("client calls = %q, want exactly [%q]", calls, ipc.MethodStatus)
				}
				h.client.mu.Lock()
				if len(h.client.hadDeadline) != 1 || !h.client.hadDeadline[0] ||
					h.client.remaining[0] <= 0 || h.client.remaining[0] > statusTimeout {
					t.Errorf("agent.status deadline = %v / %v, want one call within (0, %v]",
						h.client.hadDeadline, h.client.remaining, statusTimeout)
				}
				h.client.mu.Unlock()
				if n := len(h.logs.find(0, slog.LevelWarn, msgDataDir)); n != 1 {
					t.Errorf("%q logged %d times, want 1 (data dir checked once)", msgDataDir, n)
				}
			})
		}
	})

	t.Run("EXE-020/ensure ok mas Client não conecta em 5 s é tratado como queda", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)

		h.nextEnsure(t).ok(sampleStatus())
		h.waitClient(t)
		waitWaiters(t, h.clock, 1) // the first-dial timer (decision 4)
		p := pendingWaiters(h.clock)

		h.clock.Advance(firstDialTimeout - time.Nanosecond)
		if got := pendingWaiters(h.clock); got != p {
			t.Errorf("a timer fired before %v (pending %d -> %d)", firstDialTimeout, p, got)
		}
		if n := len(h.logs.find(0, slog.LevelWarn, msgDropped)); n != 0 {
			t.Errorf("drop logged before %v", firstDialTimeout)
		}

		from := h.logs.len()
		h.clock.Advance(time.Nanosecond)
		h.logs.wait(t, from, slog.LevelWarn, msgDropped)
		h.logs.wait(t, from, slog.LevelWarn, msgShortConn) // 5 s < 60 s: one failure
		waitWaiters(t, h.clock, 1)

		h.clock.Advance(relaunchWaits[1])
		c := h.nextEnsure(t)
		if want := supT0.Add(firstDialTimeout + relaunchWaits[1]); !c.at.Equal(want) {
			t.Errorf("relaunch Ensure at fake +%v, want +%v", c.at.Sub(supT0), want.Sub(supT0))
		}
		c.fail(errors.New("again"))
	})

	t.Run("EXE-020/timer do primeiro dial com Connected verdadeiro não é queda", func(t *testing.T) {
		t.Parallel()
		// The client connected but its StateChanged signal was not taken yet
		// when the first-dial timer fires (both ready together): the timer
		// branch reads Connected() and must treat it as a connection.
		h := newSupHarness(t, supOptions{stateCap: 1})
		h.client.connProbe = make(chan struct{}, 8)
		h.start(t)

		h.nextEnsure(t).ok(sampleStatus())
		h.waitClient(t)
		waitWaiters(t, h.clock, 1) // the first-dial timer (decision 4)
		h.client.waitConnCall(t)   // armFirstDial read Connected() == false

		h.client.connected.Store(true) // no StateChanged signal
		h.clock.Advance(firstDialTimeout)
		h.client.waitConnCall(t) // the timer branch read Connected()
		noNewWaiters(t, h.clock, 0)
		h.client.finishNotes()
		h.waitDone(t) // run returned: its fields can be read

		if n := len(h.logs.find(0, slog.LevelWarn, msgDropped)); n != 0 {
			t.Errorf("%q logged although Connected() was true; log:\n%s", msgDropped, h.logs.dump())
		}
		if n := len(h.logs.find(0, slog.LevelWarn, msgShortConn)); n != 0 {
			t.Errorf("%q logged although Connected() was true", msgShortConn)
		}
		if got := pendingWaiters(h.clock); got != 0 {
			t.Errorf("pending FakeClock waiters = %d, want 0 (no relaunch timer)", got)
		}
		if h.sup.failures != 0 {
			t.Errorf("failures = %d, want 0", h.sup.failures)
		}
		if h.sup.state != stateConnected {
			t.Errorf("state = %v, want stateConnected", h.sup.state)
		}
		if n := h.ens.n.Load(); n != 1 {
			t.Errorf("Ensure calls = %d, want 1 (startup only)", n)
		}
	})
}

func TestSupervisorDrop(t *testing.T) {
	t.Parallel()

	t.Run("EXE-020/queda após ≥ 60 s loga Warn, espera 1 s e loga relançando o agente com attempt=1 max=3", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectAndDrop(t, stableConnection)
		dropAt := h.clock.Now()

		r := h.logs.find(0, slog.LevelWarn, msgDropped)[0]
		if _, ok := r.attrs["retry_in"]; !ok {
			t.Errorf("log %q has no retry_in attr (attrs %v)", r.msg, r.attrs)
		}
		if n := len(h.logs.find(0, slog.LevelWarn, msgShortConn)); n != 0 {
			t.Errorf("%q logged for a connection of %v", msgShortConn, stableConnection)
		}

		p := pendingWaiters(h.clock)
		h.clock.Advance(time.Second - time.Nanosecond)
		if got := pendingWaiters(h.clock); got != p {
			t.Errorf("a wait timer fired before 1 s (pending %d -> %d)", p, got)
		}
		if n := len(h.logs.find(0, slog.LevelInfo, msgRelaunching)); n != 0 {
			t.Errorf("%q logged before 1 s", msgRelaunching)
		}
		h.clock.Advance(time.Nanosecond)
		info := h.logs.wait(t, 0, slog.LevelInfo, msgRelaunching)
		wantAttr(t, info, "attempt", "1")
		wantAttr(t, info, "max", "3")
		c := h.nextEnsure(t)
		if !c.at.Equal(dropAt.Add(time.Second)) {
			t.Errorf("Ensure %v after the drop, want 1s", c.at.Sub(dropAt))
		}
		c.fail(errors.New("again"))
	})

	t.Run("EXE-020/tentativa chama Ensure", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectAndDrop(t, stableConnection)
		before := h.ens.n.Load()

		h.clock.Advance(relaunchWaits[0])
		c := h.nextEnsure(t)
		if n := h.ens.n.Load(); n != before+1 {
			t.Errorf("Ensure calls = %d, want %d", n, before+1)
		}
		if err := c.ctx.Err(); err != nil {
			t.Errorf("Ensure ctx already done: %v", err)
		}
		c.fail(errors.New("again"))
	})

	t.Run("EXE-021/conexão volta durante a espera: Ensure não é chamado e loga agente reconectado", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectAndDrop(t, stableConnection)

		from := h.logs.len()
		h.client.deliver(t, true)
		h.logs.wait(t, from, slog.LevelInfo, msgReconnected)

		h.clock.Advance(relaunchWaits[len(relaunchWaits)-1]) // past any wait
		h.noEnsure(t)
		if n := len(h.logs.find(from, slog.LevelInfo, msgRelaunching)); n != 0 {
			t.Errorf("%q logged after the connection came back", msgRelaunching)
		}
	})

	t.Run("EXE-021/Ensure ok na tentativa loga agente reconectado", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectAndDrop(t, stableConnection)

		h.clock.Advance(relaunchWaits[0])
		c := h.nextEnsure(t)
		from := h.logs.len()
		c.ok(sampleStatus())
		h.logs.wait(t, from, slog.LevelInfo, msgReconnected)
	})

	t.Run("EXE-023/conexão < 60 s conta falha e loga agente caiu logo após subir", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectAndDrop(t, 30*time.Second)
		dropAt := h.clock.Now()
		h.logs.wait(t, 0, slog.LevelWarn, msgShortConn)

		// One failure: the wait is relaunchWaits[1] = 2 s.
		p := pendingWaiters(h.clock)
		h.clock.Advance(relaunchWaits[1] - time.Nanosecond)
		if got := pendingWaiters(h.clock); got != p {
			t.Errorf("a wait timer fired before 2 s (pending %d -> %d)", p, got)
		}
		h.clock.Advance(time.Nanosecond)
		info := h.logs.wait(t, 0, slog.LevelInfo, msgRelaunching)
		wantAttr(t, info, "attempt", "2")
		c := h.nextEnsure(t)
		if !c.at.Equal(dropAt.Add(relaunchWaits[1])) {
			t.Errorf("Ensure %v after the drop, want %v", c.at.Sub(dropAt), relaunchWaits[1])
		}
		c.fail(errors.New("again"))
	})

	t.Run("EXE-023/conexão ≥ 60 s zera o contador", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.failStart(t, errors.New("spawn boom")) // failures = 1

		from := h.logs.len()
		h.client.deliver(t, true) // comes back on its own
		h.logs.wait(t, from, slog.LevelInfo, msgReconnected)

		h.clock.Advance(stableConnection)
		p := pendingWaiters(h.clock)
		from = h.logs.len()
		h.client.deliver(t, false)
		h.logs.wait(t, from, slog.LevelWarn, msgDropped)
		if n := len(h.logs.find(from, slog.LevelWarn, msgShortConn)); n != 0 {
			t.Errorf("%q logged for a connection of %v", msgShortConn, stableConnection)
		}
		waitWaiters(t, h.clock, p+1)
		dropAt := h.clock.Now()

		// Counter reset: the wait is relaunchWaits[0] = 1 s and attempt = 1.
		h.clock.Advance(relaunchWaits[0])
		info := h.logs.wait(t, from, slog.LevelInfo, msgRelaunching)
		wantAttr(t, info, "attempt", "1")
		c := h.nextEnsure(t)
		if !c.at.Equal(dropAt.Add(relaunchWaits[0])) {
			t.Errorf("Ensure %v after the drop, want %v", c.at.Sub(dropAt), relaunchWaits[0])
		}
		c.fail(errors.New("again"))
	})
}

// failUntilGiveUp connects, drops after a stable connection and fails the
// three attempts. With early, each wait is first advanced to 1ns before its
// end and checked not to have fired. It returns the drop instant and the calls.
func failUntilGiveUp(t *testing.T, h *supHarness, early bool) (time.Time, []ensureCall) {
	t.Helper()
	h.connectAndDrop(t, stableConnection)
	dropAt := h.clock.Now()
	var calls []ensureCall
	for i, w := range relaunchWaits {
		p := pendingWaiters(h.clock)
		if early {
			h.clock.Advance(w - time.Nanosecond)
			if got := pendingWaiters(h.clock); got != p {
				t.Errorf("wait %d (%v) fired 1ns early (pending %d -> %d)", i, w, p, got)
			}
			h.clock.Advance(time.Nanosecond)
		} else {
			h.clock.Advance(w)
		}
		c := h.nextEnsure(t)
		calls = append(calls, c)
		q := pendingWaiters(h.clock)
		c.fail(fmt.Errorf("attempt %d boom", i+1))
		if i < len(relaunchWaits)-1 {
			waitWaiters(t, h.clock, q+1)
		}
	}
	h.logs.wait(t, 0, slog.LevelError, msgGiveUp)
	return dropAt, calls
}

func TestSupervisorLimit(t *testing.T) {
	t.Parallel()

	t.Run("EXE-022/esperas 1 s 2 s 4 s indexadas pelas falhas", func(t *testing.T) {
		t.Parallel()
		if relaunchMaxFailures != 3 {
			t.Errorf("relaunchMaxFailures = %d, want 3", relaunchMaxFailures)
		}
		if want := [3]time.Duration{time.Second, 2 * time.Second, 4 * time.Second}; relaunchWaits != want {
			t.Errorf("relaunchWaits = %v, want %v", relaunchWaits, want)
		}
		h := newSupHarness(t, supOptions{})
		h.start(t)
		dropAt, calls := failUntilGiveUp(t, h, false)
		want := []time.Duration{1 * time.Second, 3 * time.Second, 7 * time.Second}
		for i, c := range calls {
			if got := c.at.Sub(dropAt); got != want[i] {
				t.Errorf("attempt %d at drop+%v, want drop+%v", i+1, got, want[i])
			}
		}
	})

	t.Run("EXE-022/sem disparo 1ns antes de cada espera", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		_, calls := failUntilGiveUp(t, h, true)
		if len(calls) != relaunchMaxFailures {
			t.Errorf("attempts = %d, want %d", len(calls), relaunchMaxFailures)
		}
	})

	t.Run("EXE-022/cada falha loga Error com attempt e error", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		failUntilGiveUp(t, h, false)

		fails := h.logs.find(0, slog.LevelError, msgRelaunchFail)
		if len(fails) != 3 {
			t.Fatalf("%q logged %d times, want 3; log:\n%s", msgRelaunchFail, len(fails), h.logs.dump())
		}
		for i, r := range fails {
			wantAttr(t, r, "attempt", fmt.Sprint(i+1))
			if want := fmt.Sprintf("attempt %d boom", i+1); !strings.Contains(r.attrs["error"], want) {
				t.Errorf("failure %d error = %q, want it to contain %q", i+1, r.attrs["error"], want)
			}
		}
		infos := h.logs.find(0, slog.LevelInfo, msgRelaunching)
		if len(infos) != 3 {
			t.Fatalf("%q logged %d times, want 3", msgRelaunching, len(infos))
		}
		for i, r := range infos {
			wantAttr(t, r, "attempt", fmt.Sprint(i+1))
			wantAttr(t, r, "max", "3")
		}
	})

	t.Run("EXE-022/3ª falha loga desistindo de relançar o agente e não tenta mais", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		failUntilGiveUp(t, h, false)
		p := pendingWaiters(h.clock)

		noNewWaiters(t, h.clock, p)
		h.clock.Advance(time.Hour)
		h.noEnsure(t)
		if n := len(h.logs.find(0, slog.LevelError, msgGiveUp)); n != 1 {
			t.Errorf("%q logged %d times, want 1", msgGiveUp, n)
		}
		if n := h.ens.n.Load(); n != 4 {
			t.Errorf("Ensure calls = %d, want 4 (startup + 3 attempts)", n)
		}
	})

	t.Run("EXE-022/em GaveUp, conexão que volta é detectada", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		failUntilGiveUp(t, h, false)

		from := h.logs.len()
		h.client.deliver(t, true) // e.g. an agent started by hand (EXE-004)
		h.logs.wait(t, from, slog.LevelInfo, msgReconnected)
		h.noEnsure(t)
	})
}

func TestSupervisorShutdownNotice(t *testing.T) {
	t.Parallel()

	t.Run("EXE-017/agent.shutdown loga Info com reason e chama Quit uma vez", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		h.client.send(t, shutdownNote(t, "quit"))
		h.waitQuit(t)
		r := h.logs.wait(t, 0, slog.LevelInfo, msgAgentQuit)
		wantAttr(t, r, "reason", "quit")
		h.client.finishNotes()
		h.waitDone(t)
		if n := h.quitN.Load(); n != 1 {
			t.Errorf("Quit called %d times, want 1", n)
		}
	})

	t.Run("EXE-017/queda depois de agent.shutdown não chama Ensure", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{stateCap: 1}) // in Quitting the signal may be ignored
		h.start(t)
		h.connectOK(t)
		h.client.send(t, shutdownNote(t, "quit"))
		h.waitQuit(t)

		p := pendingWaiters(h.clock)
		h.client.signal(false)
		noNewWaiters(t, h.clock, p)
		h.noEnsure(t)
		h.clock.Advance(time.Minute) // also fires the first-dial timer: ignored in Quitting
		h.noEnsure(t)
		if n := len(h.logs.find(0, slog.LevelWarn, msgDropped)); n != 0 {
			t.Errorf("%q logged after agent.shutdown", msgDropped)
		}
		if n := h.ens.n.Load(); n != 1 {
			t.Errorf("Ensure calls = %d, want 1 (startup only)", n)
		}
		if n := h.quitN.Load(); n != 1 {
			t.Errorf("Quit called %d times, want 1", n)
		}
	})

	t.Run("EXE-017/agent.shutdown entregue antes do sinal de queda é visto primeiro", func(t *testing.T) {
		t.Parallel()
		// The supervisor is held inside Emit while the client delivers
		// agent.shutdown and then signals the drop (EXE-019 order). Both are
		// ready when it returns to its select; whichever it picks, it must
		// end up quitting without relaunching. Repeated to exercise both picks.
		for i := range 8 {
			entered := make(chan struct{})
			release := make(chan struct{})
			var enterOnce, releaseOnce sync.Once
			emit := func(string, any) {
				enterOnce.Do(func() {
					close(entered)
					<-release
				})
			}
			h := newSupHarness(t, supOptions{stateCap: 1, emit: emit})
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			h.start(t)
			h.connectOK(t)

			h.client.send(t, heartbeatNote(t, 1))
			select {
			case <-entered:
			case <-time.After(testTimeout):
				t.Fatalf("iteration %d: heartbeat not emitted within %v", i, testTimeout)
			}
			p := pendingWaiters(h.clock)
			h.client.send(t, shutdownNote(t, "quit"))
			h.client.signal(false)
			releaseOnce.Do(func() { close(release) })

			h.waitQuit(t)
			noNewWaiters(t, h.clock, p)
			h.noEnsure(t)
			if n := len(h.logs.find(0, slog.LevelWarn, msgDropped)); n != 0 {
				t.Errorf("iteration %d: %q logged although agent.shutdown came first", i, msgDropped)
			}
			if n := h.quitN.Load(); n != 1 {
				t.Errorf("iteration %d: Quit called %d times, want 1", i, n)
			}
			h.stop(t)
		}
	})

	t.Run("EXE-017/segundo agent.shutdown não chama Quit de novo", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		h.client.send(t, shutdownNote(t, "quit"))
		h.client.send(t, shutdownNote(t, "other"))
		h.client.finishNotes() // barrier: buffered notifications are handled before run returns
		h.waitDone(t)
		if n := h.quitN.Load(); n != 1 {
			t.Errorf("Quit called %d times, want 1", n)
		}
		if n := len(h.logs.find(0, slog.LevelInfo, msgAgentQuit)); n != 1 {
			t.Errorf("%q logged %d times, want 1", msgAgentQuit, n)
		}
	})
}

func TestSupervisorKick(t *testing.T) {
	t.Parallel()

	t.Run("EXE-012/Kick em GaveUp zera falhas e tenta sem espera", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.giveUpFromStart(t)
		gaveUpAt := h.clock.Now()

		from := h.logs.len()
		h.sup.Kick()
		c := h.nextEnsure(t) // no Advance: immediate attempt
		if !c.at.Equal(gaveUpAt) {
			t.Errorf("Kick attempt at +%v after giving up, want +0", c.at.Sub(gaveUpAt))
		}
		info := h.logs.wait(t, from, slog.LevelInfo, msgRelaunching)
		wantAttr(t, info, "attempt", "1")

		// failures was reset: this failure is the 1st, so the next wait is 2 s
		// and the supervisor does not give up again.
		q := pendingWaiters(h.clock)
		c.fail(errors.New("kick boom"))
		waitWaiters(t, h.clock, q+1)
		if n := len(h.logs.find(from, slog.LevelError, msgGiveUp)); n != 0 {
			t.Errorf("gave up again after one failure following Kick")
		}
		p := pendingWaiters(h.clock)
		h.clock.Advance(relaunchWaits[1] - time.Nanosecond)
		if got := pendingWaiters(h.clock); got != p {
			t.Errorf("a wait timer fired before 2 s (pending %d -> %d)", p, got)
		}
		h.clock.Advance(time.Nanosecond)
		c2 := h.nextEnsure(t)
		if !c2.at.Equal(gaveUpAt.Add(relaunchWaits[1])) {
			t.Errorf("next attempt at +%v, want +%v", c2.at.Sub(gaveUpAt), relaunchWaits[1])
		}
		c2.fail(errors.New("again"))
	})

	t.Run("EXE-012/Kick em outro estado não tenta", func(t *testing.T) {
		t.Parallel()

		t.Run("EXE-012/Kick esperando a tentativa", func(t *testing.T) {
			t.Parallel()
			h := newSupHarness(t, supOptions{})
			h.start(t)
			h.failStart(t, errors.New("spawn boom"))
			p := pendingWaiters(h.clock)

			h.sup.Kick()
			h.noEnsure(t)
			if got := pendingWaiters(h.clock); got != p {
				t.Errorf("Kick changed the pending timers (%d -> %d)", p, got)
			}
			h.clock.Advance(relaunchWaits[1])
			c := h.nextEnsure(t)
			if want := supT0.Add(relaunchWaits[1]); !c.at.Equal(want) {
				t.Errorf("attempt at +%v, want +%v (Kick must not shorten the wait)", c.at.Sub(supT0), relaunchWaits[1])
			}
			c.fail(errors.New("again"))
		})

		t.Run("EXE-012/Kick conectado", func(t *testing.T) {
			t.Parallel()
			h := newSupHarness(t, supOptions{})
			h.start(t)
			h.connectOK(t)

			h.sup.Kick()
			h.noEnsure(t)
			if n := h.ens.n.Load(); n != 1 {
				t.Errorf("Ensure calls = %d, want 1", n)
			}
		})
	})

	t.Run("EXE-012/Kick não bloqueia", func(t *testing.T) {
		t.Parallel()

		// Not running: nobody reads the kick channel.
		idle := newSupHarness(t, supOptions{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			for range 100 {
				idle.sup.Kick()
			}
		}()
		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Fatalf("Kick blocked on a supervisor that is not running")
		}

		// Running but busy inside Ensure.
		h := newSupHarness(t, supOptions{})
		h.start(t)
		c := h.nextEnsure(t)
		busy := make(chan struct{})
		go func() {
			defer close(busy)
			for range 100 {
				h.sup.Kick()
			}
		}()
		select {
		case <-busy:
		case <-time.After(testTimeout):
			t.Fatalf("Kick blocked while the supervisor was inside Ensure")
		}
		c.fail(errors.New("boom"))
	})
}

func TestSupervisorStop(t *testing.T) {
	t.Parallel()

	t.Run("EXE-025/ctx cancelado durante a espera: Ensure não é chamado e run retorna", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.failStart(t, errors.New("spawn boom"))

		h.cancel()
		h.waitDone(t)
		h.clock.Advance(time.Hour)
		select {
		case c := <-h.ens.calls:
			t.Errorf("Ensure called at fake +%v after ctx was cancelled", c.at.Sub(supT0))
		default:
		}
		if n := h.ens.n.Load(); n != 1 {
			t.Errorf("Ensure calls = %d, want 1", n)
		}
	})

	t.Run("EXE-025/ctx cancelado durante Ensure: start não ocorre depois e run retorna", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.failStart(t, errors.New("spawn boom"))
		h.clock.Advance(relaunchWaits[1])
		c := h.nextEnsure(t) // the attempt is in progress (blocked)

		h.cancel()
		h.waitDone(t)
		if c.ctx.Err() == nil {
			t.Errorf("the ctx passed to Ensure was not cancelled with run's ctx")
		}
		h.clock.Advance(time.Hour)
		if n := h.ens.n.Load(); n != 2 {
			t.Errorf("Ensure calls = %d, want 2 (no attempt after cancel)", n)
		}
	})

	t.Run("EXE-025/Publish falso fecha o client e sai", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{publish: func(ipcClient) bool { return false }})
		h.start(t)

		h.nextEnsure(t).ok(sampleStatus())
		h.waitClient(t)
		h.waitDone(t) // without cancelling ctx
		if !h.client.isClosed() {
			t.Errorf("client not closed after Publish returned false")
		}
		if n := h.published.Load(); n != 1 {
			t.Errorf("Publish called %d times, want 1", n)
		}
		h.clock.Advance(time.Hour)
		if n := h.ens.n.Load(); n != 1 {
			t.Errorf("Ensure calls = %d, want 1", n)
		}
	})
}

func TestSupervisorHeartbeat(t *testing.T) {
	t.Parallel()

	t.Run("DSK-003/heartbeat repassado como agent:heartbeat", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		h.client.send(t, heartbeatNote(t, 7))
		h.client.finishNotes()
		h.waitDone(t)

		close(h.emits)
		var got []supEmit
		for e := range h.emits {
			got = append(got, e)
		}
		if len(got) != 1 {
			t.Fatalf("emitted %d events, want 1: %#v", len(got), got)
		}
		if got[0].event != heartbeatEvent {
			t.Errorf("emitted event %q, want %q", got[0].event, heartbeatEvent)
		}
		if hb, ok := got[0].data.(ipc.HeartbeatParams); !ok || hb.UptimeSeconds != 7 {
			t.Errorf("emitted data = %#v, want ipc.HeartbeatParams{UptimeSeconds: 7}", got[0].data)
		}
	})

	t.Run("DSK-003/heartbeat com params inválidos não é repassado", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		h.client.send(t, ipc.Notification{Method: ipc.MethodHeartbeat}) // no params
		h.client.send(t, ipc.Notification{Method: ipc.MethodHeartbeat, Params: json.RawMessage(`"x"`)})
		h.client.send(t, ipc.Notification{Method: "agent.other", Params: json.RawMessage(`{}`)})
		h.client.send(t, heartbeatNote(t, 7))
		h.client.finishNotes()
		h.waitDone(t)

		close(h.emits)
		var got []supEmit
		for e := range h.emits {
			got = append(got, e)
		}
		if len(got) != 1 {
			t.Fatalf("emitted %d events, want 1 (only the valid heartbeat): %#v", len(got), got)
		}
		if hb, ok := got[0].data.(ipc.HeartbeatParams); !ok || hb.UptimeSeconds != 7 {
			t.Errorf("emitted data = %#v, want ipc.HeartbeatParams{UptimeSeconds: 7}", got[0].data)
		}
		if n := h.logs.countLevel(slog.LevelWarn); n != 2 {
			t.Errorf("Warn records = %d, want 2 (one per invalid heartbeat); log:\n%s", n, h.logs.dump())
		}
		if n := h.quitN.Load(); n != 0 {
			t.Errorf("Quit called %d times for heartbeats", n)
		}
	})
}

// ---------------------------------------------------------------------------
// T12 (docs/specs/phase-3-tracker/tasks.md): session.changed -> session:changed.
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - The emitted data is a SessionView value (not a pointer).
//   - Each invalid session.changed logs exactly one Warn with the message
//     msgSessionChangedInvalid (design.md) and emits nothing.

const msgSessionChangedInvalid = "session.changed com parâmetros inválidos"

func sessionChangedNote(t *testing.T, s ipc.Session) ipc.Notification {
	t.Helper()
	raw, err := json.Marshal(ipc.SessionChangedParams{Session: s})
	if err != nil {
		t.Fatalf("marshal session.changed: %v", err)
	}
	return ipc.Notification{Method: ipc.MethodSessionChanged, Params: raw}
}

// collectEmits closes the harness emit channel (run must have returned) and
// returns what was emitted.
func collectEmits(h *supHarness) []supEmit {
	close(h.emits)
	var got []supEmit
	for e := range h.emits {
		got = append(got, e)
	}
	return got
}

func TestSupervisorSessionChanged(t *testing.T) {
	t.Parallel()

	t.Run("DSK-010/session.changed emite session:changed com a SessionView", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		s := sampleSession()
		ended := s.StartedAt.Add(5 * time.Minute)
		s2 := s
		s2.State = string(domain.SessionEnded)
		s2.EndedAt = &ended
		s2.AsOf = ipc.MilliTime{Time: ended}
		h.client.send(t, sessionChangedNote(t, s))
		h.client.send(t, sessionChangedNote(t, s2))
		h.client.finishNotes()
		h.waitDone(t)

		got := collectEmits(h)
		if len(got) != 2 {
			t.Fatalf("emitted %d events, want 2: %#v", len(got), got)
		}
		for i, want := range []ipc.Session{s, s2} {
			if got[i].event != sessionChangedEvent || sessionChangedEvent != "session:changed" {
				t.Errorf("event %d = %q, want %q", i, got[i].event, "session:changed")
			}
			v, ok := got[i].data.(SessionView)
			if !ok {
				t.Errorf("event %d data = %#v, want a SessionView", i, got[i].data)
				continue
			}
			wantSessionView(t, v, want)
		}
		if n := h.logs.countLevel(slog.LevelWarn); n != 0 {
			t.Errorf("Warn records = %d, want 0; log:\n%s", n, h.logs.dump())
		}
	})

	t.Run("DSK-010/params ausentes ou inválidos logam Warn e não emitem", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		bad := []json.RawMessage{nil, json.RawMessage(`"x"`), json.RawMessage(`{"session":42}`), json.RawMessage(`{}`)}
		for _, p := range bad {
			h.client.send(t, ipc.Notification{Method: ipc.MethodSessionChanged, Params: p})
		}
		h.client.send(t, sessionChangedNote(t, sampleSession()))
		h.client.finishNotes()
		h.waitDone(t)

		got := collectEmits(h)
		if len(got) != 1 || got[0].event != sessionChangedEvent {
			t.Fatalf("emitted %#v, want only the valid session:changed", got)
		}
		if n := len(h.logs.find(0, slog.LevelWarn, msgSessionChangedInvalid)); n != len(bad) {
			t.Errorf("%q logged %d times, want %d; log:\n%s", msgSessionChangedInvalid, n, len(bad), h.logs.dump())
		}
		if n := h.quitN.Load(); n != 0 {
			t.Errorf("Quit called %d times for session.changed", n)
		}
	})

	t.Run("DSK-010/state desconhecido loga Warn e não emite", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		s := sampleSession()
		s.State = "paused"
		h.client.send(t, sessionChangedNote(t, s))
		h.client.finishNotes()
		h.waitDone(t)

		if got := collectEmits(h); len(got) != 0 {
			t.Errorf("emitted %#v, want nothing", got)
		}
		if n := len(h.logs.find(0, slog.LevelWarn, msgSessionChangedInvalid)); n != 1 {
			t.Errorf("%q logged %d times, want 1; log:\n%s", msgSessionChangedInvalid, n, h.logs.dump())
		}
	})
}
