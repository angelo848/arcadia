package importers_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/importers"
)

// IMV-007 and IMV-024: the titles of Steam, Epic and LaunchBox are the launcher's own, never normalized,
// and their candidates carry none of the file fields (File, Extension, Discs, DiscFiles).
func TestLauncherTitlesNotNormalized(t *testing.T) {
	t.Parallel()

	t.Run("IMV-007/Steam: título do launcher sem normalização e sem os campos novos", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		raw := "Game, The (USA) [!] - Edição_Especial"
		writeFile(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"), `"libraryfolders"
{
	"0"
	{
		"path"		"`+root+`"
		"apps"
		{
			"4242"		"1"
		}
	}
}
`)
		writeFile(t, filepath.Join(root, "steamapps", "appmanifest_4242.acf"), appManifest("4242", raw, "Game"))

		res := scanSteam(t, root)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		c := res.Candidates[0]
		if c.Title != raw {
			t.Errorf("Title = %q, want launcher title %q", c.Title, raw)
		}
		if c.File != "" || c.Extension != "" || c.Discs != 0 || c.DiscFiles != nil {
			t.Errorf("file fields = %q/%q/%d/%v, want empty", c.File, c.Extension, c.Discs, c.DiscFiles)
		}
	})

	t.Run("IMV-007/Epic: título do launcher sem normalização e sem os campos novos", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		raw := "Jogo, The - Edição (USA) [!]"
		writeFile(t, filepath.Join(dir, "JOGO.item"), `{"AppName":"abc123","DisplayName":"`+raw+`","InstallLocation":"D:\\Games\\Jogo"}`)

		res, err := importers.Epic().Scan(context.Background(), importers.ScanInput{Path: dir})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		c := res.Candidates[0]
		if c.Title != raw {
			t.Errorf("Title = %q, want launcher title %q", c.Title, raw)
		}
		if c.File != "" || c.Extension != "" || c.Discs != 0 || c.DiscFiles != nil {
			t.Errorf("file fields = %q/%q/%d/%v, want empty", c.File, c.Extension, c.Discs, c.DiscFiles)
		}
	})

	t.Run("IMV-007/LaunchBox: título do launcher sem normalização e sem os campos novos", func(t *testing.T) {
		t.Parallel()
		res := scanLaunchBox(t, lbFixture(t))
		if len(res.Candidates) == 0 {
			t.Fatalf("candidates = 0, want the fixture games")
		}
		// The title of the fixture game is the launcher's, with its spaces and words intact.
		byTitle(t, res, "Grand Theft Auto V Enhanced")
		for _, c := range res.Candidates {
			if c.File != "" || c.Extension != "" || c.Discs != 0 || c.DiscFiles != nil {
				t.Errorf("%q: file fields = %q/%q/%d/%v, want empty", c.Title, c.File, c.Extension, c.Discs, c.DiscFiles)
			}
		}
	})
}
