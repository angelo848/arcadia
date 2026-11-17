package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

type fakeLocationStore struct {
	game  store.Game
	rules []store.TrackingRule
}

func (f fakeLocationStore) Game(context.Context, int64) (store.Game, error) { return f.game, nil }
func (f fakeLocationStore) TrackingRules(context.Context, int64) ([]store.TrackingRule, error) {
	return f.rules, nil
}

func TestGameLocation(t *testing.T) {
	dir := t.TempDir()
	rom := filepath.Join(dir, "game.exe")
	if err := os.WriteFile(rom, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		st   fakeLocationStore
		want ipc.GameLocationResult
	}{
		{"exe: the file", fakeLocationStore{game: store.Game{ID: 1, LaunchType: "exe", LaunchTarget: rom}},
			ipc.GameLocationResult{Path: rom, Kind: ipc.LocationFile}},
		{"emulator: the ROM", fakeLocationStore{game: store.Game{ID: 1, LaunchType: "emulator", LaunchTarget: rom}},
			ipc.GameLocationResult{Path: rom, Kind: ipc.LocationFile}},
		{"exe deleted from the disk: no location", fakeLocationStore{game: store.Game{ID: 1, LaunchType: "exe", LaunchTarget: filepath.Join(dir, "gone.exe")}},
			ipc.GameLocationResult{}},
		{"uri with install_dir rule: the folder", fakeLocationStore{
			game:  store.Game{ID: 1, LaunchType: "uri", LaunchTarget: "steam://rungameid/1"},
			rules: []store.TrackingRule{{Kind: "exe_name", Value: "a.exe"}, {Kind: "install_dir", Value: dir}}},
			ipc.GameLocationResult{Path: dir, Kind: ipc.LocationDir}},
		{"uri without a folder: no location", fakeLocationStore{game: store.Game{ID: 1, LaunchType: "uri", LaunchTarget: "gog://1"}},
			ipc.GameLocationResult{}},
	}
	for _, tc := range cases {
		t.Run("1.0.3/"+tc.name, func(t *testing.T) {
			h := locationHandlers(tc.st, os.Stat, slog.New(slog.NewTextHandler(io.Discard, nil)))[ipc.MethodGameLocation]
			got, err := h(context.Background(), json.RawMessage(`{"gameId":1}`))
			if err != nil {
				t.Fatal(err)
			}
			if got != any(tc.want) {
				t.Errorf("result = %+v, want %+v", got, tc.want)
			}
		})
	}
}
