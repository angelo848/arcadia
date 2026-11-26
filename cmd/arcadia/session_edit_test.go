package main

// Tests for T22 (docs/specs/phase-16-edicao-sessoes/tasks.md): the CreateSession,
// UpdateSession and DeleteSession bindings (SED-035), the manualSeconds of the
// statistics (SED-027) and the Portuguese texts of the new errors (SED-036).
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - sessionEditError(err, log, loc) formats the instants of -32018 in loc as
//     DD/MM/AAAA HH:MM; the bindings use the package variable displayLocation.
//   - Tests that swap displayLocation are not parallel.
//   - A malformed RFC 3339 string in SessionCreateInput gives "Dados inválidos."
//     and no agent call.
//   - -32002 of game.launch keeps the DSK-011 text (userError is untouched).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

var (
	brt = time.FixedZone("BRT", -3*3600)

	sedStart = time.Date(2026, 11, 25, 13, 0, 0, 0, time.UTC)
	sedEnd   = time.Date(2026, 11, 25, 14, 30, 0, 0, time.UTC)
)

const (
	txtSessionGone   = "Esta sessão não existe mais."
	txtSessionOpen   = "Não é possível alterar uma sessão em andamento."
	txtSessionNoSave = "Nada para salvar."
)

func sedWriteResult() ipc.SessionWriteResult {
	edited := sedEnd.Add(time.Minute)
	return ipc.SessionWriteResult{
		Session: ipc.SessionItem{SessionID: 21, GameID: 3, State: "ended", StartedAt: sedStart, EndedAt: &sedEnd,
			ActiveSeconds: 5400, Source: "manual", EditedAt: &edited},
		Game: ipc.GameDetail{Game: ipc.Game{ID: 3, Title: "Celeste", TotalPlaySeconds: 9000}, SessionPlaySeconds: 5400},
	}
}

func reasonErr(reason string) error {
	return ipc.NewErrorData(ipc.CodeInvalidParams, ipc.MsgInvalidParams, ipc.InvalidParamsReasonData{Reason: reason})
}

func overlapErr(title string, started time.Time, ended *time.Time) error {
	return ipc.NewErrorData(ipc.CodeSessionOverlap, ipc.MsgSessionOverlap,
		ipc.SessionOverlapData{SessionID: 8, GameID: 3, Title: title, StartedAt: started, EndedAt: ended})
}

func notFoundSessionErr() error {
	return ipc.NewErrorData(ipc.CodeSessionNotFound, ipc.MsgSessionNotFound, ipc.SessionNotFoundData{SessionID: 21})
}

func TestSessionEditBindings(t *testing.T) {
	t.Parallel()

	t.Run("SED-035/CreateSession chama session.create com os cinco campos e prazo de 5 s, e devolve sessão e detalhe convertidos", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodSessionCreate: sedWriteResult()}}
		a := &App{client: p, log: discardLog()}

		got, err := a.CreateSession(3, SessionCreateInput{
			StartedAt: "2026-11-25T13:00:00Z", EndedAt: "2026-11-25T14:30:00Z", ActiveSeconds: 5400, IncludedInTotal: true,
		})

		if err != nil {
			t.Fatalf("CreateSession err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodSessionCreate, 5*time.Second)
		lp := rawObject(t, c)
		if !sameJSON(t, lp["gameId"], `3`) || !sameJSON(t, lp["startedAt"], `"2026-11-25T13:00:00Z"`) ||
			!sameJSON(t, lp["endedAt"], `"2026-11-25T14:30:00Z"`) || !sameJSON(t, lp["activeSeconds"], `5400`) ||
			!sameJSON(t, lp["includedInTotal"], `true`) {
			t.Errorf("session.create params = %s, want gameId, startedAt, endedAt, activeSeconds and includedInTotal", c.params)
		}
		if got.Session.SessionID != 21 || got.Session.Source != "manual" || got.Session.ActiveSeconds != 5400 {
			t.Errorf("Session = %+v, want the session of the agent", got.Session)
		}
		wantInstant(t, "Session.StartedAt", got.Session.StartedAt, sedStart)
		wantInstant(t, "Session.EndedAt", got.Session.EndedAt, sedEnd)
		if got.Game.ID != 3 || got.Game.Title != "Celeste" || got.Game.TotalPlaySeconds != 9000 || got.Game.SessionPlaySeconds != 5400 {
			t.Errorf("Game = %+v, want the detail of the agent", got.Game)
		}
	})

	t.Run("SED-035/CreateSession com instante inválido dá Dados inválidos. sem chamar o agente", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodSessionCreate: sedWriteResult()}}
		a := &App{client: p, log: discardLog()}

		_, err := a.CreateSession(3, SessionCreateInput{StartedAt: "ontem", EndedAt: "2026-11-25T14:30:00Z", ActiveSeconds: 60})

		wantErrText(t, err, txtInvalidData)
		if n := len(p.recorded()); n != 0 {
			t.Errorf("agent calls = %d, want 0", n)
		}
	})

	t.Run("SED-035/UpdateSession repassa o patch bruto: ausente não é enviado, null é enviado como null", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodSessionUpdate: sedWriteResult()}}
		a := &App{client: p, log: discardLog()}

		got, err := a.UpdateSession(21, json.RawMessage(`{"startedAt":"2026-11-25T13:00:00Z","endedAt":null}`))

		if err != nil {
			t.Fatalf("UpdateSession err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodSessionUpdate, 5*time.Second)
		lp := rawObject(t, c)
		if !sameJSON(t, lp["sessionId"], `21`) || !sameJSON(t, lp["startedAt"], `"2026-11-25T13:00:00Z"`) ||
			!sameJSON(t, lp["endedAt"], `null`) {
			t.Errorf("session.update params = %s, want sessionId and the patch members as sent (endedAt null kept)", c.params)
		}
		if _, ok := lp["activeSeconds"]; ok {
			t.Errorf("session.update params carry activeSeconds, which the patch did not send")
		}
		if got.Session.SessionID != 21 || got.Game.ID != 3 {
			t.Errorf("UpdateSession() = %+v, want the session and the detail of the agent", got)
		}

		// An empty patch sends only sessionId.
		p2 := &gamePinger{results: p.results}
		a.client = p2
		if _, err := a.UpdateSession(21, json.RawMessage(`{}`)); err != nil {
			t.Fatalf("UpdateSession({}) err = %v", err)
		}
		lp2 := rawObject(t, singleCall(t, p2, ipc.MethodSessionUpdate, 5*time.Second))
		if len(lp2) != 1 || !sameJSON(t, lp2["sessionId"], `21`) {
			t.Errorf("params for an empty patch = %v, want only sessionId", lp2)
		}
	})

	t.Run("SED-035/DeleteSession chama session.delete e devolve o detalhe", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodSessionDelete: ipc.SessionDeleteResult{
			SessionID: 21, Game: sedWriteResult().Game}}}
		a := &App{client: p, log: discardLog()}

		got, err := a.DeleteSession(21)

		if err != nil {
			t.Fatalf("DeleteSession err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodSessionDelete, 5*time.Second)
		if !sameJSON(t, rawObject(t, c)["sessionId"], `21`) {
			t.Errorf("session.delete params = %s, want {\"sessionId\":21}", c.params)
		}
		if got.ID != 3 || got.Title != "Celeste" || got.SessionPlaySeconds != 5400 {
			t.Errorf("DeleteSession() = %+v, want the detail of the agent", got)
		}
	})

	t.Run("SED-035/a view traz source, editedAt (\"\" quando nulo) e sessionPlaySeconds, sem calcular tempo", func(t *testing.T) {
		t.Parallel()
		res := sedWriteResult()
		res.Session.EditedAt = nil
		p := &gamePinger{results: map[string]any{ipc.MethodSessionCreate: res}}
		a := &App{client: p, log: discardLog()}

		got, err := a.CreateSession(3, SessionCreateInput{StartedAt: "2026-11-25T13:00:00Z", EndedAt: "2026-11-25T14:30:00Z", ActiveSeconds: 5400})
		if err != nil {
			t.Fatalf("CreateSession err = %v", err)
		}
		if got.Session.EditedAt != "" {
			t.Errorf("EditedAt = %q, want \"\" for a never edited session", got.Session.EditedAt)
		}
		m := jsonObject(t, got)
		sess, _ := m["session"].(map[string]any)
		game, _ := m["game"].(map[string]any)
		if sess["source"] != "manual" || sess["editedAt"] != "" {
			t.Errorf("session JSON = %v, want source manual and editedAt \"\"", sess)
		}
		if game["sessionPlaySeconds"] != float64(5400) || game["totalPlaySeconds"] != float64(9000) {
			t.Errorf("game JSON = %v, want sessionPlaySeconds 5400 and totalPlaySeconds 9000 as the agent sent", game)
		}

		edited := sedEnd.Add(time.Minute)
		p2 := &gamePinger{results: map[string]any{ipc.MethodSessionCreate: sedWriteResult()}}
		a.client = p2
		got2, err := a.CreateSession(3, SessionCreateInput{StartedAt: "2026-11-25T13:00:00Z", EndedAt: "2026-11-25T14:30:00Z", ActiveSeconds: 5400})
		if err != nil {
			t.Fatalf("CreateSession err = %v", err)
		}
		wantInstant(t, "Session.EditedAt", got2.Session.EditedAt, edited)
	})

	t.Run("SED-035/GetGame, GameSessions e UpdateGame também preenchem os campos novos", func(t *testing.T) {
		t.Parallel()
		edited := sedEnd.Add(time.Minute)
		detail := ipc.GameDetail{Game: ipc.Game{ID: 3, Title: "Celeste"}, SessionPlaySeconds: 777}
		p := &gamePinger{results: map[string]any{
			ipc.MethodGameGet:    ipc.GameDetailResult{Game: detail},
			ipc.MethodGameUpdate: ipc.GameDetailResult{Game: detail},
			ipc.MethodSessionList: ipc.SessionListResult{Total: 1, Sessions: []ipc.SessionItem{
				{SessionID: 5, GameID: 3, State: "ended", StartedAt: sedStart, EndedAt: &sedEnd, ActiveSeconds: 60,
					Source: "import", EditedAt: &edited},
				{SessionID: 6, GameID: 3, State: "ended", StartedAt: sedStart, EndedAt: &sedEnd, ActiveSeconds: 60, Source: "tracked"},
			}},
		}}
		a := &App{client: p, log: discardLog()}

		g, err := a.GetGame(3)
		if err != nil || g.SessionPlaySeconds != 777 {
			t.Errorf("GetGame = %+v, %v; want sessionPlaySeconds 777", g, err)
		}
		u, err := a.UpdateGame(3, json.RawMessage(`{}`))
		if err != nil || u.SessionPlaySeconds != 777 {
			t.Errorf("UpdateGame = %+v, %v; want sessionPlaySeconds 777", u, err)
		}
		s, err := a.GameSessions(3, 0, 50)
		if err != nil || len(s.Sessions) != 2 {
			t.Fatalf("GameSessions = %+v, %v", s, err)
		}
		if s.Sessions[0].Source != "import" {
			t.Errorf("Sessions[0].Source = %q, want import", s.Sessions[0].Source)
		}
		wantInstant(t, "Sessions[0].EditedAt", s.Sessions[0].EditedAt, edited)
		if s.Sessions[1].Source != "tracked" || s.Sessions[1].EditedAt != "" {
			t.Errorf("Sessions[1] = %+v, want source tracked and editedAt \"\"", s.Sessions[1])
		}
	})

	t.Run("SED-035/nenhum dos três grava nada nem emite evento Wails por conta própria (DSK-017)", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{
			ipc.MethodSessionCreate: sedWriteResult(),
			ipc.MethodSessionUpdate: sedWriteResult(),
			ipc.MethodSessionDelete: ipc.SessionDeleteResult{SessionID: 21, Game: sedWriteResult().Game},
		}}
		em := &gameEmitLog{}
		a := newBindingApp(p, em)

		if _, err := a.CreateSession(3, SessionCreateInput{StartedAt: "2026-11-25T13:00:00Z", EndedAt: "2026-11-25T14:30:00Z", ActiveSeconds: 60}); err != nil {
			t.Fatalf("CreateSession err = %v", err)
		}
		if _, err := a.UpdateSession(21, json.RawMessage(`{"activeSeconds":120}`)); err != nil {
			t.Fatalf("UpdateSession err = %v", err)
		}
		if _, err := a.DeleteSession(21); err != nil {
			t.Fatalf("DeleteSession err = %v", err)
		}

		var calls []string
		for _, c := range p.recorded() {
			calls = append(calls, c.method)
		}
		want := []string{ipc.MethodSessionCreate, ipc.MethodSessionUpdate, ipc.MethodSessionDelete}
		if fmt.Sprint(calls) != fmt.Sprint(want) {
			t.Errorf("agent calls = %v, want %v (only the IPC methods)", calls, want)
		}
		if got := em.recorded(); len(got) != 0 {
			t.Errorf("events emitted = %v, want none", got)
		}
		if files := storeImporters(t, "."); len(files) != 0 {
			t.Errorf("files importing internal/store = %v, want none", files)
		}
	})
}

func TestStatsManualSeconds(t *testing.T) {
	t.Parallel()

	t.Run("SED-027/StatsSummary repassa manualSeconds por jogo, por plataforma e no total", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodStatsSummary: ipc.StatsSummaryResult{
			Games:         []ipc.StatsGame{{GameID: 3, Title: "Celeste", ActiveSeconds: 900, ManualSeconds: 300}},
			Platforms:     []ipc.StatsPlatform{{PlatformID: 1, Name: "PC", ActiveSeconds: 900, ManualSeconds: 300}},
			TotalSeconds:  1200,
			ManualSeconds: 300,
		}}}
		a := &App{client: p, log: discardLog()}

		got, err := a.StatsSummary("all")

		if err != nil {
			t.Fatalf("StatsSummary err = %v", err)
		}
		m := jsonObject(t, got)
		games, _ := m["games"].([]any)
		plats, _ := m["platforms"].([]any)
		if len(games) != 1 || len(plats) != 1 {
			t.Fatalf("StatsSummary JSON = %v, want one game and one platform", m)
		}
		if g := games[0].(map[string]any); g["manualSeconds"] != float64(300) {
			t.Errorf("game manualSeconds = %v, want 300", g["manualSeconds"])
		}
		if pl := plats[0].(map[string]any); pl["manualSeconds"] != float64(300) {
			t.Errorf("platform manualSeconds = %v, want 300", pl["manualSeconds"])
		}
		if m["manualSeconds"] != float64(300) {
			t.Errorf("total manualSeconds = %v, want 300", m["manualSeconds"])
		}
	})
}

// sedText runs sessionEditError with a recording logger.
func sedText(t *testing.T, err error) string {
	t.Helper()
	got := sessionEditError(err, newRecLog().logger(), brt)
	if got == nil {
		t.Fatalf("sessionEditError(%v) = nil, want an error", err)
	}
	return got.Error()
}

func TestSessionEditMessages(t *testing.T) {
	// Not parallel: the binding cases swap displayLocation.

	t.Run("SED-036/-32017 vira Esta sessão não existe mais.", func(t *testing.T) {
		if got := sedText(t, notFoundSessionErr()); got != txtSessionGone {
			t.Errorf("text = %q, want %q", got, txtSessionGone)
		}
		if got := sedText(t, fmt.Errorf("desktop: session.delete: %w", notFoundSessionErr())); got != txtSessionGone {
			t.Errorf("wrapped text = %q, want %q", got, txtSessionGone)
		}
	})

	t.Run("SED-036/-32018 vira a mensagem com título e data e hora locais", func(t *testing.T) {
		// 13:00Z and 14:30Z are 10:00 and 11:30 in BRT.
		got := sedText(t, overlapErr("Celeste", sedStart, &sedEnd))
		want := "O horário cruza outra sessão de «Celeste», de 25/11/2026 10:00 a 25/11/2026 11:30."
		if got != want {
			t.Errorf("text = %q, want %q", got, want)
		}
	})

	t.Run("SED-036/-32018 com endedAt nulo usa agora no fim", func(t *testing.T) {
		got := sedText(t, overlapErr("Celeste", sedStart, nil))
		want := "O horário cruza outra sessão de «Celeste», de 25/11/2026 10:00 a agora."
		if got != want {
			t.Errorf("text = %q, want %q", got, want)
		}
	})

	t.Run("SED-036/-32018 nos bindings usa displayLocation", func(t *testing.T) {
		old := displayLocation
		displayLocation = brt
		t.Cleanup(func() { displayLocation = old })
		p := &gamePinger{errs: map[string]error{ipc.MethodSessionUpdate: overlapErr("Celeste", sedStart, &sedEnd)}}
		a := &App{client: p, log: discardLog()}

		_, err := a.UpdateSession(21, json.RawMessage(`{"activeSeconds":120}`))

		wantErrText(t, err, "O horário cruza outra sessão de «Celeste», de 25/11/2026 10:00 a 25/11/2026 11:30.")
	})

	t.Run("SED-036/-32002 em session.update e session.delete vira Não é possível alterar uma sessão em andamento.", func(t *testing.T) {
		for _, method := range []string{ipc.MethodSessionUpdate, ipc.MethodSessionDelete} {
			p := &gamePinger{errs: map[string]error{method: sessionActiveErr("Celeste")}}
			a := &App{client: p, log: discardLog()}
			var err error
			if method == ipc.MethodSessionUpdate {
				_, err = a.UpdateSession(21, json.RawMessage(`{"activeSeconds":120}`))
			} else {
				_, err = a.DeleteSession(21)
			}
			wantErrText(t, err, txtSessionOpen)
		}
		if got := sedText(t, sessionActiveErr("Celeste")); got != txtSessionOpen {
			t.Errorf("sessionEditError text = %q, want %q", got, txtSessionOpen)
		}
	})

	t.Run("SED-036/o -32002 de game.launch continua com o texto de antes", func(t *testing.T) {
		wantText(t, sessionActiveErr("Celeste"), fmt.Sprintf(sessionActiveFmt, "Celeste"))
		p := &gamePinger{errs: map[string]error{ipc.MethodGameLaunch: sessionActiveErr("Celeste")}}
		a := &App{client: p, log: discardLog()}
		_, err := a.LaunchGame(3)
		wantErrText(t, err, fmt.Sprintf(sessionActiveFmt, "Celeste"))
	})

	t.Run("SED-036/-32602 com reason: um caso por linha da tabela do contrato §4", func(t *testing.T) {
		table := []struct{ reason, want string }{
			{"end-not-after-start", "O fim precisa ser depois do início."},
			{"in-future", "O fim não pode estar no futuro."},
			{"too-early", "O início não pode ser antes de 1980."},
			{"span-too-long", "Uma sessão pode ter no máximo 24 horas."},
			{"active-too-short", "O tempo jogado precisa ter pelo menos 1 minuto."},
			{"active-exceeds-span", "O tempo jogado não pode passar do intervalo entre início e fim."},
			{"discarded", "Sessões descartadas não podem ser editadas, só excluídas."},
			{"exceeds-adjustment", "O tempo jogado é maior que a parte do tempo total que não vem de sessões."},
			{"empty-patch", txtSessionNoSave},
		}
		for _, tc := range table {
			if got := sedText(t, reasonErr(tc.reason)); got != tc.want {
				t.Errorf("reason %q: text = %q, want %q", tc.reason, got, tc.want)
			}
		}
	})

	t.Run("SED-036/-32602 sem reason e -32001 mantêm os textos de hoje", func(t *testing.T) {
		if got := sedText(t, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)); got != txtInvalidData {
			t.Errorf("-32602 without reason = %q, want %q", got, txtInvalidData)
		}
		if got := sedText(t, rawDataErr(ipc.CodeInvalidParams, ipc.MsgInvalidParams, `{"reason":"weird"}`)); got != txtInvalidData {
			t.Errorf("-32602 with unknown reason = %q, want %q", got, txtInvalidData)
		}
		if got := sedText(t, gameNotFoundErr()); got != txtGameNotFound {
			t.Errorf("-32001 = %q, want %q", got, txtGameNotFound)
		}
		if got := sedText(t, errors.New("boom")); got != txtUnexpected {
			t.Errorf("other error = %q, want %q", got, txtUnexpected)
		}
	})

	t.Run("SED-036/desconexão e prazo viram O agente não está respondendo. Tente de novo.", func(t *testing.T) {
		for _, err := range []error{
			ipc.ErrConnectionLost,
			fmt.Errorf("ipc: call session.create: %w", ipc.ErrClientClosed),
			fmt.Errorf("ipc: call session.update: %w", context.DeadlineExceeded),
			errNoClient,
		} {
			if got := sedText(t, err); got != txtNotResponding {
				t.Errorf("sessionEditError(%v) = %q, want %q", err, got, txtNotResponding)
			}
		}
	})
}
