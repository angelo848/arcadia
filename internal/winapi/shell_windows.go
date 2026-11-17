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

// ShellReveal opens the Explorer on path: a folder is opened, a file is opened in its folder with the
// file selected. The Explorer returns no handle and the call does not wait for it.
func ShellReveal(path string, isFile bool) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("reveal verb: %w", err)
	}
	exe, err := windows.UTF16PtrFromString("explorer.exe")
	if err != nil {
		return fmt.Errorf("reveal exe: %w", err)
	}
	arg := `"` + path + `"`
	if isFile {
		arg = `/select,"` + path + `"`
	}
	args, err := windows.UTF16PtrFromString(arg)
	if err != nil {
		return fmt.Errorf("reveal args: %w", err)
	}
	if err := windows.ShellExecute(0, verb, exe, args, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("reveal %q: %w", path, err)
	}
	return nil
}
