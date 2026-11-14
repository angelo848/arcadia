package store

import (
	"testing"
	"time"
)

// insertMedia writes one media row with raw SQL: the schema, not a store method, is under test.
func insertMedia(t *testing.T, s *Store, gameID int64, hash string) error {
	t.Helper()
	_, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO media (game_id, kind, provider, source_url, hash, path, created_at)
		 VALUES (?, 'cover', 'steamgriddb', '', ?, ?, 1789257600000)`,
		gameID, hash, hash+".jpg")
	return err
}

func insertJob(t *testing.T, s *Store, gameID int64) {
	t.Helper()
	_, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO media_job (game_id, kind, provider, source_url, state, created_at)
		 VALUES (?, 'screenshot', 'igdb', 'https://example.test/a.jpg', 'pending', 1789257600000)`,
		gameID)
	if err != nil {
		t.Fatalf("insert media_job: %v", err)
	}
}

func countRows(t *testing.T, s *Store, table string, gameID int64) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM "+table+" WHERE game_id = ?", gameID).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestMediaSchema(t *testing.T) {
	t.Parallel()

	t.Run("MID-004/migração 7 cria as tabelas media e media_job e SchemaVersion é 8", func(t *testing.T) {
		t.Parallel()
		if SchemaVersion != 8 {
			t.Fatalf("SchemaVersion = %d, want 8", SchemaVersion)
		}
		s := ftsOpen(t)
		for _, table := range []string{"media", "media_job"} {
			var n int
			if err := s.DB().QueryRowContext(t.Context(),
				"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&n); err != nil {
				t.Fatalf("sqlite_master: %v", err)
			}
			if n != 1 {
				t.Errorf("table %s missing", table)
			}
		}
	})

	t.Run("MID-004/o mesmo hash não se repete no mesmo jogo", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g, err := s.AddGame(t.Context(), NewGame{Title: "Jogo", Exe: `C:\j.exe`, At: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		if err := insertMedia(t, s, g.ID, "abc"); err != nil {
			t.Fatalf("first insert: %v", err)
		}
		if err := insertMedia(t, s, g.ID, "abc"); err == nil {
			t.Error("second insert with the same hash succeeded, want a unique violation")
		}
	})

	t.Run("MID-005/apagar o jogo apaga suas mídias e jobs", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g, err := s.AddGame(t.Context(), NewGame{Title: "Jogo", Exe: `C:\j.exe`, At: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		if err := insertMedia(t, s, g.ID, "h1"); err != nil {
			t.Fatalf("insert media: %v", err)
		}
		insertJob(t, s, g.ID)
		if err := s.DeleteGame(t.Context(), g.ID); err != nil {
			t.Fatalf("DeleteGame: %v", err)
		}
		if n := countRows(t, s, "media", g.ID); n != 0 {
			t.Errorf("media rows after delete = %d, want 0", n)
		}
		if n := countRows(t, s, "media_job", g.ID); n != 0 {
			t.Errorf("media_job rows after delete = %d, want 0", n)
		}
	})
}
