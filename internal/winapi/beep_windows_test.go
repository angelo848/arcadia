//go:build windows

package winapi

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/feedback"
)

const beepWait = 5 * time.Second

type tone struct{ freq, ms uint32 }

// fakeBeep records tones and can block every call on gate until it is released.
type fakeBeep struct {
	mu      sync.Mutex
	tones   []tone
	calls   chan tone // buffered; one entry per call, sent before blocking
	gate    chan struct{}
	release sync.Once
}

func newFakeBeep(blocking bool) *fakeBeep {
	f := &fakeBeep{calls: make(chan tone, 64)}
	if blocking {
		f.gate = make(chan struct{})
	}
	return f
}

func (f *fakeBeep) beep(freq, ms uint32) error {
	tn := tone{freq, ms}
	f.mu.Lock()
	f.tones = append(f.tones, tn)
	f.mu.Unlock()
	f.calls <- tn
	if f.gate != nil {
		<-f.gate
	}
	return nil
}

func (f *fakeBeep) open() {
	if f.gate != nil {
		f.release.Do(func() { close(f.gate) })
	}
}

func (f *fakeBeep) recorded() []tone {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]tone(nil), f.tones...)
}

func (f *fakeBeep) next(t *testing.T) tone {
	t.Helper()
	select {
	case tn := <-f.calls:
		return tn
	case <-time.After(beepWait):
		t.Fatal("timed out waiting for a tone")
		return tone{}
	}
}

// setupBeeper registers goleak first, then the beeper Close, then the gate
// release (cleanups run LIFO: release, Close, goleak).
func setupBeeper(t *testing.T, f *fakeBeep) *Beeper {
	t.Helper()
	ignore := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, ignore) })
	b := newBeeper(slog.New(slog.NewTextHandler(io.Discard, nil)), f.beep)
	t.Cleanup(func() { _ = b.Close() })
	t.Cleanup(f.open)
	return b
}

func TestBeeper(t *testing.T) {
	t.Run("SND-001/CuePaused toca 660 Hz por 90 ms e depois 440 Hz por 120 ms", func(t *testing.T) {
		f := newFakeBeep(false)
		b := setupBeeper(t, f)
		b.Play(feedback.CuePaused)
		if got := f.next(t); got != (tone{660, 90}) {
			t.Fatalf("first tone = %+v", got)
		}
		if got := f.next(t); got != (tone{440, 120}) {
			t.Fatalf("second tone = %+v", got)
		}
	})

	t.Run("SND-001/CueResumed toca 440 Hz por 90 ms e depois 660 Hz por 120 ms", func(t *testing.T) {
		f := newFakeBeep(false)
		b := setupBeeper(t, f)
		b.Play(feedback.CueResumed)
		if got := f.next(t); got != (tone{440, 90}) {
			t.Fatalf("first tone = %+v", got)
		}
		if got := f.next(t); got != (tone{660, 120}) {
			t.Fatalf("second tone = %+v", got)
		}
	})

	t.Run("SND-001/Play volta imediatamente com beep bloqueado", func(t *testing.T) {
		f := newFakeBeep(true)
		b := setupBeeper(t, f)
		done := make(chan struct{})
		go func() {
			defer close(done)
			b.Play(feedback.CuePaused)
			b.Play(feedback.CueResumed)
			b.Play(feedback.CuePaused)
		}()
		select {
		case <-done:
		case <-time.After(beepWait):
			t.Fatal("Play blocked while beep was blocked")
		}
	})

	t.Run("SND-002/Close termina a goroutine antes de retornar (goleak) e a segunda chamada não tem efeito", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		t.Cleanup(func() { goleak.VerifyNone(t, ignore) })
		f := newFakeBeep(false)
		b := newBeeper(slog.New(slog.NewTextHandler(io.Discard, nil)), f.beep)
		b.Play(feedback.CuePaused)
		f.next(t)
		f.next(t)
		if err := b.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		// No leak right after Close returned: Close must have waited.
		goleak.VerifyNone(t, ignore)
		if err := b.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
	})

	t.Run("SND-002/Play depois do Close não tem efeito", func(t *testing.T) {
		f := newFakeBeep(false)
		b := setupBeeper(t, f)
		if err := b.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		b.Play(feedback.CuePaused)
		b.Play(feedback.CueResumed)
		select {
		case tn := <-f.calls:
			t.Fatalf("beep called after Close: %+v", tn)
		case <-time.After(100 * time.Millisecond):
		}
		if n := len(f.recorded()); n != 0 {
			t.Fatalf("recorded %d tones after Close", n)
		}
	})

	t.Run("SND-002/Play durante um sinal em curso deixa no máximo um pendente: o mais novo substitui o pendente", func(t *testing.T) {
		f := newFakeBeep(true)
		b := setupBeeper(t, f)
		b.Play(feedback.CuePaused)
		if got := f.next(t); got != (tone{660, 90}) { // in progress, blocked
			t.Fatalf("first tone = %+v", got)
		}
		b.Play(feedback.CueResumed) // pending
		b.Play(feedback.CuePaused)  // replaces the pending one
		f.open()

		want := []tone{{440, 120}, {660, 90}, {440, 120}}
		for i, w := range want {
			if got := f.next(t); got != w {
				t.Fatalf("tone %d = %+v, want %+v", i+2, got, w)
			}
		}
		select {
		case tn := <-f.calls:
			t.Fatalf("extra tone played (replaced cue not dropped?): %+v", tn)
		case <-time.After(100 * time.Millisecond):
		}
	})

	t.Run("SND-002/Close com beep bloqueado ainda retorna depois de liberá-lo no cleanup", func(t *testing.T) {
		f := newFakeBeep(true)
		b := setupBeeper(t, f)
		b.Play(feedback.CuePaused)
		f.next(t) // goroutine is now blocked inside beep

		closed := make(chan error, 1)
		go func() { closed <- b.Close() }()
		f.open() // what the cleanup does: release the blocked beep
		select {
		case err := <-closed:
			if err != nil {
				t.Fatalf("Close: %v", err)
			}
		case <-time.After(beepWait):
			t.Fatal("Close did not return after the beep was released")
		}
	})
}
