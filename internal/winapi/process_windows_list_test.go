//go:build windows

package winapi_test

import (
	"os"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// TestListProcesses checks the snapshot against the test process itself (RUL-002).
func TestListProcesses(t *testing.T) {
	t.Parallel()

	t.Run("RUL-002/snapshot traz o próprio processo com o caminho da imagem", func(t *testing.T) {
		t.Parallel()
		procs, err := winapi.ListProcesses()
		if err != nil {
			t.Fatalf("ListProcesses: %v", err)
		}
		self := uint32(os.Getpid())
		for _, p := range procs {
			if p.PID == self {
				if p.Path == "" {
					t.Errorf("own process has an empty image path")
				}
				return
			}
		}
		t.Errorf("own process %d not in the snapshot of %d processes", self, len(procs))
	})
}
