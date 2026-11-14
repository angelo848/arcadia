package store

import (
	"testing"
	"time"
)

var statsNow = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// insertSession writes one closed session with raw SQL: the statistics read the table as it is.
func insertSession(t *testing.T, s *Store, gameID int64, startedAt time.Time, seconds int64, source string) {
	t.Helper()
	ms := startedAt.UnixMilli()
	if _, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source)
		 VALUES (?, ?, ?, ?, ?, 'ended', ?)`,
		gameID, ms, ms+seconds*1000, ms+seconds*1000, seconds, source); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

func statsGame(t *testing.T, s *Store, title string) int64 {
	t.Helper()
	g, err := s.AddGame(t.Context(), NewGame{Title: title, Exe: `C:\` + title + `.exe`, At: statsNow})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	return g.ID
}

func TestStats(t *testing.T) {
	t.Parallel()

	t.Run("EST-001/por jogo: tempo, sessões e última vez; ordem do maior tempo", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		a := statsGame(t, s, "Celeste")
		b := statsGame(t, s, "Hades")
		insertSession(t, s, a, statsNow.AddDate(0, 0, -1), 600, "tracked")
		insertSession(t, s, a, statsNow.AddDate(0, 0, -2), 300, "tracked")
		insertSession(t, s, b, statsNow.AddDate(0, 0, -3), 1200, "tracked")

		got, err := s.StatsByGame(t.Context(), time.Time{})
		if err != nil {
			t.Fatalf("StatsByGame: %v", err)
		}
		if len(got) != 2 || got[0].Title != "Hades" || got[0].ActiveSeconds != 1200 || got[0].Sessions != 1 {
			t.Fatalf("games = %+v, want Hades first with 1200 s", got)
		}
		if got[1].Title != "Celeste" || got[1].ActiveSeconds != 900 || got[1].Sessions != 2 {
			t.Errorf("Celeste = %+v, want 900 s in 2 sessions", got[1])
		}
	})

	t.Run("EST-003/por plataforma em ordem decrescente, sem plataforma sem tempo", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := statsGame(t, s, "Celeste")
		insertSession(t, s, g, statsNow.AddDate(0, 0, -1), 500, "tracked")
		got, err := s.StatsByPlatform(t.Context(), time.Time{})
		if err != nil {
			t.Fatalf("StatsByPlatform: %v", err)
		}
		if len(got) != 1 || got[0].Name != "Windows" || got[0].ActiveSeconds != 500 {
			t.Errorf("platforms = %+v, want Windows with 500 s only", got)
		}
	})

	t.Run("EST-004/o período corta sessões antigas", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := statsGame(t, s, "Celeste")
		insertSession(t, s, g, statsNow.AddDate(0, 0, -3), 100, "tracked")
		insertSession(t, s, g, statsNow.AddDate(0, 0, -40), 900, "tracked")

		week, err := s.StatsByGame(t.Context(), statsNow.AddDate(0, 0, -7))
		if err != nil || len(week) != 1 || week[0].ActiveSeconds != 100 {
			t.Errorf("7 days = %+v, %v; want 100 s", week, err)
		}
		all, err := s.StatsByGame(t.Context(), time.Time{})
		if err != nil || len(all) != 1 || all[0].ActiveSeconds != 1000 {
			t.Errorf("all = %+v, %v; want 1000 s", all, err)
		}
	})

	t.Run("EST-005/sessão importada entra no total e vem marcada à parte", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := statsGame(t, s, "Celeste")
		insertSession(t, s, g, statsNow.AddDate(0, 0, -1), 300, "tracked")
		insertSession(t, s, g, statsNow.AddDate(0, 0, -200), 3600, "import")

		got, err := s.StatsByGame(t.Context(), time.Time{})
		if err != nil || len(got) != 1 {
			t.Fatalf("StatsByGame = %+v, %v", got, err)
		}
		if got[0].ActiveSeconds != 3900 || got[0].ImportedSeconds != 3600 {
			t.Errorf("total %d imported %d, want 3900 and 3600", got[0].ActiveSeconds, got[0].ImportedSeconds)
		}
	})
}
