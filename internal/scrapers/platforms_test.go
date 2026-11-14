package scrapers_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// igdbIDs is the table of contracts/seed-platforms.md §1 (confirmed against the IGDB API in T02).
var igdbIDs = map[string]string{
	"windows":  "6",
	"3ds":      "37",
	"n64":      "4",
	"nes":      "18",
	"gba":      "24",
	"gamecube": "21",
	"switch":   "130",
	"switch2":  "508",
	"wii":      "5",
	"ps1":      "7",
	"ps2":      "8",
	"ps3":      "9",
	"snes":     "19",
}

func TestPlatformID(t *testing.T) {
	t.Parallel()

	t.Run("PLT-030/cada chave confirmada devolve o id do IGDB da tabela", func(t *testing.T) {
		t.Parallel()
		for key, want := range igdbIDs {
			got, ok := scrapers.PlatformID(key, "igdb")
			if !ok || got != want {
				t.Errorf("PlatformID(%q, igdb) = %q, %v; want %q, true", key, got, ok, want)
			}
		}
	})

	t.Run("PLT-030/chave desconhecida, chave vazia ou provedor sem valor devolvem false", func(t *testing.T) {
		t.Parallel()
		cases := []struct{ name, key, provider string }{
			{"chave desconhecida", "atari2600", "igdb"},
			{"chave vazia", "", "igdb"},
			{"nome canônico em vez da chave", "Nintendo 64", "igdb"},
			{"chave com maiúsculas", "N64", "igdb"},
			{"provedor sem valor", "n64", "steamgriddb"},
			{"provedor vazio", "n64", ""},
			{"provedor desconhecido", "windows", "mobygames"},
			{"chave e provedor vazios", "", ""},
		}
		for _, c := range cases {
			if got, ok := scrapers.PlatformID(c.key, c.provider); ok || got != "" {
				t.Errorf("%s: PlatformID(%q, %q) = %q, %v; want empty, false", c.name, c.key, c.provider, got, ok)
			}
		}
	})

	t.Run("PLT-030/toda chave do mapa existe em domain.SeedPlatforms", func(t *testing.T) {
		t.Parallel()
		// The map is private; the exported function is probed with every seed key and
		// every key of the confirmed table. Each confirmed key must be a seed key, and
		// each seed key of the table must resolve.
		for key := range igdbIDs {
			if _, ok := domain.SeedByKey(key); !ok {
				t.Errorf("key %q of the IGDB table is not in domain.SeedPlatforms", key)
			}
		}
		for _, p := range domain.SeedPlatforms {
			want, confirmed := igdbIDs[p.Key]
			got, ok := scrapers.PlatformID(p.Key, "igdb")
			switch {
			case confirmed && (!ok || got != want):
				t.Errorf("seed key %q: PlatformID = %q, %v; want %q", p.Key, got, ok, want)
			case !confirmed && ok:
				t.Errorf("seed key %q has IGDB id %q that is not in the confirmed table", p.Key, got)
			}
		}
	})
}
