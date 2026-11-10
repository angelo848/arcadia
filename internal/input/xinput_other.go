//go:build !windows

package input

import (
	"errors"
	"fmt"
)

// NewXInput is unsupported outside Windows.
func NewXInput() (PadAPI, error) {
	return nil, fmt.Errorf("xinput: %w", errors.ErrUnsupported)
}
