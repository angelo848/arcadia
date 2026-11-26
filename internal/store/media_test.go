package store

import (
	"context"
	"io/fs"
	"slices"
	"strings"
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

	t.Run("MID-004/migração 7 cria as tabelas media e media_job e SchemaVersion é 9 (alterado em 2026-11-15, fase 13)", func(t *testing.T) {
		t.Parallel()
		if SchemaVersion != 11 { // alterado em 2026-11-25 (fase 16)
			t.Fatalf("SchemaVersion = %d, want 11", SchemaVersion)
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

// amjGame adds a game and returns its id.
func amjGame(t *testing.T, s *Store, title string) int64 {
	t.Helper()
	g, err := s.AddGame(t.Context(), NewGame{Title: title, Exe: `C:\` + title + `.exe`, At: time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("AddGame(%s): %v", title, err)
	}
	return g.ID
}

// amjEnqueue queues one download of the given kind as pending and returns its job id.
func amjEnqueue(t *testing.T, s *Store, gameID int64, kind string) int64 {
	t.Helper()
	id, err := s.EnqueueMedia(t.Context(), gameID, kind, "igdb", "https://example.test/"+kind+".jpg", time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EnqueueMedia(%s): %v", kind, err)
	}
	return id
}

// amjCounts reads the active job counts of a game and fails the test on error.
func amjCounts(t *testing.T, s *Store, gameID int64) map[string]int {
	t.Helper()
	m, err := s.ActiveMediaJobCounts(t.Context(), gameID)
	if err != nil {
		t.Fatalf("ActiveMediaJobCounts: %v", err)
	}
	return m
}

func TestActiveMediaJobCounts(t *testing.T) {
	t.Parallel()

	t.Run("MET-021/pendente e em andamento contam por tipo", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := amjGame(t, s, "Jogo")
		amjEnqueue(t, s, g, "screenshot")
		amjEnqueue(t, s, g, "screenshot")
		running := amjEnqueue(t, s, g, "screenshot")
		if err := s.SetMediaJobState(t.Context(), running, JobRunning, 1, ""); err != nil {
			t.Fatalf("SetMediaJobState(running): %v", err)
		}
		amjEnqueue(t, s, g, "cover")

		m := amjCounts(t, s, g)
		if m["screenshot"] != 3 {
			t.Errorf("screenshot = %d, want 3 (2 pendentes + 1 em andamento)", m["screenshot"])
		}
		if m["cover"] != 1 {
			t.Errorf("cover = %d, want 1", m["cover"])
		}
	})

	t.Run("MET-021/concluído, falho e cancelado não contam", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := amjGame(t, s, "Jogo")
		done := amjEnqueue(t, s, g, "screenshot")
		if err := s.SetMediaJobState(t.Context(), done, JobDone, 1, ""); err != nil {
			t.Fatalf("SetMediaJobState(done): %v", err)
		}
		failed := amjEnqueue(t, s, g, "screenshot")
		if err := s.SetMediaJobState(t.Context(), failed, JobFailed, 3, "rede"); err != nil {
			t.Fatalf("SetMediaJobState(failed): %v", err)
		}
		cancelled := amjEnqueue(t, s, g, "screenshot")
		if err := s.CancelMediaJob(t.Context(), cancelled); err != nil {
			t.Fatalf("CancelMediaJob: %v", err)
		}

		m := amjCounts(t, s, g)
		for kind, n := range m {
			if n != 0 {
				t.Errorf("%s = %d, want 0 (só concluído, falho e cancelado)", kind, n)
			}
		}
		if m["screenshot"] != 0 {
			t.Errorf("screenshot = %d, want 0", m["screenshot"])
		}
	})

	t.Run("MET-021/concluído e falho não entram na contagem, o pendente entra", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := amjGame(t, s, "Jogo")
		done := amjEnqueue(t, s, g, "screenshot")
		if err := s.SetMediaJobState(t.Context(), done, JobDone, 1, ""); err != nil {
			t.Fatalf("SetMediaJobState(done): %v", err)
		}
		failed := amjEnqueue(t, s, g, "screenshot")
		if err := s.SetMediaJobState(t.Context(), failed, JobFailed, 3, "rede"); err != nil {
			t.Fatalf("SetMediaJobState(failed): %v", err)
		}
		amjEnqueue(t, s, g, "screenshot")

		if n := amjCounts(t, s, g)["screenshot"]; n != 1 {
			t.Errorf("screenshot = %d, want 1 (só o pendente)", n)
		}
	})

	t.Run("MET-021/outro jogo não conta", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		a := amjGame(t, s, "Jogo A")
		b := amjGame(t, s, "Jogo B")
		amjEnqueue(t, s, a, "screenshot")
		amjEnqueue(t, s, a, "screenshot")

		if n := amjCounts(t, s, a)["screenshot"]; n != 2 {
			t.Errorf("jogo A screenshot = %d, want 2", n)
		}
		for kind, n := range amjCounts(t, s, b) {
			if n != 0 {
				t.Errorf("jogo B %s = %d, want 0 (os jobs são do jogo A)", kind, n)
			}
		}
	})

	t.Run("MET-021/jogo sem job devolve mapa vazio", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := amjGame(t, s, "Jogo")

		if m := amjCounts(t, s, g); len(m) != 0 {
			t.Errorf("counts = %v, want empty", m)
		}
	})

	t.Run("MET-021/contexto cancelado devolve erro", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g := amjGame(t, s, "Jogo")
		amjEnqueue(t, s, g, "screenshot")

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := s.ActiveMediaJobCounts(ctx, g); err == nil {
			t.Error("ActiveMediaJobCounts with a canceled context returned nil error, want an error")
		}
	})
}

// MET-025: the phase adds no migration; the embedded list is the eight migrations already in the repository.
func TestNoNewMigrationPhase12(t *testing.T) {
	t.Parallel()

	t.Run("MET-025/nenhum arquivo novo em migrations", func(t *testing.T) {
		t.Parallel()
		entries, err := fs.ReadDir(embeddedMigrations, "migrations")
		if err != nil {
			t.Fatalf("ReadDir(migrations): %v", err)
		}
		var got []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
				got = append(got, e.Name())
			}
		}
		want := []string{
			"00001_initial.sql",
			"00002_app_setting.sql",
			"00003_session_end_reason.sql",
			"00004_pause_interval_open_uidx.sql",
			"00005_fts_rebuild.sql",
			"00006_emulators.sql",
			"00007_media.sql",
			"00008_platforms.sql",
			"00009_video.sql",        // alterado em 2026-11-15 (fase 13, VID-001)
			"00010_time_to_beat.sql", // alterado em 2026-11-22 (fase 15, TTB-001)
			"00011_session_edit.sql", // alterado em 2026-11-25 (fase 16, SED-001)
		}
		if !slices.Equal(got, want) {
			t.Errorf("migrations = %v, want %v", got, want)
		}
		if SchemaVersion != 11 { // alterado em 2026-11-25 (fase 16)
			t.Errorf("SchemaVersion = %d, want 11", SchemaVersion)
		}
	})
}
