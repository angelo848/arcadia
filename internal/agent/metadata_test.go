package agent

// Tests of metadata.search (phase 12, T20) and of the fixture shared with metadata_apply_test.go.
// Provedores são fakes; a loja é real, em t.TempDir(). Nenhuma chamada de rede, nenhum time.Sleep:
// o prazo de 20 s corre no domain.FakeClock.
// Alterado em 2026-11-15: arquivo novo (fase 12, T20).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
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

// metaProv is a provider fake for the unified service. Search answers cands (or searchErr); Details,
// Media and Fetch answer meta and media (or fetchErr). Every call is recorded. cost says how many HTTP
// requests a method stands for (missing = 1): the SteamGridDB Media is two (grids and heroes) and its
// Details is none, so the request counts of MET-030 can be checked on the fakes.
type metaProv struct {
	id        string
	cands     []scrapers.Candidate
	meta      scrapers.Metadata
	media     []scrapers.RemoteMedia
	searchErr error
	fetchErr  error
	block     bool           // Search waits for the context (a slow provider)
	started   chan struct{}  // optional: one value is sent when Search starts (buffered by the test)
	cost      map[string]int // requests per method; missing = 1

	mu       sync.Mutex
	calls    map[string]int
	requests int
	queries  []scrapers.Query
	ids      []string // provider ids asked by Details, Media and Fetch
}

func (p *metaProv) ID() string { return p.id }

// record counts one call of method and the requests it costs.
func (p *metaProv) record(method string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.calls == nil {
		p.calls = map[string]int{}
	}
	p.calls[method]++
	n, ok := p.cost[method]
	if !ok {
		n = 1
	}
	p.requests += n
}

func (p *metaProv) noteID(providerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ids = append(p.ids, providerID)
}

// count is how many times method was called.
func (p *metaProv) count(method string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[method]
}

// totalRequests is the number of requests the fake stands for, all methods together.
func (p *metaProv) totalRequests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests
}

// askedIDs are the provider ids of the Details, Media and Fetch calls, in order.
func (p *metaProv) askedIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.ids)
}

// lastQuery is the query of the last Search call; ok is false when there was none.
func (p *metaProv) lastQuery() (scrapers.Query, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queries) == 0 {
		return scrapers.Query{}, false
	}
	return p.queries[len(p.queries)-1], true
}

func (p *metaProv) Search(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error) {
	p.record("Search")
	p.mu.Lock()
	p.queries = append(p.queries, q)
	p.mu.Unlock()
	if p.started != nil {
		select {
		case p.started <- struct{}{}:
		default:
		}
	}
	if p.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return p.cands, p.searchErr
}

func (p *metaProv) Details(_ context.Context, providerID string) (scrapers.Metadata, error) {
	p.record("Details")
	p.noteID(providerID)
	return p.meta, p.fetchErr
}

func (p *metaProv) Media(_ context.Context, providerID string) ([]scrapers.RemoteMedia, error) {
	p.record("Media")
	p.noteID(providerID)
	return p.media, p.fetchErr
}

// metaFetchProv is a provider with Fetch: text and images in one request [MET-012, MET-014]. The
// agent must use Fetch when the provider has it, so Details and Media stay at zero for it.
type metaFetchProv struct {
	metaProv
}

func (p *metaFetchProv) Fetch(_ context.Context, providerID string) (scrapers.Metadata, []scrapers.RemoteMedia, error) {
	p.record("Fetch")
	p.noteID(providerID)
	return p.meta, p.media, p.fetchErr
}

// metaSinkSpy counts the MediaChanged calls of the queue (media.changed, IMG-001).
type metaSinkSpy struct {
	mu    sync.Mutex
	calls int
}

func (s *metaSinkSpy) MediaChanged(int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
}

func (s *metaSinkSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// metaFixture is the unified metadata service over a real store, with fake providers. The game is
// "Celeste", added with the Windows platform (seed key windows), so the image priority of MET-020 is
// the Windows one unless a test changes it.
type metaFixture struct {
	st       *store.Store
	clock    *domain.FakeClock
	svc      *scrapeService
	meta     *metadataService
	handlers map[string]ipc.Handler
	gameID   int64
	sink     *metaSinkSpy
}

// newMetaFixture builds the service. The providers named in noKey have no key (ErrNoKey); a name
// without a fake is not a provider (errUnknownProvider).
func newMetaFixture(t *testing.T, fakes map[string]scrapers.MetadataProvider, noKey ...string) metaFixture {
	t.Helper()
	root := t.TempDir()
	st, err := store.Open(t.Context(), filepath.Join(root, "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	g, err := st.AddGame(t.Context(), store.NewGame{Title: "Celeste", Exe: `C:\celeste.exe`, At: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	clock := domain.NewFakeClock(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sink := &metaSinkSpy{}
	queue := newMediaQueue(st, media.New(filepath.Join(root, "media"), nil), clock, log, sink)
	svc := &scrapeService{
		st: st,
		providers: func(name string) (scrapers.MetadataProvider, error) {
			if slices.Contains(noKey, name) {
				return nil, scrapers.ErrNoKey
			}
			p, ok := fakes[name]
			if !ok {
				return nil, errUnknownProvider
			}
			return p, nil
		},
		queue:     queue,
		mediaRoot: filepath.Join(root, "media"),
		clock:     clock,
		log:       log,
	}
	meta := newMetadataService(svc, st, t.Context())
	t.Cleanup(meta.Wait)
	return metaFixture{
		st: st, clock: clock, svc: svc, meta: meta, handlers: metadataHandlers(meta),
		gameID: g.ID, sink: sink,
	}
}

// call runs a handler with the params as JSON and returns its result or error.
func (f metaFixture) call(t *testing.T, method, params string) (any, error) {
	t.Helper()
	h, ok := f.handlers[method]
	if !ok {
		t.Fatalf("no handler for %s", method)
	}
	return h(t.Context(), json.RawMessage(params))
}

// addPlatform creates (or finds) a platform with the given name and category.
func addPlatform(t *testing.T, st *store.Store, name string, cat domain.PlatformCategory) store.Platform {
	t.Helper()
	p, _, err := st.AddPlatform(t.Context(), name, cat)
	if err != nil {
		t.Fatalf("AddPlatform(%q): %v", name, err)
	}
	return p
}

// metaCand is a candidate of provider with the given platform keys (confirmed when there are keys).
func metaCand(provider, id, title string, year int, platform string, keys ...string) scrapers.Candidate {
	return scrapers.Candidate{
		Provider: provider, ProviderID: id, Title: title, Year: year, Platform: platform,
		PlatformKeys: keys, PlatformConfirmed: len(keys) > 0,
	}
}

// searchJSON is the params of metadata.search as JSON.
func searchJSON(t *testing.T, p ipc.MetadataSearchParams) string {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// searchOK runs metadata.search and fails the test when it is not a success.
func searchOK(t *testing.T, f metaFixture, p ipc.MetadataSearchParams) ipc.MetadataSearchResult {
	t.Helper()
	out, err := f.call(t, ipc.MethodMetadataSearch, searchJSON(t, p))
	if err != nil {
		t.Fatalf("metadata.search: %v", err)
	}
	res, ok := out.(ipc.MetadataSearchResult)
	if !ok {
		t.Fatalf("result type %T, want ipc.MetadataSearchResult", out)
	}
	return res
}

// errData is the data member of a JSON-RPC error as a map of strings (nil when there is none).
func errData(t *testing.T, err error) map[string]string {
	t.Helper()
	var rpc *ipc.Error
	if !errors.As(err, &rpc) {
		t.Fatalf("error %v is not a JSON-RPC error", err)
	}
	var d map[string]string
	_ = json.Unmarshal(rpc.Data, &d)
	return d
}

// waitSignal waits for a value on ch, failing the test after a short timeout.
func waitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not happen", what)
	}
}

func TestMetadataSearch(t *testing.T) {
	t.Parallel()

	t.Run("MET-071/valores inválidos dão -32602 sem nenhuma chamada", func(t *testing.T) {
		t.Parallel()
		invalid := []struct {
			name   string
			params ipc.MetadataSearchParams
		}{
			{"MET-071/título vazio", ipc.MetadataSearchParams{Title: ""}},
			{"MET-071/título só de espaços", ipc.MetadataSearchParams{Title: "   "}},
			{"MET-071/título com 201 runas", ipc.MetadataSearchParams{Title: strings.Repeat("a", 201)}},
			{"MET-071/plataforma com 101 runas", ipc.MetadataSearchParams{Title: "Celeste", Platform: strings.Repeat("p", 101)}},
			{"MET-071/ano 1899", ipc.MetadataSearchParams{Title: "Celeste", Year: 1899}},
			{"MET-071/ano 2101", ipc.MetadataSearchParams{Title: "Celeste", Year: 2101}},
			{"MET-071/ano negativo", ipc.MetadataSearchParams{Title: "Celeste", Year: -1}},
			{"MET-004/título só de marcações (Demo) vira termo vazio", ipc.MetadataSearchParams{Title: "(Demo)"}},
		}
		for _, tc := range invalid {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				igdb := &metaProv{id: "igdb"}
				f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
				_, err := f.call(t, ipc.MethodMetadataSearch, searchJSON(t, tc.params))
				if codeOf(err) != ipc.CodeInvalidParams {
					t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeInvalidParams)
				}
				if n := igdb.totalRequests(); n != 0 {
					t.Errorf("provider requests = %d, want 0", n)
				}
			})
		}
	})

	t.Run("MET-071/valores de borda valem", func(t *testing.T) {
		t.Parallel()
		valid := []struct {
			name   string
			params ipc.MetadataSearchParams
		}{
			{"MET-071/título com 200 runas acentuadas passa", ipc.MetadataSearchParams{Title: strings.Repeat("é", 200)}},
			{"MET-071/plataforma com 100 runas passa", ipc.MetadataSearchParams{Title: "Celeste", Platform: strings.Repeat("p", 100)}},
			{"MET-071/ano 0 (desconhecido) vale", ipc.MetadataSearchParams{Title: "Celeste", Year: 0}},
			{"MET-071/ano 1900 vale", ipc.MetadataSearchParams{Title: "Celeste", Year: 1900}},
			{"MET-071/ano 2100 vale", ipc.MetadataSearchParams{Title: "Celeste", Year: 2100}},
		}
		for _, tc := range valid {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				igdb := &metaProv{id: "igdb"}
				f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
				if _, err := f.call(t, ipc.MethodMetadataSearch, searchJSON(t, tc.params)); err != nil {
					t.Fatalf("metadata.search: %v", err)
				}
				if n := igdb.count("Search"); n != 1 {
					t.Errorf("IGDB searches = %d, want 1", n)
				}
			})
		}
	})

	t.Run("MET-004/o provedor recebe o termo de SearchTerm e o ano pedido", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb"}
		sgdb := &metaProv{id: "steamgriddb"}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamgriddb": sgdb})
		searchOK(t, f, ipc.MetadataSearchParams{Title: "Legend of Zelda - Majora's Mask (USA)", Year: 2000})
		want := "Legend of Zelda: Majora's Mask"
		for _, p := range []*metaProv{igdb, sgdb} {
			q, ok := p.lastQuery()
			if !ok || q.Title != want || q.Year != 2000 {
				t.Errorf("%s query = %+v, ok %v; want title %q year 2000", p.id, q, ok, want)
			}
		}
	})

	t.Run("MET-001/todos respondem: cada fake é chamado uma vez e a lista é única", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb", cands: []scrapers.Candidate{metaCand("igdb", "7", "Celeste", 2018, "")}}
		sgdb := &metaProv{id: "steamgriddb", cands: []scrapers.Candidate{metaCand("steamgriddb", "55", "Celeste", 0, "")}}
		steam := &metaProv{id: "steamstore", cands: []scrapers.Candidate{metaCand("steamstore", "504230", "Celeste", 2018, "PC", "windows")}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamgriddb": sgdb, "steamstore": steam})
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		for _, p := range []*metaProv{igdb, sgdb, steam} {
			if n := p.count("Search"); n != 1 {
				t.Errorf("%s searches = %d, want 1", p.id, n)
			}
		}
		if len(res.Candidates) != 1 || len(res.Candidates[0].Refs) != 3 {
			t.Fatalf("candidates = %+v, want one game with three refs", res.Candidates)
		}
		if res.Candidates[0].Title != "Celeste" {
			t.Errorf("title = %q, want Celeste", res.Candidates[0].Title)
		}
	})

	t.Run("MET-001/provedor sem chave não é chamado nem conta como falha", func(t *testing.T) {
		t.Parallel()
		sgdb := &metaProv{id: "steamgriddb", cands: []scrapers.Candidate{metaCand("steamgriddb", "1", "Celeste", 0, "")}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}, "steamgriddb": sgdb}, "igdb")
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if len(res.Candidates) != 1 {
			t.Errorf("candidates = %d, want 1", len(res.Candidates))
		}
		for _, w := range res.Warnings {
			if strings.Contains(w, "IGDB") {
				t.Errorf("warnings %q mention IGDB, which has no key and is not a failure", res.Warnings)
			}
		}
	})

	t.Run("MET-001/um com falha temporária: lista dos outros e aviso", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb", searchErr: scrapers.ErrUnavailable}
		sgdb := &metaProv{id: "steamgriddb", cands: []scrapers.Candidate{metaCand("steamgriddb", "1", "Celeste", 0, "")}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamgriddb": sgdb})
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if len(res.Candidates) != 1 {
			t.Errorf("candidates = %d, want 1 from SteamGridDB", len(res.Candidates))
		}
		if !slices.Contains(res.Warnings, "IGDB indisponível no momento") {
			t.Errorf("warnings = %q, want IGDB indisponível no momento", res.Warnings)
		}
	})

	t.Run("MET-001/provedor lento: falha temporária quando o relógio fake passa dos 20 s", func(t *testing.T) {
		t.Parallel()
		slow := &metaProv{id: "igdb", block: true, started: make(chan struct{}, 1)}
		quick := &metaProv{id: "steamgriddb", cands: []scrapers.Candidate{metaCand("steamgriddb", "1", "Celeste", 0, "")}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": slow, "steamgriddb": quick})

		type reply struct {
			out any
			err error
		}
		done := make(chan reply, 1)
		go func() {
			out, err := f.handlers[ipc.MethodMetadataSearch](t.Context(), json.RawMessage(searchJSON(t, ipc.MetadataSearchParams{Title: "Celeste"})))
			done <- reply{out, err}
		}()
		waitSignal(t, slow.started, "IGDB search start")
		if err := f.clock.BlockUntilWaiters(t.Context(), 1); err != nil {
			t.Fatalf("the deadline timer was not set: %v", err)
		}
		f.clock.Advance(20 * time.Second)

		select {
		case r := <-done:
			if r.err != nil {
				t.Fatalf("metadata.search: %v", r.err)
			}
			res, ok := r.out.(ipc.MetadataSearchResult)
			if !ok {
				t.Fatalf("result type %T", r.out)
			}
			if !slices.Contains(res.Warnings, "IGDB indisponível no momento") {
				t.Errorf("warnings = %q, want IGDB indisponível no momento", res.Warnings)
			}
			if len(res.Candidates) != 1 {
				t.Errorf("candidates = %d, want 1 from SteamGridDB", len(res.Candidates))
			}
		case <-time.After(5 * time.Second):
			t.Fatal("metadata.search did not return after the 20 s deadline")
		}
	})

	t.Run("MET-001/cancelar o contexto durante a busca devolve erro e libera a espera", func(t *testing.T) {
		t.Parallel()
		slow := &metaProv{id: "igdb", block: true, started: make(chan struct{}, 1)}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": slow, "steamgriddb": &metaProv{id: "steamgriddb"}})
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			_, err := f.handlers[ipc.MethodMetadataSearch](ctx, json.RawMessage(searchJSON(t, ipc.MetadataSearchParams{Title: "Celeste"})))
			done <- err
		}()
		waitSignal(t, slow.started, "IGDB search start")
		cancel()
		select {
		case err := <-done:
			if err == nil {
				t.Error("metadata.search after cancel returned no error")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("metadata.search did not return after the context was canceled")
		}
	})

	t.Run("MET-001/contexto já cancelado: nenhuma chamada ao provedor", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb"}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := f.handlers[ipc.MethodMetadataSearch](ctx, json.RawMessage(searchJSON(t, ipc.MetadataSearchParams{Title: "Celeste"})))
		if err == nil {
			t.Error("metadata.search with a canceled context returned no error")
		}
		if n := igdb.count("Search"); n != 0 {
			t.Errorf("IGDB searches = %d, want 0", n)
		}
	})

	t.Run("MET-002/console sem chaves de IGDB e SteamGridDB: -32010 com igdb e nenhuma requisição", func(t *testing.T) {
		t.Parallel()
		steam := &metaProv{id: "steamstore"}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}, "steamgriddb": &metaProv{id: "steamgriddb"}, "steamstore": steam}, "igdb", "steamgriddb")
		addPlatform(t, f.st, "Nintendo 64", domain.CategoryConsole)
		_, err := f.call(t, ipc.MethodMetadataSearch, searchJSON(t, ipc.MetadataSearchParams{Title: "Celeste", Platform: "Nintendo 64"}))
		if codeOf(err) != ipc.CodeProviderNoKey {
			t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderNoKey)
		}
		if d := errData(t, err); d["provider"] != "igdb" {
			t.Errorf("data = %v, want provider igdb", d)
		}
		if n := steam.totalRequests(); n != 0 {
			t.Errorf("Steam Store requests = %d, want 0 (not applicable to a console)", n)
		}
	})

	t.Run("MET-002/sem chaves, mas Steam Store aplicável responde: sucesso só com ela", func(t *testing.T) {
		t.Parallel()
		steam := &metaProv{id: "steamstore", cands: []scrapers.Candidate{metaCand("steamstore", "504230", "Celeste", 2018, "", "windows")}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}, "steamgriddb": &metaProv{id: "steamgriddb"}, "steamstore": steam}, "igdb", "steamgriddb")
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if len(res.Candidates) != 1 || steam.count("Search") != 1 {
			t.Errorf("candidates = %d, steam searches = %d; want 1 and 1", len(res.Candidates), steam.count("Search"))
		}
	})

	t.Run("MET-002/só IGDB sem chave e SteamGridDB com chave: sucesso", func(t *testing.T) {
		t.Parallel()
		sgdb := &metaProv{id: "steamgriddb", cands: []scrapers.Candidate{metaCand("steamgriddb", "1", "Celeste", 0, "")}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}, "steamgriddb": sgdb, "steamstore": &metaProv{id: "steamstore"}}, "igdb")
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if len(res.Candidates) != 1 {
			t.Errorf("candidates = %d, want 1", len(res.Candidates))
		}
	})

	t.Run("MET-002/todos falham: -32011 com o primeiro que falhou", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{
			"igdb":        &metaProv{id: "igdb", searchErr: scrapers.ErrUnavailable},
			"steamgriddb": &metaProv{id: "steamgriddb", searchErr: scrapers.ErrUnavailable},
			"steamstore":  &metaProv{id: "steamstore", searchErr: scrapers.ErrUnavailable},
		})
		_, err := f.call(t, ipc.MethodMetadataSearch, searchJSON(t, ipc.MetadataSearchParams{Title: "Celeste"}))
		if codeOf(err) != ipc.CodeProviderUnavailable {
			t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderUnavailable)
		}
		if d := errData(t, err); d["provider"] != "igdb" {
			t.Errorf("data = %v, want provider igdb", d)
		}
	})

	t.Run("MET-002/um respondeu com lista vazia: sucesso com lista vazia e confiança none", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{
			"igdb":        &metaProv{id: "igdb"},
			"steamgriddb": &metaProv{id: "steamgriddb", searchErr: scrapers.ErrUnavailable},
			"steamstore":  &metaProv{id: "steamstore", searchErr: scrapers.ErrUnavailable},
		})
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if len(res.Candidates) != 0 || res.Confidence != ipc.MetadataConfidenceNone {
			t.Errorf("result = %+v, want no candidates and confidence none", res)
		}
		raw, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(raw), `"candidates":[]`) {
			t.Errorf("JSON %s, want candidates as an empty array", raw)
		}
	})

	t.Run("MET-003/Windows do seed: IGDB e Steam Store recebem a chave windows e a categoria pc", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb"}
		steam := &metaProv{id: "steamstore"}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamstore": steam})
		searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste", Platform: "Windows"})
		q, ok := steam.lastQuery()
		if !ok || q.SeedKey != "windows" || q.Category != "pc" {
			t.Errorf("steam query = %+v, ok %v; want seed windows and category pc", q, ok)
		}
		if q, ok := igdb.lastQuery(); !ok || q.SeedKey != "windows" {
			t.Errorf("igdb query = %+v, ok %v; want seed windows", q, ok)
		}
	})

	t.Run("MET-003/Windows do seed renomeada: Steam Store continua aplicável", func(t *testing.T) {
		t.Parallel()
		steam := &metaProv{id: "steamstore"}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}, "steamstore": steam})
		win, _, err := f.st.ResolvePlatform(t.Context(), "Windows")
		if err != nil {
			t.Fatalf("ResolvePlatform: %v", err)
		}
		if _, err := f.st.UpdatePlatform(t.Context(), win.ID, "PC de Mesa", domain.CategoryPC); err != nil {
			t.Fatalf("UpdatePlatform: %v", err)
		}
		searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste", Platform: "PC de Mesa"})
		if q, ok := steam.lastQuery(); !ok || q.SeedKey != "windows" {
			t.Errorf("steam query = %+v, ok %v; want seed windows", q, ok)
		}
	})

	t.Run("MET-003/plataforma criada pelo usuário com categoria pc: Steam Store aplicável", func(t *testing.T) {
		t.Parallel()
		steam := &metaProv{id: "steamstore"}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}, "steamstore": steam})
		addPlatform(t, f.st, "Meu PC Antigo", domain.CategoryPC)
		searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste", Platform: "Meu PC Antigo"})
		if n := steam.count("Search"); n != 1 {
			t.Errorf("steam searches = %d, want 1", n)
		}
	})

	t.Run("MET-003/plataforma criada pelo usuário com categoria console: Steam Store não aplicável", func(t *testing.T) {
		t.Parallel()
		steam := &metaProv{id: "steamstore"}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}, "steamstore": steam})
		addPlatform(t, f.st, "Meu Console", domain.CategoryConsole)
		searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste", Platform: "Meu Console"})
		if n := steam.count("Search"); n != 0 {
			t.Errorf("steam searches = %d, want 0", n)
		}
	})

	t.Run("MET-003/plataforma de console do seed: Steam Store não aplicável e IGDB recebe a chave", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb"}
		steam := &metaProv{id: "steamstore"}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamstore": steam})
		addPlatform(t, f.st, "Nintendo 64", domain.CategoryConsole)
		searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste", Platform: "Nintendo 64"})
		if n := steam.count("Search"); n != 0 {
			t.Errorf("steam searches = %d, want 0", n)
		}
		if q, ok := igdb.lastQuery(); !ok || q.SeedKey == "" || q.SeedKey == "windows" {
			t.Errorf("igdb query = %+v, ok %v; want the console seed key", q, ok)
		}
	})

	t.Run("MET-003/plataforma inexistente ou não informada: Steam Store aplicável", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"", "Plataforma Inexistente"} {
			steam := &metaProv{id: "steamstore"}
			f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}, "steamstore": steam})
			searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste", Platform: name})
			if n := steam.count("Search"); n != 1 {
				t.Errorf("platform %q: steam searches = %d, want 1", name, n)
			}
		}
	})

	t.Run("MET-003/a busca não cria plataforma", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}, "steamstore": &metaProv{id: "steamstore"}})
		before, err := f.st.Platforms(t.Context())
		if err != nil {
			t.Fatalf("Platforms: %v", err)
		}
		searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste", Platform: "Plataforma Inexistente"})
		after, err := f.st.Platforms(t.Context())
		if err != nil {
			t.Fatalf("Platforms: %v", err)
		}
		if len(after) != len(before) {
			t.Errorf("platforms = %d, want %d: the search must not create one", len(after), len(before))
		}
		if _, found, _ := f.st.ResolvePlatform(t.Context(), "Plataforma Inexistente"); found {
			t.Error("the searched platform exists after the search")
		}
	})

	t.Run("MET-008/até 10 itens, ordem decrescente e piso de 0,40", func(t *testing.T) {
		t.Parallel()
		var many []scrapers.Candidate
		for i := 1; i <= 12; i++ {
			many = append(many, metaCand("igdb", strconv.Itoa(i), fmt.Sprintf("Celeste %d", i), 0, ""))
		}
		many = append(many, metaCand("igdb", "99", "Zork Nothing", 0, ""))
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb", cands: many}})
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if len(res.Candidates) != 10 {
			t.Fatalf("candidates = %d, want 10", len(res.Candidates))
		}
		for i, c := range res.Candidates {
			if c.Score < 0.40 {
				t.Errorf("candidate %d score %.2f is below 0,40", i, c.Score)
			}
			if i > 0 && c.Score > res.Candidates[i-1].Score {
				t.Errorf("candidate %d score %.2f is above the previous %.2f", i, c.Score, res.Candidates[i-1].Score)
			}
			if c.Title == "Zork Nothing" {
				t.Errorf("candidate %d is Zork Nothing, below the 0,40 floor", i)
			}
		}
	})

	t.Run("MET-008/a pontuação maior vem primeiro", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb", cands: []scrapers.Candidate{
			metaCand("igdb", "2", "Celeste 2", 0, ""),
			metaCand("igdb", "1", "Celeste", 0, ""),
		}}})
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if len(res.Candidates) == 0 || res.Candidates[0].Title != "Celeste" {
			t.Errorf("candidates = %+v, want Celeste first", res.Candidates)
		}
	})

	t.Run("MET-008/ano 0 e plataforma vazia quando desconhecidos", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb", cands: []scrapers.Candidate{
			metaCand("igdb", "1", "Celeste", 0, ""),
		}}})
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if len(res.Candidates) != 1 || res.Candidates[0].Year != 0 || res.Candidates[0].Platform != "" {
			t.Errorf("candidates = %+v, want year 0 and platform empty", res.Candidates)
		}
	})

	t.Run("MET-008/plataforma da biblioteca quando confere", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb", cands: []scrapers.Candidate{
			metaCand("igdb", "1", "Celeste", 2018, "PC", "windows"),
		}}})
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste", Platform: "Windows"})
		if len(res.Candidates) != 1 || res.Candidates[0].Platform != "Windows" {
			t.Errorf("candidates = %+v, want platform Windows (the library name)", res.Candidates)
		}
	})

	t.Run("MET-009/confiança match para um só jogo exato", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb", cands: []scrapers.Candidate{
			metaCand("igdb", "1", "Celeste", 2018, ""),
		}}})
		if res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"}); res.Confidence != ipc.MetadataConfidenceMatch {
			t.Errorf("confidence = %q, want match", res.Confidence)
		}
	})

	t.Run("MET-009/confiança doubt quando as marcas diferem (trava de 0,79)", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb", cands: []scrapers.Candidate{
			metaCand("igdb", "2", "Celeste 2", 0, ""),
		}}})
		if res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"}); res.Confidence != ipc.MetadataConfidenceDoubt {
			t.Errorf("confidence = %q, want doubt", res.Confidence)
		}
	})

	t.Run("MET-009/confiança none sem nenhum item acima de 0,40", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb", cands: []scrapers.Candidate{
			metaCand("igdb", "9", "Zork Nothing", 0, ""),
		}}})
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if res.Confidence != ipc.MetadataConfidenceNone || len(res.Candidates) != 0 {
			t.Errorf("result = %+v, want confidence none and no candidates", res)
		}
	})

	t.Run("MET-068/thumbUrl do candidato sai só nos hosts https permitidos", func(t *testing.T) {
		t.Parallel()
		c := metaCand("igdb", "1", "Celeste", 2018, "")
		c.ThumbURL = "https://images.igdb.com/igdb/image/upload/t_cover_small/co1.jpg"
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb", cands: []scrapers.Candidate{c}}})
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if len(res.Candidates) != 1 || res.Candidates[0].ThumbURL != c.ThumbURL {
			t.Errorf("thumbUrl = %+v, want the IGDB https URL", res.Candidates)
		}
	})

	t.Run("MET-068/thumbUrl http ou de outro host vira vazio", func(t *testing.T) {
		t.Parallel()
		c := metaCand("igdb", "1", "Celeste", 2018, "")
		c.ThumbURL = "http://images.igdb.com/igdb/image/upload/t_cover_small/co1.jpg"
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb", cands: []scrapers.Candidate{c}}})
		res := searchOK(t, f, ipc.MetadataSearchParams{Title: "Celeste"})
		if len(res.Candidates) != 1 || res.Candidates[0].ThumbURL != "" {
			t.Errorf("thumbUrl = %+v, want empty for http", res.Candidates)
		}
	})
}

// TestCandidateOfLimits checks the limits of a unified candidate: the title at 200 runes, the platform at
// 100 runes (contract maxLength) and a thumbnail of at most 300 characters [MET-068].
func TestCandidateOfLimits(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("é", 250)
	t.Run("MET-068/título passa de 200 runas e plataforma de 100: são cortados", func(t *testing.T) {
		t.Parallel()
		c := candidateOf(scrapers.Group{Title: long, Platform: long})
		if got := []rune(c.Title); len(got) != 200 || string(got) != strings.Repeat("é", 200) {
			t.Errorf("title = %d runes, want the first 200", len(got))
		}
		if got := []rune(c.Platform); len(got) != 100 || string(got) != strings.Repeat("é", 100) {
			t.Errorf("platform = %d runes, want the first 100", len(got))
		}
	})
	t.Run("MET-068/thumbUrl de 300 caracteres passa; de 301 vira vazio", func(t *testing.T) {
		t.Parallel()
		const host = "https://images.igdb.com/"
		atLimit := host + strings.Repeat("a", 300-len(host))
		overLimit := host + strings.Repeat("a", 301-len(host))
		if got := candidateOf(scrapers.Group{ThumbURL: atLimit}).ThumbURL; got != atLimit {
			t.Errorf("thumbUrl of 300 characters = %q, want it kept", got)
		}
		if got := candidateOf(scrapers.Group{ThumbURL: overLimit}).ThumbURL; got != "" {
			t.Errorf("thumbUrl of 301 characters = %d chars, want empty", len(got))
		}
	})
}

func TestAllowedThumb(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, in, want string
	}{
		{"MET-068/https do IGDB passa", "https://images.igdb.com/igdb/image/upload/t_cover_small/co1.jpg", "https://images.igdb.com/igdb/image/upload/t_cover_small/co1.jpg"},
		{"MET-068/https da Steam Store passa", "https://cdn.cloudflare.steamstatic.com/steam/apps/231430/capsule_231x87.jpg", "https://cdn.cloudflare.steamstatic.com/steam/apps/231430/capsule_231x87.jpg"},
		{"MET-068/https do SteamGridDB passa", "https://cdn2.steamgriddb.com/thumb/abc.jpg", "https://cdn2.steamgriddb.com/thumb/abc.jpg"},
		{"MET-068/http vira vazio", "http://images.igdb.com/igdb/image/upload/t_cover_small/co1.jpg", ""},
		{"MET-068/outro host vira vazio", "https://evil.example.test/x.jpg", ""},
		{"MET-068/host que só termina igual vira vazio", "https://images.igdb.com.evil.example.test/x.jpg", ""},
		{"MET-068/URL malformada vira vazio", "https://[::1/x", ""},
		{"MET-068/texto sem esquema vira vazio", "::não é url", ""},
		{"MET-068/vazio continua vazio", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := allowedThumb(tc.in); got != tc.want {
				t.Errorf("allowedThumb(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
