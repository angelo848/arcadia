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

// contractPath is the source of truth: the phase-4-pause contract (v0.5.0)
// supersedes phase-3-tracker (v0.4.0) [IPC-032].
var contractPath = filepath.Join("..", "..", "docs", "specs", "phase-4-pause", "contracts", "ipc.openrpc.json")

// contractRelPath is how the package comment must name the contract.
const contractRelPath = "docs/specs/phase-4-pause/contracts/ipc.openrpc.json"

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
		} `json:"x-limits"`
		Tracking *struct {
			GracePeriodSeconds       *int `json:"gracePeriodSeconds"`
			MinCountedSessionSeconds *int `json:"minCountedSessionSeconds"`
			SessionHeartbeatSeconds  *int `json:"sessionHeartbeatSeconds"`
			JobPollSeconds           *int `json:"jobPollSeconds"`
			MaxConcurrentSessions    *int `json:"maxConcurrentSessions"`
		} `json:"x-tracking"`
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
			Code    int         `json:"code"`
			Message string      `json:"message"`
			Data    *jsonSchema `json:"data"`
		} `json:"errors"`
	} `json:"components"`
}

type jsonSchema struct {
	Ref        string                     `json:"$ref"`
	Type       string                     `json:"type"`
	Required   []string                   `json:"required"`
	Properties map[string]json.RawMessage `json:"properties"`
}

// loadContract parses the contract. The version is asserted by the
// "IPC-032/versão 0.5.0" case.
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
		// The agent.* methods; game.* and session.* are served by internal/agent
		// and are covered by "IPC-032/todos os métodos e notificações do contrato existem com os mesmos nomes, inclusive game.launchUntracked, session.pause e session.resume".
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
			"GameNotFound":       ipc.CodeGameNotFound,
			"SessionActive":      ipc.CodeSessionActive,
			"LaunchFailed":       ipc.CodeLaunchFailed,
			"InvalidExecutable":  ipc.CodeInvalidExecutable,
			"SessionNotPausable": ipc.CodeSessionNotPausable,
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

	t.Run("IPC-032/versão 0.5.0", func(t *testing.T) {
		t.Parallel()
		if c.Info.Version != "0.5.0" {
			t.Fatalf("contract %s info.version = %q, want \"0.5.0\"", contractPath, c.Info.Version)
		}
	})

	t.Run("IPC-032/todos os métodos e notificações do contrato existem com os mesmos nomes, inclusive game.launchUntracked, session.pause e session.resume", func(t *testing.T) {
		t.Parallel()
		// Go constant -> whether it is a server-to-client notification.
		want := map[string]bool{
			ipc.MethodPing:                false,
			ipc.MethodStatus:              false,
			ipc.MethodHeartbeat:           true,
			ipc.MethodShutdown:            true,
			ipc.MethodGameAdd:             false,
			ipc.MethodGameList:            false,
			ipc.MethodGameLaunch:          false,
			ipc.MethodGameLaunchUntracked: false,
			ipc.MethodSessionCurrent:      false,
			ipc.MethodSessionPause:        false,
			ipc.MethodSessionResume:       false,
			ipc.MethodSessionChanged:      true,
		}
		literals := map[string]string{
			"MethodGameAdd":             ipc.MethodGameAdd,
			"MethodGameList":            ipc.MethodGameList,
			"MethodGameLaunch":          ipc.MethodGameLaunch,
			"MethodGameLaunchUntracked": ipc.MethodGameLaunchUntracked,
			"MethodSessionCurrent":      ipc.MethodSessionCurrent,
			"MethodSessionPause":        ipc.MethodSessionPause,
			"MethodSessionResume":       ipc.MethodSessionResume,
			"MethodSessionChanged":      ipc.MethodSessionChanged,
		}
		wantLiterals := map[string]string{
			"MethodGameAdd":             "game.add",
			"MethodGameList":            "game.list",
			"MethodGameLaunch":          "game.launch",
			"MethodGameLaunchUntracked": "game.launchUntracked",
			"MethodSessionCurrent":      "session.current",
			"MethodSessionPause":        "session.pause",
			"MethodSessionResume":       "session.resume",
			"MethodSessionChanged":      "session.changed",
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
		if g, w := sortedKeys(got), sortedKeys(want); !slices.Equal(g, w) {
			t.Fatalf("contract methods = %v, Go constants = %v", g, w)
		}
		for name, notif := range want {
			if got[name] != notif {
				t.Errorf("contract %s: x-notification = %v, want %v", name, got[name], notif)
			}
		}
	})

	t.Run("IPC-032/enum Session.state contém running, paused, grace, ended e discarded", func(t *testing.T) {
		t.Parallel()
		s, ok := c.Components.Schemas["Session"]
		if !ok {
			t.Fatal("contract has no components.schemas.Session")
		}
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

	t.Run("IPC-032/campos obrigatórios de LaunchedProcess, SessionResult e SessionNotPausableData batem com os tipos Go", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			schema string
			v      any
		}{
			{"LaunchedProcess", ipc.LaunchedProcess{}},
			{"SessionResult", ipc.SessionResult{}},
		} {
			s, ok := c.Components.Schemas[tc.schema]
			if !ok {
				t.Errorf("contract has no components.schemas.%s", tc.schema)
				continue
			}
			assertExactFields(t, tc.schema, s, tc.v)
		}
		assertExactFields(t, "SessionNotPausable data", c.errorData(t, "SessionNotPausable"), ipc.SessionNotPausableData{})

		// Method results and params.
		assertExactFields(t, "game.launchUntracked result", c.resultSchema(t, ipc.MethodGameLaunchUntracked), ipc.LaunchedProcess{})
		assertExactFields(t, "session.pause result", c.resultSchema(t, ipc.MethodSessionPause), ipc.SessionResult{})
		assertExactFields(t, "session.resume result", c.resultSchema(t, ipc.MethodSessionResume), ipc.SessionResult{})

		all, required := c.paramNames(t, ipc.MethodGameLaunchUntracked)
		if want := []string{"gameId"}; !slices.Equal(all, want) || !slices.Equal(required, want) {
			t.Errorf("contract game.launchUntracked params = %v (required %v), want [gameId] required", all, required)
		}
		if got := jsonFieldNames(t, ipc.GameLaunchUntrackedParams{GameID: 1}); !slices.Equal(got, []string{"gameId"}) {
			t.Errorf("GameLaunchUntrackedParams JSON fields = %v, want [gameId]", got)
		}
		for _, m := range []string{ipc.MethodSessionPause, ipc.MethodSessionResume} {
			if all, _ := c.paramNames(t, m); len(all) != 0 {
				t.Errorf("contract %s params = %v, want none", m, all)
			}
		}

		// SessionNotPausable reason enum is exactly the Reason* constants.
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

	t.Run("IPC-032/campos obrigatórios de Game, Session e SessionRef batem com as tags JSON", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			schema string
			v      any
		}{
			{"Game", ipc.Game{}},
			{"Session", ipc.Session{}},
			{"SessionRef", ipc.SessionRef{}},
		} {
			s, ok := c.Components.Schemas[tc.schema]
			if !ok {
				t.Errorf("contract has no components.schemas.%s", tc.schema)
				continue
			}
			assertExactFields(t, tc.schema, s, tc.v)
		}

		// Method results.
		assertExactFields(t, "game.add result", c.resultSchema(t, ipc.MethodGameAdd), ipc.Game{})
		assertExactFields(t, "game.launch result", c.resultSchema(t, ipc.MethodGameLaunch), ipc.Session{})
		assertExactFields(t, "game.list result", c.resultSchema(t, ipc.MethodGameList), ipc.GameListResult{})
		assertExactFields(t, "session.current result", c.resultSchema(t, ipc.MethodSessionCurrent), ipc.SessionCurrentResult{})

		// Params by name.
		for _, tc := range []struct {
			method       string
			v            any
			wantAll      []string
			wantRequired []string
		}{
			{ipc.MethodGameAdd, ipc.GameAddParams{Path: `C:\g.exe`, Title: "G"}, []string{"path", "title"}, []string{"path"}},
			{ipc.MethodGameLaunch, ipc.GameLaunchParams{GameID: 1}, []string{"gameId"}, []string{"gameId"}},
			{ipc.MethodSessionChanged, ipc.SessionChangedParams{}, []string{"session"}, []string{"session"}},
		} {
			all, required := c.paramNames(t, tc.method)
			if !slices.Equal(all, tc.wantAll) || !slices.Equal(required, tc.wantRequired) {
				t.Errorf("contract %s params = %v (required %v), want %v (required %v)",
					tc.method, all, required, tc.wantAll, tc.wantRequired)
			}
			if got := jsonFieldNames(t, tc.v); !slices.Equal(got, all) {
				t.Errorf("%T JSON fields = %v, want exactly %v", tc.v, got, all)
			}
		}
		// title is optional: an empty title is omitted on the wire.
		if got := jsonFieldNames(t, ipc.GameAddParams{Path: `C:\g.exe`}); !slices.Equal(got, []string{"path"}) {
			t.Errorf("GameAddParams without title JSON fields = %v, want [path]", got)
		}
		// Methods without params.
		for _, m := range []string{ipc.MethodGameList, ipc.MethodSessionCurrent} {
			if all, _ := c.paramNames(t, m); len(all) != 0 {
				t.Errorf("contract %s params = %v, want none", m, all)
			}
		}
	})

	t.Run("IPC-032/códigos e mensagens de erro batem com as constantes, inclusive -32005", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			code int
			msg  string
			lit  int
		}{
			{"GameNotFound", ipc.CodeGameNotFound, ipc.MsgGameNotFound, -32001},
			{"SessionActive", ipc.CodeSessionActive, ipc.MsgSessionActive, -32002},
			{"LaunchFailed", ipc.CodeLaunchFailed, ipc.MsgLaunchFailed, -32003},
			{"InvalidExecutable", ipc.CodeInvalidExecutable, ipc.MsgInvalidExecutable, -32004},
			{"SessionNotPausable", ipc.CodeSessionNotPausable, ipc.MsgSessionNotPausable, -32005},
			{"InvalidParams", ipc.CodeInvalidParams, ipc.MsgInvalidParams, -32602},
		} {
			e, ok := c.Components.Errors[tc.name]
			if !ok {
				t.Errorf("contract has no components.errors.%s", tc.name)
				continue
			}
			if tc.code != tc.lit {
				t.Errorf("ipc code for %s = %d, want %d", tc.name, tc.code, tc.lit)
			}
			if e.Code != tc.code {
				t.Errorf("%s: contract code %d, Go constant %d", tc.name, e.Code, tc.code)
			}
			if e.Message != tc.msg {
				t.Errorf("%s: contract message %q, Go constant %q", tc.name, e.Message, tc.msg)
			}
		}
	})

	t.Run("IPC-032/data de LaunchFailed, InvalidExecutable e GameNotFound batem com os tipos", func(t *testing.T) {
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

	t.Run("IPC-032/maxGameTitleRunes é MaxGameTitleRunes", func(t *testing.T) {
		t.Parallel()
		if c.Info.Limits.MaxGameTitleRunes == nil {
			t.Fatal("contract x-limits has no maxGameTitleRunes")
		}
		if got := *c.Info.Limits.MaxGameTitleRunes; got != ipc.MaxGameTitleRunes {
			t.Errorf("x-limits.maxGameTitleRunes = %d, ipc.MaxGameTitleRunes = %d", got, ipc.MaxGameTitleRunes)
		}
		if ipc.MaxGameTitleRunes != 200 {
			t.Errorf("ipc.MaxGameTitleRunes = %d, want 200", ipc.MaxGameTitleRunes)
		}
	})

	t.Run("IPC-032/x-tracking bate com as constantes do domain", func(t *testing.T) {
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

	t.Run("IPC-032/comentário do pacote aponta para phase-4-pause", func(t *testing.T) {
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
