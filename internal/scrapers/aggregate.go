package scrapers

import (
	"cmp"
	"slices"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

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
