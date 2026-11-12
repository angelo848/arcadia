package store_test

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Fixtures of the library read tests. The games are written with raw SQL because
// the write API (UpdateGame) lands in a later task. The game_fts row uses the
// migration 5 expression, so the index is in the state the agent keeps it in
// (ADR 0020).

// dbx is the part of *sql.DB and *sql.Tx that the fixture helpers use.
type dbx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// libGame describes one game of a fixture. Zero values mean: platform PC, no
// release date, status not_played, no play, no dates, no description, no links.
type libGame struct {
	Title       string
	Platform    string // "" = "PC"
	Release     string // "" = NULL; any text is stored as given
	Status      string // "" = "not_played"
	Favorite    bool
	Seconds     int64 // total_play_seconds
	Plays       int64 // play_count
	FirstPlayed int64 // ms UTC; 0 = NULL
	LastPlayed  int64 // ms UTC; 0 = NULL
	Completed   int64 // ms UTC; 0 = NULL
	Description string
	Genres      []string
	Modes       []string
	Developers  []string
	Publishers  []string
}

// libAt is 2026-11-<day> <hour>:00 UTC in ms. It never reads the clock.
func libAt(day, hour int) int64 {
	return time.Date(2026, time.November, day, hour, 0, 0, 0, time.UTC).UnixMilli()
}

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }
func boolPtr(v bool) *bool    { return &v }

// libFilter is the empty FilterSpec (FLT-008).
func libFilter() domain.FilterSpec { return domain.FilterSpec{Version: 1} }

// libQuery is a first-page query with the default limit and the given sort.
func libQuery(f domain.FilterSpec, sort domain.Sort) store.Query {
	return store.Query{Filter: f, Sort: sort, Limit: domain.DefaultPageLimit}
}

// libRun runs ListGames and fails the test on error.
func libRun(t *testing.T, s *store.Store, q store.Query) store.Page {
	t.Helper()
	page, err := s.ListGames(t.Context(), q)
	if err != nil {
		t.Fatalf("ListGames(%+v): %v", q, err)
	}
	return page
}

// libSearch returns the games of the first page for f, ordered by title.
func libSearch(t *testing.T, s *store.Store, f domain.FilterSpec) []store.Game {
	t.Helper()
	return libRun(t, s, libQuery(f, domain.SortTitle)).Games
}

// libTitles returns the titles of games in order.
func libTitles(games []store.Game) []string {
	out := make([]string, 0, len(games))
	for _, g := range games {
		out = append(out, g.Title)
	}
	return out
}

// requireTitles checks the titles of games, in order.
func requireTitles(t *testing.T, games []store.Game, want ...string) {
	t.Helper()
	if got := libTitles(games); !slices.Equal(got, want) {
		t.Errorf("titles = %v, want %v", got, want)
	}
}

// libLookup returns the id of the row of table (platform, genre or company)
// named name.
func libLookup(t *testing.T, s *store.Store, table, name string) int64 {
	t.Helper()
	var id int64
	if err := s.DB().QueryRowContext(t.Context(),
		`SELECT id FROM `+table+` WHERE name = ?`, name).Scan(&id); err != nil {
		t.Fatalf("lookup %s %q: %v", table, name, err)
	}
	return id
}

// seedLibraryIDs inserts the games in one transaction and returns their ids in
// the same order. Platforms, genres and companies are created when missing.
func seedLibraryIDs(t *testing.T, s *store.Store, games ...libGame) []int64 {
	t.Helper()
	tx, err := s.DB().BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin fixture tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	ids := make([]int64, 0, len(games))
	for _, g := range games {
		ids = append(ids, libInsertGame(t, tx, g))
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit fixture tx: %v", err)
	}
	return ids
}

// seedLibrary is seedLibraryIDs keyed by title (titles must be unique).
func seedLibrary(t *testing.T, s *store.Store, games ...libGame) map[string]int64 {
	t.Helper()
	ids := seedLibraryIDs(t, s, games...)
	out := make(map[string]int64, len(games))
	for i, g := range games {
		out[g.Title] = ids[i]
	}
	return out
}

const libInsertGameSQL = `INSERT INTO game (
	platform_id, title, sort_title, release_date, launch_type, launch_target, working_dir,
	status, favorite, hidden, total_play_seconds, play_count,
	first_played_at, last_played_at, completed_at, description, created_at, updated_at
) VALUES (?, ?, ?, ?, 'exe', ?, '', ?, ?, 0, ?, ?, ?, ?, ?, ?, 1700000000000, 1700000000000)`

// libReindexSQL is the migration 5 expression for one game (STO-024, FTS-001).
const libReindexSQL = `INSERT INTO game_fts (rowid, title, description, developer, publisher)
SELECT g.id, g.title, g.description,
	COALESCE((SELECT group_concat(c.name, ' ') FROM game_developer gd
		JOIN company c ON c.id = gd.company_id WHERE gd.game_id = g.id), ''),
	COALESCE((SELECT group_concat(c.name, ' ') FROM game_publisher gp
		JOIN company c ON c.id = gp.company_id WHERE gp.game_id = g.id), '')
FROM game g WHERE g.id = ?`

func libExec(t *testing.T, x dbx, query string, args ...any) {
	t.Helper()
	if _, err := x.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func libNullMs(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// libPlatform returns the id of the platform name, creating it when missing.
// Every fixture platform has category pc; the category is not under test here.
func libPlatform(t *testing.T, x dbx, name string) int64 {
	t.Helper()
	libExec(t, x, `INSERT OR IGNORE INTO platform (name, category) VALUES (?, 'pc')`, name)
	var id int64
	if err := x.QueryRowContext(t.Context(), `SELECT id FROM platform WHERE name = ?`, name).Scan(&id); err != nil {
		t.Fatalf("platform id %q: %v", name, err)
	}
	return id
}

// libInsertGame writes one game, its links and its game_fts row.
func libInsertGame(t *testing.T, x dbx, g libGame) int64 {
	t.Helper()
	platform := g.Platform
	if platform == "" {
		platform = "PC"
	}
	status := g.Status
	if status == "" {
		status = "not_played"
	}
	favorite := 0
	if g.Favorite {
		favorite = 1
	}
	var release any
	if g.Release != "" {
		release = g.Release
	}
	res, err := x.ExecContext(t.Context(), libInsertGameSQL,
		libPlatform(t, x, platform), g.Title, g.Title, release, `C:\Games\`+g.Title+`.exe`,
		status, favorite, g.Seconds, g.Plays,
		libNullMs(g.FirstPlayed), libNullMs(g.LastPlayed), libNullMs(g.Completed), g.Description)
	if err != nil {
		t.Fatalf("insert game %q: %v", g.Title, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("id of game %q: %v", g.Title, err)
	}
	for _, name := range g.Genres {
		libExec(t, x, `INSERT OR IGNORE INTO genre (name) VALUES (?)`, name)
		libExec(t, x, `INSERT INTO game_genre (game_id, genre_id) SELECT ?, id FROM genre WHERE name = ?`, id, name)
	}
	for _, mode := range g.Modes {
		libExec(t, x, `INSERT INTO game_play_mode (game_id, play_mode_id) SELECT ?, id FROM play_mode WHERE name = ?`, id, mode)
	}
	for _, name := range g.Developers {
		libExec(t, x, `INSERT OR IGNORE INTO company (name) VALUES (?)`, name)
		libExec(t, x, `INSERT INTO game_developer (game_id, company_id) SELECT ?, id FROM company WHERE name = ?`, id, name)
	}
	for _, name := range g.Publishers {
		libExec(t, x, `INSERT OR IGNORE INTO company (name) VALUES (?)`, name)
		libExec(t, x, `INSERT INTO game_publisher (game_id, company_id) SELECT ?, id FROM company WHERE name = ?`, id, name)
	}
	libExec(t, x, `DELETE FROM game_fts WHERE rowid = ?`, id)
	libExec(t, x, libReindexSQL, id)
	return id
}
