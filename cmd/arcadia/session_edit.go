package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// displayLocation is the zone of the instants shown in error texts (SED-036).
var displayLocation = time.Local

// SessionCreateInput is the form of a manual session; instants are RFC 3339 (SED-035).
type SessionCreateInput struct {
	StartedAt       string `json:"startedAt"`
	EndedAt         string `json:"endedAt"`
	ActiveSeconds   int64  `json:"activeSeconds"`
	IncludedInTotal bool   `json:"includedInTotal"`
}

// SessionWriteView is the result of CreateSession and UpdateSession.
type SessionWriteView struct {
	Session SessionItemView `json:"session"`
	Game    GameDetailView  `json:"game"`
}

// CreateSession calls session.create (5 s timeout).
func (a *App) CreateSession(gameID int64, in SessionCreateInput) (SessionWriteView, error) {
	started, err := time.Parse(time.RFC3339, in.StartedAt)
	if err != nil {
		return SessionWriteView{}, errors.New(msgInvalidData)
	}
	ended, err := time.Parse(time.RFC3339, in.EndedAt)
	if err != nil {
		return SessionWriteView{}, errors.New(msgInvalidData)
	}
	params := ipc.SessionCreateParams{
		GameID: gameID, StartedAt: started, EndedAt: ended,
		ActiveSeconds: in.ActiveSeconds, IncludedInTotal: in.IncludedInTotal,
	}
	var res ipc.SessionWriteResult
	if err := a.callAgentRaw(ipc.MethodSessionCreate, libraryTimeout, params, &res); err != nil {
		return SessionWriteView{}, sessionEditError(err, a.logger(), displayLocation)
	}
	return SessionWriteView{Session: sessionItemView(res.Session), Game: gameDetailView(res.Game)}, nil
}

// UpdateSession calls session.update (5 s timeout) with the raw patch members plus sessionId.
func (a *App) UpdateSession(sessionID int64, patch json.RawMessage) (SessionWriteView, error) {
	params, err := patchParams(sessionID, patch)
	if err != nil {
		return SessionWriteView{}, errors.New(msgInvalidData)
	}
	// patchParams adds gameId; this method wants sessionId.
	delete(params, "gameId")
	id, err := json.Marshal(sessionID)
	if err != nil {
		return SessionWriteView{}, fmt.Errorf("sessionId: %w", err)
	}
	params["sessionId"] = id
	var res ipc.SessionWriteResult
	if err := a.callAgentRaw(ipc.MethodSessionUpdate, libraryTimeout, params, &res); err != nil {
		return SessionWriteView{}, sessionEditError(err, a.logger(), displayLocation)
	}
	return SessionWriteView{Session: sessionItemView(res.Session), Game: gameDetailView(res.Game)}, nil
}

// DeleteSession calls session.delete (5 s timeout).
func (a *App) DeleteSession(sessionID int64) (GameDetailView, error) {
	var res ipc.SessionDeleteResult
	err := a.callAgentRaw(ipc.MethodSessionDelete, libraryTimeout, ipc.SessionDeleteParams{SessionID: sessionID}, &res)
	if err != nil {
		return GameDetailView{}, sessionEditError(err, a.logger(), displayLocation)
	}
	return gameDetailView(res.Game), nil
}

// sessionEditError turns the errors of the three methods into the SED-036 texts; anything else goes to userError.
func sessionEditError(err error, log *slog.Logger, loc *time.Location) error {
	var rpcErr *ipc.Error
	if !isConnectionError(err) && errors.As(err, &rpcErr) {
		switch rpcErr.Code {
		case ipc.CodeSessionActive:
			return errors.New(msgSessionEditActive)
		case ipc.CodeSessionNotFound:
			return errors.New(msgSessionGone)
		case ipc.CodeSessionOverlap:
			var d ipc.SessionOverlapData
			if decodeData(rpcErr.Data, &d) {
				if loc == nil {
					loc = time.Local
				}
				end := "agora"
				if d.EndedAt != nil {
					end = d.EndedAt.In(loc).Format(sessionTimeLayout)
				}
				return fmt.Errorf(msgSessionOverlapFmt, d.Title, d.StartedAt.In(loc).Format(sessionTimeLayout), end)
			}
		case ipc.CodeInvalidParams:
			var d ipc.InvalidParamsReasonData
			if decodeData(rpcErr.Data, &d) {
				if text, ok := sessionReasonTexts[d.Reason]; ok {
					return errors.New(text)
				}
			}
		}
	}
	return userError(err, log)
}
