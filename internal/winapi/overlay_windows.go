//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/feedback"
)

const (
	msgOverlayShow = 0x8000 + 2 // WM_APP+2
	msgOverlayHide = 0x8000 + 3 // WM_APP+3

	msgPaint       = 0x000F // WM_PAINT
	msgEraseBkgnd  = 0x0014 // WM_ERASEBKGND
	wsPopup        = 0x80000000
	wsExTopmost    = 0x00000008
	wsExTransp     = 0x00000020
	wsExToolWindow = 0x00000080
	wsExLayered    = 0x00080000
	wsExNoActivate = 0x08000000

	lwaAlpha         = 0x2
	overlayAlpha     = 230
	swHide           = 0
	swpNoSize        = 0x0001
	swpNoMove        = 0x0002
	swpNoZOrder      = 0x0004
	swpHideWindow    = 0x0080
	swpNoActivate    = 0x0010
	swpShowWindow    = 0x0040
	monitorPrimary   = 1 // MONITOR_DEFAULTTOPRIMARY
	mdtEffectiveDPI  = 0
	dtCenterVCenter  = 0x0001 | 0x0004 | 0x0020 // DT_CENTER|DT_VCENTER|DT_SINGLELINE
	transparentBk    = 1                        // TRANSPARENT
	overlayFontPx    = 20
	fwNormal         = 400
	defaultCharset   = 1
	clearTypeQuality = 5
	gdiFontFaceName  = "Segoe UI"
)

// dpiAwarenessPerMonitorV2 is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4).
const dpiAwarenessPerMonitorV2 = ^uintptr(3)

var (
	overlayBgColor   = rgb(24, 24, 27)
	overlayTextColor = rgb(255, 255, 255)
)

func rgb(r, g, b uint32) uintptr { return uintptr(r | g<<8 | b<<16) }

type winRect struct{ left, top, right, bottom int32 }

type monitorInfo struct {
	size    uint32
	monitor winRect
	work    winRect
	flags   uint32
}

type paintStruct struct {
	hdc         windows.Handle
	erase       int32
	paint       winRect
	restore     int32
	incUpdate   int32
	rgbReserved [32]byte
}

// Overlay is the on-screen notice window ("Tempo pausado"/"Tempo retomado"). It
// is click-through, never takes focus and is shown on the monitor of the
// foreground window. It implements feedback.Overlay.
//
// Start can fail (OVL-006); the caller of Start is responsible for logging the
// Warn "aviso na tela indisponível" and continuing without an overlay. After a
// failed Start (or after Close) Show and Hide are silent no-ops.
type Overlay struct {
	log *slog.Logger

	mu      sync.Mutex // serialises Start and Close
	started bool
	closed  bool

	hwnd    atomic.Uintptr // set by the loop goroutine before Start returns; 0 after Close
	tid     uint32
	wg      sync.WaitGroup
	loopErr error // written by the loop goroutine, read after wg.Wait

	textMu sync.Mutex
	text   string // text of the last Show, read by the loop thread

	// postMu serialises posting messages with the loop closing exited.
	postMu sync.Mutex
	exited chan struct{}

	// Touched only by the loop thread.
	brush windows.Handle
	font  windows.Handle
	fontD int
	w, h  int
}

// NewOverlay returns an overlay; Start creates its window.
func NewOverlay(log *slog.Logger) *Overlay {
	return &Overlay{log: log, exited: make(chan struct{})}
}

// Start creates the window and its message loop. On error nothing is left
// running.
func (o *Overlay) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.started || o.closed {
		return errors.New("overlay: already started or closed")
	}
	ready := make(chan error, 1)
	o.wg.Add(1)
	go o.loop(ready)
	if err := <-ready; err != nil {
		o.wg.Wait()
		return err
	}
	o.started = true
	return nil
}

// Show displays the notice without activating the window. It never blocks.
func (o *Overlay) Show(n feedback.Notice) {
	o.textMu.Lock()
	o.text = n.Text()
	o.textMu.Unlock()
	o.post(msgOverlayShow)
}

// Hide hides the window. It never blocks.
func (o *Overlay) Hide() { o.post(msgOverlayHide) }

// post sends msg to the window thread; silent no-op when there is no window.
func (o *Overlay) post(msg uintptr) {
	o.postMu.Lock()
	defer o.postMu.Unlock()
	select {
	case <-o.exited:
		return
	default:
	}
	hwnd := o.hwnd.Load()
	if hwnd == 0 {
		return
	}
	if r, _, e := procPostMessageW.Call(hwnd, msg, 0, 0); r == 0 {
		o.log.Debug("overlay: PostMessageW failed", slog.Any("err", e))
	}
}

// Close hides and destroys the window, unregisters the class and waits for the
// loop. Idempotent.
func (o *Overlay) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil
	}
	o.closed = true
	if !o.started {
		return nil
	}
	if err := o.stopLoop(); err != nil {
		return err
	}
	o.wg.Wait()
	o.hwnd.Store(0)
	return o.loopErr
}

func (o *Overlay) stopLoop() error {
	o.postMu.Lock()
	defer o.postMu.Unlock()
	select {
	case <-o.exited:
		return nil
	default:
	}
	r, _, e := procPostMessageW.Call(o.hwnd.Load(), msgStop, 0, 0)
	if r != 0 {
		return nil
	}
	postErr := fmt.Errorf("overlay: PostMessageW: %w", e)
	r, _, e = procPostThreadMsgW.Call(uintptr(o.tid), msgQuit, 0, 0)
	if r != 0 {
		o.log.Warn("overlay: stop message fell back to WM_QUIT", slog.Any("err", postErr))
		return nil
	}
	return errors.Join(postErr, fmt.Errorf("overlay: PostThreadMessageW: %w", e))
}

// windowHandle returns the notice window; 0 when Start failed or after Close.
func (o *Overlay) windowHandle() windows.HWND { return windows.HWND(o.hwnd.Load()) }

func (o *Overlay) handleMessage(hwnd, msg, _, _ uintptr) (uintptr, bool) {
	switch msg {
	case msgOverlayShow:
		o.showOnMonitor(hwnd)
		return 0, true
	case msgOverlayHide:
		// SWP_HIDEWINDOW never activates.
		procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpHideWindow)
		return 0, true
	case msgPaint:
		o.paint(hwnd)
		return 0, true
	case msgEraseBkgnd:
		return 1, true
	case msgStop:
		procPostQuitMessage.Call(0)
		return 0, true
	case msgClose:
		o.log.Warn("ignoring external WM_CLOSE on overlay window")
		return 0, true
	}
	return 0, false
}

// showOnMonitor positions the window on the monitor of the foreground window
// and shows it without activation.
func (o *Overlay) showOnMonitor(hwnd uintptr) {
	fg, _, _ := procGetForegroundWindow.Call()
	mon, _, _ := procMonitorFromWindow.Call(fg, monitorPrimary)
	mi := monitorInfo{}
	mi.size = uint32(unsafe.Sizeof(mi))
	if r, _, e := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		o.log.Debug("overlay: GetMonitorInfoW failed", slog.Any("err", e))
		return
	}
	dpi := overlayBaseDPI
	var dx, dy uint32
	if hr, _, _ := procGetDpiForMonitor.Call(mon, mdtEffectiveDPI, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); hr == 0 && dx != 0 {
		dpi = int(dx)
	}
	o.ensureFont(dpi)
	w, h := overlaySize(dpi)
	o.w, o.h = w, h
	rc := overlayRect(Rect{Left: mi.work.left, Top: mi.work.top, Right: mi.work.right, Bottom: mi.work.bottom}, dpi, w, h)
	hwndTopmost := ^uintptr(0) // HWND_TOPMOST
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(int(rc.Left)), uintptr(int(rc.Top)), uintptr(w), uintptr(h),
		swpNoActivate|swpShowWindow)
	procInvalidateRect.Call(hwnd, 0, 1)
}

// ensureFont (re)creates the font when the monitor dpi changed. Loop thread only.
func (o *Overlay) ensureFont(dpi int) {
	if o.font != 0 && o.fontD == dpi {
		return
	}
	face, err := windows.UTF16PtrFromString(gdiFontFaceName)
	if err != nil {
		return
	}
	height := -(overlayFontPx * dpi / overlayBaseDPI)
	f, _, e := procCreateFontW.Call(uintptr(height), 0, 0, 0, fwNormal, 0, 0, 0, defaultCharset, 0, 0, clearTypeQuality, 0,
		uintptr(unsafe.Pointer(face)))
	if f == 0 {
		o.log.Debug("overlay: CreateFontW failed; using the default font", slog.Any("err", e))
		return
	}
	if o.font != 0 {
		procDeleteObject.Call(uintptr(o.font))
	}
	o.font, o.fontD = windows.Handle(f), dpi
}

func (o *Overlay) paint(hwnd uintptr) {
	var ps paintStruct
	r, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if r == 0 {
		return
	}
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	hdc := r
	rc := winRect{right: int32(o.w), bottom: int32(o.h)}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), uintptr(o.brush))
	o.textMu.Lock()
	text := o.text
	o.textMu.Unlock()
	if text == "" {
		return
	}
	p, err := windows.UTF16FromString(text)
	if err != nil {
		return
	}
	var old uintptr
	if o.font != 0 {
		old, _, _ = procSelectObject.Call(hdc, uintptr(o.font))
	}
	procSetBkMode.Call(hdc, transparentBk)
	procSetTextColor.Call(hdc, overlayTextColor)
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&p[0])), ^uintptr(0), uintptr(unsafe.Pointer(&rc)), dtCenterVCenter)
	if old != 0 {
		procSelectObject.Call(hdc, old)
	}
}

func (o *Overlay) loop(ready chan<- error) {
	defer o.wg.Done()
	runtime.LockOSThread()

	// Per-monitor V2 awareness for this thread only (OVL-004).
	if procSetThreadDpiAwarenessContext.Find() == nil {
		procSetThreadDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
	}

	var instance windows.Handle
	if err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &instance); err != nil {
		ready <- fmt.Errorf("overlay: module handle: %w", err)
		return
	}
	className, err := windows.UTF16PtrFromString(fmt.Sprintf("ArcadiaNotice-%d", classSeq.Add(1)))
	if err != nil {
		ready <- fmt.Errorf("overlay: class name: %w", err)
		return
	}
	wc := wndClassExW{
		wndProc:   sharedWndProc(),
		instance:  instance,
		className: className,
	}
	wc.size = uint32(unsafe.Sizeof(wc))
	if r, _, e := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		ready <- fmt.Errorf("overlay: RegisterClassExW: %w", e)
		return
	}
	unregister := func() error {
		if r, _, e := procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), uintptr(instance)); r == 0 {
			return fmt.Errorf("overlay: UnregisterClassW: %w", e)
		}
		return nil
	}

	b, _, e := procCreateSolidBrush.Call(overlayBgColor)
	if b == 0 {
		ready <- errors.Join(fmt.Errorf("overlay: CreateSolidBrush: %w", e), unregister())
		return
	}
	o.brush = windows.Handle(b)
	freeGDI := func() {
		if o.font != 0 {
			procDeleteObject.Call(uintptr(o.font))
			o.font = 0
		}
		procDeleteObject.Call(uintptr(o.brush))
		o.brush = 0
	}

	r, _, e := procCreateWindowExW.Call(wsExTopmost|wsExNoActivate|wsExToolWindow|wsExLayered|wsExTransp,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)),
		wsPopup, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if r == 0 {
		freeGDI()
		ready <- errors.Join(fmt.Errorf("overlay: CreateWindowExW: %w", e), unregister())
		return
	}
	hwnd := windows.HWND(r)
	if r, _, e := procSetLayeredWindowAttributes.Call(uintptr(hwnd), 0, overlayAlpha, lwaAlpha); r == 0 {
		procDestroyWindow.Call(uintptr(hwnd))
		freeGDI()
		ready <- errors.Join(fmt.Errorf("overlay: SetLayeredWindowAttributes: %w", e), unregister())
		return
	}
	registerWindowHandler(hwnd, o.handleMessage)
	o.tid = windows.GetCurrentThreadId()
	o.hwnd.Store(uintptr(hwnd))
	ready <- nil

	var errs []error
	var m msgW
	for {
		r, _, e := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) == 0 {
			break // WM_QUIT
		}
		if int32(r) == -1 {
			errs = append(errs, fmt.Errorf("overlay: GetMessageW: %w", e))
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	o.postMu.Lock()
	close(o.exited)
	o.postMu.Unlock()

	procShowWindow.Call(uintptr(hwnd), swHide)
	unregisterWindowHandler(hwnd)
	if r, _, e := procDestroyWindow.Call(uintptr(hwnd)); r == 0 {
		errs = append(errs, fmt.Errorf("overlay: DestroyWindow: %w", e))
	}
	if err := unregister(); err != nil {
		errs = append(errs, err)
	}
	freeGDI()
	o.loopErr = errors.Join(errs...)
}
