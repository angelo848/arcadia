package scrapers

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// T46 (VID-027): vetores de rules §10. Teste de caixa branca porque checkMediaOrigin nao e exportada.
// Assinatura assumida: checkMediaOrigin(base *url.URL, raw string) (*url.URL, error).

func TestCheckMediaOrigin(t *testing.T) {
	t.Parallel()
	const prod = "https://api.screenscraper.fr/api2/"
	const local = "http://127.0.0.1:5000/"
	cases := []struct {
		name string
		base string
		raw  string
		ok   bool
	}{
		{"mesmo host", prod, "https://api.screenscraper.fr/api2/mediaVideoJeu.php?systemeid=4&jeuid=1&media=video-normalized", true},
		{"sufixo", prod, "https://neoclone.screenscraper.fr/api2/mediaVideoJeu.php?x=1", true},
		{"caixa", prod, "HTTPS://NEOCLONE.SCREENSCRAPER.FR/v.mp4", true},
		{"http com base https", prod, "http://neoclone.screenscraper.fr/v.mp4", false},
		{"sufixo falso com ponto", prod, "https://screenscraper.fr.evil.example/v.mp4", false},
		{"sem ponto antes do sufixo", prod, "https://evilscreenscraper.fr/v.mp4", false},
		{"userinfo disfarcado", prod, "https://x.screenscraper.fr@evil.example/v.mp4", false},
		{"ftp", prod, "ftp://neoclone.screenscraper.fr/v.mp4", false},
		{"relativa", prod, "/api2/mediaVideoJeu.php?x=1", false},
		{"sem esquema", prod, "//evil.example/v.mp4", false},
		{"invalida", prod, "%zz", false},
		{"httptest mesmo host", local, "http://127.0.0.1:5000/v.mp4", true},
		{"httptest outra porta", local, "http://127.0.0.1:5001/v.mp4", false},
		{"httptest http fora do host da base", local, "http://neoclone.screenscraper.fr/v.mp4", false},
	}
	for _, tc := range cases {
		t.Run("VID-027/checkMediaOrigin: "+tc.name, func(t *testing.T) {
			t.Parallel()
			base, err := url.Parse(tc.base)
			if err != nil {
				t.Fatal(err)
			}
			u, err := checkMediaOrigin(base, tc.raw)
			if tc.ok {
				if err != nil || u == nil {
					t.Fatalf("checkMediaOrigin(%q) = %v, %v; want aceita", tc.raw, u, err)
				}
				return
			}
			if !errors.Is(err, ErrBadRequest) || !strings.Contains(err.Error(), "URL de vídeo fora do ScreenScraper") {
				t.Fatalf("checkMediaOrigin(%q) err = %v; want ErrBadRequest com a mensagem", tc.raw, err)
			}
		})
	}
}

// T47 (VID-028): sameHostRedirect com requisicoes montadas, sem rede.
func TestSameHostRedirect(t *testing.T) {
	t.Parallel()
	mk := func(rawURL string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	t.Run("VID-028/redirecionamento de https para http no mesmo host e recusado", func(t *testing.T) {
		t.Parallel()
		err := sameHostRedirect(mk("http://api.screenscraper.fr/b"), []*http.Request{mk("https://api.screenscraper.fr/a")})
		if !errors.Is(err, errRedirectOffHost) {
			t.Fatalf("err = %v, want errRedirectOffHost", err)
		}
	})
	t.Run("VID-028/mesmo host com caixa diferente e seguido; porta diferente nao", func(t *testing.T) {
		t.Parallel()
		if err := sameHostRedirect(mk("https://API.screenscraper.fr/b"), []*http.Request{mk("https://api.screenscraper.fr/a")}); err != nil {
			t.Errorf("caixa: err = %v", err)
		}
		if err := sameHostRedirect(mk("https://api.screenscraper.fr:444/b"), []*http.Request{mk("https://api.screenscraper.fr/a")}); !errors.Is(err, errRedirectOffHost) {
			t.Errorf("porta: err = %v", err)
		}
	})
	t.Run("VID-028/outro host e recusado", func(t *testing.T) {
		t.Parallel()
		if err := sameHostRedirect(mk("https://evil.example/b"), []*http.Request{mk("https://api.screenscraper.fr/a")}); !errors.Is(err, errRedirectOffHost) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("VID-028/teto de 10 redirecionamentos continua", func(t *testing.T) {
		t.Parallel()
		via := make([]*http.Request, 10)
		for i := range via {
			via[i] = mk("https://api.screenscraper.fr/a")
		}
		if err := sameHostRedirect(mk("https://api.screenscraper.fr/b"), via); err == nil {
			t.Error("10 saltos: err = nil, want erro do teto")
		}
	})
}

// T48 (VID-029): classifyMediaText. Assinatura assumida: classifyMediaText(body []byte) error.
func TestClassifyMediaText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		body string
		want error
	}{
		{"Votre quota de scrape est dépassée", ErrQuota},
		{"Compte blacklisté", ErrCredentials},
		{"API closed", ErrUnavailable},
		{"API totalement fermé", ErrUnavailable},
		{"Jeu non trouvée", ErrNotFound},
		{"Erreur : Rom non trouvée", ErrNotFound},
		{"NOMEDIA", ErrNotVideo},
	}
	for _, tc := range cases {
		t.Run("VID-029/classifyMediaText: "+tc.body, func(t *testing.T) {
			t.Parallel()
			if err := classifyMediaText([]byte(tc.body)); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
