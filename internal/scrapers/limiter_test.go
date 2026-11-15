package scrapers_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// limiterStart is the fake instant every limiter test begins at.
var limiterStart = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// guardTimeout only stops a broken test from hanging forever. It never decides the behaviour under test:
// the FakeClock decides when Wait may return.
const guardTimeout = 2 * time.Second

// guardContext returns a context that ends after guardTimeout, used to block on fake-clock registrations.
func guardContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), guardTimeout)
	t.Cleanup(cancel)
	return ctx
}

// callWait runs l.Wait(ctx) on its own goroutine and delivers the error on the returned channel.
// The buffered channel lets the goroutine finish even when nobody reads it.
func callWait(l *scrapers.Limiter, ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() { done <- l.Wait(ctx) }()
	return done
}

// expectReturn waits for a Wait call to return and gives its error. It fails the test if the call
// does not return within guardTimeout.
func expectReturn(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(guardTimeout):
		t.Fatal("Wait não voltou dentro do prazo de guarda")
		return nil
	}
}

// expectBlocked checks that a Wait call has not returned. It is exact right after a clock step that
// must not release the call: nothing can be sent to ch in that case.
func expectBlocked(t *testing.T, ch <-chan error) {
	t.Helper()
	select {
	case err := <-ch:
		t.Fatalf("Wait voltou antes do horário reservado (erro %v)", err)
	default:
	}
}

func TestLimiterWait(t *testing.T) {
	t.Parallel()

	t.Run("MET-010/primeira chamada sai na hora", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(limiterStart)
		l := scrapers.NewLimiter(clock, scrapers.IntervalIGDB)
		if err := expectReturn(t, callWait(l, context.Background())); err != nil {
			t.Fatalf("primeira chamada: erro %v", err)
		}
		if got := clock.Now(); !got.Equal(limiterStart) {
			t.Errorf("relógio mudou sem Advance: %v", got)
		}
	})

	t.Run("MET-010/10 chamadas simultâneas saem espaçadas pelo intervalo (instantes distintos e crescentes)", func(t *testing.T) {
		t.Parallel()
		const n = 10
		interval := scrapers.IntervalIGDB
		clock := domain.NewFakeClock(limiterStart)
		l := scrapers.NewLimiter(clock, interval)
		ctx := guardContext(t)

		released := make(chan error, n)
		for i := 0; i < n; i++ {
			go func() { released <- l.Wait(context.Background()) }()
		}

		// The first call to get a slot returns at once, before any clock step.
		select {
		case err := <-released:
			if err != nil {
				t.Fatalf("primeira chamada: erro %v", err)
			}
		case <-time.After(guardTimeout):
			t.Fatal("nenhuma chamada saiu na hora")
		}
		instants := []time.Time{clock.Now()}

		// The other nine must all be registered before the clock moves, so their deadlines are fixed.
		if err := clock.BlockUntilWaiters(ctx, n-1); err != nil {
			t.Fatalf("esperando as outras chamadas: %v", err)
		}
		for i := 1; i < n; i++ {
			clock.Advance(interval)
			// Only the call whose slot is now due returns; the clock stays put until it has been read.
			select {
			case err := <-released:
				if err != nil {
					t.Fatalf("chamada %d: erro %v", i, err)
				}
			case <-time.After(guardTimeout):
				t.Fatalf("chamada %d não saiu no seu horário", i)
			}
			instants = append(instants, clock.Now())
		}

		for i, got := range instants {
			want := limiterStart.Add(time.Duration(i) * interval)
			if !got.Equal(want) {
				t.Errorf("chamada %d saiu em %v; want %v (intervalo %v)", i, got, want, interval)
			}
		}
		for i := 1; i < len(instants); i++ {
			if !instants[i].After(instants[i-1]) {
				t.Errorf("instantes não crescem: %v depois de %v", instants[i], instants[i-1])
			}
		}
	})

	t.Run("MET-010/espera conta do horário reservado, não da chegada", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(limiterStart)
		l := scrapers.NewLimiter(clock, scrapers.IntervalIGDB)
		ctx := guardContext(t)

		if err := expectReturn(t, callWait(l, context.Background())); err != nil {
			t.Fatalf("primeira chamada: erro %v", err)
		}
		// The second call reserves start+334ms. It arrives 100 ms later, so it must wait 234 ms, not 334 ms.
		clock.Advance(100 * time.Millisecond)
		ch := callWait(l, context.Background())
		if err := clock.BlockUntilWaiters(ctx, 1); err != nil {
			t.Fatalf("segunda chamada não registrou espera: %v", err)
		}
		clock.Advance(233 * time.Millisecond)
		expectBlocked(t, ch)
		clock.Advance(1 * time.Millisecond)
		if err := expectReturn(t, ch); err != nil {
			t.Fatalf("segunda chamada: erro %v", err)
		}
		want := limiterStart.Add(scrapers.IntervalIGDB)
		if got := clock.Now(); !got.Equal(want) {
			t.Errorf("segunda chamada saiu em %v; want %v (horário reservado)", got, want)
		}
	})

	t.Run("MET-010/contexto cancelado libera quem espera e devolve ctx.Err()", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(limiterStart)
		l := scrapers.NewLimiter(clock, scrapers.IntervalSteamGridDB)
		guard := guardContext(t)

		if err := expectReturn(t, callWait(l, context.Background())); err != nil {
			t.Fatalf("primeira chamada: erro %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch := callWait(l, ctx)
		if err := clock.BlockUntilWaiters(guard, 1); err != nil {
			t.Fatalf("chamada não registrou espera: %v", err)
		}
		cancel()
		err := expectReturn(t, ch)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Wait cancelado: erro %v; want context.Canceled", err)
		}
	})

	t.Run("MET-010/a reserva de quem cancelou continua gasta", func(t *testing.T) {
		t.Parallel()
		interval := scrapers.IntervalIGDB
		clock := domain.NewFakeClock(limiterStart)
		l := scrapers.NewLimiter(clock, interval)
		guard := guardContext(t)

		if err := expectReturn(t, callWait(l, context.Background())); err != nil {
			t.Fatalf("primeira chamada: erro %v", err)
		}
		// Call 2 reserves start+334ms and is canceled while it waits.
		ctx1, cancel1 := context.WithCancel(context.Background())
		defer cancel1()
		ch1 := callWait(l, ctx1)
		if err := clock.BlockUntilWaiters(guard, 1); err != nil {
			t.Fatalf("chamada 2 não registrou espera: %v", err)
		}
		cancel1()
		if err := expectReturn(t, ch1); !errors.Is(err, context.Canceled) {
			t.Fatalf("chamada 2 cancelada: erro %v; want context.Canceled", err)
		}

		// Call 3 must still get start+668ms: the slot of call 2 is not given back.
		ch3 := callWait(l, context.Background())
		// Call 2's timer is still pending in the fake clock, so two waiters are expected here.
		if err := clock.BlockUntilWaiters(guard, 2); err != nil {
			t.Fatalf("chamada 3 não registrou espera: %v", err)
		}
		clock.Advance(interval)
		expectBlocked(t, ch3)
		clock.Advance(interval)
		if err := expectReturn(t, ch3); err != nil {
			t.Fatalf("chamada 3: erro %v", err)
		}
		want := limiterStart.Add(2 * interval)
		if got := clock.Now(); !got.Equal(want) {
			t.Errorf("chamada 3 saiu em %v; want %v (reserva de quem cancelou ficou gasta)", got, want)
		}
	})

	t.Run("MET-010/provedores independentes não se atrasam", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(limiterStart)
		igdb := scrapers.NewLimiter(clock, scrapers.IntervalIGDB)
		steamgriddb := scrapers.NewLimiter(clock, scrapers.IntervalSteamGridDB)
		steamstore := scrapers.NewLimiter(clock, scrapers.IntervalSteamStore)
		ctx := context.Background()

		// Two calls of IGDB: the second one waits.
		if err := expectReturn(t, callWait(igdb, ctx)); err != nil {
			t.Fatalf("IGDB primeira: erro %v", err)
		}
		blocked := callWait(igdb, ctx)
		if err := clock.BlockUntilWaiters(guardContext(t), 1); err != nil {
			t.Fatalf("IGDB segunda não registrou espera: %v", err)
		}
		// The other providers' first calls are not delayed by IGDB's reservation.
		if err := expectReturn(t, callWait(steamgriddb, ctx)); err != nil {
			t.Fatalf("SteamGridDB primeira: erro %v", err)
		}
		if err := expectReturn(t, callWait(steamstore, ctx)); err != nil {
			t.Fatalf("Steam Store primeira: erro %v", err)
		}
		expectBlocked(t, blocked)
	})

	t.Run("MET-010/Limiter nulo não espera", func(t *testing.T) {
		t.Parallel()
		var l *scrapers.Limiter
		if err := expectReturn(t, callWait(l, context.Background())); err != nil {
			t.Fatalf("Limiter nulo: erro %v; want nil", err)
		}
		if err := expectReturn(t, callWait(l, context.Background())); err != nil {
			t.Fatalf("Limiter nulo, segunda chamada: erro %v; want nil", err)
		}
	})

	t.Run("MET-010/constantes: 334 ms, 500 ms, 2000 ms", func(t *testing.T) {
		t.Parallel()
		if scrapers.IntervalIGDB != 334*time.Millisecond {
			t.Errorf("IntervalIGDB = %v; want 334ms", scrapers.IntervalIGDB)
		}
		if scrapers.IntervalSteamGridDB != 500*time.Millisecond {
			t.Errorf("IntervalSteamGridDB = %v; want 500ms", scrapers.IntervalSteamGridDB)
		}
		if scrapers.IntervalSteamStore != 2000*time.Millisecond {
			t.Errorf("IntervalSteamStore = %v; want 2000ms", scrapers.IntervalSteamStore)
		}
	})
}

// TestLimiterLeavesNoGoroutine is sequential on purpose: goleak must not see goroutines of parallel siblings.
func TestLimiterLeavesNoGoroutine(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("MET-010/sem goroutine restante depois de Wait", func(t *testing.T) {
		clock := domain.NewFakeClock(limiterStart)
		l := scrapers.NewLimiter(clock, scrapers.IntervalIGDB)
		guard := guardContext(t)

		if err := expectReturn(t, callWait(l, context.Background())); err != nil {
			t.Fatalf("primeira chamada: erro %v", err)
		}
		ch := callWait(l, context.Background())
		if err := clock.BlockUntilWaiters(guard, 1); err != nil {
			t.Fatalf("segunda chamada não registrou espera: %v", err)
		}
		clock.Advance(scrapers.IntervalIGDB)
		if err := expectReturn(t, ch); err != nil {
			t.Fatalf("segunda chamada: erro %v", err)
		}
		// Limiter.Wait itself owns no goroutine: goleak checks the test's own callWait goroutines
		// have ended once the calls returned.
	})
}
