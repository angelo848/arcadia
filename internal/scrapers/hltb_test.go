package scrapers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

var hlStart = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

const hlGuardTimeout = 3 * time.Second

// hlSecretBody marks a response body that must never be repeated in an error.
const hlSecretBody = "CORPO-SECRETO-DO-SERVIDOR"

type hlRec struct {
	method, path, rawQuery string
	header                 http.Header
	body                   []byte
}

// hlFake is a local HowLongToBeat: it records every request and answers through the two handlers.
type hlFake struct {
	srv *httptest.Server

	mu     sync.Mutex
	recs   []hlRec
	inits  int
	search int

	// initFn answers the n-th init (1-based); searchFn answers the n-th search (1-based).
	initFn   func(n int, w http.ResponseWriter, r *http.Request)
	searchFn func(n int, w http.ResponseWriter, r *http.Request)
}

func hlTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "hltb", name))
	if err != nil {
		t.Fatalf("testdata: %v", err)
	}
	return string(b)
}

func hlWriteJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func newHLFake(t *testing.T) *hlFake {
	t.Helper()
	f := &hlFake{}
	f.initFn = func(n int, w http.ResponseWriter, _ *http.Request) {
		hlWriteJSON(w, 200, fmt.Sprintf(`{"token":"tok-%d"}`, n))
	}
	f.searchFn = func(_ int, w http.ResponseWriter, _ *http.Request) {
		hlWriteJSON(w, 200, hlTestdata(t, "search-results.json"))
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.recs = append(f.recs, hlRec{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body})
		var n int
		isInit := r.URL.Path == scrapers.HLTBInitPath
		if isInit {
			f.inits++
			n = f.inits
		} else {
			f.search++
			n = f.search
		}
		initFn, searchFn := f.initFn, f.searchFn
		f.mu.Unlock()
		switch r.URL.Path {
		case scrapers.HLTBInitPath:
			initFn(n, w, r)
		case scrapers.HLTBSearchPath:
			searchFn(n, w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *hlFake) records() []hlRec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]hlRec(nil), f.recs...)
}

// kinds is the request sequence as "init" / "search".
func (f *hlFake) kinds() []string {
	var out []string
	for _, r := range f.records() {
		if r.path == scrapers.HLTBInitPath {
			out = append(out, "init")
		} else {
			out = append(out, "search")
		}
	}
	return out
}

func (f *hlFake) client(t *testing.T, cfg scrapers.HLTBConfig) *scrapers.HLTB {
	t.Helper()
	cfg.BaseURL = f.srv.URL
	if cfg.HTTP == nil {
		cfg.HTTP = f.srv.Client()
	}
	if cfg.Clock == nil {
		cfg.Clock = domain.NewFakeClock(hlStart)
	}
	return scrapers.NewHLTB(cfg)
}

func hlCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), hlGuardTimeout)
	t.Cleanup(cancel)
	return ctx
}

type hlResult struct {
	games []scrapers.HLTBGame
	err   error
}

// hlDrive runs Search in a goroutine and advances the fake clock 2 s for each of the waits expected from the Limiter.
func hlDrive(t *testing.T, clock *domain.FakeClock, h *scrapers.HLTB, ctx context.Context, term string, waits int) hlResult {
	t.Helper()
	ch := make(chan hlResult, 1)
	go func() {
		g, err := h.Search(ctx, term)
		ch <- hlResult{g, err}
	}()
	for i := 0; i < waits; i++ {
		if err := clock.BlockUntilWaiters(hlCtx(t), 1); err != nil {
			t.Fatalf("wait %d of %d for the limiter never started: %v", i+1, waits, err)
		}
		clock.Advance(scrapers.HLTBInterval)
	}
	select {
	case r := <-ch:
		return r
	case <-time.After(hlGuardTimeout):
		t.Fatal("Search did not return")
		return hlResult{}
	}
}

func hlSearch(t *testing.T, h *scrapers.HLTB, term string) ([]scrapers.HLTBGame, error) {
	t.Helper()
	return h.Search(hlCtx(t), term)
}

// hlLeaks lists secrets found in an error text.
func hlLeaks(err error, extra ...string) []string {
	var out []string
	msg := err.Error()
	for _, s := range append([]string{"127.0.0.1", "http://", "tok-", hlSecretBody}, extra...) {
		if strings.Contains(msg, s) {
			out = append(out, s)
		}
	}
	return out
}

func TestHLTBConstants(t *testing.T) {
	t.Parallel()
	t.Run("TTB-010/constantes de rules §1", func(t *testing.T) {
		t.Parallel()
		if scrapers.HLTBInterval != 2*time.Second {
			t.Errorf("HLTBInterval = %v", scrapers.HLTBInterval)
		}
		if scrapers.HLTBRequestTimeout != 10*time.Second {
			t.Errorf("HLTBRequestTimeout = %v", scrapers.HLTBRequestTimeout)
		}
		if scrapers.HLTBPageSize != 20 || scrapers.HLTBMaxBody != 2<<20 {
			t.Errorf("page size %d, max body %d", scrapers.HLTBPageSize, scrapers.HLTBMaxBody)
		}
		if scrapers.HLTBInitPath != "/api/search/site/init" || scrapers.HLTBSearchPath != "/api/search/site" {
			t.Errorf("paths %q %q", scrapers.HLTBInitPath, scrapers.HLTBSearchPath)
		}
	})
}

func TestHLTBToken(t *testing.T) {
	t.Parallel()

	t.Run("TTB-012/sem token guardado faz GET init com t do relógio e depois POST search com x-auth-token", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		clock := domain.NewFakeClock(hlStart)
		h := f.client(t, scrapers.HLTBConfig{Clock: clock})
		if _, err := hlSearch(t, h, "hollow knight"); err != nil {
			t.Fatalf("Search: %v", err)
		}
		recs := f.records()
		if len(recs) != 2 {
			t.Fatalf("requests = %d, want 2 (%v)", len(recs), f.kinds())
		}
		in, se := recs[0], recs[1]
		if in.method != http.MethodGet || in.path != scrapers.HLTBInitPath {
			t.Errorf("first request = %s %s", in.method, in.path)
		}
		if want := "t=" + strconv.FormatInt(hlStart.UnixMilli(), 10); in.rawQuery != want {
			t.Errorf("init query = %q, want %q", in.rawQuery, want)
		}
		if se.method != http.MethodPost || se.path != scrapers.HLTBSearchPath {
			t.Errorf("second request = %s %s", se.method, se.path)
		}
		if got := se.header.Get("x-auth-token"); got != "tok-1" {
			t.Errorf("x-auth-token = %q, want tok-1", got)
		}
	})

	t.Run("TTB-012/a segunda busca reusa o token guardado sem novo init", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		h := f.client(t, scrapers.HLTBConfig{})
		for i := 0; i < 2; i++ {
			if _, err := hlSearch(t, h, "hollow knight"); err != nil {
				t.Fatalf("Search %d: %v", i, err)
			}
		}
		if want := []string{"init", "search", "search"}; !reflect.DeepEqual(f.kinds(), want) {
			t.Fatalf("sequence = %v, want %v", f.kinds(), want)
		}
		for _, r := range f.records()[1:] {
			if r.header.Get("x-auth-token") != "tok-1" {
				t.Errorf("search token = %q", r.header.Get("x-auth-token"))
			}
		}
	})

	t.Run("TTB-012/403 na busca descarta o token, faz init novo e repete a busca uma única vez", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		f.searchFn = func(n int, w http.ResponseWriter, _ *http.Request) {
			if n == 1 {
				hlWriteJSON(w, 403, `{}`)
				return
			}
			hlWriteJSON(w, 200, hlTestdata(t, "search-results.json"))
		}
		h := f.client(t, scrapers.HLTBConfig{})
		games, err := hlSearch(t, h, "hollow knight")
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(games) == 0 {
			t.Error("no games after the retry")
		}
		if want := []string{"init", "search", "init", "search"}; !reflect.DeepEqual(f.kinds(), want) {
			t.Fatalf("sequence = %v, want %v", f.kinds(), want)
		}
		recs := f.records()
		if recs[1].header.Get("x-auth-token") != "tok-1" || recs[3].header.Get("x-auth-token") != "tok-2" {
			t.Errorf("tokens = %q then %q", recs[1].header.Get("x-auth-token"), recs[3].header.Get("x-auth-token"))
		}
		// The new token stays for the next search.
		if _, err := hlSearch(t, h, "hollow knight"); err != nil {
			t.Fatal(err)
		}
		last := f.records()[4]
		if last.path != scrapers.HLTBSearchPath || last.header.Get("x-auth-token") != "tok-2" {
			t.Errorf("fifth request = %s token %q", last.path, last.header.Get("x-auth-token"))
		}
	})

	t.Run("TTB-012/403 de novo depois da renovação é ErrUnavailable sem terceira tentativa", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		f.searchFn = func(_ int, w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 403, `{}`) }
		h := f.client(t, scrapers.HLTBConfig{})
		_, err := hlSearch(t, h, "hollow knight")
		if !errors.Is(err, scrapers.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if want := []string{"init", "search", "init", "search"}; !reflect.DeepEqual(f.kinds(), want) {
			t.Fatalf("sequence = %v, want %v", f.kinds(), want)
		}
	})

	t.Run("TTB-012/falha ao obter o token novo depois do 403 é ErrUnavailable", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		f.searchFn = func(_ int, w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 403, `{}`) }
		f.initFn = func(n int, w http.ResponseWriter, _ *http.Request) {
			if n == 1 {
				hlWriteJSON(w, 200, `{"token":"tok-1"}`)
				return
			}
			hlWriteJSON(w, 500, `{}`)
		}
		h := f.client(t, scrapers.HLTBConfig{})
		if _, err := hlSearch(t, h, "x"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if want := []string{"init", "search", "init"}; !reflect.DeepEqual(f.kinds(), want) {
			t.Fatalf("sequence = %v, want %v", f.kinds(), want)
		}
	})

	initFailures := []struct {
		name string
		fn   func(w http.ResponseWriter, r *http.Request)
	}{
		{"status 500", func(w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 500, `{"token":"tok-x"}`) }},
		{"status 404", func(w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 404, `{"token":"tok-x"}`) }},
		{"status 201", func(w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 201, `{"token":"tok-x"}`) }},
		{"JSON inválido", func(w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 200, `{"token":`) }},
		{"HTML no lugar de JSON", func(w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 200, `<html>`+hlSecretBody+`</html>`) }},
		{"sem token", func(w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 200, `{"other":"x"}`) }},
		{"token vazio", func(w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 200, `{"token":""}`) }},
		{"token não texto", func(w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 200, `{"token":123}`) }},
	}
	for _, tc := range initFailures {
		t.Run("TTB-012/init com "+tc.name+" é ErrUnavailable e a busca não é feita", func(t *testing.T) {
			t.Parallel()
			f := newHLFake(t)
			f.initFn = func(_ int, w http.ResponseWriter, r *http.Request) { tc.fn(w, r) }
			h := f.client(t, scrapers.HLTBConfig{})
			_, err := hlSearch(t, h, "hollow knight")
			if !errors.Is(err, scrapers.ErrUnavailable) {
				t.Fatalf("err = %v, want ErrUnavailable", err)
			}
			if want := []string{"init"}; !reflect.DeepEqual(f.kinds(), want) {
				t.Errorf("sequence = %v, want %v", f.kinds(), want)
			}
			if l := hlLeaks(err); len(l) > 0 {
				t.Errorf("error leaks %v: %v", l, err)
			}
		})
	}

	t.Run("TTB-012/init com rede falhando é ErrUnavailable", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.NotFoundHandler())
		base := srv.URL
		srv.Close()
		h := scrapers.NewHLTB(scrapers.HLTBConfig{BaseURL: base, Clock: domain.NewFakeClock(hlStart)})
		_, err := h.Search(hlCtx(t), "hollow knight")
		if !errors.Is(err, scrapers.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if l := hlLeaks(err); len(l) > 0 {
			t.Errorf("error leaks %v: %v", l, err)
		}
	})

	t.Run("TTB-012/404 na busca é ErrUnavailable sem novo init nem nova tentativa", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		f.searchFn = func(_ int, w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 404, `{}`) }
		h := f.client(t, scrapers.HLTBConfig{})
		if _, err := hlSearch(t, h, "hollow knight"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if want := []string{"init", "search"}; !reflect.DeepEqual(f.kinds(), want) {
			t.Errorf("sequence = %v, want %v", f.kinds(), want)
		}
	})

	// Not parallel: it swaps the process-wide slog default.
	t.Run("TTB-012/o token não aparece no erro devolvido nem no log capturado", func(t *testing.T) {
		var buf bytes.Buffer
		old := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
		t.Cleanup(func() { slog.SetDefault(old) })

		f := newHLFake(t)
		f.searchFn = func(n int, w http.ResponseWriter, _ *http.Request) {
			if n == 1 {
				hlWriteJSON(w, 403, `{}`)
				return
			}
			hlWriteJSON(w, 500, hlSecretBody)
		}
		h := f.client(t, scrapers.HLTBConfig{})
		_, err := hlSearch(t, h, "hollow knight")
		if err == nil {
			t.Fatal("expected an error")
		}
		if strings.Contains(err.Error(), "tok-") {
			t.Errorf("error has the token: %v", err)
		}
		if strings.Contains(buf.String(), "tok-") {
			t.Errorf("log has the token: %s", buf.String())
		}
	})
}

func TestHLTBParse(t *testing.T) {
	t.Parallel()

	serve := func(t *testing.T, body string) ([]scrapers.HLTBGame, error) {
		t.Helper()
		f := newHLFake(t)
		f.searchFn = func(_ int, w http.ResponseWriter, _ *http.Request) { hlWriteJSON(w, 200, body) }
		return hlSearch(t, f.client(t, scrapers.HLTBConfig{}), "x")
	}
	item := func(extra string) string {
		return `{"data":[{"game_id":7,"game_name":"Jogo","game_type":"game"` + extra + `}]}`
	}

	t.Run("TTB-005/a resposta real é lida só por data; campos extras são ignorados e dlc sai", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		games, err := hlSearch(t, f.client(t, scrapers.HLTBConfig{}), "hollow knight")
		if err != nil {
			t.Fatal(err)
		}
		want := []scrapers.HLTBGame{
			{ID: 26286, Name: "Hollow Knight", Aliases: []string{"Hollow Knight: Voidheart Edition"}, Year: 2017,
				Platforms: []string{"Linux", "Mac", "Nintendo Switch", "Nintendo Switch 2", "PC", "PlayStation 4", "Xbox One"},
				Main:      97159, MainExtra: 149918, Completionist: 236144},
			{ID: 65945, Name: "Hollow Knight: Silksong", Year: 2025,
				Platforms: []string{"Linux", "Mac", "Nintendo Switch", "Nintendo Switch 2", "PC", "PlayStation 4", "PlayStation 5", "Xbox One", "Xbox Series X/S"},
				Main:      101611, MainExtra: 170541, Completionist: 236426},
		}
		if len(games) != len(want) {
			t.Fatalf("games = %d, want %d: %+v", len(games), len(want), games)
		}
		for i := range want {
			g := games[i]
			if len(g.Aliases) == 0 {
				g.Aliases = nil
			}
			if !reflect.DeepEqual(g, want[i]) {
				t.Errorf("game %d = %+v, want %+v", i, g, want[i])
			}
		}
	})

	t.Run("TTB-005/comp_main, comp_plus e comp_100 viram segundos; zero, ausente, negativo ou não inteiro viram 0 sem afetar os outros", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name, extra string
			main, plus  int64
			full        int64
		}{
			{"todos", `,"comp_main":100,"comp_plus":200,"comp_100":300`, 100, 200, 300},
			{"zero", `,"comp_main":0,"comp_plus":200,"comp_100":300`, 0, 200, 300},
			{"ausente", `,"comp_plus":200`, 0, 200, 0},
			{"negativo", `,"comp_main":-5,"comp_plus":200,"comp_100":300`, 0, 200, 300},
			{"não inteiro", `,"comp_main":12.5,"comp_plus":200,"comp_100":300`, 0, 200, 300},
			{"texto", `,"comp_main":100,"comp_plus":"200","comp_100":300`, 100, 0, 300},
			{"nulo", `,"comp_main":null,"comp_plus":200,"comp_100":null`, 0, 200, 0},
		}
		for _, tc := range cases {
			games, err := serve(t, item(tc.extra))
			if err != nil || len(games) != 1 {
				t.Fatalf("%s: games %v err %v", tc.name, games, err)
			}
			g := games[0]
			if g.Main != tc.main || g.MainExtra != tc.plus || g.Completionist != tc.full {
				t.Errorf("%s: got %d/%d/%d, want %d/%d/%d", tc.name, g.Main, g.MainExtra, g.Completionist, tc.main, tc.plus, tc.full)
			}
		}
	})

	t.Run("TTB-005/game_id ausente ou ≤ 0, game_name vazio e game_type diferente de game são ignorados", func(t *testing.T) {
		t.Parallel()
		body := `{"data":[` +
			`{"game_name":"Sem id","game_type":"game"},` +
			`{"game_id":0,"game_name":"Zero","game_type":"game"},` +
			`{"game_id":-3,"game_name":"Negativo","game_type":"game"},` +
			`{"game_id":4,"game_name":"","game_type":"game"},` +
			`{"game_id":5,"game_name":"DLC","game_type":"dlc"},` +
			`{"game_id":6,"game_name":"Mod","game_type":"mod"},` +
			`{"game_id":8,"game_name":"Sem tipo"},` +
			`{"game_id":9,"game_name":"Bom","game_type":"game"}]}`
		games, err := serve(t, body)
		if err != nil {
			t.Fatal(err)
		}
		var ids []int64
		for _, g := range games {
			ids = append(ids, g.ID)
		}
		// "Sem tipo" has no game_type: the filter only applies when the field exists.
		if want := []int64{8, 9}; !reflect.DeepEqual(ids, want) {
			t.Errorf("ids = %v, want %v", ids, want)
		}
	})

	t.Run("TTB-005/release_world fora de 1950..2100 vira 0", func(t *testing.T) {
		t.Parallel()
		cases := map[string]int{`1949`: 0, `1950`: 1950, `2100`: 2100, `2101`: 0, `0`: 0, `-1`: 0}
		for in, want := range cases {
			games, err := serve(t, item(`,"release_world":`+in))
			if err != nil || len(games) != 1 {
				t.Fatalf("%s: %v %v", in, games, err)
			}
			if games[0].Year != want {
				t.Errorf("release_world %s -> %d, want %d", in, games[0].Year, want)
			}
		}
		games, _ := serve(t, item(``))
		if len(games) != 1 || games[0].Year != 0 {
			t.Errorf("absent release_world -> %+v", games)
		}
	})

	t.Run("TTB-005/game_alias e profile_platform separados por vírgula, sem vazios", func(t *testing.T) {
		t.Parallel()
		games, err := serve(t, item(`,"game_alias":"A, B ,, C","profile_platform":"PC,, Xbox One ,"`))
		if err != nil || len(games) != 1 {
			t.Fatalf("%v %v", games, err)
		}
		if want := []string{"A", "B", "C"}; !reflect.DeepEqual(games[0].Aliases, want) {
			t.Errorf("aliases = %q, want %q", games[0].Aliases, want)
		}
		if want := []string{"PC", "Xbox One"}; !reflect.DeepEqual(games[0].Platforms, want) {
			t.Errorf("platforms = %q, want %q", games[0].Platforms, want)
		}
	})

	t.Run("TTB-005/data vazia é resposta válida (sem resultados), não falha", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		f.searchFn = func(_ int, w http.ResponseWriter, _ *http.Request) {
			hlWriteJSON(w, 200, hlTestdata(t, "search-empty.json"))
		}
		games, err := hlSearch(t, f.client(t, scrapers.HLTBConfig{}), "Jogo Que Não Existe")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(games) != 0 {
			t.Errorf("games = %+v", games)
		}
	})
}

func TestHLTBRequest(t *testing.T) {
	t.Parallel()

	t.Run("TTB-003/o corpo da busca é o de rules §3 (year, gameplay sem difficulty, size 20, useCache true)", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		if _, err := hlSearch(t, f.client(t, scrapers.HLTBConfig{}), "hollow knight"); err != nil {
			t.Fatal(err)
		}
		const want = `{
  "searchType": "games",
  "searchTerms": ["hollow", "knight"],
  "searchPage": 1,
  "size": 20,
  "searchOptions": {
    "games": { "userId": 0, "platform": "", "sortCategory": "popular", "rangeCategory": "main",
               "rangeTime": { "min": null, "max": null },
               "gameplay": { "perspective": "", "flow": "", "genre": "" },
               "year": { "min": "", "max": "" }, "modifier": "" },
    "users": { "sortCategory": "postcount" }, "lists": { "sortCategory": "follows" },
    "filter": "", "sort": 0, "randomizer": 0
  },
  "useCache": true
}`
		var got, exp any
		if err := json.Unmarshal(f.records()[1].body, &got); err != nil {
			t.Fatalf("body is not JSON: %v", err)
		}
		if err := json.Unmarshal([]byte(want), &exp); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, exp) {
			t.Errorf("body = %s", f.records()[1].body)
		}
	})

	t.Run("TTB-003/searchTerms é o termo dividido por espaços, sem vazios, size 20", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			term string
			want []string
		}{
			{"hollow knight", []string{"hollow", "knight"}},
			{"  Super   Mario  World ", []string{"Super", "Mario", "World"}},
			{"Celeste", []string{"Celeste"}},
		}
		for _, tc := range cases {
			f := newHLFake(t)
			if _, err := hlSearch(t, f.client(t, scrapers.HLTBConfig{}), tc.term); err != nil {
				t.Fatal(err)
			}
			var b struct {
				SearchTerms []string `json:"searchTerms"`
				Size        int      `json:"size"`
			}
			if err := json.Unmarshal(f.records()[1].body, &b); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(b.SearchTerms, tc.want) || b.Size != 20 {
				t.Errorf("term %q -> %q size %d, want %q size 20", tc.term, b.SearchTerms, b.Size, tc.want)
			}
		}
	})

	t.Run("TTB-027/cabeçalhos fixos, sem Cookie nem Authorization, init só com t, mesmo com Jar no cliente recebido", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		f.initFn = func(n int, w http.ResponseWriter, _ *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "sess", Value: "abc", Path: "/"})
			hlWriteJSON(w, 200, fmt.Sprintf(`{"token":"tok-%d"}`, n))
		}
		jar, _ := cookiejar.New(nil)
		c := *f.srv.Client()
		c.Jar = jar
		h := f.client(t, scrapers.HLTBConfig{HTTP: &c})
		for i := 0; i < 2; i++ {
			if _, err := hlSearch(t, h, "Celeste"); err != nil {
				t.Fatal(err)
			}
		}
		recs := f.records()
		var ua string
		for i, r := range recs {
			if r.header.Get("Cookie") != "" || r.header.Get("Authorization") != "" {
				t.Errorf("request %d carries Cookie/Authorization: %v", i, r.header)
			}
			if r.header.Get("Accept") != "*/*" {
				t.Errorf("request %d Accept = %q", i, r.header.Get("Accept"))
			}
			if r.header.Get("Origin") != f.srv.URL || r.header.Get("Referer") != f.srv.URL+"/" {
				t.Errorf("request %d Origin/Referer = %q %q", i, r.header.Get("Origin"), r.header.Get("Referer"))
			}
			if r.header.Get("User-Agent") == "" || strings.Contains(r.header.Get("User-Agent"), "Go-http-client") {
				t.Errorf("request %d User-Agent = %q", i, r.header.Get("User-Agent"))
			}
			if ua == "" {
				ua = r.header.Get("User-Agent")
			} else if r.header.Get("User-Agent") != ua {
				t.Errorf("User-Agent changes between requests")
			}
			if strings.Contains(string(r.body), "Celeste") == false && r.path == scrapers.HLTBSearchPath {
				t.Errorf("search body lost the term")
			}
			if r.path == scrapers.HLTBInitPath {
				q := r.rawQuery
				if !strings.HasPrefix(q, "t=") || strings.ContainsAny(q, "&;") {
					t.Errorf("init query = %q, want only t", q)
				}
				if r.header.Get("x-auth-token") != "" {
					t.Errorf("init carries a token")
				}
			} else {
				if r.header.Get("Content-Type") != "application/json" || r.header.Get("x-auth-token") == "" {
					t.Errorf("search headers = %v", r.header)
				}
				if r.rawQuery != "" {
					t.Errorf("search query = %q", r.rawQuery)
				}
			}
		}
	})
}

func TestHLTBLimiterAndTimeout(t *testing.T) {
	t.Parallel()

	t.Run("TTB-010/init e busca passam pelo Limiter de 2 s", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		clock := domain.NewFakeClock(hlStart)
		h := f.client(t, scrapers.HLTBConfig{Clock: clock, Limiter: scrapers.NewLimiter(clock, scrapers.HLTBInterval)})
		ch := make(chan hlResult, 1)
		go func() {
			g, err := h.Search(hlCtx(t), "x")
			ch <- hlResult{g, err}
		}()
		if err := clock.BlockUntilWaiters(hlCtx(t), 1); err != nil {
			t.Fatalf("the search never waited for the limiter: %v", err)
		}
		if want := []string{"init"}; !reflect.DeepEqual(f.kinds(), want) {
			t.Fatalf("before the 2 s: %v, want %v", f.kinds(), want)
		}
		clock.Advance(scrapers.HLTBInterval - time.Millisecond)
		select {
		case <-ch:
			t.Fatal("search returned before the interval ended")
		default:
		}
		clock.Advance(time.Millisecond)
		select {
		case r := <-ch:
			if r.err != nil {
				t.Fatal(r.err)
			}
		case <-time.After(hlGuardTimeout):
			t.Fatal("search did not finish after the interval")
		}
	})

	t.Run("TTB-010/a busca repetida depois do 403 também espera o Limiter (4 requisições, 3 esperas)", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		f.searchFn = func(n int, w http.ResponseWriter, _ *http.Request) {
			if n == 1 {
				hlWriteJSON(w, 403, `{}`)
				return
			}
			hlWriteJSON(w, 200, hlTestdata(t, "search-results.json"))
		}
		clock := domain.NewFakeClock(hlStart)
		h := f.client(t, scrapers.HLTBConfig{Clock: clock, Limiter: scrapers.NewLimiter(clock, scrapers.HLTBInterval)})
		r := hlDrive(t, clock, h, hlCtx(t), "hollow knight", 3)
		if r.err != nil {
			t.Fatal(r.err)
		}
		if want := []string{"init", "search", "init", "search"}; !reflect.DeepEqual(f.kinds(), want) {
			t.Errorf("sequence = %v, want %v", f.kinds(), want)
		}
	})

	t.Run("TTB-010/servidor lento é abandonado pelo Timeout configurado e vira ErrUnavailable", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		f.initFn = func(_ int, w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }
		h := f.client(t, scrapers.HLTBConfig{Timeout: 50 * time.Millisecond})
		ctx := hlCtx(t)
		_, err := h.Search(ctx, "x")
		if !errors.Is(err, scrapers.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if ctx.Err() != nil {
			t.Fatal("the caller context ended: the timeout did not come from the config")
		}
	})

	t.Run("TTB-010/busca lenta depois do init também é abandonada pelo Timeout", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		f.searchFn = func(_ int, w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }
		h := f.client(t, scrapers.HLTBConfig{Timeout: 50 * time.Millisecond})
		if _, err := h.Search(hlCtx(t), "x"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})

	t.Run("TTB-010/ctx cancelado pelo chamador devolve ctx.Err() sem virar ErrUnavailable", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		h := f.client(t, scrapers.HLTBConfig{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := h.Search(ctx, "x")
		if !errors.Is(err, context.Canceled) || errors.Is(err, scrapers.ErrUnavailable) {
			t.Fatalf("err = %v, want context.Canceled and not ErrUnavailable", err)
		}
		if n := len(f.records()); n != 0 {
			t.Errorf("%d requests after cancel", n)
		}
	})

	t.Run("TTB-010/ctx cancelado enquanto espera o Limiter devolve ctx.Err()", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		clock := domain.NewFakeClock(hlStart)
		h := f.client(t, scrapers.HLTBConfig{Clock: clock, Limiter: scrapers.NewLimiter(clock, scrapers.HLTBInterval)})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch := make(chan error, 1)
		go func() { _, err := h.Search(ctx, "x"); ch <- err }()
		if err := clock.BlockUntilWaiters(hlCtx(t), 1); err != nil {
			t.Fatal(err)
		}
		cancel()
		select {
		case err := <-ch:
			if !errors.Is(err, context.Canceled) || errors.Is(err, scrapers.ErrUnavailable) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
		case <-time.After(hlGuardTimeout):
			t.Fatal("Search did not return after cancel")
		}
		if want := []string{"init"}; !reflect.DeepEqual(f.kinds(), want) {
			t.Errorf("sequence = %v, want %v", f.kinds(), want)
		}
	})
}

func TestHLTBFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		fn   func(w http.ResponseWriter)
	}{
		{"429", func(w http.ResponseWriter) { hlWriteJSON(w, 429, hlSecretBody) }},
		{"404", func(w http.ResponseWriter) { hlWriteJSON(w, 404, hlSecretBody) }},
		{"500", func(w http.ResponseWriter) { hlWriteJSON(w, 500, hlSecretBody) }},
		{"503", func(w http.ResponseWriter) { hlWriteJSON(w, 503, hlSecretBody) }},
		{"418", func(w http.ResponseWriter) { hlWriteJSON(w, 418, hlSecretBody) }},
		{"JSON inválido", func(w http.ResponseWriter) { hlWriteJSON(w, 200, `{"data":[`+hlSecretBody) }},
		{"HTML no lugar de JSON", func(w http.ResponseWriter) { hlWriteJSON(w, 200, `<html>`+hlSecretBody+`</html>`) }},
		{"data ausente", func(w http.ResponseWriter) { hlWriteJSON(w, 200, `{"count":0,"x":"`+hlSecretBody+`"}`) }},
		{"data não é lista", func(w http.ResponseWriter) { hlWriteJSON(w, 200, `{"data":"`+hlSecretBody+`"}`) }},
		{"corpo maior que 2 MiB", func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[`)
			_, _ = w.Write(bytes.Repeat([]byte(" "), scrapers.HLTBMaxBody+1024))
			_, _ = io.WriteString(w, `]}`)
		}},
	}
	for _, tc := range cases {
		t.Run("TTB-011/"+tc.name+" é ErrUnavailable sem URL, token nem corpo no erro", func(t *testing.T) {
			t.Parallel()
			f := newHLFake(t)
			f.searchFn = func(_ int, w http.ResponseWriter, _ *http.Request) { tc.fn(w) }
			h := f.client(t, scrapers.HLTBConfig{})
			_, err := hlSearch(t, h, "hollow knight")
			if !errors.Is(err, scrapers.ErrUnavailable) {
				t.Fatalf("err = %v, want ErrUnavailable", err)
			}
			if l := hlLeaks(err); len(l) > 0 {
				t.Errorf("error leaks %v: %v", l, err)
			}
		})
	}

	t.Run("TTB-011/rede caída na busca é ErrUnavailable sem URL", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		var tr http.RoundTripper = f.srv.Client().Transport
		fail := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == scrapers.HLTBSearchPath {
				return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused " + r.URL.String())}
			}
			return tr.RoundTrip(r)
		})
		h := f.client(t, scrapers.HLTBConfig{HTTP: &http.Client{Transport: fail}})
		_, err := hlSearch(t, h, "x")
		if !errors.Is(err, scrapers.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if l := hlLeaks(err); len(l) > 0 {
			t.Errorf("error leaks %v: %v", l, err)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// loopbackOnly refuses any request whose host is not a loopback address [TTB-031].
func loopbackOnly(inner http.RoundTripper, refused *int, mu *sync.Mutex) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		host := r.URL.Hostname()
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			mu.Lock()
			*refused++
			mu.Unlock()
			return nil, errors.New("host recusado pelo teste: não é loopback")
		}
		return inner.RoundTrip(r)
	})
}

func TestHLTBNoRealNetwork(t *testing.T) {
	t.Parallel()

	t.Run("TTB-031/BaseURL injetada recebe as requisições e um host não loopback é recusado", func(t *testing.T) {
		t.Parallel()
		f := newHLFake(t)
		var mu sync.Mutex
		refused := 0
		guard := &http.Client{Transport: loopbackOnly(f.srv.Client().Transport, &refused, &mu)}

		h := f.client(t, scrapers.HLTBConfig{HTTP: guard})
		if _, err := hlSearch(t, h, "x"); err != nil {
			t.Fatalf("loopback search: %v", err)
		}
		if len(f.records()) != 2 || refused != 0 {
			t.Fatalf("requests %d, refused %d", len(f.records()), refused)
		}

		other := scrapers.NewHLTB(scrapers.HLTBConfig{BaseURL: "https://howlongtobeat.com", HTTP: guard, Clock: domain.NewFakeClock(hlStart)})
		if _, err := other.Search(hlCtx(t), "x"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if refused == 0 {
			t.Error("the guard transport never saw the non-loopback host")
		}
	})
}

var _ = domain.NewFakeClock
