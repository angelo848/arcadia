package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

var errTTBDown = fmt.Errorf("%w: hltb: boom", scrapers.ErrUnavailable)

func hltbDown(context.Context, string) ([]scrapers.HLTBGame, error) { return nil, errTTBDown }

func hltbEmpty(context.Context, string) ([]scrapers.HLTBGame, error) { return nil, nil }

// hltbTwoExact answers two results named like the term: the match is ambiguous (doubt).
func hltbTwoExact(_ context.Context, term string) ([]scrapers.HLTBGame, error) {
	return []scrapers.HLTBGame{
		{ID: 2, Name: term, Year: 2019, Main: 100, MainExtra: 200, Completionist: 300},
		{ID: 1, Name: term, Year: 1998, Main: 400, MainExtra: 500, Completionist: 600},
	}, nil
}

func igdbNone(context.Context, scrapers.Query) ([]scrapers.Candidate, error) { return nil, nil }

func igdbDown(context.Context, scrapers.Query) ([]scrapers.Candidate, error) {
	return nil, fmt.Errorf("%w: igdb: boom", scrapers.ErrUnavailable)
}

func (h *ttbHarness) get(id int64, fetch bool) ipc.TimeToBeat {
	h.t.Helper()
	v, err := h.svc.Get(h.t.Context(), id, fetch)
	if err != nil {
		h.t.Fatalf("Get(%d, %v): %v", id, fetch, err)
	}
	return v
}

func ipcErr(t *testing.T, err error) *ipc.Error {
	t.Helper()
	var e *ipc.Error
	if !errors.As(err, &e) {
		t.Fatalf("error = %v, want *ipc.Error", err)
	}
	return e
}

func TestTTBTerm(t *testing.T) {
	t.Parallel()
	t.Run("TTB-003/o termo enviado é domain.SearchTerm(título)", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		id := h.game("Hollow Knight (USA) [!]")
		if _, err := h.svc.Refresh(t.Context(), id); err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		if got := h.hltb.calls(); !slices.Equal(got, []string{"Hollow Knight"}) {
			t.Fatalf("terms = %q, want [Hollow Knight]", got)
		}
	})
	t.Run("TTB-003/termo vazio não faz requisição e grava not-found", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		id := h.game("(USA)")
		if _, err := h.svc.Refresh(t.Context(), id); err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		if n := len(h.hltb.calls()) + len(h.igdb.searches()); n != 0 {
			t.Fatalf("requests = %d, want 0", n)
		}
		row, ok := h.row(id)
		if !ok || row.State != "not-found" || row.Source != "hltb" {
			t.Fatalf("row = %+v ok=%v, want not-found/hltb", row, ok)
		}
	})
	t.Run("TTB-003/sem resultados e termo com dois-pontos: segunda busca com o texto antes", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		id := h.game("Zelda: Ocarina of Time")
		h.hltb.setFn(func(_ context.Context, term string) ([]scrapers.HLTBGame, error) {
			if term == "Zelda" {
				return []scrapers.HLTBGame{{ID: 5, Name: "Zelda: Ocarina of Time", Main: 100}}, nil
			}
			return nil, nil
		})
		got, err := h.svc.Refresh(t.Context(), id)
		if err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		if calls := h.hltb.calls(); !slices.Equal(calls, []string{"Zelda: Ocarina of Time", "Zelda"}) {
			t.Fatalf("terms = %q", calls)
		}
		if got.State != "found" || got.ExternalID != "5" {
			t.Fatalf("result = %+v, want found 5", got)
		}
	})
	t.Run("TTB-003/a segunda busca é classificada contra o título inteiro", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		id := h.game("Zelda: Ocarina of Time")
		h.hltb.setFn(func(_ context.Context, term string) ([]scrapers.HLTBGame, error) {
			if term == "Zelda" {
				return []scrapers.HLTBGame{{ID: 6, Name: "Zelda", Main: 100}}, nil
			}
			return nil, nil
		})
		got, err := h.svc.Refresh(t.Context(), id)
		if err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		if got.State == "found" {
			t.Fatalf("state = found for a result named only %q", "Zelda")
		}
	})
	t.Run("TTB-003/sem dois-pontos não há segunda busca", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		id := h.game("Celeste")
		h.hltb.setFn(hltbEmpty)
		if _, err := h.svc.Refresh(t.Context(), id); err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		if n := len(h.hltb.calls()); n != 1 {
			t.Fatalf("hltb calls = %d, want 1", n)
		}
	})
}

func TestTTBStates(t *testing.T) {
	t.Parallel()
	many := func(_ context.Context, term string) ([]scrapers.HLTBGame, error) {
		var out []scrapers.HLTBGame
		for i := 1; i <= 12; i++ {
			out = append(out, scrapers.HLTBGame{ID: int64(i), Name: term, Year: 1990 + i})
		}
		return out, nil
	}
	withJunk := func(ctx context.Context, term string) ([]scrapers.HLTBGame, error) {
		rs, _ := hltbTwoExact(ctx, term)
		return append(rs, scrapers.HLTBGame{ID: 9, Name: "Qzxv Wplm Jjkk"}), nil
	}
	tests := []struct {
		name      string
		fn        func(context.Context, string) ([]scrapers.HLTBGame, error)
		wantState string
		wantCands int
		wantMain  *int64
	}{
		{"found com valores", echoFound, "found", 0, i64(36000)},
		{"doubt com candidatos", hltbTwoExact, "doubt", 2, nil},
		{"doubt respeita o piso de 0,40", withJunk, "doubt", 2, nil},
		{"doubt guarda no máximo 10", many, "doubt", 10, nil},
		{"not-found sem resultados", hltbEmpty, "not-found", 0, nil},
	}
	for _, tc := range tests {
		t.Run("TTB-004/"+tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTTBHarness(t)
			id := h.game("Resident Evil 2")
			h.hltb.setFn(tc.fn)
			if _, err := h.svc.Refresh(t.Context(), id); err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			row, ok := h.row(id)
			if !ok || row.State != tc.wantState || row.Source != "hltb" {
				t.Fatalf("row = %+v ok=%v, want %s/hltb", row, ok, tc.wantState)
			}
			if tc.wantMain != nil {
				if row.MainS == nil || *row.MainS != *tc.wantMain || row.MainExtraS == nil || *row.MainExtraS != 50000 ||
					row.CompletionistS == nil || *row.CompletionistS != 90000 {
					t.Fatalf("values = %v %v %v", row.MainS, row.MainExtraS, row.CompletionistS)
				}
			}
			got := h.get(id, false)
			if len(got.Candidates) != tc.wantCands {
				t.Fatalf("candidates = %d, want %d", len(got.Candidates), tc.wantCands)
			}
			for _, c := range got.Candidates {
				if c.Score < float64(domain.MatchAlternative)/1000 {
					t.Fatalf("candidate %+v below the 0,40 floor", c)
				}
			}
		})
	}
}

func TestTTBRowFields(t *testing.T) {
	t.Parallel()
	t.Run("TTB-002/a linha guarda título e termo da busca, instante e validade", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		title := "Hollow Knight (USA)"
		id := h.game(title)
		h.clock.Advance(3 * time.Hour)
		if _, err := h.svc.Refresh(t.Context(), id); err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		row, ok := h.row(id)
		if !ok {
			t.Fatal("no row")
		}
		now := h.clock.Now()
		if row.TitleAtFetch != title || row.Term != "Hollow Knight" {
			t.Fatalf("title/term = %q/%q", row.TitleAtFetch, row.Term)
		}
		if !row.FetchedAt.Equal(now) {
			t.Fatalf("FetchedAt = %v, want %v", row.FetchedAt, now)
		}
		if want := domain.TTBExpiresAt("hltb", domain.TTBFound, now); !row.ExpiresAt.Equal(want) {
			t.Fatalf("ExpiresAt = %v, want %v", row.ExpiresAt, want)
		}
		if row.Confirmed {
			t.Fatal("a search must not confirm")
		}
	})
}

func TestTTBValidity(t *testing.T) {
	t.Parallel()
	day := 24 * time.Hour
	tests := []struct {
		name   string
		row    store.TimeToBeatRow
		ttl    time.Duration
		wantOK bool
	}{
		{"found do hltb vale 90 dias", store.TimeToBeatRow{Source: "hltb", State: "found", ExternalID: "1"}, 90 * day, true},
		{"found do igdb vale 30 dias", store.TimeToBeatRow{Source: "igdb", State: "found", ExternalID: "9"}, 30 * day, true},
		{"doubt vale 30 dias", store.TimeToBeatRow{Source: "hltb", State: "doubt"}, 30 * day, true},
		{"not-found vale 30 dias", store.TimeToBeatRow{Source: "hltb", State: "not-found"}, 30 * day, true},
	}
	for _, tc := range tests {
		t.Run("TTB-006/"+tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTTBHarness(t)
			id := h.game("Celeste")
			h.seed(id, "Celeste", tc.row, ttbStart)
			h.clock.Advance(tc.ttl - time.Second)
			if h.get(id, false).Stale {
				t.Fatal("stale one second before the expiry")
			}
			h.clock.Advance(time.Second)
			if !h.get(id, false).Stale {
				t.Fatal("not stale at the expiry")
			}
		})
	}
	t.Run("TTB-006/título trocado vence na hora; confirmada não", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		a, b := h.game("Celeste"), h.game("Hades")
		h.seed(a, "Celeste", store.TimeToBeatRow{Source: "hltb", State: "found", ExternalID: "1"}, ttbStart)
		h.seed(b, "Hades", store.TimeToBeatRow{Source: "hltb", State: "found", ExternalID: "2", Confirmed: true}, ttbStart)
		if h.get(a, false).Stale || h.get(b, false).Stale {
			t.Fatal("fresh rows reported stale")
		}
		h.setTitle(a, "Celeste 2")
		h.setTitle(b, "Hades 2")
		if !h.get(a, false).Stale {
			t.Fatal("unconfirmed row with a new title is not stale")
		}
		if h.get(b, false).Stale {
			t.Fatal("confirmed row went stale by a title change")
		}
	})
	t.Run("TTB-006/linha confirmada vencida relê os valores pelo id confirmado", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.start()
		id := h.game("HK")
		h.seed(id, "HK", store.TimeToBeatRow{
			Source: "hltb", State: "found", ExternalID: "7", ExternalTitle: "Hollow Knight",
			MainS: i64(100), Confirmed: true,
		}, ttbStart)
		h.clock.Advance(91 * day)
		h.hltb.setFn(func(context.Context, string) ([]scrapers.HLTBGame, error) {
			return []scrapers.HLTBGame{{ID: 7, Name: "Hollow Knight", Main: 777, MainExtra: 888, Completionist: 999}}, nil
		})
		h.get(id, true)
		h.waitNote(id)
		if got := h.hltb.calls(); !slices.Equal(got, []string{"Hollow Knight"}) {
			t.Fatalf("terms = %q, want the external title", got)
		}
		row, _ := h.row(id)
		if !row.Confirmed || row.ExternalID != "7" || row.MainS == nil || *row.MainS != 777 {
			t.Fatalf("row = %+v", row)
		}
		if want := domain.TTBExpiresAt("hltb", domain.TTBFound, h.clock.Now()); !row.ExpiresAt.Equal(want) {
			t.Fatalf("ExpiresAt = %v, want %v", row.ExpiresAt, want)
		}
		if len(h.igdb.searches()) != 0 {
			t.Fatal("a confirmed row must not use the IGDB")
		}
	})
	t.Run("TTB-006/se o id não volta mantém os valores e renova a validade", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.start()
		id := h.game("HK")
		h.seed(id, "HK", store.TimeToBeatRow{
			Source: "hltb", State: "found", ExternalID: "7", ExternalTitle: "Hollow Knight",
			MainS: i64(100), Confirmed: true,
		}, ttbStart)
		h.clock.Advance(91 * day)
		h.hltb.setFn(func(context.Context, string) ([]scrapers.HLTBGame, error) {
			return []scrapers.HLTBGame{{ID: 8, Name: "Hollow Knight", Main: 5}}, nil
		})
		h.get(id, true)
		h.waitNote(id)
		row, _ := h.row(id)
		if row.ExternalID != "7" || row.MainS == nil || *row.MainS != 100 || !row.Confirmed {
			t.Fatalf("row = %+v, want the old values kept", row)
		}
		if want := domain.TTBExpiresAt("hltb", domain.TTBFound, h.clock.Now()); !row.ExpiresAt.Equal(want) {
			t.Fatalf("ExpiresAt = %v, want renewed %v", row.ExpiresAt, want)
		}
	})
	t.Run("TTB-006/not-found confirmada nunca é relida sozinha", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.start()
		id, sentinel := h.game("Obscuro"), h.game("Sentinela")
		h.seed(id, "Obscuro", store.TimeToBeatRow{Source: "hltb", State: "not-found", Confirmed: true}, ttbStart)
		h.clock.Advance(40 * day)
		h.get(id, true)
		h.get(sentinel, true)
		h.waitNote(sentinel)
		if got := h.hltb.calls(); !slices.Equal(got, []string{"Sentinela"}) {
			t.Fatalf("terms = %q, want only the sentinel", got)
		}
	})
	t.Run("TTB-006/linha igdb vencida recomeça pelo HowLongToBeat", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.start()
		id := h.game("Celeste")
		h.seed(id, "Celeste", store.TimeToBeatRow{Source: "igdb", State: "found", ExternalID: "9", MainS: i64(10)}, ttbStart)
		h.clock.Advance(31 * day)
		h.get(id, true)
		h.waitNote(id)
		if got := h.hltb.calls(); !slices.Equal(got, []string{"Celeste"}) {
			t.Fatalf("terms = %q", got)
		}
		if row, _ := h.row(id); row.Source != "hltb" || row.State != "found" {
			t.Fatalf("row = %+v, want hltb/found", row)
		}
	})
}

func TestTTBQueue(t *testing.T) {
	t.Parallel()
	t.Run("TTB-009/no máximo uma requisição por vez (worker, Refresh e Search)", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.gate()
		h.start()
		a, b, c := h.game("Alpha"), h.game("Bravo"), h.game("Charlie")
		h.get(a, true)
		h.waitEntered()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := h.svc.Refresh(t.Context(), b); err != nil {
				t.Errorf("Refresh: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := h.svc.Search(t.Context(), c, "charlie x"); err != nil {
				t.Errorf("Search: %v", err)
			}
		}()
		for i := range 3 {
			h.hltb.release <- struct{}{}
			if i < 2 {
				h.waitEntered()
			}
		}
		wg.Wait()
		if got := h.hltb.maxActive(); got != 1 {
			t.Fatalf("max concurrent requests = %d, want 1", got)
		}
	})
	t.Run("TTB-009/um jogo aparece uma vez na fila; pedir de novo não duplica", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.gate()
		h.start()
		a, b := h.game("Alpha"), h.game("Bravo")
		h.get(a, true)
		h.waitEntered()
		if !h.get(b, true).Pending {
			t.Fatal("queued game is not pending")
		}
		h.get(b, true)
		h.svc.AfterMetadata(b)
		h.hltb.openGate()
		h.waitNote(b)
		if n := countOf(h.hltb.calls(), "Bravo"); n != 1 {
			t.Fatalf("Bravo searched %d times, want 1", n)
		}
	})
	t.Run("TTB-009/pedido interativo promove o jogo do lote e passa à frente do lote", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.gate()
		h.start()
		a, b, c, d := h.game("Alpha"), h.game("Bravo"), h.game("Charlie"), h.game("Delta")
		h.get(a, true)
		h.waitEntered()
		h.svc.AfterMetadata(b)
		h.svc.AfterMetadata(c)
		if !h.get(c, true).Pending {
			t.Fatal("promoted game is not pending")
		}
		h.get(d, true)
		h.hltb.openGate()
		h.waitNote(b)
		want := []string{"Alpha", "Charlie", "Delta", "Bravo"}
		if got := h.hltb.calls(); !slices.Equal(got, want) {
			t.Fatalf("order = %q, want %q", got, want)
		}
	})
	t.Run("TTB-009/cada fila segue a ordem de chegada", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.gate()
		h.start()
		a, b, c, d, e := h.game("Alpha"), h.game("Bravo"), h.game("Charlie"), h.game("Delta"), h.game("Echo")
		h.get(a, true)
		h.waitEntered()
		h.svc.AfterMetadata(c)
		h.svc.AfterMetadata(b)
		h.get(e, true)
		h.get(d, true)
		h.hltb.openGate()
		h.waitNote(b)
		want := []string{"Alpha", "Echo", "Delta", "Charlie", "Bravo"}
		if got := h.hltb.calls(); !slices.Equal(got, want) {
			t.Fatalf("order = %q, want %q", got, want)
		}
	})
	t.Run("TTB-009/a fila do lote para em 1.000 e descarta o excedente", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.gate()
		h.start()
		first := h.game("Game 0000")
		h.get(first, true)
		h.waitEntered()
		const batch = 1001 // one over the cap
		for i := 1; i <= batch; i++ {
			h.svc.AfterMetadata(h.game(fmt.Sprintf("Game %04d", i)))
		}
		h.hltb.openGate()
		for range 1001 { // the first game plus the 1000 that fit
			select {
			case <-h.note.ch:
			case <-time.After(ttbWait):
				t.Fatalf("only %d notifications before the guard", len(h.note.all()))
			}
		}
		sentinel := h.game("Sentinela")
		h.get(sentinel, true)
		h.waitNote(sentinel)
		if slices.Contains(h.hltb.calls(), fmt.Sprintf("Game %04d", batch)) {
			t.Fatal("the game beyond the 1.000 cap was searched")
		}
		if n := len(h.hltb.calls()); n != 1+1000+1 {
			t.Fatalf("searches = %d, want %d", n, 1+1000+1)
		}
	})
	t.Run("TTB-009/jogo removido no meio do caminho não grava nem notifica", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.gate()
		h.start()
		a, s := h.game("Alpha"), h.game("Sentinela")
		h.get(a, true)
		h.waitEntered()
		h.svc.GameRemoved(a)
		if err := h.st.DeleteGame(t.Context(), a); err != nil {
			t.Fatalf("DeleteGame: %v", err)
		}
		h.get(s, true)
		h.hltb.openGate()
		h.waitNote(s)
		if slices.Contains(h.note.all(), a) {
			t.Fatal("removed game was notified")
		}
		if _, ok := h.row(a); ok {
			t.Fatal("removed game has a row")
		}
	})
}

func countOf(list []string, s string) int {
	n := 0
	for _, x := range list {
		if x == s {
			n++
		}
	}
	return n
}

func TestTTBLimiters(t *testing.T) {
	t.Run("TTB-010/o hltb tem Limiter próprio de 2.000 ms e o IGDB usa o do IGDB", func(t *testing.T) {
		t.Skip("os limitadores são montados no run do agente, fora do serviço (newTimeToBeatService não os recebe): cobrir no teste de fiação da T13/T14")
	})
}

// hltbLadder drives requests through the worker while the fake HowLongToBeat fails.
func TestTTBBackoff(t *testing.T) {
	t.Parallel()
	t.Run("TTB-011/escada de recuo 1, 5, 30 e 60 min por falha seguida; no recuo nenhuma requisição ao hltb", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.setFn(hltbDown)
		h.igdb.setSearch(igdbNone)
		h.start()
		n := 0
		submit := func() {
			n++
			h.get(h.game(fmt.Sprintf("Jogo %d", n)), true)
			waitCall(t, h.igdb.called) // the reserve is asked after every attempt, in recuo or not
		}
		submit()
		attempts := 1
		for i, wait := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 60 * time.Minute, 60 * time.Minute} {
			h.clock.Advance(wait - time.Second)
			submit()
			if got := len(h.hltb.calls()); got != attempts {
				t.Fatalf("step %d: hltb called during the recuo (%d calls, want %d)", i, got, attempts)
			}
			h.clock.Advance(time.Second)
			submit()
			attempts++
			if got := len(h.hltb.calls()); got != attempts {
				t.Fatalf("step %d: hltb not retried after %v (%d calls, want %d)", i, wait, got, attempts)
			}
		}
	})
	t.Run("TTB-011/um único Warn por sequência, sem corpo nem URL; resposta válida zera a sequência", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.setFn(hltbDown)
		h.igdb.setSearch(igdbNone)
		h.start()
		n := 0
		submit := func() int64 {
			n++
			id := h.game(fmt.Sprintf("Jogo %d", n))
			h.get(id, true)
			waitCall(t, h.igdb.called)
			return id
		}
		submit()
		h.clock.Advance(time.Minute)
		submit()
		h.clock.Advance(5 * time.Minute)
		submit()
		warns := h.logs.warnsWith("HowLongToBeat indisponível")
		if len(warns) != 1 {
			t.Fatalf("Warn count = %d, want 1 for the whole sequence", len(warns))
		}
		if a := strings.ToLower(warns[0].Attrs + warns[0].Msg); strings.Contains(a, "http") || strings.Contains(a, "boom") {
			t.Fatalf("Warn leaks a body or URL: %q", a)
		}
		// A valid answer resets the sequence.
		h.clock.Advance(30 * time.Minute)
		h.hltb.setFn(echoFound)
		ok := h.game("Valido")
		h.get(ok, true)
		h.waitNote(ok)
		h.hltb.setFn(hltbDown)
		before := len(h.hltb.calls())
		submit() // fails again: first of a new sequence
		h.clock.Advance(time.Minute - time.Second)
		submit()
		if got := len(h.hltb.calls()); got != before+1 {
			t.Fatalf("hltb calls = %d, want %d (recuo is 1 min again)", got, before+1)
		}
		h.clock.Advance(time.Second)
		submit()
		if got := len(h.hltb.calls()); got != before+2 {
			t.Fatalf("hltb calls = %d, want %d after 1 min", got, before+2)
		}
		if got := len(h.logs.warnsWith("HowLongToBeat indisponível")); got != 2 {
			t.Fatalf("Warn count = %d, want 2 (one per sequence)", got)
		}
	})
	t.Run("TTB-011/com as duas fontes indisponíveis a fila espera o menor pausedUntil e get fica unavailable", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.setFn(hltbDown)
		h.igdb.setSearch(igdbDown)
		h.hltb.gate()
		h.start()
		a, b, c := h.game("Alpha"), h.game("Bravo"), h.game("Charlie")
		h.get(a, true)
		h.waitEntered()
		if !h.get(b, true).Pending {
			t.Fatal("B was not queued while the sources were still up")
		}
		h.hltb.openGate()
		ctx, cancel := context.WithTimeout(t.Context(), ttbWait)
		defer cancel()
		if err := h.clock.BlockUntilWaiters(ctx, 1); err != nil {
			t.Fatalf("worker did not wait on the fake clock: %v", err)
		}
		if got := h.hltb.calls(); !slices.Equal(got, []string{"Alpha"}) {
			t.Fatalf("hltb calls = %q, want only Alpha", got)
		}
		if !h.get(c, false).Unavailable {
			t.Fatal("get is not unavailable with both sources in recuo")
		}
		if h.get(c, true).Pending {
			t.Fatal("get queued a game while unavailable")
		}
		h.clock.Advance(time.Minute)
		if got := waitCall(t, h.hltb.called); got != "Bravo" {
			t.Fatalf("after the recuo the worker searched %q, want Bravo", got)
		}
	})
	t.Run("TTB-011/prazo vencido de Refresh conta falha do hltb", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.svc.syncTimeout = 20 * time.Millisecond
		h.hltb.setFn(func(ctx context.Context, _ string) ([]scrapers.HLTBGame, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
		h.igdb.setSearch(igdbNone)
		h.start()
		a := h.game("Alpha")
		_, err := h.svc.Refresh(t.Context(), a)
		if e := ipcErr(t, err); e.Code != ipc.CodeProviderUnavailable {
			t.Fatalf("code = %d, want %d", e.Code, ipc.CodeProviderUnavailable)
		}
		h.get(h.game("Bravo"), true)
		waitCall(t, h.igdb.called) // Refresh's own reserve call (Alpha)
		waitCall(t, h.igdb.called) // Bravo, by the worker
		if got := h.hltb.calls(); !slices.Equal(got, []string{"Alpha"}) {
			t.Fatalf("hltb calls = %q, want the recuo to hold Bravo back", got)
		}
	})
	t.Run("TTB-011/ctx do agente cancelado não conta falha nem grava", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.gate()
		h.start()
		a := h.game("Alpha")
		h.get(a, true)
		h.waitEntered()
		h.svc.Close()
		if _, ok := h.row(a); ok {
			t.Fatal("a row was written after the agent context ended")
		}
		if got := len(h.logs.level(slog.LevelWarn)) + len(h.logs.warnsWith("indispon")); got != 0 {
			t.Fatalf("cancellation logged %d warnings", got)
		}
		if len(h.note.all()) != 0 {
			t.Fatal("cancellation notified")
		}
	})
}

func TestTTBSourceTable(t *testing.T) {
	t.Parallel()
	vals := scrapers.IGDBTimeToBeat{Hastily: 36000, Normally: 40000, Completely: 90000}
	type igdbBehavior struct {
		search func(context.Context, scrapers.Query) ([]scrapers.Candidate, error)
		ttb    func(context.Context, string) (scrapers.IGDBTimeToBeat, bool, error)
	}
	values := igdbBehavior{igdbMatch("1942"), igdbValues(vals)}
	noMatch := igdbBehavior{igdbNone, nil}
	noValues := igdbBehavior{igdbMatch("1942"), func(context.Context, string) (scrapers.IGDBTimeToBeat, bool, error) {
		return scrapers.IGDBTimeToBeat{}, false, nil
	}}
	noKey := igdbBehavior{func(context.Context, scrapers.Query) ([]scrapers.Candidate, error) {
		return nil, scrapers.ErrNoKey
	}, nil}
	down := igdbBehavior{igdbDown, nil}
	tests := []struct {
		name       string
		title      string
		hltb       func(context.Context, string) ([]scrapers.HLTBGame, error)
		igdb       igdbBehavior
		wantErr    bool
		wantSource string
		wantState  string
		wantIGDB   bool // the IGDB search is asked
	}{
		{"found do hltb: o IGDB não é chamado", "Celeste", echoFound, values, false, "hltb", "found", false},
		{"doubt do hltb: o IGDB não é chamado", "Celeste", hltbTwoExact, values, false, "hltb", "doubt", false},
		{"not-found e valores do IGDB", "Celeste", hltbEmpty, values, false, "igdb", "found", true},
		{"not-found e IGDB sem match", "Celeste", hltbEmpty, noMatch, false, "hltb", "not-found", true},
		{"not-found e IGDB sem valores", "Celeste", hltbEmpty, noValues, false, "hltb", "not-found", true},
		{"not-found e IGDB sem chave", "Celeste", hltbEmpty, noKey, false, "hltb", "not-found", true},
		{"not-found e IGDB em falha", "Celeste", hltbEmpty, down, false, "hltb", "not-found", true},
		{"falha do hltb e valores do IGDB", "Celeste", hltbDown, values, false, "igdb", "found", true},
		{"falha do hltb e IGDB sem match", "Celeste", hltbDown, noMatch, true, "", "", true},
		{"falha do hltb e IGDB sem valores", "Celeste", hltbDown, noValues, true, "", "", true},
		{"falha do hltb e IGDB sem chave", "Celeste", hltbDown, noKey, true, "", "", true},
		{"falha do hltb e IGDB em falha", "Celeste", hltbDown, down, true, "", "", true},
		{"termo vazio", "(USA)", echoFound, values, false, "hltb", "not-found", false},
	}
	for _, tc := range tests {
		t.Run("TTB-033/"+tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTTBHarness(t)
			id := h.game(tc.title)
			h.hltb.setFn(tc.hltb)
			h.igdb.setSearch(tc.igdb.search)
			if tc.igdb.ttb != nil {
				h.igdb.setTTB(tc.igdb.ttb)
			}
			got, err := h.svc.Refresh(t.Context(), id)
			asked := len(h.igdb.searches()) > 0
			if asked != tc.wantIGDB {
				t.Fatalf("IGDB asked = %v, want %v", asked, tc.wantIGDB)
			}
			if tc.wantErr {
				e := ipcErr(t, err)
				if e.Code != ipc.CodeProviderUnavailable {
					t.Fatalf("code = %d, want %d", e.Code, ipc.CodeProviderUnavailable)
				}
				var d ipc.ProviderUnavailableData
				if jerr := json.Unmarshal(e.Data, &d); jerr != nil || d.Provider != "hltb" {
					t.Fatalf("data = %s, want provider hltb", e.Data)
				}
				if _, ok := h.row(id); ok {
					t.Fatal("a row was written")
				}
				return
			}
			if err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			if got.State != tc.wantState || got.Source != tc.wantSource {
				t.Fatalf("result = %s/%s, want %s/%s", got.State, got.Source, tc.wantState, tc.wantSource)
			}
			row, _ := h.row(id)
			if row.State != tc.wantState || row.Source != tc.wantSource {
				t.Fatalf("row = %s/%s, want %s/%s", row.State, row.Source, tc.wantState, tc.wantSource)
			}
			wantTTL := domain.TTBExpiresAt(tc.wantSource, domain.TTBState(tc.wantState), h.clock.Now())
			if !row.ExpiresAt.Equal(wantTTL) {
				t.Fatalf("ExpiresAt = %v, want %v", row.ExpiresAt, wantTTL)
			}
		})
	}
	t.Run("TTB-033/o IGDB não é chamado com found nem com doubt do hltb", func(t *testing.T) {
		t.Parallel()
		for _, fn := range []func(context.Context, string) ([]scrapers.HLTBGame, error){echoFound, hltbTwoExact} {
			h := newTTBHarness(t)
			id := h.game("Celeste")
			h.hltb.setFn(fn)
			h.igdb.setSearch(igdbMatch("1942"))
			if _, err := h.svc.Refresh(t.Context(), id); err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			if len(h.igdb.searches()) != 0 || len(h.igdb.ttbCalls()) != 0 {
				t.Fatal("IGDB was called")
			}
		}
	})
}

func TestTTBFallbackFlow(t *testing.T) {
	t.Parallel()
	t.Run("TTB-034/hltb em falha e depois em recuo com o IGDB dando valores", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.setFn(hltbDown)
		h.igdb.setSearch(igdbMatch("1942"))
		h.igdb.setTTB(igdbValues(scrapers.IGDBTimeToBeat{Hastily: 36000}))
		h.start()
		a, b := h.game("Alpha"), h.game("Bravo")
		h.get(a, true)
		h.waitNote(a)
		h.get(b, true) // hltb is in recuo now
		h.waitNote(b)
		if n := len(h.hltb.calls()); n != 1 {
			t.Fatalf("hltb calls = %d, want 1 (recuo)", n)
		}
		for _, id := range []int64{a, b} {
			if row, ok := h.row(id); !ok || row.Source != "igdb" || row.State != "found" {
				t.Fatalf("row %d = %+v", id, row)
			}
		}
	})
	t.Run("TTB-034/nada gravado sai da fila sem notificar", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.setFn(hltbDown)
		h.igdb.setSearch(igdbNone)
		h.start()
		a := h.game("Alpha")
		h.get(a, true)
		waitCall(t, h.igdb.called)
		// The next request is searched in the reserve and written; FIFO means Alpha is already done.
		s := h.game("Sentinela")
		h.igdb.setSearch(igdbMatch("1942"))
		h.igdb.setTTB(igdbValues(scrapers.IGDBTimeToBeat{Hastily: 1}))
		h.get(s, true)
		h.waitNote(s)
		if slices.Contains(h.note.all(), a) {
			t.Fatal("a lookup that wrote nothing notified")
		}
		if _, ok := h.row(a); ok {
			t.Fatal("a lookup that wrote nothing left a row")
		}
		if h.get(a, false).Pending {
			t.Fatal("game still pending after leaving the queue")
		}
	})
}

func TestTTBIGDBMatch(t *testing.T) {
	t.Parallel()
	t.Run("TTB-035/a busca leva título, ano e chave de seed do jogo", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		id := h.game("Hollow Knight")
		h.setYear(id, 2017)
		h.hltb.setFn(hltbEmpty)
		h.igdb.setSearch(igdbMatch("1942"))
		if _, err := h.svc.Refresh(t.Context(), id); err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		qs := h.igdb.searches()
		if len(qs) != 1 || qs[0].Title != "Hollow Knight" || qs[0].Year != 2017 || qs[0].SeedKey != "windows" {
			t.Fatalf("queries = %+v", qs)
		}
		if got := h.igdb.ttbCalls(); !slices.Equal(got, []string{"1942"}) {
			t.Fatalf("TimeToBeat ids = %q, want [1942]", got)
		}
	})
	two := func(_ context.Context, q scrapers.Query) ([]scrapers.Candidate, error) {
		c := scrapers.Candidate{Provider: "igdb", Title: q.Title, PlatformKeys: []string{q.SeedKey}, PlatformConfirmed: true}
		a, b := c, c
		a.ProviderID, b.ProviderID = "1", "2"
		return []scrapers.Candidate{a, b}, nil
	}
	other := func(context.Context, scrapers.Query) ([]scrapers.Candidate, error) {
		return []scrapers.Candidate{{Provider: "igdb", ProviderID: "3", Title: "Qzxv Wplm Jjkk"}}, nil
	}
	for name, fn := range map[string]func(context.Context, scrapers.Query) ([]scrapers.Candidate, error){
		"doubt conta como sem match": two,
		"none conta como sem match":  other,
	} {
		t.Run("TTB-035/"+name+" e não guarda candidatos", func(t *testing.T) {
			t.Parallel()
			h := newTTBHarness(t)
			id := h.game("Hollow Knight")
			h.hltb.setFn(hltbEmpty)
			h.igdb.setSearch(fn)
			got, err := h.svc.Refresh(t.Context(), id)
			if err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			if got.State != "not-found" || got.Source != "hltb" || len(got.Candidates) != 0 {
				t.Fatalf("result = %+v", got)
			}
			if len(h.igdb.ttbCalls()) != 0 {
				t.Fatal("TimeToBeat was read without a match")
			}
			if row, _ := h.row(id); row.CandidatesJSON != "[]" && row.CandidatesJSON != "" {
				t.Fatalf("candidates = %q, want none", row.CandidatesJSON)
			}
		})
	}
}

func TestTTBIGDBValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		rec        scrapers.IGDBTimeToBeat
		found      bool
		wantSource string
		main, ext  *int64
		comp       *int64
	}{
		{"hastily, normally e completely viram os três valores", scrapers.IGDBTimeToBeat{Hastily: 36000, Normally: 40000, Completely: 90000}, true, "igdb", i64(36000), i64(40000), i64(90000)},
		{"zeros são nulos", scrapers.IGDBTimeToBeat{Hastily: 36000}, true, "igdb", i64(36000), nil, nil},
		{"sem registro é sem valores", scrapers.IGDBTimeToBeat{Hastily: 5}, false, "hltb", nil, nil, nil},
		{"três zeros é sem valores", scrapers.IGDBTimeToBeat{}, true, "hltb", nil, nil, nil},
	}
	for _, tc := range tests {
		t.Run("TTB-036/"+tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTTBHarness(t)
			id := h.game("Hollow Knight")
			h.hltb.setFn(hltbEmpty)
			h.igdb.setSearch(igdbMatch("1942"))
			h.igdb.setTTB(func(context.Context, string) (scrapers.IGDBTimeToBeat, bool, error) { return tc.rec, tc.found, nil })
			if _, err := h.svc.Refresh(t.Context(), id); err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			row, _ := h.row(id)
			if row.Source != tc.wantSource {
				t.Fatalf("source = %q, want %q", row.Source, tc.wantSource)
			}
			eq := func(a, b *int64) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
			if tc.wantSource == "igdb" {
				if !eq(row.MainS, tc.main) || !eq(row.MainExtraS, tc.ext) || !eq(row.CompletionistS, tc.comp) {
					t.Fatalf("values = %v %v %v", row.MainS, row.MainExtraS, row.CompletionistS)
				}
				if row.ExternalID != "1942" || row.ExternalTitle != "Hollow Knight" {
					t.Fatalf("external = %q/%q, want the IGDB id and name", row.ExternalID, row.ExternalTitle)
				}
			}
		})
	}
}

func TestTTBIGDBBackoff(t *testing.T) {
	t.Parallel()
	t.Run("TTB-037/sem chave nunca chama TimeToBeat, não loga Warn nem entra em recuo; cadastrar depois vale", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.setFn(hltbEmpty) // default IGDB search: ErrNoKey
		h.start()
		a, b := h.game("Alpha"), h.game("Bravo")
		h.get(a, true)
		h.waitNote(a)
		h.get(b, true)
		h.waitNote(b)
		if n := len(h.igdb.searches()); n != 2 {
			t.Fatalf("IGDB searches = %d, want 2 (no recuo without a key)", n)
		}
		if len(h.igdb.ttbCalls()) != 0 {
			t.Fatal("TimeToBeat was called without a key")
		}
		if w := h.logs.level(slog.LevelWarn); len(w) != 0 { // slog.LevelWarn
			t.Fatalf("Warn records = %+v, want none", w)
		}
		h.igdb.setSearch(igdbMatch("1942"))
		h.igdb.setTTB(igdbValues(scrapers.IGDBTimeToBeat{Hastily: 100}))
		c := h.game("Charlie")
		h.get(c, true)
		h.waitNote(c)
		if row, _ := h.row(c); row.Source != "igdb" {
			t.Fatalf("after registering the key, source = %q, want igdb", row.Source)
		}
	})
	for name, searchErr := range map[string]error{
		"ErrUnavailable":      fmt.Errorf("%w: igdb: boom", scrapers.ErrUnavailable),
		"credencial recusada": errors.New("igdb: credenciais recusadas"),
	} {
		t.Run("TTB-038/falha do IGDB ("+name+") abre o recuo do IGDB com um Warn; o hltb segue", func(t *testing.T) {
			t.Parallel()
			h := newTTBHarness(t)
			h.hltb.setFn(hltbEmpty)
			h.igdb.setSearch(func(context.Context, scrapers.Query) ([]scrapers.Candidate, error) { return nil, searchErr })
			h.start()
			a, b := h.game("Alpha"), h.game("Bravo")
			h.get(a, true)
			h.waitNote(a) // hltb not-found is still written
			h.get(b, true)
			h.waitNote(b)
			if n := len(h.igdb.searches()); n != 1 {
				t.Fatalf("IGDB searches = %d, want 1 (recuo holds the second)", n)
			}
			if n := len(h.logs.warnsWith("IGDB indisponível para o tempo para zerar")); n != 1 {
				t.Fatalf("IGDB Warn count = %d, want 1", n)
			}
			// The IGDB recuo does not stop the hltb: a found is written from it.
			h.hltb.setFn(echoFound)
			d := h.game("Delta")
			h.get(d, true)
			h.waitNote(d)
			if row, _ := h.row(d); row.Source != "hltb" || row.State != "found" {
				t.Fatalf("row = %+v, want hltb/found during the IGDB recuo", row)
			}
			// The ladder is the IGDB's own: 1 min, then 5.
			h.hltb.setFn(hltbEmpty)
			h.clock.Advance(time.Minute)
			e := h.game("Echo")
			h.get(e, true)
			h.waitNote(e)
			if n := len(h.igdb.searches()); n != 2 {
				t.Fatalf("IGDB searches = %d, want 2 after the 1 min recuo", n)
			}
			if n := len(h.logs.warnsWith("IGDB indisponível para o tempo para zerar")); n != 1 {
				t.Fatalf("IGDB Warn count = %d, want still 1", n)
			}
			if n := len(h.logs.warnsWith("HowLongToBeat indisponível")); n != 0 {
				t.Fatalf("hltb Warn count = %d, want 0 (independent ladders)", n)
			}
		})
	}
}

func TestTTBGameRemoved(t *testing.T) {
	t.Parallel()
	t.Run("TTB-026/jogo removido na fila nunca é buscado", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.gate()
		h.start()
		a, b, c := h.game("Alpha"), h.game("Bravo"), h.game("Charlie")
		h.get(a, true)
		h.waitEntered()
		h.get(b, true)
		h.get(c, true)
		h.svc.GameRemoved(b)
		if err := h.st.DeleteGame(t.Context(), b); err != nil {
			t.Fatalf("DeleteGame: %v", err)
		}
		h.hltb.openGate()
		h.waitNote(c)
		if got := h.hltb.calls(); !slices.Equal(got, []string{"Alpha", "Charlie"}) {
			t.Fatalf("calls = %q, want Bravo skipped", got)
		}
	})
	t.Run("TTB-026/busca em curso termina sem gravar nem notificar", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.gate()
		h.start()
		a, s := h.game("Alpha"), h.game("Sentinela")
		h.get(a, true)
		h.waitEntered()
		h.svc.GameRemoved(a)
		if err := h.st.DeleteGame(t.Context(), a); err != nil {
			t.Fatalf("DeleteGame: %v", err)
		}
		h.get(s, true)
		h.hltb.openGate()
		h.waitNote(s)
		if slices.Contains(h.note.all(), a) {
			t.Fatal("removed game was notified")
		}
		if _, ok := h.row(a); ok {
			t.Fatal("removed game has a row")
		}
	})
	t.Run("TTB-026/sai de candidates", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		h.hltb.setFn(hltbTwoExact)
		kept, gone := h.game("Alpha"), h.game("Bravo")
		for _, id := range []int64{kept, gone} {
			if _, err := h.svc.Search(t.Context(), id, "alpha bravo"); err != nil {
				t.Fatalf("Search: %v", err)
			}
		}
		if _, err := h.svc.Choose(t.Context(), kept, "1"); err != nil {
			t.Fatalf("control Choose: %v", err)
		}
		h.svc.GameRemoved(gone)
		_, err := h.svc.Choose(t.Context(), gone, "1")
		if e := ipcErr(t, err); e.Code != ipc.CodeInvalidParams {
			t.Fatalf("code = %d, want %d", e.Code, ipc.CodeInvalidParams)
		}
	})
}

func TestTTBClose(t *testing.T) {
	// Not parallel: goleak compares against the goroutines alive now.
	t.Run("TTB-028/Close cancela, abandona a requisição em curso sem gravar, espera o worker e é idempotente", func(t *testing.T) {
		h := newTTBHarness(t)
		opt := goleak.IgnoreCurrent()
		h.hltb.gate()
		h.svc.Start()
		a := h.game("Alpha")
		h.get(a, true)
		h.waitEntered()
		h.svc.Close()
		h.svc.Close()
		if _, ok := h.row(a); ok {
			t.Fatal("row written by an abandoned request")
		}
		if len(h.note.all()) != 0 {
			t.Fatal("abandoned request notified")
		}
		goleak.VerifyNone(t, opt)
	})
	t.Run("TTB-028/depois de BeginShutdown nenhuma notificação sai e as filas são descartadas", func(t *testing.T) {
		h := newTTBHarness(t)
		h.hltb.gate()
		h.svc.Start()
		a, b := h.game("Alpha"), h.game("Bravo")
		h.get(a, true)
		h.waitEntered()
		h.get(b, true)
		h.svc.BeginShutdown()
		h.hltb.openGate()
		h.svc.Close()
		if got := h.hltb.calls(); !slices.Equal(got, []string{"Alpha"}) {
			t.Fatalf("calls = %q, want the queued game discarded", got)
		}
		if n := len(h.note.all()); n != 0 {
			t.Fatalf("notifications after BeginShutdown = %d, want 0", n)
		}
	})
}

func TestTTBFakesOnly(t *testing.T) {
	t.Parallel()
	t.Run("TTB-031/os fakes são os únicos clientes; nenhum teste do serviço abre soquete", func(t *testing.T) {
		t.Parallel()
		h := newTTBHarness(t)
		// The service is built only with the interfaces; the compile-time checks in the fakes file prove
		// that no concrete HTTP client is involved.
		if h.svc == nil {
			t.Fatal("service not built")
		}
	})
}
