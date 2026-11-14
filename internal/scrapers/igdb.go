package scrapers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

const (
	igdbAPI     = "https://api.igdb.com/v4"
	igdbToken   = "https://id.twitch.tv/oauth2/token"
	igdbImages  = "https://images.igdb.com/igdb/image/upload/t_screenshot_big/"
	igdbName    = "igdb"
	igdbSearchN = 10
)

// IGDBConfig holds the Twitch client of IGDB. The zero URLs are the real services; tests point
// them at a local server.
type IGDBConfig struct {
	ClientID, Secret string
	APIBase          string
	TokenURL         string
	ImageBase        string
	HTTP             *http.Client
	Clock            domain.Clock
}

// IGDB is the IGDB provider (ARCHITECTURE §6): metadata, genres, modes, companies and screenshots.
type IGDB struct {
	cfg IGDBConfig

	mu      sync.Mutex
	token   string
	expires time.Time
}

// NewIGDB returns the IGDB provider with the defaults filled in.
func NewIGDB(cfg IGDBConfig) *IGDB {
	if cfg.APIBase == "" {
		cfg.APIBase = igdbAPI
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = igdbToken
	}
	if cfg.ImageBase == "" {
		cfg.ImageBase = igdbImages
	}
	if cfg.HTTP == nil {
		cfg.HTTP = http.DefaultClient
	}
	if cfg.Clock == nil {
		cfg.Clock = domain.SystemClock{}
	}
	return &IGDB{cfg: cfg}
}

// ID is the provider id stored in the media and candidate rows.
func (p *IGDB) ID() string { return igdbName }

// accessToken returns the Twitch token, asking for a new one when the cached one is near its end.
func (p *IGDB) accessToken(ctx context.Context) (string, error) {
	if p.cfg.ClientID == "" || p.cfg.Secret == "" {
		return "", ErrNoKey
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && p.cfg.Clock.Now().Before(p.expires) {
		return p.token, nil
	}
	form := url.Values{"client_id": {p.cfg.ClientID}, "client_secret": {p.cfg.Secret}, "grant_type": {"client_credentials"}}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	err := doJSON(ctx, p.cfg.HTTP, igdbName, request{
		method:  http.MethodPost,
		url:     p.cfg.TokenURL + "?" + form.Encode(),
		headers: map[string]string{},
	}, &out)
	if err != nil {
		return "", err
	}
	p.token = out.AccessToken
	// Renew a minute early, so a request never goes out with a token about to expire.
	p.expires = p.cfg.Clock.Now().Add(time.Duration(out.ExpiresIn)*time.Second - time.Minute)
	return p.token, nil
}

// query posts an IGDB query (Apicalypse) to the endpoint and decodes the answer.
func (p *IGDB) query(ctx context.Context, endpoint, body string, out any) error {
	tok, err := p.accessToken(ctx)
	if err != nil {
		return err
	}
	return doJSON(ctx, p.cfg.HTTP, igdbName, request{
		method: http.MethodPost,
		url:    p.cfg.APIBase + endpoint,
		headers: map[string]string{
			"Client-ID":     p.cfg.ClientID,
			"Authorization": "Bearer " + tok,
			"Content-Type":  "text/plain",
		},
		body: body,
	}, out)
}

// Search finds games by title; the platform of the first IGDB platform naming Windows is PC.
func (p *IGDB) Search(ctx context.Context, q Query) ([]Candidate, error) {
	var rows []struct {
		ID               int64 `json:"id"`
		Name             string
		FirstReleaseDate int64 `json:"first_release_date"`
		Platforms        []struct {
			Name string `json:"name"`
		} `json:"platforms"`
	}
	body := fmt.Sprintf(`search "%s"; fields name,first_release_date,platforms.name; limit %d;`,
		strings.ReplaceAll(q.Title, `"`, ``), igdbSearchN)
	if err := p.query(ctx, "/games", body, &rows); err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(rows))
	for _, r := range rows {
		c := Candidate{Provider: igdbName, ProviderID: strconv.FormatInt(r.ID, 10), Title: r.Name, Year: unixYear(r.FirstReleaseDate)}
		for _, pl := range r.Platforms {
			if strings.Contains(pl.Name, "Windows") {
				c.Platform = "Windows"
				break
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// Details returns the text of one game.
func (p *IGDB) Details(ctx context.Context, providerID string) (Metadata, error) {
	var rows []struct {
		Name             string `json:"name"`
		Summary          string `json:"summary"`
		FirstReleaseDate int64  `json:"first_release_date"`
		Genres           []struct {
			Name string `json:"name"`
		} `json:"genres"`
		GameModes []struct {
			Name string `json:"name"`
		} `json:"game_modes"`
		InvolvedCompanies []struct {
			Developer bool `json:"developer"`
			Publisher bool `json:"publisher"`
			Company   struct {
				Name string `json:"name"`
			} `json:"company"`
		} `json:"involved_companies"`
	}
	body := fmt.Sprintf(`fields name,summary,first_release_date,genres.name,game_modes.name,involved_companies.developer,involved_companies.publisher,involved_companies.company.name; where id = %s;`, providerID)
	if err := p.query(ctx, "/games", body, &rows); err != nil {
		return Metadata{}, err
	}
	if len(rows) == 0 {
		return Metadata{}, nil
	}
	r := rows[0]
	m := Metadata{Title: r.Name, Description: r.Summary, Year: unixYear(r.FirstReleaseDate)}
	for _, g := range r.Genres {
		m.Genres = append(m.Genres, g.Name)
	}
	for _, md := range r.GameModes {
		m.Modes = append(m.Modes, md.Name)
	}
	for _, ic := range r.InvolvedCompanies {
		if ic.Developer {
			m.Developers = append(m.Developers, ic.Company.Name)
		}
		if ic.Publisher {
			m.Publishers = append(m.Publishers, ic.Company.Name)
		}
	}
	return m, nil
}

// Media returns the screenshots of one game (IGDB has no covers in this provider).
func (p *IGDB) Media(ctx context.Context, providerID string) ([]RemoteMedia, error) {
	var rows []struct {
		Screenshots []struct {
			ImageID string `json:"image_id"`
		} `json:"screenshots"`
	}
	body := fmt.Sprintf(`fields screenshots.image_id; where id = %s;`, providerID)
	if err := p.query(ctx, "/games", body, &rows); err != nil {
		return nil, err
	}
	var out []RemoteMedia
	for _, r := range rows {
		for _, s := range r.Screenshots {
			out = append(out, RemoteMedia{Kind: KindScreenshot, URL: p.cfg.ImageBase + s.ImageID + ".jpg"})
		}
	}
	return out, nil
}

// unixYear is the year of a Unix time in seconds, 0 when the time is 0 (unknown).
func unixYear(sec int64) int {
	if sec == 0 {
		return 0
	}
	return time.Unix(sec, 0).UTC().Year()
}
