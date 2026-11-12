package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

// updateHandlerTimeout bounds the UpdateGame call of game.update [IPC-045].
const updateHandlerTimeout = 5 * time.Second

// maxPlayModesPerGame is the maxItems of playModes in the contract [IPC-045].
const maxPlayModesPerGame = 5

// handleGameUpdate serves game.update: a patch of the metadata of a game, answered
// with its detail [IPC-045, IPC-052, EDT-001..EDT-009]. The patch is checked here,
// before the store is used, so an invalid member writes nothing. The time of the
// edit is clock.Now() read at receipt (ADR 0011). No notification is sent for a
// library write [IPC-050], and the tracker is not involved (EDT-006).
func handleGameUpdate(ctx context.Context, st gameStore, clock domain.Clock, log *slog.Logger,
	params json.RawMessage) (any, error) {
	var p ipc.GameUpdateParams
	if rpcErr := ipc.DecodeParams(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if p.GameID < 1 {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	at := clock.Now()
	patch, err := patchFromParams(p, clock)
	if err != nil {
		return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	uctx, cancel := context.WithTimeout(ctx, updateHandlerTimeout)
	defer cancel()
	d, err := st.UpdateGame(uctx, p.GameID, patch, at)
	if err != nil {
		return nil, gameUpdateError(p.GameID, err, log)
	}
	return ipc.GameDetailResult{Game: gameDetailToIPC(d)}, nil
}

// patchFromParams turns the members of game.update into a store patch [IPC-052].
// An absent member gives a nil field (unchanged). A null is refused for every
// member except completedAt and year, where it clears the value. Each value is
// checked with the domain rules: title, description, status, dates, years, names
// (DOM-016, duplicates without regard to case are removed), play modes (DOM-014)
// and platformId >= 1. Any failure is an error wrapping domain.ErrInvalidField.
func patchFromParams(p ipc.GameUpdateParams, clock domain.Clock) (store.GamePatch, error) {
	var out store.GamePatch
	var err error
	if out.Title, err = field(p.Title, domain.ValidateTitle); err != nil {
		return store.GamePatch{}, fmt.Errorf("title: %w", err)
	}
	if out.PlatformID, err = field(p.PlatformID, platformID); err != nil {
		return store.GamePatch{}, fmt.Errorf("platformId: %w", err)
	}
	if out.Status, err = field(p.Status, domain.ParseGameStatus); err != nil {
		return store.GamePatch{}, fmt.Errorf("status: %w", err)
	}
	if out.Favorite, err = field(p.Favorite, keep[bool]); err != nil {
		return store.GamePatch{}, fmt.Errorf("favorite: %w", err)
	}
	if out.Description, err = field(p.Description, domain.ValidateDescription); err != nil {
		return store.GamePatch{}, fmt.Errorf("description: %w", err)
	}
	if p.CompletedAt.Present {
		var c *time.Time
		if !p.CompletedAt.Null {
			t, err := domain.ParseCompletedDate(p.CompletedAt.Value)
			if err != nil {
				return store.GamePatch{}, fmt.Errorf("completedAt: %w", err)
			}
			c = &t
		}
		out.CompletedAt = &c
	}
	if p.Year.Present {
		var y *int
		if !p.Year.Null {
			if p.Year.Value < domain.YearMin || p.Year.Value > domain.YearMax {
				return store.GamePatch{}, fmt.Errorf("year %d outside %d..%d: %w",
					p.Year.Value, domain.YearMin, domain.YearMax, domain.ErrInvalidField)
			}
			v := p.Year.Value
			y = &v
		}
		out.Year = &y
	}
	if out.Developers, err = field(p.Developers, nameList); err != nil {
		return store.GamePatch{}, fmt.Errorf("developers: %w", err)
	}
	if out.Publishers, err = field(p.Publishers, nameList); err != nil {
		return store.GamePatch{}, fmt.Errorf("publishers: %w", err)
	}
	if out.Genres, err = field(p.Genres, nameList); err != nil {
		return store.GamePatch{}, fmt.Errorf("genres: %w", err)
	}
	if out.PlayModes, err = field(p.PlayModes, playModeList); err != nil {
		return store.GamePatch{}, fmt.Errorf("playModes: %w", err)
	}
	return out, nil
}

// field converts a patch member for the store: nil when absent, an error when it
// is null (only completedAt and year accept null) or conv refuses the value.
func field[T, R any](o ipc.Optional[T], conv func(T) (R, error)) (*R, error) {
	if !o.Present {
		return nil, nil
	}
	if o.Null {
		return nil, fmt.Errorf("null is not allowed: %w", domain.ErrInvalidField)
	}
	v, err := conv(o.Value)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// keep is the conversion of a value that needs no check.
func keep[T any](v T) (T, error) {
	return v, nil
}

// platformID accepts a platform id >= 1; whether it exists is the store's check.
func platformID(id int64) (int64, error) {
	if id < 1 {
		return 0, fmt.Errorf("platform id %d: %w", id, domain.ErrInvalidField)
	}
	return id, nil
}

// nameList checks a list of catalog names [EDT-004, DOM-016]: at most
// domain.MaxNameList items, each trimmed and at most domain.MaxNameRunes runes,
// and the duplicates without regard to case removed (the first spelling stays).
// The result is never nil.
func nameList(names []string) ([]string, error) {
	if len(names) > domain.MaxNameList {
		return nil, fmt.Errorf("%d names, max %d: %w", len(names), domain.MaxNameList, domain.ErrInvalidField)
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		clean, err := domain.NormalizeName(n)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(clean)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, clean)
	}
	return out, nil
}

// playModeList checks the play modes of a game [EDT-004, DOM-014]: exact names,
// at most maxPlayModesPerGame items and no repetition. The result is never nil.
func playModeList(names []string) ([]domain.PlayMode, error) {
	if len(names) > maxPlayModesPerGame {
		return nil, fmt.Errorf("%d play modes, max %d: %w", len(names), maxPlayModesPerGame, domain.ErrInvalidField)
	}
	out := make([]domain.PlayMode, 0, len(names))
	seen := make(map[domain.PlayMode]struct{}, len(names))
	for _, n := range names {
		m, err := domain.ParsePlayMode(n)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[m]; dup {
			return nil, fmt.Errorf("play mode %q repeated: %w", n, domain.ErrInvalidField)
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out, nil
}

// gameUpdateError maps a store error of game.update: an unknown game is -32001
// [IPC-045]; an unknown platform or a rejected field is -32602 (EDT-002, EDT-007);
// anything else is logged and returned for -32603.
func gameUpdateError(gameID int64, err error, log *slog.Logger) error {
	if errors.Is(err, store.ErrGameNotFound) {
		return ipc.NewErrorData(ipc.CodeGameNotFound, ipc.MsgGameNotFound, ipc.GameNotFoundData{GameID: gameID})
	}
	if errors.Is(err, store.ErrPlatformNotFound) || errors.Is(err, domain.ErrInvalidField) {
		return ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	log.Error("falha ao alterar o jogo", "game_id", gameID, "error", err.Error())
	return fmt.Errorf("game.update: %w", err)
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
