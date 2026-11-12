//go:build windows

package winapi_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// TestSteamRunning reads only: the app id below is not installed on any machine,
// so the key is absent (STM-001).
func TestSteamRunning(t *testing.T) {
	t.Parallel()

	t.Run("STM-001/chave de app ausente é ignorada sem erro", func(t *testing.T) {
		t.Parallel()
		running, present, err := winapi.SteamRunning("9999999901")
		if err != nil {
			t.Fatalf("SteamRunning(missing app): error %v, want nil", err)
		}
		if running || present {
			t.Errorf("SteamRunning(missing app) = running %v, present %v; want false, false", running, present)
		}
	})
}
