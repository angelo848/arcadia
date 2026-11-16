package scrapers_test

import (
	"errors"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// T49 (SSC-006): SetPair recusa \n e \r em qualquer posicao, sem trim antes. Assinatura assumida: scrapers.ErrInvalidKey.

func TestSetPairRejectsLineBreaks(t *testing.T) {
	t.Parallel()
	for _, br := range []string{"\n", "\r"} {
		for pos, mk := range map[string]func(string) string{
			"comeco": func(s string) string { return br + s },
			"meio":   func(s string) string { return s[:1] + br + s[1:] },
			"fim":    func(s string) string { return s + br },
		} {
			for _, field := range []string{"id", "secret"} {
				name := "SSC-006/SetPair recusa " + map[string]string{"\n": `\n`, "\r": `\r`}[br] + " no " + pos + " do " + field
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					st := &ssKeyStore{values: map[string]string{}}
					k := scrapers.NewKeys(st)
					id, secret := "meu-id", "minha-senha"
					if field == "id" {
						id = mk(id)
					} else {
						secret = mk(secret)
					}
					if err := k.SetPair("screenscraper", id, secret); !errors.Is(err, scrapers.ErrInvalidKey) {
						t.Errorf("err = %v, want ErrInvalidKey", err)
					}
					if len(st.values) != 0 {
						t.Errorf("gravou %v", st.values)
					}
				})
			}
		}
	}

	t.Run("SSC-006/secret \"senha\\n\" (so quebra no fim) e recusado, nao aparado", func(t *testing.T) {
		t.Parallel()
		st := &ssKeyStore{values: map[string]string{}}
		if err := scrapers.NewKeys(st).SetPair("screenscraperdev", "id", "senha\n"); !errors.Is(err, scrapers.ErrInvalidKey) {
			t.Errorf("err = %v, want ErrInvalidKey", err)
		}
		if len(st.values) != 0 {
			t.Errorf("gravou %v", st.values)
		}
	})
}
