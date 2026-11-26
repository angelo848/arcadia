package ipc

import "time"

// GameListResult is the result of game.list. Games is never nil ([] when the
// page is empty) and Total counts the games passing the filter, not only this
// page [LIB-002, LIB-003].
type GameListResult struct {
	Games  []Game `json:"games"`
	Total  int64  `json:"total"`
	Offset int    `json:"offset"`
}

// GameDetail is Game plus the fields of the game page [STO-026]. The lists are
// never nil. CompletedAt is a YYYY-MM-DD date or null.
type GameDetail struct {
	Game
	Description string `json:"description"`
	// EmulatorID is the game's own emulator, nil to use the platform default (EMU-002).
	EmulatorID  *int64   `json:"emulatorId"`
	CompletedAt *string  `json:"completedAt"`
	Developers  []string `json:"developers"`
	Publishers  []string `json:"publishers"`
	Genres      []string `json:"genres"`
	PlayModes   []string `json:"playModes"`
	// LaunchType is exe or uri and LaunchArgs the arguments of the launch (EDT-010).
	LaunchType string `json:"launchType"`
	LaunchArgs string `json:"launchArgs"`
	// SessionPlaySeconds is the sum of activeSeconds of the ended sessions (SED-034).
	SessionPlaySeconds int64 `json:"sessionPlaySeconds"`
}

// GameDetailResult is the result of game.get and game.update.
type GameDetailResult struct {
	Game GameDetail `json:"game"`
}

// GameDeletedResult is the result of game.delete.
type GameDeletedResult struct {
	GameID int64 `json:"gameId"`
}

// SessionItem is a session as listed on the game page [STO-027]. EndedAt is
// null while the session is open; EndReason is null while it is open.
type SessionItem struct {
	SessionID     int64      `json:"sessionId"`
	GameID        int64      `json:"gameId"`
	State         string     `json:"state"`
	StartedAt     time.Time  `json:"startedAt"`
	EndedAt       *time.Time `json:"endedAt"`
	ActiveSeconds int64      `json:"activeSeconds"`
	EndReason     *string    `json:"endReason"`
	// Source is tracked, import, manual or an unknown value passed as is; EditedAt is null if never edited.
	Source   string     `json:"source"`
	EditedAt *time.Time `json:"editedAt"`
}

// SessionListResult is the result of session.list. Sessions is never nil.
type SessionListResult struct {
	Sessions []SessionItem `json:"sessions"`
	Total    int64         `json:"total"`
	Offset   int           `json:"offset"`
}

// Platform is a platform of the library.
type Platform struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	// EmulatorID is the default emulator of the platform, nil when none (EMU-002).
	EmulatorID *int64 `json:"emulatorId"`
	// SeedKey is the key of the seed platform, nil for a user-created one (PLT-014).
	SeedKey *string `json:"seedKey"`
	// IconPath is the absolute path of the custom icon, nil for the default one (PLT-014).
	IconPath *string `json:"iconPath"`
	// GameCount is the number of games of the platform (PLT-014).
	GameCount int64 `json:"gameCount"`
}

// PlatformListResult is the result of platform.list.
type PlatformListResult struct {
	Platforms []Platform `json:"platforms"`
}

// PlatformAddResult is the result of platform.add.
type PlatformAddResult struct {
	Platform Platform `json:"platform"`
	// Created is false when the platform already existed (PLT-016).
	Created bool `json:"created"`
}

// CatalogItem is a genre or a company of library.catalog: the database id the
// FilterSpec filters by (FLT-002, FLT-003) and the name [IPC-049].
type CatalogItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Catalog is the result of library.catalog: the options of the editor and of
// the filters [IPC-049, STO-030].
type Catalog struct {
	Platforms []Platform    `json:"platforms"`
	Genres    []CatalogItem `json:"genres"`
	Companies []CatalogItem `json:"companies"`
	PlayModes []string      `json:"playModes"`
}
