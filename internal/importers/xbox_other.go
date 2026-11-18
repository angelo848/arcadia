//go:build !windows

package importers

import "errors"

// xboxInstallRoots: the Xbox app exists only on Windows.
func xboxInstallRoots() ([]string, error) { return nil, errors.New("xbox: windows only") }

// realDir: nothing to resolve outside Windows.
func realDir(dir string) string { return dir }
