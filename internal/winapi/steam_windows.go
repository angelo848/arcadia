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

// steamAppsKey is the HKCU key with one subkey per installed Steam app (STM-001).
const steamAppsKey = `Software\Valve\Steam\Apps`

// SteamRunning reads the Running value of app appID (STM-001): present is false
// when the key or the value does not exist (Steam does not know the app), and
// running is true when Steam reports the app as running. A value that is not a
// DWORD is an error.
func SteamRunning(appID string) (running, present bool, err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, steamAppsKey+`\`+appID, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("open HKCU\\%s\\%s: %w", steamAppsKey, appID, err)
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("Running")
	if errors.Is(err, registry.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("read Running of app %s: %w", appID, err)
	}
	return v == 1, true, nil
}
