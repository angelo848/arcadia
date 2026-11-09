package main

// Tests for T12 (docs/specs/phase-3-tracker/tasks.md): the game bindings
// (PickGameExe, AddGame, ListGames, LaunchGame, CurrentSession) and the
// GameView/SessionView conversion (DSK-010, DSK-011, DSK-012).
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - The bindings call the agent through App.client (pinger); a nil client is
//     errNoClient, mapped by userError.
//   - PickGameExe calls platform.openFile with the Wails ctx given to startup
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
	"log/slog"
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

func TestPickGameExe(t *testing.T) {
	t.Parallel()

	newApp := func(answer string) (*App, *gamePinger, chan openFileCall, context.Context) {
		calls := make(chan openFileCall, 4)
		p := &gamePinger{}
		wctx := context.WithValue(context.Background(), ctxKey{}, "wails")
		a := &App{
			client:   p,
			log:      discardLog(),
			wailsCtx: wctx,
			platform: platform{
				openFile: func(ctx context.Context, title string, filters []fileFilter) (string, error) {
					calls <- openFileCall{ctx, title, filters}
					return answer, nil
				},
			},
		}
		return a, p, calls, wctx
	}

	t.Run("DSK-012/diálogo com título e filtro Executáveis (*.exe) / *.exe", func(t *testing.T) {
		t.Parallel()
		a, _, calls, wctx := newApp("")

		if _, err := a.PickGameExe(); err != nil {
			t.Fatalf("PickGameExe() err = %v", err)
		}
		select {
		case c := <-calls:
			if c.ctx != wctx {
				t.Errorf("openFile got a different context, want the Wails context")
			}
			if c.title != "Escolha o executável do jogo" || pickExeTitle != c.title {
				t.Errorf("dialog title = %q, want %q", c.title, "Escolha o executável do jogo")
			}
			want := []fileFilter{{DisplayName: "Executáveis (*.exe)", Pattern: "*.exe"}}
			if len(c.filters) != 1 || c.filters[0] != want[0] {
				t.Errorf("dialog filters = %+v, want %+v", c.filters, want)
			}
		default:
			t.Fatalf("openFile not called")
		}
		if n := len(calls); n != 0 {
			t.Errorf("openFile called %d more times, want once", n)
		}
	})

	t.Run("DSK-012/cancelar devolve vazio sem chamar o agente", func(t *testing.T) {
		t.Parallel()
		a, p, _, _ := newApp("")

		got, err := a.PickGameExe()

		if err != nil {
			t.Fatalf("PickGameExe() err = %v, want nil on cancel", err)
		}
		if got != (PickedExe{}) {
			t.Errorf("PickGameExe() = %+v, want {path: \"\", title: \"\"}", got)
		}
		if calls := p.recorded(); len(calls) != 0 {
			t.Errorf("agent calls = %+v, want none", calls)
		}
	})

	t.Run("DSK-012/arquivo escolhido devolve path e título padrão", func(t *testing.T) {
		t.Parallel()
		const path = `C:\Games\Celeste\Celeste.EXE`
		a, p, _, _ := newApp(path)

		got, err := a.PickGameExe()

		if err != nil {
			t.Fatalf("PickGameExe() err = %v", err)
		}
		if want := (PickedExe{Path: path, Title: domain.DefaultGameTitle(path)}); got != want {
			t.Errorf("PickGameExe() = %+v, want %+v", got, want)
		}
		if got.Title != "Celeste" {
			t.Errorf("Title = %q, want %q (DOM-009)", got.Title, "Celeste")
		}
		if calls := p.recorded(); len(calls) != 0 {
			t.Errorf("agent calls = %+v, want none (PickGameExe does not add the game)", calls)
		}
	})

	t.Run("DSK-012/falha do diálogo loga Error e devolve Erro inesperado. Veja o log.", func(t *testing.T) {
		t.Parallel()
		logs := newRecLog()
		p := &gamePinger{}
		a := &App{
			client:   p,
			log:      logs.logger(),
			wailsCtx: context.Background(),
			platform: platform{
				openFile: func(context.Context, string, []fileFilter) (string, error) {
					return `C:\ignored.exe`, errors.New("dialog boom")
				},
			},
		}

		got, err := a.PickGameExe()

		if err == nil || err.Error() != txtUnexpected {
			t.Errorf("PickGameExe() err = %v, want %q", err, txtUnexpected)
		}
		if got != (PickedExe{}) {
			t.Errorf("PickGameExe() = %+v, want zero PickedExe on error", got)
		}
		if n := logs.countLevel(slog.LevelError); n != 1 {
			t.Errorf("Error records = %d, want 1; log:\n%s", n, logs.dump())
		}
		if calls := p.recorded(); len(calls) != 0 {
			t.Errorf("agent calls = %+v, want none", calls)
		}
	})
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

		_, err := a.ListGames()

		if err == nil || err.Error() != txtNotResponding {
			t.Errorf("ListGames() err = %v, want %q", err, txtNotResponding)
		}
		for name, call := range map[string]func() error{
			"AddGame":        func() error { _, err := a.AddGame(`C:\G\x.exe`, ""); return err },
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

		got, err := a.ListGames()

		if err != nil {
			t.Fatalf("ListGames() err = %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("ListGames() = %#v, want non-nil empty slice", got)
		}
		if raw, _ := json.Marshal(got); string(raw) != "[]" {
			t.Errorf("ListGames() JSON = %s, want []", raw)
		}
		singleCall(t, p, ipc.MethodGameList, listGamesTimeout)

		// A null games array from the agent still yields [].
		p2 := &gamePinger{results: map[string]any{ipc.MethodGameList: json.RawMessage(`{"games":null}`)}}
		a.client = p2
		got, err = a.ListGames()
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("ListGames() with games:null = %#v, %v; want [] and nil", got, err)
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

		got, err := a.ListGames()

		if err != nil {
			t.Fatalf("ListGames() err = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("ListGames() = %+v, want 2 games", got)
		}
		g0 := got[0]
		if g0.ID != 2 || g0.Title != "B" || g0.Path != `C:\G\b.exe` || g0.TotalPlaySeconds != 3900 || g0.PlayCount != 2 {
			t.Errorf("game[0] = %+v, want id 2 / B / b.exe / 3900 / 2", g0)
		}
		if at, err := time.Parse(time.RFC3339, g0.LastPlayedAt); err != nil || !at.Equal(last) {
			t.Errorf("game[0].LastPlayedAt = %q, want RFC 3339 of %s", g0.LastPlayedAt, last.Format(time.RFC3339))
		}
		if got[1].ID != 1 || got[1].LastPlayedAt != "" {
			t.Errorf("game[1] = %+v, want id 1 with lastPlayedAt \"\"", got[1])
		}
	})

	t.Run("DSK-010/AddGame envia path e title", func(t *testing.T) {
		t.Parallel()
		const path = `C:\Games\Celeste\Celeste.exe`
		added := ipc.Game{ID: 5, Title: "Meu Celeste", Path: path}
		p := &gamePinger{results: map[string]any{ipc.MethodGameAdd: added}}
		a := &App{client: p, log: discardLog()}

		got, err := a.AddGame(path, "Meu Celeste")

		if err != nil {
			t.Fatalf("AddGame err = %v", err)
		}
		want := GameView{ID: 5, Title: "Meu Celeste", Path: path, LastPlayedAt: ""}
		if got != want {
			t.Errorf("AddGame = %+v, want %+v", got, want)
		}
		c := singleCall(t, p, ipc.MethodGameAdd, addGameTimeout)
		var ap ipc.GameAddParams
		if err := json.Unmarshal(c.params, &ap); err != nil {
			t.Fatalf("game.add params %s: %v", c.params, err)
		}
		if ap.Path != path || ap.Title != "Meu Celeste" {
			t.Errorf("game.add params = %s, want path %q and title %q", c.params, path, "Meu Celeste")
		}
	})

	t.Run("DSK-011/bindings devolvem o texto de userError", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{
			ipc.MethodGameLaunch:     gameNotFoundErr(),
			ipc.MethodGameAdd:        invalidExeErr(ipc.ReasonNotExe),
			ipc.MethodGameList:       ipc.ErrConnectionLost,
			ipc.MethodSessionCurrent: errors.New("boom"),
		}}
		a := &App{client: p, log: discardLog()}

		if _, err := a.LaunchGame(3); err == nil || err.Error() != txtGameNotFound {
			t.Errorf("LaunchGame err = %v, want %q", err, txtGameNotFound)
		}
		if _, err := a.AddGame(`C:\G\x.txt`, ""); err == nil || err.Error() != txtNotExe {
			t.Errorf("AddGame err = %v, want %q", err, txtNotExe)
		}
		if _, err := a.ListGames(); err == nil || err.Error() != txtNotResponding {
			t.Errorf("ListGames err = %v, want %q", err, txtNotResponding)
		}
		if _, err := a.CurrentSession(); err == nil || err.Error() != txtUnexpected {
			t.Errorf("CurrentSession err = %v, want %q", err, txtUnexpected)
		}
	})

	t.Run("DSK-010/LaunchGame com state desconhecido é erro inesperado", func(t *testing.T) {
		t.Parallel()
		s := sampleSession()
		s.State = "paused"
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
		for _, st := range []string{"", "Running", "paused"} {
			s := sampleSession()
			s.State = st
			if _, err := sessionView(s); err == nil {
				t.Errorf("sessionView(state %q) err = nil, want error", st)
			}
		}
	})
}
