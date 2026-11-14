package ipc_test

import (
	"encoding/json"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// libPtr returns a pointer to v, for the optional members of the library params.
func libPtr[T any](v T) *T { return &v }

func TestLibraryTypes(t *testing.T) {
	t.Parallel()

	t.Run("IPC-042/GameListResult serializa games, total e offset; games nil vira [] (checar no teste de agente, T18)", func(t *testing.T) {
		t.Parallel()
		// Games is never nil on the producer side; the nil -> [] guarantee
		// is checked through the agent handler in T18. Here an empty non-nil slice
		// must serialize as [] and the three members must be exactly games, total, offset.
		res := ipc.GameListResult{Games: []ipc.Game{}, Total: 0, Offset: 0}
		if got, want := compactJSON(t, mustMarshal(t, res)), `{"games":[],"total":0,"offset":0}`; got != want {
			t.Errorf("GameListResult (empty) JSON = %s, want %s", got, want)
		}

		res = ipc.GameListResult{
			Games:  []ipc.Game{{ID: 1, Title: "A", PlatformID: 1, Platform: "PC", Path: `C:\a.exe`, Status: "not_played"}},
			Total:  450,
			Offset: 200,
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(mustMarshal(t, res), &fields); err != nil {
			t.Fatalf("GameListResult JSON: %v", err)
		}
		if got := sortedKeys(fields); len(got) != 3 || got[0] != "games" || got[1] != "offset" || got[2] != "total" {
			t.Errorf("GameListResult JSON members = %v, want exactly games, offset, total", got)
		}
		if got := compactJSON(t, fields["total"]); got != "450" {
			t.Errorf("total = %s, want 450 (games passing the filter, not the page)", got)
		}
		if got := compactJSON(t, fields["offset"]); got != "200" {
			t.Errorf("offset = %s, want 200", got)
		}
	})

	t.Run("IPC-042/Game serializa platformId, platform, status, favorite e year (null quando nil)", func(t *testing.T) {
		t.Parallel()
		g := ipc.Game{ID: 7, Title: "Hollow Knight", PlatformID: 2, Platform: "PC", Path: `C:\hk.exe`, Status: "playing", Favorite: true, Year: nil}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(mustMarshal(t, g), &m); err != nil {
			t.Fatalf("Game JSON: %v", err)
		}
		for name, want := range map[string]string{
			"platformId": "2",
			"platform":   `"PC"`,
			"status":     `"playing"`,
			"favorite":   "true",
			"year":       "null",
		} {
			if got := compactJSON(t, m[name]); got != want {
				t.Errorf("Game.%s = %s, want %s", name, got, want)
			}
		}

		year := 1998
		g.Year = &year
		if err := json.Unmarshal(mustMarshal(t, g), &m); err != nil {
			t.Fatalf("Game JSON with year: %v", err)
		}
		if got := compactJSON(t, m["year"]); got != "1998" {
			t.Errorf("Game.year with 1998 = %s, want 1998", got)
		}
	})
}

func TestCatalogItems(t *testing.T) {
	t.Parallel()

	t.Run("IPC-049/library.catalog: genres e companies são {id, name} com o id do banco e os nomes na ordem recebida", func(t *testing.T) {
		t.Parallel()
		// The wire result as the agent sends it: id is the database id the FilterSpec
		// filters by (FLT-002, FLT-003), and the lists are never null.
		// Platform members in declaration order: id, name, category, emulatorId, seedKey, iconPath, gameCount [PLT-014].
		const wire = `{"platforms":[{"id":1,"name":"PC","category":"pc","emulatorId":null,"seedKey":"windows","iconPath":null,"gameCount":3}],` +
			`"genres":[{"id":5,"name":"Action"},{"id":6,"name":"RPG"}],` +
			`"companies":[{"id":9,"name":"Team Cherry"}],"playModes":["single"]}`
		var cat ipc.Catalog
		if err := json.Unmarshal([]byte(wire), &cat); err != nil {
			t.Fatalf("decode library.catalog result: %v", err)
		}
		if got := compactJSON(t, mustMarshal(t, cat)); got != wire {
			t.Errorf("Catalog JSON = %s, want %s", got, wire)
		}
	})
}

// mustMarshal marshals v and fails the test on error.
func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	return raw
}
