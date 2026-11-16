package agent

// Tests for T10 (docs/specs/phase-2-skeleton/tasks.md), moved from cmd/agent in
// T05 of docs/specs/single-exe/tasks.md: agent startup, version (now from
// config.Version), log level and ordered shutdown.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/feedback"
	"github.com/arcadia-launcher/arcadia/internal/input"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/paths"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// lumberjackMillRun is the lumberjack cleanup goroutine that Close does not stop
// (ADR 0013, design.md internal/logging).
const lumberjackMillRun = "gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"

const testTimeout = 5 * time.Second

// tcpListen records the TCP listeners handed to startAgent and how many times
// listen was called.
type tcpListen struct {
	mu    sync.Mutex
	calls int
	addr  string
}

func (l *tcpListen) listen() (net.Listener, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	l.addr = ln.Addr().String()
	return ln, nil
}

func (l *tcpListen) snapshot() (calls int, addr string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls, l.addr
}

func emptyEnv(string) string { return "" }

func testClock() *domain.FakeClock {
	return domain.NewFakeClock(time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC))
}

// testConfig returns a config whose data dir is a fresh, not-yet-created
// directory inside t.TempDir(). The tracker uses a fakeLauncher (T11 of
// docs/specs/phase-3-tracker/tasks.md): no test creates a real Job Object.
func testConfig(t *testing.T, level string) config {
	t.Helper()
	base := t.TempDir()
	return config{
		DataDir:  filepath.Join(base, "data"),
		LogLevel: level,
		ExeDir:   filepath.Join(base, "bin"),
		Exe:      filepath.Join(base, "bin", "arcadia.exe"),
		Version:  "test",
		Launcher: &fakeLauncher{},
	}
}

// startTestAgent starts an agent and registers a cleanup that shuts it down and
// closes its log (both idempotent per T10), before t.TempDir is removed.
func startTestAgent(t *testing.T, cfg config, ln *tcpListen) *instance {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	a, err := startAgent(ctx, cfg, testClock(), emptyEnv, ln.listen)
	if err != nil {
		cancel()
		t.Fatalf("startAgent: %v", err)
	}
	if a == nil {
		cancel()
		t.Fatal("startAgent returned nil agent without error")
	}
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
	return a
}

func shutdown(t *testing.T, a *instance) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if err := a.Shutdown(ctx, ReasonQuit); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// newTCPClient returns an ipc.Client dialing addr over TCP; the caller closes it.
func newTCPClient(addr string) *ipc.Client {
	dial := func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
	return ipc.NewClient(dial, testClock(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func callStatus(t *testing.T, c *ipc.Client) ipc.AgentStatus {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	var st ipc.AgentStatus
	if err := c.Call(ctx, ipc.MethodStatus, nil, &st); err != nil {
		t.Fatalf("Call(%s): %v", ipc.MethodStatus, err)
	}
	return st
}

// logLines parses every line of a JSON log file; a non-JSON line fails the test.
func logLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log %s: %v", path, err)
	}
	var out []map[string]any
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", line, err)
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan log %s: %v", path, err)
	}
	return out
}

func agentLogPath(cfg config) string { return filepath.Join(cfg.DataDir, "logs", "agent.log") }
func dbPath(cfg config) string       { return filepath.Join(cfg.DataDir, "library.db") }

func TestStartAgent(t *testing.T) {
	t.Run("AGT-001/cria library.db migrado e logs/agent.log", func(t *testing.T) {
		cfg := testConfig(t, "")
		ln := &tcpListen{}
		a := startTestAgent(t, cfg, ln)

		if calls, _ := ln.snapshot(); calls != 1 {
			t.Errorf("listen called %d times, want 1", calls)
		}
		if _, err := os.Stat(dbPath(cfg)); err != nil {
			t.Fatalf("library.db: %v", err)
		}
		if _, err := os.Stat(agentLogPath(cfg)); err != nil {
			t.Fatalf("logs/agent.log: %v", err)
		}

		shutdown(t, a)
		if err := a.CloseLog(); err != nil {
			t.Fatalf("CloseLog: %v", err)
		}

		// The database must be migrated: opening it without migration succeeds at SchemaVersion.
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		st, err := store.Open(ctx, dbPath(cfg), store.OpenOptions{Migrate: false})
		if err != nil {
			t.Fatalf("store.Open(Migrate:false) on agent database: %v", err)
		}
		defer st.Close()
		v, err := st.CurrentVersion(ctx)
		if err != nil {
			t.Fatalf("CurrentVersion: %v", err)
		}
		if v != store.SchemaVersion {
			t.Errorf("schema version = %d, want %d", v, store.SchemaVersion)
		}

		// agent.log is JSON lines (slog JSON handler) and has at least one record.
		if lines := logLines(t, agentLogPath(cfg)); len(lines) == 0 {
			t.Error("agent.log is empty, want at least one JSON record")
		}
	})
}

func TestVersion(t *testing.T) {
	t.Run("AGT-004/Info().Version vem de config.Version", func(t *testing.T) {
		t.Parallel()
		for _, v := range []string{"1.2.3", "dev"} {
			t.Run(v, func(t *testing.T) {
				t.Parallel()
				cfg := testConfig(t, "")
				cfg.Version = v
				ln := &tcpListen{}
				a := startTestAgent(t, cfg, ln)

				if got := a.Info().Version; got != v {
					t.Errorf("Info().Version = %q, want %q", got, v)
				}

				_, addr := ln.snapshot()
				c := newTCPClient(addr)
				defer c.Close()
				st := callStatus(t, c)
				if st.Version != v {
					t.Errorf("agent.status version = %q, want %q", st.Version, v)
				}
				if st.PID != os.Getpid() {
					t.Errorf("agent.status pid = %d, want %d", st.PID, os.Getpid())
				}
			})
		}
	})
}

func TestLogLevel(t *testing.T) {
	t.Run("AGT-005/nível inválido falha sem criar library.db", func(t *testing.T) {
		cfg := testConfig(t, "verbose")
		ln := &tcpListen{}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		a, err := startAgent(ctx, cfg, testClock(), emptyEnv, ln.listen)
		if err == nil {
			// Release whatever was started so the test does not leak.
			sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
			defer scancel()
			if a != nil {
				_ = a.Shutdown(sctx, ReasonOther)
				_ = a.CloseLog()
			}
			t.Fatal("startAgent with --log-level verbose: want error, got nil")
		}
		if a != nil {
			t.Errorf("startAgent returned non-nil agent with error %v", err)
		}
		if _, statErr := os.Stat(dbPath(cfg)); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("library.db stat = %v, want not exist", statErr)
		}
		// ParseLevel runs before paths.EnsureDirs and logging.New: no file at all.
		if _, statErr := os.Stat(cfg.DataDir); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("data dir stat = %v, want not exist (invalid level must fail before any file)", statErr)
		}
		if calls, _ := ln.snapshot(); calls != 0 {
			t.Errorf("listen called %d times, want 0", calls)
		}
	})

	t.Run("AGT-005/sem flag usa info", func(t *testing.T) {
		cfg := testConfig(t, "")
		ln := &tcpListen{}
		a := startTestAgent(t, cfg, ln)

		// Exercise the IPC server (it logs connection events at Debug).
		_, addr := ln.snapshot()
		c := newTCPClient(addr)
		_ = callStatus(t, c)
		if err := c.Close(); err != nil {
			t.Fatalf("client Close: %v", err)
		}

		shutdown(t, a)
		if err := a.CloseLog(); err != nil {
			t.Fatalf("CloseLog: %v", err)
		}

		lines := logLines(t, agentLogPath(cfg))
		var info int
		for _, rec := range lines {
			switch rec["level"] {
			case "DEBUG":
				t.Errorf("DEBUG record written with default level: %v", rec)
			case "INFO":
				info++
			}
		}
		if info == 0 {
			t.Error("no INFO record in agent.log, want at least one with the default level")
		}
	})
}

func TestShutdown(t *testing.T) {
	t.Run("AGT-003/Shutdown sem goroutines sobrando (goleak)", func(t *testing.T) {
		cfg := testConfig(t, "")
		ln := &tcpListen{}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		a, err := startAgent(ctx, cfg, testClock(), emptyEnv, ln.listen)
		if err != nil {
			t.Fatalf("startAgent: %v", err)
		}

		// Keep a live connection open across Shutdown: the server must close it.
		_, addr := ln.snapshot()
		c := newTCPClient(addr)
		_ = callStatus(t, c)

		shutdown(t, a)
		if err := a.CloseLog(); err != nil {
			t.Fatalf("CloseLog: %v", err)
		}
		if err := c.Close(); err != nil {
			t.Fatalf("client Close: %v", err)
		}

		goleak.VerifyNone(t, goleak.IgnoreTopFunction(lumberjackMillRun))
	})

	t.Run("AGT-003/Shutdown fecha o store", func(t *testing.T) {
		cfg := testConfig(t, "")
		a := startTestAgent(t, cfg, &tcpListen{})

		// Sanity check of the method: on Windows an open SQLite file cannot be removed.
		if runtime.GOOS == "windows" {
			if err := os.Remove(dbPath(cfg)); err == nil {
				t.Fatal("library.db removed while the agent is running; store is not open?")
			}
		}

		shutdown(t, a)

		for _, p := range []string{dbPath(cfg), dbPath(cfg) + "-wal", dbPath(cfg) + "-shm"} {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Errorf("remove %s after Shutdown: %v (store still open)", filepath.Base(p), err)
			}
		}
	})

	t.Run("AGT-003/Shutdown idempotente", func(t *testing.T) {
		cfg := testConfig(t, "")
		ln := &tcpListen{}
		a := startTestAgent(t, cfg, ln)

		shutdown(t, a)
		shutdown(t, a)

		// After Shutdown the IPC server no longer accepts connections.
		_, addr := ln.snapshot()
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			t.Error("dial after Shutdown succeeded, want listener closed")
		}
	})

	t.Run("AGT-003/falha ao abrir o store fecha o log", func(t *testing.T) {
		cfg := testConfig(t, "")
		// A directory named library.db makes store.Open fail after agent.log is open.
		if err := os.MkdirAll(dbPath(cfg), 0o755); err != nil {
			t.Fatalf("mkdir library.db: %v", err)
		}
		ln := &tcpListen{}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		a, err := startAgent(ctx, cfg, testClock(), emptyEnv, ln.listen)
		if err == nil {
			sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
			defer scancel()
			if a != nil {
				_ = a.Shutdown(sctx, ReasonOther)
				_ = a.CloseLog()
			}
			t.Fatal("startAgent with library.db as a directory: want error, got nil")
		}
		if a != nil {
			t.Errorf("startAgent returned non-nil agent with error %v", err)
		}
		if calls, _ := ln.snapshot(); calls != 0 {
			t.Errorf("listen called %d times, want 0 (store fails first)", calls)
		}

		var errs int
		for _, rec := range logLines(t, agentLogPath(cfg)) {
			if rec["level"] == "ERROR" {
				errs++
			}
		}
		if errs == 0 {
			t.Error("no ERROR record in agent.log after store.Open failed")
		}

		// The log handle was released (on Windows an open file cannot be removed).
		if err := os.Remove(agentLogPath(cfg)); err != nil {
			t.Errorf("remove agent.log after failed startAgent: %v (log still open)", err)
		}
	})

	t.Run("AGT-003/CloseLog idempotente", func(t *testing.T) {
		cfg := testConfig(t, "")
		a := startTestAgent(t, cfg, &tcpListen{})

		shutdown(t, a)
		if err := a.CloseLog(); err != nil {
			t.Fatalf("first CloseLog: %v", err)
		}
		if err := a.CloseLog(); err != nil {
			t.Fatalf("second CloseLog: %v", err)
		}
		// The log file handle is released (on Windows an open file cannot be removed).
		if err := os.Remove(agentLogPath(cfg)); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("remove agent.log after CloseLog: %v (log still open)", err)
		}
	})
}

func TestStartupOrderWithPause(t *testing.T) {
	verifyNoLeaks(t)

	t.Run("AGT-012/ordem: recuperação, feedback, tracker, servidor, roteador e depois fontes e energia", func(t *testing.T) {
		var (
			mu    sync.Mutex
			order []string
		)
		add := func(step string) {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, step)
		}

		cfg := testConfig(t, "debug")
		cfg.Recover = func(ctx context.Context, s tracker.RecoveryStore, c domain.Clock, l *slog.Logger) (int, error) {
			add("recover")
			return tracker.RecoverOpenSessions(ctx, s, c, l)
		}

		src := newFakeSource("test-src", nil)
		powerWatcher := &fakePowerWatcher{}

		cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
			return platformParts{
				Sources: []input.Source{
					&sourceRecorder{
						Source: src,
						onStart: func() {
							add("source")
						},
					},
				},
				Power: func(onSuspend func(at time.Time), onWake func(at time.Time, userPresent bool)) (io.Closer, error) {
					add("power")
					powerWatcher.onSuspend = onSuspend
					powerWatcher.onWake = onWake
					return powerWatcher, nil
				},
				NewOverlay: func() (feedback.Overlay, error) {
					add("feedback-overlay")
					return feedback.NopOverlay{}, nil
				},
				NewSound: func() (feedback.Sound, error) {
					add("feedback-sound")
					return feedback.NopSound{}, nil
				},
			}
		}

		ln := &tcpListenOrder{
			onListen: func() {
				add("listen")
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		a, err := startAgent(ctx, cfg, testClock(), emptyEnv, ln.listen)
		if err != nil {
			t.Fatalf("startAgent: %v", err)
		}
		t.Cleanup(func() {
			sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
			defer scancel()
			_ = a.Shutdown(sctx, ReasonOther)
			_ = a.CloseLog()
		})

		mu.Lock()
		got := make([]string, len(order))
		copy(got, order)
		mu.Unlock()

		// Verify order:
		// recover before feedback, listen, source, power
		recIdx := indexOf(got, "recover")
		ovIdx := indexOf(got, "feedback-overlay")
		sndIdx := indexOf(got, "feedback-sound")
		listenIdx := indexOf(got, "listen")
		srcIdx := indexOf(got, "source")
		pwrIdx := indexOf(got, "power")

		if recIdx == -1 || ovIdx == -1 || sndIdx == -1 || listenIdx == -1 || srcIdx == -1 || pwrIdx == -1 {
			t.Fatalf("missing steps in startup order: %v", got)
		}

		if recIdx > ovIdx || recIdx > sndIdx {
			t.Errorf("recover (%d) should be before feedback (%d, %d)", recIdx, ovIdx, sndIdx)
		}
		if ovIdx > listenIdx || sndIdx > listenIdx {
			t.Errorf("feedback (%d, %d) should be before listen (%d)", ovIdx, sndIdx, listenIdx)
		}
		if listenIdx > srcIdx || listenIdx > pwrIdx {
			t.Errorf("listen (%d) should be before source (%d) and power (%d)", listenIdx, srcIdx, pwrIdx)
		}
	})

	t.Run("AGT-012/nenhuma fonte recebe Start antes de o tracker iniciar", func(t *testing.T) {
		var (
			mu            sync.Mutex
			sourceStarted bool
			trackerReady  bool
		)

		cfg := testConfig(t, "debug")
		src := newFakeSource("test-src", nil)

		cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
			return platformParts{
				Sources: []input.Source{
					&sourceRecorder{
						Source: src,
						onStart: func() {
							mu.Lock()
							defer mu.Unlock()
							sourceStarted = true
							if !trackerReady {
								t.Errorf("source started before tracker was ready")
							}
						},
					},
				},
			}
		}

		ln := &tcpListenOrder{
			onListen: func() {
				mu.Lock()
				defer mu.Unlock()
				// listener is created after tracker.New and tr.Start()
				trackerReady = true
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		a, err := startAgent(ctx, cfg, testClock(), emptyEnv, ln.listen)
		if err != nil {
			t.Fatalf("startAgent: %v", err)
		}
		t.Cleanup(func() {
			sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
			defer scancel()
			_ = a.Shutdown(sctx, ReasonOther)
			_ = a.CloseLog()
		})

		mu.Lock()
		defer mu.Unlock()
		if !sourceStarted {
			t.Error("source was never started")
		}
	})

	t.Run("AGT-012/a recuperação fecha a pausa aberta antes de qualquer fonte", func(t *testing.T) {
		cfg := testConfig(t, "debug")
		ctx := context.Background()

		// Prepare pre-existing database with an open session and open pause
		p, err := paths.Resolve(paths.Options{Flag: cfg.DataDir, ExeDir: cfg.ExeDir, Getenv: emptyEnv})
		if err != nil {
			t.Fatalf("resolve paths: %v", err)
		}
		if err := p.EnsureDirs(); err != nil {
			t.Fatalf("ensure dirs: %v", err)
		}

		st, err := store.Open(ctx, p.DB, store.OpenOptions{Migrate: true})
		if err != nil {
			t.Fatalf("open store for setup: %v", err)
		}

		t0 := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
		g, err := st.AddGame(ctx, store.NewGame{Title: "Pending Game", Exe: `C:\Jogos\p.exe`, WorkingDir: `C:\Jogos`, At: t0})
		if err != nil {
			t.Fatalf("add game: %v", err)
		}
		sessID, err := st.StartSession(ctx, g.ID, t0)
		if err != nil {
			t.Fatalf("start session: %v", err)
		}
		// Pause session at t0 + 10s
		tPause := t0.Add(10 * time.Second)
		if err := st.PauseSession(ctx, store.PauseStart{
			SessionID: sessID,
			At:        tPause,
			Reason:    domain.PauseHotkey,
			Active:    10 * time.Second,
		}); err != nil {
			t.Fatalf("pause session: %v", err)
		}
		_ = st.Close()

		var verifiedAtSourceStart bool
		src := newFakeSource("test-src", nil)
		cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
			return platformParts{
				Sources: []input.Source{
					&sourceRecorder{
						Source: src,
						onStart: func() {
							// Check that the database session was already recovered
							checkStore, err := store.Open(ctx, p.DB, store.OpenOptions{Migrate: false})
							if err != nil {
								t.Errorf("open store at source start: %v", err)
								return
							}
							defer checkStore.Close()

							openSess, err := checkStore.OpenSessions(ctx)
							if err != nil {
								t.Errorf("list open sessions: %v", err)
								return
							}
							if len(openSess) != 0 {
								t.Errorf("session still open at source start: %v", openSess)
							}
							pauses, err := checkStore.Pauses(ctx, sessID)
							if err != nil {
								t.Errorf("get pauses: %v", err)
								return
							}
							for _, pause := range pauses {
								if pause.ResumedAt.IsZero() {
									t.Errorf("pause still open at source start")
								}
							}
							verifiedAtSourceStart = true
						},
					},
				},
			}
		}

		ln := &tcpListen{}
		cctx, ccancel := context.WithCancel(context.Background())
		defer ccancel()

		a, err := startAgent(cctx, cfg, testClock(), emptyEnv, ln.listen)
		if err != nil {
			t.Fatalf("startAgent: %v", err)
		}
		t.Cleanup(func() {
			sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
			defer scancel()
			_ = a.Shutdown(sctx, ReasonOther)
			_ = a.CloseLog()
		})

		if !verifiedAtSourceStart {
			t.Error("verification at source start was not reached")
		}
	})
}

type sourceRecorder struct {
	input.Source
	onStart func()
}

func (r *sourceRecorder) Start(out chan<- input.Trigger) error {
	if r.onStart != nil {
		r.onStart()
	}
	return r.Source.Start(out)
}

type tcpListenOrder struct {
	addr     string
	onListen func()
}

func (l *tcpListenOrder) listen() (net.Listener, error) {
	if l.onListen != nil {
		l.onListen()
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	l.addr = ln.Addr().String()
	return ln, nil
}

// TestVideoHandlersWired covers CON-026 (T26): the agent registers the four video.* handlers and keeps the others.
func TestVideoHandlersWired(t *testing.T) {
	t.Run("CON-026/o agente registra os 4 handlers video.* e continua com todos os outros", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})
		c := w.client(t)
		methods := []string{
			ipc.MethodVideoQueue, ipc.MethodVideoQueueMissing, ipc.MethodVideoCancelPending, ipc.MethodVideoStatus,
			// the rest of the surface the agent had before phase 13 stays registered
			ipc.MethodStatus, ipc.MethodGameList, ipc.MethodGameDelete, ipc.MethodMediaList, ipc.MethodMediaQueue,
			ipc.MethodScrapeSearch, ipc.MethodScrapeApply, ipc.MethodProviderKeyStatus, ipc.MethodProviderSetKey,
			ipc.MethodMetadataSearch, ipc.MethodMetadataApply, ipc.MethodMetadataBatchStart,
			ipc.MethodPlatformList, ipc.MethodSessionList, ipc.MethodImportScan,
		}
		for _, m := range methods {
			t.Run("CON-026/"+m+" registrado", func(t *testing.T) {
				resp := c.rpc(t, m, nil)
				if resp.Error != nil && resp.Error.Code == ipc.CodeMethodNotFound {
					t.Errorf("%s: method not found; the handler is not registered", m)
				}
			})
		}
		t.Run("CON-026/video.nope continua -32601", func(t *testing.T) {
			resp := c.rpc(t, "video.nope", nil)
			if resp.Error == nil || resp.Error.Code != ipc.CodeMethodNotFound {
				t.Errorf("video.nope: error = %+v, want code %d", resp.Error, ipc.CodeMethodNotFound)
			}
		})
	})
}

// TestVideoConstantsMatchContract covers CON-026: info.x-video of the phase 13 contract (v0.15.0) equals the
// constants of the agent, scrapers, domain and media.
func TestVideoConstantsMatchContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "specs", "phase-13-modo-console", "contracts", "ipc.openrpc.json"))
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var doc struct {
		Info struct {
			XVideo map[string]json.RawMessage `json:"x-video"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode contract: %v", err)
	}
	x := doc.Info.XVideo
	if x == nil {
		t.Fatal("contract has no info.x-video block")
	}
	ms := func(d time.Duration) float64 { return float64(d / time.Millisecond) }
	nums := []struct {
		key string
		got float64
	}{
		{"maxQueueGames", maxVideoQueueGames},
		{"laneWorkers", videoConcurrency},
		{"quotaPauseMs", ms(videoQuotaPause)},
		{"requestIntervalMs", ms(scrapers.IntervalScreenScraper)},
		{"kindLimit", float64(domain.MediaKindLimit("video"))},
		{"maxVideoBytes", media.MaxVideoBytes},
	}
	for _, c := range nums {
		t.Run("CON-026/x-video."+c.key, func(t *testing.T) {
			var want float64
			if err := json.Unmarshal(x[c.key], &want); err != nil {
				t.Fatalf("x-video.%s: %v", c.key, err)
			}
			if c.got != want {
				t.Errorf("%s = %v, contract %v", c.key, c.got, want)
			}
		})
	}
	strs := []struct{ key, got string }{
		{"softname", scrapers.SoftName},
		{"mediaType", scrapers.SSVideoMediaType},
	}
	for _, c := range strs {
		t.Run("CON-026/x-video."+c.key, func(t *testing.T) {
			var want string
			if err := json.Unmarshal(x[c.key], &want); err != nil {
				t.Fatalf("x-video.%s: %v", c.key, err)
			}
			if c.got != want {
				t.Errorf("%s = %q, contract %q", c.key, c.got, want)
			}
		})
	}
}
