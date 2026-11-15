// Phase 12 (T14): SteamGridDB covers in portrait and heroes as background [MET-013, MET-010].
//
// Supersedes the Media case "MID-002/busca e capas com cabeçalho Bearer" of TestSteamGridDB in
// scrapers_test.go, alterado em 2026-11-15: Media now asks for grids (portrait, static) and for heroes
// (background), so that old case must be updated by its owner (scrapers_test.go is not in this task).
package scrapers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

const (
	sgdbGameID = "123"
	// sgdbPortraitDims are the portrait sizes asked from /grids (contracts/provider-api-notes.md §2a).
	sgdbPortraitDims = "600x900,342x482,660x930"
	sgdbTwoGrids     = `{"success":true,"data":[{"url":"https://cdn.example.test/grid1.png"},{"url":"https://cdn.example.test/grid2.png"}]}`
	sgdbOneHero      = `{"success":true,"data":[{"url":"https://cdn.example.test/hero1.jpg"}]}`
)

// sgdbStart is the fake instant of the SteamGridDB tests.
var sgdbStart = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// sgdbAnswer returns a handler that counts its hits, checks the Bearer header and the query (check may
// be nil) and answers body.
func sgdbAnswer(t *testing.T, body string, check func(q url.Values)) (func(http.ResponseWriter, *http.Request), *atomic.Int32) {
	t.Helper()
	hits := new(atomic.Int32)
	h := func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer key" {
			t.Errorf("Authorization = %q, want Bearer key", got)
		}
		if check != nil {
			check(r.URL.Query())
		}
		jsonAnswer(body)(w, r)
	}
	return h, hits
}

// sgdbGridsCheck asserts the portrait grids request: static only, portrait dimensions [MET-013].
func sgdbGridsCheck(t *testing.T) func(url.Values) {
	return func(q url.Values) {
		if got := q.Get("dimensions"); got != sgdbPortraitDims {
			t.Errorf("grids dimensions = %q, want %q", got, sgdbPortraitDims)
		}
		if got := q.Get("types"); got != "static" {
			t.Errorf("grids types = %q, want static", got)
		}
	}
}

// sgdbHeroesCheck asserts the heroes request has no dimensions filter (provider-api-notes.md §2b).
func sgdbHeroesCheck(t *testing.T) func(url.Values) {
	return func(q url.Values) {
		if q.Has("dimensions") {
			t.Errorf("heroes dimensions = %q, want no filter", q.Get("dimensions"))
		}
	}
}

// sgdbRoutes returns the two routes of Media for the game sgdbGameID and their hit counters.
func sgdbRoutes(t *testing.T, grids, heroes string) (map[string]func(http.ResponseWriter, *http.Request), *atomic.Int32, *atomic.Int32) {
	t.Helper()
	gridH, gridHits := sgdbAnswer(t, grids, sgdbGridsCheck(t))
	heroH, heroHits := sgdbAnswer(t, heroes, sgdbHeroesCheck(t))
	routes := map[string]func(http.ResponseWriter, *http.Request){
		"GET /grids/game/" + sgdbGameID:  gridH,
		"GET /heroes/game/" + sgdbGameID: heroH,
	}
	return routes, gridHits, heroHits
}

// sgdbNew returns the provider against srv with the given limiter (nil for none).
func sgdbNew(srv *httptest.Server, lim *scrapers.Limiter) *scrapers.SteamGridDB {
	return scrapers.NewSteamGridDB(scrapers.SGDBConfig{Key: "key", APIBase: srv.URL, HTTP: srv.Client(), Limiter: lim})
}

func TestSteamGridDBPortraitAndHero(t *testing.T) {
	t.Parallel()

	t.Run("MET-013/Media faz duas requisições: grids e heroes", func(t *testing.T) {
		t.Parallel()
		routes, gridHits, heroHits := sgdbRoutes(t, sgdbTwoGrids, sgdbOneHero)
		srv, rt := newServer(t, routes)
		if _, err := sgdbNew(srv, nil).Media(context.Background(), sgdbGameID); err != nil {
			t.Fatalf("Media: %v", err)
		}
		if n := rt.calls.Load(); n != 2 {
			t.Errorf("requests = %d, want 2", n)
		}
		if gridHits.Load() != 1 || heroHits.Load() != 1 {
			t.Errorf("grids hits = %d, heroes hits = %d; want 1 and 1", gridHits.Load(), heroHits.Load())
		}
	})

	t.Run("MET-013/grids com dimensões de retrato e types=static na URL", func(t *testing.T) {
		t.Parallel()
		gridH, gridHits := sgdbAnswer(t, sgdbTwoGrids, sgdbGridsCheck(t))
		srv, _ := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"GET /grids/game/" + sgdbGameID: gridH,
			"GET /heroes/game/" + sgdbGameID: func(w http.ResponseWriter, r *http.Request) {
				jsonAnswer(sgdbOneHero)(w, r)
			},
		})
		if _, err := sgdbNew(srv, nil).Media(context.Background(), sgdbGameID); err != nil {
			t.Fatalf("Media: %v", err)
		}
		if gridHits.Load() != 1 {
			t.Errorf("grids requests = %d, want 1", gridHits.Load())
		}
	})

	t.Run("MET-013/heroes sem filtro de dimensões", func(t *testing.T) {
		t.Parallel()
		routes, _, heroHits := sgdbRoutes(t, sgdbTwoGrids, sgdbOneHero)
		srv, _ := newServer(t, routes)
		if _, err := sgdbNew(srv, nil).Media(context.Background(), sgdbGameID); err != nil {
			t.Fatalf("Media: %v", err)
		}
		if heroHits.Load() != 1 {
			t.Errorf("heroes requests = %d, want 1", heroHits.Load())
		}
	})

	t.Run("MET-013/capas viram cover e heróis viram background", func(t *testing.T) {
		t.Parallel()
		routes, _, _ := sgdbRoutes(t, sgdbTwoGrids, sgdbOneHero)
		srv, _ := newServer(t, routes)
		media, err := sgdbNew(srv, nil).Media(context.Background(), sgdbGameID)
		if err != nil {
			t.Fatalf("Media: %v", err)
		}
		want := []scrapers.RemoteMedia{
			{Kind: scrapers.KindCover, URL: "https://cdn.example.test/grid1.png"},
			{Kind: scrapers.KindCover, URL: "https://cdn.example.test/grid2.png"},
			{Kind: scrapers.KindBackground, URL: "https://cdn.example.test/hero1.jpg"},
		}
		if len(media) != len(want) {
			t.Fatalf("Media = %+v, want %+v", media, want)
		}
		for i := range want {
			if media[i] != want[i] {
				t.Errorf("Media[%d] = %+v, want %+v", i, media[i], want[i])
			}
		}
	})

	t.Run("MET-013/nenhum item screenshot", func(t *testing.T) {
		t.Parallel()
		routes, _, _ := sgdbRoutes(t, sgdbTwoGrids, sgdbOneHero)
		srv, _ := newServer(t, routes)
		media, err := sgdbNew(srv, nil).Media(context.Background(), sgdbGameID)
		if err != nil {
			t.Fatalf("Media: %v", err)
		}
		for _, m := range media {
			if m.Kind == scrapers.KindScreenshot {
				t.Errorf("screenshot in Media: %+v", m)
			}
			if m.Kind != scrapers.KindCover && m.Kind != scrapers.KindBackground {
				t.Errorf("unexpected kind %q in Media: %+v", m.Kind, m)
			}
		}
	})

	t.Run("MET-013/sem chave: ErrNoKey e nenhuma requisição", func(t *testing.T) {
		t.Parallel()
		srv, rt := newServer(t, map[string]func(http.ResponseWriter, *http.Request){})
		p := scrapers.NewSteamGridDB(scrapers.SGDBConfig{APIBase: srv.URL, HTTP: srv.Client()})
		if _, err := p.Media(context.Background(), sgdbGameID); !errors.Is(err, scrapers.ErrNoKey) {
			t.Errorf("err = %v, want ErrNoKey", err)
		}
		if n := rt.calls.Load(); n != 0 {
			t.Errorf("requests = %d, want 0", n)
		}
	})

	t.Run("MET-013/falha em heroes após grids devolve erro (nada parcial)", func(t *testing.T) {
		t.Parallel()
		routes, _, _ := sgdbRoutes(t, sgdbTwoGrids, sgdbOneHero)
		routes["GET /heroes/game/"+sgdbGameID] = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}
		srv, _ := newServer(t, routes)
		media, err := sgdbNew(srv, nil).Media(context.Background(), sgdbGameID)
		if !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("err = %v, want ErrUnavailable", err)
		}
		if len(media) != 0 {
			t.Errorf("Media = %+v, want nothing (capas não podem sair sem os heróis)", media)
		}
	})

	t.Run("MET-013/falha em grids (429) devolve erro e não pede heroes", func(t *testing.T) {
		t.Parallel()
		routes, _, heroHits := sgdbRoutes(t, sgdbTwoGrids, sgdbOneHero)
		routes["GET /grids/game/"+sgdbGameID] = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}
		srv, _ := newServer(t, routes)
		if _, err := sgdbNew(srv, nil).Media(context.Background(), sgdbGameID); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("err = %v, want ErrUnavailable", err)
		}
		if heroHits.Load() != 0 {
			t.Errorf("heroes requests = %d, want 0", heroHits.Load())
		}
	})

	t.Run("MET-013/Search e Details continuam como antes", func(t *testing.T) {
		t.Parallel()
		search, _ := sgdbAnswer(t, `{"success":true,"data":[{"id":123,"name":"Celeste","release_date":1514764800}]}`, nil)
		srv, rt := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"GET /search/autocomplete/Celeste": search,
		})
		p := sgdbNew(srv, nil)
		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste"})
		if err != nil || len(cands) != 1 || cands[0].Provider != "steamgriddb" || cands[0].ProviderID != "123" ||
			cands[0].Title != "Celeste" || cands[0].Year != 2018 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		meta, err := p.Details(context.Background(), sgdbGameID)
		if err != nil || !reflect.DeepEqual(meta, scrapers.Metadata{}) {
			t.Errorf("Details = %+v, %v; want vazio sem erro", meta, err)
		}
		if n := rt.calls.Load(); n != 1 {
			t.Errorf("requests = %d, want 1 (só a busca; Details não chama a rede)", n)
		}
	})
}

func TestSteamGridDBLimiter(t *testing.T) {
	t.Parallel()

	t.Run("MET-010/as duas requisições passam pelo Limiter", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(sgdbStart)
		heroAt := make(chan time.Time, 1)
		gridH, _ := sgdbAnswer(t, sgdbTwoGrids, sgdbGridsCheck(t))
		heroH, _ := sgdbAnswer(t, sgdbOneHero, sgdbHeroesCheck(t))
		srv, rt := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"GET /grids/game/" + sgdbGameID: gridH,
			"GET /heroes/game/" + sgdbGameID: func(w http.ResponseWriter, r *http.Request) {
				heroAt <- clock.Now()
				heroH(w, r)
			},
		})
		ctx := guardContext(t)
		p := sgdbNew(srv, scrapers.NewLimiter(clock, scrapers.IntervalSteamGridDB))

		done := make(chan error, 1)
		go func() {
			_, err := p.Media(context.Background(), sgdbGameID)
			done <- err
		}()
		if err := clock.BlockUntilWaiters(ctx, 1); err != nil {
			t.Fatalf("heroes não registrou espera: %v", err)
		}
		if n := rt.calls.Load(); n != 1 {
			t.Fatalf("antes do intervalo: requisições = %d, want 1 (só grids)", n)
		}
		clock.Advance(scrapers.IntervalSteamGridDB - time.Millisecond)
		expectBlocked(t, done)
		clock.Advance(time.Millisecond)
		if err := expectReturn(t, done); err != nil {
			t.Fatalf("Media: %v", err)
		}
		want := sgdbStart.Add(scrapers.IntervalSteamGridDB)
		if got := <-heroAt; !got.Equal(want) {
			t.Errorf("heroes saiu em %v; want %v (horário reservado pelo Limiter)", got, want)
		}
	})

	t.Run("MET-010/cancelamento durante a espera de heroes devolve ctx.Err() e não pede heroes", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(sgdbStart)
		routes, _, heroHits := sgdbRoutes(t, sgdbTwoGrids, sgdbOneHero)
		srv, rt := newServer(t, routes)
		guard := guardContext(t)
		p := sgdbNew(srv, scrapers.NewLimiter(clock, scrapers.IntervalSteamGridDB))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := p.Media(ctx, sgdbGameID)
			done <- err
		}()
		if err := clock.BlockUntilWaiters(guard, 1); err != nil {
			t.Fatalf("heroes não registrou espera: %v", err)
		}
		cancel()
		if err := expectReturn(t, done); !errors.Is(err, context.Canceled) {
			t.Errorf("Media cancelado: err = %v; want context.Canceled", err)
		}
		if n := rt.calls.Load(); n != 1 {
			t.Errorf("requests = %d, want 1 (só grids)", n)
		}
		if heroHits.Load() != 0 {
			t.Errorf("heroes requests = %d, want 0", heroHits.Load())
		}
	})

	t.Run("MET-010/Search também espera no Limiter", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(sgdbStart)
		searchAt := make(chan time.Time, 2)
		search, searchHits := sgdbAnswer(t, `{"success":true,"data":[]}`, nil)
		srv, _ := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"GET /search/autocomplete/Celeste": func(w http.ResponseWriter, r *http.Request) {
				searchAt <- clock.Now()
				search(w, r)
			},
		})
		ctx := guardContext(t)
		p := sgdbNew(srv, scrapers.NewLimiter(clock, scrapers.IntervalSteamGridDB))

		if _, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste"}); err != nil {
			t.Fatalf("primeira busca: %v", err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste"})
			done <- err
		}()
		if err := clock.BlockUntilWaiters(ctx, 1); err != nil {
			t.Fatalf("segunda busca não registrou espera: %v", err)
		}
		if n := searchHits.Load(); n != 1 {
			t.Fatalf("antes do intervalo: buscas = %d, want 1", n)
		}
		clock.Advance(scrapers.IntervalSteamGridDB - time.Millisecond)
		expectBlocked(t, done)
		clock.Advance(time.Millisecond)
		if err := expectReturn(t, done); err != nil {
			t.Fatalf("segunda busca: %v", err)
		}
		<-searchAt
		if got, want := <-searchAt, sgdbStart.Add(scrapers.IntervalSteamGridDB); !got.Equal(want) {
			t.Errorf("segunda busca saiu em %v; want %v", got, want)
		}
	})
}

// TestSteamGridDBLeavesNoGoroutine is sequential on purpose: goleak must not see goroutines of parallel siblings.
func TestSteamGridDBLeavesNoGoroutine(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("MET-013/Media sem goroutine nem conexão restante depois de duas requisições", func(t *testing.T) {
		routes, _, _ := sgdbRoutes(t, sgdbTwoGrids, sgdbOneHero)
		srv, _ := newServer(t, routes)
		t.Cleanup(srv.Client().CloseIdleConnections)
		if _, err := sgdbNew(srv, nil).Media(context.Background(), sgdbGameID); err != nil {
			t.Fatalf("Media: %v", err)
		}
	})
}
