package scrapers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// T47 (VID-028): redirecionamentos so para o mesmo host. Dois httptest (portas diferentes = hosts diferentes).

func TestRedirects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// sameHostHandler redirects any non-"/final/" path to "/final"+path (query kept) and serves content there.
	sameHostHandler := func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasPrefix(req.URL.Path, "/final/") {
			http.Redirect(w, req, "/final"+req.URL.Path+"?"+req.URL.RawQuery, http.StatusFound)
			return
		}
		switch {
		case strings.HasSuffix(req.URL.Path, "jeuInfos.php"):
			ssJSON(ssInfoJSON)(w, req)
		case strings.HasSuffix(req.URL.Path, "jeuRecherche.php"):
			ssJSON(ssSearchJSON)(w, req)
		default:
			ssVideo(w, req)
		}
	}

	t.Run("VID-028/busca, informacoes e video redirecionados ao mesmo host sao seguidos", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, sameHostHandler, nil)
		ssRun(t, r.clock, 2_000_000_000, func() {
			if _, _, err := r.ss.SearchGames(ctx, 4, "x"); err != nil {
				t.Errorf("busca: %v", err)
			}
			if _, _, err := r.ss.GameInfo(ctx, 4, "1234"); err != nil {
				t.Errorf("informacoes: %v", err)
			}
			rc, _, err := r.ss.OpenVideo(ctx, r.srv.URL+"/v.mp4")
			if err != nil {
				t.Errorf("video: %v", err)
				return
			}
			_ = rc.Close()
		})
	})

	t.Run("VID-028/redirecionamento a outro host: o segundo servidor conta zero, ErrBadRequest e nao ErrUnavailable", func(t *testing.T) {
		t.Parallel()
		var other atomic.Int32
		otherSrv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { other.Add(1) }))
		t.Cleanup(otherSrv.Close)
		r := newSSRig(t, func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, otherSrv.URL+"/steal?"+req.URL.RawQuery, http.StatusFound)
		}, nil)
		ssRun(t, r.clock, 2_000_000_000, func() {
			check := func(what string, err error) {
				if !errors.Is(err, scrapers.ErrBadRequest) || errors.Is(err, scrapers.ErrUnavailable) ||
					!strings.Contains(err.Error(), "redirecionamento para fora do ScreenScraper") {
					t.Errorf("%s: err = %v", what, err)
				}
				ssNoMarks(t, what+" erro", err.Error())
			}
			_, _, err := r.ss.SearchGames(ctx, 4, "x")
			check("busca", err)
			_, _, err = r.ss.GameInfo(ctx, 4, "1")
			check("informacoes", err)
			rc, _, err := r.ss.OpenVideo(ctx, r.srv.URL+"/v.mp4")
			if rc != nil {
				_ = rc.Close()
			}
			check("video", err)
		})
		if other.Load() != 0 {
			t.Errorf("segundo servidor recebeu %d requisicoes, want 0", other.Load())
		}
		ssNoMarks(t, "log", r.log.String())
	})

	t.Run("VID-028/mais de 10 redirecionamentos no mesmo host continuam falhando como no net/http", func(t *testing.T) {
		t.Parallel()
		r := newSSRig(t, func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, req.URL.Path+"r?"+req.URL.RawQuery, http.StatusFound)
		}, nil)
		var err error
		ssRun(t, r.clock, 2_000_000_000, func() { _, _, err = r.ss.GameInfo(ctx, 4, "1") })
		if err == nil {
			t.Fatal("err = nil, want falha pelo teto de redirecionamentos")
		}
		if r.hits.Load() > 11 {
			t.Errorf("hits = %d, want <= 11", r.hits.Load())
		}
	})

	t.Run("VID-028/o *http.Client passado em SSConfig.HTTP nao e alterado", func(t *testing.T) {
		t.Parallel()
		hc := &http.Client{}
		_ = scrapers.NewScreenScraper(scrapers.SSConfig{BaseURL: "http://127.0.0.1:1/", HTTP: hc, Creds: ssGoodCreds})
		if hc.CheckRedirect != nil {
			t.Error("CheckRedirect do cliente compartilhado foi alterado")
		}
	})
}
