package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// SetTotalPlaySeconds replaces the total play time of a game with the value the user typed and
// returns the game (EDM-005). Sessions are not touched. An unknown id wraps ErrGameNotFound.
func (s *Store) SetTotalPlaySeconds(ctx context.Context, id, seconds int64, at time.Time) (GameDetail, error) {
	n, err := s.q.SetTotalPlaySeconds(ctx, dbgen.SetTotalPlaySecondsParams{
		Seconds: seconds, UpdatedAt: at.UnixMilli(), ID: id,
	})
	if err != nil {
		return GameDetail{}, fmt.Errorf("store: gravar tempo jogado: %w", err)
	}
	if n == 0 {
		return GameDetail{}, fmt.Errorf("store: game %d: %w", id, ErrGameNotFound)
	}
	return s.GameDetail(ctx, id)
}

// RemoveMedia deletes the row of one image and returns it, so the caller can delete its files
// (EDM-003). It reports false for an id that does not exist.
func (s *Store) RemoveMedia(ctx context.Context, id int64) (MediaRow, bool, error) {
	r, err := s.q.GetMedia(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaRow{}, false, nil
	}
	if err != nil {
		return MediaRow{}, false, fmt.Errorf("store: ler mídia: %w", err)
	}
	if _, err := s.q.DeleteMedia(ctx, id); err != nil {
		return MediaRow{}, false, fmt.Errorf("store: apagar mídia: %w", err)
	}
	return MediaRow{
		ID: r.ID, GameID: r.GameID, Kind: r.Kind, Provider: r.Provider, SourceURL: r.SourceUrl,
		Hash: r.Hash, Path: r.Path, ThumbPath: r.ThumbPath, CreatedAt: time.UnixMilli(r.CreatedAt),
	}, true, nil
}
