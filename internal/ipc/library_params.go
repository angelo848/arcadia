package ipc

import "github.com/arcadia-launcher/arcadia/internal/domain"

// GameListParams are the by-name params of game.list [IPC-042, IPC-043]. Absent
// or {} means the first page of the empty filter ordered by title. Sort is
// validated by the agent (domain.ParseSort); Filter by FilterSpec.Validate.
type GameListParams struct {
	Filter *domain.FilterSpec `json:"filter,omitempty"`
	Sort   *string            `json:"sort,omitempty"`
	Offset *int               `json:"offset,omitempty"`
	Limit  *int               `json:"limit,omitempty"`
}

// GameGetParams are the params of game.get.
type GameGetParams struct {
	GameID int64 `json:"gameId"`
}

// GameDeleteParams are the params of game.delete.
type GameDeleteParams struct {
	GameID int64 `json:"gameId"`
}

// GameUpdateParams are the params of game.update [IPC-045, IPC-052]. GameID is
// required; every other member is a patch: absent leaves the field unchanged,
// null clears only CompletedAt and Year.
type GameUpdateParams struct {
	GameID       int64              `json:"gameId"`
	Title        Optional[string]   `json:"title"`
	PlatformID   Optional[int64]    `json:"platformId"`
	Status       Optional[string]   `json:"status"`
	Favorite     Optional[bool]     `json:"favorite"`
	CompletedAt  Optional[string]   `json:"completedAt"`
	Description  Optional[string]   `json:"description"`
	Year         Optional[int]      `json:"year"`
	Developers   Optional[[]string] `json:"developers"`
	Publishers   Optional[[]string] `json:"publishers"`
	Genres       Optional[[]string] `json:"genres"`
	PlayModes    Optional[[]string] `json:"playModes"`
	LaunchType   Optional[string]   `json:"launchType"`
	LaunchTarget Optional[string]   `json:"launchTarget"`
	LaunchArgs   Optional[string]   `json:"launchArgs"`
}

// SessionListParams are the params of session.list. GameID is required.
type SessionListParams struct {
	GameID int64 `json:"gameId"`
	Offset *int  `json:"offset,omitempty"`
	Limit  *int  `json:"limit,omitempty"`
}

// PlatformAddParams are the params of platform.add.
type PlatformAddParams struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}
