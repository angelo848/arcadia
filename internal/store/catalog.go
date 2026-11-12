package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// Platform is a platform of the library (IPC-048, EDT-008).
type Platform struct {
	ID         int64
	Name       string
	Category   domain.PlatformCategory
	EmulatorID *int64 // default emulator of the platform, nil when none (EMU-002)
}

// CatalogItem is a genre or a company of the catalog: its database id, which the
// FilterSpec filters by (FLT-002, FLT-003), and its name.
type CatalogItem struct {
	ID   int64
	Name string
}

// Catalog is what the edit form offers: the platforms, the genres and companies
// already stored, and the five play modes (STO-030). No field is nil.
type Catalog struct {
	Platforms []Platform
	Genres    []CatalogItem
	Companies []CatalogItem
	PlayModes []domain.PlayMode // the five of DOM-014
}

// catalogPlayModes are the five play modes of DOM-014, in declaration order.
var catalogPlayModes = []domain.PlayMode{
	domain.PlayModeSingle,
	domain.PlayModeMultiLocal,
	domain.PlayModeMultiOnline,
	domain.PlayModeCoopLocal,
	domain.PlayModeCoopOnline,
}

// Platforms returns every platform in name order without regard to case, then id
// (IPC-048). The result is never nil.
func (s *Store) Platforms(ctx context.Context) ([]Platform, error) {
	return platformsWith(ctx, s.q)
}

// platformsWith is Platforms reading with q, so Catalog can share a transaction.
func platformsWith(ctx context.Context, q *dbgen.Queries) ([]Platform, error) {
	rows, err := q.ListPlatformsByName(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list platforms: %w", err)
	}
	out := make([]Platform, 0, len(rows))
	for _, r := range rows {
		out = append(out, platformFromModel(r))
	}
	return out, nil
}

// Catalog reads the platforms, genres, companies and play modes in one
// transaction. Genres and companies are in name order without regard to case
// (NOCASE), each with its database id. Nothing in the result is nil.
func (s *Store) Catalog(ctx context.Context) (Catalog, error) {
	var c Catalog
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		ps, err := platformsWith(ctx, q)
		if err != nil {
			return err
		}
		genres, err := q.ListGenreNames(ctx)
		if err != nil {
			return fmt.Errorf("store: catalog genres: %w", err)
		}
		companies, err := q.ListCompanyNames(ctx)
		if err != nil {
			return fmt.Errorf("store: catalog companies: %w", err)
		}
		c = Catalog{
			Platforms: ps,
			Genres:    genreItems(genres),
			Companies: companyItems(companies),
			PlayModes: slices.Clone(catalogPlayModes),
		}
		return nil
	})
	if err != nil {
		return Catalog{}, err
	}
	return c, nil
}

// AddPlatform returns the platform named name, creating it with category when no
// platform matches the name without regard to case (EDT-008). An existing platform
// is returned as stored, with its own spelling and category, and category is
// ignored. The name is cut as DOM-016 requires; an invalid name or category is an
// error and nothing is written.
func (s *Store) AddPlatform(ctx context.Context, name string, category domain.PlatformCategory) (Platform, error) {
	clean, err := domain.NormalizeName(name)
	if err != nil {
		return Platform{}, fmt.Errorf("store: add platform: name: %w", err)
	}
	cat, err := domain.ParsePlatformCategory(string(category))
	if err != nil {
		return Platform{}, fmt.Errorf("store: add platform: category: %w", err)
	}
	var out Platform
	err = s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		row, err := q.GetPlatformByNameNoCase(ctx, clean)
		if err == nil {
			out = platformFromModel(row)
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: find platform %q: %w", clean, err)
		}
		id, err := q.InsertPlatform(ctx, dbgen.InsertPlatformParams{Name: clean, Category: string(cat)})
		if err != nil {
			return fmt.Errorf("store: insert platform %q: %w", clean, err)
		}
		out = Platform{ID: id, Name: clean, Category: cat}
		return nil
	})
	if err != nil {
		return Platform{}, err
	}
	return out, nil
}

// genreItems maps genre rows to CatalogItem, never returning nil.
func genreItems(rows []dbgen.Genre) []CatalogItem {
	out := make([]CatalogItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, CatalogItem{ID: r.ID, Name: r.Name})
	}
	return out
}

// companyItems maps company rows to CatalogItem, never returning nil.
func companyItems(rows []dbgen.Company) []CatalogItem {
	out := make([]CatalogItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, CatalogItem{ID: r.ID, Name: r.Name})
	}
	return out
}

// platformFromModel maps a platform row to Platform.
func platformFromModel(p dbgen.Platform) Platform {
	return Platform{ID: p.ID, Name: p.Name, Category: domain.PlatformCategory(p.Category), EmulatorID: nullIntPtr(p.EmulatorID)}
}
