package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"sort"
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

// scrapeStore is what the metadata handlers need from the store.
type scrapeStore interface {
	GameDetail(ctx context.Context, id int64) (store.GameDetail, error)
	ApplyMetadata(ctx context.Context, id int64, f scrapers.Fields, replace bool, at time.Time) (store.GameDetail, error)
	ListMedia(ctx context.Context, gameID int64) ([]store.MediaRow, error)
	ActiveMediaJobCounts(ctx context.Context, gameID int64) (map[string]int, error)
	EnqueueMedia(ctx context.Context, gameID int64, kind, provider, sourceURL string, at time.Time) (int64, error)
	WindowsPlatformID(ctx context.Context) (int64, bool, error)
}

// scrapeService runs the metadata and media methods (phase 7, contract v0.9.0). Every write goes
// through the agent, which is the only writer of the database (ADR 0015).
type scrapeService struct {
	st        scrapeStore
	keys      *scrapers.Keys
	providers func(name string) (scrapers.MetadataProvider, error)
	queue     *mediaQueue
	mediaRoot string
	clock     domain.Clock
	log       *slog.Logger
	locks     gameLocks // serialises the image queueing of each game (MET-023)

	// videoCredsChanged is called after the ScreenScraper credentials are saved or removed (VID-017); nil is allowed.
	videoCredsChanged func()
}

// gameLocks is a mutex per game id. An entry exists only while someone holds or waits for the lock,
// so the map does not grow with the library (MET-023). The zero value is ready to use.
type gameLocks struct {
	mu sync.Mutex
	m  map[int64]*gameLock
}

// gameLock is one game's mutex with the number of holders and waiters that use it.
type gameLock struct {
	mu   sync.Mutex
	refs int // guarded by gameLocks.mu
}

// lock takes the lock of gameID and returns the function that releases it. Calling the function more
// than once is harmless.
func (g *gameLocks) lock(gameID int64) (unlock func()) {
	g.mu.Lock()
	if g.m == nil {
		g.m = map[int64]*gameLock{}
	}
	l := g.m[gameID]
	if l == nil {
		l = &gameLock{}
		g.m[gameID] = l
	}
	l.refs++
	g.mu.Unlock()

	l.mu.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Unlock()
			g.mu.Lock()
			l.refs--
			if l.refs == 0 {
				delete(g.m, gameID)
			}
			g.mu.Unlock()
		})
	}
}

// scrapeHandlers returns the handlers of the metadata and media methods.
func scrapeHandlers(s *scrapeService) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodProviderKeyStatus: s.keyStatus,
		ipc.MethodProviderSetKey:    s.setKey,
		ipc.MethodProviderDeleteKey: s.deleteKey,
		ipc.MethodScrapeSearch:      s.search,
		ipc.MethodScrapeApply:       s.apply,
		ipc.MethodMediaList:         s.mediaList,
		ipc.MethodMediaQueue:        s.mediaQueue,
		ipc.MethodMediaCancel:       s.mediaCancel,
	}
}

func (s *scrapeService) keyStatus(_ context.Context, params json.RawMessage) (any, error) {
	var p ipc.ProviderKeyParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if !validKeyProvider(p.Provider) {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	return keyResult(p.Provider, s.keys.Configured(p.Provider)), nil
}

func (s *scrapeService) setKey(_ context.Context, params json.RawMessage) (any, error) {
	var p ipc.ProviderSetKeyParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if !validKeyProvider(p.Provider) {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	var err error
	isSS := isScreenScraperProvider(p.Provider)
	if isSS {
		if strings.ContainsAny(p.Key, "\r\n") || strings.ContainsAny(p.Secret, "\r\n") {
			return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
		if n := utf8.RuneCountInString(strings.TrimSpace(p.Secret)); n < 1 || n > 512 {
			return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
		err = s.keys.SetPair(p.Provider, p.Key, p.Secret)
	} else if p.Provider == "igdb" {
		err = s.keys.SetIGDB(p.Key, p.Secret)
	} else {
		err = s.keys.Set(p.Provider, p.Key)
	}
	if errors.Is(err, scrapers.ErrNoKey) || errors.Is(err, scrapers.ErrInvalidKey) {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	if err != nil {
		s.log.Error("gravar chave de provedor", "provider", p.Provider, "error", err.Error())
		return nil, ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
	}
	if isSS {
		s.notifyVideoCreds()
	}
	return keyResult(p.Provider, s.keys.Configured(p.Provider)), nil
}

func (s *scrapeService) notifyVideoCreds() {
	if s.videoCredsChanged != nil {
		s.videoCredsChanged()
	}
}

func (s *scrapeService) deleteKey(_ context.Context, params json.RawMessage) (any, error) {
	var p ipc.ProviderKeyParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if !validKeyProvider(p.Provider) {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	if err := s.keys.Delete(p.Provider); err != nil {
		s.log.Error("apagar chave de provedor", "provider", p.Provider, "error", err.Error())
		return nil, ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
	}
	if isScreenScraperProvider(p.Provider) {
		s.notifyVideoCreds()
	}
	return keyResult(p.Provider, s.keys.Configured(p.Provider)), nil
}

func keyResult(provider string, configured bool) ipc.ProviderKeyStatusResult {
	return ipc.ProviderKeyStatusResult{Status: ipc.ProviderKeyStatus{Provider: provider, Configured: configured}}
}

// search asks the provider for the game and scores each candidate with the match rules of
// MID-002, best first.
func (s *scrapeService) search(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.ScrapeSearchParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if !validProvider(p.Provider) {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	d, err := s.game(ctx, p.GameID)
	if err != nil {
		return nil, err
	}
	prov, err := s.providers(p.Provider)
	if err != nil {
		return nil, s.providerError(p.Provider, err)
	}
	year := yearOf(d.ReleaseDate)
	cands, err := prov.Search(ctx, scrapers.Query{Title: d.Title, Platform: d.Platform, Year: year})
	if err != nil {
		return nil, s.providerError(p.Provider, err)
	}
	game := domain.Candidate{Title: d.Title, Platform: d.Platform, Year: year}
	out := make([]ipc.ScrapeCandidate, 0, len(cands))
	for _, c := range cands {
		score := domain.MatchScore(game, domain.Candidate{Title: c.Title, Platform: c.Platform, Year: c.Year})
		out = append(out, ipc.ScrapeCandidate{
			ProviderID: c.ProviderID, Title: c.Title, Platform: c.Platform, Year: c.Year,
			Score: score, Accepted: domain.MatchAccepted(score),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return ipc.ScrapeSearchResult{Candidates: out}, nil
}

// apply takes the metadata and the images of a candidate. The provider is asked first, so a
// failure leaves the game as it was; then the fields are merged in (MID-003) and the images are
// queued (MID-004).
func (s *scrapeService) apply(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.ScrapeApplyParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if !validProvider(p.Provider) || !validProviderID(p.ProviderID) {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	if _, err := s.game(ctx, p.GameID); err != nil {
		return nil, err
	}
	prov, err := s.providers(p.Provider)
	if err != nil {
		return nil, s.providerError(p.Provider, err)
	}
	// One request through Fetcher when the provider has it, as metadata.apply does [MET-012, MET-014].
	md, images, err := fetchFrom(ctx, prov, p.ProviderID)
	if err != nil {
		return nil, s.providerError(p.Provider, err)
	}
	now := s.clock.Now()
	fields := scrapers.Merge(map[string]scrapers.Metadata{prov.ID(): md})
	d, err := s.st.ApplyMetadata(ctx, p.GameID, fields, p.Replace, now)
	if err != nil {
		return nil, s.storeError("aplicar metadados", err)
	}
	res, err := s.queueImages(ctx, p.GameID, []scrapers.Offer{{Provider: prov.ID(), Media: images}}, nil, now)
	if err != nil {
		return nil, s.storeError("enfileirar mídia", err)
	}
	return ipc.ScrapeApplyResult{Detail: gameDetailToIPC(d), Queued: res.queued}, nil
}

// mediaList returns the images of a game with their absolute paths (MID-004, MID-006).
func (s *scrapeService) mediaList(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.MediaListParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if _, err := s.game(ctx, p.GameID); err != nil {
		return nil, err
	}
	rows, err := s.st.ListMedia(ctx, p.GameID)
	if err != nil {
		return nil, s.storeError("listar mídia", err)
	}
	out := make([]ipc.MediaItem, 0, len(rows))
	for _, r := range rows {
		item := ipc.MediaItem{ID: r.ID, Kind: r.Kind, Provider: r.Provider, Path: filepath.Join(s.mediaRoot, r.Path)}
		if r.ThumbPath != "" {
			item.ThumbPath = filepath.Join(s.mediaRoot, r.ThumbPath)
		}
		out = append(out, item)
	}
	return ipc.MediaListResult{Media: out}, nil
}

// mediaQueue queues the chosen kinds of a provider's images for a game, without changing its
// metadata (MID-005).
func (s *scrapeService) mediaQueue(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.MediaQueueParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if !validProvider(p.Provider) || !validProviderID(p.ProviderID) || len(p.Kinds) == 0 {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	kinds := map[string]bool{}
	for _, k := range p.Kinds {
		if !slices.Contains([]string{scrapers.KindCover, scrapers.KindScreenshot, scrapers.KindTitle}, k) {
			return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
		kinds[k] = true
	}
	if _, err := s.game(ctx, p.GameID); err != nil {
		return nil, err
	}
	prov, err := s.providers(p.Provider)
	if err != nil {
		return nil, s.providerError(p.Provider, err)
	}
	images, err := prov.Media(ctx, p.ProviderID)
	if err != nil {
		return nil, s.providerError(p.Provider, err)
	}
	res, err := s.queueImages(ctx, p.GameID, []scrapers.Offer{{Provider: prov.ID(), Media: images}}, kinds, s.clock.Now())
	if err != nil {
		return nil, s.storeError("enfileirar mídia", err)
	}
	return ipc.MediaQueueResult{Queued: res.queued}, nil
}

// mediaCancel stops a queued or running download (MID-005).
func (s *scrapeService) mediaCancel(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.MediaCancelParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if p.JobID < 1 {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	if err := s.queue.Cancel(ctx, p.JobID); err != nil {
		if errors.Is(err, store.ErrMediaJobNotFound) {
			return nil, ipc.NewError(ipc.CodeMediaJobNotFound, ipc.MsgMediaJobNotFound)
		}
		return nil, s.storeError("cancelar mídia", err)
	}
	return ipc.MediaCancelResult{Cancelled: true}, nil
}

// queueResult counts the images queueImages put in the queue and the ones left out by the limit (MET-024).
type queueResult struct{ queued, skipped int }

// queueImages queues the offered images that fit the limit per kind (MET-021, MET-022). Only the kinds
// in kinds are queued when kinds is not nil; the others are not counted as skipped. The vacancies are
// the limit minus the images the game has and the jobs active for it. Nothing is deleted. The game's
// lock is held while counting and enqueueing, never while a provider is asked (MET-023). The queue is
// woken only when something was queued (MET-024).
func (s *scrapeService) queueImages(ctx context.Context, gameID int64, offers []scrapers.Offer, kinds map[string]bool, at time.Time) (queueResult, error) {
	unlock := s.locks.lock(gameID)
	defer unlock()

	rows, err := s.st.ListMedia(ctx, gameID)
	if err != nil {
		return queueResult{}, fmt.Errorf("listar mídia do jogo: %w", err)
	}
	have := map[string]int{}
	for _, r := range rows {
		have[r.Kind]++
	}
	active, err := s.st.ActiveMediaJobCounts(ctx, gameID)
	if err != nil {
		return queueResult{}, fmt.Errorf("contar jobs ativos: %w", err)
	}
	d, err := s.st.GameDetail(ctx, gameID)
	if err != nil {
		return queueResult{}, fmt.Errorf("ler plataforma do jogo: %w", err)
	}
	winID, hasWin, err := s.st.WindowsPlatformID(ctx)
	if err != nil {
		return queueResult{}, fmt.Errorf("ler plataforma windows: %w", err)
	}
	windows := hasWin && winID == d.PlatformID

	if kinds != nil {
		filtered := make([]scrapers.Offer, 0, len(offers))
		for _, o := range offers {
			f := scrapers.Offer{Provider: o.Provider}
			for _, m := range o.Media {
				if kinds[m.Kind] {
					f.Media = append(f.Media, m)
				}
			}
			filtered = append(filtered, f)
		}
		offers = filtered
	}

	picks, skipped := scrapers.SelectImages(offers, have, active, windows)
	res := queueResult{skipped: skipped}
	for _, p := range picks {
		if _, err := s.st.EnqueueMedia(ctx, gameID, p.Kind, p.Provider, p.URL, at); err != nil {
			s.wakeIfQueued(res.queued)
			return res, fmt.Errorf("enfileirar %s: %w", p.Kind, err)
		}
		res.queued++
	}
	s.wakeIfQueued(res.queued)
	return res, nil
}

// wakeIfQueued tells the queue to look at the jobs, only when there is something new in it.
func (s *scrapeService) wakeIfQueued(n int) {
	if n > 0 {
		s.queue.Notify()
	}
}

// game returns the game of id, or the contract's game-not-found error.
func (s *scrapeService) game(ctx context.Context, id int64) (store.GameDetail, error) {
	if id < 1 {
		return store.GameDetail{}, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	d, err := s.st.GameDetail(ctx, id)
	if errors.Is(err, store.ErrGameNotFound) {
		return store.GameDetail{}, ipc.NewError(ipc.CodeGameNotFound, ipc.MsgGameNotFound)
	}
	if err != nil {
		return store.GameDetail{}, s.storeError("ler jogo", err)
	}
	return d, nil
}

// providerError maps a provider failure to the contract: a missing key is -32010, a temporary
// failure is -32011 (MID-001, MID-008). The key is never in the error.
func (s *scrapeService) providerError(provider string, err error) error {
	switch {
	case errors.Is(err, scrapers.ErrNoKey), errors.Is(err, scrapers.ErrKeyNotFound):
		return ipc.NewErrorData(ipc.CodeProviderNoKey, ipc.MsgProviderNoKey, map[string]string{"provider": provider})
	case errors.Is(err, scrapers.ErrUnavailable):
		return ipc.NewErrorData(ipc.CodeProviderUnavailable, ipc.MsgProviderUnavailable, map[string]string{"provider": provider})
	case errors.Is(err, errUnknownProvider):
		return ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	s.log.Error("provedor de metadados", "provider", provider, "error", err.Error())
	return ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
}

// storeError logs a store failure and gives the contract's internal error.
func (s *scrapeService) storeError(what string, err error) error {
	s.log.Error("mídia e metadados: "+what, "error", err.Error())
	return ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
}

// yearOf is the year of a release date written YYYY-MM-DD, 0 when there is none.
func yearOf(releaseDate string) int {
	if len(releaseDate) < 4 {
		return 0
	}
	y, err := strconv.Atoi(releaseDate[:4])
	if err != nil {
		return 0
	}
	return y
}
