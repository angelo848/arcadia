package main

import (
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// StatsGameView is the time of one game in a period (EST-001, EST-005).
type StatsGameView struct {
	GameID          int64  `json:"gameId"`
	Title           string `json:"title"`
	Platform        string `json:"platform"`
	Sessions        int64  `json:"sessions"`
	ActiveSeconds   int64  `json:"activeSeconds"`
	ImportedSeconds int64  `json:"importedSeconds"`
	LastPlayedAt    string `json:"lastPlayedAt"`
}

// StatsPlatformView is the time of one platform in a period (EST-003).
type StatsPlatformView struct {
	PlatformID      int64  `json:"platformId"`
	Name            string `json:"name"`
	ActiveSeconds   int64  `json:"activeSeconds"`
	ImportedSeconds int64  `json:"importedSeconds"`
}

// StatsSummaryView is the result of StatsSummary.
type StatsSummaryView struct {
	Games           []StatsGameView     `json:"games"`
	Platforms       []StatsPlatformView `json:"platforms"`
	TotalSeconds    int64               `json:"totalSeconds"`
	ImportedSeconds int64               `json:"importedSeconds"`
}

// StatsWeekView is the time of one week (EST-002).
type StatsWeekView struct {
	Start   string `json:"start"`
	Seconds int64  `json:"seconds"`
	Current bool   `json:"current"`
}

// StatsWeeklyView is the result of StatsWeekly.
type StatsWeeklyView struct {
	Weeks []StatsWeekView `json:"weeks"`
}

// StatsSummary returns the time per game and per platform for a period: "7d", "30d", "365d" or "all" (EST-004).
func (a *App) StatsSummary(period string) (StatsSummaryView, error) {
	var res ipc.StatsSummaryResult
	if err := a.callAgent(ipc.MethodStatsSummary, libraryTimeout, ipc.StatsSummaryParams{Period: period}, &res); err != nil {
		return StatsSummaryView{}, err
	}
	out := StatsSummaryView{
		Games:           make([]StatsGameView, 0, len(res.Games)),
		Platforms:       make([]StatsPlatformView, 0, len(res.Platforms)),
		TotalSeconds:    res.TotalSeconds,
		ImportedSeconds: res.ImportedSeconds,
	}
	for _, g := range res.Games {
		out.Games = append(out.Games, StatsGameView{GameID: g.GameID, Title: g.Title, Platform: g.Platform,
			Sessions: g.Sessions, ActiveSeconds: g.ActiveSeconds, ImportedSeconds: g.ImportedSeconds, LastPlayedAt: g.LastPlayedAt})
	}
	for _, p := range res.Platforms {
		out.Platforms = append(out.Platforms, StatsPlatformView{PlatformID: p.PlatformID, Name: p.Name,
			ActiveSeconds: p.ActiveSeconds, ImportedSeconds: p.ImportedSeconds})
	}
	return out, nil
}

// StatsWeekly returns the time per week for the last weeks, Monday first (EST-002).
func (a *App) StatsWeekly(weeks int) (StatsWeeklyView, error) {
	var res ipc.StatsWeeklyResult
	if err := a.callAgent(ipc.MethodStatsWeekly, libraryTimeout, ipc.StatsWeeklyParams{Weeks: weeks}, &res); err != nil {
		return StatsWeeklyView{}, err
	}
	out := StatsWeeklyView{Weeks: make([]StatsWeekView, 0, len(res.Weeks))}
	for _, w := range res.Weeks {
		out.Weeks = append(out.Weeks, StatsWeekView{Start: w.Start, Seconds: w.Seconds, Current: w.Current})
	}
	return out, nil
}
