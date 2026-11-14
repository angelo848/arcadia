package agent

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// emulatorHandlersFor opens a migrated store and returns its emulator handlers and the
// id of the PC platform.
func emulatorHandlersFor(t *testing.T) (map[string]ipc.Handler, *store.Store, int64) {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	pc, err := st.AddPlatform(t.Context(), "PC", domain.PlatformCategory("pc"))
	if err != nil {
		t.Fatalf("AddPlatform: %v", err)
	}
	return emulatorHandlers(st, quietLog()), st, pc.ID
}

func rawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestEmulatorHandlers(t *testing.T) {
	t.Parallel()

	t.Run("EMU-001/emulator.add recusa nome vazio com -32602", func(t *testing.T) {
		t.Parallel()
		h, _, _ := emulatorHandlersFor(t)
		_, err := h[ipc.MethodEmulatorAdd](t.Context(), rawJSON(t, map[string]string{
			"name": "   ", "exePath": filepath.Join(t.TempDir(), "e.exe"), "argsTemplate": "{rom}",
		}))
		var rpcErr *ipc.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != ipc.CodeInvalidParams {
			t.Errorf("err = %v, want -32602", err)
		}
	})

	t.Run("EMU-001/emulator.update com id inexistente é -32001", func(t *testing.T) {
		t.Parallel()
		h, _, _ := emulatorHandlersFor(t)
		_, err := h[ipc.MethodEmulatorUpdate](t.Context(), rawJSON(t, map[string]any{
			"id": 999, "name": "X", "exePath": filepath.Join(t.TempDir(), "e.exe"), "argsTemplate": "",
		}))
		var rpcErr *ipc.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != codeEmulatorNotFound {
			t.Errorf("err = %v, want -32001", err)
		}
	})

	t.Run("EMU-002/platform.setEmulator grava o padrão e limpa com null", func(t *testing.T) {
		t.Parallel()
		h, st, pc := emulatorHandlersFor(t)
		exe := filepath.Join(t.TempDir(), "e.exe")
		added, err := st.AddEmulator(t.Context(), "Emu", exe, "{rom}")
		if err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		if _, err := h[ipc.MethodPlatformSetEmulator](t.Context(), rawJSON(t, map[string]any{
			"platformId": pc, "emulatorId": added.ID,
		})); err != nil {
			t.Fatalf("setEmulator set: %v", err)
		}
		defaultOf := func() *int64 {
			plats, err := st.Platforms(t.Context())
			if err != nil {
				t.Fatalf("Platforms: %v", err)
			}
			for _, p := range plats {
				if p.ID == pc {
					return p.EmulatorID
				}
			}
			t.Fatalf("platform %d not found in %+v", pc, plats)
			return nil
		}
		if got := defaultOf(); got == nil || *got != added.ID {
			t.Fatalf("platform default = %v; want emulator %d", got, added.ID)
		}
		if _, err := h[ipc.MethodPlatformSetEmulator](t.Context(), rawJSON(t, map[string]any{
			"platformId": pc, "emulatorId": nil,
		})); err != nil {
			t.Fatalf("setEmulator clear: %v", err)
		}
		if got := defaultOf(); got != nil {
			t.Errorf("platform default after null = %d, want none", *got)
		}
	})
}
