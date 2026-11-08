//go:build windows

package winapi_test

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// testMutexName returns a unique Local\ mutex name so tests never collide with a
// real agent holding winapi.AgentMutexName.
func testMutexName(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf(`Local\arcadia-agent-test-%016x`, rand.Uint64())
	if name == winapi.AgentMutexName {
		t.Fatalf("nome de teste colide com AgentMutexName")
	}
	return name
}

// acquire calls AcquireSingleInstance and fails the test on error. Closing the
// returned mutex is up to the caller.
func acquire(t *testing.T, name string) (*winapi.NamedMutex, bool) {
	t.Helper()
	m, already, err := winapi.AcquireSingleInstance(name)
	if err != nil {
		t.Fatalf("AcquireSingleInstance(%q): %v", name, err)
	}
	return m, already
}

func TestSingleInstance(t *testing.T) {
	t.Parallel()

	t.Run("AGT-002/primeira aquisição não é duplicada", func(t *testing.T) {
		t.Parallel()
		m, already := acquire(t, testMutexName(t))
		if already {
			t.Fatal("already = true na primeira aquisição")
		}
		if m == nil {
			t.Fatal("NamedMutex nil na primeira aquisição")
		}
		if err := m.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	t.Run("AGT-002/segunda aquisição detecta instância", func(t *testing.T) {
		t.Parallel()
		name := testMutexName(t)
		first, already := acquire(t, name)
		if already || first == nil {
			t.Fatalf("primeira aquisição: m=%v already=%v", first, already)
		}
		defer first.Close()

		second, already := acquire(t, name)
		if !already {
			t.Fatal("already = false na segunda aquisição com a primeira ainda aberta")
		}
		// Per the T09 signature, AcquireSingleInstance closes its own handle when
		// already is true; a returned mutex, if any, must not hold the name.
		if second != nil {
			_ = second.Close()
		}
	})

	t.Run("AGT-002/após Close readquire", func(t *testing.T) {
		t.Parallel()
		name := testMutexName(t)
		first, already := acquire(t, name)
		if already || first == nil {
			t.Fatalf("primeira aquisição: m=%v already=%v", first, already)
		}

		// The duplicate attempt must not keep a handle alive: otherwise the
		// kernel object (and the name) survives first.Close and the next
		// acquisition would still report already = true.
		dup, already := acquire(t, name)
		if !already {
			t.Fatal("already = false na aquisição duplicada")
		}
		_ = dup // intentionally not closed: AcquireSingleInstance must have closed it

		if err := first.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		again, already := acquire(t, name)
		if already {
			t.Fatal("already = true após Close da única instância (handle da tentativa duplicada vazou?)")
		}
		if again == nil {
			t.Fatal("NamedMutex nil na readquisição")
		}
		if err := again.Close(); err != nil {
			t.Fatalf("Close da readquisição: %v", err)
		}
	})

	t.Run("AGT-002/acesso negado ao mutex conta como instância existente", func(t *testing.T) {
		t.Parallel()
		name := testMutexName(t)
		p, err := windows.UTF16PtrFromString(name)
		if err != nil {
			t.Fatalf("UTF16PtrFromString: %v", err)
		}

		// Stand-in for a mutex created by an elevated agent: a protected, empty
		// DACL grants nobody MUTEX_ALL_ACCESS (the owner keeps only READ_CONTROL
		// and WRITE_DAC), so opening the existing object by name is denied.
		sd, err := windows.SecurityDescriptorFromString("D:P")
		if err != nil {
			t.Fatalf("SecurityDescriptorFromString: %v", err)
		}
		sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
		sa.Length = uint32(unsafe.Sizeof(*sa))
		h, err := windows.CreateMutex(sa, false, p)
		if err != nil {
			t.Fatalf("CreateMutex com DACL vazia: %v", err)
		}
		defer windows.CloseHandle(h)

		// Empirical check of the premise (design.md, revisão da T09).
		h2, rawErr := windows.CreateMutex(nil, false, p)
		if h2 != 0 {
			windows.CloseHandle(h2)
		}
		t.Logf("CreateMutexW sobre mutex com DACL vazia: handle=%#x err=%v (errno %d)", h2, rawErr, errnoOf(rawErr))
		if !errors.Is(rawErr, windows.ERROR_ACCESS_DENIED) {
			t.Fatalf("premissa do teste: CreateMutexW devolveu %v, quer ERROR_ACCESS_DENIED", rawErr)
		}

		m, already, err := winapi.AcquireSingleInstance(name)
		if err != nil {
			t.Fatalf("AcquireSingleInstance: err = %v, quer nil (ACCESS_DENIED = instância existente)", err)
		}
		if !already {
			t.Fatal("already = false com ERROR_ACCESS_DENIED")
		}
		if m != nil {
			_ = m.Close()
			t.Fatal("NamedMutex não nil com ERROR_ACCESS_DENIED")
		}
	})
}

func errnoOf(err error) uintptr {
	var e windows.Errno
	if errors.As(err, &e) {
		return uintptr(e)
	}
	return 0
}
