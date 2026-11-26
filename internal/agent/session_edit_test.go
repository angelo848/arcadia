package agent

// Tests for T20 of docs/specs/phase-16-edicao-sessoes/tasks.md: the session.create, session.update and
// session.delete handlers over a real store in t.TempDir() and a domain.FakeClock [SED-005, SED-008,
// SED-013, SED-015..SED-020, SED-022..SED-026, SED-029, SED-031, SED-032]. Red until T21
// (sessionEditHandlers, sessionEdit, sessionEditStore) exists.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// sessT is "now" (T) of the session edit tests; the table of the contract §5 uses the same instant.
var sessT = time.Date(2026, 11, 25, 20, 0, 0, 0, time.UTC)

type sessFix struct {
	h     map[string]ipc.Handler
	st    *store.Store
	clock *domain.FakeClock
	logs  *recordHandler
	game  int64 // "Jogo"
	other int64 // "Outro"
}

func newSessFix(t *testing.T) sessFix {
	t.Helper()
	st := openImgStore(t)
	clock := domain.NewFakeClock(sessT)
	logs := newRecordHandler()
	f := sessFix{
		st: st, clock: clock, logs: logs,
		h:    sessionEditHandlers(sessionEdit{st: st, clock: clock, log: slog.New(logs)}),
		game: addGame(t, st, "Jogo"), other: addGame(t, st, "Outro"),
	}
	return f
}

func (f sessFix) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.st.DB().ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// insert writes a session by SQL. A zero end leaves ended_at NULL (an open session).
func (f sessFix) insert(t *testing.T, game int64, source, state string, start, end time.Time, active int64) int64 {
	t.Helper()
	var endedAt any
	lastSeen := start.UnixMilli()
	if !end.IsZero() {
		endedAt = end.UnixMilli()
		lastSeen = end.UnixMilli()
	}
	res, err := f.st.DB().ExecContext(t.Context(),
		`INSERT INTO play_session (game_id, started_at, ended_at, last_seen_at, active_seconds, state, source)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`, game, start.UnixMilli(), endedAt, lastSeen, active, state, source)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (f sessFix) setTotal(t *testing.T, game, total int64) {
	t.Helper()
	f.exec(t, `UPDATE game SET total_play_seconds = ? WHERE id = ?`, total, game)
}

// snapshot is every session row plus the counters of both games, as text, to prove "nothing changed".
func (f sessFix) snapshot(t *testing.T) string {
	t.Helper()
	var out string
	for _, q := range []string{
		`SELECT id, game_id, started_at, ended_at, last_seen_at, active_seconds, state, source, edited_at FROM play_session ORDER BY id`,
		`SELECT id, total_play_seconds, first_played_at, last_played_at, play_count, status, completed_at, updated_at FROM game ORDER BY id`,
		`SELECT id, session_id, paused_at, resumed_at FROM pause_interval ORDER BY id`,
	} {
		rows, err := f.st.DB().QueryContext(t.Context(), q)
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatalf("snapshot scan: %v", err)
			}
			out += fmt.Sprintln(vals...)
		}
		_ = rows.Close()
	}
	return out
}

func (f sessFix) callRaw(method, params string) (json.RawMessage, error) {
	res, err := f.h[method](context.Background(), json.RawMessage(params))
	if err != nil {
		return nil, err
	}
	out, mErr := json.Marshal(res)
	if mErr != nil {
		return nil, mErr
	}
	return out, nil
}

func (f sessFix) call(t *testing.T, method string, params any) (json.RawMessage, error) {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	return f.callRaw(method, string(raw))
}

func (f sessFix) write(t *testing.T, method string, params any) ipc.SessionWriteResult {
	t.Helper()
	raw, err := f.call(t, method, params)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	var res ipc.SessionWriteResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("%s result %s: %v", method, raw, err)
	}
	return res
}

// ts is T+d in RFC 3339 (nanosecond precision kept).
func ts(d time.Duration) string { return sessT.Add(d).Format(time.RFC3339Nano) }

func rpcErr(t *testing.T, err error, code int) *ipc.Error {
	t.Helper()
	var e *ipc.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error = %v, want code %d", err, code)
	}
	return e
}

func wantReason(t *testing.T, err error, reason domain.EditReason) {
	t.Helper()
	e := rpcErr(t, err, ipc.CodeInvalidParams)
	var d ipc.InvalidParamsReasonData
	if err := json.Unmarshal(e.Data, &d); err != nil || d.Reason != string(reason) {
		t.Fatalf("error data = %s (err %v), want reason %q", e.Data, err, reason)
	}
}

func createParams(game int64, start, end time.Duration, active int64) map[string]any {
	return map[string]any{"gameId": game, "startedAt": ts(start), "endedAt": ts(end), "activeSeconds": active}
}

func TestSessionCreate(t *testing.T) {
	t.Parallel()

	t.Run("SED-031/session.create com params por nome devolve {session, game} já com a sessão nova e os valores do jogo depois da escrita", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		raw, err := f.call(t, ipc.MethodSessionCreate, createParams(f.game, -3*time.Hour, -2*time.Hour, 3600))
		if err != nil {
			t.Fatalf("session.create: %v", err)
		}
		wantKeys(t, objectOf(t, raw), "session", "game")
		var res ipc.SessionWriteResult
		if err := json.Unmarshal(raw, &res); err != nil {
			t.Fatal(err)
		}
		s := res.Session
		if s.SessionID < 1 || s.GameID != f.game || s.State != "ended" || s.Source != "manual" || s.ActiveSeconds != 3600 ||
			s.EditedAt != nil || s.EndReason != nil || s.EndedAt == nil || !s.StartedAt.Equal(sessT.Add(-3*time.Hour)) || !s.EndedAt.Equal(sessT.Add(-2*time.Hour)) {
			t.Errorf("session = %+v, want the new manual ended session", s)
		}
		g := res.Game
		if g.ID != f.game || g.TotalPlaySeconds != 3600 || g.SessionPlaySeconds != 3600 || g.PlayCount != 1 ||
			g.FirstPlayedAt == nil || !g.FirstPlayedAt.Equal(sessT.Add(-3*time.Hour)) ||
			g.LastPlayedAt == nil || !g.LastPlayedAt.Equal(sessT.Add(-2*time.Hour)) {
			t.Errorf("game = %+v, want total 3600, 1 session, first T-3h and last T-2h", g.Game)
		}
	})

	t.Run("SED-031/includedInTotal ausente vale false", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		f.setTotal(t, f.game, 5000)
		params := fmt.Sprintf(`{"gameId":%d,"startedAt":%q,"endedAt":%q,"activeSeconds":1800}`, f.game, ts(-2*time.Hour), ts(-time.Hour))
		raw, err := f.callRaw(ipc.MethodSessionCreate, params)
		if err != nil {
			t.Fatalf("session.create: %v", err)
		}
		var res ipc.SessionWriteResult
		if err := json.Unmarshal(raw, &res); err != nil {
			t.Fatal(err)
		}
		if res.Game.TotalPlaySeconds != 6800 {
			t.Errorf("total = %d, want 6800 (5000 + 1800: includedInTotal defaults to false)", res.Game.TotalPlaySeconds)
		}
	})

	t.Run("SED-031/membro desconhecido, tipo errado, instante que não é RFC 3339, startedAt ou endedAt ausente, gameId < 1 e params posicionais são -32602 sem data.reason", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		before := f.snapshot(t)
		g := f.game
		s, e := ts(-2*time.Hour), ts(-time.Hour)
		for name, params := range map[string]string{
			"membro desconhecido": fmt.Sprintf(`{"gameId":%d,"startedAt":%q,"endedAt":%q,"activeSeconds":1800,"nota":"x"}`, g, s, e),
			"gameId texto":        fmt.Sprintf(`{"gameId":"%d","startedAt":%q,"endedAt":%q,"activeSeconds":1800}`, g, s, e),
			"activeSeconds texto": fmt.Sprintf(`{"gameId":%d,"startedAt":%q,"endedAt":%q,"activeSeconds":"1800"}`, g, s, e),
			"startedAt inválido":  fmt.Sprintf(`{"gameId":%d,"startedAt":"ontem às 20h","endedAt":%q,"activeSeconds":1800}`, g, e),
			"endedAt numérico":    fmt.Sprintf(`{"gameId":%d,"startedAt":%q,"endedAt":1764000000,"activeSeconds":1800}`, g, s),
			"startedAt ausente":   fmt.Sprintf(`{"gameId":%d,"endedAt":%q,"activeSeconds":1800}`, g, e),
			"endedAt ausente":     fmt.Sprintf(`{"gameId":%d,"startedAt":%q,"activeSeconds":1800}`, g, s),
			"gameId zero":         fmt.Sprintf(`{"gameId":0,"startedAt":%q,"endedAt":%q,"activeSeconds":1800}`, s, e),
			"gameId negativo":     fmt.Sprintf(`{"gameId":-3,"startedAt":%q,"endedAt":%q,"activeSeconds":1800}`, s, e),
			"posicionais":         fmt.Sprintf(`[%d,%q,%q,1800]`, g, s, e),
			"sem params":          ``,
		} {
			_, err := f.callRaw(ipc.MethodSessionCreate, params)
			if e := rpcErr(t, err, ipc.CodeInvalidParams); len(e.Data) != 0 {
				t.Errorf("%s: data = %s, want none (malformed request has no reason)", name, e.Data)
			}
		}
		if after := f.snapshot(t); after != before {
			t.Errorf("a malformed request changed the database:\n%s\n%s", before, after)
		}
	})

	t.Run("SED-031/gameId inexistente é -32001", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		_, err := f.call(t, ipc.MethodSessionCreate, createParams(9999, -2*time.Hour, -time.Hour, 1800))
		e := rpcErr(t, err, ipc.CodeGameNotFound)
		var d ipc.GameNotFoundData
		if err := json.Unmarshal(e.Data, &d); err != nil || d.GameID != 9999 {
			t.Errorf("data = %s, want {gameId:9999}", e.Data)
		}
	})

	t.Run("SED-031/instantes com mais precisão que ms são truncados a ms (ADR 0009)", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		params := map[string]any{
			"gameId": f.game, "activeSeconds": 1800,
			"startedAt": "2026-11-25T17:00:00.123456789Z", "endedAt": "2026-11-25T17:30:00.987654321Z",
		}
		res := f.write(t, ipc.MethodSessionCreate, params)
		if got := res.Session.StartedAt.UnixMilli(); got != time.Date(2026, 11, 25, 17, 0, 0, 123_000_000, time.UTC).UnixMilli() {
			t.Errorf("startedAt ms = %d, want truncated to .123", got)
		}
		if res.Session.EndedAt == nil || res.Session.EndedAt.UnixMilli() != time.Date(2026, 11, 25, 17, 30, 0, 987_000_000, time.UTC).UnixMilli() {
			t.Errorf("endedAt = %v, want truncated to .987", res.Session.EndedAt)
		}
	})

	t.Run("SED-019/activeSeconds 29 em session.create é -32602 com data.reason active-too-short (não rejeitado por faixa antes da regra)", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		_, err := f.call(t, ipc.MethodSessionCreate, createParams(f.game, -time.Hour, 0, 29))
		wantReason(t, err, domain.ReasonActiveTooShort)
	})

	t.Run("SED-015/cada reason de domínio chega em error.data.reason e nada muda", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		f.setTotal(t, f.game, 777)
		before := f.snapshot(t)
		early := func(s string) time.Duration { return mustTime(t, s).Sub(sessT) }
		for _, tc := range []struct {
			name       string
			start, end time.Duration
			active     int64
			want       domain.EditReason
		}{
			{"SED-015/início = fim", -time.Hour, -time.Hour, 30, domain.ReasonEndNotAfterStart},
			{"SED-015/fim antes do início", -time.Hour, -2 * time.Hour, 30, domain.ReasonEndNotAfterStart},
			{"SED-016/fim depois de now", -time.Hour, time.Minute, 1800, domain.ReasonInFuture},
			{"SED-017/início antes de 1980", early("1979-12-31T23:59:00Z"), early("1980-01-01T01:00:00Z"), 1800, domain.ReasonTooEarly},
			{"SED-018/intervalo de 25 h", -25 * time.Hour, 0, 7200, domain.ReasonSpanTooLong},
			{"SED-019/29 s", -time.Hour, 0, 29, domain.ReasonActiveTooShort},
			{"SED-019/tempo maior que o intervalo", -time.Hour, 0, 3601, domain.ReasonActiveExceedsSpan},
		} {
			_, err := f.call(t, ipc.MethodSessionCreate, createParams(f.game, tc.start, tc.end, tc.active))
			t.Run(tc.name, func(t *testing.T) { wantReason(t, err, tc.want) })
		}
		if after := f.snapshot(t); after != before {
			t.Errorf("refused requests changed the database:\n%s\n%s", before, after)
		}
	})

	t.Run("SED-016/in-future usa o relógio do agente (FakeClock), e fim igual a now é aceito", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		if _, err := f.call(t, ipc.MethodSessionCreate, createParams(f.game, -2*time.Hour, 0, 7200)); err != nil {
			t.Fatalf("end == now must be accepted: %v", err)
		}
		// An end 30 min after T is in the future now and valid once the agent clock passes it.
		_, err := f.call(t, ipc.MethodSessionCreate, createParams(f.game, 10*time.Minute, 30*time.Minute, 1200))
		wantReason(t, err, domain.ReasonInFuture)
		f.clock.Advance(time.Hour)
		if _, err := f.call(t, ipc.MethodSessionCreate, createParams(f.game, 10*time.Minute, 30*time.Minute, 1200)); err != nil {
			t.Fatalf("after the agent clock moved past the end: %v", err)
		}
	})

	t.Run("SED-005/includedInTotal acima do ajuste é -32602 com reason exceeds-adjustment e nada muda", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		f.setTotal(t, f.game, 1800) // adjustment of 1800 s, no sessions
		before := f.snapshot(t)
		p := createParams(f.game, -2*time.Hour, -time.Hour, 3600)
		p["includedInTotal"] = true
		_, err := f.call(t, ipc.MethodSessionCreate, p)
		wantReason(t, err, domain.ReasonExceedsAdjustment)
		if after := f.snapshot(t); after != before {
			t.Errorf("a refused request changed the database:\n%s\n%s", before, after)
		}
	})

	t.Run("SED-005/includedInTotal dentro do ajuste mantém o total", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		f.setTotal(t, f.game, 7200)
		p := createParams(f.game, -2*time.Hour, -time.Hour, 3600)
		p["includedInTotal"] = true
		res := f.write(t, ipc.MethodSessionCreate, p)
		if res.Game.TotalPlaySeconds != 7200 || res.Game.SessionPlaySeconds != 3600 || res.Game.PlayCount != 1 {
			t.Errorf("game = %+v, want total 7200 (kept), sessions 3600, 1 session", res.Game)
		}
	})

	t.Run("SED-020/-32018 com data = SessionOverlapData (sessionId, gameId, title, startedAt, endedAt ou null para a aberta) e nada muda", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		other := f.insert(t, f.other, "tracked", "ended", sessT.Add(-3*time.Hour), sessT.Add(-2*time.Hour), 3600)
		before := f.snapshot(t)

		_, err := f.call(t, ipc.MethodSessionCreate, createParams(f.game, -150*time.Minute, -time.Hour, 3600))
		e := rpcErr(t, err, ipc.CodeSessionOverlap)
		if e.Message != ipc.MsgSessionOverlap {
			t.Errorf("message = %q, want %q", e.Message, ipc.MsgSessionOverlap)
		}
		var d ipc.SessionOverlapData
		if err := json.Unmarshal(e.Data, &d); err != nil {
			t.Fatalf("data %s: %v", e.Data, err)
		}
		if d.SessionID != other || d.GameID != f.other || d.Title != "Outro" || !d.StartedAt.Equal(sessT.Add(-3*time.Hour)) ||
			d.EndedAt == nil || !d.EndedAt.Equal(sessT.Add(-2*time.Hour)) {
			t.Errorf("data = %+v, want the other session of Outro [T-3h, T-2h]", d)
		}
		wantKeys(t, objectOf(t, e.Data), "sessionId", "gameId", "title", "startedAt", "endedAt")
		if after := f.snapshot(t); after != before {
			t.Errorf("a refused request changed the database:\n%s\n%s", before, after)
		}

		// Encostar (fim da outra = início desta) é aceito.
		if _, err := f.call(t, ipc.MethodSessionCreate, createParams(f.game, -2*time.Hour, -time.Hour, 3600)); err != nil {
			t.Errorf("touching interval refused: %v", err)
		}

		// The open session of another game: endedAt is null.
		open := f.insert(t, f.other, "tracked", "running", sessT.Add(-10*time.Minute), time.Time{}, 0)
		_, err = f.call(t, ipc.MethodSessionCreate, createParams(f.game, -30*time.Minute, -5*time.Minute, 600))
		e = rpcErr(t, err, ipc.CodeSessionOverlap)
		if err := json.Unmarshal(e.Data, &d); err != nil || d.SessionID != open {
			t.Fatalf("data = %s (err %v), want the open session %d", e.Data, err, open)
		}
		wantRaw(t, objectOf(t, e.Data), "endedAt", "null")
		if _, err := f.call(t, ipc.MethodSessionCreate, createParams(f.game, -30*time.Minute, -10*time.Minute, 600)); err != nil {
			t.Errorf("ending exactly at the open session's start must be accepted: %v", err)
		}
	})

	t.Run("SED-020/pedido recusado por sobreposição não muda nada", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		f.insert(t, f.game, "manual", "ended", sessT.Add(-3*time.Hour), sessT.Add(-2*time.Hour), 3600)
		before := f.snapshot(t)
		_, err := f.call(t, ipc.MethodSessionCreate, createParams(f.game, -150*time.Minute, -time.Hour, 3600))
		rpcErr(t, err, ipc.CodeSessionOverlap)
		if after := f.snapshot(t); after != before {
			t.Errorf("a refused request changed the database:\n%s\n%s", before, after)
		}
	})
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSessionUpdate(t *testing.T) {
	t.Parallel()

	// A tracked ended session [T-3h, T-2h] with 3600 s, in a game whose total is 10000.
	tracked := func(t *testing.T) (sessFix, int64) {
		t.Helper()
		f := newSessFix(t)
		id := f.insert(t, f.game, "tracked", "ended", sessT.Add(-3*time.Hour), sessT.Add(-2*time.Hour), 3600)
		f.setTotal(t, f.game, 10000)
		return f, id
	}

	t.Run("SED-032/session.update devolve {session, game} depois da escrita", func(t *testing.T) {
		t.Parallel()
		// Span widened to 2 h so 5400 s of active time fits (SED-019).
		f := newSessFix(t)
		id := f.insert(t, f.game, "tracked", "ended", sessT.Add(-4*time.Hour), sessT.Add(-2*time.Hour), 3600)
		f.setTotal(t, f.game, 10000)
		res := f.write(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "activeSeconds": 5400})
		if res.Session.SessionID != id || res.Session.ActiveSeconds != 5400 || res.Session.Source != "tracked" ||
			res.Session.EditedAt == nil || !res.Session.EditedAt.Equal(sessT) {
			t.Errorf("session = %+v, want active 5400, still tracked, editedAt = T", res.Session)
		}
		if res.Game.TotalPlaySeconds != 11800 || res.Game.SessionPlaySeconds != 5400 {
			t.Errorf("game total %d sessions %d, want 11800 and 5400", res.Game.TotalPlaySeconds, res.Game.SessionPlaySeconds)
		}
	})

	t.Run("SED-032/ausente não mexe, null é -32602, patch sem nenhum dos três é -32602 com reason empty-patch", func(t *testing.T) {
		t.Parallel()
		f, id := tracked(t)
		// Only the end present: start and activeSeconds keep their stored values.
		res := f.write(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "endedAt": ts(-90 * time.Minute)})
		if !res.Session.StartedAt.Equal(sessT.Add(-3*time.Hour)) || res.Session.EndedAt == nil || !res.Session.EndedAt.Equal(sessT.Add(-90*time.Minute)) || res.Session.ActiveSeconds != 3600 {
			t.Errorf("session = %+v, want only the end changed", res.Session)
		}
		before := f.snapshot(t)
		for name, params := range map[string]string{
			"startedAt null":      fmt.Sprintf(`{"sessionId":%d,"startedAt":null}`, id),
			"endedAt null":        fmt.Sprintf(`{"sessionId":%d,"endedAt":null}`, id),
			"activeSeconds null":  fmt.Sprintf(`{"sessionId":%d,"activeSeconds":null}`, id),
			"sessionId zero":      `{"sessionId":0,"activeSeconds":60}`,
			"membro desconhecido": fmt.Sprintf(`{"sessionId":%d,"activeSeconds":60,"nota":"x"}`, id),
			"posicionais":         fmt.Sprintf(`[%d,60]`, id),
		} {
			_, err := f.callRaw(ipc.MethodSessionUpdate, params)
			if e := rpcErr(t, err, ipc.CodeInvalidParams); len(e.Data) != 0 {
				t.Errorf("%s: data = %s, want none", name, e.Data)
			}
		}
		_, err := f.callRaw(ipc.MethodSessionUpdate, fmt.Sprintf(`{"sessionId":%d}`, id))
		wantReason(t, err, domain.ReasonEmptyPatch)
		if after := f.snapshot(t); after != before {
			t.Errorf("refused requests changed the database:\n%s\n%s", before, after)
		}
	})

	t.Run("SED-017/too-early em session.update", func(t *testing.T) {
		t.Parallel()
		f, id := tracked(t)
		_, err := f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "startedAt": "1979-12-31T23:00:00Z"})
		wantReason(t, err, domain.ReasonTooEarly)
	})

	t.Run("SED-015/session.update: fim resultante não posterior ao início é end-not-after-start; fim no futuro é in-future", func(t *testing.T) {
		t.Parallel()
		f, id := tracked(t)
		_, err := f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "endedAt": ts(-3 * time.Hour)})
		wantReason(t, err, domain.ReasonEndNotAfterStart)
		_, err = f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "endedAt": ts(time.Minute)})
		wantReason(t, err, domain.ReasonInFuture)
	})

	t.Run("SED-019/session.update: 2 h jogadas e fim 1 h depois do início é active-exceeds-span; 29 s é active-too-short", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		id := f.insert(t, f.game, "tracked", "ended", sessT.Add(-4*time.Hour), sessT.Add(-2*time.Hour), 7200)
		_, err := f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "endedAt": ts(-3 * time.Hour)})
		wantReason(t, err, domain.ReasonActiveExceedsSpan)
		_, err = f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "activeSeconds": 29})
		wantReason(t, err, domain.ReasonActiveTooShort)
	})

	t.Run("SED-018/span-too-long em session.update com início ou fim; e só tempo jogado numa rastreada de 30 h é aceito", func(t *testing.T) {
		t.Parallel()
		f, id := tracked(t)
		_, err := f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "startedAt": ts(-30 * time.Hour)})
		wantReason(t, err, domain.ReasonSpanTooLong)

		old := f.insert(t, f.other, "tracked", "ended", sessT.Add(-60*time.Hour), sessT.Add(-30*time.Hour), 3600)
		res := f.write(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": old, "activeSeconds": 7200})
		if res.Session.ActiveSeconds != 7200 {
			t.Errorf("session = %+v, want activeSeconds 7200 accepted on a 30 h interval", res.Session)
		}
	})

	t.Run("SED-020/session.update cruzando outra sessão é -32018 e nada muda; só o tempo jogado não checa sobreposição", func(t *testing.T) {
		t.Parallel()
		f, id := tracked(t)
		f.insert(t, f.other, "manual", "ended", sessT.Add(-100*time.Minute), sessT.Add(-60*time.Minute), 1800)
		before := f.snapshot(t)
		// The extended end crosses the other session: [T-3h, T-80min) overlaps [T-100min, T-60min).
		_, err := f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "endedAt": ts(-80 * time.Minute)})
		rpcErr(t, err, ipc.CodeSessionOverlap)
		if after := f.snapshot(t); after != before {
			t.Errorf("a refused request changed the database:\n%s\n%s", before, after)
		}
		// Active time only: no overlap check, even though the stored interval is untouched and valid.
		if _, err := f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "activeSeconds": 3000}); err != nil {
			t.Errorf("activeSeconds-only patch refused: %v", err)
		}
	})

	t.Run("SED-022/sessão running, paused e grace em update e delete é -32002 com data SessionRef (título do jogo); nada muda", func(t *testing.T) {
		t.Parallel()
		for _, state := range []string{"running", "paused", "grace"} {
			f := newSessFix(t)
			id := f.insert(t, f.game, "tracked", state, sessT.Add(-10*time.Minute), time.Time{}, 120)
			before := f.snapshot(t)
			for _, m := range []struct {
				method string
				params map[string]any
			}{
				{ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "activeSeconds": 600}},
				{ipc.MethodSessionDelete, map[string]any{"sessionId": id}},
			} {
				_, err := f.call(t, m.method, m.params)
				e := rpcErr(t, err, ipc.CodeSessionActive)
				var ref ipc.SessionRef
				if err := json.Unmarshal(e.Data, &ref); err != nil || ref.SessionID != id || ref.GameID != f.game || ref.Title != "Jogo" {
					t.Errorf("%s %s: data = %s, want SessionRef {%d, %d, Jogo}", state, m.method, e.Data, id, f.game)
				}
			}
			if after := f.snapshot(t); after != before {
				t.Errorf("%s: open session edit changed the database:\n%s\n%s", state, before, after)
			}
		}
	})

	t.Run("SED-023/descartada em update é -32602 com reason discarded", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		id := f.insert(t, f.game, "tracked", "discarded", sessT.Add(-2*time.Hour), sessT.Add(-2*time.Hour+20*time.Second), 20)
		before := f.snapshot(t)
		_, err := f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id, "activeSeconds": 60})
		wantReason(t, err, domain.ReasonDiscarded)
		if after := f.snapshot(t); after != before {
			t.Errorf("a refused request changed the database:\n%s\n%s", before, after)
		}
	})

	t.Run("SED-024/sessionId inexistente em update é -32017 com data {sessionId}", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		_, err := f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": 9999, "activeSeconds": 60})
		e := rpcErr(t, err, ipc.CodeSessionNotFound)
		if e.Message != ipc.MsgSessionNotFound {
			t.Errorf("message = %q, want %q", e.Message, ipc.MsgSessionNotFound)
		}
		wantKeys(t, objectOf(t, e.Data), "sessionId")
		wantRaw(t, objectOf(t, e.Data), "sessionId", "9999")
	})

	t.Run("SED-008/pedido sem mudança responde sucesso com a sessão e o jogo como estão e não registra Info", func(t *testing.T) {
		t.Parallel()
		f, id := tracked(t)
		before := f.snapshot(t)
		res := f.write(t, ipc.MethodSessionUpdate, map[string]any{
			"sessionId": id, "startedAt": ts(-3 * time.Hour), "endedAt": ts(-2 * time.Hour), "activeSeconds": 3600,
		})
		if res.Session.SessionID != id || res.Session.EditedAt != nil || res.Session.ActiveSeconds != 3600 || res.Game.TotalPlaySeconds != 10000 {
			t.Errorf("result = %+v / %+v, want the session and game as they were", res.Session, res.Game.Game)
		}
		if after := f.snapshot(t); after != before {
			t.Errorf("an unchanged request wrote to the database:\n%s\n%s", before, after)
		}
		if n := f.logs.count(slog.LevelInfo); n != 0 {
			t.Errorf("Info records = %d, want 0 for an unchanged request", n)
		}
	})

	t.Run("SED-025/com o jogo rodando, editar outra sessão do mesmo jogo funciona e a sessão aberta segue igual (estado, tempo ativo e pausas)", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		prev := f.insert(t, f.game, "tracked", "ended", sessT.Add(-4*time.Hour), sessT.Add(-3*time.Hour), 3600)
		f.setTotal(t, f.game, 3600)
		open := f.insert(t, f.game, "tracked", "running", sessT.Add(-10*time.Minute), time.Time{}, 0)
		f.exec(t, `INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason) VALUES (?, ?, NULL, 'ui')`, open, sessT.Add(-5*time.Minute).UnixMilli())
		openBefore := func() string {
			var s string
			row := f.st.DB().QueryRowContext(t.Context(),
				`SELECT state || '|' || active_seconds || '|' || ifnull(ended_at, 'null') || '|' || (SELECT count(*) FROM pause_interval WHERE session_id = play_session.id) FROM play_session WHERE id = ?`, open)
			if err := row.Scan(&s); err != nil {
				t.Fatal(err)
			}
			return s
		}
		want := openBefore()

		res := f.write(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": prev, "endedAt": ts(-2 * time.Hour), "activeSeconds": 7200})
		if res.Game.TotalPlaySeconds != 7200 {
			t.Errorf("total after edit = %d, want 7200", res.Game.TotalPlaySeconds)
		}
		if got := openBefore(); got != want {
			t.Errorf("open session = %q, want unchanged %q", got, want)
		}

		// The open session ends with 30 min of active time: the total grows 1 h 30 overall.
		if _, err := f.st.EndSession(t.Context(), store.SessionEnd{
			ID: open, EndedAt: sessT, Active: 30 * time.Minute, Reason: domain.EndExited, WrittenAt: sessT,
		}); err != nil {
			t.Fatalf("EndSession: %v", err)
		}
		var total, last int64
		if err := f.st.DB().QueryRowContext(t.Context(), `SELECT total_play_seconds, last_played_at FROM game WHERE id = ?`, f.game).Scan(&total, &last); err != nil {
			t.Fatal(err)
		}
		if total != 3600+5400 || last != sessT.UnixMilli() {
			t.Errorf("total = %d last = %d, want %d and %d (T)", total, last, 3600+5400, sessT.UnixMilli())
		}
	})
}

func TestSessionDelete(t *testing.T) {
	t.Parallel()

	t.Run("SED-032/session.delete devolve {sessionId, game} depois da escrita", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		id := f.insert(t, f.game, "tracked", "ended", sessT.Add(-3*time.Hour), sessT.Add(-2*time.Hour), 3600)
		f.setTotal(t, f.game, 10000)
		raw, err := f.call(t, ipc.MethodSessionDelete, map[string]any{"sessionId": id})
		if err != nil {
			t.Fatalf("session.delete: %v", err)
		}
		wantKeys(t, objectOf(t, raw), "sessionId", "game")
		var res ipc.SessionDeleteResult
		if err := json.Unmarshal(raw, &res); err != nil {
			t.Fatal(err)
		}
		if res.SessionID != id || res.Game.TotalPlaySeconds != 6400 || res.Game.SessionPlaySeconds != 0 || res.Game.PlayCount != 0 ||
			res.Game.FirstPlayedAt != nil || res.Game.LastPlayedAt != nil {
			t.Errorf("result = %d / %+v, want total 6400, no sessions left", res.SessionID, res.Game.Game)
		}
	})

	t.Run("SED-013/exclusão que levaria o total abaixo de 0 devolve total 0", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		id := f.insert(t, f.game, "tracked", "ended", sessT.Add(-5*time.Hour), sessT.Add(-2*time.Hour), 3*3600)
		f.setTotal(t, f.game, 3600) // adjusted by hand to 1 h
		raw, err := f.call(t, ipc.MethodSessionDelete, map[string]any{"sessionId": id})
		if err != nil {
			t.Fatalf("session.delete: %v", err)
		}
		var res ipc.SessionDeleteResult
		if err := json.Unmarshal(raw, &res); err != nil {
			t.Fatal(err)
		}
		if res.Game.TotalPlaySeconds != 0 {
			t.Errorf("total = %d, want 0", res.Game.TotalPlaySeconds)
		}
	})

	t.Run("SED-023/delete da descartada funciona e não muda o jogo", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		id := f.insert(t, f.game, "tracked", "discarded", sessT.Add(-2*time.Hour), sessT.Add(-2*time.Hour+20*time.Second), 20)
		f.setTotal(t, f.game, 500)
		if _, err := f.call(t, ipc.MethodSessionDelete, map[string]any{"sessionId": id}); err != nil {
			t.Fatalf("session.delete of a discarded session: %v", err)
		}
		var n, total int64
		if err := f.st.DB().QueryRowContext(t.Context(), `SELECT (SELECT count(*) FROM play_session), total_play_seconds FROM game WHERE id = ?`, f.game).Scan(&n, &total); err != nil {
			t.Fatal(err)
		}
		if n != 0 || total != 500 {
			t.Errorf("sessions = %d total = %d, want 0 and 500", n, total)
		}
	})

	t.Run("SED-024/sessionId inexistente em delete é -32017 com data {sessionId}", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		_, err := f.call(t, ipc.MethodSessionDelete, map[string]any{"sessionId": 4242})
		e := rpcErr(t, err, ipc.CodeSessionNotFound)
		wantRaw(t, objectOf(t, e.Data), "sessionId", "4242")
	})

	t.Run("SED-031/session.delete com params inválidos é -32602 sem data", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		for _, params := range []string{`{"sessionId":0}`, `{"sessionId":"1"}`, `{}`, `[1]`, `{"sessionId":1,"x":1}`} {
			_, err := f.callRaw(ipc.MethodSessionDelete, params)
			if e := rpcErr(t, err, ipc.CodeInvalidParams); len(e.Data) != 0 {
				t.Errorf("params %s: data = %s, want none", params, e.Data)
			}
		}
	})
}

// attrs flattens the attributes of a record.
func attrs(r slog.Record) map[string]any {
	m := map[string]any{}
	r.Attrs(func(a slog.Attr) bool { m[a.Key] = a.Value.Any(); return true })
	return m
}

func (h *recordHandler) find(msg string) (slog.Record, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Message == msg && r.Level == slog.LevelInfo {
			return r, true
		}
	}
	return slog.Record{}, false
}

func TestSessionEditLog(t *testing.T) {
	t.Parallel()

	// anyEquals reports whether some attribute value prints as want.
	anyEquals := func(m map[string]any, want int64) bool {
		for _, v := range m {
			if fmt.Sprint(v) == fmt.Sprint(want) {
				return true
			}
		}
		return false
	}

	t.Run("SED-029/criação, edição e exclusão gravam Info \"sessão criada\", \"sessão editada\" e \"sessão excluída\" com session_id, game_id, source, valores antigos e novos e o total do jogo antes e depois", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		f.setTotal(t, f.game, 10000)

		created := f.write(t, ipc.MethodSessionCreate, createParams(f.game, -4*time.Hour, -2*time.Hour, 3600)) // total 10000 -> 13600; 2 h span so 4000 s fits
		rec, ok := f.logs.find("sessão criada")
		if !ok {
			t.Fatal("no Info \"sessão criada\"")
		}
		a := attrs(rec)
		if fmt.Sprint(a["session_id"]) != fmt.Sprint(created.Session.SessionID) || fmt.Sprint(a["game_id"]) != fmt.Sprint(f.game) || fmt.Sprint(a["source"]) != "manual" {
			t.Errorf("created attrs = %v, want session_id, game_id and source=manual", a)
		}
		if !anyEquals(a, 10000) || !anyEquals(a, 13600) {
			t.Errorf("created attrs = %v, want the game total before (10000) and after (13600)", a)
		}

		f.write(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": created.Session.SessionID, "activeSeconds": 4000}) // 13600 -> 14000
		rec, ok = f.logs.find("sessão editada")
		if !ok {
			t.Fatal("no Info \"sessão editada\"")
		}
		a = attrs(rec)
		if fmt.Sprint(a["session_id"]) != fmt.Sprint(created.Session.SessionID) || fmt.Sprint(a["source"]) != "manual" {
			t.Errorf("edited attrs = %v", a)
		}
		if !anyEquals(a, 3600) || !anyEquals(a, 4000) || !anyEquals(a, 13600) || !anyEquals(a, 14000) {
			t.Errorf("edited attrs = %v, want old/new active (3600/4000) and the game total before/after (13600/14000)", a)
		}

		if _, err := f.call(t, ipc.MethodSessionDelete, map[string]any{"sessionId": created.Session.SessionID}); err != nil { // 14000 -> 10000
			t.Fatalf("session.delete: %v", err)
		}
		rec, ok = f.logs.find("sessão excluída")
		if !ok {
			t.Fatal("no Info \"sessão excluída\"")
		}
		a = attrs(rec)
		if fmt.Sprint(a["session_id"]) != fmt.Sprint(created.Session.SessionID) || !anyEquals(a, 14000) || !anyEquals(a, 10000) {
			t.Errorf("deleted attrs = %v, want session_id and the game total before/after (14000/10000)", a)
		}
	})

	t.Run("SED-029/pedido recusado não registra nada acima de Debug", func(t *testing.T) {
		t.Parallel()
		f := newSessFix(t)
		id := f.insert(t, f.game, "tracked", "ended", sessT.Add(-3*time.Hour), sessT.Add(-2*time.Hour), 3600)
		open := f.insert(t, f.other, "tracked", "running", sessT.Add(-5*time.Minute), time.Time{}, 0)
		_, _ = f.call(t, ipc.MethodSessionCreate, createParams(f.game, -time.Hour, -time.Hour, 30))         // end-not-after-start
		_, _ = f.call(t, ipc.MethodSessionCreate, createParams(f.game, -150*time.Minute, -time.Hour, 3600)) // overlap with id
		_, _ = f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": open, "activeSeconds": 60})   // open
		_, _ = f.call(t, ipc.MethodSessionDelete, map[string]any{"sessionId": 9999})                        // not found
		_, _ = f.call(t, ipc.MethodSessionUpdate, map[string]any{"sessionId": id})                          // empty patch
		for _, lv := range []slog.Level{slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
			if n := f.logs.count(lv); n != 0 {
				t.Errorf("%s records = %d, want 0 for refused requests", lv, n)
			}
		}
	})

	t.Run("SED-029/falha interna do store é -32603 com Error no log e a mensagem genérica", func(t *testing.T) {
		t.Parallel()
		logs := newRecordHandler()
		boom := errors.New("disco cheio: C:\\segredo\\library.db")
		h := sessionEditHandlers(sessionEdit{st: failingSessionStore{err: boom}, clock: domain.NewFakeClock(sessT), log: slog.New(logs)})
		for method, params := range map[string]string{
			ipc.MethodSessionCreate: fmt.Sprintf(`{"gameId":1,"startedAt":%q,"endedAt":%q,"activeSeconds":1800}`, ts(-2*time.Hour), ts(-time.Hour)),
			ipc.MethodSessionUpdate: `{"sessionId":1,"activeSeconds":600}`,
			ipc.MethodSessionDelete: `{"sessionId":1}`,
		} {
			before := logs.count(slog.LevelError)
			_, err := h[method](context.Background(), json.RawMessage(params))
			e := rpcErr(t, err, ipc.CodeInternalError)
			if e.Message != ipc.MsgInternalError || len(e.Data) != 0 {
				t.Errorf("%s: message %q data %s, want the generic message and no data", method, e.Message, e.Data)
			}
			if logs.count(slog.LevelError) != before+1 {
				t.Errorf("%s: Error records = %d, want one more than %d", method, logs.count(slog.LevelError), before)
			}
		}
	})
}

// failingSessionStore makes every write fail with err.
type failingSessionStore struct{ err error }

func (s failingSessionStore) CreateManualSession(context.Context, store.ManualSession, time.Time) (store.SessionWrite, error) {
	return store.SessionWrite{}, s.err
}

func (s failingSessionStore) EditSession(context.Context, int64, store.SessionPatch, time.Time) (store.SessionWrite, error) {
	return store.SessionWrite{}, s.err
}

func (s failingSessionStore) RemoveSession(context.Context, int64, time.Time) (store.SessionWrite, error) {
	return store.SessionWrite{}, s.err
}

func TestSessionEditNoNotification(t *testing.T) {
	t.Run("SED-026/com dois clientes conectados, session.create, session.update e session.delete não enviam session.changed nem outra notificação a nenhum deles", func(t *testing.T) {
		st := openImgStore(t)
		gameID := addGame(t, st, "Jogo")
		clock := domain.NewFakeClock(sessT)
		log := quietLog()
		server := ipc.NewServer(clock, sessionEditHandlers(sessionEdit{st: st, clock: clock, log: log}), log)
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); _ = server.Serve(ctx, ln) }()
		c1, c2 := newTCPClient(ln.Addr().String()), newTCPClient(ln.Addr().String())
		t.Cleanup(func() {
			_ = c1.Close()
			_ = c2.Close()
			cancel()
			_ = server.Close()
			wg.Wait()
		})

		call := func(c *ipc.Client, method string, params, result any) {
			t.Helper()
			cctx, ccancel := context.WithTimeout(context.Background(), testTimeout)
			defer ccancel()
			if err := c.Call(cctx, method, params, result); err != nil {
				t.Fatalf("%s: %v", method, err)
			}
		}
		var created ipc.SessionWriteResult
		call(c1, ipc.MethodSessionCreate, ipc.SessionCreateParams{
			GameID: gameID, StartedAt: sessT.Add(-2 * time.Hour), EndedAt: sessT.Add(-time.Hour), ActiveSeconds: 3600,
		}, &created)
		var updated ipc.SessionWriteResult
		call(c2, ipc.MethodSessionUpdate, map[string]any{"sessionId": created.Session.SessionID, "activeSeconds": 1800}, &updated)
		var deleted ipc.SessionDeleteResult
		call(c1, ipc.MethodSessionDelete, ipc.SessionDeleteParams{SessionID: created.Session.SessionID}, &deleted)

		for i, c := range []*ipc.Client{c1, c2} {
			deadline := time.After(quietWindow)
		drain:
			for {
				select {
				case n := <-c.Notifications():
					t.Fatalf("client %d got notification %s %s after the session writes", i+1, n.Method, n.Params)
				case <-deadline:
					break drain
				}
			}
		}
	})
}
