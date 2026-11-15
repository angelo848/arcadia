package agent

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// batchProviders are the providers a batch can ask, in the aggregator's order.
var batchProviders = []string{"igdb", "steamgriddb", "steamstore"}

// batchKeyProviders are the providers that need a key and so count for batchStart's key check [MET-052].
var batchKeyProviders = []string{"igdb", "steamgriddb"}

// batchProviderLabels name the providers in the batch warnings [MET-011].
var batchProviderLabels = map[string]string{
	"igdb":        "IGDB",
	"steamgriddb": "SteamGridDB",
	"steamstore":  "Steam Store",
}

// errBatchStopped is returned by a game's application when the batch is no longer applying.
var errBatchStopped = errors.New("lote não está aplicando")

// batchRow is the row kept for one game of a batch [MET-053]. selected is the index of the chosen group
// (0 when there is one and the confidence is not none) and -1 otherwise.
type batchRow struct {
	gameID     int64
	title      string
	platform   string
	confidence domain.Confidence
	selected   int
	groups     []scrapers.Group
}

// toIPC converts the row to the contract's BatchRow.
func (r *batchRow) toIPC() ipc.BatchRow {
	cands := make([]ipc.MetadataCandidate, 0, len(r.groups))
	for _, g := range r.groups {
		cands = append(cands, candidateOf(g))
	}
	return ipc.BatchRow{
		GameID:     r.gameID,
		Title:      r.title,
		Platform:   r.platform,
		Confidence: string(r.confidence),
		Selected:   r.selected,
		Candidates: cands,
	}
}

// batchEvent tells the coordinator that a worker finished a game. The counters are not in it: the worker
// that writes a game counts it under the job's mutex, in the same block as the write [MET-066].
type batchEvent struct {
	failed bool
}

// applyItem is one choice of a batch application: the game, its title and the group the user picked.
type applyItem struct {
	pos    int // position of the game in the batch
	gameID int64
	title  string
	group  scrapers.Group
}

// batchJob is the one batch of the agent [MET-058]. Its fields are guarded by mu. Only the workers write
// the rows (while searching) and the results (while applying); only the coordinator writes the counters.
// Both do it under mu and only in the state that allows it [batch-state-machine.md §4].
type batchJob struct {
	id     int64
	games  []int64
	index  map[int64]int // position of each game in games
	health *providerHealth
	ctx    context.Context // the job's context: a child of the agent's, cancelled by cancel
	cancel context.CancelFunc

	mu           sync.Mutex
	state        string
	discarded    bool // set by batchDiscard; the job is gone for every caller that still holds it
	total        int  // games of a search, choices of an application [batch-state-machine.md §2]
	done         int
	failed       int
	warnings     []string
	rows         []*batchRow // by position in games; nil until the game has its row
	imagesQueued int         // applying: images queued by the batch [MET-065]
	imagesSkip   int         // applying: images left out by the limit [MET-022]
	failedTitles []string    // applying: titles of the failed games, up to maxFailedTitles
	result       *ipc.BatchResult
}

// nextBatchID returns the clock in milliseconds, or last+1 when the clock has not moved past last, so
// batchIds are strictly increasing [MET-058].
func nextBatchID(clock domain.Clock, last int64) int64 {
	id := clock.Now().UnixMilli()
	if id <= last {
		return last + 1
	}
	return id
}

// searchOne looks up one game of a batch and asks the applicable providers, Steam Store staged, with the
// deadline of a batch game [MET-053, MET-015]. It returns the row, whether every asked provider failed
// temporarily, and an error only for a store failure or a context end. A deleted game gives a none row.
func (m *metadataService) searchOne(ctx context.Context, gameID int64, h *providerHealth) (batchRow, bool, error) {
	none := batchRow{gameID: gameID, confidence: domain.ConfidenceNone, selected: -1}
	d, err := m.st.GameDetail(ctx, gameID)
	if errors.Is(err, store.ErrGameNotFound) {
		return none, false, nil
	}
	if err != nil {
		return none, false, m.scrape.storeError("ler jogo", err)
	}
	none.title, none.platform = d.Title, d.Platform
	q, err := m.gameQuery(ctx, d)
	if err != nil {
		return none, false, err
	}
	if q.Title == "" {
		return none, false, nil
	}
	out, err := m.agg.Search(ctx, q, scrapers.SearchOptions{
		Staged:  true,
		Timeout: gameSearchTimeout,
		// Skip answers true for a provider that must not be asked (aggregate.go), the opposite of allowed.
		Skip:   func(provider string) bool { return !h.allowed(provider) },
		Report: func(provider string, err error) { h.report(provider, err) },
	})
	if ctxErr := ctx.Err(); ctxErr != nil {
		return none, false, ctxErr
	}
	switch {
	case err == nil:
	case errors.Is(err, scrapers.ErrNoKey):
		return none, false, nil
	case errors.Is(err, scrapers.ErrUnavailable):
		return none, true, nil
	default:
		return none, false, fmt.Errorf("busca do lote: %w", err)
	}
	row := none
	row.confidence = out.Confidence
	if n := min(len(out.Groups), maxRowCandidates); n > 0 {
		row.groups = out.Groups[:n]
		if row.confidence != domain.ConfidenceNone {
			row.selected = 0
		}
	}
	return row, false, nil
}

// gameQuery builds the query of a stored game: its title as the search term, its release year and its
// platform, with the seed key and category when the platform is known [MET-053].
func (m *metadataService) gameQuery(ctx context.Context, d store.GameDetail) (scrapers.Query, error) {
	q := scrapers.Query{Title: domain.SearchTerm(d.Title), Platform: d.Platform}
	if y, ok := domain.ReleaseYear(d.ReleaseDate); ok {
		q.Year = y
	}
	if d.Platform == "" {
		return q, nil
	}
	plat, found, err := m.st.ResolvePlatform(ctx, d.Platform)
	if err != nil {
		return q, m.scrape.storeError("ler plataforma", err)
	}
	if found {
		q.SeedKey, q.Category = plat.SeedKey, string(plat.Category)
	}
	return q, nil
}

// batchStart validates the ids, reads the games, checks that some applicable provider has a key for
// some game, and starts the search in the background. It returns at once [MET-052].
func (m *metadataService) batchStart(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.BatchStartParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if !validBatchIDs(p.GameIDs) {
		return nil, badParams()
	}
	details := make([]store.GameDetail, 0, len(p.GameIDs))
	for _, id := range p.GameIDs {
		d, err := m.scrape.game(ctx, id)
		if err != nil {
			return nil, err
		}
		details = append(details, d)
	}
	if err := m.checkBatchKeys(ctx, details); err != nil {
		return nil, err
	}

	m.batchMu.Lock()
	defer m.batchMu.Unlock()
	if m.batch != nil {
		m.batch.mu.Lock()
		st, cancel := m.batch.state, m.batch.cancel
		m.batch.mu.Unlock()
		if st != ipc.BatchStateDone {
			return nil, batchBusy(st)
		}
		cancel()
		m.batch = nil
	}
	n := len(p.GameIDs)
	id := nextBatchID(m.clock, m.lastBatchID)
	m.lastBatchID = id
	jctx, cancel := context.WithCancel(m.root)
	job := &batchJob{
		id:     id,
		games:  slices.Clone(p.GameIDs),
		index:  make(map[int64]int, n),
		health: newProviderHealth(m.clock),
		ctx:    jctx,
		cancel: cancel,
		state:  ipc.BatchStateSearching,
		total:  n,
		rows:   make([]*batchRow, n),
	}
	for i, gid := range job.games {
		job.index[gid] = i
	}
	m.batch = job
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runSearch(job, jctx)
	}()
	return ipc.BatchStartResult{BatchID: id, Total: n}, nil
}

// validBatchIDs reports whether ids has 1 to maxBatchGames distinct ids, each at least 1 [MET-052, MET-071].
func validBatchIDs(ids []int64) bool {
	if len(ids) < 1 || len(ids) > maxBatchGames {
		return false
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id < 1 || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

// checkBatchKeys returns nil when some applicable keyed provider has a key for some game. Steam Store has no
// key, so it does not count. Otherwise it is the contract's -32010, with the first applicable keyed provider
// (IGDB when there is none) as data.provider [MET-052].
func (m *metadataService) checkBatchKeys(ctx context.Context, details []store.GameDetail) error {
	first := ""
	for _, d := range details {
		q, err := m.gameQuery(ctx, d)
		if err != nil {
			return err
		}
		info := scrapers.PlatformInfo{SeedKey: q.SeedKey, Category: q.Category, Known: q.Platform != "" || q.SeedKey != ""}
		for _, p := range batchKeyProviders {
			if !scrapers.Applicable(p, info) {
				continue
			}
			_, err := m.provider(p)
			if err == nil {
				return nil
			}
			if !errors.Is(err, scrapers.ErrNoKey) {
				return m.scrape.storeError("ler chave de provedor", err)
			}
			if first == "" {
				first = p
			}
		}
	}
	if first == "" {
		first = batchKeyProviders[0]
	}
	return m.scrape.providerError(first, scrapers.ErrNoKey)
}

// runSearch runs one search: the coordinator (this goroutine) owns the counters and the notifications;
// batchWorkers workers take the games in order. It returns when the batch is ready or ctx ends, and it
// waits for its workers first [MET-053, MET-059].
func (m *metadataService) runSearch(job *batchJob, ctx context.Context) {
	jobs := make(chan int, len(job.games))
	for i := range job.games {
		jobs <- i
	}
	close(jobs)
	events := make(chan batchEvent)
	var workers sync.WaitGroup
	for range batchWorkers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			m.searchWorker(job, ctx, jobs, events)
		}()
	}
	m.coordinate(job, ctx, events, len(job.games))
	workers.Wait()
}

// runApply applies the choices of a batch: one worker takes the items in order and the coordinator (this
// goroutine) notifies the progress. It returns when the batch is done or ctx ends, and it waits for its
// worker first [MET-065, MET-059].
func (m *metadataService) runApply(job *batchJob, ctx context.Context, items []applyItem) {
	events := make(chan batchEvent)
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		m.applyWorker(job, ctx, items, events)
	}()
	m.coordinate(job, ctx, events, len(items))
	worker.Wait()
}

// searchWorker takes game positions from jobs and searches them until jobs is empty or ctx ends. A row
// is kept only while the batch is still searching; a search cut by a cancellation writes nothing.
func (m *metadataService) searchWorker(job *batchJob, ctx context.Context, jobs <-chan int, events chan<- batchEvent) {
	for i := range jobs {
		if ctx.Err() != nil {
			return
		}
		row, failedAll, err := m.searchOne(ctx, job.games[i], job.health)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			m.log.Error("busca do lote", "game", job.games[i], "error", err.Error())
			failedAll = true
		}
		if !job.record(i, row, job.health.takeDisabled()) {
			return
		}
		select {
		case events <- batchEvent{failed: failedAll}:
		case <-ctx.Done():
			return
		}
	}
}

// applyWorker applies the items in order, one game at a time, and sends one event per item. It returns
// when the items are over or ctx ends; a game cut by the end writes nothing more [MET-059, MET-066].
func (m *metadataService) applyWorker(job *batchJob, ctx context.Context, items []applyItem, events chan<- batchEvent) {
	for _, it := range items {
		if ctx.Err() != nil {
			return
		}
		err := m.applyGame(ctx, job, it)
		if ctx.Err() != nil {
			return
		}
		ev := batchEvent{}
		if err != nil {
			if errors.Is(err, errBatchStopped) {
				return
			}
			m.log.Error("aplicação do lote", "game", it.gameID, "error", err.Error())
			job.countFailed(it.title)
			ev.failed = true
		}
		select {
		case events <- ev:
		case <-ctx.Done():
			return
		}
	}
}

// applyGame applies the group of one choice: the texts and the images the search already brought are used
// (IGDB needs no request); the others are fetched as metadata.apply does. The write happens only while the
// batch is still applying, under its mutex, and the game is counted in the same block, so a cancelled batch
// writes nothing more and its result has every game written [MET-065, MET-066].
func (m *metadataService) applyGame(ctx context.Context, job *batchJob, it applyItem) error {
	if _, err := m.scrape.game(ctx, it.gameID); err != nil {
		return err
	}
	texts := make(map[string]scrapers.Metadata, len(it.group.Refs))
	offers := make([]scrapers.Offer, 0, len(it.group.Refs))
	for _, r := range it.group.Refs {
		if md, ok := it.group.Meta[r.Provider]; ok {
			texts[r.Provider] = md
			offers = append(offers, scrapers.Offer{Provider: r.Provider, Media: it.group.Images[r.Provider]})
			continue
		}
		md, media, err := m.fetchOne(ctx, ipc.MetadataRef{Provider: r.Provider, ProviderID: r.ProviderID})
		if err != nil {
			return err
		}
		texts[r.Provider] = md
		offers = append(offers, scrapers.Offer{Provider: r.Provider, Media: media})
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.state != ipc.BatchStateApplying {
		return errBatchStopped
	}
	_, res, err := m.commitApply(ctx, it.gameID, texts, offers, false)
	if err != nil {
		return err
	}
	job.done++
	job.imagesQueued += res.queued
	job.imagesSkip += res.skipped
	return nil
}

// countFailed counts one failed game of the application and keeps its title, up to maxFailedTitles
// [MET-065]. It does nothing when the batch is no longer applying.
func (j *batchJob) countFailed(title string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state != ipc.BatchStateApplying {
		return
	}
	j.failed++
	if len(j.failedTitles) < maxFailedTitles {
		j.failedTitles = append(j.failedTitles, title)
	}
}

// coordinate counts the finished games of total, sends the notifications through the progress coalescer
// and moves the batch on (ready or done) when every game has its event. It returns without notifying when
// ctx ends [MET-054, MET-065].
func (m *metadataService) coordinate(job *batchJob, ctx context.Context, events <-chan batchEvent, total int) {
	coal := newProgressCoalescer(progressWindow)
	var timer <-chan time.Time
	for finished := 0; finished < total; {
		now := m.clock.Now()
		if coal.due(now) {
			if p, ok := job.progress(); ok {
				m.emit(p)
			}
		}
		if next, pending := coal.next(); pending && timer == nil {
			timer = m.clock.After(next.Sub(now))
		}
		select {
		case ev := <-events:
			finished++
			if p, ok := job.advance(ev); ok && coal.counters(m.clock.Now()) {
				m.emit(p)
			}
		case <-timer:
			timer = nil
		case <-ctx.Done():
			return
		}
	}
	if p, ok := job.finish(); ok {
		coal.state(m.clock.Now())
		m.emit(p)
	}
}

// emit sends a notification to the sink, when there is one.
func (m *metadataService) emit(p ipc.MetadataBatchChangedParams) {
	if m.notify != nil {
		m.notify.BatchChanged(p)
	}
}

// record keeps the row of position i and the warnings of the providers disabled meanwhile. It returns
// false, and keeps nothing, when the batch is no longer searching.
func (j *batchJob) record(i int, row batchRow, disabled []string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state != ipc.BatchStateSearching {
		return false
	}
	j.rows[i] = &row
	for _, p := range disabled {
		label := batchProviderLabels[p]
		if label == "" {
			label = p
		}
		j.addWarningLocked(label + " indisponível: a busca continuou sem ele")
	}
	return true
}

// addWarningLocked adds a warning once, up to maxStatusWarnings. The caller holds mu.
func (j *batchJob) addWarningLocked(w string) {
	if len(j.warnings) >= maxStatusWarnings || slices.Contains(j.warnings, w) {
		return
	}
	j.warnings = append(j.warnings, w)
}

// working reports whether the batch is searching or applying. The caller holds mu.
func (j *batchJob) working() bool {
	return j.state == ipc.BatchStateSearching || j.state == ipc.BatchStateApplying
}

// advance counts one finished game of the search and returns the notification to send, when the batch is
// still working. In applying the worker has counted the game already, so it only returns the notification
// [MET-065, MET-066].
func (j *batchJob) advance(ev batchEvent) (ipc.MetadataBatchChangedParams, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.working() {
		return ipc.MetadataBatchChangedParams{}, false
	}
	if j.state == ipc.BatchStateSearching {
		if ev.failed {
			j.failed++
		} else {
			j.done++
		}
	}
	return j.progressLocked(), true
}

// progress returns the current counters for a notification, when the batch is still working.
func (j *batchJob) progress() (ipc.MetadataBatchChangedParams, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.working() {
		return ipc.MetadataBatchChangedParams{}, false
	}
	return j.progressLocked(), true
}

// finish moves the batch on when its work is over: searching goes to ready, applying to done with its
// result. It does nothing when the batch was cancelled meanwhile.
func (j *batchJob) finish() (ipc.MetadataBatchChangedParams, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	switch j.state {
	case ipc.BatchStateSearching:
		j.state = ipc.BatchStateReady
	case ipc.BatchStateApplying:
		j.endLocked()
	default:
		return ipc.MetadataBatchChangedParams{}, false
	}
	return j.progressLocked(), true
}

// endLocked moves applying to done and builds the result from the counters. The caller holds mu.
func (j *batchJob) endLocked() {
	j.state = ipc.BatchStateDone
	j.result = &ipc.BatchResult{
		Applied:              j.done,
		ImagesQueued:         j.imagesQueued,
		ImagesSkippedByLimit: j.imagesSkip,
		FailedTitles:         append([]string{}, j.failedTitles...),
	}
}

// progressLocked is the notification of the current counters. The caller holds mu.
func (j *batchJob) progressLocked() ipc.MetadataBatchChangedParams {
	return ipc.MetadataBatchChangedParams{
		BatchID: j.id,
		State:   j.state,
		Done:    j.done,
		Total:   j.total,
		Failed:  j.failed,
	}
}

// statusLocked is the batch of metadata.batchStatus. The caller holds mu.
func (j *batchJob) statusLocked() *ipc.BatchStatus {
	st := &ipc.BatchStatus{
		BatchID:  j.id,
		State:    j.state,
		Total:    j.total,
		Done:     j.done,
		Failed:   j.failed,
		Warnings: append([]string{}, j.warnings...),
	}
	if j.state == ipc.BatchStateDone {
		st.Result = j.result
	}
	return st
}

// rowsLocked returns the rows that exist, in the order of the games. The caller holds mu.
func (j *batchJob) rowsLocked() []ipc.BatchRow {
	out := make([]ipc.BatchRow, 0, len(j.rows))
	for _, r := range j.rows {
		if r != nil {
			out = append(out, r.toIPC())
		}
	}
	return out
}

// validChoiceParams checks the choices of metadata.batchApply that need no batch: 1 to maxBatchChoices of
// them, no game repeated, and a candidate of 0 or more [MET-064, MET-071].
func validChoiceParams(choices []ipc.BatchChoice) bool {
	if len(choices) < 1 || len(choices) > maxBatchChoices {
		return false
	}
	seen := make(map[int64]bool, len(choices))
	for _, c := range choices {
		if c.Candidate < 0 || seen[c.GameID] {
			return false
		}
		seen[c.GameID] = true
	}
	return true
}

// choicesValidLocked reports whether every choice names a game of the batch that has a row, and a
// candidate of that row. A discarded job has no rows. The caller holds mu.
func (j *batchJob) choicesValidLocked(choices []ipc.BatchChoice) bool {
	if j.discarded {
		return false
	}
	for _, c := range choices {
		i, ok := j.index[c.GameID]
		if !ok {
			return false
		}
		r := j.rows[i]
		if r == nil || c.Candidate >= len(r.groups) {
			return false
		}
	}
	return true
}

// planLocked returns the items of valid choices, in the order of the games. The caller holds mu and has
// checked the choices with choicesValidLocked; a discarded job plans nothing.
func (j *batchJob) planLocked(choices []ipc.BatchChoice) []applyItem {
	if j.discarded {
		return nil
	}
	items := make([]applyItem, 0, len(choices))
	for _, c := range choices {
		pos := j.index[c.GameID]
		r := j.rows[pos]
		items = append(items, applyItem{pos: pos, gameID: c.GameID, title: r.title, group: r.groups[c.Candidate]})
	}
	slices.SortFunc(items, func(a, b applyItem) int { return cmp.Compare(a.pos, b.pos) })
	return items
}

// jobOf returns the batch when id is its id; otherwise the contract's -32014 [MET-058].
func (m *metadataService) jobOf(id int64) (*batchJob, error) {
	m.batchMu.Lock()
	defer m.batchMu.Unlock()
	if m.batch == nil || m.batch.id != id {
		return nil, ipc.NewError(ipc.CodeBatchNotFound, ipc.MsgBatchNotFound)
	}
	return m.batch, nil
}

// batchBusy is the contract's -32015 with the state that blocks the new batch.
func batchBusy(state string) error {
	return ipc.NewErrorData(ipc.CodeBatchBusy, ipc.MsgBatchBusy, ipc.BatchBusyData{State: state})
}

// batchWrongState is the contract's -32016 with the state the method refused.
func batchWrongState(state string) error {
	return ipc.NewErrorData(ipc.CodeBatchWrongState, ipc.MsgBatchWrongState, ipc.BatchWrongStateData{State: state})
}

// batchStatus returns the current batch with its counters, or null. It never fails by state [MET-055].
func (m *metadataService) batchStatus(_ context.Context, _ json.RawMessage) (any, error) {
	m.batchMu.Lock()
	job := m.batch
	m.batchMu.Unlock()
	if job == nil {
		return ipc.BatchStatusResult{}, nil
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	return ipc.BatchStatusResult{Batch: job.statusLocked()}, nil
}

// batchCancel stops a search (searching goes to cancelled at once, no new request) and is a no-op in
// ready and cancelled. In applying it ends the batch as done, with the partial result. It never notifies
// [MET-056, MET-066].
func (m *metadataService) batchCancel(_ context.Context, params json.RawMessage) (any, error) {
	var p ipc.BatchIDParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if p.BatchID < 1 {
		return nil, badParams()
	}
	job, err := m.jobOf(p.BatchID)
	if err != nil {
		return nil, err
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.discarded {
		return nil, ipc.NewError(ipc.CodeBatchNotFound, ipc.MsgBatchNotFound)
	}
	switch job.state {
	case ipc.BatchStateSearching:
		job.state = ipc.BatchStateCancelled
		job.cancel()
	case ipc.BatchStateApplying:
		job.endLocked()
		job.cancel()
	}
	return ipc.BatchCancelResult{State: job.state}, nil
}

// batchDiscard frees the batch in ready, cancelled or done; searching and applying refuse it [MET-058].
// It never notifies.
func (m *metadataService) batchDiscard(_ context.Context, params json.RawMessage) (any, error) {
	var p ipc.BatchIDParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if p.BatchID < 1 {
		return nil, badParams()
	}
	m.batchMu.Lock()
	defer m.batchMu.Unlock()
	job := m.batch
	if job == nil || job.id != p.BatchID {
		return nil, ipc.NewError(ipc.CodeBatchNotFound, ipc.MsgBatchNotFound)
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.state == ipc.BatchStateSearching || job.state == ipc.BatchStateApplying {
		return nil, batchWrongState(job.state)
	}
	job.discarded = true
	job.rows = nil
	job.result = nil
	job.cancel()
	m.batch = nil
	return ipc.BatchDiscardResult{Discarded: true}, nil
}

// batchResults returns one page of the rows of a ready or cancelled batch, under the ceiling of
// maxResultBytes [MET-057].
func (m *metadataService) batchResults(_ context.Context, params json.RawMessage) (any, error) {
	var p ipc.BatchResultsParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	limit := defaultResultsLimit
	if p.Limit != nil {
		limit = *p.Limit
	}
	if p.BatchID < 1 || p.Offset < 0 || limit < 1 || limit > maxResultsLimit {
		return nil, badParams()
	}
	job, err := m.jobOf(p.BatchID)
	if err != nil {
		return nil, err
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.discarded {
		return nil, ipc.NewError(ipc.CodeBatchNotFound, ipc.MsgBatchNotFound)
	}
	if job.state != ipc.BatchStateReady && job.state != ipc.BatchStateCancelled {
		return nil, batchWrongState(job.state)
	}
	rows := job.rowsLocked()
	page, next := pageRows(rows, p.Offset, limit)
	return ipc.BatchResultsResult{Rows: page, Total: len(rows), Next: next}, nil
}

// pageRows returns the rows from offset: at most limit of them and at most maxResultBytes of JSON, where
// a row counts len(Marshal)+1 bytes. A row is never cut; a row alone over the ceiling is returned alone.
// next is the index of the first row that did not fit, or nil when no row is left [MET-057].
func pageRows(rows []ipc.BatchRow, offset, limit int) (page []ipc.BatchRow, next *int) {
	from := min(max(offset, 0), len(rows))
	end, sum := from, 0
	for end < len(rows) && end-from < limit {
		size := rowSize(rows[end])
		if end > from && sum+size > maxResultBytes {
			break
		}
		sum += size
		end++
	}
	page = make([]ipc.BatchRow, 0, end-from)
	page = append(page, rows[from:end]...)
	if end < len(rows) {
		n := end
		next = &n
	}
	return page, next
}

// rowSize is the size of a row in a page: its JSON plus the separator [MET-057]. The marshal cannot fail
// for a BatchRow, so its error is not checked.
func rowSize(r ipc.BatchRow) int {
	raw, _ := json.Marshal(r)
	return len(raw) + 1
}

// batchApply accepts the choices of a ready or cancelled batch and starts the application in the
// background: the answer comes at once, the batch goes to applying and one worker writes game by game
// [MET-064, MET-065].
func (m *metadataService) batchApply(_ context.Context, params json.RawMessage) (any, error) {
	var p ipc.BatchApplyParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if p.BatchID < 1 || !validChoiceParams(p.Choices) {
		return nil, badParams()
	}
	job, err := m.jobOf(p.BatchID)
	if err != nil {
		return nil, err
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if job.discarded {
		return nil, ipc.NewError(ipc.CodeBatchNotFound, ipc.MsgBatchNotFound)
	}
	if job.state != ipc.BatchStateReady && job.state != ipc.BatchStateCancelled {
		return nil, batchWrongState(job.state)
	}
	if !job.choicesValidLocked(p.Choices) {
		return nil, badParams()
	}
	items := job.planLocked(p.Choices)
	job.state = ipc.BatchStateApplying
	job.total = len(items)
	job.done, job.failed = 0, 0
	job.imagesQueued, job.imagesSkip = 0, 0
	job.failedTitles = nil
	job.result = nil
	// A cancelled batch has its context cancelled: the application gets a new one, child of the agent's [MET-064].
	if job.ctx.Err() != nil {
		job.ctx, job.cancel = context.WithCancel(m.root)
	}
	ctx := job.ctx
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runApply(job, ctx, items)
	}()
	return ipc.BatchApplyResult{Accepted: len(p.Choices)}, nil
}
