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

// sessionForEdit reads a session and refuses the open and the unknown ones.
func sessionForEdit(ctx context.Context, q *dbgen.Queries, id int64) (dbgen.GetSessionForEditRow, error) {
	row, err := q.GetSessionForEdit(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, fmt.Errorf("store: session %d: %w", id, ErrSessionNotFound)
	}
	if err != nil {
		return row, fmt.Errorf("store: get session %d: %w", id, err)
	}
	if !row.EndedAt.Valid {
		return row, &SessionOpenError{SessionID: id, GameID: row.GameID, Title: row.Title}
	}
	return row, nil
}

func sessionItemFromEdit(r dbgen.GetSessionForEditRow) SessionItem {
	it := SessionItem{
		ID:            r.ID,
		GameID:        r.GameID,
		State:         domain.SessionState(r.State),
		Source:        r.Source,
		StartedAt:     MillisToTime(r.StartedAt),
		ActiveSeconds: r.ActiveSeconds,
	}
	if r.EndedAt.Valid {
		it.EndedAt = MillisToTime(r.EndedAt.Int64)
	}
	if r.EndReason.Valid {
		it.EndReason = r.EndReason.String
	}
	if r.EditedAt.Valid {
		it.EditedAt = MillisToTime(r.EditedAt.Int64)
	}
	return it
}

// applyGameDelta adds delta to the game total, recomputes first/last/count and
// stamps updated_at = now only when some of those fields really changed.
func applyGameDelta(ctx context.Context, q *dbgen.Queries, gameID, delta int64, now time.Time) error {
	before, err := gameRow(ctx, q, gameID)
	if err != nil {
		return err
	}
	oldUpdated, err := q.GetGameUpdatedAt(ctx, gameID)
	if err != nil {
		return fmt.Errorf("store: read updated_at of game %d: %w", gameID, err)
	}
	if _, err := q.AddToTotal(ctx, dbgen.AddToTotalParams{Delta: delta, MaxTotal: maxTotalPlaySeconds, UpdatedAt: oldUpdated, ID: gameID}); err != nil {
		return fmt.Errorf("store: add to total of game %d: %w", gameID, err)
	}
	if _, err := q.RecomputeGamePlayFacts(ctx, dbgen.RecomputeGamePlayFactsParams{ID: gameID, UpdatedAt: oldUpdated}); err != nil {
		return fmt.Errorf("store: recompute play facts of game %d: %w", gameID, err)
	}
	after, err := gameRow(ctx, q, gameID)
	if err != nil {
		return err
	}
	if before.TotalPlaySeconds != after.TotalPlaySeconds || before.PlayCount != after.PlayCount ||
		before.FirstPlayedAt != after.FirstPlayedAt || before.LastPlayedAt != after.LastPlayedAt {
		if _, err := q.SetGameUpdatedAt(ctx, dbgen.SetGameUpdatedAtParams{UpdatedAt: TimeToMillis(now), ID: gameID}); err != nil {
			return fmt.Errorf("store: stamp game %d: %w", gameID, err)
		}
	}
	return nil
}

// EditSession changes the interval and/or the played time of a closed session
// in one transaction (design §3.4).
func (s *Store) EditSession(ctx context.Context, id int64, p SessionPatch, now time.Time) (SessionWrite, error) {
	var out SessionWrite
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		row, err := sessionForEdit(ctx, q, id)
		if err != nil {
			return err
		}
		prev := sessionItemFromEdit(row)
		out.Previous = &prev
		game, err := gameRow(ctx, q, row.GameID)
		if err != nil {
			return err
		}
		out.TotalBefore = game.TotalPlaySeconds
		if row.State == string(domain.SessionDiscarded) {
			return &SessionEditError{Reason: domain.ReasonDiscarded}
		}
		if p.Start == nil && p.End == nil && p.Active == nil {
			return &SessionEditError{Reason: domain.ReasonEmptyPatch}
		}
		start, end := prev.StartedAt, prev.EndedAt
		active := time.Duration(row.ActiveSeconds) * time.Second
		if p.Start != nil {
			start = p.Start.Truncate(time.Millisecond)
		}
		if p.End != nil {
			end = p.End.Truncate(time.Millisecond)
		}
		if p.Active != nil {
			active = (*p.Active / time.Second) * time.Second
		}
		newActive := int64(active / time.Second)
		if start.Equal(prev.StartedAt) && end.Equal(prev.EndedAt) && newActive == row.ActiveSeconds {
			out.Session = prev
			out.Game, err = readGameDetail(ctx, q, row.GameID)
			if err != nil {
				return err
			}
			out.TotalAfter = out.Game.TotalPlaySeconds
			return nil
		}
		touchesTimes := p.Start != nil || p.End != nil
		isImport := row.Source == string(domain.SourceImport)
		if reason := domain.ValidateSessionTimes(domain.SessionTimes{Start: start, End: end, Active: active}, now, !isImport && touchesTimes); reason != "" {
			return &SessionEditError{Reason: reason}
		}
		if !isImport && touchesTimes {
			other, err := q.FindOverlappingSession(ctx, dbgen.FindOverlappingSessionParams{
				Self:       id,
				RangeEnd:   TimeToMillis(end),
				RangeStart: sql.NullInt64{Int64: TimeToMillis(start), Valid: true},
			})
			switch {
			case err == nil:
				oe := &SessionOverlapError{SessionID: other.ID, GameID: other.GameID, Title: other.Title, StartedAt: MillisToTime(other.StartedAt)}
				if other.EndedAt.Valid {
					t := MillisToTime(other.EndedAt.Int64)
					oe.EndedAt = &t
				}
				return oe
			case !errors.Is(err, sql.ErrNoRows):
				return fmt.Errorf("store: find overlapping session: %w", err)
			}
		}
		lastSeen := row.LastSeenAt
		if !end.Equal(prev.EndedAt) {
			lastSeen = TimeToMillis(end)
		}
		n, err := q.UpdateSessionTimes(ctx, dbgen.UpdateSessionTimesParams{
			StartedAt:     TimeToMillis(start),
			EndedAt:       sql.NullInt64{Int64: TimeToMillis(end), Valid: true},
			LastSeenAt:    lastSeen,
			ActiveSeconds: newActive,
			EditedAt:      sql.NullInt64{Int64: TimeToMillis(now), Valid: true},
			ID:            id,
		})
		if err != nil {
			return fmt.Errorf("store: update session %d: %w", id, err)
		}
		if n != 1 {
			return &SessionOpenError{SessionID: id, GameID: row.GameID, Title: row.Title}
		}
		if !start.Equal(prev.StartedAt) || !end.Equal(prev.EndedAt) {
			pauses, err := q.ListPauses(ctx, id)
			if err != nil {
				return fmt.Errorf("store: list pauses of session %d: %w", id, err)
			}
			for _, pa := range pauses {
				var resumed *time.Time
				if pa.ResumedAt.Valid {
					t := MillisToTime(pa.ResumedAt.Int64)
					resumed = &t
				}
				cp, cr, ok := domain.ClipPause(MillisToTime(pa.PausedAt), resumed, start, end)
				switch {
				case !ok:
					err = q.DeletePause(ctx, pa.ID)
				case cp.UnixMilli() != pa.PausedAt || !pa.ResumedAt.Valid || cr.UnixMilli() != pa.ResumedAt.Int64:
					err = q.UpdatePauseBounds(ctx, dbgen.UpdatePauseBoundsParams{
						PausedAt: TimeToMillis(cp), ResumedAt: sql.NullInt64{Int64: TimeToMillis(cr), Valid: true}, ID: pa.ID,
					})
				}
				if err != nil {
					return fmt.Errorf("store: clip pause %d: %w", pa.ID, err)
				}
			}
		}
		if err := applyGameDelta(ctx, q, row.GameID, newActive-row.ActiveSeconds, now); err != nil {
			return err
		}
		out.Game, err = readGameDetail(ctx, q, row.GameID)
		if err != nil {
			return err
		}
		out.TotalAfter = out.Game.TotalPlaySeconds
		out.Session = prev
		out.Session.StartedAt, out.Session.EndedAt, out.Session.ActiveSeconds, out.Session.EditedAt = start, end, newActive, now.Truncate(time.Millisecond)
		out.Changed = true
		return nil
	})
	if err != nil {
		return SessionWrite{}, err
	}
	return out, nil
}

// RemoveSession deletes a closed session (and its pauses, by cascade) and takes
// its time off the game total (design §3.4). Discarded sessions leave the game alone.
func (s *Store) RemoveSession(ctx context.Context, id int64, now time.Time) (SessionWrite, error) {
	var out SessionWrite
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		row, err := sessionForEdit(ctx, q, id)
		if err != nil {
			return err
		}
		prev := sessionItemFromEdit(row)
		out.Previous = &prev
		game, err := gameRow(ctx, q, row.GameID)
		if err != nil {
			return err
		}
		out.TotalBefore = game.TotalPlaySeconds
		n, err := q.DeleteClosedSession(ctx, id)
		if err != nil {
			return fmt.Errorf("store: delete session %d: %w", id, err)
		}
		if n != 1 {
			return &SessionOpenError{SessionID: id, GameID: row.GameID, Title: row.Title}
		}
		if row.State == string(domain.SessionEnded) {
			if err := applyGameDelta(ctx, q, row.GameID, -row.ActiveSeconds, now); err != nil {
				return err
			}
		}
		out.Game, err = readGameDetail(ctx, q, row.GameID)
		if err != nil {
			return err
		}
		out.TotalAfter = out.Game.TotalPlaySeconds
		out.Changed = true
		return nil
	})
	if err != nil {
		return SessionWrite{}, err
	}
	return out, nil
}
