package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/importers"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// maxImportCandidates bounds one import.apply request (contract v0.7.0).
const maxImportCandidates = 500

// importStore is the part of *store.Store the import handlers use.
type importStore interface {
	PlatformIDByName(ctx context.Context, name string) (int64, bool, error)
	ImportDuplicate(ctx context.Context, launchType, launchTarget, title string, platformID int64) (bool, error)
	EnsurePlatformID(ctx context.Context, name string, category domain.PlatformCategory) (int64, error)
	EnsureWindowsPlatformID(ctx context.Context) (int64, error)
	WindowsPlatformID(ctx context.Context) (int64, bool, error)
	ApplyImport(ctx context.Context, g store.ImportGame, at time.Time) (int64, error)
}

// importLocations gives the folders of the launchers that the importers read by
// default (IMP-002, IMP-003). An error means "not installed": the scan warns.
type importLocations interface {
	SteamPath() (string, error)
	EpicManifestsPath() (string, error)
}

// importSourceSet maps every source id of the contract to its importer.
func importSourceSet() map[string]importers.Source {
	return map[string]importers.Source{
		ipc.ImportSourceSteam:     importers.Steam(),
		ipc.ImportSourceEpic:      importers.Epic(),
		ipc.ImportSourceFile:      importers.File(),
		ipc.ImportSourceFolder:    importers.Folder(),
		ipc.ImportSourceLaunchBox: importers.LaunchBox(),
	}
}

// importHandlers returns the handlers of import.scan and import.apply (IMP-001).
func importHandlers(st importStore, loc importLocations, clock domain.Clock,
	sources map[string]importers.Source, imgs importImages, log *slog.Logger) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodImportScan: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleImportScan(ctx, st, loc, sources, log, params)
		},
		ipc.MethodImportApply: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleImportApply(ctx, st, clock, imgs, log, params)
		},
	}
}

// handleImportScan reads one source and returns its candidates with the warnings
// (IMP-001, IMP-008). It writes nothing. A candidate already in the library is
// marked exists (IMP-006). An unreadable source is a warning, not an error.
func handleImportScan(ctx context.Context, st importStore, loc importLocations,
	sources map[string]importers.Source, log *slog.Logger, params json.RawMessage) (any, error) {
	if params == nil {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	var p ipc.ImportScanParams
	if rpcErr := ipc.DecodeParams(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	src, ok := sources[p.Source]
	if !ok {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	input := importers.ScanInput{Path: p.Path}
	switch p.Source {
	case ipc.ImportSourceFile, ipc.ImportSourceFolder, ipc.ImportSourceLaunchBox:
		if !filepath.IsAbs(p.Path) {
			return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
	case ipc.ImportSourceSteam:
		input.Path = locationOrEmpty(loc.SteamPath)
	case ipc.ImportSourceEpic:
		input.Path = locationOrEmpty(loc.EpicManifestsPath)
	}

	res, err := src.Scan(ctx, input)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		log.Error("falha ao ler a fonte de importação", "source", p.Source, "error", err.Error())
		res.Warnings = append(res.Warnings, fmt.Sprintf("%s: fonte indisponível", p.Source))
	}

	out := ipc.ImportScanResult{
		Candidates: make([]ipc.ImportCandidate, 0, len(res.Candidates)),
		Warnings:   append([]string{}, res.Warnings...),
	}
	for _, c := range res.Candidates {
		exists, err := candidateExists(ctx, st, c)
		if err != nil {
			return nil, fmt.Errorf("import.scan: %w", err)
		}
		out.Candidates = append(out.Candidates, toIPCCandidate(c, exists))
	}
	return out, nil
}

// locationOrEmpty returns the folder of a launcher, or "" when it is not found.
func locationOrEmpty(f func() (string, error)) string {
	path, err := f()
	if err != nil {
		return ""
	}
	return path
}

// candidateExists checks the library for a candidate without writing: by launch
// target, and by title on the PC platform when that platform exists (IMP-006).
func candidateExists(ctx context.Context, st importStore, c importers.Candidate) (bool, error) {
	var platformID int64
	switch {
	case c.Source == importers.SourceSteam || c.Source == importers.SourceEpic:
		id, ok, err := st.WindowsPlatformID(ctx)
		if err != nil {
			return false, err
		}
		if ok {
			platformID = id
		}
	case c.Platform != "":
		id, ok, err := st.PlatformIDByName(ctx, c.Platform)
		if err != nil {
			return false, err
		}
		if ok {
			platformID = id
		}
	}
	return st.ImportDuplicate(ctx, c.LaunchType, c.LaunchTarget, c.Title, platformID)
}

// toIPCCandidate converts a store-independent candidate to its wire form. Lists are
// never nil, so they encode as [] (contract v0.7.0).
func toIPCCandidate(c importers.Candidate, exists bool) ipc.ImportCandidate {
	out := ipc.ImportCandidate{
		Source:       c.Source,
		Title:        c.Title,
		Platform:     c.Platform,
		LaunchType:   c.LaunchType,
		LaunchTarget: c.LaunchTarget,
		InstallDir:   c.InstallDir,
		Developers:   nonNilStrings(c.Developers),
		Publishers:   nonNilStrings(c.Publishers),
		Genres:       nonNilStrings(c.Genres),
		Year:         c.Year,
		Description:  c.Description,
		PlayModes:    nonNilStrings(c.PlayModes),
		Exists:       exists,
	}
	if c.Played != nil {
		seconds := c.Played.Seconds
		last := c.Played.LastPlay.UTC().Format(time.RFC3339)
		out.PlayedSeconds = &seconds
		out.LastPlayed = &last
	}
	return out
}

// handleImportApply writes the selected candidates, one transaction per game
// (IMP-001, IMP-008). A candidate that is invalid, duplicated or fails is listed as
// skipped and the others go on. Nothing is launched and no tracked session is
// created (IMP-009).
func handleImportApply(ctx context.Context, st importStore, clock domain.Clock,
	imgs importImages, log *slog.Logger, params json.RawMessage) (any, error) {
	if params == nil {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	var p ipc.ImportApplyParams
	if rpcErr := ipc.DecodeParams(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if len(p.Candidates) == 0 || len(p.Candidates) > maxImportCandidates {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	out := ipc.ImportApplyResult{Applied: []ipc.ImportApplied{}, Skipped: []ipc.ImportSkipped{}}
	platforms := map[string]int64{}
	now := clock.Now()
	for _, c := range p.Candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		g, err := toStoreGame(ctx, st, c, platforms)
		if err != nil {
			log.Warn("candidato de importação inválido", "title", c.Title, "error", err.Error())
			out.Skipped = append(out.Skipped, ipc.ImportSkipped{Title: c.Title, Reason: "failed"})
			continue
		}
		id, err := st.ApplyImport(ctx, g, now)
		switch {
		case errors.Is(err, store.ErrDuplicate):
			out.Skipped = append(out.Skipped, ipc.ImportSkipped{Title: c.Title, Reason: "duplicate"})
		case err != nil:
			log.Error("falha ao gravar um jogo importado", "title", c.Title, "error", err.Error())
			out.Skipped = append(out.Skipped, ipc.ImportSkipped{Title: c.Title, Reason: "failed"})
		default:
			out.Applied = append(out.Applied, ipc.ImportApplied{GameID: id, Title: g.Title})
			copyLaunchBoxImages(ctx, imgs, log, p.LaunchBoxPath, c, id)
		}
	}
	return out, nil
}

// toStoreGame validates one wire candidate and resolves its platform. Platforms
// are created here, in their own transaction, before the game transaction starts.
func toStoreGame(ctx context.Context, st importStore, c ipc.ImportCandidate, platforms map[string]int64) (store.ImportGame, error) {
	title, err := domain.ValidateTitle(c.Title)
	if err != nil {
		return store.ImportGame{}, fmt.Errorf("title: %w", err)
	}
	if c.LaunchType != "exe" && c.LaunchType != "uri" && c.LaunchType != "emulator" {
		return store.ImportGame{}, fmt.Errorf("launch type %q", c.LaunchType)
	}
	isFile := c.Source == ipc.ImportSourceFile || c.Source == ipc.ImportSourceFolder
	if c.Platform == "" && isFile {
		return store.ImportGame{}, errors.New("file candidate without platform")
	}
	useWindows := c.Source == ipc.ImportSourceSteam || c.Source == ipc.ImportSourceEpic || c.Platform == ""
	platformName := c.Platform
	if useWindows {
		platformName = importers.PlatformWindows
	}
	platformID, ok := platforms[platformName]
	if !ok {
		if useWindows {
			platformID, err = st.EnsureWindowsPlatformID(ctx)
		} else {
			platformID, err = st.EnsurePlatformID(ctx, platformName, platformCategoryFor(c.Source, platformName))
		}
		if err != nil {
			return store.ImportGame{}, fmt.Errorf("platform %q: %w", platformName, err)
		}
		platforms[platformName] = platformID
	}
	modes := make([]domain.PlayMode, 0, len(c.PlayModes))
	for _, m := range c.PlayModes {
		mode, err := domain.ParsePlayMode(m)
		if err != nil {
			return store.ImportGame{}, fmt.Errorf("play mode: %w", err)
		}
		modes = append(modes, mode)
	}
	g := store.ImportGame{
		PlatformID:   platformID,
		Title:        title,
		LaunchType:   c.LaunchType,
		LaunchTarget: c.LaunchTarget,
		InstallDir:   c.InstallDir,
		Description:  c.Description,
		Year:         c.Year,
		Developers:   c.Developers,
		Publishers:   c.Publishers,
		Genres:       c.Genres,
		PlayModes:    modes,
	}
	if c.PlayedSeconds != nil && *c.PlayedSeconds > 0 && c.LastPlayed != nil {
		last, err := time.Parse(time.RFC3339, *c.LastPlayed)
		if err != nil {
			return store.ImportGame{}, fmt.Errorf("last played: %w", err)
		}
		g.Played = &store.ImportPlayed{Seconds: *c.PlayedSeconds, LastPlay: last.UTC()}
	}
	return g, nil
}

// platformCategoryFor returns the category of a platform created by an import: the
// seed category when the name is a seed platform, else console for LaunchBox and pc.
func platformCategoryFor(source, name string) domain.PlatformCategory {
	if seed, ok := domain.SeedByName(name); ok {
		return seed.Category
	}
	if source == ipc.ImportSourceLaunchBox {
		return domain.PlatformCategory("console")
	}
	return domain.PlatformCategory("pc")
}
