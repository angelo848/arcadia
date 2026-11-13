package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
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
	queue := newMediaQueue(st, media.New(filepath.Join(root, "media"), nil), clock, log)
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
	return scrapeFixture{st: st, keys: keys, handlers: scrapeHandlers(svc), gameID: g.ID, root: root, provider: p, queue: queue}
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

	t.Run("MID-008/provedor indisponível vira -32011", func(t *testing.T) {
		t.Parallel()
		f := newScrapeFixture(t, &fakeProvider{id: "igdb", err: scrapers.ErrUnavailable})
		_, err := f.call(t, ipc.MethodScrapeSearch, `{"gameId":1,"provider":"igdb"}`)
		if codeOf(err) != ipc.CodeProviderUnavailable {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderUnavailable)
		}
	})
}

func scrapeJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}
