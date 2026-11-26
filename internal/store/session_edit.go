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

// maxTotalPlaySeconds is the ceiling of a game's total (999_999 hours). It must
// equal ipc.MaxPlaySeconds; the store does not import ipc (a test checks both).
const maxTotalPlaySeconds int64 = 999_999 * 3600

var (
	ErrSessionNotFound = errors.New("store: session not found")
	ErrSessionOpen     = errors.New("store: session is open")
)

// SessionOpenError says the session is still open and cannot be edited.
type SessionOpenError struct {
	SessionID, GameID int64
	Title             string
}

func (e *SessionOpenError) Error() string {
	return fmt.Sprintf("store: session %d of %q is open", e.SessionID, e.Title)
}

// Is makes errors.Is(err, ErrSessionOpen) true.
func (e *SessionOpenError) Is(target error) bool { return target == ErrSessionOpen }

// SessionEditError carries the domain.EditReason of a refused session edit.
type SessionEditError struct{ Reason domain.EditReason }

func (e *SessionEditError) Error() string {
	return fmt.Sprintf("store: session edit refused: %s", e.Reason)
}

// SessionOverlapError carries the session the requested interval crosses.
type SessionOverlapError struct {
	SessionID, GameID int64
	Title             string
	StartedAt         time.Time
	EndedAt           *time.Time // nil = the other session is open
}

func (e *SessionOverlapError) Error() string {
	return fmt.Sprintf("store: interval overlaps session %d of %q", e.SessionID, e.Title)
}

// ManualSession is a session typed by the user (SED-003).
type ManualSession struct {
	GameID          int64
	Start, End      time.Time
	Active          time.Duration
	IncludedInTotal bool
}

// SessionPatch holds the fields to change in an existing session; nil = keep.
type SessionPatch struct {
	Start, End *time.Time
	Active     *time.Duration
}

// SessionWrite is what the session edit operations return.
type SessionWrite struct {
	Session                 SessionItem // after the write (zero on delete)
	Game                    GameDetail
	TotalBefore, TotalAfter int64
	Previous                *SessionItem // nil on create
	Changed                 bool         // false: nothing was written (SED-008)
}

// CreateManualSession records a session the user typed in, in one transaction
// (design §3.4). now is the Clock instant of the agent.
func (s *Store) CreateManualSession(ctx context.Context, m ManualSession, now time.Time) (SessionWrite, error) {
	start := m.Start.Truncate(time.Millisecond)
	end := m.End.Truncate(time.Millisecond)
	var out SessionWrite
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		game, err := gameRow(ctx, q, m.GameID)
		if err != nil {
			return err
		}
		out.TotalBefore = game.TotalPlaySeconds
		if reason := domain.ValidateSessionTimes(domain.SessionTimes{Start: start, End: end, Active: m.Active}, now, true); reason != "" {
			return &SessionEditError{Reason: reason}
		}
		seconds := int64(m.Active / time.Second)
		if m.IncludedInTotal {
			counted, err := q.SumCountedSeconds(ctx, m.GameID)
			if err != nil {
				return fmt.Errorf("store: sum counted seconds of game %d: %w", m.GameID, err)
			}
			if game.TotalPlaySeconds-counted < seconds {
				return &SessionEditError{Reason: domain.ReasonExceedsAdjustment}
			}
		}
		other, err := q.FindOverlappingSession(ctx, dbgen.FindOverlappingSessionParams{
			Self:       0,
			RangeEnd:   TimeToMillis(end),
			RangeStart: sql.NullInt64{Int64: TimeToMillis(start), Valid: true},
		})
		switch {
		case err == nil:
			oe := &SessionOverlapError{
				SessionID: other.ID,
				GameID:    other.GameID,
				Title:     other.Title,
				StartedAt: MillisToTime(other.StartedAt),
			}
			if other.EndedAt.Valid {
				t := MillisToTime(other.EndedAt.Int64)
				oe.EndedAt = &t
			}
			return oe
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("store: find overlapping session: %w", err)
		}
		id, err := q.InsertManualSession(ctx, dbgen.InsertManualSessionParams{
			GameID:        m.GameID,
			StartedAt:     TimeToMillis(start),
			EndedAt:       sql.NullInt64{Int64: TimeToMillis(end), Valid: true},
			ActiveSeconds: seconds,
		})
		if err != nil {
			return fmt.Errorf("store: insert manual session: %w", err)
		}
		if !m.IncludedInTotal {
			if _, err := q.AddToTotal(ctx, dbgen.AddToTotalParams{
				Delta:     seconds,
				MaxTotal:  maxTotalPlaySeconds,
				UpdatedAt: TimeToMillis(now),
				ID:        m.GameID,
			}); err != nil {
				return fmt.Errorf("store: add to total of game %d: %w", m.GameID, err)
			}
		}
		if _, err := q.RecomputeGamePlayFacts(ctx, dbgen.RecomputeGamePlayFactsParams{
			ID:        m.GameID,
			UpdatedAt: TimeToMillis(now),
		}); err != nil {
			return fmt.Errorf("store: recompute play facts of game %d: %w", m.GameID, err)
		}
		out.Game, err = readGameDetail(ctx, q, m.GameID)
		if err != nil {
			return err
		}
		out.TotalAfter = out.Game.TotalPlaySeconds
		out.Session = SessionItem{
			ID:            id,
			GameID:        m.GameID,
			State:         domain.SessionState("ended"),
			Source:        string(domain.SourceManual),
			StartedAt:     start,
			EndedAt:       end,
			ActiveSeconds: seconds,
		}
		out.Changed = true
		return nil
	})
	if err != nil {
		return SessionWrite{}, err
	}
	return out, nil
}
