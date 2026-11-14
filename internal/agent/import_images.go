package agent

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/importers"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// importImages copies the LaunchBox images of a game written by import.apply (MID-010).
type importImages interface {
	CopyLaunchBox(ctx context.Context, gameID int64, root, platform, title string) (int, error)
}

// imageImporter copies the images of the LaunchBox folders into data\media. A file that cannot
// be copied is logged and skipped: the game is still imported (MID-010).
type imageImporter struct {
	files *media.Store
	st    interface {
		ListMedia(ctx context.Context, gameID int64) ([]store.MediaRow, error)
		AddMedia(ctx context.Context, m store.MediaRow) (bool, error)
	}
	clock domain.Clock
	log   *slog.Logger
}

// CopyLaunchBox copies the images of one game and returns how many were recorded.
func (im imageImporter) CopyLaunchBox(ctx context.Context, gameID int64, root, platform, title string) (int, error) {
	existing, err := im.st.ListMedia(ctx, gameID)
	if err != nil {
		return 0, fmt.Errorf("list media: %w", err)
	}
	have := map[string]int{}
	for _, m := range existing {
		have[m.Kind]++
	}
	n := 0
	for _, ref := range importers.LaunchBoxImages(root, platform, title) {
		if have[ref.Kind] >= domain.MediaKindLimit(ref.Kind) {
			continue
		}
		stored, _, err := im.files.Import(ctx, gameID, ref.Path)
		if err != nil {
			im.log.Warn("imagem do LaunchBox não copiada", "game", gameID, "file", filepath.Base(ref.Path), "error", err.Error())
			continue
		}
		added, err := im.st.AddMedia(ctx, store.MediaRow{
			GameID: gameID, Kind: ref.Kind, Provider: importers.SourceLaunchBox, SourceURL: ref.Path,
			Hash: stored.Hash, Path: stored.Path, ThumbPath: stored.ThumbPath, CreatedAt: im.clock.Now(),
		})
		if err != nil {
			return n, err
		}
		if added {
			have[ref.Kind]++
			n++
		}
	}
	return n, nil
}

// logUnmappedFolders logs once per platform the LaunchBox image folders Arcadia does not read (AJU-022).
func logUnmappedFolders(log *slog.Logger, seen map[string]bool, root, platform string) {
	if seen[platform] {
		return
	}
	seen[platform] = true
	if folders := importers.UnmappedLaunchBoxFolders(root, platform); len(folders) > 0 {
		log.Info("pastas de imagem do LaunchBox não lidas", "platform", platform, "folders", folders)
	}
}

// launchBoxRoot returns the LaunchBox folder (the one with Data and Images) for the Data\Platforms
// folder the user scanned. Any other folder gives "", so no image is read from it (MID-010).
func launchBoxRoot(platformsDir string) string {
	dir := filepath.Clean(platformsDir)
	if !strings.EqualFold(filepath.Base(dir), "Platforms") {
		return ""
	}
	data := filepath.Dir(dir)
	if !strings.EqualFold(filepath.Base(data), "Data") {
		return ""
	}
	return filepath.Dir(data)
}

// copyLaunchBoxImages copies the images of one LaunchBox candidate after it was written. A candidate
// of another source, or an import with no LaunchBox folder, copies nothing (MID-010).
func copyLaunchBoxImages(ctx context.Context, imgs importImages, log *slog.Logger, launchBoxPath string, c ipc.ImportCandidate, gameID int64) {
	if imgs == nil || launchBoxPath == "" || c.Source != ipc.ImportSourceLaunchBox {
		return
	}
	root := launchBoxRoot(launchBoxPath)
	if root == "" {
		log.Warn("pasta do LaunchBox inválida; imagens não copiadas", "path", launchBoxPath)
		return
	}
	if _, err := imgs.CopyLaunchBox(ctx, gameID, root, c.Platform, c.Title); err != nil {
		log.Warn("falha ao copiar imagens do LaunchBox", "game", gameID, "error", err.Error())
	}
}
