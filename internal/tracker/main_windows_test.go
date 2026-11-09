//go:build windows

package tracker_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// fakeGameExe is cmd/fake-game compiled once by TestMain into a temporary
// directory whose path contains spaces (TRK-005).
var fakeGameExe string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "arcadia tracker test ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdir temp: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	goBin, err := exec.LookPath("go")
	if err != nil {
		goBin = filepath.Join(runtime.GOROOT(), "bin", "go.exe")
	}
	fakeGameExe = filepath.Join(dir, "fake game.exe")
	build := exec.Command(goBin, "build", "-o", fakeGameExe, "github.com/arcadia-launcher/arcadia/cmd/fake-game")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "go build fake-game: %v\n%s\n", err, out)
		return 1
	}
	return m.Run()
}

const (
	realWait  = 10 * time.Second // bounded real wait for process and job events
	pollEvery = 20 * time.Millisecond
	maxLife   = "30s"
	stillLive = 259 // STILL_ACTIVE
)

// --- Win32 helpers missing from x/sys/windows ------------------------------

var (
	modKernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procIsProcessInJob        = modKernel32.NewProc("IsProcessInJob")
	procGetProcessHandleCount = modKernel32.NewProc("GetProcessHandleCount")
)

// inAnyJob reports IsProcessInJob(h, NULL).
func inAnyJob(t *testing.T, h windows.Handle) bool {
	t.Helper()
	var res int32
	r, _, err := procIsProcessInJob.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&res)))
	if r == 0 {
		t.Fatalf("IsProcessInJob: %v", err)
	}
	return res != 0
}

// processHandleCount returns GetProcessHandleCount(GetCurrentProcess()).
func processHandleCount(t *testing.T) uint32 {
	t.Helper()
	var n uint32
	r, _, err := procGetProcessHandleCount.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		t.Fatalf("GetProcessHandleCount: %v", err)
	}
	return n
}

// testerInJob reports whether the test process itself runs inside a job
// (terminal/IDE). In that case IsProcessInJob(h, NULL) is true for every child
// and only the job-specific checks (events, ActiveProcesses) are meaningful.
func testerInJob(t *testing.T) bool {
	t.Helper()
	return inAnyJob(t, windows.CurrentProcess())
}

// openProcess opens a waitable, queryable handle to pid, closed in t.Cleanup.
func openProcess(t *testing.T, pid int) windows.Handle {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		t.Fatalf("OpenProcess(%d): %v", pid, err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(h) })
	return h
}

func exitCodeOf(t *testing.T, h windows.Handle) uint32 {
	t.Helper()
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		t.Fatalf("GetExitCodeProcess: %v", err)
	}
	return code
}

// waitProcessExit waits (bounded) for h to be signalled.
func waitProcessExit(t *testing.T, h windows.Handle, what string) {
	t.Helper()
	ev, err := windows.WaitForSingleObject(h, uint32(realWait/time.Millisecond))
	if ev != windows.WAIT_OBJECT_0 {
		t.Fatalf("%s did not exit within %v (wait = %#x, %v)", what, realWait, ev, err)
	}
}

// isRunning reports whether h has not exited yet (no wait).
func isRunning(t *testing.T, h windows.Handle) bool {
	t.Helper()
	ev, _ := windows.WaitForSingleObject(h, 0)
	return ev == uint32(windows.WAIT_TIMEOUT)
}

// --- fake-game report -------------------------------------------------------

type reportLine struct {
	Event string   `json:"event"`
	PID   int      `json:"pid"`
	PPID  int      `json:"ppid,omitempty"`
	Depth int      `json:"depth"`
	Cwd   string   `json:"cwd,omitempty"`
	Args  []string `json:"args,omitempty"`
	Code  *int     `json:"code,omitempty"`
	Error string   `json:"error,omitempty"`
}

// scenario owns a temp dir with a report and an until-file. Every resumed
// launch registers the size of its process tree (expect); the cleanup creates
// the until-file, waits until all expected processes wrote their start and exit
// lines, and then waits for each of them to be signalled, so no fake-game
// outlives the test and t.TempDir removal never hits an open report.
type scenario struct {
	dir    string
	report string
	until  string

	mu       sync.Mutex
	expected int
}

// expect registers a resumed launch of fake-game with argv.
func (s *scenario) expect(argv []string) {
	s.mu.Lock()
	s.expected += treeSize(argv)
	s.mu.Unlock()
}

// treeSize is the number of processes fake-game creates for argv:
// 1 + k + k² + … + k^depth.
func treeSize(argv []string) int {
	depth, fanout := 0, 1
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--" {
			break
		}
		switch argv[i] {
		case "--depth":
			depth, _ = strconv.Atoi(argv[i+1])
		case "--fanout":
			fanout, _ = strconv.Atoi(argv[i+1])
		}
	}
	n, level := 0, 1
	for d := 0; d <= depth; d++ {
		n += level
		level *= fanout
	}
	return n
}

func newScenario(t *testing.T) *scenario {
	t.Helper()
	dir := t.TempDir()
	s := &scenario{
		dir:    dir,
		report: filepath.Join(dir, "report.jsonl"),
		until:  filepath.Join(dir, "stop"),
	}
	t.Cleanup(func() { s.stop(t) })
	return s
}

// argv returns flags plus the safety flags of every test (--until-file,
// --max-life, --report). Positional arguments after "--" in flags stay last.
func (s *scenario) argv(flags ...string) []string {
	out := []string{"--until-file", s.until, "--max-life", maxLife, "--report", s.report}
	return append(out, flags...)
}

// joinArgs builds a Windows command-line tail from argv.
func joinArgs(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = windows.EscapeArg(a)
	}
	return strings.Join(parts, " ")
}

func (s *scenario) createUntil(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(s.until, nil, 0o644); err != nil {
		t.Fatalf("create until-file: %v", err)
	}
}

func (s *scenario) stop(t *testing.T) {
	_ = os.WriteFile(s.until, nil, 0o644)
	s.mu.Lock()
	expected := s.expected
	s.mu.Unlock()
	deadline := time.Now().Add(realWait)
	for {
		lines, _ := readReport(s.report)
		starts, exits := countEvents(lines)
		if starts >= expected && exits >= starts {
			s.waitSignalled(t, lines, deadline)
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("cleanup: %d start lines (expected %d) and %d exit lines after %v", starts, expected, exits, realWait)
			s.waitSignalled(t, lines, deadline)
			return
		}
		<-time.After(pollEvery)
	}
}

// waitSignalled waits (until deadline) for every process that wrote a start
// line to be gone, so its report handle is closed. A PID that can no longer be
// opened has already exited.
func (s *scenario) waitSignalled(t *testing.T, lines []reportLine, deadline time.Time) {
	for _, l := range linesOf(lines, "start", -1) {
		h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(l.PID))
		if err != nil {
			continue
		}
		left := max(time.Until(deadline), 0)
		ev, _ := windows.WaitForSingleObject(h, uint32(left/time.Millisecond))
		_ = windows.CloseHandle(h)
		if ev != windows.WAIT_OBJECT_0 {
			t.Errorf("cleanup: fake-game pid %d still running", l.PID)
		}
	}
}

// readReport parses the complete lines of the report; a missing file is no lines.
func readReport(path string) ([]reportLine, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	i := bytes.LastIndexByte(data, '\n')
	if i < 0 {
		return nil, nil
	}
	data = data[:i+1]
	var lines []reportLine
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		var l reportLine
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("report line %q: %w", raw, err)
		}
		lines = append(lines, l)
	}
	return lines, sc.Err()
}

func countEvents(lines []reportLine) (starts, exits int) {
	for _, l := range lines {
		switch l.Event {
		case "start":
			starts++
		case "exit":
			exits++
		}
	}
	return starts, exits
}

// linesOf returns the lines with the given event and depth (depth < 0 = any).
func linesOf(lines []reportLine, event string, depth int) []reportLine {
	var out []reportLine
	for _, l := range lines {
		if l.Event == event && (depth < 0 || l.Depth == depth) {
			out = append(out, l)
		}
	}
	return out
}

// waitReport polls the report until cond holds or realWait passes.
func (s *scenario) waitReport(t *testing.T, what string, cond func([]reportLine) bool) []reportLine {
	t.Helper()
	deadline := time.After(realWait)
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		lines, err := readReport(s.report)
		if err != nil {
			t.Fatalf("read report: %v", err)
		}
		if cond(lines) {
			return lines
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %s; report: %+v", what, lines)
		case <-tick.C:
		}
	}
}

// waitCounts waits until the report has at least starts start lines and exits
// exit lines.
func (s *scenario) waitCounts(t *testing.T, starts, exits int) []reportLine {
	t.Helper()
	return s.waitReport(t, fmt.Sprintf("%d start and %d exit lines", starts, exits), func(ls []reportLine) bool {
		st, ex := countEvents(ls)
		return st >= starts && ex >= exits
	})
}

// --- tracker with the real launcher -----------------------------------------

// winEnv is a Tracker with the real Windows launcher, a FakeClock for grace,
// heartbeat and periodic check, and a real store in t.TempDir().
type winEnv struct {
	clock *domain.FakeClock
	rec   *recorder
	logs  *logStore
	st    *store.Store
	game  store.Game
	tr    *tracker.Tracker
	seen  int
}

func newWinEnv(t *testing.T) *winEnv {
	t.Helper()
	st := openTestStore(t)
	w := &winEnv{
		clock: domain.NewFakeClock(baseT),
		rec:   newRecorder(),
		logs:  newLogStore(),
		st:    st,
		game:  addTestGame(t, st),
	}
	w.tr = tracker.New(tracker.Config{
		Clock:    w.clock,
		Store:    st,
		Launcher: tracker.NewLauncher(),
		Notify:   func(s domain.SessionSnapshot) { w.rec.add(call{op: opNotify, snap: s}) },
		Log:      w.logs.logger(),
		// Heartbeat and periodic check stay out of the way; the job messages
		// drive the transitions (waiter counts do not change).
		Heartbeat: time.Hour,
		Poll:      time.Hour,
	})
	w.tr.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), realWait)
		defer cancel()
		if err := w.tr.Close(ctx); err != nil {
			t.Errorf("cleanup Tracker.Close: %v", err)
		}
	})
	return w
}

func realCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), realWait)
	t.Cleanup(cancel)
	return ctx
}

func (w *winEnv) spec(argv []string, workDir string) tracker.LaunchSpec {
	return tracker.LaunchSpec{
		GameID:  w.game.ID,
		Title:   w.game.Title,
		Exe:     fakeGameExe,
		Args:    joinArgs(argv),
		WorkDir: workDir,
	}
}

// launch launches fake-game with sc.argv(flags...) in sc.dir, requires success
// and consumes the running notification.
func (w *winEnv) launch(t *testing.T, sc *scenario, flags ...string) domain.SessionSnapshot {
	t.Helper()
	return w.launchArgv(t, sc, sc.argv(flags...), sc.dir)
}

// launchArgv launches fake-game with argv in workDir and registers the tree in sc.
func (w *winEnv) launchArgv(t *testing.T, sc *scenario, argv []string, workDir string) domain.SessionSnapshot {
	t.Helper()
	spec := w.spec(argv, workDir)
	snap, err := w.tr.Launch(realCtx(t), spec)
	if err != nil {
		t.Fatalf("Launch(%+v): %v", spec, err)
	}
	sc.expect(argv)
	if snap.State != domain.SessionRunning {
		t.Fatalf("Launch state = %q, want running", snap.State)
	}
	if n := w.nextNotify(t); n.State != domain.SessionRunning {
		t.Fatalf("first notification state = %q, want running", n.State)
	}
	return snap
}

// tryNextNotify returns the next unconsumed notification, waiting up to d.
func (w *winEnv) tryNextNotify(d time.Duration) (domain.SessionSnapshot, bool) {
	timeout := time.After(d)
	for {
		calls, ch := w.rec.state()
		ns := filterOps(calls, opNotify)
		if len(ns) > w.seen {
			s := ns[w.seen].snap
			w.seen++
			return s, true
		}
		select {
		case <-ch:
		case <-timeout:
			return domain.SessionSnapshot{}, false
		}
	}
}

func (w *winEnv) nextNotify(t *testing.T) domain.SessionSnapshot {
	t.Helper()
	s, ok := w.tryNextNotify(realWait)
	if !ok {
		t.Fatalf("timeout waiting for notification #%d; got %+v", w.seen+1, w.notifications())
	}
	return s
}

func (w *winEnv) notifications() []domain.SessionSnapshot {
	var out []domain.SessionSnapshot
	for _, c := range w.rec.of(opNotify) {
		out = append(out, c.snap)
	}
	return out
}

// expireGrace advances the FakeClock by the grace period until the session
// ends, and returns the end notification. Stale waiters from earlier sessions
// make BlockUntilWaiters unreliable here, so it re-advances (bounded) until the
// end arrives; extra advances do not change ended_at (= start of grace).
func (w *winEnv) expireGrace(t *testing.T) domain.SessionSnapshot {
	t.Helper()
	deadline := time.Now().Add(realWait)
	for time.Now().Before(deadline) {
		w.clock.Advance(domain.SessionGracePeriod)
		if s, ok := w.tryNextNotify(100 * time.Millisecond); ok {
			return s
		}
	}
	t.Fatalf("session did not end after advancing the grace period; notifications %+v", w.notifications())
	return domain.SessionSnapshot{}
}

// waitLog waits (bounded by realWait) for a matching log record.
func (w *winEnv) waitLog(t *testing.T, level slog.Level, msg string, match func(logRec) bool) logRec {
	t.Helper()
	timeout := time.After(realWait)
	for {
		recs, ch := w.logs.state()
		for _, r := range recs {
			if r.level == level && r.msg == msg && (match == nil || match(r)) {
				return r
			}
		}
		select {
		case <-ch:
		case <-timeout:
			t.Fatalf("timeout waiting for log %v %q; got %v", level, msg, recs)
			return logRec{}
		}
	}
}

// waitRootExit waits for the SES-015 log of the initial process pid.
func (w *winEnv) waitRootExit(t *testing.T, pid int) logRec {
	t.Helper()
	want := fmt.Sprint(pid)
	return w.waitLog(t, slog.LevelInfo, msgRootExited, func(r logRec) bool { return r.attrs["pid"] == want })
}

func (w *winEnv) wantCurrent(t *testing.T, st domain.SessionState) {
	t.Helper()
	snap, ok := w.tr.Current()
	if !ok || snap.State != st {
		t.Errorf("Current = (%+v, %v), want state %q", snap, ok, st)
	}
}
