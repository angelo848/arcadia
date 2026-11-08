package domain_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// Compile-time checks: both clocks satisfy domain.Clock.
var (
	_ domain.Clock = domain.SystemClock{}
	_ domain.Clock = (*domain.FakeClock)(nil)
)

// clockStart is the fixed instant every FakeClock test starts from.
var clockStart = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// safetyTimeout only guards against a hung test; no assertion depends on real time elapsing.
const safetyTimeout = time.Second

// shortTimeout bounds negative BlockUntilWaiters checks (expected to time out).
const shortTimeout = 50 * time.Millisecond

func mustReceive(t *testing.T, ch <-chan time.Time, what string) time.Time {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(safetyTimeout):
		t.Fatalf("%s: no value received within %v", what, safetyTimeout)
		return time.Time{}
	}
}

func mustBeEmpty(t *testing.T, ch <-chan time.Time, what string) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("%s: unexpected value %v", what, v)
	default:
	}
}

func blockUntil(t *testing.T, c *domain.FakeClock, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), safetyTimeout)
	defer cancel()
	if err := c.BlockUntilWaiters(ctx, n); err != nil {
		t.Fatalf("BlockUntilWaiters(%d) = %v, want nil", n, err)
	}
}

func TestFakeClockAdvance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "DOM-005/Now inicial é o start",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				if got := c.Now(); !got.Equal(clockStart) {
					t.Fatalf("Now() = %v, want %v", got, clockStart)
				}
			},
		},
		{
			name: "DOM-005/Advance avança exatamente d",
			run: func(t *testing.T) {
				for _, d := range []time.Duration{time.Nanosecond, time.Millisecond, 1500 * time.Millisecond, time.Hour} {
					c := domain.NewFakeClock(clockStart)
					c.Advance(d)
					want := clockStart.Add(d)
					if got := c.Now(); !got.Equal(want) {
						t.Fatalf("Advance(%v): Now() = %v, want %v", d, got, want)
					}
				}
			},
		},
		{
			name: "DOM-005/Advance(0) não muda Now",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				c.Advance(0)
				if got := c.Now(); !got.Equal(clockStart) {
					t.Fatalf("Now() = %v, want %v", got, clockStart)
				}
			},
		},
		{
			name: "DOM-005/Now sem Advance não anda",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				a := c.Now()
				b := c.Now()
				if !a.Equal(b) || !a.Equal(clockStart) {
					t.Fatalf("Now() changed without Advance: %v then %v", a, b)
				}
			},
		},
		{
			name: "DOM-005/Advance acumulado",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				steps := []time.Duration{time.Second, 250 * time.Millisecond, time.Minute, time.Nanosecond}
				want := clockStart
				for _, d := range steps {
					c.Advance(d)
					want = want.Add(d)
					if got := c.Now(); !got.Equal(want) {
						t.Fatalf("after Advance(%v): Now() = %v, want %v", d, got, want)
					}
				}
			},
		},
		{
			name: "DOM-005/Now e Advance concorrentes",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				const (
					advancers = 8
					perWorker = 200
					readers   = 4
				)
				var wg sync.WaitGroup
				errs := make(chan error, readers)
				stop := make(chan struct{})

				var readersWG sync.WaitGroup
				for i := 0; i < readers; i++ {
					readersWG.Add(1)
					go func() {
						defer readersWG.Done()
						prev := c.Now()
						for {
							select {
							case <-stop:
								return
							default:
							}
							cur := c.Now()
							if cur.Before(prev) {
								errs <- errors.New("Now() went backwards: " + cur.String() + " < " + prev.String())
								return
							}
							prev = cur
						}
					}()
				}

				for i := 0; i < advancers; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for j := 0; j < perWorker; j++ {
							c.Advance(time.Millisecond)
						}
					}()
				}
				wg.Wait()
				close(stop)
				readersWG.Wait()
				close(errs)
				for err := range errs {
					t.Error(err)
				}

				want := clockStart.Add(advancers * perWorker * time.Millisecond)
				if got := c.Now(); !got.Equal(want) {
					t.Fatalf("Now() = %v, want %v", got, want)
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

func TestFakeClockTimers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "DOM-006/After não dispara em d-1ns",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				const d = 3 * time.Second
				ch := c.After(d)
				mustBeEmpty(t, ch, "before Advance")
				c.Advance(d - time.Nanosecond)
				mustBeEmpty(t, ch, "after Advance(d-1ns)")
			},
		},
		{
			name: "DOM-006/After dispara uma vez em d com Now resultante",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				const d = 3 * time.Second
				ch := c.After(d)
				c.Advance(d - time.Nanosecond)
				mustBeEmpty(t, ch, "after Advance(d-1ns)")
				c.Advance(time.Nanosecond)
				got := mustReceive(t, ch, "after Advance(1ns)")
				want := clockStart.Add(d)
				if !got.Equal(want) {
					t.Fatalf("fired value = %v, want %v", got, want)
				}
				if now := c.Now(); !got.Equal(now) {
					t.Fatalf("fired value = %v, want Now() = %v", got, now)
				}
				// Exactly one value, even after further advances.
				c.Advance(time.Hour)
				mustBeEmpty(t, ch, "second value")
			},
		},
		{
			name: "DOM-006/After dispara com o Now resultante quando Advance passa do prazo",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				ch := c.After(time.Second)
				c.Advance(10 * time.Second)
				got := mustReceive(t, ch, "after overshooting Advance")
				want := clockStart.Add(10 * time.Second)
				if !got.Equal(want) {
					t.Fatalf("fired value = %v, want resulting Now %v", got, want)
				}
			},
		},
		{
			name: "DOM-006/After com d<=0 já tem valor",
			run: func(t *testing.T) {
				for _, d := range []time.Duration{0, -time.Nanosecond, -time.Hour} {
					c := domain.NewFakeClock(clockStart)
					ch := c.After(d)
					// The value must already be available: no Advance, non-blocking receive.
					select {
					case got := <-ch:
						if !got.Equal(clockStart) {
							t.Fatalf("After(%v) value = %v, want Now() %v", d, got, clockStart)
						}
					default:
						t.Fatalf("After(%v): channel has no value without Advance", d)
					}
					c.Advance(time.Hour)
					mustBeEmpty(t, ch, "second value for d<=0")
				}
			},
		},
		{
			name: "DOM-006/Advance(2s) dispara 1s e 2s e mantém 5s",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				ch1 := c.After(1 * time.Second)
				ch2 := c.After(2 * time.Second)
				ch5 := c.After(5 * time.Second)

				c.Advance(2 * time.Second)
				want := clockStart.Add(2 * time.Second)
				if got := mustReceive(t, ch1, "1s waiter"); !got.Equal(want) {
					t.Fatalf("1s waiter value = %v, want %v", got, want)
				}
				if got := mustReceive(t, ch2, "2s waiter"); !got.Equal(want) {
					t.Fatalf("2s waiter value = %v, want %v", got, want)
				}
				mustBeEmpty(t, ch5, "5s waiter after Advance(2s)")

				// The 5s waiter is still pending: exactly one waiter remains.
				blockUntil(t, c, 1)
				ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
				defer cancel()
				if err := c.BlockUntilWaiters(ctx, 2); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("BlockUntilWaiters(2) = %v, want context.DeadlineExceeded (only one pending)", err)
				}

				c.Advance(3 * time.Second)
				if got, want := mustReceive(t, ch5, "5s waiter"), clockStart.Add(5*time.Second); !got.Equal(want) {
					t.Fatalf("5s waiter value = %v, want %v", got, want)
				}
			},
		},
		{
			name: "DOM-006/Advance não bloqueia sem leitor",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				chs := []<-chan time.Time{
					c.After(time.Second),
					c.After(time.Second),
					c.After(2 * time.Second),
				}
				done := make(chan struct{})
				go func() {
					defer close(done)
					c.Advance(5 * time.Second) // nobody reads the channels
					c.Advance(5 * time.Second) // fired waiters were removed; must not resend/block
				}()
				select {
				case <-done:
				case <-time.After(safetyTimeout):
					t.Fatal("Advance blocked with no reader on fired channels")
				}
				for _, ch := range chs {
					mustReceive(t, ch, "buffered value")
					mustBeEmpty(t, ch, "second value")
				}
			},
		},
		{
			name: "DOM-006/BlockUntilWaiters retorna após registro",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				ready := make(chan (<-chan time.Time), 1)
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					ready <- c.After(time.Second)
				}()

				blockUntil(t, c, 1)

				// The waiter is registered: advancing now must fire it.
				c.Advance(time.Second)
				var ch <-chan time.Time
				select {
				case ch = <-ready:
				case <-time.After(safetyTimeout):
					t.Fatal("goroutine never returned from After")
				}
				wg.Wait()
				if got, want := mustReceive(t, ch, "registered waiter"), clockStart.Add(time.Second); !got.Equal(want) {
					t.Fatalf("value = %v, want %v", got, want)
				}
			},
		},
		{
			name: "DOM-006/BlockUntilWaiters com n=0 retorna nil",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				blockUntil(t, c, 0)
			},
		},
		{
			name: "DOM-006/BlockUntilWaiters espera vários registros",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				const n = 3
				var wg sync.WaitGroup
				for i := 0; i < n; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_ = c.After(time.Minute)
					}()
				}
				blockUntil(t, c, n)
				wg.Wait()
			},
		},
		{
			name: "DOM-006/waiters disparados ou d<=0 não contam",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				_ = c.After(0)
				_ = c.After(-time.Second)
				fired := c.After(time.Second)
				c.Advance(time.Second)
				mustReceive(t, fired, "fired waiter")

				ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
				defer cancel()
				if err := c.BlockUntilWaiters(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("BlockUntilWaiters(1) = %v, want context.DeadlineExceeded (no pending waiters)", err)
				}

				// A new pending waiter counts.
				_ = c.After(time.Second)
				blockUntil(t, c, 1)
			},
		},
		{
			name: "DOM-006/BlockUntilWaiters retorna ctx.Err() ao cancelar",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				errc := make(chan error, 1)
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					errc <- c.BlockUntilWaiters(ctx, 1)
				}()
				// cancel() may run before the goroutine reaches its wait, so this case does not
				// guarantee that the "already blocked, then canceled" path is exercised. There is no
				// way to observe "is blocked inside BlockUntilWaiters" through the public API without
				// real time; that path is covered deterministically enough by the shortTimeout checks
				// ("waiters disparados ou d<=0 não contam" and "Advance(2s) ..."), where the call is
				// blocked with no pending waiters until its ctx deadline fires and must return ctx.Err().
				cancel()
				select {
				case err := <-errc:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("BlockUntilWaiters = %v, want context.Canceled", err)
					}
				case <-time.After(safetyTimeout):
					t.Fatal("BlockUntilWaiters did not return after ctx cancel")
				}
				wg.Wait()
			},
		},
		{
			name: "DOM-006/BlockUntilWaiters com ctx já cancelado e sem waiters retorna ctx.Err()",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				errc := make(chan error, 1)
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					errc <- c.BlockUntilWaiters(ctx, 1)
				}()
				select {
				case err := <-errc:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("BlockUntilWaiters = %v, want context.Canceled", err)
					}
				case <-time.After(safetyTimeout):
					t.Fatal("BlockUntilWaiters did not return with canceled ctx")
				}
				wg.Wait()
			},
		},
		{
			name: "DOM-006/After e Advance concorrentes não perdem waiters",
			run: func(t *testing.T) {
				c := domain.NewFakeClock(clockStart)
				const (
					registrants = 32
					advances    = 100
				)
				chs := make([]<-chan time.Time, registrants)
				var wg sync.WaitGroup
				for i := 0; i < registrants; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						chs[i] = c.After(time.Millisecond)
					}()
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < advances; j++ {
						c.Advance(time.Millisecond)
					}
				}()

				done := make(chan struct{})
				go func() {
					defer close(done)
					wg.Wait()
				}()
				select {
				case <-done:
				case <-time.After(safetyTimeout):
					t.Fatal("concurrent After/Advance deadlocked")
				}

				// Every waiter has deadline <= its registration time + 1ms <= Now() + 1ms,
				// so one more Advance(1ms) must fire all still-pending waiters.
				c.Advance(time.Millisecond)
				final := c.Now()
				if want := clockStart.Add((advances + 1) * time.Millisecond); !final.Equal(want) {
					t.Fatalf("Now() = %v, want %v", final, want)
				}
				blockUntilZero := func() {
					ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
					defer cancel()
					if err := c.BlockUntilWaiters(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("BlockUntilWaiters(1) = %v, want context.DeadlineExceeded (all waiters fired)", err)
					}
				}
				blockUntilZero()
				for i, ch := range chs {
					got := mustReceive(t, ch, "waiter")
					if got.Before(clockStart.Add(time.Millisecond)) || got.After(final) {
						t.Fatalf("waiter %d fired with %v, want within [%v, %v]", i, got, clockStart.Add(time.Millisecond), final)
					}
					mustBeEmpty(t, ch, "second value")
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
