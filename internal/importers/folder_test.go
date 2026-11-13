package importers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// makeFile creates a file with some content under dir, making its folders.
func makeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestFolderScan(t *testing.T) {
	t.Parallel()

	t.Run("IMP-004/pasta: todo arquivo do primeiro nível vira candidato, sem filtro por extensão", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		makeFile(t, filepath.Join(dir, "Zelda.sfc"))
		makeFile(t, filepath.Join(dir, "Mario.nes"))
		makeFile(t, filepath.Join(dir, "ABZU.exe"))
		makeFile(t, filepath.Join(dir, "Run.bat"))
		makeFile(t, filepath.Join(dir, "notas.txt"))

		res, err := Folder().Scan(context.Background(), ScanInput{Path: dir})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		got := map[string]Candidate{}
		for _, c := range res.Candidates {
			got[c.Title] = c
		}
		if len(res.Candidates) != 5 {
			t.Fatalf("candidates = %d, want 5 (every file)", len(res.Candidates))
		}
		if c := got["Zelda"]; c.LaunchType != LaunchEmulator || c.LaunchTarget != filepath.Join(dir, "Zelda.sfc") || c.Source != SourceFolder {
			t.Errorf("ROM = %+v, want emulator with the file as target", c)
		}
		if c := got["ABZU"]; c.LaunchType != LaunchExe {
			t.Errorf(".exe = %+v, want exe", c)
		}
		if c := got["Run"]; c.LaunchType != LaunchExe {
			t.Errorf(".bat = %+v, want exe", c)
		}
		if c := got["notas"]; c.LaunchType != LaunchEmulator {
			t.Errorf("other file = %+v, want emulator", c)
		}
	})

	t.Run("IMP-004/subpastas não são lidas", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		makeFile(t, filepath.Join(dir, "Top.sfc"))
		makeFile(t, filepath.Join(dir, "sub", "Deep.sfc"))
		res, err := Folder().Scan(context.Background(), ScanInput{Path: dir})
		if err != nil || len(res.Candidates) != 1 || res.Candidates[0].Title != "Top" {
			t.Errorf("result = %+v, %v; want only Top", res, err)
		}
	})

	t.Run("IMP-004/pasta vazia ou inexistente devolve aviso", func(t *testing.T) {
		t.Parallel()
		for name, path := range map[string]string{"vazia": t.TempDir(), "inexistente": filepath.Join(t.TempDir(), "x")} {
			res, err := Folder().Scan(context.Background(), ScanInput{Path: path})
			if err != nil || len(res.Candidates) != 0 || len(res.Warnings) != 1 {
				t.Errorf("%s: result = %+v, %v; want one warning and no candidate", name, res, err)
			}
		}
	})
}
