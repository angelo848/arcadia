package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Tests of Store.RemoveSession (fase 16, T11). Helpers come from
// session_edit_create_test.go and session_edit_update_test.go.

func sesRowCount(t *testing.T, s *store.Store, id int64) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM play_session WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count session %d: %v", id, err)
	}
	return n
}

func TestRemoveSession(t *testing.T) {
	t.Parallel()
	h := time.Hour
	m := time.Minute

	t.Run("SED-010/apaga a sessão e as pausas e subtrai o tempo: total X − a, updated_at = W", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 10800, Plays: 2, FirstPlayed: sesAt(-10 * h).UnixMilli(), LastPlayed: sesAt(-2 * h).UnixMilli()})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-10 * h), End: sesAt(-9 * h), Active: 3600})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 7200})
		sesPause(t, s, id, sesAt(-170*m), sesAt(-160*m))
		w, err := s.RemoveSession(t.Context(), id, sesW)
		if err != nil {
			t.Fatalf("RemoveSession: %v", err)
		}
		if sesRowCount(t, s, id) != 0 || len(sesPauses(t, s, id)) != 0 {
			t.Errorf("session or pauses still there")
		}
		g := sesGame(t, s, ids[0])
		if g.Total != 3600 || g.Updated != sesW.UnixMilli() {
			t.Errorf("game = %+v, want total 3600, updated W", g)
		}
		if w.TotalBefore != 10800 || w.TotalAfter != 3600 || w.Game.TotalPlaySeconds != 3600 || !w.Changed {
			t.Errorf("write = before %d after %d game %d changed %v", w.TotalBefore, w.TotalAfter, w.Game.TotalPlaySeconds, w.Changed)
		}
		if w.Previous == nil || w.Previous.ID != id {
			t.Errorf("Previous = %+v, want the removed session", w.Previous)
		}
	})

	t.Run("SED-010/sessão da qual restam outras: first, last e contagem recalculados; sem sessões contadas: first e last nulos e play_count = 0", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 7200, Plays: 2, FirstPlayed: sesAt(-10 * h).UnixMilli(), LastPlayed: sesAt(-2 * h).UnixMilli()})
		a := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-10 * h), End: sesAt(-9 * h), Active: 3600})
		b := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})
		if _, err := s.RemoveSession(t.Context(), a, sesW); err != nil {
			t.Fatalf("remove a: %v", err)
		}
		g := sesGame(t, s, ids[0])
		if g.First != sesAt(-3*h).UnixMilli() || g.Last != sesAt(-2*h).UnixMilli() || g.Plays != 1 {
			t.Errorf("game = %+v, want first -3h, last -2h, plays 1", g)
		}
		if _, err := s.RemoveSession(t.Context(), b, sesW); err != nil {
			t.Fatalf("remove b: %v", err)
		}
		g = sesGame(t, s, ids[0])
		if g.First != 0 || g.Last != 0 || g.Plays != 0 || g.Total != 0 {
			t.Errorf("game = %+v, want first/last NULL, plays 0, total 0", g)
		}
	})

	t.Run("SED-010/exemplo do ajuste manual: total ajustado para 1 h e exclusão de uma sessão de 3 h deixa 0 (SED-013)", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3600, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-2 * h), Active: 3 * 3600})
		w, err := s.RemoveSession(t.Context(), id, sesW)
		if err != nil {
			t.Fatalf("RemoveSession: %v", err)
		}
		if g := sesGame(t, s, ids[0]); g.Total != 0 || w.TotalAfter != 0 {
			t.Errorf("total/TotalAfter = %d/%d, want 0", g.Total, w.TotalAfter)
		}
	})

	t.Run("SED-011/descartada: a linha e as pausas saem e a linha do jogo não muda (nem updated_at)", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-5*h + 20*time.Second), Active: 20, State: "discarded"})
		sesPause(t, s, id, sesAt(-5*h+2*time.Second), sesAt(-5*h+5*time.Second))
		before := sesGame(t, s, ids[0])
		if _, err := s.RemoveSession(t.Context(), id, sesW); err != nil {
			t.Fatalf("RemoveSession: %v", err)
		}
		if sesRowCount(t, s, id) != 0 || len(sesPauses(t, s, id)) != 0 {
			t.Errorf("session or pauses still there")
		}
		if g := sesGame(t, s, ids[0]); g != before {
			t.Errorf("game = %+v, want unchanged %+v", g, before)
		}
	})

	t.Run("SED-012/descartada e aberta não entram no recálculo", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3600, Plays: 1, FirstPlayed: sesAt(-10 * h).UnixMilli(), LastPlayed: sesAt(-9 * h).UnixMilli()})
		a := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-10 * h), End: sesAt(-9 * h), Active: 3600})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-8 * h), End: sesAt(-8*h + 20*time.Second), Active: 20, State: "discarded"})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-30 * m), Active: 60, State: "running"})
		if _, err := s.RemoveSession(t.Context(), a, sesW); err != nil {
			t.Fatalf("RemoveSession: %v", err)
		}
		if g := sesGame(t, s, ids[0]); g.First != 0 || g.Last != 0 || g.Plays != 0 {
			t.Errorf("game = %+v, want first/last NULL and plays 0", g)
		}
	})

	t.Run("SED-014/falha ao atualizar o jogo (trigger) desfaz a exclusão e as pausas", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
		sesPause(t, s, id, sesAt(-4*h-55*m), sesAt(-4*h-40*m))
		libExec(t, s.DB(), `CREATE TRIGGER game_block BEFORE UPDATE ON game BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
		rowBefore, gameBefore := sesFull(t, s, id), sesGame(t, s, ids[0])
		if _, err := s.RemoveSession(t.Context(), id, sesW); err == nil {
			t.Fatalf("err = nil, want the trigger failure")
		}
		if sesRowCount(t, s, id) != 1 || sesFull(t, s, id) != rowBefore || len(sesPauses(t, s, id)) != 1 || sesGame(t, s, ids[0]) != gameBefore {
			t.Errorf("a failed delete left changes behind")
		}
	})

	t.Run("SED-022/aberta devolve *SessionOpenError e nada muda", func(t *testing.T) {
		t.Parallel()
		for _, state := range []string{"running", "paused", "grace"} {
			t.Run(state, func(t *testing.T) {
				t.Parallel()
				s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
				id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-30 * m), Active: 600, State: state})
				rowBefore, gameBefore := sesFull(t, s, id), sesGame(t, s, ids[0])
				_, err := s.RemoveSession(t.Context(), id, sesW)
				var oe *store.SessionOpenError
				if !errors.As(err, &oe) || !errors.Is(err, store.ErrSessionOpen) {
					t.Fatalf("err = %v (%T), want *SessionOpenError satisfying ErrSessionOpen", err, err)
				}
				if oe.SessionID != id || oe.GameID != ids[0] || oe.Title != "Hades" {
					t.Errorf("open error = %+v", oe)
				}
				if sesFull(t, s, id) != rowBefore || sesGame(t, s, ids[0]) != gameBefore {
					t.Errorf("a refused delete changed the database")
				}
			})
		}
	})

	t.Run("SED-024/id inexistente devolve ErrSessionNotFound", func(t *testing.T) {
		t.Parallel()
		s, _ := newSesStore(t, libGame{Title: "Hades"})
		if _, err := s.RemoveSession(t.Context(), 9999, sesW); !errors.Is(err, store.ErrSessionNotFound) {
			t.Errorf("err = %v, want ErrSessionNotFound", err)
		}
	})

	t.Run("SED-028/status e completed_at não mudam", func(t *testing.T) {
		t.Parallel()
		completed := sesAt(-500 * h).UnixMilli()
		s, ids := newSesStore(t,
			libGame{Title: "Novo", Seconds: 3000, Plays: 1},
			libGame{Title: "Zerado", Seconds: 3000, Plays: 1, Status: "completed", Completed: completed})
		for _, gid := range ids {
			id := sesInsert(t, s, sesRow{GameID: gid, Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
			if _, err := s.RemoveSession(t.Context(), id, sesW); err != nil {
				t.Fatalf("RemoveSession: %v", err)
			}
		}
		if g := sesGame(t, s, ids[0]); g.Status != "not_played" || g.Completed != 0 {
			t.Errorf("Novo status/completed = %s/%d", g.Status, g.Completed)
		}
		if g := sesGame(t, s, ids[1]); g.Status != "completed" || g.Completed != completed {
			t.Errorf("Zerado status/completed = %s/%d", g.Status, g.Completed)
		}
	})
}
