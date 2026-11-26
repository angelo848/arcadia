package store_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Tests of Store.EditSession (fase 16, T09). Helpers sesW, sesAt, sesInsert, sesGame,
// sesCount, sesRequireReason and newSesStore live in session_edit_create_test.go.

// sesFullRow is the whole play_session row, NULLs as zero values.
type sesFullRow struct {
	Start, End, LastSeen, Active int64
	State, Source, EndReason     string
	Edited                       int64
}

func sesFull(t *testing.T, s *store.Store, id int64) sesFullRow {
	t.Helper()
	var r sesFullRow
	err := s.DB().QueryRowContext(t.Context(),
		`SELECT started_at, COALESCE(ended_at, 0), last_seen_at, active_seconds, state, source,
		COALESCE(end_reason, ''), COALESCE(edited_at, 0) FROM play_session WHERE id = ?`, id).
		Scan(&r.Start, &r.End, &r.LastSeen, &r.Active, &r.State, &r.Source, &r.EndReason, &r.Edited)
	if err != nil {
		t.Fatalf("read session %d: %v", id, err)
	}
	return r
}

func sesPause(t *testing.T, s *store.Store, session int64, from, to time.Time) {
	t.Helper()
	libExec(t, s.DB(), `INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason) VALUES (?, ?, ?, 'manual')`,
		session, from.UnixMilli(), to.UnixMilli())
}

// sesPauses returns [paused_at, resumed_at] in ms, ordered.
func sesPauses(t *testing.T, s *store.Store, session int64) [][2]int64 {
	t.Helper()
	rows, err := s.DB().QueryContext(t.Context(),
		`SELECT paused_at, COALESCE(resumed_at, 0) FROM pause_interval WHERE session_id = ? ORDER BY paused_at`, session)
	if err != nil {
		t.Fatalf("read pauses: %v", err)
	}
	defer rows.Close()
	var out [][2]int64
	for rows.Next() {
		var p [2]int64
		if err := rows.Scan(&p[0], &p[1]); err != nil {
			t.Fatalf("scan pause: %v", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("pauses: %v", err)
	}
	return out
}

func sesEdit(t *testing.T, s *store.Store, id int64, p store.SessionPatch) store.SessionWrite {
	t.Helper()
	w, err := s.EditSession(t.Context(), id, p, sesW)
	if err != nil {
		t.Fatalf("EditSession(%d, %+v): %v", id, p, err)
	}
	return w
}

func tp(v time.Time) *time.Time         { return &v }
func dp(v time.Duration) *time.Duration { return &v }

func TestEditSession(t *testing.T) {
	t.Parallel()
	h := time.Hour
	m := time.Minute

	t.Run("SED-006/só início, só fim, só tempo jogado e os três: apenas os campos presentes mudam; last_seen_at acompanha o fim", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			patch store.SessionPatch
			want  func(r *sesFullRow)
		}{
			{"início", store.SessionPatch{Start: tp(sesAt(-5*h - 30*m))}, func(r *sesFullRow) { r.Start = sesAt(-5*h - 30*m).UnixMilli() }},
			{"fim", store.SessionPatch{End: tp(sesAt(-3*h - 30*m))}, func(r *sesFullRow) {
				r.End = sesAt(-3*h - 30*m).UnixMilli()
				r.LastSeen = r.End
			}},
			{"tempo", store.SessionPatch{Active: dp(3300 * time.Second)}, func(r *sesFullRow) { r.Active = 3300 }},
			{"os três", store.SessionPatch{Start: tp(sesAt(-6 * h)), End: tp(sesAt(-3 * h)), Active: dp(2 * h)}, func(r *sesFullRow) {
				r.Start = sesAt(-6 * h).UnixMilli()
				r.End = sesAt(-3 * h).UnixMilli()
				r.LastSeen = r.End
				r.Active = 7200
			}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
				id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
				want := sesFull(t, s, id)
				c.want(&want)
				want.Edited = sesW.UnixMilli()
				w := sesEdit(t, s, id, c.patch)
				if got := sesFull(t, s, id); got != want {
					t.Errorf("row = %+v, want %+v", got, want)
				}
				if !w.Changed || w.Session.ID != id {
					t.Errorf("Changed/Session.ID = %v/%d", w.Changed, w.Session.ID)
				}
			})
		}
	})

	t.Run("SED-006/source, state e end_reason ficam como estavam, para as origens tracked, import e manual; edited_at = W", func(t *testing.T) {
		t.Parallel()
		for _, src := range []string{"tracked", "import", "manual"} {
			t.Run(src, func(t *testing.T) {
				t.Parallel()
				s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
				id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000, Source: src})
				libExec(t, s.DB(), `UPDATE play_session SET end_reason = 'exited' WHERE id = ?`, id)
				w := sesEdit(t, s, id, store.SessionPatch{Active: dp(3300 * time.Second)})
				r := sesFull(t, s, id)
				if r.Source != src || r.State != "ended" || r.EndReason != "exited" || r.Edited != sesW.UnixMilli() {
					t.Errorf("row = %+v, want source %s, ended, exited, edited W", r, src)
				}
				if !w.Session.EditedAt.Equal(sesW) || w.Previous == nil || w.Previous.ActiveSeconds != 3000 || w.Session.ActiveSeconds != 3300 {
					t.Errorf("write = %+v", w)
				}
			})
		}
	})

	t.Run("SED-007/total passa a X + (a' − a); contagem, first e last recalculados", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 7200, Plays: 2, FirstPlayed: sesAt(-10 * h).UnixMilli(), LastPlayed: sesAt(-2 * h).UnixMilli()})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-10 * h), End: sesAt(-9 * h), Active: 3600})
		b := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})
		w := sesEdit(t, s, b, store.SessionPatch{End: tp(sesAt(-90 * m)), Active: dp(90 * m)})
		g := sesGame(t, s, ids[0])
		if g.Total != 9000 || g.Plays != 2 || g.First != sesAt(-10*h).UnixMilli() || g.Last != sesAt(-90*m).UnixMilli() {
			t.Errorf("game = %+v, want total 9000, plays 2, first -10h, last -90m", g)
		}
		if w.TotalBefore != 7200 || w.TotalAfter != 9000 || w.Game.TotalPlaySeconds != 9000 {
			t.Errorf("TotalBefore/After/Game = %d/%d/%d", w.TotalBefore, w.TotalAfter, w.Game.TotalPlaySeconds)
		}
	})

	t.Run("SED-007/exemplo: total 10 h (8 h de sessões e 2 h de ajuste), sessão de 1 h editada para 1 h 30 = 10 h 30", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 10 * 3600})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-30 * h), End: sesAt(-20 * h), Active: 7 * 3600, Source: "import"})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})
		sesEdit(t, s, id, store.SessionPatch{End: tp(sesAt(-90 * m)), Active: dp(90 * m)})
		if g := sesGame(t, s, ids[0]); g.Total != 10*3600+1800 {
			t.Errorf("total = %d, want %d", g.Total, 10*3600+1800)
		}
	})

	t.Run("SED-007/updated_at do jogo só muda se algum campo do jogo mudou", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 10800, Plays: 3, FirstPlayed: sesAt(-10 * h).UnixMilli(), LastPlayed: sesAt(-2 * h).UnixMilli()})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-10 * h), End: sesAt(-9 * h), Active: 3600})
		mid := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-6 * h), End: sesAt(-5 * h), Active: 3600})
		last := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})
		before := sesGame(t, s, ids[0])
		sesEdit(t, s, mid, store.SessionPatch{Start: tp(sesAt(-6*h - 30*m))})
		if g := sesGame(t, s, ids[0]); g != before {
			t.Errorf("game = %+v, want unchanged %+v", g, before)
		}
		sesEdit(t, s, last, store.SessionPatch{Start: tp(sesAt(-4 * h)), Active: dp(2 * h)})
		if g := sesGame(t, s, ids[0]); g.Total != 14400 || g.Updated != sesW.UnixMilli() {
			t.Errorf("game = %+v, want total 14400, updated W", g)
		}
	})

	t.Run("SED-008/pedido igual ao gravado não grava (edited_at e updated_at do jogo intactos), Changed = false e devolve a sessão e o jogo como estão", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
		start, end := sesAt(-5*h), sesAt(-4*h)
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: start, End: end, Active: 3000})
		rowBefore, gameBefore := sesFull(t, s, id), sesGame(t, s, ids[0])
		w := sesEdit(t, s, id, store.SessionPatch{Start: tp(start), End: tp(end), Active: dp(3000 * time.Second)})
		if w.Changed {
			t.Errorf("Changed = true, want false")
		}
		if sesFull(t, s, id) != rowBefore || sesGame(t, s, ids[0]) != gameBefore {
			t.Errorf("an equal request wrote something")
		}
		if w.Session.ID != id || w.Session.ActiveSeconds != 3000 || w.Game.TotalPlaySeconds != 3000 {
			t.Errorf("returned = %+v / total %d", w.Session, w.Game.TotalPlaySeconds)
		}
	})

	t.Run("SED-008/igualdade é comparada em ms", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
		start, end := sesAt(-5*h), sesAt(-4*h)
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: start, End: end, Active: 3000})
		before := sesFull(t, s, id)
		w := sesEdit(t, s, id, store.SessionPatch{Start: tp(start.Add(400 * time.Microsecond)), End: tp(end.Add(900 * time.Microsecond))})
		if w.Changed || sesFull(t, s, id) != before {
			t.Errorf("sub-millisecond difference was written (Changed = %v)", w.Changed)
		}
	})

	t.Run("SED-009/pausa que sai do novo intervalo é cortada nas bordas", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
		sesPause(t, s, id, sesAt(-4*h-55*m), sesAt(-4*h-40*m))
		sesPause(t, s, id, sesAt(-4*h-10*m), sesAt(-4*h-5*m))
		sesEdit(t, s, id, store.SessionPatch{Start: tp(sesAt(-4*h - 50*m)), End: tp(sesAt(-4*h - 7*m)), Active: dp(40 * m)})
		want := [][2]int64{
			{sesAt(-4*h - 50*m).UnixMilli(), sesAt(-4*h - 40*m).UnixMilli()},
			{sesAt(-4*h - 10*m).UnixMilli(), sesAt(-4*h - 7*m).UnixMilli()},
		}
		if got := sesPauses(t, s, id); !reflect.DeepEqual(got, want) {
			t.Errorf("pauses = %v, want %v", got, want)
		}
	})

	t.Run("SED-009/pausa com duração zero ou toda fora é apagada", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
		sesPause(t, s, id, sesAt(-4*h-58*m), sesAt(-4*h-52*m)) // all outside
		sesPause(t, s, id, sesAt(-4*h-55*m), sesAt(-4*h-50*m)) // zero after the cut
		sesPause(t, s, id, sesAt(-4*h-40*m), sesAt(-4*h-30*m)) // kept
		sesEdit(t, s, id, store.SessionPatch{Start: tp(sesAt(-4*h - 50*m))})
		want := [][2]int64{{sesAt(-4*h - 40*m).UnixMilli(), sesAt(-4*h - 30*m).UnixMilli()}}
		if got := sesPauses(t, s, id); !reflect.DeepEqual(got, want) {
			t.Errorf("pauses = %v, want %v", got, want)
		}
	})

	t.Run("SED-009/mudar só o tempo jogado não mexe nas pausas", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
		sesPause(t, s, id, sesAt(-4*h-55*m), sesAt(-4*h-40*m))
		before := sesPauses(t, s, id)
		sesEdit(t, s, id, store.SessionPatch{Active: dp(2400 * time.Second)})
		if got := sesPauses(t, s, id); !reflect.DeepEqual(got, before) {
			t.Errorf("pauses = %v, want %v", got, before)
		}
	})

	t.Run("SED-009/active_seconds fica o valor gravado, não é recalculado das pausas", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
		sesPause(t, s, id, sesAt(-4*h-55*m), sesAt(-4*h-40*m))
		sesEdit(t, s, id, store.SessionPatch{Start: tp(sesAt(-4*h - 50*m))})
		if r := sesFull(t, s, id); r.Active != 3000 {
			t.Errorf("active_seconds = %d, want 3000", r.Active)
		}
	})

	t.Run("SED-012/editar a primeira ou a última sessão recalcula first_played_at e last_played_at", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 7200, Plays: 2, FirstPlayed: sesAt(-10 * h).UnixMilli(), LastPlayed: sesAt(-2 * h).UnixMilli()})
		a := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-10 * h), End: sesAt(-9 * h), Active: 3600})
		b := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})
		sesEdit(t, s, a, store.SessionPatch{Start: tp(sesAt(-11 * h))})
		if g := sesGame(t, s, ids[0]); g.First != sesAt(-11*h).UnixMilli() || g.Last != sesAt(-2*h).UnixMilli() {
			t.Errorf("first/last = %d/%d", g.First, g.Last)
		}
		sesEdit(t, s, b, store.SessionPatch{End: tp(sesAt(-h))})
		if g := sesGame(t, s, ids[0]); g.Last != sesAt(-h).UnixMilli() {
			t.Errorf("last = %d, want -1h", g.Last)
		}
	})

	t.Run("SED-013/delta negativo maior que o total deixa 0", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 1000, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})
		w := sesEdit(t, s, id, store.SessionPatch{Active: dp(1800 * time.Second)})
		if g := sesGame(t, s, ids[0]); g.Total != 0 || w.TotalAfter != 0 {
			t.Errorf("total/TotalAfter = %d/%d, want 0", g.Total, w.TotalAfter)
		}
	})

	t.Run("SED-013/delta que passa do teto fica no teto", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3599996400 - 60, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})
		sesEdit(t, s, id, store.SessionPatch{Start: tp(sesAt(-4 * h)), Active: dp(2 * h)})
		if g := sesGame(t, s, ids[0]); g.Total != 3599996400 {
			t.Errorf("total = %d, want 3599996400", g.Total)
		}
	})

	t.Run("SED-014/falha ao gravar o jogo (trigger) desfaz a sessão e o corte das pausas", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
		sesPause(t, s, id, sesAt(-4*h-55*m), sesAt(-4*h-40*m))
		libExec(t, s.DB(), `CREATE TRIGGER game_block BEFORE UPDATE ON game BEGIN SELECT RAISE(ABORT, 'blocked'); END`)
		rowBefore, pausesBefore, gameBefore := sesFull(t, s, id), sesPauses(t, s, id), sesGame(t, s, ids[0])
		_, err := s.EditSession(t.Context(), id, store.SessionPatch{Start: tp(sesAt(-4*h - 50*m)), Active: dp(2400 * time.Second)}, sesW)
		if err == nil {
			t.Fatalf("err = nil, want the trigger failure")
		}
		if sesFull(t, s, id) != rowBefore || !reflect.DeepEqual(sesPauses(t, s, id), pausesBefore) || sesGame(t, s, ids[0]) != gameBefore {
			t.Errorf("a failed edit left changes behind")
		}
	})

	refusals := []struct {
		name    string
		reason  domain.EditReason
		session sesRow // GameID filled by the test
		patch   store.SessionPatch
	}{
		{"SED-015/fim resultante não posterior ao início resultante (patch só do fim)", domain.ReasonEndNotAfterStart,
			sesRow{Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000}, store.SessionPatch{End: tp(sesAt(-5 * h))}},
		{"SED-016/fim resultante depois de now", domain.ReasonInFuture,
			sesRow{Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000}, store.SessionPatch{End: tp(sesAt(m))}},
		{"SED-017/início resultante antes de 1980", domain.ReasonTooEarly,
			sesRow{Start: time.Date(1980, 1, 1, 0, 30, 0, 0, time.UTC), End: time.Date(1980, 1, 1, 1, 30, 0, 0, time.UTC), Active: 3000, Source: "manual"},
			store.SessionPatch{Start: tp(time.Date(1979, 12, 31, 23, 50, 0, 0, time.UTC))}},
		{"SED-018/início ou fim que dá mais de 24 h em rastreada", domain.ReasonSpanTooLong,
			sesRow{Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000}, store.SessionPatch{Start: tp(sesAt(-30 * h))}},
		{"SED-018/início ou fim que dá mais de 24 h em manual", domain.ReasonSpanTooLong,
			sesRow{Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000, Source: "manual"}, store.SessionPatch{Start: tp(sesAt(-30 * h))}},
		{"SED-019/tempo menor que 30 s", domain.ReasonActiveTooShort,
			sesRow{Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000}, store.SessionPatch{Active: dp(29 * time.Second)}},
		{"SED-019/tempo maior que o intervalo resultante", domain.ReasonActiveExceedsSpan,
			sesRow{Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000}, store.SessionPatch{Active: dp(h + time.Second)}},
		{"SED-019/2 h jogadas e patch só do fim para 1 h depois do início", domain.ReasonActiveExceedsSpan,
			sesRow{Start: sesAt(-5 * h), End: sesAt(-3 * h), Active: 7200}, store.SessionPatch{End: tp(sesAt(-4 * h))}},
		{"SED-021/importada: end-not-after-start", domain.ReasonEndNotAfterStart,
			sesRow{Start: sesAt(-300 * h), End: sesAt(-200 * h), Active: 3600, Source: "import"}, store.SessionPatch{End: tp(sesAt(-300 * h))}},
		{"SED-021/importada: in-future", domain.ReasonInFuture,
			sesRow{Start: sesAt(-300 * h), End: sesAt(-200 * h), Active: 3600, Source: "import"}, store.SessionPatch{End: tp(sesAt(m))}},
		{"SED-021/importada: too-early", domain.ReasonTooEarly,
			sesRow{Start: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC), End: time.Date(1980, 1, 5, 0, 0, 0, 0, time.UTC), Active: 3600, Source: "import"},
			store.SessionPatch{Start: tp(time.Date(1979, 12, 31, 0, 0, 0, 0, time.UTC))}},
		{"SED-021/importada: active-too-short", domain.ReasonActiveTooShort,
			sesRow{Start: sesAt(-300 * h), End: sesAt(-200 * h), Active: 3600, Source: "import"}, store.SessionPatch{Active: dp(29 * time.Second)}},
		{"SED-023/descartada", domain.ReasonDiscarded,
			sesRow{Start: sesAt(-5 * h), End: sesAt(-5*h + 20*time.Second), Active: 20, State: "discarded"}, store.SessionPatch{Active: dp(time.Minute)}},
	}
	for _, c := range refusals {
		t.Run(c.name+" (nada muda)", func(t *testing.T) {
			t.Parallel()
			s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
			c.session.GameID = ids[0]
			id := sesInsert(t, s, c.session)
			rowBefore, gameBefore := sesFull(t, s, id), sesGame(t, s, ids[0])
			_, err := s.EditSession(t.Context(), id, c.patch, sesW)
			sesRequireReason(t, err, c.reason)
			if sesFull(t, s, id) != rowBefore || sesGame(t, s, ids[0]) != gameBefore {
				t.Errorf("a refused edit changed the database")
			}
		})
	}

	t.Run("SED-018/patch só do tempo jogado de uma rastreada com intervalo de 30 h (gravada antes da regra) é aceito", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3600, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-40 * h), End: sesAt(-10 * h), Active: 3600})
		sesEdit(t, s, id, store.SessionPatch{Active: dp(2 * h)})
		if r := sesFull(t, s, id); r.Active != 7200 {
			t.Errorf("active = %d, want 7200", r.Active)
		}
	})

	t.Run("SED-020/a própria sessão não conta como conflito", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
		sesEdit(t, s, id, store.SessionPatch{Start: tp(sesAt(-5*h - 30*m))})
	})

	t.Run("SED-020/cruzar rastreada, manual de outro jogo ou a aberta é SessionOverlapError; encostar é aceito", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1}, libGame{Title: "Celeste"})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})

		cases := []struct {
			name       string
			wantClosed bool
		}{
			{"rastreada", true},
			{"manual de outro jogo", true},
			{"aberta", false},
		}
		for _, c := range cases {
			// one fresh database per probe: the target's end crosses a different kind of session
			s2, ids2 := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1}, libGame{Title: "Celeste"})
			tid := sesInsert(t, s2, sesRow{GameID: ids2[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
			var other int64
			switch c.name {
			case "rastreada":
				other = sesInsert(t, s2, sesRow{GameID: ids2[0], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600})
			case "manual de outro jogo":
				other = sesInsert(t, s2, sesRow{GameID: ids2[1], Start: sesAt(-3 * h), End: sesAt(-2 * h), Active: 3600, Source: "manual"})
			default:
				other = sesInsert(t, s2, sesRow{GameID: ids2[1], Start: sesAt(-3 * h), Active: 60, State: "running"})
			}
			end := sesAt(-150 * m)
			_, err := s2.EditSession(t.Context(), tid, store.SessionPatch{End: &end}, sesW)
			var oe *store.SessionOverlapError
			if !errors.As(err, &oe) {
				t.Fatalf("%s: err = %v (%T), want *store.SessionOverlapError", c.name, err, err)
			}
			wantGame, wantTitle := ids2[0], "Hades"
			if c.name != "rastreada" {
				wantGame, wantTitle = ids2[1], "Celeste"
			}
			if oe.SessionID != other || oe.GameID != wantGame || oe.Title != wantTitle || (oe.EndedAt != nil) != c.wantClosed {
				t.Errorf("%s: overlap = %+v", c.name, oe)
			}
		}

		// Touching is accepted: end exactly at the start of the next session.
		sesEdit(t, s, id, store.SessionPatch{End: tp(sesAt(-3 * h)), Active: dp(50 * m)})
	})

	t.Run("SED-020/patch só do tempo jogado não faz a checagem de sobreposição", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000 + 3600, Plays: 2})
		id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-5 * h), End: sesAt(-4 * h), Active: 3000})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-4*h - 30*m), End: sesAt(-3 * h), Active: 3600})
		sesEdit(t, s, id, store.SessionPatch{Active: dp(3300 * time.Second)})
	})

	t.Run("SED-021/importada com intervalo de 300 h e deslocada sobre uma rastreada é aceita", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 100*3600 + 3600, Plays: 2})
		imp := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-400 * h), End: sesAt(-100 * h), Active: 100 * 3600, Source: "import"})
		sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-150 * h), End: sesAt(-149 * h), Active: 3600})
		sesEdit(t, s, imp, store.SessionPatch{Start: tp(sesAt(-420 * h)), End: tp(sesAt(-120 * h))})
		sesEdit(t, s, imp, store.SessionPatch{Start: tp(sesAt(-160 * h)), End: tp(sesAt(-120 * h)), Active: dp(10 * h)})
	})

	t.Run("SED-022/sessão running, paused e grace devolve *SessionOpenError (errors.Is ErrSessionOpen) com id, jogo e título, e nada muda", func(t *testing.T) {
		t.Parallel()
		for _, state := range []string{"running", "paused", "grace"} {
			t.Run(state, func(t *testing.T) {
				t.Parallel()
				s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3000, Plays: 1})
				id := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-30 * m), Active: 600, State: state})
				rowBefore, gameBefore := sesFull(t, s, id), sesGame(t, s, ids[0])
				_, err := s.EditSession(t.Context(), id, store.SessionPatch{Active: dp(time.Hour)}, sesW)
				var oe *store.SessionOpenError
				if !errors.As(err, &oe) || !errors.Is(err, store.ErrSessionOpen) {
					t.Fatalf("err = %v (%T), want *SessionOpenError satisfying ErrSessionOpen", err, err)
				}
				if oe.SessionID != id || oe.GameID != ids[0] || oe.Title != "Hades" {
					t.Errorf("open error = %+v", oe)
				}
				if sesFull(t, s, id) != rowBefore || sesGame(t, s, ids[0]) != gameBefore {
					t.Errorf("a refused edit changed the database")
				}
			})
		}
	})

	t.Run("SED-024/id inexistente devolve ErrSessionNotFound", func(t *testing.T) {
		t.Parallel()
		s, _ := newSesStore(t, libGame{Title: "Hades"})
		_, err := s.EditSession(t.Context(), 9999, store.SessionPatch{Active: dp(time.Minute)}, sesW)
		if !errors.Is(err, store.ErrSessionNotFound) {
			t.Errorf("err = %v, want ErrSessionNotFound", err)
		}
	})

	t.Run("SED-025/com o jogo rodando: editar a sessão anterior e depois EndSession da aberta soma ao total sem tocar a aberta", func(t *testing.T) {
		t.Parallel()
		s, ids := newSesStore(t, libGame{Title: "Hades", Seconds: 3600, Plays: 1, FirstPlayed: sesAt(-4 * h).UnixMilli(), LastPlayed: sesAt(-3 * h).UnixMilli()})
		prev := sesInsert(t, s, sesRow{GameID: ids[0], Start: sesAt(-4 * h), End: sesAt(-3 * h), Active: 3600})
		open, err := s.StartSession(t.Context(), ids[0], sesAt(-30*m))
		if err != nil {
			t.Fatalf("StartSession: %v", err)
		}
		if err := s.UpdateSession(t.Context(), open, domain.SessionRunning, sesAt(-m), 29*m); err != nil {
			t.Fatalf("UpdateSession: %v", err)
		}
		openBefore := sesFull(t, s, open)

		sesEdit(t, s, prev, store.SessionPatch{End: tp(sesAt(-2 * h)), Active: dp(2 * h)})
		if sesFull(t, s, open) != openBefore {
			t.Errorf("the open session changed: %+v, want %+v", sesFull(t, s, open), openBefore)
		}
		if _, err := s.EndSession(t.Context(), store.SessionEnd{ID: open, EndedAt: sesW, Active: 30 * m, Reason: domain.EndExited, WrittenAt: sesW}); err != nil {
			t.Fatalf("EndSession: %v", err)
		}
		g := sesGame(t, s, ids[0])
		if g.Total != 7200+1800 || g.Last != sesW.UnixMilli() || g.First != sesAt(-4*h).UnixMilli() {
			t.Errorf("game = %+v, want total 9000, first -4h, last W", g)
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
			sesEdit(t, s, id, store.SessionPatch{Active: dp(3300 * time.Second)})
		}
		if g := sesGame(t, s, ids[0]); g.Status != "not_played" || g.Completed != 0 {
			t.Errorf("Novo status/completed = %s/%d", g.Status, g.Completed)
		}
		if g := sesGame(t, s, ids[1]); g.Status != "completed" || g.Completed != completed {
			t.Errorf("Zerado status/completed = %s/%d", g.Status, g.Completed)
		}
	})
}
