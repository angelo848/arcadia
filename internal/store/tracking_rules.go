package store

import (
	"context"
	"fmt"
)

// TrackingRule is one rule that adopts a game's processes (RUL-001, RUL-002).
type TrackingRule struct {
	Kind  string // install_dir, exe_path or exe_name
	Value string
}

// TrackingRules returns the tracking rules of a game, in the order they were
// written. A game without rules returns an empty list.
func (s *Store) TrackingRules(ctx context.Context, gameID int64) ([]TrackingRule, error) {
	rows, err := s.q.ListTrackingRules(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("store: tracking rules of game %d: %w", gameID, err)
	}
	out := make([]TrackingRule, 0, len(rows))
	for _, r := range rows {
		out = append(out, TrackingRule{Kind: r.Kind, Value: r.Value})
	}
	return out, nil
}
