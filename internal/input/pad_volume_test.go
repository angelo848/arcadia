package input_test

import (
	"sync"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/input"
)

// volumeLog records the calls of PadConfig.Volume.
type volumeLog struct {
	mu    sync.Mutex
	calls []bool
}

func (v *volumeLog) add(up bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.calls = append(v.calls, up)
}

func (v *volumeLog) count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.calls)
}

func (v *volumeLog) last() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.calls[len(v.calls)-1]
}

// TestPadVolume covers the Select + D-pad up/down volume chords. Not parallel: each case drives its own fake clock.
func TestPadVolume(t *testing.T) {
	const (
		up   = input.ButtonBack | input.ButtonDPadUp
		down = input.ButtonBack | input.ButtonDPadDown
	)

	t.Run("VOL-001/Select + cima dá um passo para cima na hora, sem gatilho de pausa ou fechar", func(t *testing.T) {
		var vl volumeLog
		api := newFakePad(map[int]uint16{0: up})
		clock, out, _ := startPad(t, api, input.PadConfig{Volume: vl.add}, 4)
		if vl.count() != 1 || !vl.last() {
			t.Fatalf("calls = %v, want one step up at the press", vl.calls)
		}
		steps(t, clock, 10)
		if vl.count() != 1 {
			t.Errorf("calls = %d after 160 ms, want still 1 (the repeat waits 400 ms)", vl.count())
		}
		noTrigger(t, out, "a volume chord must not pause or close")
	})

	t.Run("VOL-001/Select + baixo dá um passo para baixo", func(t *testing.T) {
		var vl volumeLog
		api := newFakePad(map[int]uint16{0: down})
		startPad(t, api, input.PadConfig{Volume: vl.add}, 4)
		if vl.count() != 1 || vl.last() {
			t.Fatalf("calls = %v, want one step down", vl.calls)
		}
	})

	t.Run("VOL-002/segurado repete após 400 ms e depois a cada 150 ms", func(t *testing.T) {
		var vl volumeLog
		api := newFakePad(map[int]uint16{0: up})
		clock, _, _ := startPad(t, api, input.PadConfig{Volume: vl.add}, 4)
		steps(t, clock, 24) // 384 ms
		if vl.count() != 1 {
			t.Fatalf("calls at 384 ms = %d, want 1", vl.count())
		}
		step(t, clock) // 400 ms
		if vl.count() != 2 {
			t.Fatalf("calls at 400 ms = %d, want 2", vl.count())
		}
		steps(t, clock, 9) // 544 ms: only 144 ms since the last step
		if vl.count() != 2 {
			t.Fatalf("calls at 544 ms = %d, want 2", vl.count())
		}
		step(t, clock) // 560 ms: 160 ms since the last step
		if vl.count() != 3 {
			t.Fatalf("calls at 560 ms = %d, want 3", vl.count())
		}
	})

	t.Run("VOL-002/soltar e apertar de novo dá um passo na hora", func(t *testing.T) {
		var vl volumeLog
		api := newFakePad(map[int]uint16{0: up})
		clock, _, _ := startPad(t, api, input.PadConfig{Volume: vl.add}, 4)
		api.set(0, true, input.ButtonBack) // D-pad released
		step(t, clock)
		api.set(0, true, up)
		step(t, clock)
		if vl.count() != 2 {
			t.Fatalf("calls = %d, want 2 (one per press)", vl.count())
		}
	})

	t.Run("VOL-003/só D-pad, só Select ou Select + outro botão não mexem no volume", func(t *testing.T) {
		var vl volumeLog
		api := newFakePad(map[int]uint16{0: input.ButtonDPadUp, 1: input.ButtonBack})
		clock, _, _ := startPad(t, api, input.PadConfig{Volume: vl.add}, 4)
		steps(t, clock, 40)
		if vl.count() != 0 {
			t.Errorf("calls = %v, want none", vl.calls)
		}
	})

	t.Run("VOL-004/cada controle tem a sua contagem", func(t *testing.T) {
		var vl volumeLog
		api := newFakePad(map[int]uint16{0: up, 1: up})
		startPad(t, api, input.PadConfig{Volume: vl.add}, 4)
		if vl.count() != 2 {
			t.Fatalf("calls = %d, want 2 (one first step per controller)", vl.count())
		}
	})

	t.Run("VOL-005/sem callback o poller segue sem erro", func(t *testing.T) {
		api := newFakePad(map[int]uint16{0: up})
		clock, out, _ := startPad(t, api, input.PadConfig{}, 4)
		steps(t, clock, 30)
		noTrigger(t, out, "with the volume off")
	})
}
