package agent

// Tests and helpers of T18 and T19 (docs/specs/phase-5-library/tasks.md): game.add
// with the v0.6.0 Game shape [IPC-020] and the helpers of the game.list tests in
// games_test.go (TestGameList): raw responses for the 1 MiB budget [IPC-051], a
// ListGames stub that pages over a fixed slice, and the worst-case games. T19:
// game.get [IPC-044] and session.list [IPC-047].

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
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
var gameDetailKeys = []string{"description", "completedAt", "developers", "publishers", "genres", "playModes"}

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
