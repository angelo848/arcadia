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
	// LaunchDetached starts a game that does not count time [SES-031..034]: it
	// is created suspended inside a Job Object of its own that only keeps the
	// tree so that it can be closed by the hotkey (CON-010, CON-011). The job
	// has no KILL_ON_JOB_CLOSE (ADR 0005), so closing it never kills the game;
	// no session, no completion port and no goroutine. hProcess and hThread are
	// closed before returning. The error is a *LaunchError.
	LaunchDetached(spec LaunchSpec) (job DetachedJob, pid uint32, err error)
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
	EventSteamStopped // Steam reports the app stopped after adoption (STM-001)
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
	// RequestClose posts WM_CLOSE to the visible, unowned top-level windows of
	// the live processes of the tree and returns how many windows got it
	// (CON-030). It never waits and never kills.
	RequestClose() (int, error)
	// Kill terminates every process of the tree (CON-030, CON-008). Unlike
	// Terminate it is valid after Resume; it never closes the job.
	Kill() error
	// Close stops the reader and releases every handle. It never kills
	// processes and is idempotent.
	Close() error
}

// DetachedJob is the Job Object of a game launched without counting time
// (CON-010, CON-011). The tracker keeps the last one only to close the game by
// the hotkey; it never records a session.
type DetachedJob interface {
	// ActiveProcesses returns the number of live processes in the job.
	ActiveProcesses() (int, error)
	// RequestClose posts WM_CLOSE to the visible, unowned top-level windows of
	// the processes in the job and returns how many windows got it (CON-030).
	RequestClose() (int, error)
	// Kill terminates every process in the job; it never closes the job.
	Kill() error
	// Close releases the job handle. It never kills and is idempotent; it is
	// the only owner of the job handle.
	Close() error
}

// WindowCloser posts WM_CLOSE to the visible, unowned top-level windows of the
// given processes and returns how many windows got it (CON-030). An empty set
// posts nothing and enumerates nothing. It is exported only so that tests can
// inject one through Config.Closer.
type WindowCloser interface {
	PostClose(pids map[uint32]struct{}) (posted int, err error)
}

// ProcessKiller terminates pid only if its image path is still wantImage
// (compared without regard to case), so that a recycled PID is never killed
// (CON-008). It is exported only so that tests can inject one through
// Config.Killer.
type ProcessKiller interface {
	Kill(pid uint32, wantImage string) error
}

// Internal names used by the platform code.
type (
	windowCloser  = WindowCloser
	processKiller = ProcessKiller
)

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
