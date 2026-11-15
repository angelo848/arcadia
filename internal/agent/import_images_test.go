package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

// pngVariant returns a PNG whose content (and so hash) differs for each n.
func pngVariant(t *testing.T, n int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := range 40 {
		img.Set(x, 0, color.RGBA{uint8(x * 6), uint8(n * 7), uint8(n * 13), 255})
	}
	img.Set(0, 1, color.RGBA{uint8(n), uint8(n >> 8), 1, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png: %v", err)
	}
	return buf.Bytes()
}

func writeLBFile(t *testing.T, lb, platform, folder, name string, data []byte) {
	t.Helper()
	dir := filepath.Join(lb, "Images", platform, folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func openImgStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func countKind(rows []store.MediaRow, kind string) int {
	n := 0
	for _, r := range rows {
		if r.Kind == kind {
			n++
		}
	}
	return n
}

// errListSt fails ListMedia and records any AddMedia.
type errListSt struct{ added int }

func (e *errListSt) ListMedia(context.Context, int64) ([]store.MediaRow, error) {
	return nil, io.ErrUnexpectedEOF
}
func (e *errListSt) AddMedia(context.Context, store.MediaRow) (bool, error) {
	e.added++
	return true, nil
}

func newImporter(t *testing.T, st *store.Store, log *slog.Logger) imageImporter {
	t.Helper()
	return imageImporter{files: media.New(filepath.Join(t.TempDir(), "media"), nil), st: st,
		clock: domain.NewFakeClock(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)), log: log}
}

func TestLaunchBoxImageLimit(t *testing.T) {
	t.Parallel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	newGame := func(t *testing.T, st *store.Store) int64 {
		g, err := st.AddGame(t.Context(), store.NewGame{Title: "Jogo", Exe: `C:\j.exe`, At: time.Now()})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		return g.ID
	}
	seed := func(t *testing.T, st *store.Store, id int64, kind string, n int) {
		for i := range n {
			if _, err := st.AddMedia(t.Context(), store.MediaRow{GameID: id, Kind: kind, Provider: "launchbox",
				Hash: kind + string(rune('a'+i)), Path: "p", ThumbPath: "t", CreatedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Run("AJU-020/12 screenshots distintos gravam 10", func(t *testing.T) {
		t.Parallel()
		lb, st := t.TempDir(), openImgStore(t)
		id := newGame(t, st)
		for i := 1; i <= 12; i++ {
			writeLBFile(t, lb, "Windows", "Screenshot - Gameplay", fmt.Sprintf("Jogo-%02d.png", i), pngVariant(t, i))
		}
		n, err := newImporter(t, st, quiet).CopyLaunchBox(t.Context(), id, lb, "Windows", "Jogo")
		rows, _ := st.ListMedia(t.Context(), id)
		if err != nil || n != 10 || countKind(rows, "screenshot") != 10 {
			t.Errorf("n=%d err=%v rows=%d; want 10", n, err, len(rows))
		}
	})

	t.Run("AJU-020/3 capas iguais pelo conteúdo gravam 1; uma 4ª distinta fica de fora, capa = 1 (alterado em 2026-11-15) (total 1)", func(t *testing.T) {
		t.Parallel()
		lb, st := t.TempDir(), openImgStore(t)
		id := newGame(t, st)
		same := pngVariant(t, 1)
		writeLBFile(t, lb, "Windows", "Box - Front", "Jogo-01.png", same)
		writeLBFile(t, lb, "Windows", "Box - Front", "Jogo-02.png", same)
		writeLBFile(t, lb, "Windows", "Box - Front", "Jogo-03.png", same)
		writeLBFile(t, lb, "Windows", "Box - Front", "Jogo-04.png", pngVariant(t, 2))
		n, err := newImporter(t, st, quiet).CopyLaunchBox(t.Context(), id, lb, "Windows", "Jogo")
		rows, _ := st.ListMedia(t.Context(), id)
		if err != nil || n != 1 || countKind(rows, "cover") != 1 {
			t.Errorf("n=%d err=%v covers=%d; want 1", n, err, countKind(rows, "cover"))
		}
	})

	t.Run("AJU-020/jogo com 2 capas já gravadas não recebe capa nova; screenshots continuam", func(t *testing.T) {
		t.Parallel()
		lb, st := t.TempDir(), openImgStore(t)
		id := newGame(t, st)
		seed(t, st, id, "cover", 2)
		writeLBFile(t, lb, "Windows", "Box - Front", "Jogo-01.png", pngVariant(t, 5))
		writeLBFile(t, lb, "Windows", "Screenshot - Gameplay", "Jogo-01.png", pngVariant(t, 6))
		n, err := newImporter(t, st, quiet).CopyLaunchBox(t.Context(), id, lb, "Windows", "Jogo")
		rows, _ := st.ListMedia(t.Context(), id)
		if err != nil || n != 1 || countKind(rows, "cover") != 2 || countKind(rows, "screenshot") != 1 {
			t.Errorf("n=%d err=%v rows=%+v", n, err, rows)
		}
	})

	t.Run("AJU-020/jogo com 3 capas antes continua com 3 (nada é apagado)", func(t *testing.T) {
		t.Parallel()
		lb, st := t.TempDir(), openImgStore(t)
		id := newGame(t, st)
		seed(t, st, id, "cover", 3)
		writeLBFile(t, lb, "Windows", "Box - Front", "Jogo-01.png", pngVariant(t, 5))
		n, err := newImporter(t, st, quiet).CopyLaunchBox(t.Context(), id, lb, "Windows", "Jogo")
		rows, _ := st.ListMedia(t.Context(), id)
		if err != nil || n != 0 || countKind(rows, "cover") != 3 {
			t.Errorf("n=%d err=%v covers=%d; want 0 new, 3 kept", n, err, countKind(rows, "cover"))
		}
	})

	t.Run("AJU-020/arquivo acima do limite não é lido", func(t *testing.T) {
		t.Parallel()
		lb, st := t.TempDir(), openImgStore(t)
		id := newGame(t, st)
		seed(t, st, id, "cover", 2)
		// Not a valid image: reading it would log a copy failure.
		writeLBFile(t, lb, "Windows", "Box - Front", "Jogo-01.png", []byte("not an image"))
		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, nil))
		n, err := newImporter(t, st, log).CopyLaunchBox(t.Context(), id, lb, "Windows", "Jogo")
		if err != nil || n != 0 || buf.Len() != 0 {
			t.Errorf("n=%d err=%v log=%q; want the file skipped without being read", n, err, buf.String())
		}
	})

	t.Run("AJU-020/ListMedia com erro devolve o erro e não grava nada", func(t *testing.T) {
		t.Parallel()
		lb := t.TempDir()
		writeLBFile(t, lb, "Windows", "Box - Front", "Jogo-01.png", pngVariant(t, 1))
		fake := &errListSt{}
		im := imageImporter{files: media.New(filepath.Join(t.TempDir(), "media"), nil), st: fake,
			clock: domain.NewFakeClock(time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)), log: quiet}
		n, err := im.CopyLaunchBox(t.Context(), 1, lb, "Windows", "Jogo")
		if err == nil || n != 0 || fake.added != 0 {
			t.Errorf("n=%d err=%v added=%d; want error and nothing written", n, err, fake.added)
		}
	})

	applyOne := func(t *testing.T, st *store.Store, imp imageImporter, lb string) ipc.ImportApplyResult {
		h := importHandlers(st, fakeLocations{}, testClock(), nil, imp, quiet)[ipc.MethodImportApply]
		cand := ipc.ImportCandidate{Source: ipc.ImportSourceLaunchBox, Title: "Jogo", Platform: "Windows",
			LaunchType: "exe", LaunchTarget: `C:\j.exe`, Developers: []string{}, Publishers: []string{}, Genres: []string{}, PlayModes: []string{}}
		out, err := h(t.Context(), rawParams(t, ipc.ImportApplyParams{
			Candidates: []ipc.ImportCandidate{cand}, LaunchBoxPath: filepath.Join(lb, "Data", "Platforms"),
		}))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		return out.(ipc.ImportApplyResult)
	}

	t.Run("AJU-018/import.apply: candidato \"já existe\" não recebe imagens", func(t *testing.T) {
		t.Parallel()
		lb, st := t.TempDir(), openImgStore(t)
		g, err := st.AddGame(t.Context(), store.NewGame{Title: "Jogo", Exe: `C:\j.exe`, At: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		writeLBFile(t, lb, "Windows", "Box - Front", "Jogo-01.png", pngVariant(t, 1))
		res := applyOne(t, st, newImporter(t, st, quiet), lb)
		rows, _ := st.ListMedia(t.Context(), g.ID)
		if len(res.Applied) != 0 || len(res.Skipped) != 1 || len(rows) != 0 {
			t.Errorf("result=%+v media=%d; want skipped and no images", res, len(rows))
		}
	})

	t.Run("AJU-026/import.apply com imagens do LaunchBox não cria sessão", func(t *testing.T) {
		t.Parallel()
		lb, st := t.TempDir(), openImgStore(t)
		writeLBFile(t, lb, "Windows", "Box - Front", "Jogo-01.png", pngVariant(t, 1))
		writeLBFile(t, lb, "Windows", "Screenshot - Gameplay", "Jogo-01.png", pngVariant(t, 2))
		res := applyOne(t, st, newImporter(t, st, quiet), lb)
		if len(res.Applied) != 1 {
			t.Fatalf("result = %+v", res)
		}
		id := res.Applied[0].GameID
		rows, _ := st.ListMedia(t.Context(), id)
		page, err := st.GameSessions(t.Context(), id, 0, 10)
		if err != nil || len(rows) != 2 || page.Total != 0 {
			t.Errorf("media=%d sessions=%d err=%v; want 2 images and no session", len(rows), page.Total, err)
		}
	})
}

type logEntry struct {
	Level, Msg, Platform string
	Folders              any
}

func jsonLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func unmappedEntries(t *testing.T, buf *bytes.Buffer) []logEntry {
	t.Helper()
	var out []logEntry
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var e logEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if e.Msg == "pastas de imagem do LaunchBox não lidas" {
			out = append(out, e)
		}
	}
	return out
}

func TestLogUnmappedFolders(t *testing.T) {
	t.Parallel()
	mk := func(t *testing.T, lb, platform string, folders ...string) {
		for _, f := range folders {
			if err := os.MkdirAll(filepath.Join(lb, "Images", platform, f), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	cand := func(title, platform, target, src string) ipc.ImportCandidate {
		return ipc.ImportCandidate{Source: src, Title: title, Platform: platform, LaunchType: "exe", LaunchTarget: target,
			Developers: []string{}, Publishers: []string{}, Genres: []string{}, PlayModes: []string{}}
	}
	apply := func(t *testing.T, lbPath string, cands ...ipc.ImportCandidate) (*bytes.Buffer, *store.Store) {
		var buf bytes.Buffer
		log := jsonLogger(&buf)
		st := openImgStore(t)
		h := importHandlers(st, fakeLocations{}, testClock(), nil, newImporter(t, st, log), log)[ipc.MethodImportApply]
		if _, err := h(t.Context(), rawParams(t, ipc.ImportApplyParams{Candidates: cands, LaunchBoxPath: lbPath})); err != nil {
			t.Fatalf("apply: %v", err)
		}
		return &buf, st
	}

	t.Run("AJU-022/dois candidatos da mesma plataforma com Clear Logo e Box - 3D: uma entrada Info com a plataforma e as duas pastas", func(t *testing.T) {
		t.Parallel()
		lb := t.TempDir()
		mk(t, lb, "Windows", "Clear Logo", "Box - 3D")
		buf, _ := apply(t, filepath.Join(lb, "Data", "Platforms"),
			cand("A", "Windows", `C:\a.exe`, ipc.ImportSourceLaunchBox), cand("B", "Windows", `C:\b.exe`, ipc.ImportSourceLaunchBox))
		es := unmappedEntries(t, buf)
		if len(es) != 1 || es[0].Level != "INFO" || es[0].Platform != "Windows" {
			t.Fatalf("entries = %+v", es)
		}
		f := fmt.Sprint(es[0].Folders)
		if !strings.Contains(f, "Clear Logo") || !strings.Contains(f, "Box - 3D") {
			t.Errorf("folders = %v", es[0].Folders)
		}
	})

	t.Run("AJU-022/duas plataformas com pastas extras: uma entrada por plataforma", func(t *testing.T) {
		t.Parallel()
		lb := t.TempDir()
		mk(t, lb, "Windows", "Clear Logo")
		mk(t, lb, "Sony Playstation", "Box - 3D")
		buf, _ := apply(t, filepath.Join(lb, "Data", "Platforms"),
			cand("A", "Windows", `C:\a.exe`, ipc.ImportSourceLaunchBox), cand("B", "Sony Playstation", `C:\b.exe`, ipc.ImportSourceLaunchBox))
		es := unmappedEntries(t, buf)
		got := map[string]int{}
		for _, e := range es {
			got[e.Platform]++
		}
		if len(es) != 2 || got["Windows"] != 1 || got["Sony Playstation"] != 1 {
			t.Errorf("entries = %+v", es)
		}
	})

	t.Run("AJU-022/sem pasta não mapeada: nenhuma entrada", func(t *testing.T) {
		t.Parallel()
		lb := t.TempDir()
		mk(t, lb, "Windows", "Box - Front", "Screenshot - Gameplay")
		var buf bytes.Buffer
		logUnmappedFolders(jsonLogger(&buf), map[string]bool{}, lb, "Windows")
		if es := unmappedEntries(t, &buf); len(es) != 0 {
			t.Errorf("entries = %+v, want none", es)
		}
	})

	t.Run("AJU-022/candidatos de outra fonte ou sem launchBoxPath: nenhuma entrada, e os jogos são gravados", func(t *testing.T) {
		t.Parallel()
		lb := t.TempDir()
		mk(t, lb, "Windows", "Clear Logo")
		buf, st := apply(t, filepath.Join(lb, "Data", "Platforms"), cand("S", "Windows", `C:\s.exe`, ipc.ImportSourceSteam))
		buf2, st2 := apply(t, "", cand("L", "Windows", `C:\l.exe`, ipc.ImportSourceLaunchBox))
		if es := append(unmappedEntries(t, buf), unmappedEntries(t, buf2)...); len(es) != 0 {
			t.Errorf("entries = %+v, want none", es)
		}
		for _, s := range []*store.Store{st, st2} {
			if gs, err := s.Games(t.Context()); err != nil || len(gs) != 1 {
				t.Errorf("games = %d, %v; want 1", len(gs), err)
			}
		}
	})
}
