package store_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

func TestEndSessionClosesPause(t *testing.T) {
	t.Parallel()

	pausedAt := sesStart.Add(100*time.Second + 250*time.Millisecond)

	t.Run("STO-021/E > P: o intervalo fica com resumed_at = E", func(t *testing.T) {
		t.Parallel()
		for _, reason := range []domain.SessionEndReason{domain.EndExited, domain.EndAgentShutdown, domain.EndRecovered} {
			s, id := pauseSetup(t)
			mustPause(t, s, store.PauseStart{SessionID: id, At: pausedAt, Reason: domain.PauseHotkey, Active: 100 * time.Second})
			endAt := pausedAt.Add(400*time.Second + 321*time.Millisecond)
			mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: 100 * time.Second, Reason: reason, WrittenAt: endAt})

			rows := readPauseRows(t, s.DB())
			want := pauseRow{SessionID: id, PausedAt: pausedAt.UnixMilli(),
				ResumedAt: sql.NullInt64{Int64: endAt.UnixMilli(), Valid: true}, Reason: "hotkey"}
			if len(rows) != 1 || rows[0] != want {
				t.Errorf("reason %q: pause rows = %+v, want [%+v]", reason, rows, want)
			}
			if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM pause_interval WHERE resumed_at IS NULL`); n != 0 {
				t.Errorf("reason %q: open pauses = %d, want 0", reason, n)
			}
		}
	})

	t.Run("STO-021/E <= P: o intervalo é removido", func(t *testing.T) {
		t.Parallel()
		for name, endAt := range map[string]time.Time{
			"E == P": pausedAt,
			"E < P":  pausedAt.Add(-time.Second),
		} {
			s, id := pauseSetup(t)
			mustPause(t, s, store.PauseStart{SessionID: id, At: pausedAt, Reason: domain.PauseGamepad, Active: 100 * time.Second})
			mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: 100 * time.Second, Reason: domain.EndRecovered, WrittenAt: endAt})

			if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM pause_interval`); n != 0 {
				t.Errorf("%s: pause_interval rows = %d, want 0", name, n)
			}
			if r := readSessionRow(t, s.DB(), id); r.State != "ended" || !r.EndedAt.Valid {
				t.Errorf("%s: session = %+v, want ended", name, r)
			}
		}
	})

	t.Run("STO-021/descartada: intervalos continuam gravados", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		p1 := sesStart.Add(5 * time.Second)
		r1 := sesStart.Add(8 * time.Second)
		mustPause(t, s, store.PauseStart{SessionID: id, At: p1, Reason: domain.PauseHotkey, Active: 5 * time.Second})
		if err := s.ResumePause(t.Context(), store.PauseEnd{SessionID: id, At: r1, State: domain.SessionRunning, Active: 5 * time.Second}); err != nil {
			t.Fatalf("ResumePause: %v", err)
		}
		p2 := sesStart.Add(12 * time.Second)
		mustPause(t, s, store.PauseStart{SessionID: id, At: p2, Reason: domain.PauseSleep, Active: 9 * time.Second})
		endAt := sesStart.Add(20 * time.Second)

		st := mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: 9 * time.Second, Reason: domain.EndExited, WrittenAt: endAt})
		if st != domain.SessionDiscarded {
			t.Fatalf("EndSession state = %q, want %q", st, domain.SessionDiscarded)
		}
		rows := readPauseRows(t, s.DB())
		want := []pauseRow{
			{SessionID: id, PausedAt: p1.UnixMilli(), ResumedAt: sql.NullInt64{Int64: r1.UnixMilli(), Valid: true}, Reason: "hotkey"},
			{SessionID: id, PausedAt: p2.UnixMilli(), ResumedAt: sql.NullInt64{Int64: endAt.UnixMilli(), Valid: true}, Reason: "sleep"},
		}
		if len(rows) != len(want) || rows[0] != want[0] || rows[1] != want[1] {
			t.Errorf("pause rows = %+v, want %+v", rows, want)
		}
	})

	t.Run("STO-021/falha ao fechar o intervalo desfaz o fim da sessão e do jogo", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		g := readSessionRow(t, s.DB(), id).GameID
		mustPause(t, s, store.PauseStart{SessionID: id, At: pausedAt, Reason: domain.PauseHotkey, Active: 100 * time.Second})
		if _, err := s.DB().ExecContext(t.Context(), `CREATE TRIGGER test_abort_pause_update BEFORE UPDATE ON pause_interval
			BEGIN SELECT RAISE(ABORT, 'test: pause update aborted'); END`); err != nil {
			t.Fatalf("create trigger: %v", err)
		}
		sesBefore := readSessionRow(t, s.DB(), id)
		gameBefore := readGameRow(t, s.DB(), g)
		pausesBefore := readPauseRows(t, s.DB())

		endAt := pausedAt.Add(time.Minute)
		_, err := s.EndSession(t.Context(), store.SessionEnd{ID: id, EndedAt: endAt, Active: 100 * time.Second, Reason: domain.EndExited, WrittenAt: endAt})
		if err == nil {
			t.Fatal("EndSession with aborting trigger: err = nil, want an error")
		}
		if after := readSessionRow(t, s.DB(), id); after != sesBefore {
			t.Errorf("session changed: %+v -> %+v", sesBefore, after)
		}
		if after := readGameRow(t, s.DB(), g); after != gameBefore {
			t.Errorf("game changed: %+v -> %+v", gameBefore, after)
		}
		after := readPauseRows(t, s.DB())
		if len(after) != 1 || after[0] != pausesBefore[0] {
			t.Errorf("pause rows = %+v, want unchanged %+v", after, pausesBefore)
		}
		open, err := s.OpenSessions(t.Context())
		if err != nil {
			t.Fatalf("OpenSessions: %v", err)
		}
		if len(open) != 1 || open[0].ID != id {
			t.Errorf("OpenSessions = %+v, want session %d still open", open, id)
		}
	})

	t.Run("STO-021/sem pausa aberta o comportamento de STO-014 e STO-015 não muda", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		g := readSessionRow(t, s.DB(), id).GameID
		p1 := sesStart.Add(10 * time.Second)
		r1 := sesStart.Add(20 * time.Second)
		mustPause(t, s, store.PauseStart{SessionID: id, At: p1, Reason: domain.PauseUI, Active: 10 * time.Second})
		if err := s.ResumePause(t.Context(), store.PauseEnd{SessionID: id, At: r1, State: domain.SessionRunning, Active: 10 * time.Second}); err != nil {
			t.Fatalf("ResumePause: %v", err)
		}
		endAt := sesStart.Add(2 * time.Minute)
		st := mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: 95*time.Second + 999*time.Millisecond, Reason: domain.EndExited, WrittenAt: endAt})
		if st != domain.SessionEnded {
			t.Errorf("EndSession state = %q, want ended", st)
		}
		r := readSessionRow(t, s.DB(), id)
		if r.State != "ended" || r.ActiveSeconds != 95 || r.EndedAt.Int64 != endAt.UnixMilli() {
			t.Errorf("session = %+v, want ended, active 95, ended_at %d", r, endAt.UnixMilli())
		}
		gr := readGameRow(t, s.DB(), g)
		if gr.TotalPlaySeconds != 95 || gr.PlayCount != 1 {
			t.Errorf("game total/count = %d/%d, want 95/1", gr.TotalPlaySeconds, gr.PlayCount)
		}
		rows := readPauseRows(t, s.DB())
		if len(rows) != 1 || rows[0].ResumedAt.Int64 != r1.UnixMilli() {
			t.Errorf("closed pause changed: %+v", rows)
		}

		// A session that never paused also ends as before.
		s2, id2 := pauseSetup(t)
		mustEndSession(t, s2, store.SessionEnd{ID: id2, EndedAt: endAt, Active: 60 * time.Second, Reason: domain.EndExited, WrittenAt: endAt})
		if n := countRows(t, s2.DB(), `SELECT COUNT(*) FROM pause_interval`); n != 0 {
			t.Errorf("pause_interval rows = %d, want 0", n)
		}
	})
}

func TestActiveDurationFromPauses(t *testing.T) {
	t.Parallel()

	t.Run("STO-023/sessão de 300 s com pausas [100,160] e [200,210] vale 230 e bate com active_seconds", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		start := sesStart
		end := start.Add(300 * time.Second)
		p1, r1 := start.Add(100*time.Second), start.Add(160*time.Second)
		p2, r2 := start.Add(200*time.Second), start.Add(210*time.Second)

		mustPause(t, s, store.PauseStart{SessionID: id, At: p1, Reason: domain.PauseHotkey, Active: 100 * time.Second})
		if err := s.ResumePause(t.Context(), store.PauseEnd{SessionID: id, At: r1, State: domain.SessionRunning, Active: 100 * time.Second}); err != nil {
			t.Fatalf("ResumePause 1: %v", err)
		}
		mustPause(t, s, store.PauseStart{SessionID: id, At: p2, Reason: domain.PauseGamepad, Active: 140 * time.Second})
		if err := s.ResumePause(t.Context(), store.PauseEnd{SessionID: id, At: r2, State: domain.SessionRunning, Active: 140 * time.Second}); err != nil {
			t.Fatalf("ResumePause 2: %v", err)
		}
		mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: end, Active: 230 * time.Second, Reason: domain.EndExited, WrittenAt: end})

		pauses, err := s.Pauses(t.Context(), id)
		if err != nil {
			t.Fatalf("Pauses: %v", err)
		}
		intervals := make([]domain.Interval, 0, len(pauses))
		for _, p := range pauses {
			intervals = append(intervals, domain.Interval{Start: p.PausedAt, End: p.ResumedAt})
		}
		r := readSessionRow(t, s.DB(), id)
		got := int64(domain.ActiveDuration(start, end, intervals) / time.Second)
		if got != 230 {
			t.Errorf("floor(ActiveDuration) = %d, want 230", got)
		}
		if got != r.ActiveSeconds {
			t.Errorf("ActiveDuration = %d, active_seconds = %d, want equal", got, r.ActiveSeconds)
		}
	})
}
