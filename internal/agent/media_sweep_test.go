package agent

// Tests of the sweep of orphan media folders at startup [DEL-010]: the agent starts it in the background, and
// Shutdown waits for it before the store closes. The order is observed with onShutdownStep, as in order_test.go.
// The library is seeded through the before hook of newWiring, so the games exist when startAgent runs.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// seedSweepFixture opens the library in cfg.DataDir (before the agent does), adds four games, removes the
// second and the fourth, and creates <media>/<id>/a.jpg for each of the four and <media>/platforms/icon.png.
// It returns the ids of the games that stay (keep) and the ids whose folders are orphans.
func seedSweepFixture(t *testing.T, cfg config) (keep, orphan []int64) {
	t.Helper()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	st, err := store.Open(t.Context(), filepath.Join(cfg.DataDir, "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	at := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	var ids []int64
	for i := range 4 {
		g, err := st.AddGame(t.Context(), store.NewGame{
			Title: fmt.Sprintf("Jogo %d", i), Exe: fmt.Sprintf(`C:\sweep\%d.exe`, i), At: at,
		})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		ids = append(ids, g.ID)
	}
	for _, id := range []int64{ids[1], ids[3]} {
		if err := st.DeleteGame(t.Context(), id); err != nil {
			t.Fatalf("DeleteGame(%d): %v", id, err)
		}
	}
	mediaDir := filepath.Join(cfg.DataDir, "media")
	for _, id := range ids {
		dmWrite(t, mediaDir, id, "a.jpg")
	}
	platform := filepath.Join(mediaDir, "platforms", "icon.png")
	if err := os.MkdirAll(filepath.Dir(platform), 0o755); err != nil {
		t.Fatalf("mkdir platforms: %v", err)
	}
	if err := os.WriteFile(platform, []byte("icon"), 0o644); err != nil {
		t.Fatalf("write icon: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}
	return []int64{ids[0], ids[2]}, []int64{ids[1], ids[3]}
}

func TestMediaSweepAtStartup(t *testing.T) {
	t.Run("DEL-010/a varredura remove a pasta órfã ao iniciar; a de jogo existente e platforms ficam; o Shutdown espera por ela", func(t *testing.T) {
		var keep, orphan []int64
		w := newWiring(t, wiringOpts{
			before: func(t *testing.T, cfg config) { keep, orphan = seedSweepFixture(t, cfg) },
		})
		mediaDir := filepath.Join(w.cfg.DataDir, "media")

		// Existence of the orphan folders when the store is about to close: the sweep must be over by then.
		atStoreClose := map[int64]bool{}
		w.a.onShutdownStep = func(step string) {
			if step != "store.close" {
				return
			}
			for _, id := range orphan {
				atStoreClose[id] = dmExists(dmGameDir(mediaDir, id))
			}
		}
		w.shutdown(t)

		for _, id := range orphan {
			exists, seen := atStoreClose[id]
			if !seen {
				t.Fatal("the store.close step was not observed")
			}
			if exists {
				t.Errorf("orphan folder %d still exists when the store closes: Shutdown did not wait for the sweep", id)
			}
			if dmExists(dmGameDir(mediaDir, id)) {
				t.Errorf("orphan folder %d exists after Shutdown", id)
			}
		}
		for _, id := range keep {
			if !dmExists(dmGameDir(mediaDir, id)) {
				t.Errorf("folder of existing game %d was removed by the sweep", id)
			}
		}
		if !dmExists(filepath.Join(mediaDir, "platforms", "icon.png")) {
			t.Error("platform icon was removed by the sweep")
		}
	})
}
