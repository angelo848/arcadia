//go:build windows

package agent

import (
	"github.com/arcadia-launcher/arcadia/internal/tracker"
	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// winProcesses adapts the Toolhelp snapshot to the tracker's ProcessLister (RUL-002).
type winProcesses struct{}

// defaultProcessLister returns the process snapshot source of this machine.
func defaultProcessLister() tracker.ProcessLister { return winProcesses{} }

// List returns the running processes with their image paths.
func (winProcesses) List() ([]tracker.ProcInfo, error) {
	procs, err := winapi.ListProcesses()
	if err != nil {
		return nil, err
	}
	out := make([]tracker.ProcInfo, 0, len(procs))
	for _, p := range procs {
		out = append(out, tracker.ProcInfo{PID: p.PID, Path: p.Path})
	}
	return out, nil
}
