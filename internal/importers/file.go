package importers

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// fileSource is the single-file importer (IMP-004): the user picks one file, of any
// type, and it becomes one candidate.
type fileSource struct{}

// File returns the file source. ScanInput.Path is the file the user picked.
func File() Source { return fileSource{} }

func (fileSource) ID() string { return SourceFile }

// runnableExts are the files launched directly (launch_type exe). Any other file is a
// ROM: it becomes a candidate of launch_type emulator, with the file as target (EMU-003).
var runnableExts = map[string]bool{".exe": true, ".bat": true, ".cmd": true}

// Scan reads the one file the user picked. A folder, a missing file or an empty
// path gives a warning and no candidate. The platform is left empty: the user picks
// it when applying.
func (fileSource) Scan(ctx context.Context, in ScanInput) (Result, error) {
	var res Result
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if in.Path == "" {
		res.Warnings = append(res.Warnings, "Arquivo: nenhum arquivo escolhido")
		return res, nil
	}
	info, err := os.Stat(in.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			res.Warnings = append(res.Warnings, "Arquivo: não encontrado")
		} else {
			res.Warnings = append(res.Warnings, "Arquivo: ilegível")
		}
		return res, nil
	}
	if info.IsDir() {
		res.Warnings = append(res.Warnings, "Arquivo: escolha um arquivo, não uma pasta")
		return res, nil
	}
	name := filepath.Base(in.Path)
	stem, ext := SplitExt(name)
	title := NormalizeTitle(stem, false)
	if title == "" {
		res.Warnings = append(res.Warnings, "Arquivo: nome vazio")
		return res, nil
	}
	launch := LaunchEmulator
	if runnableExts["."+ext] {
		launch = LaunchExe
	}
	res.Candidates = append(res.Candidates, Candidate{
		Source:       SourceFile,
		Title:        title,
		LaunchType:   launch,
		LaunchTarget: in.Path,
		InstallDir:   filepath.Dir(in.Path),
		File:         name,
		Extension:    ext,
		Discs:        1,
	})
	return res, nil
}
