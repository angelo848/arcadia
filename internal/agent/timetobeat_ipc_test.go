package agent

// Tests of T13 (docs/specs/phase-15-big-box-v2/tasks.md): the six timeToBeat.* handlers, the settings, the
// hooks of the service (metadata.apply, the batch, game.delete) and the wiring of the limiters. Fakes and the
// harness are the ones of timetobeat_fakes_test.go; the store is real, in t.TempDir(); time is the FakeClock.
//
// Assumed production surface (T14):
//   - internal/agent/timetobeat_ipc.go: func ttbHandlers(s *timeToBeatService) map[string]ipc.Handler with the six
//     methods. The results are ipc.TimeToBeat (get, refresh, choose), ipc.TimeToBeatSearchResult (search) and
//     ipc.TimeToBeatSettings (getSettings, setSettings), by value. Params are validated with ipc.DecodeParams
//     (-32602 for absent, unknown or mistyped members, gameId < 1, term empty after the trim or > 200 runes).
//   - config.TimeToBeatClient (ttbSearcher) and config.TimeToBeatFallback (ttbFallback): when set, the agent uses
//     them instead of the real HowLongToBeat and IGDB clients (nil = the real ones).
//   - instance.ttb *timeToBeatService; Shutdown calls a.step("ttb.begin") right after the service's BeginShutdown
//     (together with the tracker's) and a.step("ttb.close") right after the service's Close (before "store.close").
//   - metadataService.ttb (the service, or a type with AfterMetadata(int64)): metadata.apply and the batch
//     application call AfterMetadata(gameID) after a successful write.
//   - scrape_providers.go: providerLimiters gains hltb; newProviderLimiters(clock) builds the four;
//     newProviderFactoryWith(keys, client, clock, lim) is newProviderFactory over given limiters;
//     newTTBFallback(keys, client, clock, igdbLimiter) is the ttbFallback over *scrapers.IGDB (ErrNoKey without key).
// Alterado em 2026-11-22 (fase 15): arquivo novo (T13).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// ---- helpers ----

// ttbIPC is the harness with the handlers of the six methods.
type ttbIPC struct {
	*ttbHarness
	hs map[string]ipc.Handler
}

func newTTBIPC(t *testing.T) *ttbIPC {
	t.Helper()
	h := newTTBHarness(t)
	return &ttbIPC{ttbHarness: h, hs: ttbHandlers(h.svc)}
}

// call runs a handler; params is a JSON string, a value to marshal, or nil (no params).
func (x *ttbIPC) call(method string, params any) (any, error) {
	x.t.Helper()
	h, ok := x.hs[method]
	if !ok {
		x.t.Fatalf("no handler for %s", method)
	}
	var raw json.RawMessage
	switch v := params.(type) {
	case nil:
	case string:
		raw = json.RawMessage(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			x.t.Fatalf("marshal params: %v", err)
		}
		raw = b
	}
	return h(x.t.Context(), raw)
}

func asTTB(t *testing.T, out any, err error) ipc.TimeToBeat {
	t.Helper()
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	v, ok := out.(ipc.TimeToBeat)
	if !ok {
		t.Fatalf("result type %T, want ipc.TimeToBeat", out)
	}
	return v
}

func (x *ttbIPC) rpcGet(id int64, fetch bool) ipc.TimeToBeat {
	x.t.Helper()
	out, err := x.call(ipc.MethodTimeToBeatGet, ipc.TimeToBeatGetParams{GameID: id, Fetch: fetch})
	return asTTB(x.t, out, err)
}

func (x *ttbIPC) rpcRefresh(id int64) (ipc.TimeToBeat, error) {
	x.t.Helper()
	out, err := x.call(ipc.MethodTimeToBeatRefresh, ipc.TimeToBeatGameParams{GameID: id})
	if err != nil {
		return ipc.TimeToBeat{}, err
	}
	return asTTB(x.t, out, nil), nil
}

func (x *ttbIPC) rpcSearch(id int64, term *string) ([]ipc.TimeToBeatCandidate, error) {
	x.t.Helper()
	out, err := x.call(ipc.MethodTimeToBeatSearch, ipc.TimeToBeatSearchParams{GameID: id, Term: term})
	if err != nil {
		return nil, err
	}
	res, ok := out.(ipc.TimeToBeatSearchResult)
	if !ok {
		x.t.Fatalf("search result type %T, want ipc.TimeToBeatSearchResult", out)
	}
	if res.Candidates == nil {
		x.t.Fatal("search candidates is nil, want [] on the wire")
	}
	return res.Candidates, nil
}

func (x *ttbIPC) rpcChoose(id int64, externalID string) (ipc.TimeToBeat, error) {
	x.t.Helper()
	out, err := x.call(ipc.MethodTimeToBeatChoose, ipc.TimeToBeatChooseParams{GameID: id, ExternalID: externalID})
	if err != nil {
		return ipc.TimeToBeat{}, err
	}
	return asTTB(x.t, out, nil), nil
}

func (x *ttbIPC) settings() ipc.TimeToBeatSettings {
	x.t.Helper()
	out, err := x.call(ipc.MethodTimeToBeatGetSettings, nil)
	return asSettings(x.t, out, err)
}

func (x *ttbIPC) setSettings(params any) (ipc.TimeToBeatSettings, error) {
	x.t.Helper()
	out, err := x.call(ipc.MethodTimeToBeatSetSettings, params)
	if err != nil {
		return ipc.TimeToBeatSettings{}, err
	}
	return asSettings(x.t, out, nil), nil
}

func asSettings(t *testing.T, out any, err error) ipc.TimeToBeatSettings {
	t.Helper()
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	v, ok := out.(ipc.TimeToBeatSettings)
	if !ok {
		t.Fatalf("result type %T, want ipc.TimeToBeatSettings", out)
	}
	return v
}

func wantProviderErr(t *testing.T, err error, reason string) {
	t.Helper()
	wantCode(t, err, ipc.CodeProviderUnavailable)
	var d ipc.ProviderUnavailableData
	if jerr := json.Unmarshal(ipcErr(t, err).Data, &d); jerr != nil {
		t.Fatalf("error data: %v", jerr)
	}
	if d.Provider != ipc.ProviderHLTB || d.Reason != reason {
		t.Fatalf("error data = %+v, want provider %q reason %q", d, ipc.ProviderHLTB, reason)
	}
}

// failRefresh makes the HowLongToBeat fail once through refresh, which puts it in recuo; with the IGDB key
// reported missing the service is then "unavailable" (TTB-011).
func (x *ttbIPC) intoRecuo(id int64) {
	x.t.Helper()
	x.svc.igdbHasKey = func() bool { return false }
	x.hltb.setFn(hltbDown)
	_, err := x.rpcRefresh(id)
	wantProviderErr(x.t, err, ipc.ProviderReasonUnavailable)
	if !x.rpcGet(id, false).Unavailable {
		x.t.Fatal("service not unavailable after the failure")
	}
}

func (x *ttbIPC) seedFound(id int64, title string, confirmed bool) {
	x.t.Helper()
	x.seed(id, title, store.TimeToBeatRow{
		Source: "hltb", State: "found", ExternalID: "7", ExternalTitle: title,
		MainS: i64(100), MainExtraS: i64(200), CompletionistS: i64(300), CandidatesJSON: "[]", Confirmed: confirmed,
	}, ttbStart)
}

// ---- TTB-007 ----

func TestTTBIPCGetReadOnly(t *testing.T) {
	t.Parallel()
	t.Run("TTB-007/fetch false sem linha: none, sem requisição e sem fila", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.start()
		id := x.game("Hades")
		v := x.rpcGet(id, false)
		if v.GameID != id || v.State != ipc.TimeToBeatStateNone || v.Pending || v.Unavailable || v.Candidates == nil {
			t.Fatalf("view = %+v, want none, not pending, candidates []", v)
		}
		if x.svc.pending(id) {
			t.Fatal("game queued by a read")
		}
		if n := len(x.hltb.calls()) + len(x.igdb.searches()); n != 0 {
			t.Fatalf("requests = %d, want 0", n)
		}
	})
	t.Run("TTB-007/linha vencida volta como está com stale true e nada é pedido", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.start()
		id := x.game("Hades")
		x.seedFound(id, "Hades", false)
		x.clock.Advance(400 * 24 * time.Hour)
		v := x.rpcGet(id, false)
		if !v.Stale || v.State != ipc.TimeToBeatStateFound || v.ExternalID != "7" || v.Pending {
			t.Fatalf("view = %+v, want found 7, stale, not pending", v)
		}
		if v.MainSeconds == nil || *v.MainSeconds != 100 {
			t.Fatalf("mainSeconds = %v, want 100", v.MainSeconds)
		}
		if n := len(x.hltb.calls()); n != 0 {
			t.Fatalf("hltb calls = %d, want 0", n)
		}
	})
	t.Run("TTB-007/pending reflete a busca em curso", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.hltb.gate()
		x.start()
		id := x.game("Hades")
		if !x.rpcGet(id, true).Pending {
			t.Fatal("fetch true: pending false, want true")
		}
		x.waitEntered()
		if !x.rpcGet(id, false).Pending {
			t.Fatal("in flight: pending false, want true")
		}
		x.hltb.openGate()
		x.waitNote(id)
		if x.rpcGet(id, false).Pending {
			t.Fatal("after the write: pending true, want false")
		}
	})
	t.Run("TTB-007/unavailable reflete o recuo", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades")
		x.intoRecuo(id)
	})
	t.Run("TTB-007/jogo inexistente é -32001 e parâmetros inválidos são -32602", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		_, err := x.call(ipc.MethodTimeToBeatGet, ipc.TimeToBeatGetParams{GameID: 999})
		wantCode(t, err, ipc.CodeGameNotFound)
		for _, p := range []any{nil, `{}`, `{"gameId":0}`, `{"gameId":-1}`, `{"gameId":1.5}`, `{"gameId":"1"}`, `{"gameId":1,"fetch":"yes"}`, `{"gameId":1,"x":1}`} {
			_, err := x.call(ipc.MethodTimeToBeatGet, p)
			wantCode(t, err, ipc.CodeInvalidParams)
		}
	})
}

// ---- TTB-008 ----

func TestTTBIPCGetFetch(t *testing.T) {
	t.Parallel()
	t.Run("TTB-008/fetch true sem linha: enfileira, responde na hora com pending e o cache, e grava", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.hltb.gate()
		x.start()
		id := x.game("Hades")
		v := x.rpcGet(id, true)
		if !v.Pending || v.State != ipc.TimeToBeatStateNone {
			t.Fatalf("view = %+v, want pending with state none", v)
		}
		if term := x.waitEntered(); term != "Hades" {
			t.Fatalf("term = %q, want Hades", term)
		}
		x.hltb.openGate()
		x.waitNote(id)
		row, ok := x.row(id)
		if !ok || row.State != "found" {
			t.Fatalf("row = %+v ok=%v, want found", row, ok)
		}
	})
	t.Run("TTB-008/linha válida: nada é enfileirado", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.start()
		id, sentinel := x.game("Hades"), x.game("Celeste")
		x.seedFound(id, "Hades", false)
		v := x.rpcGet(id, true)
		if v.Pending || v.Stale || v.State != ipc.TimeToBeatStateFound {
			t.Fatalf("view = %+v, want valid found and not pending", v)
		}
		x.rpcGet(sentinel, true)
		x.waitNote(sentinel) // the queue is FIFO: a queued Hades would have been asked before
		if got := x.hltb.calls(); !slices.Equal(got, []string{"Celeste"}) {
			t.Fatalf("hltb terms = %q, want only Celeste", got)
		}
	})
	t.Run("TTB-008/linha vencida é enfileirada e renovada", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.hltb.gate()
		x.start()
		id := x.game("Hades")
		x.seedFound(id, "Hades", false)
		x.clock.Advance(400 * 24 * time.Hour)
		v := x.rpcGet(id, true)
		if !v.Pending || !v.Stale || v.State != ipc.TimeToBeatStateFound {
			t.Fatalf("view = %+v, want pending, stale, found (the cache)", v)
		}
		x.hltb.openGate()
		x.waitNote(id)
		if got := x.rpcGet(id, false); got.Stale || got.Pending {
			t.Fatalf("after the renewal: %+v, want fresh", got)
		}
	})
	t.Run("TTB-008/em recuo das duas fontes nada é enfileirado e vem unavailable", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id, other := x.game("Hades"), x.game("Celeste")
		x.intoRecuo(id)
		x.start()
		before := len(x.hltb.calls())
		v := x.rpcGet(other, true)
		if !v.Unavailable || v.Pending {
			t.Fatalf("view = %+v, want unavailable and not pending", v)
		}
		if got := len(x.hltb.calls()); got != before {
			t.Fatalf("hltb calls = %d, want %d (no request during the recuo)", got, before)
		}
	})
	t.Run("TTB-008/linha confirmada de não encontrado nunca é pedida de novo", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.start()
		id, sentinel := x.game("Hades"), x.game("Celeste")
		if _, err := x.rpcChoose(id, ""); err != nil {
			t.Fatalf("choose: %v", err)
		}
		x.clock.Advance(400 * 24 * time.Hour)
		if v := x.rpcGet(id, true); v.Pending {
			t.Fatalf("view = %+v, want not pending", v)
		}
		x.rpcGet(sentinel, true)
		x.waitNote(sentinel)
		if got := x.hltb.calls(); !slices.Equal(got, []string{"Celeste"}) {
			t.Fatalf("hltb terms = %q, want only Celeste", got)
		}
	})
}

// ---- TTB-020 ----

func TestTTBIPCRefresh(t *testing.T) {
	t.Parallel()
	t.Run("TTB-020/ignora a validade, grava e notifica", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades")
		x.seedFound(id, "Hades", false) // valid
		v, err := x.rpcRefresh(id)
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if got := x.hltb.calls(); !slices.Equal(got, []string{"Hades"}) {
			t.Fatalf("terms = %q, want [Hades]", got)
		}
		if v.ExternalID != "1" || v.MainSeconds == nil || *v.MainSeconds != 36000 {
			t.Fatalf("view = %+v, want the new values (id 1, 36000 s)", v)
		}
		if got := x.note.all(); !slices.Equal(got, []int64{id}) {
			t.Fatalf("notifications = %v, want [%d]", got, id)
		}
		if again := x.rpcGet(id, false); !reflect.DeepEqual(again, v) {
			t.Fatalf("get = %+v, refresh = %+v, want the same", again, v)
		}
	})
	t.Run("TTB-020/ignora o recuo", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades")
		x.intoRecuo(id)
		x.hltb.setFn(echoFound)
		if _, err := x.rpcRefresh(id); err != nil {
			t.Fatalf("refresh during the recuo: %v", err)
		}
		if row, ok := x.row(id); !ok || row.State != "found" {
			t.Fatalf("row = %+v ok=%v, want found", row, ok)
		}
	})
	t.Run("TTB-020/linha confirmada relê pelo id confirmado", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades")
		x.seedFound(id, "Hades", true)
		x.hltb.setFn(func(context.Context, string) ([]scrapers.HLTBGame, error) {
			return []scrapers.HLTBGame{
				{ID: 1, Name: "Hades", Main: 5, MainExtra: 5, Completionist: 5},
				{ID: 7, Name: "Hades", Main: 123, MainExtra: 456, Completionist: 789},
			}, nil
		})
		v, err := x.rpcRefresh(id)
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if v.ExternalID != "7" || !v.Confirmed || v.MainSeconds == nil || *v.MainSeconds != 123 {
			t.Fatalf("view = %+v, want confirmed 7 with 123 s", v)
		}
		if n := len(x.igdb.searches()); n != 0 {
			t.Fatalf("igdb searches = %d, want 0 (no reserve for a confirmed row)", n)
		}
	})
	t.Run("TTB-020/nada gravado por falha é -32011 com provider hltb", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades")
		x.hltb.setFn(hltbDown)
		_, err := x.rpcRefresh(id)
		wantProviderErr(t, err, ipc.ProviderReasonUnavailable)
		if _, ok := x.row(id); ok {
			t.Fatal("a row was written by a failure")
		}
		if n := len(x.note.all()); n != 0 {
			t.Fatalf("notifications = %d, want 0", n)
		}
	})
	t.Run("TTB-020/jogo inexistente é -32001 e gameId inválido é -32602", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		_, err := x.rpcRefresh(999)
		wantCode(t, err, ipc.CodeGameNotFound)
		_, err = x.call(ipc.MethodTimeToBeatRefresh, `{"gameId":0}`)
		wantCode(t, err, ipc.CodeInvalidParams)
		_, err = x.call(ipc.MethodTimeToBeatRefresh, nil)
		wantCode(t, err, ipc.CodeInvalidParams)
	})
	t.Run("TTB-020/serviço desligado é -32011 com reason disabled e sem requisição", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades")
		if _, err := x.setSettings(`{"enabled":false}`); err != nil {
			t.Fatalf("setSettings: %v", err)
		}
		_, err := x.rpcRefresh(id)
		wantProviderErr(t, err, ipc.ProviderReasonDisabled)
		if n := len(x.hltb.calls()) + len(x.igdb.searches()); n != 0 {
			t.Fatalf("requests = %d, want 0", n)
		}
	})
	t.Run("TTB-020/uma requisição por vez: dois refresh somados", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		a, b := x.game("Hades"), x.game("Celeste")
		x.hltb.gate()
		var wg sync.WaitGroup
		for _, id := range []int64{a, b} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := x.rpcRefresh(id); err != nil {
					t.Errorf("refresh %d: %v", id, err)
				}
			}()
		}
		x.waitEntered()
		x.hltb.openGate()
		wg.Wait()
		if got := x.hltb.maxActive(); got != 1 {
			t.Fatalf("max concurrent hltb calls = %d, want 1", got)
		}
	})
}

// ---- TTB-021 ----

func TestTTBIPCSearch(t *testing.T) {
	t.Parallel()
	many := func(context.Context, string) ([]scrapers.HLTBGame, error) {
		out := make([]scrapers.HLTBGame, 0, 12)
		for i := 1; i <= 12; i++ {
			name := "Hades"
			if i > 3 {
				name = fmt.Sprintf("Hades Variante %d", i)
			}
			out = append(out, scrapers.HLTBGame{ID: int64(i), Name: name, Main: int64(i * 100)})
		}
		return out, nil
	}
	t.Run("TTB-021/term cortado e sem term usa o termo de TTB-003", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades (USA) [!]")
		term := "  Celeste  "
		if _, err := x.rpcSearch(id, &term); err != nil {
			t.Fatalf("search: %v", err)
		}
		if _, err := x.rpcSearch(id, nil); err != nil {
			t.Fatalf("search without term: %v", err)
		}
		if got := x.hltb.calls(); !slices.Equal(got, []string{"Celeste", "Hades"}) {
			t.Fatalf("terms = %q, want [Celeste Hades]", got)
		}
	})
	t.Run("TTB-021/até 10 candidatos em ordem decrescente de pontuação, a linha não muda", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.hltb.setFn(many)
		id := x.game("Hades")
		got, err := x.rpcSearch(id, nil)
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if len(got) != 10 {
			t.Fatalf("candidates = %d, want 10", len(got))
		}
		for i := 1; i < len(got); i++ {
			if got[i].Score > got[i-1].Score {
				t.Fatalf("scores not decreasing at %d: %v then %v", i, got[i-1].Score, got[i].Score)
			}
		}
		if _, ok := x.row(id); ok {
			t.Fatal("search wrote the row")
		}
		if n := len(x.note.all()); n != 0 {
			t.Fatalf("notifications = %d, want 0", n)
		}
	})
	t.Run("TTB-021/term vazio depois do corte ou maior que 200 é -32602 sem requisição; 200 vale", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades")
		for _, term := range []string{"", "   ", strings.Repeat("a", 201)} {
			_, err := x.rpcSearch(id, &term)
			wantCode(t, err, ipc.CodeInvalidParams)
		}
		if n := len(x.hltb.calls()); n != 0 {
			t.Fatalf("hltb calls = %d, want 0", n)
		}
		ok := strings.Repeat("a", 200)
		if _, err := x.rpcSearch(id, &ok); err != nil {
			t.Fatalf("200 characters: %v", err)
		}
	})
	t.Run("TTB-021/falha é -32011, jogo inexistente é -32001, desligado é -32011 disabled", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades")
		x.hltb.setFn(hltbDown)
		_, err := x.rpcSearch(id, nil)
		wantProviderErr(t, err, ipc.ProviderReasonUnavailable)
		_, err = x.rpcSearch(999, nil)
		wantCode(t, err, ipc.CodeGameNotFound)
		if _, err := x.setSettings(`{"enabled":false}`); err != nil {
			t.Fatalf("setSettings: %v", err)
		}
		before := len(x.hltb.calls())
		_, err = x.rpcSearch(id, nil)
		wantProviderErr(t, err, ipc.ProviderReasonDisabled)
		if got := len(x.hltb.calls()); got != before {
			t.Fatalf("hltb called while disabled")
		}
	})
}

// ---- TTB-022 and TTB-040 ----

func TestTTBIPCChoose(t *testing.T) {
	t.Parallel()
	t.Run("TTB-022/escolhe um candidato da última busca: found confirmada, sem requisição, notifica", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.hltb.setFn(hltbTwoExact)
		id := x.game("Hades")
		if _, err := x.rpcSearch(id, nil); err != nil {
			t.Fatalf("search: %v", err)
		}
		calls := len(x.hltb.calls())
		v, err := x.rpcChoose(id, "2")
		if err != nil {
			t.Fatalf("choose: %v", err)
		}
		if v.State != ipc.TimeToBeatStateFound || !v.Confirmed || v.ExternalID != "2" || v.MainSeconds == nil || *v.MainSeconds != 100 {
			t.Fatalf("view = %+v, want found confirmed 2 with 100 s", v)
		}
		if got := len(x.hltb.calls()); got != calls {
			t.Fatalf("choose made a request (%d -> %d calls)", calls, got)
		}
		if got := x.note.all(); !slices.Equal(got, []int64{id}) {
			t.Fatalf("notifications = %v, want [%d]", got, id)
		}
	})
	t.Run("TTB-022/externalId vazio grava not-found confirmada", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades")
		v, err := x.rpcChoose(id, "")
		if err != nil {
			t.Fatalf("choose: %v", err)
		}
		if v.State != ipc.TimeToBeatStateNotFound || !v.Confirmed {
			t.Fatalf("view = %+v, want not-found confirmed", v)
		}
		if n := len(x.hltb.calls()); n != 0 {
			t.Fatalf("hltb calls = %d, want 0", n)
		}
	})
	t.Run("TTB-022/id fora dos candidatos é -32602 e nada muda", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		id := x.game("Hades")
		_, err := x.rpcChoose(id, "42")
		wantCode(t, err, ipc.CodeInvalidParams)
		if _, ok := x.row(id); ok {
			t.Fatal("a row was written")
		}
		if n := len(x.note.all()); n != 0 {
			t.Fatalf("notifications = %d, want 0", n)
		}
	})
	t.Run("TTB-022/depois de reiniciar, só os candidatos da linha em doubt valem", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.hltb.setFn(hltbTwoExact)
		id := x.game("Hades")
		v, err := x.rpcRefresh(id)
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if v.State != ipc.TimeToBeatStateDoubt || len(v.Candidates) == 0 {
			t.Fatalf("view = %+v, want doubt with candidates", v)
		}
		// "restart": a new service over the same store has no search in memory.
		restarted := &ttbIPC{ttbHarness: &ttbHarness{
			t: t, st: x.st, clock: x.clock, hltb: x.hltb, igdb: x.igdb, note: newSpyNotifier(), logs: &ttbLogCapture{},
		}}
		restarted.svc = newTimeToBeatService(x.st, x.hltb, x.igdb, x.clock, slog.New(restarted.logs), restarted.note)
		t.Cleanup(restarted.svc.Close)
		restarted.hs = ttbHandlers(restarted.svc)
		got, err := restarted.rpcChoose(id, "2")
		if err != nil || got.State != ipc.TimeToBeatStateFound || !got.Confirmed || got.ExternalID != "2" {
			t.Fatalf("choose from the doubt row = %+v, %v; want found confirmed 2", got, err)
		}
		other := x.game("Celeste")
		_, err = restarted.rpcChoose(other, "2")
		wantCode(t, err, ipc.CodeInvalidParams)
	})
	t.Run("TTB-022/jogo inexistente é -32001 e parâmetros inválidos são -32602", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		_, err := x.rpcChoose(999, "")
		wantCode(t, err, ipc.CodeGameNotFound)
		for _, p := range []any{nil, `{"gameId":1}`, `{"gameId":0,"externalId":""}`, `{"gameId":1,"externalId":5}`} {
			_, err := x.call(ipc.MethodTimeToBeatChoose, p)
			wantCode(t, err, ipc.CodeInvalidParams)
		}
	})

	reserve := func(t *testing.T) (*ttbIPC, int64) {
		t.Helper()
		x := newTTBIPC(t)
		x.hltb.setFn(hltbEmpty)
		x.igdb.setSearch(igdbMatch("9"))
		x.igdb.setTTB(igdbValues(scrapers.IGDBTimeToBeat{Hastily: 3600, Normally: 7200, Completely: 10800}))
		id := x.game("Hades")
		v, err := x.rpcRefresh(id)
		if err != nil || v.Source != ipc.TimeToBeatSourceIGDB || v.State != ipc.TimeToBeatStateFound || v.Confirmed {
			t.Fatalf("reserve row = %+v, %v; want igdb found, not confirmed", v, err)
		}
		return x, id
	}
	t.Run("TTB-040/linha igdb nunca é confirmed; choose grava found confirmado com fonte hltb e não cai mais na reserva", func(t *testing.T) {
		t.Parallel()
		x, id := reserve(t)
		x.hltb.setFn(hltbTwoExact)
		if _, err := x.rpcSearch(id, nil); err != nil {
			t.Fatalf("search: %v", err)
		}
		v, err := x.rpcChoose(id, "1")
		if err != nil || v.Source != ipc.TimeToBeatSourceHLTB || v.State != ipc.TimeToBeatStateFound || !v.Confirmed {
			t.Fatalf("choose = %+v, %v; want hltb found confirmed", v, err)
		}
		searches := len(x.igdb.searches())
		x.hltb.setFn(hltbEmpty)
		if _, err := x.rpcRefresh(id); err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if got := len(x.igdb.searches()); got != searches {
			t.Fatalf("igdb searched after the manual choice (%d -> %d)", searches, got)
		}
		if row, _ := x.row(id); row.Source != "hltb" || !row.Confirmed {
			t.Fatalf("row = %+v, want hltb confirmed", row)
		}
	})
	t.Run("TTB-040/choose vazio grava not-found confirmado com fonte hltb e não cai mais na reserva", func(t *testing.T) {
		t.Parallel()
		x, id := reserve(t)
		v, err := x.rpcChoose(id, "")
		if err != nil || v.Source != ipc.TimeToBeatSourceHLTB || v.State != ipc.TimeToBeatStateNotFound || !v.Confirmed {
			t.Fatalf("choose = %+v, %v; want hltb not-found confirmed", v, err)
		}
		searches := len(x.igdb.searches())
		if _, err := x.rpcRefresh(id); err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if got := len(x.igdb.searches()); got != searches {
			t.Fatalf("igdb searched after the manual choice (%d -> %d)", searches, got)
		}
	})
}

// ---- TTB-023 ----

func TestTTBIPCSettings(t *testing.T) {
	t.Parallel()
	t.Run("TTB-023/getSettings: ausente vale ligado; lê app_setting", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		if got := x.settings(); !got.Enabled || !got.AfterMetadata {
			t.Fatalf("default = %+v, want both on", got)
		}
		if err := x.st.SetSetting(t.Context(), "timeToBeat.afterMetadata", "0", ttbStart); err != nil {
			t.Fatalf("SetSetting: %v", err)
		}
		if got := x.settings(); !got.Enabled || got.AfterMetadata {
			t.Fatalf("settings = %+v, want enabled and not afterMetadata", got)
		}
	})
	t.Run("TTB-023/setSettings grava só os campos presentes e responde o estado final", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		got, err := x.setSettings(`{"afterMetadata":false}`)
		if err != nil || !got.Enabled || got.AfterMetadata {
			t.Fatalf("setSettings = %+v, %v; want enabled, not afterMetadata", got, err)
		}
		got, err = x.setSettings(`{"enabled":false}`)
		if err != nil || got.Enabled || got.AfterMetadata {
			t.Fatalf("setSettings = %+v, %v; want both off", got, err)
		}
		for key, want := range map[string]string{"timeToBeat.enabled": "0", "timeToBeat.afterMetadata": "0"} {
			if v, err := x.st.Setting(t.Context(), key); err != nil || v != want {
				t.Fatalf("app_setting %s = %q, %v; want %q", key, v, err, want)
			}
		}
		got, err = x.setSettings(`{"enabled":true,"afterMetadata":true}`)
		if err != nil || !got.Enabled || !got.AfterMetadata {
			t.Fatalf("setSettings = %+v, %v; want both on", got, err)
		}
	})
	t.Run("TTB-023/nenhum campo ou tipo errado é -32602", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		for _, p := range []any{nil, `{}`, `{"enabled":"x"}`, `{"enabled":1}`, `{"foo":true}`} {
			_, err := x.setSettings(p)
			wantCode(t, err, ipc.CodeInvalidParams)
		}
	})
	t.Run("TTB-023/enabled false: nenhuma requisição por caminho nenhum, fila esvaziada, get devolve o cache com pending falso", func(t *testing.T) {
		t.Parallel()
		x := newTTBIPC(t)
		x.hltb.gate()
		x.start()
		a, b, c, cached := x.game("Hades"), x.game("Celeste"), x.game("Tunic"), x.game("Inside")
		x.seedFound(cached, "Inside", false)
		x.clock.Advance(400 * 24 * time.Hour)
		x.rpcGet(a, true)
		x.waitEntered() // a is in flight
		if !x.rpcGet(b, true).Pending {
			t.Fatal("b not queued")
		}
		if _, err := x.setSettings(`{"enabled":false}`); err != nil {
			t.Fatalf("setSettings: %v", err)
		}
		if x.svc.pending(b) {
			t.Fatal("queue not emptied")
		}
		if v := x.rpcGet(cached, true); v.Pending || v.State != ipc.TimeToBeatStateFound {
			t.Fatalf("get while disabled = %+v, want the cache and pending false", v)
		}
		if v := x.rpcGet(c, true); v.Pending {
			t.Fatalf("get fetch true while disabled queued the game: %+v", v)
		}
		x.svc.AfterMetadata(c)
		if x.svc.pending(c) {
			t.Fatal("AfterMetadata queued while disabled")
		}
		x.hltb.openGate()
		x.waitNote(a) // the request in flight ends and writes normally
		if _, err := x.setSettings(`{"enabled":true}`); err != nil {
			t.Fatalf("setSettings: %v", err)
		}
		x.rpcGet(c, true)
		x.waitNote(c)
		if got := x.hltb.calls(); !slices.Equal(got, []string{"Hades", "Tunic"}) {
			t.Fatalf("hltb terms = %q, want [Hades Tunic] (Celeste was dropped, Inside never asked)", got)
		}
		if n := len(x.igdb.searches()); n != 0 {
			t.Fatalf("igdb searches = %d, want 0", n)
		}
	})
}

// ---- TTB-025 ----

// ttbForMeta builds a time-to-beat service over the store of a metadata fixture. It is not started: what it
// queues stays queued, and pending tells it.
func ttbForMeta(t *testing.T, st *store.Store, clock *domain.FakeClock) (*timeToBeatService, *ttbLogCapture) {
	t.Helper()
	logs := &ttbLogCapture{}
	svc := newTimeToBeatService(st, newFakeHLTB(), newFakeIGDB(), clock, slog.New(logs), newSpyNotifier())
	t.Cleanup(svc.Close)
	return svc, logs
}

func TestTTBAfterMetadataApply(t *testing.T) {
	t.Parallel()
	newApply := func(t *testing.T, hook bool) (metaFixture, *timeToBeatService, *ttbLogCapture) {
		t.Helper()
		igdb := &metaFetchProv{metaProv: metaProv{id: "igdb", meta: scrapers.Metadata{Description: "Texto do IGDB"}}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		svc, logs := ttbForMeta(t, f.st, f.clock)
		if hook {
			f.meta.ttb = svc
		}
		return f, svc, logs
	}
	params := func(f metaFixture) ipc.MetadataApplyParams {
		return ipc.MetadataApplyParams{GameID: f.gameID, Refs: refsOf("7", ipc.MetadataProviderIGDB)}
	}
	pending := func(t *testing.T, svc *timeToBeatService, id int64) bool {
		t.Helper()
		v, err := svc.Get(t.Context(), id, false)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		return v.Pending
	}

	t.Run("TTB-025/metadata.apply enfileira o lote ignorando a validade; o resultado não muda e nada é notificado", func(t *testing.T) {
		t.Parallel()
		f, svc, _ := newApply(t, true)
		fresh := store.TimeToBeatRow{
			GameID: f.gameID, Source: "hltb", State: "found", ExternalID: "7", ExternalTitle: "Celeste", TitleAtFetch: "Celeste",
			Term: "Celeste", CandidatesJSON: "[]", FetchedAt: f.clock.Now(), ExpiresAt: f.clock.Now().Add(24 * time.Hour),
		}
		if err := f.st.UpsertTimeToBeat(t.Context(), fresh); err != nil {
			t.Fatalf("UpsertTimeToBeat: %v", err)
		}
		got := applyOK(t, f, params(f))
		if !pending(t, svc, f.gameID) {
			t.Fatal("the lookup was not queued after the apply")
		}
		base, _, _ := newApply(t, false)
		want := applyOK(t, base, params(base))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("result with the hook = %+v, without = %+v; must be the same", got, want)
		}
		if n := f.sink.count(); n != 0 {
			t.Errorf("media.changed notices = %d, want 0", n)
		}
	})
	t.Run("TTB-025/cada condição que falta não enfileira e registra Debug", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			setup func(t *testing.T, f metaFixture, svc *timeToBeatService)
		}{
			{"desligado", func(t *testing.T, f metaFixture, svc *timeToBeatService) {
				if _, err := svc.SetSettings(t.Context(), ipc.TimeToBeatSetSettingsParams{Enabled: new(false)}); err != nil {
					t.Fatalf("SetSettings: %v", err)
				}
			}},
			{"depois dos metadados desligado", func(t *testing.T, f metaFixture, svc *timeToBeatService) {
				if _, err := svc.SetSettings(t.Context(), ipc.TimeToBeatSetSettingsParams{AfterMetadata: new(false)}); err != nil {
					t.Fatalf("SetSettings: %v", err)
				}
			}},
			{"linha confirmada", func(t *testing.T, f metaFixture, svc *timeToBeatService) {
				row := store.TimeToBeatRow{
					GameID: f.gameID, Source: "hltb", State: "found", ExternalID: "7", ExternalTitle: "Celeste", TitleAtFetch: "Celeste",
					Term: "Celeste", CandidatesJSON: "[]", Confirmed: true, FetchedAt: f.clock.Now(), ExpiresAt: f.clock.Now().Add(time.Hour),
				}
				if err := f.st.UpsertTimeToBeat(t.Context(), row); err != nil {
					t.Fatalf("UpsertTimeToBeat: %v", err)
				}
			}},
		}
		for _, tc := range cases {
			t.Run("TTB-025/"+tc.name, func(t *testing.T) {
				t.Parallel()
				f, svc, logs := newApply(t, true)
				tc.setup(t, f, svc)
				applyOK(t, f, params(f)) // the apply itself still succeeds
				if pending(t, svc, f.gameID) {
					t.Fatal("queued although a condition is missing")
				}
				if len(logs.level(slog.LevelDebug)) == 0 {
					t.Error("no Debug record naming the missing condition")
				}
			})
		}
	})
	t.Run("TTB-025/metadata.apply que falha não enfileira", func(t *testing.T) {
		t.Parallel()
		f, svc, _ := newApply(t, true)
		if _, err := f.call(t, ipc.MethodMetadataApply, `{"gameId":`+fmt.Sprint(f.gameID)+`,"refs":[]}`); err == nil {
			t.Fatal("apply with no refs succeeded")
		}
		if pending(t, svc, f.gameID) {
			t.Fatal("queued after a failed apply")
		}
	})
}

func TestTTBAfterBatchApply(t *testing.T) {
	t.Parallel()
	t.Run("TTB-025/a aplicação do lote enfileira cada jogo aplicado como lote", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades", "Dead Cells"}, igdb: true})
		svc, _ := ttbForMeta(t, w.st, w.clock)
		w.meta.ttb = svc
		id, ids := w.readyBatch(t)
		batchApplyOK(t, w, id, ids)
		awaitDone(t, w, id)
		for _, gid := range ids {
			v, err := svc.Get(t.Context(), gid, false)
			if err != nil || !v.Pending {
				t.Errorf("game %d: pending = %v, err %v; want queued", gid, v.Pending, err)
			}
		}
	})
	t.Run("TTB-025/com o serviço desligado a aplicação do lote não enfileira", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades"}, igdb: true})
		svc, _ := ttbForMeta(t, w.st, w.clock)
		w.meta.ttb = svc
		if _, err := svc.SetSettings(t.Context(), ipc.TimeToBeatSetSettingsParams{AfterMetadata: new(false)}); err != nil {
			t.Fatalf("SetSettings: %v", err)
		}
		id, ids := w.readyBatch(t)
		batchApplyOK(t, w, id, ids)
		awaitDone(t, w, id)
		for _, gid := range ids {
			if v, _ := svc.Get(t.Context(), gid, false); v.Pending {
				t.Errorf("game %d queued with afterMetadata off", gid)
			}
		}
	})
}

// ---- wiring in the real agent: TTB-008, TTB-026, TTB-037 ----

type ttbWire struct {
	*wiring
	hltb *fakeHLTB
	igdb *fakeIGDB
	keys *lockedKeys
}

func newTTBWire(t *testing.T) *ttbWire {
	t.Helper()
	hltb, igdb := newFakeHLTB(), newFakeIGDB()
	ks := &lockedKeys{m: map[string]string{}}
	w := newWiring(t, wiringOpts{mutate: func(cfg *config) {
		cfg.TimeToBeatClient = hltb
		cfg.TimeToBeatFallback = igdb
		cfg.KeyStore = ks
	}})
	t.Cleanup(hltb.openGate) // runs before the shutdown of newWiring (cleanups are LIFO)
	return &ttbWire{wiring: w, hltb: hltb, igdb: igdb, keys: ks}
}

// changedIDs lists the gameId of every timeToBeat.changed in msgs, checking it has no id (ADR 0031).
func changedIDs(t *testing.T, msgs []rawMsg) []int64 {
	t.Helper()
	var out []int64
	for _, m := range msgs {
		if m.method() != ipc.MethodTimeToBeatChanged {
			continue
		}
		if id, ok := m["id"]; ok {
			t.Errorf("timeToBeat.changed has id %s, want a notification", id)
		}
		var p ipc.TimeToBeatChangedParams
		if err := json.Unmarshal(m["params"], &p); err != nil {
			t.Fatalf("timeToBeat.changed params %s: %v", m["params"], err)
		}
		out = append(out, p.GameID)
	}
	return out
}

func waitChanged(t *testing.T, c *rawClient, id int64) {
	t.Helper()
	c.waitAll(t, fmt.Sprintf("timeToBeat.changed {%d}", id), func(all []rawMsg) bool {
		return slices.Contains(changedIDs(t, all), id)
	})
}

func (w *ttbWire) get(t *testing.T, c *rawClient, id int64, fetch bool) ipc.TimeToBeat {
	t.Helper()
	var v ipc.TimeToBeat
	resultInto(t, c.rpcOK(t, ipc.MethodTimeToBeatGet, ipc.TimeToBeatGetParams{GameID: id, Fetch: fetch}), &v)
	return v
}

func TestTTBWire(t *testing.T) {
	t.Run("TTB-008/dois clientes recebem timeToBeat.changed sem id quando a busca grava; nenhum em falha", func(t *testing.T) {
		w := newTTBWire(t)
		w.hltb.setFn(func(ctx context.Context, term string) ([]scrapers.HLTBGame, error) {
			if term == "Falha" {
				return hltbDown(ctx, term)
			}
			return echoFound(ctx, term)
		})
		c1, c2 := w.client(t), w.client(t)
		ok, _ := w.addGame(t, c1, "Hades")
		bad, _ := w.addGame(t, c1, "Falha")

		v := w.get(t, c1, ok, true)
		if !v.Pending || v.State != ipc.TimeToBeatStateNone {
			t.Fatalf("view = %+v, want pending with state none", v)
		}
		waitChanged(t, c1, ok)
		waitChanged(t, c2, ok)

		var errResp = c1.rpc(t, ipc.MethodTimeToBeatRefresh, ipc.TimeToBeatGameParams{GameID: bad})
		if errResp.Error == nil || errResp.Error.Code != ipc.CodeProviderUnavailable {
			t.Fatalf("refresh of the failing game = %+v, want -32011", errResp)
		}
		c1.rpcOK(t, ipc.MethodStatus, nil)
		c2.rpcOK(t, ipc.MethodStatus, nil)
		for i, c := range []*rawClient{c1, c2} {
			if got := changedIDs(t, c.snapshotAll()); !slices.Equal(got, []int64{ok}) {
				t.Errorf("client %d: changed = %v, want only [%d] (nothing on failure)", i+1, got, ok)
			}
		}
	})
	t.Run("TTB-008/refresh que grava também notifica os dois clientes", func(t *testing.T) {
		w := newTTBWire(t)
		c1, c2 := w.client(t), w.client(t)
		id, _ := w.addGame(t, c1, "Hades")
		var v ipc.TimeToBeat
		resultInto(t, c1.rpcOK(t, ipc.MethodTimeToBeatRefresh, ipc.TimeToBeatGameParams{GameID: id}), &v)
		if v.State != ipc.TimeToBeatStateFound {
			t.Fatalf("refresh = %+v, want found", v)
		}
		waitChanged(t, c1, id)
		waitChanged(t, c2, id)
	})
	t.Run("TTB-023/os seis métodos estão registrados no servidor", func(t *testing.T) {
		w := newTTBWire(t)
		c := w.client(t)
		id, _ := w.addGame(t, c, "Hades")
		var s ipc.TimeToBeatSettings
		resultInto(t, c.rpcOK(t, ipc.MethodTimeToBeatGetSettings, nil), &s)
		if !s.Enabled || !s.AfterMetadata {
			t.Fatalf("settings = %+v, want both on", s)
		}
		resultInto(t, c.rpcOK(t, ipc.MethodTimeToBeatSetSettings, map[string]bool{"afterMetadata": false}), &s)
		if !s.Enabled || s.AfterMetadata {
			t.Fatalf("settings = %+v, want enabled only", s)
		}
		var res ipc.TimeToBeatSearchResult
		resultInto(t, c.rpcOK(t, ipc.MethodTimeToBeatSearch, ipc.TimeToBeatSearchParams{GameID: id}), &res)
		if len(res.Candidates) == 0 {
			t.Fatal("search returned no candidates")
		}
		var chosen ipc.TimeToBeat
		resultInto(t, c.rpcOK(t, ipc.MethodTimeToBeatChoose, ipc.TimeToBeatChooseParams{GameID: id, ExternalID: res.Candidates[0].ExternalID}), &chosen)
		if !chosen.Confirmed {
			t.Fatalf("choose = %+v, want confirmed", chosen)
		}
		if r := c.rpc(t, ipc.MethodTimeToBeatGet, map[string]int{"gameId": 0}); r.Error == nil || r.Error.Code != ipc.CodeInvalidParams {
			t.Fatalf("get with gameId 0 = %+v, want -32602", r)
		}
	})
	t.Run("TTB-026/game.delete apaga a linha, tira o jogo da fila e a busca em curso termina sem gravar nem notificar", func(t *testing.T) {
		w := newTTBWire(t)
		c := w.client(t)
		inflight, _ := w.addGame(t, c, "Hades")
		queued, _ := w.addGame(t, c, "Celeste")
		withRow, _ := w.addGame(t, c, "Tunic")
		sentinel, _ := w.addGame(t, c, "Inside")
		c.rpcOK(t, ipc.MethodTimeToBeatChoose, ipc.TimeToBeatChooseParams{GameID: withRow, ExternalID: ""})

		w.hltb.gate()
		w.get(t, c, inflight, true)
		if term := waitCall(t, w.hltb.entered); term != "Hades" {
			t.Fatalf("in flight term = %q, want Hades", term)
		}
		if !w.get(t, c, queued, true).Pending {
			t.Fatal("second game not queued")
		}
		for _, id := range []int64{queued, inflight, withRow} {
			c.rpcOK(t, ipc.MethodGameDelete, ipc.GameDeleteParams{GameID: id})
		}
		w.hltb.openGate()
		w.get(t, c, sentinel, true)
		waitChanged(t, c, sentinel)

		if got := changedIDs(t, c.snapshotAll()); !slices.Equal(got, []int64{withRow, sentinel}) {
			t.Fatalf("changed = %v, want [%d %d] (the choose and the sentinel only)", got, withRow, sentinel)
		}
		if got := w.hltb.calls(); !slices.Equal(got, []string{"Hades", "Inside"}) {
			t.Fatalf("hltb terms = %q, want [Hades Inside] (the queued game was dropped)", got)
		}
		st := openDB(t, w.cfg)
		defer func() { _ = st.Close() }()
		for _, id := range []int64{inflight, queued, withRow} {
			if _, ok, err := st.GetTimeToBeat(t.Context(), id); err != nil || ok {
				t.Errorf("row of removed game %d: ok=%v err=%v, want none", id, ok, err)
			}
		}
	})
	t.Run("TTB-037/sem chave do IGDB e o hltb em recuo o get devolve unavailable; com a chave não", func(t *testing.T) {
		w := newTTBWire(t)
		w.hltb.setFn(hltbDown)
		c := w.client(t)
		id, _ := w.addGame(t, c, "Hades")
		r := c.rpc(t, ipc.MethodTimeToBeatRefresh, ipc.TimeToBeatGameParams{GameID: id})
		if r.Error == nil || r.Error.Code != ipc.CodeProviderUnavailable {
			t.Fatalf("refresh = %+v, want -32011", r)
		}
		if v := w.get(t, c, id, false); !v.Unavailable {
			t.Fatalf("view = %+v, want unavailable without the IGDB key", v)
		}
		if err := scrapers.NewKeys(w.keys).SetIGDB("cid", "sec"); err != nil {
			t.Fatalf("SetIGDB: %v", err)
		}
		if v := w.get(t, c, id, false); v.Unavailable {
			t.Fatalf("view = %+v, want available once the IGDB key is registered", v)
		}
	})
}

// ---- TTB-010: the limiters of the wiring ----

// ttbNet is a fake IGDB: the Twitch token, the games search and game_time_to_beats, all empty.
func ttbNet(t *testing.T) *http.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"tok","expires_in":3600}`)
	})
	empty := func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `[]`) }
	mux.HandleFunc("/v4/games", empty)
	mux.HandleFunc("/v4/game_time_to_beats", empty)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: provRewrite{target: target, base: srv.Client().Transport}}
}

func TestTTBLimiters(t *testing.T) {
	t.Parallel()
	t.Run("TTB-010/o hltb tem Limiter próprio de 2.000 ms, distinto do IGDB", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(provNow)
		lim := newProviderLimiters(clock)
		if lim.hltb == nil || lim.igdb == nil || lim.hltb == lim.igdb {
			t.Fatalf("limiters = %+v, want a own hltb Limiter", lim)
		}
		if err := lim.hltb.Wait(t.Context()); err != nil {
			t.Fatalf("first Wait: %v", err)
		}
		done := provAsync(t, func() { _ = lim.hltb.Wait(context.Background()) })
		bctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		if err := clock.BlockUntilWaiters(bctx, 1); err != nil {
			t.Fatalf("the second request is not waiting: %v", err)
		}
		clock.Advance(2*time.Second - time.Millisecond)
		select {
		case <-done:
			t.Fatal("second request released before 2.000 ms")
		default:
		}
		clock.Advance(time.Millisecond)
		provWait(t, done, "second request")
	})
	t.Run("TTB-010/a reserva usa o Limiter do IGDB da fase 12: a consulta depois de uma busca de metadados espera 334 ms", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(provNow)
		keys := scrapers.NewKeys(&memKeyStore{values: map[string]string{}})
		if err := keys.SetIGDB("cid", "sec"); err != nil {
			t.Fatalf("SetIGDB: %v", err)
		}
		client := ttbNet(t)
		lim := newProviderLimiters(clock)
		meta := provGet(t, newProviderFactoryWith(keys, client, clock, lim), "igdb")
		fallback := newTTBFallback(keys, client, clock, lim.igdb)

		start := clock.Now()
		done := provAsync(t, func() {
			_, _ = meta.Search(context.Background(), scrapers.Query{Title: "Hades"})
			_, _, _ = fallback.TimeToBeat(context.Background(), "1")
		})
		provDrive(t, clock, done, scrapers.IntervalIGDB)
		if got := clock.Now().Sub(start); got != scrapers.IntervalIGDB {
			t.Fatalf("elapsed = %v, want %v (one shared Limiter)", got, scrapers.IntervalIGDB)
		}
	})
	t.Run("TTB-037/sem chave do IGDB o adaptador devolve ErrNoKey sem requisição", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(provNow)
		keys := scrapers.NewKeys(&memKeyStore{values: map[string]string{}})
		var hits int
		var mu sync.Mutex
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			mu.Lock()
			hits++
			mu.Unlock()
		}))
		t.Cleanup(srv.Close)
		target, _ := url.Parse(srv.URL)
		client := &http.Client{Transport: provRewrite{target: target, base: srv.Client().Transport}}
		fb := newTTBFallback(keys, client, clock, scrapers.NewLimiter(clock, scrapers.IntervalIGDB))
		if _, err := fb.Search(t.Context(), scrapers.Query{Title: "Hades"}); !errors.Is(err, scrapers.ErrNoKey) {
			t.Fatalf("Search error = %v, want ErrNoKey", err)
		}
		if _, _, err := fb.TimeToBeat(t.Context(), "1"); !errors.Is(err, scrapers.ErrNoKey) {
			t.Fatalf("TimeToBeat error = %v, want ErrNoKey", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if hits != 0 {
			t.Fatalf("requests = %d, want 0", hits)
		}
	})
}
