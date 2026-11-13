package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// Job states of the media queue (MID-005). A failed job keeps its last error.
const (
	JobPending = "pending"
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
)

// ErrMediaJobNotFound is returned for a job id the queue does not hold.
var ErrMediaJobNotFound = errors.New("store: job de mídia não encontrado")

// MediaRow is one image kept for a game (MID-004).
type MediaRow struct {
	ID, GameID                      int64
	Kind, Provider, SourceURL, Hash string
	Path, ThumbPath                 string
	CreatedAt                       time.Time
}

// MediaJob is one download waiting in, or left by, the queue (MID-005).
type MediaJob struct {
	ID, GameID                int64
	Kind, Provider, SourceURL string
	State                     string
	Attempts                  int
	LastError                 string
	CreatedAt                 time.Time
}

// EnqueueMedia adds a download of a game's image to the queue as pending and returns its id.
func (s *Store) EnqueueMedia(ctx context.Context, gameID int64, kind, provider, sourceURL string, at time.Time) (int64, error) {
	var id int64
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		var err error
		id, err = q.InsertMediaJob(ctx, dbgen.InsertMediaJobParams{
			GameID: gameID, Kind: kind, Provider: provider, SourceUrl: sourceURL, CreatedAt: at.UnixMilli(),
		})
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("store: enfileirar mídia: %w", err)
	}
	return id, nil
}

// PendingMediaJobs returns the jobs waiting to run, in the order they were queued.
func (s *Store) PendingMediaJobs(ctx context.Context) ([]MediaJob, error) {
	rows, err := s.q.ListMediaJobsByState(ctx, JobPending)
	if err != nil {
		return nil, fmt.Errorf("store: listar jobs de mídia: %w", err)
	}
	out := make([]MediaJob, 0, len(rows))
	for _, r := range rows {
		out = append(out, mediaJobOf(r.ID, r.GameID, r.Kind, r.Provider, r.SourceUrl, r.State, r.Attempts, r.LastError, r.CreatedAt))
	}
	return out, nil
}

// ResetRunningMediaJobs puts the jobs left running by a stopped agent back to pending, so they run
// again after a restart (MID-005).
func (s *Store) ResetRunningMediaJobs(ctx context.Context) error {
	if _, err := s.q.ResetRunningMediaJobs(ctx); err != nil {
		return fmt.Errorf("store: retomar jobs de mídia: %w", err)
	}
	return nil
}

// SetMediaJobState records the state, the attempts and the last error of a job.
func (s *Store) SetMediaJobState(ctx context.Context, id int64, state string, attempts int, lastErr string) error {
	n, err := s.q.SetMediaJobState(ctx, dbgen.SetMediaJobStateParams{
		State: state, Attempts: int64(attempts), LastError: lastErr, ID: id,
	})
	if err != nil {
		return fmt.Errorf("store: atualizar job de mídia: %w", err)
	}
	if n == 0 {
		return ErrMediaJobNotFound
	}
	return nil
}

// CancelMediaJob stops a job that has not finished: it is marked failed with the reason
// "cancelado" and does not run again (MID-005). A finished job is left as it is.
func (s *Store) CancelMediaJob(ctx context.Context, id int64) error {
	r, err := s.q.GetMediaJob(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMediaJobNotFound
	}
	if err != nil {
		return fmt.Errorf("store: ler job de mídia: %w", err)
	}
	if r.State != JobPending && r.State != JobRunning {
		return nil
	}
	return s.SetMediaJobState(ctx, id, JobFailed, int(r.Attempts), "cancelado")
}

// AddMedia records an image of a game. The same hash is kept once per game: it reports false and
// writes nothing when the game already has that image (MID-004).
func (s *Store) AddMedia(ctx context.Context, m MediaRow) (bool, error) {
	n, err := s.q.InsertMediaIgnore(ctx, dbgen.InsertMediaIgnoreParams{
		GameID: m.GameID, Kind: m.Kind, Provider: m.Provider, SourceUrl: m.SourceURL, Hash: m.Hash,
		Path: m.Path, ThumbPath: m.ThumbPath, CreatedAt: m.CreatedAt.UnixMilli(),
	})
	if err != nil {
		return false, fmt.Errorf("store: gravar mídia: %w", err)
	}
	return n == 1, nil
}

// ListMedia returns the images of a game, ordered by kind.
func (s *Store) ListMedia(ctx context.Context, gameID int64) ([]MediaRow, error) {
	rows, err := s.q.ListMediaForGame(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("store: listar mídia: %w", err)
	}
	out := make([]MediaRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, MediaRow{
			ID: r.ID, GameID: r.GameID, Kind: r.Kind, Provider: r.Provider, SourceURL: r.SourceUrl,
			Hash: r.Hash, Path: r.Path, ThumbPath: r.ThumbPath, CreatedAt: time.UnixMilli(r.CreatedAt),
		})
	}
	return out, nil
}

func mediaJobOf(id, gameID int64, kind, provider, sourceURL, state string, attempts int64, lastErr string, createdAt int64) MediaJob {
	return MediaJob{
		ID: id, GameID: gameID, Kind: kind, Provider: provider, SourceURL: sourceURL,
		State: state, Attempts: int(attempts), LastError: lastErr, CreatedAt: time.UnixMilli(createdAt),
	}
}
