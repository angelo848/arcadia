package ipc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// TestMediaChangedContract checks the media.changed notification against the
// contract v0.12.0 (docs/specs/phase-10-2-imagens-ao-vivo, T02).
func TestMediaChangedContract(t *testing.T) {
	t.Parallel()
	c := loadContract(t)

	t.Run("IMG-004/media.changed é notificação com o nome literal \"media.changed\"", func(t *testing.T) {
		t.Parallel()
		if ipc.MethodMediaChanged != "media.changed" {
			t.Fatalf("ipc.MethodMediaChanged = %q, want \"media.changed\"", ipc.MethodMediaChanged)
		}
		found := false
		for _, m := range c.Methods {
			if m.Name == "media.changed" {
				found = true
				if !m.Notification {
					t.Error("contract media.changed: x-notification = false, want true")
				}
			}
		}
		if !found {
			t.Fatal("contract has no media.changed")
		}
	})

	t.Run("IMG-004/params: só gameId, obrigatório, integer com minimum 1; resultado null", func(t *testing.T) {
		t.Parallel()
		all, required := c.paramNames(t, ipc.MethodMediaChanged)
		if !slices.Equal(all, []string{"gameId"}) || !slices.Equal(required, []string{"gameId"}) {
			t.Fatalf("contract media.changed params = %v (required %v), want [gameId] required", all, required)
		}
		for _, m := range c.Methods {
			if m.Name != ipc.MethodMediaChanged {
				continue
			}
			p := m.Params[0]
			if p.Schema.Type != "integer" {
				t.Errorf("gameId type = %q, want integer", p.Schema.Type)
			}
			if p.Schema.Minimum == nil || *p.Schema.Minimum != 1 {
				t.Errorf("gameId minimum = %v, want 1", p.Schema.Minimum)
			}
			if got := m.Result.Schema.Type; got != "null" {
				t.Errorf("media.changed result type = %q, want null", got)
			}
		}
	})

	t.Run("IMG-004/MediaChangedParams codifica {\"gameId\":42} e tem só o campo gameId", func(t *testing.T) {
		t.Parallel()
		raw, err := json.Marshal(ipc.MediaChangedParams{GameID: 42})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(raw) != `{"gameId":42}` {
			t.Errorf("MediaChangedParams JSON = %s, want {\"gameId\":42}", raw)
		}
		if got := jsonFieldNames(t, ipc.MediaChangedParams{}); !slices.Equal(got, []string{"gameId"}) {
			t.Errorf("MediaChangedParams JSON fields = %v, want [gameId]", got)
		}
	})

	t.Run("IMG-004/x-media.mediaChangedWindowMs = 500", func(t *testing.T) {
		t.Parallel()
		if c.Info.Media == nil || c.Info.Media.MediaChangedWindowMs == nil {
			t.Fatal("contract info has no x-media.mediaChangedWindowMs")
		}
		if got := *c.Info.Media.MediaChangedWindowMs; got != 500 {
			t.Errorf("x-media.mediaChangedWindowMs = %d, want 500", got)
		}
	})

	t.Run("IMG-004/media.changed vem logo depois de session.changed", func(t *testing.T) {
		t.Parallel()
		var names []string
		for _, m := range c.Methods {
			names = append(names, m.Name)
		}
		i := slices.Index(names, ipc.MethodSessionChanged)
		if i < 0 {
			t.Fatal("contract has no session.changed")
		}
		if i+1 >= len(names) || names[i+1] != ipc.MethodMediaChanged {
			t.Errorf("method after session.changed = %v, want media.changed", names[i+1:])
		}
	})

	t.Run("IMG-004/nenhum handler do agente para media.changed", func(t *testing.T) {
		t.Parallel()
		if _, has := testHandlers()[ipc.MethodMediaChanged]; has {
			t.Error("AgentHandlers has a handler for the server notification media.changed")
		}
	})

	t.Run("IMG-013/v0.12.0 é a v0.11.0 mais media.changed e x-media, nada mais", func(t *testing.T) {
		t.Parallel()
		load := func(path string) map[string]any {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			return m
		}
		prev := load(filepath.Join("..", "..", "docs", "specs", "phase-9-plataformas", "contracts", "ipc.openrpc.json"))
		next := load(contractPath)

		info, ok := next["info"].(map[string]any)
		if !ok {
			t.Fatal("v0.12.0 has no info object")
		}
		if _, ok := info["x-media"]; !ok {
			t.Error("v0.12.0 info has no x-media")
		}
		delete(info, "x-media")
		delete(info, "version")
		delete(info, "description")
		pinfo, ok := prev["info"].(map[string]any)
		if !ok {
			t.Fatal("v0.11.0 has no info object")
		}
		delete(pinfo, "version")
		delete(pinfo, "description")

		methods, ok := next["methods"].([]any)
		if !ok {
			t.Fatal("v0.12.0 has no methods array")
		}
		kept := make([]any, 0, len(methods))
		removed := 0
		for _, m := range methods {
			if mm, ok := m.(map[string]any); ok && mm["name"] == "media.changed" {
				removed++
				continue
			}
			kept = append(kept, m)
		}
		if removed != 1 {
			t.Fatalf("v0.12.0 has %d media.changed methods, want 1", removed)
		}
		next["methods"] = kept

		if !reflect.DeepEqual(prev, next) {
			t.Error("v0.12.0 differs from v0.11.0 beyond media.changed, x-media, info.version and info.description")
		}
	})
}
