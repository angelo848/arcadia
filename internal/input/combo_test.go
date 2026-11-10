package input

import (
	"testing"
	"time"
)

// Detector under test is used as a zero value: Hold (1 s) and mask (Back|Y) are the
// spec defaults (PAD-001), the same ones NewPad applies for a zero PadConfig.

var comboStart = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

const comboBoth = ButtonBack | ButtonY

// feed observes slot at comboStart+from, every step, until comboStart+to (inclusive)
// and returns the offsets (from comboStart) at which the detector fired.
func feed(d *comboDetector, slot int, buttons uint16, connected bool, from, to, step time.Duration) []time.Duration {
	var fires []time.Duration
	for at := from; at <= to; at += step {
		if d.Observe(slot, buttons, connected, comboStart.Add(at)) {
			fires = append(fires, at)
		}
	}
	return fires
}

func TestComboDetector(t *testing.T) {
	t.Parallel()

	const poll = 16 * time.Millisecond

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "PAD-001/Back e Y juntos por 1 s disparam uma vez (T+992 ms não dispara, T+1008 ms dispara)",
			run: func(t *testing.T) {
				var d comboDetector
				if got := feed(&d, 0, comboBoth, true, 0, 992*time.Millisecond, poll); len(got) != 0 {
					t.Fatalf("fired at %v before 1 s held, want none", got)
				}
				got := feed(&d, 0, comboBoth, true, 1008*time.Millisecond, 3000*time.Millisecond, poll)
				if len(got) != 1 || got[0] != 1008*time.Millisecond {
					t.Fatalf("fires = %v, want exactly one at 1008ms", got)
				}
			},
		},
		{
			name: "PAD-001/outros botões pressionados junto não impedem nem atrasam",
			run: func(t *testing.T) {
				var d comboDetector
				buttons := comboBoth | 0x0001 | 0x1000 | 0x0100
				if got := feed(&d, 0, buttons, true, 0, 992*time.Millisecond, poll); len(got) != 0 {
					t.Fatalf("fired at %v before 1 s held, want none", got)
				}
				got := feed(&d, 0, buttons, true, 1008*time.Millisecond, 1008*time.Millisecond, poll)
				if len(got) != 1 {
					t.Fatalf("fires = %v, want one at 1008ms (extra buttons must not delay)", got)
				}
			},
		},
		{
			name: "PAD-002/soltar Y aos 900 ms cancela e a nova pressão recomeça a contagem",
			run: func(t *testing.T) {
				var d comboDetector
				if got := feed(&d, 0, comboBoth, true, 0, 900*time.Millisecond, poll); len(got) != 0 {
					t.Fatalf("fired at %v, want none", got)
				}
				// Y released at 900 ms (only Back stays pressed).
				if got := feed(&d, 0, ButtonBack, true, 916*time.Millisecond, 960*time.Millisecond, poll); len(got) != 0 {
					t.Fatalf("fired at %v with Y released, want none", got)
				}
				// Pressed again for 900 ms: total time since the first press exceeds 1 s,
				// but the count restarted, so nothing fires.
				start := 976 * time.Millisecond
				if got := feed(&d, 0, comboBoth, true, start, start+900*time.Millisecond, poll); len(got) != 0 {
					t.Fatalf("fired at %v after 900 ms of the second press, want none", got)
				}
			},
		},
		{
			name: "PAD-003/10 s a mais pressionado não dispara de novo; soltar e pressionar de novo por 1 s dispara",
			run: func(t *testing.T) {
				var d comboDetector
				got := feed(&d, 0, comboBoth, true, 0, 11*time.Second, poll)
				if len(got) != 1 {
					t.Fatalf("fires = %v over 11 s held, want exactly one", got)
				}
				// Release Y, then press both again.
				if got := feed(&d, 0, ButtonBack, true, 11*time.Second+poll, 11*time.Second+poll, poll); len(got) != 0 {
					t.Fatalf("fired on release: %v", got)
				}
				start := 11*time.Second + 2*poll
				got = feed(&d, 0, comboBoth, true, start, start+992*time.Millisecond, poll)
				if len(got) != 0 {
					t.Fatalf("fired at %v before 1 s of the new press, want none", got)
				}
				got = feed(&d, 0, comboBoth, true, start+1008*time.Millisecond, start+1008*time.Millisecond, poll)
				if len(got) != 1 {
					t.Fatalf("fires = %v at 1 s of the new press, want one", got)
				}
			},
		},
		{
			name: "PAD-004/Back só no slot 0 e Y só no slot 1 não disparam",
			run: func(t *testing.T) {
				var d comboDetector
				for at := time.Duration(0); at <= 3*time.Second; at += poll {
					now := comboStart.Add(at)
					if d.Observe(0, ButtonBack, true, now) || d.Observe(1, ButtonY, true, now) {
						t.Fatalf("fired at %v with the combo split across slots", at)
					}
				}
			},
		},
		{
			name: "PAD-004/dois slots que completam juntos disparam um gatilho cada",
			run: func(t *testing.T) {
				var d comboDetector
				fires := map[int]int{}
				for at := time.Duration(0); at <= 3*time.Second; at += poll {
					now := comboStart.Add(at)
					for _, slot := range []int{0, 3} {
						if d.Observe(slot, comboBoth, true, now) {
							fires[slot]++
						}
					}
				}
				if fires[0] != 1 || fires[3] != 1 || len(fires) != 2 {
					t.Fatalf("fires per slot = %v, want one each for slots 0 and 3", fires)
				}
			},
		},
		{
			name: "PAD-005/desconexão aos 600 ms zera a contagem e a reconexão pede mais 1 s inteiro",
			run: func(t *testing.T) {
				var d comboDetector
				if got := feed(&d, 0, comboBoth, true, 0, 592*time.Millisecond, poll); len(got) != 0 {
					t.Fatalf("fired at %v, want none", got)
				}
				// Disconnected at ~600 ms.
				if d.Observe(0, 0, false, comboStart.Add(608*time.Millisecond)) {
					t.Fatal("fired on disconnect")
				}
				// Reconnects with the combo still held: first sight at 700 ms.
				re := 700 * time.Millisecond
				if got := feed(&d, 0, comboBoth, true, re, re+992*time.Millisecond, poll); len(got) != 0 {
					t.Fatalf("fired at %v before 1 s after reconnect, want none (original press would have fired by now)", got)
				}
				got := feed(&d, 0, comboBoth, true, re+1008*time.Millisecond, re+1008*time.Millisecond, poll)
				if len(got) != 1 {
					t.Fatalf("fires = %v at 1 s after reconnect, want one", got)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}
