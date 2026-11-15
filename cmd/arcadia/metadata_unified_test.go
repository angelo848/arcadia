package main

// Tests for T29 (docs/specs/phase-12-metadados-unificados/tasks.md): the desktop bindings of the
// unified metadata search (MetadataSearch, MetadataApply and the MetadataBatch* family) [MET-072].
// The wire they produce follows contracts/ipc-v0.14.0-delta.md §3 and §7.
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - MetadataSearch and MetadataApply wait for providers, so they use scrapeTimeout as deadline.
//     The MetadataBatch* bindings return at once and use libraryTimeout.
//   - Optional params marked omitempty in the contract (platform, year, replace) are left out of
//     the JSON when they are empty or zero. candidate of a BatchChoice is always sent, even 0.
//   - Results are checked on the wire: the fake agent answers with the contract's JSON names.
//   - Errors pass through userError (UI-008): ipc.ErrConnectionLost -> txtNotResponding,
//     ProviderNoKey -> msgProviderNoKey, any other error -> txtUnexpected.

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// wireMap decodes the JSON params of one recorded call as a generic map.
func wireMap(t *testing.T, c gameCall) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(c.params, &m); err != nil {
		t.Fatalf("%s params %s: %v", c.method, c.params, err)
	}
	return m
}

// metaCandidateWire is one MetadataCandidate of the contract, as the agent sends it.
func metaCandidateWire(thumb string) map[string]any {
	c := map[string]any{
		"refs": []any{
			map[string]any{"provider": "igdb", "providerId": "7"},
			map[string]any{"provider": "steamstore", "providerId": "620"},
		},
		"title":    "Celeste",
		"year":     2018,
		"platform": "PC",
		"score":    0.92,
	}
	if thumb != "" {
		c["thumbUrl"] = thumb
	}
	return c
}

// metaBindingsCases are the eight bindings with the call each one makes; the agent answer is
// injected per subtest. Used by the UI-008 table.
var metaBindingsCases = []struct {
	name   string
	method string
	call   func(a *App) error
}{
	{"MetadataSearch", ipc.MethodMetadataSearch, func(a *App) error {
		_, err := a.MetadataSearch("Celeste", "", 0)
		return err
	}},
	{"MetadataApply", ipc.MethodMetadataApply, func(a *App) error {
		_, err := a.MetadataApply(3, nil, false)
		return err
	}},
	{"MetadataBatchStart", ipc.MethodMetadataBatchStart, func(a *App) error {
		_, err := a.MetadataBatchStart([]int64{3})
		return err
	}},
	{"MetadataBatchStatus", ipc.MethodMetadataBatchStatus, func(a *App) error {
		_, err := a.MetadataBatchStatus()
		return err
	}},
	{"MetadataBatchResults", ipc.MethodMetadataBatchResults, func(a *App) error {
		_, err := a.MetadataBatchResults(1763217600000, 0, 100)
		return err
	}},
	{"MetadataBatchApply", ipc.MethodMetadataBatchApply, func(a *App) error {
		_, err := a.MetadataBatchApply(1763217600000, nil)
		return err
	}},
	{"MetadataBatchCancel", ipc.MethodMetadataBatchCancel, func(a *App) error {
		_, err := a.MetadataBatchCancel(1763217600000)
		return err
	}},
	{"MetadataBatchDiscard", ipc.MethodMetadataBatchDiscard, func(a *App) error {
		return a.MetadataBatchDiscard(1763217600000)
	}},
}

func TestMetadataUnifiedBindingsExist(t *testing.T) {
	t.Parallel()

	t.Run("MET-072/o binding expõe MetadataSearch, MetadataApply e os seis MetadataBatch*", func(t *testing.T) {
		t.Parallel()
		typ := reflect.TypeOf(&App{})
		for _, name := range []string{
			"MetadataSearch", "MetadataApply", "MetadataBatchStart", "MetadataBatchStatus",
			"MetadataBatchResults", "MetadataBatchApply", "MetadataBatchCancel", "MetadataBatchDiscard",
		} {
			if _, ok := typ.MethodByName(name); !ok {
				t.Errorf("*App has no method %s", name)
			}
		}
	})
}

func TestMetadataSearchBinding(t *testing.T) {
	t.Parallel()

	t.Run("MET-072/MetadataSearch envia title, platform e year ao metadata.search com prazo de scrapeTimeout", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataSearch: map[string]any{
			"candidates": []any{}, "confidence": "none", "warnings": []any{},
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.MetadataSearch("Celeste", "Sony Playstation", 2018); err != nil {
			t.Fatalf("MetadataSearch err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodMetadataSearch, scrapeTimeout)
		m := wireMap(t, c)
		if m["title"] != "Celeste" || m["platform"] != "Sony Playstation" || m["year"] != float64(2018) {
			t.Errorf("metadata.search params = %v, want title Celeste, platform Sony Playstation, year 2018", m)
		}
	})

	t.Run("MET-072/MetadataSearch sem plataforma e sem ano omite platform e year", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataSearch: map[string]any{
			"candidates": []any{}, "confidence": "none", "warnings": []any{},
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.MetadataSearch("Celeste", "", 0); err != nil {
			t.Fatalf("MetadataSearch err = %v", err)
		}
		m := wireMap(t, singleCall(t, p, ipc.MethodMetadataSearch, scrapeTimeout))
		if _, ok := m["platform"]; ok {
			t.Errorf("metadata.search sent platform %v, want it omitted", m["platform"])
		}
		if _, ok := m["year"]; ok {
			t.Errorf("metadata.search sent year %v, want it omitted", m["year"])
		}
	})

	t.Run("MET-072/MetadataSearch devolve candidatos com refs, pontuação, miniatura, confiança e avisos", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataSearch: map[string]any{
			"candidates": []any{
				metaCandidateWire("https://images.igdb.com/igdb/image/upload/t_thumb/co1.jpg"),
				metaCandidateWire(""),
			},
			"confidence": "match",
			"warnings":   []any{"SteamGridDB sem chave"},
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.MetadataSearch("Celeste", "", 0)
		if err != nil {
			t.Fatalf("MetadataSearch err = %v", err)
		}
		if got.Confidence != "match" || len(got.Warnings) != 1 || got.Warnings[0] != "SteamGridDB sem chave" {
			t.Errorf("confidence/warnings = %q/%v, want match and one warning", got.Confidence, got.Warnings)
		}
		if len(got.Candidates) != 2 {
			t.Fatalf("candidates = %d, want 2", len(got.Candidates))
		}
		c := got.Candidates[0]
		if c.Title != "Celeste" || c.Year != 2018 || c.Platform != "PC" || c.Score != 0.92 {
			t.Errorf("candidate = %+v, want Celeste/2018/PC/0.92", c)
		}
		if len(c.Refs) != 2 || c.Refs[0].Provider != "igdb" || c.Refs[0].ProviderID != "7" ||
			c.Refs[1].Provider != "steamstore" || c.Refs[1].ProviderID != "620" {
			t.Errorf("candidate refs = %+v, want igdb/7 and steamstore/620", c.Refs)
		}
		if c.ThumbURL != "https://images.igdb.com/igdb/image/upload/t_thumb/co1.jpg" {
			t.Errorf("thumbUrl = %q, want the igdb thumb", c.ThumbURL)
		}
		if got.Candidates[1].ThumbURL != "" {
			t.Errorf("candidate without thumbUrl has ThumbURL %q, want empty", got.Candidates[1].ThumbURL)
		}
	})

	t.Run("MET-072/MetadataSearch com ProviderNoKey mostra a mensagem de cadastro", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{
			ipc.MethodMetadataSearch: ipc.NewError(ipc.CodeProviderNoKey, ipc.MsgProviderNoKey),
		}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.MetadataSearch("Celeste", "", 0); err == nil || err.Error() != msgProviderNoKey {
			t.Errorf("MetadataSearch err = %v, want %q", err, msgProviderNoKey)
		}
	})
}

func TestMetadataApplyBinding(t *testing.T) {
	t.Parallel()

	t.Run("MET-072/MetadataApply envia gameId, refs e replace ao metadata.apply", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataApply: map[string]any{
			"detail": map[string]any{"id": 3, "title": "Celeste"}, "queued": 0, "skippedByLimit": 0,
		}}}
		a := newBindingApp(p, &gameEmitLog{})
		refs := []MetadataRefView{{Provider: "igdb", ProviderID: "7"}, {Provider: "steamstore", ProviderID: "620"}}

		if _, err := a.MetadataApply(3, refs, true); err != nil {
			t.Fatalf("MetadataApply err = %v", err)
		}
		m := wireMap(t, singleCall(t, p, ipc.MethodMetadataApply, scrapeTimeout))
		if m["gameId"] != float64(3) || m["replace"] != true {
			t.Errorf("metadata.apply gameId/replace = %v/%v, want 3/true", m["gameId"], m["replace"])
		}
		wire, ok := m["refs"].([]any)
		if !ok || len(wire) != 2 {
			t.Fatalf("metadata.apply refs = %v, want 2 refs", m["refs"])
		}
		first, ok := wire[0].(map[string]any)
		if !ok || first["provider"] != "igdb" || first["providerId"] != "7" {
			t.Errorf("metadata.apply refs[0] = %v, want igdb/7", wire[0])
		}
	})

	t.Run("MET-072/MetadataApply com replace false omite o campo", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataApply: map[string]any{
			"detail": map[string]any{"id": 3, "title": "Celeste"}, "queued": 0, "skippedByLimit": 0,
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.MetadataApply(3, []MetadataRefView{{Provider: "igdb", ProviderID: "7"}}, false); err != nil {
			t.Fatalf("MetadataApply err = %v", err)
		}
		m := wireMap(t, singleCall(t, p, ipc.MethodMetadataApply, scrapeTimeout))
		if _, ok := m["replace"]; ok {
			t.Errorf("metadata.apply sent replace %v, want it omitted when false", m["replace"])
		}
	})

	t.Run("MET-072/MetadataApply devolve o jogo, queued e skippedByLimit", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataApply: map[string]any{
			"detail": map[string]any{"id": 3, "title": "Celeste"}, "queued": 3, "skippedByLimit": 1,
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.MetadataApply(3, []MetadataRefView{{Provider: "igdb", ProviderID: "7"}}, false)
		if err != nil {
			t.Fatalf("MetadataApply err = %v", err)
		}
		if got.Game.Title != "Celeste" || got.Queued != 3 || got.SkippedByLimit != 1 {
			t.Errorf("MetadataApply = title %q queued %d skipped %d, want Celeste/3/1", got.Game.Title, got.Queued, got.SkippedByLimit)
		}
	})

	t.Run("MET-072/MetadataApply com ProviderNoKey mostra a mensagem de cadastro", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{
			ipc.MethodMetadataApply: ipc.NewError(ipc.CodeProviderNoKey, ipc.MsgProviderNoKey),
		}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.MetadataApply(3, nil, false); err == nil || err.Error() != msgProviderNoKey {
			t.Errorf("MetadataApply err = %v, want %q", err, msgProviderNoKey)
		}
	})
}

func TestMetadataBatchBindings(t *testing.T) {
	t.Parallel()

	t.Run("MET-072/MetadataBatchStart envia gameIds e devolve batchId e total", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataBatchStart: map[string]any{
			"batchId": 1763217600000, "total": 80,
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.MetadataBatchStart([]int64{3, 9})
		if err != nil {
			t.Fatalf("MetadataBatchStart err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodMetadataBatchStart, libraryTimeout)
		m := wireMap(t, c)
		ids, ok := m["gameIds"].([]any)
		if !ok || len(ids) != 2 || ids[0] != float64(3) || ids[1] != float64(9) {
			t.Errorf("metadata.batchStart gameIds = %v, want [3 9]", m["gameIds"])
		}
		if got.BatchID != 1763217600000 || got.Total != 80 {
			t.Errorf("MetadataBatchStart = batch %d total %d, want 1763217600000/80", got.BatchID, got.Total)
		}
	})

	t.Run("MET-072/MetadataBatchStart com ProviderNoKey mostra a mensagem de cadastro", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{
			ipc.MethodMetadataBatchStart: ipc.NewError(ipc.CodeProviderNoKey, ipc.MsgProviderNoKey),
		}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.MetadataBatchStart([]int64{3}); err == nil || err.Error() != msgProviderNoKey {
			t.Errorf("MetadataBatchStart err = %v, want %q", err, msgProviderNoKey)
		}
	})

	t.Run("MET-072/MetadataBatchStatus sem busca devolve Batch nil", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataBatchStatus: map[string]any{"batch": nil}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.MetadataBatchStatus()
		if err != nil {
			t.Fatalf("MetadataBatchStatus err = %v", err)
		}
		singleCall(t, p, ipc.MethodMetadataBatchStatus, libraryTimeout)
		if got.Batch != nil {
			t.Errorf("MetadataBatchStatus batch = %+v, want nil", got.Batch)
		}
	})

	t.Run("MET-072/MetadataBatchStatus em busca em andamento traz os contadores e sem result", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataBatchStatus: map[string]any{
			"batch": map[string]any{
				"batchId": 1763217600000, "state": "searching", "total": 80, "done": 12, "failed": 1,
				"warnings": []any{"IGDB: limite atingido"},
			},
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.MetadataBatchStatus()
		if err != nil {
			t.Fatalf("MetadataBatchStatus err = %v", err)
		}
		if got.Batch == nil {
			t.Fatal("MetadataBatchStatus batch = nil, want the running batch")
		}
		b := got.Batch
		if b.BatchID != 1763217600000 || b.State != "searching" || b.Total != 80 || b.Done != 12 || b.Failed != 1 {
			t.Errorf("batch = %+v, want id/searching/80/12/1", b)
		}
		if len(b.Warnings) != 1 || b.Warnings[0] != "IGDB: limite atingido" {
			t.Errorf("batch warnings = %v, want the one warning", b.Warnings)
		}
		if b.Result != nil {
			t.Errorf("batch result = %+v, want nil outside done", b.Result)
		}
	})

	t.Run("MET-072/MetadataBatchStatus em done traz o result com aplicados, imagens e títulos com falha", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataBatchStatus: map[string]any{
			"batch": map[string]any{
				"batchId": 1763217600000, "state": "done", "total": 80, "done": 80, "failed": 2,
				"warnings": []any{},
				"result": map[string]any{
					"applied": 70, "imagesQueued": 140, "imagesSkippedByLimit": 5,
					"failedTitles": []any{"Jogo A", "Jogo B"},
				},
			},
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.MetadataBatchStatus()
		if err != nil {
			t.Fatalf("MetadataBatchStatus err = %v", err)
		}
		if got.Batch == nil || got.Batch.Result == nil {
			t.Fatalf("batch = %+v, want a done batch with result", got.Batch)
		}
		r := got.Batch.Result
		if r.Applied != 70 || r.ImagesQueued != 140 || r.ImagesSkippedByLimit != 5 {
			t.Errorf("result = %+v, want applied 70, imagesQueued 140, imagesSkippedByLimit 5", r)
		}
		if len(r.FailedTitles) != 2 || r.FailedTitles[0] != "Jogo A" || r.FailedTitles[1] != "Jogo B" {
			t.Errorf("failedTitles = %v, want [Jogo A Jogo B]", r.FailedTitles)
		}
	})

	t.Run("MET-072/MetadataBatchResults envia batchId, offset e limit e devolve linhas, total e next", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataBatchResults: map[string]any{
			"rows": []any{map[string]any{
				"gameId": 9, "title": "Celeste", "platform": "PC", "confidence": "match", "selected": 0,
				"candidates": []any{metaCandidateWire("")},
			}},
			"total": 120,
			"next":  100,
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.MetadataBatchResults(1763217600000, 100, 100)
		if err != nil {
			t.Fatalf("MetadataBatchResults err = %v", err)
		}
		m := wireMap(t, singleCall(t, p, ipc.MethodMetadataBatchResults, libraryTimeout))
		if m["batchId"] != float64(1763217600000) || m["offset"] != float64(100) || m["limit"] != float64(100) {
			t.Errorf("metadata.batchResults params = %v, want batchId/offset 100/limit 100", m)
		}
		if got.Total != 120 || got.Next == nil || *got.Next != 100 {
			t.Errorf("total/next = %d/%v, want 120/100", got.Total, got.Next)
		}
		if len(got.Rows) != 1 {
			t.Fatalf("rows = %d, want 1", len(got.Rows))
		}
		row := got.Rows[0]
		if row.GameID != 9 || row.Title != "Celeste" || row.Platform != "PC" || row.Confidence != "match" || row.Selected != 0 {
			t.Errorf("row = %+v, want game 9 Celeste PC match selected 0", row)
		}
		if len(row.Candidates) != 1 || row.Candidates[0].Title != "Celeste" {
			t.Errorf("row candidates = %+v, want the one candidate", row.Candidates)
		}
	})

	t.Run("MET-072/MetadataBatchResults na última página devolve next nil", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataBatchResults: map[string]any{
			"rows": []any{}, "total": 120, "next": nil,
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.MetadataBatchResults(1763217600000, 100, 100)
		if err != nil {
			t.Fatalf("MetadataBatchResults err = %v", err)
		}
		if got.Next != nil {
			t.Errorf("next = %d, want nil on the last page", *got.Next)
		}
	})

	t.Run("MET-072/MetadataBatchApply envia gameId e candidate, candidate 0 incluso, e devolve accepted", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataBatchApply: map[string]any{"accepted": 2}}}
		a := newBindingApp(p, &gameEmitLog{})
		choices := []MetadataChoiceView{{GameID: 9, Candidate: 0}, {GameID: 10, Candidate: 2}}

		n, err := a.MetadataBatchApply(1763217600000, choices)
		if err != nil {
			t.Fatalf("MetadataBatchApply err = %v", err)
		}
		if n != 2 {
			t.Errorf("accepted = %d, want 2", n)
		}
		m := wireMap(t, singleCall(t, p, ipc.MethodMetadataBatchApply, libraryTimeout))
		if m["batchId"] != float64(1763217600000) {
			t.Errorf("metadata.batchApply batchId = %v", m["batchId"])
		}
		wire, ok := m["choices"].([]any)
		if !ok || len(wire) != 2 {
			t.Fatalf("metadata.batchApply choices = %v, want 2", m["choices"])
		}
		first, ok := wire[0].(map[string]any)
		if !ok {
			t.Fatalf("choice[0] = %v, want object", wire[0])
		}
		if first["gameId"] != float64(9) {
			t.Errorf("choice[0] gameId = %v, want 9", first["gameId"])
		}
		if v, present := first["candidate"]; !present || v != float64(0) {
			t.Errorf("choice[0] candidate = %v (present %v), want 0 sent", v, present)
		}
	})

	t.Run("MET-072/MetadataBatchCancel envia batchId e devolve o estado", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataBatchCancel: map[string]any{"state": "cancelled"}}}
		a := newBindingApp(p, &gameEmitLog{})

		state, err := a.MetadataBatchCancel(1763217600000)
		if err != nil {
			t.Fatalf("MetadataBatchCancel err = %v", err)
		}
		if state != "cancelled" {
			t.Errorf("state = %q, want cancelled", state)
		}
		m := wireMap(t, singleCall(t, p, ipc.MethodMetadataBatchCancel, libraryTimeout))
		if m["batchId"] != float64(1763217600000) {
			t.Errorf("metadata.batchCancel batchId = %v", m["batchId"])
		}
	})

	t.Run("MET-072/MetadataBatchDiscard envia batchId e devolve só o erro", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodMetadataBatchDiscard: map[string]any{"discarded": true}}}
		a := newBindingApp(p, &gameEmitLog{})

		if err := a.MetadataBatchDiscard(1763217600000); err != nil {
			t.Fatalf("MetadataBatchDiscard err = %v", err)
		}
		m := wireMap(t, singleCall(t, p, ipc.MethodMetadataBatchDiscard, libraryTimeout))
		if m["batchId"] != float64(1763217600000) {
			t.Errorf("metadata.batchDiscard batchId = %v", m["batchId"])
		}
	})

	t.Run("MET-072/MetadataBatchDiscard com BatchNotFound devolve erro", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{
			ipc.MethodMetadataBatchDiscard: ipc.NewError(ipc.CodeBatchNotFound, "Metadata batch not found"),
		}}
		a := newBindingApp(p, &gameEmitLog{})

		if err := a.MetadataBatchDiscard(1763217600000); err == nil {
			t.Error("MetadataBatchDiscard err = nil, want an error for BatchNotFound")
		}
	})
}

// TestMetadataBindingsErrors checks UI-008 for the eight bindings: a lost connection shows the
// "not responding" text and any other error the "unexpected" text.
func TestMetadataBindingsErrors(t *testing.T) {
	t.Parallel()

	for _, b := range metaBindingsCases {
		t.Run("MET-072/"+b.name+" com conexão perdida mostra o texto de agente sem resposta", func(t *testing.T) {
			t.Parallel()
			p := &gamePinger{errs: map[string]error{b.method: ipc.ErrConnectionLost}}
			a := newBindingApp(p, &gameEmitLog{})
			if err := b.call(a); err == nil || err.Error() != txtNotResponding {
				t.Errorf("%s err = %v, want %q", b.name, err, txtNotResponding)
			}
		})
		t.Run("MET-072/"+b.name+" com outro erro mostra o erro inesperado", func(t *testing.T) {
			t.Parallel()
			p := &gamePinger{errs: map[string]error{b.method: errors.New("boom")}}
			a := newBindingApp(p, &gameEmitLog{})
			if err := b.call(a); err == nil || err.Error() != txtUnexpected {
				t.Errorf("%s err = %v, want %q", b.name, err, txtUnexpected)
			}
		})
	}
}
