package tracker_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// waitTimeout bounds every wait for asynchronous behaviour of the coordinator.
// It is a real-time safety net only; the behaviour itself is driven by the
// FakeClock and by the fakes.
const waitTimeout = 2 * time.Second

// testSpec is the default game launched by the coordinator tests.
var testSpec = tracker.LaunchSpec{
	GameID: testGameID,
	Title:  testTitle,
	Exe:    `C:\Games\Hollow Knight\hollow_knight.exe`,
}

// timeoutCtx returns a context cancelled after waitTimeout.
func timeoutCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	t.Cleanup(cancel)
	return ctx
}

// eventually spins (yielding the processor, never sleeping) until cond holds.
// It is used only for state published by the owner goroutine without a
// notification of its own (Tracker.Current).
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		runtime.Gosched()
	}
}

// --- call recorder ----------------------------------------------------------

// Operation names recorded by the fakes, in one shared, ordered log.
const (
	opPrepare   = "launcher.Prepare"
	opDetached  = "launcher.LaunchDetached"
	opStart     = "store.StartSession"
	opUpdate    = "store.UpdateSession"
	opEnd       = "store.EndSession"
	opDelete    = "store.DeleteSession"
	opPause     = "store.PauseSession"
	opResumeP   = "store.ResumePause"
	opFeedback  = "feedback"
	opResume    = "job.Resume"
	opTerminate = "job.Terminate"
	opJobClose  = "job.Close"
	opActive    = "job.ActiveProcesses"
	opNotify    = "notify"
)

type call struct {
	op      string
	spec    tracker.LaunchSpec // Prepare
	gameID  int64              // Start
	id      int64              // Start (returned), Update, Delete
	state   domain.SessionState
	at      time.Time // Start: started_at; Update: last_seen
	active  time.Duration
	end     store.SessionEnd       // End
	pause   store.PauseStart       // PauseSession
	unpause store.PauseEnd         // ResumePause
	fb      tracker.FeedbackKind   // Feedback
	snap    domain.SessionSnapshot // Notify
	job     *fakeJob               // job.*
	err     error                  // error returned by the fake
}

// recorder is a concurrency-safe ordered log of calls that wakes waiters on
// every new entry (no polling).
type recorder struct {
	mu      sync.Mutex
	calls   []call
	changed chan struct{}
}

func newRecorder() *recorder { return &recorder{changed: make(chan struct{})} }

func (r *recorder) add(c call) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
	close(r.changed)
	r.changed = make(chan struct{})
}

func (r *recorder) state() ([]call, chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls), r.changed
}

func (r *recorder) all() []call {
	calls, _ := r.state()
	return calls
}

func filterOps(calls []call, op string) []call {
	var out []call
	for _, c := range calls {
		if c.op == op {
			out = append(out, c)
		}
	}
	return out
}

func (r *recorder) of(op string) []call { return filterOps(r.all(), op) }

func (r *recorder) count(op string) int { return len(r.of(op)) }

// seq returns the recorded operations restricted to ops, in call order.
func (r *recorder) seq(ops ...string) []string {
	var out []string
	for _, c := range r.all() {
		if slices.Contains(ops, c.op) {
			out = append(out, c.op)
		}
	}
	return out
}

func opNames(calls []call) []string {
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.op)
	}
	return out
}

// waitN blocks until at least n calls of op were recorded and returns them.
func (r *recorder) waitN(t *testing.T, op string, n int) []call {
	t.Helper()
	timeout := time.After(waitTimeout)
	for {
		calls, ch := r.state()
		got := filterOps(calls, op)
		if len(got) >= n {
			return got
		}
		select {
		case <-ch:
		case <-timeout:
			t.Fatalf("timeout waiting for %d %s call(s); got %d; log: %v", n, op, len(got), opNames(calls))
			return nil
		}
	}
}

// --- log recorder -----------------------------------------------------------

type logRec struct {
	level slog.Level
	msg   string
	attrs map[string]string
}

type logStore struct {
	mu      sync.Mutex
	recs    []logRec
	changed chan struct{}
}

func newLogStore() *logStore { return &logStore{changed: make(chan struct{})} }

func (s *logStore) logger() *slog.Logger { return slog.New(&logHandler{s: s}) }

func (s *logStore) state() ([]logRec, chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.recs), s.changed
}

func (s *logStore) find(level slog.Level, msg string) []logRec {
	recs, _ := s.state()
	var out []logRec
	for _, r := range recs {
		if r.level == level && r.msg == msg {
			out = append(out, r)
		}
	}
	return out
}

func (s *logStore) atLeast(level slog.Level) []logRec {
	recs, _ := s.state()
	var out []logRec
	for _, r := range recs {
		if r.level >= level {
			out = append(out, r)
		}
	}
	return out
}

// wait blocks until a record matching level, msg (empty = any) and match (nil
// = any) is logged.
func (s *logStore) wait(t *testing.T, level slog.Level, msg string, match func(logRec) bool) logRec {
	t.Helper()
	timeout := time.After(waitTimeout)
	for {
		recs, ch := s.state()
		for _, r := range recs {
			if r.level == level && (msg == "" || r.msg == msg) && (match == nil || match(r)) {
				return r
			}
		}
		select {
		case <-ch:
		case <-timeout:
			t.Fatalf("timeout waiting for log %v %q; got %v", level, msg, recs)
			return logRec{}
		}
	}
}

type logHandler struct {
	s     *logStore
	attrs []slog.Attr
}

func (h *logHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *logHandler) Handle(_ context.Context, r slog.Record) error {
	rec := logRec{level: r.Level, msg: r.Message, attrs: map[string]string{}}
	for _, a := range h.attrs {
		rec.attrs[a.Key] = a.Value.Resolve().String()
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.Resolve().String()
		return true
	})
	h.s.mu.Lock()
	defer h.s.mu.Unlock()
	h.s.recs = append(h.s.recs, rec)
	close(h.s.changed)
	h.s.changed = make(chan struct{})
	return nil
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logHandler{s: h.s, attrs: append(slices.Clone(h.attrs), attrs...)}
}

func (h *logHandler) WithGroup(string) slog.Handler { return h }

// wantAttrs asserts that rec carries every key with the given text value.
func wantAttrs(t *testing.T, rec logRec, want map[string]string) {
	t.Helper()
	for k, v := range want {
		got, ok := rec.attrs[k]
		if !ok {
			t.Errorf("log %q: missing attr %q (attrs %v)", rec.msg, k, rec.attrs)
			continue
		}
		if got != v {
			t.Errorf("log %q: attr %s = %q, want %q", rec.msg, k, got, v)
		}
	}
}

// wantAttrContains asserts that attr key of rec contains sub.
func wantAttrContains(t *testing.T, rec logRec, key, sub string) {
	t.Helper()
	got, ok := rec.attrs[key]
	if !ok || !strings.Contains(got, sub) {
		t.Errorf("log %q: attr %s = %q (present %v), want it to contain %q", rec.msg, key, got, ok, sub)
	}
}

// --- error plans ------------------------------------------------------------

// errPlan makes a fake fail: left > 0 fails that many calls, left < 0 always.
type errPlan struct {
	err  error
	left int
}

func (p *errPlan) next() error {
	switch {
	case p.err == nil || p.left == 0:
		return nil
	case p.left > 0:
		p.left--
	}
	return p.err
}

// --- gates ------------------------------------------------------------------

// gate holds one fake call until released. The call signals entered (buffered
// 1) when it reaches the gate. release is idempotent and also runs in
// t.Cleanup, registered after the env, so it runs before Tracker.Close.
type gate struct {
	ch      chan struct{}
	entered chan struct{}
	release func()
}

func newGate(t *testing.T) *gate {
	t.Helper()
	g := &gate{ch: make(chan struct{}), entered: make(chan struct{}, 1)}
	var once sync.Once
	g.release = func() { once.Do(func() { close(g.ch) }) }
	t.Cleanup(g.release)
	return g
}

// pass is called by the fake: it signals entered and waits for release.
func (g *gate) pass() {
	select {
	case g.entered <- struct{}{}:
	default:
	}
	<-g.ch
}

// waitEntered blocks until the gated call is being held.
func (g *gate) waitEntered(t *testing.T, what string) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(waitTimeout):
		t.Fatalf("timeout waiting for %s to reach its gate", what)
	}
}

// takeGate returns *slot and clears it (one-shot gates).
func takeGate(mu *sync.Mutex, slot **gate) *gate {
	mu.Lock()
	defer mu.Unlock()
	g := *slot
	*slot = nil
	return g
}

// --- fake store -------------------------------------------------------------

// fakeStore records every SessionStore call and injects errors. With real set,
// successful calls are delegated to a real store.
type fakeStore struct {
	rec   *recorder
	real  tracker.SessionStore
	rules []store.TrackingRule // returned by TrackingRules when real is nil

	mu         sync.Mutex
	nextID     int64
	startErr   errPlan
	updateErr  errPlan
	endErr     errPlan
	deleteErr  errPlan
	pauseErr   errPlan
	unpauseErr errPlan
	updateGate *gate // holds the next UpdateSession call only
	endGate    *gate // holds the next EndSession call only
	endCtxHold *ctxHold
}

// ctxHold holds one fake call until its ctx is done (or release, the test's
// safety net in t.Cleanup). entered is signalled when the call is held and
// returned is closed right before the call returns.
type ctxHold struct {
	*gate
	returned chan struct{}
}

// holdNextEndUntilCancel holds the next EndSession call (before it is
// recorded) until its ctx is cancelled; the call then returns ctx.Err().
func (s *fakeStore) holdNextEndUntilCancel(t *testing.T) *ctxHold {
	h := &ctxHold{gate: newGate(t), returned: make(chan struct{})}
	s.mu.Lock()
	s.endCtxHold = h
	s.mu.Unlock()
	return h
}

// blockNextUpdate holds the next UpdateSession call (before it is recorded)
// until the returned gate is released.
func (s *fakeStore) blockNextUpdate(t *testing.T) *gate {
	g := newGate(t)
	s.mu.Lock()
	s.updateGate = g
	s.mu.Unlock()
	return g
}

// blockNextEnd holds the next EndSession call (before it is recorded) until
// the returned gate is released.
func (s *fakeStore) blockNextEnd(t *testing.T) *gate {
	g := newGate(t)
	s.mu.Lock()
	s.endGate = g
	s.mu.Unlock()
	return g
}

const firstSessionID int64 = 100

func newFakeStore(rec *recorder, real tracker.SessionStore) *fakeStore {
	return &fakeStore{rec: rec, real: real, nextID: firstSessionID}
}

func (s *fakeStore) failStart(err error, times int) {
	s.mu.Lock()
	s.startErr = errPlan{err, times}
	s.mu.Unlock()
}
func (s *fakeStore) failUpdate(err error, times int) {
	s.mu.Lock()
	s.updateErr = errPlan{err, times}
	s.mu.Unlock()
}
func (s *fakeStore) failEnd(err error, times int) {
	s.mu.Lock()
	s.endErr = errPlan{err, times}
	s.mu.Unlock()
}

func (s *fakeStore) failPause(err error, times int) {
	s.mu.Lock()
	s.pauseErr = errPlan{err, times}
	s.mu.Unlock()
}
func (s *fakeStore) failResumePause(err error, times int) {
	s.mu.Lock()
	s.unpauseErr = errPlan{err, times}
	s.mu.Unlock()
}

func (s *fakeStore) PauseSession(ctx context.Context, p store.PauseStart) error {
	s.mu.Lock()
	err := s.pauseErr.next()
	s.mu.Unlock()
	if err == nil && s.real != nil {
		err = s.real.PauseSession(ctx, p)
	}
	s.rec.add(call{op: opPause, id: p.SessionID, pause: p, err: err})
	return err
}

func (s *fakeStore) ResumePause(ctx context.Context, p store.PauseEnd) error {
	s.mu.Lock()
	err := s.unpauseErr.next()
	s.mu.Unlock()
	if err == nil && s.real != nil {
		err = s.real.ResumePause(ctx, p)
	}
	s.rec.add(call{op: opResumeP, id: p.SessionID, unpause: p, err: err})
	return err
}

func (s *fakeStore) StartSession(ctx context.Context, gameID int64, at time.Time) (int64, error) {
	s.mu.Lock()
	err := s.startErr.next()
	var id int64
	if err == nil && s.real == nil {
		id = s.nextID
		s.nextID++
	}
	s.mu.Unlock()
	if err == nil && s.real != nil {
		id, err = s.real.StartSession(ctx, gameID, at)
	}
	s.rec.add(call{op: opStart, gameID: gameID, at: at, id: id, err: err})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *fakeStore) UpdateSession(ctx context.Context, id int64, st domain.SessionState, lastSeen time.Time, active time.Duration) error {
	if g := takeGate(&s.mu, &s.updateGate); g != nil {
		g.pass()
	}
	s.mu.Lock()
	err := s.updateErr.next()
	s.mu.Unlock()
	if err == nil && s.real != nil {
		err = s.real.UpdateSession(ctx, id, st, lastSeen, active)
	}
	s.rec.add(call{op: opUpdate, id: id, state: st, at: lastSeen, active: active, err: err})
	return err
}

func (s *fakeStore) EndSession(ctx context.Context, e store.SessionEnd) (domain.SessionState, error) {
	if g := takeGate(&s.mu, &s.endGate); g != nil {
		g.pass()
	}
	s.mu.Lock()
	h := s.endCtxHold
	s.endCtxHold = nil
	s.mu.Unlock()
	if h != nil {
		defer close(h.returned)
		select {
		case h.entered <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
		case <-h.ch:
		}
		err := ctx.Err()
		if err == nil {
			err = errors.New("fake EndSession released by the test without ctx cancellation")
		}
		s.rec.add(call{op: opEnd, id: e.ID, end: e, err: err})
		return "", err
	}
	s.mu.Lock()
	err := s.endErr.next()
	s.mu.Unlock()
	st := domain.Outcome(e.Active)
	if err == nil && s.real != nil {
		st, err = s.real.EndSession(ctx, e)
	}
	s.rec.add(call{op: opEnd, id: e.ID, end: e, state: st, err: err})
	if err != nil {
		return "", err
	}
	return st, nil
}

func (s *fakeStore) DeleteSession(ctx context.Context, id int64) error {
	s.mu.Lock()
	err := s.deleteErr.next()
	s.mu.Unlock()
	if err == nil && s.real != nil {
		err = s.real.DeleteSession(ctx, id)
	}
	s.rec.add(call{op: opDelete, id: id, err: err})
	return err
}

// --- fake job and launcher --------------------------------------------------

// fakeJob is a Job whose events and ActiveProcesses are driven by the test.
// It starts no goroutines.
type fakeJob struct {
	rec    *recorder
	pid    uint32
	events chan tracker.JobEvent

	mu        sync.Mutex
	closed    bool
	active    int
	activeErr error
	resumeErr error
	closeGate *gate // holds the next Close call only
}

// blockNextClose holds the next Close call (before it closes anything or is
// recorded) until the returned gate is released.
func (j *fakeJob) blockNextClose(t *testing.T) *gate {
	g := newGate(t)
	j.mu.Lock()
	j.closeGate = g
	j.mu.Unlock()
	return g
}

func (j *fakeJob) RootPID() uint32 { return j.pid }

func (j *fakeJob) Resume() error {
	j.mu.Lock()
	err := j.resumeErr
	j.mu.Unlock()
	j.rec.add(call{op: opResume, job: j, err: err})
	return err
}

func (j *fakeJob) Terminate() error {
	j.rec.add(call{op: opTerminate, job: j})
	return nil
}

func (j *fakeJob) Events() <-chan tracker.JobEvent { return j.events }

func (j *fakeJob) ActiveProcesses() (int, error) {
	j.mu.Lock()
	n, err := j.active, j.activeErr
	j.mu.Unlock()
	j.rec.add(call{op: opActive, job: j, err: err})
	return n, err
}

// Close is idempotent and closes the events channel once, like the real job.
func (j *fakeJob) Close() error {
	if g := takeGate(&j.mu, &j.closeGate); g != nil {
		g.pass()
	}
	j.mu.Lock()
	if !j.closed {
		j.closed = true
		close(j.events)
	}
	j.mu.Unlock()
	j.rec.add(call{op: opJobClose, job: j})
	return nil
}

func (j *fakeJob) isClosed() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.closed
}

func (j *fakeJob) setActive(n int, err error) {
	j.mu.Lock()
	j.active, j.activeErr = n, err
	j.mu.Unlock()
}

// send delivers ev to the coordinator; it fails the test if the job is closed.
func (j *fakeJob) send(t *testing.T, ev tracker.JobEvent) {
	t.Helper()
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		t.Fatalf("send %+v on a closed job", ev)
	}
	select {
	case j.events <- ev:
	default:
		t.Fatalf("job events buffer full")
	}
}

// empty makes the job report no live process and sends ACTIVE_PROCESS_ZERO.
func (j *fakeJob) empty(t *testing.T) {
	t.Helper()
	j.setActive(0, nil)
	j.send(t, tracker.JobEvent{Kind: tracker.EventActiveZero})
}

// newProcess makes the job report one live process and sends NEW_PROCESS.
func (j *fakeJob) newProcess(t *testing.T, pid uint32) {
	t.Helper()
	j.setActive(1, nil)
	j.send(t, tracker.JobEvent{Kind: tracker.EventNewProcess, PID: pid})
}

// fakeLauncher creates fakeJobs; Prepare can fail once or block on a gate.
type fakeLauncher struct {
	rec *recorder

	mu         sync.Mutex
	jobs       []*fakeJob
	prepareErr errPlan
	detachErr  errPlan
	openURIErr error    // returned by OpenURI (RUL-003)
	uris       []string // URIs given to OpenURI, in order
	resumeErr  error    // given to the next job
	gate       chan struct{}
	entered    chan struct{}
}

const (
	firstPID    uint32 = 4242
	detachedPID uint32 = 7777 // PID returned by fakeLauncher.LaunchDetached
)

func (l *fakeLauncher) Prepare(spec tracker.LaunchSpec) (tracker.Job, error) {
	l.mu.Lock()
	err := l.prepareErr.next()
	gate, entered := l.gate, l.entered
	l.mu.Unlock()
	l.rec.add(call{op: opPrepare, spec: spec, err: err})
	if gate != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-gate
	}
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	j := &fakeJob{
		rec:       l.rec,
		pid:       firstPID + uint32(len(l.jobs)),
		events:    make(chan tracker.JobEvent, 64),
		active:    1,
		resumeErr: l.resumeErr,
	}
	l.resumeErr = nil
	l.jobs = append(l.jobs, j)
	return j, nil
}

// LaunchDetached records the call and returns detachedPID (no job, no goroutine).
func (l *fakeLauncher) LaunchDetached(spec tracker.LaunchSpec) (uint32, error) {
	l.mu.Lock()
	err := l.detachErr.next()
	l.mu.Unlock()
	l.rec.add(call{op: opDetached, spec: spec, err: err})
	if err != nil {
		return 0, err
	}
	return detachedPID, nil
}

func (l *fakeLauncher) failDetached(err error, times int) {
	l.mu.Lock()
	l.detachErr = errPlan{err, times}
	l.mu.Unlock()
}

func (l *fakeLauncher) failPrepare(err error, times int) {
	l.mu.Lock()
	l.prepareErr = errPlan{err, times}
	l.mu.Unlock()
}

func (l *fakeLauncher) failNextResume(err error) {
	l.mu.Lock()
	l.resumeErr = err
	l.mu.Unlock()
}

// block makes Prepare wait until release is called; entered receives a value
// when a Prepare call reaches the gate. release runs in t.Cleanup too, before
// the tracker is closed.
func (l *fakeLauncher) block(t *testing.T) (entered <-chan struct{}, release func()) {
	gate := make(chan struct{})
	ent := make(chan struct{}, 1)
	l.mu.Lock()
	l.gate, l.entered = gate, ent
	l.mu.Unlock()
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	return ent, release
}

func (l *fakeLauncher) job(t *testing.T, i int) *fakeJob {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if i >= len(l.jobs) {
		t.Fatalf("launcher created %d job(s), want index %d", len(l.jobs), i)
	}
	return l.jobs[i]
}

// --- coordinator environment ------------------------------------------------

type envConfig struct {
	grace, heartbeat, poll time.Duration // zero => domain defaults
	real                   tracker.SessionStore
	procs                  tracker.ProcessLister // nil: no adoption (RUL-002)
	watch                  time.Duration         // watcher period; zero => domain default
	rules                  []store.TrackingRule  // returned when real is nil
	noStart                bool
	// feedback runs inside the Config.Feedback callback, after the call is
	// recorded (it may block).
	feedback func(tracker.FeedbackKind)
}

// quiet keeps the heartbeat and the periodic check out of the way of tests
// about other events. Waiter counts do not change (contract table).
var quiet = envConfig{heartbeat: time.Hour, poll: time.Hour}

type env struct {
	clock    *domain.FakeClock
	rec      *recorder
	store    *fakeStore
	launcher *fakeLauncher
	logs     *logStore
	tr       *tracker.Tracker

	seenNotify int
	barriers   uint32
}

func newEnv(t *testing.T, cfg envConfig) *env {
	t.Helper()
	rec := newRecorder()
	e := &env{
		clock:    domain.NewFakeClock(baseT),
		rec:      rec,
		store:    withRules(newFakeStore(rec, cfg.real), cfg.rules),
		launcher: &fakeLauncher{rec: rec},
		logs:     newLogStore(),
	}
	e.tr = tracker.New(tracker.Config{
		Clock:    e.clock,
		Store:    e.store,
		Launcher: e.launcher,
		Notify:   func(s domain.SessionSnapshot) { rec.add(call{op: opNotify, snap: s}) },
		Feedback: func(k tracker.FeedbackKind) {
			rec.add(call{op: opFeedback, fb: k})
			if cfg.feedback != nil {
				cfg.feedback(k)
			}
		},
		Processes:     cfg.procs,
		WatchInterval: cfg.watch,
		Log:           e.logs.logger(),
		Grace:         cfg.grace,
		Heartbeat:     cfg.heartbeat,
		Poll:          cfg.poll,
	})
	if !cfg.noStart {
		e.tr.Start()
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
		defer cancel()
		if err := e.tr.Close(ctx); err != nil {
			t.Errorf("cleanup Tracker.Close: %v", err)
		}
	})
	return e
}

func (e *env) close(t *testing.T) error {
	t.Helper()
	return e.tr.Close(timeoutCtx(t))
}

// tryLaunch calls Launch with a bounded context.
func (e *env) tryLaunch(t *testing.T, spec tracker.LaunchSpec) (domain.SessionSnapshot, error) {
	t.Helper()
	return e.tr.Launch(timeoutCtx(t), spec)
}

// launchSpec launches spec, requires success and consumes the running
// notification. It returns the snapshot and the job created for it.
func (e *env) launchSpec(t *testing.T, spec tracker.LaunchSpec) (domain.SessionSnapshot, *fakeJob) {
	t.Helper()
	jobsBefore := len(e.launcher.jobsSnapshot())
	snap, err := e.tryLaunch(t, spec)
	if err != nil {
		t.Fatalf("Launch(%+v): %v", spec, err)
	}
	n := e.nextNotify(t)
	if n.State != domain.SessionRunning {
		t.Fatalf("first notification state = %q, want running", n.State)
	}
	return snap, e.launcher.job(t, jobsBefore)
}

func (e *env) launch(t *testing.T) (domain.SessionSnapshot, *fakeJob) {
	t.Helper()
	return e.launchSpec(t, testSpec)
}

func (l *fakeLauncher) jobsSnapshot() []*fakeJob {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.jobs)
}

// waiters blocks until at least n FakeClock waiters are pending.
func (e *env) waiters(t *testing.T, n int) {
	t.Helper()
	if err := e.clock.BlockUntilWaiters(timeoutCtx(t), n); err != nil {
		t.Fatalf("BlockUntilWaiters(%d): %v", n, err)
	}
}

// advance waits for want waiters and then moves the clock by d.
func (e *env) advance(t *testing.T, want int, d time.Duration) {
	t.Helper()
	e.waiters(t, want)
	e.clock.Advance(d)
}

// nextNotify returns the next notification not yet consumed by the test.
func (e *env) nextNotify(t *testing.T) domain.SessionSnapshot {
	t.Helper()
	calls := e.rec.waitN(t, opNotify, e.seenNotify+1)
	s := calls[e.seenNotify].snap
	e.seenNotify++
	return s
}

func (e *env) notifications() []domain.SessionSnapshot {
	var out []domain.SessionSnapshot
	for _, c := range e.rec.of(opNotify) {
		out = append(out, c.snap)
	}
	return out
}

// barrier makes the owner goroutine process one more job event (an exit of the
// initial process, logged by SES-015) and waits for it, so that everything the
// goroutine received before has been handled.
func (e *env) barrier(t *testing.T, j *fakeJob) {
	t.Helper()
	e.barriers++
	code := 1000 + e.barriers
	j.send(t, tracker.JobEvent{Kind: tracker.EventExitProcess, PID: j.pid, Root: true, ExitCode: code})
	want := fmt.Sprint(code)
	e.logs.wait(t, slog.LevelInfo, msgRootExited, func(r logRec) bool { return r.attrs["exit_code"] == want })
}

// waitCurrent waits until Current satisfies cond and returns it.
func (e *env) waitCurrent(t *testing.T, what string, cond func(domain.SessionSnapshot, bool) bool) (domain.SessionSnapshot, bool) {
	t.Helper()
	var (
		snap domain.SessionSnapshot
		ok   bool
	)
	eventually(t, what, func() bool {
		snap, ok = e.tr.Current()
		return cond(snap, ok)
	})
	return snap, ok
}

func (e *env) waitCurrentState(t *testing.T, st domain.SessionState) domain.SessionSnapshot {
	t.Helper()
	snap, _ := e.waitCurrent(t, "Current state "+string(st), func(s domain.SessionSnapshot, ok bool) bool {
		return ok && s.State == st
	})
	return snap
}

func (e *env) waitNoCurrent(t *testing.T) {
	t.Helper()
	e.waitCurrent(t, "Current = false", func(_ domain.SessionSnapshot, ok bool) bool { return !ok })
}

// Log messages fixed by tasks.md (T08).
const (
	msgLaunchFailed = "falha ao iniciar o jogo"
	msgWriteFailed  = "falha ao gravar a sessão"
	msgRootExited   = "processo inicial do jogo encerrou"
	msgRecovered    = "sessões abertas recuperadas"
)

// OpenURI records the URI and returns openURIErr (RUL-003). It never creates a job.
func (l *fakeLauncher) OpenURI(uri string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.uris = append(l.uris, uri)
	return l.openURIErr
}

// urisOpened returns the URIs given to OpenURI, in order (RUL-003).
func (l *fakeLauncher) urisOpened() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.uris)
}

// jobCount returns how many jobs the launcher created.
func (l *fakeLauncher) jobCount() int { return len(l.jobsSnapshot()) }

// TrackingRules delegates to the real store (RUL-002).
func (s *fakeStore) TrackingRules(ctx context.Context, gameID int64) ([]store.TrackingRule, error) {
	if s.real == nil {
		return s.rules, nil
	}
	return s.real.TrackingRules(ctx, gameID)
}

// withRules sets the rules a fake store returns when it has no real store (RUL-002).
func withRules(s *fakeStore, rules []store.TrackingRule) *fakeStore {
	s.rules = rules
	return s
}

// fakeLister returns the snapshot a test sets (RUL-002).
type fakeLister struct {
	mu    sync.Mutex
	procs []tracker.ProcInfo
	err   error
}

func (l *fakeLister) set(procs ...tracker.ProcInfo) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.procs = procs
}

func (l *fakeLister) List() ([]tracker.ProcInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return nil, l.err
	}
	return slices.Clone(l.procs), nil
}
