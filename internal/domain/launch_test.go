package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateLaunch(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", MaxLaunchTargetRunes+1)
	cases := []struct {
		name, lt, target, args string
		ok                     bool
	}{
		{"EDT-010/exe com caminho absoluto é válido", "exe", `C:\Jogos\g.exe`, "", true},
		{"EDT-010/uri é válida sem ser caminho", "uri", "steam://rungameid/730", "", true},
		{"EDT-010/tipo fora de exe e uri é recusado", "emulator", `C:\g.exe`, "", false},
		{"EDT-010/alvo vazio é recusado", "uri", "   ", "", false},
		{"EDT-010/exe com caminho relativo é recusado", "exe", `g.exe`, "", false},
		{"EDT-010/alvo longo demais é recusado", "uri", long, "", false},
		{"EDT-010/argumentos longos demais são recusados", "exe", `C:\g.exe`, long, false},
		{"EDT-010/argumentos vazios são válidos", "exe", `C:\g.exe`, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateLaunch(c.lt, c.target, c.args)
			if c.ok && err != nil {
				t.Errorf("ValidateLaunch = %v, want nil", err)
			}
			if !c.ok && !errors.Is(err, ErrInvalidField) {
				t.Errorf("ValidateLaunch = %v, want ErrInvalidField", err)
			}
		})
	}
}
