package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// migrate applies every embedded migration up to SchemaVersion (ADR 0010:
// only the agent calls this). Re-running on an up-to-date database is a no-op.
func migrate(ctx context.Context, db *sql.DB) error {
	fsys, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("store: migrations fs: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, fsys)
	if err != nil {
		return fmt.Errorf("store: goose provider: %w", err)
	}
	sources := provider.ListSources()
	if len(sources) == 0 || sources[len(sources)-1].Version != SchemaVersion {
		return fmt.Errorf("store: embedded migrations do not end at SchemaVersion %d", SchemaVersion)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("store: migrate up: %w", err)
	}
	return nil
}

// readVersion reads the applied goose version without touching the schema or
// goose_db_version: no goose call that could create that table is made here.
// (The DSN's journal_mode(WAL) pragma may still convert the journal and create
// -wal/-shm; see design.md.) found is false when the goose table does not exist.
func readVersion(ctx context.Context, db *sql.DB) (version int64, found bool, err error) {
	var n int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`).
		Scan(&n); err != nil {
		return 0, false, fmt.Errorf("store: look up goose_db_version: %w", err)
	}
	if n == 0 {
		return 0, false, nil
	}
	var v sql.NullInt64
	// Matches goose's Provider: Down deletes the version row, so the highest
	// applied version_id is the current version.
	if err := db.QueryRowContext(ctx,
		`SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`).Scan(&v); err != nil {
		return 0, true, fmt.Errorf("store: read goose version: %w", err)
	}
	return v.Int64, true, nil
}
