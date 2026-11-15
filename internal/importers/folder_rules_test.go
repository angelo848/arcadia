package importers

import (
	"io/fs"
	"slices"
	"testing"
)

func TestIsGameExt(t *testing.T) {
	t.Parallel()

	t.Run("IMV-013/classe Windows: só exe, bat e cmd", func(t *testing.T) {
		t.Parallel()
		for _, ext := range []string{"exe", "bat", "cmd"} {
			if !isGameExt(ext, KindExecutables) {
				t.Errorf("isGameExt(%q, KindExecutables) = false, want true", ext)
			}
		}
		for _, ext := range []string{"nes", "iso", "zip"} {
			if isGameExt(ext, KindExecutables) {
				t.Errorf("isGameExt(%q, KindExecutables) = true, want false", ext)
			}
		}
	})

	t.Run("IMV-013/classe console: uma extensão de cada linha da tabela", func(t *testing.T) {
		t.Parallel()
		// One extension of each row of import-rules.md §1 that is not the executables row.
		for _, ext := range []string{
			"nes", // Nintendo
			"sms", // Sega
			"iso", // Disco (Sony e outros)
			"a26", // Atari
			"pce", // NEC, SNK, Bandai e outros
			"adf", // Computadores
			"zip", // Compactados
		} {
			if !isGameExt(ext, KindConsole) {
				t.Errorf("isGameExt(%q, KindConsole) = false, want true", ext)
			}
		}
		if isGameExt("exe", KindConsole) {
			t.Error("isGameExt(\"exe\", KindConsole) = true, want false")
		}
	})

	t.Run("IMV-013/classe qualquer: as duas listas", func(t *testing.T) {
		t.Parallel()
		for _, ext := range []string{"exe", "bat", "cmd", "nes", "iso", "zip"} {
			if !isGameExt(ext, KindAny) {
				t.Errorf("isGameExt(%q, KindAny) = false, want true", ext)
			}
		}
	})

	t.Run("IMV-013/.sub, .wav, .raw, .dll, .sav, .txt, .png fora das listas", func(t *testing.T) {
		t.Parallel()
		for _, ext := range []string{"sub", "wav", "raw", "dll", "sav", "txt", "png"} {
			for _, kind := range []FolderKind{KindAny, KindExecutables, KindConsole} {
				if isGameExt(ext, kind) {
					t.Errorf("isGameExt(%q, %v) = true, want false", ext, kind)
				}
			}
		}
	})

	t.Run("IMV-013/extensão em maiúsculas", func(t *testing.T) {
		t.Parallel()
		// The comparison is on the lower-case extension that SplitExt returns (IMV-001); the end-to-end
		// case with upper-case names is also covered by the folder source (T08).
		stem, ext := SplitExt("JOGO.EXE")
		if stem != "JOGO" || ext != "exe" {
			t.Fatalf("SplitExt(\"JOGO.EXE\") = (%q, %q), want (\"JOGO\", \"exe\")", stem, ext)
		}
		if !isGameExt(ext, KindExecutables) {
			t.Errorf("isGameExt(%q, KindExecutables) = false, want true", ext)
		}
		_, ext = SplitExt("Banjo.Z64")
		if !isGameExt(ext, KindConsole) {
			t.Errorf("isGameExt(%q, KindConsole) = false, want true", ext)
		}
	})
}

func TestIsNotAGame(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		stem string
		want bool
	}{
		{"IMV-015/unins000", "unins000", true},
		{"IMV-015/uninstall", "uninstall", true},
		{"IMV-015/setup", "setup", true},
		{"IMV-015/gamesetup", "gamesetup", true},
		{"IMV-015/vcredist_x64", "vcredist_x64", true},
		{"IMV-015/vc_redist", "vc_redist", true},
		{"IMV-015/dxsetup", "dxsetup", true},
		{"IMV-015/dotnet4", "dotnet4", true},
		{"IMV-015/oalinst", "oalinst", true},
		{"IMV-015/UnityCrashHandler64", "UnityCrashHandler64", true},
		{"IMV-015/crashpad_handler", "crashpad_handler", true},
		{"IMV-015/MyCrashReportTool", "MyCrashReportTool", true},
		{"IMV-015/Redist", "Redist", true},
		{"IMV-015/DirectX9", "DirectX9", true},
		{"IMV-015/jogo.exe não é ignorado", "jogo", false},
		{"IMV-015/Setup Hero (nome de jogo) é ignorado", "Setup Hero", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isNotAGame(tc.stem); got != tc.want {
				t.Errorf("isNotAGame(%q) = %v, want %v", tc.stem, got, tc.want)
			}
		})
	}
}

func TestSkipEntry(t *testing.T) {
	t.Parallel()

	t.Run("IMV-012/.git, $RECYCLE.BIN", func(t *testing.T) {
		t.Parallel()
		if !skipEntry(".git", fs.ModeDir) {
			t.Error("skipEntry(\".git\") = false, want true")
		}
		if !skipEntry("$RECYCLE.BIN", fs.ModeDir) {
			t.Error("skipEntry(\"$RECYCLE.BIN\") = false, want true")
		}
	})

	t.Run("IMV-012/System Volume Information, @eaDir, __MACOSX, node_modules sem diferenciar maiúsculas", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{
			"System Volume Information",
			"SYSTEM VOLUME INFORMATION",
			"@eaDir",
			"__MACOSX",
			"node_modules",
			"Node_Modules",
		} {
			if !skipEntry(name, fs.ModeDir) {
				t.Errorf("skipEntry(%q) = false, want true", name)
			}
		}
	})

	t.Run("IMV-012/ModeSymlink e ModeIrregular", func(t *testing.T) {
		t.Parallel()
		if !skipEntry("Jogo", fs.ModeSymlink) {
			t.Error("skipEntry(symlink) = false, want true")
		}
		if !skipEntry("Jogo", fs.ModeIrregular) {
			t.Error("skipEntry(irregular) = false, want true")
		}
	})

	t.Run("IMV-012/pasta e arquivo comuns não são ignorados", func(t *testing.T) {
		t.Parallel()
		if skipEntry("Jogo", fs.ModeDir) {
			t.Error("skipEntry(folder \"Jogo\") = true, want false")
		}
		if skipEntry("jogo.exe", 0) {
			t.Error("skipEntry(file \"jogo.exe\") = true, want false")
		}
	})
}

func TestCompanionExts(t *testing.T) {
	t.Parallel()

	t.Run("IMV-020/tabela de contêineres e trilhas", func(t *testing.T) {
		t.Parallel()
		// Keys and values are lower-case extensions without the dot (as in isGameExt).
		want := map[string][]string{
			"cue": {"bin", "img", "sub", "wav"},
			"ccd": {"img", "sub"},
			"mds": {"mdf"},
			"gdi": {"bin", "raw"},
		}
		if len(companionExts) != len(want) {
			t.Errorf("companionExts has %d containers, want %d", len(companionExts), len(want))
		}
		for container, tracks := range want {
			got, ok := companionExts[container]
			if !ok {
				t.Errorf("companionExts has no container %q", container)
				continue
			}
			if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(tracks))) {
				t.Errorf("companionExts[%q] = %v, want %v", container, got, tracks)
			}
		}
	})
}
