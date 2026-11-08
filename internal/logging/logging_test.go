package logging

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/natefinch/lumberjack.v2"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()

	t.Run("AGT-005/debug info warn error mapeiam para slog", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			in   string
			want slog.Level
		}{
			{"debug", slog.LevelDebug},
			{"info", slog.LevelInfo},
			{"warn", slog.LevelWarn},
			{"error", slog.LevelError},
		}
		for _, tc := range cases {
			got, err := ParseLevel(tc.in)
			if err != nil {
				t.Errorf("ParseLevel(%q) error = %v, want nil", tc.in, err)
				continue
			}
			if got != tc.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
			}
		}
	})

	t.Run("AGT-005/vazio é info", func(t *testing.T) {
		t.Parallel()
		got, err := ParseLevel("")
		if err != nil {
			t.Fatalf("ParseLevel(\"\") error = %v, want nil", err)
		}
		if got != slog.LevelInfo {
			t.Fatalf("ParseLevel(\"\") = %v, want %v", got, slog.LevelInfo)
		}
	})

	t.Run("AGT-005/valor inválido retorna erro", func(t *testing.T) {
		t.Parallel()
		for _, in := range []string{"verbose", "trace", "1"} {
			if _, err := ParseLevel(in); err == nil {
				t.Errorf("ParseLevel(%q) error = nil, want error", in)
			}
		}
	})
}

// readJSONLines returns the non-empty lines of the file at path, each decoded as JSON.
func readJSONLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []map[string]any
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line is not JSON: %q: %v", line, err)
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return out
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("DSK-009/grava JSON em Dir/File", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		log, closer, err := New(Options{Dir: dir, File: "desktop.log", Level: slog.LevelInfo})
		if err != nil {
			t.Fatalf("New error = %v", err)
		}
		if log == nil || closer == nil {
			_ = closeIfNotNil(closer)
			t.Fatalf("New returned nil logger or closer")
		}
		log.Info("hello world", "game", "Doom")
		if err := closer.Close(); err != nil {
			t.Fatalf("Close error = %v", err)
		}

		lines := readJSONLines(t, filepath.Join(dir, "desktop.log"))
		if len(lines) != 1 {
			t.Fatalf("got %d log lines, want 1: %v", len(lines), lines)
		}
		rec := lines[0]
		if rec["msg"] != "hello world" {
			t.Errorf("msg = %v, want %q", rec["msg"], "hello world")
		}
		if rec["level"] != "INFO" {
			t.Errorf("level = %v, want %q", rec["level"], "INFO")
		}
		if rec["game"] != "Doom" {
			t.Errorf("game = %v, want %q", rec["game"], "Doom")
		}
	})

	t.Run("DSK-009/rotação 10 MB × 3", func(t *testing.T) {
		t.Parallel()
		if MaxSizeMB != 10 {
			t.Errorf("MaxSizeMB = %d, want 10", MaxSizeMB)
		}
		if MaxBackups != 3 {
			t.Errorf("MaxBackups = %d, want 3", MaxBackups)
		}

		dir := t.TempDir()
		log, closer, err := New(Options{Dir: dir, File: "desktop.log", Level: slog.LevelInfo})
		if err != nil {
			t.Fatalf("New error = %v", err)
		}
		defer closeIfNotNil(closer)
		if log == nil || closer == nil {
			t.Fatalf("New returned nil logger or closer")
		}
		// White-box: the Closer returned by New must be the lumberjack writer behind the
		// logger, so the rotation policy checked here is the one New actually uses.
		rot, ok := closer.(*lumberjack.Logger)
		if !ok {
			t.Fatalf("New closer type = %T, want *lumberjack.Logger", closer)
		}

		if want := filepath.Join(dir, "desktop.log"); rot.Filename != want {
			t.Errorf("Filename = %q, want %q", rot.Filename, want)
		}
		if rot.MaxSize != 10 {
			t.Errorf("MaxSize = %d, want 10", rot.MaxSize)
		}
		// Pruning of old backups (MaxBackups) is asynchronous in lumberjack (mill
		// goroutine), so only the configured field is checked, not the files on disk.
		if rot.MaxBackups != 3 {
			t.Errorf("MaxBackups = %d, want 3", rot.MaxBackups)
		}

		// Real rotation: writing more than MaxSizeMB must produce a backup file.
		chunk := bytes.Repeat([]byte("x"), 1<<20)
		for i := 0; i < MaxSizeMB+1; i++ {
			if _, err := rot.Write(chunk); err != nil {
				t.Fatalf("write %d: %v", i, err)
			}
		}
		if err := rot.Close(); err != nil {
			t.Fatalf("Close error = %v", err)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}
		var backups int
		var current bool
		for _, e := range entries {
			name := e.Name()
			switch {
			case name == "desktop.log":
				current = true
			case strings.HasPrefix(name, "desktop-") && strings.HasSuffix(name, ".log"):
				backups++
			}
		}
		if !current {
			t.Errorf("current log desktop.log missing after rotation; entries: %v", entries)
		}
		if backups < 1 {
			t.Errorf("no backup file after writing %d MiB; entries: %v", MaxSizeMB+1, entries)
		}
	})

	t.Run("DSK-009/nível filtra mensagens abaixo", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		log, closer, err := New(Options{Dir: dir, File: "desktop.log", Level: slog.LevelWarn})
		if err != nil {
			t.Fatalf("New error = %v", err)
		}
		log.Info("below threshold")
		log.Warn("at threshold")
		if err := closer.Close(); err != nil {
			t.Fatalf("Close error = %v", err)
		}

		lines := readJSONLines(t, filepath.Join(dir, "desktop.log"))
		if len(lines) != 1 {
			t.Fatalf("got %d log lines, want 1 (only warn): %v", len(lines), lines)
		}
		if lines[0]["msg"] != "at threshold" {
			t.Errorf("msg = %v, want %q", lines[0]["msg"], "at threshold")
		}
		if lines[0]["level"] != "WARN" {
			t.Errorf("level = %v, want %q", lines[0]["level"], "WARN")
		}
	})
}

func closeIfNotNil(c interface{ Close() error }) error {
	if c == nil {
		return nil
	}
	return c.Close()
}
