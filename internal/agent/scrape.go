package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"time"

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
	EnqueueMedia(ctx context.Context, gameID int64, kind, provider, sourceURL string, at time.Time) (int64, error)
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
	if p.Provider == "igdb" {
		err = s.keys.SetIGDB(p.Key, p.Secret)
	} else {
		err = s.keys.Set(p.Provider, p.Key)
	}
	if errors.Is(err, scrapers.ErrNoKey) {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	if err != nil {
		s.log.Error("gravar chave de provedor", "provider", p.Provider, "error", err.Error())
		return nil, ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
	}
	return keyResult(p.Provider, s.keys.Configured(p.Provider)), nil
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
	if !validProvider(p.Provider) || p.ProviderID == "" {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	if _, err := s.game(ctx, p.GameID); err != nil {
		return nil, err
	}
	prov, err := s.providers(p.Provider)
	if err != nil {
		return nil, s.providerError(p.Provider, err)
	}
	md, err := prov.Details(ctx, p.ProviderID)
	if err != nil {
		return nil, s.providerError(p.Provider, err)
	}
	images, err := prov.Media(ctx, p.ProviderID)
	if err != nil {
		return nil, s.providerError(p.Provider, err)
	}
	now := s.clock.Now()
	fields := scrapers.Merge(map[string]scrapers.Metadata{prov.ID(): md})
	d, err := s.st.ApplyMetadata(ctx, p.GameID, fields, p.Replace, now)
	if err != nil {
		return nil, s.storeError("aplicar metadados", err)
	}
	queued, err := s.queueImages(ctx, p.GameID, prov.ID(), images, nil, now)
	if err != nil {
		return nil, s.storeError("enfileirar mídia", err)
	}
	return ipc.ScrapeApplyResult{Detail: gameDetailToIPC(d), Queued: queued}, nil
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
	if !validProvider(p.Provider) || p.ProviderID == "" || len(p.Kinds) == 0 {
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
	queued, err := s.queueImages(ctx, p.GameID, prov.ID(), images, kinds, s.clock.Now())
	if err != nil {
		return nil, s.storeError("enfileirar mídia", err)
	}
	return ipc.MediaQueueResult{Queued: queued}, nil
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

// queueImages queues the images of the given kinds (all kinds when nil) and wakes the queue.
func (s *scrapeService) queueImages(ctx context.Context, gameID int64, provider string, images []scrapers.RemoteMedia, kinds map[string]bool, at time.Time) (int, error) {
	n := 0
	for _, m := range images {
		if kinds != nil && !kinds[m.Kind] {
			continue
		}
		if _, err := s.st.EnqueueMedia(ctx, gameID, m.Kind, provider, m.URL, at); err != nil {
			return n, err
		}
		n++
	}
	if n > 0 {
		s.queue.Notify()
	}
	return n, nil
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
