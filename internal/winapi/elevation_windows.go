//go:build windows

package winapi

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// IsProcessElevated reports whether the process pid runs with an elevated token
// (ADM-005). It opens the process with PROCESS_QUERY_LIMITED_INFORMATION and its
// token with TOKEN_QUERY, reads TokenElevation and closes both handles.
func IsProcessElevated(pid uint32) (bool, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false, fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	var tok windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok); err != nil {
		return false, fmt.Errorf("open token of process %d: %w", pid, err)
	}
	defer tok.Close()

	return tok.IsElevated(), nil
}
