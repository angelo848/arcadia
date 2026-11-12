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

// Emulator errors (EMU-001, EMU-002, EMU-005).
var (
	ErrEmulatorNotFound  = errors.New("store: emulator not found")
	ErrEmulatorNameTaken = errors.New("store: emulator name already in use")
	ErrNoEmulator        = errors.New("store: no emulator configured for this game")
)

// Emulator is a program that runs the games of one or more platforms (EMU-001).
// ArgsTemplate may use {rom} and {dir} (domain.BuildArgs).
type Emulator struct {
	ID           int64
	Name         string
	ExePath      string
	ArgsTemplate string
}

// Emulators returns every emulator, ordered by name without regard to case.
func (s *Store) Emulators(ctx context.Context) ([]Emulator, error) {
	rows, err := s.q.ListEmulators(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list emulators: %w", err)
	}
	out := make([]Emulator, 0, len(rows))
	for _, r := range rows {
		out = append(out, emulatorFromModel(r.ID, r.Name, r.ExePath, r.ArgsTemplate))
	}
	return out, nil
}

// AddEmulator stores a new emulator (EMU-001). The name is checked with DOM-016, the
// executable must be an absolute path and the template must pass
// domain.ValidateArgsTemplate; a name taken without regard to case is ErrEmulatorNameTaken.
func (s *Store) AddEmulator(ctx context.Context, name, exePath, argsTemplate string) (Emulator, error) {
	clean, exe, err := checkEmulator(name, exePath, argsTemplate)
	if err != nil {
		return Emulator{}, err
	}
	var out Emulator
	err = s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		taken, err := q.EmulatorNameTaken(ctx, dbgen.EmulatorNameTakenParams{Name: clean, ExceptID: 0})
		if err != nil {
			return fmt.Errorf("store: emulator name: %w", err)
		}
		if taken > 0 {
			return ErrEmulatorNameTaken
		}
		id, err := q.InsertEmulator(ctx, dbgen.InsertEmulatorParams{Name: clean, ExePath: exe, ArgsTemplate: argsTemplate})
		if err != nil {
			return fmt.Errorf("store: insert emulator: %w", err)
		}
		out = Emulator{ID: id, Name: clean, ExePath: exe, ArgsTemplate: argsTemplate}
		return nil
	})
	if err != nil {
		return Emulator{}, err
	}
	return out, nil
}

// UpdateEmulator changes an emulator (EMU-001). An unknown id is ErrEmulatorNotFound.
func (s *Store) UpdateEmulator(ctx context.Context, id int64, name, exePath, argsTemplate string) (Emulator, error) {
	clean, exe, err := checkEmulator(name, exePath, argsTemplate)
	if err != nil {
		return Emulator{}, err
	}
	var out Emulator
	err = s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		taken, err := q.EmulatorNameTaken(ctx, dbgen.EmulatorNameTakenParams{Name: clean, ExceptID: id})
		if err != nil {
			return fmt.Errorf("store: emulator name: %w", err)
		}
		if taken > 0 {
			return ErrEmulatorNameTaken
		}
		n, err := q.UpdateEmulator(ctx, dbgen.UpdateEmulatorParams{Name: clean, ExePath: exe, ArgsTemplate: argsTemplate, ID: id})
		if err != nil {
			return fmt.Errorf("store: update emulator %d: %w", id, err)
		}
		if n == 0 {
			return ErrEmulatorNotFound
		}
		out = Emulator{ID: id, Name: clean, ExePath: exe, ArgsTemplate: argsTemplate}
		return nil
	})
	if err != nil {
		return Emulator{}, err
	}
	return out, nil
}

// DeleteEmulator removes an emulator. Platforms and games that used it keep their
// own rows with the reference cleared (SET NULL); no game is deleted (EMU-002).
func (s *Store) DeleteEmulator(ctx context.Context, id int64) error {
	n, err := s.q.DeleteEmulator(ctx, id)
	if err != nil {
		return fmt.Errorf("store: delete emulator %d: %w", id, err)
	}
	if n == 0 {
		return ErrEmulatorNotFound
	}
	return nil
}

// SetPlatformEmulator sets the default emulator of a platform; a nil emulatorID
// clears it (EMU-002). Unknown platform or emulator ids are errors.
func (s *Store) SetPlatformEmulator(ctx context.Context, platformID int64, emulatorID *int64) error {
	if emulatorID != nil {
		if _, err := s.q.GetEmulator(ctx, *emulatorID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrEmulatorNotFound
			}
			return fmt.Errorf("store: read emulator %d: %w", *emulatorID, err)
		}
	}
	n, err := s.q.SetPlatformEmulator(ctx, dbgen.SetPlatformEmulatorParams{EmulatorID: nullInt(emulatorID), ID: platformID})
	if err != nil {
		return fmt.Errorf("store: set platform %d emulator: %w", platformID, err)
	}
	if n == 0 {
		return fmt.Errorf("store: platform %d: %w", platformID, ErrPlatformNotFound)
	}
	return nil
}

// SetGameEmulator sets the emulator of one game, overriding its platform's default
// (EMU-002). A nil emulatorID returns the game to the platform's default.
func (s *Store) SetGameEmulator(ctx context.Context, gameID int64, emulatorID *int64, at time.Time) error {
	if emulatorID != nil {
		if _, err := s.q.GetEmulator(ctx, *emulatorID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrEmulatorNotFound
			}
			return fmt.Errorf("store: read emulator %d: %w", *emulatorID, err)
		}
	}
	n, err := s.q.SetGameEmulator(ctx, dbgen.SetGameEmulatorParams{
		EmulatorID: nullInt(emulatorID), UpdatedAt: TimeToMillis(at), ID: gameID,
	})
	if err != nil {
		return fmt.Errorf("store: set game %d emulator: %w", gameID, err)
	}
	if n == 0 {
		return fmt.Errorf("store: set game %d emulator: %w", gameID, ErrGameNotFound)
	}
	return nil
}

// ResolveEmulator returns the emulator a game runs with (EMU-002): the game's own
// one when it has one, else its platform's default. A game with neither is
// ErrNoEmulator (EMU-005).
func (s *Store) ResolveEmulator(ctx context.Context, gameID int64) (Emulator, error) {
	ref, err := s.q.GameEmulatorRef(ctx, gameID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Emulator{}, fmt.Errorf("store: game %d: %w", gameID, ErrGameNotFound)
		}
		return Emulator{}, fmt.Errorf("store: emulator of game %d: %w", gameID, err)
	}
	id := ref.GameEmulatorID
	if !id.Valid {
		id = ref.PlatformEmulatorID
	}
	if !id.Valid {
		return Emulator{}, ErrNoEmulator
	}
	row, err := s.q.GetEmulator(ctx, id.Int64)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Emulator{}, ErrEmulatorNotFound
		}
		return Emulator{}, fmt.Errorf("store: read emulator %d: %w", id.Int64, err)
	}
	return emulatorFromModel(row.ID, row.Name, row.ExePath, row.ArgsTemplate), nil
}

// checkEmulator validates the fields of an emulator before they are stored.
func checkEmulator(name, exePath, argsTemplate string) (string, string, error) {
	clean, err := domain.NormalizeName(name)
	if err != nil {
		return "", "", fmt.Errorf("store: emulator name: %w", err)
	}
	if !filepath.IsAbs(exePath) {
		return "", "", fmt.Errorf("store: emulator path %q is not absolute: %w", exePath, domain.ErrInvalidField)
	}
	if err := domain.ValidateArgsTemplate(argsTemplate); err != nil {
		return "", "", fmt.Errorf("store: emulator arguments: %w", err)
	}
	return clean, filepath.Clean(exePath), nil
}

func emulatorFromModel(id int64, name, exe, args string) Emulator {
	return Emulator{ID: id, Name: name, ExePath: exe, ArgsTemplate: args}
}

// nullInt maps an optional id to the nullable column value.
func nullInt(id *int64) sql.NullInt64 {
	if id == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *id, Valid: true}
}
