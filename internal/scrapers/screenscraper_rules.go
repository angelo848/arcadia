package scrapers

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// SoftName identifies the app to ScreenScraper; SSVideoMediaType is the video media type requested [SSC-004, VID-025].
const (
	SoftName         = "arcadia"
	SSVideoMediaType = "video-normalized"
)

// IntervalScreenScraper is the default spacing between ScreenScraper requests [VID-015].
const IntervalScreenScraper = 1500 * time.Millisecond

// ssSystemIDs maps a seed platform key to the ScreenScraper system id (rules §1). switch2 is not mapped.
var ssSystemIDs = map[string]int{
	"nes": 3, "snes": 4, "n64": 14, "gba": 12, "gamecube": 13, "wii": 16, "3ds": 17,
	"switch": 225, "ps1": 57, "ps2": 58, "ps3": 59, "windows": 138,
}

// SystemID returns the ScreenScraper system id of a seed platform key.
func SystemID(seedKey string) (int, bool) {
	id, ok := ssSystemIDs[seedKey]
	return id, ok
}

// SSInterval returns the spacing for threads requests per minute: ceil(60000/threads), never below
// IntervalScreenScraper; threads <= 0 gives IntervalScreenScraper.
func SSInterval(threads int) time.Duration {
	if threads <= 0 {
		return IntervalScreenScraper
	}
	ms := (60000 + threads - 1) / threads
	d := time.Duration(ms) * time.Millisecond
	if d < IntervalScreenScraper {
		return IntervalScreenScraper
	}
	return d
}

var secretParams = map[string]bool{"devid": true, "devpassword": true, "ssid": true, "sspassword": true}

// RedactURL removes the credential parameters from raw's query, keeping the order and escaping of the rest.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<url inválida>"
	}
	if u.RawQuery == "" {
		return raw
	}
	var kept []string
	for _, part := range strings.Split(u.RawQuery, "&") {
		if part == "" {
			continue
		}
		name, _, _ := strings.Cut(part, "=")
		dec, err := url.QueryUnescape(name)
		if err != nil {
			return "<url inválida>"
		}
		if secretParams[strings.ToLower(dec)] {
			continue
		}
		kept = append(kept, part)
	}
	u.RawQuery = strings.Join(kept, "&")
	return u.String()
}

type redactedError struct {
	msg     string
	wrapped error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.wrapped }

// RedactError returns err with the URL of any *url.Error in its chain redacted. The original is never changed.
func RedactError(err error) error {
	if err == nil {
		return nil
	}
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	cp := &url.Error{Op: ue.Op, URL: RedactURL(ue.URL), Err: ue.Err}
	old := ue.Error()
	msg := strings.Replace(err.Error(), old, cp.Error(), 1)
	return &redactedError{msg: msg, wrapped: cp}
}
