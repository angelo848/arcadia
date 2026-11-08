// Package ipc implements the JSON-RPC 2.0 protocol (NDJSON framing) spoken between
// arcadia-agent and the desktop UI. The wire contract lives in
// docs/specs/phase-2-skeleton/contracts/ipc.openrpc.json.
package ipc

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	// Version is the only accepted value of the "jsonrpc" member.
	Version = "2.0"
	// MaxMessageSize is the maximum size of one message, excluding the trailing '\n'.
	MaxMessageSize = 1 << 20
	// HeartbeatInterval is the period of the agent.heartbeat notification.
	HeartbeatInterval = 5 * time.Second
	// NotificationBuffer is the capacity of the client notification channel.
	NotificationBuffer = 16
)

// Method names.
const (
	MethodPing      = "agent.ping"
	MethodStatus    = "agent.status"
	MethodHeartbeat = "agent.heartbeat"
)

// JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Standard error messages, exactly as in the contract's components.errors.
const (
	msgParseError     = "Parse error"
	msgInvalidRequest = "Invalid Request"
	msgMethodNotFound = "Method not found"
	msgInvalidParams  = "Invalid params"
	msgInternalError  = "Internal error"
)

// Request is a JSON-RPC 2.0 request or notification (no ID).
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response. Exactly one of Result and Error is set.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC 2.0 error object. Handlers may return it to control the
// code and message sent to the client.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// NewError returns an *Error with the given code and message.
func NewError(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Notification is a server-to-client JSON-RPC notification as seen by the client.
type Notification struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// PingResult is the result of agent.ping.
type PingResult struct {
	Pong bool `json:"pong"`
}

// AgentStatus is the result of agent.status.
type AgentStatus struct {
	Version       string    `json:"version"`
	PID           int       `json:"pid"`
	StartedAt     time.Time `json:"startedAt"` // RFC 3339, UTC
	DataDir       string    `json:"dataDir"`
	UptimeSeconds int64     `json:"uptimeSeconds"`
}

// HeartbeatParams are the params of the agent.heartbeat notification.
type HeartbeatParams struct {
	UptimeSeconds int64 `json:"uptimeSeconds"`
}
