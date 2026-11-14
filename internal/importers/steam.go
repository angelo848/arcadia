package importers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// steamSource is the Steam importer (IMP-002).
type steamSource struct{}

// Steam returns the Steam source. ScanInput.Path is the Steam installation folder
// (the one with steamapps\libraryfolders.vdf); the agent finds it in the registry.
func Steam() Source { return steamSource{} }

func (steamSource) ID() string { return SourceSteam }

// steamToolAppIDs are installed Steam items that are not games.
var steamToolAppIDs = map[string]bool{
	"228980": true, // Steamworks Common Redistributables
}

var appManifestRe = regexp.MustCompile(`(?i)^appmanifest_(\d+)\.acf$`)

func (steamSource) Scan(ctx context.Context, in ScanInput) (Result, error) {
	var res Result
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if in.Path == "" {
		res.Warnings = append(res.Warnings, "Steam: pasta de instalação não encontrada")
		return res, nil
	}
	libs, err := steamLibraries(in.Path)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("Steam: libraryfolders.vdf ilegível (%v)", err))
	}
	for _, lib := range libs {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		steamapps := filepath.Join(lib, "steamapps")
		entries, err := os.ReadDir(steamapps)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Steam: pasta %s ilegível", steamapps))
			continue
		}
		for _, e := range entries {
			m := appManifestRe.FindStringSubmatch(e.Name())
			if m == nil || e.IsDir() {
				continue
			}
			c, warn := steamCandidate(filepath.Join(steamapps, e.Name()), steamapps, m[1])
			if warn != "" {
				res.Warnings = append(res.Warnings, warn)
			}
			if c != nil {
				res.Candidates = append(res.Candidates, *c)
			}
		}
	}
	return res, nil
}

// steamLibraries returns the library folders listed in libraryfolders.vdf, always
// including the installation folder. A missing or broken file returns the
// installation folder and the error, so the scan still reads that library.
func steamLibraries(root string) ([]string, error) {
	libs := []string{root}
	data, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
	if errors.Is(err, fs.ErrNotExist) {
		// Only the installation library: the file lists the extra ones and is optional.
		return libs, nil
	}
	if err != nil {
		return libs, err
	}
	obj, err := parseVDF(data)
	if err != nil {
		return libs, fmt.Errorf("libraryfolders.vdf: %w", err)
	}
	// The entries sit under the "libraryfolders" key in the current format.
	entries := obj
	if lf, ok := obj.child("libraryfolders"); ok {
		entries = lf
	}
	seen := map[string]bool{strings.ToLower(filepath.Clean(root)): true}
	for _, e := range entries {
		if !e.IsObj {
			continue
		}
		p, ok := e.Object.get("path")
		if !ok || p == "" {
			continue
		}
		p = filepath.Clean(p)
		if key := strings.ToLower(p); !seen[key] {
			seen[key] = true
			libs = append(libs, p)
		}
	}
	return libs, nil
}

// steamCandidate reads one appmanifest. It returns (nil, "") for tools that are not
// games and (nil, warning) for unreadable manifests.
func steamCandidate(path, steamapps, appID string) (*Candidate, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Sprintf("Steam: %s ilegível", filepath.Base(path))
	}
	obj, err := parseVDF(data)
	if err != nil {
		return nil, fmt.Sprintf("Steam: %s inválido", filepath.Base(path))
	}
	state, ok := obj.child("AppState")
	if !ok {
		return nil, fmt.Sprintf("Steam: %s sem AppState", filepath.Base(path))
	}
	if steamToolAppIDs[appID] {
		return nil, ""
	}
	name, ok := state.get("name")
	if !ok || strings.TrimSpace(name) == "" {
		return nil, fmt.Sprintf("Steam: appid %s sem nome", appID)
	}
	c := &Candidate{
		Source:       SourceSteam,
		Title:        strings.TrimSpace(name),
		Platform:     PlatformWindows,
		LaunchType:   LaunchURI,
		LaunchTarget: "steam://rungameid/" + appID,
	}
	if dir, ok := state.get("installdir"); ok && dir != "" {
		c.InstallDir = filepath.Join(steamapps, "common", dir)
	}
	return c, ""
}
