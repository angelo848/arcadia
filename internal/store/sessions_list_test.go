package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Fixtures of the session list tests (STO-027). Sessions and pauses are written
// with raw SQL: the store API that writes them is not the subject here, and the
// list must show whatever the rows hold (leitor tolerante, ADR 0012).

// libSessionRow describes one play_session row of a fixture. Ended 0 = NULL
// (open session); EndReason "" = NULL. Source is always tracked.
type libSessionRow struct {
	GameID    int64
	Started   int64 // ms UTC
	Ended     int64 // ms UTC; 0 = NULL
	Active    int64 // active_seconds
	State     string
	EndReason string
}

// libInsertSession writes one play_session row and returns its id.
func libInsertSession(t *testing.T, x dbx, r libSessionRow) int64 {
	t.Helper()
	var endReason any
	if r.EndReason != "" {
		endReason = r.EndReason
	}
	res, err := x.ExecContext(t.Context(),
		`INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source, end_reason)
		VALUES (?, ?, ?, ?, ?, ?, 'tracked', ?)`,
		r.GameID, r.Started, libNullMs(r.Ended), r.Started, r.Active, r.State, endReason)
	if err != nil {
		t.Fatalf("insert session of game %d: %v", r.GameID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("id of session of game %d: %v", r.GameID, err)
	}
	return id
}

// libInsertPause writes one pause_interval of a session (hotkey). resumed 0 = open.
func libInsertPause(t *testing.T, x dbx, sessionID, pausedAt, resumedAt int64) {
	t.Helper()
	libExec(t, x, `INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason) VALUES (?, ?, ?, 'hotkey')`,
		sessionID, pausedAt, libNullMs(resumedAt))
}

// wantSession is the expected SessionItem of one row of a list.
type wantSession struct {
	ID        int64
	State     domain.SessionState
	Started   int64 // ms UTC
	Ended     int64 // ms UTC; 0 = open
	Active    int64
	EndReason string // "" = nil
}

// requireSessions checks the items of a page, in order, field by field.
func requireSessions(t *testing.T, gameID int64, got []store.SessionItem, want []wantSession) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sessions = %d items, want %d (got ids %v)", len(got), len(want), sessionItemIDs(got))
	}
	for i, w := range want {
		g := got[i]
		if g.ID != w.ID || g.GameID != gameID {
			t.Errorf("item %d: ID/GameID = %d/%d, want %d/%d", i, g.ID, g.GameID, w.ID, gameID)
		}
		if g.State != w.State {
			t.Errorf("item %d: State = %q, want %q", i, g.State, w.State)
		}
		if !g.StartedAt.Equal(time.UnixMilli(w.Started)) {
			t.Errorf("item %d: StartedAt = %v, want %v", i, g.StartedAt, time.UnixMilli(w.Started))
		}
		if w.Ended == 0 {
			if !g.EndedAt.IsZero() {
				t.Errorf("item %d: EndedAt = %v, want zero (open session)", i, g.EndedAt)
			}
		} else if !g.EndedAt.Equal(time.UnixMilli(w.Ended)) {
			t.Errorf("item %d: EndedAt = %v, want %v", i, g.EndedAt, time.UnixMilli(w.Ended))
		}
		if g.ActiveSeconds != w.Active {
			t.Errorf("item %d: ActiveSeconds = %d, want %d", i, g.ActiveSeconds, w.Active)
		}
		if g.EndReason != w.EndReason {
			t.Errorf("item %d: EndReason = %q, want %q", i, g.EndReason, w.EndReason)
		}
	}
}

func sessionItemIDs(items []store.SessionItem) []int64 {
	out := make([]int64, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

// mustGameSessions calls GameSessions and fails the test on error.
func mustGameSessions(t *testing.T, s *store.Store, gameID int64, offset, limit int) store.SessionPage {
	t.Helper()
	page, err := s.GameSessions(t.Context(), gameID, offset, limit)
	if err != nil {
		t.Fatalf("GameSessions(%d, %d, %d): %v", gameID, offset, limit, err)
	}
	return page
}

func TestGameSessions(t *testing.T) {
	t.Parallel()

	t.Run("STO-027/sessões ordenadas por started_at e id decrescentes, com ended, discarded e a aberta", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ids := seedLibrary(t, s, libGame{Title: "Hades"}, libGame{Title: "Celeste"})
		hades, celeste := ids["Hades"], ids["Celeste"]

		a := libInsertSession(t, s.DB(), libSessionRow{GameID: hades, Started: libAt(1, 10), Ended: libAt(1, 11),
			Active: 3600, State: "ended", EndReason: "exited"})
		b := libInsertSession(t, s.DB(), libSessionRow{GameID: hades, Started: libAt(2, 9), Ended: libAt(2, 10),
			Active: 30, State: "discarded", EndReason: "exited"})
		c := libInsertSession(t, s.DB(), libSessionRow{GameID: hades, Started: libAt(5, 8), Active: 120, State: "running"})
		// Same started_at as c and a larger id: it must come before c (id decrescente).
		e := libInsertSession(t, s.DB(), libSessionRow{GameID: hades, Started: libAt(5, 8), Ended: libAt(5, 9),
			Active: 60, State: "ended", EndReason: "exited"})
		libInsertSession(t, s.DB(), libSessionRow{GameID: celeste, Started: libAt(6, 0), Ended: libAt(6, 1),
			Active: 10, State: "ended", EndReason: "exited"})

		page := mustGameSessions(t, s, hades, 0, domain.DefaultSessionLimit)
		if page.Total != 4 {
			t.Errorf("Total = %d, want 4 (all sessions of Hades)", page.Total)
		}
		requireSessions(t, hades, page.Sessions, []wantSession{
			{ID: e, State: domain.SessionEnded, Started: libAt(5, 8), Ended: libAt(5, 9), Active: 60, EndReason: "exited"},
			{ID: c, State: domain.SessionRunning, Started: libAt(5, 8), Active: 120},
			{ID: b, State: domain.SessionDiscarded, Started: libAt(2, 9), Ended: libAt(2, 10), Active: 30, EndReason: "exited"},
			{ID: a, State: domain.SessionEnded, Started: libAt(1, 10), Ended: libAt(1, 11), Active: 3600, EndReason: "exited"},
		})
	})

	t.Run("STO-027/sessão aberta tem EndedAt zero e discarded aparece na lista", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := seedLibrary(t, s, libGame{Title: "Celeste"})["Celeste"]

		open := libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(3, 20), Active: 45, State: "paused"})
		discarded := libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(2, 20), Ended: libAt(2, 21),
			Active: 5, State: "discarded", EndReason: "exited"})

		page := mustGameSessions(t, s, g, 0, domain.DefaultSessionLimit)
		requireSessions(t, g, page.Sessions, []wantSession{
			{ID: open, State: domain.SessionPaused, Started: libAt(3, 20), Active: 45},
			{ID: discarded, State: domain.SessionDiscarded, Started: libAt(2, 20), Ended: libAt(2, 21), Active: 5, EndReason: "exited"},
		})
		if !page.Sessions[0].EndedAt.IsZero() {
			t.Errorf("open session EndedAt = %v, want zero", page.Sessions[0].EndedAt)
		}
		if page.Sessions[0].EndReason != "" {
			t.Errorf("open session EndReason = %q, want empty (NULL)", page.Sessions[0].EndReason)
		}
	})

	t.Run("STO-027/total conta todas e a página respeita offset e limit", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ids := seedLibrary(t, s, libGame{Title: "Paged"}, libGame{Title: "Other"})
		paged := ids["Paged"]

		var inserted []int64
		for h := 1; h <= 5; h++ {
			inserted = append(inserted, libInsertSession(t, s.DB(), libSessionRow{GameID: paged,
				Started: libAt(1, h), Ended: libAt(2, h), Active: int64(h), State: "ended", EndReason: "exited"}))
		}
		for h := 1; h <= 2; h++ {
			libInsertSession(t, s.DB(), libSessionRow{GameID: ids["Other"], Started: libAt(4, h),
				Ended: libAt(4, h+1), Active: 1, State: "ended", EndReason: "exited"})
		}
		// Most recent first: the reverse of insertion order (started_at grows with h).
		want := make([]int64, 0, len(inserted))
		for i := len(inserted) - 1; i >= 0; i-- {
			want = append(want, inserted[i])
		}

		var got []int64
		for _, off := range []int{0, 2, 4} {
			page := mustGameSessions(t, s, paged, off, 2)
			if page.Total != 5 {
				t.Errorf("offset %d: Total = %d, want 5 (all sessions, not the page)", off, page.Total)
			}
			got = append(got, sessionItemIDs(page.Sessions)...)
		}
		if len(got) != len(want) || !equalIDs(got, want) {
			t.Errorf("pages (offset 0,2,4; limit 2) = %v, want %v with no repeats and no gaps", got, want)
		}

		past := mustGameSessions(t, s, paged, 5, 2)
		if past.Sessions == nil || len(past.Sessions) != 0 {
			t.Errorf("offset 5 Sessions = %v, want [] (non-nil)", past.Sessions)
		}
		if past.Total != 5 {
			t.Errorf("offset 5 Total = %d, want 5", past.Total)
		}
	})

	t.Run("STO-027/jogo sem sessões dá Sessions [] e Total 0", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ids := seedLibrary(t, s, libGame{Title: "Empty"}, libGame{Title: "Busy"})
		libInsertSession(t, s.DB(), libSessionRow{GameID: ids["Busy"], Started: libAt(1, 1), Ended: libAt(1, 2),
			Active: 60, State: "ended", EndReason: "exited"})

		page := mustGameSessions(t, s, ids["Empty"], 0, domain.DefaultSessionLimit)
		if page.Sessions == nil {
			t.Errorf("Sessions = nil, want [] (non-nil)")
		}
		if len(page.Sessions) != 0 {
			t.Errorf("Sessions = %v, want empty", page.Sessions)
		}
		if page.Total != 0 {
			t.Errorf("Total = %d, want 0", page.Total)
		}
	})

	t.Run("STO-027/jogo inexistente devolve ErrGameNotFound", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s, libGame{Title: "Existe"})
		for _, id := range []int64{999, 0, -1} {
			_, err := s.GameSessions(t.Context(), id, 0, domain.DefaultSessionLimit)
			if !errors.Is(err, store.ErrGameNotFound) {
				t.Errorf("GameSessions(%d) err = %v, want errors.Is(err, store.ErrGameNotFound)", id, err)
			}
		}
	})
}

// Added 2026-11-25 (fase 16): Source and EditedAt of each item (SED-002, SED-033).
func TestGameSessionsSourceEdited(t *testing.T) {
	t.Parallel()

	setSourceEdited := func(t *testing.T, s *store.Store, id int64, source string, editedAt int64) {
		t.Helper()
		libExec(t, s.DB(), `UPDATE play_session SET source = ?, edited_at = ? WHERE id = ?`, source, libNullMs(editedAt), id)
	}

	t.Run("SED-033/GameSessions traz Source e EditedAt de cada item, na mesma ordem e paginação de antes", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := seedLibrary(t, s, libGame{Title: "Hades"})["Hades"]
		a := libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(1, 10), Ended: libAt(1, 11), Active: 3600, State: "ended", EndReason: "exited"})
		b := libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(2, 10), Ended: libAt(2, 11), Active: 3600, State: "ended"})
		c := libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(3, 10), Ended: libAt(3, 11), Active: 3600, State: "ended"})
		setSourceEdited(t, s, a, "tracked", libAt(4, 1))
		setSourceEdited(t, s, b, "import", 0)
		setSourceEdited(t, s, c, "manual", libAt(4, 2))

		page := mustGameSessions(t, s, g, 0, domain.DefaultSessionLimit)
		if got := sessionItemIDs(page.Sessions); !equalIDs(got, []int64{c, b, a}) {
			t.Fatalf("ids = %v, want %v", got, []int64{c, b, a})
		}
		wantSource := []string{"manual", "import", "tracked"}
		wantEdited := []int64{libAt(4, 2), 0, libAt(4, 1)}
		for i, it := range page.Sessions {
			if it.Source != wantSource[i] {
				t.Errorf("item %d: Source = %q, want %q", i, it.Source, wantSource[i])
			}
			if wantEdited[i] == 0 {
				if !it.EditedAt.IsZero() {
					t.Errorf("item %d: EditedAt = %v, want zero", i, it.EditedAt)
				}
			} else if !it.EditedAt.Equal(time.UnixMilli(wantEdited[i])) {
				t.Errorf("item %d: EditedAt = %v, want %v", i, it.EditedAt, time.UnixMilli(wantEdited[i]))
			}
		}
		paged := mustGameSessions(t, s, g, 1, 1)
		if paged.Total != 3 || len(paged.Sessions) != 1 || paged.Sessions[0].ID != b || paged.Sessions[0].Source != "import" {
			t.Errorf("page offset 1 limit 1 = %+v, want only session %d (import), Total 3", paged, b)
		}
	})

	t.Run("SED-033/sessão nunca editada tem EditedAt zero", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := seedLibrary(t, s, libGame{Title: "Celeste"})["Celeste"]
		libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(1, 10), Ended: libAt(1, 11), Active: 60, State: "ended"})
		libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(2, 10), Active: 60, State: "running"})
		for i, it := range mustGameSessions(t, s, g, 0, domain.DefaultSessionLimit).Sessions {
			if !it.EditedAt.IsZero() {
				t.Errorf("item %d: EditedAt = %v, want zero", i, it.EditedAt)
			}
			if it.Source != "tracked" {
				t.Errorf("item %d: Source = %q, want tracked", i, it.Source)
			}
		}
	})

	t.Run("SED-002/valor de source desconhecido no banco chega como está e a página não falha", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := seedLibrary(t, s, libGame{Title: "Futuro"})["Futuro"]
		id := libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(1, 10), Ended: libAt(1, 11), Active: 60, State: "ended"})
		setSourceEdited(t, s, id, "cloud-sync", 0)
		page := mustGameSessions(t, s, g, 0, domain.DefaultSessionLimit)
		if len(page.Sessions) != 1 || page.Sessions[0].Source != "cloud-sync" {
			t.Errorf("sessions = %+v, want one item with Source cloud-sync", page.Sessions)
		}
	})
}

// equalIDs reports whether a and b hold the same ids in the same order.
func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
