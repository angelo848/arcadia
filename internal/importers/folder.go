package importers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// folderSource is the folder importer (IMP-004): every file of the first level of a folder the user
// picked, of any type. Executables launch directly; any other file is a ROM for an emulator.
type folderSource struct{}

// Folder returns the folder source. ScanInput.Path is the folder the user picked.
func Folder() Source { return folderSource{} }

func (folderSource) ID() string { return SourceFolder }

// Scan reads the first level of the folder. Subfolders are not read (IMP-004). A missing folder, a
// path that is not a folder, or a folder without files gives a warning. The platform is left empty:
// the user picks it when applying.
func (folderSource) Scan(ctx context.Context, in ScanInput) (Result, error) {
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
	entries, err := os.ReadDir(in.Path)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("Pasta: %s ilegível", in.Path))
		return res, nil
	}
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		title := strings.TrimSpace(strings.TrimSuffix(name, ext))
		if title == "" {
			continue
		}
		launch := LaunchEmulator
		if runnableExts[strings.ToLower(ext)] {
			launch = LaunchExe
		}
		res.Candidates = append(res.Candidates, Candidate{
			Source:       SourceFolder,
			Title:        title,
			LaunchType:   launch,
			LaunchTarget: filepath.Join(in.Path, name),
			InstallDir:   in.Path,
		})
	}
	if len(res.Candidates) == 0 {
		res.Warnings = append(res.Warnings, "Pasta: nenhum arquivo no primeiro nível")
	}
	return res, nil
}
