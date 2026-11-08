package main

import (
	"log/slog"
	"path/filepath"
)

// Win32 process creation flags (Windows SDK values), kept as literals so this
// file builds on every GOOS.
const (
	flagDetachedProcess       uint32 = 0x00000008 // DETACHED_PROCESS
	flagCreateNewProcessGroup uint32 = 0x00000200 // CREATE_NEW_PROCESS_GROUP
)

// uiExeName is the desktop UI executable, expected next to the agent.
const uiExeName = "arcadia.exe"

// openUI starts exeDir\arcadia.exe as an independent process (AGT-007). If the
// executable is missing it logs an error and does not spawn (AGT-008).
func openUI(exeDir string, stat func(string) error, spawn spawnFunc, log *slog.Logger) {
	exe := filepath.Join(exeDir, uiExeName)
	if err := stat(exe); err != nil {
		log.Error("arcadia.exe não encontrado", "path", exe, "error", err.Error())
		return
	}
	if err := spawn(exe, nil, flagDetachedProcess|flagCreateNewProcessGroup); err != nil {
		log.Error("falha ao abrir o Arcadia", "path", exe, "error", err.Error())
		return
	}
	log.Info("Arcadia aberto", "path", exe)
}
