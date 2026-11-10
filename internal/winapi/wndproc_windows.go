//go:build windows

package winapi

import (
	"sync"

	"golang.org/x/sys/windows"
)

// windowHandler handles the messages of one window. handled=false makes the
// shared window procedure fall back to DefWindowProc.
type windowHandler func(hwnd, msg, wParam, lParam uintptr) (ret uintptr, handled bool)

// The window procedure is a single callback for the whole package
// (windows.NewCallback has a per-process limit); it dispatches by hwnd.
var (
	wndProcOnce sync.Once
	wndProcPtr  uintptr

	handlersMu sync.RWMutex
	handlers   = map[windows.HWND]windowHandler{}
)

// registerWindowHandler routes the messages of hwnd to h. The hwnd is only
// known after CreateWindowEx returns, so messages sent during creation
// (WM_NCCREATE, WM_CREATE, ...) go to DefWindowProc.
func registerWindowHandler(hwnd windows.HWND, h windowHandler) {
	handlersMu.Lock()
	handlers[hwnd] = h
	handlersMu.Unlock()
}

// unregisterWindowHandler must be called before DestroyWindow; later messages
// for hwnd go to DefWindowProc.
func unregisterWindowHandler(hwnd windows.HWND) {
	handlersMu.Lock()
	delete(handlers, hwnd)
	handlersMu.Unlock()
}

// sharedWndProc returns the single window procedure, created once.
func sharedWndProc() uintptr {
	wndProcOnce.Do(func() { wndProcPtr = windows.NewCallback(dispatchWndProc) })
	return wndProcPtr
}

func dispatchWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	handlersMu.RLock()
	h := handlers[windows.HWND(hwnd)]
	handlersMu.RUnlock()
	if h != nil {
		if ret, handled := h(hwnd, msg, wParam, lParam); handled {
			return ret
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}
