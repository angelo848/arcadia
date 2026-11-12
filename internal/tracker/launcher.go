// Package tracker launches games inside a Job Object and tracks their play
// sessions. This file holds the portable contracts between the session
// coordinator and the platform-specific launcher.
package tracker

import (
	"context"
	"fmt"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// LaunchKind says how a session starts.
type LaunchKind int

// Launch kinds. KindExe is the zero value: an executable launched in a Job Object.
const (
	KindExe LaunchKind = iota // exe: created suspended in a Job Object (SES-001)
	KindURI                   // uri: opened by the shell; the game is adopted later (RUL-003)
)

// LaunchSpec describes the game to start. For KindURI, Exe holds the URI.
type LaunchSpec struct {
	GameID           int64
	Title, Exe, Args string
	WorkDir          string // "" => dir(Exe)
	Kind             LaunchKind
}

// Launcher creates the game's process tree, suspended and already assigned
// to a job, so that no child can escape tracking.
type Launcher interface {
	Prepare(spec LaunchSpec) (Job, error)
	// LaunchDetached creates the process without a Job Object and without
	// CREATE_SUSPENDED; it closes hProcess and hThread before returning. The
	// error is a *LaunchError.
	LaunchDetached(spec LaunchSpec) (pid uint32, err error)
	// OpenURI asks the shell to open uri; it returns no process handle. The
	// error is a *LaunchError (RUL-003).
	OpenURI(uri string) error
}

// JobEventKind classifies a job notification.
type JobEventKind int

// Job notification kinds.
const (
	EventNewProcess JobEventKind = iota + 1
	EventExitProcess
	EventActiveZero
)

// JobEvent is a notification from the job's completion port.
type JobEvent struct {
	Kind     JobEventKind
	PID      uint32
	Root     bool   // PID is the initial process
	ExitCode uint32 // only for root exits
}

// Job is a launched game's process tree.
type Job interface {
	// RootPID returns the PID of the initial process.
	RootPID() uint32
	// Resume resumes the initial thread; it is called once.
	Resume() error
	// Terminate kills the tree; only valid before Resume (failed launch).
	Terminate() error
	// Events returns the job notifications; the channel is closed after Close.
	Events() <-chan JobEvent
	// ActiveProcesses returns the number of live processes in the job.
	ActiveProcesses() (int, error)
	// Close stops the reader and releases every handle. It never kills
	// processes and is idempotent.
	Close() error
}

// LaunchError is the error returned when a game cannot be started.
type LaunchError struct {
	Err      error
	WinError uint32 // 0 = no Windows code
}

// Error implements error.
func (e *LaunchError) Error() string {
	if e.WinError != 0 {
		return fmt.Sprintf("launch game: %v (windows error %d)", e.Err, e.WinError)
	}
	return fmt.Sprintf("launch game: %v", e.Err)
}

// Unwrap returns the underlying error.
func (e *LaunchError) Unwrap() error { return e.Err }

// SessionStore persists play sessions; *store.Store implements it.
type SessionStore interface {
	StartSession(ctx context.Context, gameID int64, at time.Time) (int64, error)
	UpdateSession(ctx context.Context, id int64, st domain.SessionState, lastSeen time.Time, active time.Duration) error
	EndSession(ctx context.Context, e store.SessionEnd) (domain.SessionState, error)
	DeleteSession(ctx context.Context, id int64) error
	// PauseSession opens a pause interval and marks the session paused [SES-016].
	PauseSession(ctx context.Context, p store.PauseStart) error
	// ResumePause closes the open pause interval [SES-017].
	ResumePause(ctx context.Context, p store.PauseEnd) error
	// TrackingRules returns the rules that adopt the processes of a game (RUL-002).
	TrackingRules(ctx context.Context, gameID int64) ([]store.TrackingRule, error)
}
