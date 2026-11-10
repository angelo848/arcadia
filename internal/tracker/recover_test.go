package tracker_test

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// openTestStore opens a migrated store in t.TempDir() and closes it in Cleanup.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Store.Close: %v", err)
		}
	})
	return s
}

func addTestGame(t *testing.T, s *store.Store) store.Game {
	t.Helper()
	g, err := s.AddGame(t.Context(), store.NewGame{
		Title: testTitle,
		Exe:   testSpec.Exe,
		At:    baseT.Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	return g
}

// fakeRecoveryStore serves a fixed list of open sessions and records EndSession.
type fakeRecoveryStore struct {
	open    []store.OpenSession
	openErr error
	endErr  map[int64]error

	mu   sync.Mutex
	ends []store.SessionEnd
}

func (s *fakeRecoveryStore) OpenSessions(context.Context) ([]store.OpenSession, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	return slices.Clone(s.open), nil
}

func (s *fakeRecoveryStore) EndSession(_ context.Context, e store.SessionEnd) (domain.SessionState, error) {
	s.mu.Lock()
	s.ends = append(s.ends, e)
	s.mu.Unlock()
	if err := s.endErr[e.ID]; err != nil {
		return "", err
	}
	return domain.Outcome(e.Active), nil
}

func (s *fakeRecoveryStore) endCalls() []store.SessionEnd {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.ends)
}

var _ tracker.RecoveryStore = (*fakeRecoveryStore)(nil)

// Two open sessions left by a crash: 590 s (counts) and 20 s (discarded).
func openRows() []store.OpenSession {
	return []store.OpenSession{
		{ID: 1, GameID: testGameID, StartedAt: baseT, LastSeenAt: at(590 * time.Second), ActiveSeconds: 590},
		{ID: 2, GameID: testGameID, StartedAt: at(time.Hour), LastSeenAt: at(time.Hour + 20*time.Second), ActiveSeconds: 20},
	}
}

// pausedSession starts a session at baseT and leaves it paused (open) since p,
// with 100 s of persisted active time, last_seen_at = l.
func pausedSession(t *testing.T, s *store.Store, p, l time.Time) int64 {
	t.Helper()
	ctx := t.Context()
	g := addTestGame(t, s)
	id, err := s.StartSession(ctx, g.ID, baseT)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if err := s.PauseSession(ctx, store.PauseStart{SessionID: id, At: p, Reason: domain.PauseHotkey, Active: 100 * time.Second}); err != nil {
		t.Fatalf("PauseSession: %v", err)
	}
	if !l.Equal(p) {
		if err := s.UpdateSession(ctx, id, domain.SessionPaused, l, 100*time.Second); err != nil {
			t.Fatalf("UpdateSession: %v", err)
		}
	}
	return id
}

func TestRecoverPaused(t *testing.T) {
	t.Parallel()
	now := at(48 * time.Hour)

	t.Run("SES-025/pausa aberta em P=T+100s e last_seen L=T+120s vira [P, L], 100 s e motivo recovered", func(t *testing.T) {
		t.Parallel()
		s := openTestStore(t)
		p, l := at(100*time.Second), at(120*time.Second)
		id := pausedSession(t, s, p, l)

		logs := newLogStore()
		n, err := tracker.RecoverOpenSessions(t.Context(), s, domain.NewFakeClock(now), logs.logger())
		if err != nil || n != 1 {
			t.Fatalf("RecoverOpenSessions = %d, %v; want 1, nil", n, err)
		}
		pauses, err := s.Pauses(t.Context(), id)
		if err != nil {
			t.Fatalf("Pauses: %v", err)
		}
		if len(pauses) != 1 || !pauses[0].PausedAt.Equal(p) || !pauses[0].ResumedAt.Equal(l) {
			t.Fatalf("Pauses = %+v, want one interval [%v, %v]", pauses, p, l)
		}
		var state, reason string
		var endedAt, active int64
		if err := s.DB().QueryRowContext(t.Context(),
			`SELECT state, end_reason, ended_at, active_seconds FROM play_session WHERE id = ?`, id).
			Scan(&state, &reason, &endedAt, &active); err != nil {
			t.Fatalf("read session: %v", err)
		}
		if state != "ended" || reason != "recovered" || endedAt != store.TimeToMillis(l) || active != 100 {
			t.Errorf("session = %s/%s/%d/%d, want ended/recovered/%d/100", state, reason, endedAt, active, store.TimeToMillis(l))
		}
		open, err := s.OpenSessions(t.Context())
		if err != nil || len(open) != 0 {
			t.Errorf("OpenSessions = %+v, %v; want none", open, err)
		}
	})

	t.Run("SES-025/P igual a L: nenhum intervalo", func(t *testing.T) {
		t.Parallel()
		s := openTestStore(t)
		p := at(100 * time.Second)
		id := pausedSession(t, s, p, p)

		n, err := tracker.RecoverOpenSessions(t.Context(), s, domain.NewFakeClock(now), newLogStore().logger())
		if err != nil || n != 1 {
			t.Fatalf("RecoverOpenSessions = %d, %v; want 1, nil", n, err)
		}
		pauses, err := s.Pauses(t.Context(), id)
		if err != nil {
			t.Fatalf("Pauses: %v", err)
		}
		if len(pauses) != 0 {
			t.Errorf("Pauses = %+v, want none", pauses)
		}
	})

	t.Run("SES-025/sem notificação nem feedback", func(t *testing.T) {
		t.Parallel()
		s := openTestStore(t)
		pausedSession(t, s, at(100*time.Second), at(120*time.Second))

		// RecoverOpenSessions takes no notifier or feedback sink; the only
		// observable output is the single start-up Warn summary.
		logs := newLogStore()
		if _, err := tracker.RecoverOpenSessions(t.Context(), s, domain.NewFakeClock(now), logs.logger()); err != nil {
			t.Fatalf("RecoverOpenSessions: %v", err)
		}
		recs := logs.atLeast(slog.LevelWarn)
		if len(recs) != 1 || recs[0].msg != msgRecovered {
			t.Errorf("Warn/Error records = %v, want only %q", recs, msgRecovered)
		}
	})
}

func TestRecoverOpenSessions(t *testing.T) {
	t.Parallel()
	now := at(48 * time.Hour) // agent start-up

	t.Run("SES-010/cada aberta encerra com ended_at = last_seen_at, o ativo gravado e recovered", func(t *testing.T) {
		t.Parallel()
		st := &fakeRecoveryStore{open: openRows()}
		logs := newLogStore()
		if _, err := tracker.RecoverOpenSessions(t.Context(), st, domain.NewFakeClock(now), logs.logger()); err != nil {
			t.Fatalf("RecoverOpenSessions: %v", err)
		}
		want := []store.SessionEnd{
			{ID: 1, EndedAt: at(590 * time.Second), Active: 590 * time.Second, Reason: domain.EndRecovered, WrittenAt: now},
			{ID: 2, EndedAt: at(time.Hour + 20*time.Second), Active: 20 * time.Second, Reason: domain.EndRecovered, WrittenAt: now},
		}
		got := st.endCalls()
		if len(got) != len(want) {
			t.Fatalf("EndSession calls = %+v, want %+v", got, want)
		}
		for i := range want {
			g, w := got[i], want[i]
			if g.ID != w.ID || !g.EndedAt.Equal(w.EndedAt) || g.Active != w.Active || g.Reason != w.Reason || !g.WrittenAt.Equal(w.WrittenAt) {
				t.Errorf("EndSession[%d] = %+v, want %+v", i, g, w)
			}
		}
	})

	t.Run("SES-010/count e Warn sessões abertas recuperadas", func(t *testing.T) {
		t.Parallel()
		st := &fakeRecoveryStore{open: openRows()}
		logs := newLogStore()
		n, err := tracker.RecoverOpenSessions(t.Context(), st, domain.NewFakeClock(now), logs.logger())
		if err != nil {
			t.Fatalf("RecoverOpenSessions: %v", err)
		}
		if n != 2 {
			t.Errorf("count = %d, want 2", n)
		}
		recs := logs.find(slog.LevelWarn, msgRecovered)
		if len(recs) != 1 {
			t.Fatalf("Warn %q records = %d, want 1", msgRecovered, len(recs))
		}
		wantAttrs(t, recs[0], map[string]string{"count": "2"})
	})

	t.Run("SES-010/sem abertas não loga Warn", func(t *testing.T) {
		t.Parallel()
		st := &fakeRecoveryStore{open: []store.OpenSession{}}
		logs := newLogStore()
		n, err := tracker.RecoverOpenSessions(t.Context(), st, domain.NewFakeClock(now), logs.logger())
		if err != nil || n != 0 {
			t.Fatalf("RecoverOpenSessions = %d, %v; want 0, nil", n, err)
		}
		if recs := logs.atLeast(slog.LevelWarn); len(recs) != 0 {
			t.Errorf("Warn/Error records = %v, want none", recs)
		}
		if got := st.endCalls(); len(got) != 0 {
			t.Errorf("EndSession calls = %+v, want none", got)
		}
	})

	t.Run("SES-010/falha em uma sessão é logada e as outras seguem", func(t *testing.T) {
		t.Parallel()
		st := &fakeRecoveryStore{open: openRows(), endErr: map[int64]error{1: errDisk}}
		logs := newLogStore()
		n, err := tracker.RecoverOpenSessions(t.Context(), st, domain.NewFakeClock(now), logs.logger())
		if err != nil {
			t.Errorf("RecoverOpenSessions error = %v, want nil (per-session failures are only logged)", err)
		}
		if n != 1 {
			t.Errorf("count = %d, want 1 (only session 2 closed)", n)
		}
		got := st.endCalls()
		if len(got) != 2 || got[1].ID != 2 {
			t.Errorf("EndSession calls = %+v, want sessions 1 and 2 attempted", got)
		}
		var failure []logRec
		for _, r := range logs.atLeast(slog.LevelWarn) {
			if r.msg != msgRecovered {
				failure = append(failure, r)
			}
		}
		if len(failure) == 0 {
			t.Errorf("no Warn/Error record for the failed session; logs %v", logs.atLeast(slog.LevelDebug))
		}
	})

	t.Run("SES-010/erro de OpenSessions é devolvido", func(t *testing.T) {
		t.Parallel()
		st := &fakeRecoveryStore{openErr: errDisk}
		logs := newLogStore()
		n, err := tracker.RecoverOpenSessions(t.Context(), st, domain.NewFakeClock(now), logs.logger())
		if !errors.Is(err, errDisk) {
			t.Errorf("RecoverOpenSessions error = %v, want it to wrap %v", err, errDisk)
		}
		if n != 0 {
			t.Errorf("count = %d, want 0", n)
		}
		if got := st.endCalls(); len(got) != 0 {
			t.Errorf("EndSession calls = %+v, want none", got)
		}
	})

	t.Run("SES-010/store real: 590 s vira ended com total +590 e 20 s vira discarded", func(t *testing.T) {
		t.Parallel()
		s := openTestStore(t)
		g := addTestGame(t, s)
		ctx := t.Context()
		long, err := s.StartSession(ctx, g.ID, baseT)
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		if err := s.UpdateSession(ctx, long, domain.SessionRunning, at(590*time.Second), 590*time.Second); err != nil {
			t.Fatalf("UpdateSession: %v", err)
		}
		short, err := s.StartSession(ctx, g.ID, at(time.Hour))
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		if err := s.UpdateSession(ctx, short, domain.SessionGrace, at(time.Hour+20*time.Second), 20*time.Second); err != nil {
			t.Fatalf("UpdateSession: %v", err)
		}

		logs := newLogStore()
		n, err := tracker.RecoverOpenSessions(ctx, s, domain.NewFakeClock(now), logs.logger())
		if err != nil || n != 2 {
			t.Fatalf("RecoverOpenSessions = %d, %v; want 2, nil", n, err)
		}
		open, err := s.OpenSessions(ctx)
		if err != nil {
			t.Fatalf("OpenSessions: %v", err)
		}
		if len(open) != 0 {
			t.Errorf("open sessions after recovery = %+v, want none", open)
		}

		type row struct {
			state, reason string
			endedAt       int64
			active        int64
		}
		read := func(id int64) row {
			t.Helper()
			var r row
			err := s.DB().QueryRowContext(ctx,
				`SELECT state, end_reason, ended_at, active_seconds FROM play_session WHERE id = ?`, id).
				Scan(&r.state, &r.reason, &r.endedAt, &r.active)
			if err != nil {
				t.Fatalf("read session %d: %v", id, err)
			}
			return r
		}
		wantRows := map[int64]row{
			long:  {"ended", "recovered", store.TimeToMillis(at(590 * time.Second)), 590},
			short: {"discarded", "recovered", store.TimeToMillis(at(time.Hour + 20*time.Second)), 20},
		}
		for id, want := range wantRows {
			if got := read(id); got != want {
				t.Errorf("session %d = %+v, want %+v", id, got, want)
			}
		}

		got, err := s.Game(ctx, g.ID)
		if err != nil {
			t.Fatalf("Game: %v", err)
		}
		if got.TotalPlaySeconds != 590 || got.PlayCount != 1 || !got.LastPlayedAt.Equal(at(590*time.Second)) {
			t.Errorf("game total/count/last = %d/%d/%v, want 590/1/%v",
				got.TotalPlaySeconds, got.PlayCount, got.LastPlayedAt, at(590*time.Second))
		}
	})
}
