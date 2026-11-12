package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// ErrGameNotFound is returned by Game when no game has the given id.
var ErrGameNotFound = errors.New("store: game not found")

// pcPlatformName is the platform every game added by executable belongs to.
const pcPlatformName = "PC"

// launchTypeExe is the launch_type of a game started from an executable.
const launchTypeExe = "exe"

// Game is a library entry as seen by the agent.
type Game struct {
	ID                                                      int64
	Title, LaunchType, LaunchTarget, LaunchArgs, WorkingDir string
	TotalPlaySeconds, PlayCount                             int64
	FirstPlayedAt, LastPlayedAt                             time.Time // zero = NULL
	PlatformID                                              int64
	Platform                                                string
	Status                                                  domain.GameStatus // tolerant reader (ADR 0012)
	Favorite                                                bool
	ReleaseDate                                             string // "" when NULL
}

// NewGame describes a game to be added from an executable.
type NewGame struct {
	Title, Exe, WorkingDir string
	At                     time.Time
}

// AddGame inserts a game launched by executable, creating the PC platform on
// first use, in a single transaction. The same executable may be added more
// than once; each addition is a distinct game (GAM-001).
func (s *Store) AddGame(ctx context.Context, g NewGame) (Game, error) {
	var out Game
	err := s.withTx(ctx, func(tx *sql.Tx, q *dbgen.Queries) error {
		if err := q.EnsurePCPlatform(ctx); err != nil {
			return fmt.Errorf("store: ensure PC platform: %w", err)
		}
		platformID, err := q.GetPlatformIDByName(ctx, pcPlatformName)
		if err != nil {
			return fmt.Errorf("store: PC platform id: %w", err)
		}
		at := TimeToMillis(g.At)
		row, err := q.InsertGame(ctx, dbgen.InsertGameParams{
			PlatformID:   platformID,
			Title:        g.Title,
			LaunchType:   launchTypeExe,
			LaunchTarget: filepath.Clean(g.Exe),
			LaunchArgs:   "",
			WorkingDir:   g.WorkingDir,
			CreatedAt:    at,
			UpdatedAt:    at,
		})
		if err != nil {
			return fmt.Errorf("store: insert game: %w", err)
		}
		if err := reindexGame(ctx, tx, row.ID); err != nil {
			return err
		}
		// Read the game back with its platform name, inside the transaction.
		full, err := q.GetGameRow(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("store: read added game %d: %w", row.ID, err)
		}
		out = gameFromRow(full)
		return nil
	})
	if err != nil {
		return Game{}, err
	}
	return out, nil
}

// Games lists every game ordered by title (case-insensitive), then id. The
// result is never nil. It is kept only for the agent's game.list until the
// paginated listing replaces it (phase 5, T18); new code uses ListGames.
func (s *Store) Games(ctx context.Context) ([]Game, error) {
	rows, err := s.q.ListGames(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list games: %w", err)
	}
	out := make([]Game, 0, len(rows))
	for _, r := range rows {
		out = append(out, gameFromListRow(r))
	}
	return out, nil
}

// Game returns the game with the given id, or an error wrapping
// ErrGameNotFound.
func (s *Store) Game(ctx context.Context, id int64) (Game, error) {
	row, err := gameRow(ctx, s.q, id)
	if err != nil {
		return Game{}, err
	}
	return gameFromRow(row), nil
}

// gameRow reads the joined game row of id with q. A missing id gives an error
// wrapping ErrGameNotFound.
func gameRow(ctx context.Context, q *dbgen.Queries, id int64) (dbgen.GetGameRowRow, error) {
	row, err := q.GetGameRow(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return dbgen.GetGameRowRow{}, fmt.Errorf("store: game %d: %w", id, ErrGameNotFound)
		}
		return dbgen.GetGameRowRow{}, fmt.Errorf("store: get game %d: %w", id, err)
	}
	return row, nil
}

// gameFromRow maps the joined game row (GetGameRow) to Game.
func gameFromRow(r dbgen.GetGameRowRow) Game {
	return Game{
		ID:               r.ID,
		Title:            r.Title,
		LaunchType:       r.LaunchType,
		LaunchTarget:     r.LaunchTarget,
		LaunchArgs:       r.LaunchArgs,
		WorkingDir:       r.WorkingDir,
		TotalPlaySeconds: r.TotalPlaySeconds,
		PlayCount:        r.PlayCount,
		FirstPlayedAt:    nullMillisToTime(r.FirstPlayedAt),
		LastPlayedAt:     nullMillisToTime(r.LastPlayedAt),
		PlatformID:       r.PlatformID,
		Platform:         r.PlatformName,
		Status:           domain.GameStatus(r.Status),
		Favorite:         r.Favorite != 0,
		ReleaseDate:      r.ReleaseDate.String,
	}
}

// gameFromListRow maps a row of ListGames (same columns as GetGameRow) to Game.
func gameFromListRow(r dbgen.ListGamesRow) Game {
	return Game{
		ID:               r.ID,
		Title:            r.Title,
		LaunchType:       r.LaunchType,
		LaunchTarget:     r.LaunchTarget,
		LaunchArgs:       r.LaunchArgs,
		WorkingDir:       r.WorkingDir,
		TotalPlaySeconds: r.TotalPlaySeconds,
		PlayCount:        r.PlayCount,
		FirstPlayedAt:    nullMillisToTime(r.FirstPlayedAt),
		LastPlayedAt:     nullMillisToTime(r.LastPlayedAt),
		PlatformID:       r.PlatformID,
		Platform:         r.PlatformName,
		Status:           domain.GameStatus(r.Status),
		Favorite:         r.Favorite != 0,
		ReleaseDate:      r.ReleaseDate.String,
	}
}

// nullMillisToTime maps a nullable *_at column to an instant (zero for NULL).
func nullMillisToTime(v sql.NullInt64) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return MillisToTime(v.Int64)
}
