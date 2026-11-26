package agent

// Tests for T18 of docs/specs/phase-16-edicao-sessoes/tasks.md: the read results gain source/editedAt
// (session.list), sessionPlaySeconds (every GameDetail result) and manualSeconds (stats.summary)
// [SED-027, SED-033, SED-034]. Red until T17 (ipc types) and T19 (agent conversions).

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// setSessionSource rewrites source and edited_at (ms UTC, 0 = NULL) of a seeded session by SQL.
func (h *gamesHarness) setSessionSource(t *testing.T, id int64, source string, editedAt time.Time) {
	t.Helper()
	var edited any
	if !editedAt.IsZero() {
		edited = editedAt.UnixMilli()
	}
	if _, err := h.st.DB().ExecContext(t.Context(),
		`UPDATE play_session SET source = ?, edited_at = ? WHERE id = ?`, source, edited, id); err != nil {
		t.Fatalf("update session %d: %v", id, err)
	}
}

func TestSessionListSourceAndEditedAt(t *testing.T) {
	t.Parallel()

	t.Run("SED-033/session.list devolve source e editedAt (RFC 3339 UTC, ou null) em cada item; ordem e paginação não mudam", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		ids := h.seedSessions(t, g.ID,
			seededSession{start: 0, active: 100 * time.Second},
			seededSession{start: time.Hour, active: 100 * time.Second},
			seededSession{start: 2 * time.Hour, active: 100 * time.Second},
		)
		edited := time.Date(2026, 11, 25, 17, 30, 15, 123_000_000, time.FixedZone("UTC-3", -3*3600))
		h.setSessionSource(t, ids[0], "import", time.Time{})
		h.setSessionSource(t, ids[1], "manual", time.Time{})
		h.setSessionSource(t, ids[2], "tracked", edited)

		resp := h.call(t, ipc.MethodSessionList, mustJSON(t, map[string]int64{"gameId": g.ID}))
		var raw []map[string]json.RawMessage
		if err := json.Unmarshal(objectOf(t, resp.Result)["sessions"], &raw); err != nil {
			t.Fatalf("sessions: %v", err)
		}
		if len(raw) != 3 {
			t.Fatalf("sessions = %d, want 3", len(raw))
		}
		for _, it := range raw {
			wantKeys(t, it, sessionItemKeys...)
		}
		// Order is started_at desc: ids[2], ids[1], ids[0].
		wantRaw(t, raw[0], "source", `"tracked"`)
		wantRaw(t, raw[0], "editedAt", `"2026-11-25T20:30:15.123Z"`)
		wantRaw(t, raw[1], "source", `"manual"`)
		wantRaw(t, raw[1], "editedAt", `null`)
		wantRaw(t, raw[2], "source", `"import"`)
		wantRaw(t, raw[2], "editedAt", `null`)

		var page ipc.SessionListResult
		resultInto(t, h.call(t, ipc.MethodSessionList, mustJSON(t, map[string]any{"gameId": g.ID, "offset": 1, "limit": 1})), &page)
		if want := []int64{ids[1]}; fmt.Sprint(sessionIDs(page.Sessions)) != fmt.Sprint(want) || page.Total != 3 || page.Offset != 1 {
			t.Errorf("page = %v total %d offset %d, want %v 3 1", sessionIDs(page.Sessions), page.Total, page.Offset, want)
		}
	})

	t.Run("SED-033/sessão rastreada nunca editada tem source tracked e editedAt nulo", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.seedSessions(t, g.ID, seededSession{start: 0, active: 100 * time.Second})
		var page ipc.SessionListResult
		resultInto(t, h.call(t, ipc.MethodSessionList, mustJSON(t, map[string]int64{"gameId": g.ID})), &page)
		if len(page.Sessions) != 1 || page.Sessions[0].Source != "tracked" || page.Sessions[0].EditedAt != nil {
			t.Fatalf("sessions = %+v, want one tracked, never edited", page.Sessions)
		}
	})

	t.Run("SED-033/valor de source desconhecido passa como está", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		ids := h.seedSessions(t, g.ID, seededSession{start: 0, active: 100 * time.Second})
		h.setSessionSource(t, ids[0], "futuro", time.Time{})
		var page ipc.SessionListResult
		resultInto(t, h.call(t, ipc.MethodSessionList, mustJSON(t, map[string]int64{"gameId": g.ID})), &page)
		if len(page.Sessions) != 1 || page.Sessions[0].Source != "futuro" {
			t.Fatalf("sessions = %+v, want source passed through as \"futuro\"", page.Sessions)
		}
	})
}

func TestGameDetailSessionPlaySeconds(t *testing.T) {
	t.Parallel()

	t.Run("SED-034/game.get, game.update e game.setPlayTime devolvem sessionPlaySeconds", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		g := h.addGame(t, h.file(t, "game.exe"), "Game")
		h.seedSessions(t, g.ID,
			seededSession{start: 0, active: 100 * time.Second},
			seededSession{start: time.Hour, active: 10 * time.Second}, // discarded: not counted
			seededSession{start: 2 * time.Hour, active: 60 * time.Second},
			seededSession{start: 3 * time.Hour, open: true}, // open: not counted
		)
		const want = int64(160)

		check := func(label string, resp ipc.Response) {
			t.Helper()
			if resp.Error != nil {
				t.Fatalf("%s: error %d %q", label, resp.Error.Code, resp.Error.Message)
			}
			var res ipc.GameDetailResult
			resultInto(t, resp, &res)
			if res.Game.SessionPlaySeconds != want {
				t.Errorf("%s: sessionPlaySeconds = %d, want %d", label, res.Game.SessionPlaySeconds, want)
			}
			wantKeys(t, objectOf(t, objectOf(t, resp.Result)["game"]), append(append([]string{}, gameKeys...), gameDetailKeys...)...)
		}
		check("game.get", h.call(t, ipc.MethodGameGet, fmt.Sprintf(`{"gameId":%d}`, g.ID)))
		check("game.update", h.call(t, ipc.MethodGameUpdate, fmt.Sprintf(`{"gameId":%d,"title":"Outro"}`, g.ID)))

		// The manual total does not change the sessions' time.
		f := newEditFixture(t)
		seedEndedSession(t, f, 100)
		raw, err := f.call(t, ipc.MethodGameSetPlayTime, ipc.GameSetPlayTimeParams{GameID: f.game, TotalPlaySeconds: 99999})
		if err != nil {
			t.Fatalf("game.setPlayTime: %v", err)
		}
		var res ipc.GameDetailResult
		if err := json.Unmarshal(raw, &res); err != nil {
			t.Fatal(err)
		}
		if res.Game.SessionPlaySeconds != 100 || res.Game.TotalPlaySeconds != 99999 {
			t.Errorf("game.setPlayTime: sessionPlaySeconds = %d total = %d, want 100 and 99999", res.Game.SessionPlaySeconds, res.Game.TotalPlaySeconds)
		}
	})
}

// seedEndedSession inserts an ended tracked session of the fixture's game by SQL.
func seedEndedSession(t *testing.T, f editFixture, seconds int64) {
	t.Helper()
	start := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	if _, err := f.st.DB().ExecContext(t.Context(),
		`INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source)
		 VALUES (?, ?, ?, ?, ?, 'ended', 'tracked')`, f.game, start, start+seconds*1000, start+seconds*1000, seconds); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

func TestStatsManualSeconds(t *testing.T) {
	t.Parallel()

	t.Run("SED-027/stats.summary devolve manualSeconds por jogo e por plataforma e o total geral é a soma dos jogos", func(t *testing.T) {
		t.Parallel()
		h, st, now := statsFixture(t)
		insertStatsSession(t, st, "Celeste", now.AddDate(0, 0, -3), 900, "manual")
		insertStatsSession(t, st, "Hades", now.AddDate(0, 0, -4), 300, "manual")

		out, err := callStats(t, h, ipc.MethodStatsSummary, `{"period":"30d"}`)
		if err != nil {
			t.Fatalf("summary: %v", err)
		}
		res := out.(ipc.StatsSummaryResult)
		byTitle := map[string]ipc.StatsGame{}
		var sum int64
		for _, g := range res.Games {
			byTitle[g.Title] = g
			sum += g.ManualSeconds
		}
		if byTitle["Celeste"].ManualSeconds != 900 || byTitle["Hades"].ManualSeconds != 300 {
			t.Errorf("games = %+v, want manualSeconds 900 (Celeste) and 300 (Hades)", res.Games)
		}
		if byTitle["Celeste"].ActiveSeconds != 1500 || byTitle["Hades"].ActiveSeconds != 1500 {
			t.Errorf("games = %+v, want activeSeconds 1500 each (manual counts as played time)", res.Games)
		}
		if res.ManualSeconds != 1200 || res.ManualSeconds != sum || res.TotalSeconds != 3000 {
			t.Errorf("summary manual %d (games sum %d) total %d, want 1200, 1200 and 3000", res.ManualSeconds, sum, res.TotalSeconds)
		}
		if len(res.Platforms) != 1 || res.Platforms[0].ManualSeconds != 1200 || res.Platforms[0].ActiveSeconds != 3000 {
			t.Errorf("platforms = %+v, want one with manualSeconds 1200 and activeSeconds 3000", res.Platforms)
		}
	})

	t.Run("SED-027/importedSeconds e manualSeconds convivem sem se misturar", func(t *testing.T) {
		t.Parallel()
		h, st, now := statsFixture(t)
		insertStatsSession(t, st, "Celeste", now.AddDate(0, 0, -3), 900, "manual")

		out, err := callStats(t, h, ipc.MethodStatsSummary, `{"period":"all"}`)
		if err != nil {
			t.Fatalf("summary: %v", err)
		}
		res := out.(ipc.StatsSummaryResult)
		if res.ImportedSeconds != 3600 || res.ManualSeconds != 900 || res.TotalSeconds != 6300 {
			t.Errorf("imported %d manual %d total %d, want 3600, 900 and 6300", res.ImportedSeconds, res.ManualSeconds, res.TotalSeconds)
		}
		for _, g := range res.Games {
			if g.Title == "Celeste" && (g.ImportedSeconds != 3600 || g.ManualSeconds != 900) {
				t.Errorf("Celeste = %+v, want imported 3600 and manual 900", g)
			}
		}
	})

	t.Run("SED-027/sem sessão manual manualSeconds é 0 e a rastreada editada conta como rastreada", func(t *testing.T) {
		t.Parallel()
		h, st, _ := statsFixture(t)
		if _, err := st.DB().ExecContext(t.Context(), `UPDATE play_session SET edited_at = started_at WHERE source = 'tracked'`); err != nil {
			t.Fatalf("mark edited: %v", err)
		}
		out, err := callStats(t, h, ipc.MethodStatsSummary, `{"period":"7d"}`)
		if err != nil {
			t.Fatalf("summary: %v", err)
		}
		if res := out.(ipc.StatsSummaryResult); res.ManualSeconds != 0 || res.TotalSeconds != 1800 {
			t.Errorf("summary = %+v, want manual 0 and total 1800", res)
		}
	})

	t.Run("SED-027/stats.weekly não ganha campo", func(t *testing.T) {
		t.Parallel()
		h, _, _ := statsFixture(t)
		out, err := callStats(t, h, ipc.MethodStatsWeekly, `{"weeks":4}`)
		if err != nil {
			t.Fatalf("weekly: %v", err)
		}
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var res struct {
			Weeks []map[string]json.RawMessage `json:"weeks"`
		}
		if err := json.Unmarshal(raw, &res); err != nil || len(res.Weeks) != 4 {
			t.Fatalf("weekly = %s (err %v), want 4 weeks", raw, err)
		}
		for _, w := range res.Weeks {
			if _, ok := w["manualSeconds"]; ok {
				t.Errorf("week %v has manualSeconds", w)
			}
		}
	})
}

// insertStatsSession inserts an ended session of the game called title with the given source.
func insertStatsSession(t *testing.T, st *store.Store, title string, when time.Time, seconds int64, source string) {
	t.Helper()
	ms := when.UnixMilli()
	if _, err := st.DB().ExecContext(t.Context(),
		`INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source)
		 SELECT id, ?, ?, ?, ?, 'ended', ? FROM game WHERE title = ?`,
		ms, ms+seconds*1000, ms+seconds*1000, seconds, source, title); err != nil {
		t.Fatalf("insert %s session: %v", source, err)
	}
}
