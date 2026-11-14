package scrapers

// igdbPlatformIDs maps a seed platform key to its IGDB platform id
// (contracts/seed-platforms.md §1).
var igdbPlatformIDs = map[string]string{
	"windows":  "6",
	"3ds":      "37",
	"n64":      "4",
	"nes":      "18",
	"gba":      "24",
	"gamecube": "21",
	"switch":   "130",
	"switch2":  "508",
	"wii":      "5",
	"ps1":      "7",
	"ps2":      "8",
	"ps3":      "9",
	"snes":     "19",
}

// PlatformID returns the id a provider uses for a seed platform key. Only "igdb"
// has values; the key must match exactly.
func PlatformID(seedKey, provider string) (string, bool) {
	if provider != "igdb" {
		return "", false
	}
	id, ok := igdbPlatformIDs[seedKey]
	return id, ok
}
