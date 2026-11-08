//go:build windows

package main

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"unsafe"

	"fyne.io/systray"
	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

//go:embed assets/icon.ico
var trayIcon []byte

var (
	modUser32              = windows.NewLazySystemDLL("user32.dll")
	procPeekMessageW       = modUser32.NewProc("PeekMessageW")
	procPostThreadMessageW = modUser32.NewProc("PostThreadMessageW")
)

const (
	wmQuit     = 0x0012
	wmUser     = 0x0400
	pmNoRemove = 0x0000
)

// winMsg mirrors the Win32 MSG structure.
type winMsg struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	ptX      int32
	ptY      int32
	lPrivate uint32
}

// runTray shows the tray icon with "Abrir Arcadia" and "Sair" and blocks until
// the systray message loop ends. The event loop (trayLoop) runs on its own
// goroutine, which is stopped and waited for before runTray returns. If the
// systray onReady never runs, ctx done or sessionEnd end the message loop by
// posting WM_QUIT to the systray thread (systray.Quit needs the tray window).
func runTray(ctx context.Context, clock domain.Clock, log *slog.Logger, sessionEnd <-chan struct{}, onOpen func()) {
	// systray's init already locked the main goroutine to its OS thread; the
	// nested lock makes the thread id below valid for systray.Run regardless.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tid := windows.GetCurrentThreadId()
	ensureMessageQueue()

	quitBeforeReady := func() {
		if err := postThreadQuit(tid); err != nil {
			log.Error("falha ao encerrar o loop da bandeja", "error", err.Error())
		}
	}

	items := make(chan trayItems, 1)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		trayLoop(ctx, clock, log, items, sessionEnd, stop, systray.Quit, quitBeforeReady, onOpen)
	}()

	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTooltip("Arcadia")
		mOpen := systray.AddMenuItem("Abrir Arcadia", "Abre a interface do Arcadia")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Sair", "Encerra o agente do Arcadia")
		items <- trayItems{open: mOpen.ClickedCh, quit: mQuit.ClickedCh}
	}, nil)

	close(stop)
	wg.Wait()
}

// ensureMessageQueue forces the creation of the calling thread's message queue,
// so a WM_QUIT posted before systray's loop starts (or after its init failed)
// is not rejected by PostThreadMessageW.
func ensureMessageQueue() {
	var m winMsg
	_, _, _ = procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, wmUser, wmUser, pmNoRemove)
}

// postThreadQuit posts WM_QUIT to the message queue of thread tid.
func postThreadQuit(tid uint32) error {
	r, _, e := procPostThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0)
	if r == 0 {
		return fmt.Errorf("PostThreadMessageW(WM_QUIT): %w", e)
	}
	return nil
}
