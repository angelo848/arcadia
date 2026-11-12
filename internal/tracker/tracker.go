package tracker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
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
	// ErrInvalidPauseReason is returned by TogglePause for reasons other than
	// hotkey and gamepad [SES-016].
	ErrInvalidPauseReason = errors.New("tracker: invalid pause reason")
)

// Reasons carried by *NotPausableError [SES-037].
const (
	ReasonNoSession = "no-session"
	ReasonGrace     = "grace"
)

// NotPausableError is returned by Pause and Resume when there is nothing to
// pause: no session or the session is in Grace [SES-037].
type NotPausableError struct{ Reason string }

// Error implements error.
func (e *NotPausableError) Error() string { return "tracker: session not pausable: " + e.Reason }

// PauseResult is the outcome of TogglePause.
type PauseResult int

// TogglePause outcomes.
const (
	PauseIgnored PauseResult = iota
	PausePaused
	PauseResumed
)

type ctrlKind int

const (
	ctrlToggle ctrlKind = iota
	ctrlPause
	ctrlResume
	ctrlSuspend
	ctrlWake
	ctrlLaunchDetached
	ctrlWithoutGame
)

// Claim states of a ctrlWithoutGame request: the owner and the caller race for
// it once, so fn starts only if the caller has not given up [DEL-002].
const (
	claimOpen    uint32 = iota // neither side has taken the request
	claimOwner                 // the owner goroutine took it: fn runs or is refused
	claimAbandon               // the caller gave up before the owner took it
)

type ctrlResult struct {
	res  PauseResult
	snap domain.SessionSnapshot
	pid  uint32
	err  error
}

type ctrlReq struct {
	kind        ctrlKind
	spec        LaunchSpec // ctrlLaunchDetached only
	reason      domain.PauseReason
	at          time.Time
	userPresent bool
	gameID      int64        // ctrlWithoutGame only
	fn          func() error // ctrlWithoutGame only
	ctx         context.Context
	claim       *atomic.Uint32  // ctrlWithoutGame only: claimOpen, claimOwner or claimAbandon
	reply       chan ctrlResult // buffered 1: the owner never blocks on it
}

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
	Feedback func(FeedbackKind)           // must not block; nil = no-op
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
	feedback func(FeedbackKind)
	log      *slog.Logger

	launchCh chan launchReq
	ctrlCh   chan ctrlReq
	stop     chan struct{} // closed by Close
	done     chan struct{} // closed when the owner goroutine exits (or by Close without Start)
	wg       sync.WaitGroup
	// work is the parent of every write made by the owner goroutine outside
	// shutdown (launch, timers, job events); Close cancels it when its ctx
	// expires so the owner can exit within the budget.
	work       context.Context
	cancelWork context.CancelFunc

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
	feedback := cfg.Feedback
	if feedback == nil {
		feedback = func(FeedbackKind) {}
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	m := NewMachine(grace, hb, poll)
	work, cancelWork := context.WithCancel(context.Background())
	return &Tracker{
		work:       work,
		cancelWork: cancelWork,
		clock:      cfg.Clock,
		store:      cfg.Store,
		launcher:   cfg.Launcher,
		notify:     notify,
		feedback:   feedback,
		log:        log,
		launchCh:   make(chan launchReq),
		ctrlCh:     make(chan ctrlReq, 16),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		m:          m,
		view:       *m,
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

// LaunchUntracked starts spec without a Job Object and without a session
// [SES-031..034]. It is refused with *SessionActiveError while a session is
// open or a launch is in progress, and with ErrShuttingDown after
// BeginShutdown or Close. pid is 0 on failure.
func (t *Tracker) LaunchUntracked(ctx context.Context, spec LaunchSpec) (uint32, error) {
	if t.isShuttingDown() {
		return 0, ErrShuttingDown
	}
	t.mu.Lock()
	v := t.view
	t.mu.Unlock()
	if v.State() != StateIdle {
		return 0, &SessionActiveError{Session: activeSession(&v, t.clock.Now())}
	}
	r, err := t.control(ctx, ctrlReq{kind: ctrlLaunchDetached, spec: spec})
	if err != nil {
		return 0, err
	}
	return r.pid, nil
}

// TogglePause flips Running <-> Paused for a hotkey or gamepad trigger at the
// event instant at [SES-016, SES-017]. Triggers without a pausable session are
// PauseIgnored [SES-018]; any other reason is ErrInvalidPauseReason.
func (t *Tracker) TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (PauseResult, error) {
	if reason != domain.PauseHotkey && reason != domain.PauseGamepad {
		return PauseIgnored, ErrInvalidPauseReason
	}
	r, err := t.control(ctx, ctrlReq{kind: ctrlToggle, reason: reason, at: at})
	return r.res, err
}

// Pause pauses the session with reason ui; idempotent in Paused [SES-035].
func (t *Tracker) Pause(ctx context.Context, at time.Time) (domain.SessionSnapshot, error) {
	r, err := t.control(ctx, ctrlReq{kind: ctrlPause, at: at})
	return r.snap, err
}

// Resume resumes a paused session whatever the reason; idempotent in Running
// [SES-036].
func (t *Tracker) Resume(ctx context.Context, at time.Time) (domain.SessionSnapshot, error) {
	r, err := t.control(ctx, ctrlReq{kind: ctrlResume, at: at})
	return r.snap, err
}

// Suspend pauses a running session because the machine is going to sleep
// [SES-026].
func (t *Tracker) Suspend(ctx context.Context, at time.Time) error {
	_, err := t.control(ctx, ctrlReq{kind: ctrlSuspend, at: at})
	return err
}

// Wake resumes a sleep pause when the user is present [SES-028].
func (t *Tracker) Wake(ctx context.Context, at time.Time, userPresent bool) error {
	_, err := t.control(ctx, ctrlReq{kind: ctrlWake, at: at, userPresent: userPresent})
	return err
}

// WithoutActiveGame runs fn in the owner goroutine when gameID has neither an
// open session nor a launch in progress [DEL-002]. Otherwise it returns
// *SessionActiveError without calling fn: ID and Title of the open session, or
// ID 0 and the title while launching. Sessions and launches of other games do
// not block it [DEL-003]. fn's error is returned as is.
//
// Context policy: if ctx is already done when the owner takes the request, fn
// does not run and ctx.Err() is returned. Once fn has started the caller waits
// for its result even if ctx ends. After BeginShutdown or Close it returns
// ErrShuttingDown without calling fn.
//
// fn runs in the owner goroutine, which is shared with the tracking loop, so it
// must be short. It does not receive the tracker's shutdown context: it must
// honor the ctx that its caller passed to WithoutActiveGame (DeleteGame uses it
// for the database).
func (t *Tracker) WithoutActiveGame(ctx context.Context, gameID int64, fn func() error) error {
	if t.isShuttingDown() {
		return ErrShuttingDown
	}
	// Fast refusal from the published view, as Launch does: a pending launch of
	// the same game is answered without waiting for it [DEL-002].
	t.mu.Lock()
	v := t.view
	t.mu.Unlock()
	if g, _, _, ok := v.Pending(); ok && g == gameID {
		return &SessionActiveError{Session: activeSession(&v, t.clock.Now())}
	}
	req := ctrlReq{
		kind:   ctrlWithoutGame,
		gameID: gameID,
		fn:     fn,
		ctx:    ctx,
		claim:  new(atomic.Uint32),
		reply:  make(chan ctrlResult, 1),
	}
	select {
	case t.ctrlCh <- req:
	case <-t.done:
		return ErrShuttingDown
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case r := <-req.reply:
		return r.err
	case <-t.done:
		select {
		case r := <-req.reply:
			return r.err
		default:
			return ErrShuttingDown
		}
	case <-ctx.Done():
		if req.claim.CompareAndSwap(claimOpen, claimAbandon) {
			return ctx.Err()
		}
		// The owner took the request, so fn has started or is refused. The owner
		// always replies before it exits, so the wait does not need ctx.
		return (<-req.reply).err
	}
}

// control sends req to the owner goroutine. The send is abandoned only when
// the owner is gone (never on ctx expiry, so a late-ctx event is still applied
// with its original instant [PWR-005]); the reply is awaited until ctx ends.
func (t *Tracker) control(ctx context.Context, req ctrlReq) (ctrlResult, error) {
	if t.isShuttingDown() {
		return ctrlResult{}, ErrShuttingDown
	}
	req.reply = make(chan ctrlResult, 1)
	select {
	case t.ctrlCh <- req:
	case <-t.done:
		return ctrlResult{}, ErrShuttingDown
	}
	select {
	case r := <-req.reply:
		return r, r.err
	case <-t.done:
		select {
		case r := <-req.reply:
			return r, r.err
		default:
			return ctrlResult{}, ErrShuttingDown
		}
	case <-ctx.Done():
		return ctrlResult{}, ctx.Err()
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
// always waits for the owner goroutine before returning; if ctx ends first it
// cancels every pending write (including those of a launch in progress), still
// waits for the owner and returns the wrapped ctx error. The session whose end
// could not be written stays open and is recovered on the next start
// [SES-010]. Close is idempotent [TRK-008].
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
		t.cancelWork()
		return nil
	}
	var err error
	select {
	case <-t.done:
	case <-ctx.Done():
		err = fmt.Errorf("tracker: close: %w", ctx.Err())
	}
	// Cancelling is harmless once the owner has exited; when ctx expired it
	// unblocks the in-flight write so the owner can finish.
	t.cancelWork()
	t.wg.Wait()
	return err
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
		case req := <-t.ctrlCh:
			req.reply <- t.handleControl(req)
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
			t.step(t.work, func() []Effect { return t.m.Heartbeat(now, gen) })
		case now := <-t.grace.ch:
			// Grace keeps its expiry instant [IPC-031].
			gen := t.grace.gen
			t.grace = timer{}
			t.applied(now)
			t.step(t.work, func() []Effect { return t.m.GraceTimeout(now, gen) })
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
	stopCtx := t.stopCtx
	t.mu.Unlock()
	// The end write honours the first Close ctx and is also cancelled when
	// any Close gives up (cancelWork).
	ctx, cancel := context.WithCancel(stopCtx)
	defer cancel()
	stopAfter := context.AfterFunc(t.work, cancel)
	defer stopAfter()
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
		t.step(t.work, t.m.PrepareFailed)
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
	// Launch writes derive from t.work so Close can cancel them when its
	// budget runs out.
	wctx, cancel := context.WithTimeout(t.work, writeTimeout)
	id, err := t.store.StartSession(wctx, spec.GameID, startAt)
	cancel()
	if err != nil {
		t.step(t.work, t.m.InsertFailed)
		return t.launchFailed(spec, err)
	}
	t.m.Inserted(id) // [ResumeJob]

	if err := job.Resume(); err != nil {
		t.step(t.work, t.m.ResumeFailed)
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
	t.exec(t.work, effs, id)
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
		t.step(t.work, func() []Effect { return t.m.ProcessAdded(now) })
	case EventActiveZero:
		t.step(t.work, func() []Effect { return t.m.TrackedEmpty(now) })
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

// handleControl applies a pause/suspend request at the event instant [SES-016
// .. SES-037, PWR-005].
func (t *Tracker) handleControl(req ctrlReq) ctrlResult {
	if t.isShuttingDown() {
		return ctrlResult{err: ErrShuttingDown}
	}
	if req.kind == ctrlWithoutGame {
		return t.withoutGame(req)
	}
	before := t.m.State()
	if req.kind == ctrlLaunchDetached {
		if before != StateIdle {
			return ctrlResult{err: &SessionActiveError{Session: activeSession(t.m, t.clock.Now())}}
		}
		pid, err := t.launcher.LaunchDetached(req.spec)
		if err != nil {
			return ctrlResult{err: t.launchFailed(req.spec, err).err}
		}
		t.log.Info("jogo iniciado sem rastrear", slog.Int64("game_id", req.spec.GameID), slog.Uint64("pid", uint64(pid)))
		return ctrlResult{pid: pid}
	}
	if req.kind == ctrlPause || req.kind == ctrlResume {
		if reason, refused := t.m.NotPausable(); refused {
			t.log.Debug("pausa ignorada", slog.Int("state", int(before)), slog.String("reason", reason))
			return ctrlResult{err: &NotPausableError{Reason: reason}}
		}
	}
	// The raw instant is compared with startedAt before the monotonic clamp,
	// otherwise a trigger from before the start would be applied [SES-018].
	if (before == StateRunning || before == StatePaused) && req.at.Before(t.m.startedAt) {
		t.log.Debug("pausa ignorada", slog.Int("state", int(before)), slog.Time("at", req.at))
		now := req.at
		if now.Before(t.lastAt) {
			now = t.lastAt
		}
		snap, _ := t.m.Snapshot(now)
		return ctrlResult{snap: snap}
	}
	now := t.monotonic(req.at)
	var effs []Effect
	switch req.kind {
	case ctrlToggle:
		effs = t.m.TogglePause(now, req.reason)
	case ctrlPause:
		effs = t.m.Pause(now)
	case ctrlResume:
		effs = t.m.Resume(now)
	case ctrlSuspend:
		effs = t.m.Suspend(now)
	case ctrlWake:
		effs = t.m.Wake(now, req.userPresent)
	}
	res := PauseIgnored
	if len(effs) == 0 {
		t.log.Debug("pausa ignorada", slog.Int("state", int(before)))
	} else {
		switch t.m.State() {
		case StatePaused:
			res = PausePaused
		case StateRunning:
			res = PauseResumed
		}
		_, sid, _, _ := t.m.Pending()
		t.exec(t.work, effs, sid)
		t.publish()
	}
	snap, _ := t.m.Snapshot(now)
	return ctrlResult{res: res, snap: snap}
}

// withoutGame runs a WithoutActiveGame request in the owner goroutine [DEL-002,
// DEL-003]. The claim is taken first: a caller that gave up is skipped, and one
// whose context ended before this point gets ctx.Err() without fn running.
func (t *Tracker) withoutGame(req ctrlReq) ctrlResult {
	if !req.claim.CompareAndSwap(claimOpen, claimOwner) {
		return ctrlResult{}
	}
	if err := req.ctx.Err(); err != nil {
		return ctrlResult{err: err}
	}
	if gameID, _, _, ok := t.m.Pending(); ok && gameID == req.gameID {
		return ctrlResult{err: &SessionActiveError{Session: activeSession(t.m, t.clock.Now())}}
	}
	return ctrlResult{err: req.fn()}
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
	t.step(t.work, func() []Effect { return t.m.Poll(now, gen, n, err) })
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
		case EffPauseSession:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := t.store.PauseSession(wctx, store.PauseStart{
				SessionID: sid, At: e.At, Reason: e.Pause, Active: e.Active,
			})
			cancel()
			t.writeFailed(sid, err)
		case EffResumePause:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := t.store.ResumePause(wctx, store.PauseEnd{
				SessionID: sid, At: e.At, State: e.State, Active: e.Active,
			})
			cancel()
			t.writeFailed(sid, err)
		case EffNotify:
			t.notify(e.Session)
		case EffFeedback:
			t.feedback(e.Feedback)
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
