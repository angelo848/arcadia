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
	// addGameTimeout bounds each AddGame binding call.
	addGameTimeout = 5 * time.Second
	// listGamesTimeout bounds each ListGames binding call.
	listGamesTimeout = 2 * time.Second
	// launchGameTimeout bounds each LaunchGame call (above the handler's 10 s).
	launchGameTimeout = 12 * time.Second
	// currentSessionTimeout bounds each CurrentSession binding call.
	currentSessionTimeout = 2 * time.Second
	// sessionChangedEvent is the Wails event that forwards session.changed (DSK-010).
	sessionChangedEvent = "session:changed"
	// viewAsOfLayout is RFC 3339 with exactly three fraction digits.
	viewAsOfLayout = "2006-01-02T15:04:05.000Z07:00"
)

// pickExeTitle is the title of the executable dialog (DSK-012). It is a var,
// not a const, only because games_test.go compares c.title against both this
// name and the literal, which go vet (bools, also run by go test) flags as
// "suspect or" when both operands are constants.
var pickExeTitle = "Escolha o executável do jogo"

// pickExeFilters is the filter of the executable dialog (DSK-012).
var pickExeFilters = []fileFilter{{DisplayName: "Executáveis (*.exe)", Pattern: "*.exe"}}

// GameView is a game as shown by the frontend.
type GameView struct {
	ID               int64  `json:"id"`
	Title            string `json:"title"`
	Path             string `json:"path"`
	TotalPlaySeconds int64  `json:"totalPlaySeconds"`
	PlayCount        int64  `json:"playCount"`
	LastPlayedAt     string `json:"lastPlayedAt"` // RFC 3339 or ""
}

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
}

// PickedExe is the result of PickGameExe; the zero value means cancelled.
type PickedExe struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// fileFilter is one filter of the open-file dialog.
type fileFilter struct{ DisplayName, Pattern string }

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func gameView(g ipc.Game) GameView {
	last := ""
	if g.LastPlayedAt != nil {
		last = formatTime(*g.LastPlayedAt)
	}
	return GameView{
		ID:               g.ID,
		Title:            g.Title,
		Path:             g.Path,
		TotalPlaySeconds: g.TotalPlaySeconds,
		PlayCount:        g.PlayCount,
		LastPlayedAt:     last,
	}
}

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
	}, nil
}

// errNoDialog is PickGameExe before startup gave the Wails context.
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
	a.mu.Lock()
	c := a.client
	a.mu.Unlock()
	if c == nil {
		return userError(errNoClient, a.logger())
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := c.Call(ctx, method, params, result); err != nil {
		return userError(fmt.Errorf("desktop: %s: %w", method, err), a.logger())
	}
	return nil
}

// PickGameExe opens the executable dialog; cancelling yields the zero
// PickedExe and nothing is sent to the agent (DSK-012).
func (a *App) PickGameExe() (PickedExe, error) {
	a.mu.Lock()
	wailsCtx := a.wailsCtx
	a.mu.Unlock()
	if wailsCtx == nil || a.platform.openFile == nil {
		return PickedExe{}, userError(errNoDialog, a.logger())
	}
	path, err := a.platform.openFile(wailsCtx, pickExeTitle, pickExeFilters)
	if err != nil {
		return PickedExe{}, userError(fmt.Errorf("desktop: pick exe: %w", err), a.logger())
	}
	if path == "" {
		return PickedExe{}, nil
	}
	return PickedExe{Path: path, Title: domain.DefaultGameTitle(path)}, nil
}

// AddGame calls game.add (5 s timeout).
func (a *App) AddGame(path, title string) (GameView, error) {
	var g ipc.Game
	if err := a.callAgent(ipc.MethodGameAdd, addGameTimeout, ipc.GameAddParams{Path: path, Title: title}, &g); err != nil {
		return GameView{}, err
	}
	return gameView(g), nil
}

// ListGames calls game.list (2 s timeout); the result is never nil.
func (a *App) ListGames() ([]GameView, error) {
	var res ipc.GameListResult
	if err := a.callAgent(ipc.MethodGameList, listGamesTimeout, nil, &res); err != nil {
		return nil, err
	}
	out := make([]GameView, 0, len(res.Games))
	for _, g := range res.Games {
		out = append(out, gameView(g))
	}
	return out, nil
}

// LaunchGame calls game.launch (12 s timeout). On a client timeout the agent
// may still have launched; session:changed tells the real state.
func (a *App) LaunchGame(id int64) (SessionView, error) {
	var s ipc.Session
	if err := a.callAgent(ipc.MethodGameLaunch, launchGameTimeout, ipc.GameLaunchParams{GameID: id}, &s); err != nil {
		return SessionView{}, err
	}
	v, err := sessionView(s)
	if err != nil {
		return SessionView{}, userError(fmt.Errorf("desktop: %s: %w", ipc.MethodGameLaunch, err), a.logger())
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
