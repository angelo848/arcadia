// Package paths resolves Arcadia's data directory layout (ADR 0006).
package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// EnvDataDir is the environment variable that overrides the data directory
// when no flag is given.
const EnvDataDir = "ARCADIA_DATA_DIR"

// Options holds every input used to resolve the data directory, injected so
// resolution can be tested without touching the real environment.
type Options struct {
	// Flag is the value of --data-dir; it has the highest precedence.
	Flag string
	// ExeDir is the directory of the running executable; <ExeDir>/data is the fallback.
	ExeDir string
	// Getenv looks up environment variables; nil means os.Getenv.
	Getenv func(string) string
}

// Paths is the resolved, absolute data directory layout.
type Paths struct {
	Root  string // data directory
	DB    string // Root/library.db
	Media string // Root/media
	Logs  string // Root/logs
}

// Resolve picks the data directory with precedence flag > EnvDataDir
// (empty ignored) > <ExeDir>/data and returns its absolute layout.
// It returns an error when Flag, the env variable and ExeDir are all empty.
func Resolve(opts Options) (Paths, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	env := getenv(EnvDataDir)

	var root string
	switch {
	case opts.Flag != "":
		root = opts.Flag
	case env != "":
		root = env
	case opts.ExeDir != "":
		root = filepath.Join(opts.ExeDir, "data")
	default:
		return Paths{}, errors.New("resolve data dir: no flag, " + EnvDataDir + " or exe dir given")
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve data dir %q: %w", root, err)
	}

	return Paths{
		Root:  abs,
		DB:    filepath.Join(abs, "library.db"),
		Media: filepath.Join(abs, "media"),
		Logs:  filepath.Join(abs, "logs"),
	}, nil
}

// EnsureDirs creates Root, Media and Logs if missing. It is idempotent and
// fails if any of them exists as a non-directory.
func (p Paths) EnsureDirs() error {
	for _, dir := range []string{p.Root, p.Media, p.Logs} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("ensure dir %q: %w", dir, err)
		}
	}
	return nil
}
