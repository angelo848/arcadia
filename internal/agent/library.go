package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// gameListTimeout bounds the ListGames call of game.list [IPC-042].
const gameListTimeout = 2 * time.Second

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
