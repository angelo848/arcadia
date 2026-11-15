package scrapers_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// Assumed signatures (tasks.md T20):
//
//	func RedactURL(raw string) string
//	func RedactError(err error) error
func TestRedactURL(t *testing.T) {
	t.Parallel()

	t.Run("VID-020/RedactURL remove os 4 parâmetros em qualquer caixa e mantém os outros, a ordem e o fragmento", func(t *testing.T) {
		t.Parallel()
		cases := []struct{ in, want string }{
			{
				"https://api.example.test/x.php?devid=A&Foo=1&SSID=b&bar=2&devpassword=p&sspassword=q&z=9#frag",
				"https://api.example.test/x.php?Foo=1&bar=2&z=9#frag",
			},
			{"https://h.test/p?DEVID=a&DevPassword=b&SsId=c&SSPASSWORD=d&keep=1", "https://h.test/p?keep=1"},
			{"https://h.test/p?a=1&devid=x", "https://h.test/p?a=1"},
			{"https://h.test/p?devid=x&a=1", "https://h.test/p?a=1"},
			{"https://h.test/p?devid=x&ssid=y&devpassword=z&sspassword=w", "https://h.test/p"},
		}
		for _, c := range cases {
			if got := scrapers.RedactURL(c.in); got != c.want {
				t.Errorf("RedactURL(%q) = %q, want %q", c.in, got, c.want)
			}
		}
	})

	t.Run("VID-020/valores com %-escape e parâmetros repetidos", func(t *testing.T) {
		t.Parallel()
		cases := []struct{ in, want string }{
			{"https://h.test/p?a=%20b&ssid=x%26y&c=d%2Fe", "https://h.test/p?a=%20b&c=d%2Fe"},
			{"https://h.test/p?devid=1&k=v&DevId=2&k=w&devid=3", "https://h.test/p?k=v&k=w"},
			{"https://h.test/p?sspassword=p%40ss%3Dw&romnom=Super%20Mario%20Bros.zip", "https://h.test/p?romnom=Super%20Mario%20Bros.zip"},
		}
		for _, c := range cases {
			got := scrapers.RedactURL(c.in)
			if got != c.want {
				t.Errorf("RedactURL(%q) = %q, want %q", c.in, got, c.want)
			}
			for _, secret := range []string{"x%26y", "p%40ss"} {
				if strings.Contains(got, secret) {
					t.Errorf("RedactURL(%q) = %q still holds a credential", c.in, got)
				}
			}
		}
	})

	t.Run("VID-020/URL sem query não muda; URL inválida vira \"<url inválida>\"", func(t *testing.T) {
		t.Parallel()
		for _, in := range []string{"https://h.test/p", "https://h.test/p#frag", "screenscraper:lookup", ""} {
			if in == "" {
				continue // an empty string is neither a URL to keep nor one to flag
			}
			if got := scrapers.RedactURL(in); got != in {
				t.Errorf("RedactURL(%q) = %q, want it unchanged", in, got)
			}
		}
		for _, in := range []string{"http://[::1", "://bad", "http://h.test/p?devid=secret\x7f%zz&a=%"} {
			got := scrapers.RedactURL(in)
			if got != "<url inválida>" {
				t.Errorf("RedactURL(%q) = %q, want %q", in, got, "<url inválida>")
			}
			if strings.Contains(got, "secret") {
				t.Errorf("RedactURL(%q) leaked the input", in)
			}
		}
	})
}

func TestRedactError(t *testing.T) {
	t.Parallel()

	const raw = "https://api.example.test/x.php?devid=DEVSECRET&devpassword=DEVPASS&ssid=USER&sspassword=PWD&systemeid=57"

	t.Run("VID-020/RedactError reescreve *url.Error (inclusive embrulhado com %w) sem credenciais e mantém errors.Is(context.Canceled)", func(t *testing.T) {
		t.Parallel()
		inner := &url.Error{Op: "Get", URL: raw, Err: context.Canceled}
		for name, in := range map[string]error{
			"direto":     inner,
			"embrulhado": fmt.Errorf("screenscraper: pedido: %w", inner),
			"duas vezes": fmt.Errorf("fila: %w", fmt.Errorf("screenscraper: %w", inner)),
		} {
			got := scrapers.RedactError(in)
			if got == nil {
				t.Fatalf("%s: RedactError = nil", name)
			}
			msg := got.Error()
			for _, secret := range []string{"DEVSECRET", "DEVPASS", "USER", "PWD"} {
				if strings.Contains(msg, secret) {
					t.Errorf("%s: message %q still holds %q", name, msg, secret)
				}
			}
			if !strings.Contains(msg, "systemeid=57") {
				t.Errorf("%s: message %q lost the non-secret parameter", name, msg)
			}
			if !errors.Is(got, context.Canceled) {
				t.Errorf("%s: errors.Is(context.Canceled) = false after redaction", name)
			}
			var ue *url.Error
			if !errors.As(got, &ue) {
				t.Fatalf("%s: no *url.Error in the chain", name)
			}
			if ue.URL != scrapers.RedactURL(raw) || ue.Op != "Get" {
				t.Errorf("%s: url.Error = {Op %q, URL %q}, want {Get, %q}", name, ue.Op, ue.URL, scrapers.RedactURL(raw))
			}
		}
		if inner.URL != raw {
			t.Error("RedactError mutated the original *url.Error")
		}
	})

	t.Run("VID-020/RedactError mantém o prefixo do embrulho e deixa outros erros e nil como estão", func(t *testing.T) {
		t.Parallel()
		inner := &url.Error{Op: "Get", URL: raw, Err: errors.New("EOF")}
		got := scrapers.RedactError(fmt.Errorf("screenscraper: pedido: %w", inner))
		if !strings.HasPrefix(got.Error(), "screenscraper: pedido: ") {
			t.Errorf("message %q lost the wrapper prefix", got.Error())
		}
		plain := errors.New("sem url")
		if got := scrapers.RedactError(plain); got != plain {
			t.Errorf("RedactError(plain) = %v, want the same error", got)
		}
		if got := scrapers.RedactError(nil); got != nil {
			t.Errorf("RedactError(nil) = %v, want nil", got)
		}
	})
}
