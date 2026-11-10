package input

import "time"

const (
	defaultHold   = time.Second
	defaultPoll   = 16 * time.Millisecond
	defaultRescan = time.Second
	defaultMask   = ButtonBack | ButtonY
)

const padSlots = 4

// comboDetector detects a button mask held for Hold, per slot. The zero value
// uses the defaults (1 s, Back|Y).
type comboDetector struct {
	Hold  time.Duration
	Mask  uint16
	since [padSlots]time.Time
	fired [padSlots]bool
}

// Observe reports whether the combo just completed on slot (once per press).
func (d *comboDetector) Observe(slot int, buttons uint16, connected bool, now time.Time) bool {
	if slot < 0 || slot >= padSlots {
		return false
	}
	hold, mask := d.Hold, d.Mask
	if hold <= 0 {
		hold = defaultHold
	}
	if mask == 0 {
		mask = defaultMask
	}
	if !connected || buttons&mask != mask {
		d.since[slot] = time.Time{}
		d.fired[slot] = false
		return false
	}
	if d.since[slot].IsZero() {
		d.since[slot] = now
	}
	if d.fired[slot] || now.Sub(d.since[slot]) < hold {
		return false
	}
	d.fired[slot] = true
	return true
}
