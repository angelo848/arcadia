//go:build windows

package winapi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// minFocusSide skips splash bars and tool windows: only a window at least this wide and tall is focused.
const minFocusSide = 120

var (
	focusMu    sync.Mutex
	focusPids  map[uint32]struct{}
	focusBest  uintptr
	focusArea  int64
	focusEnumC = windows.NewCallback(enumFocusWindow)
)

func enumFocusWindow(hwnd, _ uintptr) uintptr {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if _, ok := focusPids[pid]; !ok {
		return 1
	}
	if v, _, _ := procIsWindowVisibleFG.Call(hwnd); v == 0 {
		return 1
	}
	var rc windows.Rect
	if r, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); r == 0 {
		return 1
	}
	w, h := int64(rc.Right-rc.Left), int64(rc.Bottom-rc.Top)
	if w < minFocusSide || h < minFocusSide {
		return 1
	}
	if w*h > focusArea {
		focusArea, focusBest = w*h, hwnd
	}
	return 1
}

// processParents returns the parent of every running process (pid -> ppid) from a Toolhelp snapshot.
func processParents() (map[uint32]uint32, error) {
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
	out := map[uint32]uint32{}
	for {
		out[e.ProcessID] = e.ParentProcessID
		if err := windows.Process32Next(snap, &e); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return out, nil
			}
			return nil, fmt.Errorf("process32next: %w", err)
		}
	}
}

// biggestWindowOf returns the largest visible top-level window of any pid in pids, or 0.
func biggestWindowOf(pids map[uint32]struct{}) uintptr {
	focusMu.Lock()
	defer focusMu.Unlock()
	focusPids, focusBest, focusArea = pids, 0, 0
	procEnumWindows.Call(focusEnumC, 0)
	return focusBest
}

// FocusChildWindow waits for a window of a process started by parent (or by its descendants) and brings it
// to the foreground, so a game launched by a background agent does not open behind the other windows. It
// polls every interval until timeout or ctx ends, and reports whether a window was focused.
func FocusChildWindow(ctx context.Context, parent uint32, interval, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if parents, err := processParents(); err == nil {
			if kids := DescendantsOf(parents, parent); len(kids) > 0 {
				if h := biggestWindowOf(kids); h != 0 {
					FocusWindow(h)
					return true
				}
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-tick.C:
		}
	}
}
