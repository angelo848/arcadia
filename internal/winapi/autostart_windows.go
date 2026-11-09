//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	// RunKeyPath is the HKCU subkey whose values Windows runs at logon (AST-003).
	RunKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	// AutostartValueName is the value name of Arcadia's own Run entry.
	AutostartValueName = "Arcadia"
)

// RunValue is a REG_SZ value under HKCU\keyPath. It keeps no registry handle:
// every operation opens the key, operates and closes it (AST-009).
type RunValue struct {
	keyPath string
	name    string
}

// NewRunValue returns the value name under HKCU\keyPath.
func NewRunValue(keyPath, name string) *RunValue {
	return &RunValue{keyPath: keyPath, name: name}
}

// Get reads the value. An absent key or value yields ok=false and a nil
// error; a value that is not REG_SZ is an error. Get never creates the key.
func (r *RunValue) Get() (value string, ok bool, err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, r.keyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("open HKCU\\%s: %w", r.keyPath, err)
	}
	defer k.Close()

	v, typ, err := k.GetStringValue(r.name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read HKCU\\%s\\%s: %w", r.keyPath, r.name, err)
	}
	if typ != registry.SZ {
		return "", false, fmt.Errorf("read HKCU\\%s\\%s: value type %d is not REG_SZ: %w", r.keyPath, r.name, typ, registry.ErrUnexpectedType)
	}
	return v, true, nil
}

// Set writes value as REG_SZ, creating the key when needed.
func (r *RunValue) Set(value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, r.keyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("create HKCU\\%s: %w", r.keyPath, err)
	}
	defer k.Close()

	if err := k.SetStringValue(r.name, value); err != nil {
		return fmt.Errorf("write HKCU\\%s\\%s: %w", r.keyPath, r.name, err)
	}
	return nil
}

// Delete removes the value. An absent key or value is not an error.
func (r *RunValue) Delete() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, r.keyPath, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open HKCU\\%s: %w", r.keyPath, err)
	}
	defer k.Close()

	err = k.DeleteValue(r.name)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("delete HKCU\\%s\\%s: %w", r.keyPath, r.name, err)
	}
	return nil
}

// AutostartCommand builds the Run entry command line (AST-002): the exe always
// quoted, then " --agent", then " --data-dir <root>" (escaped by
// windows.EscapeArg) unless root is exeDir\data, compared after
// filepath.Clean and case-insensitively.
func AutostartCommand(exe, root, exeDir string) string {
	cmd := `"` + exe + `" --agent`
	defaultRoot := filepath.Join(exeDir, "data")
	if strings.EqualFold(filepath.Clean(root), filepath.Clean(defaultRoot)) {
		return cmd
	}
	return cmd + " --data-dir " + windows.EscapeArg(root)
}

// CommandExe returns the first token (the executable) of a command line, as
// parsed by windows.DecomposeCommandLine. An empty command line is an error.
func CommandExe(cmdline string) (string, error) {
	args, err := windows.DecomposeCommandLine(cmdline)
	if err != nil {
		return "", fmt.Errorf("parse command line %q: %w", cmdline, err)
	}
	if len(args) == 0 || args[0] == "" {
		return "", fmt.Errorf("parse command line %q: no executable", cmdline)
	}
	return args[0], nil
}
