package main

// Tests for T12 (docs/specs/phase-11-importacao-v2/tasks.md): the import bindings
// (ImportScan with ImportScanOptions, ImportApply) and the candidate fields file,
// extension, discs and discFiles (IMV-073, IMV-024, IMV-063).
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - ImportScan(source, path, opts) sends ipc.ImportScanParams{Source, Path, Platform,
//     TitleFrom, Recursive}; Recursive is a *bool that stays nil when opts.Recursive is nil.
//     Empty optional fields are omitted from the JSON (omitempty).
//   - ImportScanOptions JSON names are platform, titleFrom and recursive.
//   - ImportCandidateView JSON names of the new fields are file, extension, discs and
//     discFiles; they are copied both ways (scan result and apply params).
//   - Both bindings call the agent through App.client with importTimeout as deadline.
//   - Errors pass through userError: ipc.ErrConnectionLost is txtNotResponding and any
//     other error is txtUnexpected (UI-008).

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// importParamsMap decodes the JSON params of one recorded call as a generic map.
func importParamsMap(t *testing.T, c gameCall) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(c.params, &m); err != nil {
		t.Fatalf("%s params %s: %v", c.method, c.params, err)
	}
	return m
}

func TestImportScanBinding(t *testing.T) {
	t.Parallel()

	t.Run("IMV-073/ImportScan envia platform, titleFrom e recursive nos parâmetros de import.scan", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodImportScan: ipc.ImportScanResult{}}}
		a := newBindingApp(p, &gameEmitLog{})
		recursive := false

		_, err := a.ImportScan("folder", `E:\Games\ps1`, ImportScanOptions{
			Platform:  "Sony Playstation",
			TitleFrom: "folder",
			Recursive: &recursive,
		})

		if err != nil {
			t.Fatalf("ImportScan err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodImportScan, importTimeout)
		m := importParamsMap(t, c)
		if m["source"] != "folder" || m["path"] != `E:\Games\ps1` {
			t.Errorf("import.scan source/path = %v/%v, want folder/E:\\Games\\ps1", m["source"], m["path"])
		}
		if m["platform"] != "Sony Playstation" {
			t.Errorf("import.scan platform = %v, want %q", m["platform"], "Sony Playstation")
		}
		if m["titleFrom"] != "folder" {
			t.Errorf("import.scan titleFrom = %v, want %q", m["titleFrom"], "folder")
		}
		if v, ok := m["recursive"]; !ok || v != false {
			t.Errorf("import.scan recursive = %v (present %v), want false", v, ok)
		}
	})

	t.Run("IMV-073/recursive ausente (nil) não vai no JSON; false vai como false", func(t *testing.T) {
		t.Parallel()
		recursiveFalse := false
		tests := []struct {
			name        string
			opts        ImportScanOptions
			wantPresent bool
			wantValue   any
		}{
			{name: "nil", opts: ImportScanOptions{Platform: "Windows", TitleFrom: "file"}, wantPresent: false},
			{name: "false", opts: ImportScanOptions{Platform: "Windows", TitleFrom: "file", Recursive: &recursiveFalse}, wantPresent: true, wantValue: false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				p := &gamePinger{results: map[string]any{ipc.MethodImportScan: ipc.ImportScanResult{}}}
				a := newBindingApp(p, &gameEmitLog{})

				if _, err := a.ImportScan("folder", `C:\Jogos`, tt.opts); err != nil {
					t.Fatalf("ImportScan err = %v", err)
				}
				c := singleCall(t, p, ipc.MethodImportScan, importTimeout)
				m := importParamsMap(t, c)
				v, ok := m["recursive"]
				if ok != tt.wantPresent || (ok && v != tt.wantValue) {
					t.Errorf("import.scan recursive = %v (present %v), want %v (present %v); params %s",
						v, ok, tt.wantValue, tt.wantPresent, c.params)
				}
			})
		}
	})

	t.Run("IMV-073/ImportCandidateView recebe file, extension, discs e discFiles do resultado", func(t *testing.T) {
		t.Parallel()
		res := ipc.ImportScanResult{
			Candidates: []ipc.ImportCandidate{{
				Source:       "folder",
				Title:        "Final Fantasy VIII",
				Platform:     "Sony Playstation",
				LaunchType:   "emulator",
				LaunchTarget: `E:\Games\ps1\04. Final Fantasy VIII\FF8 (Disc 1).cue`,
				InstallDir:   `E:\Games\ps1\04. Final Fantasy VIII`,
				File:         "FF8 (Disc 1).cue",
				Extension:    "cue",
				Discs:        2,
				DiscFiles:    []string{"FF8 (Disc 1).cue", "FF8 (Disc 2).cue"},
			}},
			Warnings: []string{},
		}
		p := &gamePinger{results: map[string]any{ipc.MethodImportScan: res}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.ImportScan("folder", `E:\Games\ps1`, ImportScanOptions{Platform: "Sony Playstation", TitleFrom: "file"})

		if err != nil {
			t.Fatalf("ImportScan err = %v", err)
		}
		if len(got.Candidates) != 1 {
			t.Fatalf("ImportScan candidates = %+v, want 1", got.Candidates)
		}
		c := got.Candidates[0]
		if c.File != "FF8 (Disc 1).cue" || c.Extension != "cue" || c.Discs != 2 {
			t.Errorf("candidate file/extension/discs = %q/%q/%d, want %q/%q/2",
				c.File, c.Extension, c.Discs, "FF8 (Disc 1).cue", "cue")
		}
		wantDiscs := []string{"FF8 (Disc 1).cue", "FF8 (Disc 2).cue"}
		if len(c.DiscFiles) != len(wantDiscs) || c.DiscFiles[0] != wantDiscs[0] || c.DiscFiles[1] != wantDiscs[1] {
			t.Errorf("candidate discFiles = %v, want %v", c.DiscFiles, wantDiscs)
		}
	})

	t.Run("IMV-073/só import.scan e import.apply são chamados", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{
			ipc.MethodImportScan:  ipc.ImportScanResult{Candidates: []ipc.ImportCandidate{{Source: "file", Title: "Mario", Platform: "Nintendo 64"}}},
			ipc.MethodImportApply: ipc.ImportApplyResult{},
		}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.ImportScan("file", `C:\roms\mario.z64`, ImportScanOptions{Platform: "Nintendo 64", TitleFrom: "file"}); err != nil {
			t.Fatalf("ImportScan err = %v", err)
		}
		view, err := a.ImportScan("file", `C:\roms\mario.z64`, ImportScanOptions{Platform: "Nintendo 64", TitleFrom: "file"})
		if err != nil {
			t.Fatalf("ImportScan (second) err = %v", err)
		}
		if _, err := a.ImportApply(view.Candidates, ""); err != nil {
			t.Fatalf("ImportApply err = %v", err)
		}

		calls := p.recorded()
		allowed := map[string]bool{ipc.MethodImportScan: true, ipc.MethodImportApply: true}
		if len(calls) != 3 {
			t.Fatalf("agent calls = %d, want 3 (scan, scan, apply)", len(calls))
		}
		for _, c := range calls {
			if !allowed[c.method] {
				t.Errorf("unexpected agent call %q; only import.scan and import.apply are allowed", c.method)
			}
		}
	})

	t.Run("IMV-073/erro do agente sai com a mensagem de UI-008", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{errs: map[string]error{
			ipc.MethodImportScan:  ipc.ErrConnectionLost,
			ipc.MethodImportApply: errors.New("boom"),
		}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.ImportScan("folder", `C:\Jogos`, ImportScanOptions{Platform: "Windows", TitleFrom: "file"}); err == nil || err.Error() != txtNotResponding {
			t.Errorf("ImportScan err = %v, want %q", err, txtNotResponding)
		}
		if _, err := a.ImportApply([]ImportCandidateView{{Source: "file", Title: "Mario"}}, ""); err == nil || err.Error() != txtUnexpected {
			t.Errorf("ImportApply err = %v, want %q", err, txtUnexpected)
		}
	})

	t.Run("IMV-073/timeout de importTimeout", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{
			ipc.MethodImportScan:  ipc.ImportScanResult{},
			ipc.MethodImportApply: ipc.ImportApplyResult{},
		}}
		a := newBindingApp(p, &gameEmitLog{})

		if _, err := a.ImportScan("folder", `C:\Jogos`, ImportScanOptions{Platform: "Windows", TitleFrom: "file"}); err != nil {
			t.Fatalf("ImportScan err = %v", err)
		}
		singleCall(t, p, ipc.MethodImportScan, importTimeout)

		p2 := &gamePinger{results: map[string]any{ipc.MethodImportApply: ipc.ImportApplyResult{}}}
		a.client = p2
		if _, err := a.ImportApply([]ImportCandidateView{{Source: "file", Title: "Mario"}}, ""); err != nil {
			t.Fatalf("ImportApply err = %v", err)
		}
		singleCall(t, p2, ipc.MethodImportApply, importTimeout)
	})
}

func TestImportApplyBinding(t *testing.T) {
	t.Parallel()

	t.Run("IMV-073/ImportApply devolve os quatro campos ao agente", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodImportApply: ipc.ImportApplyResult{
			Applied: []ipc.ImportApplied{{GameID: 9, Title: "Final Fantasy VIII"}},
			Skipped: []ipc.ImportSkipped{},
		}}}
		a := newBindingApp(p, &gameEmitLog{})
		in := []ImportCandidateView{{
			Source:       "folder",
			Title:        "Final Fantasy VIII",
			Platform:     "Sony Playstation",
			LaunchType:   "emulator",
			LaunchTarget: `E:\Games\ps1\04. Final Fantasy VIII\FF8 (Disc 1).cue`,
			InstallDir:   `E:\Games\ps1\04. Final Fantasy VIII`,
			File:         "FF8 (Disc 1).cue",
			Extension:    "cue",
			Discs:        2,
			DiscFiles:    []string{"FF8 (Disc 1).cue", "FF8 (Disc 2).cue"},
		}}

		got, err := a.ImportApply(in, "")

		if err != nil {
			t.Fatalf("ImportApply err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodImportApply, importTimeout)
		m := importParamsMap(t, c)
		cands, ok := m["candidates"].([]any)
		if !ok || len(cands) != 1 {
			t.Fatalf("import.apply candidates = %v, want 1 candidate", m["candidates"])
		}
		wire, ok := cands[0].(map[string]any)
		if !ok {
			t.Fatalf("import.apply candidate = %v, want object", cands[0])
		}
		if wire["file"] != "FF8 (Disc 1).cue" || wire["extension"] != "cue" || wire["discs"] != float64(2) {
			t.Errorf("import.apply file/extension/discs = %v/%v/%v, want FF8 (Disc 1).cue/cue/2",
				wire["file"], wire["extension"], wire["discs"])
		}
		df, ok := wire["discFiles"].([]any)
		if !ok || len(df) != 2 || df[0] != "FF8 (Disc 1).cue" || df[1] != "FF8 (Disc 2).cue" {
			t.Errorf("import.apply discFiles = %v, want both disc files in order", wire["discFiles"])
		}
		if len(got.Applied) != 1 || got.Applied[0].GameID != 9 || got.Applied[0].Title != "Final Fantasy VIII" {
			t.Errorf("ImportApply applied = %+v, want game 9 Final Fantasy VIII", got.Applied)
		}
	})
}
