package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// gameListTimeout bounds the ListGames call of game.list [IPC-042].
const gameListTimeout = 2 * time.Second

// handleGameGet serves game.get: the game with its detail [IPC-044, STO-026].
// gameId must be an integer >= 1; an unknown game is -32001.
func handleGameGet(ctx context.Context, st gameStore, log *slog.Logger, params json.RawMessage) (any, error) {
	var p ipc.GameGetParams
	if rpcErr := ipc.DecodeParams(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if p.GameID < 1 {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	d, err := st.GameDetail(ctx, p.GameID)
	if err != nil {
		return nil, gameReadError("game.get", p.GameID, err, log)
	}
	return ipc.GameDetailResult{Game: gameDetailToIPC(d)}, nil
}

// handleSessionList serves session.list: one page of the sessions of a game,
// most recent first [IPC-047, STO-027]. limit defaults to
// domain.DefaultSessionLimit and must be in 1..domain.MaxSessionLimit; offset
// defaults to 0 and must be >= 0. Anything else is -32602 and the store is not
// used.
func handleSessionList(ctx context.Context, st gameStore, log *slog.Logger, params json.RawMessage) (any, error) {
	var p ipc.SessionListParams
	if rpcErr := ipc.DecodeParams(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if p.GameID < 1 {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	offset := 0
	if p.Offset != nil {
		if *p.Offset < 0 {
			return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
		offset = *p.Offset
	}
	limit := domain.DefaultSessionLimit
	if p.Limit != nil {
		if *p.Limit < 1 || *p.Limit > domain.MaxSessionLimit {
			return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
		limit = *p.Limit
	}
	page, err := st.GameSessions(ctx, p.GameID, offset, limit)
	if err != nil {
		return nil, gameReadError("session.list", p.GameID, err, log)
	}
	items := make([]ipc.SessionItem, 0, len(page.Sessions))
	for _, s := range page.Sessions {
		items = append(items, sessionItemToIPC(s))
	}
	return ipc.SessionListResult{Sessions: items, Total: page.Total, Offset: offset}, nil
}

// gameReadError maps a store error of a read by game id: an unknown game is
// -32001 [IPC-044, IPC-047]; anything else is logged and returned for -32603.
func gameReadError(method string, gameID int64, err error, log *slog.Logger) error {
	if errors.Is(err, store.ErrGameNotFound) {
		return ipc.NewErrorData(ipc.CodeGameNotFound, ipc.MsgGameNotFound, ipc.GameNotFoundData{GameID: gameID})
	}
	log.Error("falha ao ler o jogo", "method", method, "game_id", gameID, "error", err.Error())
	return fmt.Errorf("%s: %w", method, err)
}

// gameDetailToIPC converts a store game detail to its wire form [STO-026,
// IPC-044]. The lists are never nil; CompletedAt is a YYYY-MM-DD or null.
func gameDetailToIPC(d store.GameDetail) ipc.GameDetail {
	out := ipc.GameDetail{
		Game:        toIPCGame(d.Game),
		Description: d.Description,
		Developers:  nonNilStrings(d.Developers),
		Publishers:  nonNilStrings(d.Publishers),
		Genres:      nonNilStrings(d.Genres),
		PlayModes:   nonNilStrings(d.PlayModes),
	}
	if !d.CompletedAt.IsZero() {
		s := domain.FormatCompletedDate(d.CompletedAt)
		out.CompletedAt = &s
	}
	return out
}

// sessionItemToIPC converts a store session to its wire form [STO-027,
// IPC-047]. EndedAt and EndReason are null while the session is open.
func sessionItemToIPC(s store.SessionItem) ipc.SessionItem {
	out := ipc.SessionItem{
		SessionID:     s.ID,
		GameID:        s.GameID,
		State:         string(s.State),
		StartedAt:     s.StartedAt.UTC(),
		ActiveSeconds: s.ActiveSeconds,
	}
	if !s.EndedAt.IsZero() {
		t := s.EndedAt.UTC()
		out.EndedAt = &t
	}
	if s.EndReason != "" {
		r := s.EndReason
		out.EndReason = &r
	}
	return out
}

// nonNilStrings returns s, or [] when s is nil, so the JSON is [] and not null.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// handleGameList serves game.list: one page of the library [IPC-042, IPC-043,
// LIB-002]. Absent or {} params are the first page of the empty filter; [] and
// any other non-object params are -32602 (DecodeParams).
func handleGameList(ctx context.Context, st gameStore, log *slog.Logger, params json.RawMessage) (any, error) {
	var p ipc.GameListParams
	if rpcErr := ipc.DecodeParams(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	q, err := gameListQuery(p)
	if err != nil {
		return nil, err
	}
	lctx, cancel := context.WithTimeout(ctx, gameListTimeout)
	defer cancel()
	page, err := st.ListGames(lctx, q)
	if err != nil {
		log.Error("falha ao listar os jogos", "error", err.Error())
		return nil, fmt.Errorf("game.list: %w", err)
	}
	games := make([]ipc.Game, 0, len(page.Games))
	for _, g := range page.Games {
		games = append(games, toIPCGame(g))
	}
	games, err = fitGamesPage(ipc.RequestID(ctx), games, page.Total, q.Offset)
	if err != nil {
		log.Error("falha ao medir a página de jogos", "error", err.Error())
		return nil, fmt.Errorf("game.list: %w", err)
	}
	return ipc.GameListResult{Games: games, Total: page.Total, Offset: q.Offset}, nil
}

// gameListQuery turns the params of game.list into a store query. A missing
// filter is {version 1} (normalized to playMode all and played any); sort
// defaults to title, offset to 0 and limit to domain.DefaultPageLimit
// [IPC-042, IPC-043]. Any invalid member is -32602 and the store is not used.
func gameListQuery(p ipc.GameListParams) (store.Query, error) {
	filter := domain.FilterSpec{Version: domain.FilterVersion}
	if p.Filter != nil {
		filter = *p.Filter
	}
	filter = filter.Normalize()
	if err := filter.Validate(); err != nil {
		return store.Query{}, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}

	sort := domain.DefaultSort
	if p.Sort != nil {
		s, err := domain.ParseSort(*p.Sort)
		if err != nil {
			return store.Query{}, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
		sort = s
	}

	offset := 0
	if p.Offset != nil {
		if *p.Offset < 0 {
			return store.Query{}, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
		offset = *p.Offset
	}

	limit := domain.DefaultPageLimit
	if p.Limit != nil {
		if *p.Limit < 1 || *p.Limit > domain.MaxPageLimit {
			return store.Query{}, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
		}
		limit = *p.Limit
	}

	return store.Query{Filter: filter, Sort: sort, Offset: offset, Limit: limit}, nil
}

// fitGamesPage keeps the longest prefix of games whose whole response, envelope
// included and without the trailing '\n', is at most ipc.MaxMessageSize bytes
// [IPC-051]. id is the raw request id that goes in the envelope. total and
// offset are the values of the result and do not change with the page.
func fitGamesPage(id json.RawMessage, games []ipc.Game, total int64, offset int) ([]ipc.Game, error) {
	emptyResult, err := json.Marshal(ipc.GameListResult{Games: []ipc.Game{}, Total: total, Offset: offset})
	if err != nil {
		return nil, fmt.Errorf("encode game list: %w", err)
	}
	used, err := responseSize(id, emptyResult)
	if err != nil {
		return nil, err
	}
	for i, g := range games {
		raw, err := json.Marshal(g)
		if err != nil {
			return nil, fmt.Errorf("encode game %d: %w", g.ID, err)
		}
		add := len(raw)
		if i > 0 {
			add++ // the comma between items
		}
		if used+add > ipc.MaxMessageSize {
			return games[:i], nil
		}
		used += add
	}
	return games, nil
}

// responseSize is the length of the encoded response with the given id and
// result, as ipc.Dispatch writes it (without the trailing '\n').
func responseSize(id json.RawMessage, result []byte) (int, error) {
	data, err := json.Marshal(ipc.Response{JSONRPC: ipc.Version, ID: id, Result: result})
	if err != nil {
		return 0, fmt.Errorf("encode response: %w", err)
	}
	return len(data), nil
}
