package scrapers_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// T46 (VID-027) e T48 (VID-029): origem da URL de midia e 200 text/* no download.
// Assinaturas assumidas: scrapers.ErrNotVideo (exportado); OpenVideo confere a origem antes de Creds e do Limiter.

// ssVideo serves an mp4 header with an explicit video/mp4 Content-Type (no sniffing to text/plain).
func ssVideo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "video/mp4")
	_, _ = io.WriteString(w, "0000ftypmp42")
}

func ssNoMarks(t *testing.T, what, s string) {
	t.Helper()
	for _, m := range ssMarks {
		if strings.Contains(s, m) {
			t.Errorf("%s contem a credencial %q: %s", what, m, s)
		}
	}
}

func TestMediaOriginOpenVideo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("VID-027/URL recusada: ErrBadRequest, nenhuma requisicao, Creds nao chamada e Limiter nao reservado", func(t *testing.T) {
		t.Parallel()
		var credCalls atomic.Int32
		r := newSSRig(t, ssStatus(200, "0000ftypmp42"), func() (scrapers.SSCreds, error) {
			credCalls.Add(1)
			return ssGoodCreds()
		})
		var alien atomic.Int32
		alienSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { alien.Add(1) }))
		t.Cleanup(alienSrv.Close)

		for _, raw := range []string{alienSrv.URL + "/v.mp4", "https://evil.example/v.mp4", "/rel.mp4", "%zz"} {
			rc, _, err := r.ss.OpenVideo(ctx, raw)
			if rc != nil {
				_ = rc.Close()
			}
			if !errors.Is(err, scrapers.ErrBadRequest) || !strings.Contains(err.Error(), "URL de vídeo fora do ScreenScraper") {
				t.Errorf("OpenVideo(%q) err = %v", raw, err)
			}
		}
		if r.hits.Load() != 0 || alien.Load() != 0 {
			t.Errorf("requisicoes: base=%d alheio=%d, want 0 e 0", r.hits.Load(), alien.Load())
		}
		if credCalls.Load() != 0 {
			t.Errorf("Creds chamada %d vezes, want 0", credCalls.Load())
		}
		// The limiter was not reserved: the next request does not wait on the fake clock.
		done := make(chan error, 1)
		go func() { _, _, err := r.ss.GameInfo(ctx, 4, "1"); done <- err }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("a requisicao seguinte esperou o relogio: o Limiter foi reservado pela URL recusada")
		}
	})

	t.Run("VID-027/URL recusada: valores marcados das credenciais nao aparecem no erro nem no log", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssStatus(200, ""), nil)
		_, _, err := r.ss.OpenVideo(ctx, "https://evil.example/v.mp4?devid="+ssDevID+"&ssid="+ssUser)
		if err == nil {
			t.Fatal("err = nil")
		}
		ssNoMarks(t, "erro", err.Error())
		ssNoMarks(t, "log", r.log.String())
	})

	t.Run("VID-027/URL aceita (mesmo host do httptest) recebe as quatro credenciais e softname como antes", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssVideo, nil)
		rc, _, err := r.ss.OpenVideo(ctx, r.srv.URL+"/v.mp4?systemeid=4")
		if err != nil {
			t.Fatalf("OpenVideo: %v", err)
		}
		_ = rc.Close()
		q := r.query(0)
		if q.Get("devid") != ssDevID || q.Get("devpassword") != ssDevPw || q.Get("ssid") != ssUser || q.Get("sspassword") != ssUserPw {
			t.Errorf("query = %v", q)
		}
	})

	t.Run("VID-027/http aceito so com BaseURL http e no mesmo host; com BaseURL https um http de *.screenscraper.fr e recusado", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, ssVideo, nil)
		if rc, _, err := r.ss.OpenVideo(ctx, r.srv.URL+"/v.mp4"); err != nil {
			t.Errorf("http no mesmo host: %v", err)
		} else {
			_ = rc.Close()
		}
		if _, _, err := r.ss.OpenVideo(ctx, "http://neoclone.screenscraper.fr/v.mp4"); !errors.Is(err, scrapers.ErrBadRequest) {
			t.Errorf("http fora do host da base: err = %v", err)
		}
		prod := scrapers.NewScreenScraper(scrapers.SSConfig{
			BaseURL: "https://api.screenscraper.fr/api2/",
			Limiter: scrapers.NewLimiter(domain.NewFakeClock(ssT0), scrapers.IntervalScreenScraper),
			Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			Creds:   ssGoodCreds,
		})
		if _, _, err := prod.OpenVideo(ctx, "http://neoclone.screenscraper.fr/v.mp4"); !errors.Is(err, scrapers.ErrBadRequest) {
			t.Errorf("BaseURL https com http: err = %v", err)
		}
	})
}

// spyBody counts bytes read and records Close.
type spyBody struct {
	r      io.Reader
	n      atomic.Int64
	closed atomic.Bool
}

func (s *spyBody) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	s.n.Add(int64(n))
	return n, err
}

func (s *spyBody) Close() error { s.closed.Store(true); return nil }

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMediaTextBody(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	textHandler := func(ct, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", ct)
			_, _ = io.WriteString(w, body)
		}
	}
	cases := []struct {
		name string
		ct   string
		body string
		want error
	}{
		{"200 text/plain com \"Votre quota de scrape est dépassée\" vira ErrQuota", "text/plain", "Votre quota de scrape est dépassée", scrapers.ErrQuota},
		{"200 text/html com \"blacklisté\" vira ErrCredentials", "text/html", "Votre logiciel est blacklisté", scrapers.ErrCredentials},
		{"200 text/plain com \"API closed\" vira ErrUnavailable", "text/plain", "API closed for non-members", scrapers.ErrUnavailable},
		{"200 text/plain com \"non trouvée\" vira ErrNotFound", "text/plain", "Jeu non trouvée", scrapers.ErrNotFound},
		{"200 text/plain \"NOMEDIA\" vira ErrNotVideo", "text/plain", "NOMEDIA", scrapers.ErrNotVideo},
		{"Content-Type com parametros e caixa variada (text/plain; charset=utf-8) e reconhecido", "text/plain; charset=utf-8", "Votre quota de scrape est dépassée", scrapers.ErrQuota},
		{"Content-Type com parametros e caixa variada (TEXT/HTML) e reconhecido", "TEXT/HTML", "NOMEDIA", scrapers.ErrNotVideo},
	}
	for _, tc := range cases {
		t.Run("VID-029/"+tc.name, func(t *testing.T) {
			t.Parallel()
			r := newSSRig(t, textHandler(tc.ct, tc.body), nil)
			rc, _, err := r.ss.OpenVideo(ctx, r.srv.URL+"/v.mp4")
			if rc != nil {
				_ = rc.Close()
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("VID-029/video/mp4 e Content-Type ausente seguem para o corpo como antes", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, textHandler("video/mp4", "0000ftypmp42"), nil)
		rc, _, err := r.ss.OpenVideo(ctx, r.srv.URL+"/v.mp4")
		if err != nil {
			t.Fatalf("video/mp4: %v", err)
		}
		_ = rc.Close()
		r2 := newSSRig(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header()["Content-Type"] = nil // suppress sniffing
			_, _ = io.WriteString(w, "0000ftypmp42")
		}, nil)
		rc, _, err = r2.ss.OpenVideo(ctx, r2.srv.URL+"/v.mp4")
		if err != nil {
			t.Fatalf("sem Content-Type: %v", err)
		}
		_ = rc.Close()
	})

	t.Run("VID-029/corpo de texto lido no maximo ate 4.096 bytes e fechado (leitor espiao)", func(t *testing.T) {
		t.Parallel()
		spy := &spyBody{r: strings.NewReader("NOMEDIA" + strings.Repeat("x", 100000))}
		hc := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			h := http.Header{}
			h.Set("Content-Type", "text/plain")
			return &http.Response{StatusCode: 200, Header: h, Body: spy, ContentLength: -1, Request: req}, nil
		})}
		ss := scrapers.NewScreenScraper(scrapers.SSConfig{
			BaseURL: "http://127.0.0.1:1/api2/",
			HTTP:    hc,
			Limiter: scrapers.NewLimiter(domain.NewFakeClock(ssT0), scrapers.IntervalScreenScraper),
			Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			Creds:   ssGoodCreds,
		})
		rc, _, err := ss.OpenVideo(ctx, "http://127.0.0.1:1/v.mp4")
		if rc != nil {
			_ = rc.Close()
		}
		if !errors.Is(err, scrapers.ErrNotVideo) {
			t.Fatalf("err = %v, want ErrNotVideo", err)
		}
		if spy.n.Load() > 4096 {
			t.Errorf("leu %d bytes, want <= 4096", spy.n.Load())
		}
		if !spy.closed.Load() {
			t.Error("corpo nao foi fechado")
		}
	})
}
