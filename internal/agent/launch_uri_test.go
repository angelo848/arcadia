package agent

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// uriGameStore returns one game of the given launch type (RUL-003).
func uriGameStore(t *testing.T, launchType, target string) *recordingGameStore {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	game := store.Game{ID: 7, Title: "CS2", LaunchType: launchType, LaunchTarget: target}
	return &recordingGameStore{inner: st, gameFn: func(int64) (store.Game, error) { return game, nil }}
}

func TestLaunchURIPrepare(t *testing.T) {
	t.Parallel()

	t.Run("RUL-003/launch_type uri não é mais recusado e vira spec de URI", func(t *testing.T) {
		t.Parallel()
		gs := uriGameStore(t, launchTypeURI, "steam://rungameid/730")
		params, _ := json.Marshal(gameLaunchParams{GameID: json.RawMessage("7")})
		_, spec, err := prepareLaunch(context.Background(), gs, params, quietLog())
		if err != nil {
			t.Fatalf("prepareLaunch(uri): %v", err)
		}
		if spec.Kind != tracker.KindURI || spec.Exe != "steam://rungameid/730" {
			t.Errorf("spec = %+v, want a URI spec with the URI in Exe", spec)
		}
	})

	t.Run("RUL-003/exe continua sendo spec de job", func(t *testing.T) {
		t.Parallel()
		gs := uriGameStore(t, launchTypeExe, `C:\g.exe`)
		params, _ := json.Marshal(gameLaunchParams{GameID: json.RawMessage("7")})
		_, spec, err := prepareLaunch(context.Background(), gs, params, quietLog())
		if err != nil {
			t.Fatalf("prepareLaunch(exe): %v", err)
		}
		if spec.Kind != tracker.KindExe {
			t.Errorf("Kind = %v, want KindExe", spec.Kind)
		}
	})

	t.Run("RUL-003/lançamento sem contar tempo recusa URI", func(t *testing.T) {
		t.Parallel()
		gs := uriGameStore(t, launchTypeURI, "steam://rungameid/730")
		params, _ := json.Marshal(gameLaunchParams{GameID: json.RawMessage("7")})
		_, err := handleGameLaunchUntracked(context.Background(), gs, nil, quietLog(), params)
		var rpcErr *ipc.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != ipc.CodeLaunchFailed {
			t.Errorf("untracked URI err = %v, want -32003 launch failed", err)
		}
	})
}
