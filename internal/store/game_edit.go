package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// ErrPlatformNotFound is returned by UpdateGame when the patch names a platform
// that does not exist (EDT-002).
var ErrPlatformNotFound = errors.New("store: platform not found")

// GamePatch is a partial edit of a game (EDT-001..EDT-004). A nil field is not
// touched. For the nullable fields, a non-nil pointer to nil clears the value
// (null). A non-nil link list replaces the links of that kind; an empty list
// clears them (EDT-004).
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
	// Launch fields (EDT-010): the type, the target and the arguments.
	LaunchType, LaunchTarget, LaunchArgs *string
}

// UpdateGame applies p to the game of id in one transaction (withTx) and returns
// the game as GameDetail (decision 3). Every field is validated with the domain
// rules before anything is written, so a rejected field changes nothing. The game
// row, its links and its search index row change together: the index row is
// rewritten only when a field or a link changed, and updated_at becomes at only
// then (EDT-005, FTS-003). Any error rolls back the whole edit, catalog rows
// created by it included (EDT-007). Editing does not touch sessions or pauses
// (EDT-006). An unknown id gives an error wrapping ErrGameNotFound, and an unknown
// platform gives ErrPlatformNotFound.
func (s *Store) UpdateGame(ctx context.Context, id int64, p GamePatch, at time.Time) (GameDetail, error) {
	var out GameDetail
	err := s.withTx(ctx, func(tx *sql.Tx, q *dbgen.Queries) error {
		cur, err := q.GetGameForUpdate(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("store: update game %d: %w", id, ErrGameNotFound)
			}
			return fmt.Errorf("store: update game %d: read: %w", id, err)
		}
		// base holds the columns as they are now; next is base with the patch applied.
		base := dbgen.UpdateGameFieldsParams{
			Title:        cur.Title,
			Description:  cur.Description,
			PlatformID:   cur.PlatformID,
			Status:       cur.Status,
			Favorite:     cur.Favorite,
			CompletedAt:  cur.CompletedAt,
			ReleaseDate:  cur.ReleaseDate,
			LaunchType:   cur.LaunchType,
			LaunchTarget: cur.LaunchTarget,
			LaunchArgs:   cur.LaunchArgs,
		}
		next, err := applyGamePatch(ctx, q, p, base)
		if err != nil {
			return fmt.Errorf("store: update game %d: %w", id, err)
		}
		linksChanged, err := applyLinks(ctx, q, id, p)
		if err != nil {
			return fmt.Errorf("store: update game %d: %w", id, err)
		}
		if next != base || linksChanged {
			next.ID = id
			next.UpdatedAt = TimeToMillis(at)
			n, err := q.UpdateGameFields(ctx, next)
			if err != nil {
				return fmt.Errorf("store: update game %d: %w", id, err)
			}
			if n == 0 {
				return fmt.Errorf("store: update game %d: %w", id, ErrGameNotFound)
			}
			// The index reads the links too, so it runs after they are written.
			if err := reindexGame(ctx, tx, id); err != nil {
				return fmt.Errorf("store: update game %d: %w", id, err)
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
	if p.LaunchType != nil || p.LaunchTarget != nil || p.LaunchArgs != nil {
		lt, target, args := cur.LaunchType, cur.LaunchTarget, cur.LaunchArgs
		if p.LaunchType != nil {
			lt = *p.LaunchType
		}
		if p.LaunchTarget != nil {
			target = *p.LaunchTarget
		}
		if p.LaunchArgs != nil {
			args = *p.LaunchArgs
		}
		if err := domain.ValidateLaunch(lt, target, args); err != nil {
			return cur, fmt.Errorf("launch: %w", err)
		}
		next.LaunchType, next.LaunchTarget, next.LaunchArgs = lt, strings.TrimSpace(target), args
	}
	return next, nil
}

// applyLinks replaces each link kind that p sets (EDT-004) and reports whether any
// link changed. A list equal to the stored links writes nothing, so an equal patch
// is not a change (STO-028). New names are resolved to catalog rows first; the
// catalog insert happens only in the transaction of the edit.
func applyLinks(ctx context.Context, q *dbgen.Queries, id int64, p GamePatch) (bool, error) {
	changed := false
	if p.Developers != nil {
		want, err := resolveIDs(ctx, q, *p.Developers, resolveCompany)
		if err != nil {
			return false, fmt.Errorf("developers: %w", err)
		}
		c, err := syncLinks(want, func() ([]int64, error) { return q.GameDeveloperIDs(ctx, id) },
			func(ids []int64) error {
				if err := q.DeleteGameDevelopers(ctx, id); err != nil {
					return err
				}
				for _, cid := range ids {
					if err := q.InsertGameDeveloper(ctx, dbgen.InsertGameDeveloperParams{GameID: id, CompanyID: cid}); err != nil {
						return err
					}
				}
				return nil
			})
		if err != nil {
			return false, fmt.Errorf("developers: %w", err)
		}
		changed = changed || c
	}
	if p.Publishers != nil {
		want, err := resolveIDs(ctx, q, *p.Publishers, resolveCompany)
		if err != nil {
			return false, fmt.Errorf("publishers: %w", err)
		}
		c, err := syncLinks(want, func() ([]int64, error) { return q.GamePublisherIDs(ctx, id) },
			func(ids []int64) error {
				if err := q.DeleteGamePublishers(ctx, id); err != nil {
					return err
				}
				for _, cid := range ids {
					if err := q.InsertGamePublisher(ctx, dbgen.InsertGamePublisherParams{GameID: id, CompanyID: cid}); err != nil {
						return err
					}
				}
				return nil
			})
		if err != nil {
			return false, fmt.Errorf("publishers: %w", err)
		}
		changed = changed || c
	}
	if p.Genres != nil {
		want, err := resolveIDs(ctx, q, *p.Genres, resolveGenre)
		if err != nil {
			return false, fmt.Errorf("genres: %w", err)
		}
		c, err := syncLinks(want, func() ([]int64, error) { return q.GameGenreIDs(ctx, id) },
			func(ids []int64) error {
				if err := q.DeleteGameGenres(ctx, id); err != nil {
					return err
				}
				for _, gid := range ids {
					if err := q.InsertGameGenre(ctx, dbgen.InsertGameGenreParams{GameID: id, GenreID: gid}); err != nil {
						return err
					}
				}
				return nil
			})
		if err != nil {
			return false, fmt.Errorf("genres: %w", err)
		}
		changed = changed || c
	}
	if p.PlayModes != nil {
		want, err := resolvePlayModes(ctx, q, *p.PlayModes)
		if err != nil {
			return false, fmt.Errorf("play modes: %w", err)
		}
		c, err := syncLinks(want, func() ([]int64, error) { return q.GamePlayModeIDs(ctx, id) },
			func(ids []int64) error {
				if err := q.DeleteGamePlayModes(ctx, id); err != nil {
					return err
				}
				for _, mid := range ids {
					if err := q.InsertGamePlayMode(ctx, dbgen.InsertGamePlayModeParams{GameID: id, PlayModeID: mid}); err != nil {
						return err
					}
				}
				return nil
			})
		if err != nil {
			return false, fmt.Errorf("play modes: %w", err)
		}
		changed = changed || c
	}
	return changed, nil
}

// syncLinks compares the wanted link ids (sorted, unique) with the stored ones and,
// when they differ, calls write with the wanted ids. It reports whether it wrote.
func syncLinks(want []int64, current func() ([]int64, error), write func([]int64) error) (bool, error) {
	have, err := current()
	if err != nil {
		return false, fmt.Errorf("read links: %w", err)
	}
	if slices.Equal(have, want) {
		return false, nil
	}
	if err := write(want); err != nil {
		return false, fmt.Errorf("write links: %w", err)
	}
	return true, nil
}

// resolveIDs maps each name to its catalog id with resolve and returns the ids
// sorted and without repetition, so two spellings of one company give one link.
func resolveIDs(ctx context.Context, q *dbgen.Queries, names []string,
	resolve func(context.Context, *dbgen.Queries, string) (int64, error)) ([]int64, error) {
	ids := make([]int64, 0, len(names))
	for _, n := range names {
		id, err := resolve(ctx, q, n)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

// resolvePlayModes maps each play mode to its id, validating it against DOM-014.
// The result is sorted and without repetition.
func resolvePlayModes(ctx context.Context, q *dbgen.Queries, modes []domain.PlayMode) ([]int64, error) {
	ids := make([]int64, 0, len(modes))
	for _, m := range modes {
		valid, err := domain.ParsePlayMode(string(m))
		if err != nil {
			return nil, err
		}
		id, err := q.PlayModeIDByName(ctx, string(valid))
		if err != nil {
			return nil, fmt.Errorf("play mode %q: %w", valid, err)
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

// resolveCompany returns the id of the company named name, reusing a row that
// matches without regard to case (the stored spelling wins, EDT-004) and creating
// the row otherwise. The name is cut as DOM-016 requires.
func resolveCompany(ctx context.Context, q *dbgen.Queries, name string) (int64, error) {
	clean, err := domain.NormalizeName(name)
	if err != nil {
		return 0, fmt.Errorf("company %q: %w", name, err)
	}
	id, err := q.GetCompanyByName(ctx, clean)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store: find company %q: %w", clean, err)
	}
	id, err = q.InsertCompany(ctx, clean)
	if err != nil {
		return 0, fmt.Errorf("store: insert company %q: %w", clean, err)
	}
	return id, nil
}

// resolveGenre is resolveCompany for the genre catalog.
func resolveGenre(ctx context.Context, q *dbgen.Queries, name string) (int64, error) {
	clean, err := domain.NormalizeName(name)
	if err != nil {
		return 0, fmt.Errorf("genre %q: %w", name, err)
	}
	id, err := q.GetGenreByName(ctx, clean)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store: find genre %q: %w", clean, err)
	}
	id, err = q.InsertGenre(ctx, clean)
	if err != nil {
		return 0, fmt.Errorf("store: insert genre %q: %w", clean, err)
	}
	return id, nil
}
