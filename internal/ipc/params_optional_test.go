package ipc_test

import (
	"encoding/json"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

func TestOptional(t *testing.T) {
	t.Parallel()

	t.Run("IPC-052/completedAt ausente vira Present = false; completedAt null vira Null = true", func(t *testing.T) {
		t.Parallel()
		var absent ipc.GameUpdateParams
		if e := ipc.DecodeParams(json.RawMessage(`{"gameId":1}`), &absent); e != nil {
			t.Fatalf("DecodeParams(absent) = %v, want nil", e)
		}
		if absent.CompletedAt.Present || absent.CompletedAt.Null {
			t.Errorf("absent completedAt = %+v, want Present=false Null=false", absent.CompletedAt)
		}

		var nullable ipc.GameUpdateParams
		if e := ipc.DecodeParams(json.RawMessage(`{"gameId":1,"completedAt":null}`), &nullable); e != nil {
			t.Fatalf("DecodeParams(null) = %v, want nil", e)
		}
		if !nullable.CompletedAt.Present || !nullable.CompletedAt.Null {
			t.Errorf("null completedAt = %+v, want Present=true Null=true", nullable.CompletedAt)
		}

		// The type itself follows the same rule when decoded directly.
		var o ipc.Optional[string]
		if err := json.Unmarshal([]byte(`null`), &o); err != nil {
			t.Fatalf("Unmarshal null: %v", err)
		}
		if !o.Present || !o.Null {
			t.Errorf("Optional from null = %+v, want Present=true Null=true", o)
		}
	})

	t.Run("IPC-052/completedAt com string vira Value", func(t *testing.T) {
		t.Parallel()
		var p ipc.GameUpdateParams
		if e := ipc.DecodeParams(json.RawMessage(`{"gameId":1,"completedAt":"2026-11-12"}`), &p); e != nil {
			t.Fatalf("DecodeParams = %v, want nil", e)
		}
		if !p.CompletedAt.Present || p.CompletedAt.Null || p.CompletedAt.Value != "2026-11-12" {
			t.Errorf("completedAt = %+v, want Present=true Null=false Value=2026-11-12", p.CompletedAt)
		}
	})
}

func TestDecodeParamsLibrary(t *testing.T) {
	t.Parallel()

	t.Run("IPC-043/membro desconhecido dentro de filter é -32602", func(t *testing.T) {
		t.Parallel()
		var p ipc.GameListParams
		assertInvalidParams(t, "filter.extra", ipc.DecodeParams(json.RawMessage(`{"filter":{"version":1,"extra":true}}`), &p))
	})

	t.Run("IPC-043/array posicional em game.list é -32602", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{`[]`, `[{"version":1}]`, `["title",0,200]`} {
			var p ipc.GameListParams
			assertInvalidParams(t, raw, ipc.DecodeParams(json.RawMessage(raw), &p))
		}
	})

	t.Run("IPC-043/offset texto, limit 1.5 e version 2 são -32602", func(t *testing.T) {
		t.Parallel()
		// "version 2" is a top-level member here (not a member of filter): game.list has no
		// such member, so it is an unknown member. The filter version rule is checked
		// by the agent through domain.FilterSpec.Validate in T18.
		for _, raw := range []string{`{"offset":"10"}`, `{"offset":-1.5}`, `{"limit":1.5}`, `{"limit":"200"}`, `{"version":2}`} {
			var p ipc.GameListParams
			assertInvalidParams(t, raw, ipc.DecodeParams(json.RawMessage(raw), &p))
		}
	})

	t.Run("IPC-052/game.update com array posicional é -32602", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{`[1]`, `[1,"novo título"]`} {
			var p ipc.GameUpdateParams
			assertInvalidParams(t, raw, ipc.DecodeParams(json.RawMessage(raw), &p))
		}
	})

	t.Run("IPC-052/membro desconhecido no patch é -32602", func(t *testing.T) {
		t.Parallel()
		var p ipc.GameUpdateParams
		assertInvalidParams(t, "gameId+nope", ipc.DecodeParams(json.RawMessage(`{"gameId":1,"nope":1}`), &p))
		var q ipc.GameUpdateParams
		assertInvalidParams(t, "gameId+Title", ipc.DecodeParams(json.RawMessage(`{"gameId":1,"Title":"x"}`), &q))
	})

	t.Run("IPC-052/favorite texto é -32602", func(t *testing.T) {
		t.Parallel()
		var p ipc.GameUpdateParams
		assertInvalidParams(t, "favorite texto", ipc.DecodeParams(json.RawMessage(`{"gameId":1,"favorite":"yes"}`), &p))
	})

	t.Run("IPC-052/genres com item numérico, [1], é -32602", func(t *testing.T) {
		t.Parallel()
		var p ipc.GameUpdateParams
		assertInvalidParams(t, "genres [1]", ipc.DecodeParams(json.RawMessage(`{"gameId":1,"genres":[1]}`), &p))
	})
}
