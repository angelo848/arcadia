package ipc

// Unified metadata methods (contract v0.14.0, phase 12 [MET-070]). Field names and
// JSON tags follow docs/specs/phase-12-metadados-unificados/contracts/ipc.openrpc.json.

// Values of MetadataRef.Provider.
const (
	MetadataProviderIGDB        = "igdb"
	MetadataProviderSteamGridDB = "steamgriddb"
	MetadataProviderSteamStore  = "steamstore"
)

// Values of MetadataConfidence (match, doubt, none) [MET-009].
const (
	MetadataConfidenceMatch = "match"
	MetadataConfidenceDoubt = "doubt"
	MetadataConfidenceNone  = "none"
)

// Values of BatchState (batch-state-machine.md §1).
const (
	BatchStateSearching = "searching"
	BatchStateReady     = "ready"
	BatchStateCancelled = "cancelled"
	BatchStateApplying  = "applying"
	BatchStateDone      = "done"
)

// MetadataRef identifies one provider entry of a candidate [MET-031].
type MetadataRef struct {
	Provider   string `json:"provider"`
	ProviderID string `json:"providerId"`
}

// MetadataCandidate is one unified game found by metadata.search (or a batch row) [MET-008].
// Year is 0 when unknown; Platform is "" when unknown; ThumbURL is omitted when absent [MET-068].
type MetadataCandidate struct {
	Refs     []MetadataRef `json:"refs"`
	Title    string        `json:"title"`
	Year     int           `json:"year"`
	Platform string        `json:"platform"`
	Score    float64       `json:"score"`
	ThumbURL string        `json:"thumbUrl,omitempty"`
}

// MetadataSearchParams are the params of metadata.search.
type MetadataSearchParams struct {
	Title    string `json:"title"`
	Platform string `json:"platform,omitempty"`
	Year     int    `json:"year,omitempty"`
}

// MetadataSearchResult is the result of metadata.search. Confidence is one of the
// MetadataConfidence* values [MET-009].
type MetadataSearchResult struct {
	Candidates []MetadataCandidate `json:"candidates"`
	Confidence string              `json:"confidence"`
	Warnings   []string            `json:"warnings"`
}

// MetadataApplyParams are the params of metadata.apply. Replace nil or false keeps
// the fields the user already filled [MET-030].
type MetadataApplyParams struct {
	GameID  int64         `json:"gameId"`
	Refs    []MetadataRef `json:"refs"`
	Replace bool          `json:"replace,omitempty"`
}

// MetadataApplyResult is the result of metadata.apply [MET-024].
type MetadataApplyResult struct {
	Detail         GameDetail `json:"detail"`
	Queued         int        `json:"queued"`
	SkippedByLimit int        `json:"skippedByLimit"`
}

// BatchStartParams are the params of metadata.batchStart (1..1000 distinct ids).
type BatchStartParams struct {
	GameIDs []int64 `json:"gameIds"`
}

// BatchStartResult is the result of metadata.batchStart.
type BatchStartResult struct {
	BatchID int64 `json:"batchId"`
	Total   int   `json:"total"`
}

// BatchResult is the summary of a finished batch application (state done) [MET-065, MET-066].
type BatchResult struct {
	Applied              int      `json:"applied"`
	ImagesQueued         int      `json:"imagesQueued"`
	ImagesSkippedByLimit int      `json:"imagesSkippedByLimit"`
	FailedTitles         []string `json:"failedTitles"`
}

// BatchStatus is the state and counters of a batch. Result is set only in state done.
type BatchStatus struct {
	BatchID  int64        `json:"batchId"`
	State    string       `json:"state"`
	Total    int          `json:"total"`
	Done     int          `json:"done"`
	Failed   int          `json:"failed"`
	Warnings []string     `json:"warnings"`
	Result   *BatchResult `json:"result,omitempty"`
}

// BatchStatusResult is the result of metadata.batchStatus; Batch is null when there is no batch.
type BatchStatusResult struct {
	Batch *BatchStatus `json:"batch"`
}

// BatchResultsParams are the params of metadata.batchResults. Offset and Limit are
// the values sent by the client (the defaults are applied by the agent).
type BatchResultsParams struct {
	BatchID int64 `json:"batchId"`
	Offset  int   `json:"offset,omitempty"`
	Limit   int   `json:"limit,omitempty"`
}

// BatchRow is one game of a ready or cancelled batch. Selected is the index in
// Candidates of the chosen item, or -1 when Confidence is none [MET-061].
type BatchRow struct {
	GameID     int64               `json:"gameId"`
	Title      string              `json:"title"`
	Platform   string              `json:"platform"`
	Confidence string              `json:"confidence"`
	Selected   int                 `json:"selected"`
	Candidates []MetadataCandidate `json:"candidates"`
}

// BatchResultsResult is the result of metadata.batchResults; Next is null on the last page.
type BatchResultsResult struct {
	Rows  []BatchRow `json:"rows"`
	Total int        `json:"total"`
	Next  *int       `json:"next"`
}

// BatchChoice is the candidate the user picked for one game of a batch [MET-064].
type BatchChoice struct {
	GameID    int64 `json:"gameId"`
	Candidate int   `json:"candidate"`
}

// BatchApplyParams are the params of metadata.batchApply.
type BatchApplyParams struct {
	BatchID int64         `json:"batchId"`
	Choices []BatchChoice `json:"choices"`
}

// BatchApplyResult is the result of metadata.batchApply.
type BatchApplyResult struct {
	Accepted int `json:"accepted"`
}

// BatchIDParams are the params of the methods that take only a batch id
// (metadata.batchCancel and metadata.batchDiscard).
type BatchIDParams struct {
	BatchID int64 `json:"batchId"`
}

// BatchCancelResult is the result of metadata.batchCancel.
type BatchCancelResult struct {
	State string `json:"state"`
}

// BatchDiscardResult is the result of metadata.batchDiscard.
type BatchDiscardResult struct {
	Discarded bool `json:"discarded"`
}

// MetadataBatchChangedParams are the params of the metadata.batchChanged notification
// [MET-054]. They carry only the state and counters, never game data (ADR 0031).
type MetadataBatchChangedParams struct {
	BatchID int64  `json:"batchId"`
	State   string `json:"state"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Failed  int    `json:"failed"`
}
