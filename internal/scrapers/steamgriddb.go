package scrapers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const sgdbAPI = "https://www.steamgriddb.com/api/v2"

// SGDBConfig holds the API key of SteamGridDB (MID-001). APIBase is the real service unless a test
// points it at a local server.
type SGDBConfig struct {
	Key     string
	APIBase string
	HTTP    *http.Client
}

// SteamGridDB is the SteamGridDB provider (ARCHITECTURE §6): covers. It has no text metadata, so
// Details is empty.
type SteamGridDB struct {
	cfg SGDBConfig
}

// NewSteamGridDB returns the SteamGridDB provider with the defaults filled in.
func NewSteamGridDB(cfg SGDBConfig) *SteamGridDB {
	if cfg.APIBase == "" {
		cfg.APIBase = sgdbAPI
	}
	if cfg.HTTP == nil {
		cfg.HTTP = http.DefaultClient
	}
	return &SteamGridDB{cfg: cfg}
}

// ID is the provider id stored in the media rows.
func (p *SteamGridDB) ID() string { return "steamgriddb" }

// auth is the header of every SteamGridDB call; without a key nothing is sent (MID-001).
func (p *SteamGridDB) auth() (map[string]string, error) {
	if p.cfg.Key == "" {
		return nil, ErrNoKey
	}
	return map[string]string{"Authorization": "Bearer " + p.cfg.Key}, nil
}

// Search finds games by title through the autocomplete endpoint.
func (p *SteamGridDB) Search(ctx context.Context, q Query) ([]Candidate, error) {
	h, err := p.auth()
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			ReleaseDate int64  `json:"release_date"`
		} `json:"data"`
	}
	endpoint := p.cfg.APIBase + "/search/autocomplete/" + url.PathEscape(q.Title)
	if err := doJSON(ctx, p.cfg.HTTP, p.ID(), request{method: http.MethodGet, url: endpoint, headers: h}, &out); err != nil {
		return nil, err
	}
	cands := make([]Candidate, 0, len(out.Data))
	for _, d := range out.Data {
		cands = append(cands, Candidate{
			Provider:   p.ID(),
			ProviderID: strconv.FormatInt(d.ID, 10),
			Title:      d.Name,
			Year:       unixYear(d.ReleaseDate),
		})
	}
	return cands, nil
}

// Details is empty: SteamGridDB offers images only.
func (p *SteamGridDB) Details(context.Context, string) (Metadata, error) {
	return Metadata{}, nil
}

// Media returns the grid covers of one game.
func (p *SteamGridDB) Media(ctx context.Context, providerID string) ([]RemoteMedia, error) {
	h, err := p.auth()
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	endpoint := fmt.Sprintf("%s/grids/game/%s", p.cfg.APIBase, strings.TrimSpace(providerID))
	if err := doJSON(ctx, p.cfg.HTTP, p.ID(), request{method: http.MethodGet, url: endpoint, headers: h}, &out); err != nil {
		return nil, err
	}
	var media []RemoteMedia
	for _, d := range out.Data {
		media = append(media, RemoteMedia{Kind: KindCover, URL: d.URL})
	}
	return media, nil
}
