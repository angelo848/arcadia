package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// TimeToBeatRow is the cached time to beat of one game (TTB-002). Values are seconds; nil = no data.
type TimeToBeatRow struct {
	GameID                            int64
	Source                            string // "hltb" | "igdb"
	State                             string // "found" | "doubt" | "not-found"
	ExternalID, ExternalTitle         string
	MainS, MainExtraS, CompletionistS *int64
	TitleAtFetch, Term                string
	CandidatesJSON                    string // "[]" outside doubt
	Confirmed                         bool
	FetchedAt, ExpiresAt              time.Time // stored as Unix ms UTC
}

func nullableSeconds(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func secondsPtr(v any) *int64 {
	if n, ok := v.(int64); ok {
		return &n
	}
	return nil
}

// GetTimeToBeat returns the row of a game; ok is false when there is none.
func (s *Store) GetTimeToBeat(ctx context.Context, gameID int64) (TimeToBeatRow, bool, error) {
	r, err := s.q.GetTimeToBeat(ctx, gameID)
	if errors.Is(err, sql.ErrNoRows) {
		return TimeToBeatRow{}, false, nil
	}
	if err != nil {
		return TimeToBeatRow{}, false, fmt.Errorf("store: ler tempo para zerar: %w", err)
	}
	return TimeToBeatRow{
		GameID: r.GameID, Source: r.Source, State: r.State,
		ExternalID: r.ExternalID, ExternalTitle: r.ExternalTitle,
		MainS: secondsPtr(r.MainS), MainExtraS: secondsPtr(r.MainExtraS), CompletionistS: secondsPtr(r.CompletionistS),
		TitleAtFetch: r.TitleAtFetch, Term: r.Term, CandidatesJSON: r.Candidates,
		Confirmed: r.Confirmed != 0,
		FetchedAt: time.UnixMilli(r.FetchedAt).UTC(), ExpiresAt: time.UnixMilli(r.ExpiresAt).UTC(),
	}, true, nil
}

// UpsertTimeToBeat inserts or replaces the row of a game.
func (s *Store) UpsertTimeToBeat(ctx context.Context, row TimeToBeatRow) error {
	var confirmed int64
	if row.Confirmed {
		confirmed = 1
	}
	cands := row.CandidatesJSON
	if cands == "" {
		cands = "[]"
	}
	err := s.q.UpsertTimeToBeat(ctx, dbgen.UpsertTimeToBeatParams{
		GameID: row.GameID, Source: row.Source, State: row.State,
		ExternalID: row.ExternalID, ExternalTitle: row.ExternalTitle,
		MainS: nullableSeconds(row.MainS), MainExtraS: nullableSeconds(row.MainExtraS),
		CompletionistS: nullableSeconds(row.CompletionistS),
		TitleAtFetch:   row.TitleAtFetch, Term: row.Term, Candidates: cands, Confirmed: confirmed,
		FetchedAt: row.FetchedAt.UnixMilli(), ExpiresAt: row.ExpiresAt.UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf("store: gravar tempo para zerar: %w", err)
	}
	return nil
}
