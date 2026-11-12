package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// launchHandlerTimeout bounds the wait of game.launch for the tracker's answer
// [IPC-025]. When it ends first the launch goes on in the tracker and its real
// outcome reaches the UI through session.changed.
const launchHandlerTimeout = 10 * time.Second

// pauseHandlerTimeout bounds the wait of session.pause and session.resume for
// the tracker's answer.
const pauseHandlerTimeout = 5 * time.Second

// launchTypeExe is the only launch_type supported in this phase.
const launchTypeExe = "exe"

// Texts of LaunchFailedData.error produced by the agent itself.
const (
	launchErrShuttingDown = "agente encerrando"
	launchErrUnsupported  = "tipo de lançamento não suportado"
)

// gameStore is the part of *store.Store used by the game handlers.
type gameStore interface {
	AddGame(ctx context.Context, g store.NewGame) (store.Game, error)
	ListGames(ctx context.Context, q store.Query) (store.Page, error)
	Game(ctx context.Context, id int64) (store.Game, error)
}

// sessionTracker is the part of *tracker.Tracker used by the game handlers.
type sessionTracker interface {
	Launch(ctx context.Context, spec tracker.LaunchSpec) (domain.SessionSnapshot, error)
	Current() (domain.SessionSnapshot, bool)
	LaunchUntracked(ctx context.Context, spec tracker.LaunchSpec) (uint32, error)
	Pause(ctx context.Context, at time.Time) (domain.SessionSnapshot, error)
	Resume(ctx context.Context, at time.Time) (domain.SessionSnapshot, error)
}

// gameAddParams are the server-side params of game.add; pointers tell an
// absent member from an empty one.
type gameAddParams struct {
	Path  *string `json:"path"`
	Title *string `json:"title"`
}

// gameLaunchParams are the server-side params of game.launch; the raw gameId
// is validated by parseGameID.
type gameLaunchParams struct {
	GameID json.RawMessage `json:"gameId"`
}

// gameHandlers returns the handlers of game.add, game.list, game.launch and
// session.current [IPC-020..IPC-026, IPC-028, IPC-031, AGT-011]. stat is
// os.Stat in production.
func gameHandlers(st gameStore, tr sessionTracker, clock domain.Clock,
	stat func(string) (fs.FileInfo, error), log *slog.Logger) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodGameAdd: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleGameAdd(ctx, st, clock, stat, log, params)
		},
		ipc.MethodGameList: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleGameList(ctx, st, log, params)
		},
		ipc.MethodGameLaunch: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleGameLaunch(ctx, st, tr, log, params)
		},
		ipc.MethodGameLaunchUntracked: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleGameLaunchUntracked(ctx, st, tr, log, params)
		},
		ipc.MethodSessionPause: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleSessionPause(ctx, params, clock, log, "session.pause", tr.Pause)
		},
		ipc.MethodSessionResume: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleSessionPause(ctx, params, clock, log, "session.resume", tr.Resume)
		},
		ipc.MethodSessionCurrent: ipc.NoParams(func(context.Context) (any, error) {
			snap, ok := tr.Current()
			if !ok {
				return ipc.SessionCurrentResult{}, nil
			}
			s := ipc.SessionFromSnapshot(snap)
			return ipc.SessionCurrentResult{Session: &s}, nil
		}),
	}
}

// handleGameAdd validates the params, the executable and the title, then adds
// the game [GAM-001..GAM-003, IPC-020, IPC-021].
func handleGameAdd(ctx context.Context, st gameStore, clock domain.Clock,
	stat func(string) (fs.FileInfo, error), log *slog.Logger, params json.RawMessage) (any, error) {
	if params == nil {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	var p gameAddParams
	if rpcErr := ipc.DecodeParams(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if p.Path == nil {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}

	clean, reason, err := validateExe(*p.Path, stat)
	if err != nil {
		log.Error("falha ao verificar o executável", "path", *p.Path, "error", err.Error())
		return nil, fmt.Errorf("game.add: %w", err)
	}
	if reason != "" {
		return nil, ipc.NewErrorData(ipc.CodeInvalidExecutable, ipc.MsgInvalidExecutable,
			ipc.InvalidExecutableData{Reason: reason})
	}
	title, ok := gameTitle(p.Title, clean)
	if !ok {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}

	g, err := st.AddGame(ctx, store.NewGame{
		Title:      title,
		Exe:        clean,
		WorkingDir: filepath.Dir(clean),
		At:         clock.Now(),
	})
	if err != nil {
		log.Error("falha ao adicionar o jogo", "path", clean, "error", err.Error())
		return nil, fmt.Errorf("game.add: %w", err)
	}
	return toIPCGame(g), nil
}

// handleGameLaunch checks the game and asks the tracker to launch it
// [IPC-024, IPC-025, AGT-011].
func handleGameLaunch(ctx context.Context, st gameStore, tr sessionTracker, log *slog.Logger,
	params json.RawMessage) (any, error) {
	g, spec, err := prepareLaunch(ctx, st, params, log)
	if err != nil {
		return nil, err
	}
	lctx, cancel := context.WithTimeout(ctx, launchHandlerTimeout)
	defer cancel()
	snap, err := tr.Launch(lctx, spec)
	if err != nil {
		return nil, launchError(err, g.ID, log)
	}
	return ipc.SessionFromSnapshot(snap), nil
}

// handleGameLaunchUntracked launches the game without a session.
func handleGameLaunchUntracked(ctx context.Context, st gameStore, tr sessionTracker, log *slog.Logger,
	params json.RawMessage) (any, error) {
	g, spec, err := prepareLaunch(ctx, st, params, log)
	if err != nil {
		return nil, err
	}
	lctx, cancel := context.WithTimeout(ctx, launchHandlerTimeout)
	defer cancel()
	pid, err := tr.LaunchUntracked(lctx, spec)
	if err != nil {
		return nil, launchError(err, g.ID, log)
	}
	return ipc.LaunchedProcess{PID: pid}, nil
}

// prepareLaunch validates the params and the game shared by game.launch and
// game.launchUntracked and builds the launch spec.
func prepareLaunch(ctx context.Context, st gameStore, params json.RawMessage,
	log *slog.Logger) (store.Game, tracker.LaunchSpec, error) {
	var none tracker.LaunchSpec
	if params == nil {
		return store.Game{}, none, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	var p gameLaunchParams
	if rpcErr := ipc.DecodeParams(params, &p); rpcErr != nil {
		return store.Game{}, none, rpcErr
	}
	id, ok := parseGameID(p.GameID)
	if !ok {
		return store.Game{}, none, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}

	g, err := st.Game(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrGameNotFound) {
			return store.Game{}, none,
				ipc.NewErrorData(ipc.CodeGameNotFound, ipc.MsgGameNotFound, ipc.GameNotFoundData{GameID: id})
		}
		log.Error("falha ao ler o jogo", "game_id", id, "error", err.Error())
		return store.Game{}, none, fmt.Errorf("game.launch: %w", err)
	}
	if g.LaunchType != launchTypeExe {
		return store.Game{}, none, ipc.NewErrorData(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed,
			ipc.LaunchFailedData{Error: launchErrUnsupported})
	}
	return g, tracker.LaunchSpec{
		GameID:  g.ID,
		Title:   g.Title,
		Exe:     g.LaunchTarget,
		Args:    g.LaunchArgs,
		WorkDir: g.WorkingDir,
	}, nil
}

// handleSessionPause serves session.pause and session.resume; op is the
// tracker method and at is read when the request arrives. These calls do not
// go through the pause router [PAU-008].
func handleSessionPause(ctx context.Context, params json.RawMessage, clock domain.Clock, log *slog.Logger,
	method string, op func(context.Context, time.Time) (domain.SessionSnapshot, error)) (any, error) {
	at := clock.Now()
	return ipc.NoParams(func(ctx context.Context) (any, error) {
		pctx, cancel := context.WithTimeout(ctx, pauseHandlerTimeout)
		defer cancel()
		snap, err := op(pctx, at)
		if err != nil {
			var np *tracker.NotPausableError
			if errors.As(err, &np) {
				return nil, ipc.NewErrorData(ipc.CodeSessionNotPausable, ipc.MsgSessionNotPausable,
					ipc.SessionNotPausableData{Reason: np.Reason})
			}
			if errors.Is(err, tracker.ErrShuttingDown) {
				log.Debug("agente encerrando", "method", method)
			} else {
				log.Error("falha na pausa da sessão", "method", method, "error", err.Error())
			}
			return nil, fmt.Errorf("%s: %w", method, err)
		}
		return ipc.SessionResult{Session: ipc.SessionFromSnapshot(snap)}, nil
	})(ctx, params)
}

// launchError maps a tracker.Launch error to its JSON-RPC error [IPC-025,
// AGT-011]; anything unexpected (including the end of the handler ctx) is
// logged and becomes -32603.
func launchError(err error, gameID int64, log *slog.Logger) error {
	var active *tracker.SessionActiveError
	if errors.As(err, &active) {
		return ipc.NewErrorData(ipc.CodeSessionActive, ipc.MsgSessionActive, ipc.SessionRef{
			SessionID: active.Session.ID,
			GameID:    active.Session.GameID,
			Title:     active.Session.Title,
		})
	}
	if errors.Is(err, tracker.ErrShuttingDown) {
		return ipc.NewErrorData(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed,
			ipc.LaunchFailedData{Error: launchErrShuttingDown})
	}
	var le *tracker.LaunchError
	if errors.As(err, &le) {
		data := ipc.LaunchFailedData{Error: le.Error()}
		if le.Err != nil {
			data.Error = le.Err.Error()
		}
		if le.WinError != 0 {
			code := le.WinError
			data.WinError = &code
		}
		return ipc.NewErrorData(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed, data)
	}
	log.Error("falha ao lançar o jogo", "game_id", gameID, "error", err.Error())
	return fmt.Errorf("game.launch: %w", err)
}

// validateExe checks p in the GAM-002 order (absolute, exists, regular file,
// ".exe" extension in any case) and returns the cleaned path, or the reason
// it is invalid. A stat failure other than "does not exist" is returned as
// err.
func validateExe(p string, stat func(string) (fs.FileInfo, error)) (clean, reason string, err error) {
	if !filepath.IsAbs(p) {
		return "", ipc.ReasonNotAbsolute, nil
	}
	c := filepath.Clean(p)
	info, err := stat(c)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ipc.ReasonNotFound, nil
		}
		return "", "", fmt.Errorf("validate exe: stat: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", ipc.ReasonNotAFile, nil
	}
	if !strings.EqualFold(filepath.Ext(c), ".exe") {
		return "", ipc.ReasonNotExe, nil
	}
	return c, "", nil
}

// gameTitle returns the title to store [GAM-003]: the given one trimmed, or
// the default title of clean when absent or blank. ok is false when the
// trimmed title has more than ipc.MaxGameTitleRunes runes.
func gameTitle(title *string, clean string) (string, bool) {
	if title == nil {
		return domain.DefaultGameTitle(clean), true
	}
	t := strings.TrimSpace(*title)
	if t == "" {
		return domain.DefaultGameTitle(clean), true
	}
	if utf8.RuneCountInString(t) > ipc.MaxGameTitleRunes {
		return "", false
	}
	return t, true
}

// parseGameID accepts only a JSON integer literal in 1..MaxInt64 [IPC-028]:
// null, strings, fractions, exponents, signs and overflow are rejected.
func parseGameID(raw json.RawMessage) (int64, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s[0] < '1' || s[0] > '9' {
		return 0, false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id < 1 { // err covers overflow past MaxInt64
		return 0, false
	}
	return id, true
}

// toIPCGame converts a store game to its wire form [IPC-020, IPC-042]; zero
// instants and an invalid release date become null.
func toIPCGame(g store.Game) ipc.Game {
	out := ipc.Game{
		ID:               g.ID,
		Title:            g.Title,
		PlatformID:       g.PlatformID,
		Platform:         g.Platform,
		Path:             g.LaunchTarget,
		Status:           string(g.Status),
		Favorite:         g.Favorite,
		TotalPlaySeconds: g.TotalPlaySeconds,
		PlayCount:        g.PlayCount,
	}
	if year, ok := domain.ReleaseYear(g.ReleaseDate); ok {
		out.Year = &year
	}
	if !g.FirstPlayedAt.IsZero() {
		t := g.FirstPlayedAt.UTC()
		out.FirstPlayedAt = &t
	}
	if !g.LastPlayedAt.IsZero() {
		t := g.LastPlayedAt.UTC()
		out.LastPlayedAt = &t
	}
	return out
}
