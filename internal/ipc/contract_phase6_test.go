package ipc_test

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// schemaProps returns the property names of components.schemas[name] in contractPath.
func schemaProps(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	s, ok := doc.Components.Schemas[name]
	if !ok {
		t.Fatalf("schema %s missing in contract", name)
	}
	var names []string
	for k := range s.Properties {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

// jsonKeys returns the sorted keys of the JSON object of v.
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func TestImportContract(t *testing.T) {
	t.Run("IMP-001/import.scan aceita fonte e caminho como no contrato", func(t *testing.T) {
		t.Parallel()
		got := jsonKeys(t, ipc.ImportScanParams{Source: ipc.ImportSourceFolder, Path: `C:\Games`})
		if want := schemaProps(t, "ImportScanParams"); !slices.Equal(got, want) {
			t.Errorf("ImportScanParams keys = %v, want %v", got, want)
		}
	})

	t.Run("IMP-001/candidatos usam camelCase com as mesmas chaves do contrato", func(t *testing.T) {
		t.Parallel()
		got := jsonKeys(t, ipc.ImportCandidate{Source: ipc.ImportSourceSteam, Title: "x", LaunchType: "uri", LaunchTarget: "steam://rungameid/1"})
		if want := schemaProps(t, "ImportCandidate"); !slices.Equal(got, want) {
			t.Errorf("ImportCandidate keys = %v, want %v", got, want)
		}
	})

	t.Run("IMP-001/resultado do scan e do apply têm as chaves do contrato", func(t *testing.T) {
		t.Parallel()
		if got, want := jsonKeys(t, ipc.ImportScanResult{}), schemaProps(t, "ImportScanResult"); !slices.Equal(got, want) {
			t.Errorf("ImportScanResult keys = %v, want %v", got, want)
		}
		if got, want := jsonKeys(t, ipc.ImportApplyResult{}), schemaProps(t, "ImportApplyResult"); !slices.Equal(got, want) {
			t.Errorf("ImportApplyResult keys = %v, want %v", got, want)
		}
	})

	t.Run("RUL-003/game.launch continua recebendo só gameId; o URI vem do jogo", func(t *testing.T) {
		t.Parallel()
		raw, err := os.ReadFile(contractPath)
		if err != nil {
			t.Fatalf("read contract: %v", err)
		}
		var doc struct {
			Methods []struct {
				Name   string `json:"name"`
				Params []struct {
					Name string `json:"name"`
				} `json:"params"`
			} `json:"methods"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse contract: %v", err)
		}
		for _, m := range doc.Methods {
			if m.Name == "game.launch" {
				if len(m.Params) != 1 || m.Params[0].Name != "gameId" {
					t.Errorf("game.launch params = %+v, want only gameId", m.Params)
				}
				return
			}
		}
		t.Fatal("game.launch missing in contract")
	})
}
