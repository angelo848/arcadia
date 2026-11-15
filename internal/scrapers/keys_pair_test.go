package scrapers_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// Casos da T22 (phase-13-modo-console): pares id/segredo do ScreenScraper no KeyStore.
// Assinaturas assumidas: (*Keys).SetPair(provider, id, secret) error e (*Keys).Pair(provider) (id, secret, error).

type ssKeyStore struct{ values map[string]string }

func (f *ssKeyStore) Set(name, value string) error { f.values[name] = value; return nil }

func (f *ssKeyStore) Get(name string) (string, error) {
	v, ok := f.values[name]
	if !ok {
		return "", scrapers.ErrKeyNotFound
	}
	return v, nil
}

func (f *ssKeyStore) Delete(name string) error { delete(f.values, name); return nil }

func TestKeysPair(t *testing.T) {
	t.Parallel()

	t.Run("SSC-001/SetPair grava id\\nsenha em arcadia/<provedor> e Pair devolve o id e a senha", func(t *testing.T) {
		t.Parallel()
		for _, prov := range []string{"screenscraper", "screenscraperdev"} {
			st := &ssKeyStore{values: map[string]string{}}
			k := scrapers.NewKeys(st)
			if err := k.SetPair(prov, "meu-id", "minha-senha"); err != nil {
				t.Fatalf("%s SetPair: %v", prov, err)
			}
			if got := st.values["arcadia/"+prov]; got != "meu-id\nminha-senha" {
				t.Errorf("%s valor gravado = %q, want meu-id\\nminha-senha", prov, got)
			}
			id, secret, err := k.Pair(prov)
			if err != nil || id != "meu-id" || secret != "minha-senha" {
				t.Errorf("%s Pair = %q, %q, %v", prov, id, secret, err)
			}
			if !k.Configured(prov) {
				t.Errorf("%s Configured = false", prov)
			}
		}
	})

	t.Run("SSC-001/SetPair com id ou senha vazios e ErrNoKey e nada e gravado", func(t *testing.T) {
		t.Parallel()
		for _, tc := range [][2]string{{"", "s"}, {"i", ""}, {"  ", "s"}, {"i", "  "}, {"", ""}} {
			st := &ssKeyStore{values: map[string]string{}}
			if err := scrapers.NewKeys(st).SetPair("screenscraper", tc[0], tc[1]); !errors.Is(err, scrapers.ErrNoKey) {
				t.Errorf("SetPair(%q,%q) err = %v, want ErrNoKey", tc[0], tc[1], err)
			}
			if len(st.values) != 0 {
				t.Errorf("SetPair(%q,%q) gravou %v", tc[0], tc[1], st.values)
			}
		}
	})

	t.Run("SSC-001/Pair devolve o id e a senha; entrada sem \\n e ErrKeyNotFound", func(t *testing.T) {
		t.Parallel()
		st := &ssKeyStore{values: map[string]string{"arcadia/screenscraper": "so-uma-linha"}}
		k := scrapers.NewKeys(st)
		if _, _, err := k.Pair("screenscraper"); !errors.Is(err, scrapers.ErrKeyNotFound) {
			t.Errorf("sem \\n: err = %v, want ErrKeyNotFound", err)
		}
		if _, _, err := k.Pair("screenscraperdev"); !errors.Is(err, scrapers.ErrKeyNotFound) {
			t.Errorf("ausente: err = %v, want ErrKeyNotFound", err)
		}
	})

	t.Run("SSC-001/erro do armazenamento no SetPair nao repete o segredo", func(t *testing.T) {
		t.Parallel()
		const secret = "segredo-do-ss-9"
		st := &failingStore{err: errors.New("boom")}
		err := scrapers.NewKeys(st).SetPair("screenscraper", "id1", secret)
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Errorf("err = %v; want an error without the secret", err)
		}
	})
}

type failingStore struct{ err error }

func (f *failingStore) Set(string, string) error   { return f.err }
func (f *failingStore) Get(string) (string, error) { return "", scrapers.ErrKeyNotFound }
func (f *failingStore) Delete(string) error        { return nil }
