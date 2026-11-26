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

// insertSessionState writes one session with an explicit state and, optionally, edited_at (SED-027, SED-037).
func insertSessionState(t *testing.T, s *Store, gameID int64, startedAt time.Time, seconds int64, source, state string, edited bool) {
	t.Helper()
	ms := startedAt.UnixMilli()
	var editedAt any
	if edited {
		editedAt = ms
	}
	if _, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source, edited_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		gameID, ms, ms+seconds*1000, ms+seconds*1000, seconds, state, source, editedAt); err != nil {
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

	t.Run("SED-027/StatsByGame traz ManualSeconds = parte da origem manual; a rastreada editada conta como rastreada", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := statsGame(t, s, "Celeste")
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -1), 300, "tracked", "ended", true)
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -2), 1800, "manual", "ended", false)
		got, err := s.StatsByGame(t.Context(), time.Time{})
		if err != nil || len(got) != 1 {
			t.Fatalf("StatsByGame = %+v, %v", got, err)
		}
		if got[0].ActiveSeconds != 2100 || got[0].ManualSeconds != 1800 || got[0].ImportedSeconds != 0 {
			t.Errorf("total %d manual %d imported %d, want 2100, 1800 and 0",
				got[0].ActiveSeconds, got[0].ManualSeconds, got[0].ImportedSeconds)
		}
	})

	t.Run("SED-027/StatsByPlatform traz ManualSeconds", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := statsGame(t, s, "Celeste")
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -1), 500, "tracked", "ended", false)
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -2), 700, "manual", "ended", false)
		got, err := s.StatsByPlatform(t.Context(), time.Time{})
		if err != nil || len(got) != 1 {
			t.Fatalf("StatsByPlatform = %+v, %v", got, err)
		}
		if got[0].ActiveSeconds != 1200 || got[0].ManualSeconds != 700 {
			t.Errorf("platform = %+v, want 1200 s with 700 manual", got[0])
		}
	})

	t.Run("SED-027/as três origens entram no tempo por jogo, por plataforma e em SessionTimes", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := statsGame(t, s, "Celeste")
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -1), 100, "tracked", "ended", false)
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -2), 200, "import", "ended", false)
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -3), 400, "manual", "ended", false)

		games, err := s.StatsByGame(t.Context(), time.Time{})
		if err != nil || len(games) != 1 || games[0].ActiveSeconds != 700 || games[0].Sessions != 3 {
			t.Errorf("games = %+v, %v; want 700 s in 3 sessions", games, err)
		}
		plats, err := s.StatsByPlatform(t.Context(), time.Time{})
		if err != nil || len(plats) != 1 || plats[0].ActiveSeconds != 700 {
			t.Errorf("platforms = %+v, %v; want 700 s", plats, err)
		}
		times, err := s.SessionTimes(t.Context(), time.Time{})
		if err != nil || len(times) != 3 {
			t.Fatalf("SessionTimes = %+v, %v; want 3", times, err)
		}
		var sum int64
		for _, st := range times {
			sum += st.ActiveSeconds
		}
		if sum != 700 {
			t.Errorf("SessionTimes sum = %d, want 700", sum)
		}
	})

	t.Run("SED-037/período com uma contada de 1 h, uma descartada de 20 s e uma aberta com 10 min gravados: StatsByGame soma 1 h e Sessions = 1", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := statsGame(t, s, "Celeste")
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -1), 3600, "tracked", "ended", false)
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -2), 20, "tracked", "discarded", false)
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -3), 600, "tracked", "running", false)
		got, err := s.StatsByGame(t.Context(), time.Time{})
		if err != nil || len(got) != 1 {
			t.Fatalf("StatsByGame = %+v, %v", got, err)
		}
		if got[0].ActiveSeconds != 3600 || got[0].Sessions != 1 {
			t.Errorf("game = %+v, want 3600 s in 1 session", got[0])
		}
	})

	t.Run("SED-037/StatsByPlatform só soma as contadas (plataforma só com descartada não aparece)", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := statsGame(t, s, "Celeste")
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -1), 20, "tracked", "discarded", false)
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -2), 600, "tracked", "running", false)
		got, err := s.StatsByPlatform(t.Context(), time.Time{})
		if err != nil || len(got) != 0 {
			t.Fatalf("platforms = %+v, %v; want none", got, err)
		}
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -3), 3600, "tracked", "ended", false)
		got, err = s.StatsByPlatform(t.Context(), time.Time{})
		if err != nil || len(got) != 1 || got[0].ActiveSeconds != 3600 {
			t.Errorf("platforms = %+v, %v; want 3600 s", got, err)
		}
	})

	t.Run("SED-037/SessionTimes só devolve as contadas", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := statsGame(t, s, "Celeste")
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -1), 3600, "tracked", "ended", false)
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -2), 20, "tracked", "discarded", false)
		insertSessionState(t, s, g, statsNow.AddDate(0, 0, -3), 600, "tracked", "running", false)
		got, err := s.SessionTimes(t.Context(), time.Time{})
		if err != nil || len(got) != 1 || got[0].ActiveSeconds != 3600 {
			t.Errorf("SessionTimes = %+v, %v; want only the 3600 s session", got, err)
		}
	})
}
