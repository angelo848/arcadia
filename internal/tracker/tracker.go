package tracker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// writeTimeout bounds every session write (SQLite busy_timeout is 5 s; 2 s
// fits the shutdown budget).
const writeTimeout = 2 * time.Second

var (
	// ErrSessionActive is matched by *SessionActiveError [SES-002].
	ErrSessionActive = errors.New("tracker: session already active")
	// ErrShuttingDown is returned by Launch after BeginShutdown or Close [AGT-011].
	ErrShuttingDown = errors.New("tracker: shutting down")
)

// SessionActiveError is returned by Launch while a session is open [SES-002].
type SessionActiveError struct{ Session domain.SessionSnapshot }

// Error implements error.
func (e *SessionActiveError) Error() string {
	return fmt.Sprintf("%v (session %d, game %d)", ErrSessionActive, e.Session.ID, e.Session.GameID)
}

// Is reports whether target is ErrSessionActive.
func (e *SessionActiveError) Is(target error) bool { return target == ErrSessionActive }

// Config configures a Tracker.
type Config struct {
	Clock    domain.Clock
	Store    SessionStore
	Launcher Launcher
	Notify   func(domain.SessionSnapshot) // must not block; nil = no-op
	Log      *slog.Logger
	// Zero => domain defaults; tests may shorten them.
	Grace, Heartbeat, Poll time.Duration
}

type launchResult struct {
	snap domain.SessionSnapshot
	err  error
}

type launchReq struct {
	spec  LaunchSpec
	reply chan launchResult // buffered 1: the owner never blocks on it
}

// timer is a pending clock.After for one timer kind. clock.After cannot be
// cancelled; replacing the channel drops the old one and the generation makes
// any late fire of the same kind stale.
type timer struct {
	ch  <-chan time.Time
	gen uint64
}

// Tracker is the session coordinator. A single owner goroutine (run) is the
// only one touching the Machine, the Job and the store; the other methods talk
// to it through channels and read a published copy of the machine.
type Tracker struct {
	clock    domain.Clock
	store    SessionStore
	launcher Launcher
	notify   func(domain.SessionSnapshot)
	log      *slog.Logger

	launchCh chan launchReq
	stop     chan struct{} // closed by Close
	done     chan struct{} // closed when the owner goroutine exits (or by Close without Start)
	wg       sync.WaitGroup

	mu           sync.Mutex
	started      bool
	closing      bool
	shuttingDown bool
	stopCtx      context.Context // set before stop is closed
	view         Machine         // copy of the machine published by the owner

	// Owned by the run goroutine.
	m         *Machine
	job       Job
	events    <-chan JobEvent
	heartbeat timer
	grace     timer
	poll      timer
	lastAt    time.Time // latest event instant applied to m
}

// New returns a stopped Tracker; call Start to run it.
func New(cfg Config) *Tracker {
	grace, hb, poll := cfg.Grace, cfg.Heartbeat, cfg.Poll
	if grace <= 0 {
		grace = domain.SessionGracePeriod
	}
	if hb <= 0 {
		hb = domain.SessionHeartbeatInterval
	}
	if poll <= 0 {
		poll = domain.JobPollInterval
	}
	notify := cfg.Notify
	if notify == nil {
		notify = func(domain.SessionSnapshot) {}
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	m := NewMachine(grace, hb, poll)
	return &Tracker{
		clock:    cfg.Clock,
		store:    cfg.Store,
		launcher: cfg.Launcher,
		notify:   notify,
		log:      log,
		launchCh: make(chan launchReq),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		m:        m,
		view:     *m,
	}
}

// Start starts the owner goroutine. It is a no-op after the first call or
// after Close.
func (t *Tracker) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started || t.closing {
		return
	}
	t.started = true
	t.wg.Add(1)
	go t.run()
}

// Launch starts spec and opens its session [SES-001]. It returns
// *SessionActiveError, ErrShuttingDown, *LaunchError (every SES-012 failure,
// store failures with WinError 0) or ctx.Err() if the caller gave up waiting;
// in the last case the launch still completes in the owner goroutine.
func (t *Tracker) Launch(ctx context.Context, spec LaunchSpec) (domain.SessionSnapshot, error) {
	if t.isShuttingDown() {
		return domain.SessionSnapshot{}, ErrShuttingDown
	}
	// Fast refusal from the published view: a pending launch or open session
	// rejects at once, without waiting for the launch in progress [SES-002].
	// The owner goroutine checks again for the race between this and the send.
	t.mu.Lock()
	v := t.view
	t.mu.Unlock()
	if v.State() != StateIdle {
		return domain.SessionSnapshot{}, &SessionActiveError{Session: activeSession(&v, t.clock.Now())}
	}
	req := launchReq{spec: spec, reply: make(chan launchResult, 1)}
	select {
	case t.launchCh <- req:
	case <-t.done:
		return domain.SessionSnapshot{}, ErrShuttingDown
	case <-ctx.Done():
		return domain.SessionSnapshot{}, ctx.Err()
	}
	select {
	case r := <-req.reply:
		return r.snap, r.err
	case <-ctx.Done():
		return domain.SessionSnapshot{}, ctx.Err()
	}
}

// Current returns the open session measured at a single clock.Now() [IPC-026,
// IPC-031]; ok is false without an open session.
func (t *Tracker) Current() (domain.SessionSnapshot, bool) {
	t.mu.Lock()
	v := t.view
	t.mu.Unlock()
	return v.Snapshot(t.clock.Now())
}

// BeginShutdown makes every later Launch fail with ErrShuttingDown [AGT-011].
func (t *Tracker) BeginShutdown() {
	t.mu.Lock()
	t.shuttingDown = true
	t.mu.Unlock()
}

// Close stops the owner goroutine: the open session is ended (EndSession with
// ctx) and its job closed without killing processes [SES-011, TRK-004]. It
// waits for the goroutine (or a launch in progress) unless ctx ends first, and
// is idempotent [TRK-008].
func (t *Tracker) Close(ctx context.Context) error {
	t.mu.Lock()
	t.shuttingDown = true
	first := !t.closing
	t.closing = true
	started := t.started
	if first {
		t.stopCtx = ctx
		if started {
			close(t.stop)
		} else {
			close(t.done)
		}
	}
	t.mu.Unlock()
	if !started {
		return nil
	}
	select {
	case <-t.done:
		t.wg.Wait()
		return nil
	case <-ctx.Done():
		return fmt.Errorf("tracker: close: %w", ctx.Err())
	}
}

func (t *Tracker) isShuttingDown() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.shuttingDown
}

// run is the owner goroutine.
func (t *Tracker) run() {
	defer t.wg.Done()
	defer close(t.done)
	for {
		select {
		case <-t.stop:
			t.shutdown()
			return
		case req := <-t.launchCh:
			req.reply <- t.handleLaunch(req.spec)
		case ev, ok := <-t.events:
			if !ok {
				t.events = nil
				continue
			}
			t.handleJobEvent(ev)
		case fired := <-t.heartbeat.ch:
			gen := t.heartbeat.gen
			t.heartbeat = timer{}
			now := t.monotonic(fired)
			t.step(context.Background(), func() []Effect { return t.m.Heartbeat(now, gen) })
		case now := <-t.grace.ch:
			// Grace keeps its expiry instant [IPC-031].
			gen := t.grace.gen
			t.grace = timer{}
			t.applied(now)
			t.step(context.Background(), func() []Effect { return t.m.GraceTimeout(now, gen) })
		case fired := <-t.poll.ch:
			gen := t.poll.gen
			t.poll = timer{}
			t.handlePoll(t.monotonic(fired), gen)
		}
	}
}

// shutdown applies Stop with the Close context [SES-011].
func (t *Tracker) shutdown() {
	t.mu.Lock()
	ctx := t.stopCtx
	t.mu.Unlock()
	now := t.clock.Now()
	t.applied(now)
	t.step(ctx, func() []Effect { return t.m.Stop(now) })
	if t.job != nil { // unreachable: every end path closes the job
		t.closeJob()
	}
}

// step applies one machine event and runs its effects. The session id is read
// before the event because end effects carry no id and the machine is already
// Idle when it returns them.
func (t *Tracker) step(ctx context.Context, event func() []Effect) {
	_, sid, _, _ := t.m.Pending()
	effs := event()
	t.exec(ctx, effs, sid)
	t.publish()
}

// monotonic returns max(at, last applied instant) and records it, so a late
// tick never moves last_seen_at or graceSince backwards when the owner was
// busy (e.g. in a slow write).
func (t *Tracker) monotonic(at time.Time) time.Time {
	if at.Before(t.lastAt) {
		at = t.lastAt
	}
	t.lastAt = at
	return at
}

// applied records at as an applied event instant without adjusting it.
func (t *Tracker) applied(at time.Time) {
	if at.After(t.lastAt) {
		t.lastAt = at
	}
}

func (t *Tracker) publish() {
	t.mu.Lock()
	t.view = *t.m
	t.mu.Unlock()
	if t.m.State() == StateIdle {
		t.heartbeat, t.grace, t.poll = timer{}, timer{}, timer{}
	}
}

// handleLaunch runs the whole launch (Prepare -> StartSession -> Resume) in
// the owner goroutine, so a concurrent Launch only sees its result [SES-002].
func (t *Tracker) handleLaunch(spec LaunchSpec) launchResult {
	if t.isShuttingDown() {
		return launchResult{err: ErrShuttingDown}
	}
	if !t.m.Begin(spec.GameID, spec.Title) {
		return launchResult{err: &SessionActiveError{Session: activeSession(t.m, t.clock.Now())}}
	}
	t.publish()

	job, err := t.launcher.Prepare(spec)
	if err != nil {
		t.step(context.Background(), t.m.PrepareFailed)
		return t.launchFailed(spec, err)
	}
	t.job = job

	now := t.clock.Now()
	t.applied(now)
	var startAt time.Time
	for _, e := range t.m.Prepared(now) {
		if e.Kind == EffStartSession {
			startAt = e.At
		}
	}
	wctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	id, err := t.store.StartSession(wctx, spec.GameID, startAt)
	cancel()
	if err != nil {
		t.step(context.Background(), t.m.InsertFailed)
		return t.launchFailed(spec, err)
	}
	t.m.Inserted(id) // [ResumeJob]

	if err := job.Resume(); err != nil {
		t.step(context.Background(), t.m.ResumeFailed)
		return t.launchFailed(spec, err)
	}
	t.events = job.Events()

	effs := t.m.Resumed()
	var snap domain.SessionSnapshot
	for _, e := range effs {
		if e.Kind == EffNotify {
			snap = e.Session
		}
	}
	t.exec(context.Background(), effs, id)
	t.publish()
	return launchResult{snap: snap}
}

// activeSession describes the session of m that blocks a new launch; while
// Launching the published view still has sessionId 0 [SES-002].
func activeSession(m *Machine, now time.Time) domain.SessionSnapshot {
	if s, ok := m.Snapshot(now); ok {
		return s
	}
	gameID, sid, title, _ := m.Pending()
	return domain.SessionSnapshot{ID: sid, GameID: gameID, Title: title}
}

// launchFailed logs a SES-012 failure and wraps it in a *LaunchError.
func (t *Tracker) launchFailed(spec LaunchSpec, err error) launchResult {
	t.log.Error("falha ao iniciar o jogo",
		slog.Int64("game_id", spec.GameID), slog.String("path", spec.Exe), slog.Any("error", err))
	var le *LaunchError
	if errors.As(err, &le) {
		return launchResult{err: err}
	}
	return launchResult{err: &LaunchError{Err: err}}
}

func (t *Tracker) handleJobEvent(ev JobEvent) {
	now := t.clock.Now()
	t.applied(now)
	switch ev.Kind {
	case EventNewProcess:
		t.log.Debug("processo novo no job", slog.Uint64("pid", uint64(ev.PID)))
		t.step(context.Background(), func() []Effect { return t.m.ProcessAdded(now) })
	case EventActiveZero:
		t.step(context.Background(), func() []Effect { return t.m.TrackedEmpty(now) })
	case EventExitProcess:
		if !ev.Root {
			t.log.Debug("processo do job encerrou", slog.Uint64("pid", uint64(ev.PID)))
			return
		}
		_, sid, _, _ := t.m.Pending()
		t.log.Info("processo inicial do jogo encerrou", slog.Int64("session_id", sid),
			slog.Uint64("pid", uint64(ev.PID)), slog.Uint64("exit_code", uint64(ev.ExitCode)))
	default:
		t.log.Debug("evento do job ignorado", slog.Int("kind", int(ev.Kind)))
	}
}

// handlePoll runs the periodic job check [SES-013].
func (t *Tracker) handlePoll(now time.Time, gen uint64) {
	var (
		n   int
		err error
	)
	if t.job == nil {
		err = errors.New("no job")
	} else {
		n, err = t.job.ActiveProcesses()
	}
	if err != nil {
		_, sid, _, _ := t.m.Pending()
		t.log.Warn("falha ao ler os processos do job", slog.Int64("session_id", sid), slog.Any("error", err))
	}
	t.step(context.Background(), func() []Effect { return t.m.Poll(now, gen, n, err) })
}

// exec runs effects in order for session sid. Store writes are bounded by
// writeTimeout under ctx; a failed write is logged and the state moves on
// [SES-014].
func (t *Tracker) exec(ctx context.Context, effs []Effect, sid int64) {
	for _, e := range effs {
		switch e.Kind {
		case EffUpdateSession:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := t.store.UpdateSession(wctx, sid, e.State, e.At, e.Active)
			cancel()
			t.writeFailed(sid, err)
		case EffEndSession:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			_, err := t.store.EndSession(wctx, store.SessionEnd{
				ID:        sid,
				EndedAt:   e.At,
				Active:    e.Active,
				Reason:    e.Reason,
				WrittenAt: t.clock.Now(),
			})
			cancel()
			t.writeFailed(sid, err)
		case EffDeleteSession:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := t.store.DeleteSession(wctx, sid)
			cancel()
			t.writeFailed(sid, err)
		case EffTerminateJob:
			if t.job != nil {
				if err := t.job.Terminate(); err != nil {
					t.log.Warn("falha ao terminar o processo do jogo", slog.Int64("session_id", sid), slog.Any("error", err))
				}
			}
		case EffCloseJob:
			t.closeJob()
		case EffArmHeartbeat:
			t.heartbeat = timer{ch: t.clock.After(e.Delay), gen: e.Gen}
		case EffArmGrace:
			t.grace = timer{ch: t.clock.After(e.Delay), gen: e.Gen}
		case EffArmPoll:
			t.poll = timer{ch: t.clock.After(e.Delay), gen: e.Gen}
		case EffNotify:
			t.notify(e.Session)
		default:
			// EffStartSession and EffResumeJob are run inline by handleLaunch.
			t.log.Debug("efeito ignorado", slog.Int("kind", int(e.Kind)))
		}
	}
}

func (t *Tracker) closeJob() {
	if t.job == nil {
		return
	}
	if err := t.job.Close(); err != nil {
		t.log.Warn("falha ao fechar o job do jogo", slog.Any("error", err))
	}
	t.job = nil
	t.events = nil
}

func (t *Tracker) writeFailed(sid int64, err error) {
	if err != nil {
		t.log.Error("falha ao gravar a sessão", slog.Int64("session_id", sid), slog.Any("error", err))
	}
}
