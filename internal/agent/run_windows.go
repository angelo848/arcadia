//go:build windows

// Package agent is the Arcadia tray agent (agent mode of the executable): it
// owns the play-session tables, serves IPC to the UI over a per-user named
// pipe and shows the tray icon. main calls Run with the command line and the
// version stamped at build time (-X main.version=<v>).
package agent

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/logging"
	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// shutdownTimeout bounds agent.Shutdown (AGT-003).
const shutdownTimeout = 2 * time.Second

// autostartTimeout bounds each autostart Reconcile/Toggle (store access).
const autostartTimeout = 2 * time.Second

// Run executes the agent and returns the process exit code.
func Run(opts Options) int {
	// Bootstrap logger for failures before agent.log exists (invisible with -H windowsgui).
	boot := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// EXE-003: bad flags fail before the mutex, the data dir or any file.
	cfg, err := parseFlags(opts.Args)
	if err != nil {
		boot.Error("argumentos inválidos", "error", err.Error())
		return 2
	}
	cfg.Version = opts.Version

	// AGT-005: an invalid level fails before the mutex, the data dir or any file.
	if _, err := logging.ParseLevel(cfg.LogLevel); err != nil {
		boot.Error("nível de log inválido", "error", err.Error())
		return 2
	}

	mutex, already, err := winapi.AcquireSingleInstance(winapi.AgentMutexName)
	if already {
		// AGT-002: another agent is running; exit quietly without touching logs.
		// already takes precedence over err (e.g. failing to close the duplicate handle).
		return 0
	}
	if err != nil {
		boot.Error("falha ao criar o mutex de instância única", "error", err.Error())
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		boot.Error("falha ao localizar o executável", "error", err.Error())
		_ = mutex.Close()
		return 1
	}
	cfg.Exe = exe
	cfg.ExeDir = filepath.Dir(exe)

	sid, err := ipc.CurrentUserSID()
	if err != nil {
		boot.Error("falha ao obter o SID do usuário", "error", err.Error())
		_ = mutex.Close()
		return 1
	}
	cfg.PipeName = ipc.PipeName(sid)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Serve must not see the signal context: on Ctrl+C/SIGTERM it stays up
	// until Server.Close in Shutdown, so Drain can still send agent.shutdown
	// (design.md, "Ajustes da revisão do T06"). serveCtx is canceled only after
	// Shutdown; the signal context goes to runTray alone.
	serveCtx, serveCancel := context.WithCancel(context.Background())
	defer serveCancel()

	cfg.Platform = realPlatform

	clock := domain.SystemClock{}
	a, err := startAgent(serveCtx, cfg, clock, os.Getenv, func() (net.Listener, error) {
		return ipc.ListenPipe(cfg.PipeName)
	})
	if err != nil {
		boot.Error("falha ao iniciar o agente", "error", err.Error())
		serveCancel()
		_ = mutex.Close()
		return 1
	}
	log := a.Logger()

	// The watcher outlives the signal context: it is released and closed only
	// after IPC and store (design.md, shutdown order).
	watcher := winapi.NewSessionEndWatcher(clock, log)
	sessionEnd, err := watcher.Start(context.Background())
	if err != nil {
		log.Error("falha ao iniciar o watcher de fim de sessão", "error", err.Error())
		sessionEnd = nil
	}

	// Start with Windows (AST-003..AST-005, AST-010): after the store is open
	// and migrated, before the tray. Failures are logged and never stop the
	// agent (AST-009).
	auto := &autostart{
		dev:      cfg.Version == "dev",
		command:  winapi.AutostartCommand(cfg.Exe, a.paths.Root, cfg.ExeDir),
		exe:      cfg.Exe,
		exeOf:    winapi.CommandExe,
		run:      winapi.NewRunValue(winapi.RunKeyPath, winapi.AutostartValueName),
		settings: a.store,
		clock:    clock,
		log:      log,
	}
	rctx, rcancel := context.WithTimeout(context.Background(), autostartTimeout)
	checked := auto.Reconcile(rctx)
	rcancel()

	// runTray returns after systray.Quit ended the message loop (step 1 of the
	// shutdown order); the reason goes to the clients in agent.shutdown.
	reason := runTray(ctx, clock, log, sessionEnd, a.IPCFailed(), func() {
		openUI(cfg.Exe, a.paths.Root, winapi.CreateDetached, log)
	}, trayAutostart{
		checked:  checked,
		disabled: auto.dev,
		// Runs on the tray loop goroutine, like "Abrir Arcadia" (short: one
		// preference write and one registry operation).
		onToggle: func() bool {
			tctx, tcancel := context.WithTimeout(context.Background(), autostartTimeout)
			defer tcancel()
			return auto.Toggle(tctx)
		},
	})

	// AGT-003: the 2 s budget is measured and reported; closing is never
	// abandoned halfway (the store must end closed). Shutdown drains (≤ 500 ms),
	// closes the IPC server and then the store.
	// The budget starts here, after trayLoop ended; the systray message loop
	// exit is outside of it.
	shutdownStart := clock.Now()
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	shutdownErr := a.Shutdown(sctx, reason)
	cancel()
	serveCancel()

	// EXE-026: non-blocking check, after Shutdown: it waited for Serve (wg.Wait),
	// so the channel state is final and a Serve that failed before Close always
	// makes the exit code 1.
	ipcFailed := false
	select {
	case <-a.IPCFailed():
		ipcFailed = true
	default:
	}
	if shutdownErr != nil {
		elapsed := clock.Now().Sub(shutdownStart)
		if errors.Is(shutdownErr, context.DeadlineExceeded) {
			log.Warn("shutdown excedeu o prazo", "timeout", shutdownTimeout.String(),
				"elapsed", elapsed.String(), "error", shutdownErr.Error())
		} else {
			log.Error("shutdown com erro", "elapsed", elapsed.String(), "error", shutdownErr.Error())
		}
	}
	code := exitCode(shutdownErr, ipcFailed)
	log.Info("shutdown concluído")

	watcher.ShutdownDone()
	if err := watcher.Close(); err != nil {
		log.Warn("falha ao fechar o watcher de fim de sessão", "error", err.Error())
	}
	if err := mutex.Close(); err != nil {
		log.Warn("falha ao fechar o mutex de instância única", "error", err.Error())
	}
	// Last step: nothing may log after the log file is closed.
	if err := a.CloseLog(); err != nil {
		boot.Error("falha ao fechar o log", "error", err.Error())
	}
	return code
}

// isPipeNameTaken reports whether a listen error means the pipe name already
// exists (another agent instance or a squatting process; IPC-019).
func isPipeNameTaken(err error) bool {
	return errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
