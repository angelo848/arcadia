package scrapers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// Constants of rules §1 for the HowLongToBeat client.
const (
	HLTBInterval       = 2 * time.Second
	HLTBRequestTimeout = 10 * time.Second
	HLTBPageSize       = 20
	HLTBMaxBody        = 2 << 20
	HLTBInitPath       = "/api/search/site/init"
	HLTBSearchPath     = "/api/search/site"

	hltbBaseURL   = "https://howlongtobeat.com"
	hltbName      = "hltb"
	hltbUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// HLTBConfig configures the HowLongToBeat client.
type HLTBConfig struct {
	BaseURL string       // default https://howlongtobeat.com
	HTTP    *http.Client // copied; Jar nil, Timeout unset
	Limiter *Limiter     // nil never waits
	Timeout time.Duration
	Clock   domain.Clock // t=<epoch ms> of the token request
}

// HLTB is the HowLongToBeat client [TTB-012]. The token lives only in memory.
type HLTB struct {
	cfg    HLTBConfig
	client *http.Client

	mu    sync.Mutex
	token string
}

// NewHLTB returns the client with the defaults filled in.
func NewHLTB(cfg HLTBConfig) *HLTB {
	if cfg.BaseURL == "" {
		cfg.BaseURL = hltbBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = HLTBRequestTimeout
	}
	if cfg.Clock == nil {
		cfg.Clock = domain.SystemClock{}
	}
	c := &http.Client{}
	if cfg.HTTP != nil {
		cp := *cfg.HTTP
		c = &cp
	}
	c.Jar = nil
	c.Timeout = 0
	return &HLTB{cfg: cfg, client: c}
}

func hltbUnavailable(format string, a ...any) error {
	return fmt.Errorf("%w: %s: %s", ErrUnavailable, hltbName, fmt.Sprintf(format, a...))
}

// do sends one request through the limiter with the per-request timeout. It returns the status and the
// body (at most HLTBMaxBody). A caller cancellation returns ctx.Err(); the other failures are ErrUnavailable
// without URL, host or body.
func (h *HLTB) do(ctx context.Context, method, path, query string, body []byte, withToken string) (int, []byte, error) {
	if err := h.cfg.Limiter.Wait(ctx); err != nil {
		return 0, nil, err
	}
	rctx, cancel := context.WithTimeout(ctx, h.cfg.Timeout)
	defer cancel()
	url := h.cfg.BaseURL + path
	if query != "" {
		url += "?" + query
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(rctx, method, url, rd)
	if err != nil {
		return 0, nil, hltbUnavailable("pedido inválido")
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", h.cfg.BaseURL)
	req.Header.Set("Referer", h.cfg.BaseURL+"/")
	req.Header.Set("User-Agent", hltbUserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if withToken != "" {
		req.Header.Set("x-auth-token", withToken)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return 0, nil, cerr
		}
		cause := netCause(err)
		for _, s := range []string{url, h.cfg.BaseURL, req.URL.Host, req.URL.Hostname()} {
			if s != "" {
				cause = strings.ReplaceAll(cause, s, "")
			}
		}
		return 0, nil, hltbUnavailable("%s", cause)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, HLTBMaxBody+1))
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return 0, nil, cerr
		}
		return 0, nil, hltbUnavailable("leitura da resposta")
	}
	if len(data) > HLTBMaxBody {
		return 0, nil, hltbUnavailable("formato do HowLongToBeat mudou")
	}
	return resp.StatusCode, data, nil
}

// fetchToken asks the site for a new token and keeps it in memory.
func (h *HLTB) fetchToken(ctx context.Context) (string, error) {
	t := strconv.FormatInt(h.cfg.Clock.Now().UnixMilli(), 10)
	status, data, err := h.do(ctx, http.MethodGet, HLTBInitPath, "t="+t, nil, "")
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", hltbUnavailable("token indisponível (status %d)", status)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.Token == "" {
		return "", hltbUnavailable("token indisponível")
	}
	h.mu.Lock()
	h.token = out.Token
	h.mu.Unlock()
	slog.Debug("token do HowLongToBeat renovado")
	return out.Token, nil
}

func (h *HLTB) currentToken() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.token
}

func (h *HLTB) dropToken() {
	h.mu.Lock()
	h.token = ""
	h.mu.Unlock()
}

func searchBody(term string) []byte {
	terms := strings.Fields(term)
	if terms == nil {
		terms = []string{}
	}
	body := map[string]any{
		"searchType":  "games",
		"searchTerms": terms,
		"searchPage":  1,
		"size":        HLTBPageSize,
		"searchOptions": map[string]any{
			"games": map[string]any{
				"userId": 0, "platform": "", "sortCategory": "popular", "rangeCategory": "main",
				"rangeTime": map[string]any{"min": nil, "max": nil},
				"gameplay":  map[string]any{"perspective": "", "flow": "", "genre": ""},
				"year":      map[string]any{"min": "", "max": ""},
				"modifier":  "",
			},
			"users":  map[string]any{"sortCategory": "postcount"},
			"lists":  map[string]any{"sortCategory": "follows"},
			"filter": "", "sort": 0, "randomizer": 0,
		},
		"useCache": true,
	}
	b, _ := json.Marshal(body)
	return b
}

// Search looks the term up; a 403 renews the token and repeats the search once [TTB-012].
func (h *HLTB) Search(ctx context.Context, term string) ([]HLTBGame, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body := searchBody(term)
	tok := h.currentToken()
	for attempt := 0; ; attempt++ {
		if tok == "" {
			var err error
			if tok, err = h.fetchToken(ctx); err != nil {
				return nil, err
			}
		}
		status, data, err := h.do(ctx, http.MethodPost, HLTBSearchPath, "", body, tok)
		if err != nil {
			return nil, err
		}
		switch {
		case status == http.StatusForbidden && attempt == 0:
			h.dropToken()
			tok = ""
			continue
		case status != http.StatusOK:
			return nil, hltbUnavailable("status %d", status)
		}
		games, perr := parseHLTB(data)
		if perr != nil {
			return nil, hltbUnavailable("formato do HowLongToBeat mudou")
		}
		return games, nil
	}
}
