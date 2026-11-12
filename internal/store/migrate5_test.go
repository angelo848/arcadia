package store_test

import (
	"database/sql"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// versionFourFile builds a database at goose version 4 (goose UpTo(4)) and runs
// seed on it, if non-nil, before closing it. It returns the path.
func versionFourFile(t *testing.T, seed func(t *testing.T, db *sql.DB)) string {
	t.Helper()
	path := dbPath(t)
	db := openRaw(t, path)
	defer db.Close()
	ctx := t.Context()

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 4); err != nil {
		t.Fatalf("goose UpTo(4): %v", err)
	}
	if v, err := provider.GetDBVersion(ctx); err != nil || v != 4 {
		t.Fatalf("precondition: goose version = %d (err=%v), want 4", v, err)
	}
	if seed != nil {
		seed(t, db)
	}
	return path
}

// seedPCPlatform inserts the PC platform and returns its id.
func seedPCPlatform(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.ExecContext(t.Context(), `INSERT INTO platform (name, category) VALUES ('PC', 'pc')`)
	if err != nil {
		t.Fatalf("insert platform: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("platform LastInsertId: %v", err)
	}
	return id
}

// seedCompany inserts a company (or reuses it) and links it to the game as
// developer or publisher (table is game_developer or game_publisher).
func seedCompany(t *testing.T, db *sql.DB, gameID int64, table, name string) {
	t.Helper()
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO company (name) VALUES (?)`, name); err != nil {
		t.Fatalf("insert company %q: %v", name, err)
	}
	var companyID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM company WHERE name = ?`, name).Scan(&companyID); err != nil {
		t.Fatalf("company id %q: %v", name, err)
	}
	if _, err := db.ExecContext(ctx,
		fmt.Sprintf(`INSERT INTO %s (game_id, company_id) VALUES (?, ?)`, table), gameID, companyID); err != nil {
		t.Fatalf("link company %q to game %d in %s: %v", name, gameID, table, err)
	}
}

// matchRowids returns the rowids of game_fts that match the FTS5 expression, sorted.
func matchRowids(t *testing.T, db *sql.DB, match string) []int64 {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT rowid FROM game_fts WHERE game_fts MATCH ? ORDER BY rowid`, match)
	if err != nil {
		t.Fatalf("MATCH %q: %v", match, err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan rowid: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// dumpRows renders every column of every row of query as one string per row,
// so that two snapshots can be compared for equality.
func dumpRows(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	var out []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan %s: %v", query, err)
		}
		out = append(out, fmt.Sprint(vals...))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// libraryDumpQueries read the rows that migration 5 must leave untouched.
var libraryDumpQueries = []string{
	`SELECT id, platform_id, title, status, favorite, total_play_seconds, play_count, created_at, updated_at, release_date
		FROM game ORDER BY id`,
	`SELECT id, game_id, started_at, last_seen_at, state, active_seconds FROM play_session ORDER BY id`,
	`SELECT id, session_id, paused_at, resumed_at, reason FROM pause_interval ORDER BY id`,
}

func TestMigrate5(t *testing.T) {
	t.Parallel()

	t.Run("STO-024/banco novo fica na versão 5 e game_fts existe", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		requireGooseVersion(t, s, 6)

		if n := countRows(t, s.DB(),
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'game_fts'`); n != 1 {
			t.Errorf("game_fts table count = %d, want 1", n)
		}
	})

	t.Run("STO-024/banco v4 com 3 jogos migra e MATCH pelo título encontra cada um", func(t *testing.T) {
		t.Parallel()
		titles := []string{"Celeste", "Hollow Knight", "Hades"}
		ids := make(map[string]int64, len(titles))
		path := versionFourFile(t, func(t *testing.T, db *sql.DB) {
			pid := seedPCPlatform(t, db)
			for _, title := range titles {
				ids[title] = mustInsertGame(t, db, pid, title, "")
			}
			if n := countRows(t, db, `SELECT COUNT(*) FROM game_fts`); n != 0 {
				t.Fatalf("precondition: game_fts rows in v4 = %d, want 0", n)
			}
		})

		s := openStore(t, path, true)
		requireGooseVersion(t, s, 6)
		for _, title := range titles {
			got := matchRowids(t, s.DB(), `"`+title+`"`)
			if !slices.Equal(got, []int64{ids[title]}) {
				t.Errorf("MATCH %q rowids = %v, want [%d]", title, got, ids[title])
			}
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game_fts`); n != int64(len(titles)) {
			t.Errorf("game_fts rows = %d, want %d (one per game)", n, len(titles))
		}
	})

	t.Run("STO-024/desenvolvedora e editora de um jogo entram no índice (MATCH pelo nome da empresa)", func(t *testing.T) {
		t.Parallel()
		var gameID int64
		path := versionFourFile(t, func(t *testing.T, db *sql.DB) {
			pid := seedPCPlatform(t, db)
			gameID = mustInsertGame(t, db, pid, "Hollow Knight", "2017-02-24")
			if _, err := db.ExecContext(t.Context(),
				`UPDATE game SET description = 'Metroidvania' WHERE id = ?`, gameID); err != nil {
				t.Fatalf("set description: %v", err)
			}
			seedCompany(t, db, gameID, "game_developer", "Team Cherry")
			seedCompany(t, db, gameID, "game_publisher", "Devolver Digital")
		})

		s := openStore(t, path, true)
		requireGooseVersion(t, s, 6)
		for _, match := range []string{"Cherry", "Devolver", "Metroidvania"} {
			got := matchRowids(t, s.DB(), match)
			if !slices.Equal(got, []int64{gameID}) {
				t.Errorf("MATCH %q rowids = %v, want [%d]", match, got, gameID)
			}
		}
	})

	t.Run("STO-024/jogos, sessões e pausas permanecem iguais após a migração", func(t *testing.T) {
		t.Parallel()
		var before [][]string
		path := versionFourFile(t, func(t *testing.T, db *sql.DB) {
			pid := seedPCPlatform(t, db)
			gid := mustInsertGame(t, db, pid, "Celeste", "2018-01-25")
			sid := insertOpenSession(t, db, gid)
			if _, err := db.ExecContext(t.Context(), `INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason)
				VALUES (?, 1700000100000, 1700000200000, 'hotkey')`, sid); err != nil {
				t.Fatalf("insert closed pause: %v", err)
			}
			for _, q := range libraryDumpQueries {
				before = append(before, dumpRows(t, db, q))
			}
		})

		s := openStore(t, path, true)
		requireGooseVersion(t, s, 6)
		for i, q := range libraryDumpQueries {
			after := dumpRows(t, s.DB(), q)
			if !slices.Equal(after, before[i]) {
				t.Errorf("rows changed by migration 5 for %s:\n before %v\n after  %v", q, before[i], after)
			}
		}
	})

	t.Run("STO-024/SchemaVersion é 5 e a versão do goose é 5", func(t *testing.T) {
		t.Parallel()
		if store.SchemaVersion != 6 {
			t.Errorf("store.SchemaVersion = %d, want 6", store.SchemaVersion)
		}
		s := openStore(t, dbPath(t), true)
		requireGooseVersion(t, s, 6)
		if v := countRows(t, s.DB(),
			`SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`); v != 6 {
			t.Errorf("goose_db_version max applied version = %d, want 6", v)
		}
	})

	t.Run("STO-024/Down apaga só as linhas de game_fts e mantém os jogos", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		db := openRaw(t, dbPath(t))
		defer db.Close()

		provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
		if err != nil {
			t.Fatalf("goose.NewProvider: %v", err)
		}
		if _, err := provider.UpTo(ctx, 4); err != nil {
			t.Fatalf("goose UpTo(4): %v", err)
		}
		pid := seedPCPlatform(t, db)
		mustInsertGame(t, db, pid, "Celeste", "")
		mustInsertGame(t, db, pid, "Hades", "")

		if _, err := provider.UpTo(ctx, 5); err != nil {
			t.Fatalf("goose Up to 5: %v", err)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM game_fts`); n != 2 {
			t.Fatalf("precondition: game_fts rows after Up = %d, want 2", n)
		}

		if _, err := provider.Down(ctx); err != nil {
			t.Fatalf("goose Down: %v", err)
		}
		if v, err := provider.GetDBVersion(ctx); err != nil || v != 4 {
			t.Fatalf("goose version after Down = %d (err=%v), want 4", v, err)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'game_fts'`); n != 1 {
			t.Errorf("game_fts table count after Down = %d, want 1 (only the rows go)", n)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM game_fts`); n != 0 {
			t.Errorf("game_fts rows after Down = %d, want 0", n)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM game`); n != 2 {
			t.Errorf("game rows after Down = %d, want 2", n)
		}
	})
}
