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

// ErrSessionNotOpen is returned when a session is missing or already ended.
var ErrSessionNotOpen = errors.New("store: session not open")

// OpenSession is a play session with ended_at still NULL.
type OpenSession struct {
	ID, GameID            int64
	StartedAt, LastSeenAt time.Time
	ActiveSeconds         int64
}

// SessionEnd describes how an open session is closed.
type SessionEnd struct {
	ID        int64
	EndedAt   time.Time
	Active    time.Duration
	Reason    domain.SessionEndReason
	WrittenAt time.Time
}

// activeSeconds floors an active duration to whole seconds (STO-013).
func activeSeconds(active time.Duration) int64 { return int64(active / time.Second) }

// StartSession inserts a running, tracked session for gameID starting at at
// and returns its id (STO-012).
func (s *Store) StartSession(ctx context.Context, gameID int64, at time.Time) (int64, error) {
	ms := TimeToMillis(at)
	id, err := s.q.InsertSession(ctx, dbgen.InsertSessionParams{GameID: gameID, StartedAt: ms, LastSeenAt: ms})
	if err != nil {
		return 0, fmt.Errorf("store: insert session for game %d: %w", gameID, err)
	}
	return id, nil
}

// UpdateSession records the state, last-seen instant and active time of an
// open session. It returns an error wrapping ErrSessionNotOpen when the
// session is missing or already ended (STO-013).
func (s *Store) UpdateSession(ctx context.Context, id int64, st domain.SessionState,
	lastSeen time.Time, active time.Duration) error {
	n, err := s.q.UpdateOpenSession(ctx, dbgen.UpdateOpenSessionParams{
		State:         string(st),
		LastSeenAt:    TimeToMillis(lastSeen),
		ActiveSeconds: activeSeconds(active),
		ID:            id,
	})
	if err != nil {
		return fmt.Errorf("store: update session %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("store: update session %d: %w", id, ErrSessionNotOpen)
	}
	return nil
}

// EndSession closes an open session in one transaction. The final state is
// domain.Outcome(e.Active); when it is ended, the game's totals are updated
// in the same transaction (STO-014, STO-015). A missing or already-ended
// session yields an error wrapping ErrSessionNotOpen and changes nothing.
func (s *Store) EndSession(ctx context.Context, e SessionEnd) (domain.SessionState, error) {
	state := domain.Outcome(e.Active)
	secs := activeSeconds(e.Active)
	endedAt := TimeToMillis(e.EndedAt)
	err := s.Tx(ctx, func(q *dbgen.Queries) error {
		open, err := q.GetOpenSession(ctx, e.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("store: end session %d: %w", e.ID, ErrSessionNotOpen)
			}
			return fmt.Errorf("store: end session %d: read: %w", e.ID, err)
		}
		// Close the open pause at the end instant; one that would start at or
		// after it is dropped (STO-023).
		if _, err := q.ClosePauseAt(ctx, dbgen.ClosePauseAtParams{
			EndedAt:   sql.NullInt64{Int64: endedAt, Valid: true},
			SessionID: e.ID,
		}); err != nil {
			return fmt.Errorf("store: end session %d: close pause: %w", e.ID, err)
		}
		if err := q.DeleteOpenPause(ctx, e.ID); err != nil {
			return fmt.Errorf("store: end session %d: drop open pause: %w", e.ID, err)
		}
		n, err := q.CloseSession(ctx, dbgen.CloseSessionParams{
			EndedAt:       sql.NullInt64{Int64: endedAt, Valid: true},
			ActiveSeconds: secs,
			State:         string(state),
			EndReason:     sql.NullString{String: string(e.Reason), Valid: true},
			ID:            e.ID,
		})
		if err != nil {
			return fmt.Errorf("store: end session %d: close: %w", e.ID, err)
		}
		if n == 0 {
			return fmt.Errorf("store: end session %d: %w", e.ID, ErrSessionNotOpen)
		}
		if state != domain.SessionEnded {
			return nil
		}
		n, err = q.AddPlayToGame(ctx, dbgen.AddPlayToGameParams{
			Seconds:       secs,
			LastPlayedAt:  sql.NullInt64{Int64: endedAt, Valid: true},
			FirstPlayedAt: sql.NullInt64{Int64: open.StartedAt, Valid: true},
			UpdatedAt:     TimeToMillis(e.WrittenAt),
			ID:            open.GameID,
		})
		if err != nil {
			return fmt.Errorf("store: end session %d: update game %d: %w", e.ID, open.GameID, err)
		}
		if n == 0 {
			return fmt.Errorf("store: end session %d: game %d: %w", e.ID, open.GameID, ErrGameNotFound)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return state, nil
}

// DeleteSession removes a session row.
func (s *Store) DeleteSession(ctx context.Context, id int64) error {
	if err := s.q.DeleteSession(ctx, id); err != nil {
		return fmt.Errorf("store: delete session %d: %w", id, err)
	}
	return nil
}

// OpenSessions lists every session with ended_at NULL, ordered by id. The
// result is never nil (STO-017).
func (s *Store) OpenSessions(ctx context.Context) ([]OpenSession, error) {
	rows, err := s.q.ListOpenSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list open sessions: %w", err)
	}
	out := make([]OpenSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, OpenSession{
			ID:            r.ID,
			GameID:        r.GameID,
			StartedAt:     MillisToTime(r.StartedAt),
			LastSeenAt:    MillisToTime(r.LastSeenAt),
			ActiveSeconds: r.ActiveSeconds,
		})
	}
	return out, nil
}
