// Package feedback shows the pause/resume notice and plays the matching sound.
// It is portable: the Windows implementations of Overlay and Sound live elsewhere.
package feedback

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// Notice is the message shown by the overlay.
type Notice int

const (
	NoticePaused Notice = iota + 1
	NoticeResumed
)

// Text returns the user-visible text of the notice.
func (n Notice) Text() string {
	switch n {
	case NoticePaused:
		return "Tempo pausado"
	case NoticeResumed:
		return "Tempo retomado"
	default:
		return ""
	}
}

// Cue identifies a sound.
type Cue int

const (
	CuePaused Cue = iota + 1
	CueResumed
)

// Overlay shows a transient notice.
type Overlay interface {
	Show(Notice)
	Hide()
	Close() error
}

// Sound plays a cue.
type Sound interface {
	Play(Cue)
	Close() error
}

// NopOverlay is an Overlay that does nothing.
type NopOverlay struct{}

func (NopOverlay) Show(Notice)  {}
func (NopOverlay) Hide()        {}
func (NopOverlay) Close() error { return nil }

// NopSound is a Sound that does nothing.
type NopSound struct{}

func (NopSound) Play(Cue)     {}
func (NopSound) Close() error { return nil }

// Event is a feedback request; values match tracker.FeedbackKind.
type Event int

const (
	EventPaused Event = iota + 1
	EventResumed
)

// OverlayDuration is how long the notice stays visible after the last event.
const OverlayDuration = 2500 * time.Millisecond

const queueSize = 4

// Coordinator serializes feedback events on one owner goroutine.
type Coordinator struct {
	overlay Overlay
	sound   Sound
	clock   domain.Clock
	log     *slog.Logger

	events chan Event
	stop   chan struct{}
	wg     sync.WaitGroup

	mu     sync.Mutex // guards closed and Send's queue operations
	closed bool
}

// New builds a Coordinator; nil overlay, sound or log are replaced by no-ops.
func New(o Overlay, s Sound, clock domain.Clock, log *slog.Logger) *Coordinator {
	if o == nil {
		o = NopOverlay{}
	}
	if s == nil {
		s = NopSound{}
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Coordinator{
		overlay: o,
		sound:   s,
		clock:   clock,
		log:     log,
		events:  make(chan Event, queueSize),
		stop:    make(chan struct{}),
	}
}

// Start launches the owner goroutine. Call it once.
func (c *Coordinator) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.wg.Add(1)
	go c.run()
}

// Send enqueues e without blocking. A full queue drops the oldest event.
func (c *Coordinator) Send(e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.events <- e:
		return
	default:
	}
	select {
	case <-c.events:
	default:
	}
	c.log.Warn("feedback queue full; dropping oldest event")
	select {
	case c.events <- e:
	default:
	}
}

// Close hides the notice, closes Overlay and Sound and waits for the goroutine.
// It is idempotent.
func (c *Coordinator) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.stop)
	c.mu.Unlock()

	c.wg.Wait()

	c.overlay.Hide()
	var errs []error
	if err := c.overlay.Close(); err != nil {
		c.log.Warn("closing overlay", "err", err)
		errs = append(errs, fmt.Errorf("close overlay: %w", err))
	}
	if err := c.sound.Close(); err != nil {
		c.log.Warn("closing sound", "err", err)
		errs = append(errs, fmt.Errorf("close sound: %w", err))
	}
	return errors.Join(errs...)
}

func (c *Coordinator) run() {
	defer c.wg.Done()
	// hide fires OverlayDuration after the latest event. Replacing the channel on
	// each event invalidates the previous timer (its channel is no longer read).
	var hide <-chan time.Time
	for {
		select {
		case <-c.stop:
			return
		case e := <-c.events:
			c.handle(e)
			hide = c.clock.After(OverlayDuration)
		case <-hide:
			hide = nil
			c.overlay.Hide()
		}
	}
}

func (c *Coordinator) handle(e Event) {
	switch e {
	case EventPaused:
		c.overlay.Show(NoticePaused)
		c.sound.Play(CuePaused)
	case EventResumed:
		c.overlay.Show(NoticeResumed)
		c.sound.Play(CueResumed)
	default:
		c.log.Warn("unknown feedback event", "event", int(e))
	}
}
