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

const pauseOpenIndex = "pause_interval_open_uidx"

// versionThreeFile builds a database at goose version 3 (goose UpTo(3)) with a
// platform, a game, an open session and a closed pause on it.
func versionThreeFile(t *testing.T) (path string, gameID, sessionID int64) {
	t.Helper()
	path = dbPath(t)
	db := openRaw(t, path)
	defer db.Close()
	ctx := t.Context()

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 3); err != nil {
		t.Fatalf("goose UpTo(3): %v", err)
	}
	if v, err := provider.GetDBVersion(ctx); err != nil || v != 3 {
		t.Fatalf("precondition: goose version = %d (err=%v), want 3", v, err)
	}

	res, err := db.ExecContext(ctx, `INSERT INTO platform (name, category) VALUES ('PC', 'pc')`)
	if err != nil {
		t.Fatalf("insert platform: %v", err)
	}
	pid, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("platform LastInsertId: %v", err)
	}
	gameID = mustInsertGame(t, db, pid, "Celeste", "")
	sessionID = insertOpenSession(t, db, gameID)
	if _, err := db.ExecContext(ctx, `INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason)
		VALUES (?, 1700000100000, 1700000200000, 'hotkey')`, sessionID); err != nil {
		t.Fatalf("insert closed pause: %v", err)
	}
	return path, gameID, sessionID
}

func insertOpenSession(t *testing.T, db *sql.DB, gameID int64) int64 {
	t.Helper()
	res, err := db.ExecContext(t.Context(), `INSERT INTO play_session
		(game_id, started_at, last_seen_at, active_seconds, state, source)
		VALUES (?, 1700000000000, 1700000000000, 0, 'running', 'tracked')`, gameID)
	if err != nil {
		t.Fatalf("insert open session: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("session LastInsertId: %v", err)
	}
	return id
}

func insertOpenPause(db *sql.DB, t *testing.T, sessionID, at int64) error {
	t.Helper()
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason) VALUES (?, ?, NULL, 'hotkey')`,
		sessionID, at)
	return err
}

// migratedWithSession opens a fresh migrated store and returns it with a game
// and an open session.
func migratedWithSession(t *testing.T) (*store.Store, int64) {
	t.Helper()
	s := openStore(t, dbPath(t), true)
	db := s.DB()
	res, err := db.ExecContext(t.Context(), `INSERT INTO platform (name, category) VALUES ('PC', 'pc')`)
	if err != nil {
		t.Fatalf("insert platform: %v", err)
	}
	pid, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("platform LastInsertId: %v", err)
	}
	gameID := mustInsertGame(t, db, pid, "Celeste", "")
	return s, insertOpenSession(t, db, gameID)
}

func TestMigration4(t *testing.T) {
	t.Parallel()

	t.Run("STO-018/banco novo tem pause_interval_open_uidx e goose na versão atual", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()

		var tbl, ddl string
		if err := db.QueryRowContext(t.Context(),
			`SELECT tbl_name, sql FROM sqlite_master WHERE type = 'index' AND name = ?`, pauseOpenIndex).
			Scan(&tbl, &ddl); err != nil {
			t.Fatalf("index %s not found: %v", pauseOpenIndex, err)
		}
		if tbl != "pause_interval" {
			t.Errorf("%s is on %q, want pause_interval", pauseOpenIndex, tbl)
		}
		if cols := indexColumns(t, db, "pause_interval")[pauseOpenIndex]; !slices.Equal(cols, []string{"session_id"}) {
			t.Errorf("%s columns = %v, want [session_id]", pauseOpenIndex, cols)
		}
		norm := strings.ToLower(strings.Join(strings.Fields(ddl), " "))
		if !strings.Contains(norm, "unique") {
			t.Errorf("%s DDL = %q, want UNIQUE", pauseOpenIndex, ddl)
		}
		if !strings.Contains(norm, "where resumed_at is null") {
			t.Errorf("%s DDL = %q, want a partial index WHERE resumed_at IS NULL", pauseOpenIndex, ddl)
		}
		requireGooseVersion(t, s, store.SchemaVersion)
	})

	t.Run("STO-018/banco v3 com jogo, sessão e pausa fechada migra mantendo os dados", func(t *testing.T) {
		t.Parallel()
		path, gameID, sessionID := versionThreeFile(t)

		s := openStore(t, path, true)
		db := s.DB()
		ctx := t.Context()
		requireGooseVersion(t, s, store.SchemaVersion)

		var n int64
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, pauseOpenIndex).Scan(&n); err != nil || n != 1 {
			t.Fatalf("index %s count = %d (err=%v), want 1", pauseOpenIndex, n, err)
		}

		var title string
		if err := db.QueryRowContext(ctx, `SELECT title FROM game WHERE id = ?`, gameID).Scan(&title); err != nil {
			t.Fatalf("read game %d: %v", gameID, err)
		}
		if title != "Celeste" {
			t.Errorf("game title = %q, want Celeste", title)
		}

		var state string
		if err := db.QueryRowContext(ctx, `SELECT state FROM play_session WHERE id = ?`, sessionID).Scan(&state); err != nil {
			t.Fatalf("read session %d: %v", sessionID, err)
		}
		if state != "running" {
			t.Errorf("session state = %q, want running", state)
		}

		var (
			sid, pausedAt int64
			resumedAt     sql.NullInt64
			reason        string
		)
		if err := db.QueryRowContext(ctx,
			`SELECT session_id, paused_at, resumed_at, reason FROM pause_interval`).
			Scan(&sid, &pausedAt, &resumedAt, &reason); err != nil {
			t.Fatalf("read pause_interval: %v", err)
		}
		if sid != sessionID || pausedAt != 1_700_000_100_000 || !resumedAt.Valid ||
			resumedAt.Int64 != 1_700_000_200_000 || reason != "hotkey" {
			t.Errorf("pause after migration = {session_id:%d paused_at:%d resumed_at:%v reason:%q}, want the v3 values",
				sid, pausedAt, resumedAt, reason)
		}
	})

	t.Run("STO-018/segunda pausa aberta da mesma sessão é recusada pelo banco", func(t *testing.T) {
		t.Parallel()
		s, sessionID := migratedWithSession(t)
		db := s.DB()

		if err := insertOpenPause(db, t, sessionID, 1_700_000_100_000); err != nil {
			t.Fatalf("first open pause: %v", err)
		}
		if err := insertOpenPause(db, t, sessionID, 1_700_000_300_000); err == nil {
			t.Fatal("second open pause for the same session was accepted, want a constraint error")
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM pause_interval WHERE session_id = ?`, sessionID); n != 1 {
			t.Errorf("pause_interval rows for session = %d, want 1", n)
		}
	})

	t.Run("STO-018/pausa aberta em outra sessão e nova pausa depois de fechar a anterior são aceitas", func(t *testing.T) {
		t.Parallel()
		s, first := migratedWithSession(t)
		db := s.DB()
		ctx := t.Context()

		var gameID int64
		if err := db.QueryRowContext(ctx, `SELECT game_id FROM play_session WHERE id = ?`, first).Scan(&gameID); err != nil {
			t.Fatalf("read game id: %v", err)
		}
		second := insertOpenSession(t, db, gameID)

		if err := insertOpenPause(db, t, first, 1_700_000_100_000); err != nil {
			t.Fatalf("open pause on first session: %v", err)
		}
		if err := insertOpenPause(db, t, second, 1_700_000_110_000); err != nil {
			t.Fatalf("open pause on another session: %v", err)
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE pause_interval SET resumed_at = 1700000200000 WHERE session_id = ?`, first); err != nil {
			t.Fatalf("close pause: %v", err)
		}
		if err := insertOpenPause(db, t, first, 1_700_000_300_000); err != nil {
			t.Fatalf("new open pause after closing the previous one: %v", err)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM pause_interval`); n != 3 {
			t.Errorf("pause_interval rows = %d, want 3", n)
		}
	})

	t.Run("STO-018/Down remove só o índice e mantém a tabela e os dados", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()

		db := openRaw(t, dbPath(t))
		defer db.Close()
		provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
		if err != nil {
			t.Fatalf("goose.NewProvider: %v", err)
		}
		// Only up to version 4: migration 5 (STO-024) would otherwise be the one Down reverts.
		if _, err := provider.UpTo(ctx, 4); err != nil {
			t.Fatalf("goose UpTo(4): %v", err)
		}
		res, err := db.ExecContext(ctx, `INSERT INTO platform (name, category) VALUES ('PC', 'pc')`)
		if err != nil {
			t.Fatalf("insert platform: %v", err)
		}
		pid, _ := res.LastInsertId()
		gid := mustInsertGame(t, db, pid, "Celeste", "")
		sid := insertOpenSession(t, db, gid)
		if err := insertOpenPause(db, t, sid, 1_700_000_100_000); err != nil {
			t.Fatalf("open pause: %v", err)
		}

		if _, err := provider.Down(ctx); err != nil {
			t.Fatalf("goose Down: %v", err)
		}
		if v, err := provider.GetDBVersion(ctx); err != nil || v != 3 {
			t.Fatalf("goose version after Down = %d (err=%v), want 3", v, err)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, pauseOpenIndex); n != 0 {
			t.Errorf("index %s still exists after Down", pauseOpenIndex)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'pause_interval_session_id_idx'`); n != 1 {
			t.Errorf("pause_interval_session_id_idx count = %d after Down, want 1", n)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM pause_interval`); n != 1 {
			t.Errorf("pause_interval rows after Down = %d, want 1", n)
		}
		// Without the unique index a second open pause is accepted again.
		if err := insertOpenPause(db, t, sid, 1_700_000_200_000); err != nil {
			t.Errorf("second open pause after Down: %v, want accepted", err)
		}
	})

	t.Run("STO-024/a migração 4 continua embutida", func(t *testing.T) {
		t.Parallel()
		db := openRaw(t, dbPath(t))
		defer db.Close()
		provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
		if err != nil {
			t.Fatalf("goose.NewProvider: %v", err)
		}
		found := false
		for _, src := range provider.ListSources() {
			if src.Version == 4 {
				found = true
			}
		}
		if !found {
			t.Errorf("migration 4 is not in the provider sources")
		}
	})
}
