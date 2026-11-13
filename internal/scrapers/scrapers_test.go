package scrapers_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// route answers requests by "METHOD path" and records the headers of the last call.
type route struct {
	t       *testing.T
	answers map[string]func(w http.ResponseWriter, r *http.Request)
	calls   atomic.Int32
}

func newServer(t *testing.T, answers map[string]func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *route) {
	t.Helper()
	rt := &route{t: t, answers: answers}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt.calls.Add(1)
		h, ok := rt.answers[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, rt
}

func jsonAnswer(body string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

func TestIGDB(t *testing.T) {
	t.Parallel()

	t.Run("MID-002/busca e detalhes com Client-ID e token Bearer", func(t *testing.T) {
		t.Parallel()
		srv, _ := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"POST /oauth2/token": jsonAnswer(`{"access_token":"tok","expires_in":3600}`),
			"POST /v4/games": func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Client-ID") != "cid" || r.Header.Get("Authorization") != "Bearer tok" {
					t.Errorf("headers = %v", r.Header)
				}
				body, _ := io.ReadAll(r.Body)
				if strings.Contains(string(body), "search") {
					_, _ = io.WriteString(w, `[{"id":7,"name":"Celeste","first_release_date":1514764800,"platforms":[{"name":"PC (Microsoft Windows)"}]}]`)
					return
				}
				_, _ = io.WriteString(w, `[{"id":7,"name":"Celeste","summary":"Escalada.","genres":[{"name":"Platform"}],"game_modes":[{"name":"Single player"}],"involved_companies":[{"developer":true,"company":{"name":"Maddy Makes Games"}},{"publisher":true,"company":{"name":"Matt Makes Games"}}],"screenshots":[{"image_id":"abc"}]}]`)
			},
		})
		p := scrapers.NewIGDB(scrapers.IGDBConfig{ClientID: "cid", Secret: "sec", APIBase: srv.URL + "/v4", TokenURL: srv.URL + "/oauth2/token", HTTP: srv.Client()})

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste", Platform: "PC", Year: 2018})
		if err != nil || len(cands) != 1 || cands[0].ProviderID != "7" || cands[0].Title != "Celeste" || cands[0].Year != 2018 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		meta, err := p.Details(context.Background(), "7")
		if err != nil {
			t.Fatalf("Details: %v", err)
		}
		if meta.Description != "Escalada." || len(meta.Developers) != 1 || meta.Developers[0] != "Maddy Makes Games" ||
			len(meta.Publishers) != 1 || meta.Publishers[0] != "Matt Makes Games" || meta.Genres[0] != "Platform" {
			t.Errorf("Details = %+v", meta)
		}
		media, err := p.Media(context.Background(), "7")
		if err != nil || len(media) != 1 || media[0].Kind != "screenshot" || !strings.HasSuffix(media[0].URL, "/abc.jpg") {
			t.Errorf("Media = %+v, %v", media, err)
		}
	})

	t.Run("MID-002/o token é pedido uma vez e reaproveitado", func(t *testing.T) {
		t.Parallel()
		var tokens atomic.Int32
		srv, _ := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"POST /oauth2/token": func(w http.ResponseWriter, r *http.Request) {
				tokens.Add(1)
				jsonAnswer(`{"access_token":"tok","expires_in":3600}`)(w, r)
			},
			"POST /v4/games": jsonAnswer(`[]`),
		})
		p := scrapers.NewIGDB(scrapers.IGDBConfig{ClientID: "cid", Secret: "sec", APIBase: srv.URL + "/v4", TokenURL: srv.URL + "/oauth2/token", HTTP: srv.Client()})
		for range 2 {
			if _, err := p.Search(context.Background(), scrapers.Query{Title: "x"}); err != nil {
				t.Fatalf("Search: %v", err)
			}
		}
		if n := tokens.Load(); n != 1 {
			t.Errorf("token requests = %d, want 1", n)
		}
	})

	t.Run("MID-001/sem chave não há chamada e o erro é ErrNoKey", func(t *testing.T) {
		t.Parallel()
		srv, rt := newServer(t, map[string]func(http.ResponseWriter, *http.Request){})
		p := scrapers.NewIGDB(scrapers.IGDBConfig{APIBase: srv.URL, TokenURL: srv.URL, HTTP: srv.Client()})
		if _, err := p.Search(context.Background(), scrapers.Query{Title: "x"}); !errors.Is(err, scrapers.ErrNoKey) {
			t.Errorf("err = %v, want ErrNoKey", err)
		}
		if n := rt.calls.Load(); n != 0 {
			t.Errorf("requests = %d, want 0", n)
		}
	})

	t.Run("MID-008/429 é erro temporário", func(t *testing.T) {
		t.Parallel()
		srv, _ := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"POST /oauth2/token": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
		})
		p := scrapers.NewIGDB(scrapers.IGDBConfig{ClientID: "cid", Secret: "sec", APIBase: srv.URL, TokenURL: srv.URL + "/oauth2/token", HTTP: srv.Client()})
		if _, err := p.Search(context.Background(), scrapers.Query{Title: "x"}); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("err = %v, want ErrUnavailable", err)
		}
	})
}

func TestSteamGridDB(t *testing.T) {
	t.Parallel()

	t.Run("MID-002/busca e capas com cabeçalho Bearer", func(t *testing.T) {
		t.Parallel()
		srv, _ := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"GET /search/autocomplete/Celeste": func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer key" {
					t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
				}
				jsonAnswer(`{"success":true,"data":[{"id":123,"name":"Celeste","release_date":1514764800}]}`)(w, r)
			},
			"GET /grids/game/123": jsonAnswer(`{"success":true,"data":[{"url":"https://cdn.example.test/grid.png","width":600,"height":900}]}`),
		})
		p := scrapers.NewSteamGridDB(scrapers.SGDBConfig{Key: "key", APIBase: srv.URL, HTTP: srv.Client()})

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste"})
		if err != nil || len(cands) != 1 || cands[0].ProviderID != "123" || cands[0].Year != 2018 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		media, err := p.Media(context.Background(), "123")
		if err != nil || len(media) != 1 || media[0].Kind != "cover" || media[0].URL != "https://cdn.example.test/grid.png" {
			t.Errorf("Media = %+v, %v", media, err)
		}
	})

	t.Run("MID-001/sem chave não há chamada", func(t *testing.T) {
		t.Parallel()
		srv, rt := newServer(t, map[string]func(http.ResponseWriter, *http.Request){})
		p := scrapers.NewSteamGridDB(scrapers.SGDBConfig{APIBase: srv.URL, HTTP: srv.Client()})
		if _, err := p.Search(context.Background(), scrapers.Query{Title: "x"}); !errors.Is(err, scrapers.ErrNoKey) {
			t.Errorf("err = %v, want ErrNoKey", err)
		}
		if rt.calls.Load() != 0 {
			t.Error("request sent without a key")
		}
	})

	t.Run("MID-008/500 é erro temporário", func(t *testing.T) {
		t.Parallel()
		srv, _ := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"GET /search/autocomplete/x": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		})
		p := scrapers.NewSteamGridDB(scrapers.SGDBConfig{Key: "key", APIBase: srv.URL, HTTP: srv.Client()})
		if _, err := p.Search(context.Background(), scrapers.Query{Title: "x"}); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("err = %v, want ErrUnavailable", err)
		}
	})
}

func TestSteamStore(t *testing.T) {
	t.Parallel()

	t.Run("MID-002/busca e detalhes sem chave", func(t *testing.T) {
		t.Parallel()
		srv, _ := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"GET /api/storesearch/": jsonAnswer(`{"total":1,"items":[{"id":504230,"name":"Celeste"}]}`),
			"GET /api/appdetails": jsonAnswer(`{"504230":{"success":true,"data":{"name":"Celeste","short_description":"Escalada.",` +
				`"developers":["Maddy Makes Games"],"publishers":["Matt Makes Games"],"genres":[{"description":"Indie"}],` +
				`"release_date":{"date":"Jan 25, 2018"},"screenshots":[{"path_full":"https://cdn.example.test/s1.jpg"}]}}}`),
		})
		p := scrapers.NewSteamStore(scrapers.SteamConfig{APIBase: srv.URL, HTTP: srv.Client()})

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Celeste"})
		if err != nil || len(cands) != 1 || cands[0].ProviderID != "504230" || cands[0].Platform != "PC" {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		meta, err := p.Details(context.Background(), "504230")
		if err != nil || meta.Description != "Escalada." || meta.Year != 2018 || meta.Genres[0] != "Indie" {
			t.Errorf("Details = %+v, %v", meta, err)
		}
		media, err := p.Media(context.Background(), "504230")
		if err != nil || len(media) != 1 || media[0].Kind != "screenshot" {
			t.Errorf("Media = %+v, %v", media, err)
		}
	})

	t.Run("MID-008/429 é erro temporário", func(t *testing.T) {
		t.Parallel()
		srv, _ := newServer(t, map[string]func(http.ResponseWriter, *http.Request){
			"GET /api/storesearch/": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
		})
		p := scrapers.NewSteamStore(scrapers.SteamConfig{APIBase: srv.URL, HTTP: srv.Client()})
		if _, err := p.Search(context.Background(), scrapers.Query{Title: "x"}); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("err = %v, want ErrUnavailable", err)
		}
	})
}
