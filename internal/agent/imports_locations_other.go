//go:build !windows

package agent

import "errors"

// noLocations reports every launcher as not installed outside Windows.
type noLocations struct{}

// defaultImportLocations returns no locations outside Windows.
func defaultImportLocations() importLocations { return noLocations{} }

// SteamPath is not available outside Windows.
func (noLocations) SteamPath() (string, error) { return "", errors.New("steam: not available") }

// EpicManifestsPath is not available outside Windows.
func (noLocations) EpicManifestsPath() (string, error) {
	return "", errors.New("epic: not available")
}
