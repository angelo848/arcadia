//go:build windows

package winapi

import "golang.org/x/sys/windows"

// user32 entry points missing from golang.org/x/sys/windows.
var (
	modUser32            = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW = modUser32.NewProc("RegisterClassExW")
	procUnregisterClassW = modUser32.NewProc("UnregisterClassW")
	procCreateWindowExW  = modUser32.NewProc("CreateWindowExW")
	procDestroyWindow    = modUser32.NewProc("DestroyWindow")
	procDefWindowProcW   = modUser32.NewProc("DefWindowProcW")
	procGetMessageW      = modUser32.NewProc("GetMessageW")
	procTranslateMessage = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW = modUser32.NewProc("DispatchMessageW")
	procPostMessageW     = modUser32.NewProc("PostMessageW")
	procPostThreadMsgW   = modUser32.NewProc("PostThreadMessageW")
	procPostQuitMessage  = modUser32.NewProc("PostQuitMessage")
)
