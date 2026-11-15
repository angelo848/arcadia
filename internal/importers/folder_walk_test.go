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

// Tests of the recursive walk of the folder source (phase 11, T06): IMV-010, IMV-011, IMV-012, IMV-018, IMV-080.
// White-box, because walk, fileEntry and dirReader are unexported. The fake dirReader keeps a tree in memory under
// fakeRoot and has only ReadDir; the real trees live in t.TempDir().
//
// Premises for the implementer (see also the report of T06):
//   - walk calls dirs.ReadDir with filepath.Join(root, <relative folder>), so the fake keys match.
//   - walk decides what to skip with skipEntry(e.Name(), e.Type()), never with e.IsDir() alone: on Windows a
//     directory symlink reports ModeDir|ModeSymlink.
//   - walk lists every regular file it sees (.bin, .txt, ...); the extension filter is T08's job, not walk's.
//   - an item is a directory entry that is neither skipped nor a symlink/junction; ItemsCut is set only when an item
//     beyond MaxItems exists ("mais de 20.000 itens").

const fakeRoot = `C:\Jogos`

var errBoom = errors.New("leitura negada (teste)")

// fakeEntry is a directory entry of the fake tree. Only the name and the type bits matter to the walk.
type fakeEntry struct {
	name string
	mode fs.FileMode
}

func (e fakeEntry) Name() string      { return e.name }
func (e fakeEntry) IsDir() bool       { return e.mode.IsDir() }
func (e fakeEntry) Type() fs.FileMode { return e.mode.Type() }
func (e fakeEntry) Info() (fs.FileInfo, error) {
	return nil, errors.New("Info não é usado pela leitura")
}

func fileEnt(name string) fs.DirEntry { return fakeEntry{name: name} }
func dirEnt(name string) fs.DirEntry  { return fakeEntry{name: name, mode: fs.ModeDir} }
func linkEnt(name string) fs.DirEntry { return fakeEntry{name: name, mode: fs.ModeSymlink} }
func linkDirEnt(name string) fs.DirEntry {
	return fakeEntry{name: name, mode: fs.ModeDir | fs.ModeSymlink}
}
func irregEnt(name string) fs.DirEntry { return fakeEntry{name: name, mode: fs.ModeIrregular} }

// fakeDirs is an in-memory dirReader with only ReadDir. It records every call, injects failures per folder and runs
// hooks by 1-based call number (to cancel a context in the middle of the walk).
type fakeDirs struct {
	entries  map[string][]fs.DirEntry
	failures map[string]error
	hooks    map[int]func()
	calls    []string
}

func newFakeDirs() *fakeDirs {
	return &fakeDirs{
		entries:  map[string][]fs.DirEntry{},
		failures: map[string]error{},
		hooks:    map[int]func(){},
	}
}

// dir sets the entries of the folder rel, relative to fakeRoot with "\" separators ("" is the root).
func (f *fakeDirs) dir(rel string, entries ...fs.DirEntry) *fakeDirs {
	f.entries[filepath.Join(fakeRoot, rel)] = entries
	return f
}

// fail makes ReadDir of the folder rel return err.
func (f *fakeDirs) fail(rel string, err error) *fakeDirs {
	f.failures[filepath.Join(fakeRoot, rel)] = err
	return f
}

// ReadDir implements dirReader. Folders that were not set up return fs.ErrNotExist.
func (f *fakeDirs) ReadDir(name string) ([]fs.DirEntry, error) {
	f.calls = append(f.calls, name)
	if hook, ok := f.hooks[len(f.calls)]; ok {
		hook()
	}
	key := filepath.Clean(name)
	if err, ok := f.failures[key]; ok {
		return nil, err
	}
	if ents, ok := f.entries[key]; ok {
		return ents, nil
	}
	return nil, fs.ErrNotExist
}

// readDirOnly wraps a dirReader and exposes nothing but ReadDir [IMV-080].
type readDirOnly struct{ inner dirReader }

func (r readDirOnly) ReadDir(name string) ([]fs.DirEntry, error) { return r.inner.ReadDir(name) }

// ignoredFixture is a folder with every kind of ignored entry next to two games [IMV-012].
func ignoredFixture() *fakeDirs {
	return newFakeDirs().
		dir("",
			dirEnt(".git"), dirEnt("$RECYCLE.BIN"), dirEnt("System Volume Information"), dirEnt("@eaDir"),
			dirEnt("__MACOSX"), dirEnt("Node_Modules"), fileEnt(".hidden.exe"), fileEnt("$x.exe"),
			fileEnt("b.exe"), fileEnt("a.exe")).
		dir(".git", fileEnt("h.exe")).
		dir("$RECYCLE.BIN", fileEnt("r.exe")).
		dir("System Volume Information", fileEnt("s.exe")).
		dir("@eaDir", fileEnt("t.exe")).
		dir("__MACOSX", fileEnt("m.exe")).
		dir("Node_Modules", fileEnt("n.exe"))
}

// bigFlatFixture is a root with n files named f00001.exe, f00002.exe, ...
func bigFlatFixture(n int) *fakeDirs {
	ents := make([]fs.DirEntry, 0, n)
	for i := 1; i <= n; i++ {
		ents = append(ents, fileEnt(fmt.Sprintf("f%05d.exe", i)))
	}
	return newFakeDirs().dir("", ents...)
}

// chainParts returns n nested folder names d1..dn.
func chainParts(n int) []string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("d%d", i+1)
	}
	return parts
}

func walkFake(f *fakeDirs, opts FolderOptions) (walkResult, error) {
	return walk(context.Background(), f, fakeRoot, opts)
}

func walkReal(t *testing.T, root string, opts FolderOptions) walkResult {
	t.Helper()
	res, err := walk(context.Background(), osDirReader{}, root, opts)
	if err != nil {
		t.Fatalf("walk: erro inesperado: %v", err)
	}
	return res
}

// makeTree creates empty files; slash-separated relative paths.
func makeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
}

func rels(files []fileEntry) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Rel())
	}
	return out
}

func checkRels(t *testing.T, got []fileEntry, want ...string) {
	t.Helper()
	if g := rels(got); !slices.Equal(g, want) {
		t.Errorf("arquivos = %q, quer %q", g, want)
	}
}

// checkCalls compares the folders the walk opened, relative to fakeRoot ("." is the root), in call order.
func checkCalls(t *testing.T, f *fakeDirs, want ...string) {
	t.Helper()
	got := make([]string, 0, len(f.calls))
	for _, c := range f.calls {
		r, err := filepath.Rel(fakeRoot, c)
		if err != nil {
			t.Fatalf("filepath.Rel(%q): %v", c, err)
		}
		got = append(got, r)
	}
	if !slices.Equal(got, want) {
		t.Errorf("pastas lidas = %q, quer %q", got, want)
	}
}

func TestWalkRecursive(t *testing.T) {
	t.Parallel()

	t.Run("IMV-010/soltos e subpastas: os dois layouts do usuário", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		makeTree(t, root,
			"Banjo-Kazooie (U) [!].zip",
			"31. Spyro (USA)/Spyro (USA).cue",
			"31. Spyro (USA)/Spyro (USA).bin",
		)
		res := walkReal(t, root, FolderOptions{})
		checkRels(t, res.Files,
			`31. Spyro (USA)\Spyro (USA).bin`,
			`31. Spyro (USA)\Spyro (USA).cue`,
			`Banjo-Kazooie (U) [!].zip`,
		)
		if res.DepthCut || res.ItemsCut || res.Unreadable != 0 {
			t.Errorf("avisos = %+v, quer nenhum", res)
		}
	})

	t.Run("IMV-010/ordem natural: Jogo 2 antes de Jogo 10 e pasta/arquivo no mesmo critério", func(t *testing.T) {
		t.Parallel()
		// The entries are given out of order on purpose: the walk must sort them.
		f := newFakeDirs().
			dir("", fileEnt("Jogo 10.exe"), fileEnt("Jogo 2.exe"), dirEnt("Jogo 3")).
			dir("Jogo 3", fileEnt("x.exe"))
		res, err := walkFake(f, FolderOptions{})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		checkRels(t, res.Files, "Jogo 2.exe", `Jogo 3\x.exe`, "Jogo 10.exe")
	})

	t.Run("IMV-010/profundidade 8 é lida e 9 não", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		makeTree(t, root,
			strings.Join(chainParts(8), "/")+"/deep.exe",
			strings.Join(chainParts(9), "/")+"/far.exe",
		)
		res := walkReal(t, root, FolderOptions{})
		checkRels(t, res.Files, strings.Join(chainParts(8), `\`)+`\deep.exe`)
		if !res.DepthCut {
			t.Errorf("DepthCut = false, quer true: a pasta do nível 9 não deveria ser lida")
		}
	})

	t.Run("IMV-010/profundidade 8 sem nível 9: a leitura completa e sem aviso de profundidade", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		makeTree(t, root, strings.Join(chainParts(8), "/")+"/deep.exe")
		res := walkReal(t, root, FolderOptions{})
		checkRels(t, res.Files, strings.Join(chainParts(8), `\`)+`\deep.exe`)
		if res.DepthCut {
			t.Errorf("DepthCut = true, quer false")
		}
	})
}

func TestWalkFirstLevelOnly(t *testing.T) {
	t.Parallel()

	t.Run("IMV-011/FirstLevelOnly lê só os arquivos da raiz e a mesma árvore sem a opção lê tudo", func(t *testing.T) {
		t.Parallel()
		tree := func() *fakeDirs {
			return newFakeDirs().
				dir("", fileEnt("a.exe"), dirEnt("sub")).
				dir("sub", fileEnt("b.exe"), dirEnt("deep")).
				dir(`sub\deep`, fileEnt("c.exe"))
		}

		only := tree()
		res, err := walkFake(only, FolderOptions{FirstLevelOnly: true})
		if err != nil {
			t.Fatalf("walk FirstLevelOnly: %v", err)
		}
		checkRels(t, res.Files, "a.exe")
		checkCalls(t, only, ".")
		if res.DepthCut {
			t.Errorf("DepthCut = true com FirstLevelOnly, quer false (sem aviso de profundidade)")
		}

		all := tree()
		res, err = walkFake(all, FolderOptions{})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		checkRels(t, res.Files, "a.exe", `sub\b.exe`, `sub\deep\c.exe`)
	})

	t.Run("IMV-011/mesma árvore real: FirstLevelOnly liga e desliga", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		makeTree(t, root, "a.exe", "sub/b.exe", "sub/deep/c.exe")
		checkRels(t, walkReal(t, root, FolderOptions{FirstLevelOnly: true}).Files, "a.exe")
		checkRels(t, walkReal(t, root, FolderOptions{}).Files, "a.exe", `sub\b.exe`, `sub\deep\c.exe`)
	})
}

func TestWalkIgnoredItems(t *testing.T) {
	t.Parallel()

	t.Run("IMV-012/nomes ignorados e o que há dentro deles não são lidos nem contados", func(t *testing.T) {
		t.Parallel()
		f := ignoredFixture()
		res, err := walkFake(f, FolderOptions{})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		checkRels(t, res.Files, "a.exe", "b.exe")
		checkCalls(t, f, ".")
		if res.ItemsCut {
			t.Errorf("ItemsCut = true, quer false")
		}
	})

	t.Run("IMV-012/itens ignorados não gastam o limite de itens", func(t *testing.T) {
		t.Parallel()
		// Only a.exe and b.exe count: with MaxItems 2 nothing is cut.
		res, err := walkFake(ignoredFixture(), FolderOptions{MaxItems: 2})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		checkRels(t, res.Files, "a.exe", "b.exe")
		if res.ItemsCut {
			t.Errorf("ItemsCut = true com 2 itens e MaxItems 2, quer false")
		}
	})

	t.Run("IMV-012/subpasta ilegível: segue nas demais e Unreadable conta", func(t *testing.T) {
		t.Parallel()
		f := newFakeDirs().
			dir("", dirEnt("bad"), dirEnt("ok1"), dirEnt("ok2"), fileEnt("root.exe")).
			dir("ok1", fileEnt("a.exe")).
			dir("ok2", fileEnt("b.exe")).
			fail("bad", errBoom)
		res, err := walkFake(f, FolderOptions{})
		if err != nil {
			t.Fatalf("walk com uma subpasta ilegível deve seguir: %v", err)
		}
		checkRels(t, res.Files, `ok1\a.exe`, `ok2\b.exe`, "root.exe")
		if res.Unreadable != 1 {
			t.Errorf("Unreadable = %d, quer 1", res.Unreadable)
		}

		two := newFakeDirs().
			dir("", dirEnt("bad1"), dirEnt("bad2"), fileEnt("root.exe")).
			fail("bad1", errBoom).
			fail("bad2", errBoom)
		res, err = walkFake(two, FolderOptions{})
		if err != nil {
			t.Fatalf("walk com duas subpastas ilegíveis deve seguir: %v", err)
		}
		checkRels(t, res.Files, "root.exe")
		if res.Unreadable != 2 {
			t.Errorf("Unreadable = %d, quer 2", res.Unreadable)
		}
	})

	t.Run("IMV-012/ModeSymlink e ModeIrregular por dirReader falso", func(t *testing.T) {
		t.Parallel()
		// linkdir mirrors what Windows reports for a directory symlink: ModeDir|ModeSymlink.
		f := newFakeDirs().
			dir("",
				linkEnt("alias.exe"), linkDirEnt("linkdir"), irregEnt("pipe"), fileEnt("ok.exe")).
			dir("linkdir", fileEnt("hidden.exe"))
		res, err := walkFake(f, FolderOptions{})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		checkRels(t, res.Files, "ok.exe")
		checkCalls(t, f, ".")
	})

	t.Run("IMV-012/link simbólico real", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		makeTree(t, root, "real/game.exe")
		if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
			t.Skipf("sem permissão para criar link simbólico de pasta: %v", err)
		}
		if err := os.Symlink(filepath.Join(root, "real", "game.exe"), filepath.Join(root, "alias.exe")); err != nil {
			t.Skipf("sem permissão para criar link simbólico de arquivo: %v", err)
		}
		res := walkReal(t, root, FolderOptions{})
		checkRels(t, res.Files, `real\game.exe`)
	})
}

func TestWalkLimits(t *testing.T) {
	t.Parallel()

	t.Run("IMV-018/DepthCut com MaxDepth injetado de 2", func(t *testing.T) {
		t.Parallel()
		f := newFakeDirs().
			dir("", fileEnt("top.exe"), dirEnt("a")).
			dir("a", dirEnt("b")).
			dir(`a\b`, fileEnt("y.exe"), dirEnt("c")).
			dir(`a\b\c`, fileEnt("z.exe"))
		res, err := walkFake(f, FolderOptions{MaxDepth: 2})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		checkRels(t, res.Files, `a\b\y.exe`, "top.exe")
		if !res.DepthCut {
			t.Errorf("DepthCut = false, quer true (a pasta a\\b\\c está no nível 3)")
		}
		checkCalls(t, f, ".", "a", `a\b`)
	})

	t.Run("IMV-018/DepthCut falso quando a árvore não passa de MaxDepth", func(t *testing.T) {
		t.Parallel()
		f := newFakeDirs().
			dir("", dirEnt("a")).
			dir("a", fileEnt("x.exe"), dirEnt("b")).
			dir(`a\b`, fileEnt("y.exe"))
		res, err := walkFake(f, FolderOptions{MaxDepth: 2})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		checkRels(t, res.Files, `a\b\y.exe`, `a\x.exe`)
		if res.DepthCut {
			t.Errorf("DepthCut = true, quer false")
		}
	})

	t.Run("IMV-018/ItemsCut para na ordem natural do item MaxItems e devolve o que achou", func(t *testing.T) {
		t.Parallel()
		res, err := walkFake(bigFlatFixture(6), FolderOptions{MaxItems: 5})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		checkRels(t, res.Files, "f00001.exe", "f00002.exe", "f00003.exe", "f00004.exe", "f00005.exe")
		if !res.ItemsCut {
			t.Errorf("ItemsCut = false com 6 itens e MaxItems 5, quer true")
		}
	})

	t.Run("IMV-018/ItemsCut conta pastas em ordem de visita: a pasta conta antes do que há dentro", func(t *testing.T) {
		t.Parallel()
		// Visit order: a (item 1), a\x.exe (item 2), b.exe (item 3, beyond MaxItems 2).
		f := newFakeDirs().
			dir("", dirEnt("a"), fileEnt("b.exe")).
			dir("a", fileEnt("x.exe"))
		res, err := walkFake(f, FolderOptions{MaxItems: 2})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		checkRels(t, res.Files, `a\x.exe`)
		if !res.ItemsCut {
			t.Errorf("ItemsCut = false, quer true")
		}
	})

	t.Run("IMV-018/ItemsCut falso com exatamente MaxItems itens", func(t *testing.T) {
		t.Parallel()
		res, err := walkFake(bigFlatFixture(6), FolderOptions{MaxItems: 6})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		if len(res.Files) != 6 || res.ItemsCut {
			t.Errorf("arquivos = %d, ItemsCut = %v; quer 6 e false", len(res.Files), res.ItemsCut)
		}
	})

	t.Run("IMV-018/limites padrão: 20.000 itens e 8 níveis (MaxItems e MaxDepth zero)", func(t *testing.T) {
		t.Parallel()
		over, err := walkFake(bigFlatFixture(MaxScanItems+1), FolderOptions{})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		if len(over.Files) != MaxScanItems || !over.ItemsCut {
			t.Errorf("com %d itens: arquivos = %d, ItemsCut = %v; quer %d e true",
				MaxScanItems+1, len(over.Files), over.ItemsCut, MaxScanItems)
		}

		exact, err := walkFake(bigFlatFixture(MaxScanItems), FolderOptions{})
		if err != nil {
			t.Fatalf("walk: %v", err)
		}
		if len(exact.Files) != MaxScanItems || exact.ItemsCut {
			t.Errorf("com %d itens: arquivos = %d, ItemsCut = %v; quer %d e false",
				MaxScanItems, len(exact.Files), exact.ItemsCut, MaxScanItems)
		}
	})

	t.Run("IMV-018/cancelamento do ctx no meio devolve erro de contexto", func(t *testing.T) {
		t.Parallel()
		f := newFakeDirs().
			dir("", dirEnt("a"), fileEnt("x.exe")).
			dir("a", fileEnt("y.exe"))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		// Cancel while the root is being read: entering "a" must notice it.
		f.hooks[1] = cancel
		_, err := walk(ctx, f, fakeRoot, FolderOptions{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("erro = %v, quer context.Canceled", err)
		}
		checkCalls(t, f, ".")
	})

	t.Run("IMV-018/ctx já cancelado devolve erro de contexto", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := walk(ctx, newFakeDirs().dir("", fileEnt("a.exe")), fakeRoot, FolderOptions{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("erro = %v, quer context.Canceled", err)
		}
	})

	t.Run("IMV-018/a raiz com erro de ReadDir é erro", func(t *testing.T) {
		t.Parallel()
		_, err := walkFake(newFakeDirs().fail("", errBoom), FolderOptions{})
		if !errors.Is(err, errBoom) {
			t.Fatalf("erro = %v, quer o erro de ReadDir da raiz", err)
		}
		_, err = walkFake(newFakeDirs(), FolderOptions{})
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("raiz inexistente: erro = %v, quer fs.ErrNotExist", err)
		}
	})
}

func TestWalkReadsNamesOnly(t *testing.T) {
	t.Parallel()

	t.Run("IMV-080/dirReader espião só implementa ReadDir (nenhum Open) e a leitura completa passa", func(t *testing.T) {
		t.Parallel()
		f := newFakeDirs().
			dir("", fileEnt("a.exe"), dirEnt("sub")).
			dir("sub", fileEnt("b.exe"), dirEnt("deep")).
			dir(`sub\deep`, fileEnt("c.exe"))
		spy := readDirOnly{inner: f}

		rt := reflect.TypeOf(spy)
		if rt.NumMethod() != 1 || rt.Method(0).Name != "ReadDir" {
			t.Fatalf("o espião tem %d métodos exportados, quer só ReadDir", rt.NumMethod())
		}
		if _, ok := any(spy).(interface {
			Open(name string) (fs.File, error)
		}); ok {
			t.Fatalf("o espião implementa Open")
		}

		res, err := walk(context.Background(), spy, fakeRoot, FolderOptions{})
		if err != nil {
			t.Fatalf("walk com espião: %v", err)
		}
		checkRels(t, res.Files, "a.exe", `sub\b.exe`, `sub\deep\c.exe`)
		checkCalls(t, f, ".", "sub", `sub\deep`)
	})
}

func TestFileEntryHelpers(t *testing.T) {
	t.Parallel()

	t.Run("IMV-010/Rel junta pasta e nome com \\ e na raiz é só o nome", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			e    fileEntry
			want string
		}{
			{"subpasta", fileEntry{Dir: `04. Final Fantasy VIII (USA)\Disc 1`, Name: "ff8.bin"}, `04. Final Fantasy VIII (USA)\Disc 1\ff8.bin`},
			{"raiz", fileEntry{Dir: "", Name: "Jogo 2.exe"}, "Jogo 2.exe"},
		}
		for _, c := range cases {
			if got := c.e.Rel(); got != c.want {
				t.Errorf("%s: Rel() = %q, quer %q", c.name, got, c.want)
			}
		}
	})

	t.Run("IMV-021/Scope é o primeiro segmento da pasta; arquivo da raiz tem escopo vazio", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			e    fileEntry
			want string
		}{
			{fileEntry{Dir: `04. Final Fantasy VIII (USA)\Disc 1`, Name: "ff8.bin"}, "04. Final Fantasy VIII (USA)"},
			{fileEntry{Dir: "Solo", Name: "x.iso"}, "Solo"},
			{fileEntry{Dir: "", Name: "a.exe"}, ""},
		}
		for _, c := range cases {
			if got := c.e.Scope(); got != c.want {
				t.Errorf("Scope() de %q = %q, quer %q", c.e.Rel(), got, c.want)
			}
		}
	})

	t.Run("IMV-001/Stem e Ext seguem SplitExt", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name, stem, ext string
		}{
			{"ff8 (Disc 1).bin", "ff8 (Disc 1)", "bin"},
			{"Jogo.EXE", "Jogo", "exe"},
			{"Sem extensão", "Sem extensão", ""},
			{"a.verylongext", "a.verylongext", ""},
		}
		for _, c := range cases {
			e := fileEntry{Dir: "pasta", Name: c.name}
			if got := e.Stem(); got != c.stem {
				t.Errorf("Stem() de %q = %q, quer %q", c.name, got, c.stem)
			}
			if got := e.Ext(); got != c.ext {
				t.Errorf("Ext() de %q = %q, quer %q", c.name, got, c.ext)
			}
		}
	})
}
