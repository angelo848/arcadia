package importers_test

import (
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/importers"
)

// MET-040: a file name made only of markings never gives its raw stem back: the result has no brackets.
func TestSearchNameNoRawMarkings(t *testing.T) {
	t.Parallel()
	for _, path := range []string{`E:\Games\[].exe`, `E:\[].exe`, `[].exe`} {
		t.Run("MET-040/marcações sem nome: "+path, func(t *testing.T) {
			t.Parallel()
			if got := importers.SearchName(path); strings.ContainsAny(got, "[]()") {
				t.Errorf("SearchName(%q) = %q, want no raw markings", path, got)
			}
		})
	}
}

// MET-040: SearchName proposes the metadata search name of a file (match-rules.md §5.2). Pure: no disk, no clock.
// The paths use Windows separators, as the file source receives them.
func TestSearchName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "MET-040/Hades.exe → Hades", path: `E:\Games\Hades\Hades.exe`, want: "Hades"},
		{name: "MET-040/hollow_knight.exe → hollow knight", path: `E:\Games\Hollow Knight\hollow_knight.exe`, want: "hollow knight"},
		{name: "MET-040/MyGame-Win64-Shipping.exe sem sufixo de motor", path: `E:\Games\MyGame\MyGame-Win64-Shipping.exe`, want: "MyGame"},
		{name: "MET-040/sufixos empilhados -Win64-Shipping e -x64", path: `E:\Games\Game-x64-Win64-Shipping.exe`, want: "Game"},
		{name: "MET-040/sufixo -x64 sozinho", path: `E:\Games\Alpha-x64.exe`, want: "Alpha"},
		{name: "MET-040/sufixo _32bit sozinho", path: `E:\Games\Beta_32bit.exe`, want: "Beta"},
		{name: "MET-040/game.exe usa a pasta pai", path: `E:\Games\Some Game\game.exe`, want: "Some Game"},
		{name: "MET-040/Launcher.exe em Binaries\\Win64 sobe duas pastas", path: `D:\Games\Hades II\Binaries\Win64\Launcher.exe`, want: "Hades II"},
		{name: "MET-040/Cuphead (v1.3.4).exe", path: `E:\Games\Cuphead\Cuphead (v1.3.4).exe`, want: "Cuphead"},
		{name: "MET-040/04. Final Fantasy VIII (USA)\\launcher.exe tira prefixo e marcação", path: `E:\Games\04. Final Fantasy VIII (USA)\launcher.exe`, want: "Final Fantasy VIII"},
		{name: "MET-040/F:\\game.exe sem pasta útil devolve game", path: `F:\game.exe`, want: "game"},
		{name: "MET-040/Super Metroid (U).smc", path: `E:\Roms\snes\Super Metroid (U).smc`, want: "Super Metroid"},
		{name: "MET-040/pasta cujo título normalizado fica vazio passa para a seguinte", path: `E:\Games\Real Game\- -\game.exe`, want: "Real Game"},
		{name: "MET-040/nunca sobe além de 3 pastas nem da raiz (4º nível não é lido)", path: `E:\Games\Real Title\bin\release\x64\game.exe`, want: "game"},
		{name: "MET-040/sobe até 3 pastas (3º nível é lido)", path: `E:\Real Title\bin\release\game.exe`, want: "Real Title"},
		{name: "MET-040/nunca sobe da raiz do disco", path: `E:\bin\game.exe`, want: "game"},
		{name: "MET-040/genérico sem diferenciar maiúsculas nem separadores (Start_Game)", path: `E:\Games\Some Game\Start_Game.exe`, want: "Some Game"},
		{name: "MET-040/[].exe sem nome usa a pasta útil, sem marcações cruas", path: `E:\Games\Real Game\[].exe`, want: "Real Game"},
		{name: "MET-040/título longo cortado em 200 runas", path: `E:\Games\` + strings.Repeat("é", 250) + `.exe`, want: strings.Repeat("é", 200)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := importers.SearchName(tc.path)
			if got != tc.want {
				t.Errorf("SearchName(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
