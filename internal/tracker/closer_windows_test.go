//go:build windows

package tracker

import (
	"errors"
	"slices"
	"testing"
)

const wmClose = 0x10

type fakeWin struct {
	h       uintptr
	visible bool
	owner   uintptr
	pid     uint32
}

type fakePost struct {
	hwnd uintptr
	msg  uint32
	w, l uintptr
}

// fakeCloserAPI is a scripted closerAPI; it never touches the real desktop.
type fakeCloserAPI struct {
	wins      []fakeWin
	enumErr   error
	enumCalls int
	posts     []fakePost
}

func (a *fakeCloserAPI) find(h uintptr) fakeWin {
	for _, w := range a.wins {
		if w.h == h {
			return w
		}
	}
	return fakeWin{}
}

func (a *fakeCloserAPI) EnumTopLevel(fn func(hwnd uintptr) bool) error {
	a.enumCalls++
	if a.enumErr != nil {
		return a.enumErr
	}
	for _, w := range a.wins {
		if !fn(w.h) {
			break
		}
	}
	return nil
}
func (a *fakeCloserAPI) IsVisible(h uintptr) bool { return a.find(h).visible }
func (a *fakeCloserAPI) Owner(h uintptr) uintptr  { return a.find(h).owner }
func (a *fakeCloserAPI) ProcessOf(h uintptr) uint32 {
	return a.find(h).pid
}
func (a *fakeCloserAPI) Post(h uintptr, msg uint32, w, l uintptr) error {
	a.posts = append(a.posts, fakePost{h, msg, w, l})
	return nil
}

func TestWindowCloser(t *testing.T) {
	t.Parallel()

	t.Run("CON-030/o fechador só posta WM_CLOSE (0x10, wParam e lParam 0) a janelas de topo, visíveis e sem dono, de PIDs do conjunto", func(t *testing.T) {
		t.Parallel()
		api := &fakeCloserAPI{wins: []fakeWin{
			{h: 1, visible: true, pid: 10},            // posted
			{h: 2, visible: false, pid: 10},           // hidden
			{h: 3, visible: true, owner: 99, pid: 10}, // owned (dialog)
			{h: 4, visible: true, pid: 11},            // posted
		}}
		n, err := newWindowCloserWith(api).PostClose(map[uint32]struct{}{10: {}, 11: {}})
		if err != nil {
			t.Fatalf("PostClose: %v", err)
		}
		want := []fakePost{{1, wmClose, 0, 0}, {4, wmClose, 0, 0}}
		if n != 2 || !slices.Equal(api.posts, want) {
			t.Errorf("posted = %d %+v, want 2 %+v", n, api.posts, want)
		}
	})

	t.Run("CON-030/janela de PID fora do conjunto (o Arcadia, o cliente da loja) nunca recebe", func(t *testing.T) {
		t.Parallel()
		api := &fakeCloserAPI{wins: []fakeWin{
			{h: 1, visible: true, pid: 500}, // Arcadia
			{h: 2, visible: true, pid: 600}, // store client
		}}
		n, err := newWindowCloserWith(api).PostClose(map[uint32]struct{}{10: {}})
		if err != nil || n != 0 || len(api.posts) != 0 {
			t.Errorf("PostClose = (%d, %v), posts %+v; want 0, nil, none", n, err, api.posts)
		}
	})

	t.Run("CON-030/erro ao enumerar devolve 0 e o erro", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("enum failed")
		api := &fakeCloserAPI{enumErr: boom, wins: []fakeWin{{h: 1, visible: true, pid: 10}}}
		n, err := newWindowCloserWith(api).PostClose(map[uint32]struct{}{10: {}})
		if n != 0 || !errors.Is(err, boom) {
			t.Errorf("PostClose = (%d, %v), want 0 and the enum error", n, err)
		}
	})

	t.Run("CON-030/conjunto vazio devolve 0 sem enumerar", func(t *testing.T) {
		t.Parallel()
		api := &fakeCloserAPI{wins: []fakeWin{{h: 1, visible: true, pid: 10}}}
		for _, set := range []map[uint32]struct{}{nil, {}} {
			n, err := newWindowCloserWith(api).PostClose(set)
			if n != 0 || err != nil {
				t.Errorf("PostClose(%v) = (%d, %v), want 0, nil", set, n, err)
			}
		}
		if api.enumCalls != 0 {
			t.Errorf("EnumTopLevel called %d time(s), want 0", api.enumCalls)
		}
	})

	t.Run("CON-030/o callback do EnumWindows é criado uma vez (duas chamadas seguidas, um só NewCallback)", func(t *testing.T) {
		t.Parallel()
		// Behavioural half: two calls on the same closer keep working with the
		// same enumeration seam. The single NewCallback lives in the real
		// closerAPI and is checked by TestRealCloserAPICallbackOnce.
		api := &fakeCloserAPI{wins: []fakeWin{{h: 1, visible: true, pid: 10}}}
		c := newWindowCloserWith(api)
		for i := 0; i < 2; i++ {
			if n, err := c.PostClose(map[uint32]struct{}{10: {}}); n != 1 || err != nil {
				t.Fatalf("call %d: PostClose = (%d, %v), want 1, nil", i, n, err)
			}
		}
		if api.enumCalls != 2 {
			t.Errorf("EnumTopLevel calls = %d, want 2", api.enumCalls)
		}
	})
}

// TestRealCloserAPICallbackOnce runs the real closer twice against a PID that
// owns no window: nothing is posted and no error occurs, repeatedly (a callback
// created per call would exhaust the finite syscall callback slots).
func TestRealCloserAPICallbackOnce(t *testing.T) {
	t.Parallel()
	c := newWindowCloser()
	for i := 0; i < 2000; i++ {
		// PID 0xFFFFFFF0 does not exist, so no window can match.
		n, err := c.PostClose(map[uint32]struct{}{0xFFFFFFF0: {}})
		if err != nil || n != 0 {
			t.Fatalf("CON-030/call %d: PostClose = (%d, %v), want 0, nil", i, n, err)
		}
	}
}
