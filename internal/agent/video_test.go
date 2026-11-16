package agent

// Tests of the video service (phase 13, T25): video.queue, video.queueMissing, video.cancelPending,
// video.status and the silent hook maybeQueue. The store is real (t.TempDir()), the video queue is a fake,
// the clock is domain.FakeClock. No network and no sleeping [CON-029].
// Alterado em 2026-11-15: arquivo novo (fase 13, T25).

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

var videoT0 = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

// videoQueueFake is the videoQueue of the tests: it counts Notify and answers lane and cancel from fields.
// When st is set, CancelVideo runs the real store cancellation, so the jobs change as in production.
type videoQueueFake struct {
	mu        sync.Mutex
	notifies  int
	lane      string
	until     time.Time
	cancelN   int
	cancelErr error
	st        *store.Store
}

func (q *videoQueueFake) Notify() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.notifies++
}

func (q *videoQueueFake) notified() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.notifies
}

func (q *videoQueueFake) CancelVideo(ctx context.Context) (int, error) {
	q.mu.Lock()
	st, n, err := q.st, q.cancelN, q.cancelErr
	q.mu.Unlock()
	if err != nil {
		return 0, err
	}
	if st != nil {
		pending, running, err := st.CancelVideoJobs(ctx)
		if err != nil {
			return 0, err
		}
		return pending + len(running), nil
	}
	return n, nil
}

func (q *videoQueueFake) VideoLaneStatus() (string, time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	lane := q.lane
	if lane == "" {
		lane = ipc.VideoLaneActive
	}
	return lane, q.until
}

// videoStoreSpy counts the reads and writes the service makes, to prove that validation comes first.
type videoStoreSpy struct {
	videoStore
	mu    sync.Mutex
	calls int
}

func (s *videoStoreSpy) hit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
}

func (s *videoStoreSpy) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *videoStoreSpy) VideoCandidates(ctx context.Context, ids []int64) ([]store.VideoCandidate, error) {
	s.hit()
	return s.videoStore.VideoCandidates(ctx, ids)
}

func (s *videoStoreSpy) VideoCandidatesIn(ctx context.Context, platformID int64) ([]store.VideoCandidate, error) {
	s.hit()
	return s.videoStore.VideoCandidatesIn(ctx, platformID)
}

func (s *videoStoreSpy) EnqueueVideoJobs(ctx context.Context, ids []int64, at time.Time) (int, error) {
	s.hit()
	return s.videoStore.EnqueueVideoJobs(ctx, ids, at)
}

func (s *videoStoreSpy) VideoGameInfo(ctx context.Context, id int64) (store.VideoGameInfo, error) {
	s.hit()
	return s.videoStore.VideoGameInfo(ctx, id)
}

func (s *videoStoreSpy) VideoJobCounts(ctx context.Context) (store.VideoCounts, error) {
	s.hit()
	return s.videoStore.VideoJobCounts(ctx)
}

// videoFixture is the video service over a real store.
type videoFixture struct {
	st       *store.Store
	ks       *memKeyStore
	keys     *scrapers.Keys
	q        *videoQueueFake
	svc      *videoService
	logs     *recordHandler
	clock    *domain.FakeClock
	handlers map[string]ipc.Handler
	quota    scrapers.SSQuota
	spy      *videoStoreSpy
}

func openVideoStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// newVideoService builds a videoService over st with the given credentials registered.
func newVideoService(t *testing.T, st *store.Store, dev, user bool) (*videoService, *videoQueueFake, *recordHandler, *memKeyStore) {
	t.Helper()
	ks := &memKeyStore{values: map[string]string{}}
	keys := scrapers.NewKeys(ks)
	if dev {
		if err := keys.SetPair("screenscraperdev", "devid-X", "devpass-X"); err != nil {
			t.Fatalf("SetPair dev: %v", err)
		}
	}
	if user {
		if err := keys.SetPair("screenscraper", "user-X", "pass-X"); err != nil {
			t.Fatalf("SetPair user: %v", err)
		}
	}
	q := &videoQueueFake{}
	logs := newRecordHandler()
	svc := &videoService{
		st: st, keys: keys, q: q,
		quota: func() scrapers.SSQuota { return scrapers.SSQuota{} },
		clock: domain.NewFakeClock(videoT0),
		log:   slog.New(logs),
	}
	return svc, q, logs, ks
}

func newVideoFixture(t *testing.T, dev, user bool) *videoFixture {
	t.Helper()
	st := openVideoStore(t)
	svc, q, logs, ks := newVideoService(t, st, dev, user)
	f := &videoFixture{st: st, ks: ks, keys: svc.keys, q: q, svc: svc, logs: logs, clock: svc.clock.(*domain.FakeClock)}
	f.spy = &videoStoreSpy{videoStore: st}
	svc.st = f.spy
	svc.quota = func() scrapers.SSQuota { return f.quota }
	f.handlers = videoHandlers(svc)
	return f
}

// videoGameOn adds a game on the platform named platform ("" keeps the default).
func videoGameOn(t *testing.T, st *store.Store, title, platform string) int64 {
	t.Helper()
	g, err := st.AddGame(t.Context(), store.NewGame{Title: title, Exe: `C:\` + title + `.exe`, At: videoT0})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	if platform != "" {
		setGamePlatform(t, st, g.ID, platform)
	}
	return g.ID
}

// setGamePlatform moves the game to the platform named name (created when missing) and returns its id.
func setGamePlatform(t *testing.T, st *store.Store, gameID int64, name string) int64 {
	t.Helper()
	p, _, err := st.AddPlatform(t.Context(), name, domain.CategoryConsole)
	if err != nil {
		t.Fatalf("AddPlatform(%q): %v", name, err)
	}
	if _, err := st.UpdateGame(t.Context(), gameID, store.GamePatch{PlatformID: &p.ID}, videoT0); err != nil {
		t.Fatalf("UpdateGame: %v", err)
	}
	return p.ID
}

const (
	platMapped   = "nintendo 64" // seed key n64: mapped in the ScreenScraper
	platUnmapped = "Meu Console" // custom platform: no seed key
)

// addVideoRow gives the game a video row.
func addVideoRow(t *testing.T, st *store.Store, gameID int64) {
	t.Helper()
	if _, err := st.AddMedia(t.Context(), store.MediaRow{GameID: gameID, Kind: "video", Provider: store.VideoProvider,
		SourceURL: "u", Hash: "h", Path: "1/h.mp4", CreatedAt: videoT0}); err != nil {
		t.Fatalf("AddMedia: %v", err)
	}
}

// videoJobsOfGame are the pending video jobs of a game.
func videoJobsOfGame(t *testing.T, st *store.Store, gameID int64) []store.MediaJob {
	t.Helper()
	var out []store.MediaJob
	for _, j := range pendingJobs(t, st) {
		if j.GameID == gameID && j.Kind == "video" {
			out = append(out, j)
		}
	}
	return out
}

// callVideo runs a handler with JSON params and decodes the result through JSON (what the wire sees).
func (f *videoFixture) callVideo(t *testing.T, method, params string, out any) error {
	t.Helper()
	h, ok := f.handlers[method]
	if !ok {
		t.Fatalf("no handler for %s", method)
	}
	res, err := h(t.Context(), json.RawMessage(params))
	if err != nil {
		return err
	}
	raw, mErr := json.Marshal(res)
	if mErr != nil {
		t.Fatalf("marshal result: %v", mErr)
	}
	if out != nil {
		if uErr := json.Unmarshal(raw, out); uErr != nil {
			t.Fatalf("unmarshal %s: %v", raw, uErr)
		}
	}
	return nil
}

func (f *videoFixture) queue(t *testing.T, ids ...int64) (ipc.VideoQueueResult, error) {
	t.Helper()
	raw, _ := json.Marshal(ipc.VideoQueueParams{GameIDs: ids})
	var res ipc.VideoQueueResult
	err := f.callVideo(t, ipc.MethodVideoQueue, string(raw), &res)
	return res, err
}

// providerOf is data.provider of an RPC error.
func providerOf(t *testing.T, err error) string {
	t.Helper()
	var rpc *ipc.Error
	if !errors.As(err, &rpc) {
		t.Fatalf("error %v is not an *ipc.Error", err)
	}
	var data struct {
		Provider string `json:"provider"`
	}
	if jErr := json.Unmarshal(rpc.Data, &data); jErr != nil {
		t.Fatalf("error data %s: %v", rpc.Data, jErr)
	}
	return data.Provider
}

// debugRecords is how many Debug records the handler holds.
func debugRecords(h *recordHandler) int { return h.count(slog.LevelDebug) }

func TestVideoService(t *testing.T) {
	t.Parallel()

	t.Run("VID-005/queue cria um job por jogo e lista has-video e platform-unmapped em skipped (lista nunca nula)", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		a := videoGameOn(t, f.st, "A", platMapped)
		b := videoGameOn(t, f.st, "B", platMapped)
		withVideo := videoGameOn(t, f.st, "C", platMapped)
		addVideoRow(t, f.st, withVideo)
		unmapped := videoGameOn(t, f.st, "D", platUnmapped)

		res, err := f.queue(t, a, b, withVideo, unmapped)
		if err != nil {
			t.Fatalf("video.queue: %v", err)
		}
		if res.Queued != 2 {
			t.Errorf("queued = %d, want 2", res.Queued)
		}
		want := []ipc.VideoSkip{{GameID: withVideo, Reason: ipc.VideoSkipHasVideo}, {GameID: unmapped, Reason: ipc.VideoSkipPlatformUnmapped}}
		got := slices.Clone(res.Skipped)
		slices.SortFunc(got, func(x, y ipc.VideoSkip) int { return int(x.GameID - y.GameID) })
		if !slices.Equal(got, want) {
			t.Errorf("skipped = %+v, want %+v", got, want)
		}
		for _, id := range []int64{a, b} {
			if n := len(videoJobsOfGame(t, f.st, id)); n != 1 {
				t.Errorf("game %d has %d video jobs, want 1", id, n)
			}
		}
		for _, id := range []int64{withVideo, unmapped} {
			if n := len(videoJobsOfGame(t, f.st, id)); n != 0 {
				t.Errorf("skipped game %d has %d video jobs, want 0", id, n)
			}
		}
		if f.q.notified() == 0 {
			t.Error("the queue was not notified after jobs were created")
		}

		// every game queued: skipped is [] on the wire, never null
		f2 := newVideoFixture(t, true, true)
		only := videoGameOn(t, f2.st, "Solo", platMapped)
		h := f2.handlers[ipc.MethodVideoQueue]
		raw, _ := json.Marshal(ipc.VideoQueueParams{GameIDs: []int64{only}})
		out, err := h(t.Context(), raw)
		if err != nil {
			t.Fatalf("video.queue: %v", err)
		}
		wire, _ := json.Marshal(out)
		if !strings.Contains(string(wire), `"skipped":[]`) {
			t.Errorf("wire = %s, want skipped as an empty list", wire)
		}
	})

	t.Run("VID-005/id inexistente: -32001 e nada criado", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		a := videoGameOn(t, f.st, "A", platMapped)
		_, err := f.queue(t, a, 99999)
		if codeOf(err) != ipc.CodeGameNotFound {
			t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeGameNotFound)
		}
		if n := len(videoJobsOfGame(t, f.st, a)); n != 0 {
			t.Errorf("jobs created = %d, want 0", n)
		}
		if f.q.notified() != 0 {
			t.Error("queue notified without any job")
		}
	})

	t.Run("VID-005/falta devid ou devpassword: -32010 com data.provider screenscraperdev; só falta a conta do usuário: screenscraper; nada criado", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name      string
			dev, user bool
			want      string
		}{
			{"VID-005/nenhuma credencial", false, false, "screenscraperdev"},
			{"VID-005/só a do desenvolvedor falta", false, true, "screenscraperdev"},
			{"VID-005/só a conta do usuário falta", true, false, "screenscraper"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newVideoFixture(t, tc.dev, tc.user)
				a := videoGameOn(t, f.st, "A", platMapped)
				_, err := f.queue(t, a)
				if codeOf(err) != ipc.CodeProviderNoKey {
					t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderNoKey)
				}
				if got := providerOf(t, err); got != tc.want {
					t.Errorf("data.provider = %q, want %q", got, tc.want)
				}
				if n := len(videoJobsOfGame(t, f.st, a)); n != 0 {
					t.Errorf("jobs created = %d, want 0", n)
				}
			})
		}
	})

	t.Run("VID-005/lista vazia, mais de 1.000, repetida ou id < 1: -32602 antes de qualquer leitura", func(t *testing.T) {
		t.Parallel()
		if maxVideoQueueGames != 1000 {
			t.Errorf("maxVideoQueueGames = %d, want 1000", maxVideoQueueGames)
		}
		tooMany := make([]int64, maxVideoQueueGames+1)
		for i := range tooMany {
			tooMany[i] = int64(i + 1)
		}
		cases := []struct {
			name string
			ids  []int64
		}{
			{"VID-005/lista vazia", []int64{}},
			{"VID-005/1.001 ids", tooMany},
			{"VID-005/id repetido", []int64{1, 2, 1}},
			{"VID-005/id zero", []int64{1, 0}},
			{"VID-005/id negativo", []int64{-3}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newVideoFixture(t, true, true)
				_, err := f.queue(t, tc.ids...)
				if codeOf(err) != ipc.CodeInvalidParams {
					t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeInvalidParams)
				}
				if n := f.spy.total(); n != 0 {
					t.Errorf("store calls = %d, want 0 before validation", n)
				}
			})
		}
		t.Run("VID-005/lista ausente ou de tipo errado", func(t *testing.T) {
			t.Parallel()
			f := newVideoFixture(t, true, true)
			for _, p := range []string{`{}`, `{"gameIds":"1"}`, `[]`} {
				if err := f.callVideo(t, ipc.MethodVideoQueue, p, nil); codeOf(err) != ipc.CodeInvalidParams {
					t.Errorf("params %s: code = %d (%v), want %d", p, codeOf(err), err, ipc.CodeInvalidParams)
				}
			}
		})
	})

	t.Run("VID-005/credenciais conferidas depois de validar e antes de escrever", func(t *testing.T) {
		t.Parallel()
		// params invalid + no credentials: -32602 wins
		f := newVideoFixture(t, false, false)
		if _, err := f.queue(t); codeOf(err) != ipc.CodeInvalidParams {
			t.Errorf("invalid params without credentials: code = %d, want %d", codeOf(err), ipc.CodeInvalidParams)
		}
		// unknown game + no credentials: -32001 wins (games before credentials)
		if _, err := f.queue(t, 99999); codeOf(err) != ipc.CodeGameNotFound {
			t.Errorf("unknown game without credentials: code = %d, want %d", codeOf(err), ipc.CodeGameNotFound)
		}
		// valid + no credentials: -32010 and nothing written
		a := videoGameOn(t, f.st, "A", platMapped)
		if _, err := f.queue(t, a); codeOf(err) != ipc.CodeProviderNoKey {
			t.Errorf("valid without credentials: code = %d, want %d", codeOf(err), ipc.CodeProviderNoKey)
		}
		if n := len(videoJobsOfGame(t, f.st, a)); n != 0 {
			t.Errorf("jobs created = %d, want 0", n)
		}
	})

	t.Run("VID-006/queueMissing cria jobs para todos os jogos da plataforma (ou da biblioteca) sem vídeo, com plataforma mapeada; skipped é inteiro", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		a := videoGameOn(t, f.st, "A", platMapped)
		withVideo := videoGameOn(t, f.st, "B", platMapped)
		addVideoRow(t, f.st, withVideo)
		active := videoGameOn(t, f.st, "C", platMapped)
		if n, err := f.st.EnqueueVideoJobs(t.Context(), []int64{active}, videoT0); err != nil || n != 1 {
			t.Fatalf("EnqueueVideoJobs = %d, %v", n, err)
		}
		unmapped := videoGameOn(t, f.st, "D", platUnmapped)
		n64ID := setGamePlatform(t, f.st, a, platMapped)

		var byPlatform ipc.VideoQueueMissingResult
		raw, _ := json.Marshal(ipc.VideoQueueMissingParams{PlatformID: n64ID})
		if err := f.callVideo(t, ipc.MethodVideoQueueMissing, string(raw), &byPlatform); err != nil {
			t.Fatalf("video.queueMissing platform: %v", err)
		}
		if byPlatform.Queued != 1 || byPlatform.Skipped != 2 {
			t.Errorf("platform result = %+v, want queued 1, skipped 2", byPlatform)
		}
		if n := len(videoJobsOfGame(t, f.st, a)); n != 1 {
			t.Errorf("game A jobs = %d, want 1", n)
		}

		// whole library: A is already active now; only the unmapped one is left out, and nothing new queues
		f2 := newVideoFixture(t, true, true)
		x := videoGameOn(t, f2.st, "X", platMapped)
		y := videoGameOn(t, f2.st, "Y", platMapped)
		_ = videoGameOn(t, f2.st, "Z", platUnmapped)
		var lib ipc.VideoQueueMissingResult
		if err := f2.callVideo(t, ipc.MethodVideoQueueMissing, `{}`, &lib); err != nil {
			t.Fatalf("video.queueMissing library: %v", err)
		}
		if lib.Queued != 2 {
			t.Errorf("library queued = %d, want 2", lib.Queued)
		}
		for _, id := range []int64{x, y} {
			if n := len(videoJobsOfGame(t, f2.st, id)); n != 1 {
				t.Errorf("game %d jobs = %d, want 1", id, n)
			}
		}
		_ = unmapped
		wire, _ := json.Marshal(lib)
		if !strings.Contains(string(wire), `"skipped":`) || strings.Contains(string(wire), `"skipped":[`) {
			t.Errorf("wire = %s, want skipped as a number", wire)
		}
	})

	t.Run("VID-006/plataforma inexistente: -32001", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		err := f.callVideo(t, ipc.MethodVideoQueueMissing, `{"platformId":99999}`, nil)
		if codeOf(err) != ipc.CodeGameNotFound {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeGameNotFound)
		}
	})

	t.Run("VID-006/credencial ausente: -32010", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name      string
			dev, user bool
			want      string
		}{
			{"VID-006/falta a do desenvolvedor", false, true, "screenscraperdev"},
			{"VID-006/falta a conta do usuário", true, false, "screenscraper"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newVideoFixture(t, tc.dev, tc.user)
				a := videoGameOn(t, f.st, "A", platMapped)
				err := f.callVideo(t, ipc.MethodVideoQueueMissing, `{}`, nil)
				if codeOf(err) != ipc.CodeProviderNoKey || providerOf(t, err) != tc.want {
					t.Fatalf("err = %v, want -32010 with provider %s", err, tc.want)
				}
				if n := len(videoJobsOfGame(t, f.st, a)); n != 0 {
					t.Errorf("jobs created = %d, want 0", n)
				}
			})
		}
	})

	t.Run("VID-007/cancelPending cancela pendentes e o em andamento e devolve a soma; jobs de imagem não mudam", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		f.q.st = f.st
		a := videoGameOn(t, f.st, "A", platMapped)
		b := videoGameOn(t, f.st, "B", platMapped)
		c := videoGameOn(t, f.st, "C", platMapped)
		if _, err := f.st.EnqueueVideoJobs(t.Context(), []int64{a, b, c}, videoT0); err != nil {
			t.Fatalf("EnqueueVideoJobs: %v", err)
		}
		if _, err := f.st.EnqueueMedia(t.Context(), a, "cover", "steamgriddb", "https://cdn.example.test/a.png", videoT0); err != nil {
			t.Fatalf("EnqueueMedia: %v", err)
		}
		running := videoJobsOfGame(t, f.st, c)[0]
		if err := f.st.SetMediaJobState(t.Context(), running.ID, store.JobRunning, 0, ""); err != nil {
			t.Fatalf("SetMediaJobState: %v", err)
		}

		var res ipc.VideoCancelResult
		if err := f.callVideo(t, ipc.MethodVideoCancelPending, `{}`, &res); err != nil {
			t.Fatalf("video.cancelPending: %v", err)
		}
		if res.Cancelled != 3 {
			t.Errorf("cancelled = %d, want 3 (2 pending + 1 running)", res.Cancelled)
		}
		for _, j := range pendingJobs(t, f.st) {
			if j.Kind == "video" {
				t.Errorf("video job %d is still pending", j.ID)
			}
		}
		images := 0
		for _, j := range pendingJobs(t, f.st) {
			if j.Kind == "cover" {
				images++
			}
		}
		if images != 1 {
			t.Errorf("pending image jobs = %d, want 1", images)
		}
	})

	t.Run("VID-007/cancelPending devolve o erro da fila como -32603", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		f.q.cancelErr = errors.New("disco cheio")
		err := f.callVideo(t, ipc.MethodVideoCancelPending, `{}`, nil)
		if codeOf(err) != ipc.CodeInternalError {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeInternalError)
		}
	})

	t.Run("VID-008/status traz pending, running, failed, lane, pausedUntil só em paused-quota e a cota só quando conhecida", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		a := videoGameOn(t, f.st, "A", platMapped)
		b := videoGameOn(t, f.st, "B", platMapped)
		c := videoGameOn(t, f.st, "C", platMapped)
		if _, err := f.st.EnqueueVideoJobs(t.Context(), []int64{a, b, c}, videoT0); err != nil {
			t.Fatalf("EnqueueVideoJobs: %v", err)
		}
		if err := f.st.SetMediaJobState(t.Context(), videoJobsOfGame(t, f.st, b)[0].ID, store.JobRunning, 0, ""); err != nil {
			t.Fatal(err)
		}
		if err := f.st.SetMediaJobState(t.Context(), videoJobsOfGame(t, f.st, c)[0].ID, store.JobFailed, 3, "boom"); err != nil {
			t.Fatal(err)
		}
		status := func() ipc.VideoStatus {
			var res ipc.VideoStatusResult
			if err := f.callVideo(t, ipc.MethodVideoStatus, `{}`, &res); err != nil {
				t.Fatalf("video.status: %v", err)
			}
			return res.Status
		}

		// active, quota unknown
		s := status()
		if s.Pending != 1 || s.Running != 1 || s.Failed != 1 || s.Lane != ipc.VideoLaneActive {
			t.Errorf("status = %+v, want pending 1, running 1, failed 1, lane active", s)
		}
		if s.PausedUntil != nil || s.RequestsToday != nil || s.MaxRequestsPerDay != nil {
			t.Errorf("status = %+v, want no pausedUntil and no quota", s)
		}

		// paused by quota, quota known
		until := videoT0.Add(time.Hour)
		f.q.lane, f.q.until = ipc.VideoLanePausedQuota, until
		f.quota = scrapers.SSQuota{Known: true, RequestsToday: 120, MaxPerDay: 20000}
		s = status()
		if s.Lane != ipc.VideoLanePausedQuota || s.PausedUntil == nil || !s.PausedUntil.Equal(until) {
			t.Errorf("paused-quota status = %+v, want pausedUntil %v", s, until)
		}
		if s.RequestsToday == nil || *s.RequestsToday != 120 || s.MaxRequestsPerDay == nil || *s.MaxRequestsPerDay != 20000 {
			t.Errorf("quota = %+v, want 120 of 20000", s)
		}

		// paused-credentials: no pausedUntil even if the fake still has a time
		f.q.lane = ipc.VideoLanePausedCredentials
		s = status()
		if s.Lane != ipc.VideoLanePausedCredentials || s.PausedUntil != nil {
			t.Errorf("paused-credentials status = %+v, want no pausedUntil", s)
		}
	})

	t.Run("VID-008/no-credentials é calculado na hora, sem depender da fila", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, false) // the queue fake still says active
		var res ipc.VideoStatusResult
		if err := f.callVideo(t, ipc.MethodVideoStatus, `{}`, &res); err != nil {
			t.Fatalf("video.status: %v", err)
		}
		if res.Status.Lane != ipc.VideoLaneNoCredentials {
			t.Errorf("lane = %q, want %q", res.Status.Lane, ipc.VideoLaneNoCredentials)
		}
	})

	t.Run("VID-002/jogo com vídeo ou com job pendente ou em andamento não ganha job novo e nada é apagado", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		withVideo := videoGameOn(t, f.st, "A", platMapped)
		addVideoRow(t, f.st, withVideo)
		pending := videoGameOn(t, f.st, "B", platMapped)
		runningG := videoGameOn(t, f.st, "C", platMapped)
		if _, err := f.st.EnqueueVideoJobs(t.Context(), []int64{pending, runningG}, videoT0); err != nil {
			t.Fatal(err)
		}
		if err := f.st.SetMediaJobState(t.Context(), videoJobsOfGame(t, f.st, runningG)[0].ID, store.JobRunning, 0, ""); err != nil {
			t.Fatal(err)
		}

		res, err := f.queue(t, withVideo, pending, runningG)
		if err != nil {
			t.Fatalf("video.queue: %v", err)
		}
		if res.Queued != 0 || len(res.Skipped) != 3 {
			t.Errorf("result = %+v, want nothing queued and 3 skipped", res)
		}
		for _, sk := range res.Skipped {
			if sk.Reason != ipc.VideoSkipHasVideo {
				t.Errorf("skip %+v, want reason %s", sk, ipc.VideoSkipHasVideo)
			}
		}
		if n := len(videoJobsOfGame(t, f.st, pending)); n != 1 {
			t.Errorf("pending game has %d pending video jobs, want still 1", n)
		}
		rows, err := f.st.ListMedia(t.Context(), withVideo)
		if err != nil || len(rows) != 1 {
			t.Errorf("media of the game with video = %d rows, %v; want 1 (nothing deleted)", len(rows), err)
		}
		if f.q.notified() != 0 {
			t.Error("queue notified although nothing was queued")
		}
	})

	t.Run("VID-003/o job tem source_url screenscraper:lookup, sem título nem credencial, e o jogo é lido na hora do download", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		id := videoGameOn(t, f.st, "Celeste", platMapped)
		if _, err := f.queue(t, id); err != nil {
			t.Fatalf("video.queue: %v", err)
		}
		jobs := videoJobsOfGame(t, f.st, id)
		if len(jobs) != 1 {
			t.Fatalf("jobs = %d, want 1", len(jobs))
		}
		j := jobs[0]
		if j.Kind != "video" || j.Provider != "screenscraper" || j.SourceURL != "screenscraper:lookup" {
			t.Errorf("job = %+v, want kind video, provider screenscraper, source_url screenscraper:lookup", j)
		}
		for _, secret := range []string{"Celeste", "devid-X", "devpass-X", "user-X", "pass-X"} {
			if strings.Contains(j.SourceURL+j.LastError+j.Provider, secret) {
				t.Errorf("job carries %q", secret)
			}
		}
		// renaming the game later does not touch the job: the title is read when the download starts
		newTitle := "Celeste 2"
		if _, err := f.st.UpdateGame(t.Context(), id, store.GamePatch{Title: &newTitle}, videoT0); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		if got := videoJobsOfGame(t, f.st, id)[0]; got.SourceURL != "screenscraper:lookup" {
			t.Errorf("job after rename = %+v", got)
		}
		info, err := f.st.VideoGameInfo(t.Context(), id)
		if err != nil || info.Title != "Celeste 2" {
			t.Errorf("VideoGameInfo = %+v, %v; want the current title", info, err)
		}
	})

	t.Run("VID-004/maybeQueue cria o job quando tudo cabe, notifica a fila e não registra erro", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		id := videoGameOn(t, f.st, "A", platMapped)
		f.svc.maybeQueue(t.Context(), id)
		if n := len(videoJobsOfGame(t, f.st, id)); n != 1 {
			t.Errorf("jobs = %d, want 1", n)
		}
		if f.q.notified() == 0 {
			t.Error("queue not notified")
		}
		if f.logs.count(slog.LevelError)+f.logs.count(slog.LevelWarn) != 0 {
			t.Error("maybeQueue logged a warning or an error")
		}
	})

	t.Run("VID-004/cada condição que falta (devid, conta, plataforma sem sistema, já tem vídeo) não cria e registra Debug", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name      string
			dev, user bool
			platform  string
			hasVideo  bool
		}{
			{"VID-004/falta devid e devpassword", false, true, platMapped, false},
			{"VID-004/falta a conta do usuário", true, false, platMapped, false},
			{"VID-004/plataforma sem sistema", true, true, platUnmapped, false},
			{"VID-004/já tem vídeo", true, true, platMapped, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				f := newVideoFixture(t, tc.dev, tc.user)
				id := videoGameOn(t, f.st, "A", tc.platform)
				if tc.hasVideo {
					addVideoRow(t, f.st, id)
				}
				f.svc.maybeQueue(t.Context(), id)
				if n := len(videoJobsOfGame(t, f.st, id)); n != 0 {
					t.Errorf("jobs = %d, want 0", n)
				}
				if f.q.notified() != 0 {
					t.Error("queue notified without a job")
				}
				if f.logs.count(slog.LevelDebug) == 0 {
					t.Error("no Debug record naming the missing condition")
				}
				if f.logs.count(slog.LevelError)+f.logs.count(slog.LevelWarn)+f.logs.count(slog.LevelInfo) != 0 {
					t.Error("maybeQueue is silent above Debug")
				}
			})
		}
	})

	t.Run("VID-004/maybeQueue de jogo inexistente é silencioso e não cria nada", func(t *testing.T) {
		t.Parallel()
		f := newVideoFixture(t, true, true)
		f.svc.maybeQueue(t.Context(), 99999)
		if f.q.notified() != 0 || f.logs.count(slog.LevelError) != 0 {
			t.Error("unknown game: queue notified or error logged")
		}
	})

	t.Run("VID-004/missingCredential: screenscraperdev primeiro, depois screenscraper", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			dev, user bool
			want      string
			missing   bool
		}{
			{false, false, "screenscraperdev", true},
			{true, false, "screenscraper", true},
			{false, true, "screenscraperdev", true},
			{true, true, "", false},
		}
		for _, tc := range cases {
			f := newVideoFixture(t, tc.dev, tc.user)
			got, missing := f.svc.missingCredential()
			if missing != tc.missing || (missing && got != tc.want) {
				t.Errorf("dev=%v user=%v: missingCredential = %q, %v; want %q, %v", tc.dev, tc.user, got, missing, tc.want, tc.missing)
			}
		}
	})

	t.Run("VID-025/CancelGame do jogo interrompe o job de vídeo em andamento antes de apagar a pasta (fetcher bloqueado), e <mediaRoot>/<G> some com o .mp4", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "media")
		med := &lateMedia{}
		h, _, _ := newMediaDeleteHarness(t, med, &callLog{})
		files := media.New(root, nil)
		fetch := &ctxFetcher{root: root, gate: make(chan struct{}), started: make(chan struct{}, 4)}
		q := newMediaQueue(h.st, fetch, h.clock, slog.New(h.logs), nil)
		med.q, med.files = q, files
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		dmWrite(t, root, g.ID, "old.mp4")
		if n, err := h.st.EnqueueVideoJobs(t.Context(), []int64{g.ID}, gamesT0); err != nil || n != 1 {
			t.Fatalf("EnqueueVideoJobs = %d, %v", n, err)
		}
		stop := runQueue(t, q)
		defer stop()
		select {
		case <-fetch.started:
		case <-time.After(testTimeout):
			t.Fatal("the video job never started")
		}

		deleteOK(t, h, g.ID)
		wantGameRemoved(t, h, g.ID)
		if dmExists(dmGameDir(root, g.ID)) {
			t.Error("folder with the .mp4 still exists right after game.delete")
		}
		close(fetch.gate) // the cancelled download must not bring the folder back
		if dmExists(dmGameDir(root, g.ID)) {
			t.Error("folder of the removed game came back after the video download was released")
		}
	})

	t.Run("CON-029/nenhum teste da fase usa Sleep ou rede", func(t *testing.T) {
		t.Parallel()
		sleep := "time" + ".Sleep("
		net := "http" + "://"
		for _, name := range []string{"video_test.go", "metadata_apply_test.go", "metadata_batch_apply_test.go"} {
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			src := string(raw)
			if strings.Contains(src, sleep) {
				t.Errorf("%s uses a real sleep", name)
			}
			for _, line := range strings.Split(src, "\n") {
				if strings.Contains(line, net) && !strings.Contains(line, "example.test") && !strings.Contains(line, net+`"`) {
					t.Errorf("%s has an external URL: %s", name, strings.TrimSpace(line))
				}
			}
		}
	})
}
