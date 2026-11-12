package agent

// Tests and helpers of T18 and T19 (docs/specs/phase-5-library/tasks.md): game.add
// with the v0.6.0 Game shape [IPC-020] and the helpers of the game.list tests in
// games_test.go (TestGameList): raw responses for the 1 MiB budget [IPC-051], a
// ListGames stub that pages over a fixed slice, and the worst-case games. T19:
// game.get [IPC-044] and session.list [IPC-047]. T20: game.update [IPC-045,
// IPC-050, IPC-052] and its EDT-001..EDT-009 mapping, in TestGameUpdate (handler
// level) and TestGameUpdateNotifications (full agent with two clients). T21:
// game.delete [IPC-046, IPC-050, DEL-001..DEL-006] in TestGameDelete (handler
// level, with a deleteTracker that answers WithoutActiveGame) and
// TestGameDeleteNotifications (full agent with the real tracker). T22: platform.list,
// platform.add and library.catalog [IPC-048, IPC-049, IPC-050, EDT-008, EDT-009] in
// TestPlatformAndCatalog (handler level) and TestPlatformAndCatalogNotifications
// (full agent with two clients).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
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

// rawListResponse dispatches game.list with the raw params ("" = no params
// member) and returns the encoded response without the trailing '\n'
// [IPC-051].
func rawListResponse(t *testing.T, h *gamesHarness, params string) []byte {
	t.Helper()
	msg := `{"jsonrpc":"2.0","id":1,"method":"game.list"`
	if params != "" {
		msg += `,"params":` + params
	}
	msg += "}"
	raw, ok := ipc.Dispatch(t.Context(), h.handlers, []byte(msg))
	if !ok {
		t.Fatalf("Dispatch(%s) returned no response", msg)
	}
	return bytes.TrimSuffix(raw, []byte("\n"))
}

// listResultOf decodes a successful game.list response.
func listResultOf(t *testing.T, raw []byte) ipc.GameListResult {
	t.Helper()
	var resp ipc.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var list ipc.GameListResult
	resultInto(t, resp, &list)
	return list
}

// pagedStore is a ListGames stub over all: it honours offset and limit and
// reports len(all) as the total [LIB-002].
func pagedStore(all []store.Game) func(context.Context, store.Query) (store.Page, error) {
	return func(_ context.Context, q store.Query) (store.Page, error) {
		lo := min(q.Offset, len(all))
		hi := min(lo+q.Limit, len(all))
		return store.Page{Games: append([]store.Game{}, all[lo:hi]...), Total: int64(len(all))}, nil
	}
}

// worstGames returns n games with the given texts and every field game.list
// sends filled in, so each item is as large as the content allows.
func worstGames(n int, title, path, platform string) []store.Game {
	first := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	last := time.Date(2026, 11, 8, 22, 30, 0, 0, time.UTC)
	out := make([]store.Game, n)
	for i := range out {
		out[i] = store.Game{
			ID:               int64(i + 1),
			Title:            title,
			PlatformID:       1,
			Platform:         platform,
			LaunchType:       "exe",
			LaunchTarget:     path,
			Status:           domain.StatusPlaying,
			Favorite:         true,
			ReleaseDate:      "2009-05-01",
			TotalPlaySeconds: 1 << 40,
			PlayCount:        1 << 20,
			FirstPlayedAt:    first,
			LastPlayedAt:     last,
		}
	}
	return out
}

// TestGameAddShape checks the Game that game.add returns [IPC-020].
func TestGameAddShape(t *testing.T) {
	t.Parallel()

	t.Run("IPC-020/game.add devolve Game com platformId, platform, status, favorite e year (null quando não há data)", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		resp := h.call(t, ipc.MethodGameAdd, mustJSON(t, map[string]string{"path": h.file(t, "game.exe")}))
		if resp.Error != nil {
			t.Fatalf("error %d %q", resp.Error.Code, resp.Error.Message)
		}
		obj := objectOf(t, resp.Result)
		wantKeys(t, obj, gameKeys...)
		wantRaw(t, obj, "platform", `"PC"`)
		wantRaw(t, obj, "status", `"not_played"`)
		wantRaw(t, obj, "favorite", "false")
		wantRaw(t, obj, "year", "null")

		var g ipc.Game
		resultInto(t, resp, &g)
		if g.PlatformID < 1 {
			t.Errorf("platformId = %d, want the id of the PC platform (>= 1)", g.PlatformID)
		}
		stored, err := h.st.Game(t.Context(), g.ID)
		if err != nil {
			t.Fatalf("store.Game(%d): %v", g.ID, err)
		}
		if stored.PlatformID != g.PlatformID {
			t.Errorf("stored platformId = %d, want %d", stored.PlatformID, g.PlatformID)
		}
	})
}

// gameDetailKeys are the members GameDetail adds to Game [STO-026, IPC-044].
var gameDetailKeys = []string{"description", "completedAt", "developers", "publishers", "genres", "playModes", "launchType", "launchArgs"}

// sessionItemKeys are the members of SessionItem [STO-027, IPC-047].
var sessionItemKeys = []string{"sessionId", "gameId", "state", "startedAt", "endedAt", "activeSeconds", "endReason"}

// GameDetail and GameSessions forward to the real store. T19 adds them to
// gameStore, so the recording wrapper must have them as well.
func (s *recordingGameStore) GameDetail(ctx context.Context, id int64) (store.GameDetail, error) {
	return s.inner.GameDetail(ctx, id)
}

func (s *recordingGameStore) GameSessions(ctx context.Context, gameID int64, offset, limit int) (store.SessionPage, error) {
	return s.inner.GameSessions(ctx, gameID, offset, limit)
}

// UpdateGame forwards to the real store. T20 adds it to gameStore.
func (s *recordingGameStore) UpdateGame(ctx context.Context, id int64, p store.GamePatch, at time.Time) (store.GameDetail, error) {
	return s.inner.UpdateGame(ctx, id, p, at)
}

// Platforms, AddPlatform and Catalog forward to the real store. T22 adds them to
// gameStore [IPC-048, IPC-049].
func (s *recordingGameStore) Platforms(ctx context.Context) ([]store.Platform, error) {
	return s.inner.Platforms(ctx)
}

func (s *recordingGameStore) AddPlatform(ctx context.Context, name string, category domain.PlatformCategory) (store.Platform, error) {
	return s.inner.AddPlatform(ctx, name, category)
}

func (s *recordingGameStore) Catalog(ctx context.Context) (store.Catalog, error) {
	return s.inner.Catalog(ctx)
}

// seededSession describes a session written through the store: it starts at
// gamesT0+start and, unless open, ends after active. domain.Outcome decides
// whether it is ended or discarded [STO-012, STO-014].
type seededSession struct {
	start  time.Duration
	active time.Duration
	open   bool
}

// seedSessions writes the sessions of gameID through the store, in order, and
// returns their ids.
func (h *gamesHarness) seedSessions(t *testing.T, gameID int64, sessions ...seededSession) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(sessions))
	for _, s := range sessions {
		at := gamesT0.Add(s.start)
		id, err := h.st.StartSession(t.Context(), gameID, at)
		if err != nil {
			t.Fatalf("StartSession(game %d): %v", gameID, err)
		}
		if !s.open {
			end := at.Add(s.active)
			if _, err := h.st.EndSession(t.Context(), store.SessionEnd{
				ID: id, EndedAt: end, Active: s.active, Reason: domain.EndExited, WrittenAt: end,
			}); err != nil {
				t.Fatalf("EndSession(%d): %v", id, err)
			}
		}
		ids = append(ids, id)
	}
	return ids
}

// rawGetResponse dispatches game.get for gameID and returns the encoded response
// without the trailing '\n' [IPC-051].
func rawGetResponse(t *testing.T, h *gamesHarness, gameID int64) []byte {
	t.Helper()
	msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":{"gameId":%d}}`, ipc.MethodGameGet, gameID)
	raw, ok := ipc.Dispatch(t.Context(), h.handlers, []byte(msg))
	if !ok {
		t.Fatalf("Dispatch(%s) returned no response", msg)
	}
	return bytes.TrimSuffix(raw, []byte("\n"))
}

// wantEachInvalidParams checks that method refuses each raw params with -32602
// [IPC-028, IPC-043]. "" means no params member.
func wantEachInvalidParams(t *testing.T, h *gamesHarness, method string, params ...string) {
	t.Helper()
	for _, p := range params {
		resp := h.call(t, method, p)
		if resp.Error == nil || resp.Error.Code != ipc.CodeInvalidParams {
			t.Errorf("%s with params %q: result %s, error %+v; want -32602", method, p, resp.Result, resp.Error)
		}
	}
}

// sessionIDs returns the session ids in order.
func sessionIDs(items []ipc.SessionItem) []int64 {
	ids := make([]int64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.SessionID)
	}
	return ids
}

func TestGameGet(t *testing.T) {
	t.Parallel()

	t.Run("IPC-044/gameId existente devolve game com o detalhe completo", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		h.seedSessions(t, g.ID,
			seededSession{start: 0, active: 100 * time.Second},
			seededSession{start: time.Hour, active: 10 * time.Second}, // discarded: not counted
			seededSession{start: 2 * time.Hour, active: 60 * time.Second},
		)
		completed := ptr(time.Date(2026, time.November, 12, 0, 0, 0, 0, time.UTC))
		year := ptr(1998)
		description := "Metroidvania"
		developers := []string{"Team Cherry"}
		genres := []string{"Action", "Metroidvania"}
		modes := []domain.PlayMode{domain.PlayModeSingle}
		if _, err := h.st.UpdateGame(t.Context(), g.ID, store.GamePatch{
			Description: &description, CompletedAt: &completed, Year: &year,
			Developers: &developers, Genres: &genres, PlayModes: &modes,
		}, gamesT0); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}

		resp := h.call(t, ipc.MethodGameGet, mustJSON(t, map[string]int64{"gameId": g.ID}))
		if resp.Error != nil {
			t.Fatalf("error %d %q (data %s)", resp.Error.Code, resp.Error.Message, resp.Error.Data)
		}
		top := objectOf(t, resp.Result)
		wantKeys(t, top, "game")
		obj := objectOf(t, top["game"])
		wantKeys(t, obj, append(append([]string{}, gameKeys...), gameDetailKeys...)...)
		wantRaw(t, obj, "id", fmt.Sprint(g.ID))
		wantRaw(t, obj, "title", `"Hollow Knight"`)
		wantRaw(t, obj, "totalPlaySeconds", "160")
		wantRaw(t, obj, "playCount", "2")
		wantRaw(t, obj, "year", "1998")
		wantRaw(t, obj, "description", `"Metroidvania"`)
		wantRaw(t, obj, "completedAt", `"2026-11-12"`)
		wantRaw(t, obj, "developers", `["Team Cherry"]`)
		wantRaw(t, obj, "publishers", "[]")
		wantRaw(t, obj, "genres", `["Action","Metroidvania"]`)
		wantRaw(t, obj, "playModes", `["single"]`)
	})

	t.Run("IPC-044/gameId inexistente é -32001", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		wantError(t, h.call(t, ipc.MethodGameGet, `{"gameId":999}`), ipc.CodeGameNotFound, ipc.MsgGameNotFound)
	})

	t.Run("IPC-044/gameId 0, texto ou params ausentes são -32602", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		wantEachInvalidParams(t, h, ipc.MethodGameGet,
			"", `{}`, `{"gameId":0}`, `{"gameId":-1}`, `{"gameId":"1"}`, `[1]`)
	})

	t.Run("IPC-051/detalhe com descrição de 10 000 caracteres e 50 nomes de vínculo cabe no limite de 1 MiB", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), strings.Repeat("T", ipc.MaxGameTitleRunes))
		// Each name has the maximum of domain.MaxNameRunes runes; the index keeps them distinct.
		names := func(prefix string) []string {
			out := make([]string, 50)
			for i := range out {
				out[i] = fmt.Sprintf("%s%02d", strings.Repeat(prefix, domain.MaxNameRunes-2), i)
			}
			return out
		}
		description := strings.Repeat("é", domain.MaxDescriptionRunes)
		developers, publishers, genres := names("D"), names("P"), names("G")
		if _, err := h.st.UpdateGame(t.Context(), g.ID, store.GamePatch{
			Description: &description, Developers: &developers, Publishers: &publishers, Genres: &genres,
		}, gamesT0); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}

		raw := rawGetResponse(t, h, g.ID)
		if len(raw) > ipc.MaxMessageSize {
			t.Fatalf("game.get response is %d bytes, want <= %d", len(raw), ipc.MaxMessageSize)
		}
		var resp ipc.Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		var res ipc.GameDetailResult
		resultInto(t, resp, &res)
		if n := len([]rune(res.Game.Description)); n != domain.MaxDescriptionRunes {
			t.Errorf("description has %d runes, want %d", n, domain.MaxDescriptionRunes)
		}
		if len(res.Game.Developers) != 50 || len(res.Game.Publishers) != 50 || len(res.Game.Genres) != 50 {
			t.Errorf("links = %d developers, %d publishers, %d genres; want 50 each",
				len(res.Game.Developers), len(res.Game.Publishers), len(res.Game.Genres))
		}
	})
}

func TestSessionList(t *testing.T) {
	t.Parallel()

	t.Run("IPC-047/devolve sessions, total e offset na ordem de STO-027", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		ids := h.seedSessions(t, g.ID,
			seededSession{start: 0, active: 100 * time.Second},            // s1: ended
			seededSession{start: time.Hour, active: 5 * time.Second},      // s2: discarded
			seededSession{start: 2 * time.Hour, active: 60 * time.Second}, // s3: ended
			seededSession{start: 2 * time.Hour, open: true},               // s4: open, same start as s3, larger id
		)
		s1, s2, s3, s4 := ids[0], ids[1], ids[2], ids[3]

		resp := h.call(t, ipc.MethodSessionList, mustJSON(t, map[string]int64{"gameId": g.ID}))
		if resp.Error != nil {
			t.Fatalf("error %d %q (data %s)", resp.Error.Code, resp.Error.Message, resp.Error.Data)
		}
		var page ipc.SessionListResult
		resultInto(t, resp, &page)
		if want := []int64{s4, s3, s2, s1}; fmt.Sprint(sessionIDs(page.Sessions)) != fmt.Sprint(want) {
			t.Fatalf("session ids = %v, want %v (started_at desc, id desc)", sessionIDs(page.Sessions), want)
		}
		if page.Total != 4 || page.Offset != 0 {
			t.Errorf("total = %d, offset = %d; want 4 and 0", page.Total, page.Offset)
		}
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(objectOf(t, resp.Result)["sessions"], &items); err != nil {
			t.Fatalf("sessions: %v", err)
		}
		for _, it := range items {
			wantKeys(t, it, sessionItemKeys...)
		}

		type want struct {
			state  string
			active int64
			closed bool
		}
		wants := []want{
			{"running", 0, false},  // s4, open
			{"ended", 60, true},    // s3
			{"discarded", 5, true}, // s2
			{"ended", 100, true},   // s1
		}
		for i, w := range wants {
			it := page.Sessions[i]
			if it.State != w.state || it.ActiveSeconds != w.active {
				t.Errorf("session %d: state %q active %d; want %q and %d", it.SessionID, it.State, it.ActiveSeconds, w.state, w.active)
			}
			if closed := it.EndedAt != nil; closed != w.closed {
				t.Errorf("session %d: endedAt set = %v, want %v", it.SessionID, closed, w.closed)
			}
			if w.closed && (it.EndReason == nil || *it.EndReason != "exited") {
				t.Errorf("session %d: endReason = %v, want \"exited\"", it.SessionID, it.EndReason)
			}
			if !w.closed && it.EndReason != nil {
				t.Errorf("session %d: endReason = %q, want null while open", it.SessionID, *it.EndReason)
			}
		}

		// Paging: offset 1 and limit 2 give the second and third sessions; total still counts all four.
		resp = h.call(t, ipc.MethodSessionList, mustJSON(t, map[string]any{"gameId": g.ID, "offset": 1, "limit": 2}))
		resultInto(t, resp, &page)
		if want := []int64{s3, s2}; fmt.Sprint(sessionIDs(page.Sessions)) != fmt.Sprint(want) {
			t.Errorf("page session ids = %v, want %v", sessionIDs(page.Sessions), want)
		}
		if page.Total != 4 || page.Offset != 1 {
			t.Errorf("page total = %d, offset = %d; want 4 and 1", page.Total, page.Offset)
		}
	})

	t.Run("IPC-047/limit padrão 50; limit 201 e offset negativo são -32602", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		sessions := make([]seededSession, 51)
		for i := range sessions {
			sessions[i] = seededSession{start: time.Duration(i) * time.Minute, active: time.Minute}
		}
		h.seedSessions(t, g.ID, sessions...)

		var page ipc.SessionListResult
		resultInto(t, h.call(t, ipc.MethodSessionList, mustJSON(t, map[string]int64{"gameId": g.ID})), &page)
		if len(page.Sessions) != domain.DefaultSessionLimit || page.Total != 51 || page.Offset != 0 {
			t.Errorf("default page: %d sessions, total %d, offset %d; want %d, 51 and 0",
				len(page.Sessions), page.Total, page.Offset, domain.DefaultSessionLimit)
		}

		id := fmt.Sprint(g.ID)
		wantEachInvalidParams(t, h, ipc.MethodSessionList,
			`{"gameId":`+id+`,"limit":201}`, `{"gameId":`+id+`,"limit":0}`, `{"gameId":`+id+`,"offset":-1}`)
	})

	t.Run("IPC-047/gameId inexistente é -32001", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		wantError(t, h.call(t, ipc.MethodSessionList, `{"gameId":999}`), ipc.CodeGameNotFound, ipc.MsgGameNotFound)
	})

	t.Run("IPC-047/sessions vazio sai como [] e não null", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		obj := objectOf(t, h.call(t, ipc.MethodSessionList, fmt.Sprintf(`{"gameId":%d}`, g.ID)).Result)
		wantRaw(t, obj, "sessions", "[]")
		wantRaw(t, obj, "total", "0")
		wantRaw(t, obj, "offset", "0")
	})
}

// updateCall is one UpdateGame call seen by updateRecorder.
type updateCall struct {
	patch    store.GamePatch
	at       time.Time
	deadline time.Time // zero = the ctx had no deadline
}

// updateRecorder wraps the recording game store and records each UpdateGame
// call [IPC-045]. It forwards to the real store.
type updateRecorder struct {
	*recordingGameStore

	mu    sync.Mutex
	calls []updateCall
}

func (u *updateRecorder) UpdateGame(ctx context.Context, id int64, p store.GamePatch, at time.Time) (store.GameDetail, error) {
	dl, _ := ctx.Deadline()
	u.mu.Lock()
	u.calls = append(u.calls, updateCall{patch: p, at: at, deadline: dl})
	u.mu.Unlock()
	return u.recordingGameStore.inner.UpdateGame(ctx, id, p, at)
}

// recorded returns the UpdateGame calls so far, in order.
func (u *updateRecorder) recorded() []updateCall {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]updateCall(nil), u.calls...)
}

// newUpdateHarness is newGamesHarness whose game.update goes through an
// updateRecorder, so the tests see each UpdateGame call.
func newUpdateHarness(t *testing.T) (*gamesHarness, *updateRecorder) {
	t.Helper()
	h := newGamesHarness(t, nil)
	up := &updateRecorder{recordingGameStore: h.gs}
	h.handlers = gameHandlers(up, h.tr, h.clock, os.Stat, slog.New(h.logs))
	if h.handlers[ipc.MethodGameUpdate] == nil {
		t.Fatalf("gameHandlers has no handler for %q", ipc.MethodGameUpdate)
	}
	return h, up
}

// gameUpdate calls game.update for gameID with members, the JSON members of the
// patch without gameId ("" = only gameId) [IPC-045].
func gameUpdate(t *testing.T, h *gamesHarness, gameID int64, members string) ipc.Response {
	t.Helper()
	params := fmt.Sprintf(`{"gameId":%d`, gameID)
	if members != "" {
		params += "," + members
	}
	return h.call(t, ipc.MethodGameUpdate, params+"}")
}

// updatedGame returns the game member of a successful game.update or game.get
// response, and checks that it has no other member [IPC-045].
func updatedGame(t *testing.T, resp ipc.Response) map[string]json.RawMessage {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("error %d %q (data %s)", resp.Error.Code, resp.Error.Message, resp.Error.Data)
	}
	top := objectOf(t, resp.Result)
	wantKeys(t, top, "game")
	return objectOf(t, top["game"])
}

// getGame returns the game member that game.get gives for gameID.
func getGame(t *testing.T, h *gamesHarness, gameID int64) map[string]json.RawMessage {
	t.Helper()
	return updatedGame(t, h.call(t, ipc.MethodGameGet, fmt.Sprintf(`{"gameId":%d}`, gameID)))
}

// wantSameObject checks that got has exactly the members of want, with the same
// raw values.
func wantSameObject(t *testing.T, got, want map[string]json.RawMessage) {
	t.Helper()
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	wantKeys(t, got, keys...)
	for _, k := range keys {
		wantRaw(t, got, k, string(want[k]))
	}
}

// wantUpdateInvalid checks that each members patch of gameID is refused with
// -32602 [IPC-052, EDT-009].
func wantUpdateInvalid(t *testing.T, h *gamesHarness, gameID int64, members ...string) {
	t.Helper()
	for _, m := range members {
		resp := gameUpdate(t, h, gameID, m)
		if resp.Error == nil || resp.Error.Code != ipc.CodeInvalidParams {
			t.Errorf("game.update %q: result %s, error %+v; want -32602", m, resp.Result, resp.Error)
		}
	}
}

// wantGameUnchanged checks that game.get of gameID still gives before, the
// response that was read before a rejected update [EDT-007].
func wantGameUnchanged(t *testing.T, h *gamesHarness, gameID int64, before []byte) {
	t.Helper()
	if after := rawGetResponse(t, h, gameID); string(after) != string(before) {
		t.Errorf("game.get after the rejected update = %s, want %s", after, before)
	}
}

// catalogNames returns the company and genre names of the catalog, to check
// that a rejected patch creates none [EDT-007].
func catalogNames(t *testing.T, h *gamesHarness) string {
	t.Helper()
	cat, err := h.st.Catalog(t.Context())
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	return fmt.Sprint(cat.Companies, cat.Genres)
}

// countNotifications counts the messages without an id, that is the
// notifications [IPC-050].
func countNotifications(msgs []rawMsg) int {
	n := 0
	for _, m := range msgs {
		if m.method() != "" {
			n++
		}
	}
	return n
}

func TestGameUpdate(t *testing.T) {
	t.Parallel()

	t.Run("IPC-045/patch válido devolve game com o detalhe já alterado", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		obj := updatedGame(t, gameUpdate(t, h, g.ID,
			`"title":"Hades","favorite":true,"status":"playing","description":"Roguelike","completedAt":"2026-11-12",`+
				`"year":2020,"developers":["Supergiant"],"publishers":["Supergiant"],"genres":["Action","Roguelike"],"playModes":["single"]`))
		wantKeys(t, obj, append(append([]string{}, gameKeys...), gameDetailKeys...)...)
		wantRaw(t, obj, "id", fmt.Sprint(g.ID))
		wantRaw(t, obj, "title", `"Hades"`)
		wantRaw(t, obj, "favorite", "true")
		wantRaw(t, obj, "status", `"playing"`)
		wantRaw(t, obj, "year", "2020")
		wantRaw(t, obj, "description", `"Roguelike"`)
		wantRaw(t, obj, "completedAt", `"2026-11-12"`)
		wantRaw(t, obj, "developers", `["Supergiant"]`)
		wantRaw(t, obj, "publishers", `["Supergiant"]`)
		wantRaw(t, obj, "genres", `["Action","Roguelike"]`)
		wantRaw(t, obj, "playModes", `["single"]`)
		// The result is the detail that game.get reads afterwards.
		wantSameObject(t, obj, getGame(t, h, g.ID))
	})

	t.Run("IPC-045/gameId inexistente é -32001 e nada muda", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		wantError(t, gameUpdate(t, h, 999, `"title":"Fantasma"`), ipc.CodeGameNotFound, ipc.MsgGameNotFound)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("IPC-045/gameId 0, texto ou params ausentes são -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		wantEachInvalidParams(t, h, ipc.MethodGameUpdate,
			"", `{"title":"X"}`, `{"gameId":0,"title":"X"}`, `{"gameId":-1}`, `{"gameId":"1"}`, `{"gameId":1.5}`)
	})

	t.Run("IPC-045/patch inválido (title vazio) é -32602 e nada muda (EDT-007)", func(t *testing.T) {
		t.Parallel()
		h, up := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		wantError(t, gameUpdate(t, h, g.ID, `"title":""`), ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		wantGameUnchanged(t, h, g.ID, before)
		if n := len(up.recorded()); n != 0 {
			t.Errorf("UpdateGame calls = %d, want 0: the patch is checked before any write", n)
		}
	})

	t.Run("IPC-045/UpdateGame recebe clock.Now() lido ao receber o pedido", func(t *testing.T) {
		t.Parallel()
		h, up := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		h.clock.Advance(3 * time.Second)
		updatedGame(t, gameUpdate(t, h, g.ID, `"title":"Hades"`))
		calls := up.recorded()
		if len(calls) != 1 {
			t.Fatalf("UpdateGame calls = %d, want 1", len(calls))
		}
		if want := gamesT0.Add(3 * time.Second); !calls[0].at.Equal(want) {
			t.Errorf("UpdateGame at = %v, want %v (clock.Now at receipt)", calls[0].at, want)
		}
	})

	t.Run("IPC-045/UpdateGame tem prazo de 5 s", func(t *testing.T) {
		t.Parallel()
		h, up := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		updatedGame(t, gameUpdate(t, h, g.ID, `"title":"Hades"`))
		calls := up.recorded()
		if len(calls) != 1 || calls[0].deadline.IsZero() {
			t.Fatalf("UpdateGame calls = %+v, want one call with a deadline", calls)
		}
		if left := time.Until(calls[0].deadline); left > 5*time.Second || left < 4*time.Second {
			t.Errorf("UpdateGame ctx deadline in %v, want about 5 s", left)
		}
	})

	t.Run("EDT-007/title vazio junto com developers novos é -32602 e nenhum nome novo é criado", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		names := catalogNames(t, h)
		wantError(t, gameUpdate(t, h, g.ID, `"title":"  ","developers":["Empresa Nova"],"genres":["Gênero Novo"]`),
			ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		wantGameUnchanged(t, h, g.ID, before)
		if got := catalogNames(t, h); got != names {
			t.Errorf("catalog names = %s, want %s unchanged", got, names)
		}
	})

	t.Run("EDT-009/só gameId devolve o jogo sem alteração e com sucesso", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := getGame(t, h, g.ID)
		obj := updatedGame(t, gameUpdate(t, h, g.ID, ""))
		wantSameObject(t, obj, before)
	})

	t.Run("EDT-009/membro desconhecido é -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		wantUpdateInvalid(t, h, g.ID, `"nope":1`, `"title":"X","nope":null`, `"Title":"X"`, `"playmodes":["single"]`)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("EDT-004/playModes com nome fora de DOM-014 é -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		wantUpdateInvalid(t, h, g.ID, `"playModes":["party"]`, `"playModes":["Single"]`, `"playModes":["single","multi"]`, `"playModes":[""]`)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("EDT-004/developers com nomes repetidos sem diferenciar maiúsculas sai com uma só entrada", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		obj := updatedGame(t, gameUpdate(t, h, g.ID, `"developers":["Team Cherry"]`))
		wantRaw(t, obj, "developers", `["Team Cherry"]`)
		obj = updatedGame(t, gameUpdate(t, h, g.ID, `"developers":["Team Cherry","team cherry","TEAM CHERRY"]`))
		wantRaw(t, obj, "developers", `["Team Cherry"]`)
		// The company exists: another spelling in a link reuses the stored spelling.
		obj = updatedGame(t, gameUpdate(t, h, g.ID, `"publishers":["team cherry"]`))
		wantRaw(t, obj, "publishers", `["Team Cherry"]`)
		cat, err := h.st.Catalog(t.Context())
		if err != nil {
			t.Fatalf("Catalog: %v", err)
		}
		var companyNames []string
		for _, co := range cat.Companies {
			companyNames = append(companyNames, co.Name)
		}
		if fmt.Sprint(companyNames) != "[Team Cherry]" {
			t.Errorf("companies = %v, want one company Team Cherry", cat.Companies)
		}
	})

	t.Run("EDT-004/array vazio limpa developers, genres e playModes", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		updatedGame(t, gameUpdate(t, h, g.ID, `"developers":["Team Cherry"],"genres":["Action"],"playModes":["single"]`))
		obj := updatedGame(t, gameUpdate(t, h, g.ID, `"developers":[],"genres":[],"playModes":[]`))
		wantRaw(t, obj, "developers", "[]")
		wantRaw(t, obj, "genres", "[]")
		wantRaw(t, obj, "playModes", "[]")
	})

	t.Run("EDT-004/nome de catálogo vazio ou com 101 runas é -32602 sem criar nome", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		names := catalogNames(t, h)
		wantUpdateInvalid(t, h, g.ID,
			`"developers":["   "]`, `"genres":[""]`, `"publishers":["`+strings.Repeat("é", domain.MaxNameRunes+1)+`"]`)
		wantGameUnchanged(t, h, g.ID, before)
		if got := catalogNames(t, h); got != names {
			t.Errorf("catalog names = %s, want %s unchanged", got, names)
		}
	})

	t.Run("EDT-001/title de 200 runas aceito e 201 é -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		title := strings.Repeat("é", ipc.MaxGameTitleRunes)
		obj := updatedGame(t, gameUpdate(t, h, g.ID, `"title":"  `+title+`  "`))
		wantRaw(t, obj, "title", mustJSON(t, title))
		wantUpdateInvalid(t, h, g.ID, `"title":"`+strings.Repeat("é", ipc.MaxGameTitleRunes+1)+`"`)
	})

	t.Run("EDT-001/description de 10 000 runas aceita e 10 001 é -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		obj := updatedGame(t, gameUpdate(t, h, g.ID, `"description":"`+strings.Repeat("ç", domain.MaxDescriptionRunes)+`"`))
		var d string
		if err := json.Unmarshal(obj["description"], &d); err != nil {
			t.Fatalf("description: %v", err)
		}
		if n := len([]rune(d)); n != domain.MaxDescriptionRunes {
			t.Errorf("description has %d runes, want %d", n, domain.MaxDescriptionRunes)
		}
		wantUpdateInvalid(t, h, g.ID, `"description":"`+strings.Repeat("ç", domain.MaxDescriptionRunes+1)+`"`)
	})

	t.Run("EDT-001/status fora de DOM-012 é -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		wantUpdateInvalid(t, h, g.ID, `"status":"Playing"`, `"status":""`, `"status":"done"`)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("EDT-001/completedAt em outro formato que YYYY-MM-DD é -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		wantUpdateInvalid(t, h, g.ID, `"completedAt":"12/11/2026"`, `"completedAt":"2026-02-30"`,
			`"completedAt":"2026-11-12T00:00:00Z"`, `"completedAt":""`)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("EDT-002/platformId existente troca a plataforma e inexistente é -32602 e nada muda", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		sw, err := h.st.AddPlatform(t.Context(), "Switch", domain.CategoryConsole)
		if err != nil {
			t.Fatalf("AddPlatform: %v", err)
		}
		obj := updatedGame(t, gameUpdate(t, h, g.ID, fmt.Sprintf(`"platformId":%d`, sw.ID)))
		wantRaw(t, obj, "platformId", fmt.Sprint(sw.ID))
		wantRaw(t, obj, "platform", `"Switch"`)
		before := rawGetResponse(t, h, g.ID)
		wantUpdateInvalid(t, h, g.ID, `"platformId":999999`)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("EDT-003/year null limpa o ano e year fora de 1900..2100 é -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		obj := updatedGame(t, gameUpdate(t, h, g.ID, `"year":1900`))
		wantRaw(t, obj, "year", "1900")
		obj = updatedGame(t, gameUpdate(t, h, g.ID, `"year":2100`))
		wantRaw(t, obj, "year", "2100")
		before := rawGetResponse(t, h, g.ID)
		wantUpdateInvalid(t, h, g.ID, `"year":1899`, `"year":2101`)
		wantGameUnchanged(t, h, g.ID, before)
		obj = updatedGame(t, gameUpdate(t, h, g.ID, `"year":null`))
		wantRaw(t, obj, "year", "null")
	})

	t.Run("EDT-005/editar não muda total, contagem, sessões nem pausas", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		h.seedSessions(t, g.ID,
			seededSession{start: 0, active: 100 * time.Second},
			seededSession{start: time.Hour, active: 60 * time.Second},
		)
		beforeGame := getGame(t, h, g.ID)
		wantRaw(t, beforeGame, "totalPlaySeconds", "160")
		beforeSessions := h.call(t, ipc.MethodSessionList, fmt.Sprintf(`{"gameId":%d}`, g.ID)).Result
		obj := updatedGame(t, gameUpdate(t, h, g.ID,
			`"title":"Renomeado","favorite":true,"developers":["Team Cherry"],"genres":["Action"],"year":1998,`+
				`"completedAt":"2026-11-12","playModes":["single"]`))
		for _, k := range []string{"totalPlaySeconds", "playCount", "firstPlayedAt", "lastPlayedAt"} {
			wantRaw(t, obj, k, string(beforeGame[k]))
		}
		if got := h.call(t, ipc.MethodSessionList, fmt.Sprintf(`{"gameId":%d}`, g.ID)).Result; string(got) != string(beforeSessions) {
			t.Errorf("session.list = %s after the edit, want %s", got, beforeSessions)
		}
	})

	t.Run("IPC-052/completedAt null apaga e completedAt ausente não mexe", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		obj := updatedGame(t, gameUpdate(t, h, g.ID, `"completedAt":"2026-11-12"`))
		wantRaw(t, obj, "completedAt", `"2026-11-12"`)
		obj = updatedGame(t, gameUpdate(t, h, g.ID, `"title":"Renomeado"`))
		wantRaw(t, obj, "title", `"Renomeado"`)
		wantRaw(t, obj, "completedAt", `"2026-11-12"`)
		obj = updatedGame(t, gameUpdate(t, h, g.ID, `"completedAt":null`))
		wantRaw(t, obj, "completedAt", "null")
	})

	t.Run("IPC-052/null em campo que não aceita null é -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		wantUpdateInvalid(t, h, g.ID, `"title":null`, `"favorite":null`, `"status":null`, `"description":null`,
			`"platformId":null`, `"developers":null`, `"publishers":null`, `"genres":null`, `"playModes":null`)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("IPC-052/tipo errado em um campo é -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		wantUpdateInvalid(t, h, g.ID, `"favorite":"sim"`, `"year":"1998"`, `"year":1998.5`, `"platformId":"1"`,
			`"title":5`, `"status":1`, `"completedAt":20261112`, `"developers":"Team Cherry"`, `"description":false`)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("IPC-052/item de tipo errado em array é -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		wantUpdateInvalid(t, h, g.ID, `"genres":[1]`, `"developers":[null]`, `"playModes":[true]`, `"publishers":["Ok",2]`)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("IPC-052/params posicionais são -32602", func(t *testing.T) {
		t.Parallel()
		h, _ := newUpdateHarness(t)
		wantEachInvalidParams(t, h, ipc.MethodGameUpdate, `[1]`, `[]`, `[{"gameId":1}]`)
	})
}

// TestGameUpdateNotifications runs game.update in a full agent with two clients
// [IPC-050, EDT-006]. It is not parallel: newWiring checks for leaked goroutines.
func TestGameUpdateNotifications(t *testing.T) {
	t.Run("IPC-050/game.update não envia notificação", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c1, c2 := w.client(t), w.client(t)
		id, _ := w.addGame(t, c1, "Hollow Knight")
		c1.rpcOK(t, ipc.MethodStatus, nil)
		c2.rpcOK(t, ipc.MethodStatus, nil)
		before1, before2 := countNotifications(c1.snapshotAll()), countNotifications(c2.snapshotAll())

		c1.rpcOK(t, ipc.MethodGameUpdate, map[string]any{
			"gameId": id, "title": "Hades", "favorite": true, "developers": []string{"Team Cherry"},
		})
		// A round trip on each connection: anything queued before the response has arrived.
		c1.rpcOK(t, ipc.MethodStatus, nil)
		c2.rpcOK(t, ipc.MethodStatus, nil)
		if n := countNotifications(c1.snapshotAll()); n != before1 {
			t.Errorf("client 1 got %d notification(s) after game.update, want none", n-before1)
		}
		if n := countNotifications(c2.snapshotAll()); n != before2 {
			t.Errorf("client 2 got %d notification(s) after game.update, want none", n-before2)
		}
	})

	// checkUpdateKeepsSession launches the game (and pauses it when pause is set),
	// edits it through client 1 and checks that session.current and the
	// session.changed count of client 2 do not move [EDT-006].
	checkUpdateKeepsSession := func(t *testing.T, pause bool, state string) {
		w := newWiring(t, wiringOpts{})
		c1, c2 := w.client(t), w.client(t)
		id, _ := w.addGame(t, c1, "Hollow Knight")
		w.launch(t, c1, id)
		if pause {
			c1.rpcOK(t, ipc.MethodSessionPause, nil)
		}
		c2.rpcOK(t, ipc.MethodStatus, nil)
		cur := c1.rpcOK(t, ipc.MethodSessionCurrent, nil).Result
		wantRaw(t, objectOf(t, objectOf(t, cur)["session"]), "state", mustJSON(t, state))
		changes := countMethod(c2.snapshotAll(), ipc.MethodSessionChanged)

		c1.rpcOK(t, ipc.MethodGameUpdate, map[string]any{
			"gameId": id, "title": "Renomeado", "favorite": true, "playModes": []string{"single"},
		})
		c2.rpcOK(t, ipc.MethodStatus, nil)
		if got := c1.rpcOK(t, ipc.MethodSessionCurrent, nil).Result; string(got) != string(cur) {
			t.Errorf("session.current = %s after game.update, want %s", got, cur)
		}
		if n := countMethod(c2.snapshotAll(), ipc.MethodSessionChanged); n != changes {
			t.Errorf("session.changed count = %d after game.update, want %d", n, changes)
		}
	}

	t.Run("EDT-006/game.update com sessão Running não muda session.current nem envia session.changed", func(t *testing.T) {
		checkUpdateKeepsSession(t, false, "running")
	})

	t.Run("EDT-006/game.update com sessão Paused não muda session.current nem envia session.changed", func(t *testing.T) {
		checkUpdateKeepsSession(t, true, "paused")
	})
}

// deleteCall is one WithoutActiveGame call seen by deleteTracker.
type deleteCall struct {
	gameID   int64
	deadline time.Time // zero = the ctx had no deadline
}

// deleteTracker is the tracker of game.delete [DEL-002, DEL-003]. Its
// WithoutActiveGame refuses a game listed in active with
// *tracker.SessionActiveError without calling fn; any other game runs fn and
// returns its error. hook, when set, replaces that behaviour. Launches come
// from the embedded fakeTracker.
type deleteTracker struct {
	*fakeTracker

	mu     sync.Mutex
	active map[int64]domain.SessionSnapshot
	hook   func(ctx context.Context, gameID int64, fn func() error) error
	calls  []deleteCall
}

func (d *deleteTracker) WithoutActiveGame(ctx context.Context, gameID int64, fn func() error) error {
	dl, _ := ctx.Deadline()
	d.mu.Lock()
	d.calls = append(d.calls, deleteCall{gameID: gameID, deadline: dl})
	snap, busy := d.active[gameID]
	hook := d.hook
	d.mu.Unlock()
	if hook != nil {
		return hook(ctx, gameID, fn)
	}
	if busy {
		return &tracker.SessionActiveError{Session: snap}
	}
	return fn()
}

// recordedCalls returns the WithoutActiveGame calls so far, in order.
func (d *deleteTracker) recordedCalls() []deleteCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]deleteCall(nil), d.calls...)
}

// deleteStore is the recording game store whose DeleteGame can fail before it
// writes anything [DEL-006].
type deleteStore struct {
	*recordingGameStore
	fail error
}

func (s *deleteStore) DeleteGame(ctx context.Context, id int64) error {
	if s.fail != nil {
		return s.fail
	}
	return s.recordingGameStore.inner.DeleteGame(ctx, id)
}

// newDeleteHarness is newGamesHarness whose game.delete goes through a
// deleteTracker and a deleteStore.
func newDeleteHarness(t *testing.T) (*gamesHarness, *deleteTracker, *deleteStore) {
	t.Helper()
	h := newGamesHarness(t, nil)
	dt := &deleteTracker{fakeTracker: h.tr, active: map[int64]domain.SessionSnapshot{}}
	ds := &deleteStore{recordingGameStore: h.gs}
	h.handlers = gameHandlers(ds, dt, h.clock, os.Stat, slog.New(h.logs))
	h.need(t, ipc.MethodGameDelete)
	return h, dt, ds
}

// deleteGame calls game.delete for gameID [IPC-046].
func deleteGame(t *testing.T, h *gamesHarness, gameID int64) ipc.Response {
	t.Helper()
	return h.call(t, ipc.MethodGameDelete, fmt.Sprintf(`{"gameId":%d}`, gameID))
}

// deleteOK calls game.delete for gameID, requires success and checks that the
// result has no member but gameId [IPC-046].
func deleteOK(t *testing.T, h *gamesHarness, gameID int64) {
	t.Helper()
	resp := deleteGame(t, h, gameID)
	if resp.Error != nil {
		t.Fatalf("game.delete(%d): error %d %q (data %s)", gameID, resp.Error.Code, resp.Error.Message, resp.Error.Data)
	}
	obj := objectOf(t, resp.Result)
	wantKeys(t, obj, "gameId")
	wantRaw(t, obj, "gameId", fmt.Sprint(gameID))
}

// wantGameRemoved checks that the store has no game gameID [DEL-001].
func wantGameRemoved(t *testing.T, h *gamesHarness, gameID int64) {
	t.Helper()
	if _, err := h.st.Game(t.Context(), gameID); !errors.Is(err, store.ErrGameNotFound) {
		t.Errorf("store.Game(%d) after game.delete: %v, want ErrGameNotFound", gameID, err)
	}
}

// sessionsRaw returns the session.list result of gameID, so a test can check
// that no session changed [DEL-002, DEL-003].
func sessionsRaw(t *testing.T, h *gamesHarness, gameID int64) string {
	t.Helper()
	return string(h.call(t, ipc.MethodSessionList, fmt.Sprintf(`{"gameId":%d}`, gameID)).Result)
}

// wantSessionRef checks a -32002 response whose data is a SessionRef [IPC-046,
// DEL-002].
func wantSessionRef(t *testing.T, resp ipc.Response, sessionID, gameID int64, title string) {
	t.Helper()
	data := wantError(t, resp, ipc.CodeSessionActive, ipc.MsgSessionActive)
	wantKeys(t, data, "sessionId", "gameId", "title")
	wantRaw(t, data, "sessionId", fmt.Sprint(sessionID))
	wantRaw(t, data, "gameId", fmt.Sprint(gameID))
	wantRaw(t, data, "title", mustJSON(t, title))
}

func TestGameDelete(t *testing.T) {
	t.Parallel()

	t.Run("IPC-046/gameId sem sessão devolve {gameId} e o jogo sai do banco", func(t *testing.T) {
		t.Parallel()
		h, dt, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		other := h.addGame(t, h.file(t, "other.exe"), "Outro")
		before := rawGetResponse(t, h, other.ID)

		deleteOK(t, h, g.ID)
		wantGameRemoved(t, h, g.ID)
		wantGameUnchanged(t, h, other.ID, before)
		calls := dt.recordedCalls()
		if len(calls) != 1 || calls[0].gameID != g.ID {
			t.Errorf("WithoutActiveGame calls = %+v, want one call for game %d", calls, g.ID)
		}
	})

	t.Run("IPC-046/gameId inexistente é -32001 e nada muda", func(t *testing.T) {
		t.Parallel()
		h, _, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		data := wantError(t, deleteGame(t, h, 999), ipc.CodeGameNotFound, ipc.MsgGameNotFound)
		wantRaw(t, data, "gameId", "999")
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("IPC-046/gameId 0, texto ou params ausentes são -32602", func(t *testing.T) {
		t.Parallel()
		h, _, _ := newDeleteHarness(t)
		wantEachInvalidParams(t, h, ipc.MethodGameDelete,
			"", `{}`, `{"gameId":0}`, `{"gameId":-1}`, `{"gameId":"1"}`, `{"gameId":1.5}`, `[1]`)
	})

	t.Run("IPC-046/game.delete tem prazo de 5 s na espera do WithoutActiveGame", func(t *testing.T) {
		t.Parallel()
		h, dt, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		deleteOK(t, h, g.ID)
		calls := dt.recordedCalls()
		if len(calls) != 1 || calls[0].deadline.IsZero() {
			t.Fatalf("WithoutActiveGame calls = %+v, want one call with a deadline", calls)
		}
		if left := time.Until(calls[0].deadline); left > 5*time.Second || left < 4*time.Second {
			t.Errorf("WithoutActiveGame ctx deadline in %v, want about 5 s", left)
		}
	})

	t.Run("IPC-046/agente encerrando devolve -32603 e nada muda", func(t *testing.T) {
		t.Parallel()
		h, dt, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		dt.hook = func(context.Context, int64, func() error) error {
			return fmt.Errorf("tracker: %w", tracker.ErrShuttingDown)
		}
		wantError(t, deleteGame(t, h, g.ID), ipc.CodeInternalError, "Internal error")
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("IPC-046/prazo vencido antes de a dona pegar o pedido devolve -32603 e nada muda", func(t *testing.T) {
		t.Parallel()
		h, dt, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		before := rawGetResponse(t, h, g.ID)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		// The deadline passes before the tracker's owner takes the request: fn does not run.
		dt.hook = func(c context.Context, _ int64, _ func() error) error {
			cancel()
			return c.Err()
		}
		wantError(t, h.callCtx(t, ctx, ipc.MethodGameDelete, fmt.Sprintf(`{"gameId":%d}`, g.ID)),
			ipc.CodeInternalError, "Internal error")
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("IPC-046/remoção começada e prazo vencido: o agente espera o resultado e responde sucesso (decisão 7)", func(t *testing.T) {
		t.Parallel()
		h, dt, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		// The removal starts with the live ctx; the deadline passes while it runs.
		dt.hook = func(_ context.Context, _ int64, fn func() error) error {
			err := fn()
			cancel()
			return err
		}
		resp := h.callCtx(t, ctx, ipc.MethodGameDelete, fmt.Sprintf(`{"gameId":%d}`, g.ID))
		if resp.Error != nil {
			t.Fatalf("game.delete with the removal applied: error %d %q (data %s)", resp.Error.Code, resp.Error.Message, resp.Error.Data)
		}
		wantKeys(t, objectOf(t, resp.Result), "gameId")
		wantGameRemoved(t, h, g.ID)
	})

	t.Run("IPC-046/jogo removido por outro caminho antes da remoção devolve -32001", func(t *testing.T) {
		t.Parallel()
		h, dt, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		dt.hook = func(c context.Context, gameID int64, fn func() error) error {
			if err := h.st.DeleteGame(c, gameID); err != nil {
				t.Errorf("DeleteGame before the handler's removal: %v", err)
			}
			return fn()
		}
		wantError(t, deleteGame(t, h, g.ID), ipc.CodeGameNotFound, ipc.MsgGameNotFound)
	})

	t.Run("DEL-002/sessão ativa do jogo devolve -32002 com SessionRef (sessionId, gameId, title) e nada muda", func(t *testing.T) {
		t.Parallel()
		h, dt, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		ids := h.seedSessions(t, g.ID, seededSession{start: 0, open: true})
		dt.active[g.ID] = domain.SessionSnapshot{
			ID: ids[0], GameID: g.ID, Title: g.Title, State: domain.SessionRunning, StartedAt: gamesT0, AsOf: gamesT0,
		}
		before := rawGetResponse(t, h, g.ID)
		sessions := sessionsRaw(t, h, g.ID)
		wantSessionRef(t, deleteGame(t, h, g.ID), ids[0], g.ID, g.Title)
		wantGameUnchanged(t, h, g.ID, before)
		if got := sessionsRaw(t, h, g.ID); got != sessions {
			t.Errorf("session.list = %s after the refused removal, want %s", got, sessions)
		}
	})

	t.Run("DEL-002/lançamento pendente do jogo devolve -32002 com sessionId 0", func(t *testing.T) {
		t.Parallel()
		h, dt, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		// A tracked launch still being prepared has no session id yet.
		dt.active[g.ID] = domain.SessionSnapshot{GameID: g.ID, Title: g.Title}
		before := rawGetResponse(t, h, g.ID)
		wantSessionRef(t, deleteGame(t, h, g.ID), 0, g.ID, g.Title)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("DEL-002/ErrGameInUse do store vira -32002 com sessionId 0", func(t *testing.T) {
		t.Parallel()
		h, _, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		// The tracker knows nothing of the session, so the store's own check refuses the removal.
		h.seedSessions(t, g.ID, seededSession{start: 0, open: true})
		before := rawGetResponse(t, h, g.ID)
		wantSessionRef(t, deleteGame(t, h, g.ID), 0, g.ID, g.Title)
		wantGameUnchanged(t, h, g.ID, before)
	})

	t.Run("DEL-003/sessão de outro jogo não impede a remoção e a sessão não muda", func(t *testing.T) {
		t.Parallel()
		h, dt, _ := newDeleteHarness(t)
		a := h.addGame(t, h.file(t, "a.exe"), "Jogo A")
		b := h.addGame(t, h.file(t, "b.exe"), "Jogo B")
		ids := h.seedSessions(t, a.ID, seededSession{start: 0, open: true})
		dt.active[a.ID] = domain.SessionSnapshot{
			ID: ids[0], GameID: a.ID, Title: a.Title, State: domain.SessionRunning, StartedAt: gamesT0, AsOf: gamesT0,
		}
		sessions := sessionsRaw(t, h, a.ID)

		deleteOK(t, h, b.ID)
		wantGameRemoved(t, h, b.ID)
		if got := sessionsRaw(t, h, a.ID); got != sessions {
			t.Errorf("session.list of game A = %s after removing B, want %s", got, sessions)
		}
		if _, err := h.st.Game(t.Context(), a.ID); err != nil {
			t.Errorf("game A after removing B: store.Game: %v, want it kept", err)
		}
	})

	t.Run("DEL-004/gameId inexistente é -32001", func(t *testing.T) {
		t.Parallel()
		h, _, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		deleteOK(t, h, g.ID)
		// A game that never existed and one that was just removed both answer -32001.
		wantError(t, deleteGame(t, h, 999), ipc.CodeGameNotFound, ipc.MsgGameNotFound)
		wantError(t, deleteGame(t, h, g.ID), ipc.CodeGameNotFound, ipc.MsgGameNotFound)
	})

	t.Run("DEL-005/remoção aceita com processo lançado sem contar tempo ainda aberto; session.current continua null", func(t *testing.T) {
		t.Parallel()
		h, _, _ := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		launch := h.call(t, ipc.MethodGameLaunchUntracked, fmt.Sprintf(`{"gameId":%d}`, g.ID))
		if launch.Error != nil {
			t.Fatalf("game.launchUntracked: error %d %q", launch.Error.Code, launch.Error.Message)
		}
		if n := len(h.tr.untrackedLaunches()); n != 1 {
			t.Fatalf("untracked launches = %d, want 1", n)
		}
		deleteOK(t, h, g.ID)
		wantGameRemoved(t, h, g.ID)
		wantRaw(t, objectOf(t, h.call(t, ipc.MethodSessionCurrent, "").Result), "session", "null")
	})

	t.Run("DEL-006/falha do store no meio da remoção devolve -32603 e nada muda", func(t *testing.T) {
		t.Parallel()
		h, _, ds := newDeleteHarness(t)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		h.seedSessions(t, g.ID, seededSession{start: 0, active: 100 * time.Second})
		before := rawGetResponse(t, h, g.ID)
		sessions := sessionsRaw(t, h, g.ID)
		// The store fails before it writes anything; the error text must not reach the client.
		ds.fail = errors.New("sqlite: database is locked")
		resp := deleteGame(t, h, g.ID)
		wantError(t, resp, ipc.CodeInternalError, "Internal error")
		if strings.Contains(string(resp.Error.Data), "locked") {
			t.Errorf("error data leaks the store message: %s", resp.Error.Data)
		}
		wantGameUnchanged(t, h, g.ID, before)
		if got := sessionsRaw(t, h, g.ID); got != sessions {
			t.Errorf("session.list = %s after the failed removal, want %s", got, sessions)
		}
	})
}

// TestGameDeleteNotifications runs game.delete in a full agent with the real
// tracker and two clients [IPC-050, DEL-005]. It is not parallel: newWiring
// checks for leaked goroutines.
func TestGameDeleteNotifications(t *testing.T) {
	t.Run("IPC-050/game.delete não envia notificação", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c1, c2 := w.client(t), w.client(t)
		id, _ := w.addGame(t, c1, "Hollow Knight")
		c1.rpcOK(t, ipc.MethodStatus, nil)
		c2.rpcOK(t, ipc.MethodStatus, nil)
		before1, before2 := countNotifications(c1.snapshotAll()), countNotifications(c2.snapshotAll())

		c1.rpcOK(t, ipc.MethodGameDelete, map[string]int64{"gameId": id})
		// A round trip on each connection: anything queued before the response has arrived.
		c1.rpcOK(t, ipc.MethodStatus, nil)
		c2.rpcOK(t, ipc.MethodStatus, nil)
		if n := countNotifications(c1.snapshotAll()); n != before1 {
			t.Errorf("client 1 got %d notification(s) after game.delete, want none", n-before1)
		}
		if n := countNotifications(c2.snapshotAll()); n != before2 {
			t.Errorf("client 2 got %d notification(s) after game.delete, want none", n-before2)
		}
	})

	t.Run("DEL-005/processo de game.launchUntracked segue vivo com o tracker real e session.current continua null", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c := w.client(t)
		id, _ := w.addGame(t, c, "Hollow Knight")
		c.rpcOK(t, ipc.MethodGameLaunchUntracked, map[string]int64{"gameId": id})
		c.rpcOK(t, ipc.MethodGameDelete, map[string]int64{"gameId": id})

		w.fl.mu.Lock()
		detached := len(w.fl.detached)
		w.fl.mu.Unlock()
		if detached != 1 {
			t.Errorf("detached launches = %d, want 1: the untracked process is not touched by the removal", detached)
		}
		cur := c.rpcOK(t, ipc.MethodSessionCurrent, nil).Result
		wantRaw(t, objectOf(t, cur), "session", "null")
	})
}

// --- T22: platform.list, platform.add and library.catalog ---

// platformsRaw returns the platforms member of platform.list as sent, so a test
// can check that nothing changed [IPC-048].
func platformsRaw(t *testing.T, h *gamesHarness) string {
	t.Helper()
	resp := h.call(t, ipc.MethodPlatformList, "")
	if resp.Error != nil {
		t.Fatalf("platform.list: error %d %q (data %s)", resp.Error.Code, resp.Error.Message, resp.Error.Data)
	}
	return string(objectOf(t, resp.Result)["platforms"])
}

// platformList decodes the platforms of platform.list [IPC-048].
func platformList(t *testing.T, h *gamesHarness) []ipc.Platform {
	t.Helper()
	var out ipc.PlatformListResult
	resultInto(t, h.call(t, ipc.MethodPlatformList, ""), &out)
	return out.Platforms
}

// platformAddOK calls platform.add with name and category, requires success,
// checks that the result has exactly the member platform with id, name and
// category, and returns the platform [IPC-048, EDT-008].
func platformAddOK(t *testing.T, h *gamesHarness, name, category string) ipc.Platform {
	t.Helper()
	resp := h.call(t, ipc.MethodPlatformAdd, mustJSON(t, map[string]string{"name": name, "category": category}))
	if resp.Error != nil {
		t.Fatalf("platform.add(%q, %q): error %d %q (data %s)", name, category, resp.Error.Code, resp.Error.Message, resp.Error.Data)
	}
	top := objectOf(t, resp.Result)
	wantKeys(t, top, "platform")
	wantKeys(t, objectOf(t, top["platform"]), "id", "name", "category")
	var out ipc.PlatformAddResult
	resultInto(t, resp, &out)
	return out.Platform
}

// wantCatalogItems checks that raw is a list of {id, name} items (exactly those two
// members) with the names in want, in that order, and that each id is the database
// id of the row of table with that name [IPC-049, STO-030]. The ids are the ones the
// FilterSpec filters by (FLT-002, FLT-003).
func wantCatalogItems(t *testing.T, h *gamesHarness, raw json.RawMessage, table string, want []string) {
	t.Helper()
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("%s items %s: %v", table, raw, err)
	}
	if len(items) != len(want) {
		t.Fatalf("%s items = %s, want %d items %v", table, raw, len(want), want)
	}
	for i, it := range items {
		wantKeys(t, it, "id", "name")
		var id int64
		var name string
		if err := json.Unmarshal(it["id"], &id); err != nil {
			t.Fatalf("%s item %d id %s: %v", table, i, it["id"], err)
		}
		if err := json.Unmarshal(it["name"], &name); err != nil {
			t.Fatalf("%s item %d name %s: %v", table, i, it["name"], err)
		}
		if name != want[i] {
			t.Errorf("%s item %d name = %q, want %q", table, i, name, want[i])
		}
		var dbID int64
		if err := h.st.DB().QueryRowContext(t.Context(),
			`SELECT id FROM `+table+` WHERE name = ?`, name).Scan(&dbID); err != nil {
			t.Fatalf("lookup %s %q: %v", table, name, err)
		}
		if id != dbID {
			t.Errorf("%s item %q id = %d, want the database id %d", table, name, id, dbID)
		}
	}
}

func TestPlatformAndCatalog(t *testing.T) {
	t.Parallel()

	t.Run("IPC-048/platform.list devolve platforms em ordem de nome", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodPlatformList)
		// An empty database gives [] and not null [IPC-048].
		if raw := platformsRaw(t, h); raw != "[]" {
			t.Errorf("platforms of an empty database = %s, want []", raw)
		}
		added := []struct {
			name     string
			category domain.PlatformCategory
		}{
			{"Switch", domain.CategoryConsole},
			{"amiga", domain.CategoryPC},
			{"Atari 2600", domain.CategoryConsole},
			{"PC", domain.CategoryPC},
		}
		for _, p := range added {
			if _, err := h.st.AddPlatform(t.Context(), p.name, p.category); err != nil {
				t.Fatalf("AddPlatform(%q): %v", p.name, err)
			}
		}

		// Order by name without case: "amiga" < "Atari 2600" < "PC" < "Switch" (binary order would put "amiga" last).
		var items []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(platformsRaw(t, h)), &items); err != nil {
			t.Fatalf("platforms: %v", err)
		}
		var names []string
		for _, it := range items {
			wantKeys(t, it, "id", "name", "category")
			var name string
			if err := json.Unmarshal(it["name"], &name); err != nil {
				t.Fatalf("name: %v", err)
			}
			names = append(names, name)
		}
		if want := []string{"amiga", "Atari 2600", "PC", "Switch"}; fmt.Sprint(names) != fmt.Sprint(want) {
			t.Errorf("platform names = %v, want %v", names, want)
		}
		categories := map[string]string{"Switch": "console", "amiga": "pc", "Atari 2600": "console", "PC": "pc"}
		for _, p := range platformList(t, h) {
			if p.ID < 1 || p.Category != categories[p.Name] {
				t.Errorf("platform %+v, want id >= 1 and category %q", p, categories[p.Name])
			}
		}
	})

	t.Run("IPC-048/platform.add com nome existente em outra grafia devolve a existente", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodPlatformAdd)
		sw, err := h.st.AddPlatform(t.Context(), "Switch", domain.CategoryConsole)
		if err != nil {
			t.Fatalf("AddPlatform: %v", err)
		}
		before := platformsRaw(t, h)

		got := platformAddOK(t, h, "switch", "pc")
		if got.ID != sw.ID || got.Name != "Switch" || got.Category != "console" {
			t.Errorf("platform.add(switch, pc) = %+v, want the existing platform %d Switch console (category ignored)", got, sw.ID)
		}
		if after := platformsRaw(t, h); after != before {
			t.Errorf("platforms after the add = %s, want %s: no platform is created", after, before)
		}
	})

	t.Run("IPC-048/platform.add com nome novo devolve a plataforma criada", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodPlatformAdd)

		// The name is trimmed (DOM-016) and the category is kept (DOM-015).
		got := platformAddOK(t, h, "  Amiga 1200  ", "arcade")
		if got.ID < 1 || got.Name != "Amiga 1200" || got.Category != "arcade" {
			t.Errorf("platform.add = %+v, want a new platform Amiga 1200 arcade", got)
		}
		if list := platformList(t, h); len(list) != 1 || list[0].ID != got.ID {
			t.Errorf("platform.list after the add = %+v, want the new platform only", list)
		}

		// 100 runes is the largest accepted name (DOM-015, DOM-016).
		long := platformAddOK(t, h, strings.Repeat("é", domain.MaxNameRunes), "handheld")
		if n := len([]rune(long.Name)); n != domain.MaxNameRunes || long.Category != "handheld" {
			t.Errorf("platform.add with a %d-rune name: %d runes, category %q", domain.MaxNameRunes, n, long.Category)
		}
	})

	t.Run("EDT-008/nome vazio ou categoria inválida é -32602 e nada muda", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodPlatformAdd)
		if _, err := h.st.AddPlatform(t.Context(), "Switch", domain.CategoryConsole); err != nil {
			t.Fatalf("AddPlatform: %v", err)
		}
		before := platformsRaw(t, h)
		tooLong := strings.Repeat("é", domain.MaxNameRunes+1)
		wantEachInvalidParams(t, h, ipc.MethodPlatformAdd,
			`{"name":"","category":"pc"}`,
			`{"name":"   ","category":"pc"}`,
			`{"name":"`+tooLong+`","category":"pc"}`,
			`{"name":"Amiga","category":""}`,
			`{"name":"Amiga","category":"Console"}`,
			`{"name":"Amiga","category":"laptop"}`,
		)
		if after := platformsRaw(t, h); after != before {
			t.Errorf("platforms after the refused adds = %s, want %s", after, before)
		}
	})

	t.Run("EDT-009/params de platform.add: membro desconhecido, falta de membro, tipo errado ou posicional é -32602 e nada muda", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodPlatformAdd)
		before := platformsRaw(t, h)
		wantEachInvalidParams(t, h, ipc.MethodPlatformAdd,
			"", `{}`, `{"name":"Amiga"}`, `{"category":"pc"}`,
			`{"name":"Amiga","category":"pc","extra":1}`,
			`{"name":5,"category":"pc"}`,
			`{"Name":"Amiga","category":"pc"}`,
			`["Amiga","pc"]`,
		)
		if after := platformsRaw(t, h); after != before {
			t.Errorf("platforms after the refused adds = %s, want %s", after, before)
		}
	})

	t.Run("IPC-049/platform.list e library.catalog aceitam params ausentes, null, [] ou {} e recusam membro ou array com -32602", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		for _, m := range []string{ipc.MethodPlatformList, ipc.MethodLibraryCatalog} {
			h.need(t, m)
			for _, p := range []string{"", "null", "[]", "{}"} {
				if resp := h.call(t, m, p); resp.Error != nil {
					t.Errorf("%s with params %q: error %d %q, want success", m, p, resp.Error.Code, resp.Error.Message)
				}
			}
		}
		wantEachInvalidParams(t, h, ipc.MethodPlatformList, `{"x":1}`, `[1]`)
		wantEachInvalidParams(t, h, ipc.MethodLibraryCatalog, `{"x":1}`, `[1]`)
	})

	t.Run("IPC-049/library.catalog devolve listas não nulas e exatamente os cinco playModes", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		h.need(t, ipc.MethodLibraryCatalog)

		// An empty database: every list is [] and not null [IPC-049, STO-030].
		empty := objectOf(t, h.call(t, ipc.MethodLibraryCatalog, "").Result)
		wantKeys(t, empty, "platforms", "genres", "companies", "playModes")
		wantRaw(t, empty, "platforms", "[]")
		wantRaw(t, empty, "genres", "[]")
		wantRaw(t, empty, "companies", "[]")

		// A game with links, so the catalog has names to sort: companies and genres
		// are in name order without case.
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		developers := []string{"team cherry", "Ace"}
		genres := []string{"roguelike", "Action", "adventure"}
		if _, err := h.st.UpdateGame(t.Context(), g.ID, store.GamePatch{
			Developers: &developers, Genres: &genres,
		}, gamesT0); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}

		cat := objectOf(t, h.call(t, ipc.MethodLibraryCatalog, "").Result)
		wantKeys(t, cat, "platforms", "genres", "companies", "playModes")
		// The platforms of the catalog are the ones of platform.list [IPC-048, IPC-049].
		wantRaw(t, cat, "platforms", platformsRaw(t, h))
		// Genres and companies are {id, name} items, the ids being the database ids
		// that the FilterSpec filters by, in name order without case [IPC-049, STO-030].
		wantCatalogItems(t, h, cat["genres"], "genre", []string{"Action", "adventure", "roguelike"})
		wantCatalogItems(t, h, cat["companies"], "company", []string{"Ace", "team cherry"})

		var modes []string
		if err := json.Unmarshal(cat["playModes"], &modes); err != nil {
			t.Fatalf("playModes: %v", err)
		}
		sort.Strings(modes)
		want := []string{"coop_local", "coop_online", "multi_local", "multi_online", "single"}
		if fmt.Sprint(modes) != fmt.Sprint(want) {
			t.Errorf("playModes = %v, want exactly the five names of DOM-014 %v", modes, want)
		}
	})
}

// TestPlatformAndCatalogNotifications runs platform.add in a full agent with two
// clients [IPC-050]. It is not parallel: newWiring checks for leaked goroutines.
func TestPlatformAndCatalogNotifications(t *testing.T) {
	t.Run("IPC-050/platform.add não envia notificação", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c1, c2 := w.client(t), w.client(t)
		before1, before2 := countNotifications(c1.snapshotAll()), countNotifications(c2.snapshotAll())

		// One new platform and one existing in another spelling [IPC-048, EDT-008].
		c1.rpcOK(t, ipc.MethodPlatformAdd, map[string]string{"name": "Switch", "category": "console"})
		c1.rpcOK(t, ipc.MethodPlatformAdd, map[string]string{"name": "switch", "category": "pc"})
		// A round trip on each connection: anything queued before the response has arrived.
		c1.rpcOK(t, ipc.MethodStatus, nil)
		c2.rpcOK(t, ipc.MethodStatus, nil)
		if n := countNotifications(c1.snapshotAll()); n != before1 {
			t.Errorf("client 1 got %d notification(s) after platform.add, want none", n-before1)
		}
		if n := countNotifications(c2.snapshotAll()); n != before2 {
			t.Errorf("client 2 got %d notification(s) after platform.add, want none", n-before2)
		}
	})
}
