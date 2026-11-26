package store_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

func TestTotalCeilingMatchesIPC(t *testing.T) {
	t.Parallel()
	t.Run("SED-013/o teto do total do store (maxTotalPlaySeconds) é igual a ipc.MaxPlaySeconds", func(t *testing.T) {
		t.Parallel()
		if int64(store.MaxTotalPlaySecondsForTest) != int64(ipc.MaxPlaySeconds) {
			t.Fatalf("store ceiling = %d, ipc.MaxPlaySeconds = %d", int64(store.MaxTotalPlaySecondsForTest), int64(ipc.MaxPlaySeconds))
		}
		if ipc.MaxPlaySeconds != 3_599_996_400 {
			t.Fatalf("ipc.MaxPlaySeconds = %d, want 3599996400", ipc.MaxPlaySeconds)
		}
	})
}
