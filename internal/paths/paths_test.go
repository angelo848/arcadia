package paths_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/paths"
)

// fakeEnv returns a Getenv that only answers paths.EnvDataDir; any other key
// yields "" so a wrong key makes env-based cases fail.
func fakeEnv(value string) func(string) string {
	return func(key string) string {
		if key == paths.EnvDataDir {
			return value
		}
		return ""
	}
}

// literalEnv answers only the literal variable name from the spec/ADR 0006,
// independently of the paths.EnvDataDir constant.
func literalEnv(value string) func(string) string {
	return func(key string) string {
		if key == "ARCADIA_DATA_DIR" {
			return value
		}
		return ""
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", p, err)
	}
	return filepath.Clean(abs)
}

// isWithin reports whether child is root or lies inside root.
func isWithin(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func TestResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup builds the options and the expected Root (ignored when wantErr).
		setup   func(t *testing.T) (paths.Options, string)
		wantErr bool
	}{
		{
			name: "PTH-001/flag vence env",
			setup: func(t *testing.T) (paths.Options, string) {
				flag := filepath.Join(t.TempDir(), "flag")
				env := filepath.Join(t.TempDir(), "env")
				exe := t.TempDir()
				return paths.Options{Flag: flag, ExeDir: exe, Getenv: fakeEnv(env)}, filepath.Clean(flag)
			},
		},
		{
			name: "PTH-001/env vence exe",
			setup: func(t *testing.T) (paths.Options, string) {
				env := filepath.Join(t.TempDir(), "env")
				exe := t.TempDir()
				return paths.Options{ExeDir: exe, Getenv: fakeEnv(env)}, filepath.Clean(env)
			},
		},
		{
			name: "PTH-001/sem flag nem env usa exe/data",
			setup: func(t *testing.T) (paths.Options, string) {
				exe := t.TempDir()
				return paths.Options{ExeDir: exe, Getenv: fakeEnv("")}, filepath.Join(exe, "data")
			},
		},
		{
			name: "PTH-001/env vazio é ignorado",
			setup: func(t *testing.T) (paths.Options, string) {
				exe := t.TempDir()
				// Getenv returns "" for ARCADIA_DATA_DIR and a non-empty value for any
				// other key: must fall back to exe/data.
				other := t.TempDir()
				getenv := func(key string) string {
					if key == "ARCADIA_DATA_DIR" {
						return ""
					}
					return other
				}
				return paths.Options{ExeDir: exe, Getenv: getenv}, filepath.Join(exe, "data")
			},
		},
		{
			name: "PTH-001/env lido de ARCADIA_DATA_DIR",
			setup: func(t *testing.T) (paths.Options, string) {
				env := filepath.Join(t.TempDir(), "env")
				exe := t.TempDir()
				return paths.Options{ExeDir: exe, Getenv: literalEnv(env)}, filepath.Clean(env)
			},
		},
		{
			name: "PTH-001/tudo vazio retorna erro",
			setup: func(t *testing.T) (paths.Options, string) {
				return paths.Options{Getenv: fakeEnv("")}, ""
			},
			wantErr: true,
		},
		{
			name: "PTH-002/flag relativa vira absoluta",
			setup: func(t *testing.T) (paths.Options, string) {
				rel := filepath.Join("rel-flag", "data")
				return paths.Options{Flag: rel, ExeDir: t.TempDir(), Getenv: fakeEnv("")}, mustAbs(t, rel)
			},
		},
		{
			name: "PTH-002/env relativo vira absoluto",
			setup: func(t *testing.T) (paths.Options, string) {
				rel := filepath.Join("rel-env", "data")
				return paths.Options{ExeDir: t.TempDir(), Getenv: fakeEnv(rel)}, mustAbs(t, rel)
			},
		},
		{
			name: "PTH-002/ExeDir relativo vira absoluto",
			setup: func(t *testing.T) (paths.Options, string) {
				rel := filepath.Join("rel-exe", "bin")
				return paths.Options{ExeDir: rel, Getenv: fakeEnv("")}, mustAbs(t, filepath.Join(rel, "data"))
			},
		},
		{
			name: "PTH-002/DB Media Logs absolutos dentro de Root",
			setup: func(t *testing.T) (paths.Options, string) {
				rel := "rel-root"
				return paths.Options{Flag: rel, Getenv: fakeEnv("")}, mustAbs(t, rel)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts, wantRoot := tc.setup(t)

			got, err := paths.Resolve(opts)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Resolve(%+v) = %+v, nil; want error", opts, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%+v): unexpected error: %v", opts, err)
			}

			if filepath.Clean(got.Root) != wantRoot {
				t.Errorf("Root = %q, want %q", got.Root, wantRoot)
			}

			// Every case also checks the derived layout (PTH-002).
			for field, p := range map[string]string{"Root": got.Root, "DB": got.DB, "Media": got.Media, "Logs": got.Logs} {
				if !filepath.IsAbs(p) {
					t.Errorf("%s = %q, want absolute path", field, p)
				}
				if !isWithin(wantRoot, p) {
					t.Errorf("%s = %q, want inside Root %q", field, p, wantRoot)
				}
			}
			if want := filepath.Join(wantRoot, "library.db"); filepath.Clean(got.DB) != want {
				t.Errorf("DB = %q, want %q", got.DB, want)
			}
			if want := filepath.Join(wantRoot, "media"); filepath.Clean(got.Media) != want {
				t.Errorf("Media = %q, want %q", got.Media, want)
			}
			if want := filepath.Join(wantRoot, "logs"); filepath.Clean(got.Logs) != want {
				t.Errorf("Logs = %q, want %q", got.Logs, want)
			}
		})
	}
}

func TestEnvDataDirConstant(t *testing.T) {
	t.Parallel()
	t.Run("PTH-001/constante igual ao literal da spec", func(t *testing.T) {
		t.Parallel()
		if paths.EnvDataDir != "ARCADIA_DATA_DIR" {
			t.Errorf("EnvDataDir = %q, want %q", paths.EnvDataDir, "ARCADIA_DATA_DIR")
		}
	})
}

// Not parallel: t.Setenv mutates the process environment.
func TestResolveGetenvNil(t *testing.T) {
	t.Run("PTH-001/Getenv nil usa ambiente do processo", func(t *testing.T) {
		env := filepath.Join(t.TempDir(), "env")
		t.Setenv("ARCADIA_DATA_DIR", env)

		got, err := paths.Resolve(paths.Options{ExeDir: t.TempDir()})
		if err != nil {
			t.Fatalf("Resolve: unexpected error: %v", err)
		}
		if filepath.Clean(got.Root) != filepath.Clean(env) {
			t.Errorf("Root = %q, want %q", got.Root, env)
		}
	})
}

func assertDir(t *testing.T, p string) {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Errorf("stat %q: %v", p, err)
		return
	}
	if !info.IsDir() {
		t.Errorf("%q exists but is not a directory", p)
	}
}

func TestEnsureDirs(t *testing.T) {
	t.Parallel()

	resolve := func(t *testing.T, root string) paths.Paths {
		t.Helper()
		p, err := paths.Resolve(paths.Options{Flag: root, Getenv: fakeEnv("")})
		if err != nil {
			t.Fatalf("Resolve(Flag=%q): %v", root, err)
		}
		return p
	}

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "PTH-003/cria pastas inexistentes",
			run: func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "nested", "arcadia")
				p := resolve(t, root)
				if err := p.EnsureDirs(); err != nil {
					t.Fatalf("EnsureDirs: %v", err)
				}
				assertDir(t, p.Root)
				assertDir(t, p.Media)
				assertDir(t, p.Logs)
			},
		},
		{
			name: "PTH-003/segunda chamada não falha",
			run: func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "arcadia")
				p := resolve(t, root)
				if err := p.EnsureDirs(); err != nil {
					t.Fatalf("first EnsureDirs: %v", err)
				}
				if err := p.EnsureDirs(); err != nil {
					t.Fatalf("second EnsureDirs: %v", err)
				}
				assertDir(t, p.Root)
				assertDir(t, p.Media)
				assertDir(t, p.Logs)
			},
		},
		{
			name: "PTH-003/Root é arquivo retorna erro",
			run: func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "not-a-dir")
				if err := os.WriteFile(root, []byte("x"), 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
				p := resolve(t, root)
				if err := p.EnsureDirs(); err == nil {
					t.Fatalf("EnsureDirs with Root as a regular file: got nil error, want error")
				}
			},
		},
		{
			name: "PTH-003/Media é arquivo retorna erro",
			run: func(t *testing.T) {
				p := resolve(t, filepath.Join(t.TempDir(), "arcadia"))
				if err := os.MkdirAll(p.Root, 0o755); err != nil {
					t.Fatalf("mkdir root: %v", err)
				}
				if err := os.WriteFile(p.Media, []byte("x"), 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
				if err := p.EnsureDirs(); err == nil {
					t.Fatalf("EnsureDirs with Media as a regular file: got nil error, want error")
				}
			},
		},
		{
			name: "PTH-003/Logs é arquivo retorna erro",
			run: func(t *testing.T) {
				p := resolve(t, filepath.Join(t.TempDir(), "arcadia"))
				if err := os.MkdirAll(p.Root, 0o755); err != nil {
					t.Fatalf("mkdir root: %v", err)
				}
				if err := os.WriteFile(p.Logs, []byte("x"), 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
				if err := p.EnsureDirs(); err == nil {
					t.Fatalf("EnsureDirs with Logs as a regular file: got nil error, want error")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}
