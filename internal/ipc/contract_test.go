package ipc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// openRPC is the subset of contracts/ipc.openrpc.json the tests need.
type openRPC struct {
	Info struct {
		Limits struct {
			MaxMessageBytes          int `json:"maxMessageBytes"`
			HeartbeatIntervalSeconds int `json:"heartbeatIntervalSeconds"`
			ClientNotificationBuffer int `json:"clientNotificationBuffer"`
		} `json:"x-limits"`
	} `json:"info"`
	Methods []struct {
		Name         string `json:"name"`
		Notification bool   `json:"x-notification"`
		Params       []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
		} `json:"params"`
		Result struct {
			Schema jsonSchema `json:"schema"`
		} `json:"result"`
	} `json:"methods"`
	Components struct {
		Schemas map[string]jsonSchema `json:"schemas"`
		Errors  map[string]struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"components"`
}

type jsonSchema struct {
	Ref        string                     `json:"$ref"`
	Type       string                     `json:"type"`
	Required   []string                   `json:"required"`
	Properties map[string]json.RawMessage `json:"properties"`
}

func loadContract(t *testing.T) openRPC {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "specs", "phase-2-skeleton", "contracts", "ipc.openrpc.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var c openRPC
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	return c
}

// resultSchema returns the resolved result schema of method, following a local $ref.
func (c openRPC) resultSchema(t *testing.T, method string) jsonSchema {
	t.Helper()
	for _, m := range c.Methods {
		if m.Name != method {
			continue
		}
		s := m.Result.Schema
		if s.Ref != "" {
			name, ok := strings.CutPrefix(s.Ref, "#/components/schemas/")
			if !ok {
				t.Fatalf("unsupported $ref %q", s.Ref)
			}
			s, ok = c.Components.Schemas[name]
			if !ok {
				t.Fatalf("$ref %q not found in components.schemas", m.Result.Schema.Ref)
			}
		}
		return s
	}
	t.Fatalf("method %q not in contract", method)
	return jsonSchema{}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func jsonFieldNames(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%T is not a JSON object: %v", v, err)
	}
	return sortedKeys(m)
}

func TestContract(t *testing.T) {
	t.Parallel()
	c := loadContract(t)

	t.Run("IPC-001/status tem exatamente os campos do schema", func(t *testing.T) {
		t.Parallel()
		schema := c.resultSchema(t, ipc.MethodStatus)
		want := sortedKeys(schema.Properties)
		if len(want) == 0 {
			t.Fatal("contract AgentStatus has no properties")
		}
		required := slices.Sorted(slices.Values(schema.Required))
		if !slices.Equal(required, want) {
			t.Fatalf("contract inconsistency: required %v != properties %v", required, want)
		}

		// The Go type, even with zero values (no omitempty allowed on required fields).
		if got := jsonFieldNames(t, ipc.AgentStatus{}); !slices.Equal(got, want) {
			t.Errorf("ipc.AgentStatus JSON fields = %v, want exactly %v", got, want)
		}

		// The wire result of agent.status through Dispatch.
		info := testInfo()
		handlers := ipc.AgentHandlers(info, domain.NewFakeClock(info.StartedAt))
		resp, ok := dispatch(t, handlers, `{"jsonrpc":"2.0","id":1,"method":"agent.status"}`)
		if !ok {
			t.Fatal("agent.status: ok = false")
		}
		d := decodeResponse(t, resp)
		if d.hasErr {
			t.Fatalf("agent.status error %d", d.code)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(d.result, &fields); err != nil {
			t.Fatalf("status result %s: %v", d.result, err)
		}
		if got := sortedKeys(fields); !slices.Equal(got, want) {
			t.Errorf("agent.status result fields = %v, want exactly %v", got, want)
		}
	})

	t.Run("IPC-001/ping tem exatamente os campos do schema", func(t *testing.T) {
		t.Parallel()
		want := sortedKeys(c.resultSchema(t, ipc.MethodPing).Properties)
		if got := jsonFieldNames(t, ipc.PingResult{}); !slices.Equal(got, want) {
			t.Errorf("ipc.PingResult JSON fields = %v, want exactly %v", got, want)
		}
	})

	t.Run("IPC-001/nomes de métodos batem com o contrato", func(t *testing.T) {
		t.Parallel()
		var names []string
		for _, m := range c.Methods {
			names = append(names, m.Name)
		}
		slices.Sort(names)
		want := []string{ipc.MethodPing, ipc.MethodStatus, ipc.MethodHeartbeat}
		slices.Sort(want)
		if !slices.Equal(names, want) {
			t.Errorf("contract methods = %v, Go constants = %v", names, want)
		}

		// Every non-notification method in the contract has a handler; notifications don't.
		handlers := testHandlers()
		for _, m := range c.Methods {
			_, has := handlers[m.Name]
			if m.Notification && has {
				t.Errorf("AgentHandlers has a handler for server notification %q", m.Name)
			}
			if !m.Notification && !has {
				t.Errorf("AgentHandlers has no handler for %q", m.Name)
			}
		}
		if len(handlers) != 2 {
			t.Errorf("AgentHandlers returned %v, want exactly agent.ping and agent.status", sortedKeys(handlers))
		}
	})

	t.Run("IPC-004/params do heartbeat batem com o contrato", func(t *testing.T) {
		t.Parallel()
		var want []string
		for _, m := range c.Methods {
			if m.Name == ipc.MethodHeartbeat {
				for _, p := range m.Params {
					want = append(want, p.Name)
				}
			}
		}
		slices.Sort(want)
		if got := jsonFieldNames(t, ipc.HeartbeatParams{}); !slices.Equal(got, want) {
			t.Errorf("ipc.HeartbeatParams JSON fields = %v, want exactly %v", got, want)
		}
	})

	t.Run("IPC-001/startedAt RFC 3339 em UTC", func(t *testing.T) {
		t.Parallel()
		// Non-UTC input with sub-second precision: output must still be UTC RFC 3339.
		started := time.Date(2026, 11, 8, 23, 59, 58, 123456789, time.FixedZone("X", 5*60*60+30*60))
		info := testInfo()
		info.StartedAt = started
		handlers := ipc.AgentHandlers(info, domain.NewFakeClock(started))
		st := callStatus(t, handlers, `1`)
		assertStartedAtUTC(t, st.StartedAt, started)
	})

	t.Run("IPC-002/códigos batem com components.errors", func(t *testing.T) {
		t.Parallel()
		want := map[string]int{
			"ParseError":     ipc.CodeParseError,
			"InvalidRequest": ipc.CodeInvalidRequest,
			"MethodNotFound": ipc.CodeMethodNotFound,
			"InvalidParams":  ipc.CodeInvalidParams,
			"InternalError":  ipc.CodeInternalError,
		}
		if got, w := sortedKeys(c.Components.Errors), sortedKeys(want); !slices.Equal(got, w) {
			t.Fatalf("contract errors = %v, Go constants cover %v", got, w)
		}
		for name, code := range want {
			if got := c.Components.Errors[name].Code; got != code {
				t.Errorf("%s: contract code %d, Go constant %d", name, got, code)
			}
		}
	})

	t.Run("IPC-002/NewError preserva código e mensagem", func(t *testing.T) {
		t.Parallel()
		e := ipc.NewError(ipc.CodeInvalidParams, "Invalid params")
		if e.Code != ipc.CodeInvalidParams || e.Message != "Invalid params" {
			t.Fatalf("NewError = %+v", e)
		}
		if e.Error() == "" {
			t.Error("Error() is empty")
		}
	})

	t.Run("IPC-012/limite bate com x-limits.maxMessageBytes", func(t *testing.T) {
		t.Parallel()
		if got := c.Info.Limits.MaxMessageBytes; got != ipc.MaxMessageSize {
			t.Errorf("x-limits.maxMessageBytes = %d, ipc.MaxMessageSize = %d", got, ipc.MaxMessageSize)
		}
		if ipc.MaxMessageSize != 1048576 {
			t.Errorf("ipc.MaxMessageSize = %d, want 1048576", ipc.MaxMessageSize)
		}
	})

	t.Run("IPC-004/intervalo do heartbeat bate com x-limits", func(t *testing.T) {
		t.Parallel()
		want := time.Duration(c.Info.Limits.HeartbeatIntervalSeconds) * time.Second
		if ipc.HeartbeatInterval != want {
			t.Errorf("ipc.HeartbeatInterval = %v, contract %v", ipc.HeartbeatInterval, want)
		}
	})

	t.Run("IPC-017/buffer de notificações bate com x-limits", func(t *testing.T) {
		t.Parallel()
		if got := c.Info.Limits.ClientNotificationBuffer; got != ipc.NotificationBuffer {
			t.Errorf("x-limits.clientNotificationBuffer = %d, ipc.NotificationBuffer = %d", got, ipc.NotificationBuffer)
		}
	})

	t.Run("IPC-001/versão JSON-RPC é 2.0", func(t *testing.T) {
		t.Parallel()
		if ipc.Version != "2.0" {
			t.Errorf("ipc.Version = %q, want \"2.0\"", ipc.Version)
		}
	})
}
