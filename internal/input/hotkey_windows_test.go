//go:build windows

package input

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"go.uber.org/goleak"
	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// testHotkey is Ctrl+Alt+Shift+F24: never the real user shortcut (Ctrl+Alt+P).
var testHotkey = Hotkey{Modifiers: ModControl | ModAlt | ModShift, VK: 0x87}

const (
	wmHotkey       = 0x0312
	hotkeyTestID   = 1
	hotkeyTimeout  = 2 * time.Second
	errHotkeyTaken = windows.Errno(1409) // ERROR_HOTKEY_ALREADY_REGISTERED
)

var hotkeyStart = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

var (
	hkUser32             = windows.NewLazySystemDLL("user32.dll")
	hkPostThreadMessageW = hkUser32.NewProc("PostThreadMessageW")
	hkSendInput          = hkUser32.NewProc("SendInput")
	hkOpenInputDesktop   = hkUser32.NewProc("OpenInputDesktop")
	hkCloseDesktop       = hkUser32.NewProc("CloseDesktop")
)

// logRecord is a captured slog record.
type logRecord struct {
	level slog.Level
	msg   string
	attrs map[string]string
}

// recLog captures records and signals each one on a channel.
type recLog struct {
	mu      sync.Mutex
	records []logRecord
	pre     []slog.Attr
	notify  chan logRecord
}

func newRecLog() *recLog { return &recLog{notify: make(chan logRecord, 64)} }

func (r *recLog) Enabled(context.Context, slog.Level) bool { return true }
func (r *recLog) Handle(_ context.Context, rec slog.Record) error {
	lr := logRecord{level: rec.Level, msg: rec.Message, attrs: map[string]string{}}
	r.mu.Lock()
	for _, a := range r.pre {
		lr.attrs[a.Key] = fmt.Sprint(a.Value.Any())
	}
	r.mu.Unlock()
	rec.Attrs(func(a slog.Attr) bool {
		lr.attrs[a.Key] = fmt.Sprint(a.Value.Any())
		return true
	})
	r.mu.Lock()
	r.records = append(r.records, lr)
	r.mu.Unlock()
	select {
	case r.notify <- lr:
	default:
	}
	return nil
}
func (r *recLog) WithAttrs(as []slog.Attr) slog.Handler {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pre = append(r.pre, as...)
	return r
}
func (r *recLog) WithGroup(string) slog.Handler { return r }

func (r *recLog) find(level slog.Level, msg string) (logRecord, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, lr := range r.records {
		if lr.level == level && lr.msg == msg {
			return lr, true
		}
	}
	return logRecord{}, false
}

func (r *recLog) waitFor(t *testing.T, level slog.Level, msg string) logRecord {
	t.Helper()
	deadline := time.After(hotkeyTimeout)
	for {
		if lr, ok := r.find(level, msg); ok {
			return lr
		}
		select {
		case <-r.notify:
		case <-deadline:
			t.Fatalf("log %v %q não apareceu", level, msg)
		}
	}
}

// requireInteractiveDesktop skips the test when there is no interactive desktop
// (service session, locked workstation, CI).
func requireInteractiveDesktop(t *testing.T) {
	t.Helper()
	const desktopSwitchDesktop = 0x0100
	h, _, _ := hkOpenInputDesktop.Call(0, 0, desktopSwitchDesktop)
	if h == 0 {
		t.Skip("sem desktop interativo (OpenInputDesktop falhou)")
	}
	hkCloseDesktop.Call(h)
}

func postHotkeyMessage(t *testing.T, tid uint32, id uintptr, h Hotkey) {
	t.Helper()
	lParam := uintptr(h.Modifiers&0xFFFF) | uintptr(h.VK)<<16
	r, _, err := hkPostThreadMessageW.Call(uintptr(tid), wmHotkey, id, lParam)
	if r == 0 {
		t.Fatalf("PostThreadMessageW: %v", err)
	}
}

// startHotkey builds a listener, starts it and returns it with its channel and log.
func startHotkey(t *testing.T, clock domain.Clock, chanCap int) (*hotkey, chan Trigger, *recLog) {
	t.Helper()
	rl := newRecLog()
	src := NewHotkey(testHotkey, clock, slog.New(rl))
	hk, ok := src.(*hotkey)
	if !ok {
		t.Fatalf("NewHotkey devolveu %T, esperava *hotkey", src)
	}
	out := make(chan Trigger, chanCap)
	if err := src.Start(out); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = src.Close() })
	return hk, out, rl
}

func recvTrigger(t *testing.T, out <-chan Trigger) Trigger {
	t.Helper()
	select {
	case tr := <-out:
		return tr
	case <-time.After(hotkeyTimeout):
		t.Fatal("gatilho não chegou")
		return Trigger{}
	}
}

type kbInput struct {
	Type      uint32
	_         uint32
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	_         uint32
	ExtraInfo uintptr
	_         [8]byte
}

func sendKey(t *testing.T, vk uint16, up bool) {
	t.Helper()
	in := kbInput{Type: 1, Vk: vk}
	if up {
		in.Flags = 0x0002 // KEYEVENTF_KEYUP
	}
	r, _, err := hkSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
	if r != 1 {
		t.Fatalf("SendInput: %v", err)
	}
}

func TestHotkey(t *testing.T) {
	// Not parallel: goleak inspects process-wide goroutines and the combination is process-global.
	t.Run("HOT-001/DefaultHotkey é Ctrl+Alt+P com VK 0x50 e registerModifiers inclui MOD_NOREPEAT", func(t *testing.T) {
		if DefaultHotkey.VK != 0x50 {
			t.Errorf("VK = %#x, quer 0x50", DefaultHotkey.VK)
		}
		if DefaultHotkey.Modifiers != ModControl|ModAlt {
			t.Errorf("Modifiers = %#x, quer Ctrl|Alt", DefaultHotkey.Modifiers)
		}
		if got := registerModifiers(DefaultHotkey); got != ModControl|ModAlt|ModNoRepeat {
			t.Errorf("registerModifiers = %#x, quer %#x", got, ModControl|ModAlt|ModNoRepeat)
		}
		if got := registerModifiers(testHotkey); got != testHotkey.Modifiers|ModNoRepeat {
			t.Errorf("registerModifiers(test) = %#x", got)
		}
	})

	t.Run("HOT-001/Start registra a combinação no processo: um segundo listener com a mesma combinação falha", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered first, so it runs after startHotkey's Close
		clock := domain.NewFakeClock(hotkeyStart)
		src := NewHotkey(testHotkey, clock, slog.New(newRecLog()))
		if err := src.Start(make(chan Trigger, 1)); err != nil {
			t.Fatalf("Start: %v", err)
		}
		second := NewHotkey(testHotkey, clock, slog.New(newRecLog()))
		err := second.Start(make(chan Trigger, 1))
		if err == nil {
			_ = second.Close()
			_ = src.Close()
			t.Fatal("segundo Start deveria falhar")
		}
		if !errors.Is(err, errHotkeyTaken) {
			t.Errorf("erro = %v, quer ERROR_HOTKEY_ALREADY_REGISTERED", err)
		}
		_ = src.Close()
	})

	t.Run("HOT-001/Info registrado com o atalho em texto", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered first, so it runs after startHotkey's Close
		_, _, rl := startHotkey(t, domain.NewFakeClock(hotkeyStart), 1)
		lr := rl.waitFor(t, slog.LevelInfo, "atalho de pausa registrado")
		found := strings.Contains(lr.msg, testHotkey.String())
		for _, v := range lr.attrs {
			if strings.Contains(v, testHotkey.String()) {
				found = true
			}
		}
		if !found {
			t.Errorf("Info sem o atalho em texto %q: %+v", testHotkey.String(), lr.attrs)
		}
	})

	t.Run("HOT-001/SendInput de Ctrl+Alt+Shift+F24 entrega um gatilho", func(t *testing.T) {
		requireInteractiveDesktop(t)
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered first, so it runs after startHotkey's Close
		clock := domain.NewFakeClock(hotkeyStart)
		_, out, _ := startHotkey(t, clock, 4)
		keys := []uint16{0x11, 0x12, 0x10, 0x87} // Ctrl, Alt, Shift, F24
		defer func() {
			for i := len(keys) - 1; i >= 0; i-- {
				sendKey(t, keys[i], true)
			}
		}()
		for _, k := range keys {
			sendKey(t, k, false)
		}
		tr := recvTrigger(t, out)
		if tr.Source != domain.PauseHotkey {
			t.Errorf("Source = %v", tr.Source)
		}
	})

	t.Run("HOT-002/WM_HOTKEY com o id entrega exatamente um Trigger{hotkey, clock.Now() do recebimento}", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered first, so it runs after startHotkey's Close
		clock := domain.NewFakeClock(hotkeyStart)
		hk, out, _ := startHotkey(t, clock, 4)

		postHotkeyMessage(t, hk.threadID(), hotkeyTestID, testHotkey)
		tr := recvTrigger(t, out)
		if tr.Source != domain.PauseHotkey || !tr.At.Equal(hotkeyStart) {
			t.Errorf("trigger = %+v, quer {hotkey, %v}", tr, hotkeyStart)
		}

		clock.Advance(5 * time.Second)
		postHotkeyMessage(t, hk.threadID(), hotkeyTestID, testHotkey)
		tr = recvTrigger(t, out)
		if want := hotkeyStart.Add(5 * time.Second); !tr.At.Equal(want) {
			t.Errorf("At = %v, quer %v", tr.At, want)
		}
		select {
		case extra := <-out:
			t.Errorf("gatilho extra: %+v", extra)
		case <-time.After(50 * time.Millisecond):
		}
	})

	t.Run("HOT-002/WM_HOTKEY com outro id não gera gatilho", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered first, so it runs after startHotkey's Close
		clock := domain.NewFakeClock(hotkeyStart)
		hk, out, _ := startHotkey(t, clock, 4)

		postHotkeyMessage(t, hk.threadID(), hotkeyTestID+1, testHotkey)
		clock.Advance(time.Second)
		postHotkeyMessage(t, hk.threadID(), hotkeyTestID, testHotkey) // sentinel, processed after the other
		tr := recvTrigger(t, out)
		if want := hotkeyStart.Add(time.Second); !tr.At.Equal(want) {
			t.Errorf("primeiro gatilho At = %v, quer %v (o id errado gerou gatilho)", tr.At, want)
		}
		select {
		case extra := <-out:
			t.Errorf("gatilho extra: %+v", extra)
		default:
		}
	})

	t.Run("HOT-002/fila cheia descarta com Warn e o loop continua respondendo", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered first, so it runs after startHotkey's Close
		clock := domain.NewFakeClock(hotkeyStart)
		hk, out, rl := startHotkey(t, clock, 1)

		postHotkeyMessage(t, hk.threadID(), hotkeyTestID, testHotkey) // fills the queue
		postHotkeyMessage(t, hk.threadID(), hotkeyTestID, testHotkey) // dropped
		rl.waitFor(t, slog.LevelWarn, "gatilho de pausa descartado: fila cheia")

		first := recvTrigger(t, out)
		if !first.At.Equal(hotkeyStart) {
			t.Errorf("At = %v", first.At)
		}
		clock.Advance(3 * time.Second)
		postHotkeyMessage(t, hk.threadID(), hotkeyTestID, testHotkey)
		next := recvTrigger(t, out)
		if want := hotkeyStart.Add(3 * time.Second); !next.At.Equal(want) {
			t.Errorf("depois do descarte At = %v, quer %v", next.At, want)
		}
	})

	t.Run("HOT-003/combinação já registrada devolve erro que embrulha ERROR_HOTKEY_ALREADY_REGISTERED (1409), nenhuma goroutine fica viva e o Error é logado com hotkey e winError", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered first, so it runs after startHotkey's Close
		clock := domain.NewFakeClock(hotkeyStart)
		holder := NewHotkey(testHotkey, clock, slog.New(newRecLog()))
		if err := holder.Start(make(chan Trigger, 1)); err != nil {
			t.Fatalf("Start do detentor: %v", err)
		}

		rl := newRecLog()
		loser := NewHotkey(testHotkey, clock, slog.New(rl))
		err := loser.Start(make(chan Trigger, 1))
		if err == nil {
			_ = loser.Close()
			_ = holder.Close()
			t.Fatal("Start deveria falhar")
		}
		if !errors.Is(err, errHotkeyTaken) {
			t.Errorf("erro = %v, quer wrap de 1409", err)
		}
		lr, ok := rl.find(slog.LevelError, "atalho de pausa indisponível")
		if !ok {
			t.Fatal("Error \"atalho de pausa indisponível\" não logado pelo Start")
		}
		if _, ok := lr.attrs["hotkey"]; !ok {
			t.Errorf("Error sem atributo hotkey: %+v", lr.attrs)
		}
		if v, ok := lr.attrs["winError"]; !ok {
			t.Errorf("Error sem atributo winError: %+v", lr.attrs)
		} else if !strings.Contains(v, "1409") && !strings.Contains(strings.ToLower(v), "hot key") && !strings.Contains(strings.ToLower(v), "hotkey") {
			t.Errorf("winError = %q, esperava referência ao 1409", v)
		}
		if err := loser.Close(); err != nil {
			t.Errorf("Close após Start falho: %v", err)
		}
		_ = holder.Close()
	})

	t.Run("HOT-004/Close desregistra, a goroutine termina antes de Close retornar (goleak) e outro listener registra a mesma combinação", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered first, so it runs after startHotkey's Close
		clock := domain.NewFakeClock(hotkeyStart)
		first := NewHotkey(testHotkey, clock, slog.New(newRecLog()))
		if err := first.Start(make(chan Trigger, 1)); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if err := first.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		goleak.VerifyNone(t) // immediately after Close, no waiting

		again := NewHotkey(testHotkey, clock, slog.New(newRecLog()))
		if err := again.Start(make(chan Trigger, 1)); err != nil {
			t.Fatalf("segundo listener não registrou após Close: %v", err)
		}
		_ = again.Close()
	})

	t.Run("HOT-004/segunda chamada a Close não tem efeito", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered first, so it runs after startHotkey's Close
		src := NewHotkey(testHotkey, domain.NewFakeClock(hotkeyStart), slog.New(newRecLog()))
		if err := src.Start(make(chan Trigger, 1)); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if err := src.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- src.Close() }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("segundo Close = %v, quer nil", err)
			}
		case <-time.After(hotkeyTimeout):
			t.Fatal("segundo Close travou")
		}
	})

	t.Run("HOT-004/Close concorrente com Start não trava e não deixa goroutine", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered before any Start
		clock := domain.NewFakeClock(hotkeyStart)
		for i := 0; i < 50; i++ {
			src := NewHotkey(testHotkey, clock, slog.New(newRecLog()))
			out := make(chan Trigger, 1)
			startDone := make(chan error, 1)
			closeDone := make(chan error, 1)
			go func() { startDone <- src.Start(out) }()
			go func() { closeDone <- src.Close() }()
			for _, ch := range []chan error{startDone, closeDone} {
				select {
				case <-ch:
				case <-time.After(10 * time.Second):
					t.Fatalf("iteração %d: Start/Close concorrentes travaram", i)
				}
			}
			// Whatever the interleaving, a final Close must settle everything.
			closeAgain := make(chan error, 1)
			go func() { closeAgain <- src.Close() }()
			select {
			case <-closeAgain:
			case <-time.After(10 * time.Second):
				t.Fatalf("iteração %d: Close final travou", i)
			}
		}
		if err := goleak.Find(); err != nil {
			t.Fatalf("goroutines vazadas após as iterações: %v", err)
		}

		next := NewHotkey(testHotkey, clock, slog.New(newRecLog()))
		if err := next.Start(make(chan Trigger, 1)); err != nil {
			t.Fatalf("Start após as iterações concorrentes: %v", err)
		}
		if err := next.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	t.Run("HOT-004/Start depois de Close devolve erro e não deixa goroutine", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered before any Start
		src := NewHotkey(testHotkey, domain.NewFakeClock(hotkeyStart), slog.New(newRecLog()))
		if err := src.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- src.Start(make(chan Trigger, 1)) }()
		select {
		case err := <-done:
			if err == nil {
				_ = src.Close()
				t.Fatal("Start após Close deveria devolver erro")
			}
		case <-time.After(hotkeyTimeout):
			t.Fatal("Start após Close travou")
		}
		_ = src.Close()
		if err := goleak.Find(); err != nil {
			t.Errorf("goroutine viva após Start pós-Close: %v", err)
		}
	})

	t.Run("HOT-003/NewHotkey com log nil não entra em pânico quando o registro falha", func(t *testing.T) {
		t.Cleanup(func() { goleak.VerifyNone(t) }) // registered before any Start
		clock := domain.NewFakeClock(hotkeyStart)
		holder := NewHotkey(testHotkey, clock, slog.New(newRecLog()))
		if err := holder.Start(make(chan Trigger, 1)); err != nil {
			t.Fatalf("Start do detentor: %v", err)
		}
		defer func() { _ = holder.Close() }()

		loser := NewHotkey(testHotkey, clock, nil)
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Start com log nil entrou em pânico: %v", r)
				}
			}()
			err = loser.Start(make(chan Trigger, 1))
		}()
		if err == nil {
			_ = loser.Close()
			t.Fatal("Start deveria falhar")
		}
		if !errors.Is(err, errHotkeyTaken) {
			t.Errorf("erro = %v, quer wrap de 1409", err)
		}
		_ = loser.Close()
	})
}
