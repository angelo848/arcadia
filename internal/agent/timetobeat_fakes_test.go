package agent

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Compile-time checks: the fakes are the only clients the service sees (TTB-031).
var (
	_ ttbSearcher = (*fakeHLTB)(nil)
	_ ttbFallback = (*fakeIGDB)(nil)
	_ ttbNotifier = (*spyNotifier)(nil)
)

// ttbWait is the guard of every wait on a channel; a healthy run never reaches it.
const ttbWait = 5 * time.Second

// ttbStart is the instant of the fake clock.
var ttbStart = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

// fakeHLTB is the HowLongToBeat client. fn answers each term (default: one exact "found" result);
// gate, when enabled, holds every call until release is signalled or the context ends.
type fakeHLTB struct {
	mu      sync.Mutex
	fn      func(ctx context.Context, term string) ([]scrapers.HLTBGame, error)
	terms   []string
	active  int
	maxAct  int
	gated   bool
	open    chan struct{} // closed by openGate
	entered chan string   // a call announces its term here before waiting on release (gated only)
	release chan struct{} // one send lets one call go
	called  chan string   // buffered: every call, in order (not gated)
}

func newFakeHLTB() *fakeHLTB {
	return &fakeHLTB{
		entered: make(chan string),
		release: make(chan struct{}),
		called:  make(chan string, 4096),
	}
}

// echoFound is the default answer: one exact, unambiguous result named like the term.
func echoFound(_ context.Context, term string) ([]scrapers.HLTBGame, error) {
	return []scrapers.HLTBGame{{ID: 1, Name: term, Main: 36000, MainExtra: 50000, Completionist: 90000}}, nil
}

func (f *fakeHLTB) setFn(fn func(ctx context.Context, term string) ([]scrapers.HLTBGame, error)) {
	f.mu.Lock()
	f.fn = fn
	f.mu.Unlock()
}

// gate makes the next calls block (until openGate); each entered term can be read from entered.
func (f *fakeHLTB) gate() {
	f.mu.Lock()
	f.gated = true
	f.open = make(chan struct{})
	f.mu.Unlock()
}

// openGate lets every current and future call through (idempotent).
func (f *fakeHLTB) openGate() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gated {
		f.gated = false
		close(f.open)
	}
}

func (f *fakeHLTB) Search(ctx context.Context, term string) ([]scrapers.HLTBGame, error) {
	f.mu.Lock()
	f.terms = append(f.terms, term)
	f.active++
	f.maxAct = max(f.maxAct, f.active)
	fn, gated, open := f.fn, f.gated, f.open
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()
	if gated {
		select {
		case f.entered <- term:
		case <-open:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case <-f.release:
		case <-open:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		select {
		case f.called <- term:
		default:
		}
	}
	if fn == nil {
		fn = echoFound
	}
	return fn(ctx, term)
}

func (f *fakeHLTB) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.terms...)
}

func (f *fakeHLTB) maxActive() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxAct
}

// fakeIGDB is the IGDB reserve. search and ttb answer each call; calls are recorded.
type fakeIGDB struct {
	mu      sync.Mutex
	search  func(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error)
	ttb     func(ctx context.Context, id string) (scrapers.IGDBTimeToBeat, bool, error)
	queries []scrapers.Query
	ttbIDs  []string
	called  chan string // title of every Search call
}

func newFakeIGDB() *fakeIGDB {
	return &fakeIGDB{
		search: func(context.Context, scrapers.Query) ([]scrapers.Candidate, error) {
			return nil, scrapers.ErrNoKey
		},
		ttb: func(context.Context, string) (scrapers.IGDBTimeToBeat, bool, error) {
			return scrapers.IGDBTimeToBeat{}, false, nil
		},
		called: make(chan string, 4096),
	}
}

func (f *fakeIGDB) setSearch(fn func(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error)) {
	f.mu.Lock()
	f.search = fn
	f.mu.Unlock()
}

func (f *fakeIGDB) setTTB(fn func(ctx context.Context, id string) (scrapers.IGDBTimeToBeat, bool, error)) {
	f.mu.Lock()
	f.ttb = fn
	f.mu.Unlock()
}

func (f *fakeIGDB) Search(ctx context.Context, q scrapers.Query) ([]scrapers.Candidate, error) {
	f.mu.Lock()
	f.queries = append(f.queries, q)
	fn := f.search
	f.mu.Unlock()
	select {
	case f.called <- q.Title:
	default:
	}
	return fn(ctx, q)
}

func (f *fakeIGDB) TimeToBeat(ctx context.Context, id string) (scrapers.IGDBTimeToBeat, bool, error) {
	f.mu.Lock()
	f.ttbIDs = append(f.ttbIDs, id)
	fn := f.ttb
	f.mu.Unlock()
	return fn(ctx, id)
}

func (f *fakeIGDB) searches() []scrapers.Query {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]scrapers.Query(nil), f.queries...)
}

func (f *fakeIGDB) ttbCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ttbIDs...)
}

// igdbMatch answers one candidate named like the query (a "match") with the given id.
func igdbMatch(id string) func(context.Context, scrapers.Query) ([]scrapers.Candidate, error) {
	return func(_ context.Context, q scrapers.Query) ([]scrapers.Candidate, error) {
		return []scrapers.Candidate{{
			Provider: "igdb", ProviderID: id, Title: q.Title, Year: q.Year,
			PlatformKeys: []string{q.SeedKey}, PlatformConfirmed: q.SeedKey != "",
		}}, nil
	}
}

// igdbValues answers the given record for any id.
func igdbValues(v scrapers.IGDBTimeToBeat) func(context.Context, string) (scrapers.IGDBTimeToBeat, bool, error) {
	return func(context.Context, string) (scrapers.IGDBTimeToBeat, bool, error) { return v, true, nil }
}

// spyNotifier records every TimeToBeatChanged call.
type spyNotifier struct {
	mu  sync.Mutex
	ids []int64
	ch  chan int64
}

func newSpyNotifier() *spyNotifier { return &spyNotifier{ch: make(chan int64, 4096)} }

func (s *spyNotifier) TimeToBeatChanged(gameID int64) {
	s.mu.Lock()
	s.ids = append(s.ids, gameID)
	s.mu.Unlock()
	select {
	case s.ch <- gameID:
	default:
	}
}

func (s *spyNotifier) all() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int64(nil), s.ids...)
}

// logRec is one captured log record.
type logRec struct {
	Level slog.Level
	Msg   string
	Attrs string // "k=v k=v"
}

// ttbLogCapture is a slog handler that keeps the records.
type ttbLogCapture struct {
	mu   sync.Mutex
	recs []logRec
}

func (c *ttbLogCapture) Enabled(context.Context, slog.Level) bool { return true }
func (c *ttbLogCapture) WithAttrs([]slog.Attr) slog.Handler       { return c }
func (c *ttbLogCapture) WithGroup(string) slog.Handler            { return c }
func (c *ttbLogCapture) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	r.Attrs(func(a slog.Attr) bool {
		b.WriteString(a.Key + "=" + a.Value.String() + " ")
		return true
	})
	c.mu.Lock()
	c.recs = append(c.recs, logRec{Level: r.Level, Msg: r.Message, Attrs: b.String()})
	c.mu.Unlock()
	return nil
}

func (c *ttbLogCapture) level(l slog.Level) []logRec {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []logRec
	for _, r := range c.recs {
		if r.Level == l {
			out = append(out, r)
		}
	}
	return out
}

// warnsWith returns the Warn records whose message contains sub.
func (c *ttbLogCapture) warnsWith(sub string) []logRec {
	var out []logRec
	for _, r := range c.level(slog.LevelWarn) {
		if strings.Contains(r.Msg, sub) {
			out = append(out, r)
		}
	}
	return out
}

// ttbHarness bundles a real store, the fake clock, the fakes and the service (not started).
type ttbHarness struct {
	t     *testing.T
	st    *store.Store
	clock *domain.FakeClock
	hltb  *fakeHLTB
	igdb  *fakeIGDB
	note  *spyNotifier
	logs  *ttbLogCapture
	svc   *timeToBeatService
}

func newTTBHarness(t *testing.T) *ttbHarness {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := &ttbHarness{
		t: t, st: st, clock: domain.NewFakeClock(ttbStart),
		hltb: newFakeHLTB(), igdb: newFakeIGDB(), note: newSpyNotifier(), logs: &ttbLogCapture{},
	}
	h.svc = newTimeToBeatService(st, h.hltb, h.igdb, h.clock, slog.New(h.logs), h.note)
	return h
}

// start launches the worker and closes the service at the end of the test (the gate is opened first so
// a blocked fake never keeps Close waiting).
func (h *ttbHarness) start() {
	h.t.Helper()
	h.svc.Start()
	h.t.Cleanup(func() {
		h.hltb.openGate()
		h.svc.Close()
	})
}

// game adds a game with the title and returns its id.
func (h *ttbHarness) game(title string) int64 {
	h.t.Helper()
	g, err := h.st.AddGame(h.t.Context(), store.NewGame{Title: title, Exe: `C:\Games\` + title + `.exe`, At: ttbStart})
	if err != nil {
		h.t.Fatalf("AddGame(%q): %v", title, err)
	}
	return g.ID
}

// setYear gives the game a release year.
func (h *ttbHarness) setYear(id int64, year int) {
	h.t.Helper()
	y := &year
	if _, err := h.st.UpdateGame(h.t.Context(), id, store.GamePatch{Year: &y}, ttbStart); err != nil {
		h.t.Fatalf("UpdateGame year: %v", err)
	}
}

// setTitle renames the game.
func (h *ttbHarness) setTitle(id int64, title string) {
	h.t.Helper()
	if _, err := h.st.UpdateGame(h.t.Context(), id, store.GamePatch{Title: &title}, ttbStart); err != nil {
		h.t.Fatalf("UpdateGame title: %v", err)
	}
}

// row reads the stored row of a game.
func (h *ttbHarness) row(id int64) (store.TimeToBeatRow, bool) {
	h.t.Helper()
	r, ok, err := h.st.GetTimeToBeat(h.t.Context(), id)
	if err != nil {
		h.t.Fatalf("GetTimeToBeat: %v", err)
	}
	return r, ok
}

// seed writes a row for the game with expiry computed by the domain from fetchedAt.
func (h *ttbHarness) seed(id int64, title string, row store.TimeToBeatRow, fetchedAt time.Time) {
	h.t.Helper()
	row.GameID, row.TitleAtFetch, row.FetchedAt = id, title, fetchedAt
	if row.Term == "" {
		row.Term = domain.SearchTerm(title)
	}
	row.ExpiresAt = domain.TTBExpiresAt(row.Source, domain.TTBState(row.State), fetchedAt)
	if err := h.st.UpsertTimeToBeat(h.t.Context(), row); err != nil {
		h.t.Fatalf("UpsertTimeToBeat: %v", err)
	}
}

// waitNote waits for a notification of game id (others are skipped) or fails.
func (h *ttbHarness) waitNote(id int64) {
	h.t.Helper()
	deadline := time.After(ttbWait)
	for {
		select {
		case got := <-h.note.ch:
			if got == id {
				return
			}
		case <-deadline:
			h.t.Fatalf("no timeToBeat.changed for game %d (got %v)", id, h.note.all())
		}
	}
}

// waitCall waits for the next call recorded on ch and returns its term.
func waitCall(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(ttbWait):
		t.Fatal("expected a call that never came")
		return ""
	}
}

// waitEntered waits for a gated HowLongToBeat call and returns its term.
func (h *ttbHarness) waitEntered() string {
	h.t.Helper()
	return waitCall(h.t, h.hltb.entered)
}

// i64 returns a pointer to v.
func i64(v int64) *int64 { return &v }

var _ io.Writer = io.Discard
