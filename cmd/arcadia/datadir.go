package main

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/arcadia-launcher/arcadia/internal/logging"
	"github.com/arcadia-launcher/arcadia/internal/paths"
)

// desktopLogFile is the UI log file inside Paths.Logs (DSK-009).
const desktopLogFile = "desktop.log"

// desktopConfig holds the command-line inputs of the UI. DataDir (--data-dir)
// is resolved with paths.Resolve, like the agent does.
type desktopConfig struct {
	DataDir string // --data-dir
	Exe     string // this executable (os.Executable); the agent is started from it (EXE-007)
	ExeDir  string // directory of Exe
	BigBox  bool   // --bigbox: open straight into the Big Box (ATB-008)
}

// setupLogging opens the UI log at p.Logs/desktop.log with the shared rotation
// and a fixed Info level (DSK-009).
func setupLogging(p paths.Paths) (*slog.Logger, io.Closer, error) {
	log, closer, err := logging.New(logging.Options{Dir: p.Logs, File: desktopLogFile, Level: slog.LevelInfo})
	if err != nil {
		return nil, nil, fmt.Errorf("desktop: setup logging: %w", err)
	}
	return log, closer, nil
}

// warnIfDataDirDiffers logs one warning and returns true when the agent's data
// directory is not the UI's root (DSK-006). Both are compared after
// filepath.Clean, case-insensitively on Windows; an empty agentDataDir differs.
func warnIfDataDirDiffers(log *slog.Logger, root, agentDataDir string) bool {
	if agentDataDir != "" && sameDir(root, agentDataDir) {
		return false
	}
	log.Warn("agente usa outra pasta de dados", "ui_data_dir", root, "agent_data_dir", agentDataDir)
	return true
}

// sameDir compares two directory paths after filepath.Clean.
func sameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
