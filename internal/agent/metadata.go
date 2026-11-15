package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Limits of the parameters of metadata.search and metadata.apply [MET-071].
const (
	maxTitleRunes       = 200
	maxPlatformRunes    = 100
	minSearchYear       = 1900
	maxSearchYear       = 2100
	maxApplyRefs        = 3
	maxProviderIDDigits = 20
	maxThumbChars       = 300 // longer thumbnail URLs are dropped from a candidate [MET-068]
)

// thumbHosts are the only hosts a thumbnail may come from, over https [MET-068].
var thumbHosts = []string{"images.igdb.com", "cdn.cloudflare.steamstatic.com", "cdn2.steamgriddb.com"}

// metadataStore is what the unified service needs from the store: the scrape methods plus the
// platform lookup, which never creates a platform [MET-003].
type metadataStore interface {
	scrapeStore
	ResolvePlatform(ctx context.Context, name string) (store.Platform, bool, error)
}

// metadataService runs metadata.search and metadata.apply (phase 12, contract v0.14.0). Every write
// goes through the agent, the only writer of the database (ADR 0015).
type metadataService struct {
	scrape *scrapeService
	agg    *scrapers.Aggregator
	st     metadataStore
	clock  domain.Clock
	log    *slog.Logger
	root   context.Context // the agent's context: jobs are children of it, never of a request
	wg     sync.WaitGroup  // every goroutine of every job; Wait at shutdown [MET-059]

	notify      batchSink  // receives metadata.batchChanged (MET-054); nil = none
	batchMu     sync.Mutex // guards batch and lastBatchID; taken before a job's mu, never after
	batch       *batchJob  // the one batch of the agent, in memory only [MET-058]
	lastBatchID int64      // the last batchId issued, so ids grow even with a stopped clock
}

// newMetadataService builds the service over the scrape service, which keeps the providers, the
// queue and the clock.
func newMetadataService(s *scrapeService, st metadataStore, root context.Context) *metadataService {
	m := &metadataService{scrape: s, st: st, clock: s.clock, log: s.log, root: root}
	m.agg = &scrapers.Aggregator{Providers: m.provider, Clock: s.clock}
	return m
}

// metadataHandlers returns the handlers of the unified metadata methods.
func metadataHandlers(m *metadataService) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodMetadataSearch:       m.search,
		ipc.MethodMetadataApply:        m.apply,
		ipc.MethodMetadataBatchStart:   m.batchStart,
		ipc.MethodMetadataBatchStatus:  m.batchStatus,
		ipc.MethodMetadataBatchResults: m.batchResults,
		ipc.MethodMetadataBatchApply:   m.batchApply,
		ipc.MethodMetadataBatchCancel:  m.batchCancel,
		ipc.MethodMetadataBatchDiscard: m.batchDiscard,
	}
}

// Wait returns when every goroutine of the service has finished.
func (m *metadataService) Wait() { m.wg.Wait() }

// provider is the factory of the aggregator. A provider the agent does not have counts as one
// without key: it is skipped and is not a failure [MET-001].
func (m *metadataService) provider(name string) (scrapers.MetadataProvider, error) {
	p, err := m.scrape.providers(name)
	if errors.Is(err, errUnknownProvider) {
		return nil, scrapers.ErrNoKey
	}
	return p, err
}

// search asks the providers applicable to the game for the title and returns up to ten unified
// candidates, best first, with their confidence and warnings [MET-001..MET-009].
func (m *metadataService) search(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.MetadataSearchParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	title := strings.TrimSpace(p.Title)
	platform := strings.TrimSpace(p.Platform)
	if !validSearch(title, platform, p.Year) {
		return nil, badParams()
	}
	term := domain.SearchTerm(title)
	if term == "" {
		return nil, badParams()
	}
	q := scrapers.Query{Title: term, Year: p.Year}
	if platform != "" {
		plat, found, err := m.st.ResolvePlatform(ctx, platform)
		if err != nil {
			return nil, m.scrape.storeError("ler plataforma", err)
		}
		if found {
			q.Platform, q.SeedKey, q.Category = plat.Name, plat.SeedKey, string(plat.Category)
		}
	}
	out, err := m.agg.Search(ctx, q, scrapers.SearchOptions{Timeout: searchTimeout})
	if err != nil {
		return nil, m.searchError(ctx, out, err)
	}
	return searchResult(out), nil
}

// searchError maps a failed search to the contract: the first provider without key is -32010 and the
// first provider that failed temporarily is -32011, both with data.provider [MET-002].
func (m *metadataService) searchError(ctx context.Context, out scrapers.Outcome, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	switch {
	case errors.Is(err, scrapers.ErrNoKey) && len(out.Missing) > 0:
		return m.scrape.providerError(out.Missing[0], err)
	case errors.Is(err, scrapers.ErrUnavailable) && len(out.Failed) > 0:
		return m.scrape.providerError(out.Failed[0], err)
	}
	return m.scrape.providerError("", err)
}

// searchResult converts the outcome of the aggregator to the contract. Empty lists are never null.
func searchResult(out scrapers.Outcome) ipc.MetadataSearchResult {
	n := min(len(out.Groups), maxSearchCandidates)
	res := ipc.MetadataSearchResult{
		Candidates: make([]ipc.MetadataCandidate, 0, n),
		Confidence: string(out.Confidence),
		Warnings:   append([]string{}, out.Warnings...),
	}
	for _, g := range out.Groups[:n] {
		res.Candidates = append(res.Candidates, candidateOf(g))
	}
	return res
}

// candidateOf converts a group of the aggregator to a unified candidate; the score goes from
// thousandths to 0..1 [MET-008]. The title is cut to maxTitleRunes and the platform to maxPlatformRunes, and
// a thumbnail over maxThumbChars is dropped, so every row fits the ceiling of a page [MET-057, MET-068].
func candidateOf(g scrapers.Group) ipc.MetadataCandidate {
	refs := make([]ipc.MetadataRef, 0, len(g.Refs))
	for _, r := range g.Refs {
		refs = append(refs, ipc.MetadataRef{Provider: r.Provider, ProviderID: r.ProviderID})
	}
	thumb := allowedThumb(g.ThumbURL)
	if len(thumb) > maxThumbChars {
		thumb = ""
	}
	return ipc.MetadataCandidate{
		Refs:     refs,
		Title:    truncateRunes(g.Title, maxTitleRunes),
		Year:     g.Year,
		Platform: truncateRunes(g.Platform, maxPlatformRunes),
		Score:    float64(g.Score.Score) / 1000,
		ThumbURL: thumb,
	}
}

// truncateRunes returns s cut to at most n runes.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// validSearch checks the lengths and the year of metadata.search [MET-071]. The title is already
// trimmed; year 0 means unknown.
func validSearch(title, platform string, year int) bool {
	n := utf8.RuneCountInString(title)
	if n < 1 || n > maxTitleRunes {
		return false
	}
	if utf8.RuneCountInString(platform) > maxPlatformRunes {
		return false
	}
	return year == 0 || (year >= minSearchYear && year <= maxSearchYear)
}

// apply takes the text and the images of the refs and writes them on the game [MET-030..MET-033].
// Every ref is fetched before anything is written, so a failing provider leaves the game and the
// queue as they were. The title never changes; the queue notifies the images on its own.
func (m *metadataService) apply(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.MetadataApplyParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if p.GameID < 1 {
		return nil, badParams()
	}
	if err := validateRefs(p.Refs); err != nil {
		return nil, err
	}
	if _, err := m.scrape.game(ctx, p.GameID); err != nil {
		return nil, err
	}
	texts, offers, err := m.fetchRefs(ctx, p.Refs)
	if err != nil {
		return nil, err
	}
	d, res, err := m.commitApply(ctx, p.GameID, texts, offers, p.Replace)
	if err != nil {
		return nil, err
	}
	return ipc.MetadataApplyResult{Detail: gameDetailToIPC(d), Queued: res.queued, SkippedByLimit: res.skipped}, nil
}

// commitApply writes the merged text of the fetched providers on the game and queues the offered images
// [MET-030, MET-033]. It is the write path of metadata.apply and of the batch application [MET-065], so
// both keep the same merge and the same image limit [MET-022].
func (m *metadataService) commitApply(ctx context.Context, gameID int64, texts map[string]scrapers.Metadata, offers []scrapers.Offer, replace bool) (store.GameDetail, queueResult, error) {
	now := m.clock.Now()
	d, err := m.st.ApplyMetadata(ctx, gameID, scrapers.Merge(texts), replace, now)
	if err != nil {
		return store.GameDetail{}, queueResult{}, m.scrape.storeError("aplicar metadados", err)
	}
	res, err := m.scrape.queueImages(ctx, gameID, offers, nil, now)
	if err != nil {
		return d, queueResult{}, m.scrape.storeError("enfileirar mídia", err)
	}
	return d, res, nil
}

// fetchRefs obtains the text and the images of every ref: one request through Fetcher when the
// provider has it, Details and Media otherwise [MET-012..MET-014, MET-032]. The text is keyed by
// provider; the offers keep the order of the refs.
func (m *metadataService) fetchRefs(ctx context.Context, refs []ipc.MetadataRef) (map[string]scrapers.Metadata, []scrapers.Offer, error) {
	texts := make(map[string]scrapers.Metadata, len(refs))
	offers := make([]scrapers.Offer, 0, len(refs))
	for _, r := range refs {
		md, media, err := m.fetchOne(ctx, r)
		if err != nil {
			return nil, nil, err
		}
		texts[r.Provider] = md
		offers = append(offers, scrapers.Offer{Provider: r.Provider, Media: media})
	}
	return texts, offers, nil
}

// fetchOne obtains the text and the images of one ref, mapping a provider failure to the contract.
func (m *metadataService) fetchOne(ctx context.Context, r ipc.MetadataRef) (scrapers.Metadata, []scrapers.RemoteMedia, error) {
	prov, err := m.provider(r.Provider)
	if err != nil {
		return scrapers.Metadata{}, nil, m.scrape.providerError(r.Provider, err)
	}
	md, media, err := fetchFrom(ctx, prov, r.ProviderID)
	if err != nil {
		return scrapers.Metadata{}, nil, m.scrape.providerError(r.Provider, err)
	}
	return md, media, nil
}

// fetchFrom obtains the text and the images of one provider id: one request through Fetcher when the
// provider has it, Details and Media otherwise [MET-012, MET-014]. It returns the provider's error as is.
func fetchFrom(ctx context.Context, prov scrapers.MetadataProvider, providerID string) (scrapers.Metadata, []scrapers.RemoteMedia, error) {
	if f, ok := prov.(scrapers.Fetcher); ok {
		return f.Fetch(ctx, providerID)
	}
	md, err := prov.Details(ctx, providerID)
	if err != nil {
		return scrapers.Metadata{}, nil, err
	}
	media, err := prov.Media(ctx, providerID)
	if err != nil {
		return scrapers.Metadata{}, nil, err
	}
	return md, media, nil
}

// validateRefs checks the refs of metadata.apply before anything is read: one to three entries, each
// with a known provider, no provider repeated, and a provider id of 1 to 20 digits [MET-031, MET-071].
func validateRefs(refs []ipc.MetadataRef) error {
	if len(refs) < 1 || len(refs) > maxApplyRefs {
		return badParams()
	}
	seen := make(map[string]bool, len(refs))
	for _, r := range refs {
		if !validProvider(r.Provider) || seen[r.Provider] || !validProviderID(r.ProviderID) {
			return badParams()
		}
		seen[r.Provider] = true
	}
	return nil
}

// validProviderID reports whether id is 1 to 20 decimal digits.
func validProviderID(id string) bool {
	if n := len(id); n < 1 || n > maxProviderIDDigits {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// allowedThumb returns raw when it is an https URL of an allowed host, and "" otherwise [MET-068].
func allowedThumb(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return ""
	}
	if !slices.Contains(thumbHosts, strings.ToLower(u.Hostname())) {
		return ""
	}
	return raw
}

// badParams is the contract's -32602 error.
func badParams() error {
	return ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
}
