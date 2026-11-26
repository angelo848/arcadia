package main

// Tests for T23 and T24 (docs/specs/phase-5-library/tasks.md): the library
// bindings of the desktop (DSK-016, DSK-017), the removal of LibrarySummary and
// of the DSK-008 gate (DSK-019, DSK-008 alterado, ADR 0015). The DSK-018 message
// of a refused removal is in errors_test.go (TestDeleteMessage).
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - ListGames, GetGame, GameSessions, UpdateGame, DeleteGame, Catalog and
//     AddPlatform use the 5 s deadline of DSK-016/DSK-017. singleCall checks an
//     upper bound, so a shorter deadline also passes.
//   - ListGames sends LibraryQuery.Filter as the "filter" member of game.list.
//     An empty Filter is sent as {"version":1}.
//   - UpdateGame sends the members of the raw patch as they came, plus "gameId".
//     Absent members stay absent and null members stay null (decisão 10).
//   - GameDetailView, CatalogView and PlatformView are compared through their
//     JSON names (the wire names of the OpenRPC contract). GameView,
//     GameListView, SessionListView and SessionItemView are compared through the
//     Go fields named in tasks.md. SessionItemView.StartedAt and EndedAt are RFC
//     3339 strings ("" when EndedAt is null) and EndReason is a string ("" = null).
//   - The DSK-019 and DSK-008 checks parse the Go sources of cmd/arcadia,
//     internal/store and frontend/wailsjs, relative to the package directory.

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// goFiles lists the .go files of dir; test files only when tests is true.
func goFiles(t *testing.T, dir string, tests bool) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if !tests && strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	if len(out) == 0 {
		t.Fatalf("no Go files in %s", dir)
	}
	return out
}

// identifiers returns every identifier used or declared in the Go files of dir.
func identifiers(t *testing.T, dir string, tests bool) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	names := map[string]bool{}
	for _, path := range goFiles(t, dir, tests) {
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				names[id.Name] = true
			}
			return true
		})
	}
	return names
}

// storeImporters lists the production files of dir that import internal/store.
func storeImporters(t *testing.T, dir string) []string {
	t.Helper()
	const storePath = "github.com/arcadia-launcher/arcadia/internal/store"
	var out []string
	for _, path := range goFiles(t, dir, false) {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports of %s: %v", path, err)
		}
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) == storePath {
				out = append(out, path)
			}
		}
	}
	return out
}

// sameJSON reports whether got is the same JSON value as want. A missing
// member (empty got) is never the same as a value.
func sameJSON(t *testing.T, got json.RawMessage, want string) bool {
	t.Helper()
	if len(got) == 0 {
		return false
	}
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("json %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("expected json %s: %v", want, err)
	}
	return reflect.DeepEqual(g, w)
}

// wantJSON checks that v marshals to the JSON want (same members, same values).
func wantJSON(t *testing.T, v any, want string) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", v, err)
	}
	if !sameJSON(t, raw, want) {
		t.Errorf("JSON = %s, want %s", raw, want)
	}
}

// jsonObject returns v as the generic JSON object the frontend receives.
func jsonObject(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", v, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("json %s: %v", raw, err)
	}
	return m
}

// rawObject decodes the params of a recorded call member by member, so an
// absent member can be told from a null one.
func rawObject(t *testing.T, c gameCall) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(c.params, &m); err != nil {
		t.Fatalf("%s params %s: %v", c.method, c.params, err)
	}
	return m
}

// wantEmptyLists checks that each member of v is [] in its JSON, never null.
func wantEmptyLists(t *testing.T, v any, members ...string) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", v, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("json %s: %v", raw, err)
	}
	for _, name := range members {
		list, ok := m[name].([]any)
		if !ok || len(list) != 0 {
			t.Errorf("%s = %s, want []", name, raw)
		}
	}
}

// wantInstant checks that got is an RFC 3339 time equal to want.
func wantInstant(t *testing.T, name, got string, want time.Time) {
	t.Helper()
	at, err := time.Parse(time.RFC3339, got)
	if err != nil || !at.Equal(want) {
		t.Errorf("%s = %q, want RFC 3339 of %s (err %v)", name, got, want.Format(time.RFC3339), err)
	}
}

func wantErrText(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

func TestLibraryReads(t *testing.T) {
	t.Parallel()

	t.Run("DSK-016/ListGames repassa filter, sort, offset e limit ao agente e converte para GameView sem calcular tempo", func(t *testing.T) {
		t.Parallel()
		first := gameT0.Add(-24 * time.Hour)
		last := gameT0.Add(time.Hour)
		year := 1998
		games := []ipc.Game{
			{ID: 2, PlatformID: 1, Title: "B", Platform: "PC", Path: `C:\G\b.exe`, Status: "playing", Favorite: true,
				Year: &year, TotalPlaySeconds: 3900, PlayCount: 2, FirstPlayedAt: &first, LastPlayedAt: &last},
			{ID: 1, PlatformID: 2, Title: "A", Platform: "Switch", Path: `C:\G\a.exe`, Status: "not_played"},
		}
		p := &gamePinger{results: map[string]any{ipc.MethodGameList: ipc.GameListResult{Games: games, Total: 450, Offset: 200}}}
		a := &App{client: p, log: discardLog()}
		q := LibraryQuery{
			Filter: json.RawMessage(`{"version":1,"favorite":true,"platformIds":[1]}`),
			Sort:   "lastPlayed",
			Offset: 200,
			Limit:  200,
		}

		got, err := a.ListGames(q)

		if err != nil {
			t.Fatalf("ListGames() err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodGameList, 5*time.Second)
		lp := rawObject(t, c)
		if !sameJSON(t, lp["filter"], `{"version":1,"favorite":true,"platformIds":[1]}`) {
			t.Errorf("game.list filter = %s, want the filter of the query", lp["filter"])
		}
		if !sameJSON(t, lp["sort"], `"lastPlayed"`) || !sameJSON(t, lp["offset"], `200`) || !sameJSON(t, lp["limit"], `200`) {
			t.Errorf("game.list params = %s, want sort lastPlayed, offset 200, limit 200", c.params)
		}

		// An empty filter is sent as {"version":1}.
		p2 := &gamePinger{results: p.results}
		a.client = p2
		if _, err := a.ListGames(LibraryQuery{}); err != nil {
			t.Fatalf("ListGames(empty) err = %v", err)
		}
		lp2 := rawObject(t, singleCall(t, p2, ipc.MethodGameList, 5*time.Second))
		if !sameJSON(t, lp2["filter"], `{"version":1}`) {
			t.Errorf("game.list filter for an empty query = %s, want {\"version\":1}", lp2["filter"])
		}

		if got.Total != 450 || got.Offset != 200 || len(got.Games) != 2 {
			t.Fatalf("ListGames() = %+v, want total 450, offset 200 and 2 games", got)
		}
		g0 := got.Games[0]
		if g0.ID != 2 || g0.PlatformID != 1 || g0.Title != "B" || g0.Platform != "PC" || g0.Path != `C:\G\b.exe` ||
			g0.Status != "playing" || !g0.Favorite || g0.Year == nil || *g0.Year != 1998 ||
			g0.TotalPlaySeconds != 3900 || g0.PlayCount != 2 {
			t.Errorf("GameView[0] = %+v, want the values of the agent (no time recomputed)", g0)
		}
		wantInstant(t, "GameView[0].FirstPlayedAt", g0.FirstPlayedAt, first)
		wantInstant(t, "GameView[0].LastPlayedAt", g0.LastPlayedAt, last)
		if g1 := got.Games[1]; g1.ID != 1 || g1.Year != nil || g1.FirstPlayedAt != "" || g1.LastPlayedAt != "" {
			t.Errorf("GameView[1] = %+v, want id 1, year nil and empty times", g1)
		}
	})

	t.Run("DSK-016/GetGame devolve o detalhe com listas nunca nil", func(t *testing.T) {
		t.Parallel()
		detail := ipc.GameDetail{
			Game:        ipc.Game{ID: 3, Title: "Celeste", Platform: "PC", Status: "completed", Favorite: true},
			Description: "Platformer",
		}
		p := &gamePinger{results: map[string]any{ipc.MethodGameGet: ipc.GameDetailResult{Game: detail}}}
		a := &App{client: p, log: discardLog()}

		got, err := a.GetGame(3)

		if err != nil {
			t.Fatalf("GetGame(3) err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodGameGet, 5*time.Second)
		if !sameJSON(t, rawObject(t, c)["gameId"], `3`) {
			t.Errorf("game.get params = %s, want {\"gameId\":3}", c.params)
		}
		wantJSON(t, got, `{"id":3,"title":"Celeste","platformId":0,"platform":"PC","path":"","status":"completed",`+
			`"favorite":true,"year":null,"totalPlaySeconds":0,"playCount":0,"firstPlayedAt":"","lastPlayedAt":"","emulatorId":null,`+
			`"description":"Platformer","completedAt":"","developers":[],"publishers":[],"genres":[],"playModes":[],"launchType":"","launchArgs":"","sessionPlaySeconds":0}`)
		// alterado em 2026-11-25 (fase 16): sessionPlaySeconds (SED-034).

		done := "2026-11-12"
		detail2 := ipc.GameDetail{
			Game:        ipc.Game{ID: 3, Title: "Celeste"},
			CompletedAt: &done,
			Developers:  []string{"Maddy Makes Games"},
		}
		p2 := &gamePinger{results: map[string]any{ipc.MethodGameGet: ipc.GameDetailResult{Game: detail2}}}
		a.client = p2
		got2, err := a.GetGame(3)
		if err != nil {
			t.Fatalf("GetGame(3) with lists err = %v", err)
		}
		wantJSON(t, got2, `{"id":3,"title":"Celeste","platformId":0,"platform":"","path":"","status":"",`+
			`"favorite":false,"year":null,"totalPlaySeconds":0,"playCount":0,"firstPlayedAt":"","lastPlayedAt":"","emulatorId":null,`+
			`"description":"","completedAt":"2026-11-12","developers":["Maddy Makes Games"],"publishers":[],"genres":[],"playModes":[],"launchType":"","launchArgs":"","sessionPlaySeconds":0}`)
		// alterado em 2026-11-25 (fase 16): sessionPlaySeconds (SED-034).
	})

	t.Run("DSK-016/GameSessions repassa offset e limit e devolve sessions nunca nil", func(t *testing.T) {
		t.Parallel()
		ended := gameT0.Add(10 * time.Minute)
		reason := "exit"
		items := []ipc.SessionItem{
			{SessionID: 12, GameID: 3, State: "running", StartedAt: gameT0, ActiveSeconds: 60},
			{SessionID: 11, GameID: 3, State: "ended", StartedAt: gameT0.Add(-time.Hour), EndedAt: &ended,
				ActiveSeconds: 600, EndReason: &reason},
		}
		p := &gamePinger{results: map[string]any{ipc.MethodSessionList: ipc.SessionListResult{Sessions: items, Total: 2, Offset: 50}}}
		a := &App{client: p, log: discardLog()}

		got, err := a.GameSessions(3, 50, 50)

		if err != nil {
			t.Fatalf("GameSessions(3, 50, 50) err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodSessionList, 5*time.Second)
		lp := rawObject(t, c)
		if !sameJSON(t, lp["gameId"], `3`) || !sameJSON(t, lp["offset"], `50`) || !sameJSON(t, lp["limit"], `50`) {
			t.Errorf("session.list params = %s, want gameId 3, offset 50, limit 50", c.params)
		}
		if got.Total != 2 || got.Offset != 50 || len(got.Sessions) != 2 {
			t.Fatalf("GameSessions() = %+v, want total 2, offset 50 and 2 sessions", got)
		}
		s0, s1 := got.Sessions[0], got.Sessions[1]
		if s0.SessionID != 12 || s0.GameID != 3 || s0.State != "running" || s0.EndedAt != "" || s0.EndReason != "" {
			t.Errorf("session[0] = %+v, want id 12, running, endedAt and endReason empty", s0)
		}
		if s1.SessionID != 11 || s1.State != "ended" || s1.ActiveSeconds != 600 || s1.EndReason != "exit" {
			t.Errorf("session[1] = %+v, want id 11, ended, 600 s and endReason exit", s1)
		}
		wantInstant(t, "session[1].EndedAt", s1.EndedAt, ended)

		// Without sessions the list is [] and never null.
		p2 := &gamePinger{results: map[string]any{ipc.MethodSessionList: ipc.SessionListResult{}}}
		a.client = p2
		empty, err := a.GameSessions(3, 0, 50)
		if err != nil {
			t.Fatalf("GameSessions with no sessions err = %v", err)
		}
		if empty.Sessions == nil {
			t.Errorf("Sessions = nil, want []")
		}
		wantEmptyLists(t, empty, "sessions")
	})

	t.Run("DSK-016/erro do agente sai como mensagem de DSK-011", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{
			ipc.MethodGameList:    ipc.ErrConnectionLost,
			ipc.MethodGameGet:     gameNotFoundErr(),
			ipc.MethodSessionList: errors.New("boom"),
		}}
		a := &App{client: p, log: discardLog()}

		_, err := a.ListGames(LibraryQuery{})
		wantErrText(t, err, txtNotResponding)
		_, err = a.GetGame(3)
		wantErrText(t, err, txtGameNotFound)
		_, err = a.GameSessions(3, 0, 50)
		wantErrText(t, err, txtUnexpected)
	})
}

func TestLibrarySummaryRemoved(t *testing.T) {
	t.Parallel()

	t.Run("DSK-019/LibrarySummary e LibrarySummaryView não existem em cmd/arcadia nem em internal/store", func(t *testing.T) {
		t.Parallel()
		for _, dir := range []string{".", filepath.Join("..", "..", "internal", "store")} {
			names := identifiers(t, dir, true)
			for _, bad := range []string{"LibrarySummary", "LibrarySummaryView"} {
				if names[bad] {
					t.Errorf("%s still uses %s, want it removed (DSK-019)", dir, bad)
				}
			}
		}
	})

	t.Run("DSK-019/wailsjs não tem LibrarySummary", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join("..", "..", "frontend", "wailsjs")
		files := 0
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			files++
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(data, []byte("LibrarySummary")) {
				t.Errorf("%s mentions LibrarySummary, want the bindings regenerated without it (DSK-019)", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WalkDir(%s): %v", dir, err)
		}
		if files == 0 {
			t.Fatalf("no files under %s", dir)
		}
	})
}

func TestLibraryGateRemoved(t *testing.T) {
	t.Parallel()

	t.Run("DSK-008/libraryGate e storeOpener foram removidos; a UI não abre o banco (ADR 0015)", func(t *testing.T) {
		t.Parallel()
		names := identifiers(t, ".", false)
		for _, gone := range []string{"libraryGate", "newLibraryGate", "storeOpener", "libraryStore"} {
			if names[gone] {
				t.Errorf("production code still uses %s (DSK-008, ADR 0015)", gone)
			}
		}
		for _, path := range storeImporters(t, ".") {
			t.Errorf("%s imports internal/store; the UI must not open the database (ADR 0015)", path)
		}
	})
}

func TestLibraryWrites(t *testing.T) {
	t.Parallel()

	t.Run("DSK-017/UpdateGame envia game.update com o patch cru e devolve o detalhe", func(t *testing.T) {
		t.Parallel()
		detail := ipc.GameDetail{Game: ipc.Game{ID: 3, Title: "Celeste"}, Description: "x"}
		p := &gamePinger{results: map[string]any{ipc.MethodGameUpdate: ipc.GameDetailResult{Game: detail}}}
		em := &gameEmitLog{}
		a := newBindingApp(p, em)

		got, err := a.UpdateGame(3, json.RawMessage(`{"favorite":true,"completedAt":null,"developers":["Team Cherry"]}`))

		if err != nil {
			t.Fatalf("UpdateGame(3, patch) err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodGameUpdate, 5*time.Second)
		lp := rawObject(t, c)
		if !sameJSON(t, lp["gameId"], `3`) || !sameJSON(t, lp["favorite"], `true`) ||
			!sameJSON(t, lp["completedAt"], `null`) || !sameJSON(t, lp["developers"], `["Team Cherry"]`) {
			t.Errorf("game.update params = %s, want gameId 3 and the patch members as sent (completedAt null kept)", c.params)
		}
		if _, ok := lp["title"]; ok {
			t.Errorf("game.update params carry title, which the patch did not send")
		}
		if m := jsonObject(t, got); m["title"] != "Celeste" || m["description"] != "x" {
			t.Errorf("UpdateGame() = %+v, want the detail returned by the agent", got)
		}

		// An empty patch sends only gameId: absent members stay absent (ausente ≠ null).
		p2 := &gamePinger{results: p.results}
		a.client = p2
		if _, err := a.UpdateGame(3, json.RawMessage(`{}`)); err != nil {
			t.Fatalf("UpdateGame(3, {}) err = %v", err)
		}
		lp2 := rawObject(t, singleCall(t, p2, ipc.MethodGameUpdate, 5*time.Second))
		if len(lp2) != 1 || !sameJSON(t, lp2["gameId"], `3`) {
			t.Errorf("game.update params for an empty patch = %v, want only gameId", lp2)
		}
		if got := em.recorded(); len(got) != 0 {
			t.Errorf("events emitted = %v, want none", got)
		}
	})

	t.Run("DSK-017/DeleteGame envia game.delete e não devolve valor", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodGameDelete: ipc.GameDeletedResult{GameID: 3}}}
		em := &gameEmitLog{}
		a := newBindingApp(p, em)

		if err := a.DeleteGame(3); err != nil {
			t.Fatalf("DeleteGame(3) err = %v, want nil", err)
		}
		c := singleCall(t, p, ipc.MethodGameDelete, 5*time.Second)
		if !sameJSON(t, rawObject(t, c)["gameId"], `3`) {
			t.Errorf("game.delete params = %s, want {\"gameId\":3}", c.params)
		}
		if got := em.recorded(); len(got) != 0 {
			t.Errorf("events emitted = %v, want none", got)
		}
	})

	t.Run("DSK-017/Catalog e AddPlatform chamam library.catalog e platform.add e devolvem views", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{
			ipc.MethodLibraryCatalog: json.RawMessage(`{"platforms":[{"id":1,"name":"PC","category":"pc","seedKey":"windows","iconPath":null,"gameCount":4}],` +
				`"genres":[{"id":5,"name":"RPG"}],"companies":[{"id":7,"name":"Team Cherry"}],"playModes":["single"]}`),
			ipc.MethodPlatformAdd: ipc.PlatformAddResult{Platform: ipc.Platform{ID: 9, Name: "Arcade X", Category: "arcade"}},
		}}
		a := &App{client: p, log: discardLog()}

		cat, err := a.Catalog()

		if err != nil {
			t.Fatalf("Catalog() err = %v", err)
		}
		singleCall(t, p, ipc.MethodLibraryCatalog, 5*time.Second)
		wantJSON(t, cat, `{"platforms":[{"id":1,"name":"PC","category":"pc","emulatorId":null,"seedKey":"windows","iconUrl":"","gameCount":4}],"genres":[{"id":5,"name":"RPG"}],`+
			`"companies":[{"id":7,"name":"Team Cherry"}],"playModes":["single"]}`)

		p2 := &gamePinger{results: p.results}
		a.client = p2
		pl, err := a.AddPlatform("Arcade X", "arcade")
		if err != nil {
			t.Fatalf("AddPlatform() err = %v", err)
		}
		c := singleCall(t, p2, ipc.MethodPlatformAdd, 5*time.Second)
		lp := rawObject(t, c)
		if !sameJSON(t, lp["name"], `"Arcade X"`) || !sameJSON(t, lp["category"], `"arcade"`) {
			t.Errorf("platform.add params = %s, want name \"Arcade X\" and category \"arcade\"", c.params)
		}
		wantJSON(t, pl, `{"platform":{"id":9,"name":"Arcade X","category":"arcade","emulatorId":null,"seedKey":null,"iconUrl":"","gameCount":0},"created":false}`)

		// An empty catalog gives [] in every list, never null.
		p3 := &gamePinger{results: map[string]any{ipc.MethodLibraryCatalog: json.RawMessage(`{"platforms":[],"genres":[],"companies":[],"playModes":[]}`)}}
		a.client = p3
		empty, err := a.Catalog()
		if err != nil {
			t.Fatalf("Catalog() with an empty catalog err = %v", err)
		}
		wantEmptyLists(t, empty, "platforms", "genres", "companies", "playModes")
	})

	t.Run("DSK-017/a UI não grava no banco nem emite evento (sem caminho de escrita)", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{
			ipc.MethodGameUpdate:     ipc.GameDetailResult{Game: ipc.GameDetail{Game: ipc.Game{ID: 3}}},
			ipc.MethodGameDelete:     ipc.GameDeletedResult{GameID: 3},
			ipc.MethodLibraryCatalog: json.RawMessage(`{"platforms":[],"genres":[],"companies":[],"playModes":[]}`),
			ipc.MethodPlatformAdd:    ipc.PlatformAddResult{Platform: ipc.Platform{ID: 9}},
		}}
		em := &gameEmitLog{}
		a := newBindingApp(p, em)

		if _, err := a.UpdateGame(3, json.RawMessage(`{}`)); err != nil {
			t.Fatalf("UpdateGame err = %v", err)
		}
		if err := a.DeleteGame(3); err != nil {
			t.Fatalf("DeleteGame err = %v", err)
		}
		if _, err := a.Catalog(); err != nil {
			t.Fatalf("Catalog err = %v", err)
		}
		if _, err := a.AddPlatform("Arcade X", "arcade"); err != nil {
			t.Fatalf("AddPlatform err = %v", err)
		}

		want := []string{ipc.MethodGameUpdate, ipc.MethodGameDelete, ipc.MethodLibraryCatalog, ipc.MethodPlatformAdd}
		var calls []string
		for _, c := range p.recorded() {
			calls = append(calls, c.method)
		}
		if !reflect.DeepEqual(calls, want) {
			t.Errorf("agent calls = %v, want %v (only the IPC methods)", calls, want)
		}
		if got := em.recorded(); len(got) != 0 {
			t.Errorf("events emitted = %v, want none", got)
		}
		if files := storeImporters(t, "."); len(files) != 0 {
			t.Errorf("files importing internal/store = %v, want none (the UI does not write the database)", files)
		}
	})
}
