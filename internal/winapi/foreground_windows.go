//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Constants of ForceForeground.
const (
	swRestore     = 9
	hwndTopmost   = ^uintptr(0) // (HWND)-1
	hwndNoTopmost = ^uintptr(1) // (HWND)-2
	swpKeepGeom   = swpNoMove | swpNoSize
)

// Test seams for pinning the OS thread.
var (
	lockThread   = runtime.LockOSThread
	unlockThread = runtime.UnlockOSThread
)

// ForegroundAPI is the Win32 surface ForceForegroundWith needs (CON-023).
type ForegroundAPI interface {
	IsIconic(hwnd uintptr) bool
	ShowWindow(hwnd uintptr, cmd int)
	SetWindowPos(hwnd, insertAfter uintptr)
	SetForegroundWindow(hwnd uintptr) bool
	GetForegroundWindow() uintptr
	ThreadOf(hwnd uintptr) uint32
	CurrentThread() uint32
	AttachThreadInput(from, to uint32, attach bool) bool
	BringWindowToTop(hwnd uintptr) bool
}

// ForceForegroundWith brings hwnd to the foreground: restore when minimised, a topmost/non-topmost
// toggle, then SetForegroundWindow; if Windows denies it, it attaches to the foreground thread's input
// and tries again. The detach always runs. It reports whether hwnd ended up in the foreground.
func ForceForegroundWith(api ForegroundAPI, hwnd uintptr) bool {
	// Pin the OS thread so attach and detach see the same thread id; released last.
	lockThread()
	defer unlockThread()
	self := api.CurrentThread()
	if api.IsIconic(hwnd) {
		api.ShowWindow(hwnd, swRestore)
	}
	api.SetWindowPos(hwnd, hwndTopmost)
	api.SetWindowPos(hwnd, hwndNoTopmost)
	if api.SetForegroundWindow(hwnd) {
		return true
	}
	fg := api.GetForegroundWindow()
	if fg == hwnd {
		return true
	}
	if fg == 0 {
		return false
	}
	fgThread := api.ThreadOf(fg)
	if fgThread == 0 || fgThread == self {
		return api.BringWindowToTop(hwnd) && api.SetForegroundWindow(hwnd)
	}
	if !api.AttachThreadInput(fgThread, self, true) {
		return false
	}
	defer api.AttachThreadInput(fgThread, self, false)
	api.BringWindowToTop(hwnd)
	return api.SetForegroundWindow(hwnd)
}

// ForceForeground is ForceForegroundWith over the system API.
func ForceForeground(hwnd uintptr) bool { return ForceForegroundWith(sysForeground{}, hwnd) }

type sysForeground struct{}

func (sysForeground) IsIconic(h uintptr) bool {
	r, _, _ := procIsIconic.Call(h)
	return r != 0
}

func (sysForeground) ShowWindow(h uintptr, cmd int) { procShowWindow.Call(h, uintptr(cmd)) }

func (sysForeground) SetWindowPos(h, after uintptr) {
	procSetWindowPos.Call(h, after, 0, 0, 0, 0, swpKeepGeom)
}

func (sysForeground) SetForegroundWindow(h uintptr) bool {
	r, _, _ := procSetForegroundWindow.Call(h)
	return r != 0
}

func (sysForeground) GetForegroundWindow() uintptr {
	r, _, _ := procGetForegroundWindow.Call()
	return r
}

func (sysForeground) ThreadOf(h uintptr) uint32 {
	var pid uint32
	r, _, _ := procGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	return uint32(r)
}

func (sysForeground) CurrentThread() uint32 { return windows.GetCurrentThreadId() }

func (sysForeground) AttachThreadInput(from, to uint32, attach bool) bool {
	var v uintptr
	if attach {
		v = 1
	}
	r, _, _ := procAttachThreadInput.Call(uintptr(from), uintptr(to), v)
	return r != 0
}

func (sysForeground) BringWindowToTop(h uintptr) bool {
	r, _, _ := procBringWindowToTop.Call(h)
	return r != 0
}

// mainWindowSearch is the state of the single package-level EnumWindows callback; mainWindowMu
// serialises searches.
var (
	mainWindowMu     sync.Mutex
	mainWindowPid    uint32
	mainWindowBest   uintptr
	mainWindowArea   int64
	mainWindowEnumCb = syscall.NewCallback(enumMainWindow)
)

func enumMainWindow(hwnd, _ uintptr) uintptr {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != mainWindowPid {
		return 1
	}
	if v, _, _ := procIsWindowVisibleFG.Call(hwnd); v == 0 {
		return 1
	}
	var rc windows.Rect
	if r, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); r == 0 {
		return 1
	}
	area := int64(rc.Right-rc.Left) * int64(rc.Bottom-rc.Top)
	if area > mainWindowArea {
		mainWindowArea, mainWindowBest = area, hwnd
	}
	return 1
}

// MainWindow returns the visible top-level window of this process with the largest area.
func MainWindow() (uintptr, error) {
	mainWindowMu.Lock()
	defer mainWindowMu.Unlock()
	mainWindowPid, mainWindowBest, mainWindowArea = windows.GetCurrentProcessId(), 0, 0
	procEnumWindows.Call(mainWindowEnumCb, 0)
	if mainWindowBest == 0 {
		return 0, errors.New("winapi: no visible window of this process")
	}
	return mainWindowBest, nil
}

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
