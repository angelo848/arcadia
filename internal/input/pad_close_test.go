package input_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/input"
)

// drain returns every trigger currently queued.
func drain(out <-chan input.Trigger) []input.Trigger {
	var got []input.Trigger
	for {
		select {
		case tr := <-out:
			got = append(got, tr)
		default:
			return got
		}
	}
}

func countAction(trs []input.Trigger, a input.Action) int {
	n := 0
	for _, tr := range trs {
		if tr.Action == a {
			n++
		}
	}
	return n
}

// startPad starts a poller with cfg over api; Close is registered as cleanup.
func startPad(t *testing.T, api *fakePad, cfg input.PadConfig, qcap int) (*domain.FakeClock, chan input.Trigger, *warnLog) {
	t.Helper()
	clock := domain.NewFakeClock(padStart)
	out := make(chan input.Trigger, qcap)
	wl := &warnLog{}
	p := input.NewPad(api, cfg, clock, slog.New(wl))
	if err := p.Start(out); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return clock, out, wl
}

// TestPadClose covers the Back+B detector (phase 13, T07). Not parallel: each case drives its own fake clock.
func TestPadClose(t *testing.T) {
	closeCombo := input.ButtonBack | input.ButtonB
	pauseCombo := input.ButtonBack | input.ButtonY

	t.Run("CON-001/Back+B segurado dispara Trigger{ActionClose, gamepad} na primeira leitura com agora-T >= 1 s (T+992ms não, T+1008ms sim)", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: closeCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 4)

		steps(t, clock, 62)
		noTrigger(t, out, "at T+992ms")
		step(t, clock)
		select {
		case tr := <-out:
			if tr.Source != domain.PauseGamepad || tr.Action != input.ActionClose {
				t.Errorf("trigger = %+v, want {gamepad, ActionClose}", tr)
			}
			if want := padStart.Add(1008 * time.Millisecond); !tr.At.Equal(want) {
				t.Errorf("At = %v, want %v", tr.At, want)
			}
		case <-time.After(padTimeout):
			t.Fatal("no trigger at T+1008ms")
		}
	})

	t.Run("CON-001/a espera padrão é 1 s (x-console.closeHoldMs); ActionPause é o valor zero; ButtonB é 0x2000", func(t *testing.T) {
		if input.ActionPause != input.Action(0) {
			t.Errorf("ActionPause = %d, want the zero value", input.ActionPause)
		}
		if input.ButtonB != 0x2000 {
			t.Errorf("ButtonB = %#x, want 0x2000", input.ButtonB)
		}
		api := newFakePad(map[int]uint16{0: closeCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 4)
		steps(t, clock, 62)
		noTrigger(t, out, "before 1 s")
		step(t, clock)
		if got := drain(out); len(got) != 1 {
			t.Fatalf("triggers at 1 s = %v, want 1", got)
		}
	})

	t.Run("CON-001/soltar antes cancela", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: closeCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 4)
		steps(t, clock, 40)
		api.set(0, true, input.ButtonBack) // B released
		steps(t, clock, 100)
		noTrigger(t, out, "after releasing B early")
		api.set(0, true, closeCombo) // pressing again restarts the count
		steps(t, clock, 63)          // the first read after the change is the press instant
		noTrigger(t, out, "63 steps after the new press")
		step(t, clock)
		if got := drain(out); len(got) != 1 || got[0].Action != input.ActionClose {
			t.Fatalf("triggers = %v, want one ActionClose", got)
		}
	})

	t.Run("CON-001/um gatilho por pressão", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: closeCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 8)
		steps(t, clock, 63)
		if got := drain(out); len(got) != 1 {
			t.Fatalf("first press: triggers = %v, want 1", got)
		}
		steps(t, clock, 200)
		noTrigger(t, out, "while still held")
		api.set(0, true, 0)
		step(t, clock)
		api.set(0, true, closeCombo)
		steps(t, clock, 64)
		if got := drain(out); len(got) != 1 {
			t.Fatalf("second press: triggers = %v, want 1", got)
		}
	})

	t.Run("CON-001/controles independentes", func(t *testing.T) {
		// Slot 1 holds Back and then B on the same pad only later; slot 0 must not lend its buttons.
		api := newFakePad(map[int]uint16{0: closeCombo, 1: input.ButtonBack})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 8)
		steps(t, clock, 63)
		got := drain(out)
		if len(got) != 1 || got[0].Action != input.ActionClose {
			t.Fatalf("triggers = %v, want exactly one (slot 0)", got)
		}
		api.set(1, true, closeCombo)
		steps(t, clock, 63) // the first read after the change is the press instant
		noTrigger(t, out, "slot 1 before its own 1 s")
		step(t, clock)
		if got := drain(out); len(got) != 1 {
			t.Fatalf("slot 1 triggers = %v, want 1 at its own 1 s", got)
		}
	})

	t.Run("CON-001/desconexão zera a contagem", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: closeCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 4)
		steps(t, clock, 37)
		api.set(0, false, 0)
		step(t, clock)
		tDisc := padStart.Add(608 * time.Millisecond)
		api.set(0, true, closeCombo)
		var got *input.Trigger
		for i := 0; i < 400 && got == nil; i++ {
			step(t, clock)
			select {
			case tr := <-out:
				got = &tr
			default:
			}
		}
		if got == nil {
			t.Fatal("no trigger after reconnect")
		}
		if got.Action != input.ActionClose || got.At.Sub(tDisc) < time.Second {
			t.Errorf("trigger %+v at disconnect+%v, want ActionClose at >= 1 s", *got, got.At.Sub(tDisc))
		}
	})

	t.Run("CON-002/Back+Y só entrega ActionPause", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: pauseCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 4)
		steps(t, clock, 100)
		got := drain(out)
		if len(got) != 1 || got[0].Action != input.ActionPause || got[0].Source != domain.PauseGamepad {
			t.Fatalf("triggers = %v, want one {gamepad, ActionPause}", got)
		}
	})

	t.Run("CON-002/Back+B só entrega ActionClose", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: closeCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 4)
		steps(t, clock, 100)
		got := drain(out)
		if len(got) != 1 || got[0].Action != input.ActionClose {
			t.Fatalf("triggers = %v, want one ActionClose", got)
		}
	})

	t.Run("CON-002/Back+Y+B entrega os dois, cada um uma vez", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: pauseCombo | input.ButtonB})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 8)
		steps(t, clock, 100)
		got := drain(out)
		if len(got) != 2 || countAction(got, input.ActionPause) != 1 || countAction(got, input.ActionClose) != 1 {
			t.Fatalf("triggers = %v, want one pause and one close", got)
		}
	})

	t.Run("CON-002/soltar B não zera a contagem de Back+Y e vice-versa", func(t *testing.T) {
		both := pauseCombo | input.ButtonB
		t.Run("solta B", func(t *testing.T) {
			api := newFakePad(map[int]uint16{0: both})
			clock, out, _ := startPad(t, api, input.PadConfig{}, 8)
			steps(t, clock, 40)
			api.set(0, true, pauseCombo)
			steps(t, clock, 23)
			got := drain(out)
			if len(got) != 1 || got[0].Action != input.ActionPause || !got[0].At.Equal(padStart.Add(1008*time.Millisecond)) {
				t.Fatalf("triggers = %v, want one ActionPause at T+1008ms", got)
			}
		})
		t.Run("solta Y", func(t *testing.T) {
			api := newFakePad(map[int]uint16{0: both})
			clock, out, _ := startPad(t, api, input.PadConfig{}, 8)
			steps(t, clock, 40)
			api.set(0, true, closeCombo)
			steps(t, clock, 23)
			got := drain(out)
			if len(got) != 1 || got[0].Action != input.ActionClose || !got[0].At.Equal(padStart.Add(1008*time.Millisecond)) {
				t.Fatalf("triggers = %v, want one ActionClose at T+1008ms", got)
			}
		})
	})

	t.Run("CON-002/PadConfig.Mask e CloseMask personalizadas", func(t *testing.T) {
		const maskP, maskC uint16 = 0x0001 | 0x0002, 0x0004 | 0x0008
		api := newFakePad(map[int]uint16{0: maskP | maskC})
		clock, out, _ := startPad(t, api, input.PadConfig{Mask: maskP, CloseMask: maskC}, 8)
		steps(t, clock, 63)
		got := drain(out)
		if len(got) != 2 || countAction(got, input.ActionPause) != 1 || countAction(got, input.ActionClose) != 1 {
			t.Fatalf("triggers = %v, want one pause and one close from the custom masks", got)
		}
		// The defaults must not fire under custom masks.
		api2 := newFakePad(map[int]uint16{0: closeCombo | input.ButtonY})
		clock2, out2, _ := startPad(t, api2, input.PadConfig{Mask: maskP, CloseMask: maskC}, 8)
		steps(t, clock2, 100)
		noTrigger(t, out2, "default combos under custom masks")
	})

	t.Run("CON-002/fila de saída cheia descarta com Warn", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: closeCombo})
		clock, out, wl := startPad(t, api, input.PadConfig{}, 1)
		out <- input.Trigger{Source: domain.PauseHotkey, At: padStart}
		done := make(chan struct{})
		go func() {
			defer close(done)
			steps(t, clock, 63)
		}()
		select {
		case <-done:
		case <-time.After(padTimeout):
			t.Fatal("poller blocked on a full output queue")
		}
		if wl.count() < 1 {
			t.Error("no Warn logged for the dropped close trigger")
		}
		if len(out) != 1 || (<-out).Source != domain.PauseHotkey {
			t.Error("the dropped trigger must not displace what was queued")
		}
	})
}
