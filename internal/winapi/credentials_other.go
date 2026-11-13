//go:build !windows

package winapi

import (
	"errors"
	"fmt"
)

// ErrCredentialNotFound is returned when the Credential Manager has no entry with the name.
// Outside Windows it is never returned: every call fails with errNotWindows.
var ErrCredentialNotFound = errors.New("winapi: credencial não encontrada")

var errNotWindows = errors.New("winapi: o Credential Manager só existe no Windows")

// CredentialStore is the Windows Credential Manager. Outside Windows every call fails.
type CredentialStore struct{}

// Set fails outside Windows.
func (CredentialStore) Set(name, value string) error {
	return fmt.Errorf("set %s: %w", name, errNotWindows)
}

// Get fails outside Windows.
func (CredentialStore) Get(name string) (string, error) {
	return "", fmt.Errorf("get %s: %w", name, errNotWindows)
}

// Delete fails outside Windows.
func (CredentialStore) Delete(name string) error {
	return fmt.Errorf("delete %s: %w", name, errNotWindows)
}
