package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// importTimeout bounds one scan or apply: reading a launcher's files and writing a
// batch of games can take longer than the library calls (IMP-001).
const importTimeout = 60 * time.Second

// pickFolderTitle is the title of the folder dialog of the import (IMP-010).
const pickFolderTitle = "Escolha a pasta"

// errNoFolderDialog is returned when the desktop cannot show the folder dialog.
var errNoFolderDialog = errors.New("desktop: pick folder: dialog not available")

// ImportCandidateView is one game an import source offers, as the UI sees it. The
// JSON names are the ones of contract v0.7.0.
type ImportCandidateView struct {
	Source        string   `json:"source"`
	Title         string   `json:"title"`
	Platform      string   `json:"platform"`
	LaunchType    string   `json:"launchType"`
	LaunchTarget  string   `json:"launchTarget"`
	InstallDir    string   `json:"installDir"`
	Developers    []string `json:"developers"`
	Publishers    []string `json:"publishers"`
	Genres        []string `json:"genres"`
	Year          *int     `json:"year"`
	Description   string   `json:"description"`
	PlayModes     []string `json:"playModes"`
	PlayedSeconds *int64   `json:"playedSeconds"`
	LastPlayed    *string  `json:"lastPlayed"`
	Exists        bool     `json:"exists"`
}

// ImportScanView is the result of ImportScan: the candidates and the warnings.
type ImportScanView struct {
	Candidates []ImportCandidateView `json:"candidates"`
	Warnings   []string              `json:"warnings"`
}

// ImportAppliedView is one game the import wrote.
type ImportAppliedView struct {
	GameID int64  `json:"gameId"`
	Title  string `json:"title"`
}

// ImportSkippedView is one candidate the import did not write.
type ImportSkippedView struct {
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// ImportApplyView is the result of ImportApply.
type ImportApplyView struct {
	Applied []ImportAppliedView `json:"applied"`
	Skipped []ImportSkippedView `json:"skipped"`
}

// ImportScan reads one import source and returns its candidates without writing
// anything [IMP-001]. path is used by the folder and launchbox sources only.
func (a *App) ImportScan(source, path string) (ImportScanView, error) {
	var res ipc.ImportScanResult
	if err := a.callAgent(ipc.MethodImportScan, importTimeout, ipc.ImportScanParams{Source: source, Path: path}, &res); err != nil {
		return ImportScanView{}, err
	}
	out := ImportScanView{Candidates: make([]ImportCandidateView, 0, len(res.Candidates)), Warnings: res.Warnings}
	for _, c := range res.Candidates {
		out.Candidates = append(out.Candidates, importCandidateView(c))
	}
	return out, nil
}

// ImportApply writes the candidates the user selected [IMP-001]. The agent skips
// the duplicates and the failing ones and reports them.
func (a *App) ImportApply(candidates []ImportCandidateView) (ImportApplyView, error) {
	params := ipc.ImportApplyParams{Candidates: make([]ipc.ImportCandidate, 0, len(candidates))}
	for _, c := range candidates {
		params.Candidates = append(params.Candidates, importCandidateWire(c))
	}
	var res ipc.ImportApplyResult
	if err := a.callAgent(ipc.MethodImportApply, importTimeout, params, &res); err != nil {
		return ImportApplyView{}, err
	}
	out := ImportApplyView{Applied: []ImportAppliedView{}, Skipped: []ImportSkippedView{}}
	for _, g := range res.Applied {
		out.Applied = append(out.Applied, ImportAppliedView{GameID: g.GameID, Title: g.Title})
	}
	for _, s := range res.Skipped {
		out.Skipped = append(out.Skipped, ImportSkippedView{Title: s.Title, Reason: s.Reason})
	}
	return out, nil
}

// PickFolder shows the folder dialog for the folder and launchbox imports. An empty
// result means the user cancelled.
func (a *App) PickFolder() (string, error) {
	a.mu.Lock()
	wailsCtx := a.wailsCtx
	a.mu.Unlock()
	if wailsCtx == nil || a.platform.openDir == nil {
		return "", userError(errNoFolderDialog, a.logger())
	}
	path, err := a.platform.openDir(wailsCtx, pickFolderTitle)
	if err != nil {
		return "", userError(fmt.Errorf("desktop: pick folder: %w", err), a.logger())
	}
	return path, nil
}

func importCandidateView(c ipc.ImportCandidate) ImportCandidateView {
	return ImportCandidateView{
		Source: c.Source, Title: c.Title, Platform: c.Platform, LaunchType: c.LaunchType,
		LaunchTarget: c.LaunchTarget, InstallDir: c.InstallDir,
		Developers: c.Developers, Publishers: c.Publishers, Genres: c.Genres,
		Year: c.Year, Description: c.Description, PlayModes: c.PlayModes,
		PlayedSeconds: c.PlayedSeconds, LastPlayed: c.LastPlayed, Exists: c.Exists,
	}
}

func importCandidateWire(c ImportCandidateView) ipc.ImportCandidate {
	return ipc.ImportCandidate{
		Source: c.Source, Title: c.Title, Platform: c.Platform, LaunchType: c.LaunchType,
		LaunchTarget: c.LaunchTarget, InstallDir: c.InstallDir,
		Developers: c.Developers, Publishers: c.Publishers, Genres: c.Genres,
		Year: c.Year, Description: c.Description, PlayModes: c.PlayModes,
		PlayedSeconds: c.PlayedSeconds, LastPlayed: c.LastPlayed, Exists: c.Exists,
	}
}
