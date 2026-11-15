package agent

import (
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// batchSink receives the progress of a metadata batch (MET-054). batchNotifier is the production sink.
type batchSink interface {
	BatchChanged(p ipc.MetadataBatchChangedParams)
}

// progressCoalescer decides when a batch counter change is sent [MET-054, batch-state-machine.md §3].
// The first counter change goes out at once and opens a window of `window`; the changes inside the
// window fold into one notice, sent at the end of the window (due). A state change always goes out at
// once and reopens the window, discarding the pending counter notice. The times are the caller's
// (domain.Clock); there is no goroutine and no timer. Not safe for concurrent use: only the batch
// coordinator calls it.
type progressCoalescer struct {
	window  time.Duration
	until   time.Time // end of the open window; zero when no window was opened
	pending bool      // a counter change arrived inside the open window
}

func newProgressCoalescer(window time.Duration) *progressCoalescer {
	return &progressCoalescer{window: window}
}

// windowOpen reports whether a window is still open at now.
func (c *progressCoalescer) windowOpen(now time.Time) bool {
	return !c.until.IsZero() && now.Before(c.until)
}

// counters records a done/failed change at now and reports whether it must be sent now. Outside
// a window it is sent and opens one; inside, it is held as pending.
func (c *progressCoalescer) counters(now time.Time) (sendNow bool) {
	if c.windowOpen(now) {
		c.pending = true
		return false
	}
	c.until = now.Add(c.window)
	c.pending = false
	return true
}

// state records a state change at now: it is sent by the caller at once, reopens the window and
// drops any pending counter notice.
func (c *progressCoalescer) state(now time.Time) {
	c.until = now.Add(c.window)
	c.pending = false
}

// due reports whether the pending counter notice is ready to be sent at now. When it is, the
// window reopens from now, so later changes fold into the next notice.
func (c *progressCoalescer) due(now time.Time) bool {
	if !c.pending || c.windowOpen(now) {
		return false
	}
	c.pending = false
	c.until = now.Add(c.window)
	return true
}

// next returns the instant at which the pending notice is due, and false when nothing is pending.
// The coordinator arms its clock timer with it only when there is something to send.
func (c *progressCoalescer) next() (time.Time, bool) {
	if !c.pending {
		return time.Time{}, false
	}
	return c.until, true
}
