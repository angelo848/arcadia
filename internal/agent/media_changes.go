package agent

import (
	"sort"
	"time"
)

// mediaChangedWindow is the per-game coalescing window for media.changed
// notifications (IMG-003/IMG-004); it must match x-media.mediaChangedWindowMs
// in the IPC contract.
const mediaChangedWindow = 500 * time.Millisecond

// mediaChangeCoalescer decides when a media.changed notice may be sent for a
// game: the first change goes out immediately and opens a window; changes
// inside the window are folded into one pending notice sent when it ends.
// It is not safe for concurrent use and owns no goroutine or clock.
type mediaChangeCoalescer struct {
	window  time.Duration
	until   map[int64]time.Time
	pending map[int64]bool
}

func newMediaChangeCoalescer(window time.Duration) *mediaChangeCoalescer {
	return &mediaChangeCoalescer{
		window:  window,
		until:   make(map[int64]time.Time),
		pending: make(map[int64]bool),
	}
}

// changed records a change and reports whether the notice must be sent now.
func (c *mediaChangeCoalescer) changed(gameID int64, now time.Time) bool {
	if end, ok := c.until[gameID]; ok && (now.Before(end) || c.pending[gameID]) {
		c.pending[gameID] = true
		return false
	}
	c.until[gameID] = now.Add(c.window)
	return true
}

// due returns, sorted, the games whose pending notice must be sent at now,
// reopening their windows, and drops expired windows without pending changes.
func (c *mediaChangeCoalescer) due(now time.Time) []int64 {
	var ids []int64
	for id := range c.pending {
		if !c.until[id].After(now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		delete(c.pending, id)
		c.until[id] = now.Add(c.window)
	}
	for id, end := range c.until {
		if !end.After(now) && !c.pending[id] {
			delete(c.until, id)
		}
	}
	return ids
}

// next returns the earliest window end among pending games.
func (c *mediaChangeCoalescer) next() (time.Time, bool) {
	var best time.Time
	found := false
	for id := range c.pending {
		if end := c.until[id]; !found || end.Before(best) {
			best, found = end, true
		}
	}
	return best, found
}
