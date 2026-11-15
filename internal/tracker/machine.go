package tracker

import (
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// State is the in-memory state of the session machine.
type State int

// Machine states (contracts/session-state-machine.md). Ended and Failed are
// transient and collapse into StateIdle.
const (
	StateIdle State = iota
	StateLaunching
	StateRunning
	StateGrace
	StatePaused
	// StateClosing waits for the game to close after the close hotkey
	// (contracts/session-close-state-machine.md, phase 13). The active time is
	// frozen at closeAt; nothing is persisted while waiting.
	StateClosing
)

// EffectKind identifies a side effect the coordinator must run.
type EffectKind int

// Effect kinds.
const (
	EffStartSession EffectKind = iota + 1
	EffUpdateSession
	EffEndSession
	EffDeleteSession
	EffResumeJob
	EffTerminateJob
	EffCloseJob
	EffArmHeartbeat
	EffArmGrace
	EffArmPoll
	EffNotify
	EffPauseSession
	EffResumePause
	EffFeedback
	EffArmAdopt // arm the URI adoption timeout (RUL-003)
)

// FeedbackKind identifies the cue for a pause toggle (FBK-001).
type FeedbackKind int

// Feedback kinds.
const (
	FeedbackPaused FeedbackKind = iota + 1
	FeedbackResumed
)

// Effect is a side effect requested by a transition. Effects are returned in
// the order they must run.
type Effect struct {
	Kind     EffectKind
	State    domain.SessionState     // Update: running|grace; End/Notify: resulting state
	At       time.Time               // Start: started_at; Update: last_seen; End: ended_at
	Active   time.Duration           // Update/End: active time
	Reason   domain.SessionEndReason // End
	Gen      uint64                  // Arm*: generation to pass back
	Delay    time.Duration           // Arm*
	Session  domain.SessionSnapshot  // Notify (AsOf = event instant)
	Pause    domain.PauseReason      // PauseSession: reason of the opened interval
	Feedback FeedbackKind            // Feedback
}

// Machine is the pure session state machine: it performs no I/O, reads no
// clock and starts no goroutines. Each method applies one event and returns
// the effects to run. It is not safe for concurrent use; the coordinator's
// owner goroutine is its only caller.
type Machine struct {
	grace, heartbeat, poll time.Duration

	state     State
	gameID    int64
	sessionID int64
	title     string

	startedAt    time.Time
	runningSince time.Time
	graceSince   time.Time
	pausedAt     time.Time
	pauseReason  domain.PauseReason
	accumulated  time.Duration

	// Close by hotkey (CON-005, CON-006): the trigger instant and the state the
	// session was in. Only meaningful in StateClosing.
	closeAt   time.Time
	closeFrom State

	// Timer generations. lastGen is a single monotonic counter so that no
	// generation is ever reused; 0 means "no timer armed".
	lastGen  uint64
	hbGen    uint64
	graceGen uint64
	adoptGen uint64 // generation of the URI adoption timeout (RUL-003)
	adopted  bool   // the game process was adopted by the watcher (RUL-005)
	pollGen  uint64
}

// NewMachine returns a machine in StateIdle with the given timer delays.
func NewMachine(grace, heartbeat, poll time.Duration) *Machine {
	return &Machine{grace: grace, heartbeat: heartbeat, poll: poll}
}

// State returns the current state.
func (m *Machine) State() State { return m.state }

// Begin moves Idle -> Launching for gameID. It returns false (and changes
// nothing) outside Idle.
func (m *Machine) Begin(gameID int64, title string) bool {
	if m.state != StateIdle {
		return false
	}
	m.reset()
	m.state = StateLaunching
	m.gameID = gameID
	m.title = title
	return true
}

// Prepared records the launch instant and requests the session insert [SES-001].
func (m *Machine) Prepared(now time.Time) []Effect {
	if m.state != StateLaunching {
		return nil
	}
	m.startedAt = now
	return []Effect{{Kind: EffStartSession, At: now}}
}

// Inserted records the session id and requests the job resume.
func (m *Machine) Inserted(id int64) []Effect {
	if m.state != StateLaunching {
		return nil
	}
	m.sessionID = id
	return []Effect{{Kind: EffResumeJob}}
}

// Resumed moves Launching -> Running from the Prepared instant [SES-001].
func (m *Machine) Resumed() []Effect {
	if m.state != StateLaunching {
		return nil
	}
	m.state = StateRunning
	m.runningSince = m.startedAt
	m.accumulated = 0
	return []Effect{
		m.armHeartbeat(),
		m.armPoll(),
		m.notify(domain.SessionRunning, m.startedAt),
	}
}

// PrepareFailed moves Launching -> Idle; Prepare already cleaned up [SES-012].
func (m *Machine) PrepareFailed() []Effect {
	if m.state != StateLaunching {
		return nil
	}
	m.reset()
	return []Effect{}
}

// InsertFailed moves Launching -> Idle, killing the never-resumed process [SES-012].
func (m *Machine) InsertFailed() []Effect {
	if m.state != StateLaunching {
		return nil
	}
	m.reset()
	return []Effect{{Kind: EffTerminateJob}, {Kind: EffCloseJob}}
}

// ResumeFailed moves Launching -> Idle, killing the process and deleting the
// inserted row [SES-012].
func (m *Machine) ResumeFailed() []Effect {
	if m.state != StateLaunching {
		return nil
	}
	m.reset()
	return []Effect{{Kind: EffTerminateJob}, {Kind: EffCloseJob}, {Kind: EffDeleteSession}}
}

// ProcessAdded handles a new process in the job: Grace -> Running [SES-005];
// ignored in every other state.
func (m *Machine) ProcessAdded(now time.Time) []Effect {
	if m.state != StateGrace {
		return nil
	}
	m.state = StateRunning
	m.runningSince = now
	m.graceGen = 0 // the pending grace timer becomes stale
	return []Effect{
		{Kind: EffUpdateSession, State: domain.SessionRunning, At: now, Active: m.accumulated},
		m.armHeartbeat(),
		m.notify(domain.SessionRunning, now),
	}
}

// TrackedEmpty handles an empty job: Running -> Grace [SES-003]; ignored in
// every other state.
func (m *Machine) TrackedEmpty(now time.Time) []Effect {
	if m.state == StatePaused {
		m.state = StateGrace
		m.graceSince = now
		m.pauseReason = ""
		m.hbGen = 0
		return []Effect{
			{Kind: EffResumePause, State: domain.SessionGrace, At: now, Active: m.accumulated},
			m.armGrace(),
			m.notify(domain.SessionGrace, now),
		}
	}
	if m.state != StateRunning {
		return nil
	}
	m.accumulated += elapsed(m.runningSince, now)
	m.state = StateGrace
	m.graceSince = now
	m.hbGen = 0 // stop re-arming the heartbeat
	return []Effect{
		{Kind: EffUpdateSession, State: domain.SessionGrace, At: now, Active: m.accumulated},
		m.armGrace(),
		m.notify(domain.SessionGrace, now),
	}
}

// Heartbeat persists a running session and re-arms itself [SES-009]. Stale
// generations and ticks outside Running are ignored.
func (m *Machine) Heartbeat(now time.Time, gen uint64) []Effect {
	if (m.state != StateRunning && m.state != StatePaused) || gen == 0 || gen != m.hbGen {
		return nil
	}
	st := domain.SessionRunning
	if m.state == StatePaused {
		st = domain.SessionPaused
	}
	return []Effect{
		{Kind: EffUpdateSession, State: st, At: now, Active: m.activeAt(now)},
		m.armHeartbeat(),
	}
}

// GraceTimeout ends the session when the grace period of the current
// generation expires [SES-004, SES-006, SES-007]. now is the AsOf of the end
// notification; ended_at stays graceSince [IPC-031].
func (m *Machine) GraceTimeout(now time.Time, gen uint64) []Effect {
	if m.state != StateGrace || gen == 0 || gen != m.graceGen {
		return nil
	}
	final := domain.Outcome(m.accumulated)
	endedAt := m.graceSince
	effs := []Effect{
		{Kind: EffEndSession, State: final, At: endedAt, Active: m.accumulated, Reason: domain.EndExited},
		{Kind: EffCloseJob},
	}
	n := m.notify(final, now)
	n.Session.EndedAt = endedAt
	effs = append(effs, n)
	m.reset()
	return effs
}

// Poll applies the periodic job check [SES-013]: Running with 0 active
// processes behaves as TrackedEmpty, Grace with active processes behaves as
// ProcessAdded, and the check is always re-armed. An error changes nothing
// but the re-arm. Stale generations are ignored.
func (m *Machine) Poll(now time.Time, gen uint64, active int, err error) []Effect {
	if m.state == StateIdle || m.state == StateLaunching || gen == 0 || gen != m.pollGen {
		return nil
	}
	var effs []Effect
	if err == nil {
		switch {
		case (m.state == StateRunning || m.state == StatePaused) && active == 0:
			effs = m.TrackedEmpty(now)
		case m.state == StateGrace && active > 0:
			effs = m.ProcessAdded(now)
		}
	}
	return append(effs, m.armPoll())
}

// Stop ends the open session on agent shutdown [SES-011]. It never
// terminates processes and emits no notification.
func (m *Machine) Stop(now time.Time) []Effect {
	var end Effect
	switch m.state {
	case StateRunning:
		m.accumulated += elapsed(m.runningSince, now)
		end = Effect{Kind: EffEndSession, At: now, Reason: domain.EndAgentShutdown}
	case StatePaused:
		end = Effect{Kind: EffEndSession, At: now, Reason: domain.EndAgentShutdown}
	case StateGrace:
		end = Effect{Kind: EffEndSession, At: m.graceSince, Reason: domain.EndExited}
	case StateClosing:
		// The wait does not count: the session ends at the trigger instant, with
		// no kill and no notification (CON-033).
		end = Effect{Kind: EffEndSession, At: m.closeAt, Reason: domain.EndAgentShutdown}
	default:
		return nil
	}
	end.Active = m.accumulated
	end.State = domain.Outcome(m.accumulated)
	m.reset()
	return []Effect{end, {Kind: EffCloseJob}}
}

// Snapshot returns the current session measured at now [IPC-031]; ok is
// false in Idle and Launching [IPC-026].
func (m *Machine) Snapshot(now time.Time) (domain.SessionSnapshot, bool) {
	switch m.state {
	case StateRunning:
		return m.snapshot(domain.SessionRunning, now), true
	case StateGrace:
		return m.snapshot(domain.SessionGrace, now), true
	case StatePaused:
		return m.snapshot(domain.SessionPaused, now), true
	case StateClosing:
		// The origin state, with the time frozen at closeAt (CON-032).
		st := domain.SessionRunning
		if m.closeFrom == StatePaused {
			st = domain.SessionPaused
		}
		return m.snapshot(st, now), true
	default:
		return domain.SessionSnapshot{}, false
	}
}

// Pending returns the launching or open session (sessionID is 0 before the
// insert); ok is false in Idle.
func (m *Machine) Pending() (gameID int64, sessionID int64, title string, ok bool) {
	if m.state == StateIdle {
		return 0, 0, "", false
	}
	return m.gameID, m.sessionID, m.title, true
}

// TogglePause flips Running <-> Paused [SES-016, SES-017]. It is ignored in
// Idle, Launching and Grace and for instants before startedAt [SES-018].
func (m *Machine) TogglePause(now time.Time, reason domain.PauseReason) []Effect {
	switch m.state {
	case StateRunning:
		return m.pause(now, reason, true)
	case StatePaused:
		return m.resume(now)
	}
	return nil
}

// Pause moves Running -> Paused(ui); idempotent in Paused [SES-035].
func (m *Machine) Pause(now time.Time) []Effect {
	if m.state != StateRunning {
		return nil
	}
	return m.pause(now, domain.PauseUI, true)
}

// Resume moves Paused -> Running; idempotent in Running [SES-036].
func (m *Machine) Resume(now time.Time) []Effect {
	if m.state != StatePaused {
		return nil
	}
	return m.resume(now)
}

// Suspend moves Running -> Paused(sleep) without feedback [SES-026].
func (m *Machine) Suspend(now time.Time) []Effect {
	if m.state != StateRunning {
		return nil
	}
	return m.pause(now, domain.PauseSleep, false)
}

// Wake resumes a sleep pause when the user is present [SES-028, SES-029,
// SES-030]; otherwise it is ignored.
func (m *Machine) Wake(now time.Time, userPresent bool) []Effect {
	if m.state != StatePaused || !userPresent || m.pauseReason != domain.PauseSleep {
		return nil
	}
	return m.resume(now)
}

// NotPausable reports why Pause/Resume are refused: ok is true when refused
// ("no-session" in Idle/Launching, "grace" in Grace) and false when pausable
// [SES-037].
func (m *Machine) NotPausable() (reason string, ok bool) {
	switch m.state {
	case StateIdle, StateLaunching:
		return "no-session", true
	case StateGrace:
		return "grace", true
	}
	return "", false
}

// PausedReason returns the reason of the open pause; ok is false outside Paused.
func (m *Machine) PausedReason() (domain.PauseReason, bool) {
	if m.state != StatePaused {
		return "", false
	}
	return m.pauseReason, true
}

func (m *Machine) pause(now time.Time, reason domain.PauseReason, feedback bool) []Effect {
	if now.Before(m.startedAt) {
		return nil
	}
	m.accumulated += elapsed(m.runningSince, now)
	m.state = StatePaused
	m.pausedAt = now
	m.pauseReason = reason
	effs := []Effect{
		{Kind: EffPauseSession, State: domain.SessionPaused, At: now, Active: m.accumulated, Pause: reason},
		m.notify(domain.SessionPaused, now),
	}
	if feedback {
		effs = append(effs, Effect{Kind: EffFeedback, Feedback: FeedbackPaused})
	}
	return effs
}

func (m *Machine) resume(now time.Time) []Effect {
	if now.Before(m.startedAt) {
		return nil
	}
	m.state = StateRunning
	m.runningSince = now
	m.pauseReason = ""
	return []Effect{
		{Kind: EffResumePause, State: domain.SessionRunning, At: now, Active: m.accumulated},
		m.notify(domain.SessionRunning, now),
		{Kind: EffFeedback, Feedback: FeedbackResumed},
	}
}

// activeAt returns the active time at now: accumulated plus the open Running
// interval. The grace period never counts.
func (m *Machine) activeAt(now time.Time) time.Duration {
	if m.state == StateRunning {
		return m.accumulated + elapsed(m.runningSince, now)
	}
	return m.accumulated
}

// elapsed returns now - since, or 0 if the clock went back; a negative
// interval never subtracts accumulated time.
func elapsed(since, now time.Time) time.Duration {
	if d := now.Sub(since); d > 0 {
		return d
	}
	return 0
}

func (m *Machine) snapshot(st domain.SessionState, now time.Time) domain.SessionSnapshot {
	return domain.SessionSnapshot{
		ID:        m.sessionID,
		GameID:    m.gameID,
		Title:     m.title,
		State:     st,
		StartedAt: m.startedAt,
		Active:    m.activeAt(now),
		AsOf:      now,
		Tracking:  m.tracking(),
	}
}

func (m *Machine) notify(st domain.SessionState, now time.Time) Effect {
	return Effect{Kind: EffNotify, State: st, Session: m.snapshot(st, now)}
}

func (m *Machine) nextGen() uint64 {
	m.lastGen++
	return m.lastGen
}

func (m *Machine) armHeartbeat() Effect {
	m.hbGen = m.nextGen()
	return Effect{Kind: EffArmHeartbeat, Gen: m.hbGen, Delay: m.heartbeat}
}

func (m *Machine) armGrace() Effect {
	m.graceGen = m.nextGen()
	return Effect{Kind: EffArmGrace, Gen: m.graceGen, Delay: m.grace}
}

func (m *Machine) armPoll() Effect {
	m.pollGen = m.nextGen()
	return Effect{Kind: EffArmPoll, Gen: m.pollGen, Delay: m.poll}
}

// reset returns to Idle, invalidating every pending timer. lastGen is kept so
// that generations are never reused across sessions.
func (m *Machine) reset() {
	*m = Machine{
		grace:     m.grace,
		heartbeat: m.heartbeat,
		poll:      m.poll,
		lastGen:   m.lastGen,
		adoptGen:  0,
	}
}

// InsertedURI records the session of a URI launch (RUL-003). There is no job to
// resume: the session waits in Launching for its game process, and the adoption
// timeout is armed.
func (m *Machine) InsertedURI(id int64) []Effect {
	if m.state != StateLaunching {
		return nil
	}
	m.sessionID = id
	m.adoptGen = m.nextGen()
	return []Effect{{Kind: EffArmAdopt, Gen: m.adoptGen, Delay: domain.SessionAdoptTimeout}}
}

// AdoptTimeout ends a URI launch whose game process never appeared (RUL-003). The
// session ends with no active time, so its outcome is discarded. Stale generations
// and other states are ignored.
func (m *Machine) AdoptTimeout(now time.Time, gen uint64) []Effect {
	if m.state != StateLaunching || gen == 0 || gen != m.adoptGen {
		return nil
	}
	effs := []Effect{
		{Kind: EffEndSession, State: domain.SessionDiscarded, At: now, Active: 0, Reason: domain.EndClientNoLaunch},
		{Kind: EffCloseJob}, // stops the watcher of the adopted process
	}
	n := m.notify(domain.SessionDiscarded, now)
	n.Session.EndedAt = now
	effs = append(effs, n)
	m.reset()
	return effs
}

// Adopted moves Launching -> Running when the watcher finds the game process
// (RUL-002). The session counts from the adoption instant: the time the shell
// took to start the game is not play time. The timers start as in Resumed.
func (m *Machine) Adopted(now time.Time) []Effect {
	if m.state != StateLaunching {
		return nil
	}
	m.adopted = true
	m.state = StateRunning
	m.runningSince = now
	m.accumulated = 0
	return []Effect{
		m.armHeartbeat(),
		m.armPoll(),
		m.notify(domain.SessionRunning, now),
	}
}

// tracking reports the tracking mode of the session in progress (RUL-005).
func (m *Machine) tracking() domain.TrackingMode {
	if m.adopted {
		return domain.TrackProcess
	}
	return domain.TrackJob
}

// SteamStopped ends the grace of an adopted session at once when Steam reports the
// app as no longer running (STM-001): the grace timer is armed with no delay, so the
// session ends on the next turn of the loop instead of after SessionGracePeriod.
func (m *Machine) SteamStopped(now time.Time) []Effect {
	if m.state != StateRunning {
		return nil
	}
	m.accumulated += elapsed(m.runningSince, now)
	m.state = StateGrace
	m.graceSince = now
	m.hbGen = 0
	m.graceGen = m.nextGen()
	return []Effect{
		{Kind: EffUpdateSession, State: domain.SessionGrace, At: now, Active: m.accumulated},
		{Kind: EffArmGrace, Gen: m.graceGen, Delay: 0},
		m.notify(domain.SessionGrace, now),
	}
}

// CloseRequested applies the close hotkey at the trigger instant at
// (contracts/session-close-state-machine.md). Running and Paused move to
// StateClosing with the active time frozen at at and no effects: nothing is
// persisted or notified while the game is asked to close [CON-005, CON-006].
// Grace ends the session at once at graceSince (the job is already empty)
// [CON-007]. Idle, Launching, Closing and instants before startedAt are
// ignored [CON-012, CON-032]. No path emits feedback [CON-014].
func (m *Machine) CloseRequested(at time.Time) []Effect {
	switch m.state {
	case StateRunning:
		if at.Before(m.startedAt) {
			return nil
		}
		m.accumulated += elapsed(m.runningSince, at)
		m.closeFrom = StateRunning
	case StatePaused:
		if at.Before(m.startedAt) {
			return nil
		}
		m.closeFrom = StatePaused // the open pause interval stays open
	case StateGrace:
		final := domain.Outcome(m.accumulated)
		endedAt := m.graceSince
		effs := []Effect{
			{Kind: EffEndSession, State: final, At: endedAt, Active: m.accumulated, Reason: domain.EndClosedByHotkey},
			{Kind: EffCloseJob},
		}
		n := m.notify(final, at)
		n.Session.EndedAt = endedAt
		effs = append(effs, n)
		m.reset() // graceGen = 0: the pending grace timer becomes stale
		return effs
	default:
		return nil
	}
	m.state = StateClosing
	m.closeAt = at
	return nil
}

// CloseDone ends a closing session once its process tree is gone (it exited
// or was killed): ended_at is closeAt, the reason closed-by-hotkey and the
// state follows DOM-008 (ended or discarded) [CON-005, CON-006]. now is the
// AsOf of the end notification. It never emits feedback [CON-014].
func (m *Machine) CloseDone(now time.Time) []Effect {
	if m.state != StateClosing {
		return nil
	}
	final := domain.Outcome(m.accumulated)
	endedAt := m.closeAt
	effs := []Effect{
		{Kind: EffEndSession, State: final, At: endedAt, Active: m.accumulated, Reason: domain.EndClosedByHotkey},
		{Kind: EffCloseJob},
	}
	n := m.notify(final, now)
	n.Session.EndedAt = endedAt
	effs = append(effs, n)
	m.reset()
	return effs
}

// CloseUndone returns a closing session to its origin state when the game
// could not be closed [CON-013]. Running counts again from closeAt, so the
// wait is play time with no gap; Paused keeps its open interval. The heartbeat
// and the periodic check, which Closing does not re-arm, are re-armed with new
// generations (tasks.md, finding 2). Nothing is persisted or notified.
func (m *Machine) CloseUndone() []Effect {
	if m.state != StateClosing {
		return nil
	}
	m.state = m.closeFrom
	if m.state == StateRunning {
		m.runningSince = m.closeAt
	}
	m.closeAt, m.closeFrom = time.Time{}, StateIdle
	return []Effect{m.armHeartbeat(), m.armPoll()}
}

// ClosingAt returns the trigger instant and the origin state of a closing
// session; ok is false outside StateClosing.
func (m *Machine) ClosingAt() (at time.Time, from State, ok bool) {
	if m.state != StateClosing {
		return time.Time{}, StateIdle, false
	}
	return m.closeAt, m.closeFrom, true
}
