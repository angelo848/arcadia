package scrapers_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

const tbWantBody = "fields game_id,hastily,normally,completely; where game_id = 1234; limit 1;"

type tbFake struct {
	srv *httptest.Server

	mu       sync.Mutex
	tokens   int
	ttbPaths []string
	bodies   []string
	headers  []http.Header
	status   int
	body     string
}

func tbFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("testdata: %v", err)
	}
	return string(b)
}

func newTBFake(t *testing.T, status int, body string) *tbFake {
	t.Helper()
	f := &tbFake{status: status, body: body}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/oauth2/token" {
			f.tokens++
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"tw-tok","expires_in":3600}`)
			return
		}
		f.ttbPaths = append(f.ttbPaths, r.URL.Path)
		f.bodies = append(f.bodies, string(b))
		f.headers = append(f.headers, r.Header.Clone())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, f.body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *tbFake) igdb(cfg scrapers.IGDBConfig) *scrapers.IGDB {
	if cfg.ClientID == "" {
		cfg.ClientID, cfg.Secret = "cid", "sec"
	}
	cfg.APIBase = f.srv.URL + "/v4"
	cfg.TokenURL = f.srv.URL + "/oauth2/token"
	if cfg.HTTP == nil {
		cfg.HTTP = f.srv.Client()
	}
	return scrapers.NewIGDB(cfg)
}

func (f *tbFake) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

func tbCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestIGDBTimeToBeat(t *testing.T) {
	t.Parallel()

	t.Run("TTB-036/o corpo é fields game_id,hastily,normally,completely; where game_id = id; limit 1; com Client-ID e Bearer", func(t *testing.T) {
		t.Parallel()
		f := newTBFake(t, 200, tbFile(t, "igdb_time_to_beats.json"))
		if _, _, err := f.igdb(scrapers.IGDBConfig{ClientID: "meu-cid", Secret: "sec"}).TimeToBeat(tbCtx(t), "1234"); err != nil {
			t.Fatal(err)
		}
		if f.calls() != 1 {
			t.Fatalf("calls = %d", f.calls())
		}
		if f.ttbPaths[0] != "/v4/game_time_to_beats" {
			t.Errorf("path = %q", f.ttbPaths[0])
		}
		if f.bodies[0] != tbWantBody {
			t.Errorf("body = %q, want %q", f.bodies[0], tbWantBody)
		}
		h := f.headers[0]
		if h.Get("Client-ID") != "meu-cid" || h.Get("Authorization") != "Bearer tw-tok" {
			t.Errorf("headers Client-ID %q Authorization %q", h.Get("Client-ID"), h.Get("Authorization"))
		}
	})

	t.Run("TTB-036/hastily vira História principal, normally História + extras, completely 100%", func(t *testing.T) {
		t.Parallel()
		f := newTBFake(t, 200, tbFile(t, "igdb_time_to_beats.json"))
		got, ok, err := f.igdb(scrapers.IGDBConfig{}).TimeToBeat(tbCtx(t), "1234")
		if err != nil || !ok {
			t.Fatalf("ok %v err %v", ok, err)
		}
		want := scrapers.IGDBTimeToBeat{Hastily: 36000, Normally: 54000, Completely: 108000}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("TTB-036/campos ausentes, zero, negativo ou não inteiro viram 0 (nulo) sem afetar os outros", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			file string
			want scrapers.IGDBTimeToBeat
		}{
			{"igdb_time_to_beats_missing.json", scrapers.IGDBTimeToBeat{Normally: 54000}},
			{"igdb_time_to_beats_zeros.json", scrapers.IGDBTimeToBeat{}},
			{"igdb_time_to_beats_invalid.json", scrapers.IGDBTimeToBeat{}},
		}
		for _, tc := range cases {
			f := newTBFake(t, 200, tbFile(t, tc.file))
			got, ok, err := f.igdb(scrapers.IGDBConfig{}).TimeToBeat(tbCtx(t), "1234")
			if err != nil {
				t.Fatalf("%s: %v", tc.file, err)
			}
			if !ok {
				t.Errorf("%s: ok = false, a record exists", tc.file)
			}
			if got != tc.want {
				t.Errorf("%s: got %+v, want %+v", tc.file, got, tc.want)
			}
		}
	})

	t.Run("TTB-036/lista vazia devolve ok = false sem erro", func(t *testing.T) {
		t.Parallel()
		f := newTBFake(t, 200, tbFile(t, "igdb_time_to_beats_empty.json"))
		got, ok, err := f.igdb(scrapers.IGDBConfig{}).TimeToBeat(tbCtx(t), "1234")
		if err != nil || ok || got != (scrapers.IGDBTimeToBeat{}) {
			t.Errorf("got %+v ok %v err %v", got, ok, err)
		}
	})

	t.Run("TTB-037/sem chave devolve ErrNoKey e nenhuma requisição", func(t *testing.T) {
		t.Parallel()
		f := newTBFake(t, 200, tbFile(t, "igdb_time_to_beats.json"))
		p := scrapers.NewIGDB(scrapers.IGDBConfig{
			APIBase: f.srv.URL + "/v4", TokenURL: f.srv.URL + "/oauth2/token", HTTP: f.srv.Client(),
		})
		_, ok, err := p.TimeToBeat(tbCtx(t), "1234")
		if !errors.Is(err, scrapers.ErrNoKey) || ok {
			t.Fatalf("ok %v err %v, want ErrNoKey", ok, err)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.tokens != 0 || len(f.bodies) != 0 {
			t.Errorf("requests made without a key: tokens %d calls %d", f.tokens, len(f.bodies))
		}
	})

	t.Run("TTB-038/429, 5xx e rede são ErrUnavailable", func(t *testing.T) {
		t.Parallel()
		for _, st := range []int{429, 500, 503} {
			f := newTBFake(t, st, `{"message":"segredo"}`)
			_, ok, err := f.igdb(scrapers.IGDBConfig{}).TimeToBeat(tbCtx(t), "1234")
			if !errors.Is(err, scrapers.ErrUnavailable) || ok {
				t.Errorf("status %d: ok %v err %v, want ErrUnavailable", st, ok, err)
			}
		}
		f := newTBFake(t, 200, "[]")
		p := f.igdb(scrapers.IGDBConfig{})
		f.srv.Close()
		if _, _, err := p.TimeToBeat(tbCtx(t), "1234"); !errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("closed server: err = %v, want ErrUnavailable", err)
		}
	})

	t.Run("TTB-038/a chamada passa pelo Limiter do IGDB (o mesmo da busca de metadados)", func(t *testing.T) {
		t.Parallel()
		f := newTBFake(t, 200, tbFile(t, "igdb_time_to_beats.json"))
		clock := domain.NewFakeClock(igStart)
		p := f.igdb(scrapers.IGDBConfig{Clock: clock, Limiter: scrapers.NewLimiter(clock, scrapers.IntervalIGDB)})
		if _, _, err := p.TimeToBeat(tbCtx(t), "1234"); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, _, err := p.TimeToBeat(tbCtx(t), "1234"); done <- err }()
		if err := clock.BlockUntilWaiters(tbCtx(t), 1); err != nil {
			t.Fatalf("the second call did not wait for the limiter: %v", err)
		}
		if f.calls() != 1 {
			t.Fatalf("calls before the interval = %d, want 1", f.calls())
		}
		clock.Advance(scrapers.IntervalIGDB)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("second call did not finish")
		}
		if f.calls() != 2 {
			t.Errorf("calls = %d, want 2", f.calls())
		}
	})

	t.Run("TTB-038/ctx cancelado devolve ctx.Err()", func(t *testing.T) {
		t.Parallel()
		f := newTBFake(t, 200, tbFile(t, "igdb_time_to_beats.json"))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := f.igdb(scrapers.IGDBConfig{}).TimeToBeat(ctx, "1234")
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cancel") && !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want a cancellation", err)
		}
	})
}
