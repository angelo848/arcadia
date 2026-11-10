//go:build windows

package winapi

import (
	"fmt"
	"io"
	"log/slog"
	"sync"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/feedback"
)

var (
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	procBeep    = modKernel32.NewProc("Beep")
)

// systemBeep calls kernel32.Beep, which blocks until the tone has finished.
func systemBeep(freq, ms uint32) error {
	r, _, err := procBeep.Call(uintptr(freq), uintptr(ms))
	if r == 0 {
		return fmt.Errorf("Beep(%d, %d): %w", freq, ms, err)
	}
	return nil
}

// Beeper implements feedback.Sound with two-tone signals. Play never blocks:
// a single owner goroutine plays the signals, and a newer cue replaces the one
// still pending.
type Beeper struct {
	log  *slog.Logger
	beep func(freq, ms uint32) error

	cues chan feedback.Cue // capacity 1: the pending cue
	stop chan struct{}
	wg   sync.WaitGroup

	mu     sync.Mutex // guards closed and the replace logic in Play
	closed bool
}

var _ feedback.Sound = (*Beeper)(nil)

// NewBeeper creates a Beeper that uses kernel32.Beep.
func NewBeeper(log *slog.Logger) *Beeper { return newBeeper(log, systemBeep) }

func newBeeper(log *slog.Logger, beep func(freq, ms uint32) error) *Beeper {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	b := &Beeper{
		log:  log,
		beep: beep,
		cues: make(chan feedback.Cue, 1),
		stop: make(chan struct{}),
	}
	b.wg.Add(1)
	go b.run()
	return b
}

// Play queues the cue, replacing a pending one. It never blocks.
func (b *Beeper) Play(c feedback.Cue) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	select { // drop the pending cue, if any
	case <-b.cues:
	default:
	}
	select {
	case b.cues <- c:
	default:
	}
}

// Close stops the owner goroutine and waits for it. It is idempotent.
func (b *Beeper) Close() error {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		close(b.stop)
	}
	b.mu.Unlock()
	b.wg.Wait()
	return nil
}

func (b *Beeper) run() {
	defer b.wg.Done()
	for {
		select {
		case <-b.stop:
			return
		case c := <-b.cues:
			b.signal(c)
		}
	}
}

func (b *Beeper) signal(c feedback.Cue) {
	var first, second uint32
	switch c {
	case feedback.CuePaused:
		first, second = 660, 440
	case feedback.CueResumed:
		first, second = 440, 660
	default:
		return
	}
	b.tone(first, 90)
	select {
	case <-b.stop:
		return
	default:
	}
	b.tone(second, 120)
}

func (b *Beeper) tone(freq, ms uint32) {
	if err := b.beep(freq, ms); err != nil {
		b.log.Warn("beep failed", "freq", freq, "ms", ms, "err", err)
	}
}
