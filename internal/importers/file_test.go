package importers_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/importers"
)

func TestFileScan(t *testing.T) {
	t.Parallel()

	scan := func(t *testing.T, path string) importers.Result {
		t.Helper()
		res, err := importers.File().Scan(context.Background(), importers.ScanInput{Path: path})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		return res
	}

	t.Run("IMP-004/executável vira exe com título sem extensão", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "ABZU.exe")
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		c := res.Candidates[0]
		if c.Title != "ABZU" || c.Platform != "" || c.LaunchType != importers.LaunchExe ||
			c.LaunchTarget != path || c.InstallDir != dir || c.Source != importers.SourceFile {
			t.Errorf("candidate = %+v", c)
		}
	})

	t.Run("IMP-004/.bat e .cmd também são exe", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"Run.BAT", "start.cmd"} {
			path := filepath.Join(t.TempDir(), name)
			writeFile(t, path, "")
			res := scan(t, path)
			if len(res.Candidates) != 1 || res.Candidates[0].LaunchType != importers.LaunchExe {
				t.Errorf("%s: candidates = %+v, want exe", name, res.Candidates)
			}
		}
	})

	t.Run("IMP-004/outro tipo de arquivo (ROM) vira emulator com o arquivo como alvo", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "Zelda.sfc")
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		c := res.Candidates[0]
		if c.Title != "Zelda" || c.LaunchType != importers.LaunchEmulator || c.LaunchTarget != path {
			t.Errorf("candidate = %+v", c)
		}
	})

	t.Run("IMP-004/pasta, arquivo inexistente ou caminho vazio devolvem aviso", func(t *testing.T) {
		t.Parallel()
		for name, path := range map[string]string{
			"pasta":       t.TempDir(),
			"inexistente": filepath.Join(t.TempDir(), "x.exe"),
			"vazio":       "",
		} {
			res := scan(t, path)
			if len(res.Warnings) != 1 || len(res.Candidates) != 0 {
				t.Errorf("%s: result = %+v, want one warning and no candidate", name, res)
			}
		}
	})
}
