package agent

import "time"

// Constants of the unified metadata service (phase 12). They are the agent's part of the x-metadata
// block of contract v0.14.0 (ipc-v0.14.0-delta.md §1); the contract tests compare them with the JSON
// [MET-070]. The request intervals of the providers live in scrapers (IntervalIGDB and the others).
const (
	searchTimeout       = 20 * time.Second // deadline of a single search [MET-001]
	gameSearchTimeout   = 15 * time.Second // deadline per game of a batch search [MET-001]
	maxSearchCandidates = 10               // candidates returned by metadata.search [MET-008]
	maxRowCandidates    = 5                // candidates kept per batch row
	batchWorkers        = 3                // parallel searches of a batch
	maxBatchGames       = 1000             // games per batch
	maxBatchChoices     = 1000             // choices per metadata.batchApply
	defaultResultsLimit = 100              // default page of metadata.batchResults
	maxResultsLimit     = 200              // largest page of metadata.batchResults
	maxResultBytes      = 900 << 10        // 921600: a result stays under this [ADR 0032]
	progressWindow      = 500 * time.Millisecond
	providerMaxFailures = 3  // consecutive failures that suspend a provider in a batch [MET-011]
	maxStatusWarnings   = 10 // warnings returned by metadata.batchStatus
	maxFailedTitles     = 10 // failed titles returned in the batch result
)

// providerCooldown is how long a provider stays suspended after each run of providerMaxFailures
// failures in a batch: 30 s, then 60 s, then 120 s [MET-011].
var providerCooldown = []time.Duration{30 * time.Second, 60 * time.Second, 120 * time.Second}
