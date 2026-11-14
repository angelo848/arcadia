package agent

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/importers"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

func TestPlatformCategoryFor(t *testing.T) {
	t.Parallel()

	t.Run("IMP-005/plataforma do LaunchBox que não é PC vira console", func(t *testing.T) {
		t.Parallel()
		if got := platformCategoryFor(ipc.ImportSourceLaunchBox, "Sony Playstation 2"); got != domain.PlatformCategory("console") {
			t.Errorf("category = %q, want console", got)
		}
	})

	t.Run("PLT-029/platformCategoryFor: nome do seed usa a categoria do seed; LaunchBox fora do seed é console; outras fontes pc", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name         string
			source, plat string
			want         domain.PlatformCategory
		}{
			{"seed handheld vindo do LaunchBox", ipc.ImportSourceLaunchBox, "Nintendo 3DS", domain.CategoryHandheld},
			{"seed handheld vindo de outra fonte", ipc.ImportSourceFile, "Nintendo Game Boy Advance", domain.CategoryHandheld},
			{"seed console do LaunchBox", ipc.ImportSourceLaunchBox, "Sony Playstation 2", domain.CategoryConsole},
			{"seed Windows do LaunchBox é pc", ipc.ImportSourceLaunchBox, "Windows", domain.CategoryPC},
			{"seed Windows da Steam é pc", ipc.ImportSourceSteam, importers.PlatformWindows, domain.CategoryPC},
			{"seed console de outra fonte usa o seed", ipc.ImportSourceFolder, "Nintendo 64", domain.CategoryConsole},
			{"LaunchBox fora do seed é console", ipc.ImportSourceLaunchBox, "Amiga", domain.CategoryConsole},
			{"outra fonte fora do seed é pc", ipc.ImportSourceSteam, "Amiga", domain.CategoryPC},
			{"pasta fora do seed é pc", ipc.ImportSourceFolder, "Meu PC", domain.CategoryPC},
		}
		for _, c := range cases {
			if got := platformCategoryFor(c.source, c.plat); got != c.want {
				t.Errorf("%s: platformCategoryFor(%s, %q) = %q, want %q", c.name, c.source, c.plat, got, c.want)
			}
		}
	})

	t.Run("IMP-002/jogos de PC e pastas continuam na categoria pc", func(t *testing.T) {
		t.Parallel()
		if got := platformCategoryFor(ipc.ImportSourceSteam, importers.PlatformWindows); got != domain.PlatformCategory("pc") {
			t.Errorf("steam category = %q, want pc", got)
		}
		if got := platformCategoryFor(ipc.ImportSourceFile, "Meu PC"); got != domain.PlatformCategory("pc") {
			t.Errorf("folder category = %q, want pc", got)
		}
	})
}
