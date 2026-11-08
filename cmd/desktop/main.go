//go:build windows

// Command arcadia is the Arcadia desktop UI (Wails). It starts the agent when it
// is not running, talks to it over the per-user named pipe and reads the
// library without migrating it (ADR 0010). Closing the UI never stops the agent.
//
// Build with the Wails CLI (from cmd/desktop): wails build.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/arcadia-launcher/arcadia/frontend"
	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

func main() {
	os.Exit(run())
}

// run starts the Wails app and returns the process exit code.
func run() int {
	// Bootstrap logger for failures before desktop.log exists.
	boot := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Wails may pass its own flags (wails dev, bindings generation); unknown
	// flags are tolerated instead of aborting.
	var cfg desktopConfig
	fset := flag.NewFlagSet("arcadia", flag.ContinueOnError)
	fset.SetOutput(io.Discard)
	fset.StringVar(&cfg.DataDir, "data-dir", "", "data directory (default: $ARCADIA_DATA_DIR or <exe dir>\\data)")
	if err := fset.Parse(os.Args[1:]); err != nil {
		boot.Warn("argumentos ignorados", "error", err.Error())
	}

	exe, err := os.Executable()
	if err != nil {
		boot.Error("falha ao localizar o executável", "error", err.Error())
		return 1
	}
	cfg.ExeDir = filepath.Dir(exe)

	assets, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		boot.Error("falha ao abrir os assets do frontend", "error", err.Error())
		return 1
	}

	app := NewApp(cfg)
	err = wails.Run(&options.App{
		Title:       "Arcadia",
		Width:       1024,
		Height:      768,
		AssetServer: &assetserver.Options{Assets: assets},
		OnStartup:   app.startup,
		OnShutdown:  app.shutdown,
		Bind:        []any{app},
	})
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
		startAgent: func(exeDir, root string, log *slog.Logger) error {
			return startAgentProcess(winapi.CreateDetached, exeDir, root, log)
		},
		emit: func(ctx context.Context, event string, data any) {
			wailsruntime.EventsEmit(ctx, event, data)
		},
	}
}
