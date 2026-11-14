package domain_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestSeedPlatforms(t *testing.T) {
	t.Parallel()

	t.Run("PLT-001/13 plataformas com chave, nome e categoria do contrato, na ordem", func(t *testing.T) {
		t.Parallel()

		want := []domain.SeedPlatform{
			{Key: "windows", Name: "Windows", Category: domain.CategoryPC},
			{Key: "3ds", Name: "Nintendo 3DS", Category: domain.CategoryHandheld},
			{Key: "n64", Name: "Nintendo 64", Category: domain.CategoryConsole},
			{Key: "nes", Name: "Nintendo Entertainment System", Category: domain.CategoryConsole},
			{Key: "gba", Name: "Nintendo Game Boy Advance", Category: domain.CategoryHandheld},
			{Key: "gamecube", Name: "Nintendo GameCube", Category: domain.CategoryConsole},
			{Key: "switch", Name: "Nintendo Switch", Category: domain.CategoryConsole},
			{Key: "switch2", Name: "Nintendo Switch 2", Category: domain.CategoryConsole},
			{Key: "wii", Name: "Nintendo Wii", Category: domain.CategoryConsole},
			{Key: "ps1", Name: "Sony Playstation", Category: domain.CategoryConsole},
			{Key: "ps2", Name: "Sony Playstation 2", Category: domain.CategoryConsole},
			{Key: "ps3", Name: "Sony Playstation 3", Category: domain.CategoryConsole},
			{Key: "snes", Name: "Super Nintendo Entertainment System", Category: domain.CategoryConsole},
		}
		got := domain.SeedPlatforms
		if len(got) != len(want) {
			t.Fatalf("len(SeedPlatforms) = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("SeedPlatforms[%d] = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("PLT-001/chaves únicas e categorias válidas (DOM-015)", func(t *testing.T) {
		t.Parallel()

		keys := map[string]bool{}
		names := map[string]bool{}
		for _, p := range domain.SeedPlatforms {
			if p.Key == "" || p.Name == "" {
				t.Errorf("seed %+v has empty key or name", p)
			}
			if keys[p.Key] {
				t.Errorf("duplicate seed key %q", p.Key)
			}
			keys[p.Key] = true
			if names[p.Name] {
				t.Errorf("duplicate seed name %q", p.Name)
			}
			names[p.Name] = true
			if _, err := domain.ParsePlatformCategory(string(p.Category)); err != nil {
				t.Errorf("seed %q has invalid category %q: %v", p.Key, p.Category, err)
			}
		}
	})

	t.Run("PLT-017/SeedByName ignora maiúsculas e espaços das pontas", func(t *testing.T) {
		t.Parallel()

		for _, in := range []string{"Nintendo 64", "nintendo 64", "  NINTENDO 64  ", "\tNintendo 64\n"} {
			got, ok := domain.SeedByName(in)
			if !ok {
				t.Errorf("SeedByName(%q) ok = false, want true", in)
				continue
			}
			if got.Key != "n64" {
				t.Errorf("SeedByName(%q).Key = %q, want n64", in, got.Key)
			}
		}
		if _, ok := domain.SeedByName("   "); ok {
			t.Error("SeedByName(blank) ok = true, want false")
		}
	})

	t.Run("PLT-025/SeedByName não funde nomes parecidos (Super Nintendo)", func(t *testing.T) {
		t.Parallel()

		for _, in := range []string{"Super Nintendo", "Nintendo", "SNES", "Sony PlayStation 4", "Nintendo 6"} {
			if got, ok := domain.SeedByName(in); ok {
				t.Errorf("SeedByName(%q) = %+v, want not found", in, got)
			}
		}
		got, ok := domain.SeedByName("Super Nintendo Entertainment System")
		if !ok || got.Key != "snes" {
			t.Errorf("SeedByName(full snes name) = %+v, %v; want key snes", got, ok)
		}
	})

	t.Run("PLT-029/SeedByKey desconhecida devolve false", func(t *testing.T) {
		t.Parallel()

		for _, k := range []string{"", "xbox", "Windows", "N64 "} {
			if got, ok := domain.SeedByKey(k); ok {
				t.Errorf("SeedByKey(%q) = %+v, want not found", k, got)
			}
		}
		got, ok := domain.SeedByKey("ps2")
		if !ok || got.Name != "Sony Playstation 2" {
			t.Errorf("SeedByKey(ps2) = %+v, %v", got, ok)
		}
	})

	t.Run("PLT-001/SeedKeyWindows existe com categoria pc", func(t *testing.T) {
		t.Parallel()

		got, ok := domain.SeedByKey(domain.SeedKeyWindows)
		if !ok {
			t.Fatalf("SeedByKey(SeedKeyWindows) not found")
		}
		if got.Category != domain.CategoryPC || got.Name != "Windows" {
			t.Errorf("windows seed = %+v, want name Windows, category pc", got)
		}
	})
}
