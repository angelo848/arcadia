package agent

// Tests for T11 (docs/specs/phase-3-tracker/tasks.md): agent startup with
// session recovery and the tracker (AGT-009, SES-010), session.changed
// broadcast (IPC-027, IPC-024, SES-008) and shutdown with an open session
// (AGT-010, SES-011, AGT-003, AGT-011). The tracker runs with a fakeLauncher
// injected through config.Launcher, a FakeClock and a real store in
// t.TempDir(); clients are raw NDJSON connections over TCP.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// wiringT0 is the start of testClock(): every launch in this file happens here.
var wiringT0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

const msgRecovered = "sessões abertas recuperadas"

// --- fake launcher and job ---------------------------------------------------

// fakeLauncher creates fakeJobs. Its zero value is ready to use (testConfig
// hands one to every agent). It starts no goroutines.
type fakeLauncher struct {
	mu         sync.Mutex
	specs      []tracker.LaunchSpec
	jobs       []*fakeJob
	prepareErr error
	onJobClose func() // run on the first Close of every job, outside any lock

	detached    []tracker.LaunchSpec
	detachedErr error
}

// LaunchDetached records the spec and returns a fixed pid (or detachedErr).
func (l *fakeLauncher) LaunchDetached(spec tracker.LaunchSpec) (uint32, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.detached = append(l.detached, spec)
	if l.detachedErr != nil {
		return 0, l.detachedErr
	}
	return 4242, nil
}

func (l *fakeLauncher) Prepare(spec tracker.LaunchSpec) (tracker.Job, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.specs = append(l.specs, spec)
	if l.prepareErr != nil {
		return nil, l.prepareErr
	}
	j := &fakeJob{
		pid:     5000 + uint32(len(l.jobs)),
		events:  make(chan tracker.JobEvent, 16),
		active:  1,
		onClose: l.onJobClose,
	}
	l.jobs = append(l.jobs, j)
	return j, nil
}

func (l *fakeLauncher) prepareCalls() []tracker.LaunchSpec {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.specs)
}

func (l *fakeLauncher) job(t *testing.T, i int) *fakeJob {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if i >= len(l.jobs) {
		t.Fatalf("launcher created %d job(s), want index %d", len(l.jobs), i)
	}
	return l.jobs[i]
}

// fakeJob is a tracker.Job driven by the test.
type fakeJob struct {
	pid     uint32
	events  chan tracker.JobEvent
	onClose func()

	mu         sync.Mutex
	closed     bool
	active     int
	resumes    int
	terminates int
	closes     int
}

func (j *fakeJob) RootPID() uint32 { return j.pid }

func (j *fakeJob) Resume() error {
	j.mu.Lock()
	j.resumes++
	j.mu.Unlock()
	return nil
}

func (j *fakeJob) Terminate() error {
	j.mu.Lock()
	j.terminates++
	j.mu.Unlock()
	return nil
}

func (j *fakeJob) Events() <-chan tracker.JobEvent { return j.events }

func (j *fakeJob) ActiveProcesses() (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.active, nil
}

// Close is idempotent and closes the events channel once, like the real job.
func (j *fakeJob) Close() error {
	j.mu.Lock()
	first := !j.closed
	j.closes++
	if first {
		j.closed = true
		close(j.events)
	}
	hook := j.onClose
	j.mu.Unlock()
	if first && hook != nil {
		hook()
	}
	return nil
}

// counts returns how many times Resume, Terminate and Close were called.
func (j *fakeJob) counts() (resumes, terminates, closes int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.resumes, j.terminates, j.closes
}

// empty makes the job report no live process and sends ACTIVE_PROCESS_ZERO.
func (j *fakeJob) empty(t *testing.T) {
	t.Helper()
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		t.Fatal("empty on a closed job")
	}
	j.active = 0
	select {
	case j.events <- tracker.JobEvent{Kind: tracker.EventActiveZero}:
	default:
		t.Fatal("job events buffer full")
	}
}

// --- agent harness -------------------------------------------------------------

// closeHookListener runs onClose (once) when the IPC server closes it.
type closeHookListener struct {
	net.Listener
	once    sync.Once
	onClose func()
}

func (l *closeHookListener) Close() error {
	l.once.Do(func() {
		if l.onClose != nil {
			l.onClose()
		}
	})
	return l.Listener.Close()
}

type wiring struct {
	cfg   config
	clock *domain.FakeClock
	fl    *fakeLauncher
	a     *instance
	addr  string
}

type wiringOpts struct {
	mutate func(*config)                  // optional, after testConfig
	listen func() (net.Listener, error)   // optional; default TCP on 127.0.0.1:0
	before func(t *testing.T, cfg config) // optional, before startAgent (seeding)
}

// newWiring registers the goleak check, starts an agent with a FakeClock at
// wiringT0 and a fakeLauncher, and registers the cleanup (Shutdown, CloseLog).
func newWiring(t *testing.T, o wiringOpts) *wiring {
	t.Helper()
	verifyNoLeaks(t)
	cfg := testConfig(t, "")
	fl := &fakeLauncher{}
	cfg.Launcher = fl
	if o.mutate != nil {
		o.mutate(&cfg)
	}
	if o.before != nil {
		o.before(t, cfg)
	}
	w := &wiring{cfg: cfg, clock: testClock(), fl: fl}
	inner := o.listen
	if inner == nil {
		inner = (&tcpListen{}).listen
	}
	listen := func() (net.Listener, error) {
		l, err := inner()
		if err == nil {
			w.addr = l.Addr().String()
		}
		return l, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a, err := startAgent(ctx, cfg, w.clock, emptyEnv, listen)
	if err != nil {
		cancel()
		t.Fatalf("startAgent: %v", err)
	}
	w.a = a
	t.Cleanup(func() {
		sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
		defer scancel()
		if err := a.Shutdown(sctx, ReasonOther); err != nil {
			t.Errorf("cleanup Shutdown: %v", err)
		}
		if err := a.CloseLog(); err != nil {
			t.Errorf("cleanup CloseLog: %v", err)
		}
		cancel()
	})
	return w
}

// client dials the agent and proves the connection is registered.
func (w *wiring) client(t *testing.T) *rawClient {
	t.Helper()
	c := dialRaw(t, w.addr)
	c.rpcOK(t, ipc.MethodStatus, nil)
	return c
}

// addGame writes an .exe file and registers it through game.add.
func (w *wiring) addGame(t *testing.T, c *rawClient, title string) (int64, string) {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "Jogo "+title, "game.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatalf("write exe: %v", err)
	}
	var g ipc.Game
	resultInto(t, c.rpcOK(t, ipc.MethodGameAdd, map[string]string{"path": exe, "title": title}), &g)
	return g.ID, exe
}

// launch calls game.launch, requires success and returns the Session.
func (w *wiring) launch(t *testing.T, c *rawClient, gameID int64) ipc.Session {
	t.Helper()
	var s ipc.Session
	resultInto(t, c.rpcOK(t, ipc.MethodGameLaunch, map[string]int64{"gameId": gameID}), &s)
	return s
}

func (w *wiring) shutdown(t *testing.T) {
	t.Helper()
	sctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if err := w.a.Shutdown(sctx, ReasonQuit); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := w.a.CloseLog(); err != nil {
		t.Fatalf("CloseLog: %v", err)
	}
}

// --- raw client helpers -------------------------------------------------------

// snapshotAll returns a copy of every message received so far.
func (c *rawClient) snapshotAll() []rawMsg {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.all)
}

// waitAll waits (waking on every received message) until pred holds for the
// messages received so far and returns them.
func (c *rawClient) waitAll(t *testing.T, what string, pred func([]rawMsg) bool) []rawMsg {
	t.Helper()
	timeout := time.After(testTimeout)
	for {
		all := c.snapshotAll()
		if pred(all) {
			return all
		}
		select {
		case _, ok := <-c.msgs:
			if !ok {
				if all := c.snapshotAll(); pred(all) {
					return all
				}
				t.Fatalf("connection closed while waiting for %s", what)
			}
		case <-timeout:
			t.Fatalf("timeout waiting for %s; got %d message(s)", what, len(all))
		}
	}
}

// rpc sends a request with a fresh id and returns its response.
func (c *rawClient) rpc(t *testing.T, method string, params any) ipc.Response {
	t.Helper()
	c.ids++
	id := fmt.Sprint(c.ids)
	req := map[string]any{"jsonrpc": "2.0", "id": c.ids, "method": method}
	if params != nil {
		req["params"] = params
	}
	line, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(testTimeout)); err != nil {
		t.Fatalf("SetWriteDeadline: %v", err)
	}
	if _, err := c.conn.Write(append(line, '\n')); err != nil {
		t.Fatalf("write %s: %v", method, err)
	}
	var resp rawMsg
	c.waitAll(t, method+" response", func(all []rawMsg) bool {
		for _, m := range all {
			if string(m["id"]) == id && m.method() == "" {
				resp = m
				return true
			}
		}
		return false
	})
	b, err := json.Marshal(map[string]json.RawMessage(resp))
	if err != nil {
		t.Fatalf("re-marshal response: %v", err)
	}
	var out ipc.Response
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("decode response %s: %v", b, err)
	}
	return out
}

// rpcOK is rpc that fails the test on a JSON-RPC error.
func (c *rawClient) rpcOK(t *testing.T, method string, params any) ipc.Response {
	t.Helper()
	resp := c.rpc(t, method, params)
	if resp.Error != nil {
		t.Fatalf("%s: error %d %q (data %s)", method, resp.Error.Code, resp.Error.Message, resp.Error.Data)
	}
	return resp
}

// sessionChanges decodes every session.changed in msgs, checking the
// notification shape of IPC-027 (no id, params {session: Session}).
func sessionChanges(t *testing.T, msgs []rawMsg) []ipc.Session {
	t.Helper()
	var out []ipc.Session
	for _, m := range msgs {
		if m.method() != ipc.MethodSessionChanged {
			continue
		}
		if id, ok := m["id"]; ok {
			t.Errorf("session.changed has id %s, want a notification without id", id)
		}
		params := objectOf(t, m["params"])
		wantKeys(t, params, "session")
		wantKeys(t, objectOf(t, params["session"]), sessionKeys...)
		var p ipc.SessionChangedParams
		if err := json.Unmarshal(m["params"], &p); err != nil {
			t.Fatalf("session.changed params %s: %v", m["params"], err)
		}
		out = append(out, p.Session)
	}
	return out
}

// waitChanges waits for at least n session.changed and returns all of them.
func (c *rawClient) waitChanges(t *testing.T, n int) []ipc.Session {
	t.Helper()
	all := c.waitAll(t, fmt.Sprintf("%d session.changed", n), func(all []rawMsg) bool {
		return countMethod(all, ipc.MethodSessionChanged) >= n
	})
	return sessionChanges(t, all)
}

// flushedChanges does a round trip (everything queued before it has arrived)
// and returns every session.changed received.
func (c *rawClient) flushedChanges(t *testing.T) []ipc.Session {
	t.Helper()
	c.rpcOK(t, ipc.MethodStatus, nil)
	return sessionChanges(t, c.snapshotAll())
}

func states(ss []ipc.Session) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.State)
	}
	return out
}

// --- store helpers ------------------------------------------------------------

type sessionRow struct {
	gameID        int64
	state         string
	endedAt       sql.NullInt64
	lastSeenAt    int64
	activeSeconds int64
	endReason     sql.NullString
}

// openDB opens the agent database without migrating (the caller closes it).
func openDB(t *testing.T, cfg config) *store.Store {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	st, err := store.Open(ctx, dbPath(cfg), store.OpenOptions{Migrate: false})
	if err != nil {
		t.Fatalf("store.Open(%s): %v", dbPath(cfg), err)
	}
	return st
}

func readSessions(t *testing.T, st *store.Store) map[int64]sessionRow {
	t.Helper()
	rows, err := st.DB().QueryContext(context.Background(),
		`SELECT id, game_id, state, ended_at, last_seen_at, active_seconds, end_reason FROM play_session`)
	if err != nil {
		t.Fatalf("query play_session: %v", err)
	}
	defer rows.Close()
	out := map[int64]sessionRow{}
	for rows.Next() {
		var id int64
		var r sessionRow
		if err := rows.Scan(&id, &r.gameID, &r.state, &r.endedAt, &r.lastSeenAt, &r.activeSeconds, &r.endReason); err != nil {
			t.Fatalf("scan play_session: %v", err)
		}
		out[id] = r
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// sessionsAfterShutdown reopens the database of a stopped agent.
func sessionsAfterShutdown(t *testing.T, cfg config) (map[int64]sessionRow, []store.Game) {
	t.Helper()
	st := openDB(t, cfg)
	defer st.Close()
	games, err := st.Games(context.Background())
	if err != nil {
		t.Fatalf("Games: %v", err)
	}
	return readSessions(t, st), games
}

func wantEnded(t *testing.T, r sessionRow, state string, endedAt time.Time, active int64, reason domain.SessionEndReason) {
	t.Helper()
	if r.state != state {
		t.Errorf("state = %q, want %q", r.state, state)
	}
	if !r.endedAt.Valid {
		t.Errorf("ended_at is NULL, want %v", endedAt)
	} else if got := store.MillisToTime(r.endedAt.Int64); !got.Equal(endedAt) {
		t.Errorf("ended_at = %v, want %v", got, endedAt)
	}
	if r.activeSeconds != active {
		t.Errorf("active_seconds = %d, want %d", r.activeSeconds, active)
	}
	if !r.endReason.Valid || r.endReason.String != string(reason) {
		t.Errorf("end_reason = %v, want %q", r.endReason, reason)
	}
}

type seeded struct {
	gameID     int64
	long, shrt int64     // session ids: 590 s and 20 s
	longSeen   time.Time // last_seen_at of the 590 s session
	shortSeen  time.Time
}

// seedOpenSessions creates the database with a game and two open sessions
// left by a crash: 590 s (-> ended) and 20 s (-> discarded) [SES-010].
func seedOpenSessions(t *testing.T, cfg config) seeded {
	t.Helper()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	st, err := store.Open(ctx, dbPath(cfg), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("seed store.Open: %v", err)
	}
	defer st.Close()
	start := wiringT0.Add(-2 * time.Hour)
	g, err := st.AddGame(ctx, store.NewGame{Title: "Seed", Exe: `C:\Jogos\Seed\seed.exe`, WorkingDir: `C:\Jogos\Seed`, At: start})
	if err != nil {
		t.Fatalf("seed AddGame: %v", err)
	}
	s := seeded{gameID: g.ID, longSeen: start.Add(590 * time.Second), shortSeen: start.Add(time.Hour + 20*time.Second)}
	if s.long, err = st.StartSession(ctx, g.ID, start); err != nil {
		t.Fatalf("seed StartSession: %v", err)
	}
	if err := st.UpdateSession(ctx, s.long, domain.SessionRunning, s.longSeen, 590*time.Second); err != nil {
		t.Fatalf("seed UpdateSession: %v", err)
	}
	if s.shrt, err = st.StartSession(ctx, g.ID, start.Add(time.Hour)); err != nil {
		t.Fatalf("seed StartSession: %v", err)
	}
	if err := st.UpdateSession(ctx, s.shrt, domain.SessionGrace, s.shortSeen, 20*time.Second); err != nil {
		t.Fatalf("seed UpdateSession: %v", err)
	}
	return s
}

// trackerRunGoroutines counts the goroutines running the tracker owner loop.
//
// It counts across the whole test binary, so it is only meaningful while no
// other agent runs: callers must NOT be parallelized (no t.Parallel in the test
// or in its parent), which keeps them out of the parallel phase of the package,
// and every earlier wiring test ends with goleak clean (verifyNoLeaks), so no
// tracker goroutine of another test survives into the count. There is no
// deterministic seam instead: during listen the agent has not been returned yet
// and the tracker exposes no "started" signal.
func trackerRunGoroutines() int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	// Count the "created by" line, not the run frame: a goroutine that was
	// created but has not been scheduled yet does not always show the frame of
	// its function, while its creator is recorded as soon as the go statement
	// returns. Tracker.Start has a single go statement (the owner loop).
	return strings.Count(string(buf), "created by "+trackerStartFunc)
}

// trackerStartFunc is the creator of the tracker owner goroutine as printed by
// runtime.Stack.
const trackerStartFunc = "github.com/arcadia-launcher/arcadia/internal/tracker.(*Tracker).Start"

// errorLogsWith returns the ERROR records of agent.log whose error attribute
// contains sub.
func errorLogsWith(t *testing.T, cfg config, sub string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, rec := range logLines(t, agentLogPath(cfg)) {
		if rec["level"] != "ERROR" {
			continue
		}
		if e, ok := rec["error"].(string); ok && strings.Contains(e, sub) {
			out = append(out, rec)
		}
	}
	return out
}

// --- tests --------------------------------------------------------------------

// TestStartupRecovery must not call t.Parallel (see trackerRunGoroutines).
func TestStartupRecovery(t *testing.T) {
	t.Run("AGT-009/sessões abertas já estão fechadas quando listen é chamado", func(t *testing.T) {
		var (
			seed      seeded
			listenErr []string
			open      = -1
			rows      map[int64]sessionRow
		)
		var cfgSeen config
		w := newWiring(t, wiringOpts{
			before: func(t *testing.T, cfg config) { seed = seedOpenSessions(t, cfg); cfgSeen = cfg },
			listen: func() (net.Listener, error) {
				// A second connection to the database the agent already opened.
				st := openDB(t, cfgSeen)
				defer st.Close()
				sessions, err := st.OpenSessions(context.Background())
				if err != nil {
					listenErr = append(listenErr, err.Error())
				}
				open = len(sessions)
				rows = readSessions(t, st)
				return net.Listen("tcp", "127.0.0.1:0")
			},
		})
		if len(listenErr) > 0 {
			t.Fatalf("OpenSessions in listen: %v", listenErr)
		}
		if open != 0 {
			t.Errorf("open sessions when listen was called = %d, want 0", open)
		}
		wantEnded(t, rows[seed.long], string(domain.SessionEnded), seed.longSeen, 590, domain.EndRecovered)
		wantEnded(t, rows[seed.shrt], string(domain.SessionDiscarded), seed.shortSeen, 20, domain.EndRecovered)

		c := w.client(t)
		var list ipc.GameListResult
		resultInto(t, c.rpcOK(t, ipc.MethodGameList, nil), &list)
		if len(list.Games) != 1 || list.Games[0].TotalPlaySeconds != 590 || list.Games[0].PlayCount != 1 {
			t.Errorf("game.list = %+v, want one game with totalPlaySeconds 590 and playCount 1", list.Games)
		}

		w.shutdown(t)
		recs := findLog(t, w.cfg, "WARN", msgRecovered)
		if len(recs) != 1 {
			t.Fatalf("%d Warn %q records in agent.log, want 1", len(recs), msgRecovered)
		}
		if n, ok := recs[0]["count"].(float64); !ok || n != 2 {
			t.Errorf("count = %v, want 2", recs[0]["count"])
		}
	})

	t.Run("AGT-009/o tracker é iniciado antes do Serve", func(t *testing.T) {
		// Não paralelizar: do not add t.Parallel here or in TestStartupRecovery;
		// trackerRunGoroutines counts goroutines of the whole test binary.
		var (
			mu          sync.Mutex
			order       []string
			atRecover   = -1
			atListen    = -1
			recoverClk  domain.Clock
			recoverNilS bool
		)
		base := trackerRunGoroutines()
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Recover = func(ctx context.Context, st tracker.RecoveryStore, clock domain.Clock, log *slog.Logger) (int, error) {
					mu.Lock()
					order = append(order, "recover")
					atRecover = trackerRunGoroutines()
					recoverClk = clock
					recoverNilS = st == nil
					mu.Unlock()
					return tracker.RecoverOpenSessions(ctx, st, clock, log)
				}
			},
			listen: func() (net.Listener, error) {
				mu.Lock()
				order = append(order, "listen")
				atListen = trackerRunGoroutines()
				mu.Unlock()
				return net.Listen("tcp", "127.0.0.1:0")
			},
		})
		mu.Lock()
		gotOrder, rec, lis, clk, nilStore := slices.Clone(order), atRecover, atListen, recoverClk, recoverNilS
		mu.Unlock()
		if strings.Join(gotOrder, ",") != "recover,listen" {
			t.Errorf("startup order = %v, want [recover listen]", gotOrder)
		}
		if rec != base {
			t.Errorf("tracker goroutines during Recover = %d, want %d (tracker starts after the recovery)", rec, base)
		}
		if lis != base+1 {
			t.Errorf("tracker goroutines when listen is called = %d, want %d (tracker started before Serve)", lis, base+1)
		}
		if clk != domain.Clock(w.clock) {
			t.Errorf("Recover got clock %v, want the agent clock", clk)
		}
		if nilStore {
			t.Error("Recover got a nil store")
		}

		// The first request on the first connection already reaches a running tracker.
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Primeiro")
		if s := w.launch(t, c, gameID); s.State != string(domain.SessionRunning) {
			t.Errorf("game.launch state = %q, want running", s.State)
		}
	})

	t.Run("AGT-009/Recover que falha loga Error e a subida continua", func(t *testing.T) {
		const boom = "disco indisponível (teste)"
		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Recover = func(context.Context, tracker.RecoveryStore, domain.Clock, *slog.Logger) (int, error) {
					return 0, errors.New(boom)
				}
			},
		})
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Depois")
		if s := w.launch(t, c, gameID); s.State != string(domain.SessionRunning) {
			t.Errorf("game.launch state = %q, want running", s.State)
		}
		w.shutdown(t)
		if recs := errorLogsWith(t, w.cfg, boom); len(recs) != 1 {
			t.Errorf("%d ERROR records with error %q, want 1", len(recs), boom)
		}
	})

	t.Run("SES-010/recuperação na subida não gera notificação", func(t *testing.T) {
		var seed seeded
		w := newWiring(t, wiringOpts{before: func(t *testing.T, cfg config) { seed = seedOpenSessions(t, cfg) }})
		c := w.client(t)
		// Let time pass on the agent (a heartbeat goes out) and do round trips.
		// Without a session the only clock waiter is the server heartbeat.
		bctx, bcancel := context.WithTimeout(context.Background(), testTimeout)
		defer bcancel()
		if err := w.clock.BlockUntilWaiters(bctx, 1); err != nil {
			t.Fatalf("BlockUntilWaiters: %v", err)
		}
		w.clock.Advance(ipc.HeartbeatInterval)
		c.waitAll(t, "agent.heartbeat", func(all []rawMsg) bool { return countMethod(all, ipc.MethodHeartbeat) > 0 })
		resultInto(t, c.rpcOK(t, ipc.MethodSessionCurrent, nil), &ipc.SessionCurrentResult{})
		if got := c.flushedChanges(t); len(got) != 0 {
			t.Errorf("session.changed received after recovery: %v, want none", states(got))
		}
		w.shutdown(t)
		rows, _ := sessionsAfterShutdown(t, w.cfg)
		wantEnded(t, rows[seed.long], string(domain.SessionEnded), seed.longSeen, 590, domain.EndRecovered)
		wantEnded(t, rows[seed.shrt], string(domain.SessionDiscarded), seed.shortSeen, 20, domain.EndRecovered)
	})

	t.Run("AGT-003/falha no listen depois do Start fecha o tracker, o store e o log", func(t *testing.T) {
		verifyNoLeaks(t)
		cfg := testConfig(t, "")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		listenErr := errors.New("listen failed (test)")
		a, err := startAgent(ctx, cfg, testClock(), emptyEnv, func() (net.Listener, error) { return nil, listenErr })
		if err == nil {
			sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
			defer scancel()
			if a != nil {
				_ = a.Shutdown(sctx, ReasonOther)
				_ = a.CloseLog()
			}
			t.Fatal("startAgent with a failing listen: want error, got nil")
		}
		if !errors.Is(err, listenErr) {
			t.Errorf("startAgent error = %v, want it to wrap %v", err, listenErr)
		}
		if a != nil {
			t.Errorf("startAgent returned non-nil agent with error %v", err)
		}
		assertStoreClosed(t, cfg)
		if err := os.Remove(agentLogPath(cfg)); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("remove agent.log after failed startAgent: %v (log still open)", err)
		}
		// goleak (verifyNoLeaks) checks that the tracker goroutine has ended.
	})
}

func TestSessionChangedBroadcast(t *testing.T) {
	t.Run("IPC-027/dois clientes recebem running, grace e ended, na ordem e sem id", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c1, c2 := w.client(t), w.client(t)
		gameID, _ := w.addGame(t, c1, "Hollow Knight")
		launched := w.launch(t, c1, gameID)

		for _, c := range []*rawClient{c1, c2} {
			if got := c.waitChanges(t, 1); got[0].State != string(domain.SessionRunning) {
				t.Fatalf("first session.changed state = %q, want running", got[0].State)
			}
		}
		// The timers of Running were armed before the running notification.
		w.clock.Advance(120 * time.Second)
		w.fl.job(t, 0).empty(t)
		for _, c := range []*rawClient{c1, c2} {
			c.waitChanges(t, 2)
		}
		// Grace was armed before the grace notification.
		w.clock.Advance(domain.SessionGracePeriod)

		for i, c := range []*rawClient{c1, c2} {
			c.waitChanges(t, 3)
			got := c.flushedChanges(t)
			if strings.Join(states(got), ",") != "running,grace,ended" {
				t.Fatalf("client %d: session.changed states = %v, want [running grace ended]", i+1, states(got))
			}
			for _, s := range got {
				if s.SessionID != launched.SessionID || s.GameID != gameID || s.Title != "Hollow Knight" {
					t.Errorf("client %d: session %+v, want id %d, game %d, title Hollow Knight", i+1, s, launched.SessionID, gameID)
				}
				if !s.StartedAt.Equal(wiringT0) {
					t.Errorf("client %d: startedAt = %v, want %v", i+1, s.StartedAt, wiringT0)
				}
			}
			run, grace, end := got[0], got[1], got[2]
			if run.ActiveSeconds != 0 || run.EndedAt != nil || !run.AsOf.Equal(wiringT0) {
				t.Errorf("client %d: running = %+v, want activeSeconds 0, endedAt null, asOf T", i+1, run)
			}
			if grace.ActiveSeconds != 120 || grace.EndedAt != nil || !grace.AsOf.Equal(wiringT0.Add(120*time.Second)) {
				t.Errorf("client %d: grace = %+v, want activeSeconds 120, endedAt null, asOf T+120s", i+1, grace)
			}
			if end.ActiveSeconds != 120 || end.EndedAt == nil || !end.EndedAt.Equal(wiringT0.Add(120*time.Second)) {
				t.Errorf("client %d: ended = %+v, want activeSeconds 120 and endedAt T+120s", i+1, end)
			}
			if !end.AsOf.Equal(wiringT0.Add(120*time.Second + domain.SessionGracePeriod)) {
				t.Errorf("client %d: ended asOf = %v, want the grace expiry T+135s", i+1, end.AsOf.Time)
			}
		}
	})

	t.Run("IPC-027/discarded é notificado", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c1, c2 := w.client(t), w.client(t)
		gameID, _ := w.addGame(t, c1, "Curto")
		w.launch(t, c1, gameID)
		c1.waitChanges(t, 1)
		w.fl.job(t, 0).empty(t)
		c1.waitChanges(t, 2)
		w.clock.Advance(domain.SessionGracePeriod)
		for i, c := range []*rawClient{c1, c2} {
			c.waitChanges(t, 3)
			got := c.flushedChanges(t)
			if strings.Join(states(got), ",") != "running,grace,discarded" {
				t.Fatalf("client %d: session.changed states = %v, want [running grace discarded]", i+1, states(got))
			}
			if d := got[2]; d.ActiveSeconds != 0 || d.EndedAt == nil || !d.EndedAt.Equal(wiringT0) {
				t.Errorf("client %d: discarded = %+v, want activeSeconds 0 and endedAt T", i+1, d)
			}
		}
	})

	t.Run("IPC-027/lançamento que falhou não notifica", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		w.fl.prepareErr = &tracker.LaunchError{Err: errors.New("Acesso negado."), WinError: 5}
		c1, c2 := w.client(t), w.client(t)
		gameID, _ := w.addGame(t, c1, "Falha")
		resp := c1.rpc(t, ipc.MethodGameLaunch, map[string]int64{"gameId": gameID})
		data := wantError(t, resp, ipc.CodeLaunchFailed, ipc.MsgLaunchFailed)
		wantRaw(t, data, "winError", "5")
		for i, c := range []*rawClient{c1, c2} {
			if got := c.flushedChanges(t); len(got) != 0 {
				t.Errorf("client %d: session.changed after a failed launch: %v, want none", i+1, states(got))
			}
		}
		wantRaw(t, objectOf(t, c1.rpcOK(t, ipc.MethodSessionCurrent, nil).Result), "session", "null")
	})

	t.Run("IPC-024/game.launch fim a fim devolve a Session running", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c := w.client(t)
		gameID, exe := w.addGame(t, c, "Celeste")
		resp := c.rpcOK(t, ipc.MethodGameLaunch, map[string]int64{"gameId": gameID})
		obj := objectOf(t, resp.Result)
		wantKeys(t, obj, sessionKeys...)
		wantRaw(t, obj, "state", `"running"`)
		wantRaw(t, obj, "endedAt", "null")
		wantRaw(t, obj, "activeSeconds", "0")
		wantRaw(t, obj, "activeMs", "0")
		wantRaw(t, obj, "gameId", fmt.Sprint(gameID))
		wantRaw(t, obj, "title", `"Celeste"`)
		wantRaw(t, obj, "startedAt", `"2026-11-08T12:00:00Z"`)
		wantRaw(t, obj, "asOf", `"2026-11-08T12:00:00.000Z"`)
		var s ipc.Session
		resultInto(t, resp, &s)
		if s.SessionID < 1 {
			t.Errorf("sessionId = %d, want >= 1", s.SessionID)
		}

		specs := w.fl.prepareCalls()
		if len(specs) != 1 {
			t.Fatalf("Prepare calls = %d, want 1", len(specs))
		}
		want := tracker.LaunchSpec{GameID: gameID, Title: "Celeste", Exe: exe, WorkDir: filepath.Dir(exe)}
		if specs[0] != want {
			t.Errorf("LaunchSpec = %+v, want %+v", specs[0], want)
		}
		if r, _, _ := w.fl.job(t, 0).counts(); r != 1 {
			t.Errorf("Resume calls = %d, want 1", r)
		}

		var cur ipc.SessionCurrentResult
		resultInto(t, c.rpcOK(t, ipc.MethodSessionCurrent, nil), &cur)
		if cur.Session == nil || cur.Session.SessionID != s.SessionID || cur.Session.State != string(domain.SessionRunning) {
			t.Errorf("session.current = %+v, want the running session %d", cur.Session, s.SessionID)
		}

		// The open session is in the database.
		st := openDB(t, w.cfg)
		rows := readSessions(t, st)
		_ = st.Close()
		if r, ok := rows[s.SessionID]; !ok || r.endedAt.Valid || r.state != string(domain.SessionRunning) || r.gameID != gameID {
			t.Errorf("play_session %d = %+v (found %v), want open, running, game %d", s.SessionID, r, ok, gameID)
		}
	})
}

func TestSessionTotals(t *testing.T) {
	t.Run("SES-008/sessão de 120 s aparece em game.list com totalPlaySeconds 120, playCount 1 e lastPlayedAt = ended_at", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Total")
		var before ipc.GameListResult
		resultInto(t, c.rpcOK(t, ipc.MethodGameList, nil), &before)
		if len(before.Games) != 1 || before.Games[0].TotalPlaySeconds != 0 {
			t.Fatalf("game.list before = %+v, want one game with total 0", before.Games)
		}

		w.launch(t, c, gameID)
		c.waitChanges(t, 1)
		w.clock.Advance(120 * time.Second)
		w.fl.job(t, 0).empty(t)
		c.waitChanges(t, 2)
		w.clock.Advance(domain.SessionGracePeriod)
		got := c.waitChanges(t, 3)
		if got[2].State != string(domain.SessionEnded) || got[2].EndedAt == nil {
			t.Fatalf("third session.changed = %+v, want ended with endedAt", got[2])
		}
		endedAt := *got[2].EndedAt

		var list ipc.GameListResult
		resultInto(t, c.rpcOK(t, ipc.MethodGameList, nil), &list)
		if len(list.Games) != 1 {
			t.Fatalf("game.list has %d games, want 1", len(list.Games))
		}
		g := list.Games[0]
		if g.TotalPlaySeconds != 120 || g.PlayCount != 1 {
			t.Errorf("game = %+v, want totalPlaySeconds 120 and playCount 1", g)
		}
		if g.LastPlayedAt == nil || !g.LastPlayedAt.Equal(endedAt) || !endedAt.Equal(wiringT0.Add(120*time.Second)) {
			t.Errorf("lastPlayedAt = %v, want ended_at %v (T+120s)", g.LastPlayedAt, endedAt)
		}
	})
}

func TestShutdownWithSession(t *testing.T) {
	t.Run("AGT-010/ordem Drain, Server.Close, Tracker.Close e Store.Close", func(t *testing.T) {
		// The order is verified without timing assumptions: the drain limit is
		// one hour, so Drain can only finish because the client closed its
		// connection, and the client closes only after it received
		// agent.shutdown. The Close hooks record whether that close had already
		// happened (Drain completed) when each step ran.
		var (
			mu    sync.Mutex
			order []string
			// the client already closed its connection (after agent.shutdown)
			clientClosed                                bool
			drainedAtServerClose, drainedAtTrackerClose bool
		)
		w := newWiring(t, wiringOpts{
			listen: func() (net.Listener, error) {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					return nil, err
				}
				return &closeHookListener{Listener: ln, onClose: func() {
					mu.Lock()
					defer mu.Unlock()
					order = append(order, "server.close")
					drainedAtServerClose = clientClosed
				}}, nil
			},
		})
		w.fl.onJobClose = func() {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, "tracker.close")
			drainedAtTrackerClose = clientClosed
		}
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Ordem")
		s := w.launch(t, c, gameID)
		c.waitChanges(t, 1)
		w.clock.Advance(45 * time.Second)

		w.a.drainTimeout = time.Hour // only the client closing ends the drain
		errc := shutdownAsync(w.a, context.Background(), ReasonQuit)
		c.waitAll(t, "agent.shutdown", func(all []rawMsg) bool { return countMethod(all, ipc.MethodShutdown) == 1 })
		assertShutdownPending(t, errc)
		mu.Lock()
		early := slices.Clone(order)
		clientClosed = true
		mu.Unlock()
		if len(early) != 0 {
			t.Errorf("steps run while Drain waited for the client: %v, want none", early)
		}
		_ = c.conn.Close()
		if err := waitShutdown(t, errc); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
		if err := w.a.CloseLog(); err != nil {
			t.Fatalf("CloseLog: %v", err)
		}

		msgs := c.waitEOF(t)
		mu.Lock()
		gotOrder := slices.Clone(order)
		atServer, atTracker := drainedAtServerClose, drainedAtTrackerClose
		mu.Unlock()
		if strings.Join(gotOrder, ",") != "server.close,tracker.close" {
			t.Errorf("shutdown order = %v, want [server.close tracker.close]", gotOrder)
		}
		if !atServer || !atTracker {
			t.Errorf("Drain completed before Server.Close = %v, before Tracker.Close = %v; want both true", atServer, atTracker)
		}
		// Server side: the drain ended because no connection was left, not by
		// its limit (which would log the Warn).
		if recs := findLog(t, w.cfg, "WARN", msgDrainTimeout); len(recs) != 0 {
			t.Errorf("%d Warn %q records, want 0 (Drain completed)", len(recs), msgDrainTimeout)
		}
		shutdownAt := -1
		for i, m := range msgs {
			if m.method() == ipc.MethodShutdown {
				shutdownAt = i
			}
		}
		if shutdownAt < 0 {
			t.Fatal("client did not receive agent.shutdown")
		}
		for _, m := range msgs[shutdownAt:] {
			if m.method() == ipc.MethodSessionChanged {
				t.Errorf("session.changed after agent.shutdown: %s", m["params"])
			}
		}
		if n := countMethod(msgs, ipc.MethodSessionChanged); n != 1 {
			t.Errorf("client received %d session.changed, want 1 (running)", n)
		}

		// Store.Close came after the tracker wrote the end of the session.
		rows, _ := sessionsAfterShutdown(t, w.cfg)
		wantEnded(t, rows[s.SessionID], string(domain.SessionEnded), wiringT0.Add(45*time.Second), 45, domain.EndAgentShutdown)
	})

	t.Run("SES-011/sessão Running encerra com agent-shutdown e o tempo até o shutdown", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Running")
		s := w.launch(t, c, gameID)
		c.waitChanges(t, 1)
		w.clock.Advance(100 * time.Second)
		w.a.drainTimeout = 50 * time.Millisecond
		w.shutdown(t)

		rows, games := sessionsAfterShutdown(t, w.cfg)
		wantEnded(t, rows[s.SessionID], string(domain.SessionEnded), wiringT0.Add(100*time.Second), 100, domain.EndAgentShutdown)
		if len(games) != 1 || games[0].TotalPlaySeconds != 100 || games[0].PlayCount != 1 {
			t.Errorf("games = %+v, want total 100 and playCount 1", games)
		}
	})

	t.Run("SES-011/sessão em Grace encerra com exited e ended_at do início da carência", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Grace")
		s := w.launch(t, c, gameID)
		c.waitChanges(t, 1)
		w.clock.Advance(40 * time.Second)
		w.fl.job(t, 0).empty(t)
		c.waitChanges(t, 2)
		w.clock.Advance(5 * time.Second) // still within the grace period
		w.a.drainTimeout = 50 * time.Millisecond
		w.shutdown(t)

		rows, _ := sessionsAfterShutdown(t, w.cfg)
		wantEnded(t, rows[s.SessionID], string(domain.SessionEnded), wiringT0.Add(40*time.Second), 40, domain.EndExited)
	})

	t.Run("SES-011/Job.Terminate não é chamado no shutdown", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Vivo")
		w.launch(t, c, gameID)
		c.waitChanges(t, 1)
		w.a.drainTimeout = 50 * time.Millisecond
		w.shutdown(t)
		resumes, terminates, closes := w.fl.job(t, 0).counts()
		if terminates != 0 {
			t.Errorf("Job.Terminate calls = %d, want 0", terminates)
		}
		if closes < 1 {
			t.Errorf("Job.Close calls = %d, want >= 1", closes)
		}
		if resumes != 1 {
			t.Errorf("Job.Resume calls = %d, want 1", resumes)
		}
	})

	t.Run("IPC-027/encerramento no shutdown não notifica", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c1, c2 := w.client(t), w.client(t)
		gameID, _ := w.addGame(t, c1, "Silêncio")
		w.launch(t, c1, gameID)
		c1.waitChanges(t, 1)
		c2.waitChanges(t, 1)
		w.clock.Advance(60 * time.Second)
		w.a.drainTimeout = 50 * time.Millisecond
		w.shutdown(t)
		for i, c := range []*rawClient{c1, c2} {
			msgs := c.waitEOF(t)
			if got := sessionChanges(t, msgs); strings.Join(states(got), ",") != "running" {
				t.Errorf("client %d: session.changed states = %v, want only [running]", i+1, states(got))
			}
		}
	})

	t.Run("AGT-003/Shutdown com sessão termina em < 2 s", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c := w.client(t) // never closes: the drain uses its whole default limit
		gameID, _ := w.addGame(t, c, "Rápido")
		w.launch(t, c, gameID)
		c.waitChanges(t, 1)
		w.clock.Advance(30 * time.Second)

		start := time.Now()
		errc := shutdownAsync(w.a, context.Background(), ReasonSessionEnd)
		if err := waitShutdown(t, errc); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
		if took := time.Since(start); took >= 2*time.Second {
			t.Errorf("Shutdown with an open session took %v, want < 2s", took)
		}
		if err := w.a.CloseLog(); err != nil {
			t.Fatalf("CloseLog: %v", err)
		}
		assertStoreClosed(t, w.cfg)
	})
}

func TestLaunchDuringShutdown(t *testing.T) {
	t.Run("AGT-011/game.launch durante o drain devolve -32003 agente encerrando sem chamar Prepare", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Tarde")
		w.a.drainTimeout = time.Hour // the client closing ends the drain

		errc := shutdownAsync(w.a, context.Background(), ReasonQuit)
		c.waitAll(t, "agent.shutdown", func(all []rawMsg) bool { return countMethod(all, ipc.MethodShutdown) == 1 })
		assertShutdownPending(t, errc)

		resp := c.rpc(t, ipc.MethodGameLaunch, map[string]int64{"gameId": gameID})
		data := wantError(t, resp, ipc.CodeLaunchFailed, ipc.MsgLaunchFailed)
		wantKeys(t, data, "error")
		wantRaw(t, data, "error", `"agente encerrando"`)
		if n := len(w.fl.prepareCalls()); n != 0 {
			t.Errorf("Prepare calls = %d, want 0", n)
		}

		_ = c.conn.Close()
		if err := waitShutdown(t, errc); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
		if err := w.a.CloseLog(); err != nil {
			t.Errorf("CloseLog: %v", err)
		}
		rows, _ := sessionsAfterShutdown(t, w.cfg)
		if len(rows) != 0 {
			t.Errorf("play_session has %d row(s), want 0", len(rows))
		}
	})
}

func TestSessionNotifier(t *testing.T) {
	t.Run("IPC-027/notifier sem servidor não faz nada", func(t *testing.T) {
		var n sessionNotifier
		n.Notify(domain.SessionSnapshot{ID: 1, GameID: 1, Title: "x", State: domain.SessionRunning, StartedAt: wiringT0, AsOf: wiringT0})
	})
}
