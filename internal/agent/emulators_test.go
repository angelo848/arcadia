package agent

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
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
	win, err := st.EnsureWindowsPlatformID(t.Context())
	if err != nil {
		t.Fatalf("EnsureWindowsPlatformID: %v", err)
	}
	// platform.setEmulator moved to platformHandlers in phase 9 (T11, PLT-022).
	h := emulatorHandlers(st, quietLog())
	mediaRoot := filepath.Join(t.TempDir(), "media")
	maps.Copy(h, platformHandlers(newPlatformService(st, media.New(mediaRoot, nil), mediaRoot, os.Stat, quietLog())))
	return h, st, win
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
		out, err := h[ipc.MethodPlatformSetEmulator](t.Context(), rawJSON(t, map[string]any{
			"platformId": pc, "emulatorId": added.ID,
		}))
		if err != nil {
			t.Fatalf("setEmulator set: %v", err)
		}
		// PLT-022: the result is the complete Platform (v0.11.0), not only the ids.
		res, ok := out.(ipc.PlatformResult)
		if !ok {
			t.Fatalf("setEmulator result is %T, want ipc.PlatformResult", out)
		}
		if res.Platform.ID != pc || res.Platform.Name != "Windows" || res.Platform.EmulatorID == nil ||
			*res.Platform.EmulatorID != added.ID || res.Platform.SeedKey == nil || *res.Platform.SeedKey != "windows" {
			t.Errorf("setEmulator platform = %+v, want Windows (seedKey windows) with emulatorId %d", res.Platform, added.ID)
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
