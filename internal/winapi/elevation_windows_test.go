//go:build windows

package winapi_test

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// TestIsProcessElevated checks IsProcessElevated against the token of the test
// process itself (ADM-006). The expected value is read through another call, so
// the test does not depend on whether the suite runs elevated.
func TestIsProcessElevated(t *testing.T) {
	t.Run("ADM-006/processo atual reporta o mesmo estado do próprio token", func(t *testing.T) {
		t.Parallel()
		want := windows.GetCurrentProcessToken().IsElevated()
		got, err := winapi.IsProcessElevated(uint32(os.Getpid()))
		if err != nil {
			t.Fatalf("IsProcessElevated(self): %v", err)
		}
		if got != want {
			t.Errorf("IsProcessElevated(self) = %v, want %v", got, want)
		}
	})

	t.Run("ADM-006/PID inexistente retorna erro", func(t *testing.T) {
		t.Parallel()
		// Process IDs are multiples of 4; this one does not exist.
		if got, err := winapi.IsProcessElevated(0xFFFFFFF0); err == nil {
			t.Errorf("IsProcessElevated(missing) = %v, nil; want an error", got)
		}
	})
}
