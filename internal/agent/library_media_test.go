package agent

// Tests of the media of a removed game through game.delete [DEL-007, DEL-008, DEL-011]. The database and the
// handler are the real ones of the games harness; the media is a real media.Store under t.TempDir() and a
// real mediaQueue, except where a test needs a recording or failing fake (mediaRecorder). The handler gets
// the media through gameHandlersWithMedia (assumed signature, see the report of the test-writer).

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// callLog records the steps of game.delete in order: "db.ok" after a successful DeleteGame, and the media
// calls "media.cancel" and "media.remove".
type callLog struct {
	mu  sync.Mutex
	ops []string
}

func (l *callLog) add(op string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ops = append(l.ops, op)
}

func (l *callLog) list() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.ops...)
}

// orderStore is the game store of these tests: DeleteGame returns fail when it is set (and writes nothing),
// otherwise it deletes and records "db.ok".
type orderStore struct {
	gameStore
	log  *callLog
	fail error
}

func (s *orderStore) DeleteGame(ctx context.Context, id int64) error {
	if s.fail != nil {
		return s.fail
	}
	if err := s.gameStore.DeleteGame(ctx, id); err != nil {
		return err
	}
	s.log.add("db.ok")
	return nil
}

// mediaRecorder is a gameMedia that records its calls in log and returns the errors set.
type mediaRecorder struct {
	log       *callLog
	cancelErr error
	removeErr error
}

func (m *mediaRecorder) CancelGame(_ context.Context, _ int64) error {
	m.log.add("media.cancel")
	return m.cancelErr
}

func (m *mediaRecorder) RemoveGame(_ int64) error {
	m.log.add("media.remove")
	return m.removeErr
}

// lateMedia is the production gameMedia: the queue cancels the downloads and the store removes the folder.
// The test fills it after the harness exists, because the queue needs the harness store.
type lateMedia struct {
	q     *mediaQueue
	files *media.Store
}

func (m *lateMedia) CancelGame(ctx context.Context, gameID int64) error {
	return m.q.CancelGame(ctx, gameID)
}

func (m *lateMedia) RemoveGame(gameID int64) error {
	return m.files.RemoveGame(gameID)
}

// ctxFetcher is the fetcher of the handler test: a download waits for its gate or its ctx. Only when the gate
// opens first it writes <root>/<game>/late.jpg.
type ctxFetcher struct {
	root    string
	gate    chan struct{}
	started chan struct{}
}

// alterado em 2026-11-15 (fase 13): o mediaFetcher recebe o job.
func (f *ctxFetcher) Fetch(ctx context.Context, job store.MediaJob) (media.Stored, bool, error) {
	gameID := job.GameID
	f.started <- struct{}{}
	select {
	case <-f.gate:
	case <-ctx.Done():
		return media.Stored{}, false, ctx.Err()
	}
	dir := filepath.Join(f.root, strconv.FormatInt(gameID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return media.Stored{}, false, err
	}
	if err := os.WriteFile(filepath.Join(dir, "late.jpg"), []byte("x"), 0o644); err != nil {
		return media.Stored{}, false, err
	}
	return media.Stored{Hash: "late", Path: "late.jpg", ThumbPath: "thumb_late.jpg"}, true, nil
}

// newMediaDeleteHarness is newGamesHarness whose game.delete goes through a deleteTracker, an orderStore
// recording into log, and med as the media of the game.
func newMediaDeleteHarness(t *testing.T, med gameMedia, log *callLog) (*gamesHarness, *deleteTracker, *orderStore) {
	t.Helper()
	h := newGamesHarness(t, nil)
	dt := &deleteTracker{fakeTracker: h.tr, active: map[int64]domain.SessionSnapshot{}}
	ost := &orderStore{gameStore: h.gs, log: log}
	h.handlers = gameHandlersWithMedia(ost, dt, h.clock, os.Stat, slog.New(h.logs), med)
	h.need(t, ipc.MethodGameDelete)
	return h, dt, ost
}

// wireMedia gives med a queue that is not running (and the real store of the harness) and a media.Store under
// root. It returns the store, so a test can read the folders.
func wireMedia(t *testing.T, h *gamesHarness, med *lateMedia, root string) *media.Store {
	t.Helper()
	files := media.New(root, nil)
	med.q = newMediaQueue(h.st, imageFetcher{files: files}, h.clock, slog.New(h.logs), nil)
	med.files = files
	return files
}

// dmGameDir is the folder of gameID under root.
func dmGameDir(root string, gameID int64) string {
	return filepath.Join(root, strconv.FormatInt(gameID, 10))
}

// dmWrite writes <root>/<gameID>/<rel> with some content.
func dmWrite(t *testing.T, root string, gameID int64, rel string) {
	t.Helper()
	p := filepath.Join(dmGameDir(root, gameID), rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte("img"), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// dmExists reports whether p exists; a link is looked at, not followed.
func dmExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// dmLoggedWarn reports whether a record of level Warn or more was logged.
func dmLoggedWarn(h *gamesHarness) bool {
	h.logs.mu.Lock()
	defer h.logs.mu.Unlock()
	for _, r := range h.logs.records {
		if r.Level >= slog.LevelWarn {
			return true
		}
	}
	return false
}

func TestGameDeleteMedia(t *testing.T) {
	t.Parallel()

	t.Run("DEL-007/após game.delete com sucesso a pasta <mediaRoot>/<id> some; a de outro jogo e platforms ficam", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "media")
		med := &lateMedia{}
		h, _, _ := newMediaDeleteHarness(t, med, &callLog{})
		wireMedia(t, h, med, root)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		other := h.addGame(t, h.file(t, "other.exe"), "Outro")
		dmWrite(t, root, g.ID, "a.jpg")
		dmWrite(t, root, g.ID, "thumb_a.jpg")
		dmWrite(t, root, g.ID, filepath.Join("sub", "b.png"))
		dmWrite(t, root, other.ID, "keep.jpg")
		platform := filepath.Join(root, "platforms", "icon.png")
		if err := os.MkdirAll(filepath.Dir(platform), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(platform, []byte("icon"), 0o644); err != nil {
			t.Fatalf("write icon: %v", err)
		}

		deleteOK(t, h, g.ID)
		wantGameRemoved(t, h, g.ID)
		if dmExists(dmGameDir(root, g.ID)) {
			t.Error("folder of the removed game still exists")
		}
		if !dmExists(filepath.Join(dmGameDir(root, other.ID), "keep.jpg")) {
			t.Error("folder of another game was removed")
		}
		if !dmExists(platform) {
			t.Error("platform icon was removed by game.delete")
		}
	})

	t.Run("DEL-007/jogo sem pasta de mídia: game.delete responde sucesso", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "media")
		med := &lateMedia{}
		h, _, _ := newMediaDeleteHarness(t, med, &callLog{})
		wireMedia(t, h, med, root)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")

		deleteOK(t, h, g.ID)
		wantGameRemoved(t, h, g.ID)
	})

	t.Run("DEL-007/falha ao apagar a pasta não faz o delete falhar e é registrada em log", func(t *testing.T) {
		t.Parallel()
		log := &callLog{}
		med := &mediaRecorder{log: log, removeErr: errors.New("arquivo em uso")}
		h, _, _ := newMediaDeleteHarness(t, med, log)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")

		deleteOK(t, h, g.ID)
		wantGameRemoved(t, h, g.ID)
		if !dmLoggedWarn(h) {
			t.Error("the failed removal of the folder was not logged at Warn or above")
		}
	})

	t.Run("DEL-008/a ordem é: banco apaga com sucesso, depois CancelGame, depois RemoveGame", func(t *testing.T) {
		t.Parallel()
		log := &callLog{}
		med := &mediaRecorder{log: log}
		h, _, _ := newMediaDeleteHarness(t, med, log)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")

		deleteOK(t, h, g.ID)
		want := []string{"db.ok", "media.cancel", "media.remove"}
		if got := log.list(); !slices.Equal(got, want) {
			t.Errorf("order of the calls = %v, want %v", got, want)
		}
	})

	t.Run("DEL-007/sessão ativa do jogo recusa a remoção: a pasta continua e a mídia não é tocada", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "media")
		log := &callLog{}
		med := &lateMedia{}
		h, dt, _ := newMediaDeleteHarness(t, med, log)
		wireMedia(t, h, med, root)
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		dmWrite(t, root, g.ID, "a.jpg")
		dt.active[g.ID] = domain.SessionSnapshot{GameID: g.ID, Title: g.Title}

		wantError(t, deleteGame(t, h, g.ID), ipc.CodeSessionActive, ipc.MsgSessionActive)
		if !dmExists(filepath.Join(dmGameDir(root, g.ID), "a.jpg")) {
			t.Error("folder of a game refused by an active session was removed")
		}
		if got := log.list(); len(got) != 0 {
			t.Errorf("media calls after a refused removal = %v, want none", got)
		}
	})

	t.Run("DEL-007/falha do banco (-32001, -32002, -32603) não toca a pasta de mídia", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			err  error
			code int
			msg  string
		}{
			{"DEL-007/ErrGameNotFound do banco é -32001", store.ErrGameNotFound, ipc.CodeGameNotFound, ipc.MsgGameNotFound},
			{"DEL-007/ErrGameInUse do banco é -32002", store.ErrGameInUse, ipc.CodeSessionActive, ipc.MsgSessionActive},
			{"DEL-007/erro qualquer do banco é -32603", errors.New("disco cheio"), ipc.CodeInternalError, ipc.MsgInternalError},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				root := filepath.Join(t.TempDir(), "media")
				log := &callLog{}
				med := &lateMedia{}
				h, _, ost := newMediaDeleteHarness(t, med, log)
				wireMedia(t, h, med, root)
				g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
				dmWrite(t, root, g.ID, "a.jpg")
				ost.fail = tc.err

				wantError(t, deleteGame(t, h, g.ID), tc.code, tc.msg)
				if !dmExists(filepath.Join(dmGameDir(root, g.ID), "a.jpg")) {
					t.Error("folder removed although the database refused the removal")
				}
				if got := log.list(); len(got) != 0 {
					t.Errorf("media calls after a failed database removal = %v, want none", got)
				}
			})
		}
	})

	t.Run("DEL-008/download em curso do jogo é cancelado e esperado antes de apagar a pasta; a pasta não reaparece", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(t.TempDir(), "media")
		med := &lateMedia{}
		h, _, _ := newMediaDeleteHarness(t, med, &callLog{})
		files := media.New(root, nil)
		fetch := &ctxFetcher{root: root, gate: make(chan struct{}), started: make(chan struct{}, 4)}
		q := newMediaQueue(h.st, fetch, h.clock, slog.New(h.logs), nil)
		med.q, med.files = q, files
		g := h.addGame(t, h.file(t, "game.exe"), "Hollow Knight")
		if _, err := h.st.EnqueueMedia(t.Context(), g.ID, "cover", "steamgriddb", "https://cdn.example.test/hk.png", gamesT0); err != nil {
			t.Fatalf("EnqueueMedia: %v", err)
		}
		stop := runQueue(t, q)
		defer stop()
		<-fetch.started

		deleteOK(t, h, g.ID)
		wantGameRemoved(t, h, g.ID)
		if dmExists(dmGameDir(root, g.ID)) {
			t.Error("folder of the removed game exists right after game.delete")
		}
		// The download was cancelled and waited for: releasing it now must not bring the folder back.
		close(fetch.gate)
		if dmExists(dmGameDir(root, g.ID)) {
			t.Error("folder of the removed game came back after the download was released")
		}
	})
}
