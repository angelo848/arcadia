package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// sessionEditStore is what the session edits need from the store.
type sessionEditStore interface {
	CreateManualSession(ctx context.Context, m store.ManualSession, now time.Time) (store.SessionWrite, error)
	EditSession(ctx context.Context, id int64, p store.SessionPatch, now time.Time) (store.SessionWrite, error)
	RemoveSession(ctx context.Context, id int64, now time.Time) (store.SessionWrite, error)
}

// sessionEdit serves session.create, session.update and session.delete (phase 16, SED-031, SED-032).
// It sends no notification and never touches the tracker (SED-022, SED-026).
type sessionEdit struct {
	st    sessionEditStore
	clock domain.Clock
	log   *slog.Logger
}

func sessionEditHandlers(e sessionEdit) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodSessionCreate: e.create,
		ipc.MethodSessionUpdate: e.update,
		ipc.MethodSessionDelete: e.remove,
	}
}

// fail translates a store error to its JSON-RPC error; refusals log at Debug, the rest at Error.
func (e sessionEdit) fail(what string, err error, sessionID, gameID int64) error {
	var open *store.SessionOpenError
	var edit *store.SessionEditError
	var overlap *store.SessionOverlapError
	switch {
	case errors.Is(err, store.ErrGameNotFound):
		e.log.Debug("edição de sessão recusada: jogo inexistente", "op", what)
		return ipc.NewErrorData(ipc.CodeGameNotFound, ipc.MsgGameNotFound, ipc.GameNotFoundData{GameID: gameID})
	case errors.Is(err, store.ErrSessionNotFound):
		e.log.Debug("edição de sessão recusada: sessão inexistente", "op", what)
		return ipc.NewErrorData(ipc.CodeSessionNotFound, ipc.MsgSessionNotFound, ipc.SessionNotFoundData{SessionID: sessionID})
	case errors.As(err, &open):
		e.log.Debug("edição de sessão recusada: sessão aberta", "op", what, "session_id", open.SessionID)
		return ipc.NewErrorData(ipc.CodeSessionActive, ipc.MsgSessionActive,
			ipc.SessionRef{SessionID: open.SessionID, GameID: open.GameID, Title: open.Title})
	case errors.As(err, &edit):
		e.log.Debug("edição de sessão recusada", "op", what, "reason", string(edit.Reason))
		return ipc.NewErrorData(ipc.CodeInvalidParams, ipc.MsgInvalidParams, ipc.InvalidParamsReasonData{Reason: string(edit.Reason)})
	case errors.As(err, &overlap):
		e.log.Debug("edição de sessão recusada: sobreposição", "op", what, "other_session_id", overlap.SessionID)
		d := ipc.SessionOverlapData{SessionID: overlap.SessionID, GameID: overlap.GameID, Title: overlap.Title, StartedAt: overlap.StartedAt.UTC()}
		if overlap.EndedAt != nil {
			t := overlap.EndedAt.UTC()
			d.EndedAt = &t
		}
		return ipc.NewErrorData(ipc.CodeSessionOverlap, ipc.MsgSessionOverlap, d)
	}
	e.log.Error("edição de sessão: "+what, "error", err.Error())
	return ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
}

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func (e sessionEdit) create(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.SessionCreateParams
	if err := decodeOrInvalid(params, &p); err != nil {
		return nil, err
	}
	if p.GameID < 1 || p.StartedAt.IsZero() || p.EndedAt.IsZero() || p.ActiveSeconds < 0 || p.ActiveSeconds > ipc.MaxPlaySeconds {
		return nil, errBadEdit()
	}
	w, err := e.st.CreateManualSession(ctx, store.ManualSession{
		GameID: p.GameID, Start: p.StartedAt.Truncate(time.Millisecond), End: p.EndedAt.Truncate(time.Millisecond),
		Active: time.Duration(p.ActiveSeconds) * time.Second, IncludedInTotal: p.IncludedInTotal,
	}, e.clock.Now())
	if err != nil {
		return nil, e.fail("criar", err, 0, p.GameID)
	}
	e.log.Info("sessão criada", "session_id", w.Session.ID, "game_id", w.Session.GameID, "source", w.Session.Source,
		"started_at", rfc(w.Session.StartedAt), "ended_at", rfc(w.Session.EndedAt), "active_seconds", w.Session.ActiveSeconds,
		"included_in_total", p.IncludedInTotal, "total_before", w.TotalBefore, "total_after", w.TotalAfter)
	return ipc.SessionWriteResult{Session: sessionItemToIPC(w.Session), Game: gameDetailToIPC(w.Game)}, nil
}

func (e sessionEdit) update(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.SessionUpdateParams
	if err := decodeOrInvalid(params, &p); err != nil {
		return nil, err
	}
	if p.SessionID < 1 || p.StartedAt.Null || p.EndedAt.Null || p.ActiveSeconds.Null ||
		(p.ActiveSeconds.Present && (p.ActiveSeconds.Value < 0 || p.ActiveSeconds.Value > ipc.MaxPlaySeconds)) {
		return nil, errBadEdit()
	}
	var patch store.SessionPatch
	if p.StartedAt.Present {
		t := p.StartedAt.Value.Truncate(time.Millisecond)
		patch.Start = &t
	}
	if p.EndedAt.Present {
		t := p.EndedAt.Value.Truncate(time.Millisecond)
		patch.End = &t
	}
	if p.ActiveSeconds.Present {
		d := time.Duration(p.ActiveSeconds.Value) * time.Second
		patch.Active = &d
	}
	w, err := e.st.EditSession(ctx, p.SessionID, patch, e.clock.Now())
	if err != nil {
		return nil, e.fail("editar", err, p.SessionID, 0)
	}
	if w.Changed {
		attrs := []any{"session_id", w.Session.ID, "game_id", w.Session.GameID, "source", w.Session.Source}
		if w.Previous != nil {
			attrs = append(attrs, "old_started_at", rfc(w.Previous.StartedAt), "old_ended_at", rfc(w.Previous.EndedAt),
				"old_active_seconds", w.Previous.ActiveSeconds)
		}
		attrs = append(attrs, "new_started_at", rfc(w.Session.StartedAt), "new_ended_at", rfc(w.Session.EndedAt),
			"new_active_seconds", w.Session.ActiveSeconds, "total_before", w.TotalBefore, "total_after", w.TotalAfter)
		e.log.Info("sessão editada", attrs...)
	}
	return ipc.SessionWriteResult{Session: sessionItemToIPC(w.Session), Game: gameDetailToIPC(w.Game)}, nil
}

func (e sessionEdit) remove(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.SessionDeleteParams
	if err := decodeOrInvalid(params, &p); err != nil {
		return nil, err
	}
	if p.SessionID < 1 {
		return nil, errBadEdit()
	}
	w, err := e.st.RemoveSession(ctx, p.SessionID, e.clock.Now())
	if err != nil {
		return nil, e.fail("excluir", err, p.SessionID, 0)
	}
	if w.Changed {
		attrs := []any{"session_id", p.SessionID}
		if w.Previous != nil {
			attrs = append(attrs, "game_id", w.Previous.GameID, "source", w.Previous.Source,
				"old_started_at", rfc(w.Previous.StartedAt), "old_ended_at", rfc(w.Previous.EndedAt),
				"old_active_seconds", w.Previous.ActiveSeconds)
		}
		attrs = append(attrs, "total_before", w.TotalBefore, "total_after", w.TotalAfter)
		e.log.Info("sessão excluída", attrs...)
	}
	return ipc.SessionDeleteResult{SessionID: p.SessionID, Game: gameDetailToIPC(w.Game)}, nil
}
