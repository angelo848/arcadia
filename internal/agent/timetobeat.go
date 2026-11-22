package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Limits and setting keys of the time-to-beat service (hltb-rules §1).
const (
	ttbSyncTimeout    = 25 * time.Second // Refresh and Search [TTB-020, TTB-021]
	ttbIGDBTimeout    = 10 * time.Second // one request of the IGDB reserve [TTB-038]
	ttbBatchMax       = 1000             // batch queue cap [TTB-009]
	ttbMaxCandidates  = 10
	ttbTermMax        = 200
	ttbCandidateGames = 64 // games whose last search is kept in memory

	ttbSettingEnabled       = "timeToBeat.enabled"
	ttbSettingAfterMetadata = "timeToBeat.afterMetadata"

	ttbSourceHLTB = domain.TTBSourceHLTB
	ttbSourceIGDB = domain.TTBSourceIGDB
)

// ttbSearcher is the HowLongToBeat client (fake in tests) [TTB-031].
type ttbSearcher interface {
	Search(ctx context.Context, term string) ([]scrapers.HLTBGame, error)
}

// ttbFallback is the IGDB side of the reserve (fake in tests) [TTB-033..TTB-038].
type ttbFallback interface {
	Search(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error)
	TimeToBeat(ctx context.Context, igdbID string) (scrapers.IGDBTimeToBeat, bool, error)
}

// ttbNotifier tells the clients that the time to beat of a game changed.
type ttbNotifier interface{ TimeToBeatChanged(gameID int64) }

// ttbBackoff is the failure ladder of one source [TTB-011, TTB-038].
type ttbBackoff struct {
	failures    int
	pausedUntil time.Time
}

// timeToBeatService looks the time to beat up and caches it (phase 15). One request at a time
// reaches the sources (reqMu), whoever asks.
type timeToBeatService struct {
	st     *store.Store
	client ttbSearcher
	igdb   ttbFallback
	clock  domain.Clock
	log    *slog.Logger
	note   ttbNotifier

	// syncTimeout is the deadline of Refresh and Search (tests shorten it).
	syncTimeout time.Duration
	// igdbHasKey tells whether the IGDB key is registered; it only feeds "unavailable" and the worker's
	// wait. nil means "yes".
	igdbHasKey func() bool

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once
	wake      chan struct{}
	reqMu     sync.Mutex

	mu          sync.Mutex
	interactive []int64
	batch       []int64
	queued      map[int64]bool
	inflight    int64
	dropped     map[int64]bool
	backoff     map[string]*ttbBackoff
	candidates  map[int64][]ipc.TimeToBeatCandidate
	order       []int64
	stopped     bool
}

func newTimeToBeatService(st *store.Store, client ttbSearcher, igdb ttbFallback, clock domain.Clock, log *slog.Logger, note ttbNotifier) *timeToBeatService {
	ctx, cancel := context.WithCancel(context.Background())
	return &timeToBeatService{
		st: st, client: client, igdb: igdb, clock: clock, log: log, note: note,
		syncTimeout: ttbSyncTimeout,
		ctx:         ctx, cancel: cancel,
		wake:       make(chan struct{}, 1),
		queued:     map[int64]bool{},
		dropped:    map[int64]bool{},
		backoff:    map[string]*ttbBackoff{ttbSourceHLTB: {}, ttbSourceIGDB: {}},
		candidates: map[int64][]ipc.TimeToBeatCandidate{},
	}
}

// Start launches the worker (once).
func (s *timeToBeatService) Start() {
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.run()
	})
}

// BeginShutdown stops the notifications and drops the queues [TTB-028].
func (s *timeToBeatService) BeginShutdown() {
	s.mu.Lock()
	s.stopped = true
	s.clearQueuesLocked()
	s.mu.Unlock()
}

// Close cancels the service context and waits for the worker; it is idempotent.
func (s *timeToBeatService) Close() {
	s.cancel()
	s.wg.Wait()
}

func (s *timeToBeatService) clearQueuesLocked() {
	s.interactive, s.batch = nil, nil
	s.queued = map[int64]bool{}
}

func (s *timeToBeatService) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *timeToBeatService) hasKey() bool { return s.igdbHasKey == nil || s.igdbHasKey() }

// notify sends timeToBeat.changed unless the agent is shutting down.
func (s *timeToBeatService) notify(gameID int64) {
	s.mu.Lock()
	stopped := s.stopped
	s.mu.Unlock()
	if stopped || s.note == nil {
		return
	}
	s.note.TimeToBeatChanged(gameID)
}

// ---- worker ----

func (s *timeToBeatService) run() {
	defer s.wg.Done()
	for {
		if s.ctx.Err() != nil {
			return
		}
		id, wait, ok := s.next()
		if ok {
			s.process(id)
			continue
		}
		var timer <-chan time.Time
		if wait > 0 {
			timer = s.clock.After(wait)
		}
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		case <-timer:
		}
	}
}

// next pops the next game (interactive before batch). Without one it returns the wait until a
// source comes back (0 = until a wake-up).
func (s *timeToBeatService) next() (id int64, wait time.Duration, ok bool) {
	enabled, err := s.boolSetting(s.ctx, ttbSettingEnabled)
	hasKey := s.hasKey()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || err != nil {
		return 0, 0, false
	}
	if !enabled {
		s.clearQueuesLocked()
		return 0, 0, false
	}
	if len(s.interactive)+len(s.batch) == 0 {
		return 0, 0, false
	}
	now := s.clock.Now()
	hl := s.backoff[ttbSourceHLTB]
	ig := s.backoff[ttbSourceIGDB]
	hlPaused := now.Before(hl.pausedUntil)
	igPaused := now.Before(ig.pausedUntil)
	if hlPaused && (igPaused || !hasKey) {
		wait = hl.pausedUntil.Sub(now)
		if igPaused && ig.pausedUntil.Before(hl.pausedUntil) {
			wait = ig.pausedUntil.Sub(now)
		}
		return 0, wait, false
	}
	if len(s.interactive) > 0 {
		id, s.interactive = s.interactive[0], s.interactive[1:]
	} else {
		id, s.batch = s.batch[0], s.batch[1:]
	}
	delete(s.queued, id)
	s.inflight = id
	return id, 0, true
}

// process runs one queued lookup and writes and notifies when it produced something.
func (s *timeToBeatService) process(id int64) {
	wrote := s.lookupQueued(id)
	s.mu.Lock()
	s.inflight = 0
	delete(s.dropped, id)
	s.mu.Unlock()
	if wrote {
		s.notify(id)
	}
}

func (s *timeToBeatService) lookupQueued(id int64) bool {
	g, err := s.st.Game(s.ctx, id)
	if err != nil {
		if !errors.Is(err, store.ErrGameNotFound) && s.ctx.Err() == nil {
			s.log.Debug("tempo para zerar: ler jogo", "game", id, "err", err)
		}
		return false
	}
	prev, has, err := s.st.GetTimeToBeat(s.ctx, id)
	if err != nil {
		s.log.Debug("tempo para zerar: ler linha", "game", id, "err", err)
		return false
	}
	s.reqMu.Lock()
	out := s.lookup(s.ctx, g, prev, has, false)
	s.reqMu.Unlock()
	if !out.write || out.aborted {
		return false
	}
	s.mu.Lock()
	dropped := s.dropped[id]
	s.mu.Unlock()
	if dropped || s.ctx.Err() != nil {
		return false
	}
	if err := s.st.UpsertTimeToBeat(s.ctx, out.row); err != nil {
		s.log.Debug("tempo para zerar: gravar", "game", id, "err", err)
		return false
	}
	return true
}

// enqueue queues a game; interactive requests go before the batch and promote a queued batch game.
func (s *timeToBeatService) enqueue(id int64, interactive bool) {
	s.mu.Lock()
	if s.stopped || s.inflight == id {
		s.mu.Unlock()
		return
	}
	if s.queued[id] {
		if interactive {
			if i := slices.Index(s.batch, id); i >= 0 {
				s.batch = slices.Delete(s.batch, i, i+1)
				s.interactive = append(s.interactive, id)
			}
		}
		s.mu.Unlock()
		return
	}
	if interactive {
		s.interactive = append(s.interactive, id)
	} else {
		if len(s.batch) >= ttbBatchMax {
			s.mu.Unlock()
			s.log.Debug("tempo para zerar: fila do lote cheia, jogo descartado", "game", id)
			return
		}
		s.batch = append(s.batch, id)
	}
	s.queued[id] = true
	s.mu.Unlock()
	s.signal()
}

// AfterMetadata queues a batch lookup after a metadata apply [TTB-025].
func (s *timeToBeatService) AfterMetadata(gameID int64) {
	enabled, err := s.boolSetting(s.ctx, ttbSettingEnabled)
	if err != nil || !enabled {
		s.log.Debug("tempo para zerar: desligado, nada enfileirado", "game", gameID)
		return
	}
	after, err := s.boolSetting(s.ctx, ttbSettingAfterMetadata)
	if err != nil || !after {
		s.log.Debug("tempo para zerar: depois dos metadados desligado", "game", gameID)
		return
	}
	if row, has, err := s.st.GetTimeToBeat(s.ctx, gameID); err == nil && has && row.Confirmed {
		s.log.Debug("tempo para zerar: linha confirmada, nada enfileirado", "game", gameID)
		return
	}
	s.enqueue(gameID, false)
}

// GameRemoved takes a game out of the queues and of the kept candidates [TTB-026].
func (s *timeToBeatService) GameRemoved(gameID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := slices.Index(s.interactive, gameID); i >= 0 {
		s.interactive = slices.Delete(s.interactive, i, i+1)
	}
	if i := slices.Index(s.batch, gameID); i >= 0 {
		s.batch = slices.Delete(s.batch, i, i+1)
	}
	delete(s.queued, gameID)
	delete(s.candidates, gameID)
	if i := slices.Index(s.order, gameID); i >= 0 {
		s.order = slices.Delete(s.order, i, i+1)
	}
	if s.inflight == gameID {
		s.dropped[gameID] = true
	}
}

// ---- settings ----

func (s *timeToBeatService) boolSetting(ctx context.Context, key string) (bool, error) {
	v, err := s.st.Setting(ctx, key)
	if errors.Is(err, store.ErrSettingNotFound) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("tempo para zerar: %w", err)
	}
	return v != "0", nil
}

// Settings returns the two options [TTB-023].
func (s *timeToBeatService) Settings(ctx context.Context) (ipc.TimeToBeatSettings, error) {
	en, err := s.boolSetting(ctx, ttbSettingEnabled)
	if err != nil {
		return ipc.TimeToBeatSettings{}, err
	}
	af, err := s.boolSetting(ctx, ttbSettingAfterMetadata)
	if err != nil {
		return ipc.TimeToBeatSettings{}, err
	}
	return ipc.TimeToBeatSettings{Enabled: en, AfterMetadata: af}, nil
}

// SetSettings writes the given options; turning the service off empties the queues [TTB-023].
func (s *timeToBeatService) SetSettings(ctx context.Context, p ipc.TimeToBeatSetSettingsParams) (ipc.TimeToBeatSettings, error) {
	if p.Enabled == nil && p.AfterMetadata == nil {
		return ipc.TimeToBeatSettings{}, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	flag := func(b bool) string {
		if b {
			return "1"
		}
		return "0"
	}
	now := s.clock.Now()
	if p.Enabled != nil {
		if err := s.st.SetSetting(ctx, ttbSettingEnabled, flag(*p.Enabled), now); err != nil {
			return ipc.TimeToBeatSettings{}, fmt.Errorf("tempo para zerar: %w", err)
		}
		if !*p.Enabled {
			s.mu.Lock()
			s.clearQueuesLocked()
			s.mu.Unlock()
		}
	}
	if p.AfterMetadata != nil {
		if err := s.st.SetSetting(ctx, ttbSettingAfterMetadata, flag(*p.AfterMetadata), now); err != nil {
			return ipc.TimeToBeatSettings{}, fmt.Errorf("tempo para zerar: %w", err)
		}
	}
	return s.Settings(ctx)
}

// ---- IPC-facing operations ----

func (s *timeToBeatService) game(ctx context.Context, id int64) (store.Game, error) {
	g, err := s.st.Game(ctx, id)
	if errors.Is(err, store.ErrGameNotFound) {
		return store.Game{}, ipc.NewErrorData(ipc.CodeGameNotFound, ipc.MsgGameNotFound, ipc.GameNotFoundData{GameID: id})
	}
	if err != nil {
		return store.Game{}, fmt.Errorf("tempo para zerar: ler jogo %d: %w", id, err)
	}
	return g, nil
}

func (s *timeToBeatService) unavailableError(reason string) error {
	return ipc.NewErrorData(ipc.CodeProviderUnavailable, ipc.MsgProviderUnavailable,
		ipc.ProviderUnavailableData{Provider: ipc.ProviderHLTB, Reason: reason})
}

// Get answers from the cache and, with fetch, queues a lookup when one is needed [TTB-007, TTB-008].
func (s *timeToBeatService) Get(ctx context.Context, gameID int64, fetch bool) (ipc.TimeToBeat, error) {
	g, err := s.game(ctx, gameID)
	if err != nil {
		return ipc.TimeToBeat{}, err
	}
	row, has, err := s.st.GetTimeToBeat(ctx, gameID)
	if err != nil {
		return ipc.TimeToBeat{}, fmt.Errorf("tempo para zerar: %w", err)
	}
	if fetch {
		need := !has || (s.isStale(g, row) && !(row.Confirmed && row.State == string(domain.TTBNotFound)))
		if need && !s.unavailable() {
			if enabled, err := s.boolSetting(ctx, ttbSettingEnabled); err == nil && enabled {
				s.enqueue(gameID, true)
			}
		}
	}
	return s.view(g, row, has), nil
}

func (s *timeToBeatService) isStale(g store.Game, row store.TimeToBeatRow) bool {
	return domain.TTBStale(row.Source, row.Confirmed, row.TitleAtFetch, g.Title, row.ExpiresAt, s.clock.Now())
}

// unavailable is true with the HowLongToBeat in recuo and the IGDB in recuo or without key [TTB-011].
func (s *timeToBeatService) unavailable() bool {
	hasKey := s.hasKey()
	now := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Before(s.backoff[ttbSourceHLTB].pausedUntil) &&
		(now.Before(s.backoff[ttbSourceIGDB].pausedUntil) || !hasKey)
}

func (s *timeToBeatService) pending(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queued[id] || s.inflight == id
}

func (s *timeToBeatService) view(g store.Game, row store.TimeToBeatRow, has bool) ipc.TimeToBeat {
	v := ipc.TimeToBeat{
		GameID: g.ID, State: ipc.TimeToBeatStateNone, Candidates: []ipc.TimeToBeatCandidate{},
		Pending: s.pending(g.ID), Unavailable: s.unavailable(),
	}
	if !has {
		return v
	}
	v.State, v.Source = row.State, row.Source
	v.ExternalID, v.ExternalTitle = row.ExternalID, row.ExternalTitle
	v.MainSeconds, v.MainExtraSeconds, v.CompletionistSeconds = row.MainS, row.MainExtraS, row.CompletionistS
	fetched := row.FetchedAt
	v.FetchedAt = &fetched
	v.Confirmed = row.Confirmed
	v.Stale = s.isStale(g, row)
	if c := decodeTTBCandidates(row.CandidatesJSON); c != nil {
		v.Candidates = c
	}
	return v
}

func decodeTTBCandidates(raw string) []ipc.TimeToBeatCandidate {
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []ipc.TimeToBeatCandidate
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// Refresh looks the game up now, ignoring validity and recuo [TTB-020].
func (s *timeToBeatService) Refresh(ctx context.Context, gameID int64) (ipc.TimeToBeat, error) {
	g, err := s.game(ctx, gameID)
	if err != nil {
		return ipc.TimeToBeat{}, err
	}
	if enabled, err := s.boolSetting(ctx, ttbSettingEnabled); err != nil {
		return ipc.TimeToBeat{}, err
	} else if !enabled {
		return ipc.TimeToBeat{}, s.unavailableError(ipc.ProviderReasonDisabled)
	}
	prev, has, err := s.st.GetTimeToBeat(ctx, gameID)
	if err != nil {
		return ipc.TimeToBeat{}, fmt.Errorf("tempo para zerar: %w", err)
	}
	s.reqMu.Lock()
	cctx, cancel := context.WithTimeout(ctx, s.syncTimeout)
	out := s.lookup(cctx, g, prev, has, true)
	cancel()
	s.reqMu.Unlock()
	if out.aborted {
		if err := ctx.Err(); err != nil {
			return ipc.TimeToBeat{}, fmt.Errorf("tempo para zerar: %w", err)
		}
		return ipc.TimeToBeat{}, fmt.Errorf("tempo para zerar: %w", context.Canceled)
	}
	if !out.write {
		return ipc.TimeToBeat{}, s.unavailableError(ipc.ProviderReasonUnavailable)
	}
	if err := s.st.UpsertTimeToBeat(ctx, out.row); err != nil {
		return ipc.TimeToBeat{}, fmt.Errorf("tempo para zerar: %w", err)
	}
	s.notify(gameID)
	return s.view(g, out.row, true), nil
}

// Search looks the term up on the HowLongToBeat only and keeps the candidates [TTB-021].
func (s *timeToBeatService) Search(ctx context.Context, gameID int64, term string) ([]ipc.TimeToBeatCandidate, error) {
	g, err := s.game(ctx, gameID)
	if err != nil {
		return nil, err
	}
	term = strings.TrimSpace(term)
	if term == "" {
		term = domain.SearchTerm(g.Title)
	}
	if n := utf8.RuneCountInString(term); n == 0 || n > ttbTermMax {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	if enabled, err := s.boolSetting(ctx, ttbSettingEnabled); err != nil {
		return nil, err
	} else if !enabled {
		return nil, s.unavailableError(ipc.ProviderReasonDisabled)
	}
	s.reqMu.Lock()
	cctx, cancel := context.WithTimeout(ctx, s.syncTimeout)
	results, err := s.client.Search(cctx, term)
	cancel()
	s.reqMu.Unlock()
	if err != nil {
		if s.cancelled(err) {
			return nil, fmt.Errorf("tempo para zerar: %w", err)
		}
		s.recordFailure(ttbSourceHLTB, err)
		return nil, s.unavailableError(ipc.ProviderReasonUnavailable)
	}
	s.recordSuccess(ttbSourceHLTB)
	year, _ := domain.ReleaseYear(g.ReleaseDate)
	_, ranked := domain.RankTimeToBeat(g.Title, year, toTTBResults(results))
	out := make([]ipc.TimeToBeatCandidate, 0, ttbMaxCandidates)
	for _, r := range ranked {
		if len(out) == ttbMaxCandidates {
			break
		}
		out = append(out, ttbCandidate(results[r.Index], r.Scored))
	}
	s.keepCandidates(gameID, out)
	return out, nil
}

func (s *timeToBeatService) keepCandidates(gameID int64, c []ipc.TimeToBeatCandidate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := slices.Index(s.order, gameID); i >= 0 {
		s.order = slices.Delete(s.order, i, i+1)
	}
	s.order = append(s.order, gameID)
	s.candidates[gameID] = c
	for len(s.order) > ttbCandidateGames {
		delete(s.candidates, s.order[0])
		s.order = s.order[1:]
	}
}

// Choose confirms one candidate (or "none of these" with an empty id) without any request [TTB-022].
func (s *timeToBeatService) Choose(ctx context.Context, gameID int64, externalID string) (ipc.TimeToBeat, error) {
	g, err := s.game(ctx, gameID)
	if err != nil {
		return ipc.TimeToBeat{}, err
	}
	prev, has, err := s.st.GetTimeToBeat(ctx, gameID)
	if err != nil {
		return ipc.TimeToBeat{}, fmt.Errorf("tempo para zerar: %w", err)
	}
	now := s.clock.Now()
	row := store.TimeToBeatRow{
		GameID: gameID, Source: ttbSourceHLTB, TitleAtFetch: g.Title, Term: domain.SearchTerm(g.Title),
		CandidatesJSON: "[]", Confirmed: true, FetchedAt: now,
	}
	if externalID == "" {
		row.State = string(domain.TTBNotFound)
	} else {
		s.mu.Lock()
		cands := slices.Clone(s.candidates[gameID])
		s.mu.Unlock()
		if has {
			cands = append(cands, decodeTTBCandidates(prev.CandidatesJSON)...)
		}
		i := slices.IndexFunc(cands, func(c ipc.TimeToBeatCandidate) bool { return c.ExternalID == externalID })
		if i < 0 {
			return ipc.TimeToBeat{}, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
		c := cands[i]
		row.State = string(domain.TTBFound)
		row.ExternalID, row.ExternalTitle = c.ExternalID, c.Title
		row.MainS, row.MainExtraS, row.CompletionistS = c.MainSeconds, c.MainExtraSeconds, c.CompletionistSeconds
	}
	row.ExpiresAt = domain.TTBExpiresAt(row.Source, domain.TTBState(row.State), now)
	if err := s.st.UpsertTimeToBeat(ctx, row); err != nil {
		return ipc.TimeToBeat{}, fmt.Errorf("tempo para zerar: %w", err)
	}
	s.notify(gameID)
	return s.view(g, row, true), nil
}

// ---- conversions ----

func toTTBResults(rs []scrapers.HLTBGame) []domain.TTBResult {
	out := make([]domain.TTBResult, len(rs))
	for i, r := range rs {
		out[i] = domain.TTBResult{ID: r.ID, Name: r.Name, Aliases: r.Aliases, Year: r.Year}
	}
	return out
}

func secondsPtr(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}

func ttbCandidate(g scrapers.HLTBGame, sc domain.Scored) ipc.TimeToBeatCandidate {
	c := ipc.TimeToBeatCandidate{
		ExternalID: strconv.FormatInt(g.ID, 10), Title: g.Name, Platforms: g.Platforms,
		MainSeconds: secondsPtr(g.Main), MainExtraSeconds: secondsPtr(g.MainExtra),
		CompletionistSeconds: secondsPtr(g.Completionist), Score: float64(sc.Score) / 1000,
	}
	if c.Platforms == nil {
		c.Platforms = []string{}
	}
	if g.Year > 0 {
		y := g.Year
		c.Year = &y
	}
	return c
}
