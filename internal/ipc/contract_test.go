package ipc_test

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// contractPath is the source of truth: the phase-9-plataformas contract (v0.11.0)
// supersedes phase-8-estatisticas (v0.10.0) [IPC-041].
var contractPath = filepath.Join("..", "..", "docs", "specs", "phase-9-plataformas", "contracts", "ipc.openrpc.json")

// contractRelPath is how the package comment must name the contract.
const contractRelPath = "docs/specs/phase-9-plataformas/contracts/ipc.openrpc.json"

// openRPC is the subset of contracts/ipc.openrpc.json the tests need.
type openRPC struct {
	Info struct {
		Version string `json:"version"`
		Limits  struct {
			MaxMessageBytes          int  `json:"maxMessageBytes"`
			HeartbeatIntervalSeconds int  `json:"heartbeatIntervalSeconds"`
			ClientNotificationBuffer int  `json:"clientNotificationBuffer"`
			ShutdownDrainMs          *int `json:"shutdownDrainMs"`
			MaxGameTitleRunes        *int `json:"maxGameTitleRunes"`
			MaxNameRunes             *int `json:"maxNameRunes"`
			MaxDescriptionRunes      *int `json:"maxDescriptionRunes"`
			MaxFilterTextRunes       *int `json:"maxFilterTextRunes"`
			MaxFilterIds             *int `json:"maxFilterIds"`
			YearMin                  *int `json:"yearMin"`
			YearMax                  *int `json:"yearMax"`
			DefaultGameListLimit     *int `json:"defaultGameListLimit"`
			MaxGameListLimit         *int `json:"maxGameListLimit"`
			DefaultSessionListLimit  *int `json:"defaultSessionListLimit"`
			MaxSessionListLimit      *int `json:"maxSessionListLimit"`
		} `json:"x-limits"`
		Tracking *struct {
			GracePeriodSeconds       *int `json:"gracePeriodSeconds"`
			MinCountedSessionSeconds *int `json:"minCountedSessionSeconds"`
			SessionHeartbeatSeconds  *int `json:"sessionHeartbeatSeconds"`
			JobPollSeconds           *int `json:"jobPollSeconds"`
			MaxConcurrentSessions    *int `json:"maxConcurrentSessions"`
		} `json:"x-tracking"`
		Library *struct {
			DefaultSort string   `json:"defaultSort"`
			SortValues  []string `json:"sortValues"`
		} `json:"x-library"`
	} `json:"info"`
	Methods []struct {
		Name         string `json:"name"`
		Notification bool   `json:"x-notification"`
		Params       []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
			Schema   struct {
				Ref  string   `json:"$ref"`
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			} `json:"schema"`
		} `json:"params"`
		Result struct {
			Schema jsonSchema `json:"schema"`
		} `json:"result"`
	} `json:"methods"`
	Components struct {
		Schemas map[string]jsonSchema `json:"schemas"`
		Errors  map[string]struct {
			Code        int         `json:"code"`
			Message     string      `json:"message"`
			Description string      `json:"description"`
			Data        *jsonSchema `json:"data"`
		} `json:"errors"`
	} `json:"components"`
}

type jsonSchema struct {
	Ref        string                     `json:"$ref"`
	Type       string                     `json:"type"`
	Enum       []string                   `json:"enum"`
	Required   []string                   `json:"required"`
	Properties map[string]json.RawMessage `json:"properties"`
	Items      json.RawMessage            `json:"items"`
}

// loadContract parses the contract. The version is asserted by the
// "IPC-041/versão 0.6.0" case.
func loadContract(t *testing.T) openRPC {
	t.Helper()
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var c openRPC
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	return c
}

// resolve follows a local "#/components/schemas/<name>" $ref (if any).
func (c openRPC) resolve(t *testing.T, s jsonSchema) jsonSchema {
	t.Helper()
	if s.Ref == "" {
		return s
	}
	name, ok := strings.CutPrefix(s.Ref, "#/components/schemas/")
	if !ok {
		t.Fatalf("unsupported $ref %q", s.Ref)
	}
	r, ok := c.Components.Schemas[name]
	if !ok {
		t.Fatalf("$ref %q not found in components.schemas", s.Ref)
	}
	return r
}

// schema returns the resolved components.schemas entry name.
func (c openRPC) schema(t *testing.T, name string) jsonSchema {
	t.Helper()
	s, ok := c.Components.Schemas[name]
	if !ok {
		t.Fatalf("contract has no components.schemas.%s", name)
	}
	return s
}

// resultSchema returns the resolved result schema of method, following a local $ref.
func (c openRPC) resultSchema(t *testing.T, method string) jsonSchema {
	t.Helper()
	for _, m := range c.Methods {
		if m.Name == method {
			return c.resolve(t, m.Result.Schema)
		}
	}
	t.Fatalf("method %q not in contract", method)
	return jsonSchema{}
}

// paramNames returns the sorted param names of method and the sorted required ones.
func (c openRPC) paramNames(t *testing.T, method string) (all, required []string) {
	t.Helper()
	for _, m := range c.Methods {
		if m.Name != method {
			continue
		}
		for _, p := range m.Params {
			all = append(all, p.Name)
			if p.Required {
				required = append(required, p.Name)
			}
		}
		slices.Sort(all)
		slices.Sort(required)
		return all, required
	}
	t.Fatalf("method %q not in contract", method)
	return nil, nil
}

// paramEnum returns the enum of the param name of method (nil when it has none).
func (c openRPC) paramEnum(t *testing.T, method, name string) []string {
	t.Helper()
	for _, m := range c.Methods {
		if m.Name != method {
			continue
		}
		for _, p := range m.Params {
			if p.Name == name {
				return p.Schema.Enum
			}
		}
		t.Fatalf("%s has no param %q in contract", method, name)
	}
	t.Fatalf("method %q not in contract", method)
	return nil
}

// errorData returns the resolved data schema of the named contract error.
func (c openRPC) errorData(t *testing.T, name string) jsonSchema {
	t.Helper()
	e, ok := c.Components.Errors[name]
	if !ok {
		t.Fatalf("error %q not in contract", name)
	}
	if e.Data == nil {
		t.Fatalf("contract error %q has no data schema", name)
	}
	return c.resolve(t, *e.Data)
}

// assertExactFields checks that schema has required == properties and that the
// JSON of v has exactly those members.
func assertExactFields(t *testing.T, what string, schema jsonSchema, v any) {
	t.Helper()
	want := sortedKeys(schema.Properties)
	if len(want) == 0 {
		t.Fatalf("contract %s has no properties", what)
	}
	if got := slices.Sorted(slices.Values(schema.Required)); !slices.Equal(got, want) {
		t.Errorf("contract %s: required %v != properties %v", what, got, want)
	}
	if got := jsonFieldNames(t, v); !slices.Equal(got, want) {
		t.Errorf("%T JSON fields = %v, want exactly %v (contract %s)", v, got, want, what)
	}
}

// assertEnumMatchesDomain checks that the contract enum is exactly the Go
// values, that every contract value is accepted by parse and that a value
// outside the enum is refused.
func assertEnumMatchesDomain(t *testing.T, what string, contract, goValues []string, parse func(string) error) {
	t.Helper()
	if got, want := slices.Sorted(slices.Values(contract)), slices.Sorted(slices.Values(goValues)); !slices.Equal(got, want) {
		t.Errorf("%s: contract enum %v, domain values %v", what, got, want)
	}
	for _, v := range contract {
		if err := parse(v); err != nil {
			t.Errorf("%s: domain refuses contract value %q: %v", what, v, err)
		}
	}
	if err := parse("__fora-do-enum__"); err == nil {
		t.Errorf("%s: domain accepts a value outside the contract enum", what)
	}
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

// phase6Methods is the complete method set of contract v0.6.0 (notifications
// included) with their x-notification flag.
var phase6Methods = map[string]bool{
	ipc.MethodPing:                false,
	ipc.MethodStatus:              false,
	ipc.MethodHeartbeat:           true,
	ipc.MethodShutdown:            true,
	ipc.MethodGameAdd:             false,
	ipc.MethodGameList:            false,
	ipc.MethodGameGet:             false,
	ipc.MethodGameUpdate:          false,
	ipc.MethodGameDelete:          false,
	ipc.MethodGameLaunch:          false,
	ipc.MethodGameLaunchUntracked: false,
	ipc.MethodSessionCurrent:      false,
	ipc.MethodSessionPause:        false,
	ipc.MethodSessionResume:       false,
	ipc.MethodSessionList:         false,
	ipc.MethodPlatformList:        false,
	ipc.MethodPlatformAdd:         false,
	ipc.MethodPlatformUpdate:      false,
	ipc.MethodPlatformSetIcon:     false,
	ipc.MethodPlatformDelete:      false,
	ipc.MethodLibraryCatalog:      false,
	ipc.MethodImportScan:          false,
	ipc.MethodImportApply:         false,
	ipc.MethodEmulatorList:        false,
	ipc.MethodEmulatorAdd:         false,
	ipc.MethodEmulatorUpdate:      false,
	ipc.MethodEmulatorDelete:      false,
	ipc.MethodPlatformSetEmulator: false,
	ipc.MethodSessionChanged:      true,
	ipc.MethodProviderKeyStatus:   false,
	ipc.MethodProviderSetKey:      false,
	ipc.MethodProviderDeleteKey:   false,
	ipc.MethodScrapeSearch:        false,
	ipc.MethodScrapeApply:         false,
	ipc.MethodMediaList:           false,
	ipc.MethodMediaQueue:          false,
	ipc.MethodMediaCancel:         false,
	ipc.MethodStatsSummary:        false,
	ipc.MethodStatsWeekly:         false,
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
		// The agent.* methods; game.*, session.*, platform.* and library.* are served by
		// internal/agent and are covered by "IPC-041/os sete métodos novos e os antigos existem com os mesmos nomes".
		var names []string
		for _, m := range c.Methods {
			if strings.HasPrefix(m.Name, "agent.") {
				names = append(names, m.Name)
			}
		}
		slices.Sort(names)
		want := []string{ipc.MethodPing, ipc.MethodStatus, ipc.MethodHeartbeat, ipc.MethodShutdown}
		slices.Sort(want)
		if !slices.Equal(names, want) {
			t.Errorf("contract agent.* methods = %v, Go constants = %v", names, want)
		}

		// Every agent.* non-notification method in the contract has a handler in
		// AgentHandlers; notifications don't.
		handlers := testHandlers()
		for _, m := range c.Methods {
			if !strings.HasPrefix(m.Name, "agent.") {
				continue
			}
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
			// Application codes (v0.4.0) and -32005 added in v0.5.0 [IPC-032].
			"GameNotFound":        ipc.CodeGameNotFound,
			"SessionActive":       ipc.CodeSessionActive,
			"LaunchFailed":        ipc.CodeLaunchFailed,
			"InvalidExecutable":   ipc.CodeInvalidExecutable,
			"SessionNotPausable":  ipc.CodeSessionNotPausable,
			"ProviderNoKey":       ipc.CodeProviderNoKey,
			"ProviderUnavailable": ipc.CodeProviderUnavailable,
			"MediaJobNotFound":    ipc.CodeMediaJobNotFound,
			// PlatformNotFound shares -32001 with GameNotFound (no separate Go constant).
			"PlatformNotFound": ipc.CodeGameNotFound,
			"PlatformInUse":    ipc.CodePlatformInUse,
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

	t.Run("EXE-013/agent.shutdown é notificação com reason enum quit|session-end|signal|other", func(t *testing.T) {
		t.Parallel()
		if ipc.MethodShutdown != "agent.shutdown" {
			t.Fatalf("ipc.MethodShutdown = %q, want \"agent.shutdown\"", ipc.MethodShutdown)
		}
		found := false
		for _, m := range c.Methods {
			if m.Name != ipc.MethodShutdown {
				continue
			}
			found = true
			if !m.Notification {
				t.Errorf("contract %s: x-notification = false, want true", m.Name)
			}
			if len(m.Params) != 1 {
				t.Fatalf("contract %s params = %+v, want exactly [reason]", m.Name, m.Params)
			}
			p := m.Params[0]
			if p.Name != "reason" || !p.Required {
				t.Errorf("contract %s param = %q required=%v, want \"reason\" required", m.Name, p.Name, p.Required)
			}
			if p.Schema.Type != "string" {
				t.Errorf("contract reason type = %q, want \"string\"", p.Schema.Type)
			}
			wantEnum := []string{"quit", "session-end", "signal", "other"}
			if !slices.Equal(p.Schema.Enum, wantEnum) {
				t.Errorf("contract reason enum = %v, want %v", p.Schema.Enum, wantEnum)
			}
		}
		if !found {
			t.Fatalf("method %q not in contract", ipc.MethodShutdown)
		}
		if got := jsonFieldNames(t, ipc.ShutdownParams{}); !slices.Equal(got, []string{"reason"}) {
			t.Errorf("ipc.ShutdownParams JSON fields = %v, want exactly [reason]", got)
		}
		raw, err := json.Marshal(ipc.ShutdownParams{Reason: "session-end"})
		if err != nil {
			t.Fatalf("marshal ShutdownParams: %v", err)
		}
		if string(raw) != `{"reason":"session-end"}` {
			t.Errorf("ShutdownParams JSON = %s, want {\"reason\":\"session-end\"}", raw)
		}
		// agent.shutdown is server-to-client only: no handler for it.
		if _, has := testHandlers()[ipc.MethodShutdown]; has {
			t.Error("AgentHandlers has a handler for the server notification agent.shutdown")
		}
	})

	t.Run("EXE-014/x-limits.shutdownDrainMs = 500", func(t *testing.T) {
		t.Parallel()
		// The agent-side constant (shutdownDrainTimeout) is checked against
		// this value in internal/agent (T06); here the contract is pinned.
		if c.Info.Limits.ShutdownDrainMs == nil {
			t.Fatal("contract x-limits has no shutdownDrainMs")
		}
		if got := *c.Info.Limits.ShutdownDrainMs; got != 500 {
			t.Errorf("x-limits.shutdownDrainMs = %d, want 500", got)
		}
	})

	t.Run("IPC-001/versão JSON-RPC é 2.0", func(t *testing.T) {
		t.Parallel()
		if ipc.Version != "2.0" {
			t.Errorf("ipc.Version = %q, want \"2.0\"", ipc.Version)
		}
	})

	t.Run("IPC-041/versão 0.11.0", func(t *testing.T) {
		t.Parallel()
		if c.Info.Version != "0.11.0" {
			t.Fatalf("contract %s info.version = %q, want \"0.11.0\"", contractPath, c.Info.Version)
		}
	})

	t.Run("IPC-041/os sete métodos novos e os antigos existem com os mesmos nomes", func(t *testing.T) {
		t.Parallel()
		// Go constant -> literal wire name, for every method of v0.6.0.
		literals := map[string]string{
			"MethodPing":                ipc.MethodPing,
			"MethodStatus":              ipc.MethodStatus,
			"MethodHeartbeat":           ipc.MethodHeartbeat,
			"MethodShutdown":            ipc.MethodShutdown,
			"MethodGameAdd":             ipc.MethodGameAdd,
			"MethodGameList":            ipc.MethodGameList,
			"MethodGameLaunch":          ipc.MethodGameLaunch,
			"MethodGameLaunchUntracked": ipc.MethodGameLaunchUntracked,
			"MethodSessionCurrent":      ipc.MethodSessionCurrent,
			"MethodSessionPause":        ipc.MethodSessionPause,
			"MethodSessionResume":       ipc.MethodSessionResume,
			"MethodSessionChanged":      ipc.MethodSessionChanged,
			// The seven methods added in v0.6.0 [IPC-041].
			"MethodGameGet":             ipc.MethodGameGet,
			"MethodGameUpdate":          ipc.MethodGameUpdate,
			"MethodGameDelete":          ipc.MethodGameDelete,
			"MethodSessionList":         ipc.MethodSessionList,
			"MethodPlatformList":        ipc.MethodPlatformList,
			"MethodPlatformAdd":         ipc.MethodPlatformAdd,
			"MethodPlatformUpdate":      ipc.MethodPlatformUpdate,
			"MethodPlatformSetIcon":     ipc.MethodPlatformSetIcon,
			"MethodPlatformDelete":      ipc.MethodPlatformDelete,
			"MethodLibraryCatalog":      ipc.MethodLibraryCatalog,
			"MethodImportScan":          ipc.MethodImportScan,
			"MethodImportApply":         ipc.MethodImportApply,
			"MethodEmulatorList":        ipc.MethodEmulatorList,
			"MethodEmulatorAdd":         ipc.MethodEmulatorAdd,
			"MethodEmulatorUpdate":      ipc.MethodEmulatorUpdate,
			"MethodEmulatorDelete":      ipc.MethodEmulatorDelete,
			"MethodPlatformSetEmulator": ipc.MethodPlatformSetEmulator,
		}
		wantLiterals := map[string]string{
			"MethodPing":                "agent.ping",
			"MethodStatus":              "agent.status",
			"MethodHeartbeat":           "agent.heartbeat",
			"MethodShutdown":            "agent.shutdown",
			"MethodGameAdd":             "game.add",
			"MethodGameList":            "game.list",
			"MethodGameLaunch":          "game.launch",
			"MethodGameLaunchUntracked": "game.launchUntracked",
			"MethodSessionCurrent":      "session.current",
			"MethodSessionPause":        "session.pause",
			"MethodSessionResume":       "session.resume",
			"MethodSessionChanged":      "session.changed",
			"MethodGameGet":             "game.get",
			"MethodGameUpdate":          "game.update",
			"MethodGameDelete":          "game.delete",
			"MethodSessionList":         "session.list",
			"MethodPlatformList":        "platform.list",
			"MethodPlatformAdd":         "platform.add",
			"MethodPlatformUpdate":      "platform.update",
			"MethodPlatformSetIcon":     "platform.setIcon",
			"MethodPlatformDelete":      "platform.delete",
			"MethodLibraryCatalog":      "library.catalog",
			"MethodImportScan":          "import.scan",
			"MethodImportApply":         "import.apply",
			"MethodEmulatorList":        "emulator.list",
			"MethodEmulatorAdd":         "emulator.add",
			"MethodEmulatorUpdate":      "emulator.update",
			"MethodEmulatorDelete":      "emulator.delete",
			"MethodPlatformSetEmulator": "platform.setEmulator",
		}
		for name, got := range literals {
			if got != wantLiterals[name] {
				t.Errorf("ipc.%s = %q, want %q", name, got, wantLiterals[name])
			}
		}

		got := map[string]bool{}
		for _, m := range c.Methods {
			if _, dup := got[m.Name]; dup {
				t.Errorf("contract lists %q twice", m.Name)
			}
			got[m.Name] = m.Notification
		}
		if g, w := sortedKeys(got), sortedKeys(phase6Methods); !slices.Equal(g, w) {
			t.Fatalf("contract methods = %v, Go constants = %v", g, w)
		}
		for name, notif := range phase6Methods {
			if got[name] != notif {
				t.Errorf("contract %s: x-notification = %v, want %v", name, got[name], notif)
			}
		}
	})

	t.Run("IPC-041/parâmetros por nome dos métodos novos batem com as tags Go", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			method       string
			v            any
			wantAll      []string
			wantRequired []string
		}{
			{ipc.MethodGameList, ipc.GameListParams{Filter: &domain.FilterSpec{Version: 1}, Sort: libPtr("title"), Offset: libPtr(0), Limit: libPtr(1)},
				[]string{"filter", "limit", "offset", "sort"}, nil},
			{ipc.MethodGameUpdate, ipc.GameUpdateParams{GameID: 1}, []string{"completedAt", "description", "developers", "emulatorId", "favorite", "gameId", "genres", "launchArgs", "launchTarget", "launchType", "platformId", "playModes", "publishers", "status", "title", "year"}, []string{"gameId"}},
			{ipc.MethodSessionList, ipc.SessionListParams{GameID: 1, Offset: libPtr(0), Limit: libPtr(1)}, []string{"gameId", "limit", "offset"}, []string{"gameId"}},
			{ipc.MethodPlatformAdd, ipc.PlatformAddParams{Name: "PC", Category: "pc"}, []string{"category", "name"}, []string{"category", "name"}},
			{ipc.MethodPlatformUpdate, ipc.PlatformUpdateParams{ID: 1, Name: "PC", Category: "pc"}, []string{"category", "id", "name"}, []string{"category", "id", "name"}},
			{ipc.MethodPlatformDelete, ipc.PlatformDeleteParams{ID: 1}, []string{"id"}, []string{"id"}},
		} {
			all, required := c.paramNames(t, tc.method)
			if !slices.Equal(all, tc.wantAll) || !slices.Equal(required, tc.wantRequired) {
				t.Errorf("contract %s params = %v (required %v), want %v (required %v)",
					tc.method, all, required, tc.wantAll, tc.wantRequired)
			}
			if got := jsonFieldNames(t, tc.v); !slices.Equal(got, tc.wantAll) {
				t.Errorf("%T JSON fields = %v, want exactly %v", tc.v, got, tc.wantAll)
			}
		}
		// Methods by name with only gameId (no Go params type named in the plan).
		for _, m := range []string{ipc.MethodGameGet, ipc.MethodGameDelete} {
			if all, required := c.paramNames(t, m); !slices.Equal(all, []string{"gameId"}) || !slices.Equal(required, []string{"gameId"}) {
				t.Errorf("contract %s params = %v (required %v), want [gameId] required", m, all, required)
			}
		}
		// Methods without params.
		for _, m := range []string{ipc.MethodPlatformList, ipc.MethodLibraryCatalog} {
			if all, _ := c.paramNames(t, m); len(all) != 0 {
				t.Errorf("contract %s params = %v, want none", m, all)
			}
		}
	})

	t.Run("IPC-041/enums sort, playMode, status, played e category batem com o domain", func(t *testing.T) {
		t.Parallel()
		// sort: x-library.sortValues, the sort param of game.list and domain.ParseSort.
		sortGo := []string{string(domain.SortTitle), string(domain.SortLastPlayed), string(domain.SortTotalPlayed), string(domain.SortYear)}
		if c.Info.Library == nil {
			t.Fatal("contract info has no x-library")
		}
		assertEnumMatchesDomain(t, "sort (x-library.sortValues)", c.Info.Library.SortValues, sortGo,
			func(s string) error { _, err := domain.ParseSort(s); return err })
		assertEnumMatchesDomain(t, "sort (game.list param)", c.paramEnum(t, ipc.MethodGameList, "sort"), sortGo,
			func(s string) error { _, err := domain.ParseSort(s); return err })
		if got := c.Info.Library.DefaultSort; got != string(domain.DefaultSort) {
			t.Errorf("x-library.defaultSort = %q, domain.DefaultSort = %q", got, domain.DefaultSort)
		}

		// playMode: the group of the filter (DOM-014) and the five mode names.
		assertEnumMatchesDomain(t, "playMode (PlayModeGroup)", c.schema(t, "PlayModeGroup").Enum,
			[]string{string(domain.PlayGroupAll), string(domain.PlayGroupSingle), string(domain.PlayGroupMulti), string(domain.PlayGroupCoop)},
			func(s string) error { _, err := domain.ParsePlayModeGroup(s); return err })
		assertEnumMatchesDomain(t, "playMode (PlayModeName)", c.schema(t, "PlayModeName").Enum,
			[]string{string(domain.PlayModeSingle), string(domain.PlayModeMultiLocal), string(domain.PlayModeMultiOnline), string(domain.PlayModeCoopLocal), string(domain.PlayModeCoopOnline)},
			func(s string) error { _, err := domain.ParsePlayMode(s); return err })

		// status: components.schemas.GameStatus and DOM-012.
		assertEnumMatchesDomain(t, "status (GameStatus)", c.schema(t, "GameStatus").Enum,
			[]string{string(domain.StatusNotPlayed), string(domain.StatusPlaying), string(domain.StatusCompleted), string(domain.StatusAbandoned), string(domain.StatusBacklog)},
			func(s string) error { _, err := domain.ParseGameStatus(s); return err })

		// played: FilterSpec.properties.played, validated by FilterSpec.Validate (FLT-006).
		var played struct {
			Enum []string `json:"enum"`
		}
		raw := c.schema(t, "FilterSpec").Properties["played"]
		if err := json.Unmarshal(raw, &played); err != nil {
			t.Fatalf("parse FilterSpec.played schema %s: %v", raw, err)
		}
		assertEnumMatchesDomain(t, "played (FilterSpec)", played.Enum, []string{"any", "played", "notPlayed"},
			func(s string) error { return domain.FilterSpec{Version: domain.FilterVersion, Played: s}.Validate() })

		// category: components.schemas.PlatformCategory and DOM-015.
		assertEnumMatchesDomain(t, "category (PlatformCategory)", c.schema(t, "PlatformCategory").Enum,
			[]string{string(domain.CategoryPC), string(domain.CategoryConsole), string(domain.CategoryArcade), string(domain.CategoryHandheld)},
			func(s string) error { _, err := domain.ParsePlatformCategory(s); return err })
	})

	t.Run("IPC-041/x-limits batem com as constantes de domain e com MaxGameTitleRunes", func(t *testing.T) {
		t.Parallel()
		l := c.Info.Limits
		for _, tc := range []struct {
			name string
			got  *int
			want int
		}{
			{"maxGameTitleRunes", l.MaxGameTitleRunes, domain.MaxTitleRunes},
			{"maxNameRunes", l.MaxNameRunes, domain.MaxNameRunes},
			{"maxDescriptionRunes", l.MaxDescriptionRunes, domain.MaxDescriptionRunes},
			{"maxFilterTextRunes", l.MaxFilterTextRunes, domain.MaxFilterTextRunes},
			{"maxFilterIds", l.MaxFilterIds, domain.MaxFilterIDs},
			{"yearMin", l.YearMin, domain.YearMin},
			{"yearMax", l.YearMax, domain.YearMax},
			{"defaultGameListLimit", l.DefaultGameListLimit, domain.DefaultPageLimit},
			{"maxGameListLimit", l.MaxGameListLimit, domain.MaxPageLimit},
			{"defaultSessionListLimit", l.DefaultSessionListLimit, domain.DefaultSessionLimit},
			{"maxSessionListLimit", l.MaxSessionListLimit, domain.MaxSessionLimit},
		} {
			if tc.got == nil {
				t.Errorf("contract x-limits has no %s", tc.name)
				continue
			}
			if *tc.got != tc.want {
				t.Errorf("x-limits.%s = %d, domain constant = %d", tc.name, *tc.got, tc.want)
			}
		}
		if ipc.MaxGameTitleRunes != domain.MaxTitleRunes {
			t.Errorf("ipc.MaxGameTitleRunes = %d, domain.MaxTitleRunes = %d", ipc.MaxGameTitleRunes, domain.MaxTitleRunes)
		}
		if ipc.MaxGameTitleRunes != 200 {
			t.Errorf("ipc.MaxGameTitleRunes = %d, want 200", ipc.MaxGameTitleRunes)
		}
	})

	t.Run("IPC-041/campos obrigatórios de Game, GameDetail, GameListResult, SessionItem, SessionListResult e Catalog batem com as tags Go", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			schema string
			v      any
		}{
			{"Game", ipc.Game{}},
			{"GameDetail", ipc.GameDetail{}},
			{"GameListResult", ipc.GameListResult{}},
			{"SessionItem", ipc.SessionItem{}},
			{"SessionListResult", ipc.SessionListResult{}},
			{"Catalog", ipc.Catalog{}},
		} {
			assertExactFields(t, tc.schema, c.schema(t, tc.schema), tc.v)
		}

		// Method results of the new and changed methods.
		assertExactFields(t, "game.list result", c.resultSchema(t, ipc.MethodGameList), ipc.GameListResult{})
		assertExactFields(t, "game.get result", c.resultSchema(t, ipc.MethodGameGet), ipc.GameDetailResult{})
		assertExactFields(t, "game.update result", c.resultSchema(t, ipc.MethodGameUpdate), ipc.GameDetailResult{})
		assertExactFields(t, "game.delete result", c.resultSchema(t, ipc.MethodGameDelete), ipc.GameDeletedResult{})
		assertExactFields(t, "session.list result", c.resultSchema(t, ipc.MethodSessionList), ipc.SessionListResult{})
		assertExactFields(t, "platform.list result", c.resultSchema(t, ipc.MethodPlatformList), ipc.PlatformListResult{})
		assertExactFields(t, "platform.add result", c.resultSchema(t, ipc.MethodPlatformAdd), ipc.PlatformAddResult{})
		assertExactFields(t, "platform.update result", c.resultSchema(t, ipc.MethodPlatformUpdate), ipc.PlatformResult{})
		assertExactFields(t, "platform.setIcon result", c.resultSchema(t, ipc.MethodPlatformSetIcon), ipc.PlatformResult{})
		assertExactFields(t, "platform.delete result", c.resultSchema(t, ipc.MethodPlatformDelete), ipc.PlatformDeletedResult{})
		assertExactFields(t, "library.catalog result", c.resultSchema(t, ipc.MethodLibraryCatalog), ipc.Catalog{})
	})

	t.Run("IPC-049/Catalog: genres e companies são itens {id, name} (id inteiro, name texto) no contrato", func(t *testing.T) {
		t.Parallel()
		cc := loadContract(t)
		catalog := cc.schema(t, "Catalog")
		for _, prop := range []string{"genres", "companies"} {
			var list jsonSchema
			if err := json.Unmarshal(catalog.Properties[prop], &list); err != nil {
				t.Fatalf("Catalog.%s schema: %v", prop, err)
			}
			if list.Type != "array" {
				t.Errorf("Catalog.%s type = %q, want array", prop, list.Type)
			}
			var items jsonSchema
			if err := json.Unmarshal(list.Items, &items); err != nil {
				t.Fatalf("Catalog.%s items: %v", prop, err)
			}
			item := cc.resolve(t, items)
			// Exactly the members id and name, both required [IPC-049, FLT-002].
			assertExactFields(t, "Catalog."+prop+" item", item, map[string]any{"id": 1, "name": "RPG"})
			var idSchema, nameSchema struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(item.Properties["id"], &idSchema); err != nil || idSchema.Type != "integer" {
				t.Errorf("Catalog.%s item id type = %q (%v), want integer", prop, idSchema.Type, err)
			}
			if err := json.Unmarshal(item.Properties["name"], &nameSchema); err != nil || nameSchema.Type != "string" {
				t.Errorf("Catalog.%s item name type = %q (%v), want string", prop, nameSchema.Type, err)
			}
		}
	})

	t.Run("IPC-041/campos obrigatórios de Session, SessionRef, SessionResult e LaunchedProcess batem com as tags Go", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			schema string
			v      any
		}{
			{"Session", ipc.Session{}},
			{"SessionRef", ipc.SessionRef{}},
			{"SessionResult", ipc.SessionResult{}},
			{"LaunchedProcess", ipc.LaunchedProcess{}},
		} {
			assertExactFields(t, tc.schema, c.schema(t, tc.schema), tc.v)
		}
		assertExactFields(t, "game.add result", c.resultSchema(t, ipc.MethodGameAdd), ipc.Game{})
		assertExactFields(t, "game.launch result", c.resultSchema(t, ipc.MethodGameLaunch), ipc.Session{})
		assertExactFields(t, "session.current result", c.resultSchema(t, ipc.MethodSessionCurrent), ipc.SessionCurrentResult{})
		assertExactFields(t, "game.launchUntracked result", c.resultSchema(t, ipc.MethodGameLaunchUntracked), ipc.LaunchedProcess{})
		assertExactFields(t, "session.pause result", c.resultSchema(t, ipc.MethodSessionPause), ipc.SessionResult{})
		assertExactFields(t, "session.resume result", c.resultSchema(t, ipc.MethodSessionResume), ipc.SessionResult{})
		assertExactFields(t, "SessionNotPausable data", c.errorData(t, "SessionNotPausable"), ipc.SessionNotPausableData{})

		// game.add params (unchanged in v0.6.0): title is optional and omitted when empty.
		all, required := c.paramNames(t, ipc.MethodGameAdd)
		if !slices.Equal(all, []string{"path", "title"}) || !slices.Equal(required, []string{"path"}) {
			t.Errorf("contract game.add params = %v (required %v)", all, required)
		}
		if got := jsonFieldNames(t, ipc.GameAddParams{Path: `C:\g.exe`}); !slices.Equal(got, []string{"path"}) {
			t.Errorf("GameAddParams without title JSON fields = %v, want [path]", got)
		}
		if got := jsonFieldNames(t, ipc.GameAddParams{Path: `C:\g.exe`, Title: "G"}); !slices.Equal(got, []string{"path", "title"}) {
			t.Errorf("GameAddParams JSON fields = %v, want [path title]", got)
		}
		// The reason enum of SessionNotPausable is exactly the Reason* constants.
		var reason struct {
			Enum []string `json:"enum"`
		}
		raw := c.errorData(t, "SessionNotPausable").Properties["reason"]
		if err := json.Unmarshal(raw, &reason); err != nil {
			t.Fatalf("parse reason schema %s: %v", raw, err)
		}
		if want := []string{ipc.ReasonNoSession, ipc.ReasonGrace}; !slices.Equal(reason.Enum, want) {
			t.Errorf("contract reason enum = %v, Go constants = %v", reason.Enum, want)
		}
		if ipc.ReasonNoSession != "no-session" || ipc.ReasonGrace != "grace" {
			t.Errorf("Reason* = %q, %q, want \"no-session\", \"grace\"", ipc.ReasonNoSession, ipc.ReasonGrace)
		}
	})

	t.Run("IPC-041/enum Session.state contém running, paused, grace, ended e discarded", func(t *testing.T) {
		t.Parallel()
		s := c.schema(t, "Session")
		var state struct {
			Enum []string `json:"enum"`
		}
		raw := s.Properties["state"]
		if err := json.Unmarshal(raw, &state); err != nil {
			t.Fatalf("parse Session.state schema %s: %v", raw, err)
		}
		want := []string{"running", "paused", "grace", "ended", "discarded"}
		if !slices.Equal(state.Enum, want) {
			t.Errorf("Session.state enum = %v, want %v", state.Enum, want)
		}
		// The domain state names are the wire values.
		for _, st := range []string{string(domain.SessionRunning), string(domain.SessionPaused), string(domain.SessionGrace)} {
			if !slices.Contains(state.Enum, st) {
				t.Errorf("domain state %q is not in the contract enum %v", st, state.Enum)
			}
		}
	})

	t.Run("IPC-041/códigos de erro batem com as constantes", func(t *testing.T) {
		t.Parallel()
		want := map[string]int{
			"ParseError":          ipc.CodeParseError,
			"InvalidRequest":      ipc.CodeInvalidRequest,
			"MethodNotFound":      ipc.CodeMethodNotFound,
			"InvalidParams":       ipc.CodeInvalidParams,
			"InternalError":       ipc.CodeInternalError,
			"GameNotFound":        ipc.CodeGameNotFound,
			"SessionActive":       ipc.CodeSessionActive,
			"LaunchFailed":        ipc.CodeLaunchFailed,
			"InvalidExecutable":   ipc.CodeInvalidExecutable,
			"SessionNotPausable":  ipc.CodeSessionNotPausable,
			"ProviderNoKey":       ipc.CodeProviderNoKey,
			"ProviderUnavailable": ipc.CodeProviderUnavailable,
			"MediaJobNotFound":    ipc.CodeMediaJobNotFound,
			// PlatformNotFound shares -32001 with GameNotFound (no separate Go constant).
			"PlatformNotFound": ipc.CodeGameNotFound,
			"PlatformInUse":    ipc.CodePlatformInUse,
		}
		if got, w := sortedKeys(c.Components.Errors), sortedKeys(want); !slices.Equal(got, w) {
			t.Fatalf("contract errors = %v, Go constants cover %v", got, w)
		}
		for name, code := range want {
			if got := c.Components.Errors[name].Code; got != code {
				t.Errorf("%s: contract code %d, Go constant %d", name, got, code)
			}
		}
		// Messages of the application errors (the v0.5.0 codes; v0.6.0 adds none [IPC-041]).
		for _, tc := range []struct {
			name string
			msg  string
			code int
		}{
			{"GameNotFound", ipc.MsgGameNotFound, ipc.CodeGameNotFound},
			{"SessionActive", ipc.MsgSessionActive, ipc.CodeSessionActive},
			{"LaunchFailed", ipc.MsgLaunchFailed, ipc.CodeLaunchFailed},
			{"InvalidExecutable", ipc.MsgInvalidExecutable, ipc.CodeInvalidExecutable},
			{"SessionNotPausable", ipc.MsgSessionNotPausable, ipc.CodeSessionNotPausable},
			{"InvalidParams", ipc.MsgInvalidParams, ipc.CodeInvalidParams},
		} {
			e := c.Components.Errors[tc.name]
			if e.Message != tc.msg {
				t.Errorf("%s: contract message %q, Go constant %q", tc.name, e.Message, tc.msg)
			}
			if tc.code != want[tc.name] {
				t.Errorf("%s: Go code %d, want %d", tc.name, tc.code, want[tc.name])
			}
		}
	})

	t.Run("IPC-041/data de LaunchFailed, InvalidExecutable, GameNotFound e SessionActive batem com os tipos", func(t *testing.T) {
		t.Parallel()
		win := uint32(2)
		for _, tc := range []struct {
			name         string
			full         any // every optional member set
			wantRequired []string
		}{
			{"LaunchFailed", ipc.LaunchFailedData{Error: "x", WinError: &win}, []string{"error"}},
			{"InvalidExecutable", ipc.InvalidExecutableData{Reason: ipc.ReasonNotExe}, []string{"reason"}},
			{"GameNotFound", ipc.GameNotFoundData{GameID: 5}, []string{"gameId"}},
			{"SessionActive", ipc.SessionRef{SessionID: 1, GameID: 2, Title: "t"}, []string{"gameId", "sessionId", "title"}},
		} {
			s := c.errorData(t, tc.name)
			props := sortedKeys(s.Properties)
			if got := jsonFieldNames(t, tc.full); !slices.Equal(got, props) {
				t.Errorf("%T JSON fields = %v, contract %s data properties %v", tc.full, got, tc.name, props)
			}
			if got := slices.Sorted(slices.Values(s.Required)); !slices.Equal(got, tc.wantRequired) {
				t.Errorf("contract %s data required = %v, want %v", tc.name, got, tc.wantRequired)
			}
		}

		// The InvalidExecutable reason enum is exactly the Reason* constants.
		var reason struct {
			Enum []string `json:"enum"`
		}
		raw := c.errorData(t, "InvalidExecutable").Properties["reason"]
		if err := json.Unmarshal(raw, &reason); err != nil {
			t.Fatalf("parse reason schema %s: %v", raw, err)
		}
		want := []string{ipc.ReasonNotAbsolute, ipc.ReasonNotFound, ipc.ReasonNotAFile, ipc.ReasonNotExe}
		if !slices.Equal(reason.Enum, want) {
			t.Errorf("contract reason enum = %v, Go constants = %v", reason.Enum, want)
		}
		if lit := []string{"not-absolute", "not-found", "not-a-file", "not-exe"}; !slices.Equal(want, lit) {
			t.Errorf("Reason* constants = %v, want %v", want, lit)
		}
	})

	t.Run("IPC-041/x-tracking bate com as constantes do domain", func(t *testing.T) {
		t.Parallel()
		tr := c.Info.Tracking
		if tr == nil {
			t.Fatal("contract info has no x-tracking")
		}
		for _, tc := range []struct {
			name string
			got  *int
			want time.Duration
		}{
			{"gracePeriodSeconds", tr.GracePeriodSeconds, domain.SessionGracePeriod},
			{"minCountedSessionSeconds", tr.MinCountedSessionSeconds, domain.MinCountedSession},
			{"sessionHeartbeatSeconds", tr.SessionHeartbeatSeconds, domain.SessionHeartbeatInterval},
			{"jobPollSeconds", tr.JobPollSeconds, domain.JobPollInterval},
		} {
			if tc.got == nil {
				t.Errorf("contract x-tracking has no %s", tc.name)
				continue
			}
			if got := time.Duration(*tc.got) * time.Second; got != tc.want {
				t.Errorf("x-tracking.%s = %v, domain constant %v", tc.name, got, tc.want)
			}
		}
		if tr.MaxConcurrentSessions == nil || *tr.MaxConcurrentSessions != 1 {
			t.Errorf("x-tracking.maxConcurrentSessions = %v, want 1", tr.MaxConcurrentSessions)
		}
	})

	t.Run("IPC-041/comentário do pacote aponta para phase-6b-emulators", func(t *testing.T) {
		t.Parallel()
		files, err := filepath.Glob("*.go")
		if err != nil {
			t.Fatalf("glob: %v", err)
		}
		fset := token.NewFileSet()
		var docs []string
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			af, err := parser.ParseFile(fset, f, nil, parser.PackageClauseOnly|parser.ParseComments)
			if err != nil {
				t.Fatalf("parse %s: %v", f, err)
			}
			if af.Doc != nil {
				docs = append(docs, af.Doc.Text())
			}
		}
		if len(docs) != 1 {
			t.Fatalf("package ipc has %d package comments, want exactly 1", len(docs))
		}
		doc := strings.Join(strings.Fields(docs[0]), " ")
		if !strings.Contains(doc, contractRelPath) {
			t.Errorf("package comment does not reference %s:\n%s", contractRelPath, docs[0])
		}
	})
}
