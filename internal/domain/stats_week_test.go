package domain

import (
	"testing"
	"time"
)

// brt is a fixed zone (UTC-3), so the tests do not depend on the machine's zone.
var brt = time.FixedZone("UTC-3", -3*3600)

func TestWeeklyTotals(t *testing.T) {
	t.Parallel()

	// Friday 13 Nov 2026 (local): the current week starts Monday 9 Nov.
	now := time.Date(2026, 11, 13, 15, 0, 0, 0, brt)

	t.Run("EST-002/12 semanas, a última é a atual, semanas sem tempo ficam em zero", func(t *testing.T) {
		t.Parallel()
		got := WeeklyTotals(nil, now, brt, 12)
		if len(got) != 12 {
			t.Fatalf("weeks = %d, want 12", len(got))
		}
		if !got[11].Current || got[10].Current {
			t.Errorf("current marks = %v %v, want only the last", got[10].Current, got[11].Current)
		}
		if got[11].Start.Format("2006-01-02") != "2026-11-09" || got[0].Start.Format("2006-01-02") != "2026-08-24" {
			t.Errorf("range %s .. %s, want 2026-08-24 .. 2026-11-09", got[0].Start.Format("2006-01-02"), got[11].Start.Format("2006-01-02"))
		}
	})

	t.Run("EST-002/semana começa na segunda: domingo à noite conta na semana anterior", func(t *testing.T) {
		t.Parallel()
		sessions := []TimedSeconds{
			{Start: time.Date(2026, 11, 8, 23, 30, 0, 0, brt), Seconds: 300}, // Sunday, previous week
			{Start: time.Date(2026, 11, 9, 0, 10, 0, 0, brt), Seconds: 600},  // Monday, current week
		}
		got := WeeklyTotals(sessions, now, brt, 12)
		if got[10].Seconds != 300 || got[11].Seconds != 600 {
			t.Errorf("previous %d, current %d; want 300 and 600", got[10].Seconds, got[11].Seconds)
		}
	})

	t.Run("EST-002/sessão antiga e futura não entram nas semanas", func(t *testing.T) {
		t.Parallel()
		sessions := []TimedSeconds{
			{Start: time.Date(2026, 8, 23, 23, 0, 0, 0, brt), Seconds: 999}, // before the first week
			{Start: time.Date(2026, 11, 16, 9, 0, 0, 0, brt), Seconds: 999}, // next week
		}
		for _, w := range WeeklyTotals(sessions, now, brt, 12) {
			if w.Seconds != 0 {
				t.Errorf("week %s = %d, want 0", w.Start.Format("2006-01-02"), w.Seconds)
			}
		}
	})

	t.Run("EST-002/zero semanas não devolve nada", func(t *testing.T) {
		t.Parallel()
		if got := WeeklyTotals(nil, now, brt, 0); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}
