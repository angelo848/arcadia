package scrapers_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// Assumed signature (tasks.md T20): func SystemID(seedKey string) (int, bool).
// The table is contracts/screenscraper-rules.md §1 (the values stay subject to T02's check).
func TestSystemID(t *testing.T) {
	t.Parallel()

	t.Run("VID-010/SystemID devolve o id de cada uma das 13 chaves de rules §1 e false para switch2, vazio e chave desconhecida", func(t *testing.T) {
		t.Parallel()
		want := map[string]int{
			"windows":  138,
			"3ds":      17,
			"n64":      14,
			"nes":      3,
			"gba":      12,
			"gamecube": 13,
			"switch":   225,
			"wii":      16,
			"ps1":      57,
			"ps2":      58,
			"ps3":      59,
			"snes":     4,
		}
		for key, id := range want {
			got, ok := scrapers.SystemID(key)
			if !ok || got != id {
				t.Errorf("SystemID(%q) = (%d, %v), want (%d, true)", key, got, ok, id)
			}
		}
		for _, key := range []string{"switch2", "", "xbox360", "Windows", " windows", "dreamcast"} {
			if got, ok := scrapers.SystemID(key); ok || got != 0 {
				t.Errorf("SystemID(%q) = (%d, %v), want (0, false)", key, got, ok)
			}
		}
	})

	t.Run("VID-010/toda chave de seed é mapeada, exceto switch2", func(t *testing.T) {
		t.Parallel()
		if len(domain.SeedPlatforms) != 13 {
			t.Fatalf("precondition: %d seed platforms, want 13", len(domain.SeedPlatforms))
		}
		for _, sp := range domain.SeedPlatforms {
			_, ok := scrapers.SystemID(sp.Key)
			if sp.Key == "switch2" {
				if ok {
					t.Errorf("SystemID(switch2) mapped, want unmapped")
				}
				continue
			}
			if !ok {
				t.Errorf("seed key %q has no ScreenScraper system", sp.Key)
			}
		}
	})

	t.Run("SSC-004/SoftName é arcadia e o tipo de vídeo é video-normalized", func(t *testing.T) {
		t.Parallel()
		if scrapers.SoftName != "arcadia" {
			t.Errorf("SoftName = %q, want arcadia", scrapers.SoftName)
		}
		if scrapers.SSVideoMediaType != "video-normalized" {
			t.Errorf("SSVideoMediaType = %q, want video-normalized", scrapers.SSVideoMediaType)
		}
	})
}
