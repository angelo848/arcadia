package ipc

import "time"

// Video methods and constants (contract v0.15.0) [VID-005..VID-008, SSC-001].
const (
	MethodVideoQueue         = "video.queue"
	MethodVideoQueueMissing  = "video.queueMissing"
	MethodVideoCancelPending = "video.cancelPending"
	MethodVideoStatus        = "video.status"

	ProviderScreenScraper    = "screenscraper"
	ProviderScreenScraperDev = "screenscraperdev"

	VideoSkipHasVideo         = "has-video"
	VideoSkipPlatformUnmapped = "platform-unmapped"

	VideoLaneActive            = "active"
	VideoLanePausedQuota       = "paused-quota"
	VideoLanePausedCredentials = "paused-credentials"
	VideoLaneNoCredentials     = "no-credentials"
)

// VideoQueueParams are the params of video.queue.
type VideoQueueParams struct {
	GameIDs []int64 `json:"gameIds"`
}

// VideoSkip is a game left out of video.queue.
type VideoSkip struct {
	GameID int64  `json:"gameId"`
	Reason string `json:"reason"`
}

// VideoQueueResult is the result of video.queue; Skipped is never nil on the wire.
type VideoQueueResult struct {
	Queued  int         `json:"queued"`
	Skipped []VideoSkip `json:"skipped"`
}

// VideoQueueMissingParams are the params of video.queueMissing.
type VideoQueueMissingParams struct {
	PlatformID int64 `json:"platformId,omitempty"`
}

// VideoQueueMissingResult is the result of video.queueMissing.
type VideoQueueMissingResult struct {
	Queued  int `json:"queued"`
	Skipped int `json:"skipped"`
}

// VideoCancelResult is the result of video.cancelPending.
type VideoCancelResult struct {
	Cancelled int `json:"cancelled"`
}

// VideoStatus is the state of the video queue.
type VideoStatus struct {
	Pending           int        `json:"pending"`
	Running           int        `json:"running"`
	Failed            int        `json:"failed"`
	Lane              string     `json:"lane"`
	PausedUntil       *time.Time `json:"pausedUntil,omitempty"`
	RequestsToday     *int       `json:"requestsToday,omitempty"`
	MaxRequestsPerDay *int       `json:"maxRequestsPerDay,omitempty"`
}

// VideoStatusResult is the result of video.status.
type VideoStatusResult struct {
	Status VideoStatus `json:"status"`
}
