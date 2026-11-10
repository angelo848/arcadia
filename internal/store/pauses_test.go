package store_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

type pauseRow struct {
	SessionID, PausedAt int64
	ResumedAt           sql.NullInt64
	Reason              string
}

func readPauseRows(t *testing.T, db *sql.DB) []pauseRow {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT session_id, paused_at, resumed_at, reason FROM pause_interval ORDER BY id`)
	if err != nil {
		t.Fatalf("read pause_interval: %v", err)
	}
	defer rows.Close()
	var out []pauseRow
	for rows.Next() {
		var r pauseRow
		if err := rows.Scan(&r.SessionID, &r.PausedAt, &r.ResumedAt, &r.Reason); err != nil {
			t.Fatalf("scan pause_interval: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate pause_interval: %v", err)
	}
	return out
}

func pauseSetup(t *testing.T) (*store.Store, int64) {
	t.Helper()
	s := openStore(t, dbPath(t), true)
	g := newGame(t, s, "Hades")
	return s, mustStartSession(t, s, g.ID, sesStart)
}

func mustPause(t *testing.T, s *store.Store, p store.PauseStart) {
	t.Helper()
	if err := s.PauseSession(t.Context(), p); err != nil {
		t.Fatalf("PauseSession(%+v): %v", p, err)
	}
}

func TestPauseSession(t *testing.T) {
	t.Parallel()
	at := sesStart.Add(100*time.Second + 456*time.Millisecond)

	t.Run("STO-019/grava intervalo aberto com reason e a sessão com state paused, last_seen_at, active_seconds=floor(a) e ended_at nulo", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		mustPause(t, s, store.PauseStart{SessionID: id, At: at, Reason: domain.PauseHotkey, Active: 99*time.Second + 900*time.Millisecond})

		rows := readPauseRows(t, s.DB())
		want := pauseRow{SessionID: id, PausedAt: at.UnixMilli(), Reason: "hotkey"}
		if len(rows) != 1 || rows[0] != want {
			t.Fatalf("pause rows = %+v, want [%+v] (resumed_at NULL)", rows, want)
		}
		r := readSessionRow(t, s.DB(), id)
		if r.State != "paused" || r.LastSeenAt != at.UnixMilli() || r.ActiveSeconds != 99 || r.EndedAt.Valid || r.EndReason.Valid {
			t.Errorf("session = %+v, want paused, last_seen_at %d, active 99, ended_at NULL", r, at.UnixMilli())
		}
	})

	t.Run("STO-019/segunda pausa aberta devolve ErrPauseOpen e nada muda", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		mustPause(t, s, store.PauseStart{SessionID: id, At: at, Reason: domain.PauseHotkey, Active: 50 * time.Second})
		beforeSession := readSessionRow(t, s.DB(), id)
		beforePauses := readPauseRows(t, s.DB())

		err := s.PauseSession(t.Context(), store.PauseStart{
			SessionID: id, At: at.Add(time.Minute), Reason: domain.PauseGamepad, Active: 70 * time.Second})
		if !errors.Is(err, store.ErrPauseOpen) {
			t.Fatalf("second PauseSession error = %v, want ErrPauseOpen", err)
		}
		if got := readSessionRow(t, s.DB(), id); got != beforeSession {
			t.Errorf("session changed: %+v, want %+v", got, beforeSession)
		}
		got := readPauseRows(t, s.DB())
		if len(got) != len(beforePauses) || got[0] != beforePauses[0] {
			t.Errorf("pauses changed: %+v, want %+v", got, beforePauses)
		}
	})

	t.Run("STO-019/sessão inexistente devolve ErrSessionNotOpen sem criar linha", func(t *testing.T) {
		t.Parallel()
		s, _ := pauseSetup(t)
		err := s.PauseSession(t.Context(), store.PauseStart{SessionID: 9999, At: at, Reason: domain.PauseHotkey})
		if !errors.Is(err, store.ErrSessionNotOpen) {
			t.Fatalf("error = %v, want ErrSessionNotOpen", err)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM pause_interval`); n != 0 {
			t.Errorf("pause_interval rows = %d, want 0", n)
		}
	})

	t.Run("STO-019/sessão já encerrada devolve ErrSessionNotOpen sem criar linha", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		end := sesStart.Add(time.Hour)
		mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: end, Active: time.Hour, Reason: domain.EndExited, WrittenAt: end})
		before := readSessionRow(t, s.DB(), id)

		err := s.PauseSession(t.Context(), store.PauseStart{SessionID: id, At: at, Reason: domain.PauseHotkey})
		if !errors.Is(err, store.ErrSessionNotOpen) {
			t.Fatalf("error = %v, want ErrSessionNotOpen", err)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM pause_interval`); n != 0 {
			t.Errorf("pause_interval rows = %d, want 0", n)
		}
		if got := readSessionRow(t, s.DB(), id); got != before {
			t.Errorf("ended session changed: %+v, want %+v", got, before)
		}
	})

	t.Run("STO-019/falha no INSERT desfaz o UPDATE da sessão", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		if _, err := s.DB().ExecContext(t.Context(), `CREATE TRIGGER fail_pause_insert BEFORE INSERT ON pause_interval
			BEGIN SELECT RAISE(ABORT, 'forced failure'); END`); err != nil {
			t.Fatalf("create trigger: %v", err)
		}
		before := readSessionRow(t, s.DB(), id)

		err := s.PauseSession(t.Context(), store.PauseStart{SessionID: id, At: at, Reason: domain.PauseHotkey, Active: 42 * time.Second})
		if err == nil {
			t.Fatal("PauseSession succeeded, want the forced INSERT error")
		}
		if got := readSessionRow(t, s.DB(), id); got != before {
			t.Errorf("session = %+v, want unchanged %+v (UPDATE rolled back)", got, before)
		}
	})
}

func TestResumePause(t *testing.T) {
	t.Parallel()
	pausedAt := sesStart.Add(100*time.Second + 250*time.Millisecond)
	resumedAt := pausedAt.Add(40*time.Second + 500*time.Millisecond)

	t.Run("STO-020/fecha o intervalo em R e grava state running, last_seen_at e active_seconds", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		mustPause(t, s, store.PauseStart{SessionID: id, At: pausedAt, Reason: domain.PauseGamepad, Active: 100 * time.Second})

		if err := s.ResumePause(t.Context(), store.PauseEnd{
			SessionID: id, At: resumedAt, State: domain.SessionRunning, Active: 100*time.Second + 999*time.Millisecond}); err != nil {
			t.Fatalf("ResumePause: %v", err)
		}
		rows := readPauseRows(t, s.DB())
		want := pauseRow{SessionID: id, PausedAt: pausedAt.UnixMilli(),
			ResumedAt: sql.NullInt64{Int64: resumedAt.UnixMilli(), Valid: true}, Reason: "gamepad"}
		if len(rows) != 1 || rows[0] != want {
			t.Errorf("pause rows = %+v, want [%+v]", rows, want)
		}
		r := readSessionRow(t, s.DB(), id)
		if r.State != "running" || r.LastSeenAt != resumedAt.UnixMilli() || r.ActiveSeconds != 100 || r.EndedAt.Valid {
			t.Errorf("session = %+v, want running, last_seen_at %d, active 100, ended_at NULL", r, resumedAt.UnixMilli())
		}
	})

	t.Run("STO-020/state grace é gravado", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		mustPause(t, s, store.PauseStart{SessionID: id, At: pausedAt, Reason: domain.PauseHotkey, Active: 100 * time.Second})
		if err := s.ResumePause(t.Context(), store.PauseEnd{
			SessionID: id, At: resumedAt, State: domain.SessionGrace, Active: 100 * time.Second}); err != nil {
			t.Fatalf("ResumePause: %v", err)
		}
		if r := readSessionRow(t, s.DB(), id); r.State != "grace" {
			t.Errorf("state = %q, want grace", r.State)
		}
		rows := readPauseRows(t, s.DB())
		if len(rows) != 1 || !rows[0].ResumedAt.Valid {
			t.Errorf("pause rows = %+v, want one closed interval", rows)
		}
	})

	t.Run("STO-020/R igual a P em ms remove o intervalo", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		mustPause(t, s, store.PauseStart{SessionID: id, At: pausedAt, Reason: domain.PauseHotkey, Active: 100 * time.Second})
		if err := s.ResumePause(t.Context(), store.PauseEnd{
			SessionID: id, At: pausedAt, State: domain.SessionRunning, Active: 100 * time.Second}); err != nil {
			t.Fatalf("ResumePause: %v", err)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM pause_interval`); n != 0 {
			t.Errorf("pause_interval rows = %d, want 0 (zero-length interval removed)", n)
		}
		r := readSessionRow(t, s.DB(), id)
		if r.State != "running" || r.LastSeenAt != pausedAt.UnixMilli() {
			t.Errorf("session = %+v, want running with last_seen_at %d", r, pausedAt.UnixMilli())
		}
	})

	t.Run("STO-020/sem pausa aberta só atualiza a sessão e não dá erro", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		if err := s.ResumePause(t.Context(), store.PauseEnd{
			SessionID: id, At: resumedAt, State: domain.SessionGrace, Active: 30*time.Second + 700*time.Millisecond}); err != nil {
			t.Fatalf("ResumePause without open pause: %v", err)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM pause_interval`); n != 0 {
			t.Errorf("pause_interval rows = %d, want 0", n)
		}
		r := readSessionRow(t, s.DB(), id)
		if r.State != "grace" || r.LastSeenAt != resumedAt.UnixMilli() || r.ActiveSeconds != 30 {
			t.Errorf("session = %+v, want grace, last_seen_at %d, active 30", r, resumedAt.UnixMilli())
		}
	})

	t.Run("STO-020/sessão encerrada devolve ErrSessionNotOpen", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		end := sesStart.Add(time.Hour)
		mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: end, Active: time.Hour, Reason: domain.EndExited, WrittenAt: end})
		err := s.ResumePause(t.Context(), store.PauseEnd{
			SessionID: id, At: end.Add(time.Second), State: domain.SessionRunning, Active: time.Hour})
		if !errors.Is(err, store.ErrSessionNotOpen) {
			t.Fatalf("error = %v, want ErrSessionNotOpen", err)
		}
	})
}

func TestPauses(t *testing.T) {
	t.Parallel()

	t.Run("STO-022/ordenadas por paused_at com ResumedAt zero quando aberto", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		p1 := sesStart.Add(100 * time.Second)
		r1 := sesStart.Add(160 * time.Second)
		p2 := sesStart.Add(200*time.Second + 321*time.Millisecond)
		// Insert out of order, bypassing the API, to prove the ordering.
		ctx := t.Context()
		if _, err := s.DB().ExecContext(ctx, `INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason)
			VALUES (?, ?, NULL, 'sleep')`, id, p2.UnixMilli()); err != nil {
			t.Fatalf("insert open pause: %v", err)
		}
		if _, err := s.DB().ExecContext(ctx, `INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason)
			VALUES (?, ?, ?, 'hotkey')`, id, p1.UnixMilli(), r1.UnixMilli()); err != nil {
			t.Fatalf("insert closed pause: %v", err)
		}

		got, err := s.Pauses(ctx, id)
		if err != nil {
			t.Fatalf("Pauses: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("Pauses = %+v, want 2 intervals", got)
		}
		if !got[0].PausedAt.Equal(p1) || !got[0].ResumedAt.Equal(r1) || got[0].Reason != domain.PauseHotkey || got[0].SessionID != id {
			t.Errorf("first = %+v, want [%v, %v] hotkey", got[0], p1, r1)
		}
		if !got[1].PausedAt.Equal(p2) || !got[1].ResumedAt.IsZero() || got[1].Reason != domain.PauseSleep {
			t.Errorf("second = %+v, want open from %v, sleep", got[1], p2)
		}
		if got[0].ID == 0 || got[1].ID == 0 {
			t.Errorf("IDs not filled: %+v", got)
		}
	})

	t.Run("STO-022/sem pausas devolve lista vazia não nula", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		got, err := s.Pauses(t.Context(), id)
		if err != nil {
			t.Fatalf("Pauses: %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("Pauses = %#v, want empty non-nil slice", got)
		}
	})

	t.Run("STO-022/reason desconhecido gravado por fora é devolvido como está", func(t *testing.T) {
		t.Parallel()
		s, id := pauseSetup(t)
		if _, err := s.DB().ExecContext(t.Context(), `INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason)
			VALUES (?, ?, ?, 'idle')`, id, sesStart.Add(time.Minute).UnixMilli(), sesStart.Add(2*time.Minute).UnixMilli()); err != nil {
			t.Fatalf("insert: %v", err)
		}
		got, err := s.Pauses(t.Context(), id)
		if err != nil {
			t.Fatalf("Pauses: %v", err)
		}
		if len(got) != 1 || got[0].Reason != domain.PauseReason("idle") {
			t.Errorf("Pauses = %+v, want one interval with reason idle", got)
		}
	})
}
