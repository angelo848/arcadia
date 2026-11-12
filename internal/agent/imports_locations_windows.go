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

// EpicManifestsPath returns ProgramData\Epic\EpicGamesLauncher\Data\Manifests.
func (defaultLocations) EpicManifestsPath() (string, error) {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		return "", errors.New("ProgramData is not set")
	}
	return filepath.Join(pd, "Epic", "EpicGamesLauncher", "Data", "Manifests"), nil
}
