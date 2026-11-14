package agent

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// recImages records the copies asked for by import.apply.
type recImages struct{ calls []string }

func (r *recImages) CopyLaunchBox(_ context.Context, gameID int64, root, platform, title string) (int, error) {
	r.calls = append(r.calls, filepath.Join(root, platform, title))
	return 0, nil
}

func TestLaunchBoxImageImport(t *testing.T) {
	t.Parallel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("MID-010/a pasta Data/Platforms dá a raiz do LaunchBox", func(t *testing.T) {
		t.Parallel()
		lb := filepath.Join(t.TempDir(), "LaunchBox")
		if got := launchBoxRoot(filepath.Join(lb, "Data", "Platforms")); got != lb {
			t.Errorf("root = %q, want %q", got, lb)
		}
	})

	t.Run("MID-010/outra pasta não lê imagem nenhuma", func(t *testing.T) {
		t.Parallel()
		if got := launchBoxRoot(filepath.Join(t.TempDir(), "Games")); got != "" {
			t.Errorf("root = %q, want empty", got)
		}
	})

	t.Run("MID-010/só candidatos do LaunchBox com a pasta copiam imagens", func(t *testing.T) {
		t.Parallel()
		rec := &recImages{}
		lbPath := filepath.Join("C:", "LB", "Data", "Platforms")
		lb := ipc.ImportCandidate{Source: ipc.ImportSourceLaunchBox, Title: "Starfield", Platform: "Windows"}
		steam := ipc.ImportCandidate{Source: ipc.ImportSourceSteam, Title: "Starfield", Platform: "Windows"}

		copyLaunchBoxImages(context.Background(), rec, quiet, lbPath, lb, 1)
		copyLaunchBoxImages(context.Background(), rec, quiet, lbPath, steam, 2)
		copyLaunchBoxImages(context.Background(), rec, quiet, "", lb, 3)
		copyLaunchBoxImages(context.Background(), nil, quiet, lbPath, lb, 4)

		if len(rec.calls) != 1 || rec.calls[0] != filepath.Join("C:", "LB", "Windows", "Starfield") {
			t.Errorf("copies = %v, want only the LaunchBox candidate with a folder", rec.calls)
		}
	})

	t.Run("PLT-028/copyLaunchBoxImages lê Images\\Windows\\... para um jogo Windows (apply do LaunchBox)", func(t *testing.T) {
		t.Parallel()
		lb := t.TempDir()
		img := filepath.Join(lb, "Images", "Windows", "Screenshot - Gameplay")
		if err := os.MkdirAll(img, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(img, "Starfield-01.png"), pngFixture(t), 0o644); err != nil {
			t.Fatal(err)
		}
		st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
		if err != nil {
			t.Fatalf("store.Open: %v", err)
		}
		t.Cleanup(func() { _ = st.Close() })
		imp := imageImporter{files: media.New(filepath.Join(t.TempDir(), "media"), nil), st: st, clock: domain.NewFakeClock(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)), log: quiet}
		h := importHandlers(st, fakeLocations{}, testClock(), nil, imp, quiet)[ipc.MethodImportApply]
		cand := ipc.ImportCandidate{Source: ipc.ImportSourceLaunchBox, Title: "Starfield", Platform: "Windows",
			LaunchType: "exe", LaunchTarget: `C:\s.exe`, Developers: []string{}, Publishers: []string{}, Genres: []string{}, PlayModes: []string{}}
		out, err := h(t.Context(), rawParams(t, ipc.ImportApplyParams{
			Candidates:    []ipc.ImportCandidate{cand},
			LaunchBoxPath: filepath.Join(lb, "Data", "Platforms"),
		}))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		res := out.(ipc.ImportApplyResult)
		if len(res.Applied) != 1 {
			t.Fatalf("result = %+v, want 1 applied", res)
		}
		rows, err := st.ListMedia(t.Context(), res.Applied[0].GameID)
		if err != nil || len(rows) != 1 || rows[0].Provider != "launchbox" {
			t.Errorf("media = %+v, %v; want one launchbox image from Images\\Windows", rows, err)
		}
	})

	t.Run("MID-010/copia e registra as imagens do jogo na biblioteca", func(t *testing.T) {
		t.Parallel()
		lb := t.TempDir()
		img := filepath.Join(lb, "Images", "Windows", "Screenshot - Gameplay")
		if err := os.MkdirAll(img, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(img, "Black Myth_ Wukong-01.png"), pngFixture(t), 0o644); err != nil {
			t.Fatal(err)
		}
		st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
		if err != nil {
			t.Fatalf("store.Open: %v", err)
		}
		t.Cleanup(func() { _ = st.Close() })
		g, err := st.AddGame(t.Context(), store.NewGame{Title: "Black Myth: Wukong", Exe: `C:\b.exe`, At: time.Now()})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		dataRoot := t.TempDir()
		imp := imageImporter{files: media.New(filepath.Join(dataRoot, "media"), nil), st: st, clock: domain.NewFakeClock(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)), log: quiet}

		n, err := imp.CopyLaunchBox(t.Context(), g.ID, lb, "Windows", "Black Myth: Wukong")
		if err != nil || n != 1 {
			t.Fatalf("CopyLaunchBox = %d, %v; want 1", n, err)
		}
		rows, err := st.ListMedia(t.Context(), g.ID)
		if err != nil || len(rows) != 1 || rows[0].Kind != "screenshot" || rows[0].Provider != "launchbox" {
			t.Errorf("media = %+v, %v; want one launchbox screenshot", rows, err)
		}
	})
}

// pngFixture returns a small PNG, the content of a LaunchBox image in the tests.
func pngFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := range 40 {
		img.Set(x, 0, color.RGBA{uint8(x * 6), 90, 200, 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png: %v", err)
	}
	return buf.Bytes()
}
