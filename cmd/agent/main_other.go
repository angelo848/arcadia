//go:build !windows

package main

import (
	"log/slog"
	"os"
)

// The agent depends on Win32 (named pipe, mutex, tray); on other systems the
// binary only reports that it is unsupported, so the package still builds and vets.
func main() {
	slog.Error("arcadia-agent: suportado apenas no Windows")
	os.Exit(1)
}

// isPipeNameTaken has no pipe name semantics outside Windows.
func isPipeNameTaken(error) bool { return false }
