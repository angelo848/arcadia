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
)

// Effect is a side effect requested by a transition. Effects are returned in
// the order they must run.
type Effect struct {
	Kind    EffectKind
	State   domain.SessionState     // Update: running|grace; End/Notify: resulting state
	At      time.Time               // Start: started_at; Update: last_seen; End: ended_at
	Active  time.Duration           // Update/End: active time
	Reason  domain.SessionEndReason // End
	Gen     uint64                  // Arm*: generation to pass back
	Delay   time.Duration           // Arm*
	Session domain.SessionSnapshot  // Notify (AsOf = event instant)
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
	accumulated  time.Duration

	// Timer generations. lastGen is a single monotonic counter so that no
	// generation is ever reused; 0 means "no timer armed".
	lastGen  uint64
	hbGen    uint64
	graceGen uint64
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
	if m.state != StateRunning || gen == 0 || gen != m.hbGen {
		return nil
	}
	return []Effect{
		{Kind: EffUpdateSession, State: domain.SessionRunning, At: now, Active: m.activeAt(now)},
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
	if (m.state != StateRunning && m.state != StateGrace) || gen == 0 || gen != m.pollGen {
		return nil
	}
	var effs []Effect
	if err == nil {
		switch {
		case m.state == StateRunning && active == 0:
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
	case StateGrace:
		end = Effect{Kind: EffEndSession, At: m.graceSince, Reason: domain.EndExited}
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
	}
}
