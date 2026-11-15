package importers_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/importers"
)

func TestFileScan(t *testing.T) {
	t.Parallel()

	scan := func(t *testing.T, path string) importers.Result {
		t.Helper()
		res, err := importers.File().Scan(context.Background(), importers.ScanInput{Path: path})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		return res
	}

	t.Run("IMP-004/executável vira exe com título sem extensão", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "ABZU.exe")
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		c := res.Candidates[0]
		if c.Title != "ABZU" || c.Platform != "" || c.LaunchType != importers.LaunchExe ||
			c.LaunchTarget != path || c.InstallDir != dir || c.Source != importers.SourceFile {
			t.Errorf("candidate = %+v", c)
		}
	})

	t.Run("IMP-004/.bat e .cmd também são exe", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"Run.BAT", "start.cmd"} {
			path := filepath.Join(t.TempDir(), name)
			writeFile(t, path, "")
			res := scan(t, path)
			if len(res.Candidates) != 1 || res.Candidates[0].LaunchType != importers.LaunchExe {
				t.Errorf("%s: candidates = %+v, want exe", name, res.Candidates)
			}
		}
	})

	t.Run("IMP-004/outro tipo de arquivo (ROM) vira emulator com o arquivo como alvo", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "Zelda.sfc")
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		c := res.Candidates[0]
		if c.Title != "Zelda" || c.LaunchType != importers.LaunchEmulator || c.LaunchTarget != path {
			t.Errorf("candidate = %+v", c)
		}
	})

	t.Run("IMP-004/pasta, arquivo inexistente ou caminho vazio devolvem aviso", func(t *testing.T) {
		t.Parallel()
		for name, path := range map[string]string{
			"pasta":       t.TempDir(),
			"inexistente": filepath.Join(t.TempDir(), "x.exe"),
			"vazio":       "",
		} {
			res := scan(t, path)
			if len(res.Warnings) != 1 || len(res.Candidates) != 0 {
				t.Errorf("%s: result = %+v, want one warning and no candidate", name, res)
			}
		}
	})

	t.Run("IMV-032/avisos de arquivo inexistente, pasta e caminho vazio não mudam", func(t *testing.T) {
		t.Parallel()
		cases := map[string]struct {
			path string
			want string
		}{
			"inexistente": {filepath.Join(t.TempDir(), "x.z64"), "Arquivo: não encontrado"},
			"pasta":       {t.TempDir(), "Arquivo: escolha um arquivo, não uma pasta"},
			"vazio":       {"", "Arquivo: nenhum arquivo escolhido"},
		}
		for name, tc := range cases {
			res := scan(t, tc.path)
			if len(res.Candidates) != 0 || len(res.Warnings) != 1 || res.Warnings[0] != tc.want {
				t.Errorf("%s: result = %+v, want no candidate and warning %q", name, res, tc.want)
			}
		}
	})

	t.Run("IMV-032/Legend of Zelda, The - Majora's Mask (U) [!].z64 vira The Legend of Zelda - Majora's Mask", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "Legend of Zelda, The - Majora's Mask (U) [!].z64")
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		c := res.Candidates[0]
		if c.Title != "The Legend of Zelda - Majora's Mask" {
			t.Errorf("Title = %q, want %q", c.Title, "The Legend of Zelda - Majora's Mask")
		}
		if c.LaunchType != importers.LaunchEmulator {
			t.Errorf("LaunchType = %q, want emulator", c.LaunchType)
		}
	})

	t.Run("IMV-032/setup.exe é aceito (a lista de IMV-015 não vale)", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "setup.exe")
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 1 || len(res.Warnings) != 0 {
			t.Fatalf("result = %+v, want one candidate and no warning", res)
		}
		c := res.Candidates[0]
		if c.Title != "setup" || c.LaunchType != importers.LaunchExe || c.File != "setup.exe" || c.Extension != "exe" {
			t.Errorf("candidate = %+v", c)
		}
	})

	t.Run("IMV-032/arquivo sem extensão mantém o nome inteiro como título", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name      string
			title     string
			extension string
		}{
			{"Game 2.0", "Game 2.0", ""},
			{"Dr. Mario", "Dr. Mario", ""},
			{"Game", "Game", ""},
		}
		for _, tc := range cases {
			path := filepath.Join(t.TempDir(), tc.name)
			writeFile(t, path, "")

			res := scan(t, path)
			if len(res.Candidates) != 1 {
				t.Fatalf("%s: candidates = %+v, want one", tc.name, res)
			}
			c := res.Candidates[0]
			if c.Title != tc.title || c.Extension != tc.extension || c.File != tc.name {
				t.Errorf("%s: candidate = %+v, want title %q, extension %q", tc.name, c, tc.title, tc.extension)
			}
		}
	})

	t.Run("IMV-032/qualquer tipo continua um candidato: exe/bat/cmd são exe e o resto emulator", func(t *testing.T) {
		t.Parallel()
		cases := map[string]string{
			"Tool.EXE":    importers.LaunchExe,
			"run.bat":     importers.LaunchExe,
			"go.cmd":      importers.LaunchExe,
			"Game.sfc":    importers.LaunchEmulator,
			"Disc.iso":    importers.LaunchEmulator,
			"Pack.zip":    importers.LaunchEmulator,
			"Noext":       importers.LaunchEmulator,
			"Readme.docx": importers.LaunchEmulator,
		}
		for name, want := range cases {
			path := filepath.Join(t.TempDir(), name)
			writeFile(t, path, "")

			res := scan(t, path)
			if len(res.Candidates) != 1 || res.Candidates[0].LaunchType != want {
				t.Errorf("%s: candidates = %+v, want one with launch %q", name, res.Candidates, want)
			}
		}
	})

	t.Run("IMV-032/extensão não é regra de lista: Mr. Driller.zip vira emulator com título com ponto", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "Mr. Driller.zip")
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		c := res.Candidates[0]
		if c.Title != "Mr. Driller" || c.LaunchType != importers.LaunchEmulator || c.Extension != "zip" {
			t.Errorf("candidate = %+v", c)
		}
	})

	t.Run("IMV-024/file é o nome, extension minúscula, discs 1", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "ROM.SFC")
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		c := res.Candidates[0]
		if c.File != "ROM.SFC" || c.Extension != "sfc" || c.Discs != 1 || c.DiscFiles != nil {
			t.Errorf("candidate = %+v, want file ROM.SFC, extension sfc, discs 1, no discFiles", c)
		}
	})

	t.Run("IMV-007/launchTarget e installDir intactos; marcações continuam só no nome do arquivo", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		name := "Super_Mario_World (USA) [!].sfc"
		path := filepath.Join(dir, name)
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		c := res.Candidates[0]
		if c.LaunchTarget != path {
			t.Errorf("LaunchTarget = %q, want disk path %q", c.LaunchTarget, path)
		}
		if c.InstallDir != dir {
			t.Errorf("InstallDir = %q, want %q", c.InstallDir, dir)
		}
		if c.File != name {
			t.Errorf("File = %q, want %q", c.File, name)
		}
		if c.Title != "Super Mario World" {
			t.Errorf("Title = %q, want %q", c.Title, "Super Mario World")
		}
	})

	t.Run("IMV-005/nome só de marcações fica como o nome com espaços normalizados", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "(Disc 1).iso")
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 1 {
			t.Fatalf("candidates = %+v, want one", res.Candidates)
		}
		if got := res.Candidates[0].Title; got != "(Disc 1)" {
			t.Errorf("Title = %q, want %q", got, "(Disc 1)")
		}
	})

	t.Run("IMV-005/nome que fica vazio após aparo não vira candidato e devolve aviso", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), " .iso")
		writeFile(t, path, "")

		res := scan(t, path)
		if len(res.Candidates) != 0 || len(res.Warnings) != 1 || res.Warnings[0] != "Arquivo: nome vazio" {
			t.Errorf("result = %+v, want no candidate and warning %q", res, "Arquivo: nome vazio")
		}
	})
}
