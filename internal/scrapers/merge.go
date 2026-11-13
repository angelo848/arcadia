package scrapers

// Fields is what the providers offer for one game after the merge (MID-003). Empty values mean
// no provider had them.
type Fields struct {
	Description                           string
	Year                                  int
	Genres, Modes, Developers, Publishers []string
}

// providerOrder is the priority of the providers that give text (MID-003): the first one with a
// value for a field wins it. SteamGridDB gives images only, so it is not listed.
var providerOrder = []string{"igdb", "steamstore"}

// Merge picks each field from the first provider in providerOrder that has a value for it.
// found maps a provider id to what it returned for the game.
func Merge(found map[string]Metadata) Fields {
	var f Fields
	for _, id := range providerOrder {
		m, ok := found[id]
		if !ok {
			continue
		}
		if f.Description == "" {
			f.Description = m.Description
		}
		if f.Year == 0 {
			f.Year = m.Year
		}
		if len(f.Genres) == 0 {
			f.Genres = m.Genres
		}
		if len(f.Modes) == 0 {
			f.Modes = m.Modes
		}
		if len(f.Developers) == 0 {
			f.Developers = m.Developers
		}
		if len(f.Publishers) == 0 {
			f.Publishers = m.Publishers
		}
	}
	return f
}
