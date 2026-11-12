package ipc

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// milliLayout is RFC 3339 with exactly three fractional digits (".000" included).
const milliLayout = "2006-01-02T15:04:05.000Z07:00"

// MilliTime is an instant serialized in UTC with millisecond precision
// ("2026-11-09T14:03:12.345Z"), even when the milliseconds are zero [IPC-031].
type MilliTime struct{ time.Time }

// MarshalJSON encodes t in UTC with the milliLayout layout (fraction truncated).
func (t MilliTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.UTC().Format(milliLayout))
}

// UnmarshalJSON decodes an RFC 3339 string (with or without fraction). JSON
// null leaves t unchanged.
func (t *MilliTime) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("ipc: MilliTime: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("ipc: MilliTime: %w", err)
	}
	t.Time = parsed
	return nil
}

// SessionFromSnapshot converts a domain snapshot to its wire form: activeMs is
// Active truncated to whole milliseconds, activeSeconds is activeMs/1000, all
// instants are UTC and endedAt is null while the session has not ended [IPC-031].
func SessionFromSnapshot(s domain.SessionSnapshot) Session {
	ms := s.Active.Milliseconds()
	out := Session{
		SessionID:     s.ID,
		GameID:        s.GameID,
		Title:         s.Title,
		State:         string(s.State),
		StartedAt:     s.StartedAt.UTC(),
		ActiveSeconds: ms / 1000,
		ActiveMs:      ms,
		AsOf:          MilliTime{Time: s.AsOf.UTC()},
		Tracking:      trackingName(s.Tracking),
	}
	if !s.EndedAt.IsZero() {
		ended := s.EndedAt.UTC()
		out.EndedAt = &ended
	}
	return out
}

// trackingName returns the wire name of a tracking mode; a session without one is a
// job session, as before the watcher existed (RUL-005).
func trackingName(m domain.TrackingMode) string {
	if m == "" {
		return string(domain.TrackJob)
	}
	return string(m)
}
