package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// SessionItem is one play session of a game, as the session list shows it
// (STO-027). Discarded sessions and the open one are included.
type SessionItem struct {
	ID, GameID    int64
	State         domain.SessionState // tolerant reader (ADR 0012)
	Source        string              // tolerant reader: unknown values pass through (SED-002)
	StartedAt     time.Time
	EndedAt       time.Time // zero = NULL (open session)
	ActiveSeconds int64
	EndReason     string    // "" = NULL
	EditedAt      time.Time // zero = NULL
}

// SessionPage is one page of the sessions of a game and the count of all of them.
type SessionPage struct {
	Sessions []SessionItem // never nil
	Total    int64         // every session of the game, not only the page
}

// GameSessions returns one page of the sessions of gameID, most recent first
// (started_at, then id, both descending). The page and the total are read in
// one transaction. An unknown game gives an error wrapping ErrGameNotFound; an
// offset below 0 or a limit outside 1..MaxSessionLimit is an error, checked
// before any query runs.
func (s *Store) GameSessions(ctx context.Context, gameID int64, offset, limit int) (SessionPage, error) {
	if offset < 0 {
		return SessionPage{}, fmt.Errorf("store: game sessions: negative offset %d", offset)
	}
	if limit < 1 || limit > domain.MaxSessionLimit {
		return SessionPage{}, fmt.Errorf("store: game sessions: limit %d outside 1..%d", limit, domain.MaxSessionLimit)
	}
	page := SessionPage{Sessions: []SessionItem{}}
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		if _, err := gameRow(ctx, q, gameID); err != nil {
			return err
		}
		total, err := q.CountSessionsByGame(ctx, gameID)
		if err != nil {
			return fmt.Errorf("store: count sessions of game %d: %w", gameID, err)
		}
		rows, err := q.ListSessionsByGame(ctx, dbgen.ListSessionsByGameParams{
			GameID:     gameID,
			PageOffset: int64(offset),
			PageLimit:  int64(limit),
		})
		if err != nil {
			return fmt.Errorf("store: list sessions of game %d: %w", gameID, err)
		}
		page.Total = total
		for _, r := range rows {
			page.Sessions = append(page.Sessions, SessionItem{
				ID:            r.ID,
				GameID:        r.GameID,
				State:         domain.SessionState(r.State), // tolerant reader (ADR 0012)
				Source:        r.Source,
				StartedAt:     MillisToTime(r.StartedAt),
				EndedAt:       nullMillisToTime(r.EndedAt),
				ActiveSeconds: r.ActiveSeconds,
				EndReason:     r.EndReason.String,
				EditedAt:      nullMillisToTime(r.EditedAt),
			})
		}
		return nil
	})
	if err != nil {
		return SessionPage{}, err
	}
	return page, nil
}
