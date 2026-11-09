package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// ErrSettingNotFound is returned by Setting when the key has no stored value.
var ErrSettingNotFound = errors.New("store: setting not found")

// Setting returns the stored value of key, or ErrSettingNotFound.
func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	row, err := s.q.GetSetting(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("store: setting %q: %w", key, ErrSettingNotFound)
		}
		return "", fmt.Errorf("store: get setting %q: %w", key, err)
	}
	return row.Value, nil
}

// SetSetting inserts or replaces the value of key, recording at as updated_at.
func (s *Store) SetSetting(ctx context.Context, key, value string, at time.Time) error {
	if err := s.q.UpsertSetting(ctx, dbgen.UpsertSettingParams{
		Key:       key,
		Value:     value,
		UpdatedAt: TimeToMillis(at),
	}); err != nil {
		return fmt.Errorf("store: set setting %q: %w", key, err)
	}
	return nil
}
