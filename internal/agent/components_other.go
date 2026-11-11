//go:build !windows

package agent

import (
	"log/slog"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// realPlatform returns an empty platformParts on non-Windows platforms.
func realPlatform(clock domain.Clock, log *slog.Logger) platformParts {
	return platformParts{}
}
