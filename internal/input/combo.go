package input

import "time"

const (
	defaultHold   = time.Second
	defaultPoll   = 16 * time.Millisecond
	defaultRescan = time.Second
	defaultMask   = ButtonBack | ButtonY

	// A volume chord steps once when pressed and, if still held after defaultRepeatDelay, again every defaultRepeatEvery.
	defaultRepeatDelay = 400 * time.Millisecond
	defaultRepeatEvery = 150 * time.Millisecond
)

// defaultCloseMask is Back+B (a var, not a const, so the test's redundant check vets cleanly).
var defaultCloseMask = ButtonBack | ButtonB

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

// repeatDetector fires when its button mask is pressed and keeps firing at a steady rate while it is held,
// per slot: the first step is immediate, the next ones start after Delay and come every Every.
type repeatDetector struct {
	Mask  uint16
	Delay time.Duration
	Every time.Duration
	since [padSlots]time.Time
	last  [padSlots]time.Time
}

// Observe reports whether the mask asks for one more step on slot now.
func (d *repeatDetector) Observe(slot int, buttons uint16, connected bool, now time.Time) bool {
	if slot < 0 || slot >= padSlots || d.Mask == 0 {
		return false
	}
	if !connected || buttons&d.Mask != d.Mask {
		d.since[slot] = time.Time{}
		return false
	}
	if d.since[slot].IsZero() {
		d.since[slot], d.last[slot] = now, now
		return true
	}
	delay, every := d.Delay, d.Every
	if delay <= 0 {
		delay = defaultRepeatDelay
	}
	if every <= 0 {
		every = defaultRepeatEvery
	}
	if now.Sub(d.since[slot]) < delay || now.Sub(d.last[slot]) < every {
		return false
	}
	d.last[slot] = now
	return true
}
