package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

const (
	// listGamesTimeout bounds each ListGames call (DSK-016).
	listGamesTimeout = 5 * time.Second
	// libraryTimeout bounds the other library calls: GetGame, GameSessions,
	// UpdateGame, DeleteGame, Catalog and AddPlatform (DSK-016, DSK-017).
	libraryTimeout = 5 * time.Second
)

// LibraryQuery is the query of ListGames. Filter is a FilterSpec as JSON; an
// empty Filter means {"version":1}.
type LibraryQuery struct {
	Filter json.RawMessage `json:"filter"`
	Sort   string          `json:"sort"`
	Offset int             `json:"offset"`
	Limit  int             `json:"limit"`
}

// GameView is a game of the library as shown by the frontend. Times are RFC 3339
// or "" and are copied from the agent, never recomputed here.
type GameView struct {
	ID               int64  `json:"id"`
	PlatformID       int64  `json:"platformId"`
	Title            string `json:"title"`
	Platform         string `json:"platform"`
	Path             string `json:"path"`
	Status           string `json:"status"`
	Favorite         bool   `json:"favorite"`
	Year             *int   `json:"year"`
	TotalPlaySeconds int64  `json:"totalPlaySeconds"`
	PlayCount        int64  `json:"playCount"`
	FirstPlayedAt    string `json:"firstPlayedAt"`
	LastPlayedAt     string `json:"lastPlayedAt"`
}

// GameListView is the page returned by ListGames.
type GameListView struct {
	Games  []GameView `json:"games"`
	Total  int64      `json:"total"`
	Offset int        `json:"offset"`
}

// GameDetailView is a game page: GameView plus the editable fields and lists.
// CompletedAt is YYYY-MM-DD or "". The lists are never nil.
type GameDetailView struct {
	GameView
	Description string   `json:"description"`
	CompletedAt string   `json:"completedAt"`
	Developers  []string `json:"developers"`
	Publishers  []string `json:"publishers"`
	Genres      []string `json:"genres"`
	PlayModes   []string `json:"playModes"`
}

// SessionItemView is a session of a game as listed on the game page. EndedAt and
// EndReason are "" while the session is open.
type SessionItemView struct {
	SessionID     int64  `json:"sessionId"`
	GameID        int64  `json:"gameId"`
	State         string `json:"state"`
	StartedAt     string `json:"startedAt"`
	EndedAt       string `json:"endedAt"`
	ActiveSeconds int64  `json:"activeSeconds"`
	EndReason     string `json:"endReason"`
}

// SessionListView is the page returned by GameSessions. Sessions is never nil.
type SessionListView struct {
	Sessions []SessionItemView `json:"sessions"`
	Total    int64             `json:"total"`
	Offset   int               `json:"offset"`
}

// PlatformView is a platform of the catalog.
type PlatformView struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

// CatalogItemView is a genre or a company of the catalog: the database id the
// filters use, and the name.
type CatalogItemView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// CatalogView is the options of the editor and of the filters. Every list is
// never nil.
type CatalogView struct {
	Platforms []PlatformView    `json:"platforms"`
	Genres    []CatalogItemView `json:"genres"`
	Companies []CatalogItemView `json:"companies"`
	PlayModes []string          `json:"playModes"`
}

// gameView converts a game of the agent without recomputing its times.
func gameView(g ipc.Game) GameView {
	return GameView{
		ID:               g.ID,
		PlatformID:       g.PlatformID,
		Title:            g.Title,
		Platform:         g.Platform,
		Path:             g.Path,
		Status:           g.Status,
		Favorite:         g.Favorite,
		Year:             g.Year,
		TotalPlaySeconds: g.TotalPlaySeconds,
		PlayCount:        g.PlayCount,
		FirstPlayedAt:    optionalTime(g.FirstPlayedAt),
		LastPlayedAt:     optionalTime(g.LastPlayedAt),
	}
}

// optionalTime is RFC 3339 of t, or "" when t is nil.
func optionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}

// nonNilStrings returns s, or [] when s is nil.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func gameDetailView(d ipc.GameDetail) GameDetailView {
	completed := ""
	if d.CompletedAt != nil {
		completed = *d.CompletedAt
	}
	return GameDetailView{
		GameView:    gameView(d.Game),
		Description: d.Description,
		CompletedAt: completed,
		Developers:  nonNilStrings(d.Developers),
		Publishers:  nonNilStrings(d.Publishers),
		Genres:      nonNilStrings(d.Genres),
		PlayModes:   nonNilStrings(d.PlayModes),
	}
}

func sessionItemView(s ipc.SessionItem) SessionItemView {
	endReason := ""
	if s.EndReason != nil {
		endReason = *s.EndReason
	}
	return SessionItemView{
		SessionID:     s.SessionID,
		GameID:        s.GameID,
		State:         s.State,
		StartedAt:     formatTime(s.StartedAt),
		EndedAt:       optionalTime(s.EndedAt),
		ActiveSeconds: s.ActiveSeconds,
		EndReason:     endReason,
	}
}

func platformView(p ipc.Platform) PlatformView {
	return PlatformView{ID: p.ID, Name: p.Name, Category: p.Category}
}

// filterParam decodes the filter of a query. An empty or null filter is the
// empty FilterSpec {"version":1}; anything else must be a FilterSpec object.
func filterParam(raw json.RawMessage) (domain.FilterSpec, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return domain.FilterSpec{Version: domain.FilterVersion}, nil
	}
	var f domain.FilterSpec
	if err := json.Unmarshal(trimmed, &f); err != nil {
		return domain.FilterSpec{}, fmt.Errorf("filter: %w", err)
	}
	return f, nil
}

// patchParams builds the game.update params: the members of the raw patch as
// they came (absent stays absent, null stays null) plus gameId (decisão 10).
func patchParams(id int64, patch json.RawMessage) (map[string]json.RawMessage, error) {
	members := map[string]json.RawMessage{}
	if trimmed := bytes.TrimSpace(patch); len(trimmed) > 0 {
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &decoded); err != nil {
			return nil, fmt.Errorf("patch: %w", err)
		}
		if decoded != nil {
			members = decoded
		}
	}
	gameID, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("gameId: %w", err)
	}
	members["gameId"] = gameID
	return members, nil
}

// ListGames calls game.list (5 s timeout). The lists are never nil and no time
// is computed here: the values are the agent's.
func (a *App) ListGames(q LibraryQuery) (GameListView, error) {
	filter, err := filterParam(q.Filter)
	if err != nil {
		return GameListView{}, userError(ipc.NewError(ipc.CodeInvalidParams,
			fmt.Sprintf("game.list: %v", err)), a.logger())
	}
	params := ipc.GameListParams{Filter: &filter}
	if q.Sort != "" {
		sort := q.Sort
		params.Sort = &sort
	}
	if q.Offset != 0 {
		offset := q.Offset
		params.Offset = &offset
	}
	if q.Limit != 0 {
		limit := q.Limit
		params.Limit = &limit
	}
	var res ipc.GameListResult
	if err := a.callAgent(ipc.MethodGameList, listGamesTimeout, params, &res); err != nil {
		return GameListView{}, err
	}
	games := make([]GameView, 0, len(res.Games))
	for _, g := range res.Games {
		games = append(games, gameView(g))
	}
	return GameListView{Games: games, Total: res.Total, Offset: res.Offset}, nil
}

// GetGame calls game.get (5 s timeout).
func (a *App) GetGame(id int64) (GameDetailView, error) {
	var res ipc.GameDetailResult
	if err := a.callAgent(ipc.MethodGameGet, libraryTimeout, ipc.GameGetParams{GameID: id}, &res); err != nil {
		return GameDetailView{}, err
	}
	return gameDetailView(res.Game), nil
}

// GameSessions calls session.list (5 s timeout). Sessions is never nil.
func (a *App) GameSessions(id int64, offset, limit int) (SessionListView, error) {
	params := ipc.SessionListParams{GameID: id, Offset: &offset, Limit: &limit}
	var res ipc.SessionListResult
	if err := a.callAgent(ipc.MethodSessionList, libraryTimeout, params, &res); err != nil {
		return SessionListView{}, err
	}
	sessions := make([]SessionItemView, 0, len(res.Sessions))
	for _, s := range res.Sessions {
		sessions = append(sessions, sessionItemView(s))
	}
	return SessionListView{Sessions: sessions, Total: res.Total, Offset: res.Offset}, nil
}

// UpdateGame calls game.update (5 s timeout). The patch is passed as the raw
// JSON object it came in, so an absent member is not confused with null
// (decisão 10). It returns the game page as the agent stored it.
func (a *App) UpdateGame(id int64, patch json.RawMessage) (GameDetailView, error) {
	params, err := patchParams(id, patch)
	if err != nil {
		return GameDetailView{}, userError(ipc.NewError(ipc.CodeInvalidParams,
			fmt.Sprintf("game.update: %v", err)), a.logger())
	}
	var res ipc.GameDetailResult
	if err := a.callAgent(ipc.MethodGameUpdate, libraryTimeout, params, &res); err != nil {
		return GameDetailView{}, err
	}
	return gameDetailView(res.Game), nil
}

// DeleteGame calls game.delete (5 s timeout) and returns only the error. A
// refused removal (-32002) gets the DSK-018 text, which names no game.
func (a *App) DeleteGame(id int64) error {
	var res ipc.GameDeletedResult
	err := a.callAgentRaw(ipc.MethodGameDelete, libraryTimeout, ipc.GameDeleteParams{GameID: id}, &res)
	if err == nil {
		return nil
	}
	var rpcErr *ipc.Error
	if errors.As(err, &rpcErr) && rpcErr.Code == ipc.CodeSessionActive {
		return errors.New(msgRemoveSessionActive)
	}
	return userError(err, a.logger())
}

// Catalog calls library.catalog (5 s timeout). Every list is never nil.
func (a *App) Catalog() (CatalogView, error) {
	var res ipc.Catalog
	if err := a.callAgent(ipc.MethodLibraryCatalog, libraryTimeout, nil, &res); err != nil {
		return CatalogView{}, err
	}
	platforms := make([]PlatformView, 0, len(res.Platforms))
	for _, p := range res.Platforms {
		platforms = append(platforms, platformView(p))
	}
	return CatalogView{
		Platforms: platforms,
		Genres:    catalogItemViews(res.Genres),
		Companies: catalogItemViews(res.Companies),
		PlayModes: nonNilStrings(res.PlayModes),
	}, nil
}

// catalogItemViews converts genre or company items to views. The result is never nil.
func catalogItemViews(items []ipc.CatalogItem) []CatalogItemView {
	out := make([]CatalogItemView, 0, len(items))
	for _, it := range items {
		out = append(out, CatalogItemView{ID: it.ID, Name: it.Name})
	}
	return out
}

// AddPlatform calls platform.add (5 s timeout).
func (a *App) AddPlatform(name, category string) (PlatformView, error) {
	var res ipc.PlatformAddResult
	if err := a.callAgent(ipc.MethodPlatformAdd, libraryTimeout,
		ipc.PlatformAddParams{Name: name, Category: category}, &res); err != nil {
		return PlatformView{}, err
	}
	return platformView(res.Platform), nil
}
