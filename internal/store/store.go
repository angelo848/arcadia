// Package store owns the SQLite database: opening with the required pragmas,
// goose migrations (agent only, ADR 0010) and the sqlc-generated queries.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver (no CGO)

	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// SchemaVersion is the last migration embedded in the binary (10: time_to_beat table, TTB-001).
const SchemaVersion int64 = 10

// ErrSchemaMismatch is returned when the database schema version differs from
// SchemaVersion (or the database does not exist) and migration is disabled.
var ErrSchemaMismatch = errors.New("store: schema version mismatch")

// OpenOptions controls how Open treats the schema.
type OpenOptions struct {
	Migrate bool // true only in the agent (ADR 0010)
}

// Store wraps the database pool and the generated queries.
type Store struct {
	db *sql.DB
	q  *dbgen.Queries
}

// dsnPragmas are applied by the driver on every new connection of the pool.
const dsnPragmas = "_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)" +
	"&_pragma=synchronous(NORMAL)&_txlock=immediate"

// buildDSN returns a file: URI for path. mode is "rwc" (create) or "rw".
func buildDSN(path, mode string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("store: absolute path of %q: %w", path, err)
	}
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows: file:///C:/... (SQLite drops the slash before the drive letter)
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "mode=" + mode + "&" + dsnPragmas}
	return u.String(), nil
}

// Open opens the database at path. With Migrate it creates the file if needed
// and applies the embedded migrations; without it, the file must already
// exist at exactly SchemaVersion, otherwise ErrSchemaMismatch is returned.
// Without migration the schema and goose_db_version are never modified; the
// journal_mode(WAL) pragma in the DSN may still convert the journal of a
// non-WAL file and create -wal/-shm next to it (accepted, see design.md).
func Open(ctx context.Context, path string, opts OpenOptions) (*Store, error) {
	mode := "rwc"
	if !opts.Migrate {
		mode = "rw"
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("store: database %q does not exist (found version 0, expected %d): %w",
					path, SchemaVersion, ErrSchemaMismatch)
			}
			return nil, fmt.Errorf("store: stat %q: %w", path, err)
		}
	}

	dsn, err := buildDSN(path, mode)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %q: %w", path, err)
	}

	if err := prepare(ctx, db, path, opts); err != nil {
		if cerr := db.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("store: close after failed open: %w", cerr))
		}
		return nil, err
	}
	return &Store{db: db, q: dbgen.New(db)}, nil
}

// prepare migrates or validates the schema of a freshly opened pool.
func prepare(ctx context.Context, db *sql.DB, path string, opts OpenOptions) error {
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("store: connect %q: %w", path, err)
	}
	if opts.Migrate {
		if err := migrate(ctx, db); err != nil {
			return err
		}
	}
	version, found, err := readVersion(ctx, db)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("store: %q has no goose_db_version table (found version 0, expected %d): %w",
			path, SchemaVersion, ErrSchemaMismatch)
	}
	if version != SchemaVersion {
		return fmt.Errorf("store: %q is at version %d, expected %d: %w",
			path, version, SchemaVersion, ErrSchemaMismatch)
	}
	return nil
}

// Close closes the connection pool.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}

// DB returns the underlying pool.
func (s *Store) DB() *sql.DB { return s.db }

// Queries returns the generated queries bound to the pool.
func (s *Store) Queries() *dbgen.Queries { return s.q }

// Tx runs fn inside a write transaction (BEGIN IMMEDIATE, via _txlock). It
// commits when fn returns nil and rolls back otherwise, including when fn
// panics (the panic keeps propagating after the rollback).
func (s *Store) Tx(ctx context.Context, fn func(q *dbgen.Queries) error) error {
	return s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error { return fn(q) })
}

// withTx is Tx with the raw *sql.Tx also passed to fn, for code that needs both
// the generated queries and plain SQL (the FTS index, fts.go) in one transaction.
func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx, q *dbgen.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	// Safety net for a panic in fn. After Commit or the explicit Rollback below
	// this returns sql.ErrTxDone, which is expected and ignored.
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx, s.q.WithTx(tx)); err != nil {
		// ErrTxDone means the transaction is already finished (e.g. ctx was
		// cancelled and database/sql rolled it back); not a rollback failure.
		if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("store: rollback: %w", rerr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// CurrentVersion returns the applied goose version, reading only (0 when the
// goose table is absent).
func (s *Store) CurrentVersion(ctx context.Context) (int64, error) {
	v, _, err := readVersion(ctx, s.db)
	return v, err
}
