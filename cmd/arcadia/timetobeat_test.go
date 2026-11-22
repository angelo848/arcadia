package main

// Tests for T15 (docs/specs/phase-15-big-box-v2/tasks.md): the desktop bindings of the time to beat
// and the forwarding of timeToBeat.changed [TTB-030]. Contract: contracts/ipc-v0.16.0-delta.md §6.
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - Deadlines: TimeToBeatGet, Choose, Settings and SetSettings use 2 s; Refresh and Search use 30 s.
//   - TimeToBeatGet sends fetch only when true (omitempty); Search omits term when it is "".
//   - TimeToBeatSettings calls timeToBeat.getSettings; SetSettings sends both enabled and afterMetadata.
//   - FetchedAt of the view is "" when the wire value is null, non-empty otherwise.
//   - Candidates of the view is never nil.
//   - The -32011 texts are the literals of the delta: "Não foi possível consultar o HowLongToBeat" and
//     "Consulta ao HowLongToBeat desligada" (reason disabled).
//   - The event is "timeToBeat:changed" and the Warn message is "timeToBeat.changed com parâmetros inválidos".

import (
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

const (
	ttbShortTimeout = 2 * time.Second
	ttbLongTimeout  = 30 * time.Second

	ttbChangedEventName = "timeToBeat:changed"
	msgTTBChangedBad    = "timeToBeat.changed com parâmetros inválidos"
	msgTTBUnavailable   = "Não foi possível consultar o HowLongToBeat"
	msgTTBDisabled      = "Consulta ao HowLongToBeat desligada"
)

func ttbViewWire(fetchedAt any) map[string]any {
	return map[string]any{
		"gameId": 3, "state": "found", "source": "hltb", "externalId": "42", "externalTitle": "Celeste",
		"mainSeconds": 28800, "mainExtraSeconds": nil, "completionistSeconds": 144000,
		"fetchedAt": fetchedAt, "confirmed": true, "stale": false, "pending": false, "unavailable": false,
		"candidates": []any{},
	}
}

// ttbCallWithin checks the single call and a deadline in (lo, hi].
func ttbCallWithin(t *testing.T, p *gamePinger, method string, hi time.Duration) gameCall {
	t.Helper()
	c := singleCall(t, p, method, hi)
	if hi == ttbLongTimeout && c.remaining <= ttbShortTimeout {
		t.Errorf("%s deadline = %v, want about %v", method, c.remaining, hi)
	}
	return c
}

var ttbBindingCases = []struct {
	name    string
	method  string
	result  any
	timeout time.Duration
	call    func(a *App) error
}{
	{"TimeToBeatGet", ipc.MethodTimeToBeatGet, ttbViewWire(nil), ttbShortTimeout, func(a *App) error {
		_, err := a.TimeToBeatGet(3, true)
		return err
	}},
	{"TimeToBeatRefresh", ipc.MethodTimeToBeatRefresh, ttbViewWire(nil), ttbLongTimeout, func(a *App) error {
		_, err := a.TimeToBeatRefresh(3)
		return err
	}},
	{"TimeToBeatSearch", ipc.MethodTimeToBeatSearch, map[string]any{"candidates": []any{}}, ttbLongTimeout, func(a *App) error {
		_, err := a.TimeToBeatSearch(3, "Celeste")
		return err
	}},
	{"TimeToBeatChoose", ipc.MethodTimeToBeatChoose, ttbViewWire(nil), ttbShortTimeout, func(a *App) error {
		_, err := a.TimeToBeatChoose(3, "42")
		return err
	}},
	{"TimeToBeatSettings", ipc.MethodTimeToBeatGetSettings, map[string]any{"enabled": true, "afterMetadata": false}, ttbShortTimeout, func(a *App) error {
		_, err := a.TimeToBeatSettings()
		return err
	}},
	{"TimeToBeatSetSettings", ipc.MethodTimeToBeatSetSettings, map[string]any{"enabled": true, "afterMetadata": false}, ttbShortTimeout, func(a *App) error {
		_, err := a.TimeToBeatSetSettings(true, false)
		return err
	}},
}

func TestTimeToBeatBindings(t *testing.T) {
	t.Parallel()

	t.Run("TTB-030/os seis bindings existem com as assinaturas do delta §6", func(t *testing.T) {
		t.Parallel()
		typ := reflect.TypeOf(&App{})
		i64, str, boolT := reflect.TypeOf(int64(0)), reflect.TypeOf(""), reflect.TypeOf(true)
		errT := reflect.TypeOf((*error)(nil)).Elem()
		want := map[string]struct {
			in  []reflect.Type
			out reflect.Type
		}{
			"TimeToBeatGet":         {[]reflect.Type{i64, boolT}, reflect.TypeOf(TimeToBeatView{})},
			"TimeToBeatRefresh":     {[]reflect.Type{i64}, reflect.TypeOf(TimeToBeatView{})},
			"TimeToBeatSearch":      {[]reflect.Type{i64, str}, reflect.TypeOf([]TimeToBeatCandidateView{})},
			"TimeToBeatChoose":      {[]reflect.Type{i64, str}, reflect.TypeOf(TimeToBeatView{})},
			"TimeToBeatSettings":    {nil, reflect.TypeOf(TimeToBeatSettingsView{})},
			"TimeToBeatSetSettings": {[]reflect.Type{boolT, boolT}, reflect.TypeOf(TimeToBeatSettingsView{})},
		}
		for name, w := range want {
			m, ok := typ.MethodByName(name)
			if !ok {
				t.Errorf("*App has no method %s", name)
				continue
			}
			mt := m.Type // receiver is the first input
			if mt.NumIn() != len(w.in)+1 || mt.NumOut() != 2 || mt.Out(0) != w.out || mt.Out(1) != errT {
				t.Errorf("%s has signature %v, want inputs %v returning (%v, error)", name, mt, w.in, w.out)
				continue
			}
			for i, in := range w.in {
				if mt.In(i+1) != in {
					t.Errorf("%s input %d = %v, want %v", name, i, mt.In(i+1), in)
				}
			}
		}
	})

	t.Run("TTB-030/TimeToBeatGet envia gameId e fetch true com prazo de 2 s e devolve a visão", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodTimeToBeatGet: ttbViewWire("2026-11-22T10:00:00Z")}}
		a := newBindingApp(p, &gameEmitLog{})

		v, err := a.TimeToBeatGet(3, true)
		if err != nil {
			t.Fatalf("TimeToBeatGet err = %v", err)
		}
		m := wireMap(t, ttbCallWithin(t, p, ipc.MethodTimeToBeatGet, ttbShortTimeout))
		if m["gameId"] != float64(3) || m["fetch"] != true {
			t.Errorf("timeToBeat.get params = %v, want gameId 3 and fetch true", m)
		}
		if v.GameID != 3 || v.State != "found" || v.Source != "hltb" || v.ExternalID != "42" || v.ExternalTitle != "Celeste" ||
			!v.Confirmed || v.Stale || v.Pending || v.Unavailable {
			t.Errorf("view = %+v, want the wire fields", v)
		}
		if v.MainSeconds == nil || *v.MainSeconds != 28800 || v.MainExtraSeconds != nil ||
			v.CompletionistSeconds == nil || *v.CompletionistSeconds != 144000 {
			t.Errorf("view durations = %v/%v/%v, want 28800/nil/144000", v.MainSeconds, v.MainExtraSeconds, v.CompletionistSeconds)
		}
		if !strings.Contains(v.FetchedAt, "2026-11-22") {
			t.Errorf("FetchedAt = %q, want the date of the wire value", v.FetchedAt)
		}
		if v.Candidates == nil {
			t.Error("Candidates is nil, want an empty slice")
		}
	})

	t.Run("TTB-030/TimeToBeatGet com fetch false omite fetch e fetchedAt nulo vira texto vazio", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodTimeToBeatGet: ttbViewWire(nil)}}
		a := newBindingApp(p, &gameEmitLog{})

		v, err := a.TimeToBeatGet(3, false)
		if err != nil {
			t.Fatalf("TimeToBeatGet err = %v", err)
		}
		m := wireMap(t, ttbCallWithin(t, p, ipc.MethodTimeToBeatGet, ttbShortTimeout))
		if _, ok := m["fetch"]; ok {
			t.Errorf("timeToBeat.get sent fetch %v, want it omitted when false", m["fetch"])
		}
		if v.FetchedAt != "" {
			t.Errorf("FetchedAt = %q, want empty for a null fetchedAt", v.FetchedAt)
		}
	})

	t.Run("TTB-030/TimeToBeatRefresh envia gameId com prazo de 30 s", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodTimeToBeatRefresh: ttbViewWire(nil)}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.TimeToBeatRefresh(3); err != nil {
			t.Fatalf("TimeToBeatRefresh err = %v", err)
		}
		m := wireMap(t, ttbCallWithin(t, p, ipc.MethodTimeToBeatRefresh, ttbLongTimeout))
		if m["gameId"] != float64(3) || len(m) != 1 {
			t.Errorf("timeToBeat.refresh params = %v, want only gameId 3", m)
		}
	})

	t.Run("TTB-030/TimeToBeatSearch envia gameId e term com prazo de 30 s e devolve os candidatos", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodTimeToBeatSearch: map[string]any{"candidates": []any{
			map[string]any{
				"externalId": "42", "title": "Celeste", "year": 2018, "platforms": []any{"PC", "Switch"},
				"mainSeconds": 28800, "mainExtraSeconds": nil, "completionistSeconds": 144000, "score": 0.9,
			},
		}}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.TimeToBeatSearch(3, "Celeste")
		if err != nil {
			t.Fatalf("TimeToBeatSearch err = %v", err)
		}
		m := wireMap(t, ttbCallWithin(t, p, ipc.MethodTimeToBeatSearch, ttbLongTimeout))
		if m["gameId"] != float64(3) || m["term"] != "Celeste" {
			t.Errorf("timeToBeat.search params = %v, want gameId 3 and term Celeste", m)
		}
		if len(got) != 1 {
			t.Fatalf("candidates = %d, want 1", len(got))
		}
		c := got[0]
		if c.ExternalID != "42" || c.Title != "Celeste" || c.Year == nil || *c.Year != 2018 || c.Score != 0.9 ||
			len(c.Platforms) != 2 || c.MainSeconds == nil || *c.MainSeconds != 28800 || c.MainExtraSeconds != nil {
			t.Errorf("candidate = %+v, want the wire fields", c)
		}
	})

	t.Run("TTB-030/TimeToBeatSearch com term vazio omite term", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodTimeToBeatSearch: map[string]any{"candidates": []any{}}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.TimeToBeatSearch(3, "")
		if err != nil {
			t.Fatalf("TimeToBeatSearch err = %v", err)
		}
		m := wireMap(t, ttbCallWithin(t, p, ipc.MethodTimeToBeatSearch, ttbLongTimeout))
		if _, ok := m["term"]; ok {
			t.Errorf("timeToBeat.search sent term %v, want it omitted", m["term"])
		}
		if got == nil || len(got) != 0 {
			t.Errorf("candidates = %#v, want an empty non-nil slice", got)
		}
	})

	t.Run("TTB-030/TimeToBeatChoose envia gameId e externalId com prazo de 2 s; vazio continua enviado", func(t *testing.T) {
		t.Parallel()
		for _, ext := range []string{"42", ""} {
			p := &gamePinger{results: map[string]any{ipc.MethodTimeToBeatChoose: ttbViewWire(nil)}}
			a := newBindingApp(p, &gameEmitLog{})
			if _, err := a.TimeToBeatChoose(3, ext); err != nil {
				t.Fatalf("TimeToBeatChoose(%q) err = %v", ext, err)
			}
			m := wireMap(t, ttbCallWithin(t, p, ipc.MethodTimeToBeatChoose, ttbShortTimeout))
			if v, ok := m["externalId"]; !ok || v != ext || m["gameId"] != float64(3) {
				t.Errorf("timeToBeat.choose params = %v, want gameId 3 and externalId %q", m, ext)
			}
		}
	})

	t.Run("TTB-030/TimeToBeatSettings chama timeToBeat.getSettings sem parâmetros com prazo de 2 s", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodTimeToBeatGetSettings: map[string]any{"enabled": true, "afterMetadata": false}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.TimeToBeatSettings()
		if err != nil {
			t.Fatalf("TimeToBeatSettings err = %v", err)
		}
		c := ttbCallWithin(t, p, ipc.MethodTimeToBeatGetSettings, ttbShortTimeout)
		if s := string(c.params); s != "" && s != "null" && s != "{}" {
			t.Errorf("timeToBeat.getSettings params = %s, want none", s)
		}
		if !got.Enabled || got.AfterMetadata {
			t.Errorf("settings = %+v, want enabled and not afterMetadata", got)
		}
	})

	t.Run("TTB-030/TimeToBeatSetSettings envia enabled e afterMetadata com prazo de 2 s", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodTimeToBeatSetSettings: map[string]any{"enabled": false, "afterMetadata": true}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.TimeToBeatSetSettings(false, true)
		if err != nil {
			t.Fatalf("TimeToBeatSetSettings err = %v", err)
		}
		m := wireMap(t, ttbCallWithin(t, p, ipc.MethodTimeToBeatSetSettings, ttbShortTimeout))
		if m["enabled"] != false || m["afterMetadata"] != true {
			t.Errorf("timeToBeat.setSettings params = %v, want enabled false and afterMetadata true", m)
		}
		if got.Enabled || !got.AfterMetadata {
			t.Errorf("settings = %+v, want not enabled and afterMetadata", got)
		}
	})
}

func TestTimeToBeatBindingsErrors(t *testing.T) {
	t.Parallel()

	for _, b := range ttbBindingCases {
		t.Run("TTB-030/"+b.name+" com conexão perdida mostra o texto de agente sem resposta (UI-008)", func(t *testing.T) {
			t.Parallel()
			p := &gamePinger{errs: map[string]error{b.method: ipc.ErrConnectionLost}}
			a := newBindingApp(p, &gameEmitLog{})
			if err := b.call(a); err == nil || err.Error() != txtNotResponding {
				t.Errorf("%s err = %v, want %q", b.name, err, txtNotResponding)
			}
		})
		t.Run("TTB-030/"+b.name+" com outro erro mostra o erro inesperado (UI-008)", func(t *testing.T) {
			t.Parallel()
			p := &gamePinger{errs: map[string]error{b.method: errors.New("boom")}}
			a := newBindingApp(p, &gameEmitLog{})
			if err := b.call(a); err == nil || err.Error() != txtUnexpected {
				t.Errorf("%s err = %v, want %q", b.name, err, txtUnexpected)
			}
		})
		t.Run("TTB-030/"+b.name+" com -32011 vira a mensagem do HowLongToBeat", func(t *testing.T) {
			t.Parallel()
			e := ipc.NewErrorData(ipc.CodeProviderUnavailable, ipc.MsgProviderUnavailable,
				ipc.ProviderUnavailableData{Provider: ipc.ProviderHLTB, Reason: ipc.ProviderReasonUnavailable})
			p := &gamePinger{errs: map[string]error{b.method: e}}
			a := newBindingApp(p, &gameEmitLog{})
			if err := b.call(a); err == nil || err.Error() != msgTTBUnavailable {
				t.Errorf("%s err = %v, want %q", b.name, err, msgTTBUnavailable)
			}
		})
		t.Run("TTB-030/"+b.name+" com -32011 e reason disabled mostra que a consulta está desligada", func(t *testing.T) {
			t.Parallel()
			e := ipc.NewErrorData(ipc.CodeProviderUnavailable, ipc.MsgProviderUnavailable,
				ipc.ProviderUnavailableData{Provider: ipc.ProviderHLTB, Reason: ipc.ProviderReasonDisabled})
			p := &gamePinger{errs: map[string]error{b.method: e}}
			a := newBindingApp(p, &gameEmitLog{})
			if err := b.call(a); err == nil || err.Error() != msgTTBDisabled {
				t.Errorf("%s err = %v, want %q", b.name, err, msgTTBDisabled)
			}
		})
	}
}

func TestTimeToBeatNeverTouchesSession(t *testing.T) {
	t.Parallel()

	t.Run("TTB-030/nenhum binding chama método de sessão, pausa, fechar jogo nem emite eventos", func(t *testing.T) {
		t.Parallel()
		results := map[string]any{}
		allowed := map[string]bool{}
		for _, b := range ttbBindingCases {
			results[b.method] = b.result
			allowed[b.method] = true
		}
		p := &gamePinger{results: results}
		em := &gameEmitLog{}
		a := newBindingApp(p, em)

		for _, b := range ttbBindingCases {
			if err := b.call(a); err != nil {
				t.Fatalf("%s err = %v", b.name, err)
			}
		}
		calls := p.recorded()
		if len(calls) != len(ttbBindingCases) {
			t.Errorf("agent calls = %d, want %d (one per binding)", len(calls), len(ttbBindingCases))
		}
		for _, c := range calls {
			if !allowed[c.method] {
				t.Errorf("binding called %q, want only timeToBeat.* methods", c.method)
			}
			if strings.HasPrefix(c.method, "game.") || strings.HasPrefix(c.method, "session.") || strings.Contains(c.method, "pause") {
				t.Errorf("binding called the session/game method %q", c.method)
			}
		}
		if ev := em.recorded(); len(ev) != 0 {
			t.Errorf("bindings emitted %v, want no events", ev)
		}
	})
}

func TestSupervisorTimeToBeatChanged(t *testing.T) {
	t.Parallel()

	t.Run("TTB-030/o supervisor repassa timeToBeat.changed como o evento timeToBeat:changed {gameId}", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		h.client.send(t, ipc.Notification{Method: ipc.MethodTimeToBeatChanged, Params: json.RawMessage(`{"gameId":42}`)})
		h.client.finishNotes()
		h.waitDone(t)

		got := collectEmits(h)
		if len(got) != 1 || got[0].event != ttbChangedEventName {
			t.Fatalf("emitted %#v, want exactly one %s", got, ttbChangedEventName)
		}
		raw, err := json.Marshal(got[0].data)
		if err != nil {
			t.Fatalf("marshal event data: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil || m["gameId"] != float64(42) || len(m) != 1 {
			t.Errorf("event data = %s, want {\"gameId\":42}", raw)
		}
		if n := h.logs.countLevel(slog.LevelWarn); n != 0 {
			t.Errorf("Warn records = %d, want 0; log:\n%s", n, h.logs.dump())
		}
	})

	t.Run("TTB-030/params ausentes, JSON inválido, gameId ausente, não inteiro, 0 e negativo: um Warn cada e nada emitido", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		bad := []json.RawMessage{
			nil,
			json.RawMessage(`{bad`),
			json.RawMessage(`{}`),
			json.RawMessage(`{"gameId":null}`),
			json.RawMessage(`{"gameId":0}`),
			json.RawMessage(`{"gameId":-3}`),
			json.RawMessage(`{"gameId":1.5}`),
			json.RawMessage(`{"gameId":"7"}`),
		}
		for _, p := range bad {
			h.client.send(t, ipc.Notification{Method: ipc.MethodTimeToBeatChanged, Params: p})
		}
		h.client.finishNotes()
		h.waitDone(t)

		if got := collectEmits(h); len(got) != 0 {
			t.Errorf("emitted %#v, want nothing", got)
		}
		if n := len(h.logs.find(0, slog.LevelWarn, msgTTBChangedBad)); n != len(bad) {
			t.Errorf("%q logged %d times, want %d; log:\n%s", msgTTBChangedBad, n, len(bad), h.logs.dump())
		}
	})

	t.Run("TTB-030/o repasse não chama o agente, não chama Quit, não relança e o supervisor continua conectado", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)
		before := len(h.client.methodCalls())

		h.client.send(t, ipc.Notification{Method: ipc.MethodTimeToBeatChanged, Params: json.RawMessage(`{"gameId":7}`)})
		select {
		case e := <-h.emits:
			if e.event != ttbChangedEventName {
				t.Fatalf("event = %q, want %q", e.event, ttbChangedEventName)
			}
		case <-time.After(testTimeout):
			t.Fatal("timeToBeat:changed not emitted")
		}
		if h.isDone() || !h.client.Connected() {
			t.Error("supervisor stopped or client disconnected")
		}
		h.client.finishNotes()
		h.waitDone(t)

		if after := len(h.client.methodCalls()); after != before {
			t.Errorf("agent methods called after the notification: %v", h.client.methodCalls())
		}
		if n := h.quitN.Load(); n != 0 {
			t.Errorf("Quit called %d times", n)
		}
		if n := h.ens.n.Load(); n != 1 {
			t.Errorf("Ensure called %d times, want 1", n)
		}
	})
}
