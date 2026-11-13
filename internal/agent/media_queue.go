package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Queue limits (phase 7, MID-005, MID-008).
const (
	mediaConcurrency = 2
	mediaMaxAttempts = 3
	mediaRetryDelay  = 30 * time.Second
	mediaPoll        = time.Second
)

// mediaQueueStore is what the queue needs from the store.
type mediaQueueStore interface {
	ResetRunningMediaJobs(ctx context.Context) error
	PendingMediaJobs(ctx context.Context) ([]store.MediaJob, error)
	SetMediaJobState(ctx context.Context, id int64, state string, attempts int, lastErr string) error
	CancelMediaJob(ctx context.Context, id int64) error
	AddMedia(ctx context.Context, m store.MediaRow) (bool, error)
}

// mediaFetcher downloads one image (media.Store).
type mediaFetcher interface {
	Fetch(ctx context.Context, gameID int64, url string) (media.Stored, bool, error)
}

// mediaQueue runs the media downloads in the background (MID-005). One goroutine owns the loop;
// the downloads it starts are waited for when Run returns.
type mediaQueue struct {
	st    mediaQueueStore
	fetch mediaFetcher
	clock domain.Clock
	log   *slog.Logger
	wake  chan struct{}

	mu      sync.Mutex
	cancels map[int64]context.CancelFunc // jobs running now
	retryAt map[int64]time.Time          // failed jobs waiting for their next attempt
}

func newMediaQueue(st mediaQueueStore, fetch mediaFetcher, clock domain.Clock, log *slog.Logger) *mediaQueue {
	return &mediaQueue{
		st: st, fetch: fetch, clock: clock, log: log,
		wake:    make(chan struct{}, 1),
		cancels: map[int64]context.CancelFunc{},
		retryAt: map[int64]time.Time{},
	}
}

// Notify asks the loop to look at the queue now. It never blocks.
func (q *mediaQueue) Notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Cancel stops a job (MID-005): it is marked cancelled in the store, and a download in progress is
// interrupted. A cancelled job never runs again.
func (q *mediaQueue) Cancel(ctx context.Context, id int64) error {
	if err := q.st.CancelMediaJob(ctx, id); err != nil {
		return fmt.Errorf("agent: cancelar mídia: %w", err)
	}
	q.mu.Lock()
	cancel := q.cancels[id]
	q.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// Run resets the jobs a stopped agent left running, then starts the pending ones until ctx ends.
// It returns after every download it started has finished (MID-005).
func (q *mediaQueue) Run(ctx context.Context) error {
	if err := q.st.ResetRunningMediaJobs(ctx); err != nil {
		return fmt.Errorf("agent: retomar fila de mídia: %w", err)
	}
	sem := make(chan struct{}, mediaConcurrency)
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		q.dispatch(ctx, sem, &wg)
		select {
		case <-ctx.Done():
			return nil
		case <-q.wake:
		case <-q.clock.After(mediaPoll):
		}
	}
}

// dispatch starts the pending jobs that have a free slot and are due for their attempt.
func (q *mediaQueue) dispatch(ctx context.Context, sem chan struct{}, wg *sync.WaitGroup) {
	jobs, err := q.st.PendingMediaJobs(ctx)
	if err != nil {
		q.log.Error("fila de mídia: listar jobs", "error", err.Error())
		return
	}
	now := q.clock.Now()
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		q.mu.Lock()
		due, waiting := q.retryAt[job.ID]
		q.mu.Unlock()
		if waiting && now.Before(due) {
			continue
		}
		select {
		case sem <- struct{}{}:
		default:
			return
		}
		if err := q.st.SetMediaJobState(ctx, job.ID, store.JobRunning, job.Attempts, ""); err != nil {
			<-sem
			q.log.Error("fila de mídia: marcar job", "job", job.ID, "error", err.Error())
			continue
		}
		jobCtx, cancel := context.WithCancel(ctx)
		q.mu.Lock()
		q.cancels[job.ID] = cancel
		q.mu.Unlock()
		wg.Add(1)
		go func(job store.MediaJob) {
			defer wg.Done()
			q.run(ctx, jobCtx, cancel, job)
			<-sem
			q.Notify() // a slot is free: the loop starts the next pending job now
		}(job)
	}
}

// run downloads one job and records its outcome. The store is written with the agent's context,
// so a job cancelled by the user keeps the state Cancel wrote, and a job interrupted by shutdown is
// pending again for the next start.
func (q *mediaQueue) run(parent, jobCtx context.Context, cancel context.CancelFunc, job store.MediaJob) {
	defer func() {
		q.mu.Lock()
		delete(q.cancels, job.ID)
		q.mu.Unlock()
		cancel()
	}()
	stored, _, err := q.fetch.Fetch(jobCtx, job.GameID, job.SourceURL)
	if err == nil {
		_, err = q.st.AddMedia(jobCtx, store.MediaRow{
			GameID: job.GameID, Kind: job.Kind, Provider: job.Provider, SourceURL: job.SourceURL,
			Hash: stored.Hash, Path: stored.Path, ThumbPath: stored.ThumbPath, CreatedAt: q.clock.Now(),
		})
	}
	writeCtx := context.WithoutCancel(parent)
	switch {
	case err == nil:
		q.mu.Lock()
		delete(q.retryAt, job.ID)
		q.mu.Unlock()
		q.setState(writeCtx, job.ID, store.JobDone, job.Attempts+1, "")
	case parent.Err() != nil:
		q.setState(writeCtx, job.ID, store.JobPending, job.Attempts, "")
	case jobCtx.Err() != nil:
		// Cancelled by the user: Cancel already wrote the state.
	default:
		attempts := job.Attempts + 1
		if attempts >= mediaMaxAttempts {
			q.log.Warn("mídia: falha definitiva", "job", job.ID, "attempts", attempts, "error", err.Error())
			q.setState(writeCtx, job.ID, store.JobFailed, attempts, err.Error())
			return
		}
		q.mu.Lock()
		q.retryAt[job.ID] = q.clock.Now().Add(mediaRetryDelay)
		q.mu.Unlock()
		q.setState(writeCtx, job.ID, store.JobPending, attempts, err.Error())
	}
}

func (q *mediaQueue) setState(ctx context.Context, id int64, state string, attempts int, lastErr string) {
	if err := q.st.SetMediaJobState(ctx, id, state, attempts, lastErr); err != nil {
		q.log.Error("fila de mídia: gravar estado", "job", id, "error", err.Error())
	}
}
