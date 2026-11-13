package store

import (
	"errors"
	"testing"
	"time"
)

var mediaAt = time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

func mediaGame(t *testing.T, s *Store) int64 {
	t.Helper()
	g, err := s.AddGame(t.Context(), NewGame{Title: "Celeste", Exe: `C:\c.exe`, At: mediaAt})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	return g.ID
}

func TestMediaQueue(t *testing.T) {
	t.Parallel()

	t.Run("MID-005/um download entra na fila como pendente, em ordem", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		id := mediaGame(t, s)
		first, err := s.EnqueueMedia(t.Context(), id, "cover", "steamgriddb", "https://cdn.example.test/a.png", mediaAt)
		if err != nil {
			t.Fatalf("EnqueueMedia: %v", err)
		}
		second, _ := s.EnqueueMedia(t.Context(), id, "screenshot", "igdb", "https://cdn.example.test/b.png", mediaAt)
		jobs, err := s.PendingMediaJobs(t.Context())
		if err != nil || len(jobs) != 2 || jobs[0].ID != first || jobs[1].ID != second {
			t.Fatalf("PendingMediaJobs = %+v, %v; want the two jobs in order", jobs, err)
		}
		if jobs[0].State != JobPending || jobs[0].Attempts != 0 {
			t.Errorf("job = %+v, want pending with no attempts", jobs[0])
		}
	})

	t.Run("MID-005/job que estava rodando volta a pendente na retomada", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		id := mediaGame(t, s)
		job, _ := s.EnqueueMedia(t.Context(), id, "cover", "steamgriddb", "https://cdn.example.test/a.png", mediaAt)
		if err := s.SetMediaJobState(t.Context(), job, JobRunning, 1, ""); err != nil {
			t.Fatalf("SetMediaJobState: %v", err)
		}
		if err := s.ResetRunningMediaJobs(t.Context()); err != nil {
			t.Fatalf("ResetRunningMediaJobs: %v", err)
		}
		jobs, _ := s.PendingMediaJobs(t.Context())
		if len(jobs) != 1 || jobs[0].ID != job || jobs[0].Attempts != 1 {
			t.Errorf("after reset = %+v, want the job pending again with its attempts kept", jobs)
		}
	})

	t.Run("MID-005/cancelar um job pendente o tira da fila e não o roda de novo", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		id := mediaGame(t, s)
		job, _ := s.EnqueueMedia(t.Context(), id, "cover", "steamgriddb", "https://cdn.example.test/a.png", mediaAt)
		if err := s.CancelMediaJob(t.Context(), job); err != nil {
			t.Fatalf("CancelMediaJob: %v", err)
		}
		if jobs, _ := s.PendingMediaJobs(t.Context()); len(jobs) != 0 {
			t.Errorf("pending after cancel = %+v, want none", jobs)
		}
		if err := s.CancelMediaJob(t.Context(), 999); !errors.Is(err, ErrMediaJobNotFound) {
			t.Errorf("cancel unknown err = %v, want ErrMediaJobNotFound", err)
		}
	})

	t.Run("MID-004/a mesma imagem do jogo é registrada uma vez", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		id := mediaGame(t, s)
		row := MediaRow{GameID: id, Kind: "cover", Provider: "steamgriddb", SourceURL: "u", Hash: "h1",
			Path: "1/h1.png", ThumbPath: "1/thumb_h1.jpg", CreatedAt: mediaAt}
		added, err := s.AddMedia(t.Context(), row)
		if err != nil || !added {
			t.Fatalf("first AddMedia = %v, %v", added, err)
		}
		added, err = s.AddMedia(t.Context(), row)
		if err != nil || added {
			t.Errorf("second AddMedia = %v, %v; want false and no error", added, err)
		}
		list, err := s.ListMedia(t.Context(), id)
		if err != nil || len(list) != 1 || list[0].Path != "1/h1.png" || !list[0].CreatedAt.Equal(mediaAt) {
			t.Errorf("ListMedia = %+v, %v", list, err)
		}
	})
}
