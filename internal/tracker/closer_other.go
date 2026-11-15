//go:build !windows

package tracker

import (
	"errors"
	"fmt"
)

// newWindowCloser returns a closer that always fails: windows only exist on
// Windows.
func newWindowCloser() windowCloser { return unsupportedCloser{} }

// newProcessKiller returns a killer that always fails outside Windows.
func newProcessKiller() processKiller { return unsupportedCloser{} }

type unsupportedCloser struct{}

func (unsupportedCloser) PostClose(map[uint32]struct{}) (int, error) {
	return 0, fmt.Errorf("tracker: post WM_CLOSE: %w", errors.ErrUnsupported)
}

func (unsupportedCloser) Kill(uint32, string) error {
	return fmt.Errorf("tracker: terminate process: %w", errors.ErrUnsupported)
}
