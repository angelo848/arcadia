package domain

import (
	"fmt"
	"time"
)

// SessionSource tells how a play session was created.
type SessionSource string

const (
	SourceTracked SessionSource = "tracked"
	SourceImport  SessionSource = "import"
	SourceManual  SessionSource = "manual"
)

// ParseSessionSource accepts exactly the three wire values.
func ParseSessionSource(s string) (SessionSource, error) {
	switch v := SessionSource(s); v {
	case SourceTracked, SourceImport, SourceManual:
		return v, nil
	}
	return "", fmt.Errorf("invalid session source %q", s)
}

const (
	// MaxEditedSessionSpan is the longest interval an edited session may cover.
	MaxEditedSessionSpan = 24 * time.Hour
	// MinEditedSessionActive is the DOM-008 limit.
	MinEditedSessionActive = MinCountedSession
)

// EarliestSessionStart is the earliest start accepted for an edited session.
var EarliestSessionStart = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// EditReason is a wire-format reason for refusing a session edit.
type EditReason string

const (
	ReasonEndNotAfterStart  EditReason = "end-not-after-start"
	ReasonInFuture          EditReason = "in-future"
	ReasonTooEarly          EditReason = "too-early"
	ReasonSpanTooLong       EditReason = "span-too-long"
	ReasonActiveTooShort    EditReason = "active-too-short"
	ReasonActiveExceedsSpan EditReason = "active-exceeds-span"
	ReasonDiscarded         EditReason = "discarded"
	ReasonExceedsAdjustment EditReason = "exceeds-adjustment"
	ReasonEmptyPatch        EditReason = "empty-patch"
)

// SessionTimes is the interval and played time of a session being edited.
type SessionTimes struct {
	Start, End time.Time
	Active     time.Duration
}

// ValidateSessionTimes returns "" when valid, or the first failing reason.
func ValidateSessionTimes(t SessionTimes, now time.Time, spanChecked bool) EditReason {
	switch {
	case t.Start.Before(EarliestSessionStart):
		return ReasonTooEarly
	case !t.End.After(t.Start):
		return ReasonEndNotAfterStart
	case t.End.After(now):
		return ReasonInFuture
	case spanChecked && t.End.Sub(t.Start) > MaxEditedSessionSpan:
		return ReasonSpanTooLong
	case t.Active < MinEditedSessionActive:
		return ReasonActiveTooShort
	case t.Active > t.End.Sub(t.Start):
		return ReasonActiveExceedsSpan
	}
	return ""
}

// Overlaps reports whether two half-open intervals cross; touching is fine.
func Overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}

// ClampTotal limits seconds to [0, max].
func ClampTotal(seconds, max int64) int64 {
	if seconds < 0 {
		return 0
	}
	if seconds > max {
		return max
	}
	return seconds
}

// ClipPause cuts a pause to [start, end]; a nil resumedAt means the session end.
// ok is false when nothing is left of the pause.
func ClipPause(pausedAt time.Time, resumedAt *time.Time, start, end time.Time) (p, r time.Time, ok bool) {
	p, r = pausedAt, end
	if resumedAt != nil {
		r = *resumedAt
	}
	if p.Before(start) {
		p = start
	}
	if r.After(end) {
		r = end
	}
	if !r.After(p) {
		return time.Time{}, time.Time{}, false
	}
	return p, r, true
}
