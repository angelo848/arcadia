package scrapers_test

import (
	"strconv"
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

// T06 (MET-003): mapa inverso das plataformas do IGDB, usado para ler PlatformKeys dos candidatos.
func TestSeedKeysForIGDB(t *testing.T) {
	t.Parallel()

	t.Run("MET-003/SeedKeysForIGDB devolve as chaves dos ids conhecidos, ignora desconhecidos e mantém a ordem", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			ids  []int64
			want []string
		}{
			{"vazio", nil, []string{}},
			{"um id conhecido", []int64{6}, []string{"windows"}},
			{"ordem da entrada mantida (snes antes de windows)", []int64{19, 6}, []string{"snes", "windows"}},
			{"ordem da entrada mantida (windows antes de snes)", []int64{6, 19}, []string{"windows", "snes"}},
			{"ids desconhecidos ignorados", []int64{999, 6, 0, -1, 7777}, []string{"windows"}},
			{"só desconhecidos devolve vazio", []int64{1, 2, 3}, []string{}},
			{"id de switch2 e de switch", []int64{508, 130}, []string{"switch2", "switch"}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				got := scrapers.SeedKeysForIGDB(c.ids)
				if len(got) != len(c.want) {
					t.Fatalf("SeedKeysForIGDB(%v) = %v; want %v", c.ids, got, c.want)
				}
				for i := range got {
					if got[i] != c.want[i] {
						t.Errorf("SeedKeysForIGDB(%v) = %v; want %v", c.ids, got, c.want)
						break
					}
				}
			})
		}
	})

	t.Run("MET-003/SeedKeysForIGDB é o inverso de PlatformID(chave, \"igdb\") para todas as chaves do mapa", func(t *testing.T) {
		t.Parallel()
		seen := 0
		for _, p := range domain.SeedPlatforms {
			id, ok := scrapers.PlatformID(p.Key, "igdb")
			if !ok {
				continue
			}
			seen++
			n, err := strconv.ParseInt(id, 10, 64)
			if err != nil {
				t.Fatalf("id do IGDB %q da chave %q não é inteiro: %v", id, p.Key, err)
			}
			got := scrapers.SeedKeysForIGDB([]int64{n})
			if len(got) != 1 || got[0] != p.Key {
				t.Errorf("SeedKeysForIGDB([%d]) = %v; want [%s]", n, got, p.Key)
			}
		}
		if seen == 0 {
			t.Fatal("nenhuma chave do seed tem id do IGDB para conferir")
		}
		// Every confirmed id maps back, and the whole table comes back in one call, in order.
		all := make([]int64, 0, len(igdbIDs))
		wantKeys := make([]string, 0, len(igdbIDs))
		for _, p := range domain.SeedPlatforms {
			id, ok := scrapers.PlatformID(p.Key, "igdb")
			if !ok {
				continue
			}
			n, _ := strconv.ParseInt(id, 10, 64)
			all = append(all, n)
			wantKeys = append(wantKeys, p.Key)
		}
		got := scrapers.SeedKeysForIGDB(all)
		if len(got) != len(wantKeys) {
			t.Fatalf("SeedKeysForIGDB(todos) = %v; want %v", got, wantKeys)
		}
		for i := range got {
			if got[i] != wantKeys[i] {
				t.Errorf("SeedKeysForIGDB(todos)[%d] = %q; want %q", i, got[i], wantKeys[i])
			}
		}
	})
}
