//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

// TestCredentialStore round-trips one entry in the real Credential Manager, under a name that
// is unique to this run and removed at the end (MID-001).
func TestCredentialStore(t *testing.T) {
	name := fmt.Sprintf("arcadia-test/%d/%s", os.Getpid(), t.Name())
	var st CredentialStore
	t.Cleanup(func() { _ = st.Delete(name) })

	t.Run("MID-001/Credential Manager guarda, lê e apaga uma chave", func(t *testing.T) {
		if err := st.Set(name, "valor-de-teste"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, err := st.Get(name)
		if err != nil || got != "valor-de-teste" {
			t.Fatalf("Get = %q, %v", got, err)
		}
		if err := st.Delete(name); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := st.Get(name); !errors.Is(err, ErrCredentialNotFound) {
			t.Errorf("Get after Delete err = %v, want ErrCredentialNotFound", err)
		}
	})

	t.Run("MID-001/apagar uma entrada inexistente não é erro", func(t *testing.T) {
		if err := st.Delete(name + "/nao-existe"); err != nil {
			t.Errorf("Delete missing = %v, want nil", err)
		}
	})
}
