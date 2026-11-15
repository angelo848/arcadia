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
	mediaHTTPTimeout = 60 * time.Second // one download, including the image body
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
	sink  mediaChangeSink // nil = no notices

	mu      sync.Mutex
	changes *mediaChangeCoalescer        // guarded by mu
	cancels map[int64]context.CancelFunc // jobs running now
	retryAt map[int64]time.Time          // failed jobs waiting for their next attempt
}

// mediaChangeSink receives the media.changed notices (IMG-001..003). It is always called outside q.mu.
type mediaChangeSink interface{ MediaChanged(gameID int64) }

func newMediaQueue(st mediaQueueStore, fetch mediaFetcher, clock domain.Clock, log *slog.Logger, sink mediaChangeSink) *mediaQueue {
	return &mediaQueue{
		st: st, fetch: fetch, clock: clock, log: log, sink: sink,
		changes: newMediaChangeCoalescer(mediaChangedWindow),
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
	defer func() { // runs after wg.Wait: pending notices are dropped when Run returns
		q.mu.Lock()
		q.changes = newMediaChangeCoalescer(mediaChangedWindow)
		q.mu.Unlock()
	}()
	defer wg.Wait()
	// The loop wakes for new work (Notify), for a finished download that frees a slot, and for the
	// next retry or notice. The timer is re-armed only when its deadline changes, so a wake-up does
	// not leave abandoned waiters on the clock, and an idle queue holds none.
	var (
		armed   time.Time
		armedCh <-chan time.Time
	)
	for {
		if ctx.Err() != nil { // stopping: pending notices are dropped, not flushed [IMG-003]
			return nil
		}
		q.flushChanges()
		// The deadline is read before dispatch: a retry that is future now is covered by the timer,
		// one that is already due is started by dispatch, and a later one calls Notify.
		deadline, ok := q.nextDeadline()
		q.dispatch(ctx, sem, &wg)
		switch {
		case !ok:
			armedCh = nil
		case armedCh == nil || !deadline.Equal(armed):
			armed, armedCh = deadline, q.armTimer(deadline)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-q.wake:
		case <-armedCh:
			armedCh = nil // fired: the next volta re-arms if a deadline remains
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
		delete(q.retryAt, job.ID) // the attempt is starting: its retry time is spent
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
		var added bool
		added, err = q.st.AddMedia(jobCtx, store.MediaRow{
			GameID: job.GameID, Kind: job.Kind, Provider: job.Provider, SourceURL: job.SourceURL,
			Hash: stored.Hash, Path: stored.Path, ThumbPath: stored.ThumbPath, CreatedAt: q.clock.Now(),
		})
		if err == nil && added && parent.Err() == nil && jobCtx.Err() == nil {
			q.mu.Lock()
			send := q.changes.changed(job.GameID, q.clock.Now())
			q.mu.Unlock()
			if send && q.sink != nil {
				q.sink.MediaChanged(job.GameID)
			} else {
				q.Notify() // the loop arms the coalescing timer
			}
		}
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
		q.Notify() // the retry is scheduled: the loop arms its timer
	}
}

func (q *mediaQueue) setState(ctx context.Context, id int64, state string, attempts int, lastErr string) {
	if err := q.st.SetMediaJobState(ctx, id, state, attempts, lastErr); err != nil {
		q.log.Error("fila de mídia: gravar estado", "job", id, "error", err.Error())
	}
}

// flushChanges sends the coalesced notices whose window ended.
func (q *mediaQueue) flushChanges() {
	q.mu.Lock()
	ids := q.changes.due(q.clock.Now())
	q.mu.Unlock()
	if q.sink == nil {
		return
	}
	for _, id := range ids {
		q.sink.MediaChanged(id)
	}
}

// nextDeadline is the earliest of the next future retry and the next pending notice (which may
// already be due, so the loop fires at once).
func (q *mediaQueue) nextDeadline() (time.Time, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.clock.Now()
	var first time.Time
	found := false
	for _, due := range q.retryAt {
		if due.After(now) && (!found || due.Before(first)) {
			first, found = due, true
		}
	}
	if end, ok := q.changes.next(); ok && (!found || end.Before(first)) {
		first, found = end, true
	}
	return first, found
}

// nextTimer returns a channel that fires at nextDeadline, or nil when there is none (a nil channel
// never fires in a select). A retry already due is started by dispatch, not by a timer.
func (q *mediaQueue) nextTimer() <-chan time.Time {
	deadline, ok := q.nextDeadline()
	if !ok {
		return nil
	}
	return q.armTimer(deadline)
}

// armTimer returns a channel that fires at deadline. If the clock moved past the deadline while the
// timer was being armed (the timer may then be registered relative to the later instant), the
// returned channel fires at once instead.
func (q *mediaQueue) armTimer(deadline time.Time) <-chan time.Time {
	ch := q.clock.After(deadline.Sub(q.clock.Now())) // d <= 0 fires at once
	if q.clock.Now().Before(deadline) {
		return ch
	}
	fired := make(chan time.Time, 1)
	fired <- q.clock.Now()
	return fired
}
