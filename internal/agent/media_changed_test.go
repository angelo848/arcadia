package agent

// Tests for T05 of docs/specs/phase-10-2-imagens-ao-vivo/tasks.md: the agent sends media.changed
// to every client after a job stores an image (IMG-001), never before (IMG-002), and storing
// media creates no session (IMG-012).

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
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

// quietWindow is how long a test waits to prove that nothing arrives.
const quietWindow = 300 * time.Millisecond

// pngFor returns a small PNG whose colour depends on n, so every image has its own hash.
func pngFor(t *testing.T, n int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	c := color.RGBA{R: uint8(30 * (n + 1)), G: uint8(255 - 40*n), B: uint8(7 * n), A: 255}
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// gatedImages is an httptest.Server that serves PNGs at /<name>.png but holds each response until
// the test releases that name (or releaseAll).
type gatedImages struct {
	srv  *httptest.Server
	hit  chan string // a request for name arrived
	mu   sync.Mutex
	gate map[string]chan struct{}
	done chan struct{}
	once sync.Once
}

func newGatedImages(t *testing.T) *gatedImages {
	t.Helper()
	g := &gatedImages{hit: make(chan string, 64), gate: map[string]chan struct{}{}, done: make(chan struct{})}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".png")
		select {
		case g.hit <- name:
		default:
		}
		select {
		case <-g.gateFor(name):
		case <-g.done:
		case <-r.Context().Done():
			return
		}
		seed := 0
		if name != "" {
			seed = int(name[0]) // distinct first letter = distinct image = distinct hash
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngFor(t, seed))
	}))
	// Registered first, so it runs after the cleanups registered later (agent/queue shutdown);
	// callers also register releaseAll after starting the agent or queue so the gate opens first.
	t.Cleanup(g.srv.Close)
	return g
}

func (g *gatedImages) gateFor(name string) chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	ch, ok := g.gate[name]
	if !ok {
		ch = make(chan struct{})
		g.gate[name] = ch
	}
	return ch
}

func (g *gatedImages) url(name string) string { return g.srv.URL + "/" + name + ".png" }

func (g *gatedImages) release(name string) {
	ch := g.gateFor(name)
	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func (g *gatedImages) releaseAll() { g.once.Do(func() { close(g.done) }) }

func (g *gatedImages) waitHit(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-g.hit:
		if got != want {
			t.Fatalf("request for %q, want %q", got, want)
		}
	case <-time.After(testTimeout):
		t.Fatalf("no request for %q within %v", want, testTimeout)
	}
}

// expectMediaChanged waits for media.changed {gameId: want} on c.
func expectMediaChanged(t *testing.T, label string, c *ipc.Client, want int64) {
	t.Helper()
	id, ok := nextMediaChangedID(t, c, testTimeout)
	if !ok {
		t.Fatalf("%s: no media.changed within %v", label, testTimeout)
	}
	if id != want {
		t.Fatalf("%s: media.changed gameId = %d, want %d", label, id, want)
	}
}

// expectNoMediaChanged fails if a media.changed arrives on c within quietWindow.
func expectNoMediaChanged(t *testing.T, label string, c *ipc.Client) {
	t.Helper()
	if id, ok := nextMediaChangedID(t, c, quietWindow); ok {
		t.Fatalf("%s: unexpected media.changed {gameId: %d}", label, id)
	}
}

func callMediaList(t *testing.T, c *ipc.Client, gameID int64) []ipc.MediaItem {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	var res ipc.MediaListResult
	if err := c.Call(ctx, ipc.MethodMediaList, ipc.MediaListParams{GameID: gameID}, &res); err != nil {
		t.Fatalf("media.list: %v", err)
	}
	return res.Media
}

// ---- (a) real agent --------------------------------------------------------------------------

// seedMediaJob writes, before the agent starts, a game and a pending cover job pointing at url.
func seedMediaJob(t *testing.T, cfg config, url string) (gameID int64) {
	t.Helper()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	st, err := store.Open(t.Context(), dbPath(cfg), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer func() { _ = st.Close() }()
	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	g, err := st.AddGame(t.Context(), store.NewGame{Title: "Celeste", Exe: `C:\c.exe`, At: at})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	if _, err := st.EnqueueMedia(t.Context(), g.ID, "cover", "steamgriddb", url, at); err != nil {
		t.Fatalf("EnqueueMedia: %v", err)
	}
	return g.ID
}

// startMediaAgent seeds a held job, starts the agent (which resumes it) and connects two clients.
// The gate is released by the returned function, which is also registered to run before Shutdown.
func startMediaAgent(t *testing.T) (gameID int64, c1, c2 *ipc.Client, release func()) {
	t.Helper()
	imgs := newGatedImages(t)
	cfg := testConfig(t, "")
	gameID = seedMediaJob(t, cfg, imgs.url("cover"))
	ln := &tcpListen{}
	startTestAgent(t, cfg, ln)
	// Cleanups run last-in first-out: this one runs before the agent's Shutdown.
	t.Cleanup(imgs.releaseAll)
	_, addr := ln.snapshot()
	c1, c2 = newTCPClient(addr), newTCPClient(addr)
	t.Cleanup(func() { _ = c1.Close(); _ = c2.Close() })
	callStatus(t, c1)
	callStatus(t, c2)
	imgs.waitHit(t, "cover") // the resumed job is running and held
	return gameID, c1, c2, func() { imgs.release("cover") }
}

func TestAgentMediaChanged(t *testing.T) {
	t.Run("IMG-001/job retomado na subida grava e os dois clientes recebem media.changed {gameId: G}", func(t *testing.T) {
		g, c1, c2, release := startMediaAgent(t)
		expectNoMediaChanged(t, "before the image is stored (IMG-002)", c1)
		release()
		expectMediaChanged(t, "client 1", c1, g)
		expectMediaChanged(t, "client 2", c2, g)
	})

	t.Run("IMG-001/media.list de G feito ao receber o aviso já traz a imagem", func(t *testing.T) {
		g, c1, c2, release := startMediaAgent(t)
		if got := callMediaList(t, c1, g); len(got) != 0 {
			t.Fatalf("media.list before the job stores = %+v, want empty", got)
		}
		release()
		expectMediaChanged(t, "client 1", c1, g)
		expectMediaChanged(t, "client 2", c2, g)
		items := callMediaList(t, c2, g)
		if len(items) != 1 || items[0].Kind != "cover" {
			t.Fatalf("media.list after media.changed = %+v, want one cover", items)
		}
		if _, err := os.Stat(items[0].Path); err != nil {
			t.Errorf("stored image %s: %v", items[0].Path, err)
		}
	})

	t.Run("IMG-012/gravar mídia não cria sessão, não envia session.changed e session.current continua null", func(t *testing.T) {
		g, c1, c2, release := startMediaAgent(t)
		release()
		expectMediaChanged(t, "client 1", c1, g)
		expectMediaChanged(t, "client 2", c2, g)

		for i, c := range []*ipc.Client{c1, c2} {
			deadline := time.After(quietWindow)
		drain:
			for {
				select {
				case n := <-c.Notifications():
					if n.Method == ipc.MethodSessionChanged {
						t.Fatalf("client %d got session.changed %s", i+1, n.Params)
					}
				case <-deadline:
					break drain
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			var cur ipc.SessionCurrentResult
			err := c.Call(ctx, ipc.MethodSessionCurrent, nil, &cur)
			cancel()
			if err != nil {
				t.Fatalf("session.current: %v", err)
			}
			if cur.Session != nil {
				t.Errorf("client %d: session.current = %+v, want null", i+1, cur.Session)
			}
		}
	})
}

// ---- (b) assembled in the test ---------------------------------------------------------------

type mediaWire struct {
	st       *store.Store
	clock    *domain.FakeClock
	imgs     *gatedImages
	provider *fakeProvider
	c1, c2   *ipc.Client
	games    []int64
}

// newMediaWire builds store + scrapeService (fakeProvider) + mediaQueue (mediaNotifier) +
// ipc.Server over TCP with two clients, and runs the queue until the test ends. The queue and
// server are stopped after the gate is released.
func newMediaWire(t *testing.T, gameCount int) *mediaWire {
	t.Helper()
	root := t.TempDir()
	st, err := store.Open(t.Context(), root+string(os.PathSeparator)+"library.db", store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	w := &mediaWire{st: st, clock: domain.NewFakeClock(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))}
	for i := 0; i < gameCount; i++ {
		g, err := st.AddGame(t.Context(), store.NewGame{Title: "Celeste", Exe: `C:\c` + string(rune('a'+i)) + `.exe`, At: at})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		w.games = append(w.games, g.ID)
	}
	w.imgs = newGatedImages(t)
	w.provider = &fakeProvider{id: "igdb"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mediaRoot := root + string(os.PathSeparator) + "media"
	notes := &mediaNotifier{}
	queue := newMediaQueue(st, media.New(mediaRoot, nil), w.clock, log, notes)
	svc := &scrapeService{
		st:   st,
		keys: scrapers.NewKeys(&memKeyStore{values: map[string]string{}}),
		providers: func(name string) (scrapers.MetadataProvider, error) {
			if name != w.provider.id {
				return nil, errUnknownProvider
			}
			return w.provider, nil
		},
		queue:     queue,
		mediaRoot: mediaRoot,
		clock:     w.clock,
		log:       log,
	}
	server := ipc.NewServer(w.clock, scrapeHandlers(svc), log)
	notes.set(server)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = server.Serve(ctx, ln) }()
	go func() { defer wg.Done(); _ = queue.Run(ctx) }()
	w.c1, w.c2 = newTCPClient(ln.Addr().String()), newTCPClient(ln.Addr().String())
	// LIFO: gate released, clients closed, server and queue stopped, then store and httptest close.
	t.Cleanup(func() {
		cancel()
		_ = server.Close()
		wg.Wait()
		notes.set(nil)
	})
	t.Cleanup(func() { _ = w.c1.Close(); _ = w.c2.Close() })
	t.Cleanup(w.imgs.releaseAll)
	callMediaList(t, w.c1, w.games[0])
	callMediaList(t, w.c2, w.games[0])
	return w
}

func (w *mediaWire) call(t *testing.T, method string, params, result any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if err := w.c1.Call(ctx, method, params, result); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
}

func (w *mediaWire) setMedia(names ...string) {
	w.provider.media = nil
	for _, n := range names {
		w.provider.media = append(w.provider.media, scrapers.RemoteMedia{Kind: scrapers.KindCover, URL: w.imgs.url(n)})
	}
}

// setKindMedia is setMedia for any kind (cover is limited to 1 per game since 2026-11-15).
func (w *mediaWire) setKindMedia(kind string, names ...string) {
	w.provider.media = nil
	for _, n := range names {
		w.provider.media = append(w.provider.media, scrapers.RemoteMedia{Kind: kind, URL: w.imgs.url(n)})
	}
}

func (w *mediaWire) pendingJobs(t *testing.T) []store.MediaJob {
	t.Helper()
	jobs, err := w.st.PendingMediaJobs(t.Context())
	if err != nil {
		t.Fatalf("PendingMediaJobs: %v", err)
	}
	return jobs
}

func TestWiredMediaChanged(t *testing.T) {
	t.Run("IMG-001/scrape.apply: cada imagem gravada chega como media.changed aos dois clientes", func(t *testing.T) {
		w := newMediaWire(t, 1)
		g := w.games[0]
		// alterado em 2026-11-15 (capa = 1): duas imagens de tela, pois o limite de capa agora é 1
		w.setKindMedia(scrapers.KindScreenshot, "a", "b")
		var res ipc.ScrapeApplyResult
		w.call(t, ipc.MethodScrapeApply, ipc.ScrapeApplyParams{GameID: g, Provider: "igdb", ProviderID: "7"}, &res)
		if res.Queued != 2 {
			t.Fatalf("queued = %d, want 2", res.Queued)
		}

		w.imgs.release("a")
		expectMediaChanged(t, "image a, client 1", w.c1, g)
		expectMediaChanged(t, "image a, client 2", w.c2, g)

		// A second image inside the 500 ms window is coalesced (IMG-003); once the window is over
		// it goes out at once.
		w.clock.Advance(mediaChangedWindow + time.Millisecond)
		w.imgs.release("b")
		expectMediaChanged(t, "image b, client 1", w.c1, g)
		expectMediaChanged(t, "image b, client 2", w.c2, g)
		if items := callMediaList(t, w.c1, g); len(items) != 2 {
			t.Errorf("media.list = %d items, want 2", len(items))
		}
	})

	t.Run("IMG-001/media.queue também avisa quando o job grava", func(t *testing.T) {
		w := newMediaWire(t, 1)
		g := w.games[0]
		w.setMedia("q")
		var res ipc.MediaQueueResult
		w.call(t, ipc.MethodMediaQueue, ipc.MediaQueueParams{GameID: g, Provider: "igdb", ProviderID: "7", Kinds: []string{scrapers.KindCover}}, &res)
		if res.Queued != 1 {
			t.Fatalf("queued = %d, want 1", res.Queued)
		}
		w.imgs.release("q")
		expectMediaChanged(t, "client 1", w.c1, g)
		expectMediaChanged(t, "client 2", w.c2, g)
	})

	t.Run("IMG-002/scrape.apply, media.queue e media.cancel não enviam media.changed antes de um job gravar", func(t *testing.T) {
		w := newMediaWire(t, 1)
		g := w.games[0]
		w.setMedia("x")
		var applied ipc.ScrapeApplyResult
		w.call(t, ipc.MethodScrapeApply, ipc.ScrapeApplyParams{GameID: g, Provider: "igdb", ProviderID: "7"}, &applied)
		w.imgs.waitHit(t, "x") // the job runs, held at the gate
		expectNoMediaChanged(t, "after scrape.apply", w.c1)
		expectNoMediaChanged(t, "after scrape.apply", w.c2)

		w.setKindMedia(scrapers.KindScreenshot, "y") // alterado em 2026-11-15 (capa = 1): a capa já está ocupada pelo job "x"
		var queued ipc.MediaQueueResult
		w.call(t, ipc.MethodMediaQueue, ipc.MediaQueueParams{GameID: g, Provider: "igdb", ProviderID: "7", Kinds: []string{scrapers.KindScreenshot}}, &queued)
		w.imgs.waitHit(t, "y")
		expectNoMediaChanged(t, "after media.queue", w.c1)
		expectNoMediaChanged(t, "after media.queue", w.c2)

		var cancelled ipc.MediaCancelResult
		w.call(t, ipc.MethodMediaCancel, ipc.MediaCancelParams{JobID: 2}, &cancelled)
		expectNoMediaChanged(t, "after media.cancel", w.c1)
		expectNoMediaChanged(t, "after media.cancel", w.c2)
	})

	t.Run("IMG-002/job cancelado não envia media.changed", func(t *testing.T) {
		w := newMediaWire(t, 2)
		cancelled, sentinel := w.games[0], w.games[1]

		w.setMedia("c")
		var q1 ipc.MediaQueueResult
		w.call(t, ipc.MethodMediaQueue, ipc.MediaQueueParams{GameID: cancelled, Provider: "igdb", ProviderID: "7", Kinds: []string{scrapers.KindCover}}, &q1)
		w.imgs.waitHit(t, "c")

		var res ipc.MediaCancelResult
		w.call(t, ipc.MethodMediaCancel, ipc.MediaCancelParams{JobID: 1}, &res)
		w.imgs.release("c") // the download was interrupted; releasing is harmless

		// A sentinel job for another game finishes after the cancel: its notice must be the
		// first and only one.
		w.setMedia("s")
		var q2 ipc.MediaQueueResult
		w.call(t, ipc.MethodMediaQueue, ipc.MediaQueueParams{GameID: sentinel, Provider: "igdb", ProviderID: "7", Kinds: []string{scrapers.KindCover}}, &q2)
		w.imgs.waitHit(t, "s")
		w.imgs.release("s")
		expectMediaChanged(t, "sentinel, client 1", w.c1, sentinel)
		expectMediaChanged(t, "sentinel, client 2", w.c2, sentinel)
		expectNoMediaChanged(t, "cancelled job, client 1", w.c1)
		expectNoMediaChanged(t, "cancelled job, client 2", w.c2)
		if items := callMediaList(t, w.c1, cancelled); len(items) != 0 {
			t.Errorf("cancelled job stored %+v, want nothing", items)
		}
	})

	// Novos em 2026-11-15 (ADR 0034, MET-022/MET-024): o limite vale também no caminho que acorda a fila.
	t.Run("MET-022/media.queue com a cota de capas cheia não enfileira, não acorda a fila e não envia media.changed", func(t *testing.T) {
		w := newMediaWire(t, 1)
		g := w.games[0]
		storeImages(t, w.st, g, "launchbox", scrapers.KindCover, "old", 1) // alterado em 2026-11-15 (capa = 1)
		w.setMedia("full")
		var res ipc.MediaQueueResult
		w.call(t, ipc.MethodMediaQueue, ipc.MediaQueueParams{GameID: g, Provider: "igdb", ProviderID: "7", Kinds: []string{scrapers.KindCover}}, &res)
		if res.Queued != 0 {
			t.Fatalf("queued = %d, want 0 (the game already has its 1 cover)", res.Queued)
		}
		if jobs := w.pendingJobs(t); len(jobs) != 0 {
			t.Errorf("pending jobs = %+v, want none", jobs)
		}
		expectNoMediaChanged(t, "client 1", w.c1)
		expectNoMediaChanged(t, "client 2", w.c2)
	})

	t.Run("MET-024/scrape.apply com a vaga de capa (limite 1) enfileira 1 e o aviso sai quando essa imagem grava", func(t *testing.T) {
		w := newMediaWire(t, 1)
		g := w.games[0]
		w.setMedia("a", "b") // alterado em 2026-11-15 (capa = 1): sem capa gravada, a única vaga entra
		var res ipc.ScrapeApplyResult
		w.call(t, ipc.MethodScrapeApply, ipc.ScrapeApplyParams{GameID: g, Provider: "igdb", ProviderID: "7"}, &res)
		if res.Queued != 1 {
			t.Fatalf("queued = %d, want 1 (one cover vacancy)", res.Queued)
		}
		w.imgs.release("a")
		expectMediaChanged(t, "client 1", w.c1, g)
		expectMediaChanged(t, "client 2", w.c2, g)
	})
}
