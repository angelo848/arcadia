package scrapers_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// igStart is the fake instant every IGDB test begins at.
var igStart = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// igGuardTimeout only stops a broken test from hanging. The FakeClock decides when limiter waits end.
const igGuardTimeout = 2 * time.Second

// igGuard is a context that ends after igGuardTimeout, to bound the blocking waits on the fake clock.
func igGuard(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), igGuardTimeout)
	t.Cleanup(cancel)
	return ctx
}

// igHades is a game as IGDB answers with the phase 12 fields: Windows platform, cover, two screenshots, one artwork.
const igHades = `{"id":1234,"name":"Hades","first_release_date":1600000000,` +
	`"platforms":[{"id":6,"name":"PC (Microsoft Windows)"}],` +
	`"summary":"Roguelike.","genres":[{"name":"Hack and slash"}],"game_modes":[{"name":"Single player"}],` +
	`"involved_companies":[{"developer":true,"publisher":false,"company":{"name":"Supergiant Games"}},` +
	`{"developer":false,"publisher":true,"company":{"name":"Publisher Co"}}],` +
	`"cover":{"image_id":"co1"},"screenshots":[{"image_id":"sc1"},{"image_id":"sc2"}],"artworks":[{"image_id":"ar1"}]}`

// igSemCapa is a game without cover, screenshots or artworks.
const igSemCapa = `{"id":5,"name":"Sem capa","first_release_date":0,"platforms":[]}`

// igIGDBFields is the field list of design §4.4 that every IGDB body must ask for.
var igIGDBFields = []string{
	"name", "first_release_date", "platforms.id", "summary", "genres.name", "game_modes.name",
	"involved_companies.developer", "involved_companies.publisher", "involved_companies.company.name",
	"cover.image_id", "screenshots.image_id", "artworks.image_id",
}

// igStub is a fake IGDB: it answers the Twitch token and the /v4/games endpoint, and records every API body.
type igStub struct {
	mu     sync.Mutex
	tokens int
	bodies []string
}

func (s *igStub) tokenCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens
}

func (s *igStub) apiBodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

func (s *igStub) apiCount() int { return len(s.apiBodies()) }

// igAnswer gives the status and the JSON of the n-th API call (1-based) of a stub.
type igAnswer func(n int, body string) (status int, payload string)

// igAlways answers 200 with the same JSON to every API call.
func igAlways(payload string) igAnswer {
	return func(int, string) (int, string) { return http.StatusOK, payload }
}

// newIGStub starts the fake IGDB. onToken, when not nil, runs on every token request before it is answered.
func newIGStub(t *testing.T, answer igAnswer, onToken func()) (*igStub, *httptest.Server) {
	t.Helper()
	s := &igStub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /oauth2/token":
			s.mu.Lock()
			s.tokens++
			s.mu.Unlock()
			if onToken != nil {
				onToken()
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"tok","expires_in":3600}`)
		case "POST /v4/games":
			raw, _ := io.ReadAll(r.Body)
			if r.Header.Get("Client-ID") != "cid" || r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("cabeçalhos da API: Client-ID=%q Authorization=%q", r.Header.Get("Client-ID"), r.Header.Get("Authorization"))
			}
			s.mu.Lock()
			s.bodies = append(s.bodies, string(raw))
			n := len(s.bodies)
			s.mu.Unlock()
			status, out := answer(n, string(raw))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, out)
		default:
			t.Errorf("caminho inesperado: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return s, srv
}

// igCfg is the IGDB configuration of the tests: keys, httptest URLs, fake clock and optional limiter.
// ImageBase is left empty on purpose, so the production image host is checked.
func igCfg(srv *httptest.Server, clock domain.Clock, lim *scrapers.Limiter) scrapers.IGDBConfig {
	return scrapers.IGDBConfig{
		ClientID: "cid", Secret: "sec",
		APIBase: srv.URL + "/v4", TokenURL: srv.URL + "/oauth2/token",
		HTTP: srv.Client(), Clock: clock, Limiter: lim,
	}
}

// igFields returns the names listed in the fields clause of an Apicalypse body.
func igFields(t *testing.T, body string) map[string]bool {
	t.Helper()
	const key = "fields "
	i := strings.Index(body, key)
	if i < 0 {
		t.Fatalf("corpo sem cláusula fields: %q", body)
	}
	rest := body[i+len(key):]
	end := strings.Index(rest, ";")
	if end < 0 {
		t.Fatalf("cláusula fields sem ';': %q", body)
	}
	names := map[string]bool{}
	for _, f := range strings.Split(rest[:end], ",") {
		names[strings.TrimSpace(f)] = true
	}
	return names
}

// igCheckFields fails for each field of igIGDBFields that the body does not ask for.
func igCheckFields(t *testing.T, body string) {
	t.Helper()
	got := igFields(t, body)
	for _, f := range igIGDBFields {
		if !got[f] {
			t.Errorf("corpo sem o campo %q: %q", f, body)
		}
	}
}

// igWhere returns the text of the where clause of a body, "" when there is none.
func igWhere(t *testing.T, body string) string {
	t.Helper()
	const key = "where "
	i := strings.Index(body, key)
	if i < 0 {
		return ""
	}
	rest := body[i+len(key):]
	end := strings.Index(rest, ";")
	if end < 0 {
		t.Fatalf("cláusula where sem ';': %q", body)
	}
	return rest[:end]
}

// igGameTypes returns the numbers listed in game_type = (...) of a where clause.
func igGameTypes(t *testing.T, where string) []int {
	t.Helper()
	const key = "game_type = ("
	i := strings.Index(where, key)
	if i < 0 {
		t.Fatalf("where sem game_type: %q", where)
	}
	rest := where[i+len(key):]
	end := strings.Index(rest, ")")
	if end < 0 {
		t.Fatalf("game_type sem ')': %q", where)
	}
	var out []int
	for _, p := range strings.Split(rest[:end], ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			t.Fatalf("game_type com valor inválido %q: %v", p, err)
		}
		out = append(out, n)
	}
	return out
}

// igImageSet turns images into sorted "kind url" strings, so the order of the offers does not matter.
func igImageSet(media []scrapers.RemoteMedia) []string {
	out := make([]string, 0, len(media))
	for _, m := range media {
		out = append(out, m.Kind+" "+m.URL)
	}
	sort.Strings(out)
	return out
}

// igSearchResult carries the outcome of a Search run on its own goroutine.
type igSearchResult struct {
	cands []scrapers.Candidate
	err   error
}

// igSearchAsync runs Search on a goroutine; the goroutine ends after it sends the result.
func igSearchAsync(p *scrapers.IGDB, q scrapers.Query) <-chan igSearchResult {
	out := make(chan igSearchResult, 1)
	go func() {
		cands, err := p.Search(context.Background(), q)
		out <- igSearchResult{cands, err}
	}()
	return out
}

// igReceive waits for a value of ch or fails the test when the guard context ends first.
func igReceive[T any](t *testing.T, guard context.Context, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-guard.Done():
		t.Fatalf("%s não voltou dentro do prazo de guarda", what)
		var zero T
		return zero
	}
}

func TestIGDBMetadados(t *testing.T) {
	t.Parallel()

	t.Run("MET-003/consulta com plataforma de seed traz where platforms = (id) no corpo", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		if _, err := p.Search(context.Background(), scrapers.Query{Title: "Hades", SeedKey: "windows", Category: "pc"}); err != nil {
			t.Fatalf("Search: %v", err)
		}
		bodies := st.apiBodies()
		if len(bodies) != 1 {
			t.Fatalf("requisições de API = %d, want 1", len(bodies))
		}
		if where := igWhere(t, bodies[0]); !strings.Contains(where, "platforms = (6)") {
			t.Errorf("where = %q, want platforms = (6) (id do Windows no seed)", where)
		}
	})

	t.Run("MET-003/plataforma sem id no IGDB: sem filtro de plataforma", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		// "psp" is a seed key that has no IGDB id in the table of platforms.
		if _, err := p.Search(context.Background(), scrapers.Query{Title: "Hades", SeedKey: "psp"}); err != nil {
			t.Fatalf("Search: %v", err)
		}
		bodies := st.apiBodies()
		if len(bodies) != 1 {
			t.Fatalf("requisições de API = %d, want 1", len(bodies))
		}
		if where := igWhere(t, bodies[0]); strings.Contains(where, "platforms") {
			t.Errorf("where = %q, want sem filtro de plataforma", where)
		}
	})

	t.Run("MET-003/sem plataforma na busca: sem filtro de plataforma", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Hades"})
		if err != nil || len(cands) != 1 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		bodies := st.apiBodies()
		if len(bodies) != 1 {
			t.Fatalf("requisições de API = %d, want 1", len(bodies))
		}
		if where := igWhere(t, bodies[0]); strings.Contains(where, "platforms") {
			t.Errorf("where = %q, want sem filtro de plataforma", where)
		}
		if !cands[0].PlatformConfirmed {
			t.Errorf("PlatformConfirmed = false sem plataforma na busca, want true (sem filtro não é segunda consulta)")
		}
	})

	t.Run("MET-003/plataforma da biblioteca sem id no IGDB: sem filtro, uma requisição, PlatformConfirmed true", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Hades", SeedKey: "psp", Platform: "PSP"})
		if err != nil || len(cands) != 1 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		bodies := st.apiBodies()
		if len(bodies) != 1 {
			t.Fatalf("requisições de API = %d, want 1 (sem segunda consulta)", len(bodies))
		}
		if where := igWhere(t, bodies[0]); strings.Contains(where, "platforms") {
			t.Errorf("where = %q, want sem filtro de plataforma", where)
		}
		if !cands[0].PlatformConfirmed {
			t.Errorf("PlatformConfirmed = false sem id no IGDB, want true (não é a segunda consulta)")
		}
	})

	t.Run("MET-003/filtro vazio: segunda consulta sem filtro e candidatos com PlatformConfirmed false", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, func(n int, _ string) (int, string) {
			if n == 1 {
				return http.StatusOK, "[]"
			}
			return http.StatusOK, "[" + igHades + "]"
		}, nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Hades", SeedKey: "windows"})
		if err != nil || len(cands) != 1 {
			t.Fatalf("Search = %+v, %v; want 1 candidato da segunda consulta", cands, err)
		}
		bodies := st.apiBodies()
		if len(bodies) != 2 {
			t.Fatalf("requisições de API = %d, want 2 (com filtro e sem filtro)", len(bodies))
		}
		if where := igWhere(t, bodies[0]); !strings.Contains(where, "platforms = (6)") {
			t.Errorf("primeira consulta where = %q, want platforms = (6)", where)
		}
		if where := igWhere(t, bodies[1]); strings.Contains(where, "platforms") {
			t.Errorf("segunda consulta ainda filtra plataforma: where = %q", where)
		}
		if cands[0].PlatformConfirmed {
			t.Errorf("PlatformConfirmed = true na segunda consulta, want false")
		}
	})

	t.Run("MET-003/filtro com resultado: uma só requisição e PlatformConfirmed true", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Hades", SeedKey: "windows"})
		if err != nil || len(cands) != 1 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		if n := st.apiCount(); n != 1 {
			t.Errorf("requisições de API = %d, want 1 (sem segunda consulta)", n)
		}
		if !cands[0].PlatformConfirmed {
			t.Errorf("PlatformConfirmed = false com filtro que devolveu resultado, want true")
		}
	})

	t.Run("MET-003/PlatformKeys vem de platforms.id", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		row := `{"id":7,"name":"Multi","platforms":[{"id":6,"name":"PC (Microsoft Windows)"},` +
			`{"id":130,"name":"Nintendo Switch"},{"id":9999,"name":"Fora da tabela"}]}`
		_, srv := newIGStub(t, igAlways("["+row+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Multi"})
		if err != nil || len(cands) != 1 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		want := []string{"windows", "switch"}
		if !reflect.DeepEqual(cands[0].PlatformKeys, want) {
			t.Errorf("PlatformKeys = %v, want %v (ids fora da tabela ignorados, ordem mantida)", cands[0].PlatformKeys, want)
		}
	})

	t.Run("MET-012/o corpo pede os campos de texto e de mídia numa só consulta", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		if _, err := p.Search(context.Background(), scrapers.Query{Title: "Hades"}); err != nil {
			t.Fatalf("Search: %v", err)
		}
		bodies := st.apiBodies()
		if len(bodies) != 1 {
			t.Fatalf("requisições de API = %d, want 1", len(bodies))
		}
		if !strings.HasPrefix(bodies[0], `search "Hades";`) {
			t.Errorf("corpo = %q, want começar por search \"Hades\";", bodies[0])
		}
		igCheckFields(t, bodies[0])
	})

	t.Run("MET-012/o filtro de jogo principal está no where", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		// Main game is 0 in IGDB. DLC, expansions, bundles, mods, episodes, seasons and packs must stay out.
		// The list follows provider-api-notes.md §1 (T02, not confirmed against the live API yet).
		excluded := []int{1, 2, 3, 5, 6, 7, 13, 14}

		for _, seed := range []string{"", "windows"} {
			if _, err := p.Search(context.Background(), scrapers.Query{Title: "Hades", SeedKey: seed}); err != nil {
				t.Fatalf("Search seed=%q: %v", seed, err)
			}
		}
		bodies := st.apiBodies()
		if len(bodies) != 2 {
			t.Fatalf("requisições de API = %d, want 2", len(bodies))
		}
		for i, body := range bodies {
			where := igWhere(t, body)
			types := igGameTypes(t, where)
			hasMain := false
			for _, n := range types {
				if n == 0 {
					hasMain = true
				}
				for _, bad := range excluded {
					if n == bad {
						t.Errorf("consulta %d: game_type inclui %d (DLC/pacote), where = %q", i+1, bad, where)
					}
				}
			}
			if !hasMain {
				t.Errorf("consulta %d: game_type não inclui 0 (jogo principal), where = %q", i+1, where)
			}
		}
		if where := igWhere(t, bodies[1]); !strings.Contains(where, "game_type") {
			t.Errorf("consulta sem plataforma: where = %q, want o filtro de jogo principal", where)
		}
	})

	t.Run("MET-012/candidato traz Meta e Images embutidos (cover, screenshot, background com os tamanhos certos)", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		_, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Hades"})
		if err != nil || len(cands) != 1 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		c := cands[0]
		if c.Meta == nil {
			t.Fatalf("Meta = nil, want o texto da resposta")
		}
		if c.Meta.Description != "Roguelike." || c.Year != 2020 || c.Meta.Year != 2020 {
			t.Errorf("Description = %q, Year = %d/%d; want Roguelike., 2020", c.Meta.Description, c.Year, c.Meta.Year)
		}
		if !reflect.DeepEqual(c.Meta.Genres, []string{"Hack and slash"}) || !reflect.DeepEqual(c.Meta.Modes, []string{"Single player"}) {
			t.Errorf("Genres = %v, Modes = %v", c.Meta.Genres, c.Meta.Modes)
		}
		if !reflect.DeepEqual(c.Meta.Developers, []string{"Supergiant Games"}) || !reflect.DeepEqual(c.Meta.Publishers, []string{"Publisher Co"}) {
			t.Errorf("Developers = %v, Publishers = %v", c.Meta.Developers, c.Meta.Publishers)
		}

		want := []string{
			"background https://images.igdb.com/igdb/image/upload/t_1080p/ar1.jpg",
			"cover https://images.igdb.com/igdb/image/upload/t_cover_big/co1.jpg",
			"screenshot https://images.igdb.com/igdb/image/upload/t_screenshot_big/sc1.jpg",
			"screenshot https://images.igdb.com/igdb/image/upload/t_screenshot_big/sc2.jpg",
		}
		if got := igImageSet(c.Images); !reflect.DeepEqual(got, want) {
			t.Errorf("Images = %v\nwant %v", got, want)
		}
	})

	t.Run("MET-012/Fetch faz uma requisição por id e devolve o mesmo que a mídia embutida", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))
		var f scrapers.Fetcher = p

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Hades"})
		if err != nil || len(cands) != 1 || cands[0].Meta == nil {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		meta, media, err := f.Fetch(context.Background(), "1234")
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}

		bodies := st.apiBodies()
		if len(bodies) != 2 {
			t.Fatalf("requisições de API = %d, want 2 (uma da busca, uma do Fetch)", len(bodies))
		}
		if !strings.Contains(bodies[1], "where id = 1234;") {
			t.Errorf("corpo do Fetch = %q, want where id = 1234;", bodies[1])
		}
		igCheckFields(t, bodies[1])
		if !reflect.DeepEqual(meta, *cands[0].Meta) {
			t.Errorf("Fetch Meta = %+v; busca Meta = %+v", meta, *cands[0].Meta)
		}
		if got, want := igImageSet(media), igImageSet(cands[0].Images); !reflect.DeepEqual(got, want) {
			t.Errorf("Fetch Images = %v; busca Images = %v", got, want)
		}
	})

	t.Run("MET-012/uma só requisição por busca, sem requisição de mídia", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		if _, err := p.Search(context.Background(), scrapers.Query{Title: "Hades", SeedKey: "windows"}); err != nil {
			t.Fatalf("Search: %v", err)
		}
		bodies := st.apiBodies()
		if len(bodies) != 1 {
			t.Fatalf("requisições de API = %d, want 1", len(bodies))
		}
		for _, b := range bodies {
			if strings.Contains(b, "where id") {
				t.Errorf("busca fez requisição por id de mídia: %q", b)
			}
		}
		if n := st.tokenCount(); n != 1 {
			t.Errorf("pedidos de token = %d, want 1", n)
		}
	})

	t.Run("MET-012/aspas no título são removidas do termo", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("[]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		if _, err := p.Search(context.Background(), scrapers.Query{Title: `Hades "Zagreus" Edition`}); err != nil {
			t.Fatalf("Search: %v", err)
		}
		bodies := st.apiBodies()
		if len(bodies) != 1 {
			t.Fatalf("requisições de API = %d, want 1", len(bodies))
		}
		if !strings.HasPrefix(bodies[0], `search "Hades Zagreus Edition";`) {
			t.Errorf("corpo = %q, want search \"Hades Zagreus Edition\";", bodies[0])
		}
	})

	t.Run("MET-068/ThumbURL é https em images.igdb.com com t_cover_small; sem capa fica vazio", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		_, srv := newIGStub(t, igAlways("["+igHades+","+igSemCapa+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		cands, err := p.Search(context.Background(), scrapers.Query{Title: "Hades"})
		if err != nil || len(cands) != 2 {
			t.Fatalf("Search = %+v, %v", cands, err)
		}
		const wantThumb = "https://images.igdb.com/igdb/image/upload/t_cover_small/co1.jpg"
		if cands[0].ThumbURL != wantThumb {
			t.Errorf("ThumbURL = %q, want %q", cands[0].ThumbURL, wantThumb)
		}
		if cands[1].ThumbURL != "" {
			t.Errorf("ThumbURL sem capa = %q, want vazio", cands[1].ThumbURL)
		}
	})

	t.Run("MET-010/cada requisição de API passa pelo Limiter e a do token não", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		lim := scrapers.NewLimiter(clock, scrapers.IntervalIGDB)
		tokenSeen := make(chan struct{}, 8)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), func() { tokenSeen <- struct{}{} })
		guard := igGuard(t)

		// First instance: its token and its first API call go out at once.
		first := scrapers.NewIGDB(igCfg(srv, clock, lim))
		if _, err := first.Search(context.Background(), scrapers.Query{Title: "Hades"}); err != nil {
			t.Fatalf("primeira busca: %v", err)
		}
		<-tokenSeen // token of the first instance, already buffered
		if n := st.apiCount(); n != 1 {
			t.Fatalf("API antes da segunda busca = %d, want 1", n)
		}

		// Second instance has no cached token. Its token goes out at once, its API call waits for the slot.
		second := scrapers.NewIGDB(igCfg(srv, clock, lim))
		done := igSearchAsync(second, scrapers.Query{Title: "Hades"})
		igReceive(t, guard, tokenSeen, "pedido de token da segunda instância")
		if err := clock.BlockUntilWaiters(guard, 1); err != nil {
			t.Fatalf("a requisição de API da segunda instância não esperou o Limiter: %v", err)
		}
		if n := st.apiCount(); n != 1 {
			t.Fatalf("API saiu antes do horário reservado: %d requisições", n)
		}
		if n := st.tokenCount(); n != 2 {
			t.Fatalf("pedidos de token = %d, want 2 (o token não passa pelo Limiter)", n)
		}

		clock.Advance(scrapers.IntervalIGDB)
		res := igReceive(t, guard, done, "segunda busca")
		if res.err != nil {
			t.Fatalf("segunda busca: %v", res.err)
		}
		if n := st.apiCount(); n != 2 {
			t.Errorf("requisições de API = %d, want 2", n)
		}
		if got, want := clock.Now(), igStart.Add(scrapers.IntervalIGDB); !got.Equal(want) {
			t.Errorf("segunda API saiu em %v; want %v (intervalo do IGDB)", got, want)
		}
	})

	t.Run("MET-010/a consulta sem filtro também espera o Limiter", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		lim := scrapers.NewLimiter(clock, scrapers.IntervalIGDB)
		st, srv := newIGStub(t, func(n int, _ string) (int, string) {
			if n == 1 {
				return http.StatusOK, "[]"
			}
			return http.StatusOK, "[" + igHades + "]"
		}, nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, lim))
		guard := igGuard(t)

		done := igSearchAsync(p, scrapers.Query{Title: "Hades", SeedKey: "windows"})
		if err := clock.BlockUntilWaiters(guard, 1); err != nil {
			t.Fatalf("a segunda consulta não esperou o Limiter: %v", err)
		}
		if n := st.apiCount(); n != 1 {
			t.Fatalf("segunda consulta saiu antes do horário: %d requisições", n)
		}
		clock.Advance(scrapers.IntervalIGDB)
		res := igReceive(t, guard, done, "busca com segunda consulta")
		if res.err != nil || len(res.cands) != 1 {
			t.Fatalf("Search = %+v, %v", res.cands, res.err)
		}
		if n := st.apiCount(); n != 2 {
			t.Errorf("requisições de API = %d, want 2", n)
		}
	})

	t.Run("MET-010/Fetch também espera o Limiter", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		lim := scrapers.NewLimiter(clock, scrapers.IntervalIGDB)
		st, srv := newIGStub(t, igAlways("["+igHades+"]"), nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, lim))
		guard := igGuard(t)

		if _, err := p.Search(context.Background(), scrapers.Query{Title: "Hades"}); err != nil {
			t.Fatalf("Search: %v", err)
		}
		fetched := make(chan error, 1)
		go func() {
			_, _, err := p.Fetch(context.Background(), "1234")
			fetched <- err
		}()
		if err := clock.BlockUntilWaiters(guard, 1); err != nil {
			t.Fatalf("o Fetch não esperou o Limiter: %v", err)
		}
		if n := st.apiCount(); n != 1 {
			t.Fatalf("Fetch saiu antes do horário: %d requisições", n)
		}
		clock.Advance(scrapers.IntervalIGDB)
		if err := igReceive(t, guard, fetched, "Fetch"); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if n := st.apiCount(); n != 2 {
			t.Errorf("requisições de API = %d, want 2", n)
		}
	})

	t.Run("MET-008/429 na busca vira ErrUnavailable", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		_, srv := newIGStub(t, func(int, string) (int, string) { return http.StatusTooManyRequests, "" }, nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		if _, err := p.Search(context.Background(), scrapers.Query{Title: "Hades"}); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("Search err = %v, want ErrUnavailable", err)
		}
	})

	t.Run("MET-008/503 no Fetch vira ErrUnavailable", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		_, srv := newIGStub(t, func(int, string) (int, string) { return http.StatusServiceUnavailable, "" }, nil)
		p := scrapers.NewIGDB(igCfg(srv, clock, nil))

		if _, _, err := p.Fetch(context.Background(), "1234"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("Fetch err = %v, want ErrUnavailable", err)
		}
	})

	t.Run("MET-001/sem chave: ErrNoKey em busca e Fetch sem nenhuma requisição", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(igStart)
		st, srv := newIGStub(t, igAlways("[]"), nil)
		p := scrapers.NewIGDB(scrapers.IGDBConfig{
			APIBase: srv.URL + "/v4", TokenURL: srv.URL + "/oauth2/token", HTTP: srv.Client(), Clock: clock,
		})

		if _, err := p.Search(context.Background(), scrapers.Query{Title: "Hades"}); !errors.Is(err, scrapers.ErrNoKey) {
			t.Errorf("Search err = %v, want ErrNoKey", err)
		}
		if _, _, err := p.Fetch(context.Background(), "1234"); !errors.Is(err, scrapers.ErrNoKey) {
			t.Errorf("Fetch err = %v, want ErrNoKey", err)
		}
		if n := st.tokenCount(); n != 0 {
			t.Errorf("pedidos de token = %d, want 0", n)
		}
		if n := st.apiCount(); n != 0 {
			t.Errorf("requisições de API = %d, want 0", n)
		}
	})
}
