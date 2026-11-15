package ipc

// Metadata and media methods (phase 7, contract v0.9.0 [MID-001..MID-006, MID-010]). Keys are
// never returned; a search or an apply needs the key of its provider.
const (
	MethodProviderKeyStatus = "provider.keyStatus"
	MethodProviderSetKey    = "provider.setKey"
	MethodProviderDeleteKey = "provider.deleteKey"
	MethodScrapeSearch      = "scrape.search"
	MethodScrapeApply       = "scrape.apply"
	MethodMediaList         = "media.list"
	MethodMediaQueue        = "media.queue"
	MethodMediaCancel       = "media.cancel"
)

// ProviderKeyStatus says whether the key of a provider is registered (MID-001).
type ProviderKeyStatus struct {
	Provider   string `json:"provider"`
	Configured bool   `json:"configured"`
}

// ProviderKeyStatusResult is the result of provider.keyStatus, provider.setKey and provider.deleteKey.
type ProviderKeyStatusResult struct {
	Status ProviderKeyStatus `json:"status"`
}

// ProviderKeyParams are the params of provider.keyStatus and provider.deleteKey.
type ProviderKeyParams struct {
	Provider string `json:"provider"`
}

// ProviderSetKeyParams are the params of provider.setKey. Secret is required for igdb, screenscraper and screenscraperdev.
type ProviderSetKeyParams struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
	Secret   string `json:"secret,omitempty"`
}

// ScrapeSearchParams are the params of scrape.search.
type ScrapeSearchParams struct {
	GameID   int64  `json:"gameId"`
	Provider string `json:"provider"`
}

// ScrapeCandidate is a game a provider found, with its match score (MID-002).
type ScrapeCandidate struct {
	ProviderID string  `json:"providerId"`
	Title      string  `json:"title"`
	Platform   string  `json:"platform"`
	Year       int     `json:"year"`
	Score      float64 `json:"score"`
	Accepted   bool    `json:"accepted"`
}

// ScrapeSearchResult is the result of scrape.search, best candidate first.
type ScrapeSearchResult struct {
	Candidates []ScrapeCandidate `json:"candidates"`
}

// ScrapeApplyParams are the params of scrape.apply.
type ScrapeApplyParams struct {
	GameID     int64  `json:"gameId"`
	Provider   string `json:"provider"`
	ProviderID string `json:"providerId"`
	Replace    bool   `json:"replace"`
}

// ScrapeApplyResult is the result of scrape.apply: the game after the write and the number of
// images queued (MID-003, MID-004).
type ScrapeApplyResult struct {
	Detail GameDetail `json:"detail"`
	Queued int        `json:"queued"`
}

// MediaItem is one image of a game with its absolute paths (MID-004).
type MediaItem struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Provider  string `json:"provider"`
	Path      string `json:"path"`
	ThumbPath string `json:"thumbPath"`
}

// MediaListParams are the params of media.list.
type MediaListParams struct {
	GameID int64 `json:"gameId"`
}

// MediaListResult is the result of media.list.
type MediaListResult struct {
	Media []MediaItem `json:"media"`
}

// MediaQueueParams are the params of media.queue.
type MediaQueueParams struct {
	GameID     int64    `json:"gameId"`
	Provider   string   `json:"provider"`
	ProviderID string   `json:"providerId"`
	Kinds      []string `json:"kinds"`
}

// MediaQueueResult is the result of media.queue.
type MediaQueueResult struct {
	Queued int `json:"queued"`
}

// MediaCancelParams are the params of media.cancel.
type MediaCancelParams struct {
	JobID int64 `json:"jobId"`
}

// MediaCancelResult is the result of media.cancel.
type MediaCancelResult struct {
	Cancelled bool `json:"cancelled"`
}
