package main

// Tests for T30 (docs/specs/phase-12-metadados-unificados/tasks.md): the Wails event metadata:batch
// and the validation of metadata.batchChanged params [MET-072]. The supervisor forwarding (the
// emit, the Warn and the "does not touch the agent" rules) is in supervisor_test.go
// (TestSupervisorMetadataBatch), next to the other notifications.
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - decodeBatchChanged requires the five fields. batchId must be >= 1; done, total and failed
//     must be >= 0; state must be one of the five BatchState values; every number must be an
//     integer (1.5 and strings are refused) [MET-072, contracts/ipc-v0.14.0-delta.md §4 and §7].
//   - MetadataBatchChangedView has the camelCase JSON names batchId, state, done, total, failed.

import (
	"encoding/json"
	"strings"
	"testing"
)

// batchStates are the five values of BatchState (contracts/ipc-v0.14.0-delta.md §2).
var batchStates = []string{"searching", "ready", "cancelled", "applying", "done"}

func TestDecodeBatchChanged(t *testing.T) {
	t.Parallel()

	t.Run("MET-072/metadataBatchEvent é \"metadata:batch\"", func(t *testing.T) {
		t.Parallel()
		if metadataBatchEvent != "metadata:batch" {
			t.Errorf("metadataBatchEvent = %q, want %q", metadataBatchEvent, "metadata:batch")
		}
	})

	t.Run("MET-072/decodeBatchChanged aceita cada um dos cinco estados", func(t *testing.T) {
		t.Parallel()
		for _, st := range batchStates {
			st := st
			t.Run("MET-072/estado "+st, func(t *testing.T) {
				t.Parallel()
				raw := json.RawMessage(`{"batchId":1763217600000,"state":"` + st + `","done":12,"total":80,"failed":1}`)
				v, err := decodeBatchChanged(raw)
				if err != nil {
					t.Fatalf("error = %v, want nil", err)
				}
				want := MetadataBatchChangedView{BatchID: 1763217600000, State: st, Done: 12, Total: 80, Failed: 1}
				if v != want {
					t.Errorf("view = %+v, want %+v", v, want)
				}
			})
		}
	})

	t.Run("MET-072/contadores zero e batchId 1 são válidos", func(t *testing.T) {
		t.Parallel()
		v, err := decodeBatchChanged(json.RawMessage(`{"batchId":1,"state":"searching","done":0,"total":0,"failed":0}`))
		if err != nil {
			t.Fatalf("error = %v, want nil", err)
		}
		want := MetadataBatchChangedView{BatchID: 1, State: "searching"}
		if v != want {
			t.Errorf("view = %+v, want %+v", v, want)
		}
	})

	t.Run("MET-072/decodeBatchChanged recusa params inválidos e não devolve valor", func(t *testing.T) {
		t.Parallel()
		bad := []struct {
			name string
			raw  json.RawMessage
		}{
			{"params ausentes (nil)", nil},
			{"params vazios", json.RawMessage(``)},
			{"JSON inválido", json.RawMessage(`{bad`)},
			{"objeto vazio", json.RawMessage(`{}`)},
			{"null", json.RawMessage(`null`)},
			{"array", json.RawMessage(`[1,"searching",0,0,0]`)},
			{"sem batchId", json.RawMessage(`{"state":"searching","done":1,"total":2,"failed":0}`)},
			{"sem state", json.RawMessage(`{"batchId":1,"done":1,"total":2,"failed":0}`)},
			{"sem done", json.RawMessage(`{"batchId":1,"state":"searching","total":2,"failed":0}`)},
			{"sem total", json.RawMessage(`{"batchId":1,"state":"searching","done":1,"failed":0}`)},
			{"sem failed", json.RawMessage(`{"batchId":1,"state":"searching","done":1,"total":2}`)},
			{"batchId null", json.RawMessage(`{"batchId":null,"state":"searching","done":1,"total":2,"failed":0}`)},
			{"batchId 0", json.RawMessage(`{"batchId":0,"state":"searching","done":1,"total":2,"failed":0}`)},
			{"batchId negativo", json.RawMessage(`{"batchId":-3,"state":"searching","done":1,"total":2,"failed":0}`)},
			{"batchId fracionário", json.RawMessage(`{"batchId":1.5,"state":"searching","done":1,"total":2,"failed":0}`)},
			{"batchId string", json.RawMessage(`{"batchId":"1","state":"searching","done":1,"total":2,"failed":0}`)},
			{"done negativo", json.RawMessage(`{"batchId":1,"state":"searching","done":-1,"total":2,"failed":0}`)},
			{"done fracionário", json.RawMessage(`{"batchId":1,"state":"searching","done":1.5,"total":2,"failed":0}`)},
			{"done string", json.RawMessage(`{"batchId":1,"state":"searching","done":"1","total":2,"failed":0}`)},
			{"total negativo", json.RawMessage(`{"batchId":1,"state":"searching","done":1,"total":-1,"failed":0}`)},
			{"failed negativo", json.RawMessage(`{"batchId":1,"state":"searching","done":1,"total":2,"failed":-2}`)},
			{"state desconhecido", json.RawMessage(`{"batchId":1,"state":"running","done":1,"total":2,"failed":0}`)},
			{"state vazio", json.RawMessage(`{"batchId":1,"state":"","done":1,"total":2,"failed":0}`)},
			{"state com outra caixa", json.RawMessage(`{"batchId":1,"state":"Done","done":1,"total":2,"failed":0}`)},
			{"state número", json.RawMessage(`{"batchId":1,"state":3,"done":1,"total":2,"failed":0}`)},
		}
		for _, tc := range bad {
			t.Run("MET-072/"+tc.name, func(t *testing.T) {
				t.Parallel()
				v, err := decodeBatchChanged(tc.raw)
				if err == nil {
					t.Errorf("decodeBatchChanged(%s) = %+v, want an error", tc.raw, v)
				}
			})
		}
	})

	t.Run("MET-072/MetadataBatchChangedView usa os nomes camelCase batchId, state, done, total e failed", func(t *testing.T) {
		t.Parallel()
		raw, err := json.Marshal(MetadataBatchChangedView{BatchID: 7, State: "ready", Done: 1, Total: 2, Failed: 0})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		s := string(raw)
		for _, key := range []string{`"batchId":7`, `"state":"ready"`, `"done":1`, `"total":2`, `"failed":0`} {
			if !strings.Contains(s, key) {
				t.Errorf("JSON %s has no %s", s, key)
			}
		}
	})
}
