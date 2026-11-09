package domain_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestDefaultGameTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "DOM-009/hollow_knight.exe vira hollow_knight",
			path: `C:\Jogos\Hollow Knight\hollow_knight.exe`,
			want: "hollow_knight",
		},
		{
			name: "DOM-009/Game.EXE vira Game",
			path: `D:\x\Game.EXE`,
			want: "Game",
		},
		{
			name: "DOM-009/my.game.exe vira my.game",
			path: `C:\a\my.game.exe`,
			want: "my.game",
		},
		{
			name: "DOM-009/b.exe.exe vira b.exe",
			path: `C:\a\b.exe.exe`,
			want: "b.exe",
		},
		{
			name: "DOM-009/barra normal como separador",
			path: "C:/Jogos/Hollow Knight/hollow_knight.exe",
			want: "hollow_knight",
		},
		{
			name: "DOM-009/barra normal como separador misturada com contrabarra",
			path: `C:\Jogos/Celeste\Celeste.Exe`,
			want: "Celeste",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.DefaultGameTitle(tc.path); got != tc.want {
				t.Fatalf("DefaultGameTitle(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
