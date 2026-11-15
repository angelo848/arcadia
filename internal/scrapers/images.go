package scrapers

import "github.com/arcadia-launcher/arcadia/internal/domain"

// Offer is the list of images one provider offered for a game, in the order the provider gave them.
type Offer struct {
	Provider string
	Media    []RemoteMedia
}

// Pick is one image chosen by SelectImages.
type Pick struct {
	Provider string
	Kind     string
	URL      string
}

// selectedKinds are the only kinds that are chosen; the others are ignored and do not count as skipped [MET-025].
var selectedKinds = []string{KindCover, KindScreenshot, KindBackground}

// priorityKey selects a row of providerPriority: the kind and whether the game is on the Windows class.
type priorityKey struct {
	kind    string
	windows bool
}

// providerPriority is the order in which providers are visited for each kind and platform class [MET-020, match-rules.md §4.2].
var providerPriority = map[priorityKey][]string{
	{KindCover, true}:       {"steamgriddb", "igdb"},
	{KindCover, false}:      {"igdb", "steamgriddb"},
	{KindScreenshot, true}:  {"igdb", "steamstore"},
	{KindScreenshot, false}: {"igdb", "steamstore"},
	{KindBackground, true}:  {"steamgriddb", "igdb"},
	{KindBackground, false}: {"steamgriddb", "igdb"},
}

// SelectImages chooses the images to queue for a game [MET-020, MET-021, MET-025].
// For each selected kind the vacancies are max(0, MediaKindLimit(kind) - have[kind] - active[kind]),
// negative counts are taken as 0. Providers are visited in providerPriority order, each one keeping its own
// order; a URL already chosen or already seen is dropped. Candidates beyond the vacancies count in skipped.
func SelectImages(offers []Offer, have, active map[string]int, windows bool) (picked []Pick, skipped int) {
	seen := make(map[string]bool)
	for _, kind := range selectedKinds {
		vacancies := domain.MediaKindLimit(kind) - max(0, have[kind]) - max(0, active[kind])
		if vacancies < 0 {
			vacancies = 0
		}

		var candidates []Pick
		for _, provider := range providerPriority[priorityKey{kind, windows}] {
			for _, offer := range offers {
				if offer.Provider != provider {
					continue
				}
				for _, m := range offer.Media {
					if m.Kind != kind || seen[m.URL] {
						continue
					}
					seen[m.URL] = true
					candidates = append(candidates, Pick{Provider: provider, Kind: kind, URL: m.URL})
				}
			}
		}

		n := min(vacancies, len(candidates))
		picked = append(picked, candidates[:n]...)
		skipped += len(candidates) - n
	}
	return picked, skipped
}
