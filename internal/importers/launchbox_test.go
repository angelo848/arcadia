package importers_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/importers"
)

// lbFixture copies the anonymized real Windows.xml (3 games) into a LaunchBox
// Data\Platforms folder and returns the folder.
func lbFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "launchbox", "Windows-sample.xml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Windows.xml"), string(data))
	return dir
}

func scanLaunchBox(t *testing.T, dir string) importers.Result {
	t.Helper()
	res, err := importers.LaunchBox().Scan(context.Background(), importers.ScanInput{Path: dir})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return res
}

// byTitle finds a candidate by title.
func byTitle(t *testing.T, res importers.Result, title string) importers.Candidate {
	t.Helper()
	for _, c := range res.Candidates {
		if c.Title == title {
			return c
		}
	}
	t.Fatalf("candidate %q not found in %+v", title, res.Candidates)
	return importers.Candidate{}
}

func TestLaunchBoxScan(t *testing.T) {
	t.Parallel()

	t.Run("IMP-005/Game vira candidato com título, plataforma, empresas, gêneros, ano e caminho", func(t *testing.T) {
		t.Parallel()
		res := scanLaunchBox(t, lbFixture(t))
		if len(res.Candidates) != 3 {
			t.Fatalf("candidates = %d, want 3", len(res.Candidates))
		}
		gta := byTitle(t, res, "Grand Theft Auto V Enhanced")
		if gta.Platform != importers.PlatformPC || gta.Source != importers.SourceLaunchBox {
			t.Errorf("platform/source = %q/%q", gta.Platform, gta.Source)
		}
		if !slices.Equal(gta.Developers, []string{"Rockstar North"}) || !slices.Equal(gta.Publishers, []string{"Rockstar Games"}) {
			t.Errorf("developers/publishers = %v/%v", gta.Developers, gta.Publishers)
		}
		if !slices.Equal(gta.Genres, []string{"Racing", "Sandbox", "Shooter"}) {
			t.Errorf("genres = %v", gta.Genres)
		}
		if gta.Year == nil || *gta.Year != 2025 {
			t.Errorf("year = %v, want 2025", gta.Year)
		}
		if gta.LaunchType != importers.LaunchURI || gta.LaunchTarget != "com.epicgames.launcher://apps/8769e24080ea413b8ebca3f1b8c50951?action=launch&silent=true" {
			t.Errorf("launch = %s %s", gta.LaunchType, gta.LaunchTarget)
		}
		fc := byTitle(t, res, "EA SPORTS FC 24")
		if fc.LaunchType != importers.LaunchExe || fc.LaunchTarget != `C:\Games\EXEMPLO\FC24.exe` {
			t.Errorf("exe game launch = %s %s", fc.LaunchType, fc.LaunchTarget)
		}
	})

	t.Run("IMP-005/PlayCount e LastPlayedDate viram tempo jogado; sem tempo não há sessão", func(t *testing.T) {
		t.Parallel()
		res := scanLaunchBox(t, lbFixture(t))
		cs := byTitle(t, res, "Counter-Strike 2")
		if cs.Played == nil {
			t.Fatal("Counter-Strike 2 has PlayTime 33060: Played must be set")
		}
		if cs.Played.Seconds != 33060 {
			t.Errorf("Seconds = %d, want 33060", cs.Played.Seconds)
		}
		// 2025-01-22T00:00:08-03:00 is 03:00:08 UTC.
		want := time.Date(2025, time.January, 22, 3, 0, 8, 0, time.UTC)
		if !cs.Played.LastPlay.Equal(want) {
			t.Errorf("LastPlay = %v, want %v", cs.Played.LastPlay, want)
		}
		fc := byTitle(t, res, "EA SPORTS FC 24")
		if fc.Played == nil || fc.Played.Seconds != 5133 {
			t.Errorf("EA SPORTS FC 24 Played = %+v, want 5133 seconds", fc.Played)
		}

		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "Windows.xml"), `<LaunchBox><Game><Title>Sem tempo</Title><ApplicationPath>C:\a.exe</ApplicationPath><PlayCount>0</PlayCount><PlayTime>0</PlayTime></Game></LaunchBox>`)
		if zero := byTitle(t, scanLaunchBox(t, dir), "Sem tempo"); zero.Played != nil {
			t.Errorf("PlayTime 0: Played = %+v, want nil", zero.Played)
		}
	})

	t.Run("IMP-005/modos em português viram nomes de DOM-014", func(t *testing.T) {
		t.Parallel()
		res := scanLaunchBox(t, lbFixture(t))
		gta := byTitle(t, res, "Grand Theft Auto V Enhanced")
		if !slices.Equal(gta.PlayModes, []string{"coop_online", "multi_online"}) {
			t.Errorf("PlayModes = %v, want [coop_online multi_online]", gta.PlayModes)
		}
		if got := byTitle(t, res, "Counter-Strike 2").Description; got == "" {
			t.Errorf("Description empty; Notes must be read")
		}
	})

	t.Run("IMP-005/jogo sem título ou caminho é ignorado e contado no aviso", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "Windows.xml"), `<?xml version="1.0"?><LaunchBox>
<Game><Title>Sem caminho</Title><Platform>Windows</Platform></Game>
<Game><Title>Com caminho</Title><ApplicationPath>C:\x\y.exe</ApplicationPath></Game>
<Game><ApplicationPath>C:\z.exe</ApplicationPath></Game>
</LaunchBox>`)
		res := scanLaunchBox(t, dir)
		if len(res.Candidates) != 1 || res.Candidates[0].Title != "Com caminho" {
			t.Errorf("candidates = %+v", res.Candidates)
		}
		if len(res.Warnings) != 1 {
			t.Errorf("warnings = %v, want one counting the skipped games", res.Warnings)
		}
	})

	t.Run("IMP-008/Windows.xml ausente vira aviso e zero candidatos", func(t *testing.T) {
		t.Parallel()
		res := scanLaunchBox(t, t.TempDir())
		if len(res.Candidates) != 0 || len(res.Warnings) != 1 {
			t.Errorf("result = %+v", res)
		}
	})

	t.Run("IMP-008/XML inválido vira aviso e não derruba o scan", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "Windows.xml"), `<LaunchBox><Game>`)
		res := scanLaunchBox(t, dir)
		if len(res.Candidates) != 0 || len(res.Warnings) != 1 {
			t.Errorf("result = %+v", res)
		}
	})
}
