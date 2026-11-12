//go:build windows

package winapi

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// steamRegistryKey is the HKCU key of the Steam client (IMP-002).
const steamRegistryKey = `Software\Valve\Steam`

// SteamInstallPath returns the Steam installation folder from the SteamPath value
// of HKCU\Software\Valve\Steam. A missing key or value is an error: Steam is not
// installed.
func SteamInstallPath() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, steamRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("open HKCU\\%s: %w", steamRegistryKey, err)
	}
	defer k.Close()
	v, _, err := k.GetStringValue("SteamPath")
	if err != nil {
		return "", fmt.Errorf("read SteamPath: %w", err)
	}
	if v == "" {
		return "", errors.New("SteamPath is empty")
	}
	return v, nil
}
