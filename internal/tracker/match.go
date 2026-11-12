package tracker

import (
	"path/filepath"
	"strings"
)

// Rule is one tracking rule of a game (RUL-001). Kind is one of the RuleKind values
// stored in tracking_rule.kind.
type Rule struct {
	Kind  string
	Value string
}

// Tracking rule kinds (RUL-001, RUL-002).
const (
	RuleInstallDir = "install_dir" // a process whose image lies in this folder
	RuleExePath    = "exe_path"    // a process whose image is this exact path
	RuleExeName    = "exe_name"    // a process whose image has this file name
)

// ProcInfo is one process of a snapshot: its id and full image path ("" when unreadable).
type ProcInfo struct {
	PID  uint32
	Path string
}

// ProcessLister returns a snapshot of the running processes (RUL-002). It needs no
// administrator rights.
type ProcessLister interface {
	List() ([]ProcInfo, error)
}

// matchesAny reports whether p belongs to the game described by rules (RUL-002).
// Paths compare without regard to case; install_dir matches any path under the
// folder. A process with no readable path never matches.
func matchesAny(p ProcInfo, rules []Rule) bool {
	if p.Path == "" {
		return false
	}
	path := strings.ToLower(filepath.Clean(p.Path))
	name := filepath.Base(path)
	for _, r := range rules {
		value := strings.ToLower(filepath.Clean(r.Value))
		switch r.Kind {
		case RuleInstallDir:
			if strings.HasPrefix(path, value+string(filepath.Separator)) {
				return true
			}
		case RuleExePath:
			if path == value {
				return true
			}
		case RuleExeName:
			if name == value {
				return true
			}
		}
	}
	return false
}
