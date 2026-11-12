package agent

// Tests and helpers of T18 (docs/specs/phase-5-library/tasks.md): game.add with
// the v0.6.0 Game shape [IPC-020] and the helpers of the game.list tests in
// games_test.go (TestGameList): raw responses for the 1 MiB budget [IPC-051], a
// ListGames stub that pages over a fixed slice, and the worst-case games.

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// rawListResponse dispatches game.list with the raw params ("" = no params
// member) and returns the encoded response without the trailing '\n'
// [IPC-051].
func rawListResponse(t *testing.T, h *gamesHarness, params string) []byte {
	t.Helper()
	msg := `{"jsonrpc":"2.0","id":1,"method":"game.list"`
	if params != "" {
		msg += `,"params":` + params
	}
	msg += "}"
	raw, ok := ipc.Dispatch(t.Context(), h.handlers, []byte(msg))
	if !ok {
		t.Fatalf("Dispatch(%s) returned no response", msg)
	}
	return bytes.TrimSuffix(raw, []byte("\n"))
}

// listResultOf decodes a successful game.list response.
func listResultOf(t *testing.T, raw []byte) ipc.GameListResult {
	t.Helper()
	var resp ipc.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var list ipc.GameListResult
	resultInto(t, resp, &list)
	return list
}

// pagedStore is a ListGames stub over all: it honours offset and limit and
// reports len(all) as the total [LIB-002].
func pagedStore(all []store.Game) func(context.Context, store.Query) (store.Page, error) {
	return func(_ context.Context, q store.Query) (store.Page, error) {
		lo := min(q.Offset, len(all))
		hi := min(lo+q.Limit, len(all))
		return store.Page{Games: append([]store.Game{}, all[lo:hi]...), Total: int64(len(all))}, nil
	}
}

// worstGames returns n games with the given texts and every field game.list
// sends filled in, so each item is as large as the content allows.
func worstGames(n int, title, path, platform string) []store.Game {
	first := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	last := time.Date(2026, 11, 8, 22, 30, 0, 0, time.UTC)
	out := make([]store.Game, n)
	for i := range out {
		out[i] = store.Game{
			ID:               int64(i + 1),
			Title:            title,
			PlatformID:       1,
			Platform:         platform,
			LaunchType:       "exe",
			LaunchTarget:     path,
			Status:           domain.StatusPlaying,
			Favorite:         true,
			ReleaseDate:      "2009-05-01",
			TotalPlaySeconds: 1 << 40,
			PlayCount:        1 << 20,
			FirstPlayedAt:    first,
			LastPlayedAt:     last,
		}
	}
	return out
}

// TestGameAddShape checks the Game that game.add returns [IPC-020].
func TestGameAddShape(t *testing.T) {
	t.Parallel()

	t.Run("IPC-020/game.add devolve Game com platformId, platform, status, favorite e year (null quando não há data)", func(t *testing.T) {
		t.Parallel()
		h := newGamesHarness(t, nil)
		resp := h.call(t, ipc.MethodGameAdd, mustJSON(t, map[string]string{"path": h.file(t, "game.exe")}))
		if resp.Error != nil {
			t.Fatalf("error %d %q", resp.Error.Code, resp.Error.Message)
		}
		obj := objectOf(t, resp.Result)
		wantKeys(t, obj, gameKeys...)
		wantRaw(t, obj, "platform", `"PC"`)
		wantRaw(t, obj, "status", `"not_played"`)
		wantRaw(t, obj, "favorite", "false")
		wantRaw(t, obj, "year", "null")

		var g ipc.Game
		resultInto(t, resp, &g)
		if g.PlatformID < 1 {
			t.Errorf("platformId = %d, want the id of the PC platform (>= 1)", g.PlatformID)
		}
		stored, err := h.st.Game(t.Context(), g.ID)
		if err != nil {
			t.Fatalf("store.Game(%d): %v", g.ID, err)
		}
		if stored.PlatformID != g.PlatformID {
			t.Errorf("stored platformId = %d, want %d", stored.PlatformID, g.PlatformID)
		}
	})
}
