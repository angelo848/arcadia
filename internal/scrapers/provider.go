// Package scrapers talks to the metadata and media providers of the library (phase 7, ARCHITECTURE
// §6). Each provider implements MetadataProvider; none of them keeps state beyond its own token.
package scrapers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Kinds of remote media (MID-004), the same names as the media table.
const (
	KindCover      = "cover"
	KindScreenshot = "screenshot"
	KindTitle      = "title"
)

// ErrNoKey is returned, without any request, when a provider needs a key the user has not
// registered (MID-001).
var ErrNoKey = errors.New("scrapers: chave não cadastrada")

// ErrUnavailable is a temporary failure: a network error, a 429 or a 5xx answer (MID-008).
var ErrUnavailable = errors.New("scrapers: provedor indisponível")

// Query is what a search looks for: the title the library has, and the platform and year when known.
type Query struct {
	Title    string
	Platform string
	Year     int
}

// Candidate is a game a provider found for a query (MID-002).
type Candidate struct {
	Provider   string
	ProviderID string
	Title      string
	Platform   string
	Year       int
}

// Metadata is the text a provider knows about a game (MID-003). Empty fields mean unknown.
type Metadata struct {
	Title       string
	Description string
	Year        int
	Genres      []string
	Modes       []string
	Developers  []string
	Publishers  []string
}

// RemoteMedia is one image a provider offers; URL is downloaded by the media package (MID-004).
type RemoteMedia struct {
	Kind string
	URL  string
}

// MetadataProvider is the interface of ARCHITECTURE §6, with the context first and the errors
// above for the cases the agent maps to user messages.
type MetadataProvider interface {
	ID() string
	Search(ctx context.Context, q Query) ([]Candidate, error)
	Details(ctx context.Context, providerID string) (Metadata, error)
	Media(ctx context.Context, providerID string) ([]RemoteMedia, error)
}

// request describes one HTTP call of a provider.
type request struct {
	method  string
	url     string
	headers map[string]string
	body    string
}

// doJSON sends the request and decodes the JSON answer into out. A network error, a 429 or a
// 5xx answer wraps ErrUnavailable; any other non-2xx answer is a plain error.
func doJSON(ctx context.Context, client *http.Client, provider string, rq request, out any) error {
	var body io.Reader
	if rq.body != "" {
		body = strings.NewReader(rq.body)
	}
	req, err := http.NewRequestWithContext(ctx, rq.method, rq.url, body)
	if err != nil {
		return fmt.Errorf("scrapers: %s: %w", provider, err)
	}
	for k, v := range rq.headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrUnavailable, provider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return fmt.Errorf("%w: %s: status %d", ErrUnavailable, provider, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("scrapers: %s: status %d", provider, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("scrapers: %s: resposta inválida: %w", provider, err)
	}
	return nil
}
