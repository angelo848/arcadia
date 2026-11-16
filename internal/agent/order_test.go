package agent

// Tests for T28 of docs/specs/phase-12-metadados-unificados/tasks.md: the shutdown with a metadata batch in
// progress (MET-059), the batch notifier wired to the IPC server (MET-054) and the unified metadata handlers
// registered on the real IPC server (MET-070). Providers are blocking fakes (cfg.Providers), the store is real
// in t.TempDir(), and no test sleeps: waits are channels with testTimeout.
//
// The shutdown steps are observed through instance.onShutdownStep, a hook that is nil in production and is
// called by Shutdown with: "server.close" (after Server.Close), "metadata.wait" (after metadataService.Wait),
// "queue.stop" (before stopQueue), "tracker.close" and "store.close".

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// cancelLoggedProv is a provider of a batch in progress: Search waits for its context (a slow provider)
// and logs "provider-cancelled" when the context ends.
type cancelLoggedProv struct {
	metaProv
	log func(step string)
}

func (p *cancelLoggedProv) Search(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error) {
	_, err := p.metaProv.Search(ctx, q)
	p.log("provider-cancelled")
	return nil, err
}

// blockingBatchProviders is the provider factory (cfg.Providers) of the three batch providers, each one
// blocking its Search until cancelled. started receives one value per Search that starts.
func blockingBatchProviders(started chan struct{}, log func(string)) func(string) (scrapers.MetadataProvider, error) {
	provs := map[string]scrapers.MetadataProvider{}
	for _, name := range batchProviders {
		provs[name] = &cancelLoggedProv{metaProv: metaProv{id: name, block: true, started: started}, log: log}
	}
	return func(name string) (scrapers.MetadataProvider, error) {
		p, ok := provs[name]
		if !ok {
			return nil, errUnknownProvider
		}
		return p, nil
	}
}

// waitSearchStarted waits until a batch Search has started on a provider.
func waitSearchStarted(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(testTimeout):
		t.Fatalf("no provider Search started within %v", testTimeout)
	}
}

// startBlockingBatch starts a batch of one game whose providers block, and waits until a search is in progress.
func startBlockingBatch(t *testing.T, w *wiring, c *rawClient, started chan struct{}, log func(string)) {
	t.Helper()
	gameID, _ := w.addGame(t, c, "Shutdown Batch Game")
	c.rpcOK(t, ipc.MethodMetadataBatchStart, ipc.BatchStartParams{GameIDs: []int64{gameID}})
	waitSearchStarted(t, started)
}

func TestMetadataShutdownOrder(t *testing.T) {
	t.Run("MET-059/a ordem do Shutdown tem metadata.Wait depois de server.Close e antes de stopQueue e do fechamento do store", func(t *testing.T) {
		var (
			mu    sync.Mutex
			steps []string
		)
		add := func(step string) {
			mu.Lock()
			defer mu.Unlock()
			steps = append(steps, step)
		}
		started := make(chan struct{}, 8)

		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Providers = blockingBatchProviders(started, add)
			},
		})
		c := w.client(t)
		startBlockingBatch(t, w, c, started, add)

		// No client is left: the drain ends at once, so the order is the only thing observed.
		_ = c.conn.Close()
		w.a.onShutdownStep = add
		if err := w.a.Shutdown(contextWithTestTimeout(t), ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}

		mu.Lock()
		got := append([]string(nil), steps...)
		mu.Unlock()

		serverIdx := indexOf(got, "server.close")
		cancelIdx := indexOf(got, "provider-cancelled")
		waitIdx := indexOf(got, "metadata.wait")
		queueIdx := indexOf(got, "queue.stop")
		trackerIdx := indexOf(got, "tracker.close")
		storeIdx := indexOf(got, "store.close")
		if serverIdx == -1 || cancelIdx == -1 || waitIdx == -1 || queueIdx == -1 || trackerIdx == -1 || storeIdx == -1 {
			t.Fatalf("missing shutdown step: %v", got)
		}
		if !(serverIdx < cancelIdx && cancelIdx < waitIdx) {
			t.Errorf("want server.close < provider-cancelled < metadata.wait, got %v", got)
		}
		if !(waitIdx < queueIdx) {
			t.Errorf("metadata.wait (%d) must come before queue.stop (%d): %v", waitIdx, queueIdx, got)
		}
		if !(waitIdx < trackerIdx && waitIdx < storeIdx) {
			t.Errorf("metadata.wait (%d) must come before tracker.close (%d) and store.close (%d): %v",
				waitIdx, trackerIdx, storeIdx, got)
		}
	})

	t.Run("MET-059/Shutdown com busca em curso termina sem goroutine restante (-race)", func(t *testing.T) {
		started := make(chan struct{}, 8)
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Providers = blockingBatchProviders(started, func(string) {})
			},
		})
		c := w.client(t)
		startBlockingBatch(t, w, c, started, func(string) {})
		_ = c.conn.Close()

		if w.a.metadata == nil {
			t.Fatal("instance.metadata is nil: the metadata service is not wired")
		}
		if err := w.a.Shutdown(contextWithTestTimeout(t), ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		// Shutdown already waited for the service; a second wait must return at once, and newWiring's
		// goleak check (registered before the agent) then finds no goroutine of the batch left.
		done := make(chan struct{})
		go func() {
			defer close(done)
			w.a.metadata.Wait()
		}()
		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Fatalf("metadata.Wait did not return within %v after Shutdown", testTimeout)
		}
	})

	t.Run("MET-054/batchNotifier ligado ao servidor na subida e zerado no desligamento", func(t *testing.T) {
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Providers = blockingBatchProviders(make(chan struct{}, 8), func(string) {})
			},
		})
		if w.a.batchNotifier == nil {
			t.Fatal("instance.batchNotifier is nil")
		}
		if w.a.batchNotifier.srv.Load() == nil {
			t.Error("batchNotifier not bound to the IPC server after startAgent")
		}
		if err := w.a.Shutdown(contextWithTestTimeout(t), ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		if got := w.a.batchNotifier.srv.Load(); got != nil {
			t.Errorf("batchNotifier still bound to the IPC server after Shutdown: %p, want nil", got)
		}
	})
}

// TestVideoShutdown covers VID-016 (T26): Shutdown with a video download in flight cancels it, waits for the
// lane and leaves no goroutine (newWiring registers the goleak check; run with -race).
func TestVideoShutdown(t *testing.T) {
	t.Run("VID-016/Shutdown com job de vídeo em andamento não deixa goroutine (-race)", func(t *testing.T) {
		ss := newSSServer(t, mappedSystem(t))
		ss.setMode("hold")
		vw := newVideoWire(t, ss, videoWireOpts{games: 1, credentials: true})
		vw.apply(t, vw.games[0])
		pumpUntil(t, vw.clock, "download started", func() bool {
			select {
			case <-ss.inDownload:
				return true
			default:
				return false
			}
		})
		_ = vw.c1.conn.Close()
		_ = vw.c2.conn.Close()

		if err := vw.a.Shutdown(contextWithTestTimeout(t), ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		select {
		case <-ss.cancelled:
		case <-time.After(testTimeout):
			t.Error("the in-flight video download was not cancelled by Shutdown")
		}
	})
}

// indexOf returns the position of s in list, or -1 (used by the call-order assertions of app and shutdown tests).
func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// contextWithTestTimeout returns a context that ends after testTimeout; the cancel is registered on t.
func contextWithTestTimeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	t.Cleanup(cancel)
	return ctx
}

func TestMetadataHandlersWired(t *testing.T) {
	t.Run("MET-070/o agente registra os 8 handlers metadata.* e continua com scrape.search, scrape.apply e media.queue", func(t *testing.T) {
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Providers = blockingBatchProviders(make(chan struct{}, 8), func(string) {})
			},
		})
		c := w.client(t)

		// A registered method answers with a result or an error other than -32601; the params are
		// deliberately empty, so the handlers may refuse them, but they must exist.
		methods := []string{
			ipc.MethodMetadataSearch,
			ipc.MethodMetadataApply,
			ipc.MethodMetadataBatchStart,
			ipc.MethodMetadataBatchStatus,
			ipc.MethodMetadataBatchResults,
			ipc.MethodMetadataBatchApply,
			ipc.MethodMetadataBatchCancel,
			ipc.MethodMetadataBatchDiscard,
			ipc.MethodScrapeSearch,
			ipc.MethodScrapeApply,
			ipc.MethodMediaQueue,
		}
		for _, m := range methods {
			t.Run(fmt.Sprintf("MET-070/%s registrado", m), func(t *testing.T) {
				resp := c.rpc(t, m, nil)
				if resp.Error != nil && resp.Error.Code == ipc.CodeMethodNotFound {
					t.Errorf("%s: method not found; the handler is not registered on the IPC server", m)
				}
			})
		}

		// Control: a method that does not exist is still -32601.
		t.Run("MET-070/método inexistente continua -32601", func(t *testing.T) {
			resp := c.rpc(t, "metadata.nope", nil)
			if resp.Error == nil || resp.Error.Code != ipc.CodeMethodNotFound {
				t.Errorf("metadata.nope: error = %+v, want code %d", resp.Error, ipc.CodeMethodNotFound)
			}
		})
	})
}
