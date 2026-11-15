//go:build windows

package tracker

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Window messages and GetWindow commands (winuser.h).
const (
	msgWMClose = 0x0010
	gwOwner    = 4
)

var (
	modUser32       = windows.NewLazySystemDLL("user32.dll")
	procGetWindow   = modUser32.NewProc("GetWindow")
	procPostMessage = modUser32.NewProc("PostMessageW")
)

// closerAPI is the seam between the window closer and user32, so that tests
// never touch the real desktop.
type closerAPI interface {
	// EnumTopLevel calls fn for every top-level window until fn returns false.
	EnumTopLevel(fn func(hwnd uintptr) bool) error
	IsVisible(hwnd uintptr) bool
	Owner(hwnd uintptr) uintptr
	ProcessOf(hwnd uintptr) uint32
	// Post posts msg without waiting (PostMessageW; never SendMessage).
	Post(hwnd uintptr, msg uint32, w, l uintptr) error
}

// newWindowCloser returns the closer over the real user32 API.
func newWindowCloser() windowCloser { return newWindowCloserWith(user32API{}) }

// newWindowCloserWith returns a closer over api.
func newWindowCloserWith(api closerAPI) windowCloser { return apiCloser{api: api} }

type apiCloser struct{ api closerAPI }

// PostClose posts WM_CLOSE (wParam and lParam 0) to every top-level window
// that is visible, has no owner and belongs to a process of pids (CON-030).
// Child windows are never enumerated. A failed post is skipped; it becomes the
// error only when no window got the message.
func (c apiCloser) PostClose(pids map[uint32]struct{}) (int, error) {
	if len(pids) == 0 {
		return 0, nil
	}
	var hwnds []uintptr
	if err := c.api.EnumTopLevel(func(h uintptr) bool {
		hwnds = append(hwnds, h)
		return true
	}); err != nil {
		return 0, fmt.Errorf("tracker: enumerate windows: %w", err)
	}
	posted := 0
	var errs []error
	for _, h := range hwnds {
		if !c.api.IsVisible(h) || c.api.Owner(h) != 0 {
			continue
		}
		if _, ok := pids[c.api.ProcessOf(h)]; !ok {
			continue
		}
		if err := c.api.Post(h, msgWMClose, 0, 0); err != nil {
			errs = append(errs, fmt.Errorf("post WM_CLOSE to window %#x: %w", h, err))
			continue
		}
		posted++
	}
	if posted == 0 && len(errs) > 0 {
		return 0, fmt.Errorf("tracker: %w", errors.Join(errs...))
	}
	return posted, nil
}

// The EnumWindows callback is created once for the whole package:
// windows.NewCallback never frees its slot and the slots are finite. The scan
// state lives in package variables guarded by enumMu, held for the whole
// EnumWindows call; the callback runs synchronously on the calling thread.
var (
	enumOnce     sync.Once
	enumCallback uintptr
	enumMu       sync.Mutex
	enumFn       func(hwnd uintptr) bool
	enumStopped  bool
)

func enumWindowsProc(hwnd uintptr, _ uintptr) uintptr {
	if enumFn != nil && enumFn(hwnd) {
		return 1
	}
	enumStopped = true
	return 0
}

// user32API is the real closerAPI.
type user32API struct{}

func (user32API) EnumTopLevel(fn func(hwnd uintptr) bool) error {
	enumOnce.Do(func() { enumCallback = windows.NewCallback(enumWindowsProc) })
	enumMu.Lock()
	defer enumMu.Unlock()
	enumFn, enumStopped = fn, false
	defer func() { enumFn = nil }()
	err := windows.EnumWindows(enumCallback, nil)
	if err != nil && !enumStopped {
		return fmt.Errorf("EnumWindows: %w", err)
	}
	return nil
}

func (user32API) IsVisible(hwnd uintptr) bool { return windows.IsWindowVisible(windows.HWND(hwnd)) }

func (user32API) Owner(hwnd uintptr) uintptr {
	r, _, _ := procGetWindow.Call(hwnd, gwOwner)
	return r
}

func (user32API) ProcessOf(hwnd uintptr) uint32 {
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid); err != nil {
		return 0
	}
	return pid
}

func (user32API) Post(hwnd uintptr, msg uint32, w, l uintptr) error {
	if r, _, err := procPostMessage.Call(hwnd, uintptr(msg), w, l); r == 0 {
		return fmt.Errorf("PostMessageW: %w", err)
	}
	return nil
}

// newProcessKiller returns the real processKiller.
func newProcessKiller() processKiller { return winProcessKiller{} }

type winProcessKiller struct{}

// Kill opens pid, refuses when its image path is not wantImage (a recycled PID)
// and terminates it with exit code 1 (CON-008). The handle is always closed.
func (winProcessKiller) Kill(pid uint32, wantImage string) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("tracker: open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	got, err := imagePath(h)
	if err != nil {
		return fmt.Errorf("tracker: image of process %d: %w", pid, err)
	}
	if !samePath(got, wantImage) {
		return fmt.Errorf("tracker: process %d is %q, not %q: refused to terminate", pid, got, wantImage)
	}
	if err := windows.TerminateProcess(h, 1); err != nil {
		return fmt.Errorf("tracker: terminate process %d: %w", pid, err)
	}
	return nil
}

// imagePath returns the full image path of the process h.
func imagePath(h windows.Handle) (string, error) {
	var small [windows.MAX_PATH]uint16
	buf := small[:]
	for {
		size := uint32(len(buf))
		err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
		if err == nil {
			return windows.UTF16ToString(buf[:size]), nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || len(buf) >= windows.MAX_LONG_PATH {
			return "", fmt.Errorf("QueryFullProcessImageName: %w", err)
		}
		buf = make([]uint16, windows.MAX_LONG_PATH)
	}
}

func samePath(a, b string) bool {
	return a != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// jobProcessIDList is the header of JOBOBJECT_BASIC_PROCESS_ID_LIST.
type jobProcessIDList struct {
	NumberOfAssignedProcesses uint32
	NumberOfProcessIdsInList  uint32
	ProcessIdList             [1]uintptr
}

// jobPIDs returns the PIDs of the processes in job. The buffer grows while the
// job holds more processes than the list returned.
func jobPIDs(job windows.Handle) ([]uint32, error) {
	const word = unsafe.Sizeof(uintptr(0))
	header := int(unsafe.Offsetof(jobProcessIDList{}.ProcessIdList) / word)
	n := 64
	for {
		buf := make([]uintptr, header+n)
		err := windows.QueryInformationJobObject(job, windows.JobObjectBasicProcessIdList,
			uintptr(unsafe.Pointer(&buf[0])), uint32(uintptr(len(buf))*word), nil)
		if errors.Is(err, windows.ERROR_MORE_DATA) {
			n *= 2
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("query job process list: %w", err)
		}
		list := (*jobProcessIDList)(unsafe.Pointer(&buf[0]))
		if int(list.NumberOfAssignedProcesses) > int(list.NumberOfProcessIdsInList) {
			n = int(list.NumberOfAssignedProcesses) + 16
			continue
		}
		count := min(int(list.NumberOfProcessIdsInList), n)
		pids := make([]uint32, 0, count)
		for _, p := range buf[header : header+count] {
			pids = append(pids, uint32(p))
		}
		return pids, nil
	}
}
