package main

// Tests for T12 (docs/specs/phase-3-tracker/tasks.md): the game bindings
// (ListGames, LaunchGame, CurrentSession) and the
// GameView/SessionView conversion (DSK-010, DSK-011, DSK-012).
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - The bindings call the agent through App.client (pinger); a nil client is
//     errNoClient, mapped by userError.
//     (App.wailsCtx), like emit; openFile returns "" when the user cancels.
//   - game.add params are ipc.GameAddParams, game.launch params are
//     ipc.GameLaunchParams; game.list and session.current send no params.
//   - Each binding call has a deadline no later than its timeout constant.
//   - SessionView.StartedAt/EndedAt and GameView.LastPlayedAt are RFC 3339 (any
//     offset; compared as instants); AsOf is UTC with exactly 3 fraction digits
//     ("2006-01-02T15:04:05.000Z").

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

const asOfLayout = "2006-01-02T15:04:05.000Z07:00"

var gameT0 = time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)

type gameCall struct {
	method      string
	params      json.RawMessage // JSON of the params (nil when none)
	hasDeadline bool
	remaining   time.Duration
}

// gamePinger answers each method with a fixed result (JSON round-trip, like
// ipc.Client) or a fixed error, and records the calls.
type gamePinger struct {
	results map[string]any
	errs    map[string]error

	mu    sync.Mutex
	calls []gameCall
}

func (p *gamePinger) Call(ctx context.Context, method string, params, result any) error {
	c := gameCall{method: method}
	if dl, ok := ctx.Deadline(); ok {
		c.hasDeadline, c.remaining = true, time.Until(dl)
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		c.params = raw
	}
	p.mu.Lock()
	p.calls = append(p.calls, c)
	p.mu.Unlock()
	if err := p.errs[method]; err != nil {
		return err
	}
	v, ok := p.results[method]
	if !ok {
		return ipc.NewError(ipc.CodeMethodNotFound, "Method not found")
	}
	if result == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, result)
}

func (p *gamePinger) recorded() []gameCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]gameCall(nil), p.calls...)
}

// singleCall checks that exactly one call to method was made within timeout.
func singleCall(t *testing.T, p *gamePinger, method string, timeout time.Duration) gameCall {
	t.Helper()
	calls := p.recorded()
	if len(calls) != 1 || calls[0].method != method {
		t.Fatalf("agent calls = %+v, want one %q", calls, method)
	}
	c := calls[0]
	if !c.hasDeadline || c.remaining <= 0 || c.remaining > timeout {
		t.Errorf("%s deadline = %v (set %v), want within (0, %v]", method, c.remaining, c.hasDeadline, timeout)
	}
	return c
}

func sampleSession() ipc.Session {
	return ipc.Session{
		SessionID:     11,
		GameID:        3,
		Title:         "Celeste",
		State:         string(domain.SessionRunning),
		StartedAt:     gameT0,
		EndedAt:       nil,
		ActiveSeconds: 252,
		ActiveMs:      252400,
		AsOf:          ipc.MilliTime{Time: gameT0.Add(252400 * time.Millisecond)},
	}
}

func wantSessionView(t *testing.T, got SessionView, s ipc.Session) {
	t.Helper()
	if got.SessionID != s.SessionID || got.GameID != s.GameID || got.Title != s.Title || got.State != s.State {
		t.Errorf("SessionView ids/title/state = %d/%d/%q/%q, want %d/%d/%q/%q",
			got.SessionID, got.GameID, got.Title, got.State, s.SessionID, s.GameID, s.Title, s.State)
	}
	if at, err := time.Parse(time.RFC3339, got.StartedAt); err != nil || !at.Equal(s.StartedAt) {
		t.Errorf("StartedAt = %q, want RFC 3339 of %s (err %v)", got.StartedAt, s.StartedAt.Format(time.RFC3339), err)
	}
	if s.EndedAt == nil {
		if got.EndedAt != "" {
			t.Errorf("EndedAt = %q, want empty string", got.EndedAt)
		}
	} else if at, err := time.Parse(time.RFC3339, got.EndedAt); err != nil || !at.Equal(*s.EndedAt) {
		t.Errorf("EndedAt = %q, want RFC 3339 of %s (err %v)", got.EndedAt, s.EndedAt.Format(time.RFC3339), err)
	}
	if got.ActiveSeconds != s.ActiveSeconds || got.ActiveMs != s.ActiveMs {
		t.Errorf("ActiveSeconds/ActiveMs = %d/%d, want %d/%d", got.ActiveSeconds, got.ActiveMs, s.ActiveSeconds, s.ActiveMs)
	}
	if want := s.AsOf.UTC().Format(asOfLayout); got.AsOf != want {
		t.Errorf("AsOf = %q, want %q", got.AsOf, want)
	}
}

type openFileCall struct {
	ctx     context.Context
	title   string
	filters []fileFilter
}

func TestGameBindings(t *testing.T) {
	t.Parallel()

	t.Run("DSK-010/LaunchGame e CurrentSession devolvem SessionView com endedAt vazio, activeMs e asOf", func(t *testing.T) {
		t.Parallel()
		s := sampleSession()
		p := &gamePinger{results: map[string]any{
			ipc.MethodGameLaunch:     s,
			ipc.MethodSessionCurrent: ipc.SessionCurrentResult{Session: &s},
		}}
		a := &App{client: p, log: discardLog()}

		got, err := a.LaunchGame(3)
		if err != nil {
			t.Fatalf("LaunchGame(3) err = %v", err)
		}
		wantSessionView(t, got, s)
		if got.AsOf != "2026-11-09T14:04:12.400Z" {
			t.Errorf("AsOf = %q, want %q", got.AsOf, "2026-11-09T14:04:12.400Z")
		}
		c := singleCall(t, p, ipc.MethodGameLaunch, launchGameTimeout)
		var lp map[string]any
		if err := json.Unmarshal(c.params, &lp); err != nil {
			t.Fatalf("game.launch params %s: %v", c.params, err)
		}
		if len(lp) != 1 || lp["gameId"] != float64(3) {
			t.Errorf("game.launch params = %s, want {\"gameId\":3}", c.params)
		}

		p2 := &gamePinger{results: p.results}
		a.client = p2
		cur, err := a.CurrentSession()
		if err != nil {
			t.Fatalf("CurrentSession() err = %v", err)
		}
		if cur == nil {
			t.Fatalf("CurrentSession() = nil, want the active session")
		}
		wantSessionView(t, *cur, s)
		singleCall(t, p2, ipc.MethodSessionCurrent, currentSessionTimeout)
	})

	t.Run("DSK-010/CurrentSession com session null devolve nil", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodSessionCurrent: ipc.SessionCurrentResult{}}}
		a := &App{client: p, log: discardLog()}

		cur, err := a.CurrentSession()

		if err != nil {
			t.Fatalf("CurrentSession() err = %v", err)
		}
		if cur != nil {
			t.Errorf("CurrentSession() = %+v, want nil", *cur)
		}
	})

	t.Run("DSK-011/ListGames sem cliente devolve O agente não está respondendo.", func(t *testing.T) {
		t.Parallel()
		a := &App{log: discardLog()}

		_, err := a.ListGames(LibraryQuery{})

		if err == nil || err.Error() != txtNotResponding {
			t.Errorf("ListGames() err = %v, want %q", err, txtNotResponding)
		}
		for name, call := range map[string]func() error{
			"LaunchGame":     func() error { _, err := a.LaunchGame(1); return err },
			"CurrentSession": func() error { _, err := a.CurrentSession(); return err },
		} {
			if err := call(); err == nil || err.Error() != txtNotResponding {
				t.Errorf("%s without client err = %v, want %q", name, err, txtNotResponding)
			}
		}
	})

	t.Run("DSK-010/ListGames vazio devolve [] e não nil", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodGameList: ipc.GameListResult{Games: []ipc.Game{}}}}
		a := &App{client: p, log: discardLog()}

		got, err := a.ListGames(LibraryQuery{})

		if err != nil {
			t.Fatalf("ListGames() err = %v", err)
		}
		if got.Games == nil || len(got.Games) != 0 {
			t.Errorf("ListGames() = %#v, want non-nil empty slice", got.Games)
		}
		if raw, _ := json.Marshal(got.Games); string(raw) != "[]" {
			t.Errorf("ListGames() JSON = %s, want []", raw)
		}
		singleCall(t, p, ipc.MethodGameList, listGamesTimeout)

		// A null games array from the agent still yields [].
		p2 := &gamePinger{results: map[string]any{ipc.MethodGameList: json.RawMessage(`{"games":null}`)}}
		a.client = p2
		got, err = a.ListGames(LibraryQuery{})
		if err != nil || got.Games == nil || len(got.Games) != 0 {
			t.Errorf("ListGames() with games:null = %#v, %v; want [] and nil", got.Games, err)
		}
	})

	t.Run("DSK-010/ListGames converte cada jogo na ordem recebida", func(t *testing.T) {
		t.Parallel()
		last := gameT0.Add(time.Hour)
		games := []ipc.Game{
			{ID: 2, Title: "B", Path: `C:\G\b.exe`, TotalPlaySeconds: 3900, PlayCount: 2, FirstPlayedAt: &gameT0, LastPlayedAt: &last},
			{ID: 1, Title: "A", Path: `C:\G\a.exe`},
		}
		p := &gamePinger{results: map[string]any{ipc.MethodGameList: ipc.GameListResult{Games: games}}}
		a := &App{client: p, log: discardLog()}

		got, err := a.ListGames(LibraryQuery{})

		if err != nil {
			t.Fatalf("ListGames() err = %v", err)
		}
		if len(got.Games) != 2 {
			t.Fatalf("ListGames() = %+v, want 2 games", got.Games)
		}
		g0 := got.Games[0]
		if g0.ID != 2 || g0.Title != "B" || g0.Path != `C:\G\b.exe` || g0.TotalPlaySeconds != 3900 || g0.PlayCount != 2 {
			t.Errorf("game[0] = %+v, want id 2 / B / b.exe / 3900 / 2", g0)
		}
		if at, err := time.Parse(time.RFC3339, g0.LastPlayedAt); err != nil || !at.Equal(last) {
			t.Errorf("game[0].LastPlayedAt = %q, want RFC 3339 of %s", g0.LastPlayedAt, last.Format(time.RFC3339))
		}
		if got.Games[1].ID != 1 || got.Games[1].LastPlayedAt != "" {
			t.Errorf("game[1] = %+v, want id 1 with lastPlayedAt \"\"", got.Games[1])
		}
	})

	t.Run("DSK-011/bindings devolvem o texto de userError", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{
			ipc.MethodGameLaunch:     gameNotFoundErr(),
			ipc.MethodGameList:       ipc.ErrConnectionLost,
			ipc.MethodSessionCurrent: errors.New("boom"),
		}}
		a := &App{client: p, log: discardLog()}

		if _, err := a.LaunchGame(3); err == nil || err.Error() != txtGameNotFound {
			t.Errorf("LaunchGame err = %v, want %q", err, txtGameNotFound)
		}
		if _, err := a.ListGames(LibraryQuery{}); err == nil || err.Error() != txtNotResponding {
			t.Errorf("ListGames err = %v, want %q", err, txtNotResponding)
		}
		if _, err := a.CurrentSession(); err == nil || err.Error() != txtUnexpected {
			t.Errorf("CurrentSession err = %v, want %q", err, txtUnexpected)
		}
	})

	t.Run("DSK-010/LaunchGame com state desconhecido é erro inesperado", func(t *testing.T) {
		t.Parallel()
		s := sampleSession()
		s.State = "bogus"
		p := &gamePinger{results: map[string]any{ipc.MethodGameLaunch: s}}
		a := &App{client: p, log: discardLog()}

		if _, err := a.LaunchGame(3); err == nil || err.Error() != txtUnexpected {
			t.Errorf("LaunchGame err = %v, want %q", err, txtUnexpected)
		}
	})
}

func TestSessionView(t *testing.T) {
	t.Parallel()

	t.Run("DSK-010/sessão encerrada tem endedAt RFC 3339 e asOf com milissegundos", func(t *testing.T) {
		t.Parallel()
		s := sampleSession()
		ended := gameT0.Add(10 * time.Minute)
		s.State = string(domain.SessionEnded)
		s.EndedAt = &ended
		s.AsOf = ipc.MilliTime{Time: ended}

		got, err := sessionView(s)

		if err != nil {
			t.Fatalf("sessionView err = %v", err)
		}
		wantSessionView(t, got, s)
		if got.AsOf != "2026-11-09T14:10:00.000Z" {
			t.Errorf("AsOf = %q, want %q (ms even when zero)", got.AsOf, "2026-11-09T14:10:00.000Z")
		}
	})

	t.Run("DSK-010/state desconhecido é erro", func(t *testing.T) {
		t.Parallel()
		for _, st := range []string{"", "Running", "bogus"} {
			s := sampleSession()
			s.State = st
			if _, err := sessionView(s); err == nil {
				t.Errorf("sessionView(state %q) err = nil, want error", st)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// T21 (docs/specs/phase-4-pause/tasks.md): the LaunchGameUntracked, PauseSession
// and ResumeSession bindings (DSK-013, DSK-015).
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - The bindings emit no Wails event: platform.emit must not be called (the
//     session:changed event comes from the agent notification, DSK-014).
//   - LaunchGameUntracked calls game.launchUntracked with {gameId} and a 12 s
//     deadline, and returns only an error. PauseSession and ResumeSession call
//     session.pause and session.resume with a 5 s deadline and return the
//     SessionView of the SessionResult.
//   - Errors pass through userError (callAgent), so the texts are those of
//     DSK-011 and DSK-015.

// gameEmitLog records the names of the Wails events emitted through platform.emit.
type gameEmitLog struct {
	mu     sync.Mutex
	events []string
}

func (e *gameEmitLog) emit(_ context.Context, event string, _ any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

func (e *gameEmitLog) recorded() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.events...)
}

// newBindingApp returns an App with p as the agent client and em as the emitter.
func newBindingApp(p pinger, em *gameEmitLog) *App {
	return &App{
		client:   p,
		log:      discardLog(),
		wailsCtx: context.WithValue(context.Background(), ctxKey{}, "wails"),
		platform: platform{emit: em.emit},
	}
}

func TestLaunchGameUntracked(t *testing.T) {
	t.Parallel()

	t.Run("DSK-013/chama game.launchUntracked com {gameId: id} e prazo de 12 s, devolve sucesso sem valor e não emite session:changed", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodGameLaunchUntracked: ipc.LaunchedProcess{PID: 77}}}
		em := &gameEmitLog{}
		a := newBindingApp(p, em)

		if err := a.LaunchGameUntracked(3); err != nil {
			t.Fatalf("LaunchGameUntracked(3) err = %v, want nil", err)
		}
		c := singleCall(t, p, ipc.MethodGameLaunchUntracked, 12*time.Second)
		var lp map[string]any
		if err := json.Unmarshal(c.params, &lp); err != nil {
			t.Fatalf("game.launchUntracked params %s: %v", c.params, err)
		}
		if len(lp) != 1 || lp["gameId"] != float64(3) {
			t.Errorf("game.launchUntracked params = %s, want {\"gameId\":3}", c.params)
		}
		if got := em.recorded(); len(got) != 0 {
			t.Errorf("events emitted = %v, want none (session:changed comes from the agent)", got)
		}
	})

	t.Run("DSK-013/-32002 vira Já há um jogo em andamento: «title». Feche-o antes de iniciar outro.", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{ipc.MethodGameLaunchUntracked: sessionActiveErr("Hollow Knight")}}
		a := newBindingApp(p, &gameEmitLog{})

		err := a.LaunchGameUntracked(3)

		const want = "Já há um jogo em andamento: Hollow Knight. Feche-o antes de iniciar outro."
		if err == nil || err.Error() != want {
			t.Errorf("LaunchGameUntracked err = %v, want %q", err, want)
		}
	})

	t.Run("DSK-013/-32003 vira Não foi possível iniciar o jogo: «error»", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{ipc.MethodGameLaunchUntracked: launchFailedErr("Acesso negado. (740)")}}
		a := newBindingApp(p, &gameEmitLog{})

		err := a.LaunchGameUntracked(3)

		const want = "Não foi possível iniciar o jogo: Acesso negado. (740)"
		if err == nil || err.Error() != want {
			t.Errorf("LaunchGameUntracked err = %v, want %q", err, want)
		}
	})

	t.Run("DSK-013/-32001 vira Jogo não encontrado. Atualize a lista.", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{ipc.MethodGameLaunchUntracked: gameNotFoundErr()}}
		a := newBindingApp(p, &gameEmitLog{})

		err := a.LaunchGameUntracked(3)

		if err == nil || err.Error() != txtGameNotFound {
			t.Errorf("LaunchGameUntracked err = %v, want %q", err, txtGameNotFound)
		}
	})
}

func TestPauseResumeSession(t *testing.T) {
	t.Parallel()

	t.Run("DSK-015/PauseSession chama session.pause (5 s) e devolve a SessionView; ResumeSession chama session.resume", func(t *testing.T) {
		t.Parallel()
		paused := sampleSession()
		paused.State = string(domain.SessionPaused)
		running := sampleSession()

		pp := &gamePinger{results: map[string]any{ipc.MethodSessionPause: ipc.SessionResult{Session: paused}}}
		a := newBindingApp(pp, &gameEmitLog{})
		got, err := a.PauseSession()
		if err != nil {
			t.Fatalf("PauseSession() err = %v", err)
		}
		wantSessionView(t, got, paused)
		if got.State != "paused" {
			t.Errorf("PauseSession().State = %q, want %q", got.State, "paused")
		}
		singleCall(t, pp, ipc.MethodSessionPause, 5*time.Second)

		rp := &gamePinger{results: map[string]any{ipc.MethodSessionResume: ipc.SessionResult{Session: running}}}
		a.client = rp
		got, err = a.ResumeSession()
		if err != nil {
			t.Fatalf("ResumeSession() err = %v", err)
		}
		wantSessionView(t, got, running)
		if got.State != "running" {
			t.Errorf("ResumeSession().State = %q, want %q", got.State, "running")
		}
		singleCall(t, rp, ipc.MethodSessionResume, 5*time.Second)
	})

	t.Run("DSK-015/os bindings não emitem evento por conta própria", func(t *testing.T) {
		t.Parallel()
		s := sampleSession()
		s.State = string(domain.SessionPaused)
		p := &gamePinger{results: map[string]any{
			ipc.MethodSessionPause:  ipc.SessionResult{Session: s},
			ipc.MethodSessionResume: ipc.SessionResult{Session: s},
		}}
		em := &gameEmitLog{}
		a := newBindingApp(p, em)

		if _, err := a.PauseSession(); err != nil {
			t.Fatalf("PauseSession() err = %v", err)
		}
		if _, err := a.ResumeSession(); err != nil {
			t.Fatalf("ResumeSession() err = %v", err)
		}
		if got := em.recorded(); len(got) != 0 {
			t.Errorf("events emitted = %v, want none (session:changed comes from the agent)", got)
		}
	})

	t.Run("DSK-015/-32005 reason no-session chega à UI como Não há jogo em andamento para pausar.", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{ipc.MethodSessionPause: notPausableErr(ipc.ReasonNoSession)}}
		a := newBindingApp(p, &gameEmitLog{})

		_, err := a.PauseSession()

		if err == nil || err.Error() != txtNothingToPause {
			t.Errorf("PauseSession err = %v, want %q", err, txtNothingToPause)
		}
	})

	t.Run("DSK-015/estado desconhecido na resposta é erro inesperado", func(t *testing.T) {
		t.Parallel()
		s := sampleSession()
		s.State = "bogus"
		p := &gamePinger{results: map[string]any{
			ipc.MethodSessionPause:  ipc.SessionResult{Session: s},
			ipc.MethodSessionResume: ipc.SessionResult{Session: s},
		}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.PauseSession(); err == nil || err.Error() != txtUnexpected {
			t.Errorf("PauseSession err = %v, want %q", err, txtUnexpected)
		}
		if _, err := a.ResumeSession(); err == nil || err.Error() != txtUnexpected {
			t.Errorf("ResumeSession err = %v, want %q", err, txtUnexpected)
		}
	})
}
