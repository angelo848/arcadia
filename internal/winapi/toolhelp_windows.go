//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProcEntry is one process of a snapshot: its id and full image path, "" when the
// path cannot be read (RUL-002).
type ProcEntry struct {
	PID  uint32
	Path string
}

// ListProcesses returns the running processes from a Toolhelp snapshot. The image
// path comes from QueryFullProcessImageNameW with PROCESS_QUERY_LIMITED_INFORMATION,
// which needs no administrator rights; a process that cannot be opened keeps an
// empty path. Each handle is closed before the next one is opened.
func ListProcesses() ([]ProcEntry, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("toolhelp snapshot: %w", err)
	}
	defer windows.CloseHandle(snap)

	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	if err := windows.Process32First(snap, &e); err != nil {
		return nil, fmt.Errorf("process32first: %w", err)
	}
	var out []ProcEntry
	for {
		out = append(out, ProcEntry{PID: e.ProcessID, Path: imagePath(e.ProcessID)})
		if err := windows.Process32Next(snap, &e); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return out, nil
			}
			return nil, fmt.Errorf("process32next: %w", err)
		}
	}
}

// imagePath returns the full image path of pid, or "" when it cannot be read.
func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH*4)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}
