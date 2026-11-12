package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateArgsTemplate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		tmpl string
		ok   bool
	}{
		{"EMU-001/modelo vazio é válido", "", true},
		{"EMU-001/modelo aceita {rom} e {dir}", "-f {rom} -d {dir}", true},
		{"EMU-001/marcador desconhecido é recusado", "-x {foo}", false},
		{"EMU-001/chave sem fechar é recusada", "-x {rom", false},
		{"EMU-001/chave de fechamento sem marcador é recusada", "-x }", false},
		{"EMU-001/modelo acima de 2000 runas é recusado", strings.Repeat("a", MaxArgsTemplateRunes+1), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateArgsTemplate(c.tmpl)
			if c.ok && err != nil {
				t.Errorf("ValidateArgsTemplate(%q) = %v, want nil", c.tmpl, err)
			}
			if !c.ok && !errors.Is(err, ErrInvalidField) {
				t.Errorf("ValidateArgsTemplate(%q) = %v, want ErrInvalidField", c.tmpl, err)
			}
		})
	}
}

func TestBuildArgs(t *testing.T) {
	t.Parallel()

	t.Run("EMU-004/troca {rom} e {dir} pelos valores", func(t *testing.T) {
		t.Parallel()
		got := BuildArgs("-f {rom} -d {dir}", `D:\Roms\game.sfc`, `D:\Roms`)
		want := `-f D:\Roms\game.sfc -d D:\Roms`
		if got != want {
			t.Errorf("BuildArgs = %q, want %q", got, want)
		}
	})

	t.Run("EMU-004/valor com espaço vai entre aspas", func(t *testing.T) {
		t.Parallel()
		got := BuildArgs("-f {rom}", `D:\Meus Jogos\zelda.sfc`, `D:\Meus Jogos`)
		want := `-f "D:\Meus Jogos\zelda.sfc"`
		if got != want {
			t.Errorf("BuildArgs = %q, want %q", got, want)
		}
	})

	t.Run("EMU-004/modelo vazio gera argumentos vazios", func(t *testing.T) {
		t.Parallel()
		if got := BuildArgs("", `D:\a.sfc`, `D:\`); got != "" {
			t.Errorf("BuildArgs(empty) = %q, want empty", got)
		}
	})
}
