package agent

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/logging"
	"github.com/arcadia-launcher/arcadia/internal/paths"
	"github.com/arcadia-launcher/arcadia/internal/store"
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

// spawnFunc starts a process without waiting for it (real: winapi.CreateDetached).
type spawnFunc func(exe string, args []string, flags uint32) error

// instance owns everything startAgent opened: store, IPC server and the log file.
type instance struct {
	paths  paths.Paths
	log    *slog.Logger
	info   ipc.AgentInfo
	store  *store.Store
	server *ipc.Server

	logCloser io.Closer

	wg sync.WaitGroup // the server.Serve goroutine

	shutdownOnce sync.Once
	shutdownErr  error
	closeLogOnce sync.Once
	closeLogErr  error
}

// startAgent validates the log level, resolves and creates the data
// directory, opens agent.log, opens and migrates the store and starts serving
// IPC on the listener returned by listen. On failure everything it opened is
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

	ln, err := listen()
	if err != nil {
		if isPipeNameTaken(err) {
			log.Error("pipe já existe: outra instância ou outro processo", "pipe", cfg.PipeName, "error", err.Error())
		} else {
			log.Error("falha ao abrir o pipe", "pipe", cfg.PipeName, "error", err.Error())
		}
		return nil, errors.Join(fmt.Errorf("start agent: listen: %w", err), closeWrap("store", st), closeWrap("log", logCloser))
	}

	startedAt := clock.Now()
	info := ipc.AgentInfo{
		Version:   cfg.Version,
		PID:       os.Getpid(),
		StartedAt: startedAt,
		DataDir:   p.Root,
	}
	a := &instance{
		paths:     p,
		log:       log,
		info:      info,
		store:     st,
		server:    ipc.NewServer(clock, ipc.AgentHandlers(info, clock), log, ipc.WithStartedAt(startedAt)),
		logCloser: logCloser,
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.server.Serve(ctx, ln); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("ipc: serve ended", "error", err.Error())
		}
	}()

	log.Info("agente iniciado", "version", info.Version, "pid", info.PID, "data_dir", p.Root)
	return a, nil
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

// Shutdown closes the IPC server (and waits for its goroutines) and then the
// store. It is idempotent and does not close the log (see CloseLog). If ctx
// ends first, Shutdown still finishes closing but reports ctx's error.
func (a *instance) Shutdown(ctx context.Context) error {
	a.shutdownOnce.Do(func() {
		var errs []error
		if err := a.server.Close(); err != nil {
			errs = append(errs, fmt.Errorf("shutdown: close ipc server: %w", err))
		}
		a.wg.Wait()
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
