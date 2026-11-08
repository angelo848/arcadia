package domain

import (
	"context"
	"sync"
	"time"
)

// Clock abstracts the passage of time so that time-dependent logic can be tested
// deterministically. Production code must use Clock instead of calling the time
// package directly for the current instant or for waiting.
type Clock interface {
	// Now returns the current instant.
	Now() time.Time
	// After returns a channel that receives the current instant once d has elapsed.
	After(d time.Duration) <-chan time.Time
}

// SystemClock is the real Clock backed by the time package. It is the only place
// allowed to call time.Now and time.After.
type SystemClock struct{}

// Now returns the current wall-clock instant.
func (SystemClock) Now() time.Time { return time.Now() }

// After waits for d to elapse on the real clock and then sends the current instant.
func (SystemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// fakeWaiter is a pending After call on a FakeClock.
type fakeWaiter struct {
	deadline time.Time
	ch       chan time.Time
}

// FakeClock is a manually driven Clock for tests. It is safe for concurrent use.
// Time only moves when Advance is called.
type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []fakeWaiter
	// changed is closed and replaced every time a waiter is registered, waking
	// BlockUntilWaiters callers so they can re-check the pending count.
	changed chan struct{}
}

// NewFakeClock returns a FakeClock whose Now is start.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{now: start, changed: make(chan struct{})}
}

// Now returns the fake current instant.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After returns a channel with buffer 1 that receives the fake Now once the clock
// has been advanced by at least d. If d <= 0 the channel already holds Now and no
// waiter is registered.
func (c *FakeClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	c.mu.Lock()
	defer c.mu.Unlock()
	if d <= 0 {
		ch <- c.now
		return ch
	}
	c.waiters = append(c.waiters, fakeWaiter{deadline: c.now.Add(d), ch: ch})
	close(c.changed)
	c.changed = make(chan struct{})
	return ch
}

// Advance moves the clock forward by d, then fires and removes every waiter whose
// deadline is at or before the new Now, sending the new Now. Sends never block.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	pending := c.waiters[:0]
	for _, w := range c.waiters {
		if w.deadline.After(c.now) {
			pending = append(pending, w)
			continue
		}
		select {
		case w.ch <- c.now:
		default:
		}
	}
	// Clear the tail so fired channels can be garbage collected.
	for i := len(pending); i < len(c.waiters); i++ {
		c.waiters[i] = fakeWaiter{}
	}
	c.waiters = pending
}

// BlockUntilWaiters blocks until at least n waiters are pending or ctx is done,
// in which case it returns ctx.Err(). It wakes on every waiter registration and
// does not poll real time.
func (c *FakeClock) BlockUntilWaiters(ctx context.Context, n int) error {
	for {
		c.mu.Lock()
		count := len(c.waiters)
		changed := c.changed
		c.mu.Unlock()
		if count >= n {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}
