package store_test

// Tests of migration 00011 (phase 16, SED-001): play_session.edited_at and the index
// play_session_started_at_idx (design 3.1). Nothing else changes.

import (
	"database/sql"
	"os"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// versionTenFile builds a database at goose version 10, runs seed on it if non-nil, closes it
// and returns the path. Opening it with store.Open(Migrate) then applies 00011.
func versionTenFile(t *testing.T, seed func(t *testing.T, db *sql.DB)) string {
	t.Helper()
	path := dbPath(t)
	db := openRaw(t, path)
	defer db.Close()
	ctx := t.Context()

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 10); err != nil {
		t.Fatalf("goose UpTo(10): %v", err)
	}
	if v, err := provider.GetDBVersion(ctx); err != nil || v != 10 {
		t.Fatalf("precondition: goose version = %d (err=%v), want 10", v, err)
	}
	if seed != nil {
		seed(t, db)
	}
	return path
}

// m11Session inserts a play_session row without edited_at (valid at versions 10 and 11).
func m11Session(t *testing.T, db *sql.DB, gameID int64, startedMs int64, endedMs any, active int64, state, source string) {
	t.Helper()
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		gameID, startedMs, endedMs, startedMs, active, state, source)
	if err != nil {
		t.Fatalf("insert play_session (%s/%s): %v", state, source, err)
	}
}

// m11SeedSessions seeds one game with tracked, imported and discarded sessions.
func m11SeedSessions(t *testing.T, db *sql.DB) {
	t.Helper()
	g := m10Game(t, db, "Jogo")
	m11Session(t, db, g, 1789257600000, 1789261200000, 3600, "ended", "tracked")
	m11Session(t, db, g, 1789300000000, 1789300600000, 600, "ended", "import")
	m11Session(t, db, g, 1789400000000, 1789400060000, 60, "discarded", "tracked")
}

const m11SessionDump = `SELECT id, game_id, started_at, ended_at, last_seen_at, active_seconds, state, source, end_reason
	FROM play_session ORDER BY id`

// m11OtherSchema is sqlite_master without play_session (which gains a column and an index).
const m11OtherSchema = `SELECT type, name, tbl_name, COALESCE(sql, '') FROM sqlite_master
	WHERE tbl_name <> 'play_session' AND name NOT LIKE 'sqlite\_%' ESCAPE '\'
	  AND name NOT LIKE 'goose%' ORDER BY type, name`

func TestMigration11(t *testing.T) {
	t.Parallel()

	t.Run("SED-001/banco novo tem edited_at (INTEGER, nula) em play_session e o índice play_session_started_at_idx em (started_at)", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()

		var found bool
		for _, c := range tableColumns(t, db, "play_session") {
			if c.Name != "edited_at" {
				continue
			}
			found = true
			if c.Type != "INTEGER" {
				t.Errorf("edited_at type = %q, want INTEGER", c.Type)
			}
		}
		if !found {
			t.Fatalf("play_session.edited_at missing; columns = %v", m9Columns(t, db, "play_session"))
		}
		var notNull int
		var dflt sql.NullString
		if err := db.QueryRowContext(t.Context(),
			`SELECT "notnull", dflt_value FROM pragma_table_info('play_session') WHERE name = 'edited_at'`).
			Scan(&notNull, &dflt); err != nil {
			t.Fatalf("pragma_table_info: %v", err)
		}
		if notNull != 0 || dflt.Valid {
			t.Errorf("edited_at notnull=%d default=%v, want nullable without default", notNull, dflt)
		}

		idx := indexColumns(t, db, "play_session")
		cols, ok := idx["play_session_started_at_idx"]
		if !ok {
			t.Fatalf("index play_session_started_at_idx missing; indexes = %v", idx)
		}
		if !slices.Equal(cols, []string{"started_at"}) {
			t.Errorf("play_session_started_at_idx columns = %v, want [started_at]", cols)
		}

		g := m10Game(t, db, "Jogo")
		m11Session(t, db, g, 1789257600000, 1789261200000, 3600, "ended", "manual")
		if n := countRows(t, db, `SELECT COUNT(*) FROM play_session WHERE edited_at IS NULL`); n != 1 {
			t.Errorf("rows with edited_at NULL = %d, want 1 (default is NULL)", n)
		}
	})

	t.Run("SED-001/SchemaVersion = 11 e a versão do goose é 11", func(t *testing.T) {
		t.Parallel()
		if store.SchemaVersion != 11 {
			t.Errorf("SchemaVersion = %d, want 11", store.SchemaVersion)
		}
		s := openStore(t, dbPath(t), true)
		requireGooseVersion(t, s, 11)
	})

	t.Run("SED-001/banco na versão 10 com sessões rastreadas, importadas e descartadas migra com edited_at nulo e os outros valores iguais", func(t *testing.T) {
		t.Parallel()
		var before []string
		path := versionTenFile(t, func(t *testing.T, db *sql.DB) {
			m11SeedSessions(t, db)
			before = dumpRows(t, db, m11SessionDump)
		})
		if len(before) != 3 {
			t.Fatalf("precondition: %d sessions seeded, want 3", len(before))
		}
		s := openStore(t, path, true)
		requireGooseVersion(t, s, 11)
		if after := dumpRows(t, s.DB(), m11SessionDump); !slices.Equal(after, before) {
			t.Errorf("sessions changed:\n before %v\n after  %v", before, after)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM play_session WHERE edited_at IS NOT NULL`); n != 0 {
			t.Errorf("sessions with edited_at set = %d, want 0", n)
		}
	})

	t.Run("SED-001/os índices e as outras tabelas continuam iguais (sqlite_master sem play_session é igual ao da v10)", func(t *testing.T) {
		t.Parallel()
		var before []string
		path := versionTenFile(t, func(t *testing.T, db *sql.DB) {
			before = dumpRows(t, db, m11OtherSchema)
		})
		if len(before) == 0 {
			t.Fatal("precondition: no schema rows read from the v10 database")
		}
		s := openStore(t, path, true)
		if after := dumpRows(t, s.DB(), m11OtherSchema); !slices.Equal(after, before) {
			t.Errorf("schema outside play_session changed:\n before %v\n after  %v", before, after)
		}
	})

	t.Run("SED-001/Down apaga o índice e a coluna e preserva as linhas e as outras colunas", func(t *testing.T) {
		t.Parallel()
		path := dbPath(t)
		db := openRaw(t, path)
		defer db.Close()
		ctx := t.Context()
		provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
		if err != nil {
			t.Fatalf("goose.NewProvider: %v", err)
		}
		if _, err := provider.UpTo(ctx, 11); err != nil {
			t.Fatalf("goose UpTo(11): %v", err)
		}
		m11SeedSessions(t, db)
		if _, err := db.ExecContext(ctx, `UPDATE play_session SET edited_at = 1789500000000 WHERE id = 1`); err != nil {
			t.Fatalf("set edited_at: %v", err)
		}
		rows := dumpRows(t, db, m11SessionDump)
		others := dumpRows(t, db, m11OtherSchema)
		wantCols := slices.DeleteFunc(m9Columns(t, db, "play_session"), func(c string) bool { return c == "edited_at" })

		if _, err := provider.DownTo(ctx, 10); err != nil {
			t.Fatalf("goose DownTo(10): %v", err)
		}
		if v, err := provider.GetDBVersion(ctx); err != nil || v != 10 {
			t.Fatalf("goose version after Down = %d (err=%v), want 10", v, err)
		}
		if got := m9Columns(t, db, "play_session"); !slices.Equal(got, wantCols) {
			t.Errorf("play_session columns after Down = %v, want %v", got, wantCols)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'play_session_started_at_idx'`); n != 0 {
			t.Errorf("play_session_started_at_idx still exists after Down")
		}
		if got := dumpRows(t, db, m11SessionDump); !slices.Equal(got, rows) {
			t.Errorf("sessions after Down:\n got  %v\n want %v", got, rows)
		}
		if got := dumpRows(t, db, m11OtherSchema); !slices.Equal(got, others) {
			t.Errorf("schema changed by Down:\n before %v\n after  %v", others, got)
		}
	})
}
