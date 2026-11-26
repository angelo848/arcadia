package store_test

// Tests of migration 00010 (phase 15, TTB-001): table time_to_beat (design 3.1). Nothing else changes.

import (
	"database/sql"
	"os"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// versionNineFile builds a database at goose version 9, runs seed on it if non-nil, closes it
// and returns the path. Opening it with store.Open(Migrate) then applies 00010.
func versionNineFile(t *testing.T, seed func(t *testing.T, db *sql.DB)) string {
	t.Helper()
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
	if v, err := provider.GetDBVersion(ctx); err != nil || v != 9 {
		t.Fatalf("precondition: goose version = %d (err=%v), want 9", v, err)
	}
	if seed != nil {
		seed(t, db)
	}
	return path
}

func m10Game(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()
	var platformID int64
	if err := db.QueryRowContext(t.Context(), `SELECT MIN(id) FROM platform`).Scan(&platformID); err != nil {
		t.Fatalf("first platform: %v", err)
	}
	return mustInsertGame(t, db, platformID, title, "")
}

// m10Insert inserts a time_to_beat row; mainS/extraS/compS are SQL values (nil = NULL).
func m10Insert(t *testing.T, db *sql.DB, gameID int64, source, state string, confirmed int, mainS, extraS, compS any) error {
	t.Helper()
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO time_to_beat (game_id, source, state, main_s, main_extra_s, completionist_s,
		   title_at_fetch, term, confirmed, fetched_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'Titulo', 'termo', ?, 1789257600000, 1791849600000)`,
		gameID, source, state, mainS, extraS, compS, confirmed)
	return err
}

// alterado em 2026-11-25 (fase 16): exclui play_session (migração 00011).
const m10OtherSchema = `SELECT type, name, tbl_name, COALESCE(sql, '') FROM sqlite_master
	WHERE tbl_name NOT IN ('time_to_beat', 'play_session') AND name NOT LIKE 'sqlite\_%' ESCAPE '\'
	  AND name NOT LIKE 'goose%' ORDER BY type, name`

func TestMigration10(t *testing.T) {
	t.Parallel()

	t.Run("TTB-001/banco novo tem a tabela time_to_beat com as colunas do design §3.1", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		want := []string{"game_id", "source", "state", "external_id", "external_title", "main_s",
			"main_extra_s", "completionist_s", "title_at_fetch", "term", "candidates", "confirmed",
			"fetched_at", "expires_at"}
		if got := m9Columns(t, s.DB(), "time_to_beat"); !slices.Equal(got, want) {
			t.Errorf("time_to_beat columns = %v, want %v", got, want)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM time_to_beat`); n != 0 {
			t.Errorf("rows in a new database = %d, want 0", n)
		}
	})

	t.Run("TTB-001/SchemaVersion = 11 e a versão do goose é 11 (alterado em 2026-11-25, fase 16)", func(t *testing.T) {
		t.Parallel()
		if store.SchemaVersion != 11 {
			t.Errorf("SchemaVersion = %d, want 11", store.SchemaVersion)
		}
		s := openStore(t, dbPath(t), true)
		requireGooseVersion(t, s, 11)
	})

	t.Run("TTB-001/banco na versão 9 com jogos migra sem perder dados e sem criar linhas", func(t *testing.T) {
		t.Parallel()
		var before []string
		const games = `SELECT id, platform_id, title FROM game ORDER BY id`
		path := versionNineFile(t, func(t *testing.T, db *sql.DB) {
			m10Game(t, db, "Um")
			m10Game(t, db, "Dois")
			before = dumpRows(t, db, games)
		})
		if len(before) != 2 {
			t.Fatalf("precondition: %d games seeded, want 2", len(before))
		}
		s := openStore(t, path, true)
		requireGooseVersion(t, s, 11) // alterado em 2026-11-25 (fase 16)
		if after := dumpRows(t, s.DB(), games); !slices.Equal(after, before) {
			t.Errorf("games changed:\n before %v\n after  %v", before, after)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM time_to_beat`); n != 0 {
			t.Errorf("time_to_beat rows after migration = %d, want 0", n)
		}
	})

	t.Run("TTB-001/ON DELETE CASCADE apaga a linha ao remover o jogo", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := m10Game(t, s.DB(), "Jogo")
		other := m10Game(t, s.DB(), "Outro")
		for _, id := range []int64{g, other} {
			if err := m10Insert(t, s.DB(), id, "hltb", "found", 0, 3600, nil, nil); err != nil {
				t.Fatalf("insert: %v", err)
			}
		}
		if _, err := s.DB().ExecContext(t.Context(), `DELETE FROM game WHERE id = ?`, g); err != nil {
			t.Fatalf("delete game: %v", err)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM time_to_beat WHERE game_id = ?`, g); n != 0 {
			t.Errorf("rows of the deleted game = %d, want 0", n)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM time_to_beat WHERE game_id = ?`, other); n != 1 {
			t.Errorf("rows of the other game = %d, want 1", n)
		}
	})

	t.Run("TTB-001/CHECK: source aceita hltb e igdb, e igdb só com found e confirmed 0", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		cases := []struct {
			name      string
			source    string
			state     string
			confirmed int
			ok        bool
		}{
			{"hltb found", "hltb", "found", 0, true},
			{"hltb found confirmado", "hltb", "found", 1, true},
			{"hltb doubt", "hltb", "doubt", 0, true},
			{"hltb not-found", "hltb", "not-found", 0, true},
			{"igdb found", "igdb", "found", 0, true},
			{"igdb found confirmado", "igdb", "found", 1, false},
			{"igdb doubt", "igdb", "doubt", 0, false},
			{"igdb not-found", "igdb", "not-found", 0, false},
			{"fonte desconhecida", "steam", "found", 0, false},
			{"estado desconhecido", "hltb", "maybe", 0, false},
			{"confirmed 2", "hltb", "found", 2, false},
		}
		for _, c := range cases {
			g := m10Game(t, db, c.name)
			err := m10Insert(t, db, g, c.source, c.state, c.confirmed, nil, nil, nil)
			if c.ok && err != nil {
				t.Errorf("%s: refused: %v", c.name, err)
			}
			if !c.ok && err == nil {
				t.Errorf("%s: accepted, want refused", c.name)
			}
		}
	})

	t.Run("TTB-001/CHECK: valores em segundos ≤ 0 são recusados e nulo é aceito", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		for _, col := range []int{0, 1, 2} {
			for _, bad := range []int64{0, -1} {
				vals := []any{nil, nil, nil}
				vals[col] = bad
				g := m10Game(t, db, "bad")
				if err := m10Insert(t, db, g, "hltb", "found", 0, vals[0], vals[1], vals[2]); err == nil {
					t.Errorf("column %d accepted %d", col, bad)
				}
			}
		}
		g := m10Game(t, db, "nulls")
		if err := m10Insert(t, db, g, "hltb", "found", 0, nil, nil, nil); err != nil {
			t.Errorf("all NULL refused: %v", err)
		}
		g = m10Game(t, db, "positive")
		if err := m10Insert(t, db, g, "hltb", "found", 0, 1, 2, 3); err != nil {
			t.Errorf("positive values refused: %v", err)
		}
	})

	t.Run("TTB-001/Down apaga a tabela e não mexe nas outras", func(t *testing.T) {
		t.Parallel()
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
		g := m10Game(t, db, "Jogo")
		if err := m10Insert(t, db, g, "hltb", "found", 0, 60, nil, nil); err != nil {
			t.Fatalf("seed: %v", err)
		}
		others := dumpRows(t, db, m10OtherSchema)
		games := dumpRows(t, db, `SELECT id, title FROM game ORDER BY id`)

		if _, err := provider.DownTo(ctx, 9); err != nil {
			t.Fatalf("goose DownTo(9): %v", err)
		}
		if v, err := provider.GetDBVersion(ctx); err != nil || v != 9 {
			t.Fatalf("goose version after Down = %d (err=%v), want 9", v, err)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'time_to_beat'`); n != 0 {
			t.Errorf("time_to_beat still exists after Down")
		}
		if got := dumpRows(t, db, m10OtherSchema); !slices.Equal(got, others) {
			t.Errorf("schema changed by Down:\n before %v\n after  %v", others, got)
		}
		if got := dumpRows(t, db, `SELECT id, title FROM game ORDER BY id`); !slices.Equal(got, games) {
			t.Errorf("games after Down = %v, want %v", got, games)
		}
	})

	t.Run("TTB-001/nenhuma outra tabela muda (sqlite_master sem time_to_beat é igual ao da v9)", func(t *testing.T) {
		t.Parallel()
		var before []string
		path := versionNineFile(t, func(t *testing.T, db *sql.DB) {
			before = dumpRows(t, db, m10OtherSchema)
		})
		if len(before) == 0 {
			t.Fatal("precondition: no schema rows read from the v9 database")
		}
		s := openStore(t, path, true)
		if after := dumpRows(t, s.DB(), m10OtherSchema); !slices.Equal(after, before) {
			t.Errorf("schema outside time_to_beat changed:\n before %v\n after  %v", before, after)
		}
	})
}
