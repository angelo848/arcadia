package importers

import (
	"fmt"
	"strings"
	"testing"
)

// grpFile builds a fileEntry from a path relative to the chosen folder ("\" separators).
// A path without "\" is a root file (Dir "").
func grpFile(rel string) fileEntry {
	i := strings.LastIndex(rel, `\`)
	if i < 0 {
		return fileEntry{Name: rel}
	}
	return fileEntry{Dir: rel[:i], Name: rel[i+1:]}
}

func grpFiles(rels []string) []fileEntry {
	out := make([]fileEntry, 0, len(rels))
	for _, r := range rels {
		out = append(out, grpFile(r))
	}
	return out
}

func grpRels(es []fileEntry) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.Rel())
	}
	return out
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// grpDisc builds the Árvore A layout: <scope>\<base (Disc N)>\<base (Disc N)>.cue.
func grpDisc(scope, base string, disc int) string {
	d := fmt.Sprintf("%s (Disc %d)", base, disc)
	return scope + `\` + d + `\` + d + ".cue"
}

// grpWant is the expected shape of one groupEntry.
type grpWant struct {
	main  string
	ext   string
	discs int
	files []string
}

func checkGroups(t *testing.T, got []groupEntry, want []grpWant) {
	t.Helper()
	if len(got) != len(want) {
		mains := make([]string, 0, len(got))
		for _, g := range got {
			mains = append(mains, g.Main.Rel())
		}
		t.Fatalf("grupos = %d, quer %d (principais obtidos: %q)", len(got), len(want), mains)
	}
	for i, w := range want {
		g := got[i]
		if g.Main.Rel() != w.main {
			t.Errorf("grupo %d: principal = %q, quer %q", i, g.Main.Rel(), w.main)
		}
		if g.Main.Ext() != w.ext {
			t.Errorf("grupo %d: extensão = %q, quer %q", i, g.Main.Ext(), w.ext)
		}
		if g.Discs != w.discs {
			t.Errorf("grupo %d: discs = %d, quer %d", i, g.Discs, w.discs)
		}
		if !sameStrings(g.DiscFiles, w.files) {
			t.Errorf("grupo %d: discFiles = %q, quer %q", i, g.DiscFiles, w.files)
		}
	}
}

func TestDropCompanions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "IMV-020/cue + bin: o bin some",
			in:   []string{`Jogo (USA).cue`, `Jogo (USA).bin`},
			want: []string{`Jogo (USA).cue`},
		},
		{
			name: "IMV-020/cue com várias trilhas (Track 01, Track 02)",
			in: []string{
				`Jogo (USA).cue`,
				`Jogo (USA) (Track 01).bin`,
				`Jogo (USA) (Track 02).bin`,
				`Jogo (USA) (Track 02).sub`,
			},
			want: []string{`Jogo (USA).cue`},
		},
		{
			name: "IMV-020/bin solto continua candidato",
			in:   []string{`Jogo.cue`, `Outro.bin`},
			want: []string{`Jogo.cue`, `Outro.bin`},
		},
		{
			name: "IMV-020/ccd com img e sub",
			in:   []string{`Jogo.ccd`, `Jogo.img`, `Jogo.sub`},
			want: []string{`Jogo.ccd`},
		},
		{
			name: "IMV-020/mds com mdf",
			in:   []string{`Jogo.mds`, `Jogo.mdf`},
			want: []string{`Jogo.mds`},
		},
		{
			name: "IMV-020/gdi com bin e raw",
			in:   []string{`Jogo.gdi`, `Jogo.bin`, `Jogo.raw`},
			want: []string{`Jogo.gdi`},
		},
		{
			name: "IMV-020/mesma chave em outra pasta não é acompanhante",
			in:   []string{`A\Jogo.cue`, `B\Jogo.bin`},
			want: []string{`A\Jogo.cue`, `B\Jogo.bin`},
		},
		{
			name: "IMV-020/extensão que o contêiner não lista continua candidata",
			in:   []string{`Jogo.ccd`, `Jogo.bin`},
			want: []string{`Jogo.ccd`, `Jogo.bin`},
		},
		{
			name: "IMV-020/lista vazia devolve vazia",
			in:   nil,
			want: nil,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := grpRels(dropCompanions(grpFiles(tc.in)))
			if !sameStrings(got, tc.want) {
				t.Errorf("dropCompanions = %q, quer %q", got, tc.want)
			}
		})
	}
}

func TestGroupDiscs(t *testing.T) {
	t.Parallel()

	ffScope := `04. Final Fantasy VIII (USA)`
	reScope := `07. Resident Evil 2 (USA)`
	spScope := `31. Spyro - Year of the Dragon (USA)`
	ffBase := `Final Fantasy VIII (USA)`
	reBase := `Resident Evil 2 - Dual Shock Ver. (USA)`

	cases := []struct {
		name string
		in   []string
		want []grpWant
	}{
		{
			name: "IMV-021/árvore A: Final Fantasy VIII e Resident Evil 2 viram um candidato cada com discs 2",
			in: []string{
				grpDisc(ffScope, ffBase, 1),
				grpDisc(ffScope, ffBase, 2),
				grpDisc(reScope, reBase, 1),
				grpDisc(reScope, reBase, 2),
				spScope + `\Spyro - Year of the Dragon (USA).cue`,
			},
			want: []grpWant{
				{
					main: grpDisc(ffScope, ffBase, 1), ext: "cue", discs: 2,
					files: []string{grpDisc(ffScope, ffBase, 1), grpDisc(ffScope, ffBase, 2)},
				},
				{
					main: grpDisc(reScope, reBase, 1), ext: "cue", discs: 2,
					files: []string{grpDisc(reScope, reBase, 1), grpDisc(reScope, reBase, 2)},
				},
				{
					main: spScope + `\Spyro - Year of the Dragon (USA).cue`, ext: "cue", discs: 1,
				},
			},
		},
		{
			name: "IMV-021/discos soltos na raiz",
			in:   []string{`Jogo (Disc 1).cue`, `Jogo (Disc 2).cue`},
			want: []grpWant{
				{main: `Jogo (Disc 1).cue`, ext: "cue", discs: 2, files: []string{`Jogo (Disc 1).cue`, `Jogo (Disc 2).cue`}},
			},
		},
		{
			name: "IMV-021/escopos diferentes não se juntam",
			in:   []string{`A\Jogo (Disc 1).cue`, `B\Jogo (Disc 2).cue`},
			want: []grpWant{
				{main: `A\Jogo (Disc 1).cue`, ext: "cue", discs: 1},
				{main: `B\Jogo (Disc 2).cue`, ext: "cue", discs: 1},
			},
		},
		{
			name: "IMV-021/disco único é candidato comum com discs 1",
			in:   []string{`Jogo (Disc 1).cue`},
			want: []grpWant{
				{main: `Jogo (Disc 1).cue`, ext: "cue", discs: 1},
			},
		},
		{
			name: "IMV-021/menor número de disco é o principal e discFiles em ordem de disco",
			in:   []string{`Jogo (Disc 2).cue`, `Jogo (Disc 1).cue`, `Jogo (Disc 3).cue`},
			want: []grpWant{
				{
					main: `Jogo (Disc 1).cue`, ext: "cue", discs: 3,
					files: []string{`Jogo (Disc 1).cue`, `Jogo (Disc 2).cue`, `Jogo (Disc 3).cue`},
				},
			},
		},
		{
			name: "IMV-021/arquivo sem número de disco nunca entra em grupo",
			in:   []string{`Jogo (Disc 1).cue`, `Jogo.cue`},
			want: []grpWant{
				{main: `Jogo (Disc 1).cue`, ext: "cue", discs: 1},
				{main: `Jogo.cue`, ext: "cue", discs: 1},
			},
		},
		{
			name: "IMV-022/árvore D: dois disco 1 mais um disco 2",
			in: []string{
				`Jogo (Europe) (Disc 1).cue`,
				`Jogo (USA) (Disc 1).cue`,
				`Jogo (USA) (Disc 2).cue`,
			},
			want: []grpWant{
				{
					main: `Jogo (Europe) (Disc 1).cue`, ext: "cue", discs: 2,
					files: []string{`Jogo (Europe) (Disc 1).cue`, `Jogo (USA) (Disc 2).cue`},
				},
				{main: `Jogo (USA) (Disc 1).cue`, ext: "cue", discs: 1},
			},
		},
		{
			name: "IMV-022/cue e chd do mesmo disco",
			in:   []string{`Jogo (USA) (Disc 1).chd`, `Jogo (USA) (Disc 1).cue`},
			want: []grpWant{
				{main: `Jogo (USA) (Disc 1).chd`, ext: "chd", discs: 1},
				{main: `Jogo (USA) (Disc 1).cue`, ext: "cue", discs: 1},
			},
		},
		{
			name: "IMV-023/árvore C: m3u + 2 cue é um candidato com extension m3u",
			in:   []string{`Jogo.m3u`, `Jogo (Disc 1).cue`, `Jogo (Disc 2).cue`},
			want: []grpWant{
				{
					main: `Jogo.m3u`, ext: "m3u", discs: 2,
					files: []string{`Jogo (Disc 1).cue`, `Jogo (Disc 2).cue`},
				},
			},
		},
		{
			name: "IMV-023/m3u sozinho é candidato comum",
			in:   []string{`Jogo.m3u`},
			want: []grpWant{
				{main: `Jogo.m3u`, ext: "m3u", discs: 1},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkGroups(t, groupDiscs(grpFiles(tc.in)), tc.want)
		})
	}
}

// IMV-080: the grouping functions only receive fileEntry values (names, no paths on disk).
// Paths below do not exist anywhere; the results must come from the names alone.
func TestFolderGroupsUseNamesOnly(t *testing.T) {
	t.Parallel()

	t.Run("IMV-080/nenhuma função desta tarefa tem acesso a disco", func(t *testing.T) {
		t.Parallel()

		files := grpFiles([]string{
			`Z\inexistente\Jogo (USA).cue`,
			`Z\inexistente\Jogo (USA) (Track 01).bin`,
			`Z\inexistente\Jogo (Disc 1).cue`,
			`Z\inexistente\Jogo (Disc 2).cue`,
			`Z\inexistente\Outro.m3u`,
		})

		kept := dropCompanions(files)
		if !sameStrings(grpRels(kept), []string{
			`Z\inexistente\Jogo (USA).cue`,
			`Z\inexistente\Jogo (Disc 1).cue`,
			`Z\inexistente\Jogo (Disc 2).cue`,
			`Z\inexistente\Outro.m3u`,
		}) {
			t.Fatalf("dropCompanions sem disco = %q", grpRels(kept))
		}

		// Natural order of Main.Rel(): "Jogo (Disc 1)" < "Jogo (USA)" < "Outro".
		checkGroups(t, groupDiscs(kept), []grpWant{
			{
				main: `Z\inexistente\Jogo (Disc 1).cue`, ext: "cue", discs: 2,
				files: []string{`Z\inexistente\Jogo (Disc 1).cue`, `Z\inexistente\Jogo (Disc 2).cue`},
			},
			{main: `Z\inexistente\Jogo (USA).cue`, ext: "cue", discs: 1},
			{main: `Z\inexistente\Outro.m3u`, ext: "m3u", discs: 1},
		})
	})
}
