package main

import (
	"encoding/json"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// TestStatsBindings checks that the statistics bindings send the period and return the totals (EST-004).
func TestStatsBindings(t *testing.T) {
	t.Parallel()

	t.Run("EST-004/StatsSummary envia o período e devolve os totais", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodStatsSummary: ipc.StatsSummaryResult{
			Games:        []ipc.StatsGame{{GameID: 3, Title: "Celeste", ActiveSeconds: 900}},
			TotalSeconds: 900,
		}}}
		a := &App{client: p, log: discardLog()}
		got, err := a.StatsSummary("7d")
		if err != nil || got.TotalSeconds != 900 || len(got.Games) != 1 || got.Games[0].Title != "Celeste" {
			t.Fatalf("StatsSummary = %+v, %v", got, err)
		}
		c := singleCall(t, p, ipc.MethodStatsSummary, libraryTimeout)
		var sent ipc.StatsSummaryParams
		if err := json.Unmarshal(c.params, &sent); err != nil || sent.Period != "7d" {
			t.Errorf("params = %s, %v", c.params, err)
		}
	})

	t.Run("EST-002/StatsWeekly envia as semanas e devolve a atual marcada", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodStatsWeekly: ipc.StatsWeeklyResult{
			Weeks: []ipc.StatsWeek{{Start: "2026-11-09", Seconds: 600, Current: true}},
		}}}
		a := &App{client: p, log: discardLog()}
		got, err := a.StatsWeekly(12)
		if err != nil || len(got.Weeks) != 1 || !got.Weeks[0].Current {
			t.Errorf("StatsWeekly = %+v, %v", got, err)
		}
	})
}
