package importers_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/importers"
)

func TestFolderScan(t *testing.T) {
	t.Parallel()

	t.Run("IMP-004/só o primeiro nível, só .exe, título sem extensão", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "ABZU.exe"), "")
		writeFile(t, filepath.Join(dir, "notas.txt"), "")
		writeFile(t, filepath.Join(dir, "sub", "Game.exe"), "")

		res, err := importers.Folder().Scan(context.Background(), importers.ScanInput{Path: dir})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want only ABZU.exe", res.Candidates)
		}
		c := res.Candidates[0]
		if c.Title != "ABZU" || c.Platform != "" || c.LaunchType != importers.LaunchExe {
			t.Errorf("candidate = %+v", c)
		}
		if c.LaunchTarget != filepath.Join(dir, "ABZU.exe") || c.InstallDir != dir {
			t.Errorf("target/installDir = %q / %q", c.LaunchTarget, c.InstallDir)
		}
	})

	t.Run("IMP-004/subpastas não são lidas", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "deep", "deeper", "Hidden.exe"), "")

		res, err := importers.Folder().Scan(context.Background(), importers.ScanInput{Path: dir})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if len(res.Candidates) != 0 {
			t.Errorf("candidates = %+v, want none", res.Candidates)
		}
	})

	t.Run("IMP-004/pasta inexistente devolve aviso", func(t *testing.T) {
		t.Parallel()
		res, err := importers.Folder().Scan(context.Background(), importers.ScanInput{Path: filepath.Join(t.TempDir(), "x")})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if len(res.Warnings) != 1 || len(res.Candidates) != 0 {
			t.Errorf("result = %+v", res)
		}
	})
}
