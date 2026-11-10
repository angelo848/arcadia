package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// ErrPauseOpen is returned when a session already has an open pause interval.
var ErrPauseOpen = errors.New("store: pause already open")

// PauseStart describes the opening of a pause interval.
type PauseStart struct {
	SessionID int64
	At        time.Time
	Reason    domain.PauseReason
	Active    time.Duration
}

// PauseEnd describes the closing of a pause interval.
type PauseEnd struct {
	SessionID int64
	At        time.Time
	State     domain.SessionState // running | grace
	Active    time.Duration
}

// PauseInterval is a stored pause; a zero ResumedAt means it is still open.
type PauseInterval struct {
	ID, SessionID       int64
	PausedAt, ResumedAt time.Time
	Reason              domain.PauseReason
}

// PauseSession marks an open session as paused and opens a pause interval, in
// one transaction (STO-019). It returns ErrSessionNotOpen when the session is
// missing or ended and ErrPauseOpen when a pause is already open.
func (s *Store) PauseSession(ctx context.Context, p PauseStart) error {
	at := TimeToMillis(p.At)
	return s.Tx(ctx, func(q *dbgen.Queries) error {
		n, err := q.UpdateOpenSession(ctx, dbgen.UpdateOpenSessionParams{
			State:         string(domain.SessionPaused),
			LastSeenAt:    at,
			ActiveSeconds: activeSeconds(p.Active),
			ID:            p.SessionID,
		})
		if err != nil {
			return fmt.Errorf("store: pause session %d: update: %w", p.SessionID, err)
		}
		if n == 0 {
			return fmt.Errorf("store: pause session %d: %w", p.SessionID, ErrSessionNotOpen)
		}
		_, err = q.GetOpenPause(ctx, p.SessionID)
		if err == nil {
			return fmt.Errorf("store: pause session %d: %w", p.SessionID, ErrPauseOpen)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: pause session %d: read open pause: %w", p.SessionID, err)
		}
		if err := q.InsertPauseOpen(ctx, dbgen.InsertPauseOpenParams{
			SessionID: p.SessionID, PausedAt: at, Reason: string(p.Reason),
		}); err != nil {
			return fmt.Errorf("store: pause session %d: insert: %w", p.SessionID, err)
		}
		return nil
	})
}

// ResumePause closes the open pause of a session (deleting it when it has zero
// length in milliseconds) and records the given state (STO-020). Without an
// open pause only the session is updated. A missing or ended session yields
// ErrSessionNotOpen.
func (s *Store) ResumePause(ctx context.Context, p PauseEnd) error {
	at := TimeToMillis(p.At)
	return s.Tx(ctx, func(q *dbgen.Queries) error {
		open, err := q.GetOpenPause(ctx, p.SessionID)
		switch {
		case err == nil:
			if open.PausedAt == at {
				err = q.DeletePause(ctx, open.ID)
			} else {
				err = q.ClosePause(ctx, dbgen.ClosePauseParams{
					ResumedAt: sql.NullInt64{Int64: at, Valid: true}, ID: open.ID,
				})
			}
			if err != nil {
				return fmt.Errorf("store: resume session %d: close pause: %w", p.SessionID, err)
			}
		case errors.Is(err, sql.ErrNoRows):
		default:
			return fmt.Errorf("store: resume session %d: read open pause: %w", p.SessionID, err)
		}
		n, err := q.UpdateOpenSession(ctx, dbgen.UpdateOpenSessionParams{
			State:         string(p.State),
			LastSeenAt:    at,
			ActiveSeconds: activeSeconds(p.Active),
			ID:            p.SessionID,
		})
		if err != nil {
			return fmt.Errorf("store: resume session %d: update: %w", p.SessionID, err)
		}
		if n == 0 {
			return fmt.Errorf("store: resume session %d: %w", p.SessionID, ErrSessionNotOpen)
		}
		return nil
	})
}

// Pauses lists the pause intervals of a session ordered by start; the result
// is never nil (STO-022).
func (s *Store) Pauses(ctx context.Context, sessionID int64) ([]PauseInterval, error) {
	rows, err := s.q.ListPauses(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: list pauses of session %d: %w", sessionID, err)
	}
	out := make([]PauseInterval, 0, len(rows))
	for _, r := range rows {
		pi := PauseInterval{
			ID:        r.ID,
			SessionID: r.SessionID,
			PausedAt:  MillisToTime(r.PausedAt),
			Reason:    domain.PauseReason(r.Reason),
		}
		if r.ResumedAt.Valid {
			pi.ResumedAt = MillisToTime(r.ResumedAt.Int64)
		}
		out = append(out, pi)
	}
	return out, nil
}
