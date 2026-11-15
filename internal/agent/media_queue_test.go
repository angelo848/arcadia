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
	dup     map[string]bool // hashes AddMedia reports as already stored (added = false)
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
	if f.dup[m.Hash] {
		return false, nil
	}
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
	mu       sync.Mutex
	fail     map[string]bool
	failOnce map[string]bool          // fails only the first download of the URL
	gates    map[string]chan struct{} // holds the downloads of one URL until its channel is closed
	calls    map[string]int
	active   int
	maxSeen  int
	started  chan string
	gate     chan struct{}
}

func (f *fakeFetcher) Fetch(ctx context.Context, _ int64, url string) (media.Stored, bool, error) {
	f.mu.Lock()
	f.calls[url]++
	first := f.calls[url] == 1
	urlGate := f.gates[url]
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
	if urlGate != nil {
		select {
		case <-urlGate:
		case <-ctx.Done():
			return media.Stored{}, false, ctx.Err()
		}
	}
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return media.Stored{}, false, ctx.Err()
		}
	}
	if f.fail[url] || (first && f.failOnce[url]) {
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

func newTestQueue(st *fakeQueueStore, f *fakeFetcher, clock domain.Clock, sink mediaChangeSink) *mediaQueue {
	return newMediaQueue(st, f, clock, slog.New(slog.NewTextHandler(io.Discard, nil)), sink)
}

func TestMediaQueue(t *testing.T) {
	t.Parallel()

	t.Run("MID-005/job que estava rodando é retomado e termina", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(job(1, "https://cdn.example.test/a.png", store.JobRunning, 0))
		f := &fakeFetcher{calls: map[string]int{}, fail: map[string]bool{}}
		stop := runQueue(t, newTestQueue(st, f, domain.NewFakeClock(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)), nil))
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
		stop := runQueue(t, newTestQueue(st, f, domain.NewFakeClock(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)), nil))
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
		q := newTestQueue(st, f, domain.NewFakeClock(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)), nil)
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
		stop := runQueue(t, newTestQueue(st, f, clock, nil))
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

// spyCall is one MediaChanged call and how many rows the store held when it arrived.
type spyCall struct {
	gameID int64
	added  int
}

// spySink records every MediaChanged call (IMG-001).
type spySink struct {
	st    *fakeQueueStore
	calls chan spyCall

	mu  sync.Mutex
	all []spyCall
}

func newSpySink(st *fakeQueueStore) *spySink {
	return &spySink{st: st, calls: make(chan spyCall, 64)}
}

func (s *spySink) MediaChanged(gameID int64) {
	s.st.mu.Lock()
	c := spyCall{gameID: gameID, added: len(s.st.added)}
	s.st.mu.Unlock()
	s.mu.Lock()
	s.all = append(s.all, c)
	s.mu.Unlock()
	s.calls <- c
}

func (s *spySink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.all)
}

// next waits for the next MediaChanged call, or fails the test after five seconds.
func (s *spySink) next(t *testing.T) spyCall {
	t.Helper()
	select {
	case c := <-s.calls:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("no MediaChanged call within 5s")
		return spyCall{}
	}
}

func gameJob(id, game int64, url string) store.MediaJob {
	j := job(id, url, store.JobPending, 0)
	j.GameID = game
	return j
}

func newFetcher() *fakeFetcher {
	return &fakeFetcher{calls: map[string]int{}, fail: map[string]bool{}, failOnce: map[string]bool{}, gates: map[string]chan struct{}{}}
}

func TestMediaQueueNotices(t *testing.T) {
	t.Parallel()
	const u1, u2, u3 = "https://cdn.example.test/1.png", "https://cdn.example.test/2.png", "https://cdn.example.test/3.png"
	t0 := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)

	t.Run("IMG-001/gravação nova envia media.changed(G) uma vez, depois de AddMedia", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(gameJob(1, 7, u1))
		sink := newSpySink(st)
		stop := runQueue(t, newTestQueue(st, newFetcher(), domain.NewFakeClock(t0), sink))
		c := sink.next(t)
		if c.gameID != 7 || c.added != 1 {
			t.Errorf("call = %+v, want game 7 with 1 row already stored", c)
		}
		waitState(t, st, 1, store.JobDone)
		stop()
		if n := sink.count(); n != 1 {
			t.Errorf("MediaChanged calls = %d, want 1", n)
		}
	})

	t.Run("IMG-001/job pendente retomado na subida também avisa", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(job(1, u1, store.JobRunning, 0))
		sink := newSpySink(st)
		stop := runQueue(t, newTestQueue(st, newFetcher(), domain.NewFakeClock(t0), sink))
		defer stop()
		if c := sink.next(t); c.gameID != 1 || c.added != 1 {
			t.Errorf("call = %+v, want game 1 with 1 row stored", c)
		}
	})

	t.Run("IMG-002/download que falha e terá nova tentativa não avisa", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(gameJob(1, 1, u1))
		f := newFetcher()
		f.fail[u1] = true
		sink := newSpySink(st)
		stop := runQueue(t, newTestQueue(st, f, domain.NewFakeClock(t0), sink))
		waitAttempts(t, st, 1, 1)
		stop()
		if n := sink.count(); n != 0 {
			t.Errorf("MediaChanged calls = %d, want 0", n)
		}
	})

	t.Run("IMG-002/falha definitiva depois de 3 tentativas não avisa", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(gameJob(1, 1, u1))
		f := newFetcher()
		f.fail[u1] = true
		clock := domain.NewFakeClock(t0)
		sink := newSpySink(st)
		stop := runQueue(t, newTestQueue(st, f, clock, sink))
		for attempt := 1; attempt < mediaMaxAttempts; attempt++ {
			waitAttempts(t, st, 1, attempt)
			if err := clock.BlockUntilWaiters(t.Context(), 1); err != nil {
				t.Fatalf("BlockUntilWaiters: %v", err)
			}
			clock.Advance(mediaRetryDelay)
		}
		waitState(t, st, 1, store.JobFailed)
		stop()
		if n := sink.count(); n != 0 {
			t.Errorf("MediaChanged calls = %d, want 0", n)
		}
	})

	t.Run("IMG-002/job cancelado durante o download não avisa", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(gameJob(1, 1, u1))
		f := newFetcher()
		f.gates[u1] = make(chan struct{})
		f.started = make(chan string, 4)
		sink := newSpySink(st)
		q := newTestQueue(st, f, domain.NewFakeClock(t0), sink)
		stop := runQueue(t, q)
		<-f.started
		if err := q.Cancel(t.Context(), 1); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		waitState(t, st, 1, store.JobFailed)
		stop()
		if n := sink.count(); n != 0 {
			t.Errorf("MediaChanged calls = %d, want 0", n)
		}
	})

	t.Run("IMG-002/job interrompido pelo fim do ctx não avisa", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(gameJob(1, 1, u1))
		f := newFetcher()
		f.gates[u1] = make(chan struct{})
		f.started = make(chan string, 4)
		sink := newSpySink(st)
		stop := runQueue(t, newTestQueue(st, f, domain.NewFakeClock(t0), sink))
		<-f.started
		stop()
		if got := st.state(1); got.State != store.JobPending {
			t.Errorf("job state = %q, want pending", got.State)
		}
		if n := sink.count(); n != 0 {
			t.Errorf("MediaChanged calls = %d, want 0", n)
		}
	})

	t.Run("IMG-002/hash repetido (added = false) não avisa", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(gameJob(1, 1, u1))
		st.dup = map[string]bool{"h-" + u1: true}
		sink := newSpySink(st)
		stop := runQueue(t, newTestQueue(st, newFetcher(), domain.NewFakeClock(t0), sink))
		waitState(t, st, 1, store.JobDone)
		stop()
		if n := sink.count(); n != 0 {
			t.Errorf("MediaChanged calls = %d, want 0", n)
		}
	})

	t.Run("IMG-003/duas gravações de G dentro da janela: aviso imediato e um aviso final depois de Advance(500ms)", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(gameJob(1, 1, u1), gameJob(2, 1, u2))
		f := newFetcher()
		f.gates[u1], f.gates[u2] = make(chan struct{}), make(chan struct{})
		clock := domain.NewFakeClock(t0)
		sink := newSpySink(st)
		stop := runQueue(t, newTestQueue(st, f, clock, sink))
		defer stop()

		close(f.gates[u1])
		if c := sink.next(t); c.gameID != 1 || c.added != 1 {
			t.Errorf("first call = %+v, want game 1 with 1 row", c)
		}
		close(f.gates[u2])
		waitState(t, st, 2, store.JobDone)
		// Only the pending notice can hold a waiter here: the queue has no retry.
		if err := clock.BlockUntilWaiters(t.Context(), 1); err != nil {
			t.Fatalf("BlockUntilWaiters: %v", err)
		}
		if n := sink.count(); n != 1 {
			t.Fatalf("calls before the window ends = %d, want 1", n)
		}
		clock.Advance(mediaChangedWindow)
		if c := sink.next(t); c.gameID != 1 || c.added != 2 {
			t.Errorf("final call = %+v, want game 1 with 2 rows", c)
		}
	})

	t.Run("IMG-003/gravações de G e H juntas: um aviso imediato de cada", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(gameJob(1, 1, u1), gameJob(2, 2, u2))
		sink := newSpySink(st)
		stop := runQueue(t, newTestQueue(st, newFetcher(), domain.NewFakeClock(t0), sink))
		got := map[int64]int{}
		for range 2 {
			got[sink.next(t).gameID]++
		}
		waitState(t, st, 1, store.JobDone)
		waitState(t, st, 2, store.JobDone)
		stop()
		if got[1] != 1 || got[2] != 1 || sink.count() != 2 {
			t.Errorf("calls per game = %v (total %d), want one each", got, sink.count())
		}
	})

	t.Run("IMG-003/fila ociosa sem aviso pendente: nextDeadline false", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(gameJob(1, 1, u1))
		sink := newSpySink(st)
		q := newTestQueue(st, newFetcher(), domain.NewFakeClock(t0), sink)
		if _, ok := q.nextDeadline(); ok {
			t.Error("nextDeadline on a fresh queue = true, want false")
		}
		stop := runQueue(t, q)
		sink.next(t)
		waitState(t, st, 1, store.JobDone)
		stop()
		if d, ok := q.nextDeadline(); ok {
			t.Errorf("nextDeadline after an immediate notice = %v, true; want false", d)
		}
		if ch := q.nextTimer(); ch != nil {
			t.Error("nextTimer on an idle queue is not nil")
		}
	})

	t.Run("IMG-003/aviso pendente quando Run retorna é descartado", func(t *testing.T) {
		t.Parallel()
		st := newFakeQueueStore(gameJob(1, 1, u1), gameJob(2, 1, u2))
		f := newFetcher()
		f.gates[u1], f.gates[u2] = make(chan struct{}), make(chan struct{})
		clock := domain.NewFakeClock(t0)
		sink := newSpySink(st)
		q := newTestQueue(st, f, clock, sink)
		stop := runQueue(t, q)
		close(f.gates[u1])
		sink.next(t)
		close(f.gates[u2])
		waitState(t, st, 2, store.JobDone)
		if err := clock.BlockUntilWaiters(t.Context(), 1); err != nil {
			t.Fatalf("BlockUntilWaiters: %v", err)
		}
		stop()
		clock.Advance(mediaChangedWindow)
		q.flushChanges()
		if n := sink.count(); n != 1 {
			t.Errorf("calls = %d, want 1 (the pending notice is dropped)", n)
		}
		if _, ok := q.nextDeadline(); ok {
			t.Error("nextDeadline after Run returned = true, want false")
		}
	})

	t.Run("IMG-003/nova tentativa e aviso pendentes: o temporizador segue o prazo mais cedo e os dois acontecem", func(t *testing.T) {
		t.Parallel()
		// Jobs 1 and 2 write game 1 (the second inside the window); job 3 fails once, then succeeds.
		st := newFakeQueueStore(gameJob(1, 1, u1), gameJob(2, 1, u2), gameJob(3, 1, u3))
		f := newFetcher()
		f.gates[u1], f.gates[u2], f.gates[u3] = make(chan struct{}), make(chan struct{}), make(chan struct{})
		f.failOnce[u3] = true
		clock := domain.NewFakeClock(t0)
		sink := newSpySink(st)
		q := newTestQueue(st, f, clock, sink)
		stop := runQueue(t, q)
		defer stop()

		close(f.gates[u1])
		sink.next(t)
		close(f.gates[u2])
		waitState(t, st, 2, store.JobDone)
		if err := clock.BlockUntilWaiters(t.Context(), 1); err != nil {
			t.Fatalf("BlockUntilWaiters: %v", err)
		}
		close(f.gates[u3])
		waitAttempts(t, st, 3, 1)

		if d, ok := q.nextDeadline(); !ok || !d.Equal(t0.Add(mediaChangedWindow)) {
			t.Errorf("nextDeadline = %v, %v; want %v (the notice is earlier than the retry)", d, ok, t0.Add(mediaChangedWindow))
		}
		clock.Advance(mediaChangedWindow)
		if c := sink.next(t); c.gameID != 1 {
			t.Errorf("final call = %+v, want game 1", c)
		}
		if d, ok := q.nextDeadline(); !ok || !d.Equal(t0.Add(mediaRetryDelay)) {
			t.Errorf("nextDeadline = %v, %v; want the retry at %v", d, ok, t0.Add(mediaRetryDelay))
		}
		// The advance above fired the notice timer, so any waiter now is the retry one.
		if err := clock.BlockUntilWaiters(t.Context(), 1); err != nil {
			t.Fatalf("BlockUntilWaiters: %v", err)
		}
		clock.Advance(mediaRetryDelay - mediaChangedWindow)
		waitState(t, st, 3, store.JobDone)
		if c := sink.next(t); c.gameID != 1 || c.added != 3 {
			t.Errorf("retry call = %+v, want game 1 with 3 rows", c)
		}
	})
}
