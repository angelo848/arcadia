//go:build !windows

package tracker

import (
	"errors"
	"fmt"
)

// NewLauncher returns a Launcher that always fails: game tracking needs Job
// Objects, which only exist on Windows.
func NewLauncher() Launcher { return unsupportedLauncher{} }

type unsupportedLauncher struct{}

func (unsupportedLauncher) Prepare(LaunchSpec) (Job, error) {
	return nil, &LaunchError{Err: fmt.Errorf("tracker: job objects: %w", errors.ErrUnsupported)}
}
