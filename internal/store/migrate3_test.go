package store_test

import (
	"database/sql"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// versionTwoFile builds a database at goose version 2 by applying only the
// first two migrations from the source directory (goose UpTo(2)), then inserts
// a platform, a game, an ended session and an app_setting row, so the 2 -> 3
// migration can be checked for data loss.
func versionTwoFile(t *testing.T) (path string, gameID, sessionID int64) {
	t.Helper()
	path = dbPath(t)
	db := openRaw(t, path)
	defer db.Close()
	ctx := t.Context()

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 2); err != nil {
		t.Fatalf("goose UpTo(2): %v", err)
	}
	if v, err := provider.GetDBVersion(ctx); err != nil || v != 2 {
		t.Fatalf("precondition: goose version = %d (err=%v), want 2", v, err)
	}
	if slices.Contains(columnNames(tableColumns(t, db, "play_session")), "end_reason") {
		t.Fatal("precondition: play_session.end_reason must not exist at version 2")
	}

	res, err := db.ExecContext(ctx, `INSERT INTO platform (name, category) VALUES ('PC', 'pc')`)
	if err != nil {
		t.Fatalf("insert platform: %v", err)
	}
	pid, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("platform LastInsertId: %v", err)
	}
	gameID = mustInsertGame(t, db, pid, "Hollow Knight", "")

	res, err = db.ExecContext(ctx, `INSERT INTO play_session
		(game_id, started_at, ended_at, last_seen_at, active_seconds, state, source)
		VALUES (?, 1700000000000, 1700000600000, 1700000600000, 600, 'ended', 'tracked')`, gameID)
	if err != nil {
		t.Fatalf("insert play_session: %v", err)
	}
	sessionID, err = res.LastInsertId()
	if err != nil {
		t.Fatalf("session LastInsertId: %v", err)
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO app_setting (key, value, updated_at) VALUES ('autostart', 'true', 1700000000000)`); err != nil {
		t.Fatalf("insert app_setting: %v", err)
	}
	return path, gameID, sessionID
}

// requireEndReasonColumn checks play_session.end_reason is a nullable TEXT
// column without a default.
func requireEndReasonColumn(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT type, "notnull", dflt_value FROM pragma_table_info('play_session') WHERE name = 'end_reason'`)
	if err != nil {
		t.Fatalf("pragma_table_info(play_session): %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		t.Fatalf("play_session.end_reason missing; columns = %v",
			columnNames(tableColumns(t, db, "play_session")))
	}
	var (
		typ     string
		notNull int
		dflt    sql.NullString
	)
	if err := rows.Scan(&typ, &notNull, &dflt); err != nil {
		t.Fatalf("scan end_reason: %v", err)
	}
	if !strings.EqualFold(typ, "TEXT") {
		t.Errorf("play_session.end_reason type = %q, want TEXT", typ)
	}
	if notNull != 0 {
		t.Errorf("play_session.end_reason is NOT NULL, want nullable")
	}
	if dflt.Valid && !strings.EqualFold(dflt.String, "NULL") {
		t.Errorf("play_session.end_reason default = %q, want none (NULL)", dflt.String)
	}
}

func requireGooseVersion(t *testing.T, s *store.Store, want int64) {
	t.Helper()
	v, err := s.CurrentVersion(t.Context())
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if v != want {
		t.Errorf("CurrentVersion = %d, want %d", v, want)
	}
}

func TestMigration3(t *testing.T) {
	t.Parallel()

	t.Run("STO-011/banco novo tem play_session.end_reason TEXT nula e goose na versão 3", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		requireEndReasonColumn(t, s.DB())
		requireGooseVersion(t, s, 3)
	})

	t.Run("STO-011/banco v2 com jogo, sessão e app_setting migra para 3 mantendo os dados, com end_reason nulo", func(t *testing.T) {
		t.Parallel()
		path, gameID, sessionID := versionTwoFile(t)

		s := openStore(t, path, true)
		db := s.DB()
		ctx := t.Context()

		requireGooseVersion(t, s, 3)
		requireEndReasonColumn(t, db)

		var title string
		if err := db.QueryRowContext(ctx, `SELECT title FROM game WHERE id = ?`, gameID).Scan(&title); err != nil {
			t.Fatalf("read game %d: %v", gameID, err)
		}
		if title != "Hollow Knight" {
			t.Errorf("game title = %q, want Hollow Knight", title)
		}

		var (
			gid, started, active int64
			ended                sql.NullInt64
			state, source        string
			endReason            sql.NullString
		)
		if err := db.QueryRowContext(ctx, `SELECT game_id, started_at, ended_at, active_seconds, state, source, end_reason
			FROM play_session WHERE id = ?`, sessionID).
			Scan(&gid, &started, &ended, &active, &state, &source, &endReason); err != nil {
			t.Fatalf("read play_session %d: %v", sessionID, err)
		}
		if gid != gameID || started != 1_700_000_000_000 || !ended.Valid || ended.Int64 != 1_700_000_600_000 ||
			active != 600 || state != "ended" || source != "tracked" {
			t.Errorf("session after migration = {game_id:%d started_at:%d ended_at:%v active_seconds:%d state:%q source:%q}, want the v2 values",
				gid, started, ended, active, state, source)
		}
		if endReason.Valid {
			t.Errorf("old session end_reason = %q, want NULL", endReason.String)
		}

		got, err := s.Setting(ctx, "autostart")
		if err != nil {
			t.Fatalf("Setting(autostart): %v", err)
		}
		if got != "true" {
			t.Errorf("Setting(autostart) = %q, want true", got)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM play_mode`); n != 5 {
			t.Errorf("play_mode rows = %d, want 5", n)
		}
	})

	t.Run("STO-011/índice play_session_open_idx existe", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()

		var tbl, ddl string
		if err := db.QueryRowContext(t.Context(),
			`SELECT tbl_name, sql FROM sqlite_master WHERE type = 'index' AND name = 'play_session_open_idx'`).
			Scan(&tbl, &ddl); err != nil {
			t.Fatalf("index play_session_open_idx not found: %v", err)
		}
		if tbl != "play_session" {
			t.Errorf("play_session_open_idx is on %q, want play_session", tbl)
		}
		if cols := indexColumns(t, db, "play_session")["play_session_open_idx"]; !slices.Equal(cols, []string{"ended_at"}) {
			t.Errorf("play_session_open_idx columns = %v, want [ended_at]", cols)
		}
		norm := strings.ToLower(strings.Join(strings.Fields(ddl), " "))
		if !strings.Contains(norm, "where ended_at is null") {
			t.Errorf("play_session_open_idx DDL = %q, want a partial index WHERE ended_at IS NULL", ddl)
		}
	})

	t.Run("STO-011/SchemaVersion é 3", func(t *testing.T) {
		t.Parallel()
		if store.SchemaVersion != 3 {
			t.Errorf("store.SchemaVersion = %d, want 3", store.SchemaVersion)
		}
	})
}
