package store

import (
	"context"
	"fmt"
	"time"
)

// GameStat is the time of one game in a period (EST-001, EST-005). ImportedSeconds is the part of
// ActiveSeconds that came from LaunchBox imports.
type GameStat struct {
	GameID          int64
	Title           string
	Platform        string
	Sessions        int64
	ActiveSeconds   int64
	ImportedSeconds int64
	LastStartedAt   time.Time
}

// PlatformStat is the time of one platform in a period (EST-003).
type PlatformStat struct {
	PlatformID      int64
	Name            string
	ActiveSeconds   int64
	ImportedSeconds int64
}

// SessionTime is the start and the active time of one session, for the weekly totals (EST-002).
type SessionTime struct {
	StartedAt     time.Time
	ActiveSeconds int64
	Source        string
}

// sinceMs turns the start of a period into the ms the queries compare with; the zero time is all time.
func sinceMs(since time.Time) int64 {
	if since.IsZero() {
		return 0
	}
	return since.UnixMilli()
}

// StatsByGame returns the time of each game that has sessions since the start instant, longest first.
func (s *Store) StatsByGame(ctx context.Context, since time.Time) ([]GameStat, error) {
	rows, err := s.q.StatsByGame(ctx, sinceMs(since))
	if err != nil {
		return nil, fmt.Errorf("store: estatísticas por jogo: %w", err)
	}
	out := make([]GameStat, 0, len(rows))
	for _, r := range rows {
		out = append(out, GameStat{
			GameID: r.GameID, Title: r.Title, Platform: r.Platform, Sessions: r.Sessions,
			ActiveSeconds: r.ActiveSeconds, ImportedSeconds: r.ImportedSeconds,
			LastStartedAt: time.UnixMilli(r.LastStartedAt),
		})
	}
	return out, nil
}

// StatsByPlatform returns the time of each platform with time since the start instant, longest first.
func (s *Store) StatsByPlatform(ctx context.Context, since time.Time) ([]PlatformStat, error) {
	rows, err := s.q.StatsByPlatform(ctx, sinceMs(since))
	if err != nil {
		return nil, fmt.Errorf("store: estatísticas por plataforma: %w", err)
	}
	out := make([]PlatformStat, 0, len(rows))
	for _, r := range rows {
		out = append(out, PlatformStat{
			PlatformID: r.PlatformID, Name: r.Name, ActiveSeconds: r.ActiveSeconds, ImportedSeconds: r.ImportedSeconds,
		})
	}
	return out, nil
}

// SessionTimes returns every session that started since the start instant, oldest first.
func (s *Store) SessionTimes(ctx context.Context, since time.Time) ([]SessionTime, error) {
	rows, err := s.q.SessionTimesSince(ctx, sinceMs(since))
	if err != nil {
		return nil, fmt.Errorf("store: sessões do período: %w", err)
	}
	out := make([]SessionTime, 0, len(rows))
	for _, r := range rows {
		out = append(out, SessionTime{StartedAt: time.UnixMilli(r.StartedAt), ActiveSeconds: r.ActiveSeconds, Source: r.Source})
	}
	return out, nil
}
