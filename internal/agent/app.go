package agent

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"os"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/logging"
	"github.com/arcadia-launcher/arcadia/internal/paths"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// Options are the inputs of Run, provided by main.
type Options struct {
	Args    []string // os.Args[1:], may contain "--agent"
	Version string   // main.version; "dev" when not stamped
}

// config holds the agent inputs resolved by Run.
type config struct {
	DataDir  string // --data-dir
	LogLevel string // --log-level
	ExeDir   string // directory of the running executable
	Exe      string // path of the running executable (launched by "Abrir Arcadia")
	PipeName string // ipc.PipeName(sid); reported in the IPC-019 log record
	Version  string // reported by agent.status (AGT-004)

	// Launcher starts the games; nil => tracker.NewLauncher() (tests inject a fake).
	Launcher tracker.Launcher
	// Recover closes the sessions left open by a crash [SES-010];
	// nil => tracker.RecoverOpenSessions.
	Recover func(context.Context, tracker.RecoveryStore, domain.Clock, *slog.Logger) (int, error)
}

// parseFlags parses the agent command line (EXE-003). It accepts --agent (the
// mode selector), --data-dir and --log-level; any other flag is an error. Flag
// package output is discarded: Run reports the error itself.
func parseFlags(args []string) (config, error) {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var cfg config
	_ = fs.Bool("agent", false, "run as the tray agent")
	fs.StringVar(&cfg.DataDir, "data-dir", "", "data directory (default: $ARCADIA_DATA_DIR or <exe dir>\\data)")
	fs.StringVar(&cfg.LogLevel, "log-level", "info", "log level: debug|info|warn|error")
	if err := fs.Parse(args); err != nil {
		return config{}, fmt.Errorf("parse flags: %w", err)
	}
	return cfg, nil
}

// ShutdownReason says what triggered the agent shutdown; it is sent to the
// clients as agent.shutdown params.reason (EXE-016).
type ShutdownReason string

// Shutdown reasons (strings of the IPC contract).
const (
	ReasonQuit       ShutdownReason = "quit"
	ReasonSessionEnd ShutdownReason = "session-end"
	ReasonSignal     ShutdownReason = "signal"
	ReasonOther      ShutdownReason = "other"
)

// shutdownDrainTimeout bounds the wait for the clients to close after
// agent.shutdown (EXE-014); it is the default of instance.drainTimeout.
const shutdownDrainTimeout = 500 * time.Millisecond

// spawnFunc starts a process without waiting for it (real: winapi.CreateDetached).
type spawnFunc func(exe string, args []string, flags uint32) error

// instance owns everything startAgent opened: store, IPC server and the log file.
type instance struct {
	paths  paths.Paths
	log    *slog.Logger
	info   ipc.AgentInfo
	store  *store.Store
	server *ipc.Server

	tracker  *tracker.Tracker
	notifier *sessionNotifier

	logCloser io.Closer

	drainTimeout time.Duration // limit of the drain in Shutdown (tests override it)

	// ipcFailed is closed (at most once) by the Serve goroutine when Serve
	// ended with an error not caused by Close or by ctx (EXE-026).
	ipcFailed chan struct{}

	wg sync.WaitGroup // the server.Serve goroutine

	shutdownOnce sync.Once
	shutdownErr  error
	closeLogOnce sync.Once
	closeLogErr  error
}

// startAgent validates the log level, resolves and creates the data
// directory, opens agent.log, opens and migrates the store, recovers the open
// sessions, starts the tracker and then serves IPC on the listener returned by
// listen [AGT-009]. On failure everything it opened is
// closed before returning (no handle leaks).
func startAgent(ctx context.Context, cfg config, clock domain.Clock, getenv func(string) string,
	listen func() (net.Listener, error)) (*instance, error) {
	level, err := logging.ParseLevel(cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}

	p, err := paths.Resolve(paths.Options{Flag: cfg.DataDir, ExeDir: cfg.ExeDir, Getenv: getenv})
	if err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}
	if err := p.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}

	log, logCloser, err := logging.New(logging.Options{Dir: p.Logs, File: "agent.log", Level: level})
	if err != nil {
		return nil, fmt.Errorf("start agent: %w", err)
	}

	st, err := store.Open(ctx, p.DB, store.OpenOptions{Migrate: true})
	if err != nil {
		log.Error("falha ao abrir o banco", "path", p.DB, "error", err.Error())
		return nil, errors.Join(fmt.Errorf("start agent: %w", err), closeWrap("log", logCloser))
	}

	// Recovery runs before the tracker and the IPC server [AGT-009, SES-010];
	// its failure is logged and the startup goes on.
	recoverFn := cfg.Recover
	if recoverFn == nil {
		recoverFn = tracker.RecoverOpenSessions
	}
	if _, err := recoverFn(ctx, st, clock, log); err != nil {
		log.Error("falha ao recuperar as sessões abertas", "error", err.Error())
	}

	launcher := cfg.Launcher
	if launcher == nil {
		launcher = tracker.NewLauncher()
	}
	notifier := &sessionNotifier{}
	tr := tracker.New(tracker.Config{
		Clock:    clock,
		Store:    st,
		Launcher: launcher,
		Notify:   notifier.Notify,
		Log:      log,
	})
	tr.Start()
	// closeTracker stops the tracker on a failed startup (no session is open
	// yet, so it returns at once); it ignores the cancellation of ctx so the
	// owner goroutine is always waited for.
	closeTracker := func() error {
		if err := tr.Close(context.WithoutCancel(ctx)); err != nil {
			return fmt.Errorf("close tracker: %w", err)
		}
		return nil
	}

	ln, err := listen()
	if err != nil {
		if isPipeNameTaken(err) {
			log.Error("pipe já existe: outra instância ou outro processo", "pipe", cfg.PipeName, "error", err.Error())
		} else {
			log.Error("falha ao abrir o pipe", "pipe", cfg.PipeName, "error", err.Error())
		}
		return nil, errors.Join(fmt.Errorf("start agent: listen: %w", err), closeTracker(),
			closeWrap("store", st), closeWrap("log", logCloser))
	}

	startedAt := clock.Now()
	info := ipc.AgentInfo{
		Version:   cfg.Version,
		PID:       os.Getpid(),
		StartedAt: startedAt,
		DataDir:   p.Root,
	}
	handlers := ipc.AgentHandlers(info, clock)
	maps.Copy(handlers, gameHandlers(st, tr, clock, os.Stat, log))
	server := ipc.NewServer(clock, handlers, log, ipc.WithStartedAt(startedAt))
	notifier.set(server)
	a := &instance{
		paths:        p,
		log:          log,
		info:         info,
		store:        st,
		server:       server,
		tracker:      tr,
		notifier:     notifier,
		logCloser:    logCloser,
		drainTimeout: shutdownDrainTimeout,
		ipcFailed:    make(chan struct{}),
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		// This goroutine is the only one closing ipcFailed, and Serve returns once.
		if err := a.server.Serve(ctx, ln); serveFailed(err) {
			log.Error("IPC indisponível; encerrando o agente", "error", err.Error())
			close(a.ipcFailed)
		}
	}()

	log.Info("agente iniciado", "version", info.Version, "pid", info.PID, "data_dir", p.Root)
	return a, nil
}

// serveFailed reports whether the error returned by Server.Serve means the IPC
// is unavailable (EXE-026): nil (Close) and context.Canceled (end of ctx) are
// a requested stop; anything else is a failure.
func serveFailed(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled)
}

// exitCode is the process exit code after the shutdown: 1 if the shutdown
// failed or the agent ended because the IPC failed (EXE-026), else 0.
func exitCode(shutdownErr error, ipcFailed bool) int {
	if shutdownErr != nil || ipcFailed {
		return 1
	}
	return 0
}

// closeWrap closes c and wraps its error with what.
func closeWrap(what string, c io.Closer) error {
	if err := c.Close(); err != nil {
		return fmt.Errorf("close %s: %w", what, err)
	}
	return nil
}

// Info returns the identity reported by agent.status.
func (a *instance) Info() ipc.AgentInfo { return a.info }

// Logger returns the agent logger (valid until CloseLog).
func (a *instance) Logger() *slog.Logger { return a.log }

// IPCFailed returns a channel closed when Serve ended with an error not caused
// by Close or by ctx (EXE-026). It is the same channel for the whole life of
// the instance and a normal Shutdown never closes it.
func (a *instance) IPCFailed() <-chan struct{} { return a.ipcFailed }

// Shutdown notifies the clients with agent.shutdown{reason} and waits up to
// drainTimeout for them to close the connection (EXE-013, EXE-014), then
// closes the IPC server (and waits for its goroutines), the tracker (which
// ends the open session, SES-011) and the store [AGT-010]. A drain
// that times out only logs a Warn. It is idempotent (later calls, whatever the
// reason, return the first result) and does not close the log (see CloseLog).
// If ctx ends first, Shutdown still finishes closing but reports ctx's error.
func (a *instance) Shutdown(ctx context.Context, reason ShutdownReason) error {
	a.shutdownOnce.Do(func() {
		var errs []error

		// No game.launch starts a session from here on [AGT-011].
		a.tracker.BeginShutdown()

		dctx, cancel := context.WithTimeout(ctx, a.drainTimeout)
		err := a.server.Drain(dctx, ipc.MethodShutdown, ipc.ShutdownParams{Reason: string(reason)})
		cancel()
		if err != nil {
			attrs := []any{"error", err.Error()}
			var de *ipc.DrainError
			if errors.As(err, &de) {
				attrs = append(attrs, "pending", de.Pending)
			}
			a.log.Warn("clientes não fecharam a conexão a tempo", attrs...)
		}

		if err := a.server.Close(); err != nil {
			errs = append(errs, fmt.Errorf("shutdown: close ipc server: %w", err))
		}
		a.wg.Wait()
		a.notifier.set(nil)
		// After the server (no request can reach the tracker) and before the
		// store (the tracker writes the end of the session) [AGT-010, SES-011].
		if err := a.tracker.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown: %w", err))
		}
		if err := a.store.Close(); err != nil {
			errs = append(errs, fmt.Errorf("shutdown: close store: %w", err))
		}
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("shutdown: %w", err))
		}
		a.shutdownErr = errors.Join(errs...)
	})
	return a.shutdownErr
}

// CloseLog closes the log file. It is idempotent and must be the last step of
// the shutdown: nothing may log after it (a write would reopen the file).
func (a *instance) CloseLog() error {
	a.closeLogOnce.Do(func() {
		a.closeLogErr = closeWrap("log", a.logCloser)
	})
	return a.closeLogErr
}
