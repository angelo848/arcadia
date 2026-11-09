//go:build windows

package main

import (
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/sys/windows"
)

// spawnFunc creates a process; the real one is winapi.CreateDetached.
type spawnFunc func(exe string, args []string, flags uint32) error

// agentCommand builds the command that starts the agent decoupled from the UI
// (DSK-004, EXE-007): this same executable (exe, from os.Executable) with
// --agent --data-dir root, and flags
// DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP|CREATE_BREAKAWAY_FROM_JOB.
func agentCommand(exe, root string) (string, []string, uint32) {
	args := []string{"--agent", "--data-dir", root}
	flags := uint32(windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_BREAKAWAY_FROM_JOB)
	return exe, args, flags
}

// startAgentProcess starts the agent with spawn. When breaking away from the
// current job is denied (ERROR_ACCESS_DENIED), it logs a warning and tries once
// more without CREATE_BREAKAWAY_FROM_JOB (DSK-005); other errors are returned as is.
func startAgentProcess(spawn spawnFunc, exe, root string, log *slog.Logger) error {
	exe, args, flags := agentCommand(exe, root)
	err := spawn(exe, args, flags)
	if err == nil {
		return nil
	}
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return fmt.Errorf("start agent %q: %w", exe, err)
	}
	log.Warn("criação do agente fora do job negada; tentando sem CREATE_BREAKAWAY_FROM_JOB (o agente pode encerrar junto com o job atual)",
		"exe", exe, "error", err.Error())
	if err := spawn(exe, args, flags&^windows.CREATE_BREAKAWAY_FROM_JOB); err != nil {
		return fmt.Errorf("start agent %q without breakaway: %w", exe, err)
	}
	return nil
}
