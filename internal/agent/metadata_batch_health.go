package agent

import (
	"errors"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// providerHealth keeps, for one batch, the failures in a row of each provider, the instant until which
// it is suspended and whether it is disabled for the rest of the batch [MET-011]. Only temporary failures
// (scrapers.ErrUnavailable) count; a success clears the count. Safe for concurrent use.
type providerHealth struct {
	clock       domain.Clock
	mu          sync.Mutex
	byName      map[string]*providerState
	disabledNow []string // providers disabled since the last takeDisabled, for the batch warnings
}

// providerState is the health of one provider within a batch.
type providerState struct {
	fails    int       // failures in a row
	until    time.Time // suspended until this instant; zero when not suspended
	disabled bool      // disabled for the rest of the batch
}

func newProviderHealth(clock domain.Clock) *providerHealth {
	return &providerHealth{clock: clock, byName: make(map[string]*providerState)}
}

// allowed reports whether provider may be asked now: not suspended and not disabled.
func (h *providerHealth) allowed(provider string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.byName[provider]
	if s == nil {
		return true
	}
	if s.disabled {
		return false
	}
	return !h.clock.Now().Before(s.until)
}

// report records the result of one request to provider. A nil err resets the count. A temporary
// failure suspends the provider for the next step of providerCooldown; the providerMaxFailures-th one
// in a row disables it. It returns true only when this call disabled the provider.
func (h *providerHealth) report(provider string, err error) (nowDisabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.byName[provider]
	if err == nil {
		if s != nil && !s.disabled {
			s.fails = 0
			s.until = time.Time{}
		}
		return false
	}
	if !errors.Is(err, scrapers.ErrUnavailable) {
		return false
	}
	if s == nil {
		s = &providerState{}
		h.byName[provider] = s
	}
	if s.disabled {
		return false
	}
	s.fails++
	if s.fails >= providerMaxFailures {
		s.disabled = true
		s.until = time.Time{}
		h.disabledNow = append(h.disabledNow, provider)
		return true
	}
	step := min(s.fails-1, len(providerCooldown)-1)
	s.until = h.clock.Now().Add(providerCooldown[step])
	return false
}

// takeDisabled returns the providers disabled since the last call and forgets them, so the batch adds
// each warning once [MET-011].
func (h *providerHealth) takeDisabled() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.disabledNow
	h.disabledNow = nil
	return out
}
