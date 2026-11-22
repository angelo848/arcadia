package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/paths"
	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

const (
	// statusTimeout bounds each AgentStatus binding call (DSK-003, design.md frontend/)
	// and the one-off agent.status after a late first heartbeat (DSK-006).
	statusTimeout = 500 * time.Millisecond
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

// pinger is the part of *ipc.Client used to call the agent.
type pinger interface {
	Call(ctx context.Context, method string, params, result any) error
}

// ipcClient is the full client owned by the App.
type ipcClient interface {
	pinger
	Notifications() <-chan ipc.Notification
	StateChanged() <-chan struct{}
	Connected() bool
	Close() error
}

// platform holds the OS-specific pieces wired by main.
type platform struct {
	// newProbe returns a pinger that opens a fresh pipe connection per call and
	// fails at once when the pipe does not exist (DSK-001).
	newProbe func() (pinger, error)
	// newClient connects to the agent's pipe (reconnecting on its own).
	newClient func(log *slog.Logger) (ipcClient, error)
	// startAgent launches the agent (exe --agent) decoupled from the UI (EXE-007).
	startAgent func(exe, root string, log *slog.Logger) error
	// emit sends a Wails event to the frontend.
	emit func(ctx context.Context, event string, data any)
	// quit asks Wails to close the UI (asynchronous; runs OnShutdown, EXE-017).
	quit func(ctx context.Context)
	// showWindow restores the window and brings it to the front (EXE-011).
	showWindow func(ctx context.Context)
	// openFile shows the open-file dialog; "" means cancelled (DSK-012).
	openFile func(ctx context.Context, title string, filters []fileFilter) (string, error)
	// openDir shows the folder dialog; "" means cancelled (IMP-004, IMP-005).
	openDir func(ctx context.Context, title string) (string, error)
	// bringToFront forces the window to the foreground; false means Windows refused (CON-023).
	bringToFront func() (foreground bool, err error)
	// reveal opens the Explorer on a folder, or on a file's folder with the file selected.
	reveal func(path string, isFile bool) error
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
	owned     ipcClient // the client to close on shutdown
	logCloser io.Closer
	cancel    context.CancelFunc
	stopped   bool
	wailsCtx  context.Context // set by startup; never nil once set
	dataRoot  string          // the data folder, set by startup; media is served from its media folder (phase 7)
	sup       *supervisor     // nil until startup

	wg sync.WaitGroup // supervisor goroutine

	bigBoxRequest atomic.Bool // a pending request to open the Big Box (ATB-008, ATB-012)
}

// NewApp returns the App for cfg, wired to this OS.
func NewApp(cfg desktopConfig) *App {
	a := &App{cfg: cfg, platform: defaultPlatform(), clock: domain.SystemClock{}}
	if cfg.BigBox {
		a.bigBoxRequest.Store(true)
	}
	return a
}

// TakeBigBoxRequest returns true once per pending Big Box request and clears it (ATB-012).
func (a *App) TakeBigBoxRequest() bool {
	return a.bigBoxRequest.Swap(false)
}

// wantsBigBox reports whether args ask for --bigbox (ATB-009). It scans every argument itself:
// accepts --bigbox, -bigbox and --bigbox=<bool>; anything else is ignored.
func wantsBigBox(args []string) bool {
	for _, arg := range args {
		name, ok := strings.CutPrefix(arg, "--")
		if !ok {
			name, ok = strings.CutPrefix(arg, "-")
			if !ok {
				continue
			}
		}
		if name == "bigbox" {
			return true
		}
		if v, ok := strings.CutPrefix(name, "bigbox="); ok {
			if b, err := strconv.ParseBool(v); err == nil && b {
				return true
			}
		}
	}
	return false
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

// startup is the Wails OnStartup hook (Wails runs it on its own goroutine).
// Order: paths.Resolve -> setupLogging -> supervisor on a goroutine owned by
// wg (ensureAgent with a pipe probe, IPC client, relaunch, heartbeats). It
// returns without waiting for the agent.
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
	a.dataRoot = p.Root // a.mu is held above
	log, closer, err := setupLogging(p)
	if err != nil {
		slog.Error("falha ao abrir o log da UI", "error", err.Error())
		return
	}
	a.log, a.logCloser = log, closer
	a.wailsCtx = wailsCtx

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	// start holds a.mu so no agent is created once shutdown marked stopped
	// (EXE-025). Lock order is safe: shutdown releases a.mu before wg.Wait, and
	// startup (which holds a.mu) only starts the goroutine without waiting.
	start := func() error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.stopped {
			return fmt.Errorf("desktop: start agent: UI encerrando: %w", context.Canceled)
		}
		return a.platform.startAgent(a.cfg.Exe, p.Root, log)
	}
	a.sup = newSupervisor(supervisorConfig{
		Clock: a.clock,
		Ensure: func(ctx context.Context) (ipc.AgentStatus, error) {
			probe, err := a.platform.newProbe()
			if err != nil {
				return ipc.AgentStatus{}, fmt.Errorf("desktop: agent probe: %w", err)
			}
			return ensureAgent(ctx, probe, start, a.clock, agentStartTimeout)
		},
		NewClient: func() (ipcClient, error) { return a.platform.newClient(log) },
		Publish:   a.publishClient,
		Root:      p.Root,
		Emit:      func(event string, data any) { a.platform.emit(wailsCtx, event, data) },
		Quit:      func() { a.platform.quit(wailsCtx) },
		Log:       log,
	})
	sup := a.sup
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		sup.run(ctx)
	}()
	log.Info("UI iniciada", "data_dir", p.Root)
}

// publishClient makes c the App's client unless shutdown already began, in
// which case it returns false and the supervisor closes c.
func (a *App) publishClient(c ipcClient) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return false
	}
	a.client, a.owned = c, c
	return true
}

// secondInstance handles the notice of a second UI process (EXE-011,
// EXE-012): it brings the window to the front, logs and kicks the supervisor.
// It is safe before startup (no window context, logger or supervisor yet; the
// Info then goes to slog.Default). Once shutdown began it does nothing, since
// nothing may log after the log is closed (ADR 0013).
func (a *App) secondInstance(bigBox bool) {
	a.mu.Lock()
	stopped, wailsCtx, log, sup := a.stopped, a.wailsCtx, a.log, a.sup
	a.mu.Unlock()
	if stopped {
		return
	}
	if wailsCtx != nil && a.platform.showWindow != nil {
		a.platform.showWindow(wailsCtx)
	}
	if bigBox {
		if wailsCtx != nil && a.platform.bringToFront != nil {
			// A refusal or error only means the window may stay behind; it is not a failure.
			_, _ = a.platform.bringToFront()
		}
		// Stored before the event so the frontend can always read it (ATB-009, ATB-012).
		a.bigBoxRequest.Store(true)
		if wailsCtx != nil && a.platform.emit != nil {
			a.platform.emit(wailsCtx, "ui:bigBoxRequested", nil)
		}
	}
	if log == nil {
		log = slog.Default()
	}
	log.Info("segunda instância da UI; trazendo a janela à frente")
	if sup != nil {
		sup.Kick()
	}
}

// shutdown is the Wails OnShutdown hook: cancel (supervisor and any Ensure in
// progress) -> Client.Close -> wg.Wait (supervisor) -> log "UI encerrada" -> log
// Close (ADR 0013). The UI holds no database (ADR 0015) and the agent is never
// touched (DSK-002).
func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return
	}
	a.stopped = true
	a.client = nil
	cancel, owned, log, logCloser := a.cancel, a.owned, a.log, a.logCloser
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

// AgentElevated reports whether the agent the UI is connected to runs with
// administrator rights (ADM-006). Any failure (no agent, no PID, no access)
// reports false, so the UI shows the warning instead of using that agent.
func (a *App) AgentElevated() bool {
	st := a.AgentStatus()
	if !st.Connected || st.PID <= 0 {
		return false
	}
	elevated, err := winapi.IsProcessElevated(uint32(st.PID))
	if err != nil {
		return false
	}
	return elevated
}
