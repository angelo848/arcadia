package scrapers

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const steamStoreAPI = "https://store.steampowered.com"

// SteamConfig is the Steam Store API. It needs no key (ARCHITECTURE §6); APIBase is the real
// service unless a test points it at a local server.
type SteamConfig struct {
	APIBase string
	HTTP    *http.Client
}

// SteamStore is the Steam Store provider: description, genres, companies and screenshots of PC
// games (MID-002, MID-003).
type SteamStore struct {
	cfg SteamConfig
}

// NewSteamStore returns the Steam Store provider with the defaults filled in.
func NewSteamStore(cfg SteamConfig) *SteamStore {
	if cfg.APIBase == "" {
		cfg.APIBase = steamStoreAPI
	}
	if cfg.HTTP == nil {
		cfg.HTTP = http.DefaultClient
	}
	return &SteamStore{cfg: cfg}
}

// ID is the provider id stored in the media rows.
func (p *SteamStore) ID() string { return "steamstore" }

// Search finds PC games by title through the store search.
func (p *SteamStore) Search(ctx context.Context, q Query) ([]Candidate, error) {
	var out struct {
		Items []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	endpoint := p.cfg.APIBase + "/api/storesearch/?" + url.Values{"term": {q.Title}, "l": {"english"}, "cc": {"US"}}.Encode()
	if err := doJSON(ctx, p.cfg.HTTP, p.ID(), request{method: http.MethodGet, url: endpoint}, &out); err != nil {
		return nil, err
	}
	cands := make([]Candidate, 0, len(out.Items))
	for _, it := range out.Items {
		cands = append(cands, Candidate{
			Provider:   p.ID(),
			ProviderID: strconv.FormatInt(it.ID, 10),
			Title:      it.Name,
			Platform:   "Windows",
		})
	}
	return cands, nil
}

// appDetails is the data of one appdetails answer, the fields this provider reads.
type appDetails struct {
	Name             string   `json:"name"`
	ShortDescription string   `json:"short_description"`
	Developers       []string `json:"developers"`
	Publishers       []string `json:"publishers"`
	Genres           []struct {
		Description string `json:"description"`
	} `json:"genres"`
	ReleaseDate struct {
		Date string `json:"date"`
	} `json:"release_date"`
	Screenshots []struct {
		PathFull string `json:"path_full"`
	} `json:"screenshots"`
}

// fetchApp returns the appdetails data of one app, or an empty value when the store says it has none.
func (p *SteamStore) fetchApp(ctx context.Context, providerID string) (appDetails, error) {
	var raw map[string]struct {
		Success bool        `json:"success"`
		Data    *appDetails `json:"data"`
	}
	endpoint := p.cfg.APIBase + "/api/appdetails?" + url.Values{"appids": {providerID}, "l": {"english"}}.Encode()
	if err := doJSON(ctx, p.cfg.HTTP, p.ID(), request{method: http.MethodGet, url: endpoint}, &raw); err != nil {
		return appDetails{}, err
	}
	entry, ok := raw[providerID]
	if !ok || !entry.Success || entry.Data == nil {
		return appDetails{}, nil
	}
	return *entry.Data, nil
}

// Details returns the text of one app.
func (p *SteamStore) Details(ctx context.Context, providerID string) (Metadata, error) {
	app, err := p.fetchApp(ctx, providerID)
	if err != nil {
		return Metadata{}, err
	}
	m := Metadata{
		Title:       app.Name,
		Description: app.ShortDescription,
		Developers:  app.Developers,
		Publishers:  app.Publishers,
	}
	for _, g := range app.Genres {
		m.Genres = append(m.Genres, g.Description)
	}
	if t, err := time.Parse("Jan 2, 2006", app.ReleaseDate.Date); err == nil {
		m.Year = t.Year()
	}
	return m, nil
}

// Media returns the screenshots of one app.
func (p *SteamStore) Media(ctx context.Context, providerID string) ([]RemoteMedia, error) {
	app, err := p.fetchApp(ctx, providerID)
	if err != nil {
		return nil, err
	}
	var out []RemoteMedia
	for _, s := range app.Screenshots {
		if strings.HasPrefix(s.PathFull, "https://") {
			out = append(out, RemoteMedia{Kind: KindScreenshot, URL: s.PathFull})
		}
	}
	return out, nil
}
