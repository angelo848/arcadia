package main

import (
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// VideoSkipView is a game left out of VideoQueue and why.
type VideoSkipView struct {
	GameID int64  `json:"gameId"`
	Reason string `json:"reason"`
}

// VideoQueueView is the result of VideoQueue; Skipped is never nil.
type VideoQueueView struct {
	Queued  int             `json:"queued"`
	Skipped []VideoSkipView `json:"skipped"`
}

// VideoQueueMissingView is the result of VideoQueueMissing.
type VideoQueueMissingView struct {
	Queued  int `json:"queued"`
	Skipped int `json:"skipped"`
}

// VideoStatusView is the state of the video queue; the quota counts are -1 when unknown.
type VideoStatusView struct {
	Pending           int    `json:"pending"`
	Running           int    `json:"running"`
	Failed            int    `json:"failed"`
	Lane              string `json:"lane"`
	PausedUntil       string `json:"pausedUntil"`
	RequestsToday     int    `json:"requestsToday"`
	MaxRequestsPerDay int    `json:"maxRequestsPerDay"`
}

// VideoQueue queues the videos of the given games (VID-005).
func (a *App) VideoQueue(gameIDs []int64) (VideoQueueView, error) {
	if gameIDs == nil {
		gameIDs = []int64{}
	}
	var res ipc.VideoQueueResult
	if err := a.callAgent(ipc.MethodVideoQueue, libraryTimeout, ipc.VideoQueueParams{GameIDs: gameIDs}, &res); err != nil {
		return VideoQueueView{}, err
	}
	out := VideoQueueView{Queued: res.Queued, Skipped: make([]VideoSkipView, 0, len(res.Skipped))}
	for _, s := range res.Skipped {
		out.Skipped = append(out.Skipped, VideoSkipView{GameID: s.GameID, Reason: s.Reason})
	}
	return out, nil
}

// VideoQueueMissing queues the videos of every game without one; platformID 0 means the whole library (VID-006).
func (a *App) VideoQueueMissing(platformID int64) (VideoQueueMissingView, error) {
	var res ipc.VideoQueueMissingResult
	if err := a.callAgent(ipc.MethodVideoQueueMissing, libraryTimeout, ipc.VideoQueueMissingParams{PlatformID: platformID}, &res); err != nil {
		return VideoQueueMissingView{}, err
	}
	return VideoQueueMissingView{Queued: res.Queued, Skipped: res.Skipped}, nil
}

// VideoCancelPending cancels the video jobs that have not started (VID-007).
func (a *App) VideoCancelPending() (int, error) {
	var res ipc.VideoCancelResult
	if err := a.callAgent(ipc.MethodVideoCancelPending, libraryTimeout, nil, &res); err != nil {
		return 0, err
	}
	return res.Cancelled, nil
}

// VideoStatus returns the state of the video queue (VID-008).
func (a *App) VideoStatus() (VideoStatusView, error) {
	var res ipc.VideoStatusResult
	if err := a.callAgent(ipc.MethodVideoStatus, libraryTimeout, nil, &res); err != nil {
		return VideoStatusView{}, err
	}
	s := res.Status
	v := VideoStatusView{
		Pending: s.Pending, Running: s.Running, Failed: s.Failed, Lane: s.Lane,
		RequestsToday: -1, MaxRequestsPerDay: -1,
	}
	if s.PausedUntil != nil {
		v.PausedUntil = s.PausedUntil.UTC().Format(time.RFC3339)
	}
	if s.RequestsToday != nil {
		v.RequestsToday = *s.RequestsToday
	}
	if s.MaxRequestsPerDay != nil {
		v.MaxRequestsPerDay = *s.MaxRequestsPerDay
	}
	return v, nil
}
