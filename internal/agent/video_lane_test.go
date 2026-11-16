package agent

// Testes da raia de vídeo da fila de mídia (fase 13, T24: VID-007, VID-008, VID-009, VID-014, VID-016..VID-021).
// Assinaturas assumidas (tasks.md T24):
//
//	type mediaFetcher interface{ Fetch(ctx, job store.MediaJob) (media.Stored, bool, error) }
//	type fetchRouter struct{ images, video mediaFetcher }            // kind == "video" -> video
//	(*mediaQueue).ConfigureVideo(ready func() bool, quota func() scrapers.SSQuota)
//	(*mediaQueue).VideoCredentialsChanged()
//	(*mediaQueue).VideoLaneStatus() (lane string, until time.Time)  // active | paused-quota | paused-credentials | no-credentials
//	(*mediaQueue).CancelVideo(ctx) (int, error)                      // pendentes + em andamento cancelados
//	mediaQueueStore.CancelVideoJobs(ctx) (pending int, running []int64, err error)  // já existe em store.Store
//	media.Stored.SourceURL (T19): se não vazio, é o source_url gravado; senão o do job.
//
// A raia grava o estado (pausa) ANTES de devolver o job a pending, para o teste poder esperar por "pending"
// e depois ler VideoLaneStatus. Sem time.Sleep: relógio fake e barreiras com um job de imagem duplicado.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

var laneT0 = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// CancelVideoJobs is the fake of store.CancelVideoJobs: pending and running video jobs become failed "cancelado".
func (f *fakeQueueStore) CancelVideoJobs(context.Context) (int, []int64, error) {
	f.mu.Lock()
	pending := 0
	var running []int64
	for id, j := range f.jobs {
		if j.Kind != "video" {
			continue
		}
		switch j.State {
		case store.JobPending:
			pending++
		case store.JobRunning:
			running = append(running, id)
		default:
			continue
		}
		j.State, j.LastError = store.JobFailed, "cancelado"
		f.jobs[id] = j
	}
	f.mu.Unlock()
	select {
	case f.changed <- struct{}{}:
	default:
	}
	return pending, running, nil
}

// put adds a pending job with the next id (ids stay contiguous, as the fake store lists them 1..n).
func (f *fakeQueueStore) put(j store.MediaJob) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	j.ID = int64(len(f.jobs)) + 1
	if j.State == "" {
		j.State = store.JobPending
	}
	f.jobs[j.ID] = j
	return j.ID
}

func (f *fakeQueueStore) markDup(hash string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dup == nil {
		f.dup = map[string]bool{}
	}
	f.dup[hash] = true
}

func (f *fakeQueueStore) addedRows() []store.MediaRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.MediaRow(nil), f.added...)
}

func vJob(id, game int64) store.MediaJob {
	return store.MediaJob{ID: id, GameID: game, Kind: "video", Provider: store.VideoProvider, SourceURL: store.VideoLookupURL, State: store.JobPending}
}

// laneVideo is the video side of the router: blockable and scriptable, and it counts concurrency.
type laneVideo struct {
	mu      sync.Mutex
	calls   map[int64]int
	total   int
	active  int
	maxSeen int
	started chan int64
	hold    chan struct{}           // holds every download until closed (or its ctx ends)
	gates   map[int64]chan struct{} // holds the downloads of one job
	result  func(job store.MediaJob, n int) (media.Stored, bool, error)
}

func newLaneVideo() *laneVideo {
	return &laneVideo{calls: map[int64]int{}, started: make(chan int64, 256), gates: map[int64]chan struct{}{}}
}

func (v *laneVideo) Fetch(ctx context.Context, job store.MediaJob) (media.Stored, bool, error) {
	v.mu.Lock()
	v.calls[job.ID]++
	n := v.calls[job.ID]
	v.total++
	v.active++
	v.maxSeen = max(v.maxSeen, v.active)
	hold, gate, result := v.hold, v.gates[job.ID], v.result
	v.mu.Unlock()
	defer func() {
		v.mu.Lock()
		v.active--
		v.mu.Unlock()
	}()
	v.started <- job.ID
	for _, ch := range []chan struct{}{hold, gate} {
		if ch == nil {
			continue
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return media.Stored{}, false, ctx.Err()
		}
	}
	if result != nil {
		return result(job, n)
	}
	return media.Stored{Hash: fmt.Sprintf("v-%d", job.ID), Path: fmt.Sprintf("%d/v.mp4", job.GameID), SourceURL: "https://media.example.test/v.mp4"}, true, nil
}

func (v *laneVideo) callsOf(id int64) int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.calls[id]
}

func (v *laneVideo) totalCalls() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.total
}

func (v *laneVideo) activeNow() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active
}

type laneRig struct {
	t     *testing.T
	st    *fakeQueueStore
	q     *mediaQueue
	clock *domain.FakeClock
	vid   *laneVideo
	img   *fakeFetcher
	sink  *spySink
	logs  *lockedBuf
	ready atomic.Bool

	qmu   sync.Mutex
	quota scrapers.SSQuota

	barriers int
	stopOnce sync.Once
	stopFn   func()
}

type laneOpt func(*laneRig)

func withSink() laneOpt { return func(r *laneRig) { r.sink = newSpySink(r.st) } }

// newLaneRig builds a queue with a fetchRouter over fakes; the video credentials are ready.
func newLaneRig(t *testing.T, jobs []store.MediaJob, opts ...laneOpt) *laneRig {
	t.Helper()
	r := &laneRig{t: t, st: newFakeQueueStore(jobs...), clock: domain.NewFakeClock(laneT0), vid: newLaneVideo(), img: newFetcher(), logs: &lockedBuf{}}
	r.img.started = make(chan string, 64)
	r.ready.Store(true)
	for _, o := range opts {
		o(r)
	}
	var sink mediaChangeSink
	if r.sink != nil {
		sink = r.sink
	}
	log := slog.New(slog.NewTextHandler(r.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r.q = newMediaQueue(r.st, fetchRouter{images: r.img, video: r.vid}, r.clock, log, sink)
	r.q.ConfigureVideo(r.ready.Load, func() scrapers.SSQuota {
		r.qmu.Lock()
		defer r.qmu.Unlock()
		return r.quota
	})
	t.Cleanup(func() { r.stop() })
	return r
}

func (r *laneRig) setQuota(q scrapers.SSQuota) {
	r.qmu.Lock()
	r.quota = q
	r.qmu.Unlock()
}

func (r *laneRig) start() {
	r.t.Helper()
	r.stopFn = runQueue(r.t, r.q)
}

func (r *laneRig) stop() {
	r.stopOnce.Do(func() {
		if r.stopFn != nil {
			r.stopFn()
		}
	})
}

// barrier queues an image job whose hash is a duplicate (no notice) and waits for it to finish. The loop
// lists the pending jobs in id order, so every earlier pending video job was looked at in the same pass.
func (r *laneRig) barrier() {
	r.t.Helper()
	r.barriers++
	url := fmt.Sprintf("https://cdn.example.test/barrier-%d.png", r.barriers)
	r.st.markDup("h-" + url)
	id := r.st.put(gameJob(0, 1, url))
	r.q.Notify()
	waitState(r.t, r.st, id, store.JobDone)
}

func (r *laneRig) waitLane(want string) time.Time {
	r.t.Helper()
	var until time.Time
	eventually(r.t, "lane "+want, func() bool {
		var got string
		got, until = r.q.VideoLaneStatus()
		return got == want
	})
	return until
}

func (r *laneRig) waitWaiters(n int) {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(r.t.Context(), 5*time.Second)
	defer cancel()
	if err := r.clock.BlockUntilWaiters(ctx, n); err != nil {
		r.t.Fatalf("clock waiters < %d: %v", n, err)
	}
}

// eventually polls cond (yielding, never sleeping) until it holds or five real seconds pass.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		runtime.Gosched()
	}
}

func (r *laneRig) job(id int64) store.MediaJob { return r.st.state(id) }

func waitStarted(t *testing.T, ch <-chan int64) int64 {
	t.Helper()
	select {
	case id := <-ch:
		return id
	case <-time.After(5 * time.Second):
		t.Fatal("no video download started within 5s")
		return 0
	}
}

func waitStartedURL(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case u := <-ch:
		return u
	case <-time.After(5 * time.Second):
		t.Fatal("no image download started within 5s")
		return ""
	}
}

// recFetcher records the kinds it was asked for.
type recFetcher struct {
	mu    sync.Mutex
	kinds []string
	name  string
}

func (f *recFetcher) Fetch(_ context.Context, job store.MediaJob) (media.Stored, bool, error) {
	f.mu.Lock()
	f.kinds = append(f.kinds, job.Kind)
	f.mu.Unlock()
	return media.Stored{Hash: f.name}, true, nil
}

func TestFetchRouter(t *testing.T) {
	t.Parallel()
	t.Run("VID-009/o roteador manda kind video ao resolvedor e o resto ao media.Store", func(t *testing.T) {
		t.Parallel()
		img, vid := &recFetcher{name: "img"}, &recFetcher{name: "vid"}
		r := fetchRouter{images: img, video: vid}
		for _, kind := range []string{"cover", "logo", "background", "screenshot", "video"} {
			got, _, err := r.Fetch(t.Context(), store.MediaJob{Kind: kind})
			if err != nil {
				t.Fatalf("Fetch(%s): %v", kind, err)
			}
			want := "img"
			if kind == "video" {
				want = "vid"
			}
			if got.Hash != want {
				t.Errorf("kind %s went to %q, want %q", kind, got.Hash, want)
			}
		}
		if len(vid.kinds) != 1 || len(img.kinds) != 4 {
			t.Errorf("video got %v, images got %v", vid.kinds, img.kinds)
		}
	})
}

func TestVideoLane(t *testing.T) {
	t.Parallel()

	t.Run("VID-009/100 jobs de vídeo pendentes: um download de vídeo por vez e um job de imagem novo começa na hora", func(t *testing.T) {
		t.Parallel()
		var jobs []store.MediaJob
		for i := int64(1); i <= 100; i++ {
			jobs = append(jobs, vJob(i, i))
		}
		jobs = append(jobs, gameJob(101, 1, "https://cdn.example.test/new.png"))
		r := newLaneRig(t, jobs)
		r.vid.hold = make(chan struct{}) // never opened: stop cancels the running one
		r.start()
		waitStarted(t, r.vid.started)
		waitState(t, r.st, 101, store.JobDone)
		if r.vid.maxSeen != 1 || r.vid.totalCalls() != 1 {
			t.Errorf("video downloads: max %d, total %d; want one at a time (1, 1)", r.vid.maxSeen, r.vid.totalCalls())
		}
	})

	t.Run("VID-009/as duas vagas das imagens continuam e não são ocupadas pelo vídeo", func(t *testing.T) {
		t.Parallel()
		urls := []string{"https://cdn.example.test/a.png", "https://cdn.example.test/b.png", "https://cdn.example.test/c.png"}
		jobs := []store.MediaJob{vJob(1, 1), gameJob(2, 1, urls[0]), gameJob(3, 1, urls[1]), gameJob(4, 1, urls[2])}
		r := newLaneRig(t, jobs)
		r.vid.hold = make(chan struct{})
		gate := make(chan struct{})
		r.img.gate = gate
		r.start()
		waitStarted(t, r.vid.started)
		waitStartedURL(t, r.img.started)
		waitStartedURL(t, r.img.started)
		if n := r.img.callsOf(urls[2]); n != 0 {
			t.Errorf("third image started with two slots busy: %d calls", n)
		}
		close(gate)
		waitState(t, r.st, 2, store.JobDone)
		waitState(t, r.st, 3, store.JobDone)
		waitState(t, r.st, 4, store.JobDone)
		r.img.mu.Lock()
		maxImg := r.img.maxSeen
		r.img.mu.Unlock()
		if maxImg != 2 {
			t.Errorf("image concurrency = %d, want 2", maxImg)
		}
	})

	t.Run("VID-009/raia pausada (cota, credencial, sem credencial) não atrasa as imagens", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			lane string
			prep func(r *laneRig)
		}{
			{"cota", "paused-quota", func(r *laneRig) {
				r.vid.result = func(store.MediaJob, int) (media.Stored, bool, error) { return media.Stored{}, false, scrapers.ErrQuota }
			}},
			{"credencial recusada", "paused-credentials", func(r *laneRig) {
				r.vid.result = func(store.MediaJob, int) (media.Stored, bool, error) {
					return media.Stored{}, false, fmt.Errorf("busca: %w", scrapers.ErrCredentials)
				}
			}},
			{"sem credencial", "no-credentials", func(r *laneRig) { r.ready.Store(false) }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				r := newLaneRig(t, []store.MediaJob{vJob(1, 1), vJob(2, 2)})
				tc.prep(r)
				r.start()
				r.waitLane(tc.lane)
				r.barrier() // an image newer than the paused video jobs runs and finishes
				if tc.lane != "no-credentials" && r.vid.callsOf(2) != 0 {
					t.Errorf("second video job ran during the pause")
				}
				eventually(t, "job 1 back to pending", func() bool { return r.job(1).State == store.JobPending })
				if r.job(1).State != store.JobPending || r.job(1).Attempts != 0 {
					t.Errorf("job 1 = %+v, want pending without spent attempt", r.job(1))
				}
			})
		}
	})

	t.Run("VID-014/o job que grava vira linha de media kind video com o source_url devolvido e sem miniatura", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 5)})
		r.start()
		waitState(t, r.st, 1, store.JobDone)
		rows := r.st.addedRows()
		if len(rows) != 1 {
			t.Fatalf("rows = %+v, want 1", rows)
		}
		row := rows[0]
		if row.Kind != "video" || row.Provider != "screenscraper" || row.GameID != 5 || row.ThumbPath != "" ||
			row.SourceURL != "https://media.example.test/v.mp4" || row.Hash != "v-1" {
			t.Errorf("row = %+v", row)
		}
		if r.job(1).Attempts != 1 {
			t.Errorf("attempts = %d, want 1", r.job(1).Attempts)
		}
	})

	t.Run("VID-003/sem Stored.SourceURL a linha usa o do job (nunca credenciais)", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 5)})
		r.vid.result = func(job store.MediaJob, _ int) (media.Stored, bool, error) {
			return media.Stored{Hash: "h", Path: "5/h.mp4"}, true, nil
		}
		r.start()
		waitState(t, r.st, 1, store.JobDone)
		if rows := r.st.addedRows(); len(rows) != 1 || rows[0].SourceURL != store.VideoLookupURL {
			t.Errorf("rows = %+v, want source_url %q", rows, store.VideoLookupURL)
		}
	})

	t.Run("VID-016/ErrQuota: job volta a pending sem gastar tentativa, Warn uma vez, paused-quota com pausedUntil e a raia tenta de novo depois de 1 h", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 1)})
		r.vid.result = func(job store.MediaJob, n int) (media.Stored, bool, error) {
			if job.ID == 1 && n == 1 {
				return media.Stored{}, false, fmt.Errorf("busca: %w", scrapers.ErrQuota)
			}
			return media.Stored{Hash: "v", Path: "1/v.mp4"}, true, nil
		}
		r.start()
		until := r.waitLane("paused-quota")
		if !until.Equal(laneT0.Add(time.Hour)) {
			t.Errorf("pausedUntil = %v, want %v", until, laneT0.Add(time.Hour))
		}
		eventually(t, "job 1 back to pending", func() bool { return r.job(1).State == store.JobPending && r.vid.callsOf(1) == 1 })
		if j := r.job(1); j.Attempts != 0 {
			t.Errorf("attempts = %d, want 0 (quota does not spend an attempt)", j.Attempts)
		}
		if n := r.logs.count("cota do ScreenScraper esgotada"); n != 1 {
			t.Errorf("Warn count = %d, want 1", n)
		}
		r.st.put(vJob(0, 2)) // another video job arrives during the pause
		r.q.Notify()
		r.barrier()
		r.waitWaiters(1)
		r.clock.Advance(30 * time.Minute)
		r.barrier()
		if r.vid.totalCalls() != 1 {
			t.Fatalf("video requests during the pause = %d, want only the first", r.vid.totalCalls())
		}
		r.clock.Advance(30 * time.Minute)
		waitState(t, r.st, 1, store.JobDone)
		waitState(t, r.st, 2, store.JobDone)
		r.waitLane("active")
		if n := r.logs.count("cota do ScreenScraper esgotada"); n != 1 {
			t.Errorf("Warn count after resuming = %d, want still 1", n)
		}
	})

	t.Run("VID-016/requestsToday >= maxRequestsPerDay pausa antes do próximo job", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 1), vJob(2, 2)})
		r.setQuota(scrapers.SSQuota{Known: true, RequestsToday: 100, MaxPerDay: 100})
		r.start()
		waitState(t, r.st, 1, store.JobDone)
		r.waitLane("paused-quota")
		r.barrier()
		if r.vid.callsOf(2) != 0 || r.job(2).State != store.JobPending || r.job(2).Attempts != 0 {
			t.Fatalf("job 2 = %+v (calls %d), want untouched during the preventive pause", r.job(2), r.vid.callsOf(2))
		}
		r.setQuota(scrapers.SSQuota{})
		r.waitWaiters(1)
		r.clock.Advance(time.Hour)
		waitState(t, r.st, 2, store.JobDone)
	})

	t.Run("VID-016/quota desconhecida ou abaixo do teto não pausa", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 1), vJob(2, 2)})
		r.setQuota(scrapers.SSQuota{Known: true, RequestsToday: 99, MaxPerDay: 100})
		r.start()
		waitState(t, r.st, 1, store.JobDone)
		waitState(t, r.st, 2, store.JobDone)
		if lane, _ := r.q.VideoLaneStatus(); lane != "active" {
			t.Errorf("lane = %q, want active", lane)
		}
	})

	t.Run("VID-017/ErrCredentials: pending sem gastar tentativa, pausa até VideoCredentialsChanged, Warn uma vez", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 1), vJob(2, 2)})
		r.vid.result = func(job store.MediaJob, n int) (media.Stored, bool, error) {
			if job.ID == 1 && n == 1 {
				return media.Stored{}, false, fmt.Errorf("busca: %w", scrapers.ErrCredentials)
			}
			return media.Stored{Hash: fmt.Sprintf("v-%d", job.ID), Path: "x.mp4"}, true, nil
		}
		r.start()
		r.waitLane("paused-credentials")
		eventually(t, "job 1 back to pending", func() bool { return r.job(1).State == store.JobPending && r.vid.callsOf(1) == 1 })
		if r.job(1).Attempts != 0 {
			t.Errorf("attempts = %d, want 0", r.job(1).Attempts)
		}
		r.clock.Advance(48 * time.Hour) // time alone never resumes it
		r.barrier()
		if r.vid.totalCalls() != 1 {
			t.Fatalf("video requests while paused = %d, want 1", r.vid.totalCalls())
		}
		r.q.VideoCredentialsChanged()
		waitState(t, r.st, 1, store.JobDone)
		waitState(t, r.st, 2, store.JobDone)
		r.waitLane("active")
		if n := r.logs.count("credenciais do ScreenScraper recusadas"); n != 1 {
			t.Errorf("Warn count = %d, want 1", n)
		}
	})

	t.Run("VID-018/rede, 429 e 5xx: nova tentativa em 30 s, até 3 tentativas, depois failed", func(t *testing.T) {
		t.Parallel()
		for name, boom := range map[string]error{
			"rede":    errors.New("rede fora"),
			"429/5xx": fmt.Errorf("%w: status 503", scrapers.ErrUnavailable),
		} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				r := newLaneRig(t, []store.MediaJob{vJob(1, 1)})
				r.vid.result = func(store.MediaJob, int) (media.Stored, bool, error) { return media.Stored{}, false, boom }
				r.start()
				eventually(t, "attempt 1", func() bool { j := r.job(1); return j.State == store.JobPending && j.Attempts == 1 })
				if lane, _ := r.q.VideoLaneStatus(); lane != "active" {
					t.Errorf("lane = %q, want active (transient errors do not pause)", lane)
				}
				r.waitWaiters(1)
				r.clock.Advance(29 * time.Second)
				r.barrier()
				if r.vid.callsOf(1) != 1 {
					t.Fatalf("retried before 30 s: %d calls", r.vid.callsOf(1))
				}
				r.clock.Advance(time.Second)
				eventually(t, "attempt 2", func() bool { j := r.job(1); return j.State == store.JobPending && j.Attempts == 2 })
				r.waitWaiters(1)
				r.clock.Advance(30 * time.Second)
				waitState(t, r.st, 1, store.JobFailed)
				if j := r.job(1); j.Attempts != 3 || j.LastError == "" {
					t.Errorf("job = %+v, want 3 attempts and the last error", j)
				}
			})
		}
	})

	t.Run("VID-018/*media.PermanentError vira failed na primeira tentativa com a mensagem", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 1)})
		r.vid.result = func(store.MediaJob, int) (media.Stored, bool, error) {
			return media.Stored{}, false, fmt.Errorf("resolver: %w", &media.PermanentError{Msg: "sem vídeo no ScreenScraper"})
		}
		r.start()
		waitState(t, r.st, 1, store.JobFailed)
		j := r.job(1)
		if j.Attempts != 1 || !containsStr(j.LastError, "sem vídeo no ScreenScraper") {
			t.Errorf("job = %+v, want 1 attempt and the message", j)
		}
		r.clock.Advance(time.Hour)
		r.barrier()
		if r.vid.callsOf(1) != 1 {
			t.Errorf("calls = %d, want 1 (permanent is not retried)", r.vid.callsOf(1))
		}
	})

	t.Run("VID-019/credencial ausente: nenhuma requisição, jobs pendentes sem gastar tentativa, no-credentials e a raia volta sozinha", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 1), vJob(2, 2)})
		r.ready.Store(false)
		r.start()
		r.waitLane("no-credentials")
		r.barrier()
		if r.vid.totalCalls() != 0 {
			t.Fatalf("video requests without credentials = %d, want 0", r.vid.totalCalls())
		}
		for _, id := range []int64{1, 2} {
			if j := r.job(id); j.State != store.JobPending || j.Attempts != 0 {
				t.Errorf("job %d = %+v, want pending, 0 attempts", id, j)
			}
		}
		r.ready.Store(true)
		r.q.VideoCredentialsChanged()
		waitState(t, r.st, 1, store.JobDone)
		waitState(t, r.st, 2, store.JobDone)
		r.waitLane("active")
	})

	t.Run("VID-019/ErrNoCredentials do resolvedor: pending sem gastar tentativa e volta com VideoCredentialsChanged", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 1)})
		r.vid.result = func(_ store.MediaJob, n int) (media.Stored, bool, error) {
			if n == 1 {
				return media.Stored{}, false, fmt.Errorf("x: %w", scrapers.ErrNoCredentials)
			}
			return media.Stored{Hash: "v", Path: "1/v.mp4"}, true, nil
		}
		r.start()
		r.waitLane("no-credentials")
		eventually(t, "job back to pending", func() bool { return r.job(1).State == store.JobPending && r.vid.callsOf(1) == 1 })
		if r.job(1).Attempts != 0 {
			t.Errorf("attempts = %d, want 0", r.job(1).Attempts)
		}
		r.barrier()
		if r.vid.totalCalls() != 1 {
			t.Fatalf("requests while no-credentials = %d, want 1", r.vid.totalCalls())
		}
		r.q.VideoCredentialsChanged()
		waitState(t, r.st, 1, store.JobDone)
	})

	t.Run("VID-021/job de vídeo que grava (added=true) envia media.changed uma vez", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 7)}, withSink())
		r.start()
		c := r.sink.next(t)
		if c.gameID != 7 || c.added != 1 {
			t.Errorf("notice = %+v, want game 7 after the row was written", c)
		}
		waitState(t, r.st, 1, store.JobDone)
		r.barrier()
		if n := r.sink.count(); n != 1 {
			t.Errorf("notices = %d, want 1", n)
		}
	})

	t.Run("VID-021/falha, pausa e conteúdo repetido não avisam", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 1), vJob(2, 2), vJob(3, 3)}, withSink())
		r.st.markDup("v-3")
		r.vid.result = func(job store.MediaJob, _ int) (media.Stored, bool, error) {
			switch job.ID {
			case 1:
				return media.Stored{}, false, &media.PermanentError{Msg: "sem vídeo no ScreenScraper"}
			case 2:
				return media.Stored{Hash: "v-3", Path: "3/v.mp4"}, true, nil // AddMedia reports added = false
			}
			return media.Stored{}, false, scrapers.ErrQuota
		}
		r.start()
		waitState(t, r.st, 1, store.JobFailed)
		waitState(t, r.st, 2, store.JobDone)
		r.waitLane("paused-quota")
		r.barrier()
		if n := r.sink.count(); n != 0 {
			t.Errorf("notices = %d, want 0", n)
		}
	})

	t.Run("VID-007/CancelVideo cancela os pendentes, interrompe o job em andamento (contexto) e devolve a soma; jobs de imagem não mudam", func(t *testing.T) {
		t.Parallel()
		urls := []string{"https://cdn.example.test/a.png", "https://cdn.example.test/b.png", "https://cdn.example.test/c.png"}
		jobs := []store.MediaJob{vJob(1, 1), vJob(2, 2), vJob(3, 3), gameJob(4, 1, urls[0]), gameJob(5, 1, urls[1]), gameJob(6, 1, urls[2])}
		r := newLaneRig(t, jobs, withSink())
		r.vid.hold = make(chan struct{}) // only ctx can end the running one
		gate := make(chan struct{})
		r.img.gate = gate
		r.start()
		waitStarted(t, r.vid.started)
		waitStartedURL(t, r.img.started)
		waitStartedURL(t, r.img.started)

		n, err := r.q.CancelVideo(t.Context())
		if err != nil || n != 3 {
			t.Fatalf("CancelVideo = %d, %v; want 3 (2 pending + 1 running)", n, err)
		}
		eventually(t, "the video download to leave", func() bool { return r.vid.activeNow() == 0 })
		for _, id := range []int64{1, 2, 3} {
			if j := r.job(id); j.State != store.JobFailed || j.LastError != "cancelado" {
				t.Errorf("video job %d = %+v, want failed/cancelado", id, j)
			}
		}
		for _, id := range []int64{4, 5} {
			if r.job(id).State != store.JobRunning {
				t.Errorf("image job %d = %+v, want still running", id, r.job(id))
			}
		}
		if r.job(6).State != store.JobPending {
			t.Errorf("image job 6 = %+v, want still pending", r.job(6))
		}
		close(gate)
		for _, id := range []int64{4, 5, 6} {
			waitState(t, r.st, id, store.JobDone)
		}
		if r.vid.totalCalls() != 1 {
			t.Errorf("video downloads = %d, want only the one interrupted", r.vid.totalCalls())
		}
	})

	t.Run("VID-021/cancelar o vídeo em andamento não avisa", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{vJob(1, 1)}, withSink())
		r.vid.hold = make(chan struct{})
		r.start()
		waitStarted(t, r.vid.started)
		if n, err := r.q.CancelVideo(t.Context()); err != nil || n != 1 {
			t.Fatalf("CancelVideo = %d, %v; want 1", n, err)
		}
		eventually(t, "download to leave", func() bool { return r.vid.activeNow() == 0 })
		r.barrier()
		if n := r.sink.count(); n != 0 {
			t.Errorf("notices = %d, want 0", n)
		}
	})

	t.Run("VID-007/CancelVideo sem jobs de vídeo devolve 0", func(t *testing.T) {
		t.Parallel()
		r := newLaneRig(t, []store.MediaJob{gameJob(1, 1, "https://cdn.example.test/a.png")})
		r.start()
		waitState(t, r.st, 1, store.JobDone)
		if n, err := r.q.CancelVideo(t.Context()); err != nil || n != 0 {
			t.Errorf("CancelVideo = %d, %v; want 0", n, err)
		}
	})

	t.Run("VID-008/VideoLaneStatus nos 4 estados", func(t *testing.T) {
		t.Parallel()

		t.Run("no-credentials sem nenhum job", func(t *testing.T) {
			t.Parallel()
			r := newLaneRig(t, nil)
			r.ready.Store(false)
			if lane, _ := r.q.VideoLaneStatus(); lane != "no-credentials" {
				t.Errorf("lane = %q, want no-credentials", lane)
			}
		})
		t.Run("active", func(t *testing.T) {
			t.Parallel()
			r := newLaneRig(t, nil)
			if lane, until := r.q.VideoLaneStatus(); lane != "active" || !until.IsZero() {
				t.Errorf("lane = %q until %v, want active and no deadline", lane, until)
			}
		})
		t.Run("paused-quota com o fim da pausa", func(t *testing.T) {
			t.Parallel()
			r := newLaneRig(t, []store.MediaJob{vJob(1, 1)})
			r.vid.result = func(store.MediaJob, int) (media.Stored, bool, error) { return media.Stored{}, false, scrapers.ErrQuota }
			r.start()
			if until := r.waitLane("paused-quota"); !until.Equal(laneT0.Add(time.Hour)) {
				t.Errorf("until = %v, want %v", until, laneT0.Add(time.Hour))
			}
		})
		t.Run("paused-credentials", func(t *testing.T) {
			t.Parallel()
			r := newLaneRig(t, []store.MediaJob{vJob(1, 1)})
			r.vid.result = func(store.MediaJob, int) (media.Stored, bool, error) {
				return media.Stored{}, false, scrapers.ErrCredentials
			}
			r.start()
			r.waitLane("paused-credentials")
		})
	})

	t.Run("VID-020/last_error e o log de um job falho com credenciais marcadas não as contêm", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close() // connection refused: *url.Error with the whole URL
		var logs lockedBuf
		log := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		clock := domain.NewFakeClock(laneT0)
		client := scrapers.NewScreenScraper(scrapers.SSConfig{
			BaseURL: srv.URL, HTTP: srv.Client(), Limiter: scrapers.NewLimiter(clock, 0), Log: log,
			Creds: func() (scrapers.SSCreds, error) {
				return scrapers.SSCreds{DevID: secDevID, DevPassword: secDevPass, User: secUser, Password: secPass}, nil
			},
		})
		st := newFakeQueueStore(vJob(1, 7))
		resolver := newVideoResolver(vidGames{info: map[int64]store.VideoGameInfo{7: {Title: "Super Metroid", SeedKey: "snes"}}}, client, &vidFiles{}, log)
		q := newMediaQueue(st, fetchRouter{images: newFetcher(), video: resolver}, clock, log, nil)
		q.ConfigureVideo(func() bool { return true }, resolver.Quota)
		stop := runQueue(t, q)
		defer stop()
		eventually(t, "first attempt recorded", func() bool { j := st.state(1); return j.State == store.JobPending && j.Attempts == 1 })
		assertNoSecrets(t, "last_error", st.state(1).LastError)
		assertNoSecrets(t, "log", logs.String())
	})
}

// TestVideoLaneNoLeak is not parallel: goleak counts every goroutine of the process.
func TestVideoLaneNoLeak(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	t.Run("VID-009/a raia não deixa goroutine depois de Run (-race)", func(t *testing.T) {
		jobs := []store.MediaJob{vJob(1, 1), vJob(2, 2), gameJob(3, 1, "https://cdn.example.test/a.png")}
		r := newLaneRig(t, jobs)
		r.vid.hold = make(chan struct{}) // the video download is still in flight at shutdown
		r.start()
		waitStarted(t, r.vid.started)
		waitState(t, r.st, 3, store.JobDone)
		r.stop() // Run returns only after the lane's goroutines are gone
		if r.vid.activeNow() != 0 {
			t.Errorf("video download still active after Run returned")
		}
		if j := r.job(1); j.State != store.JobPending || j.Attempts != 0 {
			t.Errorf("job 1 = %+v, want pending without spent attempt after shutdown", j)
		}
	})
}

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }
