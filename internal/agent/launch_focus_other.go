//go:build !windows

package agent

import (
	"context"
	"log/slog"
)

// focusLaunchedWindow does nothing: the agent only runs on Windows.
func focusLaunchedWindow(context.Context, *slog.Logger) {}
