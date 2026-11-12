package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// The library read side (LIB-001..003, FLT-002..008, STO-025, STO-026). The
// filter and the sort are plain SQL built here (ADR 0020, which excepts this from
// ADR 0004): the text is made of constant fragments and one "?" per value, and
// every value goes in the argument list. Tests: library_*_test.go.

// yearValid is true when release_date is YYYY, YYYY-MM or YYYY-MM-DD, the same
// rule as domain.ReleaseYear (DOM-013, decision 5). A NULL release_date makes it
// NULL, which is not true.
const yearValid = `(g.release_date GLOB '[0-9][0-9][0-9][0-9]' ` +
	`OR g.release_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]' ` +
	`OR g.release_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]')`

// yearExpr is the year of a release_date that passes yearValid.
const yearExpr = `CAST(substr(g.release_date, 1, 4) AS INTEGER)`

// titleOrder is the default order: title without regard to case, then id (GAM-005).
const titleOrder = `g.title COLLATE NOCASE, g.id`

// libGameSelect reads the columns of Game in the order scanGame reads them.
const libGameSelect = `SELECT g.id, g.platform_id, p.name, g.title, g.release_date,
	g.launch_type, g.launch_target, g.launch_args, g.working_dir, g.favorite, g.status,
	g.total_play_seconds, g.play_count, g.first_played_at, g.last_played_at
FROM game g
JOIN platform p ON p.id = g.platform_id`

// Query is one page of the library listing (LIB-001, LIB-002).
type Query struct {
	Filter domain.FilterSpec
	Sort   domain.Sort
	Offset int
	Limit  int
}

// Page is one page of games and the size of the whole filtered set.
type Page struct {
	Games []Game // never nil
	Total int64  // counts every game that passes the filter, not only the page (LIB-002)
}

// ListGames returns one page of the games that pass q.Filter, in q.Sort order.
// The filter is normalized and validated, and the limits checked, before any
// query runs; a failure returns an error without touching the database. The
// page and the total use the same WHERE in one transaction (STO-025).
func (s *Store) ListGames(ctx context.Context, q Query) (Page, error) {
	f := q.Filter.Normalize()
	if err := f.Validate(); err != nil {
		return Page{}, fmt.Errorf("store: list games: %w", err)
	}
	if q.Limit < 1 || q.Limit > domain.MaxPageLimit {
		return Page{}, fmt.Errorf("store: list games: limit %d outside 1..%d", q.Limit, domain.MaxPageLimit)
	}
	if q.Offset < 0 {
		return Page{}, fmt.Errorf("store: list games: negative offset %d", q.Offset)
	}
	order, err := sortClause(q.Sort)
	if err != nil {
		return Page{}, err
	}
	where, args := buildGameFilter(f)

	page := Page{Games: []Game{}}
	err = s.withTx(ctx, func(tx *sql.Tx, _ *dbgen.Queries) error {
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM game g `+where, args...).Scan(&page.Total); err != nil {
			return fmt.Errorf("store: count games: %w", err)
		}
		pageArgs := append(slices.Clone(args), q.Limit, q.Offset)
		rows, err := tx.QueryContext(ctx,
			libGameSelect+" "+where+" ORDER BY "+order+" LIMIT ? OFFSET ?", pageArgs...)
		if err != nil {
			return fmt.Errorf("store: list games: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			g, err := scanGame(rows)
			if err != nil {
				return err
			}
			page.Games = append(page.Games, g)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("store: list games rows: %w", err)
		}
		return nil
	})
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

// scanGame reads one row of libGameSelect.
func scanGame(rows *sql.Rows) (Game, error) {
	var (
		g                       Game
		platform, status        string
		release                 sql.NullString
		favorite                int64
		firstPlayed, lastPlayed sql.NullInt64
	)
	if err := rows.Scan(&g.ID, &g.PlatformID, &platform, &g.Title, &release,
		&g.LaunchType, &g.LaunchTarget, &g.LaunchArgs, &g.WorkingDir, &favorite, &status,
		&g.TotalPlaySeconds, &g.PlayCount, &firstPlayed, &lastPlayed); err != nil {
		return Game{}, fmt.Errorf("store: scan game: %w", err)
	}
	g.Platform = platform
	g.Status = domain.GameStatus(status) // tolerant reader (ADR 0012)
	g.Favorite = favorite != 0
	g.ReleaseDate = release.String
	g.FirstPlayedAt = nullMillisToTime(firstPlayed)
	g.LastPlayedAt = nullMillisToTime(lastPlayed)
	return g, nil
}

// buildGameFilter returns the WHERE clause for f ("WHERE ..." or "" when f has no
// condition) and its arguments. Each facet is one condition; facets are joined
// with AND, and the ids inside a facet are any-of (FLT-002). text goes through
// ftsQuery and the game_fts index (FLT-007). f must be normalized.
func buildGameFilter(f domain.FilterSpec) (string, []any) {
	var conds []string
	var args []any
	add := func(cond string, vals ...any) {
		conds = append(conds, cond)
		args = append(args, vals...)
	}

	if len(f.PlatformIDs) > 0 {
		add("g.platform_id IN ("+placeholders(len(f.PlatformIDs))+")", idArgs(f.PlatformIDs)...)
	}
	if len(f.GenreIDs) > 0 {
		add("EXISTS (SELECT 1 FROM game_genre gg WHERE gg.game_id = g.id AND gg.genre_id IN ("+
			placeholders(len(f.GenreIDs))+"))", idArgs(f.GenreIDs)...)
	}
	if len(f.DeveloperIDs) > 0 {
		add("EXISTS (SELECT 1 FROM game_developer gd WHERE gd.game_id = g.id AND gd.company_id IN ("+
			placeholders(len(f.DeveloperIDs))+"))", idArgs(f.DeveloperIDs)...)
	}
	if len(f.PublisherIDs) > 0 {
		add("EXISTS (SELECT 1 FROM game_publisher gp WHERE gp.game_id = g.id AND gp.company_id IN ("+
			placeholders(len(f.PublisherIDs))+"))", idArgs(f.PublisherIDs)...)
	}
	if modes := f.PlayMode.Modes(); modes != nil {
		vals := make([]any, 0, len(modes))
		for _, m := range modes {
			vals = append(vals, string(m))
		}
		add("EXISTS (SELECT 1 FROM game_play_mode gm JOIN play_mode pm ON pm.id = gm.play_mode_id "+
			"WHERE gm.game_id = g.id AND pm.name IN ("+placeholders(len(modes))+"))", vals...)
	}
	if f.YearFrom != nil || f.YearTo != nil {
		// A bounded range needs a year, so a game without one is out (FLT-004).
		yearConds := []string{yearValid}
		var yearArgs []any
		if f.YearFrom != nil {
			yearConds = append(yearConds, yearExpr+" >= ?")
			yearArgs = append(yearArgs, *f.YearFrom)
		}
		if f.YearTo != nil {
			yearConds = append(yearConds, yearExpr+" <= ?")
			yearArgs = append(yearArgs, *f.YearTo)
		}
		add("("+strings.Join(yearConds, " AND ")+")", yearArgs...)
	}
	if len(f.Statuses) > 0 {
		vals := make([]any, 0, len(f.Statuses))
		for _, st := range f.Statuses {
			vals = append(vals, string(st))
		}
		add("g.status IN ("+placeholders(len(f.Statuses))+")", vals...)
	}
	if f.Favorite != nil {
		fav := 0
		if *f.Favorite {
			fav = 1
		}
		add("g.favorite = ?", fav)
	}
	switch f.Played {
	case "played": // FLT-006
		add("g.play_count > 0")
	case "notPlayed":
		add("g.play_count = 0")
	}
	if f.PlaySecondsMin != nil {
		add("g.total_play_seconds >= ?", *f.PlaySecondsMin)
	}
	if f.PlaySecondsMax != nil {
		add("g.total_play_seconds <= ?", *f.PlaySecondsMax)
	}
	if q, ok := ftsQuery(f.Text); ok {
		add("g.id IN (SELECT rowid FROM game_fts WHERE game_fts MATCH ?)", q)
	}

	if len(conds) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// sortClause returns the fixed ORDER BY of s (LIB-001). Every clause ends with
// the id, so the order is total.
func sortClause(s domain.Sort) (string, error) {
	switch s {
	case domain.SortTitle:
		return titleOrder, nil
	case domain.SortLastPlayed:
		return `g.last_played_at IS NULL, g.last_played_at DESC, ` + titleOrder, nil
	case domain.SortTotalPlayed:
		return `g.total_play_seconds DESC, ` + titleOrder, nil
	case domain.SortYear:
		// Games with a year first, by year; games without one last.
		return `CASE WHEN ` + yearValid + ` THEN 0 ELSE 1 END, ` + yearExpr + `, ` + titleOrder, nil
	}
	return "", fmt.Errorf("store: unknown sort %q", s)
}

// placeholders returns n "?" separated by commas.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// idArgs turns ids into query arguments.
func idArgs(ids []int64) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, id)
	}
	return out
}

// GameDetail is a game with its description, completion date, links and play
// modes (STO-026). The lists are never nil.
type GameDetail struct {
	Game
	EmulatorID                     *int64 // the game's own emulator, nil to use the platform default (EMU-002)
	Description                    string
	CompletedAt                    time.Time // zero = NULL
	Developers, Publishers, Genres []string  // never nil; NOCASE order
	PlayModes                      []string  // never nil; names of DOM-014
}

// GameDetail returns the game of id with its details, read in one transaction.
// An unknown id gives an error wrapping ErrGameNotFound.
func (s *Store) GameDetail(ctx context.Context, id int64) (GameDetail, error) {
	var d GameDetail
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		var err error
		d, err = readGameDetail(ctx, q, id)
		return err
	})
	if err != nil {
		return GameDetail{}, err
	}
	return d, nil
}

// readGameDetail reads the game of id with its details using q, so it can run
// inside a transaction (UpdateGame returns the same shape). An unknown id gives
// an error wrapping ErrGameNotFound.
func readGameDetail(ctx context.Context, q *dbgen.Queries, id int64) (GameDetail, error) {
	var d GameDetail
	row, err := gameRow(ctx, q, id)
	if err != nil {
		return GameDetail{}, err
	}
	d.Game = gameFromRow(row)
	d.Description = row.Description
	d.CompletedAt = nullMillisToTime(row.CompletedAt)
	if ref, err := q.GameEmulatorRef(ctx, id); err == nil {
		d.EmulatorID = nullIntPtr(ref.GameEmulatorID)
	} else {
		return GameDetail{}, fmt.Errorf("store: emulator of game %d: %w", id, err)
	}
	if d.Developers, err = q.GameDeveloperNames(ctx, id); err != nil {
		return GameDetail{}, fmt.Errorf("store: developers of game %d: %w", id, err)
	}
	if d.Publishers, err = q.GamePublisherNames(ctx, id); err != nil {
		return GameDetail{}, fmt.Errorf("store: publishers of game %d: %w", id, err)
	}
	if d.Genres, err = q.GameGenreNames(ctx, id); err != nil {
		return GameDetail{}, fmt.Errorf("store: genres of game %d: %w", id, err)
	}
	if d.PlayModes, err = q.GamePlayModeNames(ctx, id); err != nil {
		return GameDetail{}, fmt.Errorf("store: play modes of game %d: %w", id, err)
	}
	return d, nil
}
