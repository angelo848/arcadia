//go:build !windows

package agent

import "github.com/arcadia-launcher/arcadia/internal/tracker"

// defaultProcessLister returns nil outside Windows: no process can be adopted.
func defaultProcessLister() tracker.ProcessLister { return nil }

// defaultSteamRunning returns nil outside Windows: there is no Steam signal (STM-001).
func defaultSteamRunning() func(appID string) (bool, bool, error) { return nil }
