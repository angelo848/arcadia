package importers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// folderSource is the loose executables importer (IMP-004).
type folderSource struct{}

// Folder returns the folder source. ScanInput.Path is the folder the user picked.
func Folder() Source { return folderSource{} }

func (folderSource) ID() string { return SourceFolder }

// Scan reads the first level of the folder only: every .exe there is a candidate.
// Subfolders are not read in this phase (IMP-004), so installers and tools in
// nested folders do not appear. The platform is left empty: the user picks it when
// applying.
func (folderSource) Scan(ctx context.Context, in ScanInput) (Result, error) {
	var res Result
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if in.Path == "" {
		res.Warnings = append(res.Warnings, "Pasta: nenhuma pasta escolhida")
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
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".exe") {
			continue
		}
		exe := filepath.Join(in.Path, e.Name())
		title := strings.TrimSpace(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		if title == "" {
			continue
		}
		res.Candidates = append(res.Candidates, Candidate{
			Source:       SourceFolder,
			Title:        title,
			LaunchType:   LaunchExe,
			LaunchTarget: exe,
			InstallDir:   in.Path,
		})
	}
	return res, nil
}
