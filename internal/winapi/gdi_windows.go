//go:build windows

package winapi

import "golang.org/x/sys/windows"

// gdi32 / shcore entry points used by the overlay (T16).
var (
	modGdi32  = windows.NewLazySystemDLL("gdi32.dll")
	modShcore = windows.NewLazySystemDLL("shcore.dll")

	procCreateFontW      = modGdi32.NewProc("CreateFontW")
	procCreateSolidBrush = modGdi32.NewProc("CreateSolidBrush")
	procSetBkMode        = modGdi32.NewProc("SetBkMode")
	procSetTextColor     = modGdi32.NewProc("SetTextColor")
	procSelectObject     = modGdi32.NewProc("SelectObject")
	procDeleteObject     = modGdi32.NewProc("DeleteObject")

	// FillRect and DrawTextW live in user32 but belong to the painting code.
	procFillRect  = modUser32.NewProc("FillRect")
	procDrawTextW = modUser32.NewProc("DrawTextW")

	procGetDpiForMonitor = modShcore.NewProc("GetDpiForMonitor")
)
