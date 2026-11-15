package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/importers"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// capImporterCandidate is a folder (or other) candidate whose wire JSON is about pad bytes
// longer than a short one. It drives the size cut of import.scan (IMV-019).
func capImporterCandidate(src string, i, pad int) importers.Candidate {
	dir := `E:\ps1\` + strings.Repeat("c", pad)
	return importers.Candidate{
		Source:       src,
		Title:        "Jogo " + strconv.Itoa(i),
		Platform:     "Sony Playstation",
		LaunchType:   importers.LaunchEmulator,
		LaunchTarget: dir + `\` + strconv.Itoa(i) + `.cue`,
		InstallDir:   dir,
		File:         strconv.Itoa(i) + ".cue",
		Extension:    "cue",
		Discs:        1,
	}
}

// capWireCandidates converts n candidates of the given source with the same padding.
func capWireCandidates(src string, n, pad int) []ipc.ImportCandidate {
	out := make([]ipc.ImportCandidate, 0, n)
	for i := range n {
		out = append(out, toIPCCandidate(capImporterCandidate(src, i, pad), false))
	}
	return out
}

// wireSize is the size one candidate takes in the scan result: its JSON plus the separator (IMV-019).
func wireSize(t *testing.T, c ipc.ImportCandidate) int {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal candidate: %v", err)
	}
	return len(b) + 1
}

// TestCapScanResult covers the size cut of the folder and file scans (IMV-019) and the
// limit it uses (IMV-070).
func TestCapScanResult(t *testing.T) {
	t.Parallel()

	t.Run("IMV-019/muitos candidatos com caminhos longos: o resultado fica em até 921600 bytes", func(t *testing.T) {
		t.Parallel()
		cands := capWireCandidates(importers.SourceFolder, 3000, 300)
		kept, warning := capScanResult(cands)
		total := 0
		for _, c := range kept {
			total += wireSize(t, c)
		}
		if total > 921600 {
			t.Errorf("kept JSON = %d bytes, want <= 921600", total)
		}
		if len(kept) == 0 || len(kept) >= len(cands) {
			t.Errorf("kept = %d of %d, want a strict and non-empty prefix", len(kept), len(cands))
		}
		if warning == "" {
			t.Errorf("warning empty, want the cut warning")
		}
	})

	t.Run("IMV-019/aviso com o total encontrado e os mostrados", func(t *testing.T) {
		t.Parallel()
		cands := capWireCandidates(importers.SourceFolder, 3000, 300)
		kept, warning := capScanResult(cands)
		if !strings.Contains(warning, strconv.Itoa(len(cands))) {
			t.Errorf("warning %q does not carry the total %d", warning, len(cands))
		}
		if !strings.Contains(warning, strconv.Itoa(len(kept))) {
			t.Errorf("warning %q does not carry the shown count %d", warning, len(kept))
		}
	})

	t.Run("IMV-019/nenhum candidato é cortado ao meio e a ordem se mantém", func(t *testing.T) {
		t.Parallel()
		cands := capWireCandidates(importers.SourceFolder, 3000, 300)
		kept, _ := capScanResult(cands)
		for i := range kept {
			if kept[i].Title != cands[i].Title {
				t.Fatalf("kept[%d].Title = %q, want %q (prefix, same order)", i, kept[i].Title, cands[i].Title)
			}
			a, _ := json.Marshal(kept[i])
			b, _ := json.Marshal(cands[i])
			if string(a) != string(b) {
				t.Fatalf("kept[%d] changed: a candidate was cut", i)
			}
		}
		// The cut stops at the first candidate that would not fit (import-rules.md §6).
		if len(kept) < len(cands) {
			used := 0
			for _, c := range kept {
				used += wireSize(t, c)
			}
			if next := used + wireSize(t, cands[len(kept)]); next <= 921600 {
				t.Errorf("candidate %d would still fit (%d <= 921600) but was dropped", len(kept), next)
			}
		}
	})

	t.Run("IMV-019/dentro do limite: sem corte e sem aviso", func(t *testing.T) {
		t.Parallel()
		cands := capWireCandidates(importers.SourceFolder, 5, 10)
		kept, warning := capScanResult(cands)
		if len(kept) != len(cands) {
			t.Errorf("kept = %d, want all %d", len(kept), len(cands))
		}
		if warning != "" {
			t.Errorf("warning = %q, want none", warning)
		}
	})

	t.Run("IMV-019/Steam, Epic e LaunchBox não são cortados", func(t *testing.T) {
		t.Parallel()
		for _, src := range []string{importers.SourceSteam, importers.SourceEpic, importers.SourceLaunchBox} {
			cands := make([]importers.Candidate, 0, 3000)
			for i := range 3000 {
				c := capImporterCandidate(src, i, 300)
				c.Platform = importers.PlatformWindows
				c.LaunchType = importers.LaunchURI
				c.LaunchTarget = "steam://rungameid/" + strconv.Itoa(i)
				cands = append(cands, c)
			}
			spy := &spyImportSource{id: src, res: importers.Result{Candidates: cands}}
			sourceKey := map[string]string{
				importers.SourceSteam:     ipc.ImportSourceSteam,
				importers.SourceEpic:      ipc.ImportSourceEpic,
				importers.SourceLaunchBox: ipc.ImportSourceLaunchBox,
			}[src]
			res, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{sourceKey: spy},
				ipc.ImportScanParams{Source: sourceKey, Path: t.TempDir()})
			if err != nil {
				t.Fatalf("%s: scan: %v", sourceKey, err)
			}
			if len(res.Candidates) != len(cands) {
				t.Errorf("%s: candidates = %d, want all %d", sourceKey, len(res.Candidates), len(cands))
			}
			for _, w := range res.Warnings {
				if strings.Contains(w, "limite do tamanho") {
					t.Errorf("%s: warning %q, want no size cut", sourceKey, w)
				}
			}
		}
	})

	t.Run("IMV-019/pasta: lista cortada e aviso no fim de warnings", func(t *testing.T) {
		t.Parallel()
		cands := make([]importers.Candidate, 0, 3000)
		for i := range 3000 {
			cands = append(cands, capImporterCandidate(importers.SourceFolder, i, 300))
		}
		spy := &spyImportSource{id: "folder", res: importers.Result{Candidates: cands}}
		res, err := scanV2(t, newFakeImportStore(), map[string]importers.Source{ipc.ImportSourceFolder: spy},
			ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Path: t.TempDir()})
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		if len(res.Candidates) >= len(cands) {
			t.Fatalf("candidates = %d, want a cut below %d", len(res.Candidates), len(cands))
		}
		if len(res.Warnings) == 0 {
			t.Fatalf("warnings empty, want the cut warning")
		}
		last := res.Warnings[len(res.Warnings)-1]
		if !strings.Contains(last, strconv.Itoa(len(cands))) || !strings.Contains(last, strconv.Itoa(len(res.Candidates))) {
			t.Errorf("last warning %q, want total %d and shown %d", last, len(cands), len(res.Candidates))
		}
	})
}

// importContractLimits reads x-import of the v0.13.0 contract (IMV-070).
type importContractLimits struct {
	Info struct {
		XImport struct {
			MaxScanResultBytes *int `json:"maxScanResultBytes"`
		} `json:"x-import"`
	} `json:"info"`
}

// TestImportCapContract checks maxScanResultBytes against the contract (IMV-070).
func TestImportCapContract(t *testing.T) {
	t.Parallel()

	t.Run("IMV-070/maxScanResultBytes é igual a x-import.maxScanResultBytes do contrato v0.13.0", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join("..", "..", "docs", "specs", "phase-11-importacao-v2", "contracts", "ipc.openrpc.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read contract: %v", err)
		}
		var c importContractLimits
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatalf("parse contract: %v", err)
		}
		got := c.Info.XImport.MaxScanResultBytes
		if got == nil {
			t.Fatal("contract x-import has no maxScanResultBytes")
		}
		if maxScanResultBytes != *got {
			t.Errorf("maxScanResultBytes = %d, contract x-import.maxScanResultBytes = %d", maxScanResultBytes, *got)
		}
	})
}
