package main

// Tests for T29 (docs/specs/phase-12-metadados-unificados/tasks.md): ImportCandidateView gains
// SearchName ("searchName" in JSON), copied both ways between import.scan and import.apply [MET-040].
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - ipc.ImportCandidate.SearchName has json "searchName,omitempty", so candidates of the other
//     sources keep the v0.13.0 JSON byte for byte (no "searchName" key).
//   - import.apply sends SearchName back as it received it; the agent ignores it.

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

func TestImportSearchNameBinding(t *testing.T) {
	t.Parallel()

	t.Run("MET-040/ImportScan copia searchName do candidato de arquivo e ImportApply o devolve ao agente", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{
			ipc.MethodImportScan: map[string]any{
				"candidates": []any{map[string]any{
					"source": "file", "title": "Celeste", "platform": "PC", "launchType": "file",
					"launchTarget": `E:\Jogos\Celeste\Celeste-Win64-Shipping.exe`, "installDir": `E:\Jogos\Celeste`,
					"exists": true, "file": "Celeste-Win64-Shipping.exe", "extension": "exe",
					"searchName": "Celeste",
				}},
				"warnings": []any{},
			},
			ipc.MethodImportApply: map[string]any{"applied": []any{}, "skipped": []any{}},
		}}
		a := newBindingApp(p, &gameEmitLog{})

		view, err := a.ImportScan("file", `E:\Jogos\Celeste\Celeste-Win64-Shipping.exe`, ImportScanOptions{Platform: "PC", TitleFrom: "file"})
		if err != nil {
			t.Fatalf("ImportScan err = %v", err)
		}
		if len(view.Candidates) != 1 {
			t.Fatalf("candidates = %d, want 1", len(view.Candidates))
		}
		if got := view.Candidates[0].SearchName; got != "Celeste" {
			t.Errorf("SearchName = %q, want %q", got, "Celeste")
		}

		if _, err := a.ImportApply(view.Candidates, ""); err != nil {
			t.Fatalf("ImportApply err = %v", err)
		}
		// The fake also served import.scan, so the import.apply call is looked up by method.
		var applyCall gameCall
		found := false
		for _, c := range p.recorded() {
			if c.method == ipc.MethodImportApply {
				applyCall, found = c, true
			}
		}
		if !found {
			t.Fatalf("no import.apply call recorded; calls = %+v", p.recorded())
		}
		m := importParamsMap(t, applyCall)
		cands, ok := m["candidates"].([]any)
		if !ok || len(cands) != 1 {
			t.Fatalf("import.apply candidates = %v, want 1", m["candidates"])
		}
		wire, ok := cands[0].(map[string]any)
		if !ok {
			t.Fatalf("import.apply candidate = %v, want object", cands[0])
		}
		if wire["searchName"] != "Celeste" {
			t.Errorf("import.apply searchName = %v, want %q", wire["searchName"], "Celeste")
		}
	})

	t.Run("MET-040/searchName vazio é omitido no import.apply (fontes sem o campo ficam como na v0.13.0)", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodImportApply: map[string]any{"applied": []any{}, "skipped": []any{}}}}
		a := newBindingApp(p, &gameEmitLog{})

		in := []ImportCandidateView{{Source: "steam", Title: "Hades", Platform: "PC", LaunchType: "steam"}}
		if _, err := a.ImportApply(in, ""); err != nil {
			t.Fatalf("ImportApply err = %v", err)
		}
		m := importParamsMap(t, singleCall(t, p, ipc.MethodImportApply, importTimeout))
		cands, ok := m["candidates"].([]any)
		if !ok || len(cands) != 1 {
			t.Fatalf("import.apply candidates = %v, want 1", m["candidates"])
		}
		wire, ok := cands[0].(map[string]any)
		if !ok {
			t.Fatalf("import.apply candidate = %v, want object", cands[0])
		}
		if v, present := wire["searchName"]; present {
			t.Errorf("import.apply sent searchName %v for a steam candidate, want it omitted", v)
		}
	})

	t.Run("MET-040/searchName ausente em outras fontes não quebra a leitura do candidato", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodImportScan: map[string]any{
			"candidates": []any{map[string]any{"source": "epic", "title": "Fortnite", "platform": "PC", "launchType": "epic"}},
			"warnings":   []any{},
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		view, err := a.ImportScan("epic", "", ImportScanOptions{Platform: "PC", TitleFrom: "file"})
		if err != nil {
			t.Fatalf("ImportScan err = %v", err)
		}
		if len(view.Candidates) != 1 || view.Candidates[0].Title != "Fortnite" || view.Candidates[0].SearchName != "" {
			t.Errorf("candidates = %+v, want Fortnite with empty SearchName", view.Candidates)
		}
	})
}
