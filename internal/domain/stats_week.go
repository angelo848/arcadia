package domain

import "time"

// TimedSeconds is the active time of one session and the instant it started (EST-002).
type TimedSeconds struct {
	Start   time.Time
	Seconds int64
}

// WeekTotal is the time played in one week. Start is the Monday 00:00 of the week in the location
// used for the totals; Current marks the week that holds now (EST-002).
type WeekTotal struct {
	Start   time.Time
	Seconds int64
	Current bool
}

// WeeklyTotals returns the time of the last weeks, oldest first, the current week last. Weeks start
// on Monday, in loc. A week without time is in the result with zero seconds (EST-002).
func WeeklyTotals(sessions []TimedSeconds, now time.Time, loc *time.Location, weeks int) []WeekTotal {
	if weeks < 1 {
		return nil
	}
	local := now.In(loc)
	current := weekStart(local)
	first := current.AddDate(0, 0, -7*(weeks-1))

	out := make([]WeekTotal, weeks)
	for i := range out {
		start := first.AddDate(0, 0, 7*i)
		out[i] = WeekTotal{Start: start, Current: start.Equal(current)}
	}
	for _, s := range sessions {
		at := s.Start.In(loc)
		if at.Before(first) || !at.Before(current.AddDate(0, 0, 7)) {
			continue
		}
		idx := int(weekStart(at).Sub(first).Hours() / (24 * 7))
		if idx >= 0 && idx < weeks {
			out[idx].Seconds += s.Seconds
		}
	}
	return out
}

// weekStart returns 00:00 of the Monday of the week of t, in t's location.
func weekStart(t time.Time) time.Time {
	offset := (int(t.Weekday()) + 6) % 7 // Monday = 0, Sunday = 6
	d := t.AddDate(0, 0, -offset)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, t.Location())
}
