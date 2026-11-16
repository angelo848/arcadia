package scrapers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// ScreenScraper client errors (rules §6).
var (
	ErrNotFound      = errors.New("scrapers: sem correspondência no ScreenScraper")
	ErrQuota         = errors.New("scrapers: cota do ScreenScraper esgotada")
	ErrCredentials   = errors.New("scrapers: credenciais do ScreenScraper recusadas")
	ErrBadRequest    = errors.New("scrapers: pedido inválido ao ScreenScraper")
	ErrNoCredentials = errors.New("scrapers: credenciais do ScreenScraper ausentes")
	ErrNotVideo      = errors.New("scrapers: resposta do ScreenScraper não é vídeo")

	errRedirectOffHost = errors.New("scrapers: redirecionamento para fora do ScreenScraper")
)

// sameHostRedirect follows a redirect only to the same host (case-insensitive, with port) and never
// from https down to http; the net/http limit of 10 is kept.
func sameHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}
	first := via[0].URL
	if !strings.EqualFold(req.URL.Host, first.Host) || (via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https") {
		return errRedirectOffHost
	}
	return nil
}

// checkMediaOrigin applies rules §10: https on the BaseURL host or on *.screenscraper.fr; http only when the
// BaseURL itself is http and the host is the BaseURL host.
func checkMediaOrigin(base *url.URL, raw string) (*url.URL, error) {
	bad := fmt.Errorf("%w: URL de vídeo fora do ScreenScraper", ErrBadRequest)
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return nil, bad
	}
	sameHost := base != nil && strings.EqualFold(u.Host, base.Host)
	host := strings.ToLower(u.Hostname())
	switch u.Scheme {
	case "https":
		if sameHost || host == "screenscraper.fr" || strings.HasSuffix(host, ".screenscraper.fr") {
			return u, nil
		}
	case "http":
		if base != nil && base.Scheme == "http" && sameHost {
			return u, nil
		}
	}
	return nil, bad
}

// textError maps the known text messages of the API; nil when the text is not one of them.
func textError(text string) error {
	switch {
	case strings.Contains(text, "quota de scrape"):
		return ErrQuota
	case strings.Contains(text, "blacklisté"):
		return ErrCredentials
	case strings.Contains(text, "API closed"), strings.Contains(text, "API totalement fermé"):
		return ErrUnavailable
	case strings.Contains(text, "non trouvée"), strings.HasPrefix(strings.TrimSpace(text), "Erreur : Rom"):
		return ErrNotFound
	}
	return nil
}

// classifyMediaText is the text branch of classify for a 200 answer that is not a video.
func classifyMediaText(body []byte) error {
	if err := textError(string(body)); err != nil {
		return err
	}
	return ErrNotVideo
}

const ssDefaultBase = "https://api.screenscraper.fr/api2/"

// SSCreds are the four ScreenScraper credentials, read on every request.
type SSCreds struct {
	DevID, DevPassword, User, Password string
}

// SSConfig configures a ScreenScraper client. Creds returns ErrNoCredentials when one is missing.
type SSConfig struct {
	BaseURL string
	HTTP    *http.Client
	Limiter *Limiter
	Log     *slog.Logger
	Creds   func() (SSCreds, error)
}

// SSName is a game name in one region.
type SSName struct{ Region, Text string }

// SSMedia is one media of a game; Size 0 means unknown.
type SSMedia struct {
	Type, Region, Format, URL string
	Size                      int64
}

// SSGame is a game as answered by jeuRecherche or jeuInfos.
type SSGame struct {
	ID       string
	SystemID int
	Names    []SSName
	Years    []int
	Medias   []SSMedia
}

// SSQuota is the last ssuser reading; Known is false until a response carries one.
type SSQuota struct {
	Known         bool
	MaxThreads    int
	MaxPerMinute  int
	RequestsToday int
	MaxPerDay     int
	KoToday       int
	MaxKoPerDay   int
}

// ScreenScraper is the ScreenScraper API v2 client. It starts no goroutine.
type ScreenScraper struct {
	base    string
	baseURL *url.URL
	http    *http.Client
	lim     *Limiter
	log     *slog.Logger
	creds   func() (SSCreds, error)

	mu    sync.Mutex
	quota SSQuota
}

// NewScreenScraper returns a client over cfg.
func NewScreenScraper(cfg SSConfig) *ScreenScraper {
	base := cfg.BaseURL
	if base == "" {
		base = ssDefaultBase
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	c := cfg.HTTP
	if c == nil {
		c = http.DefaultClient
	}
	l := cfg.Log
	if l == nil {
		l = slog.Default()
	}
	cc := *c // shallow copy: the shared client keeps its own CheckRedirect
	cc.CheckRedirect = sameHostRedirect
	bu, _ := url.Parse(base)
	return &ScreenScraper{base: base, baseURL: bu, http: &cc, lim: cfg.Limiter, log: l, creds: cfg.Creds}
}

// Quota returns the last quota reading.
func (s *ScreenScraper) Quota() SSQuota {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.quota
}

// SearchGames asks jeuRecherche for the games matching term on a system.
func (s *ScreenScraper) SearchGames(ctx context.Context, systemID int, term string) ([]SSGame, SSQuota, error) {
	body, err := s.call(ctx, "jeuRecherche.php", url.Values{
		"systemeid": {strconv.Itoa(systemID)},
		"recherche": {term},
	})
	if err != nil {
		return nil, s.Quota(), err
	}
	var env struct {
		Response struct {
			SSUser *ssUser  `json:"ssuser"`
			Jeux   []ssGame `json:"jeux"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, s.Quota(), fmt.Errorf("%w: resposta ilegível", ErrUnavailable)
	}
	q := s.record(env.Response.SSUser)
	out := make([]SSGame, 0, len(env.Response.Jeux))
	for _, g := range env.Response.Jeux {
		out = append(out, g.game())
	}
	return out, q, nil
}

// GameInfo asks jeuInfos for one game; the answer carries a singular response.jeu.
func (s *ScreenScraper) GameInfo(ctx context.Context, systemID int, gameID string) (SSGame, SSQuota, error) {
	body, err := s.call(ctx, "jeuInfos.php", url.Values{
		"systemeid": {strconv.Itoa(systemID)},
		"gameid":    {gameID},
	})
	if err != nil {
		return SSGame{}, s.Quota(), err
	}
	var env struct {
		Response struct {
			SSUser *ssUser `json:"ssuser"`
			Jeu    *ssGame `json:"jeu"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return SSGame{}, s.Quota(), fmt.Errorf("%w: resposta ilegível", ErrUnavailable)
	}
	q := s.record(env.Response.SSUser)
	if env.Response.Jeu == nil {
		return SSGame{}, q, ErrNotFound
	}
	return env.Response.Jeu.game(), q, nil
}

// OpenVideo opens a media URL, adding the four credentials only when the URL lacks them. The caller closes
// the body; the size is the Content-Length (-1 when unknown).
func (s *ScreenScraper) OpenVideo(ctx context.Context, rawURL string) (io.ReadCloser, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, s.ctxErr(err)
	}
	u, err := checkMediaOrigin(s.baseURL, rawURL)
	if err != nil {
		return nil, 0, err
	}
	cr, err := s.creds()
	if err != nil {
		return nil, 0, fmt.Errorf("scrapers: credenciais do ScreenScraper: %w", err)
	}
	q := u.Query()
	have := map[string]bool{}
	for k := range q {
		have[strings.ToLower(k)] = true
	}
	for k, v := range map[string]string{"devid": cr.DevID, "devpassword": cr.DevPassword, "ssid": cr.User, "sspassword": cr.Password} {
		if !have[k] {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	resp, err := s.do(ctx, u.String())
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, 0, s.classify(resp.StatusCode, b, u.String())
	}
	if mt, _, perr := mime.ParseMediaType(resp.Header.Get("Content-Type")); perr == nil && strings.HasPrefix(mt, "text/") {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, 0, classifyMediaText(b)
	}
	return resp.Body, resp.ContentLength, nil
}

// call performs one API request and returns the JSON body of a successful answer.
func (s *ScreenScraper) call(ctx context.Context, endpoint string, params url.Values) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, s.ctxErr(err)
	}
	cr, err := s.creds()
	if err != nil {
		return nil, fmt.Errorf("scrapers: credenciais do ScreenScraper: %w", err)
	}
	if q := s.Quota(); q.Known && q.MaxPerDay > 0 && q.RequestsToday >= q.MaxPerDay {
		return nil, ErrQuota
	}
	params.Set("devid", cr.DevID)
	params.Set("devpassword", cr.DevPassword)
	params.Set("softname", SoftName)
	params.Set("ssid", cr.User)
	params.Set("sspassword", cr.Password)
	params.Set("output", "json")
	full := s.base + endpoint + "?" + params.Encode()
	resp, err := s.do(ctx, full)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, s.netErr(ctx, err)
	}
	if resp.StatusCode == http.StatusOK && bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		return body, nil
	}
	if resp.StatusCode == http.StatusOK {
		if err := s.classify(resp.StatusCode, body, full); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: resposta inesperada", ErrUnavailable)
	}
	return nil, s.classify(resp.StatusCode, body, full)
}

// do waits for the Limiter and sends the GET.
func (s *ScreenScraper) do(ctx context.Context, full string) (*http.Response, error) {
	if err := s.lim.Wait(ctx); err != nil {
		return nil, s.ctxErr(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: pedido", ErrBadRequest)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, s.netErr(ctx, err)
	}
	return resp, nil
}

func (s *ScreenScraper) ctxErr(err error) error {
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("scrapers: ScreenScraper: %w", err)
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}

func (s *ScreenScraper) netErr(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("scrapers: ScreenScraper: %w", context.Canceled)
	}
	if errors.Is(err, errRedirectOffHost) {
		return fmt.Errorf("%w: redirecionamento para fora do ScreenScraper", ErrBadRequest)
	}
	return fmt.Errorf("%w: %w", ErrUnavailable, RedactError(err))
}

// classify maps a non-JSON-success answer to an error: the text of the body first, then the status.
func (s *ScreenScraper) classify(status int, body []byte, full string) error {
	text := string(body)
	if !bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		switch {
		case strings.Contains(text, "quota de scrape"):
			return ErrQuota
		case strings.Contains(text, "blacklisté"):
			return ErrCredentials
		case strings.Contains(text, "API closed"), strings.Contains(text, "API totalement fermé"):
			return ErrUnavailable
		case strings.Contains(text, "non trouvée"), strings.HasPrefix(strings.TrimSpace(text), "Erreur : Rom"):
			return ErrNotFound
		}
	}
	switch {
	case status == 403 || status == 426:
		return ErrCredentials
	case status == 430 || status == 431:
		return ErrQuota
	case status == 404:
		return ErrNotFound
	case status == 400:
		s.log.Warn("screenscraper: pedido inválido", "url", RedactURL(full))
		return ErrBadRequest
	case status == 401 || status == 423 || status == 429 || status >= 500:
		return fmt.Errorf("%w: status %d", ErrUnavailable, status)
	}
	return fmt.Errorf("%w: status %d", ErrUnavailable, status)
}

// record keeps the ssuser reading and adjusts the Limiter from the next request on.
func (s *ScreenScraper) record(u *ssUser) SSQuota {
	if u == nil {
		return s.Quota()
	}
	perMin := int(u.PerMin)
	if perMin == 0 {
		perMin = int(u.PerDMin)
	}
	q := SSQuota{
		Known: true, MaxThreads: int(u.MaxThreads), MaxPerMinute: perMin, RequestsToday: int(u.Today),
		MaxPerDay: int(u.PerDay), KoToday: int(u.KoToday), MaxKoPerDay: int(u.KoPerDay),
	}
	s.mu.Lock()
	s.quota = q
	s.mu.Unlock()
	s.lim.SetInterval(SSInterval(perMin))
	return q
}

// flexInt decodes a number sent as a JSON string or number.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	t := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if t == "" || t == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

type flexStr string

func (f *flexStr) UnmarshalJSON(b []byte) error {
	t := strings.TrimSpace(string(b))
	if t == "null" {
		*f = ""
		return nil
	}
	*f = flexStr(strings.Trim(t, `"`))
	return nil
}

type ssUser struct {
	MaxThreads flexInt `json:"maxthreads"`
	PerMin     flexInt `json:"maxrequestspermin"`
	PerDMin    flexInt `json:"maxrequestsperdmin"`
	Today      flexInt `json:"requeststoday"`
	PerDay     flexInt `json:"maxrequestsperday"`
	KoToday    flexInt `json:"requestskotoday"`
	KoPerDay   flexInt `json:"maxrequestskoperday"`
}

type ssRegionText struct {
	Region string `json:"region"`
	Text   string `json:"text"`
}

type ssGame struct {
	ID      flexStr        `json:"id"`
	Noms    []ssRegionText `json:"noms"`
	Systeme struct {
		ID flexInt `json:"id"`
	} `json:"systeme"`
	Dates  []ssRegionText `json:"dates"`
	Medias []struct {
		Type   string  `json:"type"`
		Region string  `json:"region"`
		Format string  `json:"format"`
		URL    string  `json:"url"`
		Size   flexInt `json:"size"`
	} `json:"medias"`
}

func (g ssGame) game() SSGame {
	out := SSGame{ID: string(g.ID), SystemID: int(g.Systeme.ID)}
	for _, n := range g.Noms {
		out.Names = append(out.Names, SSName{Region: n.Region, Text: n.Text})
	}
	for _, d := range g.Dates {
		if len(d.Text) >= 4 {
			if y, err := strconv.Atoi(d.Text[:4]); err == nil && y > 0 {
				out.Years = append(out.Years, y)
			}
		}
	}
	for _, m := range g.Medias {
		out.Medias = append(out.Medias, SSMedia{Type: m.Type, Region: m.Region, Format: m.Format, URL: m.URL, Size: int64(m.Size)})
	}
	return out
}
