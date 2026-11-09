//go:build !windows

// Command fake-game is a Windows-only test helper; on other platforms it exits with 1.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "fake-game: Windows only")
	os.Exit(1)
}
