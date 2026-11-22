package agent

import (
	"fmt"
	"log/slog"
)

// bigBoxOpener starts the UI directly in the Big Box (ATB-004).
type bigBoxOpener interface{ OpenBigBox() error }

// uiOpener is the real bigBoxOpener.
type uiOpener struct {
	exe, root string
	spawn     spawnFunc
}

// Path returns the executable the opener starts (for logs).
func (o uiOpener) Path() string { return o.exe }

// OpenBigBox implements bigBoxOpener.
func (o uiOpener) OpenBigBox() error {
	if o.spawn == nil {
		return fmt.Errorf("open big box: no spawn function")
	}
	return openBigBox(o.exe, o.root, o.spawn)
}

// openBigBox starts exe in UI mode with --bigbox, detached from the agent (ATB-004).
func openBigBox(exe, root string, spawn spawnFunc) error {
	if err := spawn(exe, []string{"--bigbox", "--data-dir", root}, flagDetachedProcess|flagCreateNewProcessGroup); err != nil {
		return fmt.Errorf("spawn %s: %w", exe, err)
	}
	return nil
}

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
