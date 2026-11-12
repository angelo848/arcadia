//go:build !windows

package agent

import "github.com/arcadia-launcher/arcadia/internal/tracker"

// defaultProcessLister returns nil outside Windows: no process can be adopted.
func defaultProcessLister() tracker.ProcessLister { return nil }
