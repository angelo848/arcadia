package main

import (
	"errors"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// scrapeTimeout bounds the calls that ask a provider, which can be slow (phase 7).
const scrapeTimeout = 30 * time.Second

// ProviderKeyView says whether the key of a provider is registered; the key is never returned (MID-001).
type ProviderKeyView struct {
	Provider   string `json:"provider"`
	Configured bool   `json:"configured"`
}

// ScrapeCandidateView is a game a provider found, with its match score (MID-002).
type ScrapeCandidateView struct {
	ProviderID string  `json:"providerId"`
	Title      string  `json:"title"`
	Platform   string  `json:"platform"`
	Year       int     `json:"year"`
	Score      float64 `json:"score"`
	Accepted   bool    `json:"accepted"`
}

// ScrapeSearchView is the result of ScrapeSearch, best candidate first.
type ScrapeSearchView struct {
	Candidates []ScrapeCandidateView `json:"candidates"`
}

// ScrapeApplyView is the game after the metadata was applied and the number of images queued.
type ScrapeApplyView struct {
	Game   GameDetailView `json:"game"`
	Queued int            `json:"queued"`
}

// MediaItemView is one image of a game with its absolute paths (MID-004).
type MediaItemView struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Provider  string `json:"provider"`
	Path      string `json:"path"`
	ThumbPath string `json:"thumbPath"`
	// URL and ThumbURL are the paths the UI reads the images from (see serveMedia).
	URL      string `json:"url"`
	ThumbURL string `json:"thumbUrl"`
}

// MediaListView is the result of MediaList.
type MediaListView struct {
	Media []MediaItemView `json:"media"`
}

func keyView(s ipc.ProviderKeyStatus) ProviderKeyView {
	return ProviderKeyView{Provider: s.Provider, Configured: s.Configured}
}

// ProviderKeyStatus tells whether the key of a provider is registered (MID-001).
func (a *App) ProviderKeyStatus(provider string) (ProviderKeyView, error) {
	var res ipc.ProviderKeyStatusResult
	if err := a.callAgent(ipc.MethodProviderKeyStatus, libraryTimeout, ipc.ProviderKeyParams{Provider: provider}, &res); err != nil {
		return ProviderKeyView{}, err
	}
	return keyView(res.Status), nil
}

// ProviderSetKey stores the key of a provider; IGDB also takes its secret (MID-001).
func (a *App) ProviderSetKey(provider, key, secret string) (ProviderKeyView, error) {
	var res ipc.ProviderKeyStatusResult
	params := ipc.ProviderSetKeyParams{Provider: provider, Key: key, Secret: secret}
	if err := a.callAgent(ipc.MethodProviderSetKey, libraryTimeout, params, &res); err != nil {
		return ProviderKeyView{}, err
	}
	return keyView(res.Status), nil
}

// ProviderDeleteKey removes the key of a provider (MID-001).
func (a *App) ProviderDeleteKey(provider string) (ProviderKeyView, error) {
	var res ipc.ProviderKeyStatusResult
	if err := a.callAgent(ipc.MethodProviderDeleteKey, libraryTimeout, ipc.ProviderKeyParams{Provider: provider}, &res); err != nil {
		return ProviderKeyView{}, err
	}
	return keyView(res.Status), nil
}

// ScrapeSearch asks a provider for the game and returns the scored candidates (MID-002).
func (a *App) ScrapeSearch(gameID int64, provider string) (ScrapeSearchView, error) {
	var res ipc.ScrapeSearchResult
	if err := a.callAgent(ipc.MethodScrapeSearch, scrapeTimeout, ipc.ScrapeSearchParams{GameID: gameID, Provider: provider}, &res); err != nil {
		return ScrapeSearchView{}, err
	}
	out := ScrapeSearchView{Candidates: make([]ScrapeCandidateView, 0, len(res.Candidates))}
	for _, c := range res.Candidates {
		out.Candidates = append(out.Candidates, ScrapeCandidateView{
			ProviderID: c.ProviderID, Title: c.Title, Platform: c.Platform, Year: c.Year, Score: c.Score, Accepted: c.Accepted,
		})
	}
	return out, nil
}

// ScrapeApply applies the metadata of a candidate and queues its images (MID-003, MID-004).
func (a *App) ScrapeApply(gameID int64, provider, providerID string, replace bool) (ScrapeApplyView, error) {
	var res ipc.ScrapeApplyResult
	params := ipc.ScrapeApplyParams{GameID: gameID, Provider: provider, ProviderID: providerID, Replace: replace}
	if err := a.callAgent(ipc.MethodScrapeApply, scrapeTimeout, params, &res); err != nil {
		return ScrapeApplyView{}, err
	}
	return ScrapeApplyView{Game: gameDetailView(res.Detail), Queued: res.Queued}, nil
}

// MediaList returns the images of a game (MID-004, MID-006).
func (a *App) MediaList(gameID int64) (MediaListView, error) {
	var res ipc.MediaListResult
	if err := a.callAgent(ipc.MethodMediaList, libraryTimeout, ipc.MediaListParams{GameID: gameID}, &res); err != nil {
		return MediaListView{}, err
	}
	root := a.mediaRoot()
	out := MediaListView{Media: make([]MediaItemView, 0, len(res.Media))}
	for _, m := range res.Media {
		out.Media = append(out.Media, MediaItemView{
			ID: m.ID, Kind: m.Kind, Provider: m.Provider, Path: m.Path, ThumbPath: m.ThumbPath,
			URL: mediaURL(root, m.Path), ThumbURL: mediaURL(root, m.ThumbPath),
		})
	}
	return out, nil
}

// MediaQueue queues the chosen kinds of a provider's images for a game (MID-005).
func (a *App) MediaQueue(gameID int64, provider, providerID string, kinds []string) (int, error) {
	var res ipc.MediaQueueResult
	params := ipc.MediaQueueParams{GameID: gameID, Provider: provider, ProviderID: providerID, Kinds: kinds}
	if err := a.callAgent(ipc.MethodMediaQueue, scrapeTimeout, params, &res); err != nil {
		return 0, err
	}
	return res.Queued, nil
}

// MediaCancel stops a queued or running download (MID-005).
func (a *App) MediaCancel(jobID int64) error {
	var res ipc.MediaCancelResult
	err := a.callAgentRaw(ipc.MethodMediaCancel, libraryTimeout, ipc.MediaCancelParams{JobID: jobID}, &res)
	if err != nil {
		return userError(err, a.logger())
	}
	if !res.Cancelled {
		return errors.New(msgMediaJobNotFound)
	}
	return nil
}
