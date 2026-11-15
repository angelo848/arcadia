package scrapers

import (
	"context"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// Minimum spacing between two requests of the same provider [MET-010].
const (
	IntervalIGDB        = 334 * time.Millisecond
	IntervalSteamGridDB = 500 * time.Millisecond
	IntervalSteamStore  = 2 * time.Second
)

// Limiter spaces the requests of one provider [MET-010]. The next free slot is kept under a mutex:
// there is no goroutine and no queue. A nil *Limiter never waits.
type Limiter struct {
	mu       sync.Mutex
	clock    domain.Clock
	interval time.Duration
	next     time.Time // start of the next free slot; zero or in the past means free now
}

// NewLimiter returns a Limiter that lets one request through every interval, measured on clock.
func NewLimiter(clock domain.Clock, interval time.Duration) *Limiter {
	return &Limiter{clock: clock, interval: interval}
}

// reserve takes the next free slot and returns how long the caller must wait for it.
// The slot stays reserved even if the caller stops waiting.
func (l *Limiter) reserve() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	slot := now
	if l.next.After(now) {
		slot = l.next
	}
	l.next = slot.Add(l.interval)
	return slot.Sub(now)
}

// SetInterval changes the spacing used from the next reservation on; a slot already reserved does not move.
func (l *Limiter) SetInterval(d time.Duration) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.interval = d
}

// Wait reserves the caller's slot and blocks until it is due. It returns ctx.Err() when ctx ends
// first; the reservation is not given back in that case.
func (l *Limiter) Wait(ctx context.Context) error {
	if l == nil {
		return nil
	}
	wait := l.reserve()
	if err := ctx.Err(); err != nil {
		return err
	}
	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.clock.After(wait):
		return nil
	}
}
