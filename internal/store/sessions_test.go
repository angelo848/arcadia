package store_test

import (
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Fixed instants (no time.Now) with sub-second millis and a non-UTC zone: the
// store must persist UnixMilli of the instant itself (ADR 0009).
var (
	sesZone  = time.FixedZone("BRT", -3*60*60)
	sesStart = time.Date(2026, time.November, 9, 20, 0, 0, 123_000_000, sesZone)
)

// sessionRow is the raw play_session row, read bypassing the store API.
type sessionRow struct {
	GameID        int64
	StartedAt     int64
	EndedAt       sql.NullInt64
	LastSeenAt    int64
	ActiveSeconds int64
	State, Source string
	EndReason     sql.NullString
}

func readSessionRow(t *testing.T, db *sql.DB, id int64) sessionRow {
	t.Helper()
	var r sessionRow
	if err := db.QueryRowContext(t.Context(), `SELECT game_id, started_at, ended_at, last_seen_at, active_seconds,
		state, source, end_reason FROM play_session WHERE id = ?`, id).Scan(&r.GameID, &r.StartedAt, &r.EndedAt,
		&r.LastSeenAt, &r.ActiveSeconds, &r.State, &r.Source, &r.EndReason); err != nil {
		t.Fatalf("read play_session %d: %v", id, err)
	}
	return r
}

// newGame adds a game with a fixed creation instant (before any session).
func newGame(t *testing.T, s *store.Store, title string) store.Game {
	t.Helper()
	return mustAddGame(t, s, store.NewGame{
		Title: title, Exe: `C:\Games\` + title + `\` + title + `.exe`, WorkingDir: `C:\Games\` + title,
		At: sesStart.Add(-24 * time.Hour),
	})
}

func mustStartSession(t *testing.T, s *store.Store, gameID int64, at time.Time) int64 {
	t.Helper()
	id, err := s.StartSession(t.Context(), gameID, at)
	if err != nil {
		t.Fatalf("StartSession(%d, %v): %v", gameID, at, err)
	}
	return id
}

func mustEndSession(t *testing.T, s *store.Store, e store.SessionEnd) domain.SessionState {
	t.Helper()
	st, err := s.EndSession(t.Context(), e)
	if err != nil {
		t.Fatalf("EndSession(%+v): %v", e, err)
	}
	return st
}

func TestStartSession(t *testing.T) {
	t.Parallel()

	t.Run("STO-012/linha com game_id, started_at = last_seen_at = T, active 0, running, tracked, ended_at e end_reason nulos", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		id := mustStartSession(t, s, g.ID, sesStart)

		r := readSessionRow(t, s.DB(), id)
		want := sessionRow{
			GameID: g.ID, StartedAt: sesStart.UnixMilli(), LastSeenAt: sesStart.UnixMilli(),
			ActiveSeconds: 0, State: "running", Source: "tracked",
		}
		if r != want {
			t.Errorf("row = %+v, want %+v (ended_at and end_reason NULL)", r, want)
		}
	})

	t.Run("STO-012/id devolvido é o da linha", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g1, g2 := newGame(t, s, "Hades"), newGame(t, s, "Celeste")
		cases := []struct {
			gameID int64
			at     time.Time
		}{
			{g1.ID, sesStart},
			{g2.ID, sesStart.Add(time.Hour)},
			{g1.ID, sesStart.Add(2 * time.Hour)},
		}
		seen := map[int64]bool{}
		for _, c := range cases {
			id := mustStartSession(t, s, c.gameID, c.at)
			if id <= 0 || seen[id] {
				t.Fatalf("StartSession returned id %d, want a new positive id (seen %v)", id, seen)
			}
			seen[id] = true
			r := readSessionRow(t, s.DB(), id)
			if r.GameID != c.gameID || r.StartedAt != c.at.UnixMilli() {
				t.Errorf("row %d = %+v, want game_id %d and started_at %d", id, r, c.gameID, c.at.UnixMilli())
			}
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM play_session`); n != int64(len(cases)) {
			t.Errorf("play_session rows = %d, want %d", n, len(cases))
		}
	})
}

func TestUpdateSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		state      domain.SessionState
		lastSeen   time.Time
		active     time.Duration
		wantActive int64
	}{
		{"STO-013/grava estado, last_seen_at e floor(a/1s)", domain.SessionGrace,
			sesStart.Add(95*time.Second + 999*time.Millisecond), 95*time.Second + 999*time.Millisecond, 95},
		{"STO-013/grava estado, last_seen_at e floor(a/1s)/running", domain.SessionRunning,
			sesStart.Add(10 * time.Minute), 10*time.Minute + 1*time.Millisecond, 600},
		{"STO-013/29,9 s grava 29", domain.SessionRunning,
			sesStart.Add(30 * time.Second), 29*time.Second + 900*time.Millisecond, 29},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := openStore(t, dbPath(t), true)
			g := newGame(t, s, "Hades")
			id := mustStartSession(t, s, g.ID, sesStart)
			if err := s.UpdateSession(t.Context(), id, tc.state, tc.lastSeen, tc.active); err != nil {
				t.Fatalf("UpdateSession: %v", err)
			}
			r := readSessionRow(t, s.DB(), id)
			if r.State != string(tc.state) || r.LastSeenAt != tc.lastSeen.UnixMilli() || r.ActiveSeconds != tc.wantActive {
				t.Errorf("row = %+v, want state %q, last_seen_at %d, active_seconds %d",
					r, tc.state, tc.lastSeen.UnixMilli(), tc.wantActive)
			}
			if r.StartedAt != sesStart.UnixMilli() || r.GameID != g.ID || r.Source != "tracked" {
				t.Errorf("row = %+v, want started_at %d, game_id %d, source tracked unchanged", r, sesStart.UnixMilli(), g.ID)
			}
		})
	}

	t.Run("STO-013/ended_at continua nulo", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		id := mustStartSession(t, s, g.ID, sesStart)
		for i, st := range []domain.SessionState{domain.SessionRunning, domain.SessionGrace, domain.SessionRunning} {
			at := sesStart.Add(time.Duration(i+1) * time.Minute)
			if err := s.UpdateSession(t.Context(), id, st, at, time.Duration(i+1)*time.Minute); err != nil {
				t.Fatalf("UpdateSession #%d: %v", i, err)
			}
			r := readSessionRow(t, s.DB(), id)
			if r.EndedAt.Valid || r.EndReason.Valid {
				t.Errorf("after update #%d: ended_at/end_reason = %v/%v, want NULL", i, r.EndedAt, r.EndReason)
			}
		}
		open, err := s.OpenSessions(t.Context())
		if err != nil {
			t.Fatalf("OpenSessions: %v", err)
		}
		if len(open) != 1 || open[0].ID != id {
			t.Errorf("OpenSessions = %+v, want only session %d", open, id)
		}
	})

	t.Run("STO-013/sessão encerrada devolve ErrSessionNotOpen", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		id := mustStartSession(t, s, g.ID, sesStart)
		end := sesStart.Add(time.Hour)
		mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: end, Active: time.Hour, Reason: domain.EndExited, WrittenAt: end})
		before := readSessionRow(t, s.DB(), id)

		err := s.UpdateSession(t.Context(), id, domain.SessionRunning, end.Add(time.Minute), 2*time.Hour)
		if !errors.Is(err, store.ErrSessionNotOpen) {
			t.Errorf("UpdateSession on ended session err = %v, want errors.Is ErrSessionNotOpen", err)
		}
		if after := readSessionRow(t, s.DB(), id); after != before {
			t.Errorf("ended row changed: %+v -> %+v", before, after)
		}

		// Missing id is also "not open" (ErrSessionNotOpen: missing or already ended).
		err = s.UpdateSession(t.Context(), id+100, domain.SessionRunning, end, time.Minute)
		if !errors.Is(err, store.ErrSessionNotOpen) {
			t.Errorf("UpdateSession on missing session err = %v, want errors.Is ErrSessionNotOpen", err)
		}
	})
}

func TestEndSessionCounted(t *testing.T) {
	t.Parallel()

	const active = 95*time.Second + 999*time.Millisecond // floor = 95
	endAt := sesStart.Add(2 * time.Minute).Add(456 * time.Millisecond)
	writtenAt := endAt.Add(789 * time.Millisecond)

	t.Run("STO-014/sessão ended com ended_at, last_seen_at = E, floor(a), end_reason", func(t *testing.T) {
		t.Parallel()
		reasons := []domain.SessionEndReason{domain.EndExited, domain.EndAgentShutdown, domain.EndRecovered}
		for _, reason := range reasons {
			s := openStore(t, dbPath(t), true)
			g := newGame(t, s, "Hades")
			id := mustStartSession(t, s, g.ID, sesStart)
			if err := s.UpdateSession(t.Context(), id, domain.SessionGrace, sesStart.Add(time.Minute), time.Minute); err != nil {
				t.Fatalf("UpdateSession: %v", err)
			}
			st := mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: active, Reason: reason, WrittenAt: writtenAt})
			if st != domain.SessionEnded {
				t.Errorf("reason %q: EndSession state = %q, want %q", reason, st, domain.SessionEnded)
			}
			r := readSessionRow(t, s.DB(), id)
			want := sessionRow{
				GameID: g.ID, StartedAt: sesStart.UnixMilli(),
				EndedAt:    sql.NullInt64{Int64: endAt.UnixMilli(), Valid: true},
				LastSeenAt: endAt.UnixMilli(), ActiveSeconds: 95, State: "ended", Source: "tracked",
				EndReason: sql.NullString{String: string(reason), Valid: true},
			}
			if r != want {
				t.Errorf("reason %q: row = %+v, want %+v", reason, r, want)
			}
		}
	})

	t.Run("STO-014/jogo soma floor(a), play_count+1, last_played_at = E e updated_at = W", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		other := newGame(t, s, "Celeste")
		otherBefore := readGameRow(t, s.DB(), other.ID)

		id := mustStartSession(t, s, g.ID, sesStart)
		mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: active, Reason: domain.EndExited, WrittenAt: writtenAt})
		r := readGameRow(t, s.DB(), g.ID)
		if r.TotalPlaySeconds != 95 || r.PlayCount != 1 {
			t.Errorf("total_play_seconds/play_count = %d/%d, want 95/1", r.TotalPlaySeconds, r.PlayCount)
		}
		if !r.LastPlayedAt.Valid || r.LastPlayedAt.Int64 != endAt.UnixMilli() {
			t.Errorf("last_played_at = %v, want %d", r.LastPlayedAt, endAt.UnixMilli())
		}
		if r.UpdatedAt != writtenAt.UnixMilli() {
			t.Errorf("updated_at = %d, want %d (W)", r.UpdatedAt, writtenAt.UnixMilli())
		}

		// A second counted session adds on top.
		start2 := endAt.Add(time.Hour)
		end2 := start2.Add(31 * time.Second)
		w2 := end2.Add(time.Millisecond)
		id2 := mustStartSession(t, s, g.ID, start2)
		mustEndSession(t, s, store.SessionEnd{ID: id2, EndedAt: end2, Active: 30*time.Second + 500*time.Millisecond, Reason: domain.EndExited, WrittenAt: w2})
		r = readGameRow(t, s.DB(), g.ID)
		if r.TotalPlaySeconds != 125 || r.PlayCount != 2 || r.LastPlayedAt.Int64 != end2.UnixMilli() || r.UpdatedAt != w2.UnixMilli() {
			t.Errorf("after second session: %+v, want total 125, play_count 2, last_played_at %d, updated_at %d",
				r, end2.UnixMilli(), w2.UnixMilli())
		}

		if got := readGameRow(t, s.DB(), other.ID); got != otherBefore {
			t.Errorf("other game changed: %+v -> %+v", otherBefore, got)
		}
		gg, err := s.Game(t.Context(), g.ID)
		if err != nil {
			t.Fatalf("Game: %v", err)
		}
		if gg.TotalPlaySeconds != 125 || gg.PlayCount != 2 || !gg.LastPlayedAt.Equal(time.UnixMilli(end2.UnixMilli())) {
			t.Errorf("Game() = %+v, want total 125, play_count 2, last played %v", gg, end2)
		}
	})

	t.Run("STO-014/first_played_at nulo vira S", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		if r := readGameRow(t, s.DB(), g.ID); r.FirstPlayedAt.Valid {
			t.Fatalf("precondition: first_played_at = %v, want NULL", r.FirstPlayedAt)
		}
		id := mustStartSession(t, s, g.ID, sesStart)
		mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: active, Reason: domain.EndExited, WrittenAt: writtenAt})
		r := readGameRow(t, s.DB(), g.ID)
		if !r.FirstPlayedAt.Valid || r.FirstPlayedAt.Int64 != sesStart.UnixMilli() {
			t.Errorf("first_played_at = %v, want %d (S = started_at)", r.FirstPlayedAt, sesStart.UnixMilli())
		}
	})

	t.Run("STO-014/first_played_at existente é mantido", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		earlier := sesStart.Add(-30 * 24 * time.Hour)
		if _, err := s.DB().ExecContext(t.Context(), `UPDATE game SET first_played_at = ? WHERE id = ?`,
			earlier.UnixMilli(), g.ID); err != nil {
			t.Fatalf("seed first_played_at: %v", err)
		}
		id := mustStartSession(t, s, g.ID, sesStart)
		mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: active, Reason: domain.EndExited, WrittenAt: writtenAt})
		r := readGameRow(t, s.DB(), g.ID)
		if !r.FirstPlayedAt.Valid || r.FirstPlayedAt.Int64 != earlier.UnixMilli() {
			t.Errorf("first_played_at = %v, want kept at %d", r.FirstPlayedAt, earlier.UnixMilli())
		}

		// Also through the API: a second session keeps the first one's S.
		start2 := endAt.Add(time.Hour)
		id2 := mustStartSession(t, s, g.ID, start2)
		mustEndSession(t, s, store.SessionEnd{ID: id2, EndedAt: start2.Add(time.Minute), Active: time.Minute, Reason: domain.EndExited, WrittenAt: start2.Add(time.Minute)})
		if r := readGameRow(t, s.DB(), g.ID); r.FirstPlayedAt.Int64 != earlier.UnixMilli() {
			t.Errorf("after second session first_played_at = %v, want kept at %d", r.FirstPlayedAt, earlier.UnixMilli())
		}
	})

	t.Run("STO-014/trigger que aborta o UPDATE do jogo deixa a sessão aberta", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		id := mustStartSession(t, s, g.ID, sesStart)
		if err := s.UpdateSession(t.Context(), id, domain.SessionGrace, sesStart.Add(time.Minute), time.Minute); err != nil {
			t.Fatalf("UpdateSession: %v", err)
		}
		if _, err := s.DB().ExecContext(t.Context(), `CREATE TRIGGER test_abort_game_update BEFORE UPDATE ON game
			BEGIN SELECT RAISE(ABORT, 'test: game update aborted'); END`); err != nil {
			t.Fatalf("create trigger: %v", err)
		}
		sesBefore := readSessionRow(t, s.DB(), id)
		gameBefore := readGameRow(t, s.DB(), g.ID)

		_, err := s.EndSession(t.Context(), store.SessionEnd{ID: id, EndedAt: endAt, Active: active, Reason: domain.EndExited, WrittenAt: writtenAt})
		if err == nil {
			t.Fatal("EndSession with aborting trigger: err = nil, want an error")
		}
		if after := readSessionRow(t, s.DB(), id); after != sesBefore {
			t.Errorf("session changed despite failed game update: %+v -> %+v", sesBefore, after)
		}
		if after := readGameRow(t, s.DB(), g.ID); after != gameBefore {
			t.Errorf("game changed: %+v -> %+v", gameBefore, after)
		}
		open, err := s.OpenSessions(t.Context())
		if err != nil {
			t.Fatalf("OpenSessions: %v", err)
		}
		if len(open) != 1 || open[0].ID != id {
			t.Errorf("OpenSessions = %+v, want session %d still open", open, id)
		}
	})

	t.Run("STO-014/encerrar duas vezes devolve ErrSessionNotOpen e não soma de novo", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		id := mustStartSession(t, s, g.ID, sesStart)
		e := store.SessionEnd{ID: id, EndedAt: endAt, Active: active, Reason: domain.EndExited, WrittenAt: writtenAt}
		mustEndSession(t, s, e)
		sesBefore := readSessionRow(t, s.DB(), id)
		gameBefore := readGameRow(t, s.DB(), g.ID)

		again := e
		again.EndedAt = endAt.Add(time.Hour)
		again.WrittenAt = endAt.Add(time.Hour)
		again.Reason = domain.EndRecovered
		_, err := s.EndSession(t.Context(), again)
		if !errors.Is(err, store.ErrSessionNotOpen) {
			t.Errorf("second EndSession err = %v, want errors.Is ErrSessionNotOpen", err)
		}
		if after := readSessionRow(t, s.DB(), id); after != sesBefore {
			t.Errorf("session changed on second end: %+v -> %+v", sesBefore, after)
		}
		if after := readGameRow(t, s.DB(), g.ID); after != gameBefore {
			t.Errorf("game changed on second end: %+v -> %+v", gameBefore, after)
		}

		// Missing id is also "not open".
		missing := e
		missing.ID = id + 100
		if _, err := s.EndSession(t.Context(), missing); !errors.Is(err, store.ErrSessionNotOpen) {
			t.Errorf("EndSession on missing id err = %v, want errors.Is ErrSessionNotOpen", err)
		}
	})
}

func TestEndSessionClosedByHotkey(t *testing.T) {
	t.Parallel()

	t.Run("CON-016/o store grava e lê o motivo closed-by-hotkey (EndSession e GameSessions)", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		id := mustStartSession(t, s, g.ID, sesStart)
		endAt := sesStart.Add(10 * time.Minute)
		st := mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: 9 * time.Minute,
			Reason: domain.EndClosedByHotkey, WrittenAt: endAt})
		if st != domain.SessionEnded {
			t.Errorf("state = %q, want %q", st, domain.SessionEnded)
		}
		r := readSessionRow(t, s.DB(), id)
		if !r.EndReason.Valid || r.EndReason.String != "closed-by-hotkey" {
			t.Errorf("end_reason = %+v, want closed-by-hotkey", r.EndReason)
		}
		page, err := s.GameSessions(t.Context(), g.ID, 0, 10)
		if err != nil {
			t.Fatalf("GameSessions: %v", err)
		}
		if len(page.Sessions) != 1 || page.Sessions[0].EndReason != "closed-by-hotkey" {
			t.Errorf("sessions = %+v, want one with EndReason closed-by-hotkey", page.Sessions)
		}
	})
}

func TestEndSessionDiscarded(t *testing.T) {
	t.Parallel()

	endAt := sesStart.Add(31 * time.Second).Add(250 * time.Millisecond)
	writtenAt := endAt.Add(10 * time.Millisecond)

	t.Run("STO-015/a < 30 s grava discarded com os mesmos campos", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			active time.Duration
			want   int64
		}{
			{0, 0},
			{29*time.Second + 999*time.Millisecond, 29},
			{12 * time.Second, 12},
		}
		for _, tc := range tests {
			s := openStore(t, dbPath(t), true)
			g := newGame(t, s, "Hades")
			id := mustStartSession(t, s, g.ID, sesStart)
			st := mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: tc.active, Reason: domain.EndAgentShutdown, WrittenAt: writtenAt})
			if st != domain.SessionDiscarded {
				t.Errorf("active %v: state = %q, want %q", tc.active, st, domain.SessionDiscarded)
			}
			r := readSessionRow(t, s.DB(), id)
			want := sessionRow{
				GameID: g.ID, StartedAt: sesStart.UnixMilli(),
				EndedAt:    sql.NullInt64{Int64: endAt.UnixMilli(), Valid: true},
				LastSeenAt: endAt.UnixMilli(), ActiveSeconds: tc.want, State: "discarded", Source: "tracked",
				EndReason: sql.NullString{String: string(domain.EndAgentShutdown), Valid: true},
			}
			if r != want {
				t.Errorf("active %v: row = %+v, want %+v", tc.active, r, want)
			}
		}
	})

	t.Run("STO-015/linha do jogo não muda (nem updated_at)", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		before := readGameRow(t, s.DB(), g.ID)
		id := mustStartSession(t, s, g.ID, sesStart)
		mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: endAt, Active: 29*time.Second + 999*time.Millisecond, Reason: domain.EndExited, WrittenAt: writtenAt})
		if after := readGameRow(t, s.DB(), g.ID); after != before {
			t.Errorf("game row changed after discarded session: %+v -> %+v", before, after)
		}
		// Ending it again is still refused (it is no longer open).
		if _, err := s.EndSession(t.Context(), store.SessionEnd{ID: id, EndedAt: endAt, Active: time.Hour, Reason: domain.EndExited, WrittenAt: writtenAt}); !errors.Is(err, store.ErrSessionNotOpen) {
			t.Errorf("EndSession on discarded session err = %v, want ErrSessionNotOpen", err)
		}
	})
}

func TestTotalRecomputable(t *testing.T) {
	t.Parallel()

	t.Run("STO-016/10 s, 45 s, 30 s e 3600 s dão total 3675, play_count 3 e soma dos ended", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		other := newGame(t, s, "Celeste")

		at := sesStart
		wantStates := []domain.SessionState{domain.SessionDiscarded, domain.SessionEnded, domain.SessionEnded, domain.SessionEnded}
		for i, a := range []time.Duration{10 * time.Second, 45 * time.Second, 30 * time.Second, 3600 * time.Second} {
			id := mustStartSession(t, s, g.ID, at)
			end := at.Add(a)
			if st := mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: end, Active: a, Reason: domain.EndExited, WrittenAt: end}); st != wantStates[i] {
				t.Errorf("session %v: state = %q, want %q", a, st, wantStates[i])
			}
			at = end.Add(time.Minute)
		}
		// An ended session of another game must not count.
		oid := mustStartSession(t, s, other.ID, at)
		mustEndSession(t, s, store.SessionEnd{ID: oid, EndedAt: at.Add(time.Hour), Active: time.Hour, Reason: domain.EndExited, WrittenAt: at.Add(time.Hour)})

		r := readGameRow(t, s.DB(), g.ID)
		if r.TotalPlaySeconds != 3675 || r.PlayCount != 3 {
			t.Errorf("total_play_seconds/play_count = %d/%d, want 3675/3", r.TotalPlaySeconds, r.PlayCount)
		}
		sum := countRows(t, s.DB(), `SELECT COALESCE(SUM(active_seconds), 0) FROM play_session WHERE game_id = ? AND state = 'ended'`, g.ID)
		if sum != r.TotalPlaySeconds {
			t.Errorf("sum(active_seconds of ended) = %d, want = total_play_seconds %d", sum, r.TotalPlaySeconds)
		}
	})
}

func TestOpenSessions(t *testing.T) {
	t.Parallel()

	t.Run("STO-017/só as abertas, com id, game_id, started_at, last_seen_at e active_seconds", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g1, g2 := newGame(t, s, "Hades"), newGame(t, s, "Celeste")

		open1 := mustStartSession(t, s, g1.ID, sesStart)
		seen1 := sesStart.Add(5*time.Minute + 7*time.Millisecond)
		if err := s.UpdateSession(t.Context(), open1, domain.SessionGrace, seen1, 4*time.Minute+500*time.Millisecond); err != nil {
			t.Fatalf("UpdateSession: %v", err)
		}
		ended := mustStartSession(t, s, g1.ID, sesStart.Add(time.Hour))
		mustEndSession(t, s, store.SessionEnd{ID: ended, EndedAt: sesStart.Add(2 * time.Hour), Active: time.Hour, Reason: domain.EndExited, WrittenAt: sesStart.Add(2 * time.Hour)})
		start2 := sesStart.Add(3*time.Hour + 42*time.Millisecond)
		open2 := mustStartSession(t, s, g2.ID, start2)
		discarded := mustStartSession(t, s, g2.ID, sesStart.Add(4*time.Hour))
		mustEndSession(t, s, store.SessionEnd{ID: discarded, EndedAt: sesStart.Add(4*time.Hour + 5*time.Second), Active: 5 * time.Second, Reason: domain.EndRecovered, WrittenAt: sesStart.Add(5 * time.Hour)})

		got, err := s.OpenSessions(t.Context())
		if err != nil {
			t.Fatalf("OpenSessions: %v", err)
		}
		slices.SortFunc(got, func(a, b store.OpenSession) int { return int(a.ID - b.ID) })

		type flat struct{ ID, GameID, Started, LastSeen, Active int64 }
		var gotFlat []flat
		for _, o := range got {
			gotFlat = append(gotFlat, flat{o.ID, o.GameID, o.StartedAt.UnixMilli(), o.LastSeenAt.UnixMilli(), o.ActiveSeconds})
		}
		want := []flat{
			{open1, g1.ID, sesStart.UnixMilli(), seen1.UnixMilli(), 240},
			{open2, g2.ID, start2.UnixMilli(), start2.UnixMilli(), 0},
		}
		if !slices.Equal(gotFlat, want) {
			t.Errorf("OpenSessions = %+v, want %+v (ended %d and discarded %d excluded)", gotFlat, want, ended, discarded)
		}
	})

	t.Run("STO-017/sem abertas devolve vazio", func(t *testing.T) {
		t.Parallel()
		for _, withEnded := range []bool{false, true} {
			s := openStore(t, dbPath(t), true)
			if withEnded {
				g := newGame(t, s, "Hades")
				id := mustStartSession(t, s, g.ID, sesStart)
				mustEndSession(t, s, store.SessionEnd{ID: id, EndedAt: sesStart.Add(time.Hour), Active: time.Hour, Reason: domain.EndExited, WrittenAt: sesStart.Add(time.Hour)})
			}
			got, err := s.OpenSessions(t.Context())
			if err != nil {
				t.Fatalf("OpenSessions (withEnded=%v): %v", withEnded, err)
			}
			if got == nil || len(got) != 0 {
				t.Errorf("OpenSessions (withEnded=%v) = %#v, want a non-nil empty slice", withEnded, got)
			}
		}
	})
}

func TestDeleteSession(t *testing.T) {
	t.Parallel()

	t.Run("STO-012/DeleteSession remove a linha", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := newGame(t, s, "Hades")
		keep := mustStartSession(t, s, g.ID, sesStart)
		gone := mustStartSession(t, s, g.ID, sesStart.Add(time.Hour))
		gameBefore := readGameRow(t, s.DB(), g.ID)

		if err := s.DeleteSession(t.Context(), gone); err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM play_session WHERE id = ?`, gone); n != 0 {
			t.Errorf("deleted session rows = %d, want 0", n)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM play_session WHERE id = ?`, keep); n != 1 {
			t.Errorf("other session rows = %d, want 1", n)
		}
		if after := readGameRow(t, s.DB(), g.ID); after != gameBefore {
			t.Errorf("game changed by DeleteSession: %+v -> %+v", gameBefore, after)
		}
	})
}
