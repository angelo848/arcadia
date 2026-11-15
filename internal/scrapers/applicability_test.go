package scrapers_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

// infoFromSeed builds the PlatformInfo of a seed platform, as the agent resolves it (Known = found).
func infoFromSeed(p domain.SeedPlatform) scrapers.PlatformInfo {
	return scrapers.PlatformInfo{SeedKey: p.Key, Category: string(p.Category), Known: true}
}

func TestApplicable(t *testing.T) {
	t.Parallel()

	t.Run("MET-003/igdb e steamgriddb aplicam a qualquer plataforma", func(t *testing.T) {
		t.Parallel()
		infos := []scrapers.PlatformInfo{
			{},
			{SeedKey: "windows", Category: "pc", Known: true},
			{SeedKey: "snes", Category: "console", Known: true},
			{Category: "console", Known: true},
			{Category: "pc", Known: true},
			{Known: false},
		}
		for _, p := range domain.SeedPlatforms {
			infos = append(infos, infoFromSeed(p))
		}
		for _, provider := range []string{"igdb", "steamgriddb"} {
			for _, info := range infos {
				if !scrapers.Applicable(provider, info) {
					t.Errorf("Applicable(%q, %+v) = false; want true", provider, info)
				}
			}
		}
	})

	t.Run("MET-003/steamstore: regras da Steam Store", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			info scrapers.PlatformInfo
			want bool
		}{
			{"MET-003/steamstore: chave windows aplica", scrapers.PlatformInfo{SeedKey: "windows", Category: "pc", Known: true}, true},
			{"MET-003/steamstore: sem chave de seed e categoria pc aplica", scrapers.PlatformInfo{Category: "pc", Known: true}, true},
			{"MET-003/steamstore: categoria console não aplica", scrapers.PlatformInfo{Category: "console", Known: true}, false},
			{"MET-003/steamstore: plataforma não informada (Known=false) aplica", scrapers.PlatformInfo{Known: false}, true},
			{"MET-003/chave de console do seed não aplica à Steam Store", scrapers.PlatformInfo{SeedKey: "snes", Category: "console", Known: true}, false},
			{"MET-003/steamstore: chave de seed que não é windows não aplica mesmo com categoria pc", scrapers.PlatformInfo{SeedKey: "snes", Category: "pc", Known: true}, false},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				if got := scrapers.Applicable("steamstore", c.info); got != c.want {
					t.Errorf("Applicable(steamstore, %+v) = %v; want %v", c.info, got, c.want)
				}
			})
		}
	})

	t.Run("MET-003/chave de console do seed não aplica à Steam Store (todas as chaves de console)", func(t *testing.T) {
		t.Parallel()
		checked := 0
		for _, p := range domain.SeedPlatforms {
			if p.Key == "windows" {
				continue
			}
			if p.Category == domain.CategoryPC {
				continue
			}
			checked++
			if scrapers.Applicable("steamstore", infoFromSeed(p)) {
				t.Errorf("Steam Store aplica à chave %q (categoria %q); want não aplica", p.Key, p.Category)
			}
		}
		if checked == 0 {
			t.Fatal("nenhuma chave de console no seed para conferir")
		}
	})

	t.Run("MET-003/provedor desconhecido não aplica", func(t *testing.T) {
		t.Parallel()
		infos := []scrapers.PlatformInfo{
			{},
			{SeedKey: "windows", Category: "pc", Known: true},
			{SeedKey: "snes", Category: "console", Known: true},
			{Known: false},
		}
		for _, provider := range []string{"", "mobygames", "IGDB", "steamstore "} {
			for _, info := range infos {
				if scrapers.Applicable(provider, info) {
					t.Errorf("Applicable(%q, %+v) = true; want false", provider, info)
				}
			}
		}
	})
}
