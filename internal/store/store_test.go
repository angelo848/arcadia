package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite" // raw driver "sqlite" for fixtures that bypass store.Open

	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// schemaColumns is the data model of docs/ARCHITECTURE.md §5, written out
// explicitly. The migration may add columns, but every column listed here
// must exist. game_fts (FTS5) is checked separately.
var schemaColumns = map[string][]string{
	"platform": {"id", "name", "category", "icon", "emulator_id"},
	"game": {
		"id", "platform_id", "title", "sort_title", "release_date", "description", "install_dir",
		"launch_type", "launch_target", "launch_args", "working_dir",
		"rating", "favorite", "hidden",
		"status",
		"first_played_at",
		"completed_at",
		"total_play_seconds",
		"last_played_at", "play_count",
		"hltb_main_hours", "hltb_extra_hours", "hltb_complete_hours",
		"created_at", "updated_at",
	},
	"company":        {"id", "name"},
	"game_developer": {"game_id", "company_id"},
	"game_publisher": {"game_id", "company_id"},
	"genre":          {"id", "name"},
	"game_genre":     {"game_id", "genre_id"},
	"play_mode":      {"id", "name"},
	"game_play_mode": {"game_id", "play_mode_id"},
	"media_asset":    {"id", "game_id", "kind", "path", "thumb_path", "source", "width", "height", "region", "is_primary"},
	"additional_app": {"id", "game_id", "name", "target", "args", "track_time"},
	"tracking_rule":  {"id", "game_id", "kind", "value", "learned"},
	"emulator":       {"id", "name", "exe_path", "args_template"},
	"play_session":   {"id", "game_id", "started_at", "ended_at", "last_seen_at", "active_seconds", "state", "source", "end_reason"},
	"pause_interval": {"id", "session_id", "paused_at", "resumed_at", "reason"},
	"playlist":       {"id", "name", "filter_json"},
}

// knownTimeColumns are the *_at columns named in §5; the STO-006 scan must at
// least see all of them (guards against a scan that silently checks nothing).
var knownTimeColumns = []string{
	"game.first_played_at", "game.completed_at", "game.last_played_at", "game.created_at", "game.updated_at",
	"play_session.started_at", "play_session.ended_at", "play_session.last_seen_at",
	"pause_interval.paused_at", "pause_interval.resumed_at",
}

// --- helpers ---------------------------------------------------------------

func dbPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "library.db")
}

// openStore opens a store and registers Close in t.Cleanup.
func openStore(t *testing.T, path string, migrate bool) *store.Store {
	t.Helper()
	s, err := store.Open(t.Context(), path, store.OpenOptions{Migrate: migrate})
	if err != nil {
		t.Fatalf("store.Open(%q, Migrate=%v): %v", path, migrate, err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Store.Close: %v", err)
		}
	})
	return s
}

// migratedFile creates a migrated database file and closes the store, so the
// caller can reopen it as it wishes.
func migratedFile(t *testing.T) string {
	t.Helper()
	path := dbPath(t)
	s, err := store.Open(t.Context(), path, store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open(Migrate=true): %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Store.Close: %v", err)
	}
	return path
}

// openRaw opens the file with the bare modernc driver (no store pragmas).
func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open(sqlite, %q): %v", path, err)
	}
	return db
}

func execRaw(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db := openRaw(t, path)
	defer db.Close()
	for _, q := range stmts {
		if _, err := db.ExecContext(t.Context(), q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
}

// gooseTableDDL mirrors the table goose (sqlite3 dialect) creates.
const gooseTableDDL = `CREATE TABLE goose_db_version (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	version_id INTEGER NOT NULL,
	is_applied INTEGER NOT NULL,
	tstamp TIMESTAMP DEFAULT (datetime('now'))
)`

// olderVersionFile creates a database whose goose version is 0 (the row goose
// writes when it creates its table) and that has none of the §5 tables, i.e. a
// schema older than store.SchemaVersion.
func olderVersionFile(t *testing.T) string {
	t.Helper()
	path := dbPath(t)
	execRaw(t, path,
		gooseTableDDL,
		`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1)`,
		`CREATE TABLE legacy_marker (x INTEGER)`,
	)
	return path
}

func tableNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return names
}

type columnInfo struct {
	Name string
	Type string
}

func tableColumns(t *testing.T, db *sql.DB, table string) []columnInfo {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer rows.Close()
	var cols []columnInfo
	for rows.Next() {
		var (
			cid     int
			c       columnInfo
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &c.Name, &c.Type, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info(%s): %v", table, err)
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return cols
}

func columnNames(cols []columnInfo) []string {
	names := make([]string, 0, len(cols))
	for _, c := range cols {
		names = append(names, c.Name)
	}
	return names
}

// indexColumns returns the key columns (in order) of every index on table.
func indexColumns(t *testing.T, db *sql.DB, table string) map[string][]string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), fmt.Sprintf("PRAGMA index_list(%q)", table))
	if err != nil {
		t.Fatalf("PRAGMA index_list(%s): %v", table, err)
	}
	var names []string
	for rows.Next() {
		var (
			seq     int
			name    string
			unique  int
			origin  string
			partial int
		)
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			rows.Close()
			t.Fatalf("scan index_list(%s): %v", table, err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	rows.Close()

	out := make(map[string][]string, len(names))
	for _, idx := range names {
		r, err := db.QueryContext(t.Context(), fmt.Sprintf("PRAGMA index_info(%q)", idx))
		if err != nil {
			t.Fatalf("PRAGMA index_info(%s): %v", idx, err)
		}
		var cols []string
		for r.Next() {
			var (
				seqno int
				cid   int
				name  sql.NullString
			)
			if err := r.Scan(&seqno, &cid, &name); err != nil {
				r.Close()
				t.Fatalf("scan index_info(%s): %v", idx, err)
			}
			cols = append(cols, name.String)
		}
		if err := r.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		r.Close()
		out[idx] = cols
	}
	return out
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

type gooseRow struct {
	Version int64
	Applied bool
}

func gooseRows(t *testing.T, db *sql.DB) []gooseRow {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT version_id, is_applied FROM goose_db_version ORDER BY id`)
	if err != nil {
		t.Fatalf("read goose_db_version: %v", err)
	}
	defer rows.Close()
	var out []gooseRow
	for rows.Next() {
		var r gooseRow
		if err := rows.Scan(&r.Version, &r.Applied); err != nil {
			t.Fatalf("scan goose_db_version: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func insertPlatform(t *testing.T, s *store.Store, name string) int64 {
	t.Helper()
	id, err := s.Queries().InsertPlatform(t.Context(), dbgen.InsertPlatformParams{Name: name, Category: "pc"})
	if err != nil {
		t.Fatalf("InsertPlatform(%q): %v", name, err)
	}
	return id
}

// insertGame inserts a game filling every column that a reasonable migration
// could declare NOT NULL, with values from the §5 enums. releaseDate may be "".
func insertGame(db *sql.DB, ctx context.Context, platformID int64, title, releaseDate string) (int64, error) {
	var rd any
	if releaseDate != "" {
		rd = releaseDate
	}
	res, err := db.ExecContext(ctx, `INSERT INTO game (
		platform_id, title, sort_title, release_date, launch_type, launch_target,
		status, favorite, hidden, total_play_seconds, play_count, created_at, updated_at
	) VALUES (?, ?, ?, ?, 'exe', 'C:\Games\x.exe', 'not_played', 0, 0, 0, 0, ?, ?)`,
		platformID, title, title, rd, int64(1_700_000_000_000), int64(1_700_000_000_000))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func mustInsertGame(t *testing.T, db *sql.DB, platformID int64, title, releaseDate string) int64 {
	t.Helper()
	id, err := insertGame(db, t.Context(), platformID, title, releaseDate)
	if err != nil {
		t.Fatalf("insert game %q: %v", title, err)
	}
	return id
}

// mentionsVersion reports whether msg mentions version v as a standalone
// number, after removing every rendering of path (raw and %q-quoted) so that
// digits in the t.TempDir() path cannot satisfy the check.
func mentionsVersion(msg, path string, v int64) bool {
	quoted := strconv.Quote(path)
	for _, p := range []string{quoted, quoted[1 : len(quoted)-1], path, filepath.ToSlash(path)} {
		msg = strings.ReplaceAll(msg, p, "")
	}
	re := regexp.MustCompile(`\b` + strconv.FormatInt(v, 10) + `\b`)
	return re.MatchString(msg)
}

func requireMismatch(t *testing.T, s *store.Store, err error) {
	t.Helper()
	if s != nil {
		_ = s.Close()
		t.Errorf("store.Open returned a non-nil *Store together with err=%v", err)
	}
	if !errors.Is(err, store.ErrSchemaMismatch) {
		t.Fatalf("store.Open err = %v, want errors.Is(err, store.ErrSchemaMismatch)", err)
	}
}

// --- STO-001 ---------------------------------------------------------------

func TestOpenMigrateNewDatabase(t *testing.T) {
	t.Parallel()
	path := dbPath(t)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("precondition: %s should not exist (err=%v)", path, err)
	}
	s := openStore(t, path, true)
	db := s.DB()

	t.Run("STO-001/tabelas do §5 existem", func(t *testing.T) {
		got := tableNames(t, db)
		want := []string{"game_fts"}
		for table := range schemaColumns {
			want = append(want, table)
		}
		sort.Strings(want)
		for _, w := range want {
			if !slices.Contains(got, w) {
				t.Errorf("table %q missing; tables = %v", w, got)
			}
		}
	})

	t.Run("STO-001/colunas do §5 existem", func(t *testing.T) {
		for table, want := range schemaColumns {
			got := columnNames(tableColumns(t, db, table))
			for _, col := range want {
				if !slices.Contains(got, col) {
					t.Errorf("%s.%s missing; columns = %v", table, col, got)
				}
			}
		}
	})

	t.Run("STO-001/game_fts é FTS5 com title, description, developer, publisher", func(t *testing.T) {
		var ddl string
		if err := db.QueryRowContext(t.Context(),
			`SELECT sql FROM sqlite_master WHERE name = 'game_fts'`).Scan(&ddl); err != nil {
			t.Fatalf("read game_fts DDL: %v", err)
		}
		norm := strings.ToLower(strings.Join(strings.Fields(ddl), " "))
		if !strings.Contains(norm, "create virtual table") || !strings.Contains(norm, "using fts5") {
			t.Errorf("game_fts DDL = %q, want CREATE VIRTUAL TABLE ... USING fts5", ddl)
		}
		got := columnNames(tableColumns(t, db, "game_fts"))
		for _, col := range []string{"title", "description", "developer", "publisher"} {
			if !slices.Contains(got, col) {
				t.Errorf("game_fts.%s missing; columns = %v", col, got)
			}
		}
		// The FTS5 module must actually answer a MATCH query.
		if _, err := db.ExecContext(t.Context(),
			`SELECT rowid FROM game_fts WHERE game_fts MATCH 'zelda'`); err != nil {
			t.Errorf("MATCH on game_fts: %v", err)
		}
	})

	t.Run("STO-001/índices criados", func(t *testing.T) {
		cases := []struct {
			table string
			cols  []string
		}{
			{"game", []string{"platform_id"}},
			{"play_session", []string{"game_id", "started_at"}},
		}
		for _, c := range cases {
			idx := indexColumns(t, db, c.table)
			found := false
			for _, cols := range idx {
				if len(cols) >= len(c.cols) && slices.Equal(cols[:len(c.cols)], c.cols) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no index on %s(%s); indexes = %v", c.table, strings.Join(c.cols, ", "), idx)
			}
		}
	})

	t.Run("STO-001/journal_mode é wal", func(t *testing.T) {
		var mode string
		if err := db.QueryRowContext(t.Context(), `PRAGMA journal_mode`).Scan(&mode); err != nil {
			t.Fatalf("PRAGMA journal_mode: %v", err)
		}
		if !strings.EqualFold(mode, "wal") {
			t.Errorf("journal_mode = %q, want wal", mode)
		}
	})
}

// --- STO-002 ---------------------------------------------------------------

func TestReopenWithMigrationIsNoop(t *testing.T) {
	t.Parallel()
	path := dbPath(t)

	first, err := store.Open(t.Context(), path, store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	platformID := insertPlatform(t, first, "Super Nintendo")
	gameID := mustInsertGame(t, first.DB(), platformID, "Chrono Trigger", "1995-03-11")
	gooseBefore := countRows(t, first.DB(), `SELECT COUNT(*) FROM goose_db_version`)
	if err := first.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	s := openStore(t, path, true)

	t.Run("STO-002/reabrir com migração mantém a versão", func(t *testing.T) {
		v, err := s.CurrentVersion(t.Context())
		if err != nil {
			t.Fatalf("CurrentVersion: %v", err)
		}
		if v != store.SchemaVersion {
			t.Errorf("CurrentVersion = %d, want %d", v, store.SchemaVersion)
		}
	})

	t.Run("STO-002/goose_db_version não ganha linhas", func(t *testing.T) {
		after := countRows(t, s.DB(), `SELECT COUNT(*) FROM goose_db_version`)
		if after != gooseBefore {
			t.Errorf("goose_db_version rows = %d after reopen, want %d", after, gooseBefore)
		}
	})

	t.Run("STO-002/dados permanecem", func(t *testing.T) {
		platforms, err := s.Queries().ListPlatforms(t.Context())
		if err != nil {
			t.Fatalf("ListPlatforms: %v", err)
		}
		if len(platforms) != 1 || platforms[0].Name != "Super Nintendo" {
			t.Errorf("ListPlatforms = %+v, want one platform named Super Nintendo", platforms)
		}
		var title string
		if err := s.DB().QueryRowContext(t.Context(),
			`SELECT title FROM game WHERE id = ?`, gameID).Scan(&title); err != nil {
			t.Fatalf("read game %d: %v", gameID, err)
		}
		if title != "Chrono Trigger" {
			t.Errorf("game title = %q, want Chrono Trigger", title)
		}
		n, err := s.Queries().CountGames(t.Context())
		if err != nil {
			t.Fatalf("CountGames: %v", err)
		}
		if n != 1 {
			t.Errorf("CountGames = %d, want 1", n)
		}
	})
}

// --- STO-003 ---------------------------------------------------------------

func TestPlayModeSeeds(t *testing.T) {
	t.Parallel()
	s := openStore(t, dbPath(t), true)

	t.Run("STO-003/seeds presentes", func(t *testing.T) {
		rows, err := s.DB().QueryContext(t.Context(), `SELECT name FROM play_mode ORDER BY name`)
		if err != nil {
			t.Fatalf("select play_mode: %v", err)
		}
		defer rows.Close()
		var got []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				t.Fatalf("scan: %v", err)
			}
			got = append(got, n)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		want := []string{"coop_local", "coop_online", "multi_local", "multi_online", "single"}
		if !slices.Equal(got, want) {
			t.Errorf("play_mode names = %v, want exactly %v", got, want)
		}
	})
}

// --- STO-004 ---------------------------------------------------------------

func TestForeignKeys(t *testing.T) {
	t.Parallel()

	t.Run("STO-004/game com platform_id inexistente falha", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ctx := t.Context()

		// Control: the same row with a valid platform succeeds, so the failure
		// below can only come from the foreign key.
		pid := insertPlatform(t, s, "PC")
		if _, err := insertGame(s.DB(), ctx, pid, "Control", ""); err != nil {
			t.Fatalf("control insert with valid platform_id failed: %v", err)
		}

		_, err := insertGame(s.DB(), ctx, pid+9999, "Orphan", "")
		if err == nil {
			t.Fatal("insert game with nonexistent platform_id succeeded, want FOREIGN KEY error")
		}
		if !strings.Contains(strings.ToUpper(err.Error()), "FOREIGN KEY") {
			t.Errorf("insert error = %v, want a FOREIGN KEY constraint failure", err)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game WHERE title = 'Orphan'`); n != 0 {
			t.Errorf("orphan game rows = %d, want 0", n)
		}
	})

	t.Run("STO-004/FK ativa em todas as conexões do pool", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		ctx := t.Context()

		const n = 4
		db.SetMaxOpenConns(n)
		// Holding n connections at the same time forces n distinct physical connections.
		conns := make([]*sql.Conn, 0, n)
		defer func() {
			for _, c := range conns {
				_ = c.Close()
			}
		}()
		for i := range n {
			c, err := db.Conn(ctx)
			if err != nil {
				t.Fatalf("db.Conn #%d: %v", i, err)
			}
			conns = append(conns, c)
		}
		for i, c := range conns {
			var fk int
			if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
				t.Fatalf("conn #%d PRAGMA foreign_keys: %v", i, err)
			}
			if fk != 1 {
				t.Errorf("conn #%d foreign_keys = %d, want 1", i, fk)
			}
		}
	})
}

// --- STO-005 ---------------------------------------------------------------

func TestConcurrentStores(t *testing.T) {
	t.Parallel()

	t.Run("STO-005/dois Store, escrita e leitura paralelas sem SQLITE_BUSY", func(t *testing.T) {
		path := dbPath(t)
		// Two independent Stores (two pools) on the same file, as agent and UI.
		// Each Store hosts one writer and one reader.
		stores := []*store.Store{openStore(t, path, true), openStore(t, path, false)}
		reader := stores[1]
		ctx := t.Context()

		const (
			writers    = 2
			readers    = 2
			iterations = 50
		)
		errs := make(chan error, (writers+readers)*iterations*2)
		start := make(chan struct{})
		var wg sync.WaitGroup

		for w := range writers {
			writer := stores[w%len(stores)]
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for i := range iterations {
					name := fmt.Sprintf("platform-%d-%d", w, i)
					// Read before write inside the Tx: with BEGIN DEFERRED the
					// read snapshot cannot be upgraded after the other writer
					// commits (SQLITE_BUSY, not retried by busy_timeout); with
					// BEGIN IMMEDIATE the write lock is taken up front.
					err := writer.Tx(ctx, func(q *dbgen.Queries) error {
						if _, err := q.CountGames(ctx); err != nil {
							return fmt.Errorf("CountGames: %w", err)
						}
						_, err := q.InsertPlatform(ctx, dbgen.InsertPlatformParams{Name: name, Category: "pc"})
						return err
					})
					if err != nil {
						errs <- fmt.Errorf("writer %d iteration %d: %w", w, i, err)
					}
				}
			}()
		}
		for r := range readers {
			reader := stores[r%len(stores)]
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for i := range iterations {
					if _, err := reader.LibrarySummary(ctx); err != nil {
						errs <- fmt.Errorf("reader %d iteration %d LibrarySummary: %w", r, i, err)
					}
					if _, err := reader.Queries().CountGames(ctx); err != nil {
						errs <- fmt.Errorf("reader %d iteration %d CountGames: %w", r, i, err)
					}
				}
			}()
		}
		close(start)
		wg.Wait()
		close(errs)

		for err := range errs {
			t.Error(err) // any error fails; SQLITE_BUSY is the one this requirement targets
		}

		sum, err := reader.LibrarySummary(ctx)
		if err != nil {
			t.Fatalf("final LibrarySummary: %v", err)
		}
		if want := int64(writers * iterations); sum.Platforms != want {
			t.Errorf("Platforms = %d, want %d", sum.Platforms, want)
		}
		if sum.Games != 0 {
			t.Errorf("Games = %d, want 0", sum.Games)
		}
	})
}

// --- STO-006 ---------------------------------------------------------------

func TestTimeColumnTypes(t *testing.T) {
	t.Parallel()
	s := openStore(t, dbPath(t), true)
	db := s.DB()

	t.Run("STO-006/toda coluna _at é INTEGER", func(t *testing.T) {
		var seen []string
		for _, table := range tableNames(t, db) {
			for _, c := range tableColumns(t, db, table) {
				if !strings.HasSuffix(c.Name, "_at") {
					continue
				}
				seen = append(seen, table+"."+c.Name)
				if !strings.EqualFold(c.Type, "INTEGER") {
					t.Errorf("%s.%s type = %q, want INTEGER (ADR 0009)", table, c.Name, c.Type)
				}
			}
		}
		for _, k := range knownTimeColumns {
			if !slices.Contains(seen, k) {
				t.Errorf("time column %s not found by the scan; seen = %v", k, seen)
			}
		}
	})

	t.Run("STO-006/release_date é TEXT", func(t *testing.T) {
		for _, c := range tableColumns(t, db, "game") {
			if c.Name == "release_date" {
				if !strings.EqualFold(c.Type, "TEXT") {
					t.Errorf("game.release_date type = %q, want TEXT", c.Type)
				}
				return
			}
		}
		t.Error("game.release_date not found")
	})
}

// --- STO-008 ---------------------------------------------------------------

func TestOpenWithoutMigrateCompatible(t *testing.T) {
	t.Parallel()

	t.Run("STO-008/abre sem migrar banco atual e consulta", func(t *testing.T) {
		path := dbPath(t)
		agent := openStore(t, path, true)
		pid := insertPlatform(t, agent, "PlayStation")
		mustInsertGame(t, agent.DB(), pid, "Final Fantasy VII", "1997-01-31")
		if err := agent.Close(); err != nil {
			t.Fatalf("agent Close: %v", err)
		}

		ui := openStore(t, path, false)
		ctx := t.Context()

		v, err := ui.CurrentVersion(ctx)
		if err != nil {
			t.Fatalf("CurrentVersion: %v", err)
		}
		if v != store.SchemaVersion {
			t.Errorf("CurrentVersion = %d, want %d", v, store.SchemaVersion)
		}
		sum, err := ui.LibrarySummary(ctx)
		if err != nil {
			t.Fatalf("LibrarySummary: %v", err)
		}
		if sum != (store.LibrarySummary{Games: 1, Platforms: 1}) {
			t.Errorf("LibrarySummary = %+v, want {Games:1 Platforms:1}", sum)
		}
		n, err := ui.Queries().CountGames(ctx)
		if err != nil {
			t.Fatalf("CountGames: %v", err)
		}
		if n != 1 {
			t.Errorf("CountGames = %d, want 1", n)
		}
		platforms, err := ui.Queries().ListPlatforms(ctx)
		if err != nil {
			t.Fatalf("ListPlatforms: %v", err)
		}
		if len(platforms) != 1 || platforms[0].Name != "PlayStation" {
			t.Errorf("ListPlatforms = %+v, want one platform named PlayStation", platforms)
		}
	})

	t.Run("STO-008/caminho com espaço e acentos", func(t *testing.T) {
		t.Parallel()
		// Characters that break a naive "file:" DSN: space, '#', '%' and non-ASCII.
		dir := filepath.Join(t.TempDir(), "a b#ç%")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("Mkdir(%q): %v", dir, err)
		}
		path := filepath.Join(dir, "library.db")

		agent, err := store.Open(t.Context(), path, store.OpenOptions{Migrate: true})
		if err != nil {
			t.Fatalf("store.Open(%q, Migrate=true): %v", path, err)
		}
		insertPlatform(t, agent, "Mega Drive")
		if err := agent.Close(); err != nil {
			t.Fatalf("agent Close: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("database not created at the literal path %q: %v", path, err)
		}

		ui := openStore(t, path, false)
		ctx := t.Context()
		v, err := ui.CurrentVersion(ctx)
		if err != nil {
			t.Fatalf("CurrentVersion: %v", err)
		}
		if v != store.SchemaVersion {
			t.Errorf("CurrentVersion = %d, want %d", v, store.SchemaVersion)
		}
		platforms, err := ui.Queries().ListPlatforms(ctx)
		if err != nil {
			t.Fatalf("ListPlatforms: %v", err)
		}
		if len(platforms) != 1 || platforms[0].Name != "Mega Drive" {
			t.Errorf("ListPlatforms = %+v, want one platform named Mega Drive", platforms)
		}
	})
}

// --- STO-009 ---------------------------------------------------------------

func TestOpenWithoutMigrateMismatch(t *testing.T) {
	t.Parallel()

	t.Run("STO-009/arquivo inexistente", func(t *testing.T) {
		t.Parallel()
		s, err := store.Open(t.Context(), dbPath(t), store.OpenOptions{Migrate: false})
		requireMismatch(t, s, err)
	})

	t.Run("STO-009/banco sem goose_db_version", func(t *testing.T) {
		t.Parallel()
		path := dbPath(t)
		// A real SQLite file with one unrelated table and no goose table.
		execRaw(t, path, `CREATE TABLE unrelated (x INTEGER)`)
		s, err := store.Open(t.Context(), path, store.OpenOptions{Migrate: false})
		requireMismatch(t, s, err)
	})

	t.Run("STO-009/versão menor", func(t *testing.T) {
		t.Parallel()
		// goose_db_version holding only the bootstrap version 0 (< SchemaVersion).
		path := olderVersionFile(t)
		s, err := store.Open(t.Context(), path, store.OpenOptions{Migrate: false})
		requireMismatch(t, s, err)
		msg := err.Error()
		if !mentionsVersion(msg, path, 0) || !mentionsVersion(msg, path, store.SchemaVersion) {
			t.Errorf("error %q should mention both versions (found 0, expected %d)", msg, store.SchemaVersion)
		}
	})

	t.Run("STO-009/versão maior", func(t *testing.T) {
		t.Parallel()
		// A migrated database plus an applied row for a future version.
		path := migratedFile(t)
		future := store.SchemaVersion + 1
		execRaw(t, path, fmt.Sprintf(
			`INSERT INTO goose_db_version (version_id, is_applied) VALUES (%d, 1)`, future))
		s, err := store.Open(t.Context(), path, store.OpenOptions{Migrate: false})
		requireMismatch(t, s, err)
		msg := err.Error()
		if !mentionsVersion(msg, path, future) || !mentionsVersion(msg, path, store.SchemaVersion) {
			t.Errorf("error %q should mention both versions (found %d, expected %d)", msg, future, store.SchemaVersion)
		}
	})
}

// --- STO-010 ---------------------------------------------------------------

func TestOpenWithoutMigrateDoesNotModify(t *testing.T) {
	t.Parallel()

	t.Run("STO-010/arquivo inexistente não é criado", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "library.db")
		s, err := store.Open(t.Context(), path, store.OpenOptions{Migrate: false})
		requireMismatch(t, s, err)
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("os.Stat(%s) err = %v, want not-exist", path, statErr)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		if len(entries) != 0 {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("directory should stay empty, has %v", names)
		}
	})

	t.Run("STO-010/versão e tabelas inalteradas após mismatch", func(t *testing.T) {
		t.Parallel()
		fixtures := []struct {
			name  string
			setup func(t *testing.T) string
		}{
			{"versão 0 (anterior)", olderVersionFile},
			{"sem goose_db_version", func(t *testing.T) string {
				p := dbPath(t)
				execRaw(t, p, `CREATE TABLE unrelated (x INTEGER)`)
				return p
			}},
		}
		for _, f := range fixtures {
			path := f.setup(t)

			raw := openRaw(t, path)
			tablesBefore := tableNames(t, raw)
			var gooseBefore []gooseRow
			hasGoose := slices.Contains(tablesBefore, "goose_db_version")
			if hasGoose {
				gooseBefore = gooseRows(t, raw)
			}
			if err := raw.Close(); err != nil {
				t.Fatalf("%s: close raw: %v", f.name, err)
			}

			s, err := store.Open(t.Context(), path, store.OpenOptions{Migrate: false})
			requireMismatch(t, s, err)

			raw = openRaw(t, path)
			tablesAfter := tableNames(t, raw)
			if !slices.Equal(tablesAfter, tablesBefore) {
				t.Errorf("%s: tables after = %v, want unchanged %v", f.name, tablesAfter, tablesBefore)
			}
			if hasGoose {
				if gooseAfter := gooseRows(t, raw); !slices.Equal(gooseAfter, gooseBefore) {
					t.Errorf("%s: goose_db_version after = %v, want unchanged %v", f.name, gooseAfter, gooseBefore)
				}
			}
			if err := raw.Close(); err != nil {
				t.Fatalf("%s: close raw: %v", f.name, err)
			}
		}
	})
}
