package store_test

// Tests of migration 00009 (phase 13, VID-001): the CHECK of media.kind and media_job.kind accepts
// 'video'. Nothing else changes: no new column (no disc column, ADR 0037), no change in game or in
// the session tables. The tables media and media_job are rebuilt, so the data, the ids, the
// UNIQUE (game_id, hash) and the ON DELETE CASCADE must survive.

import (
	"database/sql"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// versionEightFile builds a database at goose version 8 (goose UpTo(8)), runs seed on it if non-nil,
// closes it and returns the path. Opening it with store.Open(Migrate) then applies 00009.
func versionEightFile(t *testing.T, seed func(t *testing.T, db *sql.DB)) string {
	t.Helper()
	path := dbPath(t)
	db := openRaw(t, path)
	defer db.Close()
	ctx := t.Context()

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 8); err != nil {
		t.Fatalf("goose UpTo(8): %v", err)
	}
	if v, err := provider.GetDBVersion(ctx); err != nil || v != 8 {
		t.Fatalf("precondition: goose version = %d (err=%v), want 8", v, err)
	}
	if seed != nil {
		seed(t, db)
	}
	return path
}

// m9Game inserts a game on the first platform and returns its id.
func m9Game(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()
	var platformID int64
	if err := db.QueryRowContext(t.Context(), `SELECT MIN(id) FROM platform`).Scan(&platformID); err != nil {
		t.Fatalf("first platform: %v", err)
	}
	return mustInsertGame(t, db, platformID, title, "")
}

func m9Media(t *testing.T, db *sql.DB, gameID int64, kind, hash string) error {
	t.Helper()
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO media (game_id, kind, provider, source_url, hash, path, thumb_path, created_at)
		 VALUES (?, ?, 'steamgriddb', 'https://example.test/x', ?, ?, ?, 1789257600000)`,
		gameID, kind, hash, hash+".jpg", "thumb_"+hash+".jpg")
	return err
}

func m9Job(t *testing.T, db *sql.DB, gameID int64, kind, state string) error {
	t.Helper()
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO media_job (game_id, kind, provider, source_url, state, attempts, last_error, created_at)
		 VALUES (?, ?, 'igdb', 'https://example.test/a', ?, 2, 'boom', 1789257600000)`,
		gameID, kind, state)
	return err
}

// m9Columns lists the column names of a table, in order.
func m9Columns(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT name FROM pragma_table_info('`+table+`') ORDER BY cid`)
	if err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// alterado em 2026-11-22 (fase 15): exclui time_to_beat (migração 00010).
const m9OtherSchema = `SELECT type, name, tbl_name, COALESCE(sql, '') FROM sqlite_master
	WHERE tbl_name NOT IN ('media', 'media_job', 'time_to_beat') AND name NOT LIKE 'sqlite\_%' ESCAPE '\'
	  AND name NOT LIKE 'goose%' ORDER BY type, name`

const m9MediaDump = `SELECT id, game_id, kind, provider, source_url, hash, path, thumb_path, created_at FROM media ORDER BY id`
const m9JobDump = `SELECT id, game_id, kind, provider, source_url, state, attempts, last_error, created_at FROM media_job ORDER BY id`

func TestMigration9(t *testing.T) {
	t.Parallel()

	t.Run("VID-001/banco novo vai a 9 e aceita video em media e media_job", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		requireGooseVersion(t, s, 10) // alterado em 2026-11-22 (fase 15)
		g := m9Game(t, s.DB(), "Jogo")

		if err := m9Media(t, s.DB(), g, "video", "h1"); err != nil {
			t.Errorf("insert media kind video: %v", err)
		}
		if err := m9Job(t, s.DB(), g, "video", "pending"); err != nil {
			t.Errorf("insert media_job kind video: %v", err)
		}
		for _, kind := range []string{"cover", "screenshot", "title", "banner", "background", "select"} {
			if err := m9Media(t, s.DB(), g, kind, "k-"+kind); err != nil {
				t.Errorf("insert media kind %s: %v", kind, err)
			}
			if err := m9Job(t, s.DB(), g, kind, "pending"); err != nil {
				t.Errorf("insert media_job kind %s: %v", kind, err)
			}
		}
	})

	t.Run("VID-001/banco na v8 com mídias e jobs migra preservando todos os dados e o UNIQUE (game_id, hash)", func(t *testing.T) {
		t.Parallel()
		var g1, g2 int64
		var mediaBefore, jobsBefore []string
		path := versionEightFile(t, func(t *testing.T, db *sql.DB) {
			g1 = m9Game(t, db, "Um")
			g2 = m9Game(t, db, "Dois")
			for i, kind := range []string{"cover", "screenshot", "title", "banner", "background", "select"} {
				if err := m9Media(t, db, g1, kind, "a"+kind); err != nil {
					t.Fatalf("seed media %d: %v", i, err)
				}
			}
			if err := m9Media(t, db, g2, "cover", "b1"); err != nil {
				t.Fatalf("seed media g2: %v", err)
			}
			for _, st := range []string{"pending", "running", "done", "failed"} {
				if err := m9Job(t, db, g1, "screenshot", st); err != nil {
					t.Fatalf("seed job %s: %v", st, err)
				}
			}
			mediaBefore = dumpRows(t, db, m9MediaDump)
			jobsBefore = dumpRows(t, db, m9JobDump)
		})

		s := openStore(t, path, true)
		requireGooseVersion(t, s, 10) // alterado em 2026-11-22 (fase 15)
		if got := dumpRows(t, s.DB(), m9MediaDump); !slices.Equal(got, mediaBefore) {
			t.Errorf("media changed:\n before %v\n after  %v", mediaBefore, got)
		}
		if got := dumpRows(t, s.DB(), m9JobDump); !slices.Equal(got, jobsBefore) {
			t.Errorf("media_job changed:\n before %v\n after  %v", jobsBefore, got)
		}
		// UNIQUE (game_id, hash) still holds; the same hash in another game is fine.
		if err := m9Media(t, s.DB(), g1, "cover", "acover"); err == nil {
			t.Errorf("duplicate (game_id, hash) accepted after the migration")
		}
		if err := m9Media(t, s.DB(), g2, "cover", "acover"); err != nil {
			t.Errorf("same hash in another game refused: %v", err)
		}
		// New rows get ids after the old ones (the primary key still auto-assigns).
		if err := m9Media(t, s.DB(), g1, "video", "v1"); err != nil {
			t.Errorf("insert video after migration: %v", err)
		}
	})

	t.Run("VID-001/ON DELETE CASCADE continua valendo depois da migração", func(t *testing.T) {
		t.Parallel()
		var g int64
		path := versionEightFile(t, func(t *testing.T, db *sql.DB) {
			g = m9Game(t, db, "Jogo")
			if err := m9Media(t, db, g, "cover", "c1"); err != nil {
				t.Fatalf("seed media: %v", err)
			}
			if err := m9Job(t, db, g, "cover", "pending"); err != nil {
				t.Fatalf("seed job: %v", err)
			}
		})
		s := openStore(t, path, true)
		if err := m9Media(t, s.DB(), g, "video", "v1"); err != nil {
			t.Fatalf("insert video: %v", err)
		}
		if err := m9Job(t, s.DB(), g, "video", "pending"); err != nil {
			t.Fatalf("insert video job: %v", err)
		}
		if _, err := s.DB().ExecContext(t.Context(), `DELETE FROM game WHERE id = ?`, g); err != nil {
			t.Fatalf("delete game: %v", err)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM media WHERE game_id = ?`, g); n != 0 {
			t.Errorf("media rows after deleting the game = %d, want 0", n)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM media_job WHERE game_id = ?`, g); n != 0 {
			t.Errorf("media_job rows after deleting the game = %d, want 0", n)
		}
	})

	t.Run("VID-001/a tabela game e as de sessão não mudam (esquema igual ao da v8 fora de media e media_job)", func(t *testing.T) {
		t.Parallel()
		var before []string
		path := versionEightFile(t, func(t *testing.T, db *sql.DB) {
			before = dumpRows(t, db, m9OtherSchema)
		})
		if len(before) == 0 {
			t.Fatal("precondition: no schema rows read from the v8 database")
		}
		s := openStore(t, path, true)
		if after := dumpRows(t, s.DB(), m9OtherSchema); !slices.Equal(after, before) {
			t.Errorf("schema outside media/media_job changed:\n before %v\n after  %v", before, after)
		}
	})

	t.Run("VID-001/media e media_job não ganham coluna (sem coluna de discos, ADR 0037)", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		wantMedia := []string{"id", "game_id", "kind", "provider", "source_url", "hash", "path", "thumb_path", "created_at"}
		wantJob := []string{"id", "game_id", "kind", "provider", "source_url", "state", "attempts", "last_error", "created_at"}
		if got := m9Columns(t, s.DB(), "media"); !slices.Equal(got, wantMedia) {
			t.Errorf("media columns = %v, want %v", got, wantMedia)
		}
		if got := m9Columns(t, s.DB(), "media_job"); !slices.Equal(got, wantJob) {
			t.Errorf("media_job columns = %v, want %v", got, wantJob)
		}
	})

	t.Run("VID-001/kind inválido continua recusado", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := m9Game(t, s.DB(), "Jogo")
		for _, kind := range []string{"logo", "", "Video", "videos"} {
			if err := m9Media(t, s.DB(), g, kind, "x"+kind); err == nil {
				t.Errorf("media accepted kind %q", kind)
			}
			if err := m9Job(t, s.DB(), g, kind, "pending"); err == nil {
				t.Errorf("media_job accepted kind %q", kind)
			}
		}
		if err := m9Job(t, s.DB(), g, "video", "paused"); err == nil {
			t.Errorf("media_job accepted state %q", "paused")
		}
	})

	t.Run("VID-001/Down apaga as linhas video, mantém o resto e volta ao CHECK antigo", func(t *testing.T) {
		t.Parallel()
		path := dbPath(t)
		db := openRaw(t, path)
		defer db.Close()
		ctx := t.Context()

		provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
		if err != nil {
			t.Fatalf("goose.NewProvider: %v", err)
		}
		if _, err := provider.UpTo(ctx, 9); err != nil {
			t.Fatalf("goose UpTo(9): %v", err)
		}
		g := m9Game(t, db, "Jogo")
		for _, row := range [][2]string{{"cover", "c1"}, {"video", "v1"}, {"screenshot", "s1"}} {
			if err := m9Media(t, db, g, row[0], row[1]); err != nil {
				t.Fatalf("seed media %v: %v", row, err)
			}
		}
		for _, kind := range []string{"cover", "video", "video"} {
			if err := m9Job(t, db, g, kind, "pending"); err != nil {
				t.Fatalf("seed job %s: %v", kind, err)
			}
		}
		keepMedia := dumpRows(t, db, `SELECT id, game_id, kind, hash FROM media WHERE kind <> 'video' ORDER BY id`)

		if _, err := provider.DownTo(ctx, 8); err != nil {
			t.Fatalf("goose DownTo(8): %v", err)
		}
		if v, err := provider.GetDBVersion(ctx); err != nil || v != 8 {
			t.Fatalf("goose version after Down = %d (err=%v), want 8", v, err)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM media WHERE kind = 'video'`); n != 0 {
			t.Errorf("video rows left in media = %d, want 0", n)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM media_job WHERE kind = 'video'`); n != 0 {
			t.Errorf("video rows left in media_job = %d, want 0", n)
		}
		if got := dumpRows(t, db, `SELECT id, game_id, kind, hash FROM media ORDER BY id`); !slices.Equal(got, keepMedia) {
			t.Errorf("media after Down = %v, want %v", got, keepMedia)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM media_job`); n != 1 {
			t.Errorf("media_job rows after Down = %d, want 1 (the cover job)", n)
		}
		if err := m9Media(t, db, g, "video", "v2"); err == nil {
			t.Errorf("media accepted video after Down")
		}
		if err := m9Job(t, db, g, "video", "pending"); err == nil {
			t.Errorf("media_job accepted video after Down")
		}
		if err := m9Media(t, db, g, "cover", "c1"); err == nil {
			t.Errorf("UNIQUE (game_id, hash) lost after Down")
		}
	})

	t.Run("VID-001/a lista de migrações termina na 00009 e SchemaVersion = 9", func(t *testing.T) {
		t.Parallel()
		entries, err := fs.ReadDir(os.DirFS("."), "migrations")
		if err != nil {
			t.Fatalf("ReadDir(migrations): %v", err)
		}
		var got []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
				got = append(got, e.Name())
			}
		}
		slices.Sort(got)
		// alterado em 2026-11-22 (fase 15): a lista termina na 00010
		if len(got) != 10 || got[8] != "00009_video.sql" || got[9] != "00010_time_to_beat.sql" {
			t.Errorf("migrations = %v, want 10 files ending in 00009_video.sql, 00010_time_to_beat.sql", got)
		}
		if store.SchemaVersion != 10 {
			t.Errorf("SchemaVersion = %d, want 10", store.SchemaVersion)
		}
	})

	t.Run("VID-001/00009 roda em transação (arquivo sem NO TRANSACTION)", func(t *testing.T) {
		t.Parallel()
		data, err := os.ReadFile("migrations/00009_video.sql")
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		src := strings.ToUpper(string(data))
		if strings.Contains(src, "NO TRANSACTION") {
			t.Errorf("00009_video.sql contains NO TRANSACTION; it must run in a goose transaction")
		}
		if !strings.Contains(src, "+GOOSE UP") || !strings.Contains(src, "+GOOSE DOWN") {
			t.Errorf("00009_video.sql needs +goose Up and +goose Down sections")
		}
	})
}
