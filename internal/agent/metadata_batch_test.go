package agent

// Tests of T25 (phase 12): the batch job of metadata.batch* (start, coordinator and workers, states of
// contracts/batch-state-machine.md, cancellation, discard, notifications, shutdown). The clock is a
// domain.FakeClock; IGDB is batchGate (a fake that can hold, fail or limit its calls); the store is real,
// in t.TempDir(). There is no time.Sleep: asynchronous events are awaited on channels with testTimeout,
// and goroutine leaks are checked with goleak. Shared helpers for T24 live in metadata_batch_search_test.go.
// Alterado em 2026-11-15: arquivo novo (fase 12, T25).

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// batchNone is the state the tables use for "no batch" (a fresh agent, or a batch discarded).
const batchNone = "none"

// batchIGDBInterval is the minimum spacing of IGDB requests [MET-010].
const batchIGDBInterval = 334 * time.Millisecond

// batchWarnIGDB is the warning the job adds once when IGDB is disabled for the rest of a batch [MET-011].
const batchWarnIGDB = "IGDB indisponível: a busca continuou sem ele"

// batchGate is the IGDB of the batch tests. Search answers the query with one candidate of the same
// title, platform and year (a match when the game has a platform key). It can hold the calls until
// release, answer some titles at once, fail with failWith, or wait for lim first (the request spacing
// of MET-010). Every Search entry is counted with the number of calls in flight at that moment.
type batchGate struct {
	metaProv

	blockSearch bool
	blockFetch  bool
	free        map[string]bool   // titles answered at once even when blockSearch is set (read-only after start)
	failWith    error             // when set, every Search fails with it (read-only after start)
	lim         *scrapers.Limiter // when set, every Search waits for it first (read-only after start)
	clock       domain.Clock      // when set, the instant of each Search entry is kept (read-only after start)

	open    chan struct{} // closed by release: unblocks the held calls
	once    sync.Once     // makes release idempotent
	entered chan string   // one value per Search entry, the query title (buffered)
	fetched chan struct{} // one value per Fetch entry (buffered)

	gmu         sync.Mutex
	searches    int
	inflight    int
	maxInflight int
	times       []time.Time
}

func newBatchGate(blockSearch, blockFetch bool) *batchGate {
	return &batchGate{
		metaProv:    metaProv{id: "igdb"},
		blockSearch: blockSearch,
		blockFetch:  blockFetch,
		open:        make(chan struct{}),
		entered:     make(chan string, 64),
		fetched:     make(chan struct{}, 64),
	}
}

// release lets every held call go on. Calls made after it are not held.
func (g *batchGate) release() { g.once.Do(func() { close(g.open) }) }

// enter records a Search entry: the query, the in-flight count and, when a clock is set, the instant.
func (g *batchGate) enter(q scrapers.Query) {
	g.metaProv.record("Search")
	g.metaProv.mu.Lock()
	g.metaProv.queries = append(g.metaProv.queries, q)
	g.metaProv.mu.Unlock()

	g.gmu.Lock()
	defer g.gmu.Unlock()
	g.searches++
	g.inflight++
	g.maxInflight = max(g.maxInflight, g.inflight)
	if g.clock != nil {
		g.times = append(g.times, g.clock.Now())
	}
}

func (g *batchGate) leave() {
	g.gmu.Lock()
	defer g.gmu.Unlock()
	g.inflight--
}

// stats returns the number of Search entries so far and the largest number in flight at once.
func (g *batchGate) stats() (searches, maxInflight int) {
	g.gmu.Lock()
	defer g.gmu.Unlock()
	return g.searches, g.maxInflight
}

// entryTimes returns the instant of every Search entry, in entry order.
func (g *batchGate) entryTimes() []time.Time {
	g.gmu.Lock()
	defer g.gmu.Unlock()
	return slices.Clone(g.times)
}

// queriesOf returns the queries of the Search calls, in order.
func (g *batchGate) queriesOf() []scrapers.Query {
	g.metaProv.mu.Lock()
	defer g.metaProv.mu.Unlock()
	return slices.Clone(g.metaProv.queries)
}

// batchEcho is the candidate the gate answers for a query: the same title, year, platform and key.
func batchEcho(q scrapers.Query) scrapers.Candidate {
	var keys []string
	if q.SeedKey != "" {
		keys = append(keys, q.SeedKey)
	}
	return metaCand("igdb", "100", q.Title, q.Year, q.Platform, keys...)
}

func (g *batchGate) Search(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error) {
	if g.lim != nil {
		if err := g.lim.Wait(ctx); err != nil {
			return nil, err
		}
	}
	g.enter(q)
	defer g.leave()
	select {
	case g.entered <- q.Title:
	default:
	}
	if g.failWith != nil {
		return nil, g.failWith
	}
	if g.blockSearch && !g.free[q.Title] {
		select {
		case <-g.open:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return []scrapers.Candidate{batchEcho(q)}, nil
}

// Fetch answers the text of an apply. It is held while blockFetch is set, so a test can see the
// application in progress.
func (g *batchGate) Fetch(ctx context.Context, providerID string) (scrapers.Metadata, []scrapers.RemoteMedia, error) {
	g.metaProv.record("Fetch")
	g.metaProv.noteID(providerID)
	select {
	case g.fetched <- struct{}{}:
	default:
	}
	if g.blockFetch {
		select {
		case <-g.open:
		case <-ctx.Done():
			return scrapers.Metadata{}, nil, ctx.Err()
		}
	}
	return scrapers.Metadata{Description: "Texto da busca"}, nil, nil
}

// batchSpy receives the notifications of a batch (batchSink) and keeps them in order. signal is poked
// on every notification, so a test can wait for one without polling.
type batchSpy struct {
	mu     sync.Mutex
	events []ipc.MetadataBatchChangedParams
	signal chan struct{}
}

func newBatchSpy() *batchSpy { return &batchSpy{signal: make(chan struct{}, 1)} }

func (s *batchSpy) BatchChanged(p ipc.MetadataBatchChangedParams) {
	s.mu.Lock()
	s.events = append(s.events, p)
	s.mu.Unlock()
	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// all returns the notifications received so far.
func (s *batchSpy) all() []ipc.MetadataBatchChangedParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.events)
}

// hasBatchNotice reports whether a notification of batch id with state has arrived.
func hasBatchNotice(s *batchSpy, id int64, state string) bool {
	for _, e := range s.all() {
		if e.BatchID == id && e.State == state {
			return true
		}
	}
	return false
}

// awaitBatchNotice waits until a notification matches, and returns it.
func awaitBatchNotice(t *testing.T, s *batchSpy, what string, match func(ipc.MetadataBatchChangedParams) bool) ipc.MetadataBatchChangedParams {
	t.Helper()
	deadline := time.After(testTimeout)
	for {
		for _, e := range s.all() {
			if match(e) {
				return e
			}
		}
		select {
		case <-s.signal:
		case <-deadline:
			t.Fatalf("no notification %s; got %+v", what, s.all())
		}
	}
}

// awaitBatchState waits for the notification of batch id with state.
func awaitBatchState(t *testing.T, s *batchSpy, id int64, state string) ipc.MetadataBatchChangedParams {
	t.Helper()
	return awaitBatchNotice(t, s, fmt.Sprintf("of batch %d in state %q", id, state), func(e ipc.MetadataBatchChangedParams) bool {
		return e.BatchID == id && e.State == state
	})
}

// awaitBatchDone waits for a notification of batch id that shows done >= n.
func awaitBatchDone(t *testing.T, s *batchSpy, id int64, n int) {
	t.Helper()
	awaitBatchNotice(t, s, fmt.Sprintf("of batch %d with done >= %d", id, n), func(e ipc.MetadataBatchChangedParams) bool {
		return e.BatchID == id && e.Done >= n
	})
}

// awaitEntered waits until n Search calls have entered the gate, and returns their query titles.
func awaitEntered(t *testing.T, g *batchGate, n int) []string {
	t.Helper()
	titles := make([]string, 0, n)
	deadline := time.After(testTimeout)
	for len(titles) < n {
		select {
		case title := <-g.entered:
			titles = append(titles, title)
		case <-deadline:
			t.Fatalf("only %d of %d searches started", len(titles), n)
		}
	}
	return titles
}

// awaitFetch waits until the gate has been asked to fetch, so the application is in progress.
func awaitFetch(t *testing.T, g *batchGate) {
	t.Helper()
	select {
	case <-g.fetched:
	case <-time.After(testTimeout):
		t.Fatal("the application did not start")
	}
}

// waitReturns runs wait and fails the test if it does not return in testTimeout.
func waitReturns(t *testing.T, what string, wait func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		wait()
	}()
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatalf("%s did not return", what)
	}
}

// batchFixture is metaFixture with IGDB as the gate and the batch notifications on a spy.
type batchFixture struct {
	metaFixture
	gate *batchGate
	spy  *batchSpy
}

// newBatchFixture builds the fixture: the game "Celeste" (Windows) and IGDB as the only provider with a
// key, unless noKey names it. blockSearch and blockFetch say whether the gate holds the calls.
func newBatchFixture(t *testing.T, blockSearch, blockFetch bool, noKey ...string) batchFixture {
	t.Helper()
	gate := newBatchGate(blockSearch, blockFetch)
	f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": gate}, noKey...)
	gate.clock = f.clock
	spy := newBatchSpy()
	f.meta.notify = spy
	t.Cleanup(gate.release)
	return batchFixture{metaFixture: f, gate: gate, spy: spy}
}

// addGames inserts one game per title (executable C:\batch\<i>.exe) and returns the ids in order.
func addGames(t *testing.T, f metaFixture, titles ...string) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(titles))
	for i, title := range titles {
		g, err := f.st.AddGame(t.Context(), store.NewGame{
			Title: title, Exe: fmt.Sprintf(`C:\batch\%d.exe`, i), At: f.clock.Now(),
		})
		if err != nil {
			t.Fatalf("AddGame(%q): %v", title, err)
		}
		ids = append(ids, g.ID)
	}
	return ids
}

// batchGames returns the ids of a batch: the fixture's game first, then one game per title.
func batchGames(t *testing.T, f batchFixture, titles ...string) []int64 {
	t.Helper()
	return append([]int64{f.gameID}, addGames(t, f.metaFixture, titles...)...)
}

// batchJSON is the params of a batch method as JSON.
func batchJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// choicesOf picks the first candidate of each game.
func choicesOf(ids []int64) []ipc.BatchChoice {
	out := make([]ipc.BatchChoice, 0, len(ids))
	for _, id := range ids {
		out = append(out, ipc.BatchChoice{GameID: id, Candidate: 0})
	}
	return out
}

// startBatchOK runs metadata.batchStart and fails the test when it is not a success.
func startBatchOK(t *testing.T, f batchFixture, ids ...int64) ipc.BatchStartResult {
	t.Helper()
	out, err := f.call(t, ipc.MethodMetadataBatchStart, batchJSON(t, ipc.BatchStartParams{GameIDs: ids}))
	if err != nil {
		t.Fatalf("metadata.batchStart: %v", err)
	}
	res, ok := out.(ipc.BatchStartResult)
	if !ok {
		t.Fatalf("result type %T, want ipc.BatchStartResult", out)
	}
	return res
}

// batchStatusNow is the batch of metadata.batchStatus; nil when there is none.
func batchStatusNow(t *testing.T, f metaFixture) *ipc.BatchStatus {
	t.Helper()
	out, err := f.call(t, ipc.MethodMetadataBatchStatus, "{}")
	if err != nil {
		t.Fatalf("metadata.batchStatus: %v", err)
	}
	res, ok := out.(ipc.BatchStatusResult)
	if !ok {
		t.Fatalf("result type %T, want ipc.BatchStatusResult", out)
	}
	return res.Batch
}

// wantBatchState checks the state metadata.batchStatus reports (batchNone = null).
func wantBatchState(t *testing.T, f metaFixture, want string) {
	t.Helper()
	b := batchStatusNow(t, f)
	switch {
	case want == batchNone && b != nil:
		t.Errorf("batch = %+v, want none", *b)
	case want != batchNone && (b == nil || b.State != want):
		t.Errorf("batch = %+v, want state %q", b, want)
	}
}

// batchResultsNow returns the first page of metadata.batchResults of batch id.
func batchResultsNow(t *testing.T, f metaFixture, id int64) ipc.BatchResultsResult {
	t.Helper()
	out, err := f.call(t, ipc.MethodMetadataBatchResults, batchJSON(t, ipc.BatchResultsParams{BatchID: id}))
	if err != nil {
		t.Fatalf("metadata.batchResults: %v", err)
	}
	res, ok := out.(ipc.BatchResultsResult)
	if !ok {
		t.Fatalf("result type %T, want ipc.BatchResultsResult", out)
	}
	return res
}

// cancelBatch runs metadata.batchCancel and returns the state it reports.
func cancelBatch(t *testing.T, f metaFixture, id int64) string {
	t.Helper()
	out, err := f.call(t, ipc.MethodMetadataBatchCancel, batchJSON(t, ipc.BatchIDParams{BatchID: id}))
	if err != nil {
		t.Fatalf("metadata.batchCancel: %v", err)
	}
	res, ok := out.(ipc.BatchCancelResult)
	if !ok {
		t.Fatalf("result type %T, want ipc.BatchCancelResult", out)
	}
	return res.State
}

// applyBatch sends metadata.batchApply with the rows of the batch, and fails on an error.
func applyBatch(t *testing.T, c batchCase) {
	t.Helper()
	if _, err := c.f.call(t, ipc.MethodMetadataBatchApply, batchJSON(t, ipc.BatchApplyParams{BatchID: c.id, Choices: choicesOf(c.rowIDs)})); err != nil {
		t.Fatalf("metadata.batchApply: %v", err)
	}
}

// gameDetails reads the games of ids from the store.
func gameDetails(t *testing.T, f batchFixture, ids []int64) []store.GameDetail {
	t.Helper()
	out := make([]store.GameDetail, 0, len(ids))
	for _, id := range ids {
		out = append(out, gameText(t, f.st, id))
	}
	return out
}

// batchCase is a batch brought to one state. ids are the games of the batch, rowIDs the ones that have
// a row, id its batchId (1 when there is no batch: an id that was never issued).
type batchCase struct {
	f      batchFixture
	ids    []int64
	rowIDs []int64
	id     int64
}

// reachBatch brings a fresh fixture to state: "none", searching, ready, cancelled, applying or done.
// The batch has three games: the fixture's "Celeste" and "Hades", "Dead Cells".
func reachBatch(t *testing.T, state string) batchCase {
	t.Helper()
	blockSearch := state == batchNone || state == ipc.BatchStateSearching || state == ipc.BatchStateCancelled
	f := newBatchFixture(t, blockSearch, state != ipc.BatchStateDone)
	c := batchCase{f: f, ids: batchGames(t, f, "Hades", "Dead Cells")}
	if state == batchNone {
		c.id = 1
		c.rowIDs = c.ids
		return c
	}
	if state == ipc.BatchStateCancelled {
		f.gate.free = map[string]bool{"Celeste": true} // one game finishes before the cancellation
	}
	c.id = startBatchOK(t, f, c.ids...).BatchID
	switch state {
	case ipc.BatchStateCancelled:
		awaitBatchDone(t, f.spy, c.id, 1)
		c.rowIDs = []int64{f.gameID}
		cancelBatch(t, f.metaFixture, c.id)
	case ipc.BatchStateReady:
		awaitBatchState(t, f.spy, c.id, ipc.BatchStateReady)
		c.rowIDs = c.ids
	case ipc.BatchStateApplying:
		awaitBatchState(t, f.spy, c.id, ipc.BatchStateReady)
		// A subset: total in applying is the number of choices sent, not the number of games (§2).
		c.rowIDs = c.ids[:2]
		applyBatch(t, c)
		awaitFetch(t, f.gate)
	case ipc.BatchStateDone:
		awaitBatchState(t, f.spy, c.id, ipc.BatchStateReady)
		c.rowIDs = c.ids
		applyBatch(t, c)
		awaitBatchState(t, f.spy, c.id, ipc.BatchStateDone)
	}
	return c
}

// TestBatchJob checks the batch job: start, the workers, notifications, status, cancellation, discard
// and shutdown [MET-052..MET-059].
func TestBatchJob(t *testing.T) {
	t.Run("MET-052/batchStart devolve batchId e total antes de qualquer requisição (provedor bloqueado)", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, true, false)
		// The gate holds every search: the answer comes back while no request has finished.
		res := startBatchOK(t, f, batchGames(t, f, "Hades")...)
		if res.BatchID < 1 || res.Total != 2 {
			t.Errorf("batchStart = %+v, want batchId >= 1 and total 2", res)
		}
	})

	t.Run("MET-052/gameId inexistente: -32001 e nada começa", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		_, err := f.call(t, ipc.MethodMetadataBatchStart, batchJSON(t, ipc.BatchStartParams{GameIDs: []int64{f.gameID, 999}}))
		if codeOf(err) != ipc.CodeGameNotFound {
			t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeGameNotFound)
		}
		if b := batchStatusNow(t, f.metaFixture); b != nil {
			t.Errorf("batch = %+v after a refused start, want none", *b)
		}
		if n, _ := f.gate.stats(); n != 0 {
			t.Errorf("searches = %d after a refused start, want 0", n)
		}
	})

	t.Run("MET-052/nenhum provedor aplicável com chave para nenhum jogo: -32010", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false, "igdb")
		_, err := f.call(t, ipc.MethodMetadataBatchStart, batchJSON(t, ipc.BatchStartParams{GameIDs: []int64{f.gameID}}))
		if codeOf(err) != ipc.CodeProviderNoKey {
			t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderNoKey)
		}
	})

	t.Run("MET-052/Steam Store não conta como chave: sem IGDB nem SteamGridDB é -32010 com data.provider igdb", func(t *testing.T) {
		t.Parallel()
		steam := newApplyProv("steamstore", nil) // a provider with no key of its own is still not a keyed provider
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{
			"igdb":        newApplyProv("igdb", nil),
			"steamgriddb": newApplyProv("steamgriddb", nil),
			"steamstore":  steam,
		}, "igdb", "steamgriddb")
		_, err := f.call(t, ipc.MethodMetadataBatchStart, batchJSON(t, ipc.BatchStartParams{GameIDs: []int64{f.gameID}}))
		if codeOf(err) != ipc.CodeProviderNoKey {
			t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeProviderNoKey)
		}
		if d := errData(t, err); d["provider"] != "igdb" {
			t.Errorf("data.provider = %q, want igdb", d["provider"])
		}
		if n := steam.totalRequests(); n != 0 {
			t.Errorf("Steam Store requests = %d, want 0", n)
		}
	})

	t.Run("MET-052/em done a nova substitui a antiga (batchId cresce)", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateDone)
		c.f.gate.blockSearch = true // the new search stays in searching while the state is checked
		res := startBatchOK(t, c.f, c.ids...)
		if res.BatchID <= c.id {
			t.Errorf("new batchId %d, want more than the old %d", res.BatchID, c.id)
		}
		wantBatchState(t, c.f.metaFixture, ipc.BatchStateSearching)
	})

	t.Run("MET-052/mais de 1.000 ids e ids repetidos: -32602", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		many := make([]int64, 1001)
		for i := range many {
			many[i] = int64(i + 1)
		}
		cases := []struct {
			name string
			ids  []int64
		}{
			{"1001 ids", many},
			{"ids repetidos", []int64{f.gameID, f.gameID}},
		}
		for _, tc := range cases {
			_, err := f.call(t, ipc.MethodMetadataBatchStart, batchJSON(t, ipc.BatchStartParams{GameIDs: tc.ids}))
			if codeOf(err) != ipc.CodeInvalidParams {
				t.Errorf("%s: code = %d (%v), want %d", tc.name, codeOf(err), err, ipc.CodeInvalidParams)
			}
		}
	})

	t.Run("MET-071/gameIds vazio, id 0 e batchId 0: -32602", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		cases := []struct {
			name, method, params string
		}{
			{"gameIds vazio", ipc.MethodMetadataBatchStart, `{"gameIds":[]}`},
			{"gameId 0", ipc.MethodMetadataBatchStart, `{"gameIds":[0]}`},
			{"batchId 0", ipc.MethodMetadataBatchCancel, `{"batchId":0}`},
		}
		for _, tc := range cases {
			_, err := f.call(t, tc.method, tc.params)
			if codeOf(err) != ipc.CodeInvalidParams {
				t.Errorf("%s: code = %d (%v), want %d", tc.name, codeOf(err), err, ipc.CodeInvalidParams)
			}
		}
	})

	t.Run("MET-053/3 trabalhadores pegam os jogos na ordem recebida (no máximo 3 buscas ao mesmo tempo)", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, true, false)
		res := startBatchOK(t, f, batchGames(t, f, "Hades", "Dead Cells", "Hollow Knight", "Inside")...)

		first := awaitEntered(t, f.gate, 3)
		slices.Sort(first)
		want := []string{domain.SearchTerm("Celeste"), domain.SearchTerm("Hades"), domain.SearchTerm("Dead Cells")}
		slices.Sort(want)
		if !slices.Equal(first, want) {
			t.Errorf("the first three searches are %q, want the first three games %q", first, want)
		}
		// Negative check: a fourth search must not start while three are held.
		select {
		case extra := <-f.gate.entered:
			t.Errorf("search of %q started while three were running", extra)
		case <-time.After(100 * time.Millisecond):
		}
		if _, maxIn := f.gate.stats(); maxIn != 3 {
			t.Errorf("max searches in flight = %d, want 3", maxIn)
		}

		f.gate.release()
		awaitBatchState(t, f.spy, res.BatchID, ipc.BatchStateReady)
		if n, _ := f.gate.stats(); n != 5 {
			t.Errorf("searches = %d, want 5 (one per game)", n)
		}
	})

	t.Run("MET-053/todas as chamadas passam pelos limitadores (contagem de requisições por provedor com intervalo, relógio fake)", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		f.gate.lim = scrapers.NewLimiter(f.clock, batchIGDBInterval)
		res := startBatchOK(t, f, batchGames(t, f, "Hades", "Dead Cells")...)

		// Each step moves the fake clock by the spacing; the held requests run as their slot comes.
		deadline := time.After(testTimeout)
		for {
			n, _ := f.gate.stats()
			if n >= 3 {
				break
			}
			f.clock.Advance(batchIGDBInterval)
			select {
			case <-f.gate.entered:
			case <-time.After(20 * time.Millisecond): // the released request needs real time to run
			case <-deadline:
				t.Fatalf("only %d of 3 requests went out", n)
			}
		}
		awaitBatchState(t, f.spy, res.BatchID, ipc.BatchStateReady)

		times := f.gate.entryTimes()
		slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
		if len(times) != 3 {
			t.Fatalf("requests = %d, want 3", len(times))
		}
		for i := 1; i < len(times); i++ {
			if d := times[i].Sub(times[i-1]); d < batchIGDBInterval {
				t.Errorf("requests %d and %d are %v apart, want at least %v", i-1, i, d, batchIGDBInterval)
			}
		}
	})

	t.Run("MET-053/estado vai a ready quando todos têm linha", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		res := startBatchOK(t, f, batchGames(t, f, "Hades")...)
		awaitBatchState(t, f.spy, res.BatchID, ipc.BatchStateReady)
		b := batchStatusNow(t, f.metaFixture)
		if b == nil || b.State != ipc.BatchStateReady || b.Done != 2 || b.Total != 2 {
			t.Errorf("batch = %+v, want ready with done 2 of total 2", b)
		}
	})

	t.Run("MET-053/a plataforma de cada jogo vai na sua consulta", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		plat := addPlatform(t, f.st, "PlayStation 2", domain.PlatformCategory("console"))
		hades := addGames(t, f.metaFixture, "Hades")[0]
		pid := plat.ID
		if _, err := f.st.UpdateGame(t.Context(), hades, store.GamePatch{PlatformID: &pid}, f.clock.Now()); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		celesteP := gameText(t, f.st, f.gameID).Platform
		hadesP := gameText(t, f.st, hades).Platform
		if celesteP == "" || hadesP == celesteP {
			t.Fatalf("platforms %q and %q, want two different ones", celesteP, hadesP)
		}

		res := startBatchOK(t, f, f.gameID, hades)
		awaitBatchState(t, f.spy, res.BatchID, ipc.BatchStateReady)
		got := map[string]string{}
		for _, q := range f.gate.queriesOf() {
			got[q.Title] = q.Platform
		}
		if got[domain.SearchTerm("Celeste")] != celesteP {
			t.Errorf("Celeste asked with platform %q, want %q", got[domain.SearchTerm("Celeste")], celesteP)
		}
		if got[domain.SearchTerm("Hades")] != hadesP {
			t.Errorf("Hades asked with platform %q, want %q", got[domain.SearchTerm("Hades")], hadesP)
		}
	})

	t.Run("MET-011/3 falhas seguidas desativam o IGDB até o fim do lote e o aviso sai uma só vez", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		f.gate.failWith = scrapers.ErrUnavailable
		res := startBatchOK(t, f, batchGames(t, f, "Hades", "Dead Cells", "Hollow Knight", "Inside", "Celeste 2")...)

		// The suspensions run on the fake clock: each step moves it on until the batch is ready.
		deadline := time.After(testTimeout)
		for !hasBatchNotice(f.spy, res.BatchID, ipc.BatchStateReady) {
			f.clock.Advance(time.Minute)
			select {
			case <-f.spy.signal:
			case <-time.After(20 * time.Millisecond):
			case <-deadline:
				t.Fatalf("the batch did not get ready: %+v", batchStatusNow(t, f.metaFixture))
			}
		}

		b := batchStatusNow(t, f.metaFixture)
		if b == nil {
			t.Fatal("batch is null after it was ready")
		}
		count := 0
		for _, w := range b.Warnings {
			if w == batchWarnIGDB {
				count++
			}
		}
		if count != 1 {
			t.Errorf("warning %q appears %d times in %q, want once", batchWarnIGDB, count, b.Warnings)
		}
		if b.Done+b.Failed != 6 {
			t.Errorf("done %d + failed %d, want 6 games accounted for", b.Done, b.Failed)
		}
		if n, _ := f.gate.stats(); n < 3 {
			t.Errorf("IGDB asked %d times, want at least the 3 failures that disable it", n)
		}
	})

	t.Run("MET-054/mudanças de contador agrupadas em 500 ms e estado final sempre avisado", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		res := startBatchOK(t, f, batchGames(t, f, "Hades", "Dead Cells", "Inside")...)
		final := awaitBatchState(t, f.spy, res.BatchID, ipc.BatchStateReady)
		if final.Done != 4 || final.Total != 4 || final.Failed != 0 {
			t.Errorf("final notice = %+v, want done 4 of 4 with no failure", final)
		}

		// The fake clock does not move, so the window never closes: the four finishes fold into the
		// first counter notice (sent at once) and the state change that follows.
		var searching []ipc.MetadataBatchChangedParams
		for _, e := range f.spy.all() {
			if e.BatchID == res.BatchID && e.State == ipc.BatchStateSearching {
				searching = append(searching, e)
			}
		}
		if len(searching) == 0 || searching[0].Done != 1 {
			t.Errorf("counter notices = %+v, want the first one at once with done 1", searching)
		}
		if len(searching) > 2 {
			t.Errorf("%d counter notices for four finishes in one instant, want them grouped", len(searching))
		}
	})

	t.Run("MET-054/batchStart, batchCancel e batchDiscard não geram notificação", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, true, false)
		res := startBatchOK(t, f, batchGames(t, f, "Hades")...)
		if n := len(f.spy.all()); n != 0 {
			t.Errorf("%d notifications after batchStart, want none", n)
		}
		cancelBatch(t, f.metaFixture, res.BatchID)
		waitReturns(t, "the workers", f.meta.Wait)
		if n := len(f.spy.all()); n != 0 {
			t.Errorf("%d notifications after batchCancel, want none", n)
		}

		// A cancelled batch keeps the slot until it is discarded (§2: batchStart in cancelled is -32015).
		if _, err := f.call(t, ipc.MethodMetadataBatchDiscard, batchJSON(t, ipc.BatchIDParams{BatchID: res.BatchID})); err != nil {
			t.Fatalf("batchDiscard of the cancelled batch: %v", err)
		}
		f.gate.blockSearch = false
		res2 := startBatchOK(t, f, batchGames(t, f, "Inside")...)
		awaitBatchState(t, f.spy, res2.BatchID, ipc.BatchStateReady)
		before := len(f.spy.all())
		if _, err := f.call(t, ipc.MethodMetadataBatchDiscard, batchJSON(t, ipc.BatchIDParams{BatchID: res2.BatchID})); err != nil {
			t.Fatalf("batchDiscard: %v", err)
		}
		if n := len(f.spy.all()); n != before {
			t.Errorf("%d notifications after batchDiscard, want none", n-before)
		}
	})

	t.Run("MET-054/searching → ready notifica na hora (o relógio falso não avança)", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		res := startBatchOK(t, f, batchGames(t, f, "Hades")...)
		awaitBatchState(t, f.spy, res.BatchID, ipc.BatchStateReady)
	})

	t.Run("MET-054/dois clientes IPC reais recebem as mesmas notificações", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		srv := ipc.NewServer(testClock(), metadataHandlers(f.meta), quietLog())
		ctx, cancel := context.WithCancel(context.Background())
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = srv.Serve(ctx, ln)
		}()
		t.Cleanup(func() {
			cancel()
			_ = srv.Close()
			wg.Wait()
		})
		n := &batchNotifier{}
		n.set(srv)
		f.meta.notify = n

		c1 := newTCPClient(ln.Addr().String())
		c2 := newTCPClient(ln.Addr().String())
		t.Cleanup(func() {
			_ = c1.Close()
			_ = c2.Close()
		})
		cctx, ccancel := context.WithTimeout(context.Background(), testTimeout)
		defer ccancel()
		for _, c := range []*ipc.Client{c1, c2} {
			_ = c.Call(cctx, ipc.MethodMetadataBatchStatus, nil, nil) // establishes the connection
		}

		ids := batchGames(t, f, "Hades")
		var res ipc.BatchStartResult
		if err := c1.Call(cctx, ipc.MethodMetadataBatchStart, ipc.BatchStartParams{GameIDs: ids}, &res); err != nil {
			t.Fatalf("batchStart over IPC: %v", err)
		}
		for i, c := range []*ipc.Client{c1, c2} {
			p := awaitClientBatch(t, c, res.BatchID, ipc.BatchStateReady)
			if p.Done != len(ids) || p.Total != len(ids) || p.Failed != 0 {
				t.Errorf("client %d: ready notice = %+v, want done and total %d", i+1, p, len(ids))
			}
		}
	})

	t.Run("MET-055/batchStatus em cada estado: null sem busca, contadores, result só em done, warnings ≤ 10", func(t *testing.T) {
		t.Parallel()
		states := []string{batchNone, ipc.BatchStateSearching, ipc.BatchStateReady, ipc.BatchStateCancelled, ipc.BatchStateApplying, ipc.BatchStateDone}
		for _, state := range states {
			t.Run(state, func(t *testing.T) {
				t.Parallel()
				c := reachBatch(t, state)
				b := batchStatusNow(t, c.f.metaFixture) // never fails by state [MET-055]
				if state == batchNone {
					if b != nil {
						t.Errorf("batch = %+v, want null", *b)
					}
					return
				}
				wantTotal := len(c.ids)
				if state == ipc.BatchStateApplying {
					wantTotal = len(c.rowIDs) // the choices sent (batch-state-machine.md §2)
				}
				if b == nil || b.State != state || b.BatchID != c.id || b.Total != wantTotal {
					t.Fatalf("batch = %+v, want state %q, id %d, total %d", b, state, c.id, wantTotal)
				}
				if len(b.Warnings) > 10 {
					t.Errorf("warnings = %d, want at most 10", len(b.Warnings))
				}
				if (state == ipc.BatchStateDone) != (b.Result != nil) {
					t.Errorf("result = %+v in state %q, want a result only in done", b.Result, state)
				}
				if state == ipc.BatchStateReady && (b.Done != b.Total || b.Failed != 0) {
					t.Errorf("ready counters done %d failed %d, want done %d and no failure", b.Done, b.Failed, b.Total)
				}
			})
		}
	})

	t.Run("MET-056/cancelar no meio: cancelled na hora, nenhuma requisição nova, em andamento não grava linha", func(t *testing.T) {
		f := newBatchFixture(t, true, false)
		snap := goleak.IgnoreCurrent() // taken before the batch starts, so its goroutines are checked
		res := startBatchOK(t, f, batchGames(t, f, "Hades", "Dead Cells", "Hollow Knight", "Inside")...)
		awaitEntered(t, f.gate, 3)

		if state := cancelBatch(t, f.metaFixture, res.BatchID); state != ipc.BatchStateCancelled {
			t.Errorf("batchCancel state = %q, want cancelled at once", state)
		}
		wantBatchState(t, f.metaFixture, ipc.BatchStateCancelled)

		// The held searches see the cancelled context and return without writing a row.
		waitReturns(t, "the workers", f.meta.Wait)
		if n, _ := f.gate.stats(); n != 3 {
			t.Errorf("searches = %d, want 3: nothing new starts after the cancellation", n)
		}
		if rows := batchResultsNow(t, f.metaFixture, res.BatchID); len(rows.Rows) != 0 {
			t.Errorf("rows = %d, want 0: the searches in progress must not write", len(rows.Rows))
		}
		goleak.VerifyNone(t, snap) // the workers end without goroutines left [MET-059]
	})

	t.Run("MET-056/linhas já buscadas continuam disponíveis", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, true, false)
		f.gate.free = map[string]bool{"Celeste": true}
		res := startBatchOK(t, f, batchGames(t, f, "Hades", "Dead Cells")...)
		awaitBatchDone(t, f.spy, res.BatchID, 1)

		cancelBatch(t, f.metaFixture, res.BatchID)
		waitReturns(t, "the workers", f.meta.Wait)
		rows := batchResultsNow(t, f.metaFixture, res.BatchID).Rows
		if len(rows) != 1 || rows[0].GameID != f.gameID {
			t.Errorf("rows = %+v, want only the game searched before the cancellation", rows)
		}
	})

	for _, state := range []string{ipc.BatchStateReady, ipc.BatchStateCancelled} {
		t.Run("MET-056/cancelar em "+state+" é no-op que devolve o estado", func(t *testing.T) {
			t.Parallel()
			c := reachBatch(t, state)
			before := len(c.f.spy.all())
			if got := cancelBatch(t, c.f.metaFixture, c.id); got != state {
				t.Errorf("batchCancel state = %q, want %q", got, state)
			}
			wantBatchState(t, c.f.metaFixture, state)
			if n := len(c.f.spy.all()); n != before {
				t.Errorf("%d notifications from a no-op cancel, want none", n-before)
			}
		})
	}

	for _, state := range []string{ipc.BatchStateReady, ipc.BatchStateCancelled, ipc.BatchStateDone} {
		t.Run("MET-058/batchDiscard em "+state+" libera: discarded true e sem busca", func(t *testing.T) {
			t.Parallel()
			c := reachBatch(t, state)
			out, err := c.f.call(t, ipc.MethodMetadataBatchDiscard, batchJSON(t, ipc.BatchIDParams{BatchID: c.id}))
			if err != nil {
				t.Fatalf("batchDiscard: %v", err)
			}
			if res, ok := out.(ipc.BatchDiscardResult); !ok || !res.Discarded {
				t.Errorf("batchDiscard = %+v, want discarded true", out)
			}
			wantBatchState(t, c.f.metaFixture, batchNone)
		})
	}

	t.Run("MET-058/batchId já descartado: -32014", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateReady)
		discard := func() error {
			_, err := c.f.call(t, ipc.MethodMetadataBatchDiscard, batchJSON(t, ipc.BatchIDParams{BatchID: c.id}))
			return err
		}
		if err := discard(); err != nil {
			t.Fatalf("first batchDiscard: %v", err)
		}
		if code := codeOf(discard()); code != ipc.CodeBatchNotFound {
			t.Errorf("second batchDiscard code = %d, want %d", code, ipc.CodeBatchNotFound)
		}
	})

	for _, state := range []string{batchNone, ipc.BatchStateSearching, ipc.BatchStateReady, ipc.BatchStateCancelled, ipc.BatchStateApplying, ipc.BatchStateDone} {
		t.Run("MET-058/batchId desconhecido em "+state+": -32014", func(t *testing.T) {
			t.Parallel()
			c := reachBatch(t, state)
			_, err := c.f.call(t, ipc.MethodMetadataBatchDiscard, batchJSON(t, ipc.BatchIDParams{BatchID: c.id + 1000}))
			if codeOf(err) != ipc.CodeBatchNotFound {
				t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeBatchNotFound)
			}
		})
	}

	t.Run("MET-058/nextBatchID cresce estritamente mesmo com o relógio parado", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(healthStart)
		first := nextBatchID(clock, 0)
		if first != healthStart.UnixMilli() {
			t.Errorf("first id = %d, want the clock in ms %d", first, healthStart.UnixMilli())
		}
		second := nextBatchID(clock, first)
		if second != first+1 {
			t.Errorf("second id = %d, want %d", second, first+1)
		}
		if third := nextBatchID(clock, first+10); third != first+11 {
			t.Errorf("id after a later last = %d, want %d", third, first+11)
		}
	})

	t.Run("MET-058/batchId cresce estritamente em batches sucessivos com o relógio parado", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, false, false)
		ids := batchGames(t, f, "Hades")
		var last int64
		for i := 0; i < 3; i++ {
			res := startBatchOK(t, f, ids...)
			if res.BatchID <= last {
				t.Fatalf("batch %d: id %d not greater than %d", i, res.BatchID, last)
			}
			last = res.BatchID
			awaitBatchState(t, f.spy, res.BatchID, ipc.BatchStateReady)
			if _, err := f.call(t, ipc.MethodMetadataBatchDiscard, batchJSON(t, ipc.BatchIDParams{BatchID: res.BatchID})); err != nil {
				t.Fatalf("batchDiscard: %v", err)
			}
		}
	})

	t.Run("MET-058/nada é gravado no banco durante a busca (contagem de linhas antes e depois)", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, true, false)
		ids := batchGames(t, f, "Hades")
		before := gameDetails(t, f, ids)
		res := startBatchOK(t, f, ids...)
		awaitEntered(t, f.gate, 2)
		if during := gameDetails(t, f, ids); !reflect.DeepEqual(before, during) {
			t.Errorf("games changed while searching:\nbefore %+v\nduring %+v", before, during)
		}
		if jobs := pendingJobs(t, f.st); len(jobs) != 0 {
			t.Errorf("media jobs while searching = %d, want 0", len(jobs))
		}

		f.gate.release()
		awaitBatchState(t, f.spy, res.BatchID, ipc.BatchStateReady)
		if after := gameDetails(t, f, ids); !reflect.DeepEqual(before, after) {
			t.Errorf("games changed by the search:\nbefore %+v\nafter %+v", before, after)
		}
		if jobs := pendingJobs(t, f.st); len(jobs) != 0 {
			t.Errorf("media jobs after the search = %d, want 0", len(jobs))
		}
	})

	t.Run("MET-058/reiniciar o agente perde a busca (novo serviço: batchStatus null)", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, true, false)
		startBatchOK(t, f, batchGames(t, f, "Hades")...)
		restarted := newMetadataService(f.svc, f.st, t.Context())
		h, ok := metadataHandlers(restarted)[ipc.MethodMetadataBatchStatus]
		if !ok {
			t.Fatal("metadataHandlers has no metadata.batchStatus")
		}
		out, err := h(t.Context(), json.RawMessage("{}"))
		if err != nil {
			t.Fatalf("batchStatus: %v", err)
		}
		if res, ok := out.(ipc.BatchStatusResult); !ok || res.Batch != nil {
			t.Errorf("after a restart batchStatus = %+v, want batch null", out)
		}
	})

	t.Run("MET-055/batchStart e batchStatus logo depois: searching, done 0, total n, sem notificação da chamada", func(t *testing.T) {
		t.Parallel()
		f := newBatchFixture(t, true, false)
		ids := batchGames(t, f, "Hades")
		res := startBatchOK(t, f, ids...)
		b := batchStatusNow(t, f.metaFixture)
		if b == nil || b.BatchID != res.BatchID || b.State != ipc.BatchStateSearching || b.Done != 0 || b.Total != len(ids) || b.Failed != 0 {
			t.Errorf("batch = %+v, want searching with done 0 of %d", b, len(ids))
		}
		if n := len(f.spy.all()); n != 0 {
			t.Errorf("%d notifications from batchStart and batchStatus, want none (MET-054)", n)
		}
	})

	t.Run("MET-058/batchDiscard concorrente com apply, results e cancel: sem pânico e sem ler linhas descartadas", func(t *testing.T) {
		t.Parallel()
		expected := map[int]bool{ipc.CodeBatchNotFound: true, ipc.CodeBatchWrongState: true, ipc.CodeBatchBusy: true, ipc.CodeInvalidParams: true}
		f := newBatchFixture(t, false, false)
		ids := batchGames(t, f, "Hades", "Dead Cells")
		for round := range 100 {
			res := startBatchOK(t, f, ids...)
			awaitBatchState(t, f.spy, res.BatchID, ipc.BatchStateReady)
			id := res.BatchID
			start := make(chan struct{})
			var wg sync.WaitGroup
			run := func(name string, call func() (any, error)) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("round %d %s: panic: %v", round, name, r)
						}
					}()
					<-start
					out, err := call()
					if err != nil && !expected[codeOf(err)] {
						t.Errorf("round %d %s: %v", round, name, err)
					}
					if rs, ok := out.(ipc.BatchResultsResult); ok && err == nil && rs.Total != len(ids) {
						t.Errorf("round %d batchResults read %d rows of a discarded batch, want %d", round, rs.Total, len(ids))
					}
				}()
			}
			run("batchApply", func() (any, error) {
				return f.call(t, ipc.MethodMetadataBatchApply, batchJSON(t, ipc.BatchApplyParams{BatchID: id, Choices: choicesOf(ids)}))
			})
			run("batchResults", func() (any, error) {
				return f.call(t, ipc.MethodMetadataBatchResults, batchJSON(t, ipc.BatchResultsParams{BatchID: id}))
			})
			run("batchCancel", func() (any, error) {
				return f.call(t, ipc.MethodMetadataBatchCancel, batchJSON(t, ipc.BatchIDParams{BatchID: id}))
			})
			run("batchDiscard", func() (any, error) {
				return f.call(t, ipc.MethodMetadataBatchDiscard, batchJSON(t, ipc.BatchIDParams{BatchID: id}))
			})
			close(start)
			wg.Wait()

			// Clean up: a batch left applying ends as done with a cancel; then it is discarded.
			_, _ = f.call(t, ipc.MethodMetadataBatchCancel, batchJSON(t, ipc.BatchIDParams{BatchID: id}))
			waitReturns(t, "the workers of the round", f.meta.Wait)
			_, _ = f.call(t, ipc.MethodMetadataBatchDiscard, batchJSON(t, ipc.BatchIDParams{BatchID: id}))
			wantBatchState(t, f.metaFixture, batchNone)
		}
	})

	t.Run("MET-059/desligamento cancela o contexto raiz, Wait espera trabalhadores e nenhuma requisição nova é feita", func(t *testing.T) {
		f := newBatchFixture(t, true, false)
		root, stop := context.WithCancel(t.Context())
		m := newMetadataService(f.svc, f.st, root)
		m.notify = f.spy
		h, ok := metadataHandlers(m)[ipc.MethodMetadataBatchStart]
		if !ok {
			t.Fatal("metadataHandlers has no metadata.batchStart")
		}
		snap := goleak.IgnoreCurrent()

		ids := batchGames(t, f, "Hades", "Dead Cells", "Hollow Knight", "Inside")
		if _, err := h(t.Context(), json.RawMessage(batchJSON(t, ipc.BatchStartParams{GameIDs: ids}))); err != nil {
			t.Fatalf("batchStart: %v", err)
		}
		awaitEntered(t, f.gate, 3)

		stop()
		waitReturns(t, "Wait of the service", m.Wait)
		if n, _ := f.gate.stats(); n != 3 {
			t.Errorf("searches = %d, want 3: nothing new after the shutdown", n)
		}
		goleak.VerifyNone(t, snap)
	})
}

// awaitClientBatch reads metadata.batchChanged from c until one for batch id with state arrives.
func awaitClientBatch(t *testing.T, c *ipc.Client, id int64, state string) ipc.MetadataBatchChangedParams {
	t.Helper()
	for {
		p, ok := nextBatchChanged(t, c, testTimeout)
		if !ok {
			t.Fatalf("no metadata.batchChanged %q for batch %d", state, id)
		}
		if p.BatchID == id && p.State == state {
			return p
		}
	}
}

// TestBatchStateMachine is the table of contracts/batch-state-machine.md §2: one row per pair of
// state and event. Each row starts from a fresh fixture brought to the state.
func TestBatchStateMachine(t *testing.T) {
	t.Parallel()

	type row struct {
		name      string
		state     string // state before the event
		method    string
		params    func(t *testing.T, c batchCase) string
		prepare   func(c *batchCase) // optional: set up the fixture before the event
		wantCode  int                // 0 = success
		dataState string             // data.state of the error, when the code carries it
		wantState string             // state after the event (batchNone = null)
	}
	start := func(t *testing.T, c batchCase) string { return batchJSON(t, ipc.BatchStartParams{GameIDs: c.ids}) }
	status := func(t *testing.T, c batchCase) string { return "{}" }
	results := func(t *testing.T, c batchCase) string {
		return batchJSON(t, ipc.BatchResultsParams{BatchID: c.id})
	}
	apply := func(t *testing.T, c batchCase) string {
		return batchJSON(t, ipc.BatchApplyParams{BatchID: c.id, Choices: choicesOf(c.rowIDs)})
	}
	applyOutside := func(t *testing.T, c batchCase) string {
		return batchJSON(t, ipc.BatchApplyParams{BatchID: c.id, Choices: choicesOf([]int64{999})})
	}
	byID := func(t *testing.T, c batchCase) string { return batchJSON(t, ipc.BatchIDParams{BatchID: c.id}) }
	holdSearch := func(c *batchCase) { c.f.gate.blockSearch = true } // the new batch must stay in searching

	rows := []row{
		// no batch
		{name: "MET-052/sem busca + batchStart válido → searching", state: batchNone, method: ipc.MethodMetadataBatchStart, params: start, wantState: ipc.BatchStateSearching},
		{name: "MET-055/sem busca + batchStatus → null", state: batchNone, method: ipc.MethodMetadataBatchStatus, params: status, wantState: batchNone},
		{name: "MET-057/sem busca + batchResults → -32014", state: batchNone, method: ipc.MethodMetadataBatchResults, params: results, wantCode: ipc.CodeBatchNotFound, wantState: batchNone},
		{name: "MET-064/sem busca + batchApply → -32014", state: batchNone, method: ipc.MethodMetadataBatchApply, params: apply, wantCode: ipc.CodeBatchNotFound, wantState: batchNone},
		{name: "MET-056/sem busca + batchCancel → -32014", state: batchNone, method: ipc.MethodMetadataBatchCancel, params: byID, wantCode: ipc.CodeBatchNotFound, wantState: batchNone},
		{name: "MET-058/sem busca + batchDiscard → -32014", state: batchNone, method: ipc.MethodMetadataBatchDiscard, params: byID, wantCode: ipc.CodeBatchNotFound, wantState: batchNone},

		// searching
		{name: "MET-056/searching + batchCancel → cancelled", state: ipc.BatchStateSearching, method: ipc.MethodMetadataBatchCancel, params: byID, wantState: ipc.BatchStateCancelled},
		{name: "MET-052/searching + batchStart → -32015 com data.state", state: ipc.BatchStateSearching, method: ipc.MethodMetadataBatchStart, params: start, wantCode: ipc.CodeBatchBusy, dataState: ipc.BatchStateSearching, wantState: ipc.BatchStateSearching},
		{name: "MET-057/searching + batchResults → -32016", state: ipc.BatchStateSearching, method: ipc.MethodMetadataBatchResults, params: results, wantCode: ipc.CodeBatchWrongState, dataState: ipc.BatchStateSearching, wantState: ipc.BatchStateSearching},
		{name: "MET-064/searching + batchApply → -32016", state: ipc.BatchStateSearching, method: ipc.MethodMetadataBatchApply, params: apply, prepare: func(c *batchCase) { c.rowIDs = c.ids[:1] }, wantCode: ipc.CodeBatchWrongState, dataState: ipc.BatchStateSearching, wantState: ipc.BatchStateSearching},
		{name: "MET-058/searching + batchDiscard → -32016", state: ipc.BatchStateSearching, method: ipc.MethodMetadataBatchDiscard, params: byID, wantCode: ipc.CodeBatchWrongState, dataState: ipc.BatchStateSearching, wantState: ipc.BatchStateSearching},
		{name: "MET-055/searching + batchStatus → searching", state: ipc.BatchStateSearching, method: ipc.MethodMetadataBatchStatus, params: status, wantState: ipc.BatchStateSearching},

		// ready
		{name: "MET-057/ready + batchResults → ready", state: ipc.BatchStateReady, method: ipc.MethodMetadataBatchResults, params: results, wantState: ipc.BatchStateReady},
		{name: "MET-064/ready + batchApply válido → applying", state: ipc.BatchStateReady, method: ipc.MethodMetadataBatchApply, params: apply, wantState: ipc.BatchStateApplying},
		{name: "MET-064/ready + batchApply com gameId fora da busca → -32602", state: ipc.BatchStateReady, method: ipc.MethodMetadataBatchApply, params: applyOutside, wantCode: ipc.CodeInvalidParams, wantState: ipc.BatchStateReady},
		{name: "MET-056/ready + batchCancel → ready (no-op)", state: ipc.BatchStateReady, method: ipc.MethodMetadataBatchCancel, params: byID, wantState: ipc.BatchStateReady},
		{name: "MET-058/ready + batchDiscard → sem busca", state: ipc.BatchStateReady, method: ipc.MethodMetadataBatchDiscard, params: byID, wantState: batchNone},
		{name: "MET-052/ready + batchStart → -32015 com data.state", state: ipc.BatchStateReady, method: ipc.MethodMetadataBatchStart, params: start, wantCode: ipc.CodeBatchBusy, dataState: ipc.BatchStateReady, wantState: ipc.BatchStateReady},

		// cancelled
		{name: "MET-057/cancelled + batchResults → cancelled", state: ipc.BatchStateCancelled, method: ipc.MethodMetadataBatchResults, params: results, wantState: ipc.BatchStateCancelled},
		{name: "MET-064/cancelled + batchApply válido → applying", state: ipc.BatchStateCancelled, method: ipc.MethodMetadataBatchApply, params: apply, wantState: ipc.BatchStateApplying},
		{name: "MET-056/cancelled + batchCancel → cancelled (no-op)", state: ipc.BatchStateCancelled, method: ipc.MethodMetadataBatchCancel, params: byID, wantState: ipc.BatchStateCancelled},
		{name: "MET-058/cancelled + batchDiscard → sem busca", state: ipc.BatchStateCancelled, method: ipc.MethodMetadataBatchDiscard, params: byID, wantState: batchNone},
		{name: "MET-052/cancelled + batchStart → -32015 com data.state", state: ipc.BatchStateCancelled, method: ipc.MethodMetadataBatchStart, params: start, wantCode: ipc.CodeBatchBusy, dataState: ipc.BatchStateCancelled, wantState: ipc.BatchStateCancelled},

		// applying
		{name: "MET-052/applying + batchStart → -32015 com data.state", state: ipc.BatchStateApplying, method: ipc.MethodMetadataBatchStart, params: start, wantCode: ipc.CodeBatchBusy, dataState: ipc.BatchStateApplying, wantState: ipc.BatchStateApplying},
		{name: "MET-064/applying + batchApply → -32016", state: ipc.BatchStateApplying, method: ipc.MethodMetadataBatchApply, params: apply, wantCode: ipc.CodeBatchWrongState, dataState: ipc.BatchStateApplying, wantState: ipc.BatchStateApplying},
		{name: "MET-057/applying + batchResults → -32016", state: ipc.BatchStateApplying, method: ipc.MethodMetadataBatchResults, params: results, wantCode: ipc.CodeBatchWrongState, dataState: ipc.BatchStateApplying, wantState: ipc.BatchStateApplying},
		{name: "MET-058/applying + batchDiscard → -32016", state: ipc.BatchStateApplying, method: ipc.MethodMetadataBatchDiscard, params: byID, wantCode: ipc.CodeBatchWrongState, dataState: ipc.BatchStateApplying, wantState: ipc.BatchStateApplying},
		{name: "MET-056/applying + batchCancel → done (resultado parcial)", state: ipc.BatchStateApplying, method: ipc.MethodMetadataBatchCancel, params: byID, wantState: ipc.BatchStateDone},
		{name: "MET-055/applying + batchStatus → applying", state: ipc.BatchStateApplying, method: ipc.MethodMetadataBatchStatus, params: status, wantState: ipc.BatchStateApplying},

		// done
		{name: "MET-055/done + batchStatus → done com result", state: ipc.BatchStateDone, method: ipc.MethodMetadataBatchStatus, params: status, wantState: ipc.BatchStateDone},
		{name: "MET-058/done + batchDiscard → sem busca", state: ipc.BatchStateDone, method: ipc.MethodMetadataBatchDiscard, params: byID, wantState: batchNone},
		{name: "MET-052/done + batchStart válido → searching (substitui a antiga)", state: ipc.BatchStateDone, method: ipc.MethodMetadataBatchStart, params: start, prepare: holdSearch, wantState: ipc.BatchStateSearching},
		{name: "MET-057/done + batchResults → -32016", state: ipc.BatchStateDone, method: ipc.MethodMetadataBatchResults, params: results, wantCode: ipc.CodeBatchWrongState, dataState: ipc.BatchStateDone, wantState: ipc.BatchStateDone},
		{name: "MET-064/done + batchApply → -32016", state: ipc.BatchStateDone, method: ipc.MethodMetadataBatchApply, params: apply, wantCode: ipc.CodeBatchWrongState, dataState: ipc.BatchStateDone, wantState: ipc.BatchStateDone},
		{name: "MET-056/done + batchCancel → done (no-op)", state: ipc.BatchStateDone, method: ipc.MethodMetadataBatchCancel, params: byID, wantState: ipc.BatchStateDone},
	}

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			c := reachBatch(t, r.state)
			if r.prepare != nil {
				r.prepare(&c)
			}
			_, err := c.f.call(t, r.method, r.params(t, c))
			if r.wantCode == 0 {
				if err != nil {
					t.Fatalf("error %v, want success", err)
				}
			} else if codeOf(err) != r.wantCode {
				t.Fatalf("code = %d (%v), want %d", codeOf(err), err, r.wantCode)
			}
			if r.dataState != "" {
				if d := errData(t, err); d["state"] != r.dataState {
					t.Errorf("data.state = %q, want %q", d["state"], r.dataState)
				}
			}
			wantBatchState(t, c.f.metaFixture, r.wantState)
		})
	}
}
