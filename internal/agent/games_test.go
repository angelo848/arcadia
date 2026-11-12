package agent

// Tests for T10 (docs/specs/phase-3-tracker/tasks.md): game.add, game.list,
// game.launch and session.current handlers, called through ipc.Dispatch with a
// real store in t.TempDir(), a fake session tracker and an injectable stat.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// gamesT0 is the fake clock start of every game handler test.
var gamesT0 = time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)

// recordingGameStore wraps a real store, records the calls and lets a test
// override Game and ListGames.
type recordingGameStore struct {
	inner *store.Store

	mu      sync.Mutex
	added   []store.NewGame
	gameIDs []int64
	queries []store.Query
	listDLs []time.Time // ListGames ctx deadlines, zero = none
	gameFn  func(id int64) (store.Game, error)
	listFn  func(ctx context.Context, q store.Query) (store.Page, error)
}

func (s *recordingGameStore) AddGame(ctx context.Context, g store.NewGame) (store.Game, error) {
	s.mu.Lock()
	s.added = append(s.added, g)
	s.mu.Unlock()
	return s.inner.AddGame(ctx, g)
}

// ListGames records the query and its ctx deadline, then runs listFn or the
// real store [IPC-042].
func (s *recordingGameStore) ListGames(ctx context.Context, q store.Query) (store.Page, error) {
	s.mu.Lock()
	s.queries = append(s.queries, q)
	dl, _ := ctx.Deadline()
	s.listDLs = append(s.listDLs, dl)
	fn := s.listFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, q)
	}
	return s.inner.ListGames(ctx, q)
}

// listQueries returns the queries ListGames received, in order.
func (s *recordingGameStore) listQueries() []store.Query {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.Query(nil), s.queries...)
}

// listDeadlines returns the ctx deadline of each ListGames call, in order.
func (s *recordingGameStore) listDeadlines() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.listDLs...)
}

// Games is the pre-T18 listing. It is kept only so the package compiles until
// Store.Games and gameStore.Games leave in T18; then delete this method.
func (s *recordingGameStore) Games(ctx context.Context) ([]store.Game, error) {
	return s.inner.Games(ctx)
}

func (s *recordingGameStore) Game(ctx context.Context, id int64) (store.Game, error) {
	s.mu.Lock()
	s.gameIDs = append(s.gameIDs, id)
	fn := s.gameFn
	s.mu.Unlock()
	if fn != nil {
		return fn(id)
	}
	return s.inner.Game(ctx, id)
}

func (s *recordingGameStore) addedGames() []store.NewGame {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]store.NewGame(nil), s.added...)
}

func (s *recordingGameStore) gameCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.gameIDs)
}

// DeleteGame forwards to the wrapped store [DEL-001].
func (s *recordingGameStore) DeleteGame(ctx context.Context, id int64) error {
	return s.inner.DeleteGame(ctx, id)
}

// fakeTracker is a programmable sessionTracker.
type fakeTracker struct {
	mu         sync.Mutex
	specs      []tracker.LaunchSpec
	deadlines  []time.Time // zero = the Launch ctx had no deadline
	launchFn   func(ctx context.Context, spec tracker.LaunchSpec) (domain.SessionSnapshot, error)
	current    domain.SessionSnapshot
	hasCurrent bool
	curCalls   int

	// T18: LaunchUntracked, Pause and Resume.
	untrackedFn        func(ctx context.Context, spec tracker.LaunchSpec) (uint32, error)
	untrackedSpecs     []tracker.LaunchSpec
	untrackedDeadlines []time.Time
	pauseFn            func(ctx context.Context, at time.Time) (domain.SessionSnapshot, error)
	resumeFn           func(ctx context.Context, at time.Time) (domain.SessionSnapshot, error)
	pauseAts           []time.Time
	resumeAts          []time.Time
	pauseDeadlines     []time.Time
	resumeDeadlines    []time.Time
}

func (f *fakeTracker) LaunchUntracked(ctx context.Context, spec tracker.LaunchSpec) (uint32, error) {
	f.mu.Lock()
	f.untrackedSpecs = append(f.untrackedSpecs, spec)
	dl, _ := ctx.Deadline()
	f.untrackedDeadlines = append(f.untrackedDeadlines, dl)
	fn := f.untrackedFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, spec)
	}
	return 4242, nil
}

func (f *fakeTracker) Pause(ctx context.Context, at time.Time) (domain.SessionSnapshot, error) {
	f.mu.Lock()
	f.pauseAts = append(f.pauseAts, at)
	dl, _ := ctx.Deadline()
	f.pauseDeadlines = append(f.pauseDeadlines, dl)
	fn := f.pauseFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, at)
	}
	return pausedAt(at), nil
}

func (f *fakeTracker) Resume(ctx context.Context, at time.Time) (domain.SessionSnapshot, error) {
	f.mu.Lock()
	f.resumeAts = append(f.resumeAts, at)
	dl, _ := ctx.Deadline()
	f.resumeDeadlines = append(f.resumeDeadlines, dl)
	fn := f.resumeFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, at)
	}
	return runningAt(at), nil
}

func (f *fakeTracker) untrackedLaunches() []tracker.LaunchSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]tracker.LaunchSpec(nil), f.untrackedSpecs...)
}

// pausedAt is a session started at gamesT0 and paused at "at" (100 s active
// when at = gamesT0+100s).
func pausedAt(at time.Time) domain.SessionSnapshot {
	return domain.SessionSnapshot{
		ID: 3, GameID: 1, Title: "G", State: domain.SessionPaused,
		StartedAt: gamesT0, Active: at.Sub(gamesT0), AsOf: at,
	}
}

func runningAt(at time.Time) domain.SessionSnapshot {
	return domain.SessionSnapshot{
		ID: 3, GameID: 1, Title: "G", State: domain.SessionRunning,
		StartedAt: gamesT0, Active: 100 * time.Second, AsOf: at,
	}
}

func (f *fakeTracker) Launch(ctx context.Context, spec tracker.LaunchSpec) (domain.SessionSnapshot, error) {
	f.mu.Lock()
	f.specs = append(f.specs, spec)
	dl, _ := ctx.Deadline()
	f.deadlines = append(f.deadlines, dl)
	fn := f.launchFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, spec)
	}
	return domain.SessionSnapshot{
		ID: 1, GameID: spec.GameID, Title: spec.Title, State: domain.SessionRunning,
		StartedAt: gamesT0, AsOf: gamesT0,
	}, nil
}

func (f *fakeTracker) Current() (domain.SessionSnapshot, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.curCalls++
	return f.current, f.hasCurrent
}

// WithoutActiveGame refuses a game whose session is the programmed current one
// with *tracker.SessionActiveError, without calling fn; otherwise it runs fn and
// returns its error [DEL-002, DEL-003]. deleteTracker overrides it for the
// delete tests.
func (f *fakeTracker) WithoutActiveGame(ctx context.Context, gameID int64, fn func() error) error {
	f.mu.Lock()
	busy := f.hasCurrent && f.current.GameID == gameID
	snap := f.current
	f.mu.Unlock()
	if busy {
		return &tracker.SessionActiveError{Session: snap}
	}
	return fn()
}

func (f *fakeTracker) launches() []tracker.LaunchSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]tracker.LaunchSpec(nil), f.specs...)
}

type gamesHarness struct {
	dir      string
	st       *store.Store
	gs       *recordingGameStore
	tr       *fakeTracker
	clock    *domain.FakeClock
	logs     *recordHandler
	handlers map[string]ipc.Handler
}

// newGamesHarness opens a migrated store in t.TempDir() and builds the
// handlers. stat nil means os.Stat.
func newGamesHarness(t *testing.T, stat func(string) (fs.FileInfo, error)) *gamesHarness {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(t.Context(), filepath.Join(dir, "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if stat == nil {
		stat = os.Stat
	}
	h := &gamesHarness{
		dir:   filepath.Join(dir, "games"),
		st:    st,
		gs:    &recordingGameStore{inner: st},
		tr:    &fakeTracker{},
		clock: domain.NewFakeClock(gamesT0),
		logs:  newRecordHandler(),
	}
	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	h.handlers = gameHandlers(h.gs, h.tr, h.clock, stat, slog.New(h.logs))
	for _, m := range []string{ipc.MethodGameAdd, ipc.MethodGameList, ipc.MethodGameLaunch, ipc.MethodSessionCurrent} {
		if h.handlers[m] == nil {
			t.Fatalf("gameHandlers has no handler for %q", m)
		}
	}
	return h
}

// file creates name (relative to h.dir) with some content and returns its path.
func (h *gamesHarness) file(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(h.dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("MZ"), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// call dispatches method with the raw params JSON ("" = no params member).
func (h *gamesHarness) call(t *testing.T, method, params string) ipc.Response {
	t.Helper()
	return callHandlers(t, t.Context(), h.handlers, method, params)
}

func callHandlers(t *testing.T, ctx context.Context, handlers map[string]ipc.Handler, method, params string) ipc.Response {
	t.Helper()
	msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q`, method)
	if params != "" {
		msg += `,"params":` + params
	}
	msg += "}"
	raw, ok := ipc.Dispatch(ctx, handlers, []byte(msg))
	if !ok {
		t.Fatalf("Dispatch(%s) returned no response", msg)
	}
	var resp ipc.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode response %s: %v", raw, err)
	}
	return resp
}

// addGame calls game.add and returns the decoded Game.
func (h *gamesHarness) addGame(t *testing.T, path, title string) ipc.Game {
	t.Helper()
	params := map[string]string{"path": path}
	if title != "" {
		params["title"] = title
	}
	resp := h.call(t, ipc.MethodGameAdd, mustJSON(t, params))
	var g ipc.Game
	resultInto(t, resp, &g)
	return g
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func resultInto(t *testing.T, resp ipc.Response, v any) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected error %d %q (data %s)", resp.Error.Code, resp.Error.Message, resp.Error.Data)
	}
	if err := json.Unmarshal(resp.Result, v); err != nil {
		t.Fatalf("decode result %s: %v", resp.Result, err)
	}
}

// wantError checks the error code (and message when not empty) and returns the
// data members (nil when there is no data).
func wantError(t *testing.T, resp ipc.Response, code int, message string) map[string]json.RawMessage {
	t.Helper()
	if resp.Error == nil {
		t.Fatalf("result %s, want error %d", resp.Result, code)
	}
	if resp.Error.Code != code {
		t.Fatalf("error code = %d (%q, data %s), want %d", resp.Error.Code, resp.Error.Message, resp.Error.Data, code)
	}
	if message != "" && resp.Error.Message != message {
		t.Errorf("error message = %q, want %q", resp.Error.Message, message)
	}
	if len(resp.Error.Data) == 0 {
		return nil
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(resp.Error.Data, &data); err != nil {
		t.Fatalf("error data %s is not an object: %v", resp.Error.Data, err)
	}
	return data
}

func objectOf(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("%s is not an object: %v", raw, err)
	}
	return obj
}

// wantKeys checks that obj has exactly the given members.
func wantKeys(t *testing.T, obj map[string]json.RawMessage, keys ...string) {
	t.Helper()
	got := make([]string, 0, len(obj))
	for k := range obj {
		got = append(got, k)
	}
	sort.Strings(got)
	want := append([]string(nil), keys...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("members = %v, want %v", got, want)
	}
}

func wantRaw(t *testing.T, obj map[string]json.RawMessage, key, want string) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Errorf("member %q missing, want %s", key, want)
		return
	}
	if string(got) != want {
		t.Errorf("%s = %s, want %s", key, got, want)
	}
}

// gameKeys are the members of Game in v0.6.0 (IPC-020, IPC-042).
var gameKeys = []string{"id", "title", "platformId", "platform", "path", "status", "favorite", "year",
	"totalPlaySeconds", "playCount", "firstPlayedAt", "lastPlayedAt"}

var sessionKeys = []string{"sessionId", "gameId", "title", "state", "startedAt", "endedAt", "activeSeconds", "activeMs", "asOf"}

func TestGameAdd(t *testing.T) {
	t.Parallel()

	t.Run("IPC-020/devolve o Game com totais 0 e datas null", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		p := h.file(t, "Hollow Knight/hollow_knight.exe")
		resp := h.call(t, ipc.MethodGameAdd, mustJSON(t, map[string]string{"path": p, "title": "Hollow Knight"}))
		if resp.Error != nil {
			t.Fatalf("error %d %q", resp.Error.Code, resp.Error.Message)
		}
		obj := objectOf(t, resp.Result)
		wantKeys(t, obj, gameKeys...)
		wantRaw(t, obj, "totalPlaySeconds", "0")
		wantRaw(t, obj, "playCount", "0")
		wantRaw(t, obj, "firstPlayedAt", "null")
		wantRaw(t, obj, "lastPlayedAt", "null")
		wantRaw(t, obj, "year", "null")
		wantRaw(t, obj, "status", `"not_played"`)
		var g ipc.Game
		resultInto(t, resp, &g)
		if g.ID < 1 {
			t.Errorf("id = %d, want >= 1", g.ID)
		}
		if g.Title != "Hollow Knight" {
			t.Errorf("title = %q, want %q", g.Title, "Hollow Knight")
		}
		if g.Path != filepath.Clean(p) {
			t.Errorf("path = %q, want %q", g.Path, filepath.Clean(p))
		}
		stored, err := h.st.Game(t.Context(), g.ID)
		if err != nil {
			t.Fatalf("store.Game(%d): %v", g.ID, err)
		}
		if stored.Title != g.Title || stored.LaunchTarget != g.Path {
			t.Errorf("stored game = %+v, want title %q target %q", stored, g.Title, g.Path)
		}
	})

	t.Run("GAM-001/AddGame recebe exe limpo, pasta do exe e At do relógio", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		clean := h.file(t, "game.exe")
		h.clock.Advance(1500 * time.Millisecond)
		dirty := h.dir + string(filepath.Separator) + "." + string(filepath.Separator) + "game.exe"
		g := h.addGame(t, dirty, "X")
		added := h.gs.addedGames()
		if len(added) != 1 {
			t.Fatalf("AddGame calls = %d, want 1", len(added))
		}
		ng := added[0]
		if ng.Exe != clean {
			t.Errorf("NewGame.Exe = %q, want %q", ng.Exe, clean)
		}
		if ng.WorkingDir != filepath.Dir(clean) {
			t.Errorf("NewGame.WorkingDir = %q, want %q", ng.WorkingDir, filepath.Dir(clean))
		}
		if !ng.At.Equal(gamesT0.Add(1500 * time.Millisecond)) {
			t.Errorf("NewGame.At = %v, want %v", ng.At, gamesT0.Add(1500*time.Millisecond))
		}
		if ng.Title != "X" {
			t.Errorf("NewGame.Title = %q, want X", ng.Title)
		}
		if g.Path != clean {
			t.Errorf("path = %q, want %q", g.Path, clean)
		}
	})

	t.Run("GAM-001/o mesmo caminho duas vezes é aceito", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		p := h.file(t, "game.exe")
		first := h.addGame(t, p, "A")
		second := h.addGame(t, p, "B")
		if first.ID == second.ID {
			t.Fatalf("both additions have id %d, want distinct ids", first.ID)
		}
		var list ipc.GameListResult
		resultInto(t, h.call(t, ipc.MethodGameList, ""), &list)
		if len(list.Games) != 2 {
			t.Fatalf("game.list has %d games, want 2", len(list.Games))
		}
		for _, g := range list.Games {
			if g.Path != filepath.Clean(p) {
				t.Errorf("game %d path = %q, want %q", g.ID, g.Path, p)
			}
		}
	})

	invalid := []struct {
		name   string
		path   func(h *gamesHarness, t *testing.T) string
		reason string
	}{
		{"IPC-021/relativo devolve -32004 not-absolute", func(*gamesHarness, *testing.T) string {
			return filepath.Join("games", "game.exe")
		}, ipc.ReasonNotAbsolute},
		{"IPC-021/inexistente devolve -32004 not-found", func(h *gamesHarness, _ *testing.T) string {
			// .txt too: stat comes before the extension check.
			return filepath.Join(h.dir, "missing.txt")
		}, ipc.ReasonNotFound},
		{"IPC-021/pasta devolve -32004 not-a-file", func(h *gamesHarness, t *testing.T) string {
			// Named .exe: IsRegular comes before the extension check.
			p := filepath.Join(h.dir, "Folder.exe")
			if err := os.Mkdir(p, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			return p
		}, ipc.ReasonNotAFile},
		{"IPC-021/.txt devolve -32004 not-exe", func(h *gamesHarness, t *testing.T) string {
			return h.file(t, "readme.txt")
		}, ipc.ReasonNotExe},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			resp := h.call(t, ipc.MethodGameAdd, mustJSON(t, map[string]string{"path": tc.path(h, t)}))
			data := wantError(t, resp, ipc.CodeInvalidExecutable, ipc.MsgInvalidExecutable)
			wantKeys(t, data, "reason")
			wantRaw(t, data, "reason", mustJSON(t, tc.reason))
		})
	}

	t.Run("GAM-002/.EXE maiúsculo é aceito", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		p := h.file(t, "Game.EXE")
		g := h.addGame(t, p, "")
		if g.Path != p {
			t.Errorf("path = %q, want %q", g.Path, p)
		}
		if g.Title != "Game" {
			t.Errorf("default title = %q, want Game", g.Title)
		}
	})

	t.Run("GAM-002/inválido não chama AddGame", func(t *testing.T) {
		t.Parallel()
		for _, tc := range invalid {
			h := newGamesHarness(t, nil)
			resp := h.call(t, ipc.MethodGameAdd, mustJSON(t, map[string]string{"path": tc.path(h, t)}))
			wantError(t, resp, ipc.CodeInvalidExecutable, "")
			if n := len(h.gs.addedGames()); n != 0 {
				t.Errorf("%s: AddGame calls = %d, want 0", tc.reason, n)
			}
			page, err := h.st.ListGames(t.Context(), store.Query{Filter: domain.FilterSpec{Version: 1}, Sort: domain.SortTitle, Limit: 200})
			if err != nil {
				t.Fatalf("store.ListGames: %v", err)
			}
			if len(page.Games) != 0 {
				t.Errorf("%s: %d games stored, want 0", tc.reason, len(page.Games))
			}
		}
	})

	t.Run("GAM-002/stat com erro diferente de inexistente devolve -32603 e loga Error", func(t *testing.T) {
		t.Parallel()
		denied := func(string) (fs.FileInfo, error) {
			return nil, &fs.PathError{Op: "stat", Path: "x", Err: fs.ErrPermission}
		}
		h := newGamesHarness(t, denied)
		p := filepath.Join(h.dir, "locked.exe")
		resp := h.call(t, ipc.MethodGameAdd, mustJSON(t, map[string]string{"path": p}))
		wantError(t, resp, ipc.CodeInternalError, "Internal error")
		if n := len(h.gs.addedGames()); n != 0 {
			t.Errorf("AddGame calls = %d, want 0", n)
		}
		if h.logs.count(slog.LevelError) == 0 {
			t.Errorf("no Error record logged for the stat failure")
		}
	})

	titles := []struct {
		name  string
		title *string // nil = member absent
		want  string  // "" = default title
	}{
		{"GAM-003/título com espaços é cortado", ptr("  Hollow Knight \t"), "Hollow Knight"},
		{"GAM-003/ausente ou só espaços usa o título padrão", nil, ""},
		{"GAM-003/ausente ou só espaços usa o título padrão", ptr("   "), ""},
		{"GAM-003/ausente ou só espaços usa o título padrão", ptr(""), ""},
		{"GAM-003/200 runas aceito", ptr(" " + strings.Repeat("é", ipc.MaxGameTitleRunes) + " "), strings.Repeat("é", ipc.MaxGameTitleRunes)},
	}
	for _, tc := range titles {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			p := h.file(t, "my.game.exe")
			params := map[string]any{"path": p}
			if tc.title != nil {
				params["title"] = *tc.title
			}
			var g ipc.Game
			resultInto(t, h.call(t, ipc.MethodGameAdd, mustJSON(t, params)), &g)
			want := tc.want
			if want == "" {
				want = domain.DefaultGameTitle(p)
			}
			if g.Title != want {
				t.Errorf("title = %q, want %q", g.Title, want)
			}
			stored, err := h.st.Game(t.Context(), g.ID)
			if err != nil {
				t.Fatalf("store.Game: %v", err)
			}
			if stored.Title != want {
				t.Errorf("stored title = %q, want %q", stored.Title, want)
			}
		})
	}

	t.Run("GAM-003/201 runas devolve -32602 sem gravar", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		p := h.file(t, "game.exe")
		title := "  " + strings.Repeat("é", ipc.MaxGameTitleRunes+1) + "  "
		resp := h.call(t, ipc.MethodGameAdd, mustJSON(t, map[string]string{"path": p, "title": title}))
		wantError(t, resp, ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		if n := len(h.gs.addedGames()); n != 0 {
			t.Errorf("AddGame calls = %d, want 0", n)
		}
	})
}

func ptr[T any](v T) *T { return &v }

func TestValidateExe(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := filepath.Join(dir, "game.exe")
	txt := filepath.Join(dir, "notes.txt")
	for _, p := range []string{exe, txt} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name       string
		path       string
		wantClean  string
		wantReason string
	}{
		{"GAM-002/válido devolve o caminho limpo", dir + string(filepath.Separator) + "." + string(filepath.Separator) + "game.exe", exe, ""},
		{"GAM-002/relativo é not-absolute", "game.exe", "", ipc.ReasonNotAbsolute},
		{"GAM-002/inexistente é not-found", filepath.Join(dir, "nope.exe"), "", ipc.ReasonNotFound},
		{"GAM-002/pasta é not-a-file", dir, "", ipc.ReasonNotAFile},
		{"GAM-002/extensão errada é not-exe", txt, "", ipc.ReasonNotExe},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clean, reason, err := validateExe(tc.path, os.Stat)
			if err != nil {
				t.Fatalf("validateExe(%q) err = %v", tc.path, err)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
			if tc.wantReason == "" && clean != tc.wantClean {
				t.Errorf("clean = %q, want %q", clean, tc.wantClean)
			}
		})
	}
	t.Run("GAM-002/erro de stat diferente de inexistente devolve err", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("access denied")
		_, _, err := validateExe(exe, func(string) (fs.FileInfo, error) { return nil, boom })
		if !errors.Is(err, boom) {
			t.Errorf("err = %v, want wrapping %v", err, boom)
		}
	})
}

func TestGameTitle(t *testing.T) {
	t.Parallel()
	clean := filepath.Join(t.TempDir(), "my.game.exe")
	cases := []struct {
		name   string
		title  *string
		want   string
		wantOK bool
	}{
		{"GAM-003/título com espaços é cortado", ptr("  X  "), "X", true},
		{"GAM-003/ausente ou só espaços usa o título padrão", nil, "my.game", true},
		{"GAM-003/ausente ou só espaços usa o título padrão", ptr(" \t "), "my.game", true},
		{"GAM-003/200 runas aceito", ptr(strings.Repeat("日", 200)), strings.Repeat("日", 200), true},
		{"GAM-003/201 runas devolve -32602 sem gravar", ptr(strings.Repeat("日", 201)), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := gameTitle(tc.title, clean)
			if ok != tc.wantOK {
				t.Fatalf("gameTitle ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("gameTitle = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGameList(t *testing.T) {
	t.Parallel()

	t.Run("IPC-023/games na ordem do store", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		p := h.file(t, "game.exe")
		for _, title := range []string{"b", "A", "a", "C"} {
			h.addGame(t, p, title)
		}
		resp := h.call(t, ipc.MethodGameList, "")
		obj := objectOf(t, resp.Result)
		wantKeys(t, obj, "games", "total", "offset")
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(obj["games"], &items); err != nil {
			t.Fatalf("games: %v", err)
		}
		for _, it := range items {
			wantKeys(t, it, gameKeys...)
		}
		var list ipc.GameListResult
		resultInto(t, resp, &list)
		var got []string
		for _, g := range list.Games {
			got = append(got, fmt.Sprintf("%s(%d)", g.Title, g.ID))
		}
		if want := "A(2),a(3),b(1),C(4)"; strings.Join(got, ",") != want {
			t.Errorf("order = %s, want %s", strings.Join(got, ","), want)
		}
	})

	t.Run("IPC-023/vazio sai como [] e não null", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		wantRaw(t, objectOf(t, h.call(t, ipc.MethodGameList, "").Result), "games", "[]")

		h.gs.listFn = func(context.Context, store.Query) (store.Page, error) { return store.Page{}, nil }
		wantRaw(t, objectOf(t, h.call(t, ipc.MethodGameList, "").Result), "games", "[]")
	})

	t.Run("GAM-005/a ordem do store é preservada", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.gs.listFn = func(context.Context, store.Query) (store.Page, error) {
			return store.Page{Games: []store.Game{
				{ID: 3, Title: "z", LaunchType: "exe", LaunchTarget: `C:\z.exe`},
				{ID: 1, Title: "a", LaunchType: "exe", LaunchTarget: `C:\a.exe`},
				{ID: 2, Title: "m", LaunchType: "exe", LaunchTarget: `C:\m.exe`},
			}, Total: 3}, nil
		}
		var list ipc.GameListResult
		resultInto(t, h.call(t, ipc.MethodGameList, ""), &list)
		var ids []int64
		for _, g := range list.Games {
			ids = append(ids, g.ID)
		}
		if fmt.Sprint(ids) != "[3 1 2]" {
			t.Errorf("ids = %v, want [3 1 2]", ids)
		}
	})

	t.Run("IPC-042/sem params devolve primeira página de 200 com filtro vazio e ordem title", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		obj := objectOf(t, h.call(t, ipc.MethodGameList, "").Result)
		wantKeys(t, obj, "games", "total", "offset")
		wantRaw(t, obj, "offset", "0")
		qs := h.gs.listQueries()
		if len(qs) != 1 {
			t.Fatalf("ListGames calls = %d, want 1", len(qs))
		}
		want := store.Query{
			Filter: domain.FilterSpec{Version: 1, PlayMode: domain.PlayGroupAll, Played: "any"},
			Sort:   domain.SortTitle,
			Offset: 0,
			Limit:  200,
		}
		if !reflect.DeepEqual(qs[0], want) {
			t.Errorf("query = %+v, want %+v", qs[0], want)
		}
	})

	t.Run("IPC-042/{} é igual a sem params", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		a := h.call(t, ipc.MethodGameList, "")
		b := h.call(t, ipc.MethodGameList, "{}")
		if string(a.Result) != string(b.Result) {
			t.Errorf("result for {} = %s, want %s", b.Result, a.Result)
		}
		qs := h.gs.listQueries()
		if len(qs) != 2 || !reflect.DeepEqual(qs[0], qs[1]) {
			t.Errorf("queries = %+v, want two equal queries", qs)
		}
	})

	t.Run("IPC-042/filter, sort, offset e limit chegam ao store e o resultado traz games, total e offset", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.gs.listFn = func(context.Context, store.Query) (store.Page, error) {
			return store.Page{Games: []store.Game{{ID: 9, Title: "Hollow", LaunchType: "exe", LaunchTarget: `C:\h.exe`,
				PlatformID: 1, Platform: "PC", Status: domain.StatusPlaying}}, Total: 777}, nil
		}
		params := `{"filter":{"version":1,"platformIds":[2,1,2],"statuses":["playing","backlog"],"text":"  Hollow  ",` +
			`"playMode":"single","played":"played","yearFrom":2000,"yearTo":2010,"favorite":true},` +
			`"sort":"lastPlayed","offset":200,"limit":50}`
		var list ipc.GameListResult
		resultInto(t, h.call(t, ipc.MethodGameList, params), &list)
		if list.Total != 777 || list.Offset != 200 || len(list.Games) != 1 || list.Games[0].ID != 9 {
			t.Errorf("result = %+v, want total 777, offset 200, game 9", list)
		}
		qs := h.gs.listQueries()
		if len(qs) != 1 {
			t.Fatalf("ListGames calls = %d, want 1", len(qs))
		}
		want := store.Query{
			Filter: domain.FilterSpec{
				Version:     1,
				PlatformIDs: []int64{1, 2},
				Statuses:    []domain.GameStatus{domain.StatusBacklog, domain.StatusPlaying},
				Text:        "Hollow",
				PlayMode:    domain.PlayGroupSingle,
				Played:      "played",
				YearFrom:    ptr(2000),
				YearTo:      ptr(2010),
				Favorite:    ptr(true),
			},
			Sort:   domain.SortLastPlayed,
			Offset: 200,
			Limit:  50,
		}
		if !reflect.DeepEqual(qs[0], want) {
			t.Errorf("query = %+v, want %+v", qs[0], want)
		}
	})

	t.Run("IPC-042/ListGames tem prazo de 2 s", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.call(t, ipc.MethodGameList, "")
		dls := h.gs.listDeadlines()
		if len(dls) != 1 || dls[0].IsZero() {
			t.Fatalf("ListGames ctx deadlines = %v, want one deadline about 2 s away", dls)
		}
		if left := time.Until(dls[0]); left > 2*time.Second || left < time.Second {
			t.Errorf("ListGames ctx deadline in %v, want about 2 s", left)
		}
	})

	t.Run("IPC-042/games vazio sai como [] e não null (LIB-003)", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		obj := objectOf(t, h.call(t, ipc.MethodGameList, "").Result)
		wantRaw(t, obj, "games", "[]")
		wantRaw(t, obj, "total", "0")

		h.addGame(t, h.file(t, "game.exe"), "Game")
		obj = objectOf(t, h.call(t, ipc.MethodGameList, `{"filter":{"version":1,"platformIds":[999]}}`).Result)
		wantRaw(t, obj, "games", "[]")
		wantRaw(t, obj, "total", "0")
		wantRaw(t, obj, "offset", "0")
	})

	t.Run("IPC-043/limit 1 e 500, offset 0 e sort year são aceitos", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		for _, p := range []string{`{"limit":1}`, `{"limit":500}`, `{"offset":0,"sort":"year"}`} {
			if resp := h.call(t, ipc.MethodGameList, p); resp.Error != nil {
				t.Errorf("game.list %s: error %d %q, want a result", p, resp.Error.Code, resp.Error.Message)
			}
		}
		qs := h.gs.listQueries()
		if len(qs) != 3 {
			t.Fatalf("ListGames calls = %d, want 3", len(qs))
		}
		if qs[0].Limit != 1 || qs[1].Limit != 500 || qs[2].Offset != 0 || qs[2].Sort != domain.SortYear {
			t.Errorf("queries = %+v, want limits 1 and 500, then offset 0 and sort year", qs)
		}
	})

	// IPC-043 and IPC-052: invalid game.list params are -32602 and the store is
	// not queried. IPC-008: [] is a positional array here, not an empty params.
	for _, tc := range []struct{ name, params string }{
		{"IPC-043/sort desconhecido é -32602 sem consultar o store", `{"sort":"nome"}`},
		{"IPC-043/sort vazio é -32602 sem consultar o store", `{"sort":""}`},
		{"IPC-043/limit 0 é -32602 sem consultar o store", `{"limit":0}`},
		{"IPC-043/limit 501 é -32602 sem consultar o store", `{"limit":501}`},
		{"IPC-043/limit negativo é -32602 sem consultar o store", `{"limit":-1}`},
		{"IPC-043/limit texto é -32602 sem consultar o store", `{"limit":"10"}`},
		{"IPC-043/offset -1 é -32602 sem consultar o store", `{"offset":-1}`},
		{"IPC-043/offset texto é -32602 sem consultar o store", `{"offset":"2"}`},
		{"IPC-043/offset fracionário é -32602 sem consultar o store", `{"offset":1.5}`},
		{"IPC-043/filter com version 2 é -32602 sem consultar o store", `{"filter":{"version":2}}`},
		{"IPC-043/filter sem version é -32602 sem consultar o store", `{"filter":{}}`},
		{"IPC-043/filter com membro desconhecido é -32602 sem consultar o store", `{"filter":{"version":1,"nope":1}}`},
		{"IPC-043/statuses fora do conjunto é -32602 sem consultar o store", `{"filter":{"version":1,"statuses":["nope"]}}`},
		{"IPC-043/array posicional é -32602 sem consultar o store", `[1]`},
		{"IPC-043/membro desconhecido no topo é -32602 sem consultar o store", `{"a":1}`},
		{"IPC-008/game.list com array vazio [] é -32602 (params posicionais)", `[]`},
		{"IPC-052/membro com caixa diferente (Limit) é -32602 sem consultar o store", `{"Limit":10}`},
		{"IPC-052/platformIds com texto dentro de filter é -32602 sem consultar o store", `{"filter":{"version":1,"platformIds":["1"]}}`},
		{"IPC-052/statuses com item numérico é -32602 sem consultar o store", `{"filter":{"version":1,"statuses":[1]}}`},
		{"IPC-052/favorite texto dentro de filter é -32602 sem consultar o store", `{"filter":{"version":1,"favorite":"sim"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			wantError(t, h.call(t, ipc.MethodGameList, tc.params), ipc.CodeInvalidParams, ipc.MsgInvalidParams)
			if n := len(h.gs.listQueries()); n != 0 {
				t.Errorf("ListGames calls = %d, want 0", n)
			}
		})
	}

	t.Run("IPC-051/página de 500 jogos de pior caso (título de 200 runas, caminho de 260, plataforma de 100) cabe em 1 048 576 bytes", func(t *testing.T) {
		t.Parallel()
		t.Run("sem escape a página tem os 500 jogos dentro do limite", func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			h.gs.listFn = pagedStore(worstGames(500, strings.Repeat("日", 200), `C:\`+strings.Repeat("a", 257), strings.Repeat("P", 100)))
			raw := rawListResponse(t, h, `{"limit":500}`)
			if len(raw) > ipc.MaxMessageSize {
				t.Errorf("response is %d bytes, want <= %d", len(raw), ipc.MaxMessageSize)
			}
			if list := listResultOf(t, raw); len(list.Games) != 500 || list.Total != 500 {
				t.Errorf("games = %d, total = %d, want 500 and 500", len(list.Games), list.Total)
			}
		})

		t.Run("com caracteres que o JSON escapa a página para antes de 1 MiB e não repete nem pula jogos", func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			all := worstGames(500, strings.Repeat("<", 200), `C:\`+strings.Repeat("&", 257), strings.Repeat("<", 100))
			h.gs.listFn = pagedStore(all)
			raw := rawListResponse(t, h, `{"limit":500}`)
			if len(raw) > ipc.MaxMessageSize {
				t.Fatalf("response is %d bytes, want <= %d", len(raw), ipc.MaxMessageSize)
			}
			list := listResultOf(t, raw)
			n := len(list.Games)
			if n == 0 || n >= len(all) {
				t.Fatalf("page has %d games, want some but not all of %d", n, len(all))
			}
			if list.Total != int64(len(all)) || list.Offset != 0 {
				t.Errorf("total = %d, offset = %d, want %d and 0", list.Total, list.Offset, len(all))
			}
			// The page stops before the limit: the next game would not fit.
			next, err := json.Marshal(toIPCGame(all[n]))
			if err != nil {
				t.Fatal(err)
			}
			if len(raw)+1+len(next) <= ipc.MaxMessageSize {
				t.Errorf("page has %d games (%d bytes) but game %d (%d bytes) would still fit",
					n, len(raw), n+1, len(next))
			}
			// Walking the pages by the returned count sees every game once, in order.
			seen := 0
			for offset := 0; ; {
				page := listResultOf(t, rawListResponse(t, h, fmt.Sprintf(`{"limit":500,"offset":%d}`, offset)))
				if len(page.Games) == 0 {
					break
				}
				for _, g := range page.Games {
					seen++
					if g.ID != int64(seen) {
						t.Fatalf("game at position %d has id %d, want %d", seen, g.ID, seen)
					}
				}
				offset += len(page.Games)
				if seen > len(all) {
					t.Fatalf("walk returned %d games, want %d", seen, len(all))
				}
			}
			if seen != len(all) {
				t.Errorf("walk returned %d games, want %d", seen, len(all))
			}
		})

		t.Run("conteúdo curto não corta a página de 500", func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			h.gs.listFn = pagedStore(worstGames(500, "T", `C:\a.exe`, "PC"))
			if list := listResultOf(t, rawListResponse(t, h, `{"limit":500}`)); len(list.Games) != 500 {
				t.Errorf("games = %d, want all 500 when the content is small", len(list.Games))
			}
		})
	})

	t.Run("IPC-023/erro do store devolve -32603", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.gs.listFn = func(context.Context, store.Query) (store.Page, error) {
			return store.Page{}, errors.New("disk I/O error")
		}
		wantError(t, h.call(t, ipc.MethodGameList, ""), ipc.CodeInternalError, "Internal error")
	})
}

func TestToIPCGame(t *testing.T) {
	t.Parallel()
	first := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	last := time.Date(2026, 11, 8, 22, 30, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   store.Game
		want string
	}{
		{
			"IPC-023/datas zero saem como null",
			store.Game{ID: 4, Title: "T", PlatformID: 1, Platform: "PC", Status: domain.StatusNotPlayed, LaunchType: "exe", LaunchTarget: `C:\g\t.exe`, WorkingDir: `C:\g`},
			`{"id":4,"title":"T","platformId":1,"platform":"PC","path":"C:\\g\\t.exe","status":"not_played","favorite":false,"year":null,"totalPlaySeconds":0,"playCount":0,"firstPlayedAt":null,"lastPlayedAt":null}`,
		},
		{
			"IPC-023/totais e datas do store",
			store.Game{ID: 5, Title: "U", PlatformID: 2, Platform: "PC", Status: domain.StatusPlaying, Favorite: true, ReleaseDate: "2009-05-01", LaunchType: "exe", LaunchTarget: `C:\u.exe`, TotalPlaySeconds: 3600, PlayCount: 2, FirstPlayedAt: first, LastPlayedAt: last},
			`{"id":5,"title":"U","platformId":2,"platform":"PC","path":"C:\\u.exe","status":"playing","favorite":true,"year":2009,"totalPlaySeconds":3600,"playCount":2,"firstPlayedAt":"2026-11-01T10:00:00Z","lastPlayedAt":"2026-11-08T22:30:00Z"}`,
		},
		{
			"IPC-042/ReleaseDate YYYY-MM dá o ano (DOM-013)",
			store.Game{ID: 6, Title: "V", PlatformID: 1, Platform: "PC", Status: domain.StatusCompleted, ReleaseDate: "2009-05", LaunchTarget: `C:\v.exe`},
			`{"id":6,"title":"V","platformId":1,"platform":"PC","path":"C:\\v.exe","status":"completed","favorite":false,"year":2009,"totalPlaySeconds":0,"playCount":0,"firstPlayedAt":null,"lastPlayedAt":null}`,
		},
		{
			"IPC-042/ReleaseDate inválida dá year null (DOM-013)",
			store.Game{ID: 7, Title: "W", PlatformID: 1, Platform: "PC", Status: domain.StatusBacklog, ReleaseDate: "bogus", LaunchTarget: `C:\w.exe`},
			`{"id":7,"title":"W","platformId":1,"platform":"PC","path":"C:\\w.exe","status":"backlog","favorite":false,"year":null,"totalPlaySeconds":0,"playCount":0,"firstPlayedAt":null,"lastPlayedAt":null}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := json.Marshal(toIPCGame(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("toIPCGame = %s\nwant          %s", got, tc.want)
			}
		})
	}
}

func TestGameLaunch(t *testing.T) {
	t.Parallel()

	t.Run("IPC-024/devolve Session running com endedAt null e activeSeconds 0", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		resp := h.call(t, ipc.MethodGameLaunch, fmt.Sprintf(`{"gameId":%d}`, g.ID))
		if resp.Error != nil {
			t.Fatalf("error %d %q %s", resp.Error.Code, resp.Error.Message, resp.Error.Data)
		}
		obj := objectOf(t, resp.Result)
		wantKeys(t, obj, sessionKeys...)
		wantRaw(t, obj, "state", `"running"`)
		wantRaw(t, obj, "endedAt", "null")
		wantRaw(t, obj, "activeSeconds", "0")
		wantRaw(t, obj, "activeMs", "0")
		wantRaw(t, obj, "gameId", fmt.Sprint(g.ID))
		wantRaw(t, obj, "title", `"Game"`)
		wantRaw(t, obj, "startedAt", `"2026-11-09T14:00:00Z"`)
		if n := len(h.tr.launches()); n != 1 {
			t.Errorf("Launch calls = %d, want 1", n)
		}
	})

	t.Run("IPC-031/a Session tem activeMs e asOf do snapshot", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.tr.launchFn = func(_ context.Context, spec tracker.LaunchSpec) (domain.SessionSnapshot, error) {
			return domain.SessionSnapshot{
				ID: 9, GameID: spec.GameID, Title: spec.Title, State: domain.SessionRunning,
				StartedAt: gamesT0, Active: 252400 * time.Millisecond, AsOf: gamesT0.Add(252400 * time.Millisecond),
			}, nil
		}
		obj := objectOf(t, h.call(t, ipc.MethodGameLaunch, fmt.Sprintf(`{"gameId":%d}`, g.ID)).Result)
		wantRaw(t, obj, "activeMs", "252400")
		wantRaw(t, obj, "activeSeconds", "252")
		wantRaw(t, obj, "asOf", `"2026-11-09T14:04:12.400Z"`)
		wantRaw(t, obj, "sessionId", "9")
	})

	t.Run("IPC-025/gameId inexistente devolve -32001 sem chamar Launch", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		// Even with an active session, existence is checked first.
		h.tr.launchFn = func(context.Context, tracker.LaunchSpec) (domain.SessionSnapshot, error) {
			return domain.SessionSnapshot{}, &tracker.SessionActiveError{Session: domain.SessionSnapshot{ID: 1, GameID: 1, Title: "x"}}
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunch, `{"gameId":99}`), ipc.CodeGameNotFound, ipc.MsgGameNotFound)
		wantKeys(t, data, "gameId")
		wantRaw(t, data, "gameId", "99")
		if n := len(h.tr.launches()); n != 0 {
			t.Errorf("Launch calls = %d, want 0", n)
		}
	})

	t.Run("IPC-025/sessão ativa devolve -32002 com sessionId, gameId e title", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.tr.launchFn = func(context.Context, tracker.LaunchSpec) (domain.SessionSnapshot, error) {
			return domain.SessionSnapshot{}, fmt.Errorf("launch: %w", &tracker.SessionActiveError{Session: domain.SessionSnapshot{
				ID: 7, GameID: 3, Title: "Other", State: domain.SessionGrace, StartedAt: gamesT0, AsOf: gamesT0,
			}})
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunch, fmt.Sprintf(`{"gameId":%d}`, g.ID)), ipc.CodeSessionActive, ipc.MsgSessionActive)
		wantKeys(t, data, "sessionId", "gameId", "title")
		wantRaw(t, data, "sessionId", "7")
		wantRaw(t, data, "gameId", "3")
		wantRaw(t, data, "title", `"Other"`)
	})

	t.Run("IPC-025/sessão ativa em Launching devolve sessionId 0", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.tr.launchFn = func(context.Context, tracker.LaunchSpec) (domain.SessionSnapshot, error) {
			return domain.SessionSnapshot{}, &tracker.SessionActiveError{Session: domain.SessionSnapshot{GameID: 2, Title: "Pending"}}
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunch, fmt.Sprintf(`{"gameId":%d}`, g.ID)), ipc.CodeSessionActive, ipc.MsgSessionActive)
		wantRaw(t, data, "sessionId", "0")
		wantRaw(t, data, "gameId", "2")
	})

	t.Run("IPC-025/LaunchError com WinError devolve -32003 com error e winError", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.tr.launchFn = func(context.Context, tracker.LaunchSpec) (domain.SessionSnapshot, error) {
			return domain.SessionSnapshot{}, &tracker.LaunchError{Err: errors.New("O sistema não pode encontrar o arquivo especificado."), WinError: 2}
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunch, fmt.Sprintf(`{"gameId":%d}`, g.ID)), ipc.CodeLaunchFailed, ipc.MsgLaunchFailed)
		wantKeys(t, data, "error", "winError")
		wantRaw(t, data, "winError", "2")
		var text string
		if err := json.Unmarshal(data["error"], &text); err != nil || text == "" {
			t.Errorf("data.error = %s, want a non-empty string", data["error"])
		}
	})

	t.Run("IPC-025/LaunchError sem WinError omite winError", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.tr.launchFn = func(context.Context, tracker.LaunchSpec) (domain.SessionSnapshot, error) {
			return domain.SessionSnapshot{}, &tracker.LaunchError{Err: fmt.Errorf("tracker: job objects: %w", errors.ErrUnsupported)}
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunch, fmt.Sprintf(`{"gameId":%d}`, g.ID)), ipc.CodeLaunchFailed, ipc.MsgLaunchFailed)
		wantKeys(t, data, "error")
		var text string
		if err := json.Unmarshal(data["error"], &text); err != nil || text == "" {
			t.Errorf("data.error = %s, want a non-empty string", data["error"])
		}
	})

	t.Run("AGT-011/ErrShuttingDown devolve -32003 com error agente encerrando", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.tr.launchFn = func(context.Context, tracker.LaunchSpec) (domain.SessionSnapshot, error) {
			return domain.SessionSnapshot{}, fmt.Errorf("launch: %w", tracker.ErrShuttingDown)
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunch, fmt.Sprintf(`{"gameId":%d}`, g.ID)), ipc.CodeLaunchFailed, ipc.MsgLaunchFailed)
		wantKeys(t, data, "error")
		wantRaw(t, data, "error", `"agente encerrando"`)
	})

	t.Run("IPC-024/LaunchSpec montado de launch_target, launch_args e working_dir", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.gs.gameFn = func(id int64) (store.Game, error) {
			return store.Game{
				ID: id, Title: "Hollow Knight", LaunchType: "exe",
				LaunchTarget: `C:\Jogos\Hollow Knight\hollow_knight.exe`, LaunchArgs: `-windowed "x y"`,
				WorkingDir: `D:\work dir`,
			}, nil
		}
		resultInto(t, h.call(t, ipc.MethodGameLaunch, `{"gameId":42}`), &ipc.Session{})
		specs := h.tr.launches()
		if len(specs) != 1 {
			t.Fatalf("Launch calls = %d, want 1", len(specs))
		}
		want := tracker.LaunchSpec{
			GameID: 42, Title: "Hollow Knight", Exe: `C:\Jogos\Hollow Knight\hollow_knight.exe`,
			Args: `-windowed "x y"`, WorkDir: `D:\work dir`,
		}
		if specs[0] != want {
			t.Errorf("LaunchSpec = %+v, want %+v", specs[0], want)
		}
	})

	t.Run("IPC-025/launch_type diferente de exe devolve -32003 sem chamar Launch", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.gs.gameFn = func(id int64) (store.Game, error) {
			return store.Game{ID: id, Title: "Steam", LaunchType: "url", LaunchTarget: "steam://run/1"}, nil
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunch, `{"gameId":5}`), ipc.CodeLaunchFailed, ipc.MsgLaunchFailed)
		wantRaw(t, data, "error", `"tipo de lançamento não suportado"`)
		if n := len(h.tr.launches()); n != 0 {
			t.Errorf("Launch calls = %d, want 0", n)
		}
	})

	t.Run("IPC-025/ctx do Launch tem prazo de 10 s", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		resultInto(t, h.call(t, ipc.MethodGameLaunch, fmt.Sprintf(`{"gameId":%d}`, g.ID)), &ipc.Session{})
		h.tr.mu.Lock()
		dl := h.tr.deadlines[0]
		h.tr.mu.Unlock()
		if dl.IsZero() {
			t.Fatalf("Launch ctx has no deadline, want %v", launchHandlerTimeout)
		}
		if launchHandlerTimeout != 10*time.Second {
			t.Errorf("launchHandlerTimeout = %v, want 10s", launchHandlerTimeout)
		}
		if left := time.Until(dl); left > launchHandlerTimeout || left < launchHandlerTimeout-5*time.Second {
			t.Errorf("Launch ctx deadline in %v, want about %v", left, launchHandlerTimeout)
		}
	})

	t.Run("IPC-025/prazo do handler esgotado antes da resposta devolve -32603", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		started := make(chan struct{})
		h.tr.launchFn = func(ctx context.Context, _ tracker.LaunchSpec) (domain.SessionSnapshot, error) {
			close(started)
			<-ctx.Done()
			return domain.SessionSnapshot{}, ctx.Err()
		}
		var wg sync.WaitGroup
		defer wg.Wait() // runs after cancel below
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan ipc.Response, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"game.launch","params":{"gameId":%d}}`, g.ID)
			raw, _ := ipc.Dispatch(ctx, h.handlers, []byte(msg))
			var resp ipc.Response
			_ = json.Unmarshal(raw, &resp)
			done <- resp
		}()
		select {
		case <-started:
		case <-time.After(testTimeout):
			t.Fatal("Launch was not called")
		}
		cancel()
		select {
		case resp := <-done:
			wantError(t, resp, ipc.CodeInternalError, "Internal error")
		case <-time.After(testTimeout):
			t.Fatal("handler did not return after its ctx ended")
		}
	})

	t.Run("IPC-025/Launch devolve DeadlineExceeded vira -32603", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.tr.launchFn = func(context.Context, tracker.LaunchSpec) (domain.SessionSnapshot, error) {
			return domain.SessionSnapshot{}, fmt.Errorf("tracker: launch: %w", context.DeadlineExceeded)
		}
		wantError(t, h.call(t, ipc.MethodGameLaunch, fmt.Sprintf(`{"gameId":%d}`, g.ID)), ipc.CodeInternalError, "Internal error")
	})
}

func TestSessionCurrent(t *testing.T) {
	t.Parallel()

	t.Run("IPC-026/sem sessão devolve session null", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		resp := h.call(t, ipc.MethodSessionCurrent, "")
		if resp.Error != nil {
			t.Fatalf("error %d %q", resp.Error.Code, resp.Error.Message)
		}
		obj := objectOf(t, resp.Result)
		wantKeys(t, obj, "session")
		wantRaw(t, obj, "session", "null")
	})

	cases := []struct {
		name                     string
		snap                     domain.SessionSnapshot
		state, activeMs, seconds string
		asOf                     string
	}{
		{
			"IPC-026/com sessão devolve state e o ativo do snapshot",
			domain.SessionSnapshot{ID: 3, GameID: 1, Title: "G", State: domain.SessionRunning, StartedAt: gamesT0,
				Active: 252400 * time.Millisecond, AsOf: gamesT0.Add(252400 * time.Millisecond)},
			`"running"`, "252400", "252", `"2026-11-09T14:04:12.400Z"`,
		},
		{
			"IPC-026/com sessão devolve state e o ativo do snapshot",
			domain.SessionSnapshot{ID: 3, GameID: 1, Title: "G", State: domain.SessionGrace, StartedAt: gamesT0,
				Active: 300 * time.Second, AsOf: gamesT0.Add(310 * time.Second)},
			`"grace"`, "300000", "300", `"2026-11-09T14:05:10.000Z"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			h.tr.current, h.tr.hasCurrent = tc.snap, true
			resp := h.call(t, ipc.MethodSessionCurrent, "")
			if resp.Error != nil {
				t.Fatalf("error %d %q", resp.Error.Code, resp.Error.Message)
			}
			sess := objectOf(t, objectOf(t, resp.Result)["session"])
			wantKeys(t, sess, sessionKeys...)
			wantRaw(t, sess, "state", tc.state)
			wantRaw(t, sess, "activeMs", tc.activeMs)
			wantRaw(t, sess, "activeSeconds", tc.seconds)
			wantRaw(t, sess, "asOf", tc.asOf)
			wantRaw(t, sess, "endedAt", "null")
			wantRaw(t, sess, "sessionId", "3")
			h.tr.mu.Lock()
			calls := h.tr.curCalls
			h.tr.mu.Unlock()
			if calls != 1 {
				t.Errorf("Current calls = %d, want 1", calls)
			}
		})
	}
}

func TestParseGameID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		raw    json.RawMessage
		want   int64
		wantOK bool
	}{
		{"IPC-028/gameId inteiro >= 1 é aceito", json.RawMessage(`1`), 1, true},
		{"IPC-028/gameId inteiro >= 1 é aceito", json.RawMessage(`9223372036854775807`), 9223372036854775807, true},
		{"IPC-028/gameId texto devolve -32602", json.RawMessage(`"5"`), 0, false},
		{"IPC-028/gameId fracionário devolve -32602", json.RawMessage(`1.5`), 0, false},
		{"IPC-028/gameId fracionário devolve -32602", json.RawMessage(`1.0`), 0, false},
		{"IPC-028/gameId fracionário devolve -32602", json.RawMessage(`1e0`), 0, false},
		{"IPC-028/gameId fracionário devolve -32602", json.RawMessage(`1E2`), 0, false},
		{"IPC-028/gameId 0 devolve -32602", json.RawMessage(`0`), 0, false},
		{"IPC-028/gameId 0 devolve -32602", json.RawMessage(`-1`), 0, false},
		{"IPC-028/gameId 0 devolve -32602", json.RawMessage(`-0`), 0, false},
		{"IPC-028/gameId 0 devolve -32602", json.RawMessage(`9223372036854775808`), 0, false},
		{"IPC-028/gameId ausente ou null devolve -32602", nil, 0, false},
		{"IPC-028/gameId ausente ou null devolve -32602", json.RawMessage(`null`), 0, false},
		{"IPC-028/gameId texto devolve -32602", json.RawMessage(`true`), 0, false},
		{"IPC-028/gameId texto devolve -32602", json.RawMessage(`[1]`), 0, false},
		{"IPC-028/gameId texto devolve -32602", json.RawMessage(`{"id":1}`), 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseGameID(tc.raw)
			if ok != tc.wantOK || (ok && got != tc.want) {
				t.Errorf("parseGameID(%s) = %d, %v; want %d, %v", tc.raw, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestNewMethodParams(t *testing.T) {
	t.Parallel()

	type call struct{ method, params string }
	// launchCalls and addCalls must all yield -32602 without touching the
	// store (game.launch) or writing (game.add).
	rejected := []struct {
		name  string
		calls func(t *testing.T, exe string) []call
	}{
		{"IPC-028/gameId texto devolve -32602", func(*testing.T, string) []call {
			return []call{{ipc.MethodGameLaunch, `{"gameId":"1"}`}, {ipc.MethodGameLaunch, `{"gameId":true}`}}
		}},
		{"IPC-028/gameId fracionário devolve -32602", func(*testing.T, string) []call {
			return []call{{ipc.MethodGameLaunch, `{"gameId":1.5}`}, {ipc.MethodGameLaunch, `{"gameId":1.0}`}, {ipc.MethodGameLaunch, `{"gameId":1e0}`}}
		}},
		{"IPC-028/gameId 0 devolve -32602", func(*testing.T, string) []call {
			return []call{{ipc.MethodGameLaunch, `{"gameId":0}`}, {ipc.MethodGameLaunch, `{"gameId":-3}`}}
		}},
		{"IPC-028/gameId ausente ou null devolve -32602", func(*testing.T, string) []call {
			return []call{
				{ipc.MethodGameLaunch, `{"gameId":null}`}, {ipc.MethodGameLaunch, `{}`},
				{ipc.MethodGameLaunch, `null`}, {ipc.MethodGameLaunch, ``},
			}
		}},
		{"IPC-028/path ausente devolve -32602", func(*testing.T, string) []call {
			return []call{
				{ipc.MethodGameAdd, `{"title":"X"}`}, {ipc.MethodGameAdd, `{}`},
				{ipc.MethodGameAdd, `null`}, {ipc.MethodGameAdd, ``},
			}
		}},
		{"IPC-028/path não texto devolve -32602", func(t *testing.T, exe string) []call {
			return []call{
				{ipc.MethodGameAdd, `{"path":5}`}, {ipc.MethodGameAdd, `{"path":["a"]}`},
				{ipc.MethodGameAdd, mustJSON(t, map[string]any{"path": exe, "title": 5})},
			}
		}},
		{"IPC-028/array posicional devolve -32602", func(t *testing.T, exe string) []call {
			return []call{{ipc.MethodGameAdd, mustJSON(t, []string{exe})}, {ipc.MethodGameLaunch, `[1]`}}
		}},
		{"IPC-028/membro desconhecido devolve -32602", func(t *testing.T, exe string) []call {
			return []call{
				{ipc.MethodGameAdd, mustJSON(t, map[string]any{"path": exe, "extra": 1})},
				{ipc.MethodGameAdd, mustJSON(t, map[string]any{"Path": exe})},
				{ipc.MethodGameLaunch, `{"gameId":1,"force":true}`},
				{ipc.MethodGameLaunch, `{"gameID":1}`},
			}
		}},
		{"IPC-008/game.list com array vazio [] é -32602 (params posicionais)", func(*testing.T, string) []call {
			return []call{{ipc.MethodGameList, `[]`}}
		}},
		{"IPC-028/game.list e session.current com params não vazios devolvem -32602", func(*testing.T, string) []call {
			return []call{
				{ipc.MethodGameList, `{"a":1}`}, {ipc.MethodGameList, `[1]`},
				{ipc.MethodSessionCurrent, `{"a":1}`}, {ipc.MethodSessionCurrent, `["x"]`},
			}
		}},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			exe := h.file(t, "game.exe")
			for _, c := range tc.calls(t, exe) {
				resp := h.call(t, c.method, c.params)
				if resp.Error == nil || resp.Error.Code != ipc.CodeInvalidParams {
					t.Errorf("%s params %s: response %+v (result %s), want -32602", c.method, c.params, resp.Error, resp.Result)
				}
			}
			if n := len(h.gs.addedGames()); n != 0 {
				t.Errorf("AddGame calls = %d, want 0", n)
			}
			if n := h.gs.gameCalls(); n != 0 {
				t.Errorf("store Game calls = %d, want 0", n)
			}
			if n := len(h.tr.launches()); n != 0 {
				t.Errorf("Launch calls = %d, want 0", n)
			}
		})
	}

	t.Run("IPC-028/vazios são aceitos", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		for _, m := range []string{ipc.MethodSessionCurrent} {
			for _, p := range []string{"", "null", "{}", "[]"} {
				resp := h.call(t, m, p)
				if resp.Error != nil {
					t.Errorf("%s params %q: error %d %q, want a result", m, p, resp.Error.Code, resp.Error.Message)
				}
			}
		}
		// game.list leaves the no-params methods (IPC-008, IPC-042): "" and {} are the
		// first page; [] and null are covered by the rejected table and by IPC-008.
		for _, p := range []string{"", "{}"} {
			if resp := h.call(t, ipc.MethodGameList, p); resp.Error != nil {
				t.Errorf("%s params %q: error %d %q, want a result", ipc.MethodGameList, p, resp.Error.Code, resp.Error.Message)
			}
		}
		// Valid named params of the methods with params are accepted too.
		g := h.addGame(t, h.file(t, "game.exe"), "")
		resp := h.call(t, ipc.MethodGameLaunch, fmt.Sprintf(`{"gameId":%d}`, g.ID))
		if resp.Error != nil {
			t.Errorf("game.launch: error %d %q", resp.Error.Code, resp.Error.Message)
		}
	})
}

// --- T18: game.launchUntracked, session.pause and session.resume ---

// need fails the test when the handlers do not register method yet.
func (h *gamesHarness) need(t *testing.T, method string) {
	t.Helper()
	if h.handlers[method] == nil {
		t.Fatalf("gameHandlers has no handler for %q", method)
	}
}

// callCtx dispatches method with ctx and waits for the response; the handler
// is expected to return once ctx ends.
func (h *gamesHarness) callCtx(t *testing.T, ctx context.Context, method, params string) ipc.Response {
	t.Helper()
	done := make(chan ipc.Response, 1)
	go func() { done <- callHandlers(t, ctx, h.handlers, method, params) }()
	select {
	case resp := <-done:
		return resp
	case <-time.After(testTimeout):
		t.Fatalf("%s did not return", method)
		return ipc.Response{}
	}
}

func TestHandleGameLaunchUntracked(t *testing.T) {
	t.Parallel()
	untracked := func(id int64) string { return fmt.Sprintf(`{"gameId":%d}`, id) }

	t.Run("IPC-033/jogo existente e sem sessão devolve {pid: N >= 1} e session.current continua {session: null}", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodGameLaunchUntracked)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		resp := h.call(t, ipc.MethodGameLaunchUntracked, untracked(g.ID))
		if resp.Error != nil {
			t.Fatalf("error %d %q %s", resp.Error.Code, resp.Error.Message, resp.Error.Data)
		}
		obj := objectOf(t, resp.Result)
		wantKeys(t, obj, "pid")
		var lp ipc.LaunchedProcess
		resultInto(t, resp, &lp)
		if lp.PID < 1 {
			t.Errorf("pid = %d, want >= 1", lp.PID)
		}
		wantRaw(t, obj, "pid", "4242")
		specs := h.tr.untrackedLaunches()
		if len(specs) != 1 || specs[0].GameID != g.ID || specs[0].Title != "Game" || specs[0].Exe != g.Path {
			t.Errorf("LaunchUntracked specs = %+v, want one for game %d", specs, g.ID)
		}
		cur := h.call(t, ipc.MethodSessionCurrent, "")
		wantRaw(t, objectOf(t, cur.Result), "session", "null")
	})

	t.Run("IPC-033/nenhuma notificação session.changed é enviada", func(t *testing.T) {
		t.Parallel()
		// The handler has no notifier: it must not touch the session machine
		// (Launch, Pause, Resume), which is what produces session.changed.
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodGameLaunchUntracked)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		resultInto(t, h.call(t, ipc.MethodGameLaunchUntracked, untracked(g.ID)), &ipc.LaunchedProcess{})
		h.tr.mu.Lock()
		defer h.tr.mu.Unlock()
		if len(h.tr.specs) != 0 || len(h.tr.pauseAts) != 0 || len(h.tr.resumeAts) != 0 {
			t.Errorf("tracked Launch/Pause/Resume called: %d/%d/%d, want 0", len(h.tr.specs), len(h.tr.pauseAts), len(h.tr.resumeAts))
		}
	})

	t.Run("IPC-033/LaunchSpec montado de launch_target, launch_args e working_dir", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodGameLaunchUntracked)
		h.gs.gameFn = func(id int64) (store.Game, error) {
			return store.Game{ID: id, Title: "HK", LaunchType: "exe", LaunchTarget: `C:\hk.exe`, LaunchArgs: `-w "x y"`, WorkingDir: `D:\w`}, nil
		}
		resultInto(t, h.call(t, ipc.MethodGameLaunchUntracked, untracked(42)), &ipc.LaunchedProcess{})
		want := tracker.LaunchSpec{GameID: 42, Title: "HK", Exe: `C:\hk.exe`, Args: `-w "x y"`, WorkDir: `D:\w`}
		specs := h.tr.untrackedLaunches()
		if len(specs) != 1 || specs[0] != want {
			t.Errorf("LaunchSpec = %+v, want %+v", specs, want)
		}
	})

	t.Run("IPC-034/gameId inexistente devolve -32001 antes de consultar a sessão", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodGameLaunchUntracked)
		h.tr.untrackedFn = func(context.Context, tracker.LaunchSpec) (uint32, error) {
			return 0, &tracker.SessionActiveError{Session: domain.SessionSnapshot{ID: 1, GameID: 1, Title: "x"}}
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunchUntracked, `{"gameId":99}`), ipc.CodeGameNotFound, ipc.MsgGameNotFound)
		wantKeys(t, data, "gameId")
		wantRaw(t, data, "gameId", "99")
		if n := len(h.tr.untrackedLaunches()); n != 0 {
			t.Errorf("LaunchUntracked calls = %d, want 0", n)
		}
	})

	for _, st := range []struct {
		name string
		snap domain.SessionSnapshot
		sid  string
	}{
		{"Running", domain.SessionSnapshot{ID: 7, GameID: 3, Title: "Other", State: domain.SessionRunning}, "7"},
		{"Grace", domain.SessionSnapshot{ID: 7, GameID: 3, Title: "Other", State: domain.SessionGrace}, "7"},
		{"Paused", domain.SessionSnapshot{ID: 7, GameID: 3, Title: "Other", State: domain.SessionPaused}, "7"},
		{"lançamento em andamento", domain.SessionSnapshot{GameID: 3, Title: "Other"}, "0"},
	} {
		t.Run("IPC-034/sessão Running, Grace, Paused ou lançamento em andamento devolve -32002 com sessionId, gameId e title/"+st.name, func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			h.need(t, ipc.MethodGameLaunchUntracked)
			g := h.addGame(t, h.file(t, "game.exe"), "Game")
			h.tr.untrackedFn = func(context.Context, tracker.LaunchSpec) (uint32, error) {
				return 0, fmt.Errorf("launch untracked: %w", &tracker.SessionActiveError{Session: st.snap})
			}
			data := wantError(t, h.call(t, ipc.MethodGameLaunchUntracked, untracked(g.ID)), ipc.CodeSessionActive, ipc.MsgSessionActive)
			wantKeys(t, data, "sessionId", "gameId", "title")
			wantRaw(t, data, "sessionId", st.sid)
			wantRaw(t, data, "gameId", "3")
			wantRaw(t, data, "title", `"Other"`)
		})
	}

	t.Run("IPC-034/falha do lançamento devolve -32003 com data.error e winError", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodGameLaunchUntracked)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.tr.untrackedFn = func(context.Context, tracker.LaunchSpec) (uint32, error) {
			return 0, &tracker.LaunchError{Err: errors.New("arquivo não encontrado"), WinError: 2}
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunchUntracked, untracked(g.ID)), ipc.CodeLaunchFailed, ipc.MsgLaunchFailed)
		wantKeys(t, data, "error", "winError")
		wantRaw(t, data, "error", `"arquivo não encontrado"`)
		wantRaw(t, data, "winError", "2")
	})

	t.Run("IPC-034/agente em shutdown devolve -32003 com error agente encerrando", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodGameLaunchUntracked)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.tr.untrackedFn = func(context.Context, tracker.LaunchSpec) (uint32, error) {
			return 0, fmt.Errorf("launch untracked: %w", tracker.ErrShuttingDown)
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunchUntracked, untracked(g.ID)), ipc.CodeLaunchFailed, ipc.MsgLaunchFailed)
		wantKeys(t, data, "error")
		wantRaw(t, data, "error", `"agente encerrando"`)
	})

	t.Run("IPC-034/launch_type diferente de exe devolve -32003 tipo de lançamento não suportado", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodGameLaunchUntracked)
		h.gs.gameFn = func(id int64) (store.Game, error) {
			return store.Game{ID: id, Title: "Steam", LaunchType: "url", LaunchTarget: "steam://run/1"}, nil
		}
		data := wantError(t, h.call(t, ipc.MethodGameLaunchUntracked, untracked(5)), ipc.CodeLaunchFailed, ipc.MsgLaunchFailed)
		wantRaw(t, data, "error", `"tipo de lançamento não suportado"`)
		if n := len(h.tr.untrackedLaunches()); n != 0 {
			t.Errorf("LaunchUntracked calls = %d, want 0", n)
		}
	})

	t.Run("IPC-034/prazo de 10 s esgotado devolve -32603", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodGameLaunchUntracked)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")

		// The ctx of the call carries the 10 s deadline.
		resultInto(t, h.call(t, ipc.MethodGameLaunchUntracked, untracked(g.ID)), &ipc.LaunchedProcess{})
		h.tr.mu.Lock()
		dl := h.tr.untrackedDeadlines[0]
		h.tr.mu.Unlock()
		if dl.IsZero() {
			t.Fatalf("LaunchUntracked ctx has no deadline, want %v", launchHandlerTimeout)
		}
		if left := time.Until(dl); left > launchHandlerTimeout || left < launchHandlerTimeout-5*time.Second {
			t.Errorf("deadline in %v, want about %v", left, launchHandlerTimeout)
		}

		// A deadline error from the tracker becomes -32603.
		h.tr.untrackedFn = func(context.Context, tracker.LaunchSpec) (uint32, error) {
			return 0, fmt.Errorf("tracker: launch: %w", context.DeadlineExceeded)
		}
		wantError(t, h.call(t, ipc.MethodGameLaunchUntracked, untracked(g.ID)), ipc.CodeInternalError, "Internal error")

		// The end of the dispatch ctx while the tracker waits too.
		h.tr.untrackedFn = func(ctx context.Context, _ tracker.LaunchSpec) (uint32, error) {
			<-ctx.Done()
			return 0, ctx.Err()
		}
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(10*time.Millisecond, cancel)
		defer cancel()
		wantError(t, h.callCtx(t, ctx, ipc.MethodGameLaunchUntracked, untracked(g.ID)), ipc.CodeInternalError, "Internal error")
	})

	rejected := []struct {
		name   string
		params []string
	}{
		{"IPC-035/gameId texto, fracionário, < 1, array posicional, ausente ou membro desconhecido devolve -32602/texto",
			[]string{`{"gameId":"1"}`, `{"gameId":true}`}},
		{"IPC-035/gameId texto, fracionário, < 1, array posicional, ausente ou membro desconhecido devolve -32602/fracionário",
			[]string{`{"gameId":1.5}`, `{"gameId":1.0}`, `{"gameId":1e0}`}},
		{"IPC-035/gameId texto, fracionário, < 1, array posicional, ausente ou membro desconhecido devolve -32602/< 1",
			[]string{`{"gameId":0}`, `{"gameId":-3}`}},
		{"IPC-035/gameId texto, fracionário, < 1, array posicional, ausente ou membro desconhecido devolve -32602/array posicional",
			[]string{`[1]`}},
		{"IPC-035/gameId texto, fracionário, < 1, array posicional, ausente ou membro desconhecido devolve -32602/membro desconhecido",
			[]string{`{"gameId":1,"force":true}`, `{"gameID":1}`}},
		{"IPC-035/params vazios ({}, [], null, ausente) devolvem -32602",
			[]string{`{}`, `[]`, `null`, ``, `{"gameId":null}`}},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			h.need(t, ipc.MethodGameLaunchUntracked)
			for _, p := range tc.params {
				resp := h.call(t, ipc.MethodGameLaunchUntracked, p)
				if resp.Error == nil || resp.Error.Code != ipc.CodeInvalidParams {
					t.Errorf("params %q: response %+v (result %s), want -32602", p, resp.Error, resp.Result)
				}
			}
			if n := h.gs.gameCalls(); n != 0 {
				t.Errorf("store Game calls = %d, want 0", n)
			}
			if n := len(h.tr.untrackedLaunches()); n != 0 {
				t.Errorf("LaunchUntracked calls = %d, want 0", n)
			}
		})
	}
}

func TestHandleSessionCurrentPaused(t *testing.T) {
	t.Parallel()

	t.Run("IPC-036/sessão pausada devolve state paused, activeMs 100000, activeSeconds 100 e asOf do momento da chamada; sem sessão continua {session: null}", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		// The tracker snapshot of a session paused at T+100s, read at T+130s:
		// active time frozen, asOf = the call instant.
		h.tr.current, h.tr.hasCurrent = domain.SessionSnapshot{
			ID: 3, GameID: 1, Title: "G", State: domain.SessionPaused, StartedAt: gamesT0,
			Active: 100 * time.Second, AsOf: gamesT0.Add(130 * time.Second),
		}, true
		resp := h.call(t, ipc.MethodSessionCurrent, "")
		sess := objectOf(t, objectOf(t, resp.Result)["session"])
		wantKeys(t, sess, sessionKeys...)
		wantRaw(t, sess, "state", `"paused"`)
		wantRaw(t, sess, "activeMs", "100000")
		wantRaw(t, sess, "activeSeconds", "100")
		wantRaw(t, sess, "asOf", `"2026-11-09T14:02:10.000Z"`)
		wantRaw(t, sess, "endedAt", "null")

		h.tr.mu.Lock()
		h.tr.hasCurrent = false
		h.tr.mu.Unlock()
		wantRaw(t, objectOf(t, h.call(t, ipc.MethodSessionCurrent, "").Result), "session", "null")
	})
}

// explicitMethods are the two explicit-request methods with their fake hooks.
var explicitMethods = []struct {
	method string
	ats    func(f *fakeTracker) []time.Time
	set    func(f *fakeTracker, fn func(context.Context, time.Time) (domain.SessionSnapshot, error))
}{
	{ipc.MethodSessionPause,
		func(f *fakeTracker) []time.Time { return f.pauseAts },
		func(f *fakeTracker, fn func(context.Context, time.Time) (domain.SessionSnapshot, error)) {
			f.pauseFn = fn
		}},
	{ipc.MethodSessionResume,
		func(f *fakeTracker) []time.Time { return f.resumeAts },
		func(f *fakeTracker, fn func(context.Context, time.Time) (domain.SessionSnapshot, error)) {
			f.resumeFn = fn
		}},
}

// explicitParamsRules checks the empty/non-empty params rule of IPC-038/039.
func explicitParamsRules(t *testing.T, method string) {
	t.Helper()
	h := newGamesHarness(t, nil)
	h.need(t, method)
	for _, p := range []string{``, `null`, `{}`, `[]`} {
		resp := h.call(t, method, p)
		if resp.Error != nil {
			t.Errorf("%s params %q: error %d %q, want a result", method, p, resp.Error.Code, resp.Error.Message)
		}
	}
	h.tr.mu.Lock()
	before := len(h.tr.pauseAts) + len(h.tr.resumeAts)
	h.tr.mu.Unlock()
	for _, p := range []string{`{"a":1}`, `[1]`, `["x"]`, `{"gameId":1}`} {
		resp := h.call(t, method, p)
		if resp.Error == nil || resp.Error.Code != ipc.CodeInvalidParams {
			t.Errorf("%s params %s: response %+v (result %s), want -32602", method, p, resp.Error, resp.Result)
		}
	}
	h.tr.mu.Lock()
	after := len(h.tr.pauseAts) + len(h.tr.resumeAts)
	h.tr.mu.Unlock()
	if after != before {
		t.Errorf("%s: tracker called %d times with invalid params, want 0", method, after-before)
	}
}

func TestHandleSessionPause(t *testing.T) {
	t.Parallel()

	t.Run("IPC-038/session.pause em Running devolve {session} paused com activeMs congelado e asOf do pedido", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodSessionPause)
		h.clock.Advance(100 * time.Second)
		resp := h.call(t, ipc.MethodSessionPause, "")
		if resp.Error != nil {
			t.Fatalf("error %d %q %s", resp.Error.Code, resp.Error.Message, resp.Error.Data)
		}
		obj := objectOf(t, resp.Result)
		wantKeys(t, obj, "session")
		sess := objectOf(t, obj["session"])
		wantKeys(t, sess, sessionKeys...)
		wantRaw(t, sess, "state", `"paused"`)
		wantRaw(t, sess, "activeMs", "100000")
		wantRaw(t, sess, "activeSeconds", "100")
		wantRaw(t, sess, "asOf", `"2026-11-09T14:01:40.000Z"`)
		wantRaw(t, sess, "endedAt", "null")
	})

	t.Run("IPC-038/em Paused devolve o estado atual como sucesso", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodSessionPause)
		h.tr.pauseFn = func(_ context.Context, at time.Time) (domain.SessionSnapshot, error) {
			// Already paused (e.g. by sleep): the tracker reports the current
			// state as of the request.
			return domain.SessionSnapshot{
				ID: 3, GameID: 1, Title: "G", State: domain.SessionPaused, StartedAt: gamesT0,
				Active: 100 * time.Second, AsOf: at,
			}, nil
		}
		h.clock.Advance(130 * time.Second)
		sess := objectOf(t, objectOf(t, h.call(t, ipc.MethodSessionPause, "").Result)["session"])
		wantRaw(t, sess, "state", `"paused"`)
		wantRaw(t, sess, "activeMs", "100000")
		wantRaw(t, sess, "asOf", `"2026-11-09T14:02:10.000Z"`)
	})

	t.Run("IPC-038/params não vazios devolvem -32602 e params vazios ({}, [], null, ausente) são aceitos", func(t *testing.T) {
		t.Parallel()
		explicitParamsRules(t, ipc.MethodSessionPause)
	})

	t.Run("SES-035/o handler usa clock.Now() lido ao receber como at", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodSessionPause)
		h.clock.Advance(100 * time.Second)
		resultInto(t, h.call(t, ipc.MethodSessionPause, ""), &ipc.SessionResult{})
		h.clock.Advance(7 * time.Second)
		resultInto(t, h.call(t, ipc.MethodSessionPause, ""), &ipc.SessionResult{})
		h.tr.mu.Lock()
		defer h.tr.mu.Unlock()
		want := []time.Time{gamesT0.Add(100 * time.Second), gamesT0.Add(107 * time.Second)}
		if len(h.tr.pauseAts) != 2 || !h.tr.pauseAts[0].Equal(want[0]) || !h.tr.pauseAts[1].Equal(want[1]) {
			t.Errorf("Pause at = %v, want %v", h.tr.pauseAts, want)
		}
		if len(h.tr.resumeAts) != 0 {
			t.Errorf("Resume calls = %d, want 0", len(h.tr.resumeAts))
		}
	})

	t.Run("IPC-038/ctx do Pause tem prazo de 5 s", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodSessionPause)
		resultInto(t, h.call(t, ipc.MethodSessionPause, ""), &ipc.SessionResult{})
		h.tr.mu.Lock()
		dl := h.tr.pauseDeadlines[0]
		h.tr.mu.Unlock()
		if pauseHandlerTimeout != 5*time.Second {
			t.Errorf("pauseHandlerTimeout = %v, want 5s", pauseHandlerTimeout)
		}
		if dl.IsZero() {
			t.Fatalf("Pause ctx has no deadline, want %v", pauseHandlerTimeout)
		}
		if left := time.Until(dl); left > pauseHandlerTimeout || left < pauseHandlerTimeout-3*time.Second {
			t.Errorf("deadline in %v, want about %v", left, pauseHandlerTimeout)
		}
	})
}

func TestHandleSessionResume(t *testing.T) {
	t.Parallel()

	t.Run("IPC-039/session.resume em Paused devolve {session} running", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodSessionResume)
		h.clock.Advance(160 * time.Second)
		resp := h.call(t, ipc.MethodSessionResume, "")
		if resp.Error != nil {
			t.Fatalf("error %d %q %s", resp.Error.Code, resp.Error.Message, resp.Error.Data)
		}
		obj := objectOf(t, resp.Result)
		wantKeys(t, obj, "session")
		sess := objectOf(t, obj["session"])
		wantKeys(t, sess, sessionKeys...)
		wantRaw(t, sess, "state", `"running"`)
		wantRaw(t, sess, "endedAt", "null")
		wantRaw(t, sess, "asOf", `"2026-11-09T14:02:40.000Z"`)
	})

	t.Run("IPC-039/em Running devolve o estado atual como sucesso", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodSessionResume)
		h.tr.resumeFn = func(_ context.Context, at time.Time) (domain.SessionSnapshot, error) {
			return domain.SessionSnapshot{
				ID: 3, GameID: 1, Title: "G", State: domain.SessionRunning, StartedAt: gamesT0,
				Active: 42 * time.Second, AsOf: at,
			}, nil
		}
		h.clock.Advance(50 * time.Second)
		sess := objectOf(t, objectOf(t, h.call(t, ipc.MethodSessionResume, "").Result)["session"])
		wantRaw(t, sess, "state", `"running"`)
		wantRaw(t, sess, "activeMs", "42000")
		wantRaw(t, sess, "asOf", `"2026-11-09T14:00:50.000Z"`)
	})

	t.Run("IPC-039/mesma regra de params de IPC-038", func(t *testing.T) {
		t.Parallel()
		explicitParamsRules(t, ipc.MethodSessionResume)
	})

	t.Run("SES-036/o handler usa clock.Now() lido ao receber como at", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodSessionResume)
		h.clock.Advance(160 * time.Second)
		resultInto(t, h.call(t, ipc.MethodSessionResume, ""), &ipc.SessionResult{})
		h.clock.Advance(3 * time.Second)
		resultInto(t, h.call(t, ipc.MethodSessionResume, ""), &ipc.SessionResult{})
		h.tr.mu.Lock()
		defer h.tr.mu.Unlock()
		want := []time.Time{gamesT0.Add(160 * time.Second), gamesT0.Add(163 * time.Second)}
		if len(h.tr.resumeAts) != 2 || !h.tr.resumeAts[0].Equal(want[0]) || !h.tr.resumeAts[1].Equal(want[1]) {
			t.Errorf("Resume at = %v, want %v", h.tr.resumeAts, want)
		}
		if len(h.tr.pauseAts) != 0 {
			t.Errorf("Pause calls = %d, want 0", len(h.tr.pauseAts))
		}
	})

	t.Run("IPC-039/ctx do Resume tem prazo de 5 s", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodSessionResume)
		resultInto(t, h.call(t, ipc.MethodSessionResume, ""), &ipc.SessionResult{})
		h.tr.mu.Lock()
		dl := h.tr.resumeDeadlines[0]
		h.tr.mu.Unlock()
		if dl.IsZero() {
			t.Fatalf("Resume ctx has no deadline, want %v", pauseHandlerTimeout)
		}
		if left := time.Until(dl); left > pauseHandlerTimeout || left < pauseHandlerTimeout-3*time.Second {
			t.Errorf("deadline in %v, want about %v", left, pauseHandlerTimeout)
		}
	})
}

func TestHandleSessionNotPausable(t *testing.T) {
	t.Parallel()

	for _, em := range explicitMethods {
		for _, reason := range []string{ipc.ReasonNoSession, ipc.ReasonGrace} {
			name := "IPC-040/sem sessão devolve -32005 com data.reason no-session (pause e resume)"
			if reason == ipc.ReasonGrace {
				name = "IPC-040/em Grace devolve -32005 com data.reason grace"
			}
			t.Run(name+"/"+em.method, func(t *testing.T) {
				t.Parallel()
				h := newGamesHarness(t, nil)
				h.need(t, em.method)
				em.set(h.tr, func(context.Context, time.Time) (domain.SessionSnapshot, error) {
					return domain.SessionSnapshot{}, fmt.Errorf("tracker: %w", &tracker.NotPausableError{Reason: reason})
				})
				data := wantError(t, h.call(t, em.method, ""), ipc.CodeSessionNotPausable, ipc.MsgSessionNotPausable)
				wantKeys(t, data, "reason")
				wantRaw(t, data, "reason", mustJSON(t, reason))
				if h.logs.count(slog.LevelError) != 0 {
					t.Errorf("Error records logged for a refused request, want none")
				}
			})
		}

		t.Run("SES-037/o erro não grava nem notifica/"+em.method, func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			h.need(t, em.method)
			em.set(h.tr, func(context.Context, time.Time) (domain.SessionSnapshot, error) {
				return domain.SessionSnapshot{}, &tracker.NotPausableError{Reason: tracker.ReasonGrace}
			})
			wantError(t, h.call(t, em.method, ""), ipc.CodeSessionNotPausable, "")
			// The handler only asks the tracker (which owns write and notify):
			// one call, no launch, no store write.
			h.tr.mu.Lock()
			calls := len(em.ats(h.tr))
			launches := len(h.tr.specs) + len(h.tr.untrackedSpecs)
			h.tr.mu.Unlock()
			if calls != 1 || launches != 0 {
				t.Errorf("tracker calls = %d (launches %d), want 1 and 0", calls, launches)
			}
			if n := len(h.gs.addedGames()); n != 0 {
				t.Errorf("AddGame calls = %d, want 0", n)
			}
		})

		t.Run("IPC-040/ErrShuttingDown devolve -32603 e prazo de 5 s esgotado devolve -32603/"+em.method, func(t *testing.T) {
			t.Parallel()
			h := newGamesHarness(t, nil)
			h.need(t, em.method)

			em.set(h.tr, func(context.Context, time.Time) (domain.SessionSnapshot, error) {
				return domain.SessionSnapshot{}, fmt.Errorf("tracker: %w", tracker.ErrShuttingDown)
			})
			wantError(t, h.call(t, em.method, ""), ipc.CodeInternalError, "Internal error")
			if h.logs.count(slog.LevelDebug) == 0 {
				t.Errorf("no Debug record logged for ErrShuttingDown")
			}
			if n := h.logs.count(slog.LevelError); n != 0 {
				t.Errorf("Error records for ErrShuttingDown = %d, want 0", n)
			}

			em.set(h.tr, func(context.Context, time.Time) (domain.SessionSnapshot, error) {
				return domain.SessionSnapshot{}, fmt.Errorf("tracker: %w", context.DeadlineExceeded)
			})
			wantError(t, h.call(t, em.method, ""), ipc.CodeInternalError, "Internal error")
			if h.logs.count(slog.LevelError) == 0 {
				t.Errorf("no Error record logged for the ctx expiry")
			}

			// The end of the dispatch ctx while the tracker waits.
			em.set(h.tr, func(ctx context.Context, _ time.Time) (domain.SessionSnapshot, error) {
				<-ctx.Done()
				return domain.SessionSnapshot{}, ctx.Err()
			})
			ctx, cancel := context.WithCancel(t.Context())
			time.AfterFunc(10*time.Millisecond, cancel)
			defer cancel()
			wantError(t, h.callCtx(t, ctx, em.method, ""), ipc.CodeInternalError, "Internal error")
		})
	}
}
