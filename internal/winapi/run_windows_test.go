//go:build windows

package winapi_test

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"

	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// Compile-time checks of the T04 signatures that have no automated behavior
// test (EXE-011 is verified manually).
var (
	_ func(uint32) error = winapi.AllowSetForegroundWindow
	_ uint32             = winapi.ASFWAny
)

func TestRunConstants(t *testing.T) {
	t.Parallel()

	t.Run("ADM-004/entrada Run antiga em HKCU com o valor Arcadia", func(t *testing.T) {
		t.Parallel()
		if got, want := winapi.RunKeyPath, `Software\Microsoft\Windows\CurrentVersion\Run`; got != want {
			t.Errorf("RunKeyPath = %q, quer %q", got, want)
		}
		if got, want := winapi.RunValueName, "Arcadia"; got != want {
			t.Errorf("RunValueName = %q, quer %q", got, want)
		}
		if got, want := winapi.ASFWAny, uint32(0xFFFFFFFF); got != want {
			t.Errorf("ASFWAny = %#x, quer %#x", got, want)
		}
	})
}

func TestCommandExe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmdline string
		want    string
	}{
		{
			name:    "AST-005/primeiro token entre aspas",
			cmdline: `"C:\Program Files\Arcadia\arcadia.exe" --agent --data-dir "E:\Minha Biblioteca"`,
			want:    `C:\Program Files\Arcadia\arcadia.exe`,
		},
		{
			name:    "AST-005/primeiro token sem aspas",
			cmdline: `C:\Jogos\Arcadia\arcadia.exe --agent`,
			want:    `C:\Jogos\Arcadia\arcadia.exe`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := winapi.CommandExe(tc.cmdline)
			if err != nil {
				t.Fatalf("CommandExe(%q): %v", tc.cmdline, err)
			}
			if got != tc.want {
				t.Errorf("CommandExe(%q) = %q, quer %q", tc.cmdline, got, tc.want)
			}
		})
	}

	t.Run("AST-005/linha vazia retorna erro", func(t *testing.T) {
		t.Parallel()
		got, err := winapi.CommandExe("")
		if err == nil {
			t.Fatalf("CommandExe(\"\") = %q, nil; quer erro", got)
		}
	})

}

// testRunKeyPath returns a throwaway HKCU subkey path, removed in t.Cleanup.
// It never points at the real Run key.
func testRunKeyPath(t *testing.T) string {
	t.Helper()
	path := fmt.Sprintf(`Software\ArcadiaTest-%016x`, rand.Uint64())
	if strings.EqualFold(path, winapi.RunKeyPath) || !strings.HasPrefix(path, `Software\ArcadiaTest-`) {
		t.Fatalf("subchave de teste inválida: %q", path)
	}
	t.Cleanup(func() {
		err := registry.DeleteKey(registry.CURRENT_USER, path)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			t.Errorf("limpeza: DeleteKey(HKCU\\%s): %v", path, err)
		}
	})
	return path
}

// rawValue reads a value directly with the registry package (independent of
// RunValue). exists=false when the key or the value is absent.
func rawValue(t *testing.T, path, name string) (value string, valtype uint32, exists bool) {
	t.Helper()
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", 0, false
	}
	if err != nil {
		t.Fatalf("OpenKey(HKCU\\%s): %v", path, err)
	}
	defer k.Close()
	v, typ, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", 0, false
	}
	if err != nil {
		t.Fatalf("GetStringValue(%q): %v", name, err)
	}
	return v, typ, true
}

// createKey creates the throwaway subkey without any value.
func createKey(t *testing.T, path string) registry.Key {
	t.Helper()
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("CreateKey(HKCU\\%s): %v", path, err)
	}
	return k
}

// processHandleCount (GetProcessHandleCount) is shared with process_windows_test.go.

// TestRunValue is intentionally not parallel: the handle-count case measures
// the whole process, and sequential top-level tests run while every parallel
// test of the package is still paused, so no other test opens handles meanwhile.
func TestRunValue(t *testing.T) {
	const name = "Arcadia"

	t.Run("AST-003/Set grava REG_SZ e Get lê o mesmo valor", func(t *testing.T) {
		path := testRunKeyPath(t)
		rv := winapi.NewRunValue(path, name)
		const want = `"C:\Program Files\Arcadia\arcadia.exe" --agent --data-dir "E:\Minha Biblioteca"`

		if err := rv.Set(want); err != nil {
			t.Fatalf("Set: %v", err)
		}
		raw, typ, exists := rawValue(t, path, name)
		if !exists {
			t.Fatal("valor não existe no registro após Set")
		}
		if typ != registry.SZ {
			t.Errorf("tipo = %d, quer REG_SZ (%d)", typ, registry.SZ)
		}
		if raw != want {
			t.Errorf("registro = %q, quer %q", raw, want)
		}

		got, ok, err := rv.Get()
		if err != nil || !ok || got != want {
			t.Fatalf("Get = (%q, %v, %v), quer (%q, true, nil)", got, ok, err, want)
		}

		// Overwrite (AST-004 regrava o valor) keeps a single REG_SZ value.
		const want2 = `"C:\Jogos\Arcadia\arcadia.exe" --agent`
		if err := rv.Set(want2); err != nil {
			t.Fatalf("Set (regravação): %v", err)
		}
		got, ok, err = rv.Get()
		if err != nil || !ok || got != want2 {
			t.Fatalf("Get após regravação = (%q, %v, %v), quer (%q, true, nil)", got, ok, err, want2)
		}
	})

	t.Run("AST-004/Get de valor ausente retorna ok=false sem erro", func(t *testing.T) {
		path := testRunKeyPath(t)
		k := createKey(t, path)
		if err := k.SetStringValue("Outro", "x"); err != nil {
			k.Close()
			t.Fatalf("SetStringValue(Outro): %v", err)
		}
		k.Close()

		got, ok, err := winapi.NewRunValue(path, name).Get()
		if err != nil || ok {
			t.Fatalf("Get = (%q, %v, %v), quer (\"\", false, nil)", got, ok, err)
		}
	})

	t.Run("AST-004/Get com subchave ausente retorna ok=false sem erro", func(t *testing.T) {
		path := testRunKeyPath(t) // never created
		got, ok, err := winapi.NewRunValue(path, name).Get()
		if err != nil || ok {
			t.Fatalf("Get = (%q, %v, %v), quer (\"\", false, nil)", got, ok, err)
		}
		if _, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE); !errors.Is(err, registry.ErrNotExist) {
			t.Errorf("Get criou a subchave (OpenKey err = %v); Get não deve escrever no registro", err)
		}
	})

	t.Run("AST-005/Delete remove o valor", func(t *testing.T) {
		path := testRunKeyPath(t)
		rv := winapi.NewRunValue(path, name)
		if err := rv.Set(`"C:\Jogos\Arcadia\arcadia.exe" --agent`); err != nil {
			t.Fatalf("Set: %v", err)
		}
		// A sibling value must survive: Delete removes only its own value.
		k := createKey(t, path)
		if err := k.SetStringValue("Outro", "x"); err != nil {
			k.Close()
			t.Fatalf("SetStringValue(Outro): %v", err)
		}
		k.Close()

		if err := rv.Delete(); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, _, exists := rawValue(t, path, name); exists {
			t.Error("valor ainda existe no registro após Delete")
		}
		if v, _, exists := rawValue(t, path, "Outro"); !exists || v != "x" {
			t.Errorf("valor vizinho Outro = (%q, %v), quer (\"x\", true)", v, exists)
		}
		got, ok, err := rv.Get()
		if err != nil || ok {
			t.Fatalf("Get após Delete = (%q, %v, %v), quer (\"\", false, nil)", got, ok, err)
		}
	})

	t.Run("AST-005/Delete de valor ausente retorna nil", func(t *testing.T) {
		path := testRunKeyPath(t)
		k := createKey(t, path)
		k.Close()
		rv := winapi.NewRunValue(path, name)
		if err := rv.Delete(); err != nil {
			t.Errorf("Delete com valor ausente: %v, quer nil", err)
		}

		// Absent key is also "absent" (OpenKey yields ERROR_FILE_NOT_FOUND).
		missing := winapi.NewRunValue(testRunKeyPath(t), name)
		if err := missing.Delete(); err != nil {
			t.Errorf("Delete com subchave ausente: %v, quer nil", err)
		}
	})

	t.Run("AST-009/valor que não é REG_SZ retorna erro em Get", func(t *testing.T) {
		path := testRunKeyPath(t)
		k := createKey(t, path)
		if err := k.SetDWordValue(name, 1); err != nil {
			k.Close()
			t.Fatalf("SetDWordValue: %v", err)
		}
		k.Close()

		got, ok, err := winapi.NewRunValue(path, name).Get()
		if err == nil {
			t.Fatalf("Get de DWORD = (%q, %v, nil), quer erro", got, ok)
		}
	})

	t.Run("AST-009/nenhum handle de registro sobra", func(t *testing.T) {
		path := testRunKeyPath(t)
		rv := winapi.NewRunValue(path, name)
		cycle := func() {
			if err := rv.Set(`"C:\Jogos\Arcadia\arcadia.exe" --agent`); err != nil {
				t.Fatalf("Set: %v", err)
			}
			if _, _, err := rv.Get(); err != nil {
				t.Fatalf("Get: %v", err)
			}
			if err := rv.Delete(); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if _, _, err := rv.Get(); err != nil {
				t.Fatalf("Get (ausente): %v", err)
			}
		}

		// Warm-up: lazy DLL/proc resolution and one-time runtime allocations
		// happen outside the measurement.
		cycle()

		before := processHandleCount(t)
		for range 5 { // 5 cycles x 4 operations = 20 operations
			cycle()
		}
		after := processHandleCount(t)

		// Tolerance: the Go runtime may create an OS thread (thread + event
		// handles) while this goroutine is blocked in a syscall, which can
		// move the process-wide count by a few handles regardless of
		// RunValue. A leak of one key handle per Set/Get/Delete would add
		// ~20 handles (at least 10 for the operations that find the key), so
		// a slack of 3 still detects it.
		const slack = 3
		if after > before+slack {
			t.Errorf("handles do processo: antes=%d depois=%d (+%d) após 20 operações; quer no máximo +%d", before, after, after-before, slack)
		}
	})
}

// TestRemoveOwnRunEntry covers the one-time cleanup of the Run entry older versions
// wrote (ADM-004). Every case uses a throwaway key.
func TestRemoveOwnRunEntry(t *testing.T) {
	const exe = `C:\Arcadia\arcadia.exe`

	t.Run("ADM-004/apaga a entrada quando aponta para o mesmo exe, sem diferenciar maiúsculas", func(t *testing.T) {
		t.Parallel()
		path := testRunKeyPath(t)
		run := winapi.NewRunValue(path, winapi.RunValueName)
		if err := run.Set(`"c:\arcadia\ARCADIA.exe" --agent`); err != nil {
			t.Fatalf("Set: %v", err)
		}
		removed, err := winapi.RemoveOwnRunEntry(run, exe)
		if err != nil {
			t.Fatalf("RemoveOwnRunEntry: %v", err)
		}
		if !removed {
			t.Errorf("removed = false, want true")
		}
		if _, _, exists := rawValue(t, path, winapi.RunValueName); exists {
			t.Errorf("value still present after removal")
		}
	})

	t.Run("ADM-004/mantém entrada de outro exe", func(t *testing.T) {
		t.Parallel()
		path := testRunKeyPath(t)
		run := winapi.NewRunValue(path, winapi.RunValueName)
		const other = `"C:\Outro\app.exe" --agent`
		if err := run.Set(other); err != nil {
			t.Fatalf("Set: %v", err)
		}
		removed, err := winapi.RemoveOwnRunEntry(run, exe)
		if err != nil || removed {
			t.Fatalf("RemoveOwnRunEntry = (%v, %v), want (false, nil)", removed, err)
		}
		if got, _, _ := rawValue(t, path, winapi.RunValueName); got != other {
			t.Errorf("value = %q, want it kept as %q", got, other)
		}
	})

	t.Run("ADM-004/ausência não é erro", func(t *testing.T) {
		t.Parallel()
		run := winapi.NewRunValue(testRunKeyPath(t), winapi.RunValueName)
		removed, err := winapi.RemoveOwnRunEntry(run, exe)
		if err != nil || removed {
			t.Errorf("RemoveOwnRunEntry = (%v, %v), want (false, nil)", removed, err)
		}
	})

	t.Run("ADM-004/valor ilegível é mantido e reportado", func(t *testing.T) {
		t.Parallel()
		path := testRunKeyPath(t)
		run := winapi.NewRunValue(path, winapi.RunValueName)
		if err := run.Set(""); err != nil {
			t.Fatalf("Set: %v", err)
		}
		removed, err := winapi.RemoveOwnRunEntry(run, exe)
		if err == nil || removed {
			t.Errorf("RemoveOwnRunEntry = (%v, %v), want (false, error)", removed, err)
		}
		if _, _, exists := rawValue(t, path, winapi.RunValueName); !exists {
			t.Errorf("unreadable value was deleted, want it kept")
		}
	})
}
