package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Tests of Store.CreateManualSession (fase 16, T07). Fixtures are written with raw SQL.
// `now` (W) is sesW; every instant is relative to it.

var sesW = time.Date(2026, time.November, 25, 20, 0, 0, 0, time.UTC)

func sesAt(d time.Duration) time.Time { return sesW.Add(d) }

// sesRow is one play_session row of a fixture. Ended zero = NULL (open session).
type sesRow struct {
	GameID        int64
	Start, End    time.Time
	Active        int64
	State, Source string // "" = ended / tracked
}

func sesInsert(t *testing.T, s *store.Store, r sesRow) int64 {
	t.Helper()
	if r.State == "" {
		r.State = "ended"
	}
	if r.Source == "" {
		r.Source = "tracked"
	}
	var ended any
	if !r.End.IsZero() {
		ended = r.End.UnixMilli()
	}
	res, err := s.DB().ExecContext(t.Context(),
		`INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.GameID, r.Start.UnixMilli(), ended, r.Start.UnixMilli(), r.Active, r.State, r.Source)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("session id: %v", err)
	}
	return id
}

// sesGameRow is the part of the game row that the session writes touch.
type sesGameRow struct {
	Total, Plays         int64
	First, Last, Updated int64 // ms; 0 = NULL for First/Last
	Status               string
	Completed            int64
}

func sesGame(t *testing.T, s *store.Store, id int64) sesGameRow {
	t.Helper()
	var r sesGameRow
	var first, last, completed *int64
	err := s.DB().QueryRowContext(t.Context(),
		`SELECT total_play_seconds, play_count, first_played_at, last_played_at, updated_at, status, completed_at
		FROM game WHERE id = ?`, id).Scan(&r.Total, &r.Plays, &first, &last, &r.Updated, &r.Status, &completed)
	if err != nil {
		t.Fatalf("read game %d: %v", id, err)
	}
	if first != nil {
		r.First = *first
	}
	if last != nil {
		r.Last = *last
	}
	if completed != nil {
		r.Completed = *completed
	}
	return r
}

func sesCount(t *testing.T, s *store.Store) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM play_session`).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return n
}

func sesCreate(t *testing.T, s *store.Store, m store.ManualSession) store.SessionWrite {
	t.Helper()
	w, err := s.CreateManualSession(t.Context(), m, sesW)
	if err != nil {
		t.Fatalf("CreateManualSession(%+v): %v", m, err)
	}
	return w
}

// sesRequireReason asserts a *SessionEditError with the reason and that nothing changed.
func sesRequireReason(t *testing.T, err error, want domain.EditReason) {
	t.Helper()
	var ee *store.SessionEditError
	if !errors.As(err, &ee) {
		t.Fatalf("err = %v (%T), want *store.SessionEditError", err, err)
	}
	if ee.Reason != want {
		t.Errorf("Reason = %q, want %q", ee.Reason, want)
	}
}

func newSesStore(t *testing.T, games ...libGame) (*store.Store, []int64) {
	t.Helper()
	s := openStore(t, dbPath(t), true)
	return s, seedLibraryIDs(t, s, games...)
}

func TestCreateManualSession(t *testing.T) {
	t.Parallel()
	h := time.Hour

	t.Run("SED-003/grava game_id, started_at, ended_at, last_seen_at = ended_at, active_seconds, state ended, source manual, end_reason nulo e edited_at nulo, sem pause_interval", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades"})
		start, end := sesAt(-3*h), sesAt(-2*h)
		w := sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: start, End: end, Active: 50 * time.Minute})

		var gameID, started, ended, lastSeen, active int64
		var state, source string
		var endReason, editedAt *string
		var editedRaw *int64
		err := s.DB().QueryRowContext(t.Context(),
			`SELECT game_id, started_at, ended_at, last_seen_at, active_seconds, state, source, end_reason, edited_at
			FROM play_session WHERE id = ?`, w.Session.ID).
			Scan(&gameID, &started, &ended, &lastSeen, &active, &state, &source, &endReason, &editedRaw)
		if err != nil {
			t.Fatalf("read session: %v", err)
		}
		_ = editedAt
		if gameID != ids[0] || started != start.UnixMilli() || ended != end.UnixMilli() || lastSeen != end.UnixMilli() {
			t.Errorf("row = game %d [%d, %d] seen %d", gameID, started, ended, lastSeen)
		}
		if active != 3000 || state != "ended" || source != "manual" {
			t.Errorf("active/state/source = %d/%s/%s, want 3000/ended/manual", active, state, source)
		}
		if endReason != nil || editedRaw != nil {
			t.Errorf("end_reason/edited_at = %v/%v, want NULL/NULL", endReason, editedRaw)
		}
		var pauses int
		if err := s.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM pause_interval WHERE session_id = ?`, w.Session.ID).Scan(&pauses); err != nil || pauses != 0 {
			t.Errorf("pause_interval rows = %d (err %v), want 0", pauses, err)
		}
		if w.Session.Source != "manual" || w.Session.State != domain.SessionEnded || w.Session.ActiveSeconds != 3000 || !w.Session.EditedAt.IsZero() {
			t.Errorf("returned Session = %+v", w.Session)
		}
		if w.Previous != nil {
			t.Errorf("Previous = %+v, want nil on create", w.Previous)
		}
		if !w.Changed {
			t.Errorf("Changed = false, want true")
		}
	})

	t.Run("SED-004/soma o tempo ao total, atualiza first, last e contagem e updated_at = W", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3600, Plays: 1, FirstPlayed: sesAt(-10 * h).UnixMilli(), LastPlayed: sesAt(-9 * h).UnixMilli()})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-10 * h), End: sesAt(-9 * h), Active: 3600})
		w := sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: h})
		g := sesGame(t, s, ids[0])
		if g.Total != 7200 || g.Plays != 2 || g.First != sesAt(-10*h).UnixMilli() || g.Last != sesAt(-2*h).UnixMilli() || g.Updated != sesW.UnixMilli() {
			t.Errorf("game = %+v, want total 7200, plays 2, first -10h, last -2h, updated W", g)
		}
		if w.TotalBefore != 3600 || w.TotalAfter != 7200 || w.Game.TotalPlaySeconds != 7200 {
			t.Errorf("TotalBefore/After/Game = %d/%d/%d, want 3600/7200/7200", w.TotalBefore, w.TotalAfter, w.Game.TotalPlaySeconds)
		}
	})

	t.Run("SED-004/jogo sem sessões: first = início, last = fim, play_count = 1", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades"})
		start, end := sesAt(-5*h), sesAt(-4*h)
		sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: start, End: end, Active: h})
		g := sesGame(t, s, ids[0])
		if g.First != start.UnixMilli() || g.Last != end.UnixMilli() || g.Plays != 1 || g.Total != 3600 {
			t.Errorf("game = %+v, want first start, last end, plays 1, total 3600", g)
		}
	})

	t.Run("SED-004/o ajuste manual se conserva: total 10 h (8 h de sessões e 2 h de ajuste) + sessão de 1 h = 11 h", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 10 * 3600})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-20 * h), End: sesAt(-12 * h), Active: 8 * 3600, Source: "import"})
		sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: h})
		if g := sesGame(t, s, ids[0]); g.Total != 11*3600 {
			t.Errorf("total = %d, want %d", g.Total, 11*3600)
		}
	})

	t.Run("SED-005/includedInTotal mantém o total e atualiza first, last e contagem", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 5 * 3600})
		start, end := sesAt(-3*h), sesAt(-2*h)
		w := sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: start, End: end, Active: h, IncludedInTotal: true})
		g := sesGame(t, s, ids[0])
		if g.Total != 5*3600 || g.First != start.UnixMilli() || g.Last != end.UnixMilli() || g.Plays != 1 {
			t.Errorf("game = %+v, want total 18000, first start, last end, plays 1", g)
		}
		if w.TotalBefore != 5*3600 || w.TotalAfter != 5*3600 {
			t.Errorf("TotalBefore/After = %d/%d, want 18000/18000", w.TotalBefore, w.TotalAfter)
		}
	})

	t.Run("SED-005/X − Y igual ao tempo é aceito; X − Y menor é SessionEditError exceeds-adjustment e nada muda", func(t *testing.T) {
		t.Parallel()
		// X = 3h, Y = 2h -> adjustment of 1h.
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3 * 3600})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-20 * h), End: sesAt(-18 * h), Active: 2 * 3600})

		before := sesGame(t, s, ids[0])
		n := sesCount(t, s)
		_, err := s.CreateManualSession(t.Context(), store.ManualSession{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2*h + time.Minute), Active: h + time.Minute, IncludedInTotal: true}, sesW)
		sesRequireReason(t, err, domain.ReasonExceedsAdjustment)
		if sesGame(t, s, ids[0]) != before || sesCount(t, s) != n {
			t.Errorf("a refused request changed the database")
		}

		sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: h, IncludedInTotal: true})
		if g := sesGame(t, s, ids[0]); g.Total != 3*3600 {
			t.Errorf("total = %d, want 10800 (unchanged)", g.Total)
		}
	})

	t.Run("SED-005/includedInTotal sem ajuste positivo é exceeds-adjustment", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3600})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-20 * h), End: sesAt(-19 * h), Active: 3600})
		_, err := s.CreateManualSession(t.Context(), store.ManualSession{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: h, IncludedInTotal: true}, sesW)
		sesRequireReason(t, err, domain.ReasonExceedsAdjustment)
	})

	t.Run("SED-012/first é o menor início e last é o maior fim das contadas; descartada e aberta não entram", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades"})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-100 * h), End: sesAt(-99 * h), Active: 20, State: "discarded"})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-1 * h), Active: 20, State: "running"})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-50 * h), End: sesAt(-49 * h), Active: 3600, Source: "import"})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-30 * h), End: sesAt(-29 * h), Active: 3600})
		sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-20 * h), End: sesAt(-19 * h), Active: h})
		g := sesGame(t, s, ids[0])
		if g.First != sesAt(-50*h).UnixMilli() || g.Last != sesAt(-19*h).UnixMilli() || g.Plays != 3 {
			t.Errorf("game = %+v, want first -50h, last -19h, plays 3", g)
		}
	})

	t.Run("SED-013/total no teto 3599996400 não passa do teto", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3599996400 - 60})
		w := sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: h})
		if g := sesGame(t, s, ids[0]); g.Total != 3599996400 || w.TotalAfter != 3599996400 {
			t.Errorf("total/TotalAfter = %d/%d, want 3599996400", g.Total, w.TotalAfter)
		}
	})

	t.Run("SED-014/falha ao atualizar o jogo (trigger BEFORE UPDATE ON game com RAISE(ABORT)) desfaz a sessão inserida", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 100})
		libExec(t, s.DB(), `CREATE TRIGGER game_block BEFORE UPDATE ON game BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
		before := sesGame(t, s, ids[0])
		_, err := s.CreateManualSession(t.Context(), store.ManualSession{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: h}, sesW)
		if err == nil {
			t.Fatalf("err = nil, want the trigger failure")
		}
		if n := sesCount(t, s); n != 0 {
			t.Errorf("sessions = %d, want 0 (rolled back)", n)
		}
		if sesGame(t, s, ids[0]) != before {
			t.Errorf("game changed after a failed create")
		}
	})

	validation := []struct {
		name   string
		id     string
		m      func(g int64) store.ManualSession
		reason domain.EditReason
	}{
		{"SED-015/fim igual ao início é end-not-after-start", "SED-015", func(g int64) store.ManualSession {
			return store.ManualSession{GameID: g, Start: sesAt(-h), End: sesAt(-h), Active: 30 * time.Second}
		}, domain.ReasonEndNotAfterStart},
		{"SED-015/fim antes do início é end-not-after-start", "SED-015", func(g int64) store.ManualSession {
			return store.ManualSession{GameID: g, Start: sesAt(-h), End: sesAt(-2 * h), Active: 30 * time.Second}
		}, domain.ReasonEndNotAfterStart},
		{"SED-016/fim depois de now é in-future", "SED-016", func(g int64) store.ManualSession {
			return store.ManualSession{GameID: g, Start: sesAt(-h), End: sesAt(time.Minute), Active: 30 * time.Minute}
		}, domain.ReasonInFuture},
		{"SED-017/início antes de 1980 é too-early", "SED-017", func(g int64) store.ManualSession {
			return store.ManualSession{GameID: g, Start: time.Date(1979, 12, 31, 23, 59, 0, 0, time.UTC), End: time.Date(1980, 1, 1, 1, 0, 0, 0, time.UTC), Active: 30 * time.Minute}
		}, domain.ReasonTooEarly},
		{"SED-018/intervalo de 25 h é span-too-long", "SED-018", func(g int64) store.ManualSession {
			return store.ManualSession{GameID: g, Start: sesAt(-25 * h), End: sesW, Active: 2 * h}
		}, domain.ReasonSpanTooLong},
		{"SED-019/29 s é active-too-short", "SED-019", func(g int64) store.ManualSession {
			return store.ManualSession{GameID: g, Start: sesAt(-h), End: sesW, Active: 29 * time.Second}
		}, domain.ReasonActiveTooShort},
		{"SED-019/tempo maior que o intervalo é active-exceeds-span", "SED-019", func(g int64) store.ManualSession {
			return store.ManualSession{GameID: g, Start: sesAt(-h), End: sesW, Active: h + time.Minute}
		}, domain.ReasonActiveExceedsSpan},
	}
	for _, c := range validation {
		t.Run(c.name+" (nada é gravado)", func(t *testing.T) {
			t.Parallel()
			s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 500})
			before := sesGame(t, s, ids[0])
			_, err := s.CreateManualSession(t.Context(), c.m(ids[0]), sesW)
			sesRequireReason(t, err, c.reason)
			if sesCount(t, s) != 0 || sesGame(t, s, ids[0]) != before {
				t.Errorf("a refused request changed the database")
			}
		})
	}

	t.Run("SED-016/fim igual a now é aceito", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades"})
		sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-2 * h), End: sesW, Active: 2 * h})
	})

	t.Run("SED-018/24 h exatas é aceito", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades"})
		sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-24 * h), End: sesW, Active: 24 * h})
	})

	t.Run("SED-020/contrato §5: encostar na rastreada é aceito e cruzar é SessionOverlapError com o id, o jogo, o título e os instantes da outra", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades"})
		other := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})
		sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-2 * h), End: sesAt(-h), Active: h})

		before := sesCount(t, s)
		_, err := s.CreateManualSession(t.Context(), store.ManualSession{GameID: ids[0], Start: sesAt(-2*h - 30*time.Minute), End: sesAt(-h), Active: h}, sesW)
		// The new manual session [-2h, -1h) also collides now; the first conflict found must be a valid one.
		var oe *store.SessionOverlapError
		if !errors.As(err, &oe) {
			t.Fatalf("err = %v (%T), want *store.SessionOverlapError", err, err)
		}
		if oe.SessionID == 0 || oe.GameID != ids[0] || oe.Title != "Hades" || oe.EndedAt == nil {
			t.Errorf("overlap = %+v, want a closed session of Hades", oe)
		}
		if sesCount(t, s) != before {
			t.Errorf("a refused request inserted a row")
		}

		// Isolated: only the tracked session, exact identification.
		s2, ids2 := newSesStore(t, libGame{Title: "Celeste"})
		o2 := sesInsert(t, s2, sesRow{GameID: ids2[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})
		_, err = s2.CreateManualSession(t.Context(), store.ManualSession{GameID: ids2[0], Start: sesAt(-2*h - 30*time.Minute), End: sesAt(-h), Active: h}, sesW)
		if !errors.As(err, &oe) {
			t.Fatalf("err = %v, want *store.SessionOverlapError", err)
		}
		if oe.SessionID != o2 || oe.GameID != ids2[0] || oe.Title != "Celeste" ||
			!oe.StartedAt.Equal(sesAt(-3*h)) || oe.EndedAt == nil || !oe.EndedAt.Equal(sesAt(-2*h)) {
			t.Errorf("overlap = %+v, want session %d of Celeste [-3h, -2h)", oe, o2)
		}
		_ = other
	})

	t.Run("SED-020/cruza uma manual de outro jogo", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades"}, libGame{Title: "Celeste"})
		o := sesInsert(t, s, sesRow{GameID: ids[1], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600, Source: "manual"})
		_, err := s.CreateManualSession(t.Context(), store.ManualSession{GameID: ids[0], Start: sesAt(-150 * time.Minute), End: sesAt(-h), Active: h}, sesW)
		var oe *store.SessionOverlapError
		if !errors.As(err, &oe) {
			t.Fatalf("err = %v, want *store.SessionOverlapError", err)
		}
		if oe.SessionID != o || oe.GameID != ids[1] || oe.Title != "Celeste" {
			t.Errorf("overlap = %+v, want manual session %d of Celeste", oe, o)
		}
	})

	t.Run("SED-020/cruza a sessão aberta [started_at, now): EndedAt nulo no erro; encostar no início da aberta é aceito", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades"}, libGame{Title: "Celeste"})
		open := sesInsert(t, s, sesRow{GameID: ids[1], Start: sesAt(-10 * time.Minute), Active: 60, State: "running"})
		_, err := s.CreateManualSession(t.Context(), store.ManualSession{GameID: ids[0], Start: sesAt(-30 * time.Minute), End: sesAt(-5 * time.Minute), Active: 20 * time.Minute}, sesW)
		var oe *store.SessionOverlapError
		if !errors.As(err, &oe) {
			t.Fatalf("err = %v, want *store.SessionOverlapError", err)
		}
		if oe.SessionID != open || oe.GameID != ids[1] || oe.EndedAt != nil || !oe.StartedAt.Equal(sesAt(-10*time.Minute)) {
			t.Errorf("overlap = %+v, want open session %d with nil EndedAt", oe, open)
		}
		sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-30 * time.Minute), End: sesAt(-10 * time.Minute), Active: 20 * time.Minute})
	})

	t.Run("SED-020/sessões importadas e descartadas não entram na conta", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades"})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-300 * h), End: sesW, Active: 100 * 3600, Source: "import"})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-h), End: sesAt(-59*time.Minute - 40*time.Second), Active: 20, State: "discarded"})
		sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-2 * h), End: sesAt(-30 * time.Minute), Active: h})
	})

	t.Run("SED-024/jogo inexistente devolve erro que satisfaz ErrGameNotFound", func(t *testing.T) {
		t.Parallel()
		s, _ := newSesStore(t, libGame{Title: "Hades"})
		_, err := s.CreateManualSession(t.Context(), store.ManualSession{GameID: 9999, Start: sesAt(-2 * h), End: sesAt(-h), Active: h}, sesW)
		if !errors.Is(err, store.ErrGameNotFound) {
			t.Errorf("err = %v, want errors.Is(err, store.ErrGameNotFound)", err)
		}
		if n := sesCount(t, s); n != 0 {
			t.Errorf("sessions = %d, want 0", n)
		}
	})

	t.Run("SED-028/status e completed_at do jogo não mudam (inclusive not_played)", func(t *testing.T) {
		t.Parallel()
		completed := sesAt(-500 * h).UnixMilli()
		s, ids := newSesStore(t,
			libGame{Title: "Novo"},
			libGame{Title: "Zerado", Status: "completed", Completed: completed})
		sesCreate(t, s, store.ManualSession{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: h})
		sesCreate(t, s, store.ManualSession{GameID: ids[1], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: h})
		if g := sesGame(t, s, ids[0]); g.Status != "not_played" || g.Completed != 0 {
			t.Errorf("Novo status/completed = %s/%d, want not_played/0", g.Status, g.Completed)
		}
		if g := sesGame(t, s, ids[1]); g.Status != "completed" || g.Completed != completed {
			t.Errorf("Zerado status/completed = %s/%d, want completed/%d", g.Status, g.Completed, completed)
		}
	})
}
