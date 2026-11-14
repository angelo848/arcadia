package domain

import "strings"

// SeedPlatform is a platform created by migration 00008.
type SeedPlatform struct {
	Key      string // stable seed key, e.g. "n64"
	Name     string // canonical name, e.g. "Nintendo 64"
	Category PlatformCategory
}

// SeedPlatforms is the seed of migration 00008, in the order of contracts/seed-platforms.md.
var SeedPlatforms = []SeedPlatform{
	{Key: "windows", Name: "Windows", Category: CategoryPC},
	{Key: "3ds", Name: "Nintendo 3DS", Category: CategoryHandheld},
	{Key: "n64", Name: "Nintendo 64", Category: CategoryConsole},
	{Key: "nes", Name: "Nintendo Entertainment System", Category: CategoryConsole},
	{Key: "gba", Name: "Nintendo Game Boy Advance", Category: CategoryHandheld},
	{Key: "gamecube", Name: "Nintendo GameCube", Category: CategoryConsole},
	{Key: "switch", Name: "Nintendo Switch", Category: CategoryConsole},
	{Key: "switch2", Name: "Nintendo Switch 2", Category: CategoryConsole},
	{Key: "wii", Name: "Nintendo Wii", Category: CategoryConsole},
	{Key: "ps1", Name: "Sony Playstation", Category: CategoryConsole},
	{Key: "ps2", Name: "Sony Playstation 2", Category: CategoryConsole},
	{Key: "ps3", Name: "Sony Playstation 3", Category: CategoryConsole},
	{Key: "snes", Name: "Super Nintendo Entertainment System", Category: CategoryConsole},
}

// SeedKeyWindows is the key of the platform of executable, Steam and Epic games.
const SeedKeyWindows = "windows"

// SeedByKey returns the seed platform of key; false for an unknown key.
func SeedByKey(key string) (SeedPlatform, bool) {
	for _, p := range SeedPlatforms {
		if p.Key == key {
			return p, true
		}
	}
	return SeedPlatform{}, false
}

// SeedByName returns the seed platform whose canonical name equals name after
// trimming, ignoring ASCII case (same rule as SQLite COLLATE NOCASE).
func SeedByName(name string) (SeedPlatform, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return SeedPlatform{}, false
	}
	for _, p := range SeedPlatforms {
		if equalFoldASCII(p.Name, name) {
			return p, true
		}
	}
	return SeedPlatform{}, false
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if lowerASCII(a[i]) != lowerASCII(b[i]) {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
