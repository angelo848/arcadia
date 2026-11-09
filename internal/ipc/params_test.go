package ipc_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// assertInvalidParams checks e is exactly -32602 "Invalid params".
func assertInvalidParams(t *testing.T, what string, e *ipc.Error) {
	t.Helper()
	if e == nil {
		t.Fatalf("%s: DecodeParams = nil, want -32602", what)
	}
	if e.Code != ipc.CodeInvalidParams || e.Message != ipc.MsgInvalidParams {
		t.Errorf("%s: error = {%d %q}, want {%d %q}", what, e.Code, e.Message, ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
}

func TestDecodeParams(t *testing.T) {
	t.Parallel()

	t.Run("IPC-028/objeto por nome é decodificado", func(t *testing.T) {
		t.Parallel()
		var p ipc.GameAddParams
		if e := ipc.DecodeParams(json.RawMessage(`{"path":"C:\\g\\a.exe","title":"A"}`), &p); e != nil {
			t.Fatalf("DecodeParams = %v, want nil", e)
		}
		if p.Path != `C:\g\a.exe` || p.Title != "A" {
			t.Errorf("decoded = %+v", p)
		}
		var l ipc.GameLaunchParams
		if e := ipc.DecodeParams(json.RawMessage(`{"gameId":5}`), &l); e != nil {
			t.Fatalf("DecodeParams gameId = %v, want nil", e)
		}
		if l.GameID != 5 {
			t.Errorf("GameID = %d, want 5", l.GameID)
		}
	})

	t.Run("IPC-028/array posicional é -32602", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{`["C:\\g\\a.exe"]`, `[5]`} {
			var p ipc.GameAddParams
			assertInvalidParams(t, raw, ipc.DecodeParams(json.RawMessage(raw), &p))
		}
	})

	t.Run("IPC-028/params que não são objeto são -32602", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{`"C:\\g\\a.exe"`, `5`, `true`} {
			var p ipc.GameAddParams
			assertInvalidParams(t, raw, ipc.DecodeParams(json.RawMessage(raw), &p))
		}
	})

	t.Run("IPC-028/membro desconhecido é -32602", func(t *testing.T) {
		t.Parallel()
		var p ipc.GameAddParams
		assertInvalidParams(t, "path+extra", ipc.DecodeParams(json.RawMessage(`{"path":"C:\\a.exe","extra":1}`), &p))
		var l ipc.GameLaunchParams
		assertInvalidParams(t, "gameId+id", ipc.DecodeParams(json.RawMessage(`{"gameId":1,"id":1}`), &l))
	})

	t.Run("IPC-028/membro com caixa diferente é -32602", func(t *testing.T) {
		t.Parallel()
		var p ipc.GameAddParams
		assertInvalidParams(t, "Path", ipc.DecodeParams(json.RawMessage(`{"Path":"C:\\a.exe"}`), &p))
		var l ipc.GameLaunchParams
		assertInvalidParams(t, "GAMEID", ipc.DecodeParams(json.RawMessage(`{"GAMEID":5}`), &l))
	})

	t.Run("IPC-028/membro duplicado é -32602", func(t *testing.T) {
		t.Parallel()
		var p ipc.GameAddParams
		assertInvalidParams(t, "path+path", ipc.DecodeParams(json.RawMessage(`{"path":"a","path":"b"}`), &p))
		var l ipc.GameLaunchParams
		assertInvalidParams(t, "gameId+gameId", ipc.DecodeParams(json.RawMessage(`{"gameId":1,"gameId":2}`), &l))
	})

	t.Run("IPC-028/tipo errado é -32602", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{`{"path":5}`, `{"path":null,"title":1}`, `{"path":["a"]}`} {
			var p ipc.GameAddParams
			assertInvalidParams(t, raw, ipc.DecodeParams(json.RawMessage(raw), &p))
		}
		for _, raw := range []string{`{"gameId":"5"}`, `{"gameId":5.5}`, `{"gameId":true}`} {
			var l ipc.GameLaunchParams
			assertInvalidParams(t, raw, ipc.DecodeParams(json.RawMessage(raw), &l))
		}
	})

	t.Run("IPC-028/nil não é erro", func(t *testing.T) {
		t.Parallel()
		p := ipc.GameAddParams{Path: "keep", Title: "me"}
		if e := ipc.DecodeParams(nil, &p); e != nil {
			t.Fatalf("DecodeParams(nil) = %v, want nil", e)
		}
		if p.Path != "keep" || p.Title != "me" {
			t.Errorf("DecodeParams(nil) touched v: %+v", p)
		}
	})

	t.Run("IPC-028/dados depois do objeto são -32602", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{`{"path":"a"} {}`, `{"path":"a"}x`, `{"path":"a"}{"path":"b"}`} {
			var p ipc.GameAddParams
			assertInvalidParams(t, raw, ipc.DecodeParams(json.RawMessage(raw), &p))
		}
	})
}

func TestNoParams(t *testing.T) {
	t.Parallel()

	t.Run("IPC-008/params nil chamam o handler", func(t *testing.T) {
		t.Parallel()
		called := false
		h := ipc.NoParams(func(context.Context) (any, error) {
			called = true
			return ipc.PingResult{Pong: true}, nil
		})
		res, err := h(t.Context(), nil)
		if err != nil || !called {
			t.Fatalf("NoParams(nil) = (%v, %v), called %v", res, err, called)
		}
		if r, ok := res.(ipc.PingResult); !ok || !r.Pong {
			t.Errorf("result = %#v, want PingResult{Pong:true}", res)
		}
	})

	t.Run("IPC-002/params não vazios dão -32602 sem chamar o handler", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{`[1]`, `{"x":1}`} {
			called := false
			h := ipc.NoParams(func(context.Context) (any, error) {
				called = true
				return nil, nil
			})
			_, err := h(t.Context(), json.RawMessage(raw))
			var e *ipc.Error
			if !errors.As(err, &e) {
				t.Fatalf("%s: err = %v, want *ipc.Error", raw, err)
			}
			if e.Code != ipc.CodeInvalidParams || e.Message != ipc.MsgInvalidParams {
				t.Errorf("%s: error = {%d %q}, want -32602 \"Invalid params\"", raw, e.Code, e.Message)
			}
			if called {
				t.Errorf("%s: handler was called", raw)
			}
		}
	})
}

func TestNewErrorData(t *testing.T) {
	t.Parallel()

	wire := func(t *testing.T, e *ipc.Error) string {
		t.Helper()
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("marshal Error: %v", err)
		}
		return compactJSON(t, raw)
	}

	t.Run("IPC-021/InvalidExecutableData serializa reason em data", func(t *testing.T) {
		t.Parallel()
		for _, reason := range []string{ipc.ReasonNotAbsolute, ipc.ReasonNotFound, ipc.ReasonNotAFile, ipc.ReasonNotExe} {
			e := ipc.NewErrorData(ipc.CodeInvalidExecutable, ipc.MsgInvalidExecutable, ipc.InvalidExecutableData{Reason: reason})
			want := `{"code":-32004,"message":"Invalid executable","data":{"reason":"` + reason + `"}}`
			if got := wire(t, e); got != want {
				t.Errorf("error JSON = %s, want %s", got, want)
			}
		}
	})

	t.Run("IPC-025/LaunchFailedData sem winError omite o campo", func(t *testing.T) {
		t.Parallel()
		e := ipc.NewErrorData(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed, ipc.LaunchFailedData{Error: "agente encerrando"})
		want := `{"code":-32003,"message":"Launch failed","data":{"error":"agente encerrando"}}`
		if got := wire(t, e); got != want {
			t.Errorf("error JSON = %s, want %s", got, want)
		}
	})

	t.Run("IPC-025/LaunchFailedData com winError inclui o inteiro", func(t *testing.T) {
		t.Parallel()
		code := uint32(2)
		e := ipc.NewErrorData(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed, ipc.LaunchFailedData{Error: "x", WinError: &code})
		want := `{"code":-32003,"message":"Launch failed","data":{"error":"x","winError":2}}`
		if got := wire(t, e); got != want {
			t.Errorf("error JSON = %s, want %s", got, want)
		}
	})

	t.Run("IPC-025/GameNotFound e SessionActive levam data", func(t *testing.T) {
		t.Parallel()
		e := ipc.NewErrorData(ipc.CodeGameNotFound, ipc.MsgGameNotFound, ipc.GameNotFoundData{GameID: 42})
		if got, want := wire(t, e), `{"code":-32001,"message":"Game not found","data":{"gameId":42}}`; got != want {
			t.Errorf("error JSON = %s, want %s", got, want)
		}
		e = ipc.NewErrorData(ipc.CodeSessionActive, ipc.MsgSessionActive, ipc.SessionRef{SessionID: 0, GameID: 3, Title: "HK"})
		if got, want := wire(t, e), `{"code":-32002,"message":"Session already active","data":{"sessionId":0,"gameId":3,"title":"HK"}}`; got != want {
			t.Errorf("error JSON = %s, want %s", got, want)
		}
	})

	t.Run("IPC-025/Error com data passa pelo Dispatch", func(t *testing.T) {
		t.Parallel()
		handlers := map[string]ipc.Handler{
			ipc.MethodGameLaunch: func(context.Context, json.RawMessage) (any, error) {
				return nil, ipc.NewErrorData(ipc.CodeGameNotFound, ipc.MsgGameNotFound, ipc.GameNotFoundData{GameID: 9})
			},
		}
		resp, ok := dispatch(t, handlers, `{"jsonrpc":"2.0","id":1,"method":"game.launch","params":{"gameId":9}}`)
		if !ok {
			t.Fatal("dispatch ok = false")
		}
		d := decodeResponse(t, resp)
		if !d.hasErr || d.code != ipc.CodeGameNotFound || d.msg != ipc.MsgGameNotFound {
			t.Fatalf("response = %s, want -32001 Game not found", resp)
		}
		var e struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(d.keys["error"], &e); err != nil {
			t.Fatalf("error member: %v", err)
		}
		if got := compactJSON(t, e.Data); got != `{"gameId":9}` {
			t.Errorf("error.data = %s, want {\"gameId\":9}", got)
		}
	})
}
