package importers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeXboxPackage creates <root> with the two files of a Gaming Services package.
func writeXboxPackage(t *testing.T, root, config, manifest string) {
	t.Helper()
	content := root
	if err := os.MkdirAll(content, 0o755); err != nil {
		t.Fatal(err)
	}
	if config != "" {
		if err := os.WriteFile(filepath.Join(content, "MicrosoftGame.Config"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(content, "appxmanifest.xml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const (
	xboxCfg = "\xef\xbb\xbf" + `<?xml version="1.0" encoding="utf-8"?>
<Game configVersion="1"><Identity Name="38985CA0.COREBase"/>
<ShellVisuals DefaultDisplayName="Call of Duty®" PublisherDisplayName="Activision"/>
<StoreId>9N201KQXS5BM</StoreId></Game>`
	xboxMan = `<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10">
<Applications><Application Id="codShip" Executable="GameLaunchHelper.exe"/></Applications></Package>`
)

func scanXbox(t *testing.T, roots ...string) Result {
	t.Helper()
	res, err := XboxFrom(func() ([]string, error) { return roots, nil }).Scan(context.Background(), ScanInput{})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestXbox(t *testing.T) {
	tmp := t.TempDir()
	game := filepath.Join(tmp, "Call of Duty")
	writeXboxPackage(t, game, xboxCfg, xboxMan)
	dlc := filepath.Join(tmp, "DLC")
	writeXboxPackage(t, dlc, `<Game><ShellVisuals DefaultDisplayName="Pack"/></Game>`, xboxMan) // no StoreId
	noApp := filepath.Join(tmp, "NoApp")
	writeXboxPackage(t, noApp, xboxCfg, `<Package><Applications/></Package>`)
	resTitle := filepath.Join(tmp, "Res")
	writeXboxPackage(t, resTitle, `<Game><ShellVisuals DefaultDisplayName="ms-resource:Name"/><StoreId>X</StoreId></Game>`, xboxMan)
	empty := filepath.Join(tmp, "Empty") // empty folder
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}

	res := scanXbox(t, game, dlc, noApp, resTitle, empty)
	if len(res.Candidates) != 1 {
		t.Fatalf("candidates = %+v, want only the game", res.Candidates)
	}
	c := res.Candidates[0]
	want := Candidate{
		Source:       SourceXbox,
		Title:        "Call of Duty",
		Platform:     PlatformWindows,
		LaunchType:   LaunchURI,
		LaunchTarget: "msgamelaunch://shortcutLaunch/?ProductId=9N201KQXS5BM&Exe=codShip",
		InstallDir:   realDir(game), // the real path, with junctions and short names resolved
	}
	if c.Source != want.Source || c.Title != want.Title || c.Platform != want.Platform || c.LaunchType != want.LaunchType ||
		c.LaunchTarget != want.LaunchTarget || c.InstallDir != want.InstallDir {
		t.Errorf("candidate = %+v, want %+v", c, want)
	}
	if len(res.Warnings) != 1 {
		t.Errorf("warnings = %v, want one about the unnamed game", res.Warnings)
	}
}

func TestXboxSortedByTitle(t *testing.T) {
	tmp := t.TempDir()
	b := filepath.Join(tmp, "b")
	a := filepath.Join(tmp, "a")
	writeXboxPackage(t, b, `<Game><ShellVisuals DefaultDisplayName="Zelda"/><StoreId>B</StoreId></Game>`, xboxMan)
	writeXboxPackage(t, a, `<Game><ShellVisuals DefaultDisplayName="Alan Wake"/><StoreId>A</StoreId></Game>`, xboxMan)
	res := scanXbox(t, b, a)
	if len(res.Candidates) != 2 || res.Candidates[0].Title != "Alan Wake" {
		t.Errorf("candidates = %+v, want sorted by title", res.Candidates)
	}
}

func TestXboxListFailure(t *testing.T) {
	res, err := XboxFrom(func() ([]string, error) { return nil, errors.New("boom") }).Scan(context.Background(), ScanInput{})
	if err != nil || len(res.Candidates) != 0 || len(res.Warnings) != 1 {
		t.Errorf("res = %+v, err = %v, want one warning and no candidates", res, err)
	}
}
