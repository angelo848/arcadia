package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// fakeQueueStore is an in-memory mediaQueueStore; changed is signalled after every job write.
type fakeQueueStore struct {
	mu      sync.Mutex
	jobs    map[int64]store.MediaJob
	added   []store.MediaRow
	changed chan struct{}
}

func newFakeQueueStore(jobs ...store.MediaJob) *fakeQueueStore {
	f := &fakeQueueStore{jobs: map[int64]store.MediaJob{}, changed: make(chan struct{}, 64)}
	for _, j := range jobs {
		f.jobs[j.ID] = j
	}
	return f
}

func (f *fakeQueueStore) ResetRunningMediaJobs(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, j := range f.jobs {
		if j.State == store.JobRunning {
			j.State = store.JobPending
			f.jobs[id] = j
		}
	}
	return nil
}

func (f *fakeQueueStore) PendingMediaJobs(context.Context) ([]store.MediaJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.MediaJob
	for id := int64(1); id <= int64(len(f.jobs)); id++ {
		if j, ok := f.jobs[id]; ok && j.State == store.JobPending {
			out = append(out, j)
		}
	}
	return out, nil
}

func (f *fakeQueueStore) SetMediaJobState(_ context.Context, id int64, state string, attempts int, lastErr string) error {
	f.mu.Lock()
	j, ok := f.jobs[id]
	if !ok {
		f.mu.Unlock()
		return store.ErrMediaJobNotFound
	}
	j.State, j.Attempts, j.LastError = state, attempts, lastErr
	f.jobs[id] = j
	f.mu.Unlock()
	f.changed <- struct{}{}
	return nil
}

func (f *fakeQueueStore) CancelMediaJob(_ context.Context, id int64) error {
	f.mu.Lock()
	j, ok := f.jobs[id]
	if !ok {
		f.mu.Unlock()
		return store.ErrMediaJobNotFound
	}
	if j.State == store.JobPending || j.State == store.JobRunning {
		j.State, j.LastError = store.JobFailed, "cancelado"
		f.jobs[id] = j
	}
	f.mu.Unlock()
	f.changed <- struct{}{}
	return nil
}

func (f *fakeQueueStore) AddMedia(_ context.Context, m store.MediaRow) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, m)
	return true, nil
}

func (f *fakeQueueStore) state(id int64) store.MediaJob {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.jobs[id]
}

// waitState waits until job id has the state, or fails the test after five seconds.
func waitState(t *testing.T, f *fakeQueueStore, id int64, state string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if f.state(id).State == state {
			return
		}
		select {
		case <-f.changed:
		case <-deadline:
			t.Fatalf("job %d state = %q, want %q", id, f.state(id).State, state)
		}
	}
}

// fakeFetcher answers by URL. gate, when set, holds every download until it is closed.
type fakeFetcher struct {
	mu      sync.Mutex
	fail    map[string]bool
	calls   map[string]int
	active  int
	maxSeen int
	started chan string
	gate    chan struct{}
}

func (f *fakeFetcher) Fetch(ctx context.Context, _ int64, url string) (media.Stored, bool, error) {
	f.mu.Lock()
	f.calls[url]++
	f.active++
	if f.active > f.maxSeen {
		f.maxSeen = f.active
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()
	if f.started != nil {
		f.started <- url
	}
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return media.Stored{}, false, ctx.Err()
		}
	}
	if f.fail[url] {
		return media.Stored{}, false, errors.New("download recusado")
	}
	return media.Stored{Hash: "h-" + url, Path: "1/h.png", ThumbPath: "1/t.jpg"}, true, nil
}

func (f *fakeFetcher) callsOf(url string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[url]
}

func job(id int64, url string, state string, attempts int) store.MediaJob {
	return store.MediaJob{ID: id, GameID: 1, Kind: "cover", Provider: "steamgriddb", SourceURL: url, State: state, Attempts: attempts}
}

// runQueue starts the queue and returns a stop function that waits for Run to return.
func runQueue(t *testing.T, q *mediaQueue) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- q.Run(ctx) }()
	return func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run: %v", err)
		}
	}
}

func newTestQueue(st *fakeQueueStore, f *fakeFetcher, clock domain.Clock) *mediaQueue {
	return newMediaQueue(st, f, clock, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestMediaQueue(t *testing.T) {
	t.Parallel()

	t.Run("MID-005/job que estava rodando é retomado e termina", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(job(1, "https://cdn.example.test/a.png", store.JobRunning, 0))
		f := &fakeFetcher{calls: map[string]int{}, fail: map[string]bool{}}
		stop := runQueue(t, newTestQueue(st, f, domain.NewFakeClock(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))))
		defer stop()

		waitState(t, st, 1, store.JobDone)
		if n := f.callsOf("https://cdn.example.test/a.png"); n != 1 {
			t.Errorf("downloads = %d, want 1", n)
		}
		if len(st.added) != 1 || st.added[0].Hash != "h-https://cdn.example.test/a.png" {
			t.Errorf("media rows = %+v, want the downloaded image", st.added)
		}
	})

	t.Run("MID-005/no máximo dois downloads ao mesmo tempo", func(t *testing.T) {
		t.Parallel()
		jobs := []store.MediaJob{}
		for i := int64(1); i <= 5; i++ {
			jobs = append(jobs, job(i, "https://cdn.example.test/"+string(rune('a'+i))+".png", store.JobPending, 0))
		}
		st := newFakeQueueStore(jobs...)
		f := &fakeFetcher{calls: map[string]int{}, fail: map[string]bool{}, started: make(chan string, 8), gate: make(chan struct{})}
		stop := runQueue(t, newTestQueue(st, f, domain.NewFakeClock(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))))
		defer stop()

		<-f.started
		<-f.started
		select {
		case <-f.started:
			t.Fatal("a third download started while two were running")
		case <-time.After(100 * time.Millisecond):
		}
		close(f.gate)
		for i := int64(1); i <= 5; i++ {
			waitState(t, st, i, store.JobDone)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.maxSeen != 2 {
			t.Errorf("max concurrent downloads = %d, want 2", f.maxSeen)
		}
	})

	t.Run("MID-005/job cancelado não é baixado", func(t *testing.T) {
		t.Parallel()
		cancelled := "https://cdn.example.test/cancelado.png"
		kept := "https://cdn.example.test/ok.png"
		st := newFakeQueueStore(job(1, cancelled, store.JobPending, 0), job(2, kept, store.JobPending, 0))
		f := &fakeFetcher{calls: map[string]int{}, fail: map[string]bool{}}
		q := newTestQueue(st, f, domain.NewFakeClock(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)))
		if err := q.Cancel(t.Context(), 1); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		stop := runQueue(t, q)
		defer stop()

		waitState(t, st, 2, store.JobDone)
		if n := f.callsOf(cancelled); n != 0 {
			t.Errorf("cancelled job downloaded %d times, want 0", n)
		}
		if st.state(1).State != store.JobFailed || st.state(1).LastError != "cancelado" {
			t.Errorf("cancelled job = %+v, want failed with cancelado", st.state(1))
		}
	})

	t.Run("MID-008/três tentativas e depois failed, sem derrubar as outras", func(t *testing.T) {
		t.Parallel()
		bad := "https://cdn.example.test/ruim.png"
		good := "https://cdn.example.test/bom.png"
		st := newFakeQueueStore(job(1, bad, store.JobPending, 0), job(2, good, store.JobPending, 0))
		f := &fakeFetcher{calls: map[string]int{}, fail: map[string]bool{bad: true}}
		clock := domain.NewFakeClock(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))
		stop := runQueue(t, newTestQueue(st, f, clock))
		defer stop()

		waitState(t, st, 2, store.JobDone)
		for attempt := 1; attempt < mediaMaxAttempts; attempt++ {
			waitAttempts(t, st, 1, attempt)
			if err := clock.BlockUntilWaiters(t.Context(), 1); err != nil {
				t.Fatalf("BlockUntilWaiters: %v", err)
			}
			clock.Advance(mediaRetryDelay)
		}
		waitState(t, st, 1, store.JobFailed)
		if n := f.callsOf(bad); n != mediaMaxAttempts {
			t.Errorf("attempts = %d, want %d", n, mediaMaxAttempts)
		}
		if got := st.state(1); got.Attempts != mediaMaxAttempts || got.LastError == "" {
			t.Errorf("failed job = %+v, want attempts %d and the last error", got, mediaMaxAttempts)
		}
	})
}

// waitAttempts waits until job id has recorded n attempts and is pending again, or fails the
// test after five seconds.
func waitAttempts(t *testing.T, f *fakeQueueStore, id int64, n int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if got := f.state(id); got.Attempts == n && got.State == store.JobPending {
			return
		}
		select {
		case <-f.changed:
		case <-deadline:
			t.Fatalf("job %d = %+v, want %d attempts and pending", id, f.state(id), n)
		}
	}
}
