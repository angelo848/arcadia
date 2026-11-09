package main

// Tests for the lazy library gate (DSK-007, DSK-008; ADR 0010).

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// *store.Store must satisfy libraryStore (the real opener returns it).
var _ libraryStore = (*store.Store)(nil)

func summaryOf(games, platforms int64) store.LibrarySummary {
	return store.LibrarySummary{Games: games, Platforms: platforms}
}

type fakeStore struct {
	summary store.LibrarySummary
	err     error

	mu     sync.Mutex
	calls  int
	closed int
}

func (s *fakeStore) LibrarySummary(context.Context) (store.LibrarySummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.summary, s.err
}

func (s *fakeStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
	return nil
}

func (s *fakeStore) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

type openResult struct {
	st  libraryStore
	err error
}

// fakeOpener returns results in order; after the last one it keeps returning it.
// With no results it fails with errOpenUnexpected.
type fakeOpener struct {
	mu      sync.Mutex
	results []openResult
	calls   int
}

var errOpenUnexpected = errors.New("fake open: unexpected call")

func (o *fakeOpener) open(context.Context) (libraryStore, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls++
	if len(o.results) == 0 {
		return nil, errOpenUnexpected
	}
	i := o.calls - 1
	if i >= len(o.results) {
		i = len(o.results) - 1
	}
	r := o.results[i]
	return r.st, r.err
}

func (o *fakeOpener) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.calls
}

func assertNotReady(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrAgentNotReady) {
		t.Fatalf("err = %v, want errors.Is(err, ErrAgentNotReady)", err)
	}
	if !strings.Contains(err.Error(), "agente ainda não pronto") {
		t.Errorf("err message = %q, want it to contain %q", err.Error(), "agente ainda não pronto")
	}
}

func TestLibraryGate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("DSK-007/antes de conectar não chama open", func(t *testing.T) {
		t.Parallel()
		op := &fakeOpener{results: []openResult{{st: &fakeStore{}}}}
		g := newLibraryGate(op.open)
		t.Cleanup(func() { _ = g.Close() })

		for i := 0; i < 2; i++ {
			_, err := g.Summary(ctx)
			assertNotReady(t, err)
		}
		if n := op.count(); n != 0 {
			t.Errorf("open called %d times before MarkConnected, want 0", n)
		}
	})

	t.Run("DSK-007/após MarkConnected abre e responde", func(t *testing.T) {
		t.Parallel()
		fs := &fakeStore{summary: summaryOf(12, 3)}
		op := &fakeOpener{results: []openResult{{st: fs}}}
		g := newLibraryGate(op.open)

		g.MarkConnected()
		for i := 0; i < 2; i++ {
			got, err := g.Summary(ctx)
			if err != nil {
				t.Fatalf("Summary #%d err = %v", i+1, err)
			}
			if want := (LibrarySummaryView{Games: 12, Platforms: 3}); got != want {
				t.Errorf("Summary #%d = %+v, want %+v", i+1, got, want)
			}
		}
		if n := op.count(); n != 1 {
			t.Errorf("open called %d times, want 1 (store kept open after success)", n)
		}

		if err := g.Close(); err != nil {
			t.Errorf("Close err = %v", err)
		}
		if n := fs.closeCount(); n != 1 {
			t.Errorf("store closed %d times by gate.Close, want 1", n)
		}
	})

	t.Run("DSK-007/MarkConnected sozinho não abre o banco", func(t *testing.T) {
		t.Parallel()
		op := &fakeOpener{results: []openResult{{st: &fakeStore{}}}}
		g := newLibraryGate(op.open)
		t.Cleanup(func() { _ = g.Close() })

		g.MarkConnected()
		g.MarkConnected() // idempotent

		if n := op.count(); n != 0 {
			t.Errorf("open called %d times by MarkConnected, want 0 (lazy open on Summary)", n)
		}
	})

	mismatch := []struct {
		name string
		err  error
	}{
		{"DSK-008/ErrSchemaMismatch retorna ErrAgentNotReady", store.ErrSchemaMismatch},
		{"DSK-008/ErrSchemaMismatch embrulhado retorna ErrAgentNotReady",
			fmt.Errorf("store: %q is at version 0, expected 1: %w", "library.db", store.ErrSchemaMismatch)},
	}
	for _, tc := range mismatch {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			op := &fakeOpener{results: []openResult{{err: tc.err}}}
			g := newLibraryGate(op.open)
			t.Cleanup(func() { _ = g.Close() })
			g.MarkConnected()

			_, err := g.Summary(ctx)

			assertNotReady(t, err)
			if n := op.count(); n != 1 {
				t.Errorf("open called %d times, want 1", n)
			}
		})
	}

	t.Run("DSK-008/próxima chamada tenta abrir de novo", func(t *testing.T) {
		t.Parallel()
		fs := &fakeStore{summary: summaryOf(1, 1)}
		op := &fakeOpener{results: []openResult{
			{err: store.ErrSchemaMismatch},
			{err: fmt.Errorf("open: %w", store.ErrSchemaMismatch)},
			{st: fs},
		}}
		g := newLibraryGate(op.open)
		t.Cleanup(func() { _ = g.Close() })
		g.MarkConnected()

		_, err := g.Summary(ctx)
		assertNotReady(t, err)
		_, err = g.Summary(ctx)
		assertNotReady(t, err)
		got, err := g.Summary(ctx)
		if err != nil {
			t.Fatalf("third Summary err = %v, want success after schema is ready", err)
		}
		if want := (LibrarySummaryView{Games: 1, Platforms: 1}); got != want {
			t.Errorf("third Summary = %+v, want %+v", got, want)
		}
		if n := op.count(); n != 3 {
			t.Errorf("open called %d times, want 3 (no cached failure)", n)
		}
	})

	t.Run("DSK-008/outro erro de open é repassado e tentado de novo", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("disk on fire")
		op := &fakeOpener{results: []openResult{{err: boom}}}
		g := newLibraryGate(op.open)
		t.Cleanup(func() { _ = g.Close() })
		g.MarkConnected()

		_, err := g.Summary(ctx)
		if !errors.Is(err, boom) {
			t.Errorf("Summary err = %v, want errors.Is(err, open error)", err)
		}
		if errors.Is(err, ErrAgentNotReady) {
			t.Errorf("Summary err = %v, must not be ErrAgentNotReady for a non-schema error", err)
		}
		_, _ = g.Summary(ctx)
		if n := op.count(); n != 2 {
			t.Errorf("open called %d times, want 2 (no cached failure)", n)
		}
	})

	t.Run("DSK-007/erro do store no resumo é repassado", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("query failed")
		op := &fakeOpener{results: []openResult{{st: &fakeStore{err: boom}}}}
		g := newLibraryGate(op.open)
		t.Cleanup(func() { _ = g.Close() })
		g.MarkConnected()

		_, err := g.Summary(ctx)
		if !errors.Is(err, boom) {
			t.Errorf("Summary err = %v, want errors.Is(err, store error)", err)
		}
	})

	t.Run("DSK-007/Close sem banco aberto não falha e é idempotente", func(t *testing.T) {
		t.Parallel()
		op := &fakeOpener{}
		g := newLibraryGate(op.open)
		if err := g.Close(); err != nil {
			t.Errorf("first Close err = %v", err)
		}
		if err := g.Close(); err != nil {
			t.Errorf("second Close err = %v", err)
		}
		if n := op.count(); n != 0 {
			t.Errorf("open called %d times by Close, want 0", n)
		}
	})

	t.Run("DSK-007/chamadas concorrentes abrem o banco uma vez", func(t *testing.T) {
		t.Parallel()
		fs := &fakeStore{summary: summaryOf(5, 4)}
		op := &fakeOpener{results: []openResult{{st: fs}}}
		g := newLibraryGate(op.open)
		t.Cleanup(func() { _ = g.Close() })
		g.MarkConnected()

		const n = 16
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := g.Summary(ctx)
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Errorf("concurrent Summary err = %v", err)
			}
		}
		if c := op.count(); c != 1 {
			t.Errorf("open called %d times by concurrent Summary, want 1", c)
		}
	})

	t.Run("DSK-008/banco real inexistente sem migração vira ErrAgentNotReady e depois abre", func(t *testing.T) {
		t.Parallel()
		db := filepath.Join(t.TempDir(), "library.db")
		open := func(ctx context.Context) (libraryStore, error) {
			st, err := store.Open(ctx, db, store.OpenOptions{Migrate: false})
			if err != nil {
				return nil, err
			}
			return st, nil
		}
		g := newLibraryGate(open)
		t.Cleanup(func() { _ = g.Close() })
		g.MarkConnected()

		_, err := g.Summary(ctx)
		assertNotReady(t, err)

		// The agent creates and migrates the database.
		agentStore, err := store.Open(ctx, db, store.OpenOptions{Migrate: true})
		if err != nil {
			t.Fatalf("agent store.Open: %v", err)
		}
		if err := agentStore.Close(); err != nil {
			t.Fatalf("agent store.Close: %v", err)
		}

		got, err := g.Summary(ctx)
		if err != nil {
			t.Fatalf("Summary after migration err = %v", err)
		}
		if want := (LibrarySummaryView{}); got != want {
			t.Errorf("Summary = %+v, want %+v (empty library)", got, want)
		}
	})
}
