//go:build windows

package winapi

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// ShellOpen asks the shell to open uri with the program registered for its scheme
// (steam://, com.epicgames.launcher://, ...). The shell returns no process handle:
// the game is found later by the watcher (RUL-003).
func ShellOpen(uri string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("shell open verb: %w", err)
	}
	target, err := windows.UTF16PtrFromString(uri)
	if err != nil {
		return fmt.Errorf("shell open target: %w", err)
	}
	if err := windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("shell open %q: %w", uri, err)
	}
	return nil
}
