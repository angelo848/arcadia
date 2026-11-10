//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

const (
	msgPowerBroadcast = 0x0218 // WM_POWERBROADCAST

	pbtAPMSuspendEvt         = 0x0004 // PBT_APMSUSPEND
	pbtAPMResumeSuspendEvt   = 0x0007 // PBT_APMRESUMESUSPEND
	pbtAPMResumeAutomaticEvt = 0x0012 // PBT_APMRESUMEAUTOMATIC
)

// PowerCallbacks are invoked on the watcher's loop thread; they must return
// quickly. Nil callbacks are ignored.
type PowerCallbacks struct {
	OnSuspend func(at time.Time)
	OnWake    func(at time.Time, userPresent bool)
}

// PowerWatcher owns a hidden top-level window that receives
// WM_POWERBROADCAST, with its message loop on a locked OS thread.
type PowerWatcher struct {
	clock domain.Clock
	log   *slog.Logger
	cb    PowerCallbacks

	mu      sync.Mutex // serialises Start and Close
	started bool
	closed  bool

	hwnd    windows.HWND // set by the loop goroutine before Start returns
	tid     uint32
	wg      sync.WaitGroup
	loopErr error // written by the loop goroutine, read after wg.Wait

	// postMu serialises posting the stop message with the loop closing exited.
	postMu sync.Mutex
	exited chan struct{}
}

// NewPowerWatcher returns a watcher; Start creates its window.
func NewPowerWatcher(clock domain.Clock, log *slog.Logger, cb PowerCallbacks) *PowerWatcher {
	return &PowerWatcher{clock: clock, log: log, cb: cb, exited: make(chan struct{})}
}

// Start creates the hidden window and its message loop. On error nothing is
// left running.
func (w *PowerWatcher) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started || w.closed {
		return errors.New("power watcher: already started or closed")
	}
	ready := make(chan error, 1)
	w.wg.Add(1)
	go w.loop(ready)
	if err := <-ready; err != nil {
		w.wg.Wait()
		return err
	}
	w.started = true
	return nil
}

// Close stops the loop, waits for it and destroys the window and class.
// Idempotent.
func (w *PowerWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if !w.started {
		return nil
	}
	if err := w.stopLoop(); err != nil {
		return err
	}
	w.wg.Wait()
	return w.loopErr
}

func (w *PowerWatcher) stopLoop() error {
	w.postMu.Lock()
	defer w.postMu.Unlock()
	select {
	case <-w.exited:
		return nil
	default:
	}
	r, _, e := procPostMessageW.Call(uintptr(w.hwnd), msgStop, 0, 0)
	if r != 0 {
		return nil
	}
	postErr := fmt.Errorf("power watcher: PostMessageW: %w", e)
	r, _, e = procPostThreadMsgW.Call(uintptr(w.tid), msgQuit, 0, 0)
	if r != 0 {
		w.log.Warn("power watcher: stop message fell back to WM_QUIT", slog.Any("err", postErr))
		return nil
	}
	return errors.Join(postErr, fmt.Errorf("power watcher: PostThreadMessageW: %w", e))
}

// windowHandle returns the hidden window; valid once Start returned nil.
func (w *PowerWatcher) windowHandle() windows.HWND { return w.hwnd }

func (w *PowerWatcher) handleMessage(_, msg, wParam, _ uintptr) (uintptr, bool) {
	switch msg {
	case msgPowerBroadcast:
		at := w.clock.Now()
		switch wParam {
		case pbtAPMSuspendEvt:
			if w.cb.OnSuspend != nil {
				w.cb.OnSuspend(at)
			}
		case pbtAPMResumeSuspendEvt:
			if w.cb.OnWake != nil {
				w.cb.OnWake(at, true)
			}
		case pbtAPMResumeAutomaticEvt:
			if w.cb.OnWake != nil {
				w.cb.OnWake(at, false)
			}
		}
		return 1, true // TRUE: never deny a power broadcast
	case msgStop:
		procPostQuitMessage.Call(0)
		return 0, true
	case msgClose:
		w.log.Warn("ignoring external WM_CLOSE on power watcher window")
		return 0, true
	}
	return 0, false
}

func (w *PowerWatcher) loop(ready chan<- error) {
	defer w.wg.Done()
	runtime.LockOSThread()

	var instance windows.Handle
	if err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &instance); err != nil {
		ready <- fmt.Errorf("power watcher: module handle: %w", err)
		return
	}
	className, err := windows.UTF16PtrFromString(fmt.Sprintf("ArcadiaPowerWatcher-%d", classSeq.Add(1)))
	if err != nil {
		ready <- fmt.Errorf("power watcher: class name: %w", err)
		return
	}
	wc := wndClassExW{
		wndProc:   sharedWndProc(),
		instance:  instance,
		className: className,
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	if r, _, e := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		ready <- fmt.Errorf("power watcher: RegisterClassExW: %w", e)
		return
	}
	unregister := func() error {
		if r, _, e := procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), uintptr(instance)); r == 0 {
			return fmt.Errorf("power watcher: UnregisterClassW: %w", e)
		}
		return nil
	}

	r, _, e := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)),
		0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if r == 0 {
		ready <- errors.Join(fmt.Errorf("power watcher: CreateWindowExW: %w", e), unregister())
		return
	}
	hwnd := windows.HWND(r)
	registerWindowHandler(hwnd, w.handleMessage)
	w.hwnd = hwnd
	w.tid = windows.GetCurrentThreadId()
	ready <- nil

	var errs []error
	var m msgW
	for {
		r, _, e := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) == 0 {
			break // WM_QUIT
		}
		if int32(r) == -1 {
			errs = append(errs, fmt.Errorf("power watcher: GetMessageW: %w", e))
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	w.postMu.Lock()
	close(w.exited)
	w.postMu.Unlock()

	unregisterWindowHandler(hwnd)
	if r, _, e := procDestroyWindow.Call(uintptr(hwnd)); r == 0 {
		errs = append(errs, fmt.Errorf("power watcher: DestroyWindow: %w", e))
	}
	if err := unregister(); err != nil {
		errs = append(errs, err)
	}
	w.loopErr = errors.Join(errs...)
}
