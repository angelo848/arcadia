package ipc

import "time"

// Methods of IPC v0.16.0 for the time to beat [TTB-029].
const (
	MethodTimeToBeatGet         = "timeToBeat.get"
	MethodTimeToBeatRefresh     = "timeToBeat.refresh"
	MethodTimeToBeatSearch      = "timeToBeat.search"
	MethodTimeToBeatChoose      = "timeToBeat.choose"
	MethodTimeToBeatGetSettings = "timeToBeat.getSettings"
	MethodTimeToBeatSetSettings = "timeToBeat.setSettings"
	MethodTimeToBeatChanged     = "timeToBeat.changed" // notification
)

// Values of TimeToBeat.State.
const (
	TimeToBeatStateNone     = "none"
	TimeToBeatStateFound    = "found"
	TimeToBeatStateDoubt    = "doubt"
	TimeToBeatStateNotFound = "not-found"
)

// Values of TimeToBeat.Source (empty with state none).
const (
	TimeToBeatSourceHLTB = "hltb"
	TimeToBeatSourceIGDB = "igdb"
)

// Provider and reasons carried in the data of ProviderUnavailable (-32011) for the time to beat.
const (
	ProviderHLTB              = "hltb"
	ProviderReasonUnavailable = "unavailable"
	ProviderReasonDisabled    = "disabled"
)

// ProviderUnavailableData is the data of ProviderUnavailable; Reason is optional.
type ProviderUnavailableData struct {
	Provider string `json:"provider"`
	Reason   string `json:"reason,omitempty"`
}

// TimeToBeatGetParams are the params of timeToBeat.get.
type TimeToBeatGetParams struct {
	GameID int64 `json:"gameId"`
	Fetch  bool  `json:"fetch,omitempty"`
}

// TimeToBeatGameParams are the params of timeToBeat.refresh.
type TimeToBeatGameParams struct {
	GameID int64 `json:"gameId"`
}

// TimeToBeatSearchParams are the params of timeToBeat.search.
type TimeToBeatSearchParams struct {
	GameID int64   `json:"gameId"`
	Term   *string `json:"term,omitempty"`
}

// TimeToBeatChooseParams are the params of timeToBeat.choose; an empty ExternalId means "none of these".
type TimeToBeatChooseParams struct {
	GameID     int64  `json:"gameId"`
	ExternalID string `json:"externalId"`
}

// TimeToBeatSetSettingsParams are the params of timeToBeat.setSettings (at least one member).
type TimeToBeatSetSettingsParams struct {
	Enabled       *bool `json:"enabled,omitempty"`
	AfterMetadata *bool `json:"afterMetadata,omitempty"`
}

// TimeToBeatChangedParams are the params of the timeToBeat.changed notification.
type TimeToBeatChangedParams struct {
	GameID int64 `json:"gameId"`
}

// TimeToBeatSettings is the result of timeToBeat.getSettings and timeToBeat.setSettings.
type TimeToBeatSettings struct {
	Enabled       bool `json:"enabled"`
	AfterMetadata bool `json:"afterMetadata"`
}

// TimeToBeat is the result of timeToBeat.get, refresh and choose. Durations are seconds, null when unknown.
// Candidates is never nil on the wire.
type TimeToBeat struct {
	GameID               int64                 `json:"gameId"`
	State                string                `json:"state"`
	Source               string                `json:"source"`
	ExternalID           string                `json:"externalId"`
	ExternalTitle        string                `json:"externalTitle"`
	MainSeconds          *int64                `json:"mainSeconds"`
	MainExtraSeconds     *int64                `json:"mainExtraSeconds"`
	CompletionistSeconds *int64                `json:"completionistSeconds"`
	FetchedAt            *time.Time            `json:"fetchedAt"`
	Confirmed            bool                  `json:"confirmed"`
	Stale                bool                  `json:"stale"`
	Pending              bool                  `json:"pending"`
	Unavailable          bool                  `json:"unavailable"`
	Candidates           []TimeToBeatCandidate `json:"candidates"`
}

// TimeToBeatCandidate is one HowLongToBeat match offered for confirmation.
type TimeToBeatCandidate struct {
	ExternalID           string   `json:"externalId"`
	Title                string   `json:"title"`
	Year                 *int     `json:"year"`
	Platforms            []string `json:"platforms"`
	MainSeconds          *int64   `json:"mainSeconds"`
	MainExtraSeconds     *int64   `json:"mainExtraSeconds"`
	CompletionistSeconds *int64   `json:"completionistSeconds"`
	Score                float64  `json:"score"`
}

// TimeToBeatSearchResult is the result of timeToBeat.search.
type TimeToBeatSearchResult struct {
	Candidates []TimeToBeatCandidate `json:"candidates"`
}
