package importers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// TitleFrom is where the title of a folder candidate comes from [IMV-017].
type TitleFrom string

const (
	TitleFromFile   TitleFrom = "file"
	TitleFromFolder TitleFrom = "folder"
)

// FolderKind is the class of extensions a folder scan reads [IMV-013].
type FolderKind int

const (
	KindAny         FolderKind = iota // both lists (no platform, unknown platform, user-created platform)
	KindExecutables                   // platform of seed key windows
	KindConsole                       // platform of any other seed key
)

// FolderOptions are the scan options of the folder source. The zero value is the default of the contract:
// recursive, title from the file, both lists. MaxDepth and MaxItems are 0 for the defaults (MaxScanDepth,
// MaxScanItems); tests inject small values [IMV-018].
type FolderOptions struct {
	FirstLevelOnly bool // zero value = read the subfolders [IMV-010, IMV-011]
	TitleFrom      TitleFrom
	Kind           FolderKind
	MaxDepth       int
	MaxItems       int
}

const (
	MaxScanDepth = 8     // x-import.maxScanDepth [IMV-018]
	MaxScanItems = 20000 // x-import.maxScanItems [IMV-018]
)

// dirReader is all the folder scan needs from the disk: directory entries and nothing else. It has no Open, so a
// spy that implements only ReadDir proves that no file is opened [IMV-080].
type dirReader interface {
	ReadDir(name string) ([]fs.DirEntry, error)
}

// osDirReader reads directories with os.ReadDir, which opens and closes the directory internally.
type osDirReader struct{}

// ReadDir implements dirReader.
func (osDirReader) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }

// folderSource is the folder importer (IMP-004, phase 11 IMV-010..IMV-024): it reads the folder the user picked,
// recursively or only its first level, and proposes one candidate per game. Executables launch directly; any other
// game file is a ROM for an emulator.
type folderSource struct {
	dirs dirReader
}

// Folder returns the folder source that reads the disk.
func Folder() Source { return newFolderSource(osDirReader{}) }

// newFolderSource returns the folder source that reads directories through dirs (tests inject a fake).
func newFolderSource(dirs dirReader) Source { return folderSource{dirs: dirs} }

func (folderSource) ID() string { return SourceFolder }

// ignoredCounts are the files a scan left out by extension, for the warnings of import-rules.md §5 [IMV-013, IMV-014].
// ByExt is keyed by the lower-case extension without the dot ("" for a file without extension). Console counts the
// console game files a Windows scan ignored; Exe counts the executables a console scan ignored.
type ignoredCounts struct {
	ByExt   map[string]int
	Console int
	Exe     int
}

// Scan reads the folder the user picked. A missing folder, a path that is not a folder, or an unreadable root gives
// a warning and no candidate. The platform is left empty: the user picks it when applying.
func (s folderSource) Scan(ctx context.Context, in ScanInput) (Result, error) {
	var res Result
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if in.Path == "" {
		res.Warnings = append(res.Warnings, "Pasta: nenhuma pasta escolhida")
		return res, nil
	}
	info, err := os.Stat(in.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			res.Warnings = append(res.Warnings, "Pasta: não encontrada")
		} else {
			res.Warnings = append(res.Warnings, "Pasta: ilegível")
		}
		return res, nil
	}
	if !info.IsDir() {
		res.Warnings = append(res.Warnings, "Pasta: escolha uma pasta, não um arquivo")
		return res, nil
	}

	walked, err := walk(ctx, s.dirs, in.Path, in.Folder)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return res, err
		}
		res.Warnings = append(res.Warnings, fmt.Sprintf("Pasta: %s ilegível", in.Path))
		return res, nil
	}

	kept, ign := classify(dropCompanions(walked.Files), in.Folder.Kind)
	entries := groupDiscs(kept)
	res.Candidates = toCandidates(entries, in.Path, in.Folder)
	res.Warnings = buildWarnings(ign, walked, len(res.Candidates), in.Folder)
	return res, nil
}

// classify keeps the files whose extension is read by the kind of the scan [IMV-013] and drops the executables that
// are not games [IMV-015]. Everything else is counted by extension, and the console and executable totals feed the
// wrong-platform warning [IMV-014]. Kept files keep their order.
func classify(files []fileEntry, kind FolderKind) ([]fileEntry, ignoredCounts) {
	ign := ignoredCounts{ByExt: make(map[string]int)}
	kept := make([]fileEntry, 0, len(files))
	for _, f := range files {
		ext := f.Ext()
		switch {
		case isExecutableExt(ext) && kind == KindConsole:
			ign.Exe++
			ign.ByExt[ext]++
		case isExecutableExt(ext) && isNotAGame(f.Stem()):
			ign.ByExt[ext]++
		case isGameExt(ext, kind):
			kept = append(kept, f)
		default:
			ign.ByExt[ext]++
			if kind == KindExecutables && consoleExts[ext] {
				ign.Console++
			}
		}
	}
	return kept, ign
}

// toCandidates builds the candidates of the folder groups [IMV-016, IMV-017, IMV-024]. The title comes from the file
// name, or from the first-level folder when TitleFrom is folder and the file is not in the root (falling back to the
// file name when that title is empty). launchType is exe for executables and emulator for the rest; launchTarget is
// the file on disk and installDir is its folder. Entries without a title are dropped. The result is in the natural
// order of the main file.
func toCandidates(entries []groupEntry, root string, opts FolderOptions) []Candidate {
	out := make([]Candidate, 0, len(entries))
	for _, e := range entries {
		main := e.Main
		stem, ext := SplitExt(main.Name)
		title := NormalizeTitle(stem, false)
		if opts.TitleFrom == TitleFromFolder && main.Dir != "" {
			if t := NormalizeTitle(main.Scope(), true); t != "" {
				title = t
			}
		}
		if title == "" {
			continue
		}
		launch := LaunchEmulator
		if isExecutableExt(ext) {
			launch = LaunchExe
		}
		target := filepath.Join(root, main.Rel())
		c := Candidate{
			Source:       SourceFolder,
			Title:        title,
			LaunchType:   launch,
			LaunchTarget: target,
			InstallDir:   filepath.Dir(target),
			File:         main.Rel(),
			Extension:    ext,
			Discs:        e.Discs,
		}
		if e.Discs >= 2 {
			c.DiscFiles = e.DiscFiles
		}
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Candidate) int {
		switch {
		case naturalLess(a.File, b.File):
			return -1
		case naturalLess(b.File, a.File):
			return 1
		}
		return 0
	})
	return out
}

// buildWarnings returns the warnings of import-rules.md §5 in their order: ignored by extension, wrong platform,
// depth, items, unreadable folders and, when there is no candidate, "no game" [IMV-014, IMV-018].
func buildWarnings(ign ignoredCounts, w walkResult, candidates int, opts FolderOptions) []string {
	var out []string
	if total := sumCounts(ign.ByExt); total > 0 {
		out = append(out, fmt.Sprintf("Pasta: %d arquivos ignorados pela extensão (%s)", total, topExts(ign.ByExt, 5)))
	}
	if candidates == 0 {
		switch opts.Kind {
		case KindExecutables:
			if ign.Console > 0 {
				out = append(out, fmt.Sprintf("Pasta: a plataforma escolhida lê só executáveis; %d arquivos de jogo de console foram ignorados", ign.Console))
			}
		case KindConsole:
			if ign.Exe > 0 {
				out = append(out, fmt.Sprintf("Pasta: a plataforma escolhida lê só arquivos de jogo de console; %d executáveis foram ignorados", ign.Exe))
			}
		}
	}
	if w.DepthCut {
		maxDepth := opts.MaxDepth
		if maxDepth == 0 {
			maxDepth = MaxScanDepth
		}
		out = append(out, fmt.Sprintf("Pasta: subpastas além do nível %d não foram lidas", maxDepth))
	}
	if w.ItemsCut {
		maxItems := opts.MaxItems
		if maxItems == 0 {
			maxItems = MaxScanItems
		}
		out = append(out, fmt.Sprintf("Pasta: leitura interrompida em %d itens; escolha uma subpasta", maxItems))
	}
	if w.Unreadable > 0 {
		out = append(out, fmt.Sprintf("Pasta: %d subpastas ilegíveis", w.Unreadable))
	}
	if candidates == 0 {
		if opts.FirstLevelOnly {
			out = append(out, "Pasta: nenhum jogo no primeiro nível")
		} else {
			out = append(out, "Pasta: nenhum jogo encontrado")
		}
	}
	return out
}

// sumCounts adds the counts of a map.
func sumCounts(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

// topExts lists up to limit extensions, the most frequent first and ties in alphabetical order, as ".ext ×n".
func topExts(m map[string]int, limit int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if m[a] != m[b] {
			return m[b] - m[a]
		}
		return strings.Compare(a, b)
	})
	if len(keys) > limit {
		keys = keys[:limit]
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		label := "." + k
		if k == "" {
			label = "sem extensão"
		}
		parts = append(parts, fmt.Sprintf("%s ×%d", label, m[k]))
	}
	return strings.Join(parts, ", ")
}
