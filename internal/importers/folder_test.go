package importers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Tests of the folder source (phase 11, T08): scan through Source.Scan, plus the internal steps classify,
// toCandidates and buildWarnings. Fixtures are empty files in t.TempDir(); the spies never open a file [IMV-080].
// Layouts and warning texts come from contracts/import-rules.md (§4.4 and §5).

// errFtDenied is the read error injected into fake folders [IMV-012].
var errFtDenied = errors.New("leitura negada (teste)")

// Expected main files of the árvore A of import-rules.md §4.4 (titleFrom = file).
const (
	ftFFDisc1  = `04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 1)\Final Fantasy VIII (USA) (Disc 1).cue`
	ftFFDisc2  = `04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 2)\Final Fantasy VIII (USA) (Disc 2).cue`
	ftRE2Disc1 = `07. Resident Evil 2 (USA)\Resident Evil 2 - Dual Shock Ver. (USA) (Disc 1)\Resident Evil 2 - Dual Shock Ver. (USA) (Disc 1).cue`
	ftRE2Disc2 = `07. Resident Evil 2 (USA)\Resident Evil 2 - Dual Shock Ver. (USA) (Disc 2)\Resident Evil 2 - Dual Shock Ver. (USA) (Disc 2).cue`
	ftSpyro    = `31. Spyro - Year of the Dragon (USA)\Spyro - Year of the Dragon (USA).cue`
)

// mkTree creates empty files under root, with their folders. rels use "\" as the scan does.
func mkTree(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir para %q: %v", rel, err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatalf("criar %q: %v", rel, err)
		}
	}
}

// ftArvoreA creates the layout of import-rules.md §4.4 (árvore A) and returns its root.
func ftArvoreA(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mkTree(t, root,
		ftFFDisc1, `04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 1)\Final Fantasy VIII (USA) (Disc 1).bin`,
		ftFFDisc2, `04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 2)\Final Fantasy VIII (USA) (Disc 2).bin`,
		ftRE2Disc1,
		ftRE2Disc2,
		ftSpyro, `31. Spyro - Year of the Dragon (USA)\Spyro - Year of the Dragon (USA).bin`,
	)
	return root
}

// ftScan runs a scan through src with the given folder options and fails the test on error.
func ftScan(t *testing.T, src Source, root string, opts FolderOptions) Result {
	t.Helper()
	res, err := src.Scan(context.Background(), ScanInput{Path: root, Folder: opts})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return res
}

// ftCheck compares two string lists; nil and empty are the same.
func ftCheck(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}

// ftTitles returns the titles of the candidates, in order.
func ftTitles(cs []Candidate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Title)
	}
	return out
}

// ftByFile indexes candidates by their File field, for tests that do not depend on order.
func ftByFile(cs []Candidate) map[string]Candidate {
	out := make(map[string]Candidate, len(cs))
	for _, c := range cs {
		out[c.File] = c
	}
	return out
}

// ftWant is the expected content of one folder candidate.
type ftWant struct {
	title, file, ext, launch string
	discs                    int
	discFiles                []string
}

// wantDiscs is the expected Discs: a simple candidate (zero value in ftWant) has 1 disc [IMV-024].
func wantDiscs(w ftWant) int {
	if w.discs == 0 {
		return 1
	}
	return w.discs
}

// ftCheckCands compares the candidates, in order, with the expected ones. It also checks that launchTarget is the
// disk path of the main file and that installDir is its folder [IMV-016, IMV-007].
func ftCheckCands(t *testing.T, root string, got []Candidate, want []ftWant) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("candidates = %d (%q), want %d", len(got), ftTitles(got), len(want))
	}
	for i, w := range want {
		c := got[i]
		target := filepath.Join(root, w.file)
		switch {
		case c.Title != w.title:
			t.Errorf("[%d] Title = %q, want %q", i, c.Title, w.title)
		case c.File != w.file:
			t.Errorf("[%d] File = %q, want %q", i, c.File, w.file)
		case c.Extension != w.ext:
			t.Errorf("[%d] Extension = %q, want %q", i, c.Extension, w.ext)
		case c.LaunchType != w.launch:
			t.Errorf("[%d] LaunchType = %q, want %q", i, c.LaunchType, w.launch)
		case c.LaunchTarget != target:
			t.Errorf("[%d] LaunchTarget = %q, want %q", i, c.LaunchTarget, target)
		case c.InstallDir != filepath.Dir(target):
			t.Errorf("[%d] InstallDir = %q, want %q", i, c.InstallDir, filepath.Dir(target))
		case c.Source != SourceFolder:
			t.Errorf("[%d] Source = %q, want %q", i, c.Source, SourceFolder)
		case c.Discs != wantDiscs(w):
			t.Errorf("[%d] Discs = %d, want %d", i, c.Discs, wantDiscs(w))
		case !slices.Equal(c.DiscFiles, w.discFiles) || (w.discs < 2 && c.DiscFiles != nil):
			t.Errorf("[%d] DiscFiles = %q, want %q", i, c.DiscFiles, w.discFiles)
		}
	}
}

// ftCheckIgn compares the ignored counts with the expected ones.
func ftCheckIgn(t *testing.T, got ignoredCounts, byExt map[string]int, console, exe int) {
	t.Helper()
	if len(got.ByExt) != len(byExt) {
		t.Errorf("ByExt = %v, want %v", got.ByExt, byExt)
	} else {
		for k, v := range byExt {
			if got.ByExt[k] != v {
				t.Errorf("ByExt[%q] = %d, want %d (all: %v)", k, got.ByExt[k], v, got.ByExt)
			}
		}
	}
	if got.Console != console || got.Exe != exe {
		t.Errorf("Console, Exe = %d, %d; want %d, %d", got.Console, got.Exe, console, exe)
	}
}

// ftPrefixWarn checks that exactly one warning starts with prefix (used where the number in the text depends on
// injected limits, which the spec does not fix).
func ftPrefixWarn(t *testing.T, warnings []string, prefix string) {
	t.Helper()
	if len(warnings) != 1 || !strings.HasPrefix(warnings[0], prefix) {
		t.Errorf("warnings = %q, want one starting with %q", warnings, prefix)
	}
}

// ftDirs is a fake dirReader: a tree in memory keyed by the path relative to root ("." is the root). It records the
// names it was asked for and can fail some folders. It has no Open [IMV-080].
type ftDirs struct {
	root  string
	tree  map[string][]fs.DirEntry
	fail  map[string]error
	calls []string
}

// ReadDir implements dirReader.
func (d *ftDirs) ReadDir(name string) ([]fs.DirEntry, error) {
	rel, err := filepath.Rel(d.root, name)
	if err != nil {
		return nil, err
	}
	d.calls = append(d.calls, rel)
	if e, ok := d.fail[rel]; ok {
		return nil, e
	}
	entries, ok := d.tree[rel]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return entries, nil
}

// ftEntry is a fake directory entry of a given type.
type ftEntry struct {
	name string
	mode fs.FileMode
}

func (e ftEntry) Name() string      { return e.name }
func (e ftEntry) IsDir() bool       { return e.mode.IsDir() }
func (e ftEntry) Type() fs.FileMode { return e.mode.Type() }
func (e ftEntry) Info() (fs.FileInfo, error) {
	return nil, errors.New("Info não usado pela leitura (teste)")
}

func ftFile(name string) fs.DirEntry { return ftEntry{name: name} }
func ftDir(name string) fs.DirEntry  { return ftEntry{name: name, mode: fs.ModeDir} }
func ftLink(name string) fs.DirEntry { return ftEntry{name: name, mode: fs.ModeSymlink} }
func ftIrregular(name string) fs.DirEntry {
	return ftEntry{name: name, mode: fs.ModeIrregular}
}

// ftSpy wraps a real dirReader and records every name it is asked for. It only implements ReadDir [IMV-080].
type ftSpy struct {
	inner  dirReader
	calls  []string
	onCall func()
}

// ReadDir implements dirReader.
func (s *ftSpy) ReadDir(name string) ([]fs.DirEntry, error) {
	s.calls = append(s.calls, name)
	if s.onCall != nil {
		s.onCall()
	}
	return s.inner.ReadDir(name)
}

// ftSnapshot describes a tree: the existence of each folder, and the name, size and modification time of each
// file. Folder mtimes are left out: NTFS changes them when entries are listed, which is not a write by the scan.
func ftSnapshot(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			fmt.Fprintf(&b, "%s|dir\n", rel)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s|file|%d|%d\n", rel, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot da pasta: %v", err)
	}
	return b.String()
}

func TestFolderLayouts(t *testing.T) {
	t.Parallel()

	t.Run("IMV-010/layout ps1: soltos e um jogo por subpasta, em ordem natural", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root,
			`Avulso (USA).chd`,
			`10. Dez (USA)\Dez (USA).cue`,
			`2. Dois (USA)\Dois (USA).cue`,
			`2. Dois (USA)\Dois (USA).bin`,
		)
		res := ftScan(t, Folder(), root, FolderOptions{})
		titles := ftTitles(res.Candidates)
		if len(res.Candidates) != 3 {
			t.Fatalf("titles = %q, want 3 (the .bin of Dois is a track, not a candidate)", titles)
		}
		iDois, iDez := slices.Index(titles, "Dois"), slices.Index(titles, "Dez")
		if iDois < 0 || iDez < 0 || iDois > iDez {
			t.Errorf("titles = %q, want Dois (2. Dois) before Dez (10. Dez)", titles)
		}
		if !slices.Contains(titles, "Avulso") {
			t.Errorf("titles = %q, want the loose file Avulso", titles)
		}
	})

	t.Run("IMV-010/árvore B (n64): três candidatos com título limpo e file igual ao nome", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root,
			`Toy Story 2 (U) [!].zip`,
			`Legend of Zelda, The - Majora's Mask (U) [!].zip`,
			`Bomberman 64 - Arcade Edition (J).zip`,
		)
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheckCands(t, root, res.Candidates, []ftWant{
			{title: "Bomberman 64 - Arcade Edition", file: `Bomberman 64 - Arcade Edition (J).zip`, ext: "zip", launch: LaunchEmulator},
			{title: "The Legend of Zelda - Majora's Mask", file: `Legend of Zelda, The - Majora's Mask (U) [!].zip`, ext: "zip", launch: LaunchEmulator},
			{title: "Toy Story 2", file: `Toy Story 2 (U) [!].zip`, ext: "zip", launch: LaunchEmulator},
		})
		ftCheck(t, "warnings", res.Warnings, nil)
	})

	t.Run("IMV-010/ordem natural: Jogo 2 antes de Jogo 10 e pasta/arquivo no mesmo critério", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `Jogo 10.sfc`, `Jogo 2.sfc`, `Jogo 3\Interno.sfc`)
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"Jogo 2", "Interno", "Jogo 10"})
	})

	t.Run("IMV-010/profundidade 8 é lida e 9 não", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		eight := `n1\n2\n3\n4\n5\n6\n7\n8\Oito.sfc`
		nine := `n1\n2\n3\n4\n5\n6\n7\n8\n9\Nove.sfc`
		mkTree(t, root, eight, nine)
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"Oito"})
		ftPrefixWarn(t, res.Warnings, "Pasta: subpastas além do nível 8 não foram lidas")
	})

	t.Run("IMV-010/pasta vazia, inexistente, arquivo no lugar de pasta e caminho vazio devolvem os avisos da fase 6", func(t *testing.T) {
		t.Parallel()
		file := filepath.Join(t.TempDir(), "x.sfc")
		mkTree(t, filepath.Dir(file), "x.sfc")
		cases := map[string]struct {
			path string
			want string
		}{
			"vazia":         {t.TempDir(), "Pasta: nenhum jogo encontrado"},
			"inexistente":   {filepath.Join(t.TempDir(), "x"), "Pasta: não encontrada"},
			"arquivo":       {file, "Pasta: escolha uma pasta, não um arquivo"},
			"caminho vazio": {"", "Pasta: nenhuma pasta escolhida"},
		}
		for name, tc := range cases {
			res := ftScan(t, Folder(), tc.path, FolderOptions{})
			if len(res.Candidates) != 0 {
				t.Errorf("%s: candidates = %q, want none", name, ftTitles(res.Candidates))
			}
			ftCheck(t, name+": warnings", res.Warnings, []string{tc.want})
		}
	})

	t.Run("IMV-010/pasta cuja leitura falha na raiz devolve o aviso de ilegível da fase 6", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		src := newFolderSource(&ftDirs{root: root, fail: map[string]error{".": errFtDenied}})
		res := ftScan(t, src, root, FolderOptions{})
		if len(res.Candidates) != 0 {
			t.Errorf("candidates = %q, want none", ftTitles(res.Candidates))
		}
		ftCheck(t, "warnings", res.Warnings, []string{"Pasta: " + root + " ilegível"})
	})

	t.Run("IMV-010/Folder() lê o disco pelo caminho real: soltos e subpasta", func(t *testing.T) {
		t.Parallel()
		if Folder().ID() != SourceFolder {
			t.Errorf("ID = %q, want %q", Folder().ID(), SourceFolder)
		}
		root := t.TempDir()
		mkTree(t, root, `Solto.sfc`, `Sub\Dentro.sfc`)
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"Solto", "Dentro"}) // "solto.sfc" < "sub": 'o' < 'u' [import-rules.md §3]
	})
}

func TestFolderFields(t *testing.T) {
	t.Parallel()

	t.Run("IMV-016/launchType exe para exe, bat e cmd; emulator para os demais; alvo e pasta do disco", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `Alfa.exe`, `Jogo B\Beta.bat`, `Jogo D\Delta.sfc`, `Jogo G\Gama.cmd`)
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheckCands(t, root, res.Candidates, []ftWant{
			{title: "Alfa", file: `Alfa.exe`, ext: "exe", launch: LaunchExe},
			{title: "Beta", file: `Jogo B\Beta.bat`, ext: "bat", launch: LaunchExe},
			{title: "Delta", file: `Jogo D\Delta.sfc`, ext: "sfc", launch: LaunchEmulator},
			{title: "Gama", file: `Jogo G\Gama.cmd`, ext: "cmd", launch: LaunchExe},
		})
		if got := res.Candidates[1].InstallDir; got != filepath.Join(root, "Jogo B") {
			t.Errorf("InstallDir de arquivo em subpasta = %q, want a pasta do arquivo %q", got, filepath.Join(root, "Jogo B"))
		}
		if got := res.Candidates[0].InstallDir; got != root {
			t.Errorf("InstallDir de arquivo da raiz = %q, want a pasta escolhida %q", got, root)
		}
	})

	t.Run("IMV-017/título pelo arquivo e pela pasta (04. Final Fantasy VIII e 07. Resident Evil 2 nas duas opções)", func(t *testing.T) {
		t.Parallel()
		root := ftArvoreA(t)
		fileOpt := ftScan(t, Folder(), root, FolderOptions{TitleFrom: TitleFromFile})
		folderOpt := ftScan(t, Folder(), root, FolderOptions{TitleFrom: TitleFromFolder})

		ftCheckCands(t, root, fileOpt.Candidates, []ftWant{
			{title: "Final Fantasy VIII", file: ftFFDisc1, ext: "cue", launch: LaunchEmulator, discs: 2, discFiles: []string{ftFFDisc1, ftFFDisc2}},
			{title: "Resident Evil 2 - Dual Shock Ver.", file: ftRE2Disc1, ext: "cue", launch: LaunchEmulator, discs: 2, discFiles: []string{ftRE2Disc1, ftRE2Disc2}},
			{title: "Spyro - Year of the Dragon", file: ftSpyro, ext: "cue", launch: LaunchEmulator, discs: 1},
		})
		ftCheckCands(t, root, folderOpt.Candidates, []ftWant{
			{title: "Final Fantasy VIII", file: ftFFDisc1, ext: "cue", launch: LaunchEmulator, discs: 2, discFiles: []string{ftFFDisc1, ftFFDisc2}},
			{title: "Resident Evil 2", file: ftRE2Disc1, ext: "cue", launch: LaunchEmulator, discs: 2, discFiles: []string{ftRE2Disc1, ftRE2Disc2}},
			{title: "Spyro - Year of the Dragon", file: ftSpyro, ext: "cue", launch: LaunchEmulator, discs: 1},
		})
	})

	t.Run("IMV-017/arquivo na raiz com titleFrom folder usa o nome do arquivo", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `Solto (USA).sfc`)
		res := ftScan(t, Folder(), root, FolderOptions{TitleFrom: TitleFromFolder})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"Solto"})
	})

	t.Run("IMV-017/pasta só com marcações mantém o nome da pasta, sem queda para o arquivo", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `(USA)\Jogo Real.sfc`)
		res := ftScan(t, Folder(), root, FolderOptions{TitleFrom: TitleFromFolder})
		// NormalizeTitle("(USA)", true) keeps "(USA)" (import-rules.md §2 step 7); the user edits it in the review window.
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"(USA)"})
	})

	t.Run("IMV-024/campos file, extension, discs e discFiles; discFiles só com discs >= 2", func(t *testing.T) {
		t.Parallel()
		root := ftArvoreA(t)
		mkTree(t, root, `Solo.SFC`)
		res := ftScan(t, Folder(), root, FolderOptions{})
		by := ftByFile(res.Candidates)

		if c, ok := by["Solo.SFC"]; !ok || c.Extension != "sfc" || c.Discs != 1 || c.DiscFiles != nil {
			t.Errorf("Solo.SFC = %+v, want extension sfc, discs 1, no discFiles", c)
		}
		if c, ok := by[ftFFDisc1]; !ok || c.Discs != 2 || len(c.DiscFiles) != 2 {
			t.Errorf("disco 1 de Final Fantasy = %+v, want discs 2 with two discFiles", c)
		}
		if c, ok := by[ftSpyro]; !ok || c.Discs != 1 || c.DiscFiles != nil {
			t.Errorf("Spyro = %+v, want discs 1 and no discFiles", c)
		}
	})

	t.Run("IMV-007/só o título muda: launchTarget e installDir são os do disco; marcações seguem em file", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		name := `Jogos\Super_Mario_World (USA) [!].sfc`
		mkTree(t, root, name)
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheckCands(t, root, res.Candidates, []ftWant{
			{title: "Super Mario World", file: name, ext: "sfc", launch: LaunchEmulator},
		})
	})
}

func TestFolderExtensions(t *testing.T) {
	t.Parallel()

	t.Run("IMV-013/uma extensão de cada classe: Windows lê só exe/bat/cmd, console só ROM e disco, qualquer as duas", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `a.exe`, `b.bat`, `c.cmd`, `d.sfc`, `e.zip`, `f.txt`, `g.txt`)
		cases := map[string]struct {
			kind FolderKind
			want []string
		}{
			"executáveis": {KindExecutables, []string{"a", "b", "c"}},
			"console":     {KindConsole, []string{"d", "e"}},
			"qualquer":    {KindAny, []string{"a", "b", "c", "d", "e"}},
		}
		for name, tc := range cases {
			res := ftScan(t, Folder(), root, FolderOptions{Kind: tc.kind})
			ftCheck(t, name+": titles", ftTitles(res.Candidates), tc.want)
		}
	})

	t.Run("IMV-013/extensão em maiúsculas (.ZIP, .EXE) é lida e vira minúscula", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `GAME.ZIP`, `Tool.EXE`)
		res := ftScan(t, Folder(), root, FolderOptions{})
		by := ftByFile(res.Candidates)
		if len(res.Candidates) != 2 {
			t.Fatalf("candidates = %q, want 2", ftTitles(res.Candidates))
		}
		if c := by["GAME.ZIP"]; c.Extension != "zip" || c.LaunchType != LaunchEmulator {
			t.Errorf("GAME.ZIP = %+v, want extension zip and emulator", c)
		}
		if c := by["Tool.EXE"]; c.Extension != "exe" || c.LaunchType != LaunchExe {
			t.Errorf("Tool.EXE = %+v, want extension exe and exe", c)
		}
	})

	t.Run("IMV-013/arquivo de outra extensão não vira candidato e entra no aviso", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `notas.txt`, `capa.png`, `save.sav`, `lib.dll`, `Jogo.sfc`)
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"Jogo"})
		ftCheck(t, "warnings", res.Warnings, []string{"Pasta: 4 arquivos ignorados pela extensão (.dll ×1, .png ×1, .sav ×1, .txt ×1)"})
	})

	t.Run("IMV-015/setup.exe ignorado na pasta e contado como .exe", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `setup.exe`, `unins000.exe`, `Jogo.exe`)
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"Jogo"})
		ftCheck(t, "warnings", res.Warnings, []string{"Pasta: 2 arquivos ignorados pela extensão (.exe ×2)"})
	})

	t.Run("IMV-015/a lista de não-jogos vale só para executáveis: setup.zip continua candidato", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `setup.zip`)
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"setup"})
	})
}

func TestFolderWarnings(t *testing.T) {
	t.Parallel()

	t.Run("IMV-014/árvore F: aviso exato com .exe ×1, .pak ×1, .txt ×1", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `Jogo X\bin\jogo.exe`, `Jogo X\unins000.exe`, `Jogo X\readme.txt`, `Jogo X\data.pak`, `Jogo Y.exe`)
		res := ftScan(t, Folder(), root, FolderOptions{Kind: KindExecutables})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"jogo", "Jogo Y"})
		ftCheck(t, "warnings", res.Warnings, []string{"Pasta: 3 arquivos ignorados pela extensão (.exe ×1, .pak ×1, .txt ×1)"})

		folderOpt := ftScan(t, Folder(), root, FolderOptions{Kind: KindExecutables, TitleFrom: TitleFromFolder})
		ftCheck(t, "titles com titleFrom folder", ftTitles(folderOpt.Candidates), []string{"Jogo X", "Jogo Y"})
	})

	t.Run("IMV-014/nenhum candidato com ROMs numa leitura Windows: aviso de plataforma trocada", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `Jogo.sfc`, `Outro.iso`)
		res := ftScan(t, Folder(), root, FolderOptions{Kind: KindExecutables})
		if len(res.Candidates) != 0 {
			t.Fatalf("candidates = %q, want none", ftTitles(res.Candidates))
		}
		if len(res.Warnings) < 2 ||
			res.Warnings[0] != "Pasta: 2 arquivos ignorados pela extensão (.iso ×1, .sfc ×1)" ||
			!slices.Contains(res.Warnings, "Pasta: a plataforma escolhida lê só executáveis; 2 arquivos de jogo de console foram ignorados") {
			t.Errorf("warnings = %q, want the extension warning first and the wrong-platform warning", res.Warnings)
		}
	})

	t.Run("IMV-014/nenhum candidato com executáveis numa leitura de console: aviso de plataforma trocada", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `Jogo.exe`, `Setup.exe`)
		res := ftScan(t, Folder(), root, FolderOptions{Kind: KindConsole})
		if len(res.Candidates) != 0 {
			t.Fatalf("candidates = %q, want none", ftTitles(res.Candidates))
		}
		if len(res.Warnings) < 2 ||
			res.Warnings[0] != "Pasta: 2 arquivos ignorados pela extensão (.exe ×2)" ||
			!slices.Contains(res.Warnings, "Pasta: a plataforma escolhida lê só arquivos de jogo de console; 2 executáveis foram ignorados") {
			t.Errorf("warnings = %q, want the extension warning first and the wrong-platform warning", res.Warnings)
		}
	})

	t.Run("IMV-014/nenhum jogo no primeiro nível quando FirstLevelOnly liga o aviso próprio", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `Sub\Jogo.sfc`)
		first := ftScan(t, Folder(), root, FolderOptions{FirstLevelOnly: true})
		ftCheck(t, "FirstLevelOnly: warnings", first.Warnings, []string{"Pasta: nenhum jogo no primeiro nível"})

		deep := ftScan(t, Folder(), root, FolderOptions{})
		ftCheck(t, "leitura recursiva: titles", ftTitles(deep.Candidates), []string{"Jogo"})
		ftCheck(t, "leitura recursiva: warnings", deep.Warnings, nil)
	})

	t.Run("IMV-011/FirstLevelOnly liga e desliga na mesma árvore", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `Solto.sfc`, `Alfa\Dentro.sfc`, `Alfa\notas.txt`, `Alfa\capa.png`)

		first := ftScan(t, Folder(), root, FolderOptions{FirstLevelOnly: true})
		ftCheck(t, "ligado: titles", ftTitles(first.Candidates), []string{"Solto"})
		ftCheck(t, "ligado: warnings (subpasta não lida não é contada)", first.Warnings, nil)

		all := ftScan(t, Folder(), root, FolderOptions{})
		ftCheck(t, "desligado: titles", ftTitles(all.Candidates), []string{"Dentro", "Solto"})
		ftCheck(t, "desligado: warnings", all.Warnings, []string{"Pasta: 2 arquivos ignorados pela extensão (.png ×1, .txt ×1)"})
	})

	t.Run("IMV-012/nomes ignorados e o que há dentro deles não são lidos nem contados", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root,
			`.git\a.sfc`,
			`$RECYCLE.BIN\b.sfc`,
			`System Volume Information\c.sfc`,
			`@eaDir\d.sfc`,
			`__MACOSX\e.sfc`,
			`node_modules\f.sfc`,
			`.oculto.sfc`,
			`$lixo.sfc`,
			`Real (USA).sfc`,
		)
		spy := &ftSpy{inner: osDirReader{}}
		res := ftScan(t, newFolderSource(spy), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"Real"})
		ftCheck(t, "warnings", res.Warnings, nil)
		if len(spy.calls) != 1 || spy.calls[0] != filepath.Clean(root) {
			t.Errorf("ReadDir calls = %q, want only the root", spy.calls)
		}
	})

	t.Run("IMV-012/ModeSymlink e ModeIrregular por dirReader falso: não são lidos nem contados", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		dirs := &ftDirs{root: root, tree: map[string][]fs.DirEntry{
			".":    {ftIrregular("Junc"), ftLink("link.txt"), ftFile("ok.sfc")},
			"Junc": {ftFile("x.sfc")},
		}}
		res := ftScan(t, newFolderSource(dirs), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"ok"})
		ftCheck(t, "warnings", res.Warnings, nil)
		for _, c := range dirs.calls {
			if c == "Junc" {
				t.Errorf("ReadDir de %q: a entrada irregular não deve ser lida (calls %q)", c, dirs.calls)
			}
		}
	})

	t.Run("IMV-012/link simbólico real: não é seguido (t.Skip se o processo não puder criá-lo)", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `real\x.sfc`)
		if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "atalho")); err != nil {
			t.Skipf("sem privilégio para criar link simbólico neste processo: %v", err)
		}
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"x"})
	})

	t.Run("IMV-012/aviso de n subpastas ilegíveis, e a leitura segue nas demais", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		dirs := &ftDirs{
			root: root,
			tree: map[string][]fs.DirEntry{
				".":    {ftFile("Ok.sfc"), ftDir("Bad1"), ftDir("Bad2"), ftDir("Fine")},
				"Fine": {ftFile("Dentro.sfc")},
			},
			fail: map[string]error{"Bad1": errFtDenied, "Bad2": errFtDenied},
		}
		res := ftScan(t, newFolderSource(dirs), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"Dentro", "Ok"})
		ftCheck(t, "warnings", res.Warnings, []string{"Pasta: 2 subpastas ilegíveis"})
	})

	t.Run("IMV-014/arquivo sem extensão entra em ByExt com chave vazia e o aviso diz sem extensão", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `LEIA`, `COPIA`, `Jogo.sfc`)
		res := ftScan(t, Folder(), root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"Jogo"})
		ftCheck(t, "warnings", res.Warnings, []string{"Pasta: 2 arquivos ignorados pela extensão (sem extensão ×2)"})
	})

	t.Run("IMV-018/aviso de profundidade com MaxDepth injetado de 2", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `a\Um.sfc`, `a\b\Dois.sfc`, `a\b\c\Fundo.sfc`)
		res := ftScan(t, Folder(), root, FolderOptions{MaxDepth: 2})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"Dois", "Um"})
		ftPrefixWarn(t, res.Warnings, "Pasta: subpastas além do nível")
	})

	t.Run("IMV-018/aviso de leitura interrompida com MaxItems injetado de 3", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `a1.sfc`, `a2.sfc`, `a3.sfc`, `a4.sfc`, `a5.sfc`)
		res := ftScan(t, Folder(), root, FolderOptions{MaxItems: 3})
		ftCheck(t, "titles", ftTitles(res.Candidates), []string{"a1", "a2", "a3"})
		ftPrefixWarn(t, res.Warnings, "Pasta: leitura interrompida em")
	})
}

func TestFolderLimitsAndCancel(t *testing.T) {
	t.Parallel()

	t.Run("IMV-018/cancelamento do ctx antes da leitura devolve erro de contexto", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `Jogo.sfc`)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := newFolderSource(osDirReader{}).Scan(ctx, ScanInput{Path: root})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Scan com ctx cancelado: err = %v, want context.Canceled", err)
		}
	})

	t.Run("IMV-018/cancelamento no meio da leitura (ao entrar numa subpasta) devolve erro de contexto", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		mkTree(t, root, `Sub\x.sfc`)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		spy := &ftSpy{inner: osDirReader{}, onCall: cancel}
		_, err := newFolderSource(spy).Scan(ctx, ScanInput{Path: root})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Scan cancelado no meio: err = %v, want context.Canceled", err)
		}
	})
}

func TestFolderNoWrites(t *testing.T) {
	t.Parallel()

	t.Run("IMV-080/nenhum Open: dirReader só tem ReadDir e toda leitura passa por ele", func(t *testing.T) {
		t.Parallel()
		if n := reflect.TypeOf((*dirReader)(nil)).Elem().NumMethod(); n != 1 {
			t.Fatalf("dirReader tem %d métodos, want 1 (só ReadDir)", n)
		}
		root := ftArvoreA(t)
		spy := &ftSpy{inner: osDirReader{}}
		res := ftScan(t, newFolderSource(spy), root, FolderOptions{})
		if len(res.Candidates) != 3 {
			t.Errorf("candidates = %q, want 3", ftTitles(res.Candidates))
		}
		if len(spy.calls) == 0 {
			t.Fatalf("o espião não recebeu nenhum ReadDir")
		}
		for _, name := range spy.calls {
			if !strings.HasPrefix(name, root) {
				t.Errorf("ReadDir de %q está fora da pasta escolhida %q", name, root)
			}
		}
	})

	t.Run("IMV-080/o scan não grava nada na pasta: árvore igual antes e depois", func(t *testing.T) {
		t.Parallel()
		root := ftArvoreA(t)
		mkTree(t, root, `Solto (USA).sfc`, `notas.txt`)
		before := ftSnapshot(t, root)
		ftScan(t, Folder(), root, FolderOptions{})
		ftScan(t, Folder(), root, FolderOptions{TitleFrom: TitleFromFolder, FirstLevelOnly: true})
		if after := ftSnapshot(t, root); after != before {
			t.Errorf("a árvore mudou depois do scan:\nantes:\n%s\ndepois:\n%s", before, after)
		}
	})
}

func TestClassify(t *testing.T) {
	t.Parallel()

	t.Run("IMV-013/classify: Windows mantém só executáveis e conta os de console", func(t *testing.T) {
		t.Parallel()
		files := []fileEntry{{Name: "a.exe"}, {Name: "setup.exe"}, {Name: "unins000.exe"}, {Name: "b.sfc"}, {Name: "c.zip"}, {Name: "d.txt"}}
		kept, ign := classify(files, KindExecutables)
		ftCheck(t, "mantidos", fileRels(kept), []string{"a.exe"})
		ftCheckIgn(t, ign, map[string]int{"exe": 2, "sfc": 1, "zip": 1, "txt": 1}, 2, 0)
	})

	t.Run("IMV-013/classify: console mantém só ROM e disco e conta executáveis como Exe", func(t *testing.T) {
		t.Parallel()
		files := []fileEntry{{Name: "a.sfc"}, {Name: "b.exe"}, {Name: "c.bat"}, {Name: "d.cmd"}, {Name: "e.cue"}}
		kept, ign := classify(files, KindConsole)
		ftCheck(t, "mantidos", fileRels(kept), []string{"a.sfc", "e.cue"})
		ftCheckIgn(t, ign, map[string]int{"exe": 1, "bat": 1, "cmd": 1}, 0, 3)
	})

	t.Run("IMV-013/classify: qualquer mantém as duas listas e não conta por plataforma", func(t *testing.T) {
		t.Parallel()
		files := []fileEntry{{Name: "a.exe"}, {Name: "setup.exe"}, {Name: "unins000.exe"}, {Name: "b.sfc"}, {Name: "notas.txt"}}
		kept, ign := classify(files, KindAny)
		ftCheck(t, "mantidos", fileRels(kept), []string{"a.exe", "b.sfc"})
		ftCheckIgn(t, ign, map[string]int{"exe": 2, "txt": 1}, 0, 0)
	})

	t.Run("IMV-013/classify: extensão em maiúsculas é candidata", func(t *testing.T) {
		t.Parallel()
		kept, ign := classify([]fileEntry{{Name: "GAME.ZIP"}}, KindAny)
		ftCheck(t, "mantidos", fileRels(kept), []string{"GAME.ZIP"})
		ftCheckIgn(t, ign, map[string]int{}, 0, 0)
	})

	t.Run("IMV-015/classify: setup.zip não é executável e fica; setup.exe sai na classe qualquer", func(t *testing.T) {
		t.Parallel()
		kept, _ := classify([]fileEntry{{Name: "setup.zip"}, {Name: "setup.exe"}}, KindAny)
		ftCheck(t, "mantidos", fileRels(kept), []string{"setup.zip"})
	})
}

// fileRels returns the relative paths of the files, in order.
func fileRels(files []fileEntry) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Rel())
	}
	return out
}

func TestToCandidates(t *testing.T) {
	t.Parallel()
	const root = `C:\Jogos`

	t.Run("IMV-016/launchType, launchTarget e installDir de arquivos da raiz e de subpastas", func(t *testing.T) {
		t.Parallel()
		entries := []groupEntry{
			{Main: fileEntry{Dir: `Jogo G`, Name: "Gama.cmd"}, Discs: 1},
			{Main: fileEntry{Dir: `Jogo B`, Name: "Beta.bat"}, Discs: 1},
			{Main: fileEntry{Name: "Alfa.exe"}, Discs: 1},
			{Main: fileEntry{Dir: `Jogo D`, Name: "Delta.sfc"}, Discs: 1},
		}
		cands := toCandidates(entries, root, FolderOptions{})
		ftCheckCands(t, root, cands, []ftWant{
			{title: "Alfa", file: `Alfa.exe`, ext: "exe", launch: LaunchExe},
			{title: "Beta", file: `Jogo B\Beta.bat`, ext: "bat", launch: LaunchExe},
			{title: "Delta", file: `Jogo D\Delta.sfc`, ext: "sfc", launch: LaunchEmulator},
			{title: "Gama", file: `Jogo G\Gama.cmd`, ext: "cmd", launch: LaunchExe},
		})
	})

	t.Run("IMV-016/launchType por extensão: exe, bat e cmd são exe; os demais emulator", func(t *testing.T) {
		t.Parallel()
		for ext, want := range map[string]string{
			"exe": LaunchExe, "bat": LaunchExe, "cmd": LaunchExe,
			"sfc": LaunchEmulator, "zip": LaunchEmulator, "cue": LaunchEmulator, "m3u": LaunchEmulator,
		} {
			cands := toCandidates([]groupEntry{{Main: fileEntry{Name: "Jogo." + ext}, Discs: 1}}, root, FolderOptions{})
			if len(cands) != 1 || cands[0].LaunchType != want {
				t.Errorf(".%s: candidates = %+v, want launchType %q", ext, cands, want)
			}
		}
	})

	t.Run("IMV-010/a saída segue a ordem natural de Main, não a de entrada", func(t *testing.T) {
		t.Parallel()
		entries := []groupEntry{
			{Main: fileEntry{Name: "Zeta.sfc"}, Discs: 1},
			{Main: fileEntry{Dir: "Alfa", Name: "Jogo 10.sfc"}, Discs: 1},
			{Main: fileEntry{Dir: "Alfa", Name: "Jogo 2.sfc"}, Discs: 1},
		}
		cands := toCandidates(entries, root, FolderOptions{})
		ftCheck(t, "titles", ftTitles(cands), []string{"Jogo 2", "Jogo 10", "Zeta"})
	})

	t.Run("IMV-024/discFiles vem em ordem de disco e só com discs >= 2; campos file e extension", func(t *testing.T) {
		t.Parallel()
		entries := []groupEntry{
			{Main: fileEntry{Dir: "Jogo", Name: "Jogo (Disc 1).cue"}, Discs: 2, DiscFiles: []string{`Jogo\Jogo (Disc 1).cue`, `Jogo\Jogo (Disc 2).cue`}},
			{Main: fileEntry{Name: "Solo (Disc 1).cue"}, Discs: 1},
		}
		cands := toCandidates(entries, root, FolderOptions{})
		by := ftByFile(cands)
		if c := by[`Jogo\Jogo (Disc 1).cue`]; c.Discs != 2 || !slices.Equal(c.DiscFiles, []string{`Jogo\Jogo (Disc 1).cue`, `Jogo\Jogo (Disc 2).cue`}) {
			t.Errorf("grupo = %+v, want discs 2 with discFiles in disc order", c)
		}
		if c := by["Solo (Disc 1).cue"]; c.Discs != 1 || c.DiscFiles != nil || c.Extension != "cue" {
			t.Errorf("disco único = %+v, want discs 1, no discFiles, extension cue", c)
		}
	})

	t.Run("IMV-017/titleFrom folder usa a subpasta de primeiro nível; raiz e pasta vazia usam o arquivo", func(t *testing.T) {
		t.Parallel()
		entries := []groupEntry{
			{Main: fileEntry{Dir: `04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 1)`, Name: "Final Fantasy VIII (USA) (Disc 1).cue"}, Discs: 2,
				DiscFiles: []string{`04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 1)\Final Fantasy VIII (USA) (Disc 1).cue`, `04. Final Fantasy VIII (USA)\Final Fantasy VIII (USA) (Disc 2)\Final Fantasy VIII (USA) (Disc 2).cue`}},
			{Main: fileEntry{Name: "Solto (USA).sfc"}, Discs: 1},
			{Main: fileEntry{Dir: "(USA)", Name: "Jogo Real.sfc"}, Discs: 1},
		}
		cands := toCandidates(entries, root, FolderOptions{TitleFrom: TitleFromFolder})
		by := ftByFile(cands)
		if c, ok := by[ftFFDisc1]; !ok || c.Title != "Final Fantasy VIII" {
			t.Errorf("título pela pasta = %+v, want %q", c, "Final Fantasy VIII")
		}
		if c := by["Solto (USA).sfc"]; c.Title != "Solto" {
			t.Errorf("arquivo da raiz = %q, want %q", c.Title, "Solto")
		}
		if c := by[`(USA)\Jogo Real.sfc`]; c.Title != "(USA)" {
			t.Errorf("pasta só com marcações = %q, want %q (kept, no fallback to the file)", c.Title, "(USA)")
		}
	})

	t.Run("IMV-007/título é o único campo normalizado: file e launchTarget com as marcações", func(t *testing.T) {
		t.Parallel()
		cands := toCandidates([]groupEntry{{Main: fileEntry{Dir: "Jogos", Name: "Super_Mario (USA) [!].sfc"}, Discs: 1}}, root, FolderOptions{})
		ftCheckCands(t, root, cands, []ftWant{
			{title: "Super Mario", file: `Jogos\Super_Mario (USA) [!].sfc`, ext: "sfc", launch: LaunchEmulator},
		})
	})

	t.Run("IMV-024/candidato de pasta sai com Source folder", func(t *testing.T) {
		t.Parallel()
		cands := toCandidates([]groupEntry{{Main: fileEntry{Name: "Jogo.m3u"}, Discs: 1}}, root, FolderOptions{})
		if len(cands) != 1 || cands[0].Source != SourceFolder || cands[0].Title != "Jogo" || cands[0].Extension != "m3u" {
			t.Errorf("candidates = %+v, want folder source, title Jogo, extension m3u", cands)
		}
	})
}

func TestBuildWarnings(t *testing.T) {
	t.Parallel()

	t.Run("IMV-014/aviso com total e as 5 extensões mais frequentes, empate por ordem alfabética", func(t *testing.T) {
		t.Parallel()
		ign := ignoredCounts{ByExt: map[string]int{"png": 3, "sav": 2, "txt": 2, "dll": 1, "ini": 1, "nfo": 1}}
		got := buildWarnings(ign, walkResult{}, 1, FolderOptions{})
		ftCheck(t, "warnings", got, []string{"Pasta: 10 arquivos ignorados pela extensão (.png ×3, .sav ×2, .txt ×2, .dll ×1, .ini ×1)"})
	})

	t.Run("IMV-014/com candidatos não há aviso de plataforma trocada", func(t *testing.T) {
		t.Parallel()
		ign := ignoredCounts{ByExt: map[string]int{"sfc": 2}, Console: 2}
		got := buildWarnings(ign, walkResult{}, 1, FolderOptions{Kind: KindExecutables})
		ftCheck(t, "warnings", got, []string{"Pasta: 2 arquivos ignorados pela extensão (.sfc ×2)"})
	})

	t.Run("IMV-014/nenhum candidato com ROMs numa leitura Windows: aviso de plataforma com o total de console", func(t *testing.T) {
		t.Parallel()
		ign := ignoredCounts{ByExt: map[string]int{"sfc": 2}, Console: 2}
		got := buildWarnings(ign, walkResult{}, 0, FolderOptions{Kind: KindExecutables})
		if len(got) < 2 || got[0] != "Pasta: 2 arquivos ignorados pela extensão (.sfc ×2)" ||
			!slices.Contains(got, "Pasta: a plataforma escolhida lê só executáveis; 2 arquivos de jogo de console foram ignorados") {
			t.Errorf("warnings = %q, want the extension warning first and the Windows wrong-platform warning", got)
		}
	})

	t.Run("IMV-014/nenhum candidato com executáveis numa leitura de console: aviso de plataforma com o total de exe", func(t *testing.T) {
		t.Parallel()
		ign := ignoredCounts{ByExt: map[string]int{"exe": 3}, Exe: 3}
		got := buildWarnings(ign, walkResult{}, 0, FolderOptions{Kind: KindConsole})
		if len(got) < 2 || got[0] != "Pasta: 3 arquivos ignorados pela extensão (.exe ×3)" ||
			!slices.Contains(got, "Pasta: a plataforma escolhida lê só arquivos de jogo de console; 3 executáveis foram ignorados") {
			t.Errorf("warnings = %q, want the extension warning first and the console wrong-platform warning", got)
		}
	})

	t.Run("IMV-018/ordem: ignorados, plataforma, profundidade, itens, ilegíveis", func(t *testing.T) {
		t.Parallel()
		ign := ignoredCounts{ByExt: map[string]int{"txt": 2}}
		w := walkResult{DepthCut: true, ItemsCut: true, Unreadable: 2}
		got := buildWarnings(ign, w, 3, FolderOptions{Kind: KindAny})
		ftCheck(t, "warnings", got, []string{
			"Pasta: 2 arquivos ignorados pela extensão (.txt ×2)",
			"Pasta: subpastas além do nível 8 não foram lidas",
			"Pasta: leitura interrompida em 20000 itens; escolha uma subpasta",
			"Pasta: 2 subpastas ilegíveis",
		})
	})

	t.Run("IMV-010/nenhum jogo encontrado sem outros motivos", func(t *testing.T) {
		t.Parallel()
		ftCheck(t, "warnings", buildWarnings(ignoredCounts{}, walkResult{}, 0, FolderOptions{}), []string{"Pasta: nenhum jogo encontrado"})
	})

	t.Run("IMV-011/nenhum jogo no primeiro nível com FirstLevelOnly", func(t *testing.T) {
		t.Parallel()
		ftCheck(t, "warnings", buildWarnings(ignoredCounts{}, walkResult{}, 0, FolderOptions{FirstLevelOnly: true}), []string{"Pasta: nenhum jogo no primeiro nível"})
	})

	t.Run("IMV-014/sem ignorados e com candidatos não há avisos", func(t *testing.T) {
		t.Parallel()
		ftCheck(t, "warnings", buildWarnings(ignoredCounts{}, walkResult{}, 2, FolderOptions{}), nil)
	})

	t.Run("IMV-014/arquivo sem extensão aparece como sem extensão ×k no aviso", func(t *testing.T) {
		t.Parallel()
		ign := ignoredCounts{ByExt: map[string]int{"": 2, "txt": 1}}
		ftCheck(t, "warnings", buildWarnings(ign, walkResult{}, 1, FolderOptions{}),
			[]string{"Pasta: 3 arquivos ignorados pela extensão (sem extensão ×2, .txt ×1)"})
	})

	t.Run("IMV-018/limites injetados: aviso de profundidade e de itens com o prefixo fixo", func(t *testing.T) {
		t.Parallel()
		ftPrefixWarn(t, buildWarnings(ignoredCounts{}, walkResult{DepthCut: true}, 1, FolderOptions{MaxDepth: 2}), "Pasta: subpastas além do nível")
		ftPrefixWarn(t, buildWarnings(ignoredCounts{}, walkResult{ItemsCut: true}, 1, FolderOptions{MaxItems: 3}), "Pasta: leitura interrompida em")
	})
}
