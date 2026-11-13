package ipc

// Statistics methods (phase 8, contract v0.10.0 [EST-001..EST-006]). Both only read the database.
const (
	MethodStatsSummary = "stats.summary"
	MethodStatsWeekly  = "stats.weekly"
)

// StatsSummaryParams are the params of stats.summary. Period is "7d", "30d", "365d" or "all".
type StatsSummaryParams struct {
	Period string `json:"period"`
}

// StatsGame is the time of one game in the period (EST-001, EST-005).
type StatsGame struct {
	GameID          int64  `json:"gameId"`
	Title           string `json:"title"`
	Platform        string `json:"platform"`
	Sessions        int64  `json:"sessions"`
	ActiveSeconds   int64  `json:"activeSeconds"`
	ImportedSeconds int64  `json:"importedSeconds"`
	LastPlayedAt    string `json:"lastPlayedAt"`
}

// StatsPlatform is the time of one platform in the period (EST-003).
type StatsPlatform struct {
	PlatformID      int64  `json:"platformId"`
	Name            string `json:"name"`
	ActiveSeconds   int64  `json:"activeSeconds"`
	ImportedSeconds int64  `json:"importedSeconds"`
}

// StatsSummaryResult is the result of stats.summary.
type StatsSummaryResult struct {
	Games           []StatsGame     `json:"games"`
	Platforms       []StatsPlatform `json:"platforms"`
	TotalSeconds    int64           `json:"totalSeconds"`
	ImportedSeconds int64           `json:"importedSeconds"`
}

// StatsWeeklyParams are the params of stats.weekly.
type StatsWeeklyParams struct {
	Weeks int `json:"weeks"`
}

// StatsWeek is the time of one week (EST-002). Start is the Monday, YYYY-MM-DD, local time.
type StatsWeek struct {
	Start   string `json:"start"`
	Seconds int64  `json:"seconds"`
	Current bool   `json:"current"`
}

// StatsWeeklyResult is the result of stats.weekly.
type StatsWeeklyResult struct {
	Weeks []StatsWeek `json:"weeks"`
}
