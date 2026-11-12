package importers_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/importers"
)

// writeFile creates path (and its folders) with the given text.
func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// appManifest returns the text of an appmanifest for one app.
func appManifest(appID, name, installDir string) string {
	return `"AppState"
{
	"appid"		"` + appID + `"
	"name"		"` + name + `"
	"installdir"		"` + installDir + `"
	"StateFlags"		"4"
}
`
}

func scanSteam(t *testing.T, root string) importers.Result {
	t.Helper()
	res, err := importers.Steam().Scan(context.Background(), importers.ScanInput{Path: root})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return res
}

func TestSteamScan(t *testing.T) {
	t.Parallel()

	t.Run("IMP-002/lê appmanifest e devolve candidato com appid, nome, pasta e uri", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"), `"libraryfolders"
{
	"0"
	{
		"path"		"`+root+`"
		"apps"
		{
			"730"		"1234"
		}
	}
}
`)
		writeFile(t, filepath.Join(root, "steamapps", "appmanifest_730.acf"), appManifest("730", "Counter-Strike 2", "Counter-Strike Global Offensive"))

		res := scanSteam(t, root)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %d, want 1 (%+v)", len(res.Candidates), res)
		}
		c := res.Candidates[0]
		if c.Title != "Counter-Strike 2" || c.Source != importers.SourceSteam || c.Platform != importers.PlatformPC {
			t.Errorf("candidate = %+v", c)
		}
		if c.LaunchType != importers.LaunchURI || c.LaunchTarget != "steam://rungameid/730" {
			t.Errorf("launch = %s %s, want uri steam://rungameid/730", c.LaunchType, c.LaunchTarget)
		}
		want := filepath.Join(root, "steamapps", "common", "Counter-Strike Global Offensive")
		if c.InstallDir != want {
			t.Errorf("InstallDir = %q, want %q", c.InstallDir, want)
		}
	})

	t.Run("IMP-002/biblioteca em outra pasta listada no libraryfolders também é lida", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		other := t.TempDir()
		writeFile(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"), `"libraryfolders"
{
	"1"
	{
		"path"		"`+strings.ReplaceAll(other, `\`, `\\`)+`"
	}
}
`)
		writeFile(t, filepath.Join(other, "steamapps", "appmanifest_440.acf"), appManifest("440", "Team Fortress 2", "Team Fortress 2"))

		res := scanSteam(t, root)
		if len(res.Candidates) != 1 || res.Candidates[0].Title != "Team Fortress 2" {
			t.Errorf("candidates = %+v, want Team Fortress 2 from the second library", res.Candidates)
		}
	})

	t.Run("IMP-002/biblioteca inexistente devolve zero candidatos e aviso", func(t *testing.T) {
		t.Parallel()
		res := scanSteam(t, t.TempDir())
		if len(res.Candidates) != 0 {
			t.Errorf("candidates = %d, want 0", len(res.Candidates))
		}
		if len(res.Warnings) == 0 {
			t.Errorf("warnings empty, want one for the missing library")
		}
	})

	t.Run("IMP-002/appmanifest ilegível vira aviso e não derruba o scan", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "steamapps", "appmanifest_570.acf"), `"AppState" {`)
		writeFile(t, filepath.Join(root, "steamapps", "appmanifest_730.acf"), appManifest("730", "Counter-Strike 2", "CS2"))

		res := scanSteam(t, root)
		if len(res.Candidates) != 1 {
			t.Errorf("candidates = %d, want the good manifest only", len(res.Candidates))
		}
		if len(res.Warnings) != 1 {
			t.Errorf("warnings = %v, want one for the broken manifest", res.Warnings)
		}
	})

	t.Run("IMP-002/ferramentas da Steam não são jogos", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "steamapps", "appmanifest_228980.acf"), appManifest("228980", "Steamworks Common Redistributables", "Steamworks Shared"))

		res := scanSteam(t, root)
		if len(res.Candidates) != 0 || len(res.Warnings) != 0 {
			t.Errorf("result = %+v, want nothing for a tool", res)
		}
	})
}
