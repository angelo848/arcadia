package domain_test

import (
	"errors"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestReleaseYear(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		inputs []string
		want   int
		ok     bool
	}{
		{
			name:   "DOM-013/YYYY, YYYY-MM e YYYY-MM-DD dão o ano",
			inputs: []string{"1998", "1998-06", "1998-06-12"},
			want:   1998,
			ok:     true,
		},
		{
			name:   "DOM-013/abc, 98 e vazio não dão ano",
			inputs: []string{"abc", "98", ""},
			ok:     false,
		},
		{
			name:   "DOM-013/sufixo ou mês sem zero à esquerda não dá ano (1998abc, 1998-6-12)",
			inputs: []string{"1998abc", "1998-6-12"},
			ok:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, in := range tc.inputs {
				year, ok := domain.ReleaseYear(in)
				if ok != tc.ok {
					t.Errorf("ReleaseYear(%q) ok = %v, want %v", in, ok, tc.ok)
					continue
				}
				if tc.ok && year != tc.want {
					t.Errorf("ReleaseYear(%q) = %d, want %d", in, year, tc.want)
				}
			}
		})
	}
}

func TestWithYear(t *testing.T) {
	t.Parallel()

	t.Run("DOM-013/1998-06-12 com 1998 fica igual; com 1999 vira 1999", func(t *testing.T) {
		t.Parallel()

		got, err := domain.WithYear("1998-06-12", 1998)
		if err != nil || got != "1998-06-12" {
			t.Errorf("WithYear(1998-06-12, 1998) = %q, %v; want %q", got, err, "1998-06-12")
		}
		got, err = domain.WithYear("1998-06-12", 1999)
		if err != nil || got != "1999" {
			t.Errorf("WithYear(1998-06-12, 1999) = %q, %v; want %q", got, err, "1999")
		}
	})

	t.Run("DOM-013/vazio com 2001 vira 2001", func(t *testing.T) {
		t.Parallel()

		got, err := domain.WithYear("", 2001)
		if err != nil || got != "2001" {
			t.Errorf("WithYear(\"\", 2001) = %q, %v; want %q", got, err, "2001")
		}
	})

	t.Run("DOM-013/YYYY-MM com o mesmo ano fica igual", func(t *testing.T) {
		t.Parallel()

		got, err := domain.WithYear("1998-06", 1998)
		if err != nil || got != "1998-06" {
			t.Errorf("WithYear(1998-06, 1998) = %q, %v; want %q", got, err, "1998-06")
		}
	})

	t.Run("DOM-013/data sem ano válido é substituída pelo ano com quatro dígitos", func(t *testing.T) {
		t.Parallel()

		got, err := domain.WithYear("abc", 2001)
		if err != nil || got != "2001" {
			t.Errorf("WithYear(abc, 2001) = %q, %v; want %q", got, err, "2001")
		}
	})

	t.Run("EDT-003/ano 1899 ou 2101 é erro; 1900 e 2100 passam", func(t *testing.T) {
		t.Parallel()

		for _, y := range []int{1899, 2101} {
			got, err := domain.WithYear("", y)
			if !errors.Is(err, domain.ErrInvalidField) {
				t.Errorf("WithYear(\"\", %d) = %q, %v; want ErrInvalidField", y, got, err)
			}
		}
		for _, y := range []int{1900, 2100} {
			got, err := domain.WithYear("", y)
			want := map[int]string{1900: "1900", 2100: "2100"}[y]
			if err != nil || got != want {
				t.Errorf("WithYear(\"\", %d) = %q, %v; want %q", y, got, err, want)
			}
		}
	})
}
