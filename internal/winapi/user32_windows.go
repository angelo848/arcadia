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

	procAllowSetForegroundWindow = modUser32.NewProc("AllowSetForegroundWindow")
	procSetForegroundWindow      = modUser32.NewProc("SetForegroundWindow")
	procIsIconic                 = modUser32.NewProc("IsIconic")
	procAttachThreadInput        = modUser32.NewProc("AttachThreadInput")
	procBringWindowToTop         = modUser32.NewProc("BringWindowToTop")
	procGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisibleFG        = modUser32.NewProc("IsWindowVisible")
	procGetWindowRect            = modUser32.NewProc("GetWindowRect")
	procEnumWindows              = modUser32.NewProc("EnumWindows")
)

// Declarations used by the power watcher (T13) and the overlay (T16).
// SendMessageW, IsWindow, IsWindowVisible and GetAncestor are declared in
// sessionend_windows_test.go (same package), so they are not repeated here.
var (
	procGetGuiResources              = modUser32.NewProc("GetGuiResources")
	procPeekMessageW                 = modUser32.NewProc("PeekMessageW")
	procGetForegroundWindow          = modUser32.NewProc("GetForegroundWindow")
	procMonitorFromWindow            = modUser32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW              = modUser32.NewProc("GetMonitorInfoW")
	procSetWindowPos                 = modUser32.NewProc("SetWindowPos")
	procSetLayeredWindowAttributes   = modUser32.NewProc("SetLayeredWindowAttributes")
	procShowWindow                   = modUser32.NewProc("ShowWindow")
	procInvalidateRect               = modUser32.NewProc("InvalidateRect")
	procBeginPaint                   = modUser32.NewProc("BeginPaint")
	procEndPaint                     = modUser32.NewProc("EndPaint")
	procGetWindowLongPtrW            = modUser32.NewProc("GetWindowLongPtrW")
	procSetThreadDpiAwarenessContext = modUser32.NewProc("SetThreadDpiAwarenessContext")
	procGetDpiForWindow              = modUser32.NewProc("GetDpiForWindow")
)
