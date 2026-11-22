package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// ttbOutcome is what one lookup produced: a row to write, nothing, or an abort by cancellation.
type ttbOutcome struct {
	row     store.TimeToBeatRow
	write   bool
	aborted bool
}

// ttbIGDBMatch is the IGDB game identified by the reserve, with its values.
type ttbIGDBMatch struct {
	id, title string
	vals      scrapers.IGDBTimeToBeat
}

// cancelled reports whether err comes from the end of the agent or from a cancelled caller, which
// is neither a failure of the source nor worth a log [TTB-011].
func (s *timeToBeatService) cancelled(err error) bool {
	return s.ctx.Err() != nil || errors.Is(err, context.Canceled)
}

func (s *timeToBeatService) inBackoff(source string) bool {
	now := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Before(s.backoff[source].pausedUntil)
}

func ttbCause(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "prazo vencido"
	case errors.Is(err, scrapers.ErrUnavailable):
		return "indisponível"
	}
	return "erro"
}

// recordFailure counts one failure of the source, opens its recuo and warns once per sequence. The
// log carries only a coarse cause: never a body, a URL or a key.
func (s *timeToBeatService) recordFailure(source string, err error) {
	now := s.clock.Now()
	s.mu.Lock()
	b := s.backoff[source]
	b.failures++
	wait := domain.TTBBackoff(b.failures)
	b.pausedUntil = now.Add(wait)
	first := b.failures == 1
	s.mu.Unlock()
	msg := "HowLongToBeat indisponível"
	if source == ttbSourceIGDB {
		msg = "IGDB indisponível para o tempo para zerar"
	}
	level := slog.LevelDebug
	if first {
		level = slog.LevelWarn
	}
	s.log.Log(context.Background(), level, msg, "cause", ttbCause(err), "retry_in", wait.String())
}

func (s *timeToBeatService) recordSuccess(source string) {
	s.mu.Lock()
	*s.backoff[source] = ttbBackoff{}
	s.mu.Unlock()
}

// lookup runs the source table of hltb-rules §10.1 for one game. explicit (Refresh) ignores the
// recuo of both sources. The caller holds reqMu.
func (s *timeToBeatService) lookup(ctx context.Context, g store.Game, prev store.TimeToBeatRow, hasPrev, explicit bool) ttbOutcome {
	if hasPrev && prev.Confirmed {
		if prev.State == string(domain.TTBFound) {
			return s.reread(ctx, prev, explicit)
		}
		if !explicit {
			return ttbOutcome{} // a confirmed "none of these" is never re-read alone
		}
	}
	term := domain.SearchTerm(g.Title)
	base := store.TimeToBeatRow{GameID: g.ID, TitleAtFetch: g.Title, Term: term, CandidatesJSON: "[]"}
	if term == "" {
		return s.finish(base, domain.HLTBEmptyTerm, domain.IGDBNotAsked, nil, nil, nil)
	}
	year, _ := domain.ReleaseYear(g.ReleaseDate)

	h := domain.HLTBFailed
	var results []scrapers.HLTBGame
	var ranked []domain.TTBRanked
	if explicit || !s.inBackoff(ttbSourceHLTB) {
		rs, err := s.searchHLTB(ctx, term)
		switch {
		case err == nil:
			results = rs
			var state domain.TTBState
			state, ranked = domain.RankTimeToBeat(g.Title, year, toTTBResults(rs))
			switch state {
			case domain.TTBFound:
				h = domain.HLTBFound
			case domain.TTBDoubt:
				h = domain.HLTBDoubt
			default:
				h = domain.HLTBNotFound
			}
		case s.cancelled(err):
			return ttbOutcome{aborted: true}
		}
	}

	i := domain.IGDBNotAsked
	var match *ttbIGDBMatch
	if h == domain.HLTBNotFound || h == domain.HLTBFailed {
		var aborted bool
		i, match, aborted = s.igdbReserve(ctx, g, year, explicit)
		if aborted {
			return ttbOutcome{aborted: true}
		}
	}
	return s.finish(base, h, i, results, ranked, match)
}

// searchHLTB runs the search and, with no result and a ": " in the term, a second one with the text
// before it. Errors are already accounted for (failure counted) unless they are a cancellation.
func (s *timeToBeatService) searchHLTB(ctx context.Context, term string) ([]scrapers.HLTBGame, error) {
	rs, err := s.client.Search(ctx, term)
	if err == nil && len(rs) == 0 {
		if head, _, ok := strings.Cut(term, ": "); ok && strings.TrimSpace(head) != "" {
			rs, err = s.client.Search(ctx, strings.TrimSpace(head))
		}
	}
	if err != nil {
		if !s.cancelled(err) {
			s.recordFailure(ttbSourceHLTB, err)
		}
		return nil, err
	}
	s.recordSuccess(ttbSourceHLTB)
	return rs, nil
}

// reread renews the values of a confirmed row by its confirmed id, with no fallback [TTB-006].
func (s *timeToBeatService) reread(ctx context.Context, prev store.TimeToBeatRow, explicit bool) ttbOutcome {
	if !explicit && s.inBackoff(ttbSourceHLTB) {
		return ttbOutcome{}
	}
	term := prev.ExternalTitle
	if strings.TrimSpace(term) == "" {
		term = prev.Term
	}
	rs, err := s.client.Search(ctx, term)
	if err != nil {
		if s.cancelled(err) {
			return ttbOutcome{aborted: true}
		}
		s.recordFailure(ttbSourceHLTB, err)
		return ttbOutcome{}
	}
	s.recordSuccess(ttbSourceHLTB)
	row := prev
	now := s.clock.Now()
	row.FetchedAt = now
	row.ExpiresAt = domain.TTBExpiresAt(prev.Source, domain.TTBFound, now)
	for _, r := range rs {
		if strconv.FormatInt(r.ID, 10) == prev.ExternalID {
			row.ExternalTitle = r.Name
			row.MainS, row.MainExtraS, row.CompletionistS = secondsPtr(r.Main), secondsPtr(r.MainExtra), secondsPtr(r.Completionist)
			break
		}
	}
	return ttbOutcome{row: row, write: true}
}

// igdbReserve identifies the game on the IGDB and reads its time to beat [TTB-035..TTB-038].
func (s *timeToBeatService) igdbReserve(ctx context.Context, g store.Game, year int, explicit bool) (domain.IGDBOutcome, *ttbIGDBMatch, bool) {
	if !explicit && s.inBackoff(ttbSourceIGDB) {
		return domain.IGDBUnavailable, nil, false
	}
	// A deadline already spent on the HowLongToBeat must not starve the reserve.
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		c, cancel := context.WithTimeout(s.ctx, ttbIGDBTimeout)
		defer cancel()
		ctx = c
	}
	var seed, category string
	if p, err := s.st.PlatformByID(ctx, g.PlatformID); err == nil {
		seed, category = p.SeedKey, string(p.Category)
	}
	sctx, cancel := context.WithTimeout(ctx, ttbIGDBTimeout)
	cands, err := s.igdb.Search(sctx, scrapers.Query{Title: g.Title, Platform: g.Platform, Year: year, SeedKey: seed, Category: category})
	cancel()
	if out, done, aborted := s.igdbError(err); done {
		return out, nil, aborted
	}
	s.recordSuccess(ttbSourceIGDB)

	type scored struct {
		c  scrapers.Candidate
		sc domain.Scored
	}
	list := make([]scored, 0, len(cands))
	for _, c := range cands {
		m := domain.Member{Provider: "igdb", Title: c.Title, Year: c.Year, PlatformKeys: c.PlatformKeys, PlatformConfirmed: c.PlatformConfirmed}
		list = append(list, scored{c, domain.ScoreGroup(domain.Query{Title: g.Title, Year: year, LibraryKey: seed}, []domain.Member{m})})
	}
	sort.SliceStable(list, func(a, b int) bool { return list[a].sc.Score > list[b].sc.Score })
	if len(list) == 0 {
		return domain.IGDBNoMatch, nil, false
	}
	var second *domain.Scored
	if len(list) > 1 {
		second = &list[1].sc
	}
	if domain.ClassifyMatch(&list[0].sc, second) != domain.ConfidenceMatch {
		return domain.IGDBNoMatch, nil, false
	}
	best := list[0].c

	tctx, tcancel := context.WithTimeout(ctx, ttbIGDBTimeout)
	vals, found, err := s.igdb.TimeToBeat(tctx, best.ProviderID)
	tcancel()
	if out, done, aborted := s.igdbError(err); done {
		return out, nil, aborted
	}
	s.recordSuccess(ttbSourceIGDB)
	if !found || (vals.Hastily <= 0 && vals.Normally <= 0 && vals.Completely <= 0) {
		return domain.IGDBNoMatch, nil, false
	}
	return domain.IGDBValues, &ttbIGDBMatch{id: best.ProviderID, title: best.Title, vals: vals}, false
}

// igdbError maps an IGDB error to an outcome; done is false when err is nil.
func (s *timeToBeatService) igdbError(err error) (out domain.IGDBOutcome, done, aborted bool) {
	switch {
	case err == nil:
		return 0, false, false
	case errors.Is(err, scrapers.ErrNoKey):
		s.log.Debug("tempo para zerar: IGDB sem chave, reserva ignorada")
		return domain.IGDBNoKey, true, false
	case s.cancelled(err):
		return 0, true, true
	}
	s.recordFailure(ttbSourceIGDB, err)
	return domain.IGDBUnavailable, true, false
}

// finish applies the source table and builds the row to write.
func (s *timeToBeatService) finish(base store.TimeToBeatRow, h domain.HLTBOutcome, i domain.IGDBOutcome,
	results []scrapers.HLTBGame, ranked []domain.TTBRanked, match *ttbIGDBMatch) ttbOutcome {
	write, source, state := domain.TTBResolve(h, i)
	if !write {
		return ttbOutcome{}
	}
	now := s.clock.Now()
	row := base
	row.Source, row.State = source, string(state)
	row.FetchedAt = now
	row.ExpiresAt = domain.TTBExpiresAt(source, state, now)
	switch {
	case source == ttbSourceIGDB && match != nil:
		row.ExternalID, row.ExternalTitle = match.id, match.title
		row.MainS, row.MainExtraS, row.CompletionistS = secondsPtr(match.vals.Hastily), secondsPtr(match.vals.Normally), secondsPtr(match.vals.Completely)
	case state == domain.TTBFound && len(ranked) > 0:
		r := results[ranked[0].Index]
		row.ExternalID, row.ExternalTitle = strconv.FormatInt(r.ID, 10), r.Name
		row.MainS, row.MainExtraS, row.CompletionistS = secondsPtr(r.Main), secondsPtr(r.MainExtra), secondsPtr(r.Completionist)
	case state == domain.TTBDoubt:
		cands := make([]ipc.TimeToBeatCandidate, 0, ttbMaxCandidates)
		for _, r := range ranked {
			if len(cands) == ttbMaxCandidates || r.Scored.Score < domain.MatchAlternative {
				break
			}
			cands = append(cands, ttbCandidate(results[r.Index], r.Scored))
		}
		if raw, err := json.Marshal(cands); err == nil {
			row.CandidatesJSON = string(raw)
		}
	}
	return ttbOutcome{row: row, write: true}
}
