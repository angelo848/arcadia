package main

import (
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// MetadataRefView identifies one provider entry of a candidate [MET-031].
type MetadataRefView struct {
	Provider   string `json:"provider"`
	ProviderID string `json:"providerId"`
}

// MetadataCandidateView is one unified game of a search, with its best score [MET-008].
// Year is 0 when unknown; ThumbURL is absent when the provider has no small cover [MET-068].
type MetadataCandidateView struct {
	Refs     []MetadataRefView `json:"refs"`
	Title    string            `json:"title"`
	Year     int               `json:"year"`
	Platform string            `json:"platform"`
	Score    float64           `json:"score"`
	ThumbURL string            `json:"thumbUrl,omitempty"`
}

// MetadataSearchView is the result of MetadataSearch: the candidates, the confidence of the
// best one [MET-009] and the warnings of the providers.
type MetadataSearchView struct {
	Candidates []MetadataCandidateView `json:"candidates"`
	Confidence string                  `json:"confidence"`
	Warnings   []string                `json:"warnings"`
}

// MetadataApplyView is the game after the metadata was applied, with the images queued and
// the ones left out by the per-kind limit [MET-024].
type MetadataApplyView struct {
	Game           GameDetailView `json:"game"`
	Queued         int            `json:"queued"`
	SkippedByLimit int            `json:"skippedByLimit"`
}

// MetadataBatchStartView is the result of MetadataBatchStart.
type MetadataBatchStartView struct {
	BatchID int64 `json:"batchId"`
	Total   int   `json:"total"`
}

// MetadataBatchResultView is the summary of a finished batch application [MET-065, MET-066].
type MetadataBatchResultView struct {
	Applied              int      `json:"applied"`
	ImagesQueued         int      `json:"imagesQueued"`
	ImagesSkippedByLimit int      `json:"imagesSkippedByLimit"`
	FailedTitles         []string `json:"failedTitles"`
}

// MetadataBatchView is the state and counters of a batch. Result is set only in state done.
type MetadataBatchView struct {
	BatchID  int64                    `json:"batchId"`
	State    string                   `json:"state"`
	Total    int                      `json:"total"`
	Done     int                      `json:"done"`
	Failed   int                      `json:"failed"`
	Warnings []string                 `json:"warnings"`
	Result   *MetadataBatchResultView `json:"result,omitempty"`
}

// MetadataBatchStatusView is the result of MetadataBatchStatus; Batch is nil when there is no batch.
type MetadataBatchStatusView struct {
	Batch *MetadataBatchView `json:"batch"`
}

// MetadataBatchRowView is one game of a ready or cancelled batch. Selected is the index in
// Candidates of the chosen item, or -1 when Confidence is none [MET-061].
type MetadataBatchRowView struct {
	GameID     int64                   `json:"gameId"`
	Title      string                  `json:"title"`
	Platform   string                  `json:"platform"`
	Confidence string                  `json:"confidence"`
	Selected   int                     `json:"selected"`
	Candidates []MetadataCandidateView `json:"candidates"`
}

// MetadataBatchResultsView is one page of the rows of a batch; Next is nil on the last page.
type MetadataBatchResultsView struct {
	Rows  []MetadataBatchRowView `json:"rows"`
	Total int                    `json:"total"`
	Next  *int                   `json:"next"`
}

// MetadataChoiceView is the candidate the user picked for one game of a batch [MET-064].
type MetadataChoiceView struct {
	GameID    int64 `json:"gameId"`
	Candidate int   `json:"candidate"`
}

// refsWire converts the refs of the UI to the wire. The search and the apply use scrapeTimeout
// (the providers can be slow); the batch calls return at once and use libraryTimeout [MET-072].
func refsWire(in []MetadataRefView) []ipc.MetadataRef {
	out := make([]ipc.MetadataRef, 0, len(in))
	for _, r := range in {
		out = append(out, ipc.MetadataRef{Provider: r.Provider, ProviderID: r.ProviderID})
	}
	return out
}

func candidatesView(in []ipc.MetadataCandidate) []MetadataCandidateView {
	out := make([]MetadataCandidateView, 0, len(in))
	for _, c := range in {
		refs := make([]MetadataRefView, 0, len(c.Refs))
		for _, r := range c.Refs {
			refs = append(refs, MetadataRefView{Provider: r.Provider, ProviderID: r.ProviderID})
		}
		out = append(out, MetadataCandidateView{
			Refs: refs, Title: c.Title, Year: c.Year, Platform: c.Platform,
			Score: c.Score, ThumbURL: c.ThumbURL,
		})
	}
	return out
}

func batchView(b ipc.BatchStatus) *MetadataBatchView {
	v := &MetadataBatchView{
		BatchID: b.BatchID, State: b.State, Total: b.Total, Done: b.Done,
		Failed: b.Failed, Warnings: b.Warnings,
	}
	if b.Result != nil {
		v.Result = &MetadataBatchResultView{
			Applied: b.Result.Applied, ImagesQueued: b.Result.ImagesQueued,
			ImagesSkippedByLimit: b.Result.ImagesSkippedByLimit, FailedTitles: b.Result.FailedTitles,
		}
	}
	return v
}

// MetadataSearch searches every applicable provider at once and returns one scored list [MET-001..MET-009].
func (a *App) MetadataSearch(title, platform string, year int) (MetadataSearchView, error) {
	var res ipc.MetadataSearchResult
	params := ipc.MetadataSearchParams{Title: title, Platform: platform, Year: year}
	if err := a.callAgent(ipc.MethodMetadataSearch, scrapeTimeout, params, &res); err != nil {
		return MetadataSearchView{}, err
	}
	return MetadataSearchView{
		Candidates: candidatesView(res.Candidates), Confidence: res.Confidence, Warnings: res.Warnings,
	}, nil
}

// MetadataApply applies the text of one unified candidate and queues its images [MET-030].
func (a *App) MetadataApply(gameID int64, refs []MetadataRefView, replace bool) (MetadataApplyView, error) {
	var res ipc.MetadataApplyResult
	params := ipc.MetadataApplyParams{GameID: gameID, Refs: refsWire(refs), Replace: replace}
	if err := a.callAgent(ipc.MethodMetadataApply, scrapeTimeout, params, &res); err != nil {
		return MetadataApplyView{}, err
	}
	return MetadataApplyView{
		Game: gameDetailView(res.Detail), Queued: res.Queued, SkippedByLimit: res.SkippedByLimit,
	}, nil
}

// MetadataBatchStart starts the background search of the given games [MET-052].
func (a *App) MetadataBatchStart(gameIDs []int64) (MetadataBatchStartView, error) {
	var res ipc.BatchStartResult
	if err := a.callAgent(ipc.MethodMetadataBatchStart, libraryTimeout, ipc.BatchStartParams{GameIDs: gameIDs}, &res); err != nil {
		return MetadataBatchStartView{}, err
	}
	return MetadataBatchStartView{BatchID: res.BatchID, Total: res.Total}, nil
}

// MetadataBatchStatus returns the current batch, or Batch nil when there is none [MET-055].
func (a *App) MetadataBatchStatus() (MetadataBatchStatusView, error) {
	var res ipc.BatchStatusResult
	if err := a.callAgent(ipc.MethodMetadataBatchStatus, libraryTimeout, nil, &res); err != nil {
		return MetadataBatchStatusView{}, err
	}
	if res.Batch == nil {
		return MetadataBatchStatusView{}, nil
	}
	return MetadataBatchStatusView{Batch: batchView(*res.Batch)}, nil
}

// MetadataBatchResults returns one page of the rows of a ready or cancelled batch [MET-057].
func (a *App) MetadataBatchResults(batchID int64, offset, limit int) (MetadataBatchResultsView, error) {
	var res ipc.BatchResultsResult
	params := ipc.BatchResultsParams{BatchID: batchID, Offset: offset, Limit: limit}
	if err := a.callAgent(ipc.MethodMetadataBatchResults, libraryTimeout, params, &res); err != nil {
		return MetadataBatchResultsView{}, err
	}
	rows := make([]MetadataBatchRowView, 0, len(res.Rows))
	for _, r := range res.Rows {
		rows = append(rows, MetadataBatchRowView{
			GameID: r.GameID, Title: r.Title, Platform: r.Platform, Confidence: r.Confidence,
			Selected: r.Selected, Candidates: candidatesView(r.Candidates),
		})
	}
	return MetadataBatchResultsView{Rows: rows, Total: res.Total, Next: res.Next}, nil
}

// MetadataBatchApply accepts the user's choices and applies them in the background [MET-064].
// It returns how many choices the agent accepted.
func (a *App) MetadataBatchApply(batchID int64, choices []MetadataChoiceView) (int, error) {
	wire := make([]ipc.BatchChoice, 0, len(choices))
	for _, c := range choices {
		wire = append(wire, ipc.BatchChoice{GameID: c.GameID, Candidate: c.Candidate})
	}
	var res ipc.BatchApplyResult
	if err := a.callAgent(ipc.MethodMetadataBatchApply, libraryTimeout, ipc.BatchApplyParams{BatchID: batchID, Choices: wire}, &res); err != nil {
		return 0, err
	}
	return res.Accepted, nil
}

// MetadataBatchCancel cancels a search or an application and returns the new state [MET-056].
func (a *App) MetadataBatchCancel(batchID int64) (string, error) {
	var res ipc.BatchCancelResult
	if err := a.callAgent(ipc.MethodMetadataBatchCancel, libraryTimeout, ipc.BatchIDParams{BatchID: batchID}, &res); err != nil {
		return "", err
	}
	return res.State, nil
}

// MetadataBatchDiscard frees a ready, cancelled or done batch [MET-058].
func (a *App) MetadataBatchDiscard(batchID int64) error {
	var res ipc.BatchDiscardResult
	return a.callAgent(ipc.MethodMetadataBatchDiscard, libraryTimeout, ipc.BatchIDParams{BatchID: batchID}, &res)
}
