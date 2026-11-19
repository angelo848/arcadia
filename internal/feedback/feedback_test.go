package feedback_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/feedback"
)

const waitTimeout = 2 * time.Second

// quiet is how long a negative assertion ("nothing else happens") observes.
const quiet = 50 * time.Millisecond

// recorder records the calls made to the fake Overlay and Sound, in order.
type recorder struct {
	mu    sync.Mutex
	calls []string
	ch    chan string
}

func newRecorder() *recorder { return &recorder{ch: make(chan string, 256)} }

func (r *recorder) add(s string) {
	r.mu.Lock()
	r.calls = append(r.calls, s)
	r.mu.Unlock()
	r.ch <- s
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *recorder) count(s string) int {
	n := 0
	for _, c := range r.snapshot() {
		if c == s {
			n++
		}
	}
	return n
}

// next waits for the next recorded call.
func (r *recorder) next(t *testing.T) string {
	t.Helper()
	select {
	case s := <-r.ch:
		return s
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for a call")
		return ""
	}
}

// expect waits for exactly the call want as the next one.
func (r *recorder) expect(t *testing.T, want string) {
	t.Helper()
	if got := r.next(t); got != want {
		t.Fatalf("next call = %q, want %q", got, want)
	}
}

// expectSet waits for len(want) calls and compares them ignoring order.
func (r *recorder) expectSet(t *testing.T, want ...string) {
	t.Helper()
	remaining := map[string]int{}
	for _, w := range want {
		remaining[w]++
	}
	for range want {
		got := r.next(t)
		if remaining[got] == 0 {
			t.Fatalf("unexpected call %q (still expecting %v)", got, remaining)
		}
		remaining[got]--
	}
}

// expectNone asserts that no call arrives during a short window.
func (r *recorder) expectNone(t *testing.T) {
	t.Helper()
	select {
	case s := <-r.ch:
		t.Fatalf("unexpected call %q", s)
	case <-time.After(quiet):
	}
}

type fakeOverlay struct {
	rec      *recorder
	closeErr error
	// block, when non-nil, makes Show wait until it is closed.
	block   chan struct{}
	entered chan struct{}
}

func (o *fakeOverlay) Show(n feedback.Notice) {
	if o.entered != nil {
		select {
		case o.entered <- struct{}{}:
		default:
		}
	}
	if o.block != nil {
		<-o.block
	}
	o.rec.add("show:" + n.Text())
}
func (o *fakeOverlay) Hide() { o.rec.add("hide") }
func (o *fakeOverlay) Close() error {
	o.rec.add("close-overlay")
	return o.closeErr
}

type fakeSound struct {
	rec      *recorder
	closeErr error
}

func (s *fakeSound) Play(c feedback.Cue) {
	switch c {
	case feedback.CuePaused:
		s.rec.add("play:paused")
	case feedback.CueResumed:
		s.rec.add("play:resumed")
	case feedback.CueRefused: // alterado em 2026-11-15 (fase 13)
		s.rec.add("play:refused")
	default:
		s.rec.add("play:unknown")
	}
}
func (s *fakeSound) Close() error {
	s.rec.add("close-sound")
	return s.closeErr
}

// warnHandler counts records at Warn level or above.
type warnHandler struct{ n *atomic.Int32 }

func (h warnHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h warnHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		h.n.Add(1)
	}
	return nil
}
func (h warnHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h warnHandler) WithGroup(string) slog.Handler      { return h }

func newLogger() (*slog.Logger, *atomic.Int32) {
	n := new(atomic.Int32)
	return slog.New(warnHandler{n: n}), n
}

func waitWaiters(t *testing.T, clk *domain.FakeClock, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	if err := clk.BlockUntilWaiters(ctx, n); err != nil {
		t.Fatalf("waiting for %d clock waiters: %v", n, err)
	}
}

func startAt() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }

func TestNotice(t *testing.T) {
	t.Run("OVL-001/NoticePaused é Tempo pausado e NoticeResumed é Tempo retomado", func(t *testing.T) {
		tests := []struct {
			n    feedback.Notice
			want string
		}{
			{feedback.NoticePaused, "Tempo pausado"},
			{feedback.NoticeResumed, "Tempo retomado"},
		}
		for _, tc := range tests {
			if got := tc.n.Text(); got != tc.want {
				t.Errorf("Notice(%d).Text() = %q, want %q", tc.n, got, tc.want)
			}
		}
	})
}

// TestCoordinator is not parallel: goleak inspects process-wide goroutines.
func TestCoordinator(t *testing.T) {
	// start builds a started coordinator. goleak is registered before Start so it
	// runs after the Close cleanup (LIFO).
	start := func(t *testing.T, o feedback.Overlay, s feedback.Sound, clk domain.Clock, log *slog.Logger) *feedback.Coordinator {
		t.Helper()
		t.Cleanup(func() { goleak.VerifyNone(t) })
		c := feedback.New(o, s, clk, log)
		c.Start()
		t.Cleanup(func() { _ = c.Close() })
		return c
	}

	t.Run("FBK-002/EventPaused chama Show(NoticePaused) e Play(CuePaused) uma vez cada; EventResumed os de retomada", func(t *testing.T) {
		rec := newRecorder()
		log, warns := newLogger()
		c := start(t, &fakeOverlay{rec: rec}, &fakeSound{rec: rec}, domain.NewFakeClock(startAt()), log)

		c.Send(feedback.EventPaused)
		rec.expectSet(t, "show:Tempo pausado", "play:paused")
		c.Send(feedback.EventResumed)
		rec.expectSet(t, "show:Tempo retomado", "play:resumed")
		rec.expectNone(t)
		if n := warns.Load(); n != 0 {
			t.Errorf("Warn count = %d, want 0", n)
		}
	})

	t.Run("CON-015/EventRefused toca só CueRefused e nunca chama Overlay.Show", func(t *testing.T) {
		rec := newRecorder()
		log, warns := newLogger()
		clk := domain.NewFakeClock(startAt())
		c := start(t, &fakeOverlay{rec: rec}, &fakeSound{rec: rec}, clk, log)

		if feedback.EventRefused != feedback.Event(3) || feedback.CueRefused != feedback.Cue(3) {
			t.Fatalf("EventRefused = %d, CueRefused = %d, want 3 and 3", feedback.EventRefused, feedback.CueRefused)
		}
		c.Send(feedback.EventRefused)
		rec.expect(t, "play:refused")
		rec.expectNone(t) // no show, no hide
		if n := warns.Load(); n != 0 {
			t.Errorf("Warn count = %d, want 0", n)
		}
	})

	t.Run("CON-015/EventRefused não esconde o aviso de pausa, que fica até a retomada", func(t *testing.T) {
		rec := newRecorder()
		log, _ := newLogger()
		clk := domain.NewFakeClock(startAt())
		c := start(t, &fakeOverlay{rec: rec}, &fakeSound{rec: rec}, clk, log)

		c.Send(feedback.EventPaused)
		rec.expectSet(t, "show:Tempo pausado", "play:paused")

		clk.Advance(time.Minute)
		c.Send(feedback.EventRefused)
		rec.expect(t, "play:refused")
		clk.Advance(time.Hour)
		rec.expectNone(t)
	})

	t.Run("FBK-006/o aviso de pausa fica fixo; EventDismiss o esconde, e sem aviso fixo não faz nada", func(t *testing.T) {
		rec := newRecorder()
		log, _ := newLogger()
		clk := domain.NewFakeClock(startAt())
		c := start(t, &fakeOverlay{rec: rec}, nil, clk, log)

		c.Send(feedback.EventDismiss)
		rec.expectNone(t)
		c.Send(feedback.EventPaused)
		rec.expect(t, "show:Tempo pausado")
		clk.Advance(time.Hour)
		rec.expectNone(t)
		c.Send(feedback.EventDismiss)
		rec.expect(t, "hide")
		c.Send(feedback.EventDismiss)
		rec.expectNone(t)
	})

	t.Run("CON-015/EventRefused sem overlay (e sem som) não falha", func(t *testing.T) {
		t.Run("overlay nil", func(t *testing.T) {
			rec := newRecorder()
			log, _ := newLogger()
			c := start(t, nil, &fakeSound{rec: rec}, domain.NewFakeClock(startAt()), log)
			c.Send(feedback.EventRefused)
			rec.expect(t, "play:refused")
		})
		t.Run("ambos nil", func(t *testing.T) {
			c := start(t, nil, nil, domain.NewFakeClock(startAt()), nil)
			c.Send(feedback.EventRefused)
		})
	})

	t.Run("CON-014/EventPaused e EventResumed continuam como antes (FBK-002); sem som de sucesso do fechar", func(t *testing.T) {
		rec := newRecorder()
		c := start(t, &fakeOverlay{rec: rec}, &fakeSound{rec: rec}, domain.NewFakeClock(startAt()), nil)
		c.Send(feedback.EventPaused)
		rec.expectSet(t, "show:Tempo pausado", "play:paused")
		c.Send(feedback.EventResumed)
		rec.expectSet(t, "show:Tempo retomado", "play:resumed")
		rec.expectNone(t)
		for _, name := range []string{"play:unknown", "play:refused"} {
			if rec.count(name) != 0 {
				t.Errorf("unexpected %s", name)
			}
		}
	})

	t.Run("FBK-002/Overlay ausente (nil) ainda toca o som e vice-versa; nada é propagado", func(t *testing.T) {
		t.Run("overlay nil", func(t *testing.T) {
			rec := newRecorder()
			log, _ := newLogger()
			c := start(t, nil, &fakeSound{rec: rec}, domain.NewFakeClock(startAt()), log)
			c.Send(feedback.EventPaused)
			rec.expect(t, "play:paused")
		})
		t.Run("sound nil", func(t *testing.T) {
			rec := newRecorder()
			log, _ := newLogger()
			c := start(t, &fakeOverlay{rec: rec}, nil, domain.NewFakeClock(startAt()), log)
			c.Send(feedback.EventResumed)
			rec.expect(t, "show:Tempo retomado")
		})
		t.Run("ambos nil e logger nil", func(t *testing.T) {
			c := start(t, nil, nil, domain.NewFakeClock(startAt()), nil)
			c.Send(feedback.EventPaused)
			c.Send(feedback.EventResumed)
		})
	})

	t.Run("FBK-002/um dos dois com Close que devolve erro é logado em Warn e o outro é fechado mesmo assim", func(t *testing.T) {
		boom := errors.New("boom")
		tests := []struct {
			name string
			o    *fakeOverlay
			s    *fakeSound
		}{
			{"overlay falha", &fakeOverlay{closeErr: boom}, &fakeSound{}},
			{"sound falha", &fakeOverlay{}, &fakeSound{closeErr: boom}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				rec := newRecorder()
				tc.o.rec, tc.s.rec = rec, rec
				log, warns := newLogger()
				t.Cleanup(func() { goleak.VerifyNone(t) })
				c := feedback.New(tc.o, tc.s, domain.NewFakeClock(startAt()), log)
				c.Start()
				_ = c.Close() // the returned error is not asserted (spec is silent)
				if rec.count("close-overlay") != 1 || rec.count("close-sound") != 1 {
					t.Errorf("calls = %v, want one close-overlay and one close-sound", rec.snapshot())
				}
				if warns.Load() < 1 {
					t.Errorf("Warn count = %d, want >= 1", warns.Load())
				}
			})
		}
	})

	t.Run("OVL-003/o aviso de retomada esconde em t + 2,5 s e não em t + 2,5 s − 1 ns", func(t *testing.T) {
		rec := newRecorder()
		clk := domain.NewFakeClock(startAt())
		log, _ := newLogger()
		c := start(t, &fakeOverlay{rec: rec}, nil, clk, log)

		c.Send(feedback.EventResumed)
		rec.expect(t, "show:Tempo retomado")
		waitWaiters(t, clk, 1)

		clk.Advance(feedback.OverlayDuration - time.Nanosecond)
		rec.expectNone(t)
		clk.Advance(time.Nanosecond)
		rec.expect(t, "hide")
		rec.expectNone(t)
	})

	t.Run("OVL-003/pausa fixa, retomada em t + 1 s troca o texto e esconde 2,5 s depois, uma única vez", func(t *testing.T) {
		rec := newRecorder()
		clk := domain.NewFakeClock(startAt())
		log, _ := newLogger()
		c := start(t, &fakeOverlay{rec: rec}, nil, clk, log)

		c.Send(feedback.EventPaused)
		rec.expect(t, "show:Tempo pausado")

		clk.Advance(time.Second) // t + 1 s
		c.Send(feedback.EventResumed)
		rec.expect(t, "show:Tempo retomado")
		waitWaiters(t, clk, 1)

		clk.Advance(feedback.OverlayDuration - time.Nanosecond)
		rec.expectNone(t)
		clk.Advance(time.Nanosecond)
		rec.expect(t, "hide")
		rec.expectNone(t) // a single Hide
	})

	t.Run("FBK-004/Paused seguido de Resumed 100 ms depois são processados em ordem e o último texto é Tempo retomado", func(t *testing.T) {
		rec := newRecorder()
		clk := domain.NewFakeClock(startAt())
		log, _ := newLogger()
		c := start(t, &fakeOverlay{rec: rec}, &fakeSound{rec: rec}, clk, log)

		c.Send(feedback.EventPaused)
		rec.expectSet(t, "show:Tempo pausado", "play:paused")
		clk.Advance(100 * time.Millisecond)
		c.Send(feedback.EventResumed)
		rec.expectSet(t, "show:Tempo retomado", "play:resumed")

		var shows []string
		for _, s := range rec.snapshot() {
			if len(s) > 5 && s[:5] == "show:" {
				shows = append(shows, s)
			}
		}
		want := []string{"show:Tempo pausado", "show:Tempo retomado"}
		if len(shows) != 2 || shows[0] != want[0] || shows[1] != want[1] {
			t.Errorf("shows = %v, want %v", shows, want)
		}
	})

	t.Run("FBK-003/Show que bloqueia não bloqueia Send: a fila cap 4 cheia descarta o mais antigo com um Warn", func(t *testing.T) {
		rec := newRecorder()
		log, warns := newLogger()
		block := make(chan struct{})
		var once sync.Once
		release := func() { once.Do(func() { close(block) }) }
		ov := &fakeOverlay{rec: rec, block: block, entered: make(chan struct{}, 1)}

		t.Cleanup(func() { goleak.VerifyNone(t) })
		c := feedback.New(ov, nil, domain.NewFakeClock(startAt()), log)
		c.Start()
		t.Cleanup(func() { _ = c.Close() })
		t.Cleanup(release) // runs before Close: Show must be unblocked first

		c.Send(feedback.EventPaused) // taken by the goroutine, which blocks in Show
		select {
		case <-ov.entered:
		case <-time.After(waitTimeout):
			t.Fatal("Show was not called")
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			c.Send(feedback.EventResumed) // oldest queued event: must be the one dropped
			c.Send(feedback.EventPaused)
			c.Send(feedback.EventPaused)
			c.Send(feedback.EventPaused) // queue full (4)
			if n := warns.Load(); n != 0 {
				t.Errorf("Warn count before overflow = %d, want 0", n)
			}
			c.Send(feedback.EventPaused) // overflow: drops the oldest
		}()
		select {
		case <-done:
		case <-time.After(waitTimeout):
			t.Fatal("Send blocked while Show was blocked")
		}
		if n := warns.Load(); n != 1 {
			t.Errorf("Warn count = %d, want 1", n)
		}

		release()
		for i := 0; i < 5; i++ {
			rec.expect(t, "show:Tempo pausado") // the dropped EventResumed never shows
		}
		rec.expectNone(t)
	})

	t.Run("FBK-005/Close esconde o aviso, fecha Overlay e Sound e espera a goroutine (goleak); segunda chamada sem efeito", func(t *testing.T) {
		rec := newRecorder()
		log, _ := newLogger()
		goleak.VerifyNone(t) // clean baseline
		c := feedback.New(&fakeOverlay{rec: rec}, &fakeSound{rec: rec}, domain.NewFakeClock(startAt()), log)
		c.Start()
		c.Send(feedback.EventPaused)
		rec.expectSet(t, "show:Tempo pausado", "play:paused")

		_ = c.Close()
		goleak.VerifyNone(t) // immediately after Close, no waiting
		after := rec.snapshot()
		hide, closeO, closeS := -1, -1, -1
		for i, s := range after {
			switch s {
			case "hide":
				hide = i
			case "close-overlay":
				closeO = i
			case "close-sound":
				closeS = i
			}
		}
		if hide < 0 || closeO < 0 || closeS < 0 {
			t.Fatalf("calls = %v, want hide, close-overlay and close-sound", after)
		}
		if hide > closeO {
			t.Errorf("Hide came after Overlay.Close: %v", after)
		}

		_ = c.Close()
		if again := rec.snapshot(); len(again) != len(after) {
			t.Errorf("second Close had effects: %v -> %v", after, again)
		}
	})

	t.Run("FBK-005/Send depois do Close é descartado sem pânico", func(t *testing.T) {
		rec := newRecorder()
		log, _ := newLogger()
		t.Cleanup(func() { goleak.VerifyNone(t) })
		c := feedback.New(&fakeOverlay{rec: rec}, &fakeSound{rec: rec}, domain.NewFakeClock(startAt()), log)
		c.Start()
		_ = c.Close()
		before := len(rec.snapshot())
		for range before { // drain the Close calls
			rec.next(t)
		}

		for i := 0; i < 10; i++ { // more than the queue size
			c.Send(feedback.EventPaused)
			c.Send(feedback.EventResumed)
		}
		rec.expectNone(t)
		if after := len(rec.snapshot()); after != before {
			t.Errorf("calls after Close = %d, want %d", after, before)
		}
	})
}
