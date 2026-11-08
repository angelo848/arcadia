//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

// AgentMutexName is the per-session mutex that guarantees a single agent.
const AgentMutexName = `Local\arcadia-agent`

// NamedMutex owns the handle of a named kernel mutex held for the life of the
// process. Close releases it.
type NamedMutex struct {
	mu sync.Mutex
	h  windows.Handle
}

// AcquireSingleInstance creates the named mutex. When another instance already
// owns the name it closes its own handle and returns already = true with a nil
// mutex. ERROR_ACCESS_DENIED (a mutex created by an elevated agent) also means
// already = true.
func AcquireSingleInstance(name string) (m *NamedMutex, already bool, err error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, false, fmt.Errorf("mutex name %q: %w", name, err)
	}
	h, err := windows.CreateMutex(nil, false, p)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if h != 0 {
			if cerr := windows.CloseHandle(h); cerr != nil {
				return nil, true, fmt.Errorf("close duplicate mutex %q: %w", name, cerr)
			}
		}
		return nil, true, nil
	}
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		// The name exists but was created with a DACL we cannot open (e.g. by an
		// elevated agent): another instance owns it. No handle was returned.
		if h != 0 {
			if cerr := windows.CloseHandle(h); cerr != nil {
				return nil, true, fmt.Errorf("close denied mutex %q: %w", name, cerr)
			}
		}
		return nil, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("create mutex %q: %w", name, err)
	}
	return &NamedMutex{h: h}, false, nil
}

// Close releases the mutex handle. It is idempotent.
func (m *NamedMutex) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.h == 0 {
		return nil
	}
	h := m.h
	m.h = 0
	if err := windows.CloseHandle(h); err != nil {
		return fmt.Errorf("close mutex: %w", err)
	}
	return nil
}
