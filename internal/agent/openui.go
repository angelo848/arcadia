package agent

import (
	"log/slog"
)

// Win32 process creation flags (Windows SDK values), kept as literals so this
// file builds on every GOOS.
const (
	flagDetachedProcess       uint32 = 0x00000008 // DETACHED_PROCESS
	flagCreateNewProcessGroup uint32 = 0x00000200 // CREATE_NEW_PROCESS_GROUP
)

// openUI starts exe (the running executable) in UI mode, as an independent
// process, pointing it at the agent data directory root (EXE-009). A spawn
// failure is logged and does not stop the agent (EXE-010).
func openUI(exe, root string, spawn spawnFunc, log *slog.Logger) {
	if err := spawn(exe, []string{"--data-dir", root}, flagDetachedProcess|flagCreateNewProcessGroup); err != nil {
		log.Error("falha ao abrir o Arcadia", "path", exe, "error", err.Error())
		return
	}
	log.Info("Arcadia aberto", "path", exe)
}
