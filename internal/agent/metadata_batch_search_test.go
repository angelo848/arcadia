package agent

// Tests of T24 (phase 12): the health of a provider during a batch (MET-011), the Staged search of
// a batch game (MET-015) and the row kept for each game (MET-053), all through metadataService.searchOne
// and providerHealth. Providers are fakes (metaProv, metaCand of metadata_test.go); the store is real,
// in t.TempDir(); the clock is a domain.FakeClock. There is no time.Sleep: a short real wait only
// proves that something did NOT happen, and it is always a channel select with a timeout.
// Alterado em 2026-11-15: arquivo novo (fase 12, T24).

import (
	"fmt"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// healthStart is the instant the health tests start at.
var healthStart = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// searchGame runs searchOne for one game and fails the test on an error; it returns the row and
// whether every provider failed.
func searchGame(t *testing.T, f metaFixture, h *providerHealth, gameID int64) (batchRow, bool) {
	t.Helper()
	row, failedAll, err := f.meta.searchOne(t.Context(), gameID, h)
	if err != nil {
		t.Fatalf("searchOne(%d): %v", gameID, err)
	}
	return row, failedAll
}

// TestProviderHealth checks the suspensions and the disabling of providerHealth on its own, with a
// fake clock [MET-011].
func TestProviderHealth(t *testing.T) {
	t.Parallel()

	t.Run("MET-011/falha temporária suspende 30 s, depois 60 s, e a 3ª seguida desativa até o fim do lote", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(healthStart)
		h := newProviderHealth(clock)
		if !h.allowed("igdb") {
			t.Fatal("igdb not allowed before any failure")
		}

		if h.report("igdb", scrapers.ErrUnavailable) {
			t.Fatal("1st failure disabled the provider, want a suspension only")
		}
		if h.allowed("igdb") {
			t.Fatal("allowed right after the 1st failure, want suspended for 30 s")
		}
		clock.Advance(29 * time.Second)
		if h.allowed("igdb") {
			t.Fatal("allowed at 29 s, want suspended until 30 s")
		}
		clock.Advance(time.Second)
		if !h.allowed("igdb") {
			t.Fatal("not allowed at 30 s, want the suspension over")
		}

		if h.report("igdb", scrapers.ErrUnavailable) {
			t.Fatal("2nd failure disabled the provider, want a suspension of 60 s")
		}
		clock.Advance(59 * time.Second)
		if h.allowed("igdb") {
			t.Fatal("allowed at 59 s of the 60 s suspension")
		}
		clock.Advance(time.Second)
		if !h.allowed("igdb") {
			t.Fatal("not allowed at 60 s, want the suspension over")
		}

		// The 3rd failure in a row disables. There is no 120 s step (alterado em 2026-11-15, MET-011).
		if !h.report("igdb", scrapers.ErrUnavailable) {
			t.Fatal("3rd failure in a row did not disable the provider")
		}
		if h.allowed("igdb") {
			t.Fatal("allowed right after the 3rd failure")
		}
		clock.Advance(time.Hour)
		if h.allowed("igdb") {
			t.Fatal("disabled provider came back: it must stay off until the end of the batch")
		}
	})

	t.Run("MET-011/sucesso zera a contagem", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(healthStart)
		h := newProviderHealth(clock)
		h.report("igdb", scrapers.ErrUnavailable)
		clock.Advance(30 * time.Second)
		if h.report("igdb", nil) {
			t.Fatal("a success disabled the provider")
		}
		if !h.allowed("igdb") {
			t.Fatal("not allowed after a success")
		}

		// Starting again from zero: two failures in a row do not disable, and the next suspension is 30 s.
		if h.report("igdb", scrapers.ErrUnavailable) {
			t.Fatal("failure after a success disabled the provider")
		}
		if h.allowed("igdb") {
			t.Fatal("allowed right after the failure")
		}
		clock.Advance(29 * time.Second)
		if h.allowed("igdb") {
			t.Fatal("allowed at 29 s: the count was not reset, the suspension grew")
		}
		clock.Advance(time.Second)
		if !h.allowed("igdb") {
			t.Fatal("not allowed at 30 s: the suspension is not 30 s after a success")
		}
		if h.report("igdb", scrapers.ErrUnavailable) {
			t.Fatal("2nd failure after a success disabled the provider")
		}
	})

	t.Run("MET-011/provedores têm contagens independentes", func(t *testing.T) {
		t.Parallel()
		clock := domain.NewFakeClock(healthStart)
		h := newProviderHealth(clock)

		h.report("igdb", scrapers.ErrUnavailable)
		clock.Advance(30 * time.Second)
		h.report("igdb", scrapers.ErrUnavailable)
		if h.report("steamgriddb", scrapers.ErrUnavailable) {
			t.Fatal("the first failure of steamgriddb disabled it: counts are shared")
		}
		if h.allowed("steamgriddb") {
			t.Fatal("steamgriddb allowed right after its first failure")
		}
		if !h.allowed("steamstore") {
			t.Fatal("steamstore suspended without any failure of its own")
		}

		clock.Advance(60 * time.Second)
		if !h.report("igdb", scrapers.ErrUnavailable) {
			t.Fatal("3rd failure of igdb did not disable it")
		}
		clock.Advance(time.Hour)
		if h.allowed("igdb") {
			t.Fatal("igdb came back after being disabled")
		}
		if !h.allowed("steamgriddb") {
			t.Fatal("steamgriddb still suspended an hour after its only failure")
		}
	})
}

// TestBatchSearchOne checks the search of one game of a batch through metadataService.searchOne
// [MET-011, MET-015, MET-053].
func TestBatchSearchOne(t *testing.T) {
	t.Parallel()

	t.Run("MET-011/falha temporária suspende 30 s, depois 60 s, e a 3ª desativa (nos jogos do lote)", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb", searchErr: scrapers.ErrUnavailable}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		ids := append([]int64{f.gameID}, addGames(t, f, "Celeste", "Celeste", "Celeste", "Celeste", "Celeste")...)
		h := newProviderHealth(f.clock)

		// step searches game i and checks how many times IGDB has been asked by then.
		step := func(i, wantAsked int) bool {
			t.Helper()
			row, failedAll := searchGame(t, f, h, ids[i])
			if n := igdb.count("Search"); n != wantAsked {
				t.Fatalf("after game %d: IGDB asked %d times, want %d", i, n, wantAsked)
			}
			if row.confidence != domain.ConfidenceNone {
				t.Fatalf("game %d: confidence %q, want none", i, row.confidence)
			}
			return failedAll
		}

		if !step(0, 1) {
			t.Fatal("game 0: the only provider failed, want failedAll")
		}
		step(1, 1) // suspended for 30 s: IGDB is not asked again
		f.clock.Advance(30 * time.Second)
		step(2, 2) // 2nd failure in a row: suspended for 60 s
		f.clock.Advance(59 * time.Second)
		step(3, 2)
		f.clock.Advance(time.Second)
		step(4, 3) // 3rd failure in a row: disabled for the rest of the batch
		f.clock.Advance(time.Hour)
		step(5, 3)
	})

	t.Run("MET-011/durante a suspensão, os jogos seguem com os demais provedores", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb", searchErr: scrapers.ErrUnavailable}
		steam := &metaProv{id: "steamstore", cands: []scrapers.Candidate{
			metaCand("steamstore", "620", "Celeste", 0, "Windows", "windows"),
		}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamstore": steam})
		ids := append([]int64{f.gameID}, addGames(t, f, "Celeste")...)
		h := newProviderHealth(f.clock)

		searchGame(t, f, h, ids[0]) // IGDB fails: suspended; the Steam Store answers
		row, _ := searchGame(t, f, h, ids[1])

		if n := igdb.count("Search"); n != 1 {
			t.Errorf("IGDB asked %d times, want 1 (suspended during the second game)", n)
		}
		if n := steam.count("Search"); n != 2 {
			t.Errorf("Steam Store asked %d times, want 2 (one per game)", n)
		}
		if len(row.groups) == 0 {
			t.Error("second game has no group: the answer of the Steam Store was lost")
		}
	})

	t.Run("MET-011/jogo cuja busca falhou em todos os provedores aplicáveis fica none", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb", searchErr: scrapers.ErrUnavailable}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		h := newProviderHealth(f.clock)

		row, failedAll := searchGame(t, f, h, f.gameID)
		if !failedAll {
			t.Error("failedAll = false, want true: every applicable provider failed")
		}
		if row.confidence != domain.ConfidenceNone {
			t.Errorf("confidence = %q, want none", row.confidence)
		}
		if row.selected != -1 || len(row.groups) != 0 {
			t.Errorf("row = selected %d, %d groups; want -1 and no groups", row.selected, len(row.groups))
		}
	})

	t.Run("MET-015/IGDB match: zero chamadas à Steam Store", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb", cands: []scrapers.Candidate{metaCand("igdb", "1", "Celeste", 0, "Windows", "windows")}}
		steam := &metaProv{id: "steamstore", cands: []scrapers.Candidate{metaCand("steamstore", "620", "Celeste", 0, "Windows", "windows")}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamstore": steam})
		h := newProviderHealth(f.clock)

		row, _ := searchGame(t, f, h, f.gameID)
		if n := steam.count("Search"); n != 0 {
			t.Errorf("Steam Store asked %d times, want 0 (IGDB already gave a match)", n)
		}
		if row.confidence != domain.ConfidenceMatch {
			t.Errorf("confidence = %q, want match", row.confidence)
		}
	})

	t.Run("MET-015/IGDB doubt: uma chamada à Steam Store", func(t *testing.T) {
		t.Parallel()
		// "Resident Evil 2" against "Resident Evil" without platform: a doubt (as in the aggregator tests).
		igdb := &metaProv{id: "igdb", cands: []scrapers.Candidate{metaCand("igdb", "2", "Resident Evil 2", 0, "")}}
		steam := &metaProv{id: "steamstore"}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb, "steamstore": steam})
		ids := addGames(t, f, "Resident Evil")
		h := newProviderHealth(f.clock)

		searchGame(t, f, h, ids[0])
		if n := steam.count("Search"); n != 1 {
			t.Errorf("Steam Store asked %d times, want 1 (IGDB gave only a doubt)", n)
		}
	})

	t.Run("MET-015/sem chave do IGDB: uma chamada à Steam Store", func(t *testing.T) {
		t.Parallel()
		steam := &metaProv{id: "steamstore", cands: []scrapers.Candidate{metaCand("steamstore", "620", "Celeste", 0, "Windows", "windows")}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}, "steamstore": steam}, "igdb")
		h := newProviderHealth(f.clock)

		searchGame(t, f, h, f.gameID)
		if n := steam.count("Search"); n != 1 {
			t.Errorf("Steam Store asked %d times, want 1 (no IGDB key)", n)
		}
	})

	t.Run("MET-053/a consulta usa título, plataforma e ano gravados do jogo", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb"}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		year := 2018
		yp := &year
		if _, err := f.st.UpdateGame(t.Context(), f.gameID, store.GamePatch{Year: &yp}, f.clock.Now()); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		d := gameText(t, f.st, f.gameID)
		h := newProviderHealth(f.clock)

		searchGame(t, f, h, f.gameID)
		q, ok := igdb.lastQuery()
		if !ok {
			t.Fatal("IGDB was not asked")
		}
		if want := domain.SearchTerm("Celeste"); q.Title != want {
			t.Errorf("query title = %q, want %q", q.Title, want)
		}
		if d.Platform == "" || q.Platform != d.Platform {
			t.Errorf("query platform = %q, want the game's platform %q", q.Platform, d.Platform)
		}
		if q.SeedKey != "windows" {
			t.Errorf("query seed key = %q, want windows", q.SeedKey)
		}
		if q.Year != 2018 {
			t.Errorf("query year = %d, want 2018", q.Year)
		}
	})

	t.Run("MET-053/a linha guarda confiança, item selecionado (o primeiro) e até 5 alternativas ≥ 0,40", func(t *testing.T) {
		t.Parallel()
		// Seven near-identical titles on Windows: all of them score at least 0,40 and the row keeps five.
		cands := []scrapers.Candidate{metaCand("igdb", "1", "Celeste", 0, "Windows", "windows")}
		for i := 2; i <= 7; i++ {
			cands = append(cands, metaCand("igdb", fmt.Sprint(i), fmt.Sprintf("Celeste %d", i), 0, "Windows", "windows"))
		}
		igdb := &metaProv{id: "igdb", cands: cands}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		h := newProviderHealth(f.clock)

		row, _ := searchGame(t, f, h, f.gameID)
		if row.confidence == domain.ConfidenceNone {
			t.Fatal("confidence none for seven matching candidates")
		}
		if len(row.groups) != 5 {
			t.Errorf("row keeps %d groups, want 5 (the limit of the row)", len(row.groups))
		}
		if row.selected != 0 {
			t.Errorf("selected = %d, want 0 (the first item)", row.selected)
		}
		for i, g := range row.groups {
			if g.Score.Score < domain.MatchAlternative {
				t.Errorf("group %d score %d, want at least %d", i, g.Score.Score, domain.MatchAlternative)
			}
		}
	})

	t.Run("MET-053/none: selected -1", func(t *testing.T) {
		t.Parallel()
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": &metaProv{id: "igdb"}})
		h := newProviderHealth(f.clock)

		row, failedAll := searchGame(t, f, h, f.gameID)
		if failedAll {
			t.Error("failedAll = true, want false: the provider answered, with nothing found")
		}
		if row.confidence != domain.ConfidenceNone || row.selected != -1 || len(row.groups) != 0 {
			t.Errorf("row = confidence %q, selected %d, %d groups; want none, -1, no groups",
				row.confidence, row.selected, len(row.groups))
		}
	})

	t.Run("MET-053/jogo apagado: linha none sem alternativas", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb", cands: []scrapers.Candidate{metaCand("igdb", "1", "Celeste", 0, "Windows", "windows")}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		h := newProviderHealth(f.clock)

		row, _, err := f.meta.searchOne(t.Context(), 999, h)
		if err != nil {
			t.Fatalf("searchOne of a deleted game: %v, want a none row", err)
		}
		if row.confidence != domain.ConfidenceNone || row.selected != -1 || len(row.groups) != 0 {
			t.Errorf("row = confidence %q, selected %d, %d groups; want none, -1, no groups",
				row.confidence, row.selected, len(row.groups))
		}
	})

	t.Run("MET-053/prazo de 15 s por jogo", func(t *testing.T) {
		t.Parallel()
		igdb := &metaProv{id: "igdb", block: true, started: make(chan struct{}, 1)}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		h := newProviderHealth(f.clock)

		type result struct {
			row       batchRow
			failedAll bool
			err       error
		}
		done := make(chan result, 1)
		ctx := t.Context()
		go func() {
			row, failedAll, err := f.meta.searchOne(ctx, f.gameID, h)
			done <- result{row: row, failedAll: failedAll, err: err}
		}()
		waitSignal(t, igdb.started, "the IGDB request of the game")

		f.clock.Advance(14 * time.Second)
		select {
		case r := <-done:
			t.Fatalf("the search ended at 14 s (%+v), want it to run until 15 s", r)
		case <-time.After(20 * time.Millisecond): // still running: a negative check with a short timeout
		}

		f.clock.Advance(time.Second)
		select {
		case r := <-done:
			if r.err != nil {
				t.Fatalf("searchOne: %v", r.err)
			}
			if !r.failedAll || r.row.confidence != domain.ConfidenceNone {
				t.Errorf("at 15 s: failedAll %v, confidence %q; want true and none", r.failedAll, r.row.confidence)
			}
		case <-time.After(testTimeout):
			t.Fatal("the search did not end at 15 s")
		}
	})

	t.Run("MET-053/a linha guarda as imagens que cada provedor devolveu (melhores imagens)", func(t *testing.T) {
		t.Parallel()
		c := metaCand("igdb", "1", "Celeste", 0, "Windows", "windows")
		c.Images = []scrapers.RemoteMedia{
			{Kind: "cover", URL: "https://images.igdb.com/igdb/image/upload/t_cover_big/co1.jpg"},
			{Kind: "screenshot", URL: "https://images.igdb.com/igdb/image/upload/t_screenshot_big/sc1.jpg"},
		}
		igdb := &metaProv{id: "igdb", cands: []scrapers.Candidate{c}}
		f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": igdb})
		h := newProviderHealth(f.clock)

		row, _ := searchGame(t, f, h, f.gameID)
		if len(row.groups) != 1 {
			t.Fatalf("groups = %d, want 1", len(row.groups))
		}
		imgs := row.groups[0].Images["igdb"]
		if len(imgs) != 2 || imgs[0].URL != c.Images[0].URL || imgs[1].URL != c.Images[1].URL {
			t.Errorf("IGDB images in the row = %+v, want the two the provider returned (no second request)", imgs)
		}
	})
}
