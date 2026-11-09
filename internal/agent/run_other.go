//go:build !windows

package agent

import "log/slog"

// Run reports that the agent is unsupported and returns 1: the agent depends
// on Win32 (named pipe, mutex, tray); this file keeps the package building and
// vetting on other systems.
func Run(Options) int {
	slog.Error("arcadia: agente suportado apenas no Windows")
	return 1
}

// isPipeNameTaken has no pipe name semantics outside Windows.
func isPipeNameTaken(error) bool { return false }
