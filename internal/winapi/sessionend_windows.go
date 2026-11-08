//go:build windows

package winapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

const (
	msgClose           = 0x0010 // WM_CLOSE
	msgQueryEndSession = 0x0011 // WM_QUERYENDSESSION
	msgEndSession      = 0x0016 // WM_ENDSESSION
	msgQuit            = 0x0012 // WM_QUIT
	// msgStop is the private message that stops the loop (WM_APP+1). WM_CLOSE
	// is not used because anyone (e.g. taskkill without /F) can send it.
	msgStop = 0x8000 + 1

	// endSessionWait bounds how long the WM_ENDSESSION handler waits for
	// ShutdownDone: Windows may terminate the process as soon as it returns.
	endSessionWait = 2 * time.Second
)

type wndClassExW struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menuName   *uint16
	className  *uint16
	iconSm     windows.Handle
}

type point struct{ x, y int32 }

type msgW struct {
	hwnd     windows.HWND
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       point
	lPrivate uint32
}

// The window procedure is a single callback for the whole package
// (windows.NewCallback has a per-process limit); it dispatches to the watcher
// that owns the hwnd.
var (
	wndProcOnce sync.Once
	wndProcPtr  uintptr

	watchersMu sync.Mutex
	watchers   = map[windows.HWND]*SessionEndWatcher{}

	classSeq atomic.Uint64
)

func sharedWndProc() uintptr {
	wndProcOnce.Do(func() { wndProcPtr = windows.NewCallback(wndProc) })
	return wndProcPtr
}

func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	watchersMu.Lock()
	w := watchers[windows.HWND(hwnd)]
	watchersMu.Unlock()
	if w != nil {
		switch msg {
		case msgQueryEndSession:
			return 1 // never veto the session end
		case msgEndSession:
			w.onEndSession(wParam != 0)
			return 0
		case msgStop:
			// Only stop the loop; the loop goroutine destroys the window.
			procPostQuitMessage.Call(0)
			return 0
		case msgClose:
			// External WM_CLOSE (e.g. taskkill without /F): ignored, so
			// DefWindowProc does not destroy the window.
			w.log.Warn("ignoring external WM_CLOSE on session end watcher window")
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// SessionEndWatcher owns a hidden top-level window (not HWND_MESSAGE, which
// does not receive WM_QUERYENDSESSION/WM_ENDSESSION) whose message loop runs on
// a locked OS thread.
type SessionEndWatcher struct {
	clock domain.Clock
	log   *slog.Logger

	ended    chan struct{} // buffer 1, signalled on WM_ENDSESSION TRUE
	done     chan struct{} // closed by ShutdownDone
	doneOnce sync.Once
	stop     chan struct{} // closed by Close; stops the ctx goroutine

	mu      sync.Mutex // serialises Start and Close
	started bool
	closed  bool

	hwnd    windows.HWND // set by the loop goroutine before Start returns
	tid     uint32       // loop's OS thread id, set before Start returns
	wg      sync.WaitGroup
	loopErr error // written by the loop goroutine, read after wg.Wait

	// postMu serialises posting the stop message with the loop closing exited,
	// so a stop is never posted to the hwnd after DestroyWindow (it could be
	// recycled by then).
	postMu sync.Mutex
	exited chan struct{} // closed by the loop when it leaves the message pump
}

// NewSessionEndWatcher returns a watcher; Start creates its window.
func NewSessionEndWatcher(clock domain.Clock, log *slog.Logger) *SessionEndWatcher {
	return &SessionEndWatcher{
		clock:  clock,
		log:    log,
		ended:  make(chan struct{}, 1),
		done:   make(chan struct{}),
		stop:   make(chan struct{}),
		exited: make(chan struct{}),
	}
}

// Start creates the hidden window and its message loop. The returned channel
// is signalled when Windows sends WM_ENDSESSION with wParam = TRUE. Cancelling
// ctx stops the loop (private stop message); Close still has to be called.
func (w *SessionEndWatcher) Start(ctx context.Context) (<-chan struct{}, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started || w.closed {
		return nil, errors.New("session end watcher: already started or closed")
	}

	ready := make(chan error, 1)
	w.wg.Add(1)
	go w.loop(ready)
	if err := <-ready; err != nil {
		w.wg.Wait()
		return nil, err
	}
	w.started = true

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		select {
		case <-ctx.Done():
			if err := w.stopLoop(); err != nil {
				w.log.Error("session end watcher: stop on context cancel", slog.Any("err", err))
			}
		case <-w.stop:
		}
	}()
	return w.ended, nil
}

// ShutdownDone lets a pending WM_ENDSESSION handler return. Idempotent and
// safe from any goroutine; without a pending handler it is a no-op.
func (w *SessionEndWatcher) ShutdownDone() {
	w.doneOnce.Do(func() { close(w.done) })
}

// Close stops the message loop (private stop message), waits for it to exit,
// and destroys the window and its class (on the loop's own thread, as Win32
// requires). Idempotent. A pending WM_ENDSESSION handler is waited for (it is
// released by ShutdownDone or after endSessionWait).
//
// If the stop message cannot be posted at all (PostMessageW and
// PostThreadMessageW both fail), Close returns that error without waiting: the
// loop goroutine, its locked OS thread, the hidden window and its class stay
// alive until the process exits. Later calls return nil.
func (w *SessionEndWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if !w.started {
		return nil
	}
	close(w.stop)
	if err := w.stopLoop(); err != nil {
		return err
	}
	w.wg.Wait()
	return w.loopErr
}

// stopLoop posts the private stop message while the loop is alive; it is a
// no-op once the loop has exited. If PostMessageW fails it falls back to
// WM_QUIT on the loop thread.
func (w *SessionEndWatcher) stopLoop() error {
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
	postErr := fmt.Errorf("session end watcher: PostMessageW: %w", e)
	r, _, e = procPostThreadMsgW.Call(uintptr(w.tid), msgQuit, 0, 0)
	if r != 0 {
		w.log.Warn("session end watcher: stop message fell back to WM_QUIT", slog.Any("err", postErr))
		return nil
	}
	return errors.Join(postErr, fmt.Errorf("session end watcher: PostThreadMessageW: %w", e))
}

// windowHandle returns the hidden window; valid once Start returned nil.
func (w *SessionEndWatcher) windowHandle() windows.HWND { return w.hwnd }

func (w *SessionEndWatcher) onEndSession(ending bool) {
	if !ending {
		return
	}
	w.log.Info("session ending (WM_ENDSESSION)")
	select {
	case w.ended <- struct{}{}:
	default:
	}
	select {
	case <-w.done:
	case <-w.clock.After(endSessionWait):
		w.log.Warn("WM_ENDSESSION handler timed out waiting for shutdown", slog.Duration("timeout", endSessionWait))
	}
}

// loop owns the window for its whole life: it creates it, pumps messages and
// destroys it, all on the same locked OS thread. The thread is left locked so
// the runtime discards it when the goroutine exits.
func (w *SessionEndWatcher) loop(ready chan<- error) {
	defer w.wg.Done()
	runtime.LockOSThread()

	var instance windows.Handle
	if err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &instance); err != nil {
		ready <- fmt.Errorf("session end watcher: module handle: %w", err)
		return
	}
	className, err := windows.UTF16PtrFromString(fmt.Sprintf("ArcadiaSessionEndWatcher-%d", classSeq.Add(1)))
	if err != nil {
		ready <- fmt.Errorf("session end watcher: class name: %w", err)
		return
	}
	wc := wndClassExW{
		wndProc:   sharedWndProc(),
		instance:  instance,
		className: className,
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	if r, _, e := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		ready <- fmt.Errorf("session end watcher: RegisterClassExW: %w", e)
		return
	}
	unregister := func() error {
		if r, _, e := procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), uintptr(instance)); r == 0 {
			return fmt.Errorf("session end watcher: UnregisterClassW: %w", e)
		}
		return nil
	}

	// Style 0 (WS_OVERLAPPED without WS_VISIBLE), no parent: hidden top-level.
	r, _, e := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)),
		0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if r == 0 {
		ready <- errors.Join(fmt.Errorf("session end watcher: CreateWindowExW: %w", e), unregister())
		return
	}
	hwnd := windows.HWND(r)
	watchersMu.Lock()
	watchers[hwnd] = w
	watchersMu.Unlock()
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
			errs = append(errs, fmt.Errorf("session end watcher: GetMessageW: %w", e))
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	// From here on no stop message may be posted to hwnd.
	w.postMu.Lock()
	close(w.exited)
	w.postMu.Unlock()

	watchersMu.Lock()
	delete(watchers, hwnd)
	watchersMu.Unlock()
	if r, _, e := procDestroyWindow.Call(uintptr(hwnd)); r == 0 {
		errs = append(errs, fmt.Errorf("session end watcher: DestroyWindow: %w", e))
	}
	if err := unregister(); err != nil {
		errs = append(errs, err)
	}
	w.loopErr = errors.Join(errs...)
}
