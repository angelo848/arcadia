package importers

import (
	"fmt"
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

	t.Run("MID-010/lê os tipos do Windows, incluindo pastas de região, na ordem da tabela", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, "Images", "Windows", "Banner", "Starfield-01.jpg")
		touchImage(t, root, "Images", "Windows", "Fanart - Background", "Starfield-01.jpg")
		touchImage(t, root, "Images", "Windows", "Screenshot - Game Title", "Starfield-01.png")
		touchImage(t, root, "Images", "Windows", "Screenshot - Gameplay", "Starfield-01.jpg")
		touchImage(t, root, "Images", "Windows", "Screenshot - Game Select", "North America", "Starfield-01.jpg")
		touchImage(t, root, "Images", "Windows", "Screenshot - Game Select", "World", "Starfield-02.jpg")

		got := LaunchBoxImages(filepath.Join(root), "Windows", "Starfield")
		want := []string{"screenshot", "title", "select", "select", "background", "banner"}
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

	const guid = "0ba52b39-70d5-4dd5-87ce-73de60080878"
	lbPath := func(parts ...string) []string { return append([]string{"Images", "Windows"}, parts...) }
	base := func(refs []ImageRef) []string {
		out := []string{}
		for _, r := range refs {
			out = append(out, filepath.Base(r.Path))
		}
		return out
	}

	t.Run("AJU-018/cada pasta da tabela vira o tipo indicado", func(t *testing.T) {
		t.Parallel()
		table := map[string][]string{
			"cover":      {"Box - Front", "Box - Front - Reconstructed", "Fanart - Box - Front", "Steam Poster", "Epic Games Poster", "GOG Poster", "Origin Poster", "Uplay Thumbnail", "Amazon Poster"},
			"screenshot": {"Screenshot - Gameplay", "Steam Screenshot", "Epic Games Screenshot", "GOG Screenshot", "Origin Screenshot", "Amazon Screenshot", "Screenshot - High Scores", "Screenshot - Game Over"},
			"title":      {"Screenshot - Game Title"},
			"select":     {"Screenshot - Game Select"},
			"background": {"Fanart - Background", "Epic Games Background", "Origin Background", "Uplay Background", "Amazon Background"},
			"banner":     {"Banner", "Steam Banner"},
		}
		for kind, folders := range table {
			for _, f := range folders {
				root := t.TempDir()
				touchImage(t, root, lbPath(f, "Jogo-01.jpg")...)
				got := LaunchBoxImages(root, "Windows", "Jogo")
				if len(got) != 1 || got[0].Kind != kind {
					t.Errorf("folder %q: got %+v, want one %s", f, got, kind)
				}
			}
		}
	})

	t.Run("AJU-018/Box - Front e Steam Poster viram cover; Steam Banner vira banner; Epic Games Background vira background", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, lbPath("Box - Front", "Jogo-01.jpg")...)
		touchImage(t, root, lbPath("Steam Poster", "Jogo-01.jpg")...)
		touchImage(t, root, lbPath("Steam Banner", "Jogo-01.jpg")...)
		touchImage(t, root, lbPath("Epic Games Background", "Jogo-01.jpg")...)
		got := strings.Join(kindsOf(LaunchBoxImages(root, "Windows", "Jogo")), ",")
		if got != "cover,cover,background,banner" {
			t.Errorf("kinds = %s", got)
		}
	})

	t.Run("AJU-018/pasta não mapeada (Clear Logo, Box - 3D) não é lida", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		for _, f := range []string{"Clear Logo", "Box - 3D", "Box - Back", "Box - Spine", "Cart - Front", "Disc", "Advertisement Flyer - Front", "Arcade - Marquee", "Manual"} {
			touchImage(t, root, lbPath(f, "Jogo-01.jpg")...)
		}
		if got := LaunchBoxImages(root, "Windows", "Jogo"); len(got) != 0 {
			t.Errorf("images = %+v, want none", got)
		}
	})

	t.Run("AJU-018/nome de pasta com outras maiúsculas é lido", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, lbPath("BOX - front", "Jogo-01.jpg")...)
		got := LaunchBoxImages(root, "Windows", "Jogo")
		if len(got) != 1 || got[0].Kind != "cover" {
			t.Errorf("images = %+v, want one cover", got)
		}
	})

	t.Run("AJU-018/pasta ausente ou sem arquivo do jogo devolve nada, sem erro", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		if got := LaunchBoxImages(root, "Windows", "Jogo"); len(got) != 0 {
			t.Errorf("absent: %+v", got)
		}
		touchImage(t, root, lbPath("Box - Front", "Outro-01.jpg")...)
		if got := LaunchBoxImages(root, "Windows", "Jogo"); len(got) != 0 {
			t.Errorf("other game: %+v", got)
		}
	})

	t.Run("AJU-019/Box - Front raiz antes de região, antes de Steam Poster", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, lbPath("Steam Poster", "Jogo-01.jpg")...)
		touchImage(t, root, lbPath("Box - Front", "North America", "Jogo-01.jpg")...)
		touchImage(t, root, lbPath("Box - Front", "Jogo-01.jpg")...)
		got := LaunchBoxImages(root, "Windows", "Jogo")
		if len(got) != 3 {
			t.Fatalf("images = %+v", got)
		}
		gotDirs := []string{
			filepath.Base(filepath.Dir(got[0].Path)),
			filepath.Base(filepath.Dir(got[1].Path)),
			filepath.Base(filepath.Dir(got[2].Path)),
		}
		if strings.Join(gotDirs, "|") != "Box - Front|North America|Steam Poster" {
			t.Errorf("dirs = %v", gotDirs)
		}
	})

	t.Run("AJU-019/regiões em ordem alfabética sem maiúsculas; -01 antes de -02", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, lbPath("Box - Front", "world", "Jogo-01.jpg")...)
		touchImage(t, root, lbPath("Box - Front", "Japan", "Jogo-02.jpg")...)
		touchImage(t, root, lbPath("Box - Front", "Japan", "Jogo-01.jpg")...)
		touchImage(t, root, lbPath("Box - Front", "Europe", "Jogo-01.jpg")...)
		var seq []string
		for _, r := range LaunchBoxImages(root, "Windows", "Jogo") {
			seq = append(seq, filepath.Base(filepath.Dir(r.Path))+"/"+filepath.Base(r.Path))
		}
		want := "Europe/Jogo-01.jpg,Japan/Jogo-01.jpg,Japan/Jogo-02.jpg,world/Jogo-01.jpg"
		if strings.Join(seq, ",") != want {
			t.Errorf("order = %v, want %s", seq, want)
		}
	})

	t.Run("AJU-019/ordem entre tipos segue a tabela", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		for _, f := range []string{"Banner", "Fanart - Background", "Screenshot - Game Select", "Screenshot - Game Title", "Screenshot - Gameplay", "Box - Front"} {
			touchImage(t, root, lbPath(f, "Jogo-01.jpg")...)
		}
		got := strings.Join(kindsOf(LaunchBoxImages(root, "Windows", "Jogo")), ",")
		if got != "cover,screenshot,title,select,background,banner" {
			t.Errorf("kinds = %s", got)
		}
	})

	t.Run("AJU-020/LaunchBoxImages não limita: 12 screenshots dão 12 referências", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		for i := 1; i <= 12; i++ {
			touchImage(t, root, lbPath("Screenshot - Gameplay", fmt.Sprintf("Jogo-%02d.jpg", i))...)
		}
		if got := LaunchBoxImages(root, "Windows", "Jogo"); len(got) != 12 {
			t.Errorf("images = %d, want 12", len(got))
		}
	})

	t.Run("AJU-021/aceita T-01.jpg e T.<GUID>-01.jpg com .jpg, .jpeg e .png", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		names := []string{
			"Jump Super Stars-01.jpg",
			"Jump Super Stars." + guid + "-01.jpg",
			"Jump Super Stars." + guid + "-02.jpeg",
			"Jump Super Stars." + guid + "-03.png",
			"Jump Super Stars." + strings.ToUpper(guid) + "-04.jpg",
		}
		for _, n := range names {
			touchImage(t, root, lbPath("Screenshot - Gameplay", n)...)
		}
		touchImage(t, root, lbPath("Fanart - Background", "Jump Super Stars."+guid+"-01.png")...)
		touchImage(t, root, lbPath("Box - Front", "Japan", "Jump Super Stars."+guid+"-01.jpg")...)
		got := LaunchBoxImages(root, "Windows", "Jump Super Stars")
		if len(got) != len(names)+2 {
			t.Fatalf("images = %v, want %d", base(got), len(names)+2)
		}
	})

	t.Run("AJU-021/recusa T 2-01.jpg, T.naoguid-01.jpg, T.<GUID>.jpg e .gif", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		for _, n := range []string{
			"Jogo 2-01.jpg",
			"Jogo.naoguid-01.jpg",
			"Jogo." + guid + ".jpg",
			"Jogo-01.gif",
			"Jogo." + guid + "-01.gif",
			"Jogo-01.webp",
		} {
			touchImage(t, root, lbPath("Screenshot - Gameplay", n)...)
		}
		if got := LaunchBoxImages(root, "Windows", "Jogo"); len(got) != 0 {
			t.Errorf("images = %v, want none", base(got))
		}
	})

	t.Run("AJU-021/título com ':' e apóstrofo trocados por '_' também com GUID", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, lbPath("Box - Front", "North America", "Mass Effect_ Legendary Edition-01.jpg")...)
		touchImage(t, root, lbPath("Screenshot - Gameplay", "Assassin_s Creed Mirage."+guid+"-01.jpg")...)
		if got := LaunchBoxImages(root, "Windows", "Mass Effect: Legendary Edition"); len(got) != 1 {
			t.Errorf("colon: %d, want 1", len(got))
		}
		if got := LaunchBoxImages(root, "Windows", "Assassin's Creed Mirage"); len(got) != 1 {
			t.Errorf("apostrophe+GUID: %d, want 1", len(got))
		}
	})
}

func TestUnmappedLaunchBoxFolders(t *testing.T) {
	t.Parallel()

	t.Run("AJU-022/UnmappedLaunchBoxFolders lista só as subpastas fora da tabela, em ordem; arquivos soltos não entram", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, "Images", "Windows", "Clear Logo", "a-01.png")
		touchImage(t, root, "Images", "Windows", "Box - 3D", "a-01.png")
		touchImage(t, root, "Images", "Windows", "Box - Front", "a-01.png")
		touchImage(t, root, "Images", "Windows", "banner", "a-01.png")
		touchImage(t, root, "Images", "Windows", "Manual", "a-01.png")
		touchImage(t, root, "Images", "Windows", "solto.txt")
		got := UnmappedLaunchBoxFolders(root, "Windows")
		if strings.Join(got, "|") != "Box - 3D|Clear Logo|Manual" {
			t.Errorf("folders = %v", got)
		}
	})

	t.Run("AJU-022/pasta da plataforma ausente ou plataforma com separador dá nil", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		touchImage(t, root, "Images", "Windows", "Clear Logo", "a-01.png")
		if got := UnmappedLaunchBoxFolders(root, "Atari 2600"); got != nil {
			t.Errorf("absent = %v, want nil", got)
		}
		if got := UnmappedLaunchBoxFolders(root, `..\Windows`); got != nil {
			t.Errorf("separator = %v, want nil", got)
		}
	})
}
