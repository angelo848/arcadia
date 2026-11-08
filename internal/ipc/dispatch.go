package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
)

// Handler handles one method call. params is nil when the request carried no
// params or empty ones (absent, null, [] or {}). Returning an *Error sends its
// code and message to the client; any other error becomes -32603.
type Handler func(ctx context.Context, params json.RawMessage) (any, error)

// nullID is the id used when the request id is absent or unreadable.
var nullID = json.RawMessage("null")

// Dispatch validates msg in design order (parse -> batch -> request ->
// notification -> method -> params), calls the matching handler and returns the
// encoded response without a trailing '\n'. ok=false means no response is due
// (notification).
func Dispatch(ctx context.Context, handlers map[string]Handler, msg []byte) (resp []byte, ok bool) {
	// 1. Parse.
	if !json.Valid(msg) {
		return errorResponse(nullID, CodeParseError, msgParseError), true
	}
	trimmed := bytes.TrimLeft(msg, " \t\r\n")

	// 2. Batch.
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return errorResponse(nullID, CodeInvalidRequest, msgInvalidRequest), true
	}

	// 3. Request shape.
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errorResponse(nullID, CodeInvalidRequest, msgInvalidRequest), true
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(msg, &members); err != nil {
		return errorResponse(nullID, CodeInvalidRequest, msgInvalidRequest), true
	}
	rawID, hasID := members["id"]
	respID := nullID
	if hasID && validID(rawID) {
		respID = rawID
	}
	method, valid := validRequest(members)
	if !valid || (hasID && !validID(rawID)) {
		return errorResponse(respID, CodeInvalidRequest, msgInvalidRequest), true
	}
	params := normalizeParams(members["params"])

	// 4. Notification: process (or ignore unknown method), never respond.
	if !hasID {
		if h, found := handlers[method]; found {
			_, _ = h(ctx, params)
		}
		return nil, false
	}

	// 5. Method.
	h, found := handlers[method]
	if !found {
		return errorResponse(respID, CodeMethodNotFound, msgMethodNotFound), true
	}

	// 6. Params are validated by the handler; empty ones were normalized to nil.
	result, err := h(ctx, params)
	if err != nil {
		var rpcErr *Error
		if errors.As(err, &rpcErr) && rpcErr != nil {
			return encodeResponse(Response{JSONRPC: Version, ID: respID, Error: rpcErr}), true
		}
		return errorResponse(respID, CodeInternalError, msgInternalError), true
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return errorResponse(respID, CodeInternalError, msgInternalError), true
	}
	return encodeResponse(Response{JSONRPC: Version, ID: respID, Result: raw}), true
}

// validRequest checks jsonrpc, method and params (design step 3) and returns the method.
func validRequest(members map[string]json.RawMessage) (string, bool) {
	var version string
	rawVersion, ok := members["jsonrpc"]
	if !ok || !isJSONString(rawVersion) || json.Unmarshal(rawVersion, &version) != nil || version != Version {
		return "", false
	}
	var method string
	rawMethod, ok := members["method"]
	if !ok || !isJSONString(rawMethod) || json.Unmarshal(rawMethod, &method) != nil {
		return "", false
	}
	if rawParams, ok := members["params"]; ok {
		switch firstByte(rawParams) {
		case 'n', '[', '{':
		default:
			return "", false
		}
	}
	return method, true
}

// validID reports whether raw is a string, number or null, as JSON-RPC 2.0 requires.
func validID(raw json.RawMessage) bool {
	switch c := firstByte(raw); {
	case c == '"', c == 'n', c == '-', c >= '0' && c <= '9':
		return true
	default:
		return false
	}
}

func isJSONString(raw json.RawMessage) bool { return firstByte(raw) == '"' }

func firstByte(raw json.RawMessage) byte {
	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	if len(trimmed) == 0 {
		return 0
	}
	return trimmed[0]
}

// normalizeParams maps absent, null, [] and {} to nil; other values pass through.
func normalizeParams(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	switch firstByte(raw) {
	case 'n':
		return nil
	case '[':
		var arr []json.RawMessage
		if json.Unmarshal(raw, &arr) == nil && len(arr) == 0 {
			return nil
		}
	case '{':
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) == nil && len(obj) == 0 {
			return nil
		}
	}
	return raw
}

func errorResponse(id json.RawMessage, code int, message string) []byte {
	return encodeResponse(Response{JSONRPC: Version, ID: id, Error: NewError(code, message)})
}

// encodeResponse marshals resp. A failure (e.g. an *Error with invalid Data)
// falls back to a fixed internal error that keeps resp.ID when it is valid.
func encodeResponse(resp Response) []byte {
	data, err := json.Marshal(resp)
	if err == nil {
		return data
	}
	id := []byte(nullID)
	var compact bytes.Buffer
	if len(resp.ID) > 0 && json.Compact(&compact, resp.ID) == nil && validID(compact.Bytes()) {
		id = compact.Bytes()
	}
	out := make([]byte, 0, 96)
	out = append(out, `{"jsonrpc":"2.0","id":`...)
	out = append(out, id...)
	out = append(out, `,"error":{"code":-32603,"message":"Internal error"}}`...)
	return out
}
