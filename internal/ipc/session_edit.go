package ipc

import "time"

// Session edit methods (phase 16, contract v0.17.0 [SED-030..SED-032]). None of them sends a notification.
const (
	MethodSessionCreate = "session.create"
	MethodSessionUpdate = "session.update"
	MethodSessionDelete = "session.delete"
)

// Error codes and messages added by v0.17.0.
const (
	CodeSessionNotFound = -32017
	CodeSessionOverlap  = -32018

	MsgSessionNotFound = "Session not found"
	MsgSessionOverlap  = "Session overlaps another"
)

// SessionCreateParams are the params of session.create. Instants are RFC 3339; a malformed one fails decoding.
type SessionCreateParams struct {
	GameID          int64     `json:"gameId"`
	StartedAt       time.Time `json:"startedAt"`
	EndedAt         time.Time `json:"endedAt"`
	ActiveSeconds   int64     `json:"activeSeconds"`
	IncludedInTotal bool      `json:"includedInTotal,omitempty"`
}

// SessionUpdateParams are the params of session.update (server side only): an absent member keeps the
// value and null is invalid.
type SessionUpdateParams struct {
	SessionID     int64               `json:"sessionId"`
	StartedAt     Optional[time.Time] `json:"startedAt"`
	EndedAt       Optional[time.Time] `json:"endedAt"`
	ActiveSeconds Optional[int64]     `json:"activeSeconds"`
}

// SessionDeleteParams are the params of session.delete.
type SessionDeleteParams struct {
	SessionID int64 `json:"sessionId"`
}

// SessionWriteResult is the result of session.create and session.update.
type SessionWriteResult struct {
	Session SessionItem `json:"session"`
	Game    GameDetail  `json:"game"`
}

// SessionDeleteResult is the result of session.delete.
type SessionDeleteResult struct {
	SessionID int64      `json:"sessionId"`
	Game      GameDetail `json:"game"`
}

// SessionOverlapData is the data of SessionOverlap. EndedAt is null when the other session is open.
type SessionOverlapData struct {
	SessionID int64      `json:"sessionId"`
	GameID    int64      `json:"gameId"`
	Title     string     `json:"title"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt"`
}

// InvalidParamsReasonData is the data of an InvalidParams caused by a rule of the session edit.
type InvalidParamsReasonData struct {
	Reason string `json:"reason"`
}

// SessionNotFoundData is the data of SessionNotFound.
type SessionNotFoundData struct {
	SessionID int64 `json:"sessionId"`
}
