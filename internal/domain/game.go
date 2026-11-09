package domain

import (
	"strings"
)

// DefaultGameTitle returns the default title for a game executable: the file
// name without one trailing ".exe" (case-insensitive). Both '\' and '/' are
// treated as path separators regardless of the host OS.
func DefaultGameTitle(path string) string {
	name := path
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:]
	}
	const ext = ".exe"
	if len(name) >= len(ext) && strings.EqualFold(name[len(name)-len(ext):], ext) {
		name = name[:len(name)-len(ext)]
	}
	return name
}
