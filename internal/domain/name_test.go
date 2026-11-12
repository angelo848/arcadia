package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// runes repete r n vezes. Usa runas não ASCII para provar que o limite conta runas e não bytes.
func runes(r rune, n int) string {
	return strings.Repeat(string(r), n)
}

func TestNormalizeName(t *testing.T) {
	t.Parallel()

	t.Run("DOM-016/corta espaços das pontas e mantém a capitalização", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			in, want string
		}{
			{"  Team Cherry  ", "Team Cherry"},
			{"\tRPG ", "RPG"},
			{"Ação", "Ação"},
		}
		for _, tc := range tests {
			got, err := domain.NormalizeName(tc.in)
			if err != nil {
				t.Fatalf("NormalizeName(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	})

	t.Run("DOM-016/vazio depois de cortar é erro", func(t *testing.T) {
		t.Parallel()

		for _, in := range []string{"", "   ", "\t "} {
			got, err := domain.NormalizeName(in)
			if !errors.Is(err, domain.ErrInvalidField) {
				t.Errorf("NormalizeName(%q) = %q, %v; want ErrInvalidField", in, got, err)
			}
		}
	})

	t.Run("DOM-016/100 runas passam e 101 são erro", func(t *testing.T) {
		t.Parallel()

		ok := runes('á', 100)
		if got, err := domain.NormalizeName(ok); err != nil || got != ok {
			t.Errorf("NormalizeName(100 runas) = %q, %v; want the same 100 runas", got, err)
		}
		if got, err := domain.NormalizeName("  " + ok + "  "); err != nil || got != ok {
			t.Errorf("NormalizeName(100 runas com espaços) = %q, %v; want 100 runas após o corte", got, err)
		}
		if got, err := domain.NormalizeName(runes('á', 101)); !errors.Is(err, domain.ErrInvalidField) {
			t.Errorf("NormalizeName(101 runas) = %q, %v; want ErrInvalidField", got, err)
		}
	})

	t.Run("DOM-015/nome de plataforma usa a mesma regra de 1 a 100 runas", func(t *testing.T) {
		t.Parallel()

		if got, err := domain.NormalizeName("  Nintendo 64  "); err != nil || got != "Nintendo 64" {
			t.Errorf("NormalizeName(platform) = %q, %v; want %q", got, err, "Nintendo 64")
		}
		if got, err := domain.NormalizeName("x"); err != nil || got != "x" {
			t.Errorf("NormalizeName(1 runa) = %q, %v; want %q", got, err, "x")
		}
		if got, err := domain.NormalizeName(""); !errors.Is(err, domain.ErrInvalidField) {
			t.Errorf("NormalizeName(\"\") = %q, %v; want ErrInvalidField", got, err)
		}
		if got, err := domain.NormalizeName(runes('ç', 101)); !errors.Is(err, domain.ErrInvalidField) {
			t.Errorf("NormalizeName(101 runas de plataforma) = %q, %v; want ErrInvalidField", got, err)
		}
	})
}
