package agent

// Tests of metadata.batchApply (phase 12, T27): the choices and the states that refuse the call (MET-064,
// MET-071), the application in the background by one worker (MET-065), the cancellation and the summary
// (MET-066), the image limit shared with metadata.apply (MET-022, MET-023) and the shutdown during applying
// (MET-059). Providers are fakes (applyProv, below); the store is real, in t.TempDir(); the clock is a
// domain.FakeClock. There is no time.Sleep: asynchronous events are awaited on channels with testTimeout.
// The helpers of the batch tests are in metadata_batch_test.go; metadata_batch_results_test.go uses some of
// the helpers of this file too.
// Alterado em 2026-11-15: arquivo novo (fase 12, T27).

import (
	"context"
	"encoding/json"
	"fmt"
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

// applyProv is a provider fake for the batch tests of the search and of the application. Search answers
// what echo says for the query (nil echo: no candidate). Fetch, the one request of the application, is
// counted, its provider id is kept in order and announced on fetched. It waits for block, or for the channel
// of its id in holdIDs, or for the context; it fails with failIDs[id] or with fetchErr.
type applyProv struct {
	metaFetchProv
	echo    func(q scrapers.Query) []scrapers.Candidate
	block   chan struct{}            // when set, every Fetch waits for it (or for the context)
	holdIDs map[string]chan struct{} // when set, the Fetch of that provider id waits for its channel
	failIDs map[string]error         // the Fetch of that provider id fails with the error
	fetched chan string              // one value per Fetch entry (buffered)
}

func newApplyProv(id string, echo func(q scrapers.Query) []scrapers.Candidate) *applyProv {
	return &applyProv{
		metaFetchProv: metaFetchProv{metaProv: metaProv{id: id}},
		echo:          echo,
		fetched:       make(chan string, 64),
	}
}

func (p *applyProv) Search(_ context.Context, q scrapers.Query) ([]scrapers.Candidate, error) {
	p.record("Search")
	if p.echo == nil {
		return nil, nil
	}
	return p.echo(q), nil
}

func (p *applyProv) Fetch(ctx context.Context, providerID string) (scrapers.Metadata, []scrapers.RemoteMedia, error) {
	p.record("Fetch")
	p.noteID(providerID)
	select {
	case p.fetched <- providerID:
	default:
	}
	if ch := p.gateOf(providerID); ch != nil {
		select {
		case <-ch:
		case <-ctx.Done():
			return scrapers.Metadata{}, nil, ctx.Err()
		}
	}
	if err := p.failIDs[providerID]; err != nil {
		return scrapers.Metadata{}, nil, err
	}
	if p.fetchErr != nil {
		return scrapers.Metadata{}, nil, p.fetchErr
	}
	return p.meta, p.media, nil
}

// gateOf is the channel a Fetch of providerID waits for, or nil when it does not wait.
func (p *applyProv) gateOf(providerID string) chan struct{} {
	if ch, ok := p.holdIDs[providerID]; ok {
		return ch
	}
	return p.block
}

// awaitFetchOf waits until the provider is asked to fetch id; the other fetches seen meanwhile are skipped.
func awaitFetchOf(t *testing.T, p *applyProv, id string) {
	t.Helper()
	deadline := time.After(testTimeout)
	for {
		select {
		case got := <-p.fetched:
			if got == id {
				return
			}
		case <-deadline:
			t.Fatalf("no fetch of provider id %q", id)
		}
	}
}

// expectNoFetch fails the test when the provider starts a fetch within a short window: a negative check
// on the fetched channel, not a wait for behaviour.
func expectNoFetch(t *testing.T, p *applyProv, what string) {
	t.Helper()
	select {
	case id := <-p.fetched:
		t.Errorf("%s: fetch of %q started", what, id)
	case <-time.After(100 * time.Millisecond):
	}
}

// newGate returns a channel that the test opens with open (idempotent). It is opened again at cleanup, so
// a held request never outlives the test.
func newGate(t *testing.T) (chan struct{}, func()) {
	t.Helper()
	gate := make(chan struct{})
	var once sync.Once
	open := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(open)
	return gate, open
}

// callWithin runs the handler of method and fails the test when it does not return in testTimeout. A batch
// call answers at once, whatever the application is doing [MET-064].
func callWithin(t *testing.T, f metaFixture, method, params string) (any, error) {
	t.Helper()
	h, ok := f.handlers[method]
	if !ok {
		t.Fatalf("no handler for %s", method)
	}
	type reply struct {
		out any
		err error
	}
	ctx := t.Context()
	done := make(chan reply, 1)
	go func() {
		out, err := h(ctx, json.RawMessage(params))
		done <- reply{out, err}
	}()
	select {
	case r := <-done:
		return r.out, r.err
	case <-time.After(testTimeout):
		t.Fatalf("%s did not return", method)
		return nil, nil
	}
}

// numberedTitles returns n titles, prefix 001, prefix 002, ...
func numberedTitles(prefix string, n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, fmt.Sprintf("%s %03d", prefix, i+1))
	}
	return out
}

// imagesOf is the IGDB images of a game, by its search term: covers, then screenshots. The URLs carry the
// term, so two games never offer the same image.
func imagesOf(term string, covers, screenshots int) []scrapers.RemoteMedia {
	slug := strings.ReplaceAll(term, " ", "-")
	var out []scrapers.RemoteMedia
	for i := range covers {
		out = append(out, scrapers.RemoteMedia{Kind: scrapers.KindCover, URL: fmt.Sprintf("https://cdn.example.test/%s/cover-%d.png", slug, i)})
	}
	for i := range screenshots {
		out = append(out, scrapers.RemoteMedia{Kind: scrapers.KindScreenshot, URL: fmt.Sprintf("https://cdn.example.test/%s/shot-%d.png", slug, i)})
	}
	return out
}

// applyCfg describes the world of the application tests. titles are the games added after the fixture's
// "Celeste". igdb: IGDB matches every game, with text and images (covers and screenshots per game). steam:
// Steam Store answers every search (its provider id is the game's position: Celeste is 1). SteamGridDB
// answers nothing, so no group ever has it.
type applyCfg struct {
	titles      []string
	igdb        bool
	steam       bool
	covers      int
	screenshots int
}

// applyWorld is the fixture of the application tests: the metadata service over a real store, the three
// provider fakes and the batch notifications on a spy.
type applyWorld struct {
	batchFixture
	cfg   applyCfg
	igdb  *applyProv
	steam *applyProv
	sgdb  *applyProv
}

// newApplyWorld builds the world described by cfg. The IGDB text and images are the ones of the candidate
// the search keeps, so the application of an IGDB group must not ask IGDB again [MET-065].
func newApplyWorld(t *testing.T, cfg applyCfg) applyWorld {
	t.Helper()
	all := append([]string{"Celeste"}, cfg.titles...)
	steamIDs := make(map[string]string, len(all))
	for i, title := range all {
		steamIDs[domain.SearchTerm(title)] = fmt.Sprint(i + 1)
	}

	igdb := newApplyProv("igdb", nil)
	igdb.meta = scrapers.Metadata{Description: "Texto do IGDB", Genres: []string{"Platform"}}
	igdb.media = imagesOf(domain.SearchTerm("Celeste"), cfg.covers, cfg.screenshots)
	if cfg.igdb {
		igdb.echo = func(q scrapers.Query) []scrapers.Candidate {
			c := batchEcho(q)
			c.Meta = &scrapers.Metadata{Description: "Texto do IGDB", Genres: []string{"Platform"}}
			c.Images = imagesOf(q.Title, cfg.covers, cfg.screenshots)
			return []scrapers.Candidate{c}
		}
	}

	steam := newApplyProv("steamstore", nil)
	steam.meta = scrapers.Metadata{Description: "Texto da Steam", Developers: []string{"Dev"}}
	if cfg.steam {
		steam.echo = func(q scrapers.Query) []scrapers.Candidate {
			return []scrapers.Candidate{metaCand("steamstore", steamIDs[q.Title], q.Title, 0, "")}
		}
	}

	sgdb := newApplyProv("steamgriddb", nil)
	f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamstore": steam, "steamgriddb": sgdb})
	spy := newBatchSpy()
	f.meta.notify = spy
	return applyWorld{
		batchFixture: batchFixture{metaFixture: f, spy: spy},
		cfg:          cfg,
		igdb:         igdb,
		steam:        steam,
		sgdb:         sgdb,
	}
}

// readyBatch starts the batch of the fixture's game and the world's titles, and waits for ready. It returns
// the batch id and the game ids, in the order sent.
func (w applyWorld) readyBatch(t *testing.T) (int64, []int64) {
	t.Helper()
	ids := batchGames(t, w.batchFixture, w.cfg.titles...)
	res := startBatchOK(t, w.batchFixture, ids...)
	awaitBatchState(t, w.spy, res.BatchID, ipc.BatchStateReady)
	return res.BatchID, ids
}

// applyParams is the params of metadata.batchApply for the games ids, each with candidate 0.
func applyParams(t *testing.T, id int64, ids []int64) string {
	t.Helper()
	return batchJSON(t, ipc.BatchApplyParams{BatchID: id, Choices: choicesOf(ids)})
}

// applyErr sends metadata.batchApply for batch id with the choices and returns the error.
func applyErr(t *testing.T, f metaFixture, id int64, choices []ipc.BatchChoice) error {
	t.Helper()
	_, err := f.call(t, ipc.MethodMetadataBatchApply, batchJSON(t, ipc.BatchApplyParams{BatchID: id, Choices: choices}))
	return err
}

// applyWant sends metadata.batchApply with the choices to batch c and checks the error code (0 = success).
func applyWant(t *testing.T, c batchCase, choices []ipc.BatchChoice, want int) {
	t.Helper()
	if err := applyErr(t, c.f.metaFixture, c.id, choices); codeOf(err) != want {
		t.Errorf("code = %d (%v), want %d", codeOf(err), err, want)
	}
}

// batchApplyOK sends metadata.batchApply for the games ids (candidate 0 each) and returns its result. The
// call must answer at once.
func batchApplyOK(t *testing.T, w applyWorld, id int64, ids []int64) ipc.BatchApplyResult {
	t.Helper()
	out, err := callWithin(t, w.metaFixture, ipc.MethodMetadataBatchApply, applyParams(t, id, ids))
	if err != nil {
		t.Fatalf("metadata.batchApply: %v", err)
	}
	res, ok := out.(ipc.BatchApplyResult)
	if !ok {
		t.Fatalf("result type %T, want ipc.BatchApplyResult", out)
	}
	return res
}

// awaitDone waits for the done notification of batch id and returns its status, which carries the result.
func awaitDone(t *testing.T, w applyWorld, id int64) ipc.BatchStatus {
	t.Helper()
	awaitBatchState(t, w.spy, id, ipc.BatchStateDone)
	b := batchStatusNow(t, w.metaFixture)
	if b == nil || b.Result == nil {
		t.Fatalf("batch = %+v, want done with a result", b)
	}
	return *b
}

// refIDs returns the provider ids of provider that the first candidate of each row offers: the candidate
// the application takes, since every choice is candidate 0.
func refIDs(rows []ipc.BatchRow, provider string) []string {
	var out []string
	for _, r := range rows {
		if len(r.Candidates) == 0 {
			continue
		}
		for _, ref := range r.Candidates[0].Refs {
			if ref.Provider == provider {
				out = append(out, ref.ProviderID)
			}
		}
	}
	return out
}

// TestBatchApplyChoices checks the calls of metadata.batchApply that are refused, and that refusing them
// leaves the batch as it was: the choices that are not valid [MET-064, MET-071] and the states that do not
// take an application [MET-064].
func TestBatchApplyChoices(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		choices func(c batchCase) []ipc.BatchChoice
	}{
		{"MET-064/gameId fora da busca: -32602", func(c batchCase) []ipc.BatchChoice {
			return []ipc.BatchChoice{{GameID: 999, Candidate: 0}}
		}},
		{"MET-064/candidate fora da lista da linha: -32602", func(c batchCase) []ipc.BatchChoice {
			return []ipc.BatchChoice{{GameID: c.ids[0], Candidate: 99}}
		}},
		{"MET-071/candidate negativo: -32602", func(c batchCase) []ipc.BatchChoice {
			return []ipc.BatchChoice{{GameID: c.ids[0], Candidate: -1}}
		}},
		{"MET-064/gameId repetido: -32602", func(c batchCase) []ipc.BatchChoice {
			return []ipc.BatchChoice{{GameID: c.ids[0], Candidate: 0}, {GameID: c.ids[0], Candidate: 0}}
		}},
		{"MET-064/mais de 1000 escolhas: -32602", func(c batchCase) []ipc.BatchChoice {
			out := make([]ipc.BatchChoice, 1001)
			for i := range out {
				out[i] = ipc.BatchChoice{GameID: c.ids[i%len(c.ids)], Candidate: 0}
			}
			return out
		}},
		{"MET-071/escolhas vazias: -32602", func(c batchCase) []ipc.BatchChoice {
			return []ipc.BatchChoice{}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := reachBatch(t, ipc.BatchStateReady)
			applyWant(t, c, tc.choices(c), ipc.CodeInvalidParams)
			wantBatchState(t, c.f.metaFixture, ipc.BatchStateReady)
		})
	}

	t.Run("MET-071/batchId 0: -32602", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateReady)
		applyWant(t, batchCase{f: c.f, ids: c.ids, rowIDs: c.rowIDs, id: 0}, choicesOf(c.ids), ipc.CodeInvalidParams)
	})

	t.Run("MET-064/batchId desconhecido: -32014", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateReady)
		applyWant(t, batchCase{f: c.f, ids: c.ids, rowIDs: c.rowIDs, id: c.id + 1000}, choicesOf(c.ids), ipc.CodeBatchNotFound)
	})

	t.Run("MET-064/sem busca: -32014", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, batchNone)
		applyWant(t, c, choicesOf(c.ids), ipc.CodeBatchNotFound)
	})

	for _, state := range []string{ipc.BatchStateSearching, ipc.BatchStateApplying, ipc.BatchStateDone} {
		t.Run("MET-064/em "+state+": -32016 com data.state", func(t *testing.T) {
			t.Parallel()
			c := reachBatch(t, state)
			err := applyErr(t, c.f.metaFixture, c.id, choicesOf(c.ids))
			if codeOf(err) != ipc.CodeBatchWrongState {
				t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeBatchWrongState)
			}
			if d := errData(t, err); d["state"] != state {
				t.Errorf("data.state = %q, want %q", d["state"], state)
			}
		})
	}

	t.Run("MET-064/em cancelled só aceita jogos com linha", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateCancelled)
		applyWant(t, c, choicesOf(c.ids), ipc.CodeInvalidParams) // the games without a row are not in the search
		out, err := c.f.call(t, ipc.MethodMetadataBatchApply, batchJSON(t, ipc.BatchApplyParams{BatchID: c.id, Choices: choicesOf(c.rowIDs)}))
		if err != nil {
			t.Fatalf("batchApply with the games that have a row: %v", err)
		}
		if res, ok := out.(ipc.BatchApplyResult); !ok || res.Accepted != len(c.rowIDs) {
			t.Errorf("batchApply = %+v, want accepted %d", out, len(c.rowIDs))
		}
	})
}

// TestBatchApply checks the application of the choices in the background [MET-064..MET-066, MET-022, MET-023,
// MET-065]: the answer at once, one worker in order, the text and the images of each group, the failures,
// the counters of the result, the queue that is not waited for, and the image limit shared with metadata.apply.
func TestBatchApply(t *testing.T) {
	t.Parallel()

	t.Run("MET-064/em ready vai a applying e devolve accepted na hora", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades", "Dead Cells"}, steam: true})
		gate, open := newGate(t)
		w.steam.block = gate // every application request is held: the state stays applying
		id, ids := w.readyBatch(t)
		res := batchApplyOK(t, w, id, ids)
		if res.Accepted != len(ids) {
			t.Errorf("accepted = %d, want %d (one per choice)", res.Accepted, len(ids))
		}
		awaitFetchOf(t, w.steam, "1")
		wantBatchState(t, w.metaFixture, ipc.BatchStateApplying)
		open()
		awaitDone(t, w, id)
	})

	t.Run("MET-065/aplica em ordem com um só trabalhador: o segundo jogo espera o primeiro", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades", "Dead Cells"}, steam: true})
		gate, open := newGate(t)
		w.steam.holdIDs = map[string]chan struct{}{"1": gate}
		id, ids := w.readyBatch(t)
		batchApplyOK(t, w, id, ids)
		awaitFetchOf(t, w.steam, "1")
		expectNoFetch(t, w.steam, "while the first game is held")
		open()
		awaitDone(t, w, id)
		if got := w.steam.askedIDs(); !slices.Equal(got, []string{"1", "2", "3"}) {
			t.Errorf("Steam Store asked in the order %q, want [1 2 3]", got)
		}
	})

	t.Run("MET-065/IGDB não recebe requisição na aplicação; Steam Store e SteamGridDB só se o grupo os tem", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades", "Dead Cells"}, igdb: true, steam: true})
		id, ids := w.readyBatch(t)
		rows := batchResultsNow(t, w.metaFixture, id).Rows
		searches := w.igdb.count("Search")
		batchApplyOK(t, w, id, ids)
		awaitDone(t, w, id)
		if n := w.igdb.count("Fetch") + w.igdb.count("Details") + w.igdb.count("Media"); n != 0 {
			t.Errorf("IGDB requests in the application = %d, want 0: the group keeps what the search brought", n)
		}
		if n := w.igdb.count("Search") - searches; n != 0 {
			t.Errorf("IGDB searches in the application = %d, want 0", n)
		}
		allowed := refIDs(rows, ipc.MetadataProviderSteamStore)
		for _, got := range w.steam.askedIDs() {
			if !slices.Contains(allowed, got) {
				t.Errorf("Steam Store asked for %q, which no group of the batch has", got)
			}
		}
		// The batch search asks SteamGridDB too (its answers are empty); only the application is measured here.
		if n := w.sgdb.count("Fetch") + w.sgdb.count("Details") + w.sgdb.count("Media"); n != 0 {
			t.Errorf("SteamGridDB fetches in the application = %d, want 0: no group has it", n)
		}
	})

	t.Run("MET-065/replace sempre falso: o campo que o usuário preencheu fica", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades"}, steam: true})
		id, ids := w.readyBatch(t)
		mine := "Meu texto"
		if _, err := w.st.UpdateGame(t.Context(), ids[1], store.GamePatch{Description: &mine}, w.clock.Now()); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		batchApplyOK(t, w, id, ids)
		awaitDone(t, w, id)
		if d := gameText(t, w.st, ids[1]).Description; d != mine {
			t.Errorf("Hades description = %q, want the user's %q", d, mine)
		}
		if d := gameText(t, w.st, ids[0]).Description; d != "Texto da Steam" {
			t.Errorf("Celeste description = %q, want the Steam text", d)
		}
	})

	t.Run("MET-065/um jogo falha: failed+1, título guardado e o próximo segue", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades", "Dead Cells"}, steam: true})
		w.steam.failIDs = map[string]error{"2": scrapers.ErrUnavailable} // Hades
		id, ids := w.readyBatch(t)
		batchApplyOK(t, w, id, ids)
		st := awaitDone(t, w, id)
		if st.Failed != 1 || st.Result.Applied != 2 {
			t.Errorf("failed %d applied %d, want 1 and 2", st.Failed, st.Result.Applied)
		}
		if !slices.Equal(st.Result.FailedTitles, []string{"Hades"}) {
			t.Errorf("failedTitles = %q, want [Hades]", st.Result.FailedTitles)
		}
		if d := gameText(t, w.st, ids[2]).Description; d != "Texto da Steam" {
			t.Errorf("Dead Cells description = %q, want the Steam text: the application goes on after a failure", d)
		}
	})

	t.Run("MET-065/failedTitles guarda no máximo 10 títulos; failed conta todos", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: numberedTitles("Jogo", 11), steam: true})
		w.steam.fetchErr = scrapers.ErrUnavailable
		id, ids := w.readyBatch(t)
		batchApplyOK(t, w, id, ids)
		st := awaitDone(t, w, id)
		if len(st.Result.FailedTitles) != 10 {
			t.Errorf("failedTitles = %d, want 10", len(st.Result.FailedTitles))
		}
		if st.Failed != 12 || st.Result.Applied != 0 {
			t.Errorf("failed %d applied %d, want 12 and 0", st.Failed, st.Result.Applied)
		}
	})

	t.Run("MET-065/jogo apagado conta como falha", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades", "Dead Cells"}, steam: true})
		id, ids := w.readyBatch(t)
		if err := w.st.DeleteGame(t.Context(), ids[1]); err != nil {
			t.Fatalf("DeleteGame: %v", err)
		}
		batchApplyOK(t, w, id, ids)
		st := awaitDone(t, w, id)
		if st.Failed != 1 || st.Result.Applied != 2 || !slices.Contains(st.Result.FailedTitles, "Hades") {
			t.Errorf("status failed %d applied %d failedTitles %q, want 1, 2 and Hades", st.Failed, st.Result.Applied, st.Result.FailedTitles)
		}
	})

	t.Run("MET-065/contadores finais do result e done avisado na hora", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades", "Dead Cells"}, igdb: true, covers: 3})
		id, ids := w.readyBatch(t)
		batchApplyOK(t, w, id, ids)
		st := awaitDone(t, w, id)
		// Each game offers three covers: two fit the limit of MET-021, the third is left out.
		r := st.Result
		if r.Applied != 3 || r.ImagesQueued != 6 || r.ImagesSkippedByLimit != 3 || len(r.FailedTitles) != 0 {
			t.Errorf("result = %+v, want applied 3, imagesQueued 6, imagesSkippedByLimit 3, no failed title", r)
		}
		if st.Done != 3 || st.Total != 3 || st.Failed != 0 {
			t.Errorf("counters done %d total %d failed %d, want 3, 3, 0", st.Done, st.Total, st.Failed)
		}
	})

	t.Run("MET-065/não espera pela fila de mídia: done chega com as imagens na fila", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades"}, igdb: true, covers: 2})
		id, ids := w.readyBatch(t)
		batchApplyOK(t, w, id, ids)
		// The queue never runs in this fixture, so the done notice must come without it.
		awaitDone(t, w, id)
		if n := len(pendingJobs(t, w.st)); n == 0 {
			t.Errorf("queued jobs = 0, want the images of the application waiting in the queue")
		}
	})

	t.Run("MET-022/mesmo jogo com 1 vaga de screenshot recebe 1 pelo lote", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{igdb: true, screenshots: 3})
		storeImages(t, w.st, w.gameID, "igdb", scrapers.KindScreenshot, "old", 9) // one vacancy of the ten
		id, ids := w.readyBatch(t)
		batchApplyOK(t, w, id, ids)
		st := awaitDone(t, w, id)
		if st.Result.ImagesQueued != 1 || st.Result.ImagesSkippedByLimit != 2 {
			t.Errorf("imagesQueued %d skipped %d, want 1 and 2", st.Result.ImagesQueued, st.Result.ImagesSkippedByLimit)
		}
	})

	t.Run("MET-023/lote e metadata.apply concorrentes do mesmo jogo não passam do limite", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{igdb: true, screenshots: 3})
		storeImages(t, w.st, w.gameID, "igdb", scrapers.KindScreenshot, "old", 9) // one vacancy of the ten
		id, ids := w.readyBatch(t)
		params := applyJSON(t, ipc.MetadataApplyParams{GameID: w.gameID, Refs: refsOf("100", ipc.MetadataProviderIGDB)})
		ctx := t.Context()
		h := w.handlers[ipc.MethodMetadataApply]
		var (
			wg        sync.WaitGroup
			single    any
			singleErr error
		)
		wg.Add(1)
		go func() {
			defer wg.Done()
			single, singleErr = h(ctx, json.RawMessage(params))
		}()
		batchApplyOK(t, w, id, ids)
		waitReturns(t, "the concurrent metadata.apply", wg.Wait)
		st := awaitDone(t, w, id)
		if singleErr != nil {
			t.Fatalf("metadata.apply: %v", singleErr)
		}
		res, ok := single.(ipc.MetadataApplyResult)
		if !ok {
			t.Fatalf("result type %T, want ipc.MetadataApplyResult", single)
		}
		if total := res.Queued + st.Result.ImagesQueued; total != 1 {
			t.Errorf("images queued by both paths = %d, want 1 (one vacancy)", total)
		}
		screenshots := 0
		for _, j := range pendingJobs(t, w.st) {
			if j.Kind == scrapers.KindScreenshot {
				screenshots++
			}
		}
		if screenshots != 1 {
			t.Errorf("screenshot jobs waiting = %d, want 1", screenshots)
		}
	})

	t.Run("MET-066/cancelar no meio: nenhum jogo novo, o em curso termina, o aplicado fica, done com resultado parcial", func(t *testing.T) {
		t.Parallel()
		w := newApplyWorld(t, applyCfg{titles: []string{"Hades", "Dead Cells"}, steam: true})
		gate, open := newGate(t)
		w.steam.holdIDs = map[string]chan struct{}{"2": gate} // Hades is in progress when the cancel comes
		id, ids := w.readyBatch(t)
		batchApplyOK(t, w, id, ids)
		awaitBatchDone(t, w.spy, id, 1) // Celeste is applied and counted
		awaitFetchOf(t, w.steam, "2")

		if state := cancelBatch(t, w.metaFixture, id); state != ipc.BatchStateDone {
			t.Errorf("batchCancel state = %q, want done at once", state)
		}
		wantBatchState(t, w.metaFixture, ipc.BatchStateDone)
		open()
		waitReturns(t, "the application", w.meta.Wait)

		if got := w.steam.askedIDs(); !slices.Equal(got, []string{"1", "2"}) {
			t.Errorf("Steam Store asked %q, want [1 2]: no game starts after the cancel", got)
		}
		if d := gameText(t, w.st, ids[2]).Description; d != "" {
			t.Errorf("Dead Cells description = %q after the cancel, want none", d)
		}
		if d := gameText(t, w.st, ids[0]).Description; d != "Texto da Steam" {
			t.Errorf("Celeste description = %q, want the text applied before the cancel", d)
		}
		b := batchStatusNow(t, w.metaFixture)
		if b == nil || b.Result == nil || b.Result.Applied < 1 || b.Result.Applied > 2 {
			t.Errorf("batch = %+v, want done with a partial result (1 or 2 applied)", b)
		}
	})

	t.Run("MET-066/batchCancel em done é no-op que devolve o estado", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateDone)
		before := len(c.f.spy.all())
		if got := cancelBatch(t, c.f.metaFixture, c.id); got != ipc.BatchStateDone {
			t.Errorf("batchCancel state = %q, want done", got)
		}
		wantBatchState(t, c.f.metaFixture, ipc.BatchStateDone)
		if n := len(c.f.spy.all()); n != before {
			t.Errorf("%d notifications from a no-op cancel, want none", n-before)
		}
	})

	t.Run("MET-066/batchDiscard em done libera", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateDone)
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

// TestBatchApplyShutdown checks the shutdown during applying: the root context ends the application, Wait
// returns with the worker done, what was written stays, and no request starts after the shutdown [MET-059].
// It is not parallel: goleak counts every goroutine of the process.
func TestBatchApplyShutdown(t *testing.T) {
	w := newApplyWorld(t, applyCfg{titles: []string{"Hades", "Dead Cells"}, steam: true})
	gate, _ := newGate(t)
	w.steam.holdIDs = map[string]chan struct{}{"2": gate}
	snap := goleak.IgnoreCurrent()

	root, stop := context.WithCancel(t.Context())
	defer stop()
	m := newMetadataService(w.svc, w.st, root)
	m.notify = w.spy
	handlers := metadataHandlers(m)

	ids := batchGames(t, w.batchFixture, w.cfg.titles...)
	out, err := handlers[ipc.MethodMetadataBatchStart](t.Context(), json.RawMessage(batchJSON(t, ipc.BatchStartParams{GameIDs: ids})))
	if err != nil {
		t.Fatalf("batchStart: %v", err)
	}
	start, ok := out.(ipc.BatchStartResult)
	if !ok {
		t.Fatalf("result type %T, want ipc.BatchStartResult", out)
	}
	awaitBatchState(t, w.spy, start.BatchID, ipc.BatchStateReady)

	if _, err := handlers[ipc.MethodMetadataBatchApply](t.Context(), json.RawMessage(applyParams(t, start.BatchID, ids))); err != nil {
		t.Fatalf("batchApply: %v", err)
	}
	awaitBatchDone(t, w.spy, start.BatchID, 1) // Celeste is written before the shutdown
	awaitFetchOf(t, w.steam, "2")

	stop()
	waitReturns(t, "the service", m.Wait)
	if got := w.steam.askedIDs(); !slices.Equal(got, []string{"1", "2"}) {
		t.Errorf("Steam Store asked %q, want [1 2]: nothing new after the shutdown", got)
	}
	if d := gameText(t, w.st, ids[2]).Description; d != "" {
		t.Errorf("Dead Cells description = %q after the shutdown, want none", d)
	}
	if d := gameText(t, w.st, ids[0]).Description; d != "Texto da Steam" {
		t.Errorf("Celeste description = %q, want the text written before the shutdown", d)
	}
	goleak.VerifyNone(t, snap)
}
