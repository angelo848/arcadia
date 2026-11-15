package scrapers_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// steamStart is the fake instant every Steam Store limiter test begins at (phase 12, MET-010).
var steamStart = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

const (
	steamSearchOne = `{"total":1,"items":[{"id":504230,"name":"Celeste"}]}`
	steamSearchTwo = `{"total":2,"items":[{"id":504230,"name":"Celeste"},{"id":892970,"name":"Hades"}]}`

	// steamAppOK is one appdetails answer with text and two https screenshots (MET-014).
	steamAppOK = `{"504230":{"success":true,"data":{"name":"Celeste","short_description":"Escalada.",` +
		`"developers":["Maddy Makes Games"],"publishers":["Matt Makes Games"],"genres":[{"description":"Indie"}],` +
		`"release_date":{"date":"Jan 25, 2018"},"screenshots":[` +
		`{"path_full":"https://cdn.example.test/s1.jpg"},{"path_full":"https://cdn.example.test/s2.jpg"}]}}}`

	// steamAppMixed mixes an http and an https screenshot; only the https one may come out (MET-014).
	steamAppMixed = `{"504230":{"success":true,"data":{"name":"Celeste","screenshots":[` +
		`{"path_full":"http://cdn.example.test/h.jpg"},{"path_full":"https://cdn.example.test/s1.jpg"}]}}}`
)

// steamRoutes is the answer set of a Steam Store server. A nil app means appdetails is not expected at all,
// so any call to it is reported by newServer as an unexpected request.
func steamRoutes(search, app string) map[string]func(http.ResponseWriter, *http.Request) {
	routes := map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/storesearch/": jsonAnswer(search),
	}
	if app != "" {
		routes["GET /api/appdetails"] = jsonAnswer(app)
	}
	return routes
}

// newSteamStore starts a counting server with the given answers and returns the provider on it.
// The route counter is the number of HTTP requests the provider sent.
func newSteamStore(t *testing.T, lim *scrapers.Limiter, answers map[string]func(http.ResponseWriter, *http.Request)) (*scrapers.SteamStore, *route) {
	t.Helper()
	srv, rt := newServer(t, answers)
	// Registered after newServer, so it runs before srv.Close: no keep-alive goroutine outlives the test.
	t.Cleanup(srv.Client().CloseIdleConnections)
	p := scrapers.NewSteamStore(scrapers.SteamConfig{APIBase: srv.URL, HTTP: srv.Client(), Limiter: lim})
	return p, rt
}

// steamAsync runs fn on its own goroutine and delivers its error on the returned channel. The buffer lets
// the goroutine finish even when the test has already stopped reading.
func steamAsync(fn func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	return done
}

// TestSteamStoreContract holds the phase 12 rules of the Steam Store provider: one appdetails request for text and
// screenshots, the platform it confirms, the thumbnail built from the appid, and the limiter (MET-003, MET-008,
// MET-010, MET-014, MET-068).
func TestSteamStoreContract(t *testing.T) {
	t.Parallel()

	t.Run("MET-014/Fetch faz uma só requisição appdetails e devolve texto e screenshots", func(t *testing.T) {
		t.Parallel()
		p, rt := newSteamStore(t, nil, steamRoutes(steamSearchOne, steamAppOK))

		meta, media, err := p.Fetch(context.Background(), "504230")
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if meta.Description != "Escalada." || meta.Year != 2018 || !slices.Equal(meta.Genres, []string{"Indie"}) ||
			!slices.Equal(meta.Developers, []string{"Maddy Makes Games"}) || !slices.Equal(meta.Publishers, []string{"Matt Makes Games"}) {
			t.Errorf("Fetch meta = %+v", meta)
		}
		if len(media) != 2 {
			t.Fatalf("Fetch media = %+v; want 2 screenshots", media)
		}
		for i, want := range []string{"https://cdn.example.test/s1.jpg", "https://cdn.example.test/s2.jpg"} {
			if media[i].Kind != scrapers.KindScreenshot || media[i].URL != want {
				t.Errorf("media[%d] = %+v; want screenshot %s", i, media[i], want)
			}
		}
		if n := rt.calls.Load(); n != 1 {
			t.Errorf("requests = %d; want 1 (appdetails only, no storesearch)", n)
		}
	})

	t.Run("MET-014/Details e Media continuam funcionando (uma requisição cada)", func(t *testing.T) {
		t.Parallel()
		p, rt := newSteamStore(t, nil, steamRoutes(steamSearchOne, steamAppOK))

		meta, err := p.Details(context.Background(), "504230")
		if err != nil || meta.Description != "Escalada." || meta.Year != 2018 || !slices.Equal(meta.Genres, []string{"Indie"}) {
			t.Errorf("Details = %+v, %v", meta, err)
		}
		if n := rt.calls.Load(); n != 1 {
			t.Fatalf("requests after Details = %d; want 1", n)
		}

		media, err := p.Media(context.Background(), "504230")
		if err != nil || len(media) != 2 || media[0].Kind != scrapers.KindScreenshot {
			t.Errorf("Media = %+v, %v", media, err)
		}
		if n := rt.calls.Load(); n != 2 {
			t.Errorf("requests after Details and Media = %d; want 2 (one each)", n)
		}
	})

	t.Run("MET-014/screenshots http são descartados", func(t *testing.T) {
		t.Parallel()
		p, _ := newSteamStore(t, nil, steamRoutes(steamSearchOne, steamAppMixed))

		_, media, err := p.Fetch(context.Background(), "504230")
		if err != nil || len(media) != 1 || media[0].URL != "https://cdn.example.test/s1.jpg" {
			t.Errorf("Fetch media = %+v, %v; want only the https screenshot", media, err)
		}
		legacy, err := p.Media(context.Background(), "504230")
		if err != nil || len(legacy) != 1 || legacy[0].URL != "https://cdn.example.test/s1.jpg" {
			t.Errorf("Media = %+v, %v; want only the https screenshot", legacy, err)
		}
	})

	t.Run("MET-014/ThumbURL pelo appid sem requisição", func(t *testing.T) {
		t.Parallel()
		// No appdetails route: a thumbnail that needed a request would be reported as unexpected.
		p, rt := newSteamStore(t, nil, steamRoutes(steamSearchTwo, ""))

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste"})
		if err != nil || len(cands) != 2 {
			t.Fatalf("Search = %+v, %v; want 2 candidates", cands, err)
		}
		want := []string{
			"https://cdn.cloudflare.steamstatic.com/steam/apps/504230/capsule_231x87.jpg",
			"https://cdn.cloudflare.steamstatic.com/steam/apps/892970/capsule_231x87.jpg",
		}
		for i := range cands {
			if cands[i].ThumbURL != want[i] {
				t.Errorf("candidate %d ThumbURL = %q; want %q", i, cands[i].ThumbURL, want[i])
			}
		}
		if n := rt.calls.Load(); n != 1 {
			t.Errorf("requests = %d; want 1 (storesearch only)", n)
		}
	})

	t.Run("MET-003/PlatformKeys é windows e a plataforma está confirmada", func(t *testing.T) {
		t.Parallel()
		p, _ := newSteamStore(t, nil, steamRoutes(steamSearchOne, ""))

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste"})
		if err != nil || len(cands) != 1 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		if !slices.Equal(cands[0].PlatformKeys, []string{"windows"}) {
			t.Errorf("PlatformKeys = %v; want [windows]", cands[0].PlatformKeys)
		}
		if !cands[0].PlatformConfirmed {
			t.Error("PlatformConfirmed = false; the Steam Store only has PC games, so it confirms the platform")
		}
	})

	t.Run("MET-068/ThumbURL no host cdn.cloudflare.steamstatic.com em https", func(t *testing.T) {
		t.Parallel()
		p, _ := newSteamStore(t, nil, steamRoutes(steamSearchTwo, ""))

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste"})
		if err != nil || len(cands) != 2 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		for i, c := range cands {
			u, err := url.Parse(c.ThumbURL)
			if err != nil {
				t.Fatalf("candidate %d ThumbURL %q: %v", i, c.ThumbURL, err)
			}
			if u.Scheme != "https" || u.Host != "cdn.cloudflare.steamstatic.com" {
				t.Errorf("candidate %d ThumbURL %q; want https://cdn.cloudflare.steamstatic.com/...", i, c.ThumbURL)
			}
		}
	})

	t.Run("MET-014/app sem dados devolve vazio sem erro", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			body string
		}{
			{"MET-014/app sem dados: success falso", `{"504230":{"success":false}}`},
			{"MET-014/app sem dados: success verdadeiro e data nulo", `{"504230":{"success":true,"data":null}}`},
			{"MET-014/app sem dados: chave do appid ausente", `{}`},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				p, _ := newSteamStore(t, nil, steamRoutes(steamSearchOne, c.body))
				ctx := context.Background()

				meta, media, err := p.Fetch(ctx, "504230")
				if err != nil || !reflect.ValueOf(meta).IsZero() || len(media) != 0 {
					t.Errorf("Fetch = %+v, %+v, %v; want empty and no error", meta, media, err)
				}
				text, err := p.Details(ctx, "504230")
				if err != nil || !reflect.ValueOf(text).IsZero() {
					t.Errorf("Details = %+v, %v; want empty and no error", text, err)
				}
				shots, err := p.Media(ctx, "504230")
				if err != nil || len(shots) != 0 {
					t.Errorf("Media = %+v, %v; want empty and no error", shots, err)
				}
			})
		}
	})

	t.Run("MET-008/429 em appdetails é erro temporário", func(t *testing.T) {
		t.Parallel()
		tooMany := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }
		answers := map[string]func(http.ResponseWriter, *http.Request){"GET /api/appdetails": tooMany}
		ctx := context.Background()

		p, _ := newSteamStore(t, nil, answers)
		if _, _, err := p.Fetch(ctx, "504230"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("Fetch err = %v; want ErrUnavailable", err)
		}
		if _, err := p.Details(ctx, "504230"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("Details err = %v; want ErrUnavailable", err)
		}
		if _, err := p.Media(ctx, "504230"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("Media err = %v; want ErrUnavailable", err)
		}
	})
}

// steamCall is one provider method that must wait for the Steam Store limiter.
type steamCall struct {
	name string
	call func(ctx context.Context, p *scrapers.SteamStore) error
}

// TestSteamStoreLimiter checks that every method of the provider, whatever its origin, waits for the
// Steam Store interval [MET-010]. The FakeClock decides when the waiting request may go out.
func TestSteamStoreLimiter(t *testing.T) {
	t.Parallel()

	calls := []steamCall{
		{"MET-010/Search passa pelo Limiter", func(ctx context.Context, p *scrapers.SteamStore) error {
			_, err := p.Search(ctx, scrapers.Query{Title: "Celeste"})
			return err
		}},
		{"MET-010/Fetch passa pelo Limiter", func(ctx context.Context, p *scrapers.SteamStore) error {
			_, _, err := p.Fetch(ctx, "504230")
			return err
		}},
		{"MET-010/Details passa pelo Limiter", func(ctx context.Context, p *scrapers.SteamStore) error {
			_, err := p.Details(ctx, "504230")
			return err
		}},
		{"MET-010/Media passa pelo Limiter", func(ctx context.Context, p *scrapers.SteamStore) error {
			_, err := p.Media(ctx, "504230")
			return err
		}},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			clock := domain.NewFakeClock(steamStart)
			lim := scrapers.NewLimiter(clock, scrapers.IntervalSteamStore)
			p, rt := newSteamStore(t, lim, steamRoutes(steamSearchOne, steamAppOK))
			ctx := guardContext(t)

			// The first request takes the free slot at once, whatever method it is.
			if _, err := p.Search(ctx, scrapers.Query{Title: "Celeste"}); err != nil {
				t.Fatalf("primeira requisição: %v", err)
			}
			if n := rt.calls.Load(); n != 1 {
				t.Fatalf("requisições antes da espera = %d; want 1", n)
			}

			done := steamAsync(func() error { return c.call(ctx, p) })
			if err := clock.BlockUntilWaiters(ctx, 1); err != nil {
				t.Fatalf("esperando o limitador: %v", err)
			}
			// The waiting call has not reached the server: the request is only sent after Wait returns.
			if n := rt.calls.Load(); n != 1 {
				t.Fatalf("requisição saiu antes do horário reservado: total %d; want 1", n)
			}

			clock.Advance(scrapers.IntervalSteamStore)
			if err := expectReturn(t, done); err != nil {
				t.Fatalf("chamada depois do intervalo: %v", err)
			}
			if n := rt.calls.Load(); n != 2 {
				t.Errorf("requisições = %d; want 2", n)
			}
		})
	}

	t.Run("MET-010/cancelamento enquanto espera não envia a requisição", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(steamStart)
		lim := scrapers.NewLimiter(clock, scrapers.IntervalSteamStore)
		p, rt := newSteamStore(t, lim, steamRoutes(steamSearchOne, steamAppOK))
		if _, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste"}); err != nil {
			t.Fatalf("primeira requisição: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := steamAsync(func() error {
			_, err := p.Details(ctx, "504230")
			return err
		})
		if err := clock.BlockUntilWaiters(guardContext(t), 1); err != nil {
			t.Fatalf("esperando o limitador: %v", err)
		}
		cancel()
		if err := expectReturn(t, done); !errors.Is(err, context.Canceled) {
			t.Errorf("Details err = %v; want context.Canceled", err)
		}
		if n := rt.calls.Load(); n != 1 {
			t.Errorf("requisições = %d; want 1 (the cancelled call never went out)", n)
		}
	})
}

// TestSteamStoreLeavesNoGoroutine is sequential on purpose: goleak must not see goroutines of parallel siblings.
func TestSteamStoreLeavesNoGoroutine(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("MET-010/sem goroutine restante depois de espera cancelada", func(t *testing.T) {
		clock := domain.NewFakeClock(steamStart)
		lim := scrapers.NewLimiter(clock, scrapers.IntervalSteamStore)
		p, _ := newSteamStore(t, lim, steamRoutes(steamSearchOne, steamAppOK))
		if _, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste"}); err != nil {
			t.Fatalf("primeira requisição: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := steamAsync(func() error {
			_, _, err := p.Fetch(ctx, "504230")
			return err
		})
		if err := clock.BlockUntilWaiters(guardContext(t), 1); err != nil {
			t.Fatalf("esperando o limitador: %v", err)
		}
		cancel()
		if err := expectReturn(t, done); !errors.Is(err, context.Canceled) {
			t.Errorf("Fetch err = %v; want context.Canceled", err)
		}
	})
}
