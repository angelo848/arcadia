package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/paths"
)

const (
	// statusTimeout bounds each AgentStatus binding call (DSK-003, design.md frontend/)
	// and the one-off agent.status after a late first heartbeat (DSK-006).
	statusTimeout = 500 * time.Millisecond
	// summaryTimeout bounds each LibrarySummary binding call.
	summaryTimeout = 2 * time.Second
	// agentStartTimeout is how long the UI waits for the agent to answer (DSK-001).
	agentStartTimeout = 5 * time.Second
	// firstProbeTimeout bounds the initial "is the agent already running?" probe.
	firstProbeTimeout = time.Second
	// probeRetryPause separates probes that failed without waiting (e.g. the pipe
	// does not exist yet).
	probeRetryPause = 100 * time.Millisecond
	// heartbeatEvent is the Wails event that forwards agent.heartbeat to the frontend.
	heartbeatEvent = "agent:heartbeat"
)

// StatusView is the agent status as shown by the frontend. The zero value
// means disconnected.
type StatusView struct {
	Connected     bool   `json:"connected"`
	Version       string `json:"version"`
	PID           int    `json:"pid"`
	StartedAt     string `json:"startedAt"` // RFC 3339, UTC
	DataDir       string `json:"dataDir"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
}

// LibrarySummaryView is the library counts shown by the empty state.
type LibrarySummaryView struct {
	Games     int64 `json:"games"`
	Platforms int64 `json:"platforms"`
}

// pinger is the part of *ipc.Client used to call the agent.
type pinger interface {
	Call(ctx context.Context, method string, params, result any) error
}

// ipcClient is the full client owned by the App.
type ipcClient interface {
	pinger
	Notifications() <-chan ipc.Notification
	Close() error
}

// platform holds the OS-specific pieces wired by main.
type platform struct {
	// newProbe returns a pinger that opens a fresh pipe connection per call and
	// fails at once when the pipe does not exist (DSK-001).
	newProbe func() (pinger, error)
	// newClient connects to the agent's pipe (reconnecting on its own).
	newClient func(log *slog.Logger) (ipcClient, error)
	// startAgent launches the agent decoupled from the UI.
	startAgent func(exeDir, root string, log *slog.Logger) error
	// emit sends a Wails event to the frontend.
	emit func(ctx context.Context, event string, data any)
}

// App is the Wails-bound application. Its exported methods are the frontend
// bindings. It never stops the agent (DSK-002).
type App struct {
	cfg      desktopConfig
	platform platform
	clock    domain.Clock

	mu        sync.Mutex
	client    pinger // nil until startup created the IPC client
	log       *slog.Logger
	gate      *libraryGate
	owned     ipcClient // the client to close on shutdown
	logCloser io.Closer
	cancel    context.CancelFunc
	stopped   bool

	wg sync.WaitGroup // connect-and-forward goroutine
}

// NewApp returns the App for cfg, wired to this OS.
func NewApp(cfg desktopConfig) *App {
	return &App{cfg: cfg, platform: defaultPlatform(), clock: domain.SystemClock{}}
}

// AgentStatus calls agent.status with a 500 ms timeout. Any failure (or no
// client yet) yields the zero StatusView, i.e. Connected=false (DSK-003).
func (a *App) AgentStatus() StatusView {
	a.mu.Lock()
	c := a.client
	a.mu.Unlock()
	if c == nil {
		return StatusView{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()
	var st ipc.AgentStatus
	if err := c.Call(ctx, ipc.MethodStatus, nil, &st); err != nil {
		return StatusView{}
	}
	startedAt := ""
	if !st.StartedAt.IsZero() {
		startedAt = st.StartedAt.UTC().Format(time.RFC3339)
	}
	return StatusView{
		Connected:     true,
		Version:       st.Version,
		PID:           st.PID,
		StartedAt:     startedAt,
		DataDir:       st.DataDir,
		UptimeSeconds: st.UptimeSeconds,
	}
}

// LibrarySummary returns the library counts through the lazy gate; before the
// agent was reached it returns ErrAgentNotReady (DSK-007, DSK-008). Each call
// is bounded by a 2 s timeout.
func (a *App) LibrarySummary() (LibrarySummaryView, error) {
	a.mu.Lock()
	g := a.gate
	a.mu.Unlock()
	if g == nil {
		return LibrarySummaryView{}, ErrAgentNotReady
	}
	ctx, cancel := context.WithTimeout(context.Background(), summaryTimeout)
	defer cancel()
	return g.Summary(ctx)
}

// startup is the Wails OnStartup hook (Wails runs it on its own goroutine).
// Order: paths.Resolve -> setupLogging -> (owned goroutine) ensureAgent with a
// pipe probe -> gate.MarkConnected -> warnIfDataDirDiffers -> IPC client ->
// forward heartbeats.
func (a *App) startup(wailsCtx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return
	}

	p, err := paths.Resolve(paths.Options{Flag: a.cfg.DataDir, ExeDir: a.cfg.ExeDir})
	if err != nil {
		slog.Error("falha ao resolver a pasta de dados", "error", err.Error())
		return
	}
	log, closer, err := setupLogging(p)
	if err != nil {
		slog.Error("falha ao abrir o log da UI", "error", err.Error())
		return
	}
	a.log, a.logCloser = log, closer
	a.gate = newLibraryGate(storeOpener(p.DB))

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	gate := a.gate
	start := func() error { return a.platform.startAgent(a.cfg.ExeDir, p.Root, log) }
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.connectAndForward(ctx, wailsCtx, gate, start, p.Root, log)
	}()
	log.Info("UI iniciada", "data_dir", p.Root)
}

// connectAndForward makes sure the agent runs (probing the pipe directly, so
// the ipc.Client backoff does not delay DSK-001), then creates the long-lived
// IPC client and forwards agent.heartbeat notifications as Wails events until
// the client's notification channel is closed (by Client.Close on shutdown).
func (a *App) connectAndForward(ctx, wailsCtx context.Context, gate *libraryGate,
	start func() error, root string, log *slog.Logger) {
	connected := a.waitAgent(ctx, gate, start, root, log)
	if ctx.Err() != nil {
		return
	}

	c, err := a.platform.newClient(log)
	if err != nil {
		log.Error("falha ao criar o cliente IPC", "error", err.Error())
		return
	}
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		if err := c.Close(); err != nil {
			log.Warn("falha ao fechar o cliente IPC", "error", err.Error())
		}
		return
	}
	a.client, a.owned = c, c
	a.mu.Unlock()

	for n := range c.Notifications() {
		if n.Method != ipc.MethodHeartbeat {
			continue
		}
		if !connected {
			// The first heartbeat is the first successful connection (DSK-007)
			// of an agent that missed the ensureAgent window.
			connected = true
			a.lateConnect(c, gate, root, log)
		}
		var hb ipc.HeartbeatParams
		if len(n.Params) == 0 {
			log.Warn("heartbeat com parâmetros inválidos", "error", "params ausentes")
			continue
		}
		if err := json.Unmarshal(n.Params, &hb); err != nil {
			log.Warn("heartbeat com parâmetros inválidos", "error", err.Error())
			continue
		}
		a.platform.emit(wailsCtx, heartbeatEvent, hb)
	}
}

// waitAgent runs ensureAgent with a per-call pipe probe and, on success, opens
// the library gate and checks the data dir (DSK-001, DSK-006, DSK-007). It
// reports whether the agent answered.
func (a *App) waitAgent(ctx context.Context, gate *libraryGate, start func() error, root string,
	log *slog.Logger) bool {
	probe, err := a.platform.newProbe()
	if err != nil {
		log.Error("falha ao criar a sondagem do agente", "error", err.Error())
		return false
	}
	st, err := ensureAgent(ctx, probe, start, a.clock, agentStartTimeout)
	if err != nil {
		if ctx.Err() == nil {
			log.Error("agente não respondeu", "timeout", agentStartTimeout.String(), "error", err.Error())
		}
		return false
	}
	gate.MarkConnected()
	warnIfDataDirDiffers(log, root, st.DataDir)
	return true
}

// lateConnect handles the first heartbeat of an agent that ensureAgent did not
// reach: it opens the library gate, checks the data dir once (500 ms) and logs
// the connection.
func (a *App) lateConnect(c pinger, gate *libraryGate, root string, log *slog.Logger) {
	gate.MarkConnected()
	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()
	var st ipc.AgentStatus
	if err := c.Call(ctx, ipc.MethodStatus, nil, &st); err != nil {
		log.Warn("falha ao consultar agent.status", "error", err.Error())
	} else {
		warnIfDataDirDiffers(log, root, st.DataDir)
	}
	log.Info("agente conectado")
}

// shutdown is the Wails OnShutdown hook: cancel ensureAgent -> Client.Close ->
// wg.Wait -> libraryGate.Close -> log Close. The agent is never touched (DSK-002).
func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return
	}
	a.stopped = true
	a.client = nil
	cancel, owned, gate, log, logCloser := a.cancel, a.owned, a.gate, a.log, a.logCloser
	a.mu.Unlock()
	// From here on the goroutine sees stopped and never sets a new client, so
	// owned is the only client that may exist.

	if cancel != nil {
		cancel()
	}
	if owned != nil {
		if err := owned.Close(); err != nil && log != nil {
			log.Warn("falha ao fechar o cliente IPC", "error", err.Error())
		}
	}
	a.wg.Wait()
	if gate != nil {
		if err := gate.Close(); err != nil && log != nil {
			log.Warn("falha ao fechar a biblioteca", "error", err.Error())
		}
	}
	if log != nil {
		log.Info("UI encerrada")
	}
	// Last step: nothing may log after the log file is closed.
	if logCloser != nil {
		if err := logCloser.Close(); err != nil {
			slog.Error("falha ao fechar o log da UI", "error", err.Error())
		}
	}
}

// ensureAgent returns the agent status, starting the agent first when a short
// probe gets no answer (DSK-001). It gives up when the agent does not answer
// within timeout after start, when start fails or when ctx is done. All waits
// use clock. p is expected to fail at once while the pipe does not exist
// (probePinger); such failures are retried every probeRetryPause until the
// timeout, counted from the start of the call.
func ensureAgent(ctx context.Context, p pinger, start func() error, clock domain.Clock,
	timeout time.Duration) (ipc.AgentStatus, error) {
	if err := ctx.Err(); err != nil {
		return ipc.AgentStatus{}, fmt.Errorf("ensure agent: %w", err)
	}
	deadline := clock.After(timeout)

	firstLimit := firstProbeTimeout
	if firstLimit >= timeout {
		firstLimit = timeout / 2
	}
	st, _, err := probeStatus(ctx, p, clock.After(firstLimit))
	if err == nil {
		return st, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ipc.AgentStatus{}, fmt.Errorf("ensure agent: %w", ctxErr)
	}

	if err := start(); err != nil {
		return ipc.AgentStatus{}, fmt.Errorf("ensure agent: start: %w", err)
	}

	for {
		st, expired, err := probeStatus(ctx, p, deadline)
		if err == nil {
			return st, nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ipc.AgentStatus{}, fmt.Errorf("ensure agent: %w", ctxErr)
		}
		if expired {
			return ipc.AgentStatus{}, fmt.Errorf("ensure agent: no answer within %s: %w", timeout, err)
		}
		select {
		case <-clock.After(probeRetryPause):
		case <-deadline:
			return ipc.AgentStatus{}, fmt.Errorf("ensure agent: no answer within %s: %w", timeout, err)
		case <-ctx.Done():
			return ipc.AgentStatus{}, fmt.Errorf("ensure agent: %w", ctx.Err())
		}
	}
}

// probeStatus calls agent.status until it answers or limit fires; expired
// reports that limit fired. The call goroutine is always waited for.
func probeStatus(ctx context.Context, p pinger, limit <-chan time.Time) (st ipc.AgentStatus, expired bool, err error) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		st  ipc.AgentStatus
		err error
	}
	ch := make(chan result, 1)
	go func() {
		var s ipc.AgentStatus
		e := p.Call(cctx, ipc.MethodStatus, nil, &s)
		ch <- result{s, e}
	}()
	select {
	case r := <-ch:
		return r.st, false, r.err
	case <-limit:
		cancel()
		r := <-ch
		return r.st, r.err != nil, r.err
	}
}
