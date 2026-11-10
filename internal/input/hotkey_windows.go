//go:build windows

package input

import (
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

const (
	wmHotkeyMsg = 0x0312
	wmQuitMsg   = 0x0012
	pmNoRemove  = 0x0000
	hotkeyID    = 1
)

var (
	inUser32         = windows.NewLazySystemDLL("user32.dll")
	procPeekMessage  = inUser32.NewProc("PeekMessageW")
	procGetMessage   = inUser32.NewProc("GetMessageW")
	procRegisterHK   = inUser32.NewProc("RegisterHotKey")
	procUnregisterHK = inUser32.NewProc("UnregisterHotKey")
	procPostThreadMs = inUser32.NewProc("PostThreadMessageW")
)

// winMsg mirrors the Win32 MSG structure.
type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

// hotkey is the RegisterHotKey-based Source. The registration lives on a
// dedicated OS thread that owns the message queue.
type hotkey struct {
	hk    Hotkey
	clock domain.Clock
	log   *slog.Logger

	tid       uint32
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewHotkey returns a Source that fires when the global shortcut h is pressed.
func NewHotkey(h Hotkey, clock domain.Clock, log *slog.Logger) Source {
	return &hotkey{hk: h, clock: clock, log: log}
}

func registerModifiers(h Hotkey) uint32 { return h.Modifiers | ModNoRepeat }

func (s *hotkey) Name() string { return "hotkey" }

// threadID is the id of the thread that owns the registration (0 before Start).
func (s *hotkey) threadID() uint32 { return s.tid }

type hotkeyReady struct {
	tid uint32
	err error
}

func (s *hotkey) Start(out chan<- Trigger) error {
	ready := make(chan hotkeyReady, 1)
	s.wg.Add(1)
	go s.loop(out, ready)
	r := <-ready
	if r.err != nil {
		s.wg.Wait()
		s.log.Error("atalho de pausa indisponível", "hotkey", s.hk.String(), "winError", r.err.Error())
		return fmt.Errorf("registrar atalho %s: %w", s.hk, r.err)
	}
	s.tid = r.tid
	s.log.Info("atalho de pausa registrado", "hotkey", s.hk.String())
	return nil
}

func (s *hotkey) loop(out chan<- Trigger, ready chan<- hotkeyReady) {
	defer s.wg.Done()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var msg winMsg
	// Force creation of the thread message queue before anyone posts to it.
	procPeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmNoRemove)

	r, _, err := procRegisterHK.Call(0, hotkeyID, uintptr(registerModifiers(s.hk)), uintptr(s.hk.VK))
	if r == 0 {
		ready <- hotkeyReady{err: fmt.Errorf("RegisterHotKey: %w", err)}
		return
	}
	defer procUnregisterHK.Call(0, hotkeyID)
	ready <- hotkeyReady{tid: windows.GetCurrentThreadId()}

	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 { // WM_QUIT or error
			return
		}
		if msg.Message != wmHotkeyMsg || msg.WParam != hotkeyID {
			continue
		}
		t := Trigger{Source: domain.PauseHotkey, At: s.clock.Now()}
		select {
		case out <- t:
		default:
			s.log.Warn("gatilho de pausa descartado: fila cheia")
		}
	}
}

func (s *hotkey) Close() error {
	s.closeOnce.Do(func() {
		if s.tid != 0 {
			procPostThreadMs.Call(uintptr(s.tid), wmQuitMsg, 0, 0)
		}
		s.wg.Wait()
	})
	return nil
}
