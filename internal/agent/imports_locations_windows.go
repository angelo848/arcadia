//go:build windows

package agent

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// defaultLocations finds the launchers' folders on this machine (IMP-002, IMP-003).
type defaultLocations struct{}

// defaultImportLocations returns the Windows locations used by import.scan.
func defaultImportLocations() importLocations { return defaultLocations{} }

// SteamPath returns the Steam installation folder from the registry.
func (defaultLocations) SteamPath() (string, error) { return winapi.SteamInstallPath() }

// EpicManifestsPath returns the folder the Epic importer reads: the data folder of the new launcher
// (%LOCALAPPDATA%\com.epicgames.eglv2) when it has an installations.jsonl, else the old launcher's
// ProgramData\Epic\EpicGamesLauncher\Data\Manifests.
func (defaultLocations) EpicManifestsPath() (string, error) {
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		dir := filepath.Join(la, "com.epicgames.eglv2")
		if _, err := os.Stat(filepath.Join(dir, "installations.jsonl")); err == nil {
			return dir, nil
		}
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		return "", errors.New("ProgramData is not set")
	}
	return filepath.Join(pd, "Epic", "EpicGamesLauncher", "Data", "Manifests"), nil
}
