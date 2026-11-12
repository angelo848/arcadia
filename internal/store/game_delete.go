package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// ErrGameInUse is returned by DeleteGame when the game has a session that is still
// open (STO-029). The agent maps it to a refusal with sessionId 0 (planner decision 6).
var ErrGameInUse = errors.New("store: game has an open session")

// DeleteGame removes the game of id in one transaction (STO-029, DEL-001). It
// refuses with ErrGameInUse while a session of the game is open, checked inside
// the transaction. The search index row is deleted first (FTS-004), because game
// ids can be reused. The game row goes last; sessions, pauses, links, additional
// apps, tracking rules and media assets follow by ON DELETE CASCADE. Platforms,
// genres, companies and play modes are kept. An unknown id gives an error wrapping
// ErrGameNotFound, and nothing changes in either error case.
func (s *Store) DeleteGame(ctx context.Context, id int64) error {
	return s.withTx(ctx, func(tx *sql.Tx, q *dbgen.Queries) error {
		open, err := q.HasOpenSession(ctx, id)
		if err != nil {
			return fmt.Errorf("store: delete game %d: open sessions: %w", id, err)
		}
		if open > 0 {
			return fmt.Errorf("store: delete game %d: %w", id, ErrGameInUse)
		}
		if err := deleteGameIndex(ctx, tx, id); err != nil {
			return fmt.Errorf("store: delete game %d: %w", id, err)
		}
		n, err := q.DeleteGameRow(ctx, id)
		if err != nil {
			return fmt.Errorf("store: delete game %d: %w", id, err)
		}
		if n == 0 {
			return fmt.Errorf("store: delete game %d: %w", id, ErrGameNotFound)
		}
		return nil
	})
}
