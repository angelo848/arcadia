package domain

import (
	"errors"
	"fmt"
	"time"
)

// SessionState is the persisted state of a play session (ADR 0012: text column
// validated by the application).
type SessionState string

// Session states.
const (
	SessionRunning   SessionState = "running"
	SessionPaused    SessionState = "paused"
	SessionGrace     SessionState = "grace"
	SessionEnded     SessionState = "ended"
	SessionDiscarded SessionState = "discarded"
)

// errInvalidSessionState is returned when a text value is not a known SessionState.
var errInvalidSessionState = errors.New("invalid session state")

// ParseSessionState converts a stored text value into a SessionState. The match
// is exact (case-sensitive, no trimming).
func ParseSessionState(s string) (SessionState, error) {
	switch st := SessionState(s); st {
	case SessionRunning, SessionPaused, SessionGrace, SessionEnded, SessionDiscarded:
		return st, nil
	default:
		return "", fmt.Errorf("parse session state %q: %w", s, errInvalidSessionState)
	}
}

// SessionEndReason records why a play session ended.
type SessionEndReason string

// Session end reasons.
const (
	EndExited        SessionEndReason = "exited"
	EndAgentShutdown SessionEndReason = "agent-shutdown"
	EndRecovered     SessionEndReason = "recovered"
)

// errInvalidSessionEndReason is returned when a text value is not a known SessionEndReason.
var errInvalidSessionEndReason = errors.New("invalid session end reason")

// ParseSessionEndReason converts a stored text value into a SessionEndReason.
// The match is exact (case-sensitive, no trimming).
func ParseSessionEndReason(s string) (SessionEndReason, error) {
	switch r := SessionEndReason(s); r {
	case EndExited, EndAgentShutdown, EndRecovered:
		return r, nil
	default:
		return "", fmt.Errorf("parse session end reason %q: %w", s, errInvalidSessionEndReason)
	}
}

// Session timing parameters.
const (
	// SessionGracePeriod is how long an empty job may stay empty before the session ends.
	SessionGracePeriod = 15 * time.Second
	// MinCountedSession is the minimum active time for a session to count.
	MinCountedSession = 30 * time.Second
	// SessionHeartbeatInterval is how often a running session is persisted.
	SessionHeartbeatInterval = 30 * time.Second
	// JobPollInterval is how often the job is checked for live processes.
	JobPollInterval = 5 * time.Second
)

// Outcome returns the final state of a finished session with the given active
// time: SessionEnded if active >= MinCountedSession, otherwise SessionDiscarded.
func Outcome(active time.Duration) SessionState {
	if active >= MinCountedSession {
		return SessionEnded
	}
	return SessionDiscarded
}

// SessionSnapshot is a point-in-time view of a play session.
type SessionSnapshot struct {
	ID, GameID int64
	Title      string
	State      SessionState
	StartedAt  time.Time
	EndedAt    time.Time     // zero while running/grace
	Active     time.Duration // measured at AsOf
	AsOf       time.Time
}
