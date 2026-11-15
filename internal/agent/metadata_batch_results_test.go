package agent

// Tests of metadata.batchResults (phase 12, T26): the pages of the rows of a ready or cancelled batch under
// the ceiling of maxResultBytes (MET-057), the states and parameters that refuse the call (MET-057, MET-071),
// the thumbnails of the rows (MET-068) and pageRows on its own. Providers are fakes (applyProv, metaCand); the
// store is real, in t.TempDir(). There is no time.Sleep and no network.
// Alterado em 2026-11-15: arquivo novo (fase 12, T26).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// resultsFixture is a batch over the IGDB fake alone: echo answers the search of each title (nil: none).
func resultsFixture(t *testing.T, echo func(q scrapers.Query) []scrapers.Candidate) batchFixture {
	t.Helper()
	f := newMetaFixture(t, map[string]scrapers.MetadataProvider{"igdb": newApplyProv("igdb", echo)})
	spy := newBatchSpy()
	f.meta.notify = spy
	return batchFixture{metaFixture: f, spy: spy}
}

// answerAll is an echo that answers every title with the candidate of the batch tests.
func answerAll(q scrapers.Query) []scrapers.Candidate {
	return []scrapers.Candidate{batchEcho(q)}
}

// readyRows starts the batch of the fixture's game and the titles, and waits for ready. It returns the batch
// id and the game ids, in the order sent.
func readyRows(t *testing.T, f batchFixture, titles ...string) (int64, []int64) {
	t.Helper()
	ids := batchGames(t, f, titles...)
	res := startBatchOK(t, f, ids...)
	awaitBatchState(t, f.spy, res.BatchID, ipc.BatchStateReady)
	return res.BatchID, ids
}

// resultsPage runs metadata.batchResults with the offset and the limit (0 = absent) and returns the result.
func resultsPage(t *testing.T, f metaFixture, id int64, offset, limit int) ipc.BatchResultsResult {
	t.Helper()
	params := ipc.BatchResultsParams{BatchID: id, Offset: offset}
	if limit != 0 { // 0 here means absent: the default of 100 [MET-071]
		params.Limit = &limit
	}
	out, err := f.call(t, ipc.MethodMetadataBatchResults, batchJSON(t, params))
	if err != nil {
		t.Fatalf("metadata.batchResults(offset %d, limit %d): %v", offset, limit, err)
	}
	res, ok := out.(ipc.BatchResultsResult)
	if !ok {
		t.Fatalf("result type %T, want ipc.BatchResultsResult", out)
	}
	return res
}

// walkPages reads every page of batch id with the limit, following next, and returns the rows in order. It
// fails the test when a page is over the limit or when next does not follow the page.
func walkPages(t *testing.T, f metaFixture, id int64, limit int) []ipc.BatchRow {
	t.Helper()
	var all []ipc.BatchRow
	offset := 0
	for range 1000 {
		res := resultsPage(t, f, id, offset, limit)
		if len(res.Rows) > limit {
			t.Fatalf("page at %d has %d rows, want at most %d", offset, len(res.Rows), limit)
		}
		all = append(all, res.Rows...)
		if res.Next == nil {
			return all
		}
		if *res.Next != offset+len(res.Rows) {
			t.Fatalf("next = %d after %d rows from %d, want %d", *res.Next, len(res.Rows), offset, offset+len(res.Rows))
		}
		offset = *res.Next
	}
	t.Fatal("the pages never end")
	return nil
}

// gameIDsOf returns the game ids of the rows, in order.
func gameIDsOf(rows []ipc.BatchRow) []int64 {
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.GameID)
	}
	return out
}

// checkSelected checks the selected index of a row: -1 only when there is no match [MET-061]; otherwise an index
// into its candidates, and the first one for a doubt (the row comes with it already chosen).
func checkSelected(t *testing.T, r ipc.BatchRow) {
	t.Helper()
	if r.Confidence == ipc.MetadataConfidenceNone {
		if r.Selected != -1 {
			t.Errorf("game %d: selected = %d with confidence none, want -1", r.GameID, r.Selected)
		}
		return
	}
	if r.Selected < 0 || r.Selected >= len(r.Candidates) {
		t.Errorf("game %d: selected = %d with %d candidates, want an index into them", r.GameID, r.Selected, len(r.Candidates))
	}
	if r.Confidence == ipc.MetadataConfidenceDoubt && r.Selected != 0 {
		t.Errorf("game %d: doubt selects %d, want 0 (the first candidate comes chosen)", r.GameID, r.Selected)
	}
}

// TestBatchResults checks metadata.batchResults on the batches of each state and its paging [MET-057, MET-071].
func TestBatchResults(t *testing.T) {
	t.Parallel()

	for _, state := range []string{ipc.BatchStateSearching, ipc.BatchStateApplying, ipc.BatchStateDone} {
		t.Run("MET-057/estado "+state+": -32016 com data.state", func(t *testing.T) {
			t.Parallel()
			c := reachBatch(t, state)
			_, err := c.f.call(t, ipc.MethodMetadataBatchResults, batchJSON(t, ipc.BatchResultsParams{BatchID: c.id}))
			if codeOf(err) != ipc.CodeBatchWrongState {
				t.Fatalf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeBatchWrongState)
			}
			if d := errData(t, err); d["state"] != state {
				t.Errorf("data.state = %q, want %q", d["state"], state)
			}
		})
	}

	t.Run("MET-057/sem busca: -32014", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, batchNone)
		_, err := c.f.call(t, ipc.MethodMetadataBatchResults, batchJSON(t, ipc.BatchResultsParams{BatchID: c.id}))
		if codeOf(err) != ipc.CodeBatchNotFound {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeBatchNotFound)
		}
	})

	t.Run("MET-057/batchId desconhecido em ready: -32014", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateReady)
		_, err := c.f.call(t, ipc.MethodMetadataBatchResults, batchJSON(t, ipc.BatchResultsParams{BatchID: c.id + 1000}))
		if codeOf(err) != ipc.CodeBatchNotFound {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeBatchNotFound)
		}
	})

	t.Run("MET-071/offset negativo, limit negativo, limit 201 e batchId 0: -32602", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateReady)
		params := []string{
			fmt.Sprintf(`{"batchId":%d,"offset":-1}`, c.id),
			fmt.Sprintf(`{"batchId":%d,"limit":-1}`, c.id),
			fmt.Sprintf(`{"batchId":%d,"limit":201}`, c.id),
			`{"batchId":0}`,
		}
		for _, p := range params {
			_, err := c.f.call(t, ipc.MethodMetadataBatchResults, p)
			if codeOf(err) != ipc.CodeInvalidParams {
				t.Errorf("params %s: code = %d (%v), want %d", p, codeOf(err), err, ipc.CodeInvalidParams)
			}
		}
	})

	t.Run("MET-071/limit 0 explícito em ready: -32602 (ausente é 100)", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateReady)
		zero := 0
		_, err := c.f.call(t, ipc.MethodMetadataBatchResults, batchJSON(t, ipc.BatchResultsParams{BatchID: c.id, Limit: &zero}))
		if codeOf(err) != ipc.CodeInvalidParams {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeInvalidParams)
		}
	})

	t.Run("MET-071/limit 0 e 201 em searching: -32602 antes da checagem de estado", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateSearching)
		zero, big := 0, 201
		for _, limit := range []*int{&zero, &big} {
			_, err := c.f.call(t, ipc.MethodMetadataBatchResults, batchJSON(t, ipc.BatchResultsParams{BatchID: c.id, Limit: limit}))
			if codeOf(err) != ipc.CodeInvalidParams {
				t.Errorf("limit %d: code = %d (%v), want %d", *limit, codeOf(err), err, ipc.CodeInvalidParams)
			}
		}
	})

	t.Run("MET-058/batchResults depois do descarte: -32014", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateReady)
		if _, err := c.f.call(t, ipc.MethodMetadataBatchDiscard, batchJSON(t, ipc.BatchIDParams{BatchID: c.id})); err != nil {
			t.Fatalf("batchDiscard: %v", err)
		}
		_, err := c.f.call(t, ipc.MethodMetadataBatchResults, batchJSON(t, ipc.BatchResultsParams{BatchID: c.id}))
		if codeOf(err) != ipc.CodeBatchNotFound {
			t.Errorf("code = %d (%v), want %d", codeOf(err), err, ipc.CodeBatchNotFound)
		}
	})

	t.Run("MET-057/em ready: linhas na ordem dos jogos, total e next nulo numa página só", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateReady)
		res := batchResultsNow(t, c.f.metaFixture, c.id)
		if !slices.Equal(gameIDsOf(res.Rows), c.ids) {
			t.Errorf("rows = %v, want the games in order %v", gameIDsOf(res.Rows), c.ids)
		}
		if res.Total != len(c.ids) || res.Next != nil {
			t.Errorf("total %d next %v, want %d and null", res.Total, res.Next, len(c.ids))
		}
	})

	t.Run("MET-057/em cancelled devolve só as linhas que existem", func(t *testing.T) {
		t.Parallel()
		c := reachBatch(t, ipc.BatchStateCancelled)
		res := batchResultsNow(t, c.f.metaFixture, c.id)
		if len(res.Rows) != 1 || res.Rows[0].GameID != c.f.gameID {
			t.Errorf("rows = %v, want only the game searched before the cancel (%d)", gameIDsOf(res.Rows), c.f.gameID)
		}
		// The total of a cancelled batch counts the rows that exist: the games still to search have none.
		if res.Total != len(res.Rows) || res.Next != nil {
			t.Errorf("total %d next %v, want %d and null", res.Total, res.Next, len(res.Rows))
		}
	})

	t.Run("MET-057/limit ausente usa 100; 200 devolve o resto; a página seguinte começa em next", func(t *testing.T) {
		t.Parallel()
		f := resultsFixture(t, answerAll)
		id, _ := readyRows(t, f, numberedTitles("Jogo", 101)...) // 102 games
		first := resultsPage(t, f.metaFixture, id, 0, 0)
		if len(first.Rows) != 100 || first.Next == nil || *first.Next != 100 || first.Total != 102 {
			t.Errorf("default page: rows %d next %v total %d, want 100, 100 and 102", len(first.Rows), first.Next, first.Total)
		}
		all := resultsPage(t, f.metaFixture, id, 0, 200)
		if len(all.Rows) != 102 || all.Next != nil {
			t.Errorf("limit 200: rows %d next %v, want 102 and null", len(all.Rows), all.Next)
		}
		rest := resultsPage(t, f.metaFixture, id, 100, 200)
		if len(rest.Rows) != 2 || rest.Next != nil {
			t.Errorf("offset 100: rows %d next %v, want 2 and null", len(rest.Rows), rest.Next)
		}
	})

	t.Run("MET-057/união das páginas é todas as linhas, na ordem, sem repetir nem pular", func(t *testing.T) {
		t.Parallel()
		f := resultsFixture(t, answerAll)
		id, ids := readyRows(t, f, numberedTitles("Jogo", 101)...)
		rows := walkPages(t, f.metaFixture, id, 40)
		if !slices.Equal(gameIDsOf(rows), ids) {
			t.Errorf("union of the pages = %d rows %v, want the %d games in order", len(rows), gameIDsOf(rows), len(ids))
		}
	})

	t.Run("MET-057/offset além do fim: rows como lista vazia e next nulo", func(t *testing.T) {
		t.Parallel()
		f := resultsFixture(t, answerAll)
		id, _ := readyRows(t, f, numberedTitles("Jogo", 101)...)
		res := resultsPage(t, f.metaFixture, id, 500, 10)
		if len(res.Rows) != 0 || res.Next != nil || res.Total != 102 {
			t.Errorf("rows %d next %v total %d, want 0, null and 102", len(res.Rows), res.Next, res.Total)
		}
		raw, err := json.Marshal(res)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(raw), `"rows":[]`) {
			t.Errorf("JSON = %s, want rows as an empty array", raw)
		}
	})

	t.Run("MET-057/selected é -1 só sem match; nos outros é um índice válido, e doubt vem com o primeiro", func(t *testing.T) {
		t.Parallel()
		echo := func(q scrapers.Query) []scrapers.Candidate {
			if q.Title == domain.SearchTerm("Hades") {
				return []scrapers.Candidate{batchEcho(q)}
			}
			return nil
		}
		f := resultsFixture(t, echo)
		id, _ := readyRows(t, f, "Hades", "Dead Cells")
		rows := resultsPage(t, f.metaFixture, id, 0, 0).Rows
		if len(rows) != 3 {
			t.Fatalf("rows = %d, want 3", len(rows))
		}
		for _, r := range rows {
			checkSelected(t, r)
		}
		// Celeste and Dead Cells have no candidate: none, selected -1, and an empty list of candidates.
		for _, i := range []int{0, 2} {
			if rows[i].Confidence != ipc.MetadataConfidenceNone || rows[i].Selected != -1 {
				t.Errorf("game %d: confidence %q selected %d, want none and -1", rows[i].GameID, rows[i].Confidence, rows[i].Selected)
			}
			raw, err := json.Marshal(rows[i])
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.Contains(string(raw), `"candidates":[]`) {
				t.Errorf("row JSON = %s, want candidates as an empty array", raw)
			}
		}
	})

	t.Run("MET-068/thumbUrl só dos hosts https permitidos; os outros viram vazio", func(t *testing.T) {
		t.Parallel()
		const allowed = "https://images.igdb.com/igdb/image/upload/t_thumb/a.jpg"
		echo := func(q scrapers.Query) []scrapers.Candidate {
			withThumb := func(id, thumb string) scrapers.Candidate {
				c := metaCand("igdb", id, q.Title, 0, "")
				c.ThumbURL = thumb
				return c
			}
			return []scrapers.Candidate{
				withThumb("1", allowed),
				withThumb("2", "http://images.igdb.com/igdb/image/upload/t_thumb/b.jpg"),
				withThumb("3", "https://evil.example.test/c.jpg"),
			}
		}
		f := resultsFixture(t, echo)
		id, _ := readyRows(t, f) // the fixture's game only
		rows := resultsPage(t, f.metaFixture, id, 0, 0).Rows
		if len(rows) != 1 {
			t.Fatalf("rows = %d, want 1", len(rows))
		}
		var thumbs []string
		for _, c := range rows[0].Candidates {
			if c.ThumbURL != "" {
				thumbs = append(thumbs, c.ThumbURL)
			}
		}
		if !slices.Contains(thumbs, allowed) {
			t.Errorf("thumbUrls = %q, want the https igdb one", thumbs)
		}
		for _, th := range thumbs {
			if th != allowed {
				t.Errorf("thumbUrl %q passed the host filter", th)
			}
		}
	})
}

// realRow is a row at the limits of validation: 200-rune titles, 100-rune platforms, five candidates of three
// refs each, thumbnails of at most 300 characters. Every page of such rows must fit the ceiling.
func realRow(id int64) ipc.BatchRow {
	title := strings.Repeat("T", 200)
	platform := strings.Repeat("P", 100)
	thumb := "https://images.igdb.com/" + strings.Repeat("a", 250)
	refs := []ipc.MetadataRef{
		{Provider: ipc.MetadataProviderIGDB, ProviderID: "12345678901234567890"},
		{Provider: ipc.MetadataProviderSteamStore, ProviderID: "12345678901234567890"},
		{Provider: ipc.MetadataProviderSteamGridDB, ProviderID: "12345678901234567890"},
	}
	cands := make([]ipc.MetadataCandidate, 5)
	for i := range cands {
		cands[i] = ipc.MetadataCandidate{Refs: refs, Title: title, Year: 2100, Platform: platform, Score: 1, ThumbURL: thumb}
	}
	return ipc.BatchRow{GameID: id, Title: title, Platform: platform, Confidence: ipc.MetadataConfidenceMatch, Selected: 0, Candidates: cands}
}

// fatRow is a row over the limits (a title of n bytes, no candidates): pageRows must still cut the pages by bytes.
func fatRow(id int64, n int) ipc.BatchRow {
	return ipc.BatchRow{
		GameID: id, Title: strings.Repeat("x", n), Platform: "PC",
		Confidence: ipc.MetadataConfidenceNone, Selected: -1, Candidates: []ipc.MetadataCandidate{},
	}
}

// rowBytes is the size of a row in a page: its JSON plus the separator [MET-057].
func rowBytes(r ipc.BatchRow) int {
	raw, _ := json.Marshal(r)
	return len(raw) + 1
}

// pageBytes is the size of the rows of a page.
func pageBytes(rows []ipc.BatchRow) int {
	n := 0
	for _, r := range rows {
		n += rowBytes(r)
	}
	return n
}

// fitCount is how many rows from offset fit one page: at most limit, and within maxResultBytes.
func fitCount(rows []ipc.BatchRow, offset, limit int) int {
	sum, n := 0, 0
	for i := offset; i < len(rows) && n < limit; i++ {
		size := rowBytes(rows[i])
		if sum+size > maxResultBytes {
			break
		}
		sum += size
		n++
	}
	return n
}

// TestBatchResultPages checks pageRows on its own: the ceiling of bytes of a page, the cut between rows, the
// next offset, and the rows that are alone over the ceiling [MET-057]. It is pure: no store, no providers.
func TestBatchResultPages(t *testing.T) {
	t.Parallel()

	t.Run("MET-057/500 linhas de 200 caracteres e 5 alternativas: cada página cabe no teto, a união é todas, sem repetir nem pular", func(t *testing.T) {
		t.Parallel()
		rows := make([]ipc.BatchRow, 0, 500)
		for i := range 500 {
			rows = append(rows, realRow(int64(i+1)))
		}
		var got []ipc.BatchRow
		offset := 0
		for offset < len(rows) {
			page, next := pageRows(rows, offset, 200)
			if len(page) == 0 || len(page) > 200 {
				t.Fatalf("page at %d has %d rows", offset, len(page))
			}
			if n := pageBytes(page); n > maxResultBytes {
				t.Fatalf("page at %d is %d bytes, want at most %d", offset, n, maxResultBytes)
			}
			for i, r := range page {
				if !reflect.DeepEqual(r, rows[offset+i]) {
					t.Fatalf("row %d of the page at %d is not the original row", i, offset)
				}
			}
			got = append(got, page...)
			if next == nil {
				if offset+len(page) != len(rows) {
					t.Fatalf("the last page ends at %d, want %d", offset+len(page), len(rows))
				}
				break
			}
			if *next != offset+len(page) {
				t.Fatalf("next = %d after %d rows from %d", *next, len(page), offset)
			}
			offset = *next
		}
		if !reflect.DeepEqual(got, rows) {
			t.Errorf("union of the pages has %d rows, want the %d in order", len(got), len(rows))
		}
	})

	t.Run("MET-057/menos linhas que limit quando preciso: next aponta para a primeira que não coube", func(t *testing.T) {
		t.Parallel()
		rows := make([]ipc.BatchRow, 0, 30)
		for i := range 30 {
			rows = append(rows, fatRow(int64(i+1), 60_000)) // about 15 fit the ceiling
		}
		page, next := pageRows(rows, 0, 200)
		if want := fitCount(rows, 0, 200); len(page) != want || want >= 200 {
			t.Fatalf("page has %d rows, want %d (fewer than 200, the ceiling is hit first)", len(page), want)
		}
		if next == nil || *next != len(page) {
			t.Fatalf("next = %v, want %d: the first row that did not fit", next, len(page))
		}
		if pageBytes(page)+rowBytes(rows[*next]) <= maxResultBytes {
			t.Errorf("the row at next would have fit the page")
		}
		for i, r := range page {
			if !reflect.DeepEqual(r, rows[i]) {
				t.Errorf("row %d of the page is cut or changed", i)
			}
		}
	})

	t.Run("MET-057/uma linha sozinha maior que o teto ainda é devolvida", func(t *testing.T) {
		t.Parallel()
		rows := []ipc.BatchRow{fatRow(1, 1_100_000), fatRow(2, 10)}
		page, next := pageRows(rows, 0, 100)
		if len(page) != 1 || page[0].GameID != 1 || next == nil || *next != 1 {
			t.Errorf("page has %d rows, next %v; want the oversized row alone and next 1", len(page), next)
		}
	})

	t.Run("MET-057/a maior linha válida (200 runas por campo, 5 alternativas) cabe no teto sozinha", func(t *testing.T) {
		t.Parallel()
		row := realRow(1)
		if n := rowBytes(row); n >= maxResultBytes {
			t.Fatalf("the largest valid row is %d bytes, want under %d", n, maxResultBytes)
		}
		page, next := pageRows([]ipc.BatchRow{row}, 0, 200)
		if len(page) != 1 || next != nil {
			t.Errorf("page has %d rows, next %v; want the row and null", len(page), next)
		}
	})

	t.Run("MET-057/offset além do fim: página vazia e next nulo", func(t *testing.T) {
		t.Parallel()
		page, next := pageRows([]ipc.BatchRow{realRow(1), realRow(2), realRow(3)}, 10, 100)
		if len(page) != 0 || next != nil {
			t.Errorf("page has %d rows, next %v; want none and null", len(page), next)
		}
	})

	t.Run("MET-057/maxResultBytes é o x-metadata.maxResultBytes do contrato v0.14.0 (921600)", func(t *testing.T) {
		t.Parallel()
		raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "phase-12-metadados-unificados", "contracts", "ipc.openrpc.json"))
		if err != nil {
			t.Fatalf("read the contract: %v", err)
		}
		var doc struct {
			Info struct {
				XMetadata struct {
					MaxResultBytes int `json:"maxResultBytes"`
				} `json:"x-metadata"`
			} `json:"info"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse the contract: %v", err)
		}
		if doc.Info.XMetadata.MaxResultBytes != maxResultBytes || maxResultBytes != 921600 {
			t.Errorf("contract maxResultBytes = %d, code = %d, want both 921600", doc.Info.XMetadata.MaxResultBytes, maxResultBytes)
		}
	})
}
