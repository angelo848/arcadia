//go:build windows

package agent

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

const (
	focusPollInterval = 250 * time.Millisecond
	focusTimeout      = 30 * time.Second
)

// focusLaunchedWindow focuses the first window of a process this agent started.
func focusLaunchedWindow(ctx context.Context, log *slog.Logger) {
	ok := winapi.FocusChildWindow(ctx, windows.GetCurrentProcessId(), focusPollInterval, focusTimeout)
	log.Debug("janela do jogo ao primeiro plano", "focused", ok)
}
