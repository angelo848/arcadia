package input_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/input"
)

var padStart = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

const (
	padPoll    = 16 * time.Millisecond
	padTimeout = 2 * time.Second
)

// fakePad is a scripted PadAPI. Slots not configured as connected report ErrDisconnected.
type fakePad struct {
	mu      sync.Mutex
	buttons [4]uint16
	conn    [4]bool
	reads   [4]int
}

func newFakePad(connected map[int]uint16) *fakePad {
	f := &fakePad{}
	for slot, b := range connected {
		f.conn[slot] = true
		f.buttons[slot] = b
	}
	return f
}

func (f *fakePad) State(slot int) (input.PadState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads[slot]++
	if !f.conn[slot] {
		return input.PadState{}, fmt.Errorf("slot %d: %w", slot, input.ErrDisconnected)
	}
	return input.PadState{Buttons: f.buttons[slot]}, nil
}

func (f *fakePad) set(slot int, connected bool, buttons uint16) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.conn[slot] = connected
	f.buttons[slot] = buttons
}

func (f *fakePad) readCount(slot int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[slot]
}

// warnLog records slog records at Warn or above.
type warnLog struct {
	mu    sync.Mutex
	warns int
}

func (w *warnLog) Enabled(context.Context, slog.Level) bool { return true }
func (w *warnLog) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		w.mu.Lock()
		w.warns++
		w.mu.Unlock()
	}
	return nil
}
func (w *warnLog) WithAttrs([]slog.Attr) slog.Handler { return w }
func (w *warnLog) WithGroup(string) slog.Handler      { return w }
func (w *warnLog) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.warns
}

func waitWaiter(t *testing.T, c *domain.FakeClock) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), padTimeout)
	defer cancel()
	if err := c.BlockUntilWaiters(ctx, 1); err != nil {
		t.Fatalf("poller never waited on the clock: %v", err)
	}
}

// step advances the clock by one poll interval once the poller is waiting, then waits
// until the poller has processed the tick and is waiting again.
func step(t *testing.T, c *domain.FakeClock) {
	t.Helper()
	waitWaiter(t, c)
	c.Advance(padPoll)
	waitWaiter(t, c)
}

func steps(t *testing.T, c *domain.FakeClock, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		step(t, c)
	}
}

func noTrigger(t *testing.T, out <-chan input.Trigger, when string) {
	t.Helper()
	select {
	case tr := <-out:
		t.Fatalf("unexpected trigger %+v %s", tr, when)
	default:
	}
}

func TestPad(t *testing.T) {
	// Not parallel: goleak inspects process-wide goroutines.
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "PAD-001/62 avanços de 16 ms não entregam Trigger e o 63º entrega um com Source gamepad e At = instante lido",
			run: func(t *testing.T) {
				clock := domain.NewFakeClock(padStart)
				api := newFakePad(map[int]uint16{0: input.ButtonBack | input.ButtonY})
				out := make(chan input.Trigger, 4)
				p := input.NewPad(api, input.PadConfig{}, clock, slog.New(&warnLog{}))
				if err := p.Start(out); err != nil {
					t.Fatalf("Start: %v", err)
				}
				defer p.Close()

				steps(t, clock, 62)
				noTrigger(t, out, "at T+992ms")

				step(t, clock)
				select {
				case tr := <-out:
					if tr.Source != domain.PauseGamepad {
						t.Errorf("Source = %q, want %q", tr.Source, domain.PauseGamepad)
					}
					if want := padStart.Add(1008 * time.Millisecond); !tr.At.Equal(want) {
						t.Errorf("At = %v, want %v (instant of the read)", tr.At, want)
					}
				case <-time.After(padTimeout):
					t.Fatal("no trigger at T+1008ms")
				}
				step(t, clock)
				noTrigger(t, out, "(combo still held, one trigger per press)")
			},
		},
		{
			name: "PAD-006/ao iniciar cada slot é lido uma vez; após 62 avanços o conectado tem 63 leituras e os desconectados 1; no 63º avanço cada desconectado é sondado de novo (2)",
			run: func(t *testing.T) {
				clock := domain.NewFakeClock(padStart)
				api := newFakePad(map[int]uint16{1: 0})
				out := make(chan input.Trigger, 4)
				p := input.NewPad(api, input.PadConfig{}, clock, slog.New(&warnLog{}))
				if err := p.Start(out); err != nil {
					t.Fatalf("Start: %v", err)
				}
				defer p.Close()

				for slot := 0; slot < 4; slot++ {
					if got := api.readCount(slot); got != 1 {
						t.Errorf("after Start: slot %d reads = %d, want 1", slot, got)
					}
				}
				steps(t, clock, 62)
				for slot, want := range [4]int{1, 63, 1, 1} {
					if got := api.readCount(slot); got != want {
						t.Errorf("after 62 advances: slot %d reads = %d, want %d", slot, got, want)
					}
				}
				step(t, clock)
				for slot, want := range [4]int{2, 64, 2, 2} {
					if got := api.readCount(slot); got != want {
						t.Errorf("after 63 advances: slot %d reads = %d, want %d", slot, got, want)
					}
				}
			},
		},
		{
			name: "PAD-005/ERROR_DEVICE_NOT_CONNECTED no meio da pressão zera a contagem",
			run: func(t *testing.T) {
				clock := domain.NewFakeClock(padStart)
				combo := input.ButtonBack | input.ButtonY
				api := newFakePad(map[int]uint16{0: combo})
				out := make(chan input.Trigger, 4)
				p := input.NewPad(api, input.PadConfig{}, clock, slog.New(&warnLog{}))
				if err := p.Start(out); err != nil {
					t.Fatalf("Start: %v", err)
				}
				defer p.Close()

				steps(t, clock, 37) // 592 ms held
				noTrigger(t, out, "before disconnect")
				api.set(0, false, 0)
				step(t, clock) // 608 ms: read fails with ErrDisconnected
				tDisc := padStart.Add(608 * time.Millisecond)
				api.set(0, true, combo) // reconnects, combo still held

				// Without the reset the original press would fire at 1008 ms.
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
					t.Fatal("no trigger after reconnect with the combo held")
				}
				if got.At.Sub(tDisc) < time.Second {
					t.Errorf("trigger at disconnect+%v, want >= 1 s (count must restart)", got.At.Sub(tDisc))
				}
			},
		},
		{
			name: "PAD-008/Close sem avançar o relógio retorna, a goroutine termina (goleak) e a segunda chamada não tem efeito",
			run: func(t *testing.T) {
				clock := domain.NewFakeClock(padStart)
				api := newFakePad(map[int]uint16{0: 0})
				out := make(chan input.Trigger, 4)
				p := input.NewPad(api, input.PadConfig{}, clock, slog.New(&warnLog{}))
				if err := p.Start(out); err != nil {
					t.Fatalf("Start: %v", err)
				}
				waitWaiter(t, clock)

				done := make(chan [2]error, 1)
				go func() { done <- [2]error{p.Close(), p.Close()} }()
				select {
				case errs := <-done:
					for i, err := range errs {
						if err != nil {
							t.Errorf("Close call %d = %v, want nil", i+1, err)
						}
					}
				case <-time.After(padTimeout):
					t.Fatal("Close did not return without the clock advancing")
				}
				goleak.VerifyNone(t)
			},
		},
		{
			name: "PAD-001/fila de saída cheia descarta o gatilho com Warn sem bloquear",
			run: func(t *testing.T) {
				clock := domain.NewFakeClock(padStart)
				api := newFakePad(map[int]uint16{0: input.ButtonBack | input.ButtonY})
				out := make(chan input.Trigger, 1)
				out <- input.Trigger{Source: domain.PauseHotkey, At: padStart} // queue already full
				wl := &warnLog{}
				p := input.NewPad(api, input.PadConfig{}, clock, slog.New(wl))
				if err := p.Start(out); err != nil {
					t.Fatalf("Start: %v", err)
				}
				defer p.Close()

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
					t.Error("no Warn logged for the dropped trigger")
				}
				if len(out) != 1 || (<-out).Source != domain.PauseHotkey {
					t.Error("the dropped trigger must not displace what was already queued")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}
