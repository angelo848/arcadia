//go:build !windows

// Command arcadia-agent is a temporary shell around internal/agent (removed in
// T08 of docs/specs/single-exe); the agent only runs on Windows.
package main

import (
	"os"

	"github.com/arcadia-launcher/arcadia/internal/agent"
)

// version is reported by agent.status (AGT-004); release builds stamp it.
var version = "dev"

func main() {
	os.Exit(agent.Run(agent.Options{Args: os.Args[1:], Version: version}))
}
