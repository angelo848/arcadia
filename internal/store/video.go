package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// Video jobs ride the media queue: the job carries no title or credential, only this pseudo-URL (VID-003).
const (
	VideoProvider  = "screenscraper"
	VideoLookupURL = "screenscraper:lookup"
)

const videoKind = "video"

// VideoGameInfo is what the ScreenScraper lookup needs about a game (VID-002).
type VideoGameInfo struct {
	Title   string
	Year    int
	SeedKey string // "" when the platform has none
}

// VideoCandidate is a game seen by the video batch. HasVideo is true when the game already has a
// video row or an active (pending/running) video job.
type VideoCandidate struct {
	GameID   int64
	SeedKey  string
	HasVideo bool
}

// VideoCounts counts the video jobs by state; Failed leaves out the cancelled ones (VID-008).
type VideoCounts struct{ Pending, Running, Failed int }

func hasVideoOf(v any) bool {
	switch x := v.(type) {
	case int64:
		return x != 0
	case bool:
		return x
	}
	return false
}

// VideoGameInfo returns the data the video lookup of a game needs; ErrGameNotFound when it is unknown.
func (s *Store) VideoGameInfo(ctx context.Context, gameID int64) (VideoGameInfo, error) {
	r, err := s.q.VideoGameInfoRow(ctx, gameID)
	if errors.Is(err, sql.ErrNoRows) {
		return VideoGameInfo{}, ErrGameNotFound
	}
	if err != nil {
		return VideoGameInfo{}, fmt.Errorf("store: ler jogo para vídeo: %w", err)
	}
	info := VideoGameInfo{Title: r.Title, SeedKey: r.SeedKey.String}
	if len(r.ReleaseDate.String) >= 4 {
		if y, err := strconv.Atoi(r.ReleaseDate.String[:4]); err == nil {
			info.Year = y
		}
	}
	return info, nil
}

// VideoCandidates returns the candidates among the given ids; unknown ids are absent.
func (s *Store) VideoCandidates(ctx context.Context, gameIDs []int64) ([]VideoCandidate, error) {
	out := make([]VideoCandidate, 0, len(gameIDs))
	for _, id := range gameIDs {
		r, err := s.q.VideoCandidateByGame(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("store: candidatos de vídeo: %w", err)
		}
		out = append(out, VideoCandidate{GameID: r.ID, SeedKey: r.SeedKey.String, HasVideo: hasVideoOf(r.HasVideo)})
	}
	return out, nil
}

// VideoCandidatesIn returns the candidates of a platform, or of the whole library when platformID is 0.
func (s *Store) VideoCandidatesIn(ctx context.Context, platformID int64) ([]VideoCandidate, error) {
	var out []VideoCandidate
	if platformID == 0 {
		rows, err := s.q.VideoCandidatesAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("store: candidatos de vídeo: %w", err)
		}
		for _, r := range rows {
			out = append(out, VideoCandidate{GameID: r.ID, SeedKey: r.SeedKey.String, HasVideo: hasVideoOf(r.HasVideo)})
		}
		return out, nil
	}
	if _, err := s.q.GetPlatformWithCount(ctx, platformID); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPlatformNotFound
	} else if err != nil {
		return nil, fmt.Errorf("store: ler plataforma: %w", err)
	}
	rows, err := s.q.VideoCandidatesByPlatform(ctx, platformID)
	if err != nil {
		return nil, fmt.Errorf("store: candidatos de vídeo: %w", err)
	}
	for _, r := range rows {
		out = append(out, VideoCandidate{GameID: r.ID, SeedKey: r.SeedKey.String, HasVideo: hasVideoOf(r.HasVideo)})
	}
	return out, nil
}

// EnqueueVideoJobs queues one pending video job per game that still has no video and no active video
// job, all in one transaction (an unknown id rolls everything back). It returns how many it queued.
func (s *Store) EnqueueVideoJobs(ctx context.Context, gameIDs []int64, at time.Time) (int, error) {
	n := 0
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		n = 0
		for _, id := range gameIDs {
			r, err := q.VideoCandidateByGame(ctx, id)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("conferir jogo %d: %w", id, err)
			}
			if err == nil && hasVideoOf(r.HasVideo) {
				continue
			}
			if _, err := q.InsertMediaJob(ctx, dbgen.InsertMediaJobParams{
				GameID: id, Kind: videoKind, Provider: VideoProvider, SourceUrl: VideoLookupURL, CreatedAt: at.UnixMilli(),
			}); err != nil {
				return fmt.Errorf("enfileirar vídeo do jogo %d: %w", id, err)
			}
			n++
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("store: enfileirar vídeos: %w", err)
	}
	return n, nil
}

// CancelVideoJobs marks every pending and running video job failed with "cancelado". It returns the
// number of pending jobs cancelled and the ids of the jobs that were running; image jobs are untouched.
func (s *Store) CancelVideoJobs(ctx context.Context) (int, []int64, error) {
	var pending int
	var running []int64
	err := s.withTx(ctx, func(_ *sql.Tx, q *dbgen.Queries) error {
		ids, err := q.ListRunningVideoJobIDs(ctx)
		if err != nil {
			return err
		}
		running = ids
		n, err := q.CancelPendingVideoJobs(ctx)
		if err != nil {
			return err
		}
		pending = int(n)
		_, err = q.CancelRunningVideoJobs(ctx)
		return err
	})
	if err != nil {
		return 0, nil, fmt.Errorf("store: cancelar vídeos: %w", err)
	}
	return pending, running, nil
}

// VideoJobCounts counts the video jobs pending, running and failed (cancelled ones are not failures).
func (s *Store) VideoJobCounts(ctx context.Context) (VideoCounts, error) {
	r, err := s.q.VideoJobCounts(ctx)
	if err != nil {
		return VideoCounts{}, fmt.Errorf("store: contar vídeos: %w", err)
	}
	return VideoCounts{Pending: int(r.Pending), Running: int(r.Running), Failed: int(r.Failed)}, nil
}
