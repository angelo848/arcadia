package scrapers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const steamStoreAPI = "https://store.steampowered.com"

// steamCDNBase is the host of the capsule images; the thumbnail is built from the appid, without a request [MET-068].
const steamCDNBase = "https://cdn.cloudflare.steamstatic.com"

// SteamConfig is the Steam Store API. It needs no key (ARCHITECTURE §6); APIBase is the real
// service unless a test points it at a local server. Limiter spaces the requests [MET-010]; nil means no wait.
type SteamConfig struct {
	APIBase string
	HTTP    *http.Client
	Limiter *Limiter
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

// Search finds PC games by title through the store search. Every candidate is a Windows game
// (PlatformKeys "windows", confirmed) and its thumbnail comes from the appid [MET-003, MET-068].
func (p *SteamStore) Search(ctx context.Context, q Query) ([]Candidate, error) {
	var out struct {
		Items []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	endpoint := p.cfg.APIBase + "/api/storesearch/?" + url.Values{"term": {q.Title}, "l": {"english"}, "cc": {"US"}}.Encode()
	if err := p.cfg.Limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("scrapers: %s: %w", p.ID(), err)
	}
	if err := doJSON(ctx, p.cfg.HTTP, p.ID(), request{method: http.MethodGet, url: endpoint}, &out); err != nil {
		return nil, err
	}
	cands := make([]Candidate, 0, len(out.Items))
	for _, it := range out.Items {
		id := strconv.FormatInt(it.ID, 10)
		cands = append(cands, Candidate{
			Provider:          p.ID(),
			ProviderID:        id,
			Title:             it.Name,
			Platform:          "Windows",
			PlatformKeys:      []string{"windows"},
			PlatformConfirmed: true,
			ThumbURL:          steamCDNBase + "/steam/apps/" + id + "/capsule_231x87.jpg",
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
	if err := p.cfg.Limiter.Wait(ctx); err != nil {
		return appDetails{}, fmt.Errorf("scrapers: %s: %w", p.ID(), err)
	}
	if err := doJSON(ctx, p.cfg.HTTP, p.ID(), request{method: http.MethodGet, url: endpoint}, &raw); err != nil {
		return appDetails{}, err
	}
	entry, ok := raw[providerID]
	if !ok || !entry.Success || entry.Data == nil {
		return appDetails{}, nil
	}
	return *entry.Data, nil
}

// textOf converts the appdetails data into the Metadata of the provider.
func textOf(app appDetails) Metadata {
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
	return m
}

// screenshotsOf keeps the https screenshots of the appdetails data [MET-014].
func screenshotsOf(app appDetails) []RemoteMedia {
	var out []RemoteMedia
	for _, s := range app.Screenshots {
		if strings.HasPrefix(s.PathFull, "https://") {
			out = append(out, RemoteMedia{Kind: KindScreenshot, URL: s.PathFull})
		}
	}
	return out
}

// Fetch returns the text and the screenshots of one app in a single appdetails request [MET-014].
func (p *SteamStore) Fetch(ctx context.Context, providerID string) (Metadata, []RemoteMedia, error) {
	app, err := p.fetchApp(ctx, providerID)
	if err != nil {
		return Metadata{}, nil, err
	}
	return textOf(app), screenshotsOf(app), nil
}

// Details returns the text of one app.
func (p *SteamStore) Details(ctx context.Context, providerID string) (Metadata, error) {
	meta, _, err := p.Fetch(ctx, providerID)
	return meta, err
}

// Media returns the screenshots of one app.
func (p *SteamStore) Media(ctx context.Context, providerID string) ([]RemoteMedia, error) {
	_, media, err := p.Fetch(ctx, providerID)
	return media, err
}
