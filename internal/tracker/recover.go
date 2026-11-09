package tracker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// RecoveryStore is the store subset used by RecoverOpenSessions; *store.Store
// implements it.
type RecoveryStore interface {
	OpenSessions(ctx context.Context) ([]store.OpenSession, error)
	EndSession(ctx context.Context, e store.SessionEnd) (domain.SessionState, error)
}

// RecoverOpenSessions closes every session left open by a crash with
// ended_at = last_seen_at, the persisted active time and reason recovered
// [SES-010]. A failure on one session is logged and the others proceed; the
// count is the number of sessions closed. Only an OpenSessions failure is
// returned.
func RecoverOpenSessions(ctx context.Context, st RecoveryStore, clock domain.Clock, log *slog.Logger) (int, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	open, err := st.OpenSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("recover open sessions: %w", err)
	}
	now := clock.Now()
	n := 0
	for _, s := range open {
		_, err := st.EndSession(ctx, store.SessionEnd{
			ID:        s.ID,
			EndedAt:   s.LastSeenAt,
			Active:    time.Duration(s.ActiveSeconds) * time.Second,
			Reason:    domain.EndRecovered,
			WrittenAt: now,
		})
		if err != nil {
			log.Warn("falha ao recuperar a sessão aberta", slog.Int64("session_id", s.ID), slog.Any("error", err))
			continue
		}
		n++
	}
	if n > 0 {
		log.Warn("sessões abertas recuperadas", slog.Int("count", n))
	}
	return n, nil
}
