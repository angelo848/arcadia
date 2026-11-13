package scrapers

import (
	"errors"
	"strings"
	"testing"
)

// fakeStore is an in-memory KeyStore; the Credential Manager is never touched in tests.
type fakeStore struct {
	values map[string]string
	fail   error
}

func (f *fakeStore) Set(name, value string) error {
	if f.fail != nil {
		return f.fail
	}
	f.values[name] = value
	return nil
}

func (f *fakeStore) Get(name string) (string, error) {
	v, ok := f.values[name]
	if !ok {
		return "", ErrKeyNotFound
	}
	return v, nil
}

func (f *fakeStore) Delete(name string) error {
	delete(f.values, name)
	return nil
}

func TestKeys(t *testing.T) {
	t.Parallel()

	t.Run("MID-001/chave gravada é lida pelo mesmo nome do provedor", func(t *testing.T) {
		t.Parallel()
		st := &fakeStore{values: map[string]string{}}
		k := NewKeys(st)
		if err := k.Set("steamgriddb", "minha-chave"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		v, err := k.Value("steamgriddb")
		if err != nil || v != "minha-chave" {
			t.Errorf("Value = %q, %v; want minha-chave", v, err)
		}
	})

	t.Run("MID-001/status mostra só cadastrada ou não cadastrada", func(t *testing.T) {
		t.Parallel()
		k := NewKeys(&fakeStore{values: map[string]string{}})
		if k.Configured("steamgriddb") {
			t.Error("Configured before Set = true")
		}
		if err := k.Set("steamgriddb", "segredo"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if !k.Configured("steamgriddb") {
			t.Error("Configured after Set = false")
		}
	})

	t.Run("MID-001/apagar a chave volta a não cadastrada", func(t *testing.T) {
		t.Parallel()
		k := NewKeys(&fakeStore{values: map[string]string{}})
		_ = k.Set("steamgriddb", "segredo")
		if err := k.Delete("steamgriddb"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if k.Configured("steamgriddb") {
			t.Error("Configured after Delete = true")
		}
	})

	t.Run("MID-001/IGDB guarda id e segredo em uma só entrada separada por quebra de linha", func(t *testing.T) {
		t.Parallel()
		st := &fakeStore{values: map[string]string{}}
		k := NewKeys(st)
		if err := k.SetIGDB("cid", "sec"); err != nil {
			t.Fatalf("SetIGDB: %v", err)
		}
		id, secret, err := k.IGDB()
		if err != nil || id != "cid" || secret != "sec" {
			t.Errorf("IGDB() = %q, %q, %v", id, secret, err)
		}
	})

	t.Run("MID-001/sem valor ou com erro do armazenamento, o erro não repete a chave", func(t *testing.T) {
		t.Parallel()
		const secret = "chave-super-secreta"
		st := &fakeStore{values: map[string]string{}, fail: errors.New("boom")}
		k := NewKeys(st)
		err := k.Set("steamgriddb", secret)
		if err == nil {
			t.Fatal("Set err = nil, want the store error")
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q repeats the key", err)
		}
	})

	t.Run("MID-001/valor vazio é recusado sem gravar", func(t *testing.T) {
		t.Parallel()
		st := &fakeStore{values: map[string]string{}}
		if err := NewKeys(st).Set("steamgriddb", "   "); !errors.Is(err, ErrNoKey) {
			t.Errorf("Set empty err = %v, want ErrNoKey", err)
		}
		if len(st.values) != 0 {
			t.Error("empty key was stored")
		}
	})
}
