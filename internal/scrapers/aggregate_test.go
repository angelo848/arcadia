package scrapers_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// Casos da T17 (phase-12-metadados-unificados): Aggregator.Search com provedores fake e relógio fake.
// Sem rede e sem time.Sleep. Os time.After que aparecem aqui são só guardas de falha ao esperar um canal.
// Arquivo novo nesta fase: nenhum caso existente foi alterado, por isso não há anotação "alterado em".
//
// Premissas assumidas onde a spec não fecha (ver o relatório da T17):
//   - Known de PlatformInfo = Platform ou SeedKey informados; quando há SeedKey, os testes também informam Platform.
//   - Provedor que o factory não conhece (nome fora dos fakes) responde ErrNoKey: é a forma de "sem chave" usada aqui.
//   - Quando o prazo do agregador cancela o ctx de um provedor lento, o erro devolvido pelo provedor (ctx.Err())
//     conta como falha temporária, independentemente do valor do erro.
//   - Erro que não é ErrUnavailable nem ErrNoKey propaga como erro da busca e não vira falha temporária (design §4.5).
//   - Mensagens de aviso: "<Nome> indisponível no momento", com "IGDB", "SteamGridDB" e "Steam Store".
//   - Erro de Search com ErrNoKey/ErrUnavailable carrega o nome do provedor no texto (errors.Is + strings.Contains).
//   - Contexto já cancelado antes de Search devolve erro que envolve context.Canceled.

var aggNow = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// aggFake answers MetadataProvider. hook, when set, answers in place of cands and err.
type aggFake struct {
	id    string
	cands []scrapers.Candidate
	err   error
	hook  func(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error)

	calls   atomic.Int32
	mu      sync.Mutex
	queries []scrapers.Query
}

func (f *aggFake) ID() string { return f.id }

func (f *aggFake) Search(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.queries = append(f.queries, q)
	f.mu.Unlock()
	if f.hook != nil {
		return f.hook(ctx, q)
	}
	return f.cands, f.err
}

func (f *aggFake) Details(context.Context, string) (scrapers.Metadata, error) {
	return scrapers.Metadata{}, nil
}

func (f *aggFake) Media(context.Context, string) ([]scrapers.RemoteMedia, error) { return nil, nil }

// aggFakes maps a provider name to its fake. A name missing from the map is "sem chave" (ErrNoKey).
type aggFakes map[string]*aggFake

func (fs aggFakes) factory() func(string) (scrapers.MetadataProvider, error) {
	return func(name string) (scrapers.MetadataProvider, error) {
		f, ok := fs[name]
		if !ok {
			return nil, scrapers.ErrNoKey
		}
		return f, nil
	}
}

func aggNewAgg(clock *domain.FakeClock, fs aggFakes) *scrapers.Aggregator {
	return &scrapers.Aggregator{Providers: fs.factory(), Clock: clock}
}

func aggCand(provider, id, title string, year int) scrapers.Candidate {
	return scrapers.Candidate{Provider: provider, ProviderID: id, Title: title, Year: year}
}

// aggResult carries the return values of an asynchronous Search.
type aggResult struct {
	out scrapers.Outcome
	err error
}

func aggSearchAsync(ctx context.Context, a *scrapers.Aggregator, q scrapers.Query, opt scrapers.SearchOptions) <-chan aggResult {
	ch := make(chan aggResult, 1)
	go func() {
		out, err := a.Search(ctx, q, opt)
		ch <- aggResult{out: out, err: err}
	}()
	return ch
}

func aggAwait(t *testing.T, ch <-chan aggResult) aggResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("Search não devolveu")
		return aggResult{}
	}
}

func aggNotYet(t *testing.T, ch <-chan aggResult) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("Search devolveu antes do prazo")
	default:
	}
}

// aggWaitWaiter blocks until the aggregator has registered a timer on the fake clock.
func aggWaitWaiter(t *testing.T, clock *domain.FakeClock) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := clock.BlockUntilWaiters(ctx, 1); err != nil {
		t.Fatalf("o agregador não registrou prazo no relógio fake: %v", err)
	}
}

// aggWaitCancel is a provider that never answers: it returns when the aggregator cancels its context.
func aggWaitCancel(ctx context.Context, _ scrapers.Query) ([]scrapers.Candidate, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestAggregatorSearchParallel(t *testing.T) {
	t.Parallel()

	t.Run("MET-001/todos aplicáveis com chave são consultados em paralelo (um barrier prova a simultaneidade)", func(t *testing.T) {
		t.Parallel()
		const n = 3
		var arrivals atomic.Int32
		all := make(chan struct{})
		barrier := func(ctx context.Context, _ scrapers.Query) ([]scrapers.Candidate, error) {
			if arrivals.Add(1) == n {
				close(all)
			}
			select {
			case <-all:
				return nil, nil
			case <-time.After(5 * time.Second):
				return nil, errors.New("sem simultaneidade: os provedores não chegaram juntos")
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		fakes := aggFakes{
			"igdb":        {id: "igdb", hook: barrier},
			"steamgriddb": {id: "steamgriddb", hook: barrier},
			"steamstore":  {id: "steamstore", hook: barrier},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(out.Asked) != n || len(out.Failed) != 0 {
			t.Fatalf("Asked = %v, Failed = %v; want %d perguntados e nenhuma falha", out.Asked, out.Failed, n)
		}
		for name, f := range fakes {
			if c := f.calls.Load(); c != 1 {
				t.Errorf("%s: chamadas = %d, want 1", name, c)
			}
		}
	})

	t.Run("MET-001/provedor sem chave não é consultado e não conta como falha", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{
			"igdb":       {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}},
			"steamstore": {id: "steamstore"},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if !slices.Equal(out.Missing, []string{"steamgriddb"}) {
			t.Errorf("Missing = %v, want [steamgriddb]", out.Missing)
		}
		if len(out.Failed) != 0 || len(out.Warnings) != 0 {
			t.Errorf("Failed = %v, Warnings = %v; want nenhum (sem chave não é falha)", out.Failed, out.Warnings)
		}
		if slices.Contains(out.Asked, "steamgriddb") {
			t.Errorf("Asked = %v: steamgriddb sem chave foi perguntado", out.Asked)
		}
	})

	t.Run("MET-001/provedor não aplicável (Steam Store em console) não é consultado", func(t *testing.T) {
		t.Parallel()
		steam := &aggFake{id: "steamstore"}
		fakes := aggFakes{
			"igdb":        {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 0)}},
			"steamgriddb": {id: "steamgriddb"},
			"steamstore":  steam,
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		q := scrapers.Query{Title: "Hades", Platform: "PlayStation 2", SeedKey: "ps2", Category: "console"}
		out, err := a.Search(context.Background(), q, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if n := steam.calls.Load(); n != 0 {
			t.Errorf("steamstore chamadas = %d, want 0 (console)", n)
		}
		if slices.Contains(out.Asked, "steamstore") {
			t.Errorf("Asked = %v: steamstore não aplicável foi perguntado", out.Asked)
		}
	})

	t.Run("MET-001/falha temporária de um: lista dos outros e warning IGDB indisponível no momento", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{
			"igdb":        {id: "igdb", err: scrapers.ErrUnavailable},
			"steamgriddb": {id: "steamgriddb", cands: []scrapers.Candidate{aggCand("steamgriddb", "9", "Hades", 0)}},
			"steamstore":  {id: "steamstore", cands: []scrapers.Candidate{aggCand("steamstore", "1145360", "Hades", 0)}},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("Search: %v (uma falha de um provedor não derruba a busca)", err)
		}
		if !slices.Equal(out.Failed, []string{"igdb"}) {
			t.Errorf("Failed = %v, want [igdb]", out.Failed)
		}
		if !slices.Equal(out.Warnings, []string{"IGDB indisponível no momento"}) {
			t.Errorf("Warnings = %q, want exatamente [IGDB indisponível no momento]", out.Warnings)
		}
		if len(out.Groups) != 1 || len(out.Groups[0].Refs) != 2 {
			t.Errorf("Groups = %+v, want 1 grupo com os dois provedores que responderam", out.Groups)
		}
	})

	t.Run("MET-001/prazo de 20 s: provedor lento conta como falha temporária (relógio fake avança)", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(aggNow)
		fakes := aggFakes{
			"igdb":        {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}},
			"steamgriddb": {id: "steamgriddb", hook: aggWaitCancel},
			"steamstore":  {id: "steamstore"},
		}
		a := aggNewAgg(clock, fakes)
		ch := aggSearchAsync(context.Background(), a, scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		aggWaitWaiter(t, clock)

		clock.Advance(19 * time.Second)
		aggNotYet(t, ch)

		clock.Advance(time.Second)
		r := aggAwait(t, ch)
		if r.err != nil {
			t.Fatalf("Search: %v", r.err)
		}
		if !slices.Equal(r.out.Failed, []string{"steamgriddb"}) {
			t.Errorf("Failed = %v, want [steamgriddb] (lento depois de 20 s)", r.out.Failed)
		}
		if len(r.out.Warnings) != 1 {
			t.Errorf("Warnings = %q, want 1 aviso do SteamGridDB", r.out.Warnings)
		}
		if len(r.out.Groups) != 1 {
			t.Errorf("Groups = %d, want 1 (resultado do IGDB)", len(r.out.Groups))
		}
	})

	t.Run("MET-001/Timeout da opção vale 15 s no lote", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(aggNow)
		fakes := aggFakes{
			"igdb":        {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}},
			"steamgriddb": {id: "steamgriddb", hook: aggWaitCancel},
		}
		a := aggNewAgg(clock, fakes)
		ch := aggSearchAsync(context.Background(), a, scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 15 * time.Second, Staged: true})
		aggWaitWaiter(t, clock)

		clock.Advance(14 * time.Second)
		aggNotYet(t, ch)

		clock.Advance(time.Second)
		r := aggAwait(t, ch)
		if r.err != nil {
			t.Fatalf("Search: %v", r.err)
		}
		if !slices.Equal(r.out.Failed, []string{"steamgriddb"}) {
			t.Errorf("Failed = %v, want [steamgriddb] (prazo de 15 s)", r.out.Failed)
		}
	})

	t.Run("MET-001/contexto cancelado devolve erro que envolve context.Canceled", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{
			"igdb": {id: "igdb", hook: aggWaitCancel},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := a.Search(ctx, scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want um erro que envolve context.Canceled", err)
		}
	})

	t.Run("MET-002/erro que não é temporário nem sem chave propaga e não vira falha temporária", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{
			"igdb":       {id: "igdb", err: errors.New("resposta inválida")},
			"steamstore": {id: "steamstore", cands: []scrapers.Candidate{aggCand("steamstore", "1", "Hades", 0)}},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		_, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err == nil {
			t.Fatal("err = nil, want erro interno")
		}
		if errors.Is(err, scrapers.ErrUnavailable) {
			t.Errorf("err = %v: erro interno não pode virar ErrUnavailable", err)
		}
	})
}

func TestAggregatorSearchErrors(t *testing.T) {
	t.Parallel()

	t.Run("MET-002/nenhum provedor aplicável com chave: ErrNoKey com o provedor igdb e zero chamadas", func(t *testing.T) {
		t.Parallel()
		a := aggNewAgg(domain.NewFakeClock(aggNow), aggFakes{})
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if !errors.Is(err, scrapers.ErrNoKey) {
			t.Fatalf("err = %v, want ErrNoKey", err)
		}
		if !strings.Contains(err.Error(), "igdb") {
			t.Errorf("err = %q, want o nome do provedor igdb", err.Error())
		}
		if len(out.Asked) != 0 {
			t.Errorf("Asked = %v, want nenhum", out.Asked)
		}
		if !slices.Contains(out.Missing, "igdb") {
			t.Errorf("Missing = %v, want igdb", out.Missing)
		}
	})

	t.Run("MET-002/só a chave do SteamGridDB falta: ErrNoKey não ocorre se o IGDB tem chave", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{
			"igdb":       {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}},
			"steamstore": {id: "steamstore"},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("err = %v, want nil (IGDB tem chave)", err)
		}
		if !slices.Contains(out.Missing, "steamgriddb") {
			t.Errorf("Missing = %v, want steamgriddb", out.Missing)
		}
	})

	t.Run("MET-002/todos falham: ErrUnavailable com o primeiro que falhou", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{
			"igdb":        {id: "igdb", err: scrapers.ErrUnavailable},
			"steamgriddb": {id: "steamgriddb", err: scrapers.ErrUnavailable},
			"steamstore":  {id: "steamstore", err: scrapers.ErrUnavailable},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if !errors.Is(err, scrapers.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
		if !strings.Contains(err.Error(), "igdb") {
			t.Errorf("err = %q, want o primeiro que falhou (igdb)", err.Error())
		}
		if len(out.Failed) != 3 || out.Failed[0] != "igdb" {
			t.Errorf("Failed = %v, want 3 itens começando por igdb", out.Failed)
		}
	})

	t.Run("MET-002/um respondeu com lista vazia: sucesso", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{
			"igdb":        {id: "igdb"},
			"steamgriddb": {id: "steamgriddb", err: scrapers.ErrUnavailable},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("err = %v, want nil (o IGDB respondeu)", err)
		}
		if len(out.Groups) != 0 || out.Confidence != domain.ConfidenceNone {
			t.Errorf("Groups = %d, Confidence = %q; want lista vazia e none", len(out.Groups), out.Confidence)
		}
		if !slices.Equal(out.Failed, []string{"steamgriddb"}) {
			t.Errorf("Failed = %v, want [steamgriddb]", out.Failed)
		}
	})
}

func TestAggregatorStaged(t *testing.T) {
	t.Parallel()

	t.Run("MET-015/Staged: IGDB acha match, Steam Store não é consultada", func(t *testing.T) {
		t.Parallel()
		steam := &aggFake{id: "steamstore"}
		fakes := aggFakes{
			"igdb":       {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}},
			"steamstore": steam,
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 15 * time.Second, Staged: true})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if n := steam.calls.Load(); n != 0 {
			t.Errorf("steamstore chamadas = %d, want 0 (IGDB já deu match)", n)
		}
		if out.Confidence != domain.ConfidenceMatch {
			t.Errorf("Confidence = %q, want match", out.Confidence)
		}
	})

	t.Run("MET-015/Staged: IGDB acha doubt, Steam Store é consultada uma vez", func(t *testing.T) {
		t.Parallel()
		steam := &aggFake{id: "steamstore"}
		fakes := aggFakes{
			// "Resident Evil 2" contra "Resident Evil": marcas diferentes, trava em 0,79 (doubt).
			"igdb":       {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "2", "Resident Evil 2", 0)}},
			"steamstore": steam,
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		_, err := a.Search(context.Background(), scrapers.Query{Title: "Resident Evil"}, scrapers.SearchOptions{Timeout: 15 * time.Second, Staged: true})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if n := steam.calls.Load(); n != 1 {
			t.Errorf("steamstore chamadas = %d, want 1 (IGDB sem match)", n)
		}
	})

	t.Run("MET-015/Staged sem chave do IGDB: Steam Store consultada", func(t *testing.T) {
		t.Parallel()
		steam := &aggFake{id: "steamstore", cands: []scrapers.Candidate{aggCand("steamstore", "1145360", "Hades", 0)}}
		fakes := aggFakes{"steamstore": steam}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 15 * time.Second, Staged: true})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if n := steam.calls.Load(); n != 1 {
			t.Errorf("steamstore chamadas = %d, want 1 (nenhum outro provedor com chave)", n)
		}
		if !slices.Contains(out.Missing, "igdb") {
			t.Errorf("Missing = %v, want igdb", out.Missing)
		}
	})

	t.Run("MET-015/busca única sempre consulta a Steam Store quando aplicável", func(t *testing.T) {
		t.Parallel()
		steam := &aggFake{id: "steamstore"}
		fakes := aggFakes{
			"igdb":       {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}},
			"steamstore": steam,
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		if _, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second}); err != nil {
			t.Fatalf("Search: %v", err)
		}
		if n := steam.calls.Load(); n != 1 {
			t.Errorf("steamstore chamadas = %d, want 1 (busca única, Staged false)", n)
		}
	})

	t.Run("MET-015/Staged depois: a Steam Store entra no agrupamento", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{
			"igdb": {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "2", "Resident Evil 2", 0)}},
			"steamstore": {id: "steamstore", cands: []scrapers.Candidate{{
				Provider: "steamstore", ProviderID: "304240", Title: "Resident Evil 2",
				PlatformKeys: []string{"windows"}, PlatformConfirmed: true,
			}}},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Resident Evil"}, scrapers.SearchOptions{Timeout: 15 * time.Second, Staged: true})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(out.Groups) != 1 {
			t.Fatalf("Groups = %d, want 1 (a Steam Store junta-se ao IGDB)", len(out.Groups))
		}
		want := scrapers.Ref{Provider: "steamstore", ProviderID: "304240"}
		if !slices.Contains(out.Groups[0].Refs, want) {
			t.Errorf("Refs = %+v, want %+v", out.Groups[0].Refs, want)
		}
	})
}

func TestAggregatorSkipReport(t *testing.T) {
	t.Parallel()

	t.Run("MET-011/Skip evita a pergunta ao provedor suspenso", func(t *testing.T) {
		t.Parallel()
		igdb := &aggFake{id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}}
		fakes := aggFakes{
			"igdb":        igdb,
			"steamgriddb": {id: "steamgriddb"},
			"steamstore":  {id: "steamstore"},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		opt := scrapers.SearchOptions{
			Timeout: 15 * time.Second,
			Skip:    func(provider string) bool { return provider == "igdb" },
		}
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, opt)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if n := igdb.calls.Load(); n != 0 {
			t.Errorf("igdb chamadas = %d, want 0 (suspenso)", n)
		}
		if slices.Contains(out.Asked, "igdb") {
			t.Errorf("Asked = %v: igdb suspenso foi perguntado", out.Asked)
		}
	})

	t.Run("MET-011/Report recebe nil para sucesso e o erro para falha", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{
			"igdb":        {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}},
			"steamgriddb": {id: "steamgriddb", err: scrapers.ErrUnavailable},
			"steamstore":  {id: "steamstore"},
		}
		var mu sync.Mutex
		reports := map[string][]error{}
		opt := scrapers.SearchOptions{
			Timeout: 15 * time.Second,
			Report: func(provider string, err error) {
				mu.Lock()
				defer mu.Unlock()
				reports[provider] = append(reports[provider], err)
			},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, opt)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(reports) != len(out.Asked) {
			t.Errorf("reports = %v, want uma entrada por perguntado (%v)", reports, out.Asked)
		}
		for _, p := range out.Asked {
			if len(reports[p]) != 1 {
				t.Errorf("Report(%s) chamado %d vezes, want 1", p, len(reports[p]))
			}
		}
		if got := reports["igdb"]; len(got) == 1 && got[0] != nil {
			t.Errorf("Report(igdb) = %v, want nil (sucesso)", got[0])
		}
		if got := reports["steamgriddb"]; len(got) == 1 && !errors.Is(got[0], scrapers.ErrUnavailable) {
			t.Errorf("Report(steamgriddb) = %v, want ErrUnavailable", got[0])
		}
	})

	t.Run("MET-011/Skip e Report nil: a busca não muda", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{
			"igdb":       {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}},
			"steamstore": {id: "steamstore"},
		}
		a := aggNewAgg(domain.NewFakeClock(aggNow), fakes)
		out, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if !slices.Contains(out.Asked, "igdb") || !slices.Contains(out.Asked, "steamstore") {
			t.Errorf("Asked = %v, want igdb e steamstore", out.Asked)
		}
	})
}

func TestAggregatorTerm(t *testing.T) {
	t.Parallel()

	t.Run("MET-004/o fake recebe o termo exatamente como veio em Query.Title", func(t *testing.T) {
		t.Parallel()
		// O termo já vem processado pelo chamador (domain.SearchTerm); o agregador não o altera.
		const title = "Zelda - Majora's Mask (USA)"
		igdb := &aggFake{id: "igdb"}
		a := aggNewAgg(domain.NewFakeClock(aggNow), aggFakes{"igdb": igdb})
		if _, err := a.Search(context.Background(), scrapers.Query{Title: title}, scrapers.SearchOptions{Timeout: 20 * time.Second}); err != nil {
			t.Fatalf("Search: %v", err)
		}
		igdb.mu.Lock()
		defer igdb.mu.Unlock()
		if len(igdb.queries) != 1 || igdb.queries[0].Title != title {
			t.Fatalf("termo recebido = %+v, want Title %q", igdb.queries, title)
		}
	})
}

func TestAggregatorConfidenceAndLimit(t *testing.T) {
	t.Parallel()

	t.Run("MET-009/confiança calculada pela lista ordenada: match", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{"igdb": {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}}}
		out, err := aggNewAgg(domain.NewFakeClock(aggNow), fakes).Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if out.Confidence != domain.ConfidenceMatch {
			t.Errorf("Confidence = %q, want match", out.Confidence)
		}
	})

	t.Run("MET-009/confiança calculada pela lista ordenada: doubt por trava de marcas", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{"igdb": {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "2", "Resident Evil 2", 0)}}}
		out, err := aggNewAgg(domain.NewFakeClock(aggNow), fakes).Search(context.Background(), scrapers.Query{Title: "Resident Evil"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if out.Confidence != domain.ConfidenceDoubt {
			t.Errorf("Confidence = %q, want doubt", out.Confidence)
		}
	})

	t.Run("MET-009/confiança calculada pela lista ordenada: none sem resultados", func(t *testing.T) {
		t.Parallel()
		fakes := aggFakes{"igdb": {id: "igdb"}}
		out, err := aggNewAgg(domain.NewFakeClock(aggNow), fakes).Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if out.Confidence != domain.ConfidenceNone {
			t.Errorf("Confidence = %q, want none", out.Confidence)
		}
	})

	t.Run("MET-008/lista limitada a 10", func(t *testing.T) {
		t.Parallel()
		cands := []scrapers.Candidate{aggCand("igdb", "0", "Doom", 0)}
		for i := 2; i <= 12; i++ {
			title := fmt.Sprintf("Doom %d", i)
			cands = append(cands, aggCand("igdb", title, title, 0))
		}
		fakes := aggFakes{"igdb": {id: "igdb", cands: cands}}
		out, err := aggNewAgg(domain.NewFakeClock(aggNow), fakes).Search(context.Background(), scrapers.Query{Title: "Doom"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(out.Groups) != 10 {
			t.Fatalf("Groups = %d, want 10", len(out.Groups))
		}
		if out.Groups[0].Title != "Doom" {
			t.Errorf("primeiro = %q, want Doom (melhor pontuação)", out.Groups[0].Title)
		}
	})
}

// TestAggregatorLeavesNoGoroutine is sequential on purpose: goleak must not see goroutines of parallel siblings.
func TestAggregatorLeavesNoGoroutine(t *testing.T) {
	t.Run("MET-001/nenhuma goroutine fica depois de Search (sucesso, falha e cancelamento)", func(t *testing.T) {
		defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

		clock := domain.NewFakeClock(aggNow)
		fakes := aggFakes{
			"igdb":        {id: "igdb", cands: []scrapers.Candidate{aggCand("igdb", "1", "Hades", 2020)}},
			"steamgriddb": {id: "steamgriddb", err: scrapers.ErrUnavailable},
			"steamstore":  {id: "steamstore", cands: []scrapers.Candidate{aggCand("steamstore", "1", "Hades", 0)}},
		}
		a := aggNewAgg(clock, fakes)
		if _, err := a.Search(context.Background(), scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second}); err != nil {
			t.Fatalf("Search: %v", err)
		}

		// Relógio separado: o prazo do primeiro Search continua pendente no relógio dele (domain.Clock não cancela timers).
		slowClock := domain.NewFakeClock(aggNow)
		slow := aggFakes{"igdb": {id: "igdb", hook: aggWaitCancel}}
		b := aggNewAgg(slowClock, slow)
		ch := aggSearchAsync(context.Background(), b, scrapers.Query{Title: "Hades"}, scrapers.SearchOptions{Timeout: 20 * time.Second})
		aggWaitWaiter(t, slowClock)
		slowClock.Advance(20 * time.Second)
		// Único provedor perguntado expirou: é ErrUnavailable (todos os perguntados falharam).
		if r := aggAwait(t, ch); !errors.Is(r.err, scrapers.ErrUnavailable) {
			t.Fatalf("Search lento: err = %v, want ErrUnavailable", r.err)
		}
	})
}
