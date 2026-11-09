package main

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// ErrAgentNotReady is returned while the library database cannot be used yet:
// the agent was never reached or has not migrated the schema (DSK-007, DSK-008).
var ErrAgentNotReady = errors.New("agente ainda não pronto")

// errLibraryClosed is returned by Summary after Close.
var errLibraryClosed = errors.New("library: closed")

// libraryStore is the part of *store.Store the UI uses.
type libraryStore interface {
	LibrarySummary(ctx context.Context) (store.LibrarySummary, error)
	Close() error
}

// libraryGate opens the library lazily, only after the first successful
// connection to the agent and without migrating (ADR 0010). A failed open is
// not remembered: the next Summary tries again. All access is serialized.
type libraryGate struct {
	open func(ctx context.Context) (libraryStore, error)

	mu        sync.Mutex
	st        libraryStore
	connected bool
	closed    bool
}

// newLibraryGate returns a gate that opens the store with open.
func newLibraryGate(open func(ctx context.Context) (libraryStore, error)) *libraryGate {
	return &libraryGate{open: open}
}

// storeOpener returns the real opener: store.Open without migration.
func storeOpener(dbPath string) func(ctx context.Context) (libraryStore, error) {
	return func(ctx context.Context) (libraryStore, error) {
		st, err := store.Open(ctx, dbPath, store.OpenOptions{Migrate: false})
		if err != nil {
			return nil, err
		}
		return st, nil
	}
}

// MarkConnected records that the agent answered at least once. It does not
// open the database. It is idempotent.
func (g *libraryGate) MarkConnected() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.connected = true
}

// Summary returns the library counts. Before MarkConnected, or when opening
// fails with store.ErrSchemaMismatch, it returns ErrAgentNotReady.
func (g *libraryGate) Summary(ctx context.Context) (LibrarySummaryView, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return LibrarySummaryView{}, errLibraryClosed
	}
	if !g.connected {
		return LibrarySummaryView{}, ErrAgentNotReady
	}
	if g.st == nil {
		st, err := g.open(ctx)
		if err != nil {
			if errors.Is(err, store.ErrSchemaMismatch) {
				return LibrarySummaryView{}, ErrAgentNotReady
			}
			return LibrarySummaryView{}, fmt.Errorf("library: open: %w", err)
		}
		g.st = st
	}
	sum, err := g.st.LibrarySummary(ctx)
	if err != nil {
		return LibrarySummaryView{}, fmt.Errorf("library: summary: %w", err)
	}
	return LibrarySummaryView{Games: sum.Games, Platforms: sum.Platforms}, nil
}

// Close closes the store if it was opened. It is idempotent.
func (g *libraryGate) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
	if g.st == nil {
		return nil
	}
	st := g.st
	g.st = nil
	if err := st.Close(); err != nil {
		return fmt.Errorf("library: close: %w", err)
	}
	return nil
}
