//go:build windows

package winapi

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CreateDetached starts exe with args (each quoted with windows.EscapeArg) and
// the given creation flags, closing hProcess and hThread right after creation.
// The returned error wraps the windows.Errno, so errors.Is works.
func CreateDetached(exe string, args []string, flags uint32) error {
	var b strings.Builder
	b.WriteString(quoteExe(exe))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(windows.EscapeArg(a))
	}
	appName, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return fmt.Errorf("create process %q: %w", exe, err)
	}
	cmdLine, err := windows.UTF16FromString(b.String())
	if err != nil {
		return fmt.Errorf("create process %q: %w", exe, err)
	}
	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(appName, &cmdLine[0], nil, nil, false, flags, nil, nil, &si, &pi); err != nil {
		return fmt.Errorf("create process %q: %w", exe, err)
	}
	// The process is already running: a failure to close our own handles is not
	// a launch failure, so it is not reported.
	_ = windows.CloseHandle(pi.Thread)
	_ = windows.CloseHandle(pi.Process)
	return nil
}

// quoteExe always wraps the executable path in quotes (paths may contain
// spaces); EscapeArg only quotes when needed.
func quoteExe(exe string) string {
	q := windows.EscapeArg(exe)
	if strings.HasPrefix(q, `"`) {
		return q
	}
	return `"` + q + `"`
}
