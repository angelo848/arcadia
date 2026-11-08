package domain

import (
	"slices"
	"time"
)

// Interval is a time span. A zero End means the interval is still open.
type Interval struct {
	Start, End time.Time
}

// ActiveDuration returns the time between start and end that is not covered by
// any pause. Open pauses (zero End) extend to end, closed pauses with
// End < Start are ignored, pauses are clipped to [start, end] and overlapping
// pauses are counted once. It returns 0 when end <= start. The pauses slice is
// not modified.
func ActiveDuration(start, end time.Time, pauses []Interval) time.Duration {
	if !end.After(start) {
		return 0
	}

	clipped := make([]Interval, 0, len(pauses))
	for _, p := range pauses {
		pEnd := p.End
		if pEnd.IsZero() {
			pEnd = end
		} else if pEnd.Before(p.Start) {
			continue
		}
		pStart := p.Start
		if pStart.Before(start) {
			pStart = start
		}
		if pEnd.After(end) {
			pEnd = end
		}
		if !pEnd.After(pStart) {
			continue
		}
		clipped = append(clipped, Interval{Start: pStart, End: pEnd})
	}

	slices.SortFunc(clipped, func(a, b Interval) int { return a.Start.Compare(b.Start) })

	var paused time.Duration
	var cur Interval
	for i, p := range clipped {
		if i == 0 {
			cur = p
			continue
		}
		if !p.Start.After(cur.End) {
			if p.End.After(cur.End) {
				cur.End = p.End
			}
			continue
		}
		paused += cur.End.Sub(cur.Start)
		cur = p
	}
	if len(clipped) > 0 {
		paused += cur.End.Sub(cur.Start)
	}

	return end.Sub(start) - paused
}
