//go:build windows

package tracker

import (
	"errors"
	"fmt"
)

// NewLauncher returns the Windows Launcher.
//
// Placeholder until T09 (Job Object, IOCP and CreateProcess): it always fails,
// like the non-Windows launcher, so that the portable coordinator builds and
// is testable on Windows.
func NewLauncher() Launcher { return pendingLauncher{} }

type pendingLauncher struct{}

func (pendingLauncher) Prepare(LaunchSpec) (Job, error) {
	return nil, &LaunchError{Err: fmt.Errorf("tracker: windows launcher (T09): %w", errors.ErrUnsupported)}
}
