package scrapers

import "strconv"

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

// igdbSeedKeys is the inverse of igdbPlatformIDs, keyed by the numeric IGDB id.
var igdbSeedKeys = func() map[int64]string {
	m := make(map[int64]string, len(igdbPlatformIDs))
	for key, id := range igdbPlatformIDs {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			continue
		}
		m[n] = key
	}
	return m
}()

// SeedKeysForIGDB maps IGDB platform ids to seed platform keys, keeping the order of ids.
// Ids outside the table are ignored; the result is never nil.
func SeedKeysForIGDB(ids []int64) []string {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		if key, ok := igdbSeedKeys[id]; ok {
			keys = append(keys, key)
		}
	}
	return keys
}
