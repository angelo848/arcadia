//go:build windows

// Command arcadia-agent is a temporary shell around internal/agent (removed in
// T08 of docs/specs/single-exe, when cmd/arcadia becomes the single executable).
//
// Release build (no console window, version stamped):
//
//	go build -ldflags "-H windowsgui -X main.version=<v>" -o arcadia-agent.exe ./cmd/agent
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
