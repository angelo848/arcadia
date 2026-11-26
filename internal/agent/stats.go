package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// statsStore is the read side of the store the statistics use (EST-006): the handlers can read the
// totals and nothing else, so they cannot write the database.
type statsStore interface {
	StatsByGame(ctx context.Context, since time.Time) ([]store.GameStat, error)
	StatsByPlatform(ctx context.Context, since time.Time) ([]store.PlatformStat, error)
	SessionTimes(ctx context.Context, since time.Time) ([]store.SessionTime, error)
}

// statsDays maps the period names of the contract to days; "all" is zero, meaning no start.
var statsDays = map[string]int{"7d": 7, "30d": 30, "365d": 365}

// statsHandlers returns the handlers of stats.summary and stats.weekly (phase 8).
func statsHandlers(st statsStore, clock domain.Clock, loc *time.Location, log *slog.Logger) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodStatsSummary: func(ctx context.Context, params json.RawMessage) (any, error) {
			var p ipc.StatsSummaryParams
			if e := decodeOrInvalid(params, &p); e != nil {
				return nil, e
			}
			since, err := periodStart(p.Period, clock.Now().In(loc))
			if err != nil {
				return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
			}
			games, err := st.StatsByGame(ctx, since)
			if err != nil {
				log.Error("estatísticas por jogo", "error", err.Error())
				return nil, ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
			}
			platforms, err := st.StatsByPlatform(ctx, since)
			if err != nil {
				log.Error("estatísticas por plataforma", "error", err.Error())
				return nil, ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
			}
			return summaryResult(games, platforms), nil
		},
		ipc.MethodStatsWeekly: func(ctx context.Context, params json.RawMessage) (any, error) {
			var p ipc.StatsWeeklyParams
			if e := decodeOrInvalid(params, &p); e != nil {
				return nil, e
			}
			if p.Weeks < 1 || p.Weeks > 52 {
				return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
			}
			now := clock.Now().In(loc)
			// One extra week of margin: WeeklyTotals drops what falls before the first week itself.
			since := now.AddDate(0, 0, -7*(p.Weeks+1))
			rows, err := st.SessionTimes(ctx, since)
			if err != nil {
				log.Error("sessões do período", "error", err.Error())
				return nil, ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
			}
			sessions := make([]domain.TimedSeconds, 0, len(rows))
			for _, r := range rows {
				sessions = append(sessions, domain.TimedSeconds{Start: r.StartedAt, Seconds: r.ActiveSeconds})
			}
			totals := domain.WeeklyTotals(sessions, now, loc, p.Weeks)
			out := ipc.StatsWeeklyResult{Weeks: make([]ipc.StatsWeek, 0, len(totals))}
			for _, w := range totals {
				out.Weeks = append(out.Weeks, ipc.StatsWeek{Start: w.Start.Format("2006-01-02"), Seconds: w.Seconds, Current: w.Current})
			}
			return out, nil
		},
	}
}

// periodStart returns the instant a period starts, counted back from now; "all" gives the zero time.
func periodStart(period string, now time.Time) (time.Time, error) {
	if period == "all" {
		return time.Time{}, nil
	}
	days, ok := statsDays[period]
	if !ok {
		return time.Time{}, fmt.Errorf("período desconhecido %q", period)
	}
	return now.AddDate(0, 0, -days), nil
}

// summaryResult converts the store rows to the contract.
func summaryResult(games []store.GameStat, platforms []store.PlatformStat) ipc.StatsSummaryResult {
	out := ipc.StatsSummaryResult{Games: make([]ipc.StatsGame, 0, len(games)), Platforms: make([]ipc.StatsPlatform, 0, len(platforms))}
	for _, g := range games {
		last := ""
		if !g.LastStartedAt.IsZero() {
			last = g.LastStartedAt.UTC().Format(time.RFC3339)
		}
		out.Games = append(out.Games, ipc.StatsGame{
			GameID: g.GameID, Title: g.Title, Platform: g.Platform, Sessions: g.Sessions,
			ActiveSeconds: g.ActiveSeconds, ImportedSeconds: g.ImportedSeconds, ManualSeconds: g.ManualSeconds, LastPlayedAt: last,
		})
		out.TotalSeconds += g.ActiveSeconds
		out.ImportedSeconds += g.ImportedSeconds
		out.ManualSeconds += g.ManualSeconds
	}
	for _, p := range platforms {
		out.Platforms = append(out.Platforms, ipc.StatsPlatform{
			PlatformID: p.PlatformID, Name: p.Name, ActiveSeconds: p.ActiveSeconds, ImportedSeconds: p.ImportedSeconds,
			ManualSeconds: p.ManualSeconds,
		})
	}
	return out
}
