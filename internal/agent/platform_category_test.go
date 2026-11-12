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

	t.Run("IMP-002/jogos de PC e pastas continuam na categoria pc", func(t *testing.T) {
		t.Parallel()
		if got := platformCategoryFor(ipc.ImportSourceSteam, importers.PlatformPC); got != domain.PlatformCategory("pc") {
			t.Errorf("steam category = %q, want pc", got)
		}
		if got := platformCategoryFor(ipc.ImportSourceFolder, "Meu PC"); got != domain.PlatformCategory("pc") {
			t.Errorf("folder category = %q, want pc", got)
		}
	})
}
