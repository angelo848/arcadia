package domain_test

import (
	"errors"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestValidateText(t *testing.T) {
	t.Parallel()

	t.Run("EDT-001/title corta espaços e recusa vazio ou 201 runas", func(t *testing.T) {
		t.Parallel()

		got, err := domain.ValidateTitle("  Hollow Knight  ")
		if err != nil || got != "Hollow Knight" {
			t.Errorf("ValidateTitle(padded) = %q, %v; want %q", got, err, "Hollow Knight")
		}

		for _, in := range []string{"", "   "} {
			if got, err := domain.ValidateTitle(in); !errors.Is(err, domain.ErrInvalidField) {
				t.Errorf("ValidateTitle(%q) = %q, %v; want ErrInvalidField", in, got, err)
			}
		}

		ok200 := runes('é', 200)
		if got, err := domain.ValidateTitle(ok200); err != nil || got != ok200 {
			t.Errorf("ValidateTitle(200 runas) = %q, %v; want 200 runas", got, err)
		}
		if got, err := domain.ValidateTitle("  " + ok200 + "  "); err != nil || got != ok200 {
			t.Errorf("ValidateTitle(200 runas com espaços) = %q, %v; want 200 runas após o corte", got, err)
		}
		if got, err := domain.ValidateTitle(runes('é', 201)); !errors.Is(err, domain.ErrInvalidField) {
			t.Errorf("ValidateTitle(201 runas) = %q, %v; want ErrInvalidField", got, err)
		}
	})

	t.Run("EDT-001/description aceita vazio e 10 000 runas, e recusa 10 001", func(t *testing.T) {
		t.Parallel()

		if got, err := domain.ValidateDescription(""); err != nil || got != "" {
			t.Errorf("ValidateDescription(\"\") = %q, %v; want empty and no error", got, err)
		}

		// ValidateDescription não corta: espaços das pontas ficam como foram digitados.
		if got, err := domain.ValidateDescription("  texto  "); err != nil || got != "  texto  " {
			t.Errorf("ValidateDescription(padded) = %q, %v; want unchanged", got, err)
		}

		ok := runes('ç', 10000)
		if got, err := domain.ValidateDescription(ok); err != nil || got != ok {
			t.Errorf("ValidateDescription(10 000 runas) = len %d, %v; want unchanged", len(got), err)
		}
		if got, err := domain.ValidateDescription(runes('ç', 10001)); !errors.Is(err, domain.ErrInvalidField) {
			t.Errorf("ValidateDescription(10 001 runas) = len %d, %v; want ErrInvalidField", len(got), err)
		}
	})
}
