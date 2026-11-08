package ipc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// testStart is a non-UTC instant so tests can verify the UTC conversion of startedAt.
var testStart = time.Date(2026, 11, 8, 10, 30, 0, 0, time.FixedZone("BRT", -3*60*60))

func testInfo() ipc.AgentInfo {
	return ipc.AgentInfo{
		Version:   "1.2.3",
		PID:       4242,
		StartedAt: testStart,
		DataDir:   `C:\Users\someone\AppData\Local\Arcadia`,
	}
}

func testHandlers() map[string]ipc.Handler {
	return ipc.AgentHandlers(testInfo(), domain.NewFakeClock(testStart))
}

// decodedResponse is the observable shape of a JSON-RPC response line.
type decodedResponse struct {
	keys   map[string]json.RawMessage
	id     string // compact JSON of "id"
	result json.RawMessage
	code   int
	msg    string
	hasErr bool
}

func compactJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("compact %q: %v", raw, err)
	}
	return buf.String()
}

func decodeResponse(t *testing.T, resp []byte) decodedResponse {
	t.Helper()
	if bytes.ContainsRune(resp, '\n') {
		t.Fatalf("response must not contain '\\n' (framing belongs to the Encoder): %q", resp)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(resp, &keys); err != nil {
		t.Fatalf("response is not a JSON object: %v (%q)", err, resp)
	}
	var d decodedResponse
	d.keys = keys

	var version string
	if err := json.Unmarshal(keys["jsonrpc"], &version); err != nil || version != "2.0" {
		t.Fatalf("response jsonrpc = %s, want \"2.0\" (%q)", keys["jsonrpc"], resp)
	}
	rawID, ok := keys["id"]
	if !ok {
		t.Fatalf("response has no id member: %q", resp)
	}
	d.id = compactJSON(t, rawID)

	rawErr, hasErr := keys["error"]
	rawRes, hasRes := keys["result"]
	if hasErr == hasRes {
		t.Fatalf("response must have exactly one of result/error: %q", resp)
	}
	if hasErr {
		var e struct {
			Code    *int    `json:"code"`
			Message *string `json:"message"`
		}
		if err := json.Unmarshal(rawErr, &e); err != nil {
			t.Fatalf("error member is not an object: %v (%q)", err, resp)
		}
		if e.Code == nil || e.Message == nil {
			t.Fatalf("error must have code and message: %q", resp)
		}
		d.hasErr = true
		d.code = *e.Code
		d.msg = *e.Message
	} else {
		d.result = rawRes
	}
	return d
}

func dispatch(t *testing.T, handlers map[string]ipc.Handler, msg string) ([]byte, bool) {
	t.Helper()
	return ipc.Dispatch(t.Context(), handlers, []byte(msg))
}

func TestDispatch(t *testing.T) {
	t.Parallel()

	const pong = `{"pong":true}`

	// Table cases use the agent handlers (agent.ping / agent.status, both without params).
	// wantCode == 0 means success with result wantResult.
	tests := []struct {
		name       string
		msg        string
		wantCode   int
		wantID     string
		wantResult string
	}{
		{"IPC-001/ping retorna exatamente {pong:true}",
			`{"jsonrpc":"2.0","id":1,"method":"agent.ping"}`, 0, `1`, pong},
		{"IPC-001/id string é ecoado na resposta",
			`{"jsonrpc":"2.0","id":"abc-1","method":"agent.ping"}`, 0, `"abc-1"`, pong},

		{"IPC-002/linha não-JSON retorna -32700 id null",
			`{"jsonrpc":"2.0","id":1,"method":`, ipc.CodeParseError, `null`, ""},
		{"IPC-002/texto solto retorna -32700 id null",
			`hello`, ipc.CodeParseError, `null`, ""},
		{"IPC-002/método inexistente retorna -32601",
			`{"jsonrpc":"2.0","id":2,"method":"agent.nope"}`, ipc.CodeMethodNotFound, `2`, ""},
		{"IPC-002/método inexistente com params não vazios retorna -32601 (método antes de params)",
			`{"jsonrpc":"2.0","id":3,"method":"agent.nope","params":[1]}`, ipc.CodeMethodNotFound, `3`, ""},
		{"IPC-002/params [1] retorna -32602",
			`{"jsonrpc":"2.0","id":4,"method":"agent.ping","params":[1]}`, ipc.CodeInvalidParams, `4`, ""},
		{"IPC-002/params {a:1} retorna -32602",
			`{"jsonrpc":"2.0","id":5,"method":"agent.ping","params":{"a":1}}`, ipc.CodeInvalidParams, `5`, ""},
		{"IPC-002/params não vazios em agent.status retornam -32602",
			`{"jsonrpc":"2.0","id":"s","method":"agent.status","params":[null]}`, ipc.CodeInvalidParams, `"s"`, ""},

		{"IPC-008/params ausente",
			`{"jsonrpc":"2.0","id":10,"method":"agent.ping"}`, 0, `10`, pong},
		{"IPC-008/params null",
			`{"jsonrpc":"2.0","id":11,"method":"agent.ping","params":null}`, 0, `11`, pong},
		{"IPC-008/params []",
			`{"jsonrpc":"2.0","id":12,"method":"agent.ping","params":[]}`, 0, `12`, pong},
		{"IPC-008/params {}",
			`{"jsonrpc":"2.0","id":13,"method":"agent.ping","params":{}}`, 0, `13`, pong},
		{"IPC-008/params [] com espaços",
			`{"jsonrpc":"2.0","id":14,"method":"agent.ping","params":[ ]}`, 0, `14`, pong},

		// IPC-009: the id is readable in all of these, so the -32600 response carries it (design step 3).
		{"IPC-009/params string",
			`{"jsonrpc":"2.0","id":20,"method":"agent.ping","params":"x"}`, ipc.CodeInvalidRequest, `20`, ""},
		{"IPC-009/params número",
			`{"jsonrpc":"2.0","id":21,"method":"agent.ping","params":1}`, ipc.CodeInvalidRequest, `21`, ""},
		{"IPC-009/params bool",
			`{"jsonrpc":"2.0","id":22,"method":"agent.ping","params":true}`, ipc.CodeInvalidRequest, `22`, ""},
		{"IPC-009/jsonrpc ausente",
			`{"id":23,"method":"agent.ping"}`, ipc.CodeInvalidRequest, `23`, ""},
		{"IPC-009/jsonrpc 1.0",
			`{"jsonrpc":"1.0","id":"v1","method":"agent.ping"}`, ipc.CodeInvalidRequest, `"v1"`, ""},
		{"IPC-009/jsonrpc numérico",
			`{"jsonrpc":2.0,"id":24,"method":"agent.ping"}`, ipc.CodeInvalidRequest, `24`, ""},
		{"IPC-009/method não string",
			`{"jsonrpc":"2.0","id":25,"method":7}`, ipc.CodeInvalidRequest, `25`, ""},
		{"IPC-009/params inválido em método inexistente retorna -32600 (requisição antes de método)",
			`{"jsonrpc":"2.0","id":26,"method":"agent.nope","params":"x"}`, ipc.CodeInvalidRequest, `26`, ""},
		{"IPC-009/JSON não-objeto retorna -32600 id null",
			`42`, ipc.CodeInvalidRequest, `null`, ""},
		{"IPC-009/string JSON retorna -32600 id null",
			`"agent.ping"`, ipc.CodeInvalidRequest, `null`, ""},
		{"IPC-009/requisição inválida sem id ainda responde -32600 id null (requisição antes de notificação)",
			`{"jsonrpc":"1.0","method":"agent.ping"}`, ipc.CodeInvalidRequest, `null`, ""},

		// IPC-009 (design step 3, 2026-11-08): an id that is not string/number/null → -32600 with id null.
		{"IPC-009/id objeto retorna -32600 id null",
			`{"jsonrpc":"2.0","id":{"a":1},"method":"agent.ping"}`, ipc.CodeInvalidRequest, `null`, ""},
		{"IPC-009/id array retorna -32600 id null",
			`{"jsonrpc":"2.0","id":[1],"method":"agent.ping"}`, ipc.CodeInvalidRequest, `null`, ""},
		{"IPC-009/id bool true retorna -32600 id null",
			`{"jsonrpc":"2.0","id":true,"method":"agent.ping"}`, ipc.CodeInvalidRequest, `null`, ""},
		{"IPC-009/id bool false retorna -32600 id null",
			`{"jsonrpc":"2.0","id":false,"method":"agent.ping"}`, ipc.CodeInvalidRequest, `null`, ""},
		{"IPC-009/id objeto em método inexistente retorna -32600 (requisição antes de método)",
			`{"jsonrpc":"2.0","id":{},"method":"agent.nope"}`, ipc.CodeInvalidRequest, `null`, ""},

		// IPC-011 (design step 4, 2026-11-08): explicit "id": null is a request, answered with id null.
		{"IPC-011/id null explícito é requisição respondida com id null",
			`{"jsonrpc":"2.0","id":null,"method":"agent.ping"}`, 0, `null`, pong},
		{"IPC-011/id null explícito com método inexistente recebe -32601 id null",
			`{"jsonrpc":"2.0","id":null,"method":"agent.nope"}`, ipc.CodeMethodNotFound, `null`, ""},
		{"IPC-011/id null explícito com params não vazios recebe -32602 id null",
			`{"jsonrpc":"2.0","id":null,"method":"agent.ping","params":[1]}`, ipc.CodeInvalidParams, `null`, ""},

		{"IPC-010/batch vazio retorna um -32600 id null",
			`[]`, ipc.CodeInvalidRequest, `null`, ""},
		{"IPC-010/batch com itens retorna um -32600 id null",
			`[{"jsonrpc":"2.0","id":1,"method":"agent.ping"},{"jsonrpc":"2.0","id":2,"method":"agent.status"}]`,
			ipc.CodeInvalidRequest, `null`, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp, ok := dispatch(t, testHandlers(), tc.msg)
			if !ok {
				t.Fatalf("Dispatch(%s) ok = false, want a response", tc.msg)
			}
			d := decodeResponse(t, resp)
			if d.id != tc.wantID {
				t.Errorf("id = %s, want %s", d.id, tc.wantID)
			}
			if tc.wantCode == 0 {
				if d.hasErr {
					t.Fatalf("got error code %d, want result %s (%q)", d.code, tc.wantResult, resp)
				}
				if got := compactJSON(t, d.result); got != tc.wantResult {
					t.Errorf("result = %s, want exactly %s", got, tc.wantResult)
				}
				return
			}
			if !d.hasErr {
				t.Fatalf("got result %s, want error %d", d.result, tc.wantCode)
			}
			if d.code != tc.wantCode {
				t.Errorf("error code = %d, want %d (%q)", d.code, tc.wantCode, resp)
			}
		})
	}

	t.Run("IPC-001/status calcula uptime pelo relógio", func(t *testing.T) {
		t.Parallel()
		info := testInfo()
		clock := domain.NewFakeClock(info.StartedAt)
		handlers := ipc.AgentHandlers(info, clock)
		clock.Advance(90*time.Second + 900*time.Millisecond)

		st := callStatus(t, handlers, `7`)
		if st.UptimeSeconds != 90 {
			t.Errorf("uptimeSeconds = %d, want 90 (floor of 90.9 s)", st.UptimeSeconds)
		}
		if st.PID != info.PID {
			t.Errorf("pid = %d, want %d", st.PID, info.PID)
		}
		if st.DataDir != info.DataDir {
			t.Errorf("dataDir = %q, want %q", st.DataDir, info.DataDir)
		}
		if st.Version != info.Version {
			t.Errorf("version = %q, want %q", st.Version, info.Version)
		}
		assertStartedAtUTC(t, st.StartedAt, info.StartedAt)

		// Uptime follows the clock on every call, not a value captured once.
		clock.Advance(10 * time.Second)
		if st2 := callStatus(t, handlers, `8`); st2.UptimeSeconds != 100 {
			t.Errorf("uptimeSeconds after +10 s = %d, want 100", st2.UptimeSeconds)
		}
	})

	t.Run("IPC-001/status sem avanço do relógio tem uptime 0", func(t *testing.T) {
		t.Parallel()
		info := testInfo()
		handlers := ipc.AgentHandlers(info, domain.NewFakeClock(info.StartedAt))
		if st := callStatus(t, handlers, `"z"`); st.UptimeSeconds != 0 {
			t.Errorf("uptimeSeconds = %d, want 0", st.UptimeSeconds)
		}
	})

	t.Run("AGT-004/status devolve AgentInfo.Version", func(t *testing.T) {
		t.Parallel()
		for _, version := range []string{"1.2.3", "dev"} {
			info := testInfo()
			info.Version = version
			handlers := ipc.AgentHandlers(info, domain.NewFakeClock(info.StartedAt))
			if st := callStatus(t, handlers, `1`); st.Version != version {
				t.Errorf("version = %q, want %q", st.Version, version)
			}
		}
	})

	t.Run("IPC-011/notificação para método existente sem resposta", func(t *testing.T) {
		t.Parallel()
		var calls atomic.Int32
		handlers := testHandlers()
		handlers["test.touch"] = func(ctx context.Context, params json.RawMessage) (any, error) {
			calls.Add(1)
			return map[string]bool{"ok": true}, nil
		}

		resp, ok := dispatch(t, handlers, `{"jsonrpc":"2.0","method":"agent.ping"}`)
		if ok {
			t.Fatalf("notification agent.ping: ok = true, resp = %q; want no response", resp)
		}
		resp, ok = dispatch(t, handlers, `{"jsonrpc":"2.0","method":"test.touch","params":{}}`)
		if ok {
			t.Fatalf("notification test.touch: ok = true, resp = %q; want no response", resp)
		}
		if got := calls.Load(); got != 1 {
			t.Errorf("handler calls = %d, want 1 (notifications are processed, design step 4)", got)
		}

		// The next request with id is answered normally.
		resp, ok = dispatch(t, handlers, `{"jsonrpc":"2.0","id":99,"method":"agent.ping"}`)
		if !ok {
			t.Fatal("request after notification: ok = false")
		}
		d := decodeResponse(t, resp)
		if d.hasErr || compactJSON(t, d.result) != pong || d.id != `99` {
			t.Errorf("request after notification = %q, want result %s id 99", resp, pong)
		}
	})

	t.Run("IPC-011/notificação para método inexistente sem resposta", func(t *testing.T) {
		t.Parallel()
		for _, msg := range []string{
			`{"jsonrpc":"2.0","method":"agent.nope"}`,
			`{"jsonrpc":"2.0","method":"agent.nope","params":[1,2]}`,
		} {
			if resp, ok := dispatch(t, testHandlers(), msg); ok {
				t.Errorf("Dispatch(%s) ok = true, resp = %q; want no response", msg, resp)
			}
		}
	})

	t.Run("IPC-011/notificação com params não vazios para método sem params sem resposta", func(t *testing.T) {
		t.Parallel()
		msg := `{"jsonrpc":"2.0","method":"agent.ping","params":[1]}`
		if resp, ok := dispatch(t, testHandlers(), msg); ok {
			t.Errorf("Dispatch(%s) ok = true, resp = %q; want no response", msg, resp)
		}
	})
}

// TestDispatchHandlerErrors covers the handler-error mapping defined in design.md
// (internal/ipc, "Erro de handler", 2026-11-08).
func TestDispatchHandlerErrors(t *testing.T) {
	t.Parallel()

	const secret = `open C:\secret\path\db.sqlite: access denied`

	tests := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
		leakText string // must not appear anywhere in the response; "" = no check
	}{
		{"IPC-002/erro comum do handler vira -32603 Internal error sem vazar o texto",
			errors.New(secret), ipc.CodeInternalError, "Internal error", secret},
		{"IPC-002/erro embrulhado com %w vira -32603 sem vazar o texto",
			fmt.Errorf("load library: %w", errors.New(secret)), ipc.CodeInternalError, "Internal error", secret},
		{"IPC-002/*Error do handler propaga código e mensagem",
			ipc.NewError(-32001, "game not found"), -32001, "game not found", ""},
		{"IPC-002/*Error com código padrão propaga a mensagem do handler",
			ipc.NewError(ipc.CodeInvalidParams, "gameId must be positive"), ipc.CodeInvalidParams, "gameId must be positive", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			handlers := testHandlers()
			handlers["test.fail"] = func(ctx context.Context, params json.RawMessage) (any, error) {
				calls.Add(1)
				return nil, tc.err
			}

			resp, ok := dispatch(t, handlers, `{"jsonrpc":"2.0","id":"e1","method":"test.fail"}`)
			if !ok {
				t.Fatal("Dispatch ok = false, want an error response")
			}
			if got := calls.Load(); got != 1 {
				t.Fatalf("handler calls = %d, want 1", got)
			}
			d := decodeResponse(t, resp)
			if !d.hasErr {
				t.Fatalf("got result %s, want error %d", d.result, tc.wantCode)
			}
			if d.id != `"e1"` {
				t.Errorf("id = %s, want \"e1\"", d.id)
			}
			if d.code != tc.wantCode {
				t.Errorf("error code = %d, want %d (%q)", d.code, tc.wantCode, resp)
			}
			if d.msg != tc.wantMsg {
				t.Errorf("error message = %q, want %q", d.msg, tc.wantMsg)
			}
			if tc.leakText != "" {
				// The original text must not leak, neither in message nor in data (also JSON-escaped).
				escaped, _ := json.Marshal(tc.leakText)
				inner := string(escaped[1 : len(escaped)-1])
				if bytes.Contains(resp, []byte(inner)) || bytes.Contains(resp, []byte("access denied")) {
					t.Errorf("response leaks the handler error text: %q", resp)
				}
			}
		})
	}

	t.Run("IPC-011/erro de handler em notificação não gera resposta", func(t *testing.T) {
		t.Parallel()
		handlers := testHandlers()
		handlers["test.fail"] = func(ctx context.Context, params json.RawMessage) (any, error) {
			return nil, errors.New(secret)
		}
		if resp, ok := dispatch(t, handlers, `{"jsonrpc":"2.0","method":"test.fail"}`); ok {
			t.Errorf("notification with failing handler: ok = true, resp = %q; want no response", resp)
		}
	})
}

// TestDispatchErrorMessages checks that dispatcher-generated error messages are exactly
// the texts of components.errors in the contract (design.md, "Textos das message").
func TestDispatchErrorMessages(t *testing.T) {
	t.Parallel()
	c := loadContract(t)

	tests := []struct {
		name         string
		contractName string
		wantCode     int
		msg          string
	}{
		{"IPC-002/-32700 usa a message de components.errors.ParseError",
			"ParseError", ipc.CodeParseError, `{"jsonrpc":`},
		{"IPC-002/-32600 usa a message de components.errors.InvalidRequest",
			"InvalidRequest", ipc.CodeInvalidRequest, `{"jsonrpc":"1.0","id":1,"method":"agent.ping"}`},
		{"IPC-002/-32600 de batch usa a message de components.errors.InvalidRequest",
			"InvalidRequest", ipc.CodeInvalidRequest, `[]`},
		{"IPC-002/-32601 usa a message de components.errors.MethodNotFound",
			"MethodNotFound", ipc.CodeMethodNotFound, `{"jsonrpc":"2.0","id":1,"method":"agent.nope"}`},
		{"IPC-002/-32602 usa a message de components.errors.InvalidParams",
			"InvalidParams", ipc.CodeInvalidParams, `{"jsonrpc":"2.0","id":1,"method":"agent.ping","params":[1]}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want, found := c.Components.Errors[tc.contractName]
			if !found {
				t.Fatalf("contract has no components.errors.%s", tc.contractName)
			}
			if want.Code != tc.wantCode {
				t.Fatalf("contract %s code = %d, want %d", tc.contractName, want.Code, tc.wantCode)
			}
			resp, ok := dispatch(t, testHandlers(), tc.msg)
			if !ok {
				t.Fatalf("Dispatch(%s) ok = false", tc.msg)
			}
			d := decodeResponse(t, resp)
			if !d.hasErr || d.code != tc.wantCode {
				t.Fatalf("response = %q, want error %d", resp, tc.wantCode)
			}
			if d.msg != want.Message {
				t.Errorf("error message = %q, want exactly %q (contract)", d.msg, want.Message)
			}
		})
	}

	t.Run("IPC-002/-32603 de handler usa a message de components.errors.InternalError", func(t *testing.T) {
		t.Parallel()
		want := c.Components.Errors["InternalError"]
		handlers := testHandlers()
		handlers["test.fail"] = func(ctx context.Context, params json.RawMessage) (any, error) {
			return nil, errors.New("boom")
		}
		resp, ok := dispatch(t, handlers, `{"jsonrpc":"2.0","id":1,"method":"test.fail"}`)
		if !ok {
			t.Fatal("Dispatch ok = false")
		}
		d := decodeResponse(t, resp)
		if !d.hasErr || d.code != want.Code || d.msg != want.Message {
			t.Errorf("response = %q, want error %d %q", resp, want.Code, want.Message)
		}
	})
}

// TestDispatchReviewGaps covers cases raised in the T06 review that weaker
// implementations could otherwise pass.
func TestDispatchReviewGaps(t *testing.T) {
	t.Parallel()

	t.Run("IPC-001/uptime nunca negativo com StartedAt no futuro", func(t *testing.T) {
		t.Parallel()
		info := testInfo()
		clock := domain.NewFakeClock(info.StartedAt)
		info.StartedAt = info.StartedAt.Add(10 * time.Second)
		handlers := ipc.AgentHandlers(info, clock)
		if st := callStatus(t, handlers, `1`); st.UptimeSeconds != 0 {
			t.Errorf("uptimeSeconds = %d, want 0 (never negative)", st.UptimeSeconds)
		}
	})

	t.Run("IPC-009/method ausente retorna -32600 com id ecoado (não -32601)", func(t *testing.T) {
		t.Parallel()
		resp, ok := dispatch(t, testHandlers(), `{"jsonrpc":"2.0","id":1}`)
		if !ok {
			t.Fatal("Dispatch ok = false, want a response")
		}
		d := decodeResponse(t, resp)
		if !d.hasErr || d.code != ipc.CodeInvalidRequest {
			t.Fatalf("response = %q, want error %d", resp, ipc.CodeInvalidRequest)
		}
		if d.id != `1` {
			t.Errorf("id = %s, want 1", d.id)
		}
	})

	for _, id := range []string{`-1`, `1.5`} {
		t.Run("IPC-002/id numérico "+id+" é ecoado exatamente", func(t *testing.T) {
			t.Parallel()
			resp, ok := dispatch(t, testHandlers(), `{"jsonrpc":"2.0","id":`+id+`,"method":"agent.ping"}`)
			if !ok {
				t.Fatal("Dispatch ok = false")
			}
			d := decodeResponse(t, resp)
			if d.hasErr {
				t.Fatalf("got error %d, want result (%q)", d.code, resp)
			}
			if d.id != id {
				t.Errorf("id = %s, want exactly %s", d.id, id)
			}
		})
		t.Run("IPC-001/id numérico "+id+" é ecoado exatamente em agent.status", func(t *testing.T) {
			t.Parallel()
			_ = callStatus(t, testHandlers(), id) // callStatus checks the echoed id
		})
	}

	t.Run("IPC-002/handler devolve *Error nil dentro de error vira -32603", func(t *testing.T) {
		t.Parallel()
		handlers := testHandlers()
		handlers["test.nilerr"] = func(ctx context.Context, params json.RawMessage) (any, error) {
			var e *ipc.Error
			return nil, e
		}
		resp, ok := dispatch(t, handlers, `{"jsonrpc":"2.0","id":"n1","method":"test.nilerr"}`)
		if !ok {
			t.Fatal("Dispatch ok = false, want an error response")
		}
		d := decodeResponse(t, resp) // fails if not exactly one of result/error
		if !d.hasErr || d.code != ipc.CodeInternalError || d.msg != "Internal error" {
			t.Fatalf("response = %q, want error -32603 \"Internal error\"", resp)
		}
		if d.id != `"n1"` {
			t.Errorf("id = %s, want \"n1\"", d.id)
		}
	})
}

// callStatus dispatches agent.status with the given raw id and decodes the result.
func callStatus(t *testing.T, handlers map[string]ipc.Handler, id string) statusWire {
	t.Helper()
	resp, ok := dispatch(t, handlers, `{"jsonrpc":"2.0","id":`+id+`,"method":"agent.status"}`)
	if !ok {
		t.Fatal("agent.status: ok = false")
	}
	d := decodeResponse(t, resp)
	if d.hasErr {
		t.Fatalf("agent.status returned error %d (%q)", d.code, resp)
	}
	if d.id != compactJSON(t, []byte(id)) {
		t.Errorf("agent.status id = %s, want %s", d.id, id)
	}
	var st statusWire
	dec := json.NewDecoder(bytes.NewReader(d.result))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&st); err != nil {
		t.Fatalf("decode status result %s: %v", d.result, err)
	}
	return st
}

// statusWire mirrors the contract's AgentStatus, independently of the production type.
type statusWire struct {
	Version       string `json:"version"`
	PID           int    `json:"pid"`
	StartedAt     string `json:"startedAt"`
	DataDir       string `json:"dataDir"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
}

func assertStartedAtUTC(t *testing.T, got string, want time.Time) {
	t.Helper()
	if len(got) == 0 || got[len(got)-1] != 'Z' {
		t.Errorf("startedAt = %q, want RFC 3339 in UTC (suffix Z)", got)
	}
	parsed, err := time.Parse(time.RFC3339Nano, got)
	if err != nil {
		t.Fatalf("startedAt %q does not parse as RFC 3339: %v", got, err)
	}
	if !parsed.Equal(want) {
		t.Errorf("startedAt = %s, want instant %s", parsed, want.UTC())
	}
}
