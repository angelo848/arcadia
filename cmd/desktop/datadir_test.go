package main

// Tests for the data dir check (DSK-006) and the desktop log (DSK-009).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/paths"
)

func TestDataDir(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "Arcadia", "data")

	tests := []struct {
		name        string
		agentDir    string
		windowsOnly bool
		wantWarn    bool
	}{
		{"DSK-006/dataDir diferente loga aviso", filepath.Join(filepath.Dir(root), "other-data"), false, true},
		{"DSK-006/dataDir vazio loga aviso", "", false, true},
		{"DSK-006/mesmo dir não avisa", root, false, false},
		{"DSK-006/mesmo dir com barra final não avisa", root + string(filepath.Separator), false, false},
		{"DSK-006/mesmo dir com . e .. não avisa",
			filepath.Join(root, "..", filepath.Base(root)) + string(filepath.Separator) + ".", false, false},
		{"DSK-006/mesmo dir com caixa/barra diferente não avisa",
			strings.ReplaceAll(strings.ToUpper(root), `\`, `/`) + "/", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.windowsOnly && runtime.GOOS != "windows" {
				t.Skip("case-insensitive comparison only applies on Windows")
			}
			log, buf := captureLog()

			got := warnIfDataDirDiffers(log, root, tc.agentDir)

			if got != tc.wantWarn {
				t.Errorf("warnIfDataDirDiffers(%q, %q) = %v, want %v", root, tc.agentDir, got, tc.wantWarn)
			}
			warns := countLevel(t, buf, "WARN")
			if tc.wantWarn && warns != 1 {
				t.Errorf("WARN records = %d, want 1; log: %s", warns, buf.String())
			}
			if !tc.wantWarn && warns != 0 {
				t.Errorf("WARN records = %d, want 0; log: %s", warns, buf.String())
			}
		})
	}
}

func TestSetupLogging(t *testing.T) {
	t.Parallel()

	t.Run("DSK-009/registra em R/logs/desktop.log", func(t *testing.T) {
		t.Parallel()
		p, err := paths.Resolve(paths.Options{Flag: filepath.Join(t.TempDir(), "data"), Getenv: func(string) string { return "" }})
		if err != nil {
			t.Fatalf("paths.Resolve: %v", err)
		}

		log, closer, err := setupLogging(p)
		if err != nil {
			t.Fatalf("setupLogging err = %v", err)
		}
		const marker = "dsk-009-marker"
		log.Info(marker)
		log.Debug("dsk-009-debug-should-not-appear")
		if err := closer.Close(); err != nil {
			t.Fatalf("closer.Close err = %v", err)
		}

		file := filepath.Join(p.Root, "logs", "desktop.log")
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		var found bool
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var rec map[string]any
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatalf("desktop.log line is not JSON: %q: %v", line, err)
			}
			if rec["msg"] == marker {
				found = true
				if rec["level"] != "INFO" {
					t.Errorf("marker level = %v, want INFO", rec["level"])
				}
			}
		}
		if !found {
			t.Errorf("desktop.log does not contain %q: %s", marker, data)
		}
		if strings.Contains(string(data), "dsk-009-debug-should-not-appear") {
			t.Errorf("desktop.log contains a Debug record; the UI logs at Info: %s", data)
		}
	})
}
