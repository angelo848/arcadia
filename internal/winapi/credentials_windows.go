//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ErrCredentialNotFound is returned when the Credential Manager has no entry with the name.
var ErrCredentialNotFound = errors.New("winapi: credencial não encontrada")

var (
	advapi32        = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1 // CRED_TYPE_GENERIC
	credPersistLocalMachine = 2 // CRED_PERSIST_LOCAL_MACHINE: kept on this computer only
	credMaxBlobBytes        = 2560
)

// credential mirrors CREDENTIALW (wincred.h); only the fields used here are set.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// CredentialStore keeps secrets in the Windows Credential Manager, as generic credentials on this
// computer (MID-001). It satisfies scrapers.KeyStore.
type CredentialStore struct{}

// Set writes the value under the name, replacing an existing entry.
func (CredentialStore) Set(name, value string) error {
	target, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("winapi: nome da credencial: %w", err)
	}
	blob := []byte(value)
	if len(blob) == 0 || len(blob) > credMaxBlobBytes {
		return fmt.Errorf("winapi: valor da credencial com %d bytes fora do limite", len(blob))
	}
	c := credential{
		Type:               credTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
	}
	r, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&c)), 0)
	runtime.KeepAlive(blob)
	runtime.KeepAlive(target)
	if r == 0 {
		return fmt.Errorf("winapi: CredWriteW: %w", callErr)
	}
	return nil
}

// Get returns the value under the name, or ErrCredentialNotFound.
func (CredentialStore) Get(name string) (string, error) {
	target, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return "", fmt.Errorf("winapi: nome da credencial: %w", err)
	}
	var pcred *credential
	r, _, callErr := procCredReadW.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0,
		uintptr(unsafe.Pointer(&pcred)))
	runtime.KeepAlive(target)
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return "", ErrCredentialNotFound
		}
		return "", fmt.Errorf("winapi: CredReadW: %w", callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(pcred)))
	if pcred.CredentialBlobSize == 0 {
		return "", nil
	}
	blob := unsafe.Slice(pcred.CredentialBlob, pcred.CredentialBlobSize)
	return string(blob), nil
}

// Delete removes the entry under the name; a missing entry is not an error.
func (CredentialStore) Delete(name string) error {
	target, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("winapi: nome da credencial: %w", err)
	}
	r, _, callErr := procCredDeleteW.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0)
	runtime.KeepAlive(target)
	if r == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return nil
		}
		return fmt.Errorf("winapi: CredDeleteW: %w", callErr)
	}
	return nil
}
