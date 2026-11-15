package importers

import (
	"strings"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	t.Parallel()

	// check asserts naturalLess(a, b) == want.
	check := func(t *testing.T, a, b string, want bool) {
		t.Helper()
		if got := naturalLess(a, b); got != want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", a, b, got, want)
		}
	}

	t.Run("IMV-010/Jogo 2 antes de Jogo 10", func(t *testing.T) {
		t.Parallel()
		check(t, "Jogo 2", "Jogo 10", true)
		check(t, "Jogo 10", "Jogo 2", false)
	})

	t.Run("IMV-010/04. FF antes de 31. Spyro", func(t *testing.T) {
		t.Parallel()
		check(t, "04. FF", "31. Spyro", true)
		check(t, "31. Spyro", "04. FF", false)
	})

	t.Run("IMV-010/sem diferenciar maiúsculas", func(t *testing.T) {
		t.Parallel()
		check(t, "alfa", "Beta", true)
		check(t, "Beta", "alfa", false)
		check(t, "ALFA", "alfb", true)
	})

	t.Run("IMV-010/pasta e arquivo no mesmo critério", func(t *testing.T) {
		t.Parallel()
		check(t, `Jogo 2\Jogo.exe`, "Jogo 10.exe", true)
		check(t, "Jogo 10.exe", `Jogo 2\Jogo.exe`, false)
		check(t, "Abc.exe", `Abd\x.exe`, true)
		check(t, `Abd\x.exe`, "Abc.exe", false)
		check(t, "a/2", "a/10", true)
	})

	t.Run("IMV-010/zeros à esquerda: mais curto primeiro e determinístico", func(t *testing.T) {
		t.Parallel()
		check(t, "Jogo 2", "Jogo 02", true)
		check(t, "Jogo 02", "Jogo 2", false)
		check(t, "Jogo 02", "Jogo 002", true)
		check(t, "Jogo 002", "Jogo 02", false)
	})

	t.Run("IMV-010/sequência de dígitos enorme não estoura", func(t *testing.T) {
		t.Parallel()
		a := "a" + strings.Repeat("9", 400)
		b := "a" + strings.Repeat("9", 401)
		check(t, a, b, true)
		check(t, b, a, false)

		x := "p" + strings.Repeat("1", 300) + "2"
		y := "p" + strings.Repeat("1", 300) + "3"
		check(t, x, y, true)
		check(t, y, x, false)
	})

	t.Run("IMV-010/acentos comuns", func(t *testing.T) {
		t.Parallel()
		check(t, "Ação", "Bola", true)
		check(t, "Bola", "Ação", false)
		check(t, "Ação", "Açúcar", true)
	})

	t.Run("IMV-010/irreflexiva e transitiva num conjunto", func(t *testing.T) {
		t.Parallel()
		set := []string{
			"Jogo 2", "Jogo 10", "jogo 2", "Jogo 02", "04. FF", "31. Spyro",
			`Zelda\a.exe`, "Zelda.exe", "Ação", "Bola", "x", "X", "a/2", "a/10",
		}
		for _, a := range set {
			if naturalLess(a, a) {
				t.Errorf("naturalLess(%q, %q) = true, want irreflexive", a, a)
			}
		}
		for _, a := range set {
			for _, b := range set {
				if a == b {
					continue
				}
				ab, ba := naturalLess(a, b), naturalLess(b, a)
				if ab == ba {
					t.Errorf("distinct %q and %q: naturalLess both ways = %v, want exactly one order", a, b, ab)
				}
			}
		}
		for _, a := range set {
			for _, b := range set {
				for _, c := range set {
					if naturalLess(a, b) && naturalLess(b, c) && !naturalLess(a, c) {
						t.Errorf("not transitive: %q < %q < %q but not %q < %q", a, b, c, a, c)
					}
				}
			}
		}
	})
}
