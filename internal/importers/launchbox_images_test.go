package importers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// touchImage creates an empty file at root/rel, making the folders on the way.
func touchImage(t *testing.T, root string, rel ...string) {
	t.Helper()
	p := filepath.Join(append([]string{root}, rel...)...)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// kindsOf lists the kinds of the references, in order, for comparison.
func kindsOf(refs []ImageRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Kind)
	}
	return out
}

func TestLaunchBoxImages(t *testing.T) {
	t.Parallel()

	t.Run("MID-010/lê os tipos do Windows, incluindo pastas de região", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, "Images", "Windows", "Banner", "Starfield-01.jpg")
		touchImage(t, root, "Images", "Windows", "Fanart - Background", "Starfield-01.jpg")
		touchImage(t, root, "Images", "Windows", "Screenshot - Game Title", "Starfield-01.png")
		touchImage(t, root, "Images", "Windows", "Screenshot - Gameplay", "Starfield-01.jpg")
		touchImage(t, root, "Images", "Windows", "Screenshot - Game Select", "North America", "Starfield-01.jpg")
		touchImage(t, root, "Images", "Windows", "Screenshot - Game Select", "World", "Starfield-02.jpg")

		got := LaunchBoxImages(filepath.Join(root), "Windows", "Starfield")
		want := []string{"banner", "background", "title", "screenshot", "select", "select"}
		if strings.Join(kindsOf(got), ",") != strings.Join(want, ",") {
			t.Fatalf("kinds = %v, want %v", kindsOf(got), want)
		}
	})

	t.Run("MID-010/o nome troca ': ' por '_ ' e apóstrofo por '_'", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, "Images", "Windows", "Screenshot - Gameplay", "Black Myth_ Wukong-01.jpg")
		touchImage(t, root, "Images", "Windows", "Screenshot - Gameplay", "Assassin_s Creed Mirage-01.jpg")

		if got := LaunchBoxImages(root, "Windows", "Black Myth: Wukong"); len(got) != 1 {
			t.Errorf("colon title: %d images, want 1", len(got))
		}
		if got := LaunchBoxImages(root, "Windows", "Assassin's Creed Mirage"); len(got) != 1 {
			t.Errorf("apostrophe title: %d images, want 1", len(got))
		}
	})

	t.Run("MID-010/não pega imagem de outro jogo com o título no começo", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, "Images", "Windows", "Screenshot - Gameplay", "Doom-01.jpg")
		touchImage(t, root, "Images", "Windows", "Screenshot - Gameplay", "Doom Eternal-01.jpg")
		touchImage(t, root, "Images", "Windows", "Screenshot - Gameplay", "Doom-notes.txt")

		got := LaunchBoxImages(root, "Windows", "Doom")
		if len(got) != 1 || filepath.Base(got[0].Path) != "Doom-01.jpg" {
			t.Errorf("images = %+v, want only Doom-01.jpg", got)
		}
	})

	t.Run("MID-010/no máximo cinco imagens por tipo, na ordem do número", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		for i := 1; i <= 7; i++ {
			touchImage(t, root, "Images", "Windows", "Screenshot - Gameplay", "Wukong-0"+string(rune('0'+i))+".jpg")
		}
		got := LaunchBoxImages(root, "Windows", "Wukong")
		if len(got) != maxImagesPerKind {
			t.Fatalf("images = %d, want %d", len(got), maxImagesPerKind)
		}
		if filepath.Base(got[0].Path) != "Wukong-01.jpg" {
			t.Errorf("first = %s, want Wukong-01.jpg", filepath.Base(got[0].Path))
		}
	})

	t.Run("MID-010/pasta de imagens ausente devolve nada, sem erro", func(t *testing.T) {
		t.Parallel()
		if got := LaunchBoxImages(t.TempDir(), "Windows", "Starfield"); len(got) != 0 {
			t.Errorf("images = %+v, want none", got)
		}
	})

	t.Run("PLT-028/copyLaunchBoxImages lê Images\\Windows\\... para um jogo Windows", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		plats := filepath.Join(root, "Data", "Platforms")
		if err := os.MkdirAll(plats, 0o755); err != nil {
			t.Fatal(err)
		}
		xml := `<LaunchBox>` +
			`<Game><Title>Starfield</Title><Platform>Windows</Platform><ApplicationPath>C:\g\s.exe</ApplicationPath></Game>` +
			`<Game><Title>Mario</Title><Platform>Nintendo 64</Platform><ApplicationPath>C:/r/m.z64</ApplicationPath><Emulator>P64</Emulator></Game>` +
			`</LaunchBox>`
		if err := os.WriteFile(filepath.Join(plats, "Windows.xml"), []byte(xml), 0o644); err != nil {
			t.Fatal(err)
		}
		touchImage(t, root, "Images", "Windows", "Screenshot - Gameplay", "Starfield-01.jpg")
		touchImage(t, root, "Images", "Nintendo 64", "Banner", "Mario-01.jpg")

		res, err := LaunchBox().Scan(t.Context(), ScanInput{Path: plats})
		if err != nil || len(res.Candidates) != 2 {
			t.Fatalf("Scan = %+v, %v; want 2 candidates", res, err)
		}
		for _, c := range res.Candidates {
			// The folder searched is the platform name of the XML, for any platform.
			got := LaunchBoxImages(root, c.Platform, c.Title)
			if len(got) != 1 {
				t.Errorf("%s (%s): %d images, want 1", c.Title, c.Platform, len(got))
			}
		}
		if res.Candidates[0].Platform != "Windows" && res.Candidates[1].Platform != "Windows" {
			t.Errorf("no candidate with platform Windows: %+v", res.Candidates)
		}
	})

	t.Run("MID-010/nome com separador de pasta é recusado", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, "Images", "Windows", "Screenshot - Gameplay", "x-01.jpg")
		if got := LaunchBoxImages(root, "Windows", "..\\x"); len(got) != 0 {
			t.Errorf("images = %+v, want none for a title with a separator", got)
		}
	})
}
