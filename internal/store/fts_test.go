package store

// White-box tests: reindexGame and deleteGameIndex are unexported (ADR 0020).

import (
	"database/sql"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

var ftsAddAt = time.Date(2026, time.November, 9, 14, 20, 30, 0, time.UTC)

// ftsOpen opens a migrated store in t.TempDir() and closes it at cleanup.
func ftsOpen(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("Open(Migrate=true): %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Store.Close: %v", err)
		}
	})
	return s
}

// ftsInTx runs fn in a write transaction, committing on success.
func ftsInTx(t *testing.T, s *Store, fn func(tx *sql.Tx) error) error {
	t.Helper()
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return nil
}

// ftsMatchRowids returns the sorted rowids of game_fts matching the FTS5 expression.
func ftsMatchRowids(t *testing.T, s *Store, match string) []int64 {
	t.Helper()
	rows, err := s.db.QueryContext(t.Context(),
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

func ftsCount(t *testing.T, s *Store, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := s.db.QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// ftsCompany inserts a company (reusing an existing row) and links it to the game.
func ftsCompany(t *testing.T, s *Store, gameID int64, table, name string) {
	t.Helper()
	ctx := t.Context()
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO company (name) VALUES (?)`, name); err != nil {
		t.Fatalf("insert company %q: %v", name, err)
	}
	var companyID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM company WHERE name = ?`, name).Scan(&companyID); err != nil {
		t.Fatalf("company id %q: %v", name, err)
	}
	q := "INSERT INTO " + table + " (game_id, company_id) VALUES (?, ?)"
	if _, err := s.db.ExecContext(ctx, q, gameID, companyID); err != nil {
		t.Fatalf("link %q to game %d in %s: %v", name, gameID, table, err)
	}
}

func TestFTSIndexing(t *testing.T) {
	t.Parallel()

	t.Run("FTS-001/linha com rowid = id e colunas título, descrição, desenvolvedoras e editoras (empresas inseridas direto no teste)", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()

		g, err := s.AddGame(ctx, NewGame{Title: "Hollow Knight", Exe: `C:\Games\hk.exe`, At: ftsAddAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE game SET description = 'Metroidvania' WHERE id = ?`, g.ID); err != nil {
			t.Fatalf("set description: %v", err)
		}
		ftsCompany(t, s, g.ID, "game_developer", "Team Cherry")
		ftsCompany(t, s, g.ID, "game_publisher", "Devolver Digital")

		if err := ftsInTx(t, s, func(tx *sql.Tx) error { return reindexGame(ctx, tx, g.ID) }); err != nil {
			t.Fatalf("reindexGame: %v", err)
		}

		var rowid int64
		var title, description, developer, publisher string
		if err := s.db.QueryRowContext(ctx,
			`SELECT rowid, title, description, developer, publisher FROM game_fts WHERE rowid = ?`, g.ID).
			Scan(&rowid, &title, &description, &developer, &publisher); err != nil {
			t.Fatalf("read game_fts row %d: %v", g.ID, err)
		}
		if rowid != g.ID {
			t.Errorf("rowid = %d, want %d", rowid, g.ID)
		}
		if title != "Hollow Knight" {
			t.Errorf("title = %q, want Hollow Knight", title)
		}
		if description != "Metroidvania" {
			t.Errorf("description = %q, want Metroidvania", description)
		}
		if developer != "Team Cherry" {
			t.Errorf("developer = %q, want Team Cherry", developer)
		}
		if publisher != "Devolver Digital" {
			t.Errorf("publisher = %q, want Devolver Digital", publisher)
		}
	})

	t.Run("FTS-001/nome de gênero, de plataforma, ano e modo não são encontrados pelo índice", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()

		g, err := s.AddGame(ctx, NewGame{Title: "Dungeon Run", Exe: `C:\Games\dr.exe`, At: ftsAddAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		res, err := s.db.ExecContext(ctx, `INSERT INTO platform (name, category) VALUES ('Atari2600', 'console')`)
		if err != nil {
			t.Fatalf("insert platform: %v", err)
		}
		atariID, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("platform LastInsertId: %v", err)
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE game SET platform_id = ?, release_date = '1998-06-12' WHERE id = ?`, atariID, g.ID); err != nil {
			t.Fatalf("update game platform and year: %v", err)
		}
		res, err = s.db.ExecContext(ctx, `INSERT INTO genre (name) VALUES ('Roguelike')`)
		if err != nil {
			t.Fatalf("insert genre: %v", err)
		}
		genreID, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("genre LastInsertId: %v", err)
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO game_genre (game_id, genre_id) VALUES (?, ?)`, g.ID, genreID); err != nil {
			t.Fatalf("link genre: %v", err)
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO game_play_mode (game_id, play_mode_id)
			 SELECT ?, id FROM play_mode WHERE name = 'multi_local'`, g.ID); err != nil {
			t.Fatalf("link play mode: %v", err)
		}

		if err := ftsInTx(t, s, func(tx *sql.Tx) error { return reindexGame(ctx, tx, g.ID) }); err != nil {
			t.Fatalf("reindexGame: %v", err)
		}

		if got := ftsMatchRowids(t, s, "Dungeon"); !slices.Equal(got, []int64{g.ID}) {
			t.Fatalf("MATCH title rowids = %v, want [%d] (sanity check)", got, g.ID)
		}
		for _, match := range []string{"Roguelike", "Atari2600", "1998", "multi"} {
			if got := ftsMatchRowids(t, s, match); len(got) != 0 {
				t.Errorf("MATCH %q rowids = %v, want none (not indexed)", match, got)
			}
		}
	})

	t.Run("FTS-002/AddGame deixa a linha de game_fts com o título depois do commit", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()

		g, err := s.AddGame(ctx, NewGame{Title: "Celeste", Exe: `C:\Games\Celeste.exe`, At: ftsAddAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		var rowid int64
		var title string
		if err := s.db.QueryRowContext(ctx,
			`SELECT rowid, title FROM game_fts WHERE rowid = ?`, g.ID).Scan(&rowid, &title); err != nil {
			t.Fatalf("read game_fts row %d after AddGame: %v", g.ID, err)
		}
		if title != "Celeste" {
			t.Errorf("game_fts title = %q, want Celeste", title)
		}
	})

	t.Run("FTS-002/falha no índice (game_fts removida no teste com DROP TABLE) faz AddGame devolver erro e não grava o jogo", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()

		if _, err := s.db.ExecContext(ctx, `DROP TABLE game_fts`); err != nil {
			t.Fatalf("DROP TABLE game_fts: %v", err)
		}
		if _, err := s.AddGame(ctx, NewGame{Title: "Celeste", Exe: `C:\Games\Celeste.exe`, At: ftsAddAt}); err == nil {
			t.Fatal("AddGame with a broken index returned nil error, want an error")
		}
		if n := ftsCount(t, s, `SELECT COUNT(*) FROM game`); n != 0 {
			t.Errorf("game rows after failed AddGame = %d, want 0", n)
		}
	})

	t.Run("GAM-001/o mesmo executável adicionado duas vezes gera dois jogos e duas linhas de índice", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()

		exe := `C:\Games\Celeste.exe`
		first, err := s.AddGame(ctx, NewGame{Title: "Celeste", Exe: exe, At: ftsAddAt})
		if err != nil {
			t.Fatalf("first AddGame: %v", err)
		}
		second, err := s.AddGame(ctx, NewGame{Title: "Celeste", Exe: exe, At: ftsAddAt})
		if err != nil {
			t.Fatalf("second AddGame: %v", err)
		}
		if first.ID == second.ID {
			t.Fatalf("both games have id %d, want distinct ids", first.ID)
		}
		if n := ftsCount(t, s, `SELECT COUNT(*) FROM game`); n != 2 {
			t.Errorf("game rows = %d, want 2", n)
		}
		if n := ftsCount(t, s, `SELECT COUNT(*) FROM game_fts`); n != 2 {
			t.Errorf("game_fts rows = %d, want 2", n)
		}
		if got := ftsMatchRowids(t, s, "Celeste"); !slices.Equal(got, []int64{min(first.ID, second.ID), max(first.ID, second.ID)}) {
			t.Errorf("MATCH Celeste rowids = %v, want both game ids", got)
		}
	})
}
