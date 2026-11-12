package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

const (
	// launchGameTimeout bounds each LaunchGame call (above the handler's 10 s).
	launchGameTimeout = 12 * time.Second
	// launchUntrackedTimeout bounds each LaunchGameUntracked call (DSK-013).
	launchUntrackedTimeout = 12 * time.Second
	// pauseSessionTimeout bounds each PauseSession and ResumeSession call (DSK-015).
	pauseSessionTimeout = 5 * time.Second
	// currentSessionTimeout bounds each CurrentSession binding call.
	currentSessionTimeout = 2 * time.Second
	// sessionChangedEvent is the Wails event that forwards session.changed (DSK-010).
	sessionChangedEvent = "session:changed"
	// viewAsOfLayout is RFC 3339 with exactly three fraction digits.
	viewAsOfLayout = "2006-01-02T15:04:05.000Z07:00"
)

// pickExeFilters is the filter of the executable dialogs (DSK-012, EMU-006).
var pickExeFilters = []fileFilter{{DisplayName: "Executáveis (*.exe)", Pattern: "*.exe"}}

// SessionView is a session as shown by the frontend (DSK-010).
type SessionView struct {
	SessionID     int64  `json:"sessionId"`
	GameID        int64  `json:"gameId"`
	Title         string `json:"title"`
	State         string `json:"state"`
	StartedAt     string `json:"startedAt"`
	EndedAt       string `json:"endedAt"` // RFC 3339 or ""
	ActiveSeconds int64  `json:"activeSeconds"`
	ActiveMs      int64  `json:"activeMs"`
	AsOf          string `json:"asOf"` // RFC 3339 UTC with ms
	// Tracking is "job" or "process": the UI says so for adopted games (RUL-005).
	Tracking string `json:"tracking"`
}

// fileFilter is one filter of the open-file dialog.
type fileFilter struct{ DisplayName, Pattern string }

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// sessionView converts s; it fails when the state is unknown.
func sessionView(s ipc.Session) (SessionView, error) {
	if _, err := domain.ParseSessionState(s.State); err != nil {
		return SessionView{}, fmt.Errorf("desktop: session view: %w", err)
	}
	ended := ""
	if s.EndedAt != nil {
		ended = formatTime(*s.EndedAt)
	}
	return SessionView{
		SessionID:     s.SessionID,
		GameID:        s.GameID,
		Title:         s.Title,
		State:         s.State,
		StartedAt:     formatTime(s.StartedAt),
		EndedAt:       ended,
		ActiveSeconds: s.ActiveSeconds,
		ActiveMs:      s.ActiveMs,
		AsOf:          s.AsOf.UTC().Format(viewAsOfLayout),
		Tracking:      s.Tracking,
	}, nil
}

// errNoDialog is a file dialog asked for before startup gave the Wails context.
var errNoDialog = errors.New("desktop: pick exe: dialog not available")

// logger returns the App's logger (slog.Default before startup).
func (a *App) logger() *slog.Logger {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.log == nil {
		return slog.Default()
	}
	return a.log
}

// callAgent calls method with timeout; failures are already user texts.
func (a *App) callAgent(method string, timeout time.Duration, params, result any) error {
	if err := a.callAgentRaw(method, timeout, params, result); err != nil {
		return userError(err, a.logger())
	}
	return nil
}

// callAgentRaw calls method with timeout and returns the error unconverted, so
// a caller can map a JSON-RPC code itself (DeleteGame, DSK-018).
func (a *App) callAgentRaw(method string, timeout time.Duration, params, result any) error {
	a.mu.Lock()
	c := a.client
	a.mu.Unlock()
	if c == nil {
		return errNoClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := c.Call(ctx, method, params, result); err != nil {
		return fmt.Errorf("desktop: %s: %w", method, err)
	}
	return nil
}

// LaunchGame calls game.launch (12 s timeout). On a client timeout the agent
// may still have launched; session:changed tells the real state.
func (a *App) LaunchGame(id int64) (SessionView, error) {
	var s ipc.Session
	if err := a.callAgent(ipc.MethodGameLaunch, launchGameTimeout, ipc.GameLaunchParams{GameID: id}, &s); err != nil {
		return SessionView{}, err
	}
	return a.convertSession(ipc.MethodGameLaunch, s)
}

// LaunchGameUntracked calls game.launchUntracked (12 s timeout, DSK-013). The
// game runs without time tracking. It returns no value and emits no event:
// session:changed, if any, comes from the agent notification (DSK-014).
func (a *App) LaunchGameUntracked(id int64) error {
	var res ipc.LaunchedProcess
	return a.callAgent(ipc.MethodGameLaunchUntracked, launchUntrackedTimeout,
		ipc.GameLaunchUntrackedParams{GameID: id}, &res)
}

// PauseSession calls session.pause (5 s timeout, DSK-015) and returns the
// session measured by the agent. It emits no event.
func (a *App) PauseSession() (SessionView, error) {
	return a.sessionControl(ipc.MethodSessionPause)
}

// ResumeSession calls session.resume (5 s timeout, DSK-015) and returns the
// session measured by the agent. It emits no event.
func (a *App) ResumeSession() (SessionView, error) {
	return a.sessionControl(ipc.MethodSessionResume)
}

// sessionControl calls a session.pause or session.resume method.
func (a *App) sessionControl(method string) (SessionView, error) {
	var res ipc.SessionResult
	if err := a.callAgent(method, pauseSessionTimeout, nil, &res); err != nil {
		return SessionView{}, err
	}
	return a.convertSession(method, res.Session)
}

// convertSession converts the session of a method result; an unknown state is
// an unexpected failure.
func (a *App) convertSession(method string, s ipc.Session) (SessionView, error) {
	v, err := sessionView(s)
	if err != nil {
		return SessionView{}, userError(fmt.Errorf("desktop: %s: %w", method, err), a.logger())
	}
	return v, nil
}

// CurrentSession calls session.current (2 s timeout); nil means no session.
func (a *App) CurrentSession() (*SessionView, error) {
	var res ipc.SessionCurrentResult
	if err := a.callAgent(ipc.MethodSessionCurrent, currentSessionTimeout, nil, &res); err != nil {
		return nil, err
	}
	if res.Session == nil {
		return nil, nil
	}
	v, err := sessionView(*res.Session)
	if err != nil {
		return nil, userError(fmt.Errorf("desktop: %s: %w", ipc.MethodSessionCurrent, err), a.logger())
	}
	return &v, nil
}
