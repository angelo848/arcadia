//go:build windows

package winapi

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// ASFWAny (ASFW_ANY) lets any process set the foreground window.
const ASFWAny uint32 = 0xFFFFFFFF

// AllowSetForegroundWindow lets the process pid (or any process, with ASFWAny)
// bring its window to the foreground (EXE-011).
func AllowSetForegroundWindow(pid uint32) error {
	r, _, e := procAllowSetForegroundWindow.Call(uintptr(pid))
	if r == 0 {
		if errno, ok := e.(windows.Errno); ok && errno != 0 {
			return fmt.Errorf("AllowSetForegroundWindow(%#x): %w", pid, errno)
		}
		return fmt.Errorf("AllowSetForegroundWindow(%#x): failed", pid)
	}
	return nil
}
