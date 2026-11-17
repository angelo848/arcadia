package importers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEpicInstallationsScan(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Games", "Rocket League")
	if err := os.MkdirAll(game, 0o755); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(root, "Games", "Removed")
	rec := func(id, dir, item string, complete bool) string {
		c := "false"
		if complete {
			c = "true"
		}
		return `{"installationId":"` + id + `","complete":` + c + `,"dir":` + jsonString(dir) +
			`,"namespace":"ns1","itemId":"` + item + `","artifactId":"Sugar"}`
	}
	lines := []string{
		`{"version":1,"entries":0}`,
		rec("A", game, "item1", false),
		rec("A", game, "item1", true),
		rec("B", gone, "item2", true),  // folder no longer exists
		rec("C", game, "item3", false), // never completed
		"not json",
	}
	if err := os.WriteFile(filepath.Join(root, installationsFile), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(root, "cache", "prod")
	if err := os.MkdirAll(catalog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalog, "catalog-cache.json"),
		[]byte(`{"items":{"item1":{"item":{"title":"Rocket League"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("IMP-003/novo launcher: a última linha de cada instalação manda e só entra a completa que existe em disco", func(t *testing.T) {
		res, err := Epic().Scan(context.Background(), ScanInput{Path: root})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want 1", res.Candidates)
		}
		c := res.Candidates[0]
		if c.Title != "Rocket League" || c.InstallDir != game || c.LaunchType != LaunchURI || c.Platform != PlatformWindows {
			t.Errorf("candidate = %+v", c)
		}
		if want := "com.epicgames.launcher://apps/ns1:item1:Sugar?action=launch&silent=true"; c.LaunchTarget != want {
			t.Errorf("target = %q, want %q", c.LaunchTarget, want)
		}
	})

	t.Run("IMP-003/sem o catálogo o título é o nome da pasta do jogo", func(t *testing.T) {
		if err := os.RemoveAll(filepath.Join(root, "cache")); err != nil {
			t.Fatal(err)
		}
		res, err := Epic().Scan(context.Background(), ScanInput{Path: root})
		if err != nil || len(res.Candidates) != 1 || res.Candidates[0].Title != "Rocket League" {
			t.Errorf("res = %+v, err = %v", res, err)
		}
	})
}

func jsonString(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}
