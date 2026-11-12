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

// ErrPlatformNotFound is returned by UpdateGame when the patch names a platform
// that does not exist (EDT-002).
var ErrPlatformNotFound = errors.New("store: platform not found")

// GamePatch is a partial edit of a game (EDT-001..EDT-004). A nil field is not
// touched. For the nullable fields, a non-nil pointer to nil clears the value
// (null). The links (Developers, Publishers, Genres, PlayModes) are accepted here
// but are written by a later task (phase 5, T12).
type GamePatch struct {
	Title, Description *string
	PlatformID         *int64
	Status             *domain.GameStatus
	Favorite           *bool
	CompletedAt        **time.Time
	Year               **int
	Developers         *[]string
	Publishers         *[]string
	Genres             *[]string
	PlayModes          *[]domain.PlayMode
}

// UpdateGame applies p to the game of id in one transaction (withTx) and returns
// the game as GameDetail (decision 3). Every value is validated with the domain
// rules before anything is written, so a rejected field changes nothing. The row
// is written only when a column changes, and updated_at becomes at only then
// (EDT-005); an empty patch returns the game unchanged. Editing does not touch
// sessions or pauses (EDT-006). An unknown id gives an error wrapping
// ErrGameNotFound, and an unknown platform gives ErrPlatformNotFound.
func (s *Store) UpdateGame(ctx context.Context, id int64, p GamePatch, at time.Time) (GameDetail, error) {
	var out GameDetail
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		cur, err := q.GetGameForUpdate(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("store: update game %d: %w", id, ErrGameNotFound)
			}
			return fmt.Errorf("store: update game %d: read: %w", id, err)
		}
		// base holds the columns as they are now; next is base with the patch applied.
		base := dbgen.UpdateGameFieldsParams{
			Title:       cur.Title,
			Description: cur.Description,
			PlatformID:  cur.PlatformID,
			Status:      cur.Status,
			Favorite:    cur.Favorite,
			CompletedAt: cur.CompletedAt,
			ReleaseDate: cur.ReleaseDate,
		}
		next, err := applyGamePatch(ctx, q, p, base)
		if err != nil {
			return fmt.Errorf("store: update game %d: %w", id, err)
		}
		if next != base {
			next.ID = id
			next.UpdatedAt = TimeToMillis(at)
			n, err := q.UpdateGameFields(ctx, next)
			if err != nil {
				return fmt.Errorf("store: update game %d: %w", id, err)
			}
			if n == 0 {
				return fmt.Errorf("store: update game %d: %w", id, ErrGameNotFound)
			}
		}
		out, err = readGameDetail(ctx, q, id)
		return err
	})
	if err != nil {
		return GameDetail{}, err
	}
	return out, nil
}

// applyGamePatch returns cur with the fields of p applied. Each new value is
// checked first: a text outside its limits, an unknown status, an invalid year or
// date, or a platform that does not exist is an error, and the caller writes
// nothing.
func applyGamePatch(ctx context.Context, q *dbgen.Queries, p GamePatch,
	cur dbgen.UpdateGameFieldsParams) (dbgen.UpdateGameFieldsParams, error) {
	next := cur
	if p.Title != nil {
		t, err := domain.ValidateTitle(*p.Title)
		if err != nil {
			return cur, fmt.Errorf("title: %w", err)
		}
		next.Title = t
	}
	if p.Description != nil {
		d, err := domain.ValidateDescription(*p.Description)
		if err != nil {
			return cur, fmt.Errorf("description: %w", err)
		}
		next.Description = d
	}
	if p.PlatformID != nil {
		n, err := q.PlatformExists(ctx, *p.PlatformID)
		if err != nil {
			return cur, fmt.Errorf("platform %d: %w", *p.PlatformID, err)
		}
		if n == 0 {
			return cur, fmt.Errorf("platform %d: %w", *p.PlatformID, ErrPlatformNotFound)
		}
		next.PlatformID = *p.PlatformID
	}
	if p.Status != nil {
		st, err := domain.ParseGameStatus(string(*p.Status))
		if err != nil {
			return cur, fmt.Errorf("status: %w", err)
		}
		next.Status = string(st)
	}
	if p.Favorite != nil {
		next.Favorite = 0
		if *p.Favorite {
			next.Favorite = 1
		}
	}
	if p.CompletedAt != nil {
		if *p.CompletedAt == nil {
			next.CompletedAt = sql.NullInt64{}
		} else {
			// The completion date is a day: store midnight UTC of that day (EDT-001).
			day := (**p.CompletedAt).UTC()
			midnight := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
			next.CompletedAt = sql.NullInt64{Int64: TimeToMillis(midnight), Valid: true}
		}
	}
	if p.Year != nil {
		if *p.Year == nil {
			next.ReleaseDate = sql.NullString{}
		} else {
			// WithYear keeps the full date when it already has this year (EDT-003).
			rd, err := domain.WithYear(cur.ReleaseDate.String, **p.Year)
			if err != nil {
				return cur, fmt.Errorf("year: %w", err)
			}
			next.ReleaseDate = sql.NullString{String: rd, Valid: true}
		}
	}
	// The links (developers, publishers, genres, play modes) are written in T12.
	return next, nil
}
