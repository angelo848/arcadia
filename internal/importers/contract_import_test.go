package importers_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/importers"
)

// importContractLimits is the x-import block of the v0.13.0 contract (IMV-070).
type importContractLimits struct {
	Info struct {
		XImport struct {
			MaxScanDepth *int `json:"maxScanDepth"`
			MaxScanItems *int `json:"maxScanItems"`
		} `json:"x-import"`
	} `json:"info"`
}

// readImportContractLimits reads x-import from the phase 11 contract file.
func readImportContractLimits(t *testing.T) importContractLimits {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "specs", "phase-11-importacao-v2", "contracts", "ipc.openrpc.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var c importContractLimits
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	return c
}

// TestImportContractLimits checks the folder reading limits against the contract (IMV-070).
func TestImportContractLimits(t *testing.T) {
	t.Parallel()
	c := readImportContractLimits(t)

	t.Run("IMV-070/MaxScanDepth é igual a x-import.maxScanDepth do contrato v0.13.0", func(t *testing.T) {
		t.Parallel()
		if c.Info.XImport.MaxScanDepth == nil {
			t.Fatal("contract x-import has no maxScanDepth")
		}
		if importers.MaxScanDepth != *c.Info.XImport.MaxScanDepth {
			t.Errorf("MaxScanDepth = %d, contract = %d", importers.MaxScanDepth, *c.Info.XImport.MaxScanDepth)
		}
	})

	t.Run("IMV-070/MaxScanItems é igual a x-import.maxScanItems do contrato v0.13.0", func(t *testing.T) {
		t.Parallel()
		if c.Info.XImport.MaxScanItems == nil {
			t.Fatal("contract x-import has no maxScanItems")
		}
		if importers.MaxScanItems != *c.Info.XImport.MaxScanItems {
			t.Errorf("MaxScanItems = %d, contract = %d", importers.MaxScanItems, *c.Info.XImport.MaxScanItems)
		}
	})
}
