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

// launchTypeURI is the launch_type of a game opened by a URI (RUL-003).
const launchTypeURI = "uri"

// ErrDuplicate is returned by ApplyImport when the library already has a game with
// the same launch target, or with the same title on the same platform (IMP-006).
var ErrDuplicate = errors.New("store: game already in the library")

// ImportPlayed is the play time an import brings: Seconds of play, the last play
// in UTC (IMP-007).
type ImportPlayed struct {
	Seconds  int64
	LastPlay time.Time
}

// ImportGame is one game an import writes. The agent resolves PlatformID and
// validates the strings through the domain rules before calling ApplyImport.
type ImportGame struct {
	PlatformID   int64
	Title        string
	LaunchType   string // domain launch types: "exe" or "uri"
	LaunchTarget string
	InstallDir   string
	Description  string
	Year         *int
	Developers   []string
	Publishers   []string
	Genres       []string
	PlayModes    []domain.PlayMode
	Played       *ImportPlayed
}

// ApplyImport writes one imported game in a single transaction (IMP-001, IMP-008):
// the game row, its links (EDT-004), its search row (ADR 0020), a tracking rule
// (RUL-001) and, when there is play time, one session of source import (IMP-007).
// It returns ErrDuplicate and writes nothing when the game is already in the
// library (IMP-006). Any error rolls the whole game back.
func (s *Store) ApplyImport(ctx context.Context, g ImportGame, at time.Time) (int64, error) {
	title, err := domain.ValidateTitle(g.Title)
	if err != nil {
		return 0, fmt.Errorf("store: import title: %w", err)
	}
	desc, err := domain.ValidateDescription(g.Description)
	if err != nil {
		return 0, fmt.Errorf("store: import description: %w", err)
	}
	if g.LaunchTarget == "" {
		return 0, fmt.Errorf("store: import %q: empty launch target", title)
	}
	if g.LaunchType != launchTypeExe && g.LaunchType != launchTypeURI {
		return 0, fmt.Errorf("store: import %q: launch type %q", title, g.LaunchType)
	}
	var id int64
	err = s.withTx(ctx, func(tx *sql.Tx, q *dbgen.Queries) error {
		if _, err := q.FindGameByLaunchTarget(ctx, dbgen.FindGameByLaunchTargetParams{
			LaunchType: g.LaunchType, LaunchTarget: g.LaunchTarget,
		}); err == nil {
			return fmt.Errorf("store: import %q: launch target: %w", title, ErrDuplicate)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: import %q: find by target: %w", title, err)
		}
		if _, err := q.FindGameByTitleAndPlatform(ctx, dbgen.FindGameByTitleAndPlatformParams{
			Title: title, PlatformID: g.PlatformID,
		}); err == nil {
			return fmt.Errorf("store: import %q: title and platform: %w", title, ErrDuplicate)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: import %q: find by title: %w", title, err)
		}

		release := sql.NullString{}
		if g.Year != nil {
			rd, err := domain.WithYear("", *g.Year)
			if err != nil {
				return fmt.Errorf("store: import %q: year: %w", title, err)
			}
			release = sql.NullString{String: rd, Valid: true}
		}
		at := TimeToMillis(at)
		id, err = q.InsertImportedGame(ctx, dbgen.InsertImportedGameParams{
			PlatformID:   g.PlatformID,
			Title:        title,
			LaunchType:   g.LaunchType,
			LaunchTarget: g.LaunchTarget,
			InstallDir:   g.InstallDir,
			Description:  desc,
			ReleaseDate:  release,
			CreatedAt:    at,
			UpdatedAt:    at,
		})
		if err != nil {
			return fmt.Errorf("store: import %q: insert game: %w", title, err)
		}
		if err := linkImported(ctx, q, id, g); err != nil {
			return fmt.Errorf("store: import %q: links: %w", title, err)
		}
		if err := insertImportRules(ctx, q, id, g); err != nil {
			return fmt.Errorf("store: import %q: rules: %w", title, err)
		}
		if err := reindexGame(ctx, tx, id); err != nil {
			return err
		}
		if p := g.Played; p != nil && p.Seconds > 0 && !p.LastPlay.IsZero() {
			lastPlay := p.LastPlay.UTC()
			started := lastPlay.Add(-time.Duration(p.Seconds) * time.Second)
			if _, err := q.InsertImportedSession(ctx, dbgen.InsertImportedSessionParams{
				GameID:        id,
				StartedAt:     TimeToMillis(started),
				EndedAt:       sql.NullInt64{Int64: TimeToMillis(lastPlay), Valid: true},
				LastSeenAt:    TimeToMillis(lastPlay),
				ActiveSeconds: p.Seconds,
			}); err != nil {
				return fmt.Errorf("store: import %q: session: %w", title, err)
			}
			// The game is new, so no tracked session exists and the totals start from the import.
			if _, err := q.AddPlayToGame(ctx, dbgen.AddPlayToGameParams{
				Seconds:       p.Seconds,
				LastPlayedAt:  sql.NullInt64{Int64: TimeToMillis(lastPlay), Valid: true},
				FirstPlayedAt: sql.NullInt64{Int64: TimeToMillis(started), Valid: true},
				UpdatedAt:     at,
				ID:            id,
			}); err != nil {
				return fmt.Errorf("store: import %q: totals: %w", title, err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// linkImported writes the developers, publishers, genres and play modes of g.
// Names resolve to catalog rows exactly as in EDT-004.
func linkImported(ctx context.Context, q *dbgen.Queries, id int64, g ImportGame) error {
	devs, err := resolveIDs(ctx, q, g.Developers, resolveCompany)
	if err != nil {
		return err
	}
	for _, c := range devs {
		if err := q.InsertGameDeveloper(ctx, dbgen.InsertGameDeveloperParams{GameID: id, CompanyID: c}); err != nil {
			return err
		}
	}
	pubs, err := resolveIDs(ctx, q, g.Publishers, resolveCompany)
	if err != nil {
		return err
	}
	for _, c := range pubs {
		if err := q.InsertGamePublisher(ctx, dbgen.InsertGamePublisherParams{GameID: id, CompanyID: c}); err != nil {
			return err
		}
	}
	genres, err := resolveIDs(ctx, q, g.Genres, resolveGenre)
	if err != nil {
		return err
	}
	for _, gid := range genres {
		if err := q.InsertGameGenre(ctx, dbgen.InsertGameGenreParams{GameID: id, GenreID: gid}); err != nil {
			return err
		}
	}
	modes, err := resolvePlayModes(ctx, q, g.PlayModes)
	if err != nil {
		return err
	}
	for _, m := range modes {
		if err := q.InsertGamePlayMode(ctx, dbgen.InsertGamePlayModeParams{GameID: id, PlayModeID: m}); err != nil {
			return err
		}
	}
	return nil
}

// insertImportRules writes the tracking rules of an imported game (RUL-001):
// the install folder when there is one, and the executable for exe launches.
func insertImportRules(ctx context.Context, q *dbgen.Queries, id int64, g ImportGame) error {
	if g.InstallDir != "" {
		if err := q.InsertTrackingRule(ctx, dbgen.InsertTrackingRuleParams{GameID: id, Kind: "install_dir", Value: g.InstallDir}); err != nil {
			return err
		}
	}
	if g.LaunchType == launchTypeExe {
		return q.InsertTrackingRule(ctx, dbgen.InsertTrackingRuleParams{GameID: id, Kind: "exe_path", Value: g.LaunchTarget})
	}
	return nil
}

// EnsurePlatformID returns the id of the platform called name, creating it with
// the given category when it does not exist (EDT-008). It is a separate
// transaction, so the agent calls it before an import transaction starts.
func (s *Store) EnsurePlatformID(ctx context.Context, name string, category domain.PlatformCategory) (int64, error) {
	p, err := s.AddPlatform(ctx, name, category)
	if err != nil {
		return 0, err
	}
	return p.ID, nil
}

// ImportDuplicate reports whether the library already has the game a candidate
// describes: the same launch target, or the same title on the same platform
// (IMP-006). A platform id of 0 (a folder candidate without a platform yet)
// compares the launch target only.
func (s *Store) ImportDuplicate(ctx context.Context, launchType, launchTarget, title string, platformID int64) (bool, error) {
	_, err := s.q.FindGameByLaunchTarget(ctx, dbgen.FindGameByLaunchTargetParams{
		LaunchType: launchType, LaunchTarget: launchTarget,
	})
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("store: import duplicate by target: %w", err)
	}
	if platformID == 0 {
		return false, nil
	}
	_, err = s.q.FindGameByTitleAndPlatform(ctx, dbgen.FindGameByTitleAndPlatformParams{
		Title: title, PlatformID: platformID,
	})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("store: import duplicate by title: %w", err)
}

// PlatformIDByName returns the id of the platform called name, without regard to
// case, and false when there is none. It never creates a platform, so the scan
// can use it (IMP-001).
func (s *Store) PlatformIDByName(ctx context.Context, name string) (int64, bool, error) {
	row, err := s.q.GetPlatformByNameNoCase(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("store: platform %q: %w", name, err)
	}
	return row.ID, true, nil
}
