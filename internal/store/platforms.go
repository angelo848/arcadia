package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// ErrPlatformNameTaken is returned when another platform already has the name,
// without regard to case (PLT-018).
var ErrPlatformNameTaken = errors.New("store: platform name already used")

// ErrPlatformInUse is returned by DeletePlatform when the platform has games (PLT-021).
var ErrPlatformInUse = errors.New("store: platform has games")

// PlatformInUseError carries the platform and its game count (hidden games included).
type PlatformInUseError struct{ PlatformID, GameCount int64 }

// Error implements error.
func (e *PlatformInUseError) Error() string {
	return fmt.Sprintf("store: platform %d has %d games", e.PlatformID, e.GameCount)
}

// Unwrap makes errors.Is(err, ErrPlatformInUse) true.
func (e *PlatformInUseError) Unwrap() error { return ErrPlatformInUse }

// platformByIDWith reads a platform with its game count; ErrPlatformNotFound when absent.
func platformByIDWith(ctx context.Context, q *dbgen.Queries, id int64) (Platform, error) {
	r, err := q.GetPlatformWithCount(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Platform{}, fmt.Errorf("store: platform %d: %w", id, ErrPlatformNotFound)
	}
	if err != nil {
		return Platform{}, fmt.Errorf("store: get platform %d: %w", id, err)
	}
	return Platform{
		ID: r.ID, Name: r.Name, Category: domain.PlatformCategory(r.Category),
		EmulatorID: nullIntPtr(r.EmulatorID), SeedKey: r.SeedKey.String, Icon: r.Icon.String,
		GameCount: r.GameCount,
	}, nil
}

// PlatformByID returns the platform with its game count, or ErrPlatformNotFound.
func (s *Store) PlatformByID(ctx context.Context, id int64) (Platform, error) {
	return platformByIDWith(ctx, s.q, id)
}

// resolvePlatformWith applies PLT-025 steps 1 and 2 to a clean name: same name
// without regard to case, then the platform of the seed key whose canonical name it is.
func resolvePlatformWith(ctx context.Context, q *dbgen.Queries, clean string) (Platform, bool, error) {
	row, err := q.GetPlatformByNameNoCase(ctx, clean)
	if err == nil {
		p, err := platformByIDWith(ctx, q, row.ID)
		return p, err == nil, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Platform{}, false, fmt.Errorf("store: find platform %q: %w", clean, err)
	}
	seed, ok := domain.SeedByName(clean)
	if !ok {
		return Platform{}, false, nil
	}
	row, err = q.GetPlatformBySeedKey(ctx, sql.NullString{String: seed.Key, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return Platform{}, false, nil
	}
	if err != nil {
		return Platform{}, false, fmt.Errorf("store: find platform by key %q: %w", seed.Key, err)
	}
	p, err := platformByIDWith(ctx, q, row.ID)
	return p, err == nil, err
}

// ResolvePlatform finds a platform by name (PLT-025): the same name without regard
// to case, or the platform of the seed key when name is canonical. It never
// creates; an invalid name is simply not found.
func (s *Store) ResolvePlatform(ctx context.Context, name string) (Platform, bool, error) {
	clean, err := domain.NormalizeName(name)
	if err != nil {
		return Platform{}, false, nil
	}
	return resolvePlatformWith(ctx, s.q, clean)
}

// AddPlatform resolves name (PLT-025) and returns the platform found with
// created=false, or creates it with created=true. A canonical seed name whose key
// is free creates the platform with that key and the seed category (PLT-017);
// otherwise it uses the given category. An invalid name or category is an error.
func (s *Store) AddPlatform(ctx context.Context, name string, category domain.PlatformCategory) (Platform, bool, error) {
	clean, err := domain.NormalizeName(name)
	if err != nil {
		return Platform{}, false, fmt.Errorf("store: add platform: name: %w", err)
	}
	cat, err := domain.ParsePlatformCategory(string(category))
	if err != nil {
		return Platform{}, false, fmt.Errorf("store: add platform: category: %w", err)
	}
	var out Platform
	var created bool
	err = s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		found, ok, err := resolvePlatformWith(ctx, q, clean)
		if err != nil {
			return err
		}
		if ok {
			out, created = found, false
			return nil
		}
		key := sql.NullString{}
		if seed, isSeed := domain.SeedByName(clean); isSeed {
			key = sql.NullString{String: seed.Key, Valid: true}
			cat = seed.Category
		}
		id, err := q.InsertSeedPlatform(ctx, dbgen.InsertSeedPlatformParams{Name: clean, Category: string(cat), SeedKey: key})
		if err != nil {
			return fmt.Errorf("store: insert platform %q: %w", clean, err)
		}
		out, err = platformByIDWith(ctx, q, id)
		created = true
		return err
	})
	if err != nil {
		return Platform{}, false, err
	}
	return out, created, nil
}

// UpdatePlatform renames and recategorizes a platform (PLT-018, PLT-019). Seed key,
// icon, emulator and games stay. ErrPlatformNotFound for a missing id,
// ErrPlatformNameTaken when another platform has the name.
func (s *Store) UpdatePlatform(ctx context.Context, id int64, name string, category domain.PlatformCategory) (Platform, error) {
	clean, err := domain.NormalizeName(name)
	if err != nil {
		return Platform{}, fmt.Errorf("store: update platform: name: %w", err)
	}
	cat, err := domain.ParsePlatformCategory(string(category))
	if err != nil {
		return Platform{}, fmt.Errorf("store: update platform: category: %w", err)
	}
	var out Platform
	err = s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		if _, err := platformByIDWith(ctx, q, id); err != nil {
			return err
		}
		other, err := q.GetPlatformByNameNoCase(ctx, clean)
		if err == nil && other.ID != id {
			return fmt.Errorf("store: update platform %d: %q: %w", id, clean, ErrPlatformNameTaken)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: find platform %q: %w", clean, err)
		}
		if _, err := q.UpdatePlatform(ctx, dbgen.UpdatePlatformParams{Name: clean, Category: string(cat), ID: id}); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return fmt.Errorf("store: update platform %d: %q: %w", id, clean, ErrPlatformNameTaken)
			}
			return fmt.Errorf("store: update platform %d: %w", id, err)
		}
		out, err = platformByIDWith(ctx, q, id)
		return err
	})
	if err != nil {
		return Platform{}, err
	}
	return out, nil
}

// SetPlatformIcon stores icon ("" clears it) and returns the platform and the
// previous icon (PLT-013). ErrPlatformNotFound for a missing id.
func (s *Store) SetPlatformIcon(ctx context.Context, id int64, icon string) (p Platform, previous string, err error) {
	err = s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		cur, err := platformByIDWith(ctx, q, id)
		if err != nil {
			return err
		}
		previous = cur.Icon
		if _, err := q.SetPlatformIcon(ctx, dbgen.SetPlatformIconParams{
			Icon: sql.NullString{String: icon, Valid: icon != ""}, ID: id,
		}); err != nil {
			return fmt.Errorf("store: set platform %d icon: %w", id, err)
		}
		p, err = platformByIDWith(ctx, q, id)
		return err
	})
	if err != nil {
		return Platform{}, "", err
	}
	return p, previous, nil
}

// PlatformIconInUse reports whether any platform uses icon (PLT-013).
func (s *Store) PlatformIconInUse(ctx context.Context, icon string) (bool, error) {
	n, err := s.q.CountPlatformsWithIcon(ctx, sql.NullString{String: icon, Valid: true})
	if err != nil {
		return false, fmt.Errorf("store: count platforms with icon: %w", err)
	}
	return n > 0, nil
}

// DeletePlatform deletes a platform without games and returns its icon ("" when
// none). A platform with games (hidden ones included) gives *PlatformInUseError
// and nothing is deleted (PLT-020, PLT-021); a missing id gives ErrPlatformNotFound.
func (s *Store) DeletePlatform(ctx context.Context, id int64) (icon string, err error) {
	err = s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		cur, err := platformByIDWith(ctx, q, id)
		if err != nil {
			return err
		}
		if cur.GameCount > 0 {
			return &PlatformInUseError{PlatformID: id, GameCount: cur.GameCount}
		}
		if _, err := q.DeletePlatformRow(ctx, id); err != nil {
			return fmt.Errorf("store: delete platform %d: %w", id, err)
		}
		icon = cur.Icon
		return nil
	})
	if err != nil {
		return "", err
	}
	return icon, nil
}

// ensureWindowsPlatformWith returns the id of the platform of seed key "windows",
// creating ("Windows", pc, "windows") when missing. It never resolves by name,
// except that a keyless platform already named Windows receives the key instead
// of a second insert (PLT-026, PLT-027).
func ensureWindowsPlatformWith(ctx context.Context, q *dbgen.Queries) (int64, error) {
	key := sql.NullString{String: domain.SeedKeyWindows, Valid: true}
	row, err := q.GetPlatformBySeedKey(ctx, key)
	if err == nil {
		return row.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store: windows platform: %w", err)
	}
	seed, _ := domain.SeedByKey(domain.SeedKeyWindows)
	named, err := q.GetPlatformByNameNoCase(ctx, seed.Name)
	if err == nil {
		if !named.SeedKey.Valid {
			if _, err := q.SetPlatformSeedKey(ctx, dbgen.SetPlatformSeedKeyParams{SeedKey: key, ID: named.ID}); err != nil {
				return 0, fmt.Errorf("store: set windows key: %w", err)
			}
		}
		return named.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store: find platform %q: %w", seed.Name, err)
	}
	id, err := q.InsertSeedPlatform(ctx, dbgen.InsertSeedPlatformParams{Name: seed.Name, Category: string(seed.Category), SeedKey: key})
	if err != nil {
		return 0, fmt.Errorf("store: insert windows platform: %w", err)
	}
	return id, nil
}

// EnsureWindowsPlatformID returns the id of the platform of seed key "windows"
// (renamed or not), creating it when missing (PLT-026, PLT-027).
func (s *Store) EnsureWindowsPlatformID(ctx context.Context) (int64, error) {
	var id int64
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		var err error
		id, err = ensureWindowsPlatformWith(ctx, q)
		return err
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// WindowsPlatformID returns the id of the platform of seed key "windows"; false
// when none. It never creates, so the scan can use it (PLT-026).
func (s *Store) WindowsPlatformID(ctx context.Context) (int64, bool, error) {
	row, err := s.q.GetPlatformBySeedKey(ctx, sql.NullString{String: domain.SeedKeyWindows, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("store: windows platform: %w", err)
	}
	return row.ID, true, nil
}
