package input_test

import (
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/input"
)

// TestPadBigBox covers the Back+A detector (phase 15, T21). Not parallel: each case drives its own fake clock.
func TestPadBigBox(t *testing.T) {
	bigCombo := input.ButtonBack | input.ButtonA
	closeCombo := input.ButtonBack | input.ButtonB
	pauseCombo := input.ButtonBack | input.ButtonY

	t.Run("ATB-001/Back+A segurado dispara Trigger{ActionBigBox, gamepad} na primeira leitura com agora-T >= 1 s (T+992ms não, T+1008ms sim)", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: bigCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 4)
		steps(t, clock, 62)
		noTrigger(t, out, "at T+992ms")
		step(t, clock)
		select {
		case tr := <-out:
			if tr.Source != domain.PauseGamepad || tr.Action != input.ActionBigBox {
				t.Errorf("trigger = %+v, want {gamepad, ActionBigBox}", tr)
			}
			if want := padStart.Add(1008 * time.Millisecond); !tr.At.Equal(want) {
				t.Errorf("At = %v, want %v", tr.At, want)
			}
		case <-time.After(padTimeout):
			t.Fatal("no trigger at T+1008ms")
		}
	})

	t.Run("ATB-001/soltar antes cancela", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: bigCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 4)
		steps(t, clock, 40)
		api.set(0, true, input.ButtonBack)
		steps(t, clock, 100)
		noTrigger(t, out, "after releasing A early")
		api.set(0, true, bigCombo)
		steps(t, clock, 63)
		noTrigger(t, out, "63 steps after the new press")
		step(t, clock)
		if got := drain(out); len(got) != 1 || got[0].Action != input.ActionBigBox {
			t.Fatalf("triggers = %v, want one ActionBigBox", got)
		}
	})

	t.Run("ATB-001/um gatilho por pressão", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: bigCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 8)
		steps(t, clock, 63)
		if got := drain(out); len(got) != 1 {
			t.Fatalf("first press: triggers = %v, want 1", got)
		}
		steps(t, clock, 200)
		noTrigger(t, out, "while still held")
		api.set(0, true, 0)
		step(t, clock)
		api.set(0, true, bigCombo)
		steps(t, clock, 64)
		if got := drain(out); len(got) != 1 {
			t.Fatalf("second press: triggers = %v, want 1", got)
		}
	})

	t.Run("ATB-001/controles independentes", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: bigCombo, 1: input.ButtonBack})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 8)
		steps(t, clock, 63)
		got := drain(out)
		if len(got) != 1 || got[0].Action != input.ActionBigBox {
			t.Fatalf("triggers = %v, want exactly one (slot 0)", got)
		}
		api.set(1, true, bigCombo)
		steps(t, clock, 63)
		noTrigger(t, out, "slot 1 before its own 1 s")
		step(t, clock)
		if got := drain(out); len(got) != 1 {
			t.Fatalf("slot 1 triggers = %v, want 1 at its own 1 s", got)
		}
	})

	t.Run("ATB-001/desconexão zera a contagem", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: bigCombo})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 4)
		steps(t, clock, 37)
		api.set(0, false, 0)
		step(t, clock)
		tDisc := padStart.Add(608 * time.Millisecond)
		api.set(0, true, bigCombo)
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
		if got.Action != input.ActionBigBox || got.At.Sub(tDisc) < time.Second {
			t.Errorf("trigger %+v at disconnect+%v, want ActionBigBox at >= 1 s", *got, got.At.Sub(tDisc))
		}
	})

	t.Run("ATB-001/PadConfig.BigBoxMask personalizada e o padrão Back|A (0x0020|0x1000)", func(t *testing.T) {
		const mask uint16 = 0x0004 | 0x0008
		api := newFakePad(map[int]uint16{0: mask})
		clock, out, _ := startPad(t, api, input.PadConfig{BigBoxMask: mask}, 4)
		steps(t, clock, 63)
		if got := drain(out); len(got) != 1 || got[0].Action != input.ActionBigBox {
			t.Fatalf("custom mask: triggers = %v, want one ActionBigBox", got)
		}
		api2 := newFakePad(map[int]uint16{0: 0x0020 | 0x1000})
		clock2, out2, _ := startPad(t, api2, input.PadConfig{}, 4)
		steps(t, clock2, 63)
		if got := drain(out2); len(got) != 1 || got[0].Action != input.ActionBigBox {
			t.Fatalf("default mask: triggers = %v, want one ActionBigBox", got)
		}
		api3 := newFakePad(map[int]uint16{0: bigCombo})
		clock3, out3, _ := startPad(t, api3, input.PadConfig{BigBoxMask: mask}, 4)
		steps(t, clock3, 100)
		noTrigger(t, out3, "default combo under a custom BigBoxMask")
	})

	t.Run("ATB-001/o atalho de teclado Ctrl+Alt+P continua só ActionPause", func(t *testing.T) {
		// The zero Action is pause; the keyboard source never sets another one (see hotkey_action_windows_test.go).
		if input.ActionPause != input.Action(0) || input.ActionBigBox == input.ActionPause {
			t.Fatalf("ActionPause = %d, ActionBigBox = %d", input.ActionPause, input.ActionBigBox)
		}
	})

	t.Run("ATB-002/Back+Y só entrega pause, Back+B só close e Back+A só bigbox", func(t *testing.T) {
		for name, tc := range map[string]struct {
			buttons uint16
			want    input.Action
		}{"pause": {pauseCombo, input.ActionPause}, "close": {closeCombo, input.ActionClose}, "bigbox": {bigCombo, input.ActionBigBox}} {
			api := newFakePad(map[int]uint16{0: tc.buttons})
			clock, out, _ := startPad(t, api, input.PadConfig{}, 8)
			steps(t, clock, 100)
			got := drain(out)
			if len(got) != 1 || got[0].Action != tc.want {
				t.Errorf("%s: triggers = %v, want one action %d", name, got, tc.want)
			}
		}
	})

	t.Run("ATB-002/Back+A+B entrega um close e um bigbox, cada um uma vez por pressão", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: bigCombo | input.ButtonB})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 8)
		steps(t, clock, 100)
		got := drain(out)
		if len(got) != 2 || countAction(got, input.ActionClose) != 1 || countAction(got, input.ActionBigBox) != 1 {
			t.Fatalf("triggers = %v, want one close and one bigbox", got)
		}
		steps(t, clock, 200)
		noTrigger(t, out, "while still held")
		api.set(0, true, 0)
		step(t, clock)
		api.set(0, true, bigCombo|input.ButtonB)
		steps(t, clock, 70)
		got = drain(out)
		if len(got) != 2 || countAction(got, input.ActionClose) != 1 || countAction(got, input.ActionBigBox) != 1 {
			t.Fatalf("second press: triggers = %v, want one close and one bigbox", got)
		}
	})

	t.Run("ATB-002/soltar A não zera a contagem de Back+B nem de Back+Y, e vice-versa", func(t *testing.T) {
		at := padStart.Add(1008 * time.Millisecond)
		cases := []struct {
			name      string
			all, left uint16
			want      input.Action
		}{
			{"solta A, fica close", bigCombo | input.ButtonB, closeCombo, input.ActionClose},
			{"solta A, fica pause", bigCombo | input.ButtonY, pauseCombo, input.ActionPause},
			{"solta B, fica bigbox", bigCombo | input.ButtonB, bigCombo, input.ActionBigBox},
			{"solta Y, fica bigbox", bigCombo | input.ButtonY, bigCombo, input.ActionBigBox},
		}
		for _, tc := range cases {
			api := newFakePad(map[int]uint16{0: tc.all})
			clock, out, _ := startPad(t, api, input.PadConfig{}, 8)
			steps(t, clock, 40)
			api.set(0, true, tc.left)
			steps(t, clock, 23)
			got := drain(out)
			if len(got) != 1 || got[0].Action != tc.want || !got[0].At.Equal(at) {
				t.Errorf("%s: triggers = %v, want one action %d at T+1008ms", tc.name, got, tc.want)
			}
		}
	})

	t.Run("ATB-002/os acordes de volume Back+cima/baixo continuam como estão", func(t *testing.T) {
		var ups, downs int
		vol := func(up bool) {
			if up {
				ups++
			} else {
				downs++
			}
		}
		api := newFakePad(map[int]uint16{0: input.ButtonBack | input.ButtonDPadUp})
		clock, out, _ := startPad(t, api, input.PadConfig{Volume: vol}, 8)
		step(t, clock)
		api.set(0, true, input.ButtonBack|input.ButtonDPadDown)
		steps(t, clock, 3)
		if ups < 1 || downs < 1 {
			t.Errorf("volume calls up=%d down=%d, want both >= 1", ups, downs)
		}
		steps(t, clock, 100)
		noTrigger(t, out, "volume chords must not emit triggers")
	})

	t.Run("ATB-002/fila cheia descarta com Warn", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: bigCombo})
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
			t.Error("no Warn logged for the dropped bigbox trigger")
		}
		if len(out) != 1 || (<-out).Source != domain.PauseHotkey {
			t.Error("the dropped trigger must not displace what was queued")
		}
	})
}
