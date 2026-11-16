package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// memKeyStore is an in-memory scrapers.KeyStore: the Credential Manager is not used in tests.
type memKeyStore struct{ values map[string]string }

func (m *memKeyStore) Set(name, value string) error { m.values[name] = value; return nil }

func (m *memKeyStore) Get(name string) (string, error) {
	v, ok := m.values[name]
	if !ok {
		return "", scrapers.ErrKeyNotFound
	}
	return v, nil
}

func (m *memKeyStore) Delete(name string) error { delete(m.values, name); return nil }

// fakeProvider answers the three methods of MetadataProvider from fields.
type fakeProvider struct {
	id         string
	candidates []scrapers.Candidate
	meta       scrapers.Metadata
	media      []scrapers.RemoteMedia
	err        error
}

func (p *fakeProvider) ID() string { return p.id }

func (p *fakeProvider) Search(context.Context, scrapers.Query) ([]scrapers.Candidate, error) {
	return p.candidates, p.err
}

func (p *fakeProvider) Details(context.Context, string) (scrapers.Metadata, error) {
	return p.meta, p.err
}

func (p *fakeProvider) Media(context.Context, string) ([]scrapers.RemoteMedia, error) {
	return p.media, p.err
}

// scrapeFixture is a scrape service over a real store, with fake providers.
type scrapeFixture struct {
	st       *store.Store
	keys     *scrapers.Keys
	handlers map[string]ipc.Handler
	gameID   int64
	root     string
	provider *fakeProvider
	queue    *mediaQueue
	svc      *scrapeService
}

func newScrapeFixture(t *testing.T, p *fakeProvider) scrapeFixture {
	t.Helper()
	root := t.TempDir()
	st, err := store.Open(t.Context(), filepath.Join(root, "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	g, err := st.AddGame(t.Context(), store.NewGame{Title: "Celeste", Exe: `C:\c.exe`, At: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	keys := scrapers.NewKeys(&memKeyStore{values: map[string]string{}})
	clock := domain.NewFakeClock(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	queue := newMediaQueue(st, imageFetcher{files: media.New(filepath.Join(root, "media"), nil)}, clock, log, nil)
	svc := &scrapeService{
		st:   st,
		keys: keys,
		providers: func(name string) (scrapers.MetadataProvider, error) {
			if p == nil || name != p.id {
				return nil, errUnknownProvider
			}
			if p.err != nil && errors.Is(p.err, scrapers.ErrNoKey) {
				return nil, p.err
			}
			return p, nil
		},
		queue:     queue,
		mediaRoot: filepath.Join(root, "media"),
		clock:     clock,
		log:       log,
	}
	return scrapeFixture{st: st, keys: keys, handlers: scrapeHandlers(svc), gameID: g.ID, root: root, provider: p, queue: queue, svc: svc}
}

// call runs a handler with the params as JSON and returns its result or error.
func (f scrapeFixture) call(t *testing.T, method string, params string) (any, error) {
	t.Helper()
	h, ok := f.handlers[method]
	if !ok {
		t.Fatalf("no handler for %s", method)
	}
	return h(t.Context(), json.RawMessage(params))
}

// codeOf returns the JSON-RPC code of an error, or 0.
func codeOf(err error) int {
	var rpc *ipc.Error
	if errors.As(err, &rpc) {
		return rpc.Code
	}
	return 0
}

func TestScrapeHandlers(t *testing.T) {
	t.Parallel()

	t.Run("MID-001/status não devolve a chave e setKey guarda o IGDB com segredo", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		out, err := f.call(t, ipc.MethodProviderSetKey, `{"provider":"igdb","key":"cid-secreto","secret":"sec-secreto"}`)
		if err != nil {
			t.Fatalf("setKey: %v", err)
		}
		raw, _ := json.Marshal(out)
		if strings.Contains(string(raw), "secreto") {
			t.Errorf("result %s repeats a key", raw)
		}
		if !strings.Contains(string(raw), `"configured":true`) {
			t.Errorf("result %s, want configured true", raw)
		}
		status, err := f.call(t, ipc.MethodProviderKeyStatus, `{"provider":"igdb"}`)
		if err != nil || !strings.Contains(scrapeJSON(t, status), `"configured":true`) {
			t.Errorf("keyStatus = %v, %v", status, err)
		}
	})

	t.Run("MID-001/sem chave a busca é recusada com -32010 e nenhuma chamada é feita", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb", err: scrapers.ErrNoKey})
		_, err := f.call(t, ipc.MethodScrapeSearch, `{"gameId":1,"provider":"igdb"}`)
		if codeOf(err) != ipc.CodeProviderNoKey {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderNoKey)
		}
	})

	t.Run("MID-002/candidatos saem do melhor para o pior e só o que passa de 0,80 é aceito", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb", candidates: []scrapers.Candidate{
			{ProviderID: "9", Title: "Totally Different", Platform: "Switch", Year: 1999},
			{ProviderID: "7", Title: "Celeste", Platform: "PC", Year: 2018},
		}})
		out, err := f.call(t, ipc.MethodScrapeSearch, `{"gameId":1,"provider":"igdb"}`)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		cands := out.(ipc.ScrapeSearchResult).Candidates
		if len(cands) != 2 || cands[0].ProviderID != "7" || !cands[0].Accepted || cands[1].Accepted {
			t.Errorf("candidates = %+v, want Celeste first and accepted", cands)
		}
	})

	t.Run("MID-003/apply aplica os metadados e enfileira as imagens", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{
			id:    "igdb",
			meta:  scrapers.Metadata{Description: "Escalada.", Genres: []string{"Platform"}, Year: 2018},
			media: []scrapers.RemoteMedia{{Kind: scrapers.KindCover, URL: "https://cdn.example.test/c.png"}, {Kind: scrapers.KindScreenshot, URL: "https://cdn.example.test/s.png"}},
		})
		out, err := f.call(t, ipc.MethodScrapeApply, `{"gameId":1,"provider":"igdb","providerId":"7","replace":false}`)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		res := out.(ipc.ScrapeApplyResult)
		if res.Detail.Description != "Escalada." || res.Queued != 2 {
			t.Errorf("apply = description %q queued %d, want Escalada. and 2", res.Detail.Description, res.Queued)
		}
		jobs, err := f.st.PendingMediaJobs(t.Context())
		if err != nil || len(jobs) != 2 {
			t.Errorf("pending jobs = %+v, %v; want 2", jobs, err)
		}
	})

	t.Run("MID-004/media.list devolve caminhos absolutos", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		if _, err := f.st.AddMedia(t.Context(), store.MediaRow{GameID: f.gameID, Kind: "cover", Provider: "steamgriddb",
			Hash: "h1", Path: filepath.Join("1", "h1.png"), ThumbPath: filepath.Join("1", "thumb_h1.jpg"), CreatedAt: time.Now()}); err != nil {
			t.Fatalf("AddMedia: %v", err)
		}
		out, err := f.call(t, ipc.MethodMediaList, `{"gameId":1}`)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		items := out.(ipc.MediaListResult).Media
		want := filepath.Join(f.root, "media", "1", "h1.png")
		if len(items) != 1 || items[0].Path != want || !filepath.IsAbs(items[0].Path) {
			t.Errorf("items = %+v, want path %q", items, want)
		}
	})

	t.Run("MID-005/media.queue enfileira só os tipos pedidos", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "steamgriddb", media: []scrapers.RemoteMedia{
			{Kind: scrapers.KindCover, URL: "https://cdn.example.test/c.png"},
			{Kind: scrapers.KindScreenshot, URL: "https://cdn.example.test/s.png"},
		}})
		out, err := f.call(t, ipc.MethodMediaQueue, `{"gameId":1,"provider":"steamgriddb","providerId":"123","kinds":["cover"]}`)
		if err != nil {
			t.Fatalf("queue: %v", err)
		}
		if n := out.(ipc.MediaQueueResult).Queued; n != 1 {
			t.Errorf("queued = %d, want 1 (cover only)", n)
		}
	})

	t.Run("MID-005/cancelar um job que não existe é -32012", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, nil)
		_, err := f.call(t, ipc.MethodMediaCancel, `{"jobId":999}`)
		if codeOf(err) != ipc.CodeMediaJobNotFound {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeMediaJobNotFound)
		}
	})

	for _, id := range []string{"steamstore", "igdb"} {
		t.Run("MET-012/MET-014/scrape.apply de "+id+" usa Fetch: uma requisição, sem Details nem Media", func(t *testing.T) {
			t.Parallel()
			p := &metaFetchProv{metaProv: metaProv{id: id, meta: scrapers.Metadata{Description: "Texto"},
				media: []scrapers.RemoteMedia{{Kind: scrapers.KindCover, URL: "https://cdn.example.test/c.png"}}}}
			f := newScrapeFixtureWith(t, map[string]scrapers.MetadataProvider{id: p})
			if _, err := f.call(t, ipc.MethodScrapeApply, scrapeJSON(t, map[string]any{"gameId": f.gameID, "provider": id, "providerId": "7"})); err != nil {
				t.Fatalf("scrape.apply: %v", err)
			}
			if n := p.count("Fetch"); n != 1 {
				t.Errorf("Fetch calls = %d, want 1", n)
			}
			if n := p.count("Details") + p.count("Media"); n != 0 {
				t.Errorf("Details and Media calls = %d, want 0 when Fetch exists", n)
			}
			if n := p.totalRequests(); n != 1 {
				t.Errorf("requests = %d, want 1", n)
			}
		})
	}

	for _, bad := range []string{"", "abc", "12a", "-1", "1.5", strings.Repeat("9", 21)} {
		t.Run(fmt.Sprintf("MET-071/providerId %q: scrape.apply e media.queue dão -32602 sem chamar o provedor", bad), func(t *testing.T) {
			t.Parallel()
			p := &metaFetchProv{metaProv: metaProv{id: "steamstore", meta: scrapers.Metadata{Description: "Texto"},
				media: []scrapers.RemoteMedia{{Kind: scrapers.KindCover, URL: "https://cdn.example.test/c.png"}}}}
			f := newScrapeFixtureWith(t, map[string]scrapers.MetadataProvider{"steamstore": p})
			_, err := f.call(t, ipc.MethodScrapeApply, scrapeJSON(t, map[string]any{"gameId": f.gameID, "provider": "steamstore", "providerId": bad}))
			if codeOf(err) != ipc.CodeInvalidParams {
				t.Errorf("scrape.apply: code = %d (%v), want %d", codeOf(err), err, ipc.CodeInvalidParams)
			}
			_, err = f.call(t, ipc.MethodMediaQueue, scrapeJSON(t, map[string]any{"gameId": f.gameID, "provider": "steamstore", "providerId": bad, "kinds": []string{"cover"}}))
			if codeOf(err) != ipc.CodeInvalidParams {
				t.Errorf("media.queue: code = %d (%v), want %d", codeOf(err), err, ipc.CodeInvalidParams)
			}
			if n := p.totalRequests(); n != 0 {
				t.Errorf("provider requests = %d, want 0", n)
			}
		})
	}

	t.Run("MID-008/provedor indisponível vira -32011", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb", err: scrapers.ErrUnavailable})
		_, err := f.call(t, ipc.MethodScrapeSearch, `{"gameId":1,"provider":"igdb"}`)
		if codeOf(err) != ipc.CodeProviderUnavailable {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderUnavailable)
		}
	})
}

// newScrapeFixtureWith is newScrapeFixture with the providers given by name (fakes with or without Fetch).
func newScrapeFixtureWith(t *testing.T, provs map[string]scrapers.MetadataProvider) scrapeFixture {
	t.Helper()
	root := t.TempDir()
	st, err := store.Open(t.Context(), filepath.Join(root, "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	g, err := st.AddGame(t.Context(), store.NewGame{Title: "Celeste", Exe: `C:\c.exe`, At: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	keys := scrapers.NewKeys(&memKeyStore{values: map[string]string{}})
	clock := domain.NewFakeClock(time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	queue := newMediaQueue(st, imageFetcher{files: media.New(filepath.Join(root, "media"), nil)}, clock, log, nil)
	svc := &scrapeService{
		st:   st,
		keys: keys,
		providers: func(name string) (scrapers.MetadataProvider, error) {
			p, ok := provs[name]
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
	return scrapeFixture{st: st, keys: keys, handlers: scrapeHandlers(svc), gameID: g.ID, root: root, queue: queue, svc: svc}
}

func scrapeJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// ---- MET-021..MET-024 (ADR 0034, design §5.1): the image limit of every queue path ----
// Alterado em 2026-11-15: os casos acima continuam valendo; estes são novos. Os casos de scrape.apply e
// media.queue de MID-003/MID-005 não mudam de resultado (há vaga), por isso nenhum deles foi reescrito.

// imageOffer is n remote images of kind offered by provider; the URLs are unique per tag.
func imageOffer(provider, kind, tag string, n int) scrapers.Offer {
	out := scrapers.Offer{Provider: provider}
	for i := range n {
		out.Media = append(out.Media, scrapers.RemoteMedia{
			Kind: kind,
			URL:  fmt.Sprintf("https://cdn.example.test/%s/%s-%d.png", provider, tag, i),
		})
	}
	return out
}

// storeImages records n images of kind as already stored for gameID, whatever their origin (MET-021).
func storeImages(t *testing.T, st *store.Store, gameID int64, provider, kind, tag string, n int) {
	t.Helper()
	at := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	for i := range n {
		if _, err := st.AddMedia(t.Context(), store.MediaRow{
			GameID: gameID, Kind: kind, Provider: provider, SourceURL: "https://cdn.example.test/old/" + tag,
			Hash: fmt.Sprintf("%s-%s-%d", provider, tag, i), Path: fmt.Sprintf("%d/%s-%d.png", gameID, tag, i),
			ThumbPath: fmt.Sprintf("%d/thumb_%s-%d.jpg", gameID, tag, i), CreatedAt: at,
		}); err != nil {
			t.Fatalf("AddMedia: %v", err)
		}
	}
}

// jobSeq makes every queued job's source URL unique.
var jobSeq atomic.Int64

// enqueueIn queues one download of kind for gameID and leaves it in state: store.JobPending,
// store.JobRunning, store.JobFailed, or "cancelled" (CancelMediaJob, which the store records as failed).
func enqueueIn(t *testing.T, st *store.Store, gameID int64, kind, state string) {
	t.Helper()
	ctx := t.Context()
	url := fmt.Sprintf("https://cdn.example.test/job-%d.png", jobSeq.Add(1))
	id, err := st.EnqueueMedia(ctx, gameID, kind, "igdb", url, time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EnqueueMedia: %v", err)
	}
	switch state {
	case store.JobPending:
	case store.JobRunning:
		err = st.SetMediaJobState(ctx, id, store.JobRunning, 1, "")
	case store.JobFailed:
		err = st.SetMediaJobState(ctx, id, store.JobFailed, 3, "falhou")
	case "cancelled":
		err = st.CancelMediaJob(ctx, id)
	default:
		t.Fatalf("enqueueIn: unknown state %q", state)
	}
	if err != nil {
		t.Fatalf("job %d to %s: %v", id, state, err)
	}
}

// activeCounts returns the pending and running jobs of a game by kind (ActiveMediaJobCounts).
func activeCounts(t *testing.T, st *store.Store, gameID int64) map[string]int {
	t.Helper()
	m, err := st.ActiveMediaJobCounts(t.Context(), gameID)
	if err != nil {
		t.Fatalf("ActiveMediaJobCounts: %v", err)
	}
	return m
}

// addGame records another game and returns its id.
func addGame(t *testing.T, st *store.Store, title string) int64 {
	t.Helper()
	g, err := st.AddGame(t.Context(), store.NewGame{Title: title, Exe: `C:\` + title + `.exe`, At: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	return g.ID
}

func TestQueueImagesLimits(t *testing.T) {
	t.Parallel()

	t.Run("MET-021/jogo vazio recebe até o limite por tipo", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
		res, err := f.svc.queueImages(t.Context(), f.gameID, []scrapers.Offer{
			imageOffer("igdb", scrapers.KindScreenshot, "s", 12),
			imageOffer("igdb", scrapers.KindCover, "c", 3),
		}, nil, f.svc.clock.Now())
		if err != nil {
			t.Fatalf("queueImages: %v", err)
		}
		if res.queued != 11 || res.skipped != 4 {
			t.Errorf("result = %+v, want queued 11 (10 screenshots, 1 cover, alterado em 2026-11-15, capa = 1) and skipped 4", res)
		}
		if a := activeCounts(t, f.st, f.gameID); a[scrapers.KindScreenshot] != 10 || a[scrapers.KindCover] != 1 {
			t.Errorf("active jobs = %v, want 10 screenshots and 1 cover", a)
		}
	})

	t.Run("MET-021/jogo com 9 de 10 screenshots recebe 1 e deixa 2 de fora", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
		storeImages(t, f.st, f.gameID, "launchbox", scrapers.KindScreenshot, "old", 9)
		res, err := f.svc.queueImages(t.Context(), f.gameID, []scrapers.Offer{imageOffer("igdb", scrapers.KindScreenshot, "s", 3)}, nil, f.svc.clock.Now())
		if err != nil {
			t.Fatalf("queueImages: %v", err)
		}
		if res.queued != 1 || res.skipped != 2 {
			t.Errorf("result = %+v, want queued 1 and skipped 2", res)
		}
	})

	t.Run("MET-021/jogo com 2 capas não recebe capa; capas do LaunchBox contam", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
		storeImages(t, f.st, f.gameID, "launchbox", scrapers.KindCover, "old", 2)
		res, err := f.svc.queueImages(t.Context(), f.gameID, []scrapers.Offer{imageOffer("igdb", scrapers.KindCover, "c", 2)}, nil, f.svc.clock.Now())
		if err != nil {
			t.Fatalf("queueImages: %v", err)
		}
		if res.queued != 0 || res.skipped != 2 {
			t.Errorf("result = %+v, want queued 0 and skipped 2", res)
		}
		if a := activeCounts(t, f.st, f.gameID); len(a) != 0 {
			t.Errorf("active jobs = %v, want none", a)
		}
	})

	// Jobs ativos contam; falhos e cancelados não (MET-021). Com duas capas ofertadas e limite 1 (capa = 1, 2026-11-15).
	jobCases := []struct {
		name    string
		states  []string
		queued  int
		skipped int
	}{
		{"MET-021/job pendente conta", []string{store.JobPending}, 0, 2}, // alterado em 2026-11-15 (capa = 1)
		{"MET-021/job em andamento conta", []string{store.JobRunning}, 0, 2},
		{"MET-021/pendente e em andamento contam juntos", []string{store.JobPending, store.JobRunning}, 0, 2},
		{"MET-021/job falho não conta", []string{store.JobFailed}, 1, 1},
		{"MET-021/job cancelado não conta", []string{"cancelled"}, 1, 1},
	}
	for _, tc := range jobCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
			for _, s := range tc.states {
				enqueueIn(t, f.st, f.gameID, scrapers.KindCover, s)
			}
			res, err := f.svc.queueImages(t.Context(), f.gameID, []scrapers.Offer{imageOffer("igdb", scrapers.KindCover, "c", 2)}, nil, f.svc.clock.Now())
			if err != nil {
				t.Fatalf("queueImages: %v", err)
			}
			if res.queued != tc.queued || res.skipped != tc.skipped {
				t.Errorf("result = %+v, want queued %d and skipped %d", res, tc.queued, tc.skipped)
			}
		})
	}

	t.Run("MET-021/nada é apagado: imagens e jobs atuais ficam como estavam", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
		storeImages(t, f.st, f.gameID, "launchbox", scrapers.KindCover, "old-c", 2)
		storeImages(t, f.st, f.gameID, "steamgriddb", scrapers.KindScreenshot, "old-s", 9)
		enqueueIn(t, f.st, f.gameID, scrapers.KindScreenshot, store.JobPending)
		before := activeCounts(t, f.st, f.gameID)
		res, err := f.svc.queueImages(t.Context(), f.gameID, []scrapers.Offer{imageOffer("igdb", scrapers.KindCover, "c", 3)}, nil, f.svc.clock.Now())
		if err != nil {
			t.Fatalf("queueImages: %v", err)
		}
		if res.queued != 0 || res.skipped != 3 {
			t.Errorf("result = %+v, want queued 0 and skipped 3", res)
		}
		rows, err := f.st.ListMedia(t.Context(), f.gameID)
		if err != nil || len(rows) != 11 {
			t.Errorf("ListMedia = %d rows, %v; want 11 kept", len(rows), err)
		}
		if after := activeCounts(t, f.st, f.gameID); after[scrapers.KindScreenshot] != before[scrapers.KindScreenshot] {
			t.Errorf("active jobs changed from %v to %v", before, after)
		}
	})

	// MET-024: o resultado conta o que entrou e o que ficou de fora, por tipo.
	t.Run("MET-024/queued e skipped contados por tipo: tela cheia e capas", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
		storeImages(t, f.st, f.gameID, "steamgriddb", scrapers.KindScreenshot, "old", 10)
		res, err := f.svc.queueImages(t.Context(), f.gameID, []scrapers.Offer{
			imageOffer("igdb", scrapers.KindScreenshot, "s", 3),
			imageOffer("igdb", scrapers.KindCover, "c", 2),
		}, nil, f.svc.clock.Now())
		if err != nil {
			t.Fatalf("queueImages: %v", err)
		}
		if res.queued != 1 || res.skipped != 4 {
			t.Errorf("result = %+v, want queued 1 (one cover vacancy) and skipped 4 (3 screenshots, 1 cover; alterado em 2026-11-15, capa = 1)", res)
		}
	})

	// MET-024: o aviso à fila só sai quando algo foi enfileirado.
	t.Run("MET-024/queue.Notify só é chamado quando enfileirou", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
		storeImages(t, f.st, f.gameID, "launchbox", scrapers.KindCover, "old", 2)
		if _, err := f.svc.queueImages(t.Context(), f.gameID, []scrapers.Offer{imageOffer("igdb", scrapers.KindCover, "c", 2)}, nil, f.svc.clock.Now()); err != nil {
			t.Fatalf("queueImages (full): %v", err)
		}
		if n := len(f.queue.wake); n != 0 {
			t.Errorf("Notify sent %d wake-ups with nothing queued, want 0", n)
		}
		other := addGame(t, f.st, "Hades")
		if _, err := f.svc.queueImages(t.Context(), other, []scrapers.Offer{imageOffer("igdb", scrapers.KindCover, "c", 1)}, nil, f.svc.clock.Now()); err != nil {
			t.Fatalf("queueImages: %v", err)
		}
		if n := len(f.queue.wake); n != 1 {
			t.Errorf("wake-ups after queueing = %d, want 1", n)
		}
	})
}

func TestQueuePaths(t *testing.T) {
	t.Parallel()

	// MET-022: scrape.apply passa pela mesma função e pelo mesmo limite.
	t.Run("MET-022/mesmo jogo com 1 vaga de screenshot enfileira 1 pelo scrape.apply", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{
			id:    "igdb",
			meta:  scrapers.Metadata{Description: "Escalada."},
			media: imageOffer("igdb", scrapers.KindScreenshot, "s", 3).Media,
		})
		storeImages(t, f.st, f.gameID, "launchbox", scrapers.KindScreenshot, "old", 9)
		out, err := f.call(t, ipc.MethodScrapeApply, fmt.Sprintf(`{"gameId":%d,"provider":"igdb","providerId":"7"}`, f.gameID))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if q := out.(ipc.ScrapeApplyResult).Queued; q != 1 {
			t.Errorf("queued = %d, want 1", q)
		}
		if a := activeCounts(t, f.st, f.gameID); a[scrapers.KindScreenshot] != 1 {
			t.Errorf("active screenshot jobs = %d, want 1", a[scrapers.KindScreenshot])
		}
	})

	// MET-022: media.queue passa pela mesma função e pelo mesmo limite.
	t.Run("MET-022/mesmo jogo com 1 vaga de screenshot enfileira 1 pelo media.queue", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{
			id:    "igdb",
			media: imageOffer("igdb", scrapers.KindScreenshot, "s", 3).Media,
		})
		storeImages(t, f.st, f.gameID, "launchbox", scrapers.KindScreenshot, "old", 9)
		out, err := f.call(t, ipc.MethodMediaQueue, fmt.Sprintf(`{"gameId":%d,"provider":"igdb","providerId":"7","kinds":["screenshot"]}`, f.gameID))
		if err != nil {
			t.Fatalf("queue: %v", err)
		}
		if q := out.(ipc.MediaQueueResult).Queued; q != 1 {
			t.Errorf("queued = %d, want 1", q)
		}
	})

	t.Run("MET-022/media.queue com tipos explícitos limita cada tipo pedido", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{
			id:    "igdb",
			media: append(imageOffer("igdb", scrapers.KindCover, "c", 3).Media, imageOffer("igdb", scrapers.KindScreenshot, "s", 3).Media...),
		})
		storeImages(t, f.st, f.gameID, "launchbox", scrapers.KindCover, "old-c", 2)
		storeImages(t, f.st, f.gameID, "launchbox", scrapers.KindScreenshot, "old-s", 9)

		// Só capa pedida: as duas vagas de capa já estão ocupadas.
		out, err := f.call(t, ipc.MethodMediaQueue, fmt.Sprintf(`{"gameId":%d,"provider":"igdb","providerId":"7","kinds":["cover"]}`, f.gameID))
		if err != nil {
			t.Fatalf("queue cover: %v", err)
		}
		if q := out.(ipc.MediaQueueResult).Queued; q != 0 {
			t.Errorf("cover only: queued = %d, want 0 (cover full)", q)
		}

		// Capa e screenshot pedidas: só a vaga de screenshot entra.
		out, err = f.call(t, ipc.MethodMediaQueue, fmt.Sprintf(`{"gameId":%d,"provider":"igdb","providerId":"7","kinds":["cover","screenshot"]}`, f.gameID))
		if err != nil {
			t.Fatalf("queue both: %v", err)
		}
		if q := out.(ipc.MediaQueueResult).Queued; q != 1 {
			t.Errorf("cover and screenshot: queued = %d, want 1 (one screenshot vacancy)", q)
		}
		if a := activeCounts(t, f.st, f.gameID); a[scrapers.KindCover] != 0 || a[scrapers.KindScreenshot] != 1 {
			t.Errorf("active jobs = %v, want only 1 screenshot", a)
		}
	})
}

// gatedCount is a store whose first read of the active jobs waits until a second request reads them
// too, or a quiet window passes. A request not serialised per game shows up as a second concurrent
// reader (peak > 1) while the first one is still inside the gate.
type gatedCount struct {
	*store.Store
	arrived chan struct{}

	mu     sync.Mutex
	reads  int
	inside int
	peak   int
}

func (g *gatedCount) ActiveMediaJobCounts(ctx context.Context, gameID int64) (map[string]int, error) {
	g.mu.Lock()
	g.reads++
	first := g.reads == 1
	g.inside++
	g.peak = max(g.peak, g.inside)
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		g.inside--
		g.mu.Unlock()
	}()
	if first {
		select {
		case <-g.arrived:
		case <-time.After(quietWindow):
		}
	} else {
		select {
		case g.arrived <- struct{}{}:
		default:
		}
	}
	return g.Store.ActiveMediaJobCounts(ctx, gameID)
}

// failingEnqueue is a store whose downloads cannot be recorded.
type failingEnqueue struct{ *store.Store }

func (failingEnqueue) EnqueueMedia(context.Context, int64, string, string, string, time.Time) (int64, error) {
	return 0, errors.New("disco cheio")
}

func TestQueueLocks(t *testing.T) {
	t.Parallel()

	t.Run("MET-023/dois pedidos concorrentes do mesmo jogo não passam do limite (-race)", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
		gated := &gatedCount{Store: f.st, arrived: make(chan struct{}, 1)}
		svc := &scrapeService{st: gated, queue: f.queue, clock: f.svc.clock, log: f.svc.log}
		offers := []scrapers.Offer{imageOffer("igdb", scrapers.KindCover, "c", 2)}

		results := make(chan queueResult, 2)
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res, err := svc.queueImages(t.Context(), f.gameID, offers, nil, f.svc.clock.Now())
				results <- res
				errs <- err
			}()
		}
		wg.Wait()
		close(results)
		close(errs)

		for err := range errs {
			if err != nil {
				t.Errorf("queueImages: %v", err)
			}
		}
		total := 0
		for res := range results {
			total += res.queued
		}
		if gated.peak != 1 {
			t.Errorf("concurrent reads of the active jobs = %d, want 1 (serialised per game)", gated.peak)
		}
		if total > 1 {
			t.Errorf("queued %d covers in two requests, want at most 1 (the limit, alterado em 2026-11-15, capa = 1)", total)
		}
		if a := activeCounts(t, f.st, f.gameID); a[scrapers.KindCover] != total {
			t.Errorf("active cover jobs = %d, want %d (what was queued)", a[scrapers.KindCover], total)
		}
	})

	t.Run("MET-023/jogos diferentes não se bloqueiam (um lock preso não atrasa o outro)", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
		other := addGame(t, f.st, "Hades")
		unlock := f.svc.locks.lock(f.gameID)
		defer unlock()

		done := make(chan error, 1)
		go func() {
			_, err := f.svc.queueImages(t.Context(), other, []scrapers.Offer{imageOffer("igdb", scrapers.KindCover, "c", 1)}, nil, f.svc.clock.Now())
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("queueImages: %v", err)
			}
		case <-time.After(testTimeout):
			t.Fatalf("game %d waited for the lock held by game %d", other, f.gameID)
		}
	})

	t.Run("MET-023/o lock é liberado em erro do store", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
		svc := &scrapeService{st: failingEnqueue{f.st}, queue: f.queue, clock: f.svc.clock, log: f.svc.log}
		if _, err := svc.queueImages(t.Context(), f.gameID, []scrapers.Offer{imageOffer("igdb", scrapers.KindCover, "c", 1)}, nil, f.svc.clock.Now()); err == nil {
			t.Fatal("queueImages with a failing store: want an error")
		}
		done := make(chan struct{})
		go func() {
			unlock := svc.locks.lock(f.gameID)
			unlock()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Fatal("the lock of the game is still held after a store error")
		}
	})

	t.Run("MET-023/mapa de locks não vaza entradas depois de liberar", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb"})
		other := addGame(t, f.st, "Hades")
		for _, id := range []int64{f.gameID, other, f.gameID} {
			unlock := f.svc.locks.lock(id)
			unlock()
		}
		if _, err := f.svc.queueImages(t.Context(), f.gameID, []scrapers.Offer{imageOffer("igdb", scrapers.KindCover, "c", 1)}, nil, f.svc.clock.Now()); err != nil {
			t.Fatalf("queueImages: %v", err)
		}
		f.svc.locks.mu.Lock()
		n := len(f.svc.locks.m)
		f.svc.locks.mu.Unlock()
		if n != 0 {
			t.Errorf("lock map keeps %d entries after release, want 0", n)
		}
	})
}
