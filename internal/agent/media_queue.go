package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Queue limits (phase 7, MID-005, MID-008).
const (
	mediaConcurrency = 2
	mediaMaxAttempts = 3
	mediaRetryDelay  = 30 * time.Second
	mediaHTTPTimeout = 60 * time.Second // one download, including the image body

	videoConcurrency = 1         // the video lane downloads one at a time (VID-009)
	videoQuotaPause  = time.Hour // pause after the daily quota is spent (VID-016)
)

// mediaQueueStore is what the queue needs from the store.
type mediaQueueStore interface {
	ResetRunningMediaJobs(ctx context.Context) error
	PendingMediaJobs(ctx context.Context) ([]store.MediaJob, error)
	SetMediaJobState(ctx context.Context, id int64, state string, attempts int, lastErr string) error
	CancelMediaJob(ctx context.Context, id int64) error
	AddMedia(ctx context.Context, m store.MediaRow) (bool, error)
	CancelVideoJobs(ctx context.Context) (pending int, running []int64, err error)
}

// mediaFetcher downloads the media of one job.
type mediaFetcher interface {
	Fetch(ctx context.Context, job store.MediaJob) (media.Stored, bool, error)
}

// imageFetcher downloads images with the media store.
type imageFetcher struct{ files *media.Store }

func (f imageFetcher) Fetch(ctx context.Context, job store.MediaJob) (media.Stored, bool, error) {
	return f.files.Fetch(ctx, job.GameID, job.SourceURL)
}

// fetchRouter sends video jobs to the video resolver and the rest to the image fetcher.
type fetchRouter struct{ images, video mediaFetcher }

func (r fetchRouter) Fetch(ctx context.Context, job store.MediaJob) (media.Stored, bool, error) {
	if isVideoJob(job) {
		return r.video.Fetch(ctx, job)
	}
	return r.images.Fetch(ctx, job)
}

// isVideoJob is the only lane criterion (VID-009): which slot and which fetcher.
func isVideoJob(job store.MediaJob) bool { return job.Kind == "video" }

// videoPause is why the video lane is not taking jobs.
type videoPause int

const (
	videoActive videoPause = iota
	videoPausedQuota
	videoPausedCredentials
	videoNoCredentials
)

// videoLane is the state of the video lane; guarded by mediaQueue.mu.
type videoLane struct {
	kind   videoPause
	until  time.Time // end of the quota pause
	warned bool      // the Warn of this pause was already logged
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
	changes *mediaChangeCoalescer // guarded by mu
	active  map[int64]*activeJob  // jobs running now, by job id; guarded by mu
	retryAt map[int64]time.Time   // failed jobs waiting for their next attempt

	video      videoLane               // guarded by mu
	videoReady func() bool             // set once before Run; nil = always ready
	videoQuota func() scrapers.SSQuota // set once before Run; nil = unknown
}

// activeJob is a download in progress: its game (for CancelGame), the cancel of its context and
// done, closed once its goroutine has finished (DEL-008).
type activeJob struct {
	gameID int64
	cancel context.CancelFunc
	done   chan struct{}
}

// mediaCancelWait bounds the wait of CancelGame for the downloads it interrupted (DEL-008).
const mediaCancelWait = 5 * time.Second

// mediaChangeSink receives the media.changed notices (IMG-001..003). It is always called outside q.mu.
type mediaChangeSink interface{ MediaChanged(gameID int64) }

func newMediaQueue(st mediaQueueStore, fetch mediaFetcher, clock domain.Clock, log *slog.Logger, sink mediaChangeSink) *mediaQueue {
	return &mediaQueue{
		st: st, fetch: fetch, clock: clock, log: log, sink: sink,
		changes: newMediaChangeCoalescer(mediaChangedWindow),
		wake:    make(chan struct{}, 1),
		active:  map[int64]*activeJob{},
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
	act := q.active[id]
	q.mu.Unlock()
	if act != nil {
		act.cancel()
	}
	return nil
}

// CancelGame interrupts the downloads in progress of gameID and waits until they have left, so no
// file of the game is written after it returns (DEL-008). Downloads of other games and pending jobs
// are not touched: pending jobs of the game go with its row. The wait is bounded by mediaCancelWait
// on the clock; past it, or when ctx ends, CancelGame returns an error and the caller goes on.
func (q *mediaQueue) CancelGame(ctx context.Context, gameID int64) error {
	q.mu.Lock()
	var acts []*activeJob
	for _, act := range q.active {
		if act.gameID == gameID {
			acts = append(acts, act)
		}
	}
	q.mu.Unlock()
	if len(acts) == 0 {
		return nil
	}
	for _, act := range acts {
		act.cancel()
	}
	deadline := q.clock.After(mediaCancelWait)
	for _, act := range acts {
		select {
		case <-act.done:
		case <-deadline:
			return fmt.Errorf("agent: downloads do jogo %d ainda em curso após %v", gameID, mediaCancelWait)
		case <-ctx.Done():
			return fmt.Errorf("agent: esperar downloads do jogo %d: %w", gameID, ctx.Err())
		}
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
	vsem := make(chan struct{}, videoConcurrency)
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
		q.dispatch(ctx, sem, vsem, &wg)
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
func (q *mediaQueue) dispatch(ctx context.Context, isem, vsem chan struct{}, wg *sync.WaitGroup) {
	jobs, err := q.st.PendingMediaJobs(ctx)
	if err != nil {
		q.log.Error("fila de mídia: listar jobs", "error", err.Error())
		return
	}
	now := q.clock.Now()
	videoOpen := q.videoOpen(now)
	imagesFull, videoBlocked := false, !videoOpen
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		if imagesFull && videoBlocked {
			return
		}
		isVideo := isVideoJob(job)
		if (isVideo && videoBlocked) || (!isVideo && imagesFull) {
			continue
		}
		q.mu.Lock()
		due, waiting := q.retryAt[job.ID]
		q.mu.Unlock()
		if waiting && now.Before(due) {
			continue
		}
		sem := isem
		if isVideo {
			sem = vsem
		}
		select {
		case sem <- struct{}{}:
		default: // this lane is full; the other one is not held back
			if isVideo {
				videoBlocked = true
			} else {
				imagesFull = true
			}
			continue
		}
		if err := q.st.SetMediaJobState(ctx, job.ID, store.JobRunning, job.Attempts, ""); err != nil {
			<-sem
			q.log.Error("fila de mídia: marcar job", "job", job.ID, "error", err.Error())
			continue
		}
		jobCtx, cancel := context.WithCancel(ctx)
		q.mu.Lock()
		delete(q.retryAt, job.ID) // the attempt is starting: its retry time is spent
		q.active[job.ID] = &activeJob{gameID: job.GameID, cancel: cancel, done: make(chan struct{})}
		q.mu.Unlock()
		wg.Add(1)
		go func(job store.MediaJob) {
			defer wg.Done()
			q.run(ctx, jobCtx, cancel, job)
			if isVideo {
				q.checkVideoQuota()
			}
			q.finish(job.ID)
			<-sem
			q.Notify() // a slot is free: the loop starts the next pending job now
		}(job)
	}
}

// run downloads one job and records its outcome. The store is written with the agent's context,
// so a job cancelled by the user keeps the state Cancel wrote, and a job interrupted by shutdown is
// pending again for the next start.
func (q *mediaQueue) run(parent, jobCtx context.Context, cancel context.CancelFunc, job store.MediaJob) {
	defer cancel()
	stored, _, err := q.fetch.Fetch(jobCtx, job)
	if err == nil {
		var added bool
		sourceURL := job.SourceURL
		if stored.SourceURL != "" {
			sourceURL = stored.SourceURL
		}
		added, err = q.st.AddMedia(jobCtx, store.MediaRow{
			GameID: job.GameID, Kind: job.Kind, Provider: job.Provider, SourceURL: sourceURL,
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
		var perm *media.PermanentError
		switch {
		case errors.As(err, &perm):
			q.log.Info("mídia: recusada de vez", "job", job.ID, "error", err.Error())
			q.setState(writeCtx, job.ID, store.JobFailed, job.Attempts+1, err.Error())
			return
		case errors.Is(err, scrapers.ErrQuota):
			q.pauseVideo(videoPausedQuota, q.clock.Now().Add(videoQuotaPause), "cota do ScreenScraper esgotada")
			q.setState(writeCtx, job.ID, store.JobPending, job.Attempts, "")
			q.Notify()
			return
		case errors.Is(err, scrapers.ErrCredentials):
			q.pauseVideo(videoPausedCredentials, time.Time{}, "credenciais do ScreenScraper recusadas")
			q.setState(writeCtx, job.ID, store.JobPending, job.Attempts, "")
			q.Notify()
			return
		case errors.Is(err, scrapers.ErrNoCredentials):
			q.pauseVideo(videoNoCredentials, time.Time{}, "")
			q.setState(writeCtx, job.ID, store.JobPending, job.Attempts, "")
			q.Notify()
			return
		}
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

// finish forgets a job whose download has returned and wakes the CancelGame waiting for it.
func (q *mediaQueue) finish(id int64) {
	q.mu.Lock()
	act := q.active[id]
	delete(q.active, id)
	q.mu.Unlock()
	if act != nil {
		close(act.done)
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
	if q.video.kind == videoPausedQuota && q.video.until.After(now) && (!found || q.video.until.Before(first)) {
		first, found = q.video.until, true
	}
	return first, found
}

// ConfigureVideo sets the video lane hooks; call it once before Run. ready reports whether both
// ScreenScraper credentials exist; quota is the last quota reading of the client.
func (q *mediaQueue) ConfigureVideo(ready func() bool, quota func() scrapers.SSQuota) {
	q.videoReady, q.videoQuota = ready, quota
}

// VideoCredentialsChanged lifts the credentials pauses and wakes the loop (VID-017, VID-019).
func (q *mediaQueue) VideoCredentialsChanged() {
	q.mu.Lock()
	if q.video.kind == videoPausedCredentials || q.video.kind == videoNoCredentials {
		q.video = videoLane{}
	}
	q.mu.Unlock()
	q.Notify()
}

// VideoLaneStatus is the state of the video lane: active, paused-quota (with the end of the pause),
// paused-credentials or no-credentials (VID-008).
func (q *mediaQueue) VideoLaneStatus() (string, time.Time) {
	if q.videoReady != nil && !q.videoReady() {
		return "no-credentials", time.Time{}
	}
	now := q.clock.Now()
	q.mu.Lock()
	defer q.mu.Unlock()
	switch q.video.kind {
	case videoPausedQuota:
		if now.Before(q.video.until) {
			return "paused-quota", q.video.until
		}
	case videoPausedCredentials:
		return "paused-credentials", time.Time{}
	case videoNoCredentials:
		return "no-credentials", time.Time{}
	}
	return "active", time.Time{}
}

// CancelVideo cancels the pending video jobs and interrupts the running ones; it returns how many (VID-007).
func (q *mediaQueue) CancelVideo(ctx context.Context) (int, error) {
	pending, running, err := q.st.CancelVideoJobs(ctx)
	if err != nil {
		return 0, fmt.Errorf("agent: cancelar vídeos: %w", err)
	}
	q.mu.Lock()
	for _, id := range running {
		delete(q.retryAt, id)
		if act := q.active[id]; act != nil {
			act.cancel()
		}
	}
	q.mu.Unlock()
	return pending + len(running), nil
}

// videoOpen reports whether the video lane may start a job now, ending an elapsed quota pause.
func (q *mediaQueue) videoOpen(now time.Time) bool {
	if q.videoReady != nil && !q.videoReady() {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	switch q.video.kind {
	case videoPausedQuota:
		if now.Before(q.video.until) {
			return false
		}
		q.video = videoLane{}
	case videoPausedCredentials, videoNoCredentials:
		return false
	}
	return true
}

// pauseVideo puts the lane in a pause and logs msg once per pause.
func (q *mediaQueue) pauseVideo(kind videoPause, until time.Time, msg string) {
	q.mu.Lock()
	warn := !(q.video.kind == kind && q.video.warned)
	q.video = videoLane{kind: kind, until: until, warned: true}
	q.mu.Unlock()
	if warn && msg != "" {
		q.log.Warn(msg)
	}
}

// checkVideoQuota pauses the lane when the last reading says today's requests are spent (VID-016).
func (q *mediaQueue) checkVideoQuota() {
	if q.videoQuota == nil {
		return
	}
	if sq := q.videoQuota(); sq.Known && sq.MaxPerDay > 0 && sq.RequestsToday >= sq.MaxPerDay {
		q.pauseVideo(videoPausedQuota, q.clock.Now().Add(videoQuotaPause), "cota do ScreenScraper esgotada")
	}
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
