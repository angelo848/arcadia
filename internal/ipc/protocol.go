// Package ipc implements the JSON-RPC 2.0 protocol (NDJSON framing) spoken between
// arcadia-agent and the desktop UI. The wire contract lives in
// docs/specs/phase-15-big-box-v2/contracts/ipc.openrpc.json (v0.16.0, which supersedes
// the phase-13-modo-console v0.15.0, the phase-12-metadados-unificados v0.14.0, the phase-11-importacao-v2 v0.13.0, the phase-10-2-imagens-ao-vivo v0.12.0, the phase-9-plataformas v0.11.0, the phase-8-estatisticas v0.10.0, the phase-7-midia v0.9.0, the phase-6b-emulators v0.8.0 contract, the phase-6-importers v0.7.0, the phase-5-library v0.6.0 contract,
// the phase-4-pause v0.5.0, phase-3 v0.4.0, single-exe v0.3.0 and phase-2 v0.2.0
// contracts kept in their specs as history).
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
	// MethodShutdown is the server-to-client notification sent by Server.Drain
	// when the agent is about to exit.
	MethodShutdown = "agent.shutdown"

	MethodGameAdd        = "game.add"
	MethodGameList       = "game.list"
	MethodGameLaunch     = "game.launch"
	MethodSessionCurrent = "session.current"

	MethodGameLaunchUntracked = "game.launchUntracked"
	MethodSessionPause        = "session.pause"
	MethodSessionResume       = "session.resume"
	// MethodSessionChanged is the server-to-client notification of a session
	// state change.
	MethodSessionChanged = "session.changed"

	// Methods added in v0.6.0 [IPC-041].
	MethodGameGet        = "game.get"
	MethodGameUpdate     = "game.update"
	MethodGameDelete     = "game.delete"
	MethodSessionList    = "session.list"
	MethodPlatformList   = "platform.list"
	MethodPlatformAdd    = "platform.add"
	MethodLibraryCatalog = "library.catalog"
	// Phase 6 (contract v0.7.0): import.scan reads, import.apply writes [IMP-001].
	MethodImportScan  = "import.scan"
	MethodImportApply = "import.apply"

	// MethodMediaChanged is the server-to-client notification that the media
	// list of a game changed (v0.12.0) [IMG-001].
	MethodMediaChanged = "media.changed"

	// Unified metadata methods, added in v0.14.0 [MET-070]; after scrape.apply in the contract.
	MethodMetadataSearch       = "metadata.search"
	MethodMetadataApply        = "metadata.apply"
	MethodMetadataBatchStart   = "metadata.batchStart"
	MethodMetadataBatchStatus  = "metadata.batchStatus"
	MethodMetadataBatchResults = "metadata.batchResults"
	MethodMetadataBatchApply   = "metadata.batchApply"
	MethodMetadataBatchCancel  = "metadata.batchCancel"
	MethodMetadataBatchDiscard = "metadata.batchDiscard"
	// MethodMetadataBatchChanged is the server-to-client notification of a metadata
	// batch state or counter change (v0.14.0) [MET-054].
	MethodMetadataBatchChanged = "metadata.batchChanged"
)

// MediaChangedParams are the params of the media.changed notification
// [IMG-001, IMG-004].
type MediaChangedParams struct {
	GameID int64 `json:"gameId"`
}

// JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Application error codes.
const (
	CodeGameNotFound       = -32001
	CodeSessionActive      = -32002
	CodeLaunchFailed       = -32003
	CodeInvalidExecutable  = -32004
	CodeSessionNotPausable = -32005

	// Metadata and media codes (phase 7, contract v0.9.0).
	CodeProviderNoKey       = -32010
	CodeProviderUnavailable = -32011
	CodeMediaJobNotFound    = -32012

	// Unified metadata batch codes (contract v0.14.0 [MET-070]).
	CodeBatchNotFound   = -32014
	CodeBatchBusy       = -32015
	CodeBatchWrongState = -32016
)

// Application error messages, exactly as in the contract's components.errors.
const (
	MsgGameNotFound        = "Game not found"
	MsgSessionActive       = "Session already active"
	MsgLaunchFailed        = "Launch failed"
	MsgInvalidExecutable   = "Invalid executable"
	MsgSessionNotPausable  = "Session not pausable"
	MsgProviderNoKey       = "Provider key not registered"
	MsgProviderUnavailable = "Provider unavailable"
	MsgMediaJobNotFound    = "Media job not found"
	MsgBatchNotFound       = "Metadata batch not found"
	MsgBatchBusy           = "Metadata batch in progress"
	MsgBatchWrongState     = "Wrong batch state"
	// MsgInternalError is the message of CodeInternalError, for handlers outside this package.
	MsgInternalError = msgInternalError
	// MsgInvalidParams is the exported alias of msgInvalidParams.
	MsgInvalidParams = msgInvalidParams
)

// Reasons carried by InvalidExecutableData.
const (
	ReasonNotAbsolute = "not-absolute"
	ReasonNotFound    = "not-found"
	ReasonNotAFile    = "not-a-file"
	ReasonNotExe      = "not-exe"
)

// Reasons carried by SessionNotPausableData.
const (
	ReasonNoSession = "no-session"
	ReasonGrace     = "grace"
)

// MaxGameTitleRunes is the maximum length of a game title (x-limits.maxGameTitleRunes).
const MaxGameTitleRunes = 200

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

// NewErrorData returns an *Error with the given code and message whose data
// member is the JSON encoding of data. data must be JSON-serializable: if it
// cannot be encoded, the data member is silently omitted and the error is
// returned without data.
func NewErrorData(code int, message string, data any) *Error {
	e := NewError(code, message)
	if raw, err := json.Marshal(data); err == nil {
		e.Data = raw
	}
	return e
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

// ShutdownParams are the params of the agent.shutdown notification. Reason is
// one of "quit", "session-end", "signal" or "other".
type ShutdownParams struct {
	Reason string `json:"reason"`
}

// Game is a library entry with its play totals: the item of game.list and the
// result of game.add [IPC-042, IPC-020]. Year is null when there is no release
// date. FirstPlayedAt and LastPlayedAt are null until the first counted session.
type Game struct {
	ID               int64      `json:"id"`
	Title            string     `json:"title"`
	PlatformID       int64      `json:"platformId"`
	Platform         string     `json:"platform"`
	Path             string     `json:"path"`
	Status           string     `json:"status"`
	Favorite         bool       `json:"favorite"`
	Year             *int       `json:"year"`
	TotalPlaySeconds int64      `json:"totalPlaySeconds"`
	PlayCount        int64      `json:"playCount"`
	FirstPlayedAt    *time.Time `json:"firstPlayedAt"`
	LastPlayedAt     *time.Time `json:"lastPlayedAt"`
}

// Session is the wire view of a play session (see SessionFromSnapshot).
type Session struct {
	SessionID     int64      `json:"sessionId"`
	GameID        int64      `json:"gameId"`
	Title         string     `json:"title"`
	State         string     `json:"state"`
	StartedAt     time.Time  `json:"startedAt"`
	EndedAt       *time.Time `json:"endedAt"`
	ActiveSeconds int64      `json:"activeSeconds"`
	ActiveMs      int64      `json:"activeMs"`
	AsOf          MilliTime  `json:"asOf"`
	// Tracking is "job" or "process" (RUL-005); "process" sessions have no Job Object.
	Tracking string `json:"tracking"`
}

// GameAddParams are the params of game.add (client-side encoding).
type GameAddParams struct {
	Path  string `json:"path"`
	Title string `json:"title,omitempty"`
}

// GameLaunchParams are the params of game.launch (client-side encoding).
type GameLaunchParams struct {
	GameID int64 `json:"gameId"`
}

// GameLaunchUntrackedParams are the params of game.launchUntracked (client-side encoding).
type GameLaunchUntrackedParams struct {
	GameID int64 `json:"gameId"`
}

// LaunchedProcess is the result of game.launchUntracked.
type LaunchedProcess struct {
	PID uint32 `json:"pid"`
}

// SessionResult is the result of session.pause and session.resume.
type SessionResult struct {
	Session Session `json:"session"`
}

// SessionNotPausableData is the data of SessionNotPausable; Reason is one of
// ReasonNoSession and ReasonGrace.
type SessionNotPausableData struct {
	Reason string `json:"reason"`
}

// SessionCurrentResult is the result of session.current; Session is null when
// no session is active.
type SessionCurrentResult struct {
	Session *Session `json:"session"`
}

// SessionChangedParams are the params of the session.changed notification.
type SessionChangedParams struct {
	Session Session `json:"session"`
}

// SessionRef identifies a session (data of SessionActive).
type SessionRef struct {
	SessionID int64  `json:"sessionId"`
	GameID    int64  `json:"gameId"`
	Title     string `json:"title"`
}

// LaunchFailedData is the data of LaunchFailed. WinError is the Win32 error
// code, when there is one.
type LaunchFailedData struct {
	Error    string  `json:"error"`
	WinError *uint32 `json:"winError,omitempty"`
}

// InvalidExecutableData is the data of InvalidExecutable; Reason is one of the
// Reason* constants.
type InvalidExecutableData struct {
	Reason string `json:"reason"`
}

// GameNotFoundData is the data of GameNotFound.
type GameNotFoundData struct {
	GameID int64 `json:"gameId"`
}
