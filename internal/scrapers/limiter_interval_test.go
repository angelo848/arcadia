package scrapers_test

import (
	"context"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// Assumed (tasks.md T20):
//
//	const IntervalScreenScraper = 1500 * time.Millisecond
//	func SSInterval(maxPerMinute int) time.Duration
//	func (l *Limiter) SetInterval(d time.Duration)
func TestSSInterval(t *testing.T) {
	t.Parallel()

	t.Run("VID-015/IntervalScreenScraper = 1500 ms (x-video.requestIntervalMs)", func(t *testing.T) {
		t.Parallel()
		if scrapers.IntervalScreenScraper != 1500*time.Millisecond {
			t.Errorf("IntervalScreenScraper = %v, want 1500ms", scrapers.IntervalScreenScraper)
		}
	})

	t.Run("VID-015/SSInterval: 1500 ms por padrão, 20 por minuto = 3000 ms, 100 por minuto = 1500 ms, 0 ou negativo = 1500 ms", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			perMinute int
			want      time.Duration
		}{
			{0, 1500 * time.Millisecond},
			{-1, 1500 * time.Millisecond},
			{-100, 1500 * time.Millisecond},
			{20, 3000 * time.Millisecond},
			{40, 1500 * time.Millisecond},
			{41, 1500 * time.Millisecond}, // ceil(60000/41) = 1463 < 1500
			{100, 1500 * time.Millisecond},
			{1, 60000 * time.Millisecond},
			{30, 2000 * time.Millisecond},
			{7, 8572 * time.Millisecond}, // ceil(60000/7) = 8572
		}
		for _, c := range cases {
			if got := scrapers.SSInterval(c.perMinute); got != c.want {
				t.Errorf("SSInterval(%d) = %v, want %v", c.perMinute, got, c.want)
			}
		}
	})
}

func TestLimiterSetInterval(t *testing.T) {
	t.Parallel()

	t.Run("VID-015/Limiter.SetInterval vale a partir da próxima reserva e não reabre slots já reservados (relógio fake)", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(limiterStart)
		l := scrapers.NewLimiter(clock, scrapers.IntervalScreenScraper)
		guard := guardContext(t)

		// Call 1 goes through at once and reserves the slot start+1500ms for the next one.
		if err := expectReturn(t, callWait(l, context.Background())); err != nil {
			t.Fatalf("primeira chamada: erro %v", err)
		}
		l.SetInterval(3000 * time.Millisecond)

		// Call 2 keeps the slot already reserved (start+1500ms): the new interval does not move it.
		ch2 := callWait(l, context.Background())
		if err := clock.BlockUntilWaiters(guard, 1); err != nil {
			t.Fatalf("chamada 2 não registrou espera: %v", err)
		}
		clock.Advance(1499 * time.Millisecond)
		expectBlocked(t, ch2)
		clock.Advance(1 * time.Millisecond)
		if err := expectReturn(t, ch2); err != nil {
			t.Fatalf("chamada 2: erro %v", err)
		}
		if got, want := clock.Now(), limiterStart.Add(1500*time.Millisecond); !got.Equal(want) {
			t.Errorf("chamada 2 saiu em %v, want %v (slot já reservado)", got, want)
		}

		// Call 3 is spaced by the new interval from call 2's slot: start+1500+3000.
		ch3 := callWait(l, context.Background())
		if err := clock.BlockUntilWaiters(guard, 1); err != nil {
			t.Fatalf("chamada 3 não registrou espera: %v", err)
		}
		clock.Advance(2999 * time.Millisecond)
		expectBlocked(t, ch3)
		clock.Advance(1 * time.Millisecond)
		if err := expectReturn(t, ch3); err != nil {
			t.Fatalf("chamada 3: erro %v", err)
		}
		if got, want := clock.Now(), limiterStart.Add(4500*time.Millisecond); !got.Equal(want) {
			t.Errorf("chamada 3 saiu em %v, want %v (novo intervalo)", got, want)
		}
	})

	t.Run("VID-015/SetInterval em Limiter nulo não faz nada", func(t *testing.T) {
		t.Parallel()
		var l *scrapers.Limiter
		l.SetInterval(5 * time.Second) // must not panic
		if err := expectReturn(t, callWait(l, context.Background())); err != nil {
			t.Errorf("Wait em Limiter nulo: erro %v, want nil", err)
		}
	})
}
