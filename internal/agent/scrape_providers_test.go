package agent

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// Casos da T18 (phase-12-metadados-unificados): fábrica de provedores com limitadores e IGDB compartilhado.
// Pacote interno: newProviderFactory é não exportada. Sem rede: a fábrica usa URLs de produção, então o
// cliente HTTP do teste reescreve o host de cada requisição para um httptest.Server. Relógio fake, sem Sleep.
//
// Premissas assumidas onde a spec não fecha (ver o relatório da T18):
//   - o IGDB da fábrica usa os caminhos de produção (/oauth2/token, /v4/games) e o cliente recebido;
//   - a requisição de token do IGDB não passa pelo limitador (só as da API), então não gasta slot;
//   - Steam Store e SteamGridDB passam pelo limitador também em Search (storesearch e autocomplete), e a
//     Media do SteamGridDB faz duas requisições (grids e heroes), cada uma com seu slot;
//   - a troca de chave do IGDB cria outra instância, mas o limitador continua o mesmo (está na fábrica).

var provNow = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// provNet is an httptest server that answers the paths of the three providers and counts the hits.
type provNet struct {
	srv    *httptest.Server
	tokens atomic.Int32
	igdb   atomic.Int32 // POST /v4/games
	sgdb   atomic.Int32 // autocomplete, grids and heroes
	steam  atomic.Int32 // storesearch and appdetails

	mu           sync.Mutex
	lastClientID string // client_id of the last token request
}

func newProvNet(t *testing.T) *provNet {
	t.Helper()
	n := &provNet{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		n.tokens.Add(1)
		// The credentials may travel in the query or in a form body: accept either, the test checks the value.
		clientID := r.URL.Query().Get("client_id")
		if clientID == "" {
			clientID = r.PostFormValue("client_id")
		}
		n.mu.Lock()
		n.lastClientID = clientID
		n.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"tok","expires_in":3600}`)
	})
	mux.HandleFunc("/v4/games", func(w http.ResponseWriter, _ *http.Request) {
		n.igdb.Add(1)
		_, _ = io.WriteString(w, `[]`)
	})
	mux.HandleFunc("/api/v2/search/autocomplete/", func(w http.ResponseWriter, _ *http.Request) {
		n.sgdb.Add(1)
		_, _ = io.WriteString(w, `{"success":true,"data":[]}`)
	})
	mux.HandleFunc("/api/v2/grids/game/", func(w http.ResponseWriter, _ *http.Request) {
		n.sgdb.Add(1)
		_, _ = io.WriteString(w, `{"success":true,"data":[]}`)
	})
	mux.HandleFunc("/api/v2/heroes/game/", func(w http.ResponseWriter, _ *http.Request) {
		n.sgdb.Add(1)
		_, _ = io.WriteString(w, `{"success":true,"data":[]}`)
	})
	mux.HandleFunc("/api/storesearch/", func(w http.ResponseWriter, _ *http.Request) {
		n.steam.Add(1)
		_, _ = io.WriteString(w, `{"total":0,"items":[]}`)
	})
	n.srv = httptest.NewServer(mux)
	t.Cleanup(n.srv.Close)
	return n
}

// provRewrite sends every request to the test server, keeping the path and query.
type provRewrite struct {
	target *url.URL
	base   http.RoundTripper
}

func (p provRewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Scheme = p.target.Scheme
	r2.URL.Host = p.target.Host
	r2.Host = p.target.Host
	return p.base.RoundTrip(r2)
}

func provFactory(t *testing.T, n *provNet, clock domain.Clock, keys *scrapers.Keys) func(string) (scrapers.MetadataProvider, error) {
	t.Helper()
	target, err := url.Parse(n.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: provRewrite{target: target, base: n.srv.Client().Transport}}
	return newProviderFactory(keys, client, clock)
}

func provKeys(t *testing.T) *scrapers.Keys {
	t.Helper()
	k := scrapers.NewKeys(&memKeyStore{values: map[string]string{}})
	if err := k.SetIGDB("cid", "sec"); err != nil {
		t.Fatalf("SetIGDB: %v", err)
	}
	if err := k.Set("steamgriddb", "key"); err != nil {
		t.Fatalf("Set steamgriddb: %v", err)
	}
	return k
}

// provGet builds the provider of name through the factory, failing the test on error.
func provGet(t *testing.T, f func(string) (scrapers.MetadataProvider, error), name string) scrapers.MetadataProvider {
	t.Helper()
	p, err := f(name)
	if err != nil {
		t.Fatalf("fábrica(%s): %v", name, err)
	}
	return p
}

// provAsync runs fn in a goroutine and returns a channel closed when fn returns.
func provAsync(t *testing.T, fn func()) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	return done
}

// provJoin returns a channel closed once every channel in chs is closed.
func provJoin(chs ...<-chan struct{}) <-chan struct{} {
	out := make(chan struct{})
	go func() {
		for _, c := range chs {
			<-c
		}
		close(out)
	}()
	return out
}

// provDrive moves the fake clock by step each time a request is waiting for it, until done is closed.
// It never sleeps: it waits on the clock's own waiter registration, with a real timeout as failure guard.
func provDrive(t *testing.T, clock *domain.FakeClock, done <-chan struct{}, step time.Duration) {
	t.Helper()
	for range 100 {
		select {
		case <-done:
			return
		default:
		}
		ctx, cancel := context.WithCancel(context.Background())
		blocked := make(chan error, 1)
		go func() { blocked <- clock.BlockUntilWaiters(ctx, 1) }()
		select {
		case <-done:
			cancel()
			<-blocked
			return
		case <-blocked:
			cancel()
			clock.Advance(step)
		case <-time.After(5 * time.Second):
			cancel()
			<-blocked
			t.Fatal("nenhuma espera pendente no relógio fake")
		}
	}
	t.Fatal("limite de passos excedido")
}

// provWait fails the test if done is not closed within a real timeout (the answer must not need the clock).
func provWait(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s não terminou sem avançar o relógio", what)
	}
}

func TestProviderFactoryLimiters(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	igdbQuery := scrapers.Query{Title: "Hades"}

	t.Run("MET-010/duas chamadas à fábrica compartilham o limitador do IGDB (as requisições saem espaçadas, relógio fake)", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(provNow)
		n := newProvNet(t)
		f := provFactory(t, n, clock, provKeys(t))
		p1 := provGet(t, f, "igdb")
		p2 := provGet(t, f, "igdb")

		start := clock.Now()
		d1 := provAsync(t, func() { _, _ = p1.Search(ctx, igdbQuery) })
		d2 := provAsync(t, func() { _, _ = p2.Search(ctx, igdbQuery) })
		provDrive(t, clock, provJoin(d1, d2), scrapers.IntervalIGDB)

		if got := clock.Now().Sub(start); got != scrapers.IntervalIGDB {
			t.Errorf("tempo gasto = %v, want %v (a segunda requisição espera o intervalo)", got, scrapers.IntervalIGDB)
		}
		if got := n.igdb.Load(); got != 2 {
			t.Errorf("requisições à API = %d, want 2", got)
		}
	})

	t.Run("MET-010/SteamGridDB e Steam Store compartilham o seu", func(t *testing.T) {
		t.Parallel()

		t.Run("SteamGridDB", func(t *testing.T) {
			t.Parallel()
			clock := domain.NewFakeClock(provNow)
			n := newProvNet(t)
			f := provFactory(t, n, clock, provKeys(t))
			p1 := provGet(t, f, "steamgriddb")
			p2 := provGet(t, f, "steamgriddb")

			// Cada Media faz grids e heroes: quatro requisições, uma a cada 500 ms, a última em 1500 ms.
			start := clock.Now()
			d1 := provAsync(t, func() { _, _ = p1.Media(ctx, "123") })
			d2 := provAsync(t, func() { _, _ = p2.Media(ctx, "456") })
			provDrive(t, clock, provJoin(d1, d2), scrapers.IntervalSteamGridDB)

			if got := clock.Now().Sub(start); got != 3*scrapers.IntervalSteamGridDB {
				t.Errorf("tempo gasto = %v, want %v", got, 3*scrapers.IntervalSteamGridDB)
			}
			if got := n.sgdb.Load(); got != 4 {
				t.Errorf("requisições ao SteamGridDB = %d, want 4", got)
			}
		})

		t.Run("Steam Store", func(t *testing.T) {
			t.Parallel()
			clock := domain.NewFakeClock(provNow)
			n := newProvNet(t)
			f := provFactory(t, n, clock, provKeys(t))
			p1 := provGet(t, f, "steamstore")
			p2 := provGet(t, f, "steamstore")

			start := clock.Now()
			d1 := provAsync(t, func() { _, _ = p1.Search(ctx, igdbQuery) })
			d2 := provAsync(t, func() { _, _ = p2.Search(ctx, igdbQuery) })
			provDrive(t, clock, provJoin(d1, d2), scrapers.IntervalSteamStore)

			if got := clock.Now().Sub(start); got != scrapers.IntervalSteamStore {
				t.Errorf("tempo gasto = %v, want %v", got, scrapers.IntervalSteamStore)
			}
			if got := n.steam.Load(); got != 2 {
				t.Errorf("requisições à Steam Store = %d, want 2", got)
			}
		})
	})

	t.Run("MET-010/os três limitadores são independentes", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(provNow)
		n := newProvNet(t)
		f := provFactory(t, n, clock, provKeys(t))
		igdb := provGet(t, f, "igdb")
		sgdb := provGet(t, f, "steamgriddb")
		steam := provGet(t, f, "steamstore")

		// Primeira chamada do IGDB sai na hora e reserva o próximo slot em +334 ms.
		provDrive(t, clock, provAsync(t, func() { _, _ = igdb.Search(ctx, igdbQuery) }), scrapers.IntervalIGDB)
		pending := provAsync(t, func() { _, _ = igdb.Search(ctx, igdbQuery) })

		// SteamGridDB e Steam Store não esperam o IGDB: terminam sem avançar o relógio.
		provWait(t, provAsync(t, func() { _, _ = sgdb.Search(ctx, scrapers.Query{Title: "Hades"}) }), "busca no SteamGridDB")
		provWait(t, provAsync(t, func() { _, _ = steam.Search(ctx, igdbQuery) }), "busca na Steam Store")

		if got := n.igdb.Load(); got != 1 {
			t.Errorf("requisições ao IGDB antes do horário = %d, want 1", got)
		}
		provDrive(t, clock, pending, scrapers.IntervalIGDB)
	})

	t.Run("MET-012/duas chamadas à fábrica pedem um só token (httptest conta o /token)", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(provNow)
		n := newProvNet(t)
		f := provFactory(t, n, clock, provKeys(t))

		p1 := provGet(t, f, "igdb")
		provDrive(t, clock, provAsync(t, func() { _, _ = p1.Search(ctx, igdbQuery) }), scrapers.IntervalIGDB)
		p2 := provGet(t, f, "igdb")
		provDrive(t, clock, provAsync(t, func() { _, _ = p2.Search(ctx, igdbQuery) }), scrapers.IntervalIGDB)

		if got := n.tokens.Load(); got != 1 {
			t.Errorf("requisições de token = %d, want 1", got)
		}
	})

	t.Run("MET-012/a chave mudou: novo token", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(provNow)
		n := newProvNet(t)
		keys := provKeys(t)
		f := provFactory(t, n, clock, keys)

		p1 := provGet(t, f, "igdb")
		provDrive(t, clock, provAsync(t, func() { _, _ = p1.Search(ctx, igdbQuery) }), scrapers.IntervalIGDB)
		if err := keys.SetIGDB("cid2", "sec2"); err != nil {
			t.Fatalf("SetIGDB: %v", err)
		}
		p2 := provGet(t, f, "igdb")
		provDrive(t, clock, provAsync(t, func() { _, _ = p2.Search(ctx, igdbQuery) }), scrapers.IntervalIGDB)

		if got := n.tokens.Load(); got != 2 {
			t.Errorf("requisições de token = %d, want 2 (chave nova)", got)
		}
		n.mu.Lock()
		defer n.mu.Unlock()
		if n.lastClientID != "cid2" {
			t.Errorf("client_id do último token = %q, want cid2 (chave nova)", n.lastClientID)
		}
	})

	t.Run("MET-012/sem chave: ErrNoKey sem requisição", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(provNow)
		n := newProvNet(t)
		f := provFactory(t, n, clock, scrapers.NewKeys(&memKeyStore{values: map[string]string{}}))

		for _, name := range []string{"igdb", "steamgriddb"} {
			if _, err := f(name); !errors.Is(err, scrapers.ErrNoKey) {
				t.Errorf("fábrica(%s) err = %v, want ErrNoKey", name, err)
			}
		}
		if n.tokens.Load()+n.igdb.Load()+n.sgdb.Load() != 0 {
			t.Errorf("houve requisição sem chave")
		}
	})
}
