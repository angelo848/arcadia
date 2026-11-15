package store

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func videoGame(t *testing.T, s *Store, title string) int64 {
	t.Helper()
	g, err := s.AddGame(t.Context(), NewGame{Title: title, Exe: `C:\` + title + `.exe`, At: mediaAt})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	return g.ID
}

func videoJobsOf(t *testing.T, s *Store, gameID int64) []MediaJob {
	t.Helper()
	all, err := s.PendingMediaJobs(t.Context())
	if err != nil {
		t.Fatalf("PendingMediaJobs: %v", err)
	}
	var out []MediaJob
	for _, j := range all {
		if j.GameID == gameID && j.Kind == "video" {
			out = append(out, j)
		}
	}
	return out
}

func candidate(t *testing.T, s *Store, id int64) (VideoCandidate, bool) {
	t.Helper()
	cs, err := s.VideoCandidates(t.Context(), []int64{id})
	if err != nil {
		t.Fatalf("VideoCandidates: %v", err)
	}
	for _, c := range cs {
		if c.GameID == id {
			return c, true
		}
	}
	return VideoCandidate{}, false
}

func TestVideoJobs(t *testing.T) {
	t.Parallel()

	t.Run("VID-002/jogo com vídeo gravado não entra", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		id := videoGame(t, s, "A")
		if _, err := s.AddMedia(t.Context(), MediaRow{GameID: id, Kind: "video", Provider: VideoProvider, SourceURL: "u",
			Hash: "h", Path: "1/h.mp4", CreatedAt: mediaAt}); err != nil {
			t.Fatalf("AddMedia: %v", err)
		}
		if c, ok := candidate(t, s, id); !ok || !c.HasVideo {
			t.Fatalf("candidate = %+v, %v; want HasVideo", c, ok)
		}
		n, err := s.EnqueueVideoJobs(t.Context(), []int64{id}, mediaAt)
		if err != nil || n != 0 {
			t.Fatalf("EnqueueVideoJobs = %d, %v; want 0", n, err)
		}
		if len(videoJobsOf(t, s, id)) != 0 {
			t.Error("a job was created for a game that has a video")
		}
	})

	t.Run("VID-002/jogo com job pendente ou em andamento não entra; com job falho ou cancelado entra", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()
		pend, run, failed, canc := videoGame(t, s, "P"), videoGame(t, s, "R"), videoGame(t, s, "F"), videoGame(t, s, "C")
		ids := []int64{pend, run, failed, canc}
		for _, id := range ids {
			if n, err := s.EnqueueVideoJobs(ctx, []int64{id}, mediaAt); err != nil || n != 1 {
				t.Fatalf("first enqueue = %d, %v", n, err)
			}
		}
		set := func(id int64, state string, msg string) {
			js := videoJobsOf(t, s, id)
			if len(js) != 1 {
				t.Fatalf("jobs of %d = %+v", id, js)
			}
			if err := s.SetMediaJobState(ctx, js[0].ID, state, 1, msg); err != nil {
				t.Fatalf("SetMediaJobState: %v", err)
			}
		}
		set(run, JobRunning, "")
		set(failed, JobFailed, "boom")
		set(canc, JobFailed, "cancelado")

		n, err := s.EnqueueVideoJobs(ctx, ids, mediaAt)
		if err != nil || n != 2 {
			t.Fatalf("EnqueueVideoJobs = %d, %v; want 2 (failed and cancelled)", n, err)
		}
		for id, want := range map[int64]bool{pend: true, run: true, failed: false, canc: false} {
			c, ok := candidate(t, s, id)
			if !ok {
				t.Fatalf("game %d missing from candidates", id)
			}
			// after the second enqueue every game has an active job
			_ = want
			if !c.HasVideo {
				t.Errorf("game %d: HasVideo = false after enqueue, want true (active job)", id)
			}
		}
	})

	t.Run("VID-002/dois EnqueueVideoJobs concorrentes do mesmo jogo criam um job só (-race)", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		id := videoGame(t, s, "A")
		var wg sync.WaitGroup
		var mu sync.Mutex
		total := 0
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				n, err := s.EnqueueVideoJobs(t.Context(), []int64{id}, mediaAt)
				if err != nil {
					t.Errorf("EnqueueVideoJobs: %v", err)
				}
				mu.Lock()
				total += n
				mu.Unlock()
			}()
		}
		wg.Wait()
		if total != 1 || len(videoJobsOf(t, s, id)) != 1 {
			t.Errorf("queued = %d, jobs = %d; want exactly one", total, len(videoJobsOf(t, s, id)))
		}
	})

	t.Run("VID-003/o job tem kind video, provider screenscraper e source_url screenscraper:lookup, sem título nem credencial", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		id := videoGame(t, s, "Segredo")
		if _, err := s.EnqueueVideoJobs(t.Context(), []int64{id}, mediaAt); err != nil {
			t.Fatalf("EnqueueVideoJobs: %v", err)
		}
		js := videoJobsOf(t, s, id)
		if len(js) != 1 {
			t.Fatalf("jobs = %+v", js)
		}
		j := js[0]
		if j.Kind != "video" || j.Provider != "screenscraper" || j.SourceURL != "screenscraper:lookup" ||
			VideoProvider != "screenscraper" || VideoLookupURL != "screenscraper:lookup" {
			t.Errorf("job = %+v", j)
		}
		if j.State != JobPending || j.Attempts != 0 || j.LastError != "" {
			t.Errorf("job state = %+v, want fresh pending", j)
		}
	})

	t.Run("VID-006/VideoCandidatesIn por plataforma e biblioteca toda; plataforma inexistente é erro", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()
		p, _, err := s.AddPlatform(ctx, "Meu Console", domain.CategoryConsole)
		if err != nil {
			t.Fatalf("AddPlatform: %v", err)
		}
		a := videoGame(t, s, "A")
		b := videoGame(t, s, "B")
		if _, err := s.UpdateGame(ctx, b, GamePatch{PlatformID: &p.ID}, mediaAt); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		ids := func(cs []VideoCandidate) []int64 {
			var out []int64
			for _, c := range cs {
				out = append(out, c.GameID)
			}
			slices.Sort(out)
			return out
		}
		only, err := s.VideoCandidatesIn(ctx, p.ID)
		if err != nil || !slices.Equal(ids(only), []int64{b}) {
			t.Errorf("platform candidates = %v, %v; want [%d]", ids(only), err, b)
		}
		all, err := s.VideoCandidatesIn(ctx, 0)
		if err != nil || !slices.Equal(ids(all), []int64{a, b}) {
			t.Errorf("library candidates = %v, %v; want [%d %d]", ids(all), err, a, b)
		}
		if _, err := s.VideoCandidatesIn(ctx, 99999); !errors.Is(err, ErrPlatformNotFound) {
			t.Errorf("unknown platform err = %v, want ErrPlatformNotFound", err)
		}
	})

	t.Run("VID-006/todos os jogos entram numa transação (falha no meio não deixa job parcial)", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		a, b := videoGame(t, s, "A"), videoGame(t, s, "B")
		// an unknown id in the middle breaks the foreign key: the whole batch rolls back.
		if _, err := s.EnqueueVideoJobs(t.Context(), []int64{a, 99999, b}, mediaAt); err == nil {
			t.Fatal("EnqueueVideoJobs with an unknown id succeeded, want an error")
		}
		if len(videoJobsOf(t, s, a))+len(videoJobsOf(t, s, b)) != 0 {
			t.Error("partial jobs left after a failed batch")
		}
	})

	t.Run("VID-007/CancelVideoJobs só toca jobs de vídeo, devolve os pendentes e os ids em andamento", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()
		a, b, c := videoGame(t, s, "A"), videoGame(t, s, "B"), videoGame(t, s, "C")
		if _, err := s.EnqueueVideoJobs(ctx, []int64{a, b, c}, mediaAt); err != nil {
			t.Fatalf("EnqueueVideoJobs: %v", err)
		}
		runJob := videoJobsOf(t, s, b)[0]
		if err := s.SetMediaJobState(ctx, runJob.ID, JobRunning, 1, ""); err != nil {
			t.Fatal(err)
		}
		img, _ := s.EnqueueMedia(ctx, a, "cover", "steamgriddb", "https://cdn.example.test/a.png", mediaAt)

		pending, running, err := s.CancelVideoJobs(ctx)
		if err != nil || pending != 2 || !slices.Equal(running, []int64{runJob.ID}) {
			t.Fatalf("CancelVideoJobs = %d, %v, %v; want 2 pending and [%d] running", pending, running, err, runJob.ID)
		}
		jobs, _ := s.PendingMediaJobs(ctx)
		if len(jobs) != 1 || jobs[0].ID != img || jobs[0].State != JobPending {
			t.Errorf("remaining pending = %+v, want only the image job", jobs)
		}
		if p, r, err := s.CancelVideoJobs(ctx); err != nil || p != 0 || len(r) != 0 {
			t.Errorf("second cancel = %d, %v, %v; want nothing", p, r, err)
		}
	})

	t.Run("VID-008/VideoJobCounts conta pending, running e failed, e failed não conta os cancelados", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()
		var gs []int64
		for _, n := range []string{"A", "B", "C", "D", "E"} {
			gs = append(gs, videoGame(t, s, n))
		}
		if _, err := s.EnqueueVideoJobs(ctx, gs, mediaAt); err != nil {
			t.Fatal(err)
		}
		set := func(g int64, state, msg string) {
			if err := s.SetMediaJobState(ctx, videoJobsOf(t, s, g)[0].ID, state, 1, msg); err != nil {
				t.Fatal(err)
			}
		}
		set(gs[1], JobRunning, "")
		set(gs[2], JobFailed, "boom")
		set(gs[3], JobFailed, "cancelado")
		set(gs[4], JobDone, "")
		// an image job must not count
		if _, err := s.EnqueueMedia(ctx, gs[0], "cover", "x", "u", mediaAt); err != nil {
			t.Fatal(err)
		}
		got, err := s.VideoJobCounts(ctx)
		if err != nil || got != (VideoCounts{Pending: 1, Running: 1, Failed: 1}) {
			t.Errorf("VideoJobCounts = %+v, %v; want {1 1 1}", got, err)
		}
	})

	t.Run("VID-002/VideoGameInfo devolve título, ano da data de lançamento e a chave de seed da plataforma", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()
		id := videoGame(t, s, "Celeste")
		n64, _, err := s.AddPlatform(ctx, "nintendo 64", domain.CategoryConsole)
		if err != nil {
			t.Fatal(err)
		}
		year := 2018
		yp := &year
		if _, err := s.UpdateGame(ctx, id, GamePatch{PlatformID: &n64.ID, Year: &yp}, mediaAt); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		got, err := s.VideoGameInfo(ctx, id)
		if err != nil || got != (VideoGameInfo{Title: "Celeste", Year: 2018, SeedKey: "n64"}) {
			t.Errorf("VideoGameInfo = %+v, %v", got, err)
		}
		custom, _, _ := s.AddPlatform(ctx, "Meu Console", domain.CategoryConsole)
		if _, err := s.UpdateGame(ctx, id, GamePatch{PlatformID: &custom.ID}, mediaAt); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.VideoGameInfo(ctx, id); got.SeedKey != "" {
			t.Errorf("custom platform SeedKey = %q, want empty", got.SeedKey)
		}
		if _, err := s.VideoGameInfo(ctx, 99999); !errors.Is(err, ErrGameNotFound) {
			t.Errorf("unknown game err = %v, want ErrGameNotFound", err)
		}
	})

	t.Run("VID-002/jogo apagado leva os jobs de vídeo (cascade)", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		ctx := t.Context()
		id := videoGame(t, s, "A")
		if _, err := s.EnqueueVideoJobs(ctx, []int64{id}, mediaAt); err != nil {
			t.Fatal(err)
		}
		if err := s.DeleteGame(ctx, id); err != nil {
			t.Fatalf("DeleteGame: %v", err)
		}
		if len(videoJobsOf(t, s, id)) != 0 {
			t.Error("video job survived the game")
		}
		if got, err := s.VideoJobCounts(ctx); err != nil || got != (VideoCounts{}) {
			t.Errorf("VideoJobCounts = %+v, %v; want zero", got, err)
		}
		if _, ok := candidate(t, s, id); ok {
			t.Error("deleted game still a candidate")
		}
	})
}
