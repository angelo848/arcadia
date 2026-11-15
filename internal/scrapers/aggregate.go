package scrapers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// Search limits [MET-001, MET-008]: 20 s for a single search, 15 s per game of a batch (set by the caller).
const (
	defaultSearchTimeout = 20 * time.Second
	maxGroups            = 10
)

// askOrder is the order in which providers are asked and reported.
var askOrder = []string{"igdb", "steamgriddb", "steamstore"}

// providerLabels are the names shown in the warnings to the user.
var providerLabels = map[string]string{
	"igdb":        "IGDB",
	"steamgriddb": "SteamGridDB",
	"steamstore":  "Steam Store",
}

// Aggregator asks the applicable providers of a query in parallel and groups their answers [MET-001].
type Aggregator struct {
	Providers func(name string) (MetadataProvider, error) // ErrNoKey = skip
	Clock     domain.Clock
}

// SearchOptions tunes one Search. Nil callbacks are ignored.
type SearchOptions struct {
	Staged  bool                             // batch: Steam Store only when the others did not find a "match" [MET-015]
	Timeout time.Duration                    // 20 s single, 15 s per game of the batch
	Skip    func(provider string) bool       // batch: provider suspended or disabled [MET-011]; nil = never
	Report  func(provider string, err error) // batch: result of each asked provider (nil err = ok); nil = ignored
}

// Outcome is the result of a Search. Groups are sorted; Confidence classifies the first one [MET-009].
// Asked, Failed and Missing list provider names in provider order: asked, failed temporarily, without key.
type Outcome struct {
	Groups     []Group
	Confidence domain.Confidence
	Warnings   []string
	Missing    []string
	Failed     []string
	Asked      []string
}

// Search asks the providers applicable to q and groups what they answer [MET-001, MET-002, MET-015].
// A provider without key is skipped (Missing); a temporary failure, or a provider still running when
// the deadline passes, is a warning (Failed). When every asked provider failed the error is
// ErrUnavailable; when none could be asked for lack of keys it is ErrNoKey. Any other provider error
// is returned. No goroutine outlives the call.
func (a *Aggregator) Search(ctx context.Context, q Query, opt SearchOptions) (Outcome, error) {
	var out Outcome
	if err := ctx.Err(); err != nil {
		return out, fmt.Errorf("scrapers: busca: %w", err)
	}
	clock := a.Clock
	if clock == nil {
		clock = domain.SystemClock{}
	}
	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = defaultSearchTimeout
	}

	runCtx, cancel := context.WithCancel(ctx)
	var expired atomic.Bool
	timer := clock.After(timeout)
	var watch sync.WaitGroup
	watch.Add(1)
	go func() {
		defer watch.Done()
		select {
		case <-timer:
			expired.Store(true)
			cancel()
		case <-runCtx.Done():
		}
	}()
	defer func() {
		cancel()
		watch.Wait()
	}()

	info := PlatformInfo{SeedKey: q.SeedKey, Category: q.Category, Known: q.Platform != "" || q.SeedKey != ""}
	byProvider := make(map[string][]Candidate)
	var first, second []string
	for _, p := range askOrder {
		if !Applicable(p, info) || (opt.Skip != nil && opt.Skip(p)) {
			continue
		}
		if opt.Staged && p == "steamstore" {
			second = append(second, p)
			continue
		}
		first = append(first, p)
	}

	// runStage asks the providers of one stage in parallel and waits for all of them.
	runStage := func(names []string) error {
		type call struct {
			name  string
			p     MetadataProvider
			cands []Candidate
			err   error
		}
		var calls []*call
		for _, name := range names {
			p, err := a.Providers(name)
			if errors.Is(err, ErrNoKey) {
				out.Missing = append(out.Missing, name)
				continue
			}
			if err != nil {
				return fmt.Errorf("scrapers: %s: %w", name, err)
			}
			calls = append(calls, &call{name: name, p: p})
		}
		var wg sync.WaitGroup
		for _, c := range calls {
			out.Asked = append(out.Asked, c.name)
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.cands, c.err = c.p.Search(runCtx, q)
			}()
		}
		wg.Wait()
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("scrapers: busca: %w", err)
		}
		for _, c := range calls {
			if opt.Report != nil {
				opt.Report(c.name, c.err)
			}
			switch {
			case c.err == nil:
				byProvider[c.name] = c.cands
			case errors.Is(c.err, ErrUnavailable) || expired.Load():
				out.Failed = append(out.Failed, c.name)
				out.Warnings = append(out.Warnings, providerLabels[c.name]+" indisponível no momento")
			default:
				return fmt.Errorf("scrapers: %s: %w", c.name, c.err)
			}
		}
		return nil
	}

	groupsSoFar := func() []Group {
		return buildGroups(domain.Query{Title: q.Title, Year: q.Year, LibraryKey: q.SeedKey}, byProvider, q.Platform, maxGroups)
	}

	if err := runStage(first); err != nil {
		return out, err
	}
	if len(second) > 0 && confidenceOf(groupsSoFar()) != domain.ConfidenceMatch {
		if err := runStage(second); err != nil {
			return out, err
		}
	}

	out.Groups = groupsSoFar()
	out.Confidence = confidenceOf(out.Groups)
	if len(out.Asked) == 0 && len(out.Missing) > 0 {
		return out, fmt.Errorf("scrapers: %s: %w", out.Missing[0], ErrNoKey)
	}
	if n := len(out.Asked); n > 0 && len(out.Failed) == n {
		return out, fmt.Errorf("scrapers: %s: %w", out.Failed[0], ErrUnavailable)
	}
	return out, nil
}

// confidenceOf classifies a sorted list of groups; no groups is ConfidenceNone.
func confidenceOf(groups []Group) domain.Confidence {
	if len(groups) == 0 {
		return domain.ConfidenceNone
	}
	var second *domain.Scored
	if len(groups) > 1 {
		second = &groups[1].Score
	}
	return domain.ClassifyMatch(&groups[0].Score, second)
}

// Ref points to a game inside one provider.
type Ref struct{ Provider, ProviderID string }

// Group is one game of the search result: the candidates of different providers that are the same
// game, with their score and the text and images each provider already returned [MET-005, MET-008].
type Group struct {
	Members []domain.Member
	Refs    []Ref // one per member, in provider order: igdb, steamstore, steamgriddb
	Score   domain.Scored
	Title   string
	Year    int
	// Platform is the name shown for the group; ThumbURL is that of the first member with one.
	Platform string
	ThumbURL string
	Meta     map[string]Metadata
	Images   map[string][]RemoteMedia
}

// groupProviderOrder is the order in which candidates join groups and in which members are listed.
var groupProviderOrder = []string{"igdb", "steamstore", "steamgriddb"}

// pendingGroup keeps the candidates of a group while it is being built.
type pendingGroup struct {
	cands []Candidate
}

func (g *pendingGroup) has(provider string) bool {
	for _, c := range g.cands {
		if c.Provider == provider {
			return true
		}
	}
	return false
}

// memberOf converts a candidate to the member the domain rules work with.
func memberOf(c Candidate) domain.Member {
	return domain.Member{
		Provider:          c.Provider,
		Title:             c.Title,
		Year:              c.Year,
		PlatformKeys:      c.PlatformKeys,
		PlatformConfirmed: c.PlatformConfirmed,
	}
}

// buildGroups joins candidates by domain.SameGame in provider order, scores each group with
// domain.ScoreGroup, sorts by score (desc), number of providers (desc) and title (asc), drops
// scores below domain.MatchAlternative and keeps at most max groups [MET-005, MET-006, MET-008].
// The result is deterministic and never nil.
func buildGroups(q domain.Query, byProvider map[string][]Candidate, libraryName string, max int) []Group {
	var pending []*pendingGroup
	for _, p := range groupProviderOrder {
		for _, c := range byProvider[p] {
			m := memberOf(c)
			var best *pendingGroup
			bestSim := -1
			for _, g := range pending {
				if g.has(c.Provider) {
					continue
				}
				first := memberOf(g.cands[0])
				if !domain.SameGame(first, m) {
					continue
				}
				// Strictly greater: on a tie the oldest group wins.
				if s := domain.GameSimilarity(first, m); s > bestSim {
					best, bestSim = g, s
				}
			}
			if best == nil {
				pending = append(pending, &pendingGroup{cands: []Candidate{c}})
				continue
			}
			best.cands = append(best.cands, c)
		}
	}

	groups := make([]Group, 0, len(pending))
	for _, g := range pending {
		grp := buildGroup(q, g.cands, libraryName)
		if grp.Score.Score < domain.MatchAlternative {
			continue
		}
		groups = append(groups, grp)
	}
	slices.SortStableFunc(groups, func(a, b Group) int {
		if c := cmp.Compare(b.Score.Score, a.Score.Score); c != 0 {
			return c
		}
		if c := cmp.Compare(len(b.Refs), len(a.Refs)); c != 0 {
			return c
		}
		return cmp.Compare(a.Title, b.Title)
	})
	if len(groups) > max {
		groups = groups[:max]
	}
	return groups
}

// buildGroup fills a Group from its candidates, which are in provider order.
func buildGroup(q domain.Query, cands []Candidate, libraryName string) Group {
	members := make([]domain.Member, 0, len(cands))
	refs := make([]Ref, 0, len(cands))
	grp := Group{
		Meta:   make(map[string]Metadata),
		Images: make(map[string][]RemoteMedia),
	}
	for _, c := range cands {
		members = append(members, memberOf(c))
		refs = append(refs, Ref{Provider: c.Provider, ProviderID: c.ProviderID})
		if c.Meta != nil {
			grp.Meta[c.Provider] = *c.Meta
		}
		if len(c.Images) > 0 {
			grp.Images[c.Provider] = c.Images
		}
		if grp.Year == 0 {
			grp.Year = c.Year
		}
		if grp.ThumbURL == "" {
			grp.ThumbURL = c.ThumbURL
		}
	}
	grp.Members = members
	grp.Refs = refs
	grp.Title = cands[0].Title
	grp.Score = domain.ScoreGroup(q, members)
	grp.Platform = groupPlatform(cands, q.LibraryKey, libraryName)
	return grp
}

// groupPlatform names the platform of a group: the library's name when a confirmed member lists the
// library key; otherwise the canonical name of the first known key; otherwise "" [MET-008].
func groupPlatform(cands []Candidate, libKey, libName string) string {
	if libKey != "" && libName != "" {
		for _, c := range cands {
			if (c.Provider != "igdb" || c.PlatformConfirmed) && slices.Contains(c.PlatformKeys, libKey) {
				return libName
			}
		}
	}
	for _, c := range cands {
		for _, k := range c.PlatformKeys {
			if p, ok := domain.SeedByKey(k); ok {
				return p.Name
			}
		}
	}
	return ""
}
