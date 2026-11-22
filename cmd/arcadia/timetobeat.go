package main

import (
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// timeToBeatChangedEvent is the Wails event emitted when the time to beat of a game changes [TTB-030].
const timeToBeatChangedEvent = "timeToBeat:changed"

// Texts of -32011 for the HowLongToBeat provider.
const (
	textHLTBUnavailable = "Não foi possível consultar o HowLongToBeat"
	textHLTBDisabled    = "Consulta ao HowLongToBeat desligada"
)

const (
	timeToBeatShort = 2 * time.Second
	timeToBeatLong  = 30 * time.Second
)

// TimeToBeatCandidateView is one HowLongToBeat match for the frontend.
type TimeToBeatCandidateView struct {
	ExternalID           string   `json:"externalId"`
	Title                string   `json:"title"`
	Year                 *int     `json:"year"`
	Platforms            []string `json:"platforms"`
	MainSeconds          *int64   `json:"mainSeconds"`
	MainExtraSeconds     *int64   `json:"mainExtraSeconds"`
	CompletionistSeconds *int64   `json:"completionistSeconds"`
	Score                float64  `json:"score"`
}

// TimeToBeatView is the time to beat of a game for the frontend; FetchedAt is "" when unknown.
type TimeToBeatView struct {
	GameID               int64                     `json:"gameId"`
	State                string                    `json:"state"`
	Source               string                    `json:"source"`
	ExternalID           string                    `json:"externalId"`
	ExternalTitle        string                    `json:"externalTitle"`
	MainSeconds          *int64                    `json:"mainSeconds"`
	MainExtraSeconds     *int64                    `json:"mainExtraSeconds"`
	CompletionistSeconds *int64                    `json:"completionistSeconds"`
	FetchedAt            string                    `json:"fetchedAt"`
	Confirmed            bool                      `json:"confirmed"`
	Stale                bool                      `json:"stale"`
	Pending              bool                      `json:"pending"`
	Unavailable          bool                      `json:"unavailable"`
	Candidates           []TimeToBeatCandidateView `json:"candidates"`
}

// TimeToBeatSettingsView are the time to beat settings.
type TimeToBeatSettingsView struct {
	Enabled       bool `json:"enabled"`
	AfterMetadata bool `json:"afterMetadata"`
}

func timeToBeatCandidateViews(in []ipc.TimeToBeatCandidate) []TimeToBeatCandidateView {
	out := make([]TimeToBeatCandidateView, 0, len(in))
	for _, c := range in {
		out = append(out, TimeToBeatCandidateView(c))
	}
	return out
}

func timeToBeatView(t ipc.TimeToBeat) TimeToBeatView {
	v := TimeToBeatView{
		GameID: t.GameID, State: t.State, Source: t.Source, ExternalID: t.ExternalID, ExternalTitle: t.ExternalTitle,
		MainSeconds: t.MainSeconds, MainExtraSeconds: t.MainExtraSeconds, CompletionistSeconds: t.CompletionistSeconds,
		Confirmed: t.Confirmed, Stale: t.Stale, Pending: t.Pending, Unavailable: t.Unavailable,
		Candidates: timeToBeatCandidateViews(t.Candidates),
	}
	if t.FetchedAt != nil {
		v.FetchedAt = t.FetchedAt.Format(time.RFC3339)
	}
	return v
}

// TimeToBeatGet returns the cached time to beat; fetch asks the agent to query when missing.
func (a *App) TimeToBeatGet(gameID int64, fetch bool) (TimeToBeatView, error) {
	var res ipc.TimeToBeat
	err := a.callAgent(ipc.MethodTimeToBeatGet, timeToBeatShort, ipc.TimeToBeatGetParams{GameID: gameID, Fetch: fetch}, &res)
	if err != nil {
		return TimeToBeatView{}, err
	}
	return timeToBeatView(res), nil
}

// TimeToBeatRefresh queries the time to beat again.
func (a *App) TimeToBeatRefresh(gameID int64) (TimeToBeatView, error) {
	var res ipc.TimeToBeat
	if err := a.callAgent(ipc.MethodTimeToBeatRefresh, timeToBeatLong, ipc.TimeToBeatGameParams{GameID: gameID}, &res); err != nil {
		return TimeToBeatView{}, err
	}
	return timeToBeatView(res), nil
}

// TimeToBeatSearch searches candidates; an empty term uses the game title.
func (a *App) TimeToBeatSearch(gameID int64, term string) ([]TimeToBeatCandidateView, error) {
	p := ipc.TimeToBeatSearchParams{GameID: gameID}
	if term != "" {
		p.Term = &term
	}
	var res ipc.TimeToBeatSearchResult
	if err := a.callAgent(ipc.MethodTimeToBeatSearch, timeToBeatLong, p, &res); err != nil {
		return nil, err
	}
	return timeToBeatCandidateViews(res.Candidates), nil
}

// TimeToBeatChoose confirms a candidate; an empty externalId means "none of these".
func (a *App) TimeToBeatChoose(gameID int64, externalID string) (TimeToBeatView, error) {
	var res ipc.TimeToBeat
	err := a.callAgent(ipc.MethodTimeToBeatChoose, timeToBeatShort, ipc.TimeToBeatChooseParams{GameID: gameID, ExternalID: externalID}, &res)
	if err != nil {
		return TimeToBeatView{}, err
	}
	return timeToBeatView(res), nil
}

// TimeToBeatSettings returns the time to beat settings.
func (a *App) TimeToBeatSettings() (TimeToBeatSettingsView, error) {
	var res ipc.TimeToBeatSettings
	if err := a.callAgent(ipc.MethodTimeToBeatGetSettings, timeToBeatShort, nil, &res); err != nil {
		return TimeToBeatSettingsView{}, err
	}
	return TimeToBeatSettingsView(res), nil
}

// TimeToBeatSetSettings saves the time to beat settings.
func (a *App) TimeToBeatSetSettings(enabled, afterMetadata bool) (TimeToBeatSettingsView, error) {
	var res ipc.TimeToBeatSettings
	p := ipc.TimeToBeatSetSettingsParams{Enabled: &enabled, AfterMetadata: &afterMetadata}
	if err := a.callAgent(ipc.MethodTimeToBeatSetSettings, timeToBeatShort, p, &res); err != nil {
		return TimeToBeatSettingsView{}, err
	}
	return TimeToBeatSettingsView(res), nil
}
