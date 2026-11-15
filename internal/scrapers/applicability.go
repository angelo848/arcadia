package scrapers

import "github.com/arcadia-launcher/arcadia/internal/domain"

// PlatformInfo describes the platform of the library game a search is made for.
// SeedKey is the key of a seed platform ("" when there is none), Category its category,
// and Known is false when the platform is not informed or was not found.
type PlatformInfo struct {
	SeedKey  string
	Category string
	Known    bool
}

// Applicable tells whether provider takes part in a search for p [MET-003, match-rules.md §1].
// igdb and steamgriddb always apply. steamstore only applies to Windows, to PC platforms
// without a seed key, and when there is no platform at all. Unknown providers never apply.
func Applicable(provider string, p PlatformInfo) bool {
	switch provider {
	case "igdb", "steamgriddb":
		return true
	case "steamstore":
		if !p.Known {
			return true
		}
		if p.SeedKey == "windows" {
			return true
		}
		if p.SeedKey != "" {
			return false
		}
		return p.Category == string(domain.CategoryPC)
	default:
		return false
	}
}
