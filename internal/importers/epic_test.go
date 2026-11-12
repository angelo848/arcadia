package importers_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/importers"
)

func TestEpicScan(t *testing.T) {
	t.Parallel()

	t.Run("IMP-003/item vira candidato com uri de lançamento silencioso", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "GTA.item"), `{"AppName":"8769e24080ea413b8ebca3f1b8c50951","DisplayName":"Grand Theft Auto V Enhanced","InstallLocation":"D:\\Games\\GTA V"}`)

		res, err := importers.Epic().Scan(context.Background(), importers.ScanInput{Path: dir})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %d, want 1", len(res.Candidates))
		}
		c := res.Candidates[0]
		if c.Title != "Grand Theft Auto V Enhanced" || c.Platform != importers.PlatformPC || c.Source != importers.SourceEpic {
			t.Errorf("candidate = %+v", c)
		}
		want := "com.epicgames.launcher://apps/8769e24080ea413b8ebca3f1b8c50951?action=launch&silent=true"
		if c.LaunchType != importers.LaunchURI || c.LaunchTarget != want {
			t.Errorf("launch = %s %s, want uri %s", c.LaunchType, c.LaunchTarget, want)
		}
		if c.InstallDir != `D:\Games\GTA V` {
			t.Errorf("InstallDir = %q", c.InstallDir)
		}
	})

	t.Run("IMP-003/item sem DisplayName é ignorado e contado no aviso", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "a.item"), `{"AppName":"abc","DisplayName":""}`)
		writeFile(t, filepath.Join(dir, "b.item"), `{"AppName":"def","DisplayName":"Jogo"}`)

		res, err := importers.Epic().Scan(context.Background(), importers.ScanInput{Path: dir})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if len(res.Candidates) != 1 || res.Candidates[0].Title != "Jogo" {
			t.Errorf("candidates = %+v, want only Jogo", res.Candidates)
		}
		if len(res.Warnings) != 1 {
			t.Errorf("warnings = %v, want one counting the unnamed item", res.Warnings)
		}
	})

	t.Run("IMP-003/pasta de manifestos inexistente devolve aviso e zero candidatos", func(t *testing.T) {
		t.Parallel()
		res, err := importers.Epic().Scan(context.Background(), importers.ScanInput{Path: filepath.Join(t.TempDir(), "nao-existe")})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if len(res.Candidates) != 0 || len(res.Warnings) != 1 {
			t.Errorf("result = %+v", res)
		}
	})

	t.Run("IMP-003/item com JSON inválido vira aviso e não derruba o scan", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "bad.item"), `{"AppName":`)
		writeFile(t, filepath.Join(dir, "ok.item"), `{"AppName":"x","DisplayName":"Ok"}`)
		if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignored"), 0o644); err != nil {
			t.Fatal(err)
		}

		res, err := importers.Epic().Scan(context.Background(), importers.ScanInput{Path: dir})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if len(res.Candidates) != 1 || len(res.Warnings) != 1 {
			t.Errorf("result = %+v", res)
		}
	})
}
