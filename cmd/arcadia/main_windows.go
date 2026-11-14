//go:build windows

// Command arcadia is the single Arcadia executable (EXE-001). With --agent it
// runs the tray agent (internal/agent); otherwise it runs the desktop UI
// (Wails), which starts the agent (this same executable with --agent) when it
// is not running, talks to it over the per-user named pipe and reads the
// library without migrating it (ADR 0010). Closing the UI never stops the agent.
//
// Build with the Wails CLI: cd cmd/arcadia && wails build -ldflags "-X main.version=<v>"
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/arcadia-launcher/arcadia/frontend"
	"github.com/arcadia-launcher/arcadia/internal/agent"
	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run selects the mode before anything else (no mutex, log, paths or Wails
// before the choice, EXE-004) and returns the process exit code.
func run(args []string) int {
	if isAgentMode(args) {
		return agent.Run(agent.Options{Args: args, Version: version})
	}
	return runUI(args)
}

// runUI starts the Wails app and returns the process exit code. It never
// acquires the agent mutex nor listens on the pipe (EXE-004).
func runUI(args []string) int {
	// Bootstrap logger for failures before desktop.log exists.
	boot := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Wails may pass its own flags (wails dev, bindings generation); unknown
	// flags are tolerated instead of aborting.
	var cfg desktopConfig
	fset := flag.NewFlagSet("arcadia", flag.ContinueOnError)
	fset.SetOutput(io.Discard)
	fset.StringVar(&cfg.DataDir, "data-dir", "", "data directory (default: $ARCADIA_DATA_DIR or <exe dir>\\data)")
	if err := fset.Parse(args); err != nil {
		boot.Warn("argumentos ignorados", "error", err.Error())
	}

	exe, err := os.Executable()
	if err != nil {
		boot.Error("falha ao localizar o executável", "error", err.Error())
		return 1
	}
	cfg.Exe = exe
	cfg.ExeDir = filepath.Dir(exe)

	assets, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		boot.Error("falha ao abrir os assets do frontend", "error", err.Error())
		return 1
	}

	// Best effort: lets the instance that already runs come to the front when
	// this process turns out to be a second instance (EXE-011).
	if err := winapi.AllowSetForegroundWindow(winapi.ASFWAny); err != nil {
		boot.Debug("falha ao liberar a troca de foco", "error", err.Error())
	}

	app := NewApp(cfg)
	appOptions := &options.App{
		Title:       "Arcadia",
		AssetServer: &assetserver.Options{Assets: assets, Handler: http.HandlerFunc(app.serveMedia)},
		OnStartup:   app.startup,
		OnShutdown:  app.shutdown,
		Bind:        []any{app},
		// A second UI process exits inside wails.Run, before OnStartup, and the
		// first one is notified (EXE-011, EXE-012).
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               singleInstanceID,
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
	}
	applyWindowOptions(appOptions)
	err = wails.Run(appOptions)
	if err != nil {
		boot.Error("falha ao executar a UI", "error", err.Error())
		return 1
	}
	return 0
}

// defaultPlatform wires the Windows pieces: the per-user pipe, detached agent
// creation and Wails events.
func defaultPlatform() platform {
	return platform{
		newProbe: func() (pinger, error) {
			sid, err := ipc.CurrentUserSID()
			if err != nil {
				return nil, fmt.Errorf("desktop: user sid: %w", err)
			}
			name := ipc.PipeName(sid)
			return probePinger{dial: func(ctx context.Context) (net.Conn, error) {
				return ipc.DialPipe(ctx, name)
			}}, nil
		},
		newClient: func(log *slog.Logger) (ipcClient, error) {
			sid, err := ipc.CurrentUserSID()
			if err != nil {
				return nil, fmt.Errorf("desktop: user sid: %w", err)
			}
			return ipc.NewClient(ipc.PipeDialer(ipc.PipeName(sid)), domain.SystemClock{}, log), nil
		},
		startAgent: func(exe, root string, log *slog.Logger) error {
			return startAgentProcess(winapi.CreateDetached, exe, root, log)
		},
		emit: func(ctx context.Context, event string, data any) {
			wailsruntime.EventsEmit(ctx, event, data)
		},
		quit: wailsruntime.Quit,
		showWindow: func(ctx context.Context) {
			wailsruntime.WindowUnminimise(ctx)
			wailsruntime.WindowShow(ctx)
		},
		openFile: func(ctx context.Context, title string, filters []fileFilter) (string, error) {
			wf := make([]wailsruntime.FileFilter, 0, len(filters))
			for _, f := range filters {
				wf = append(wf, wailsruntime.FileFilter{DisplayName: f.DisplayName, Pattern: f.Pattern})
			}
			path, err := wailsruntime.OpenFileDialog(ctx, wailsruntime.OpenDialogOptions{Title: title, Filters: wf})
			if err != nil {
				return "", fmt.Errorf("desktop: open file dialog: %w", err)
			}
			return path, nil
		},
		openDir: func(ctx context.Context, title string) (string, error) {
			path, err := wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{Title: title})
			if err != nil {
				return "", fmt.Errorf("desktop: open directory dialog: %w", err)
			}
			return path, nil
		},
	}
}

// singleInstanceID is the fixed Wails single-instance id of the UI (EXE-011).
const singleInstanceID = "arcadia-ui-5b0f7c1e"

// onSecondInstance is the Wails OnSecondInstanceLaunch hook. The second
// instance's arguments are ignored (design.md, "Instância única da UI").
func (a *App) onSecondInstance(options.SecondInstanceData) {
	a.secondInstance()
}
