package domain

import (
	"math"
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, in, want string
	}{
		{"MID-002/remove ™ e ®", "Hollow Knight™", "hollow knight"},
		{"MID-002/remove ® e ©", "Tetris® ©", "tetris"},
		{"MID-002/remove edição e GOTY", "Celeste Edição GOTY", "celeste"},
		{"MID-002/remove edição em francês sem acento", "Pokémon Édition", "pokemon"},
		{"MID-002/remove região entre parênteses", "Final Fantasy VII (USA)", "final fantasy vii"},
		{"MID-002/remove marcas entre colchetes", "Doom [Europe] [Disc 1]", "doom"},
		{"MID-002/colapsa espaços e pontuação", "  Super   Mario  Bros.  ", "super mario bros"},
		{"MID-002/acentos viram letras simples", "Ação Côrte", "acao corte"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeTitle(tc.in); got != tc.want {
				t.Errorf("NormalizeTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestTitleSimilarity(t *testing.T) {
	t.Parallel()

	t.Run("MID-002/títulos iguais têm similaridade 1", func(t *testing.T) {
		t.Parallel()
		if got := TitleSimilarity("Celeste", "celeste"); got != 1 {
			t.Errorf("similarity = %v, want 1", got)
		}
	})

	t.Run("MID-002/títulos sem nada em comum têm similaridade 0", func(t *testing.T) {
		t.Parallel()
		if got := TitleSimilarity("abc", "xyz"); got != 0 {
			t.Errorf("similarity = %v, want 0", got)
		}
	})

	t.Run("MID-002/similaridade Dice sobre bigramas", func(t *testing.T) {
		t.Parallel()
		// Bigrams: abcdefgh -> 7, abcdefgx -> 7, shared 6: 2*6/14.
		want := 12.0 / 14.0
		if got := TitleSimilarity("abcdefgh", "abcdefgx"); math.Abs(got-want) > 1e-9 {
			t.Errorf("similarity = %v, want %v", got, want)
		}
	})
}

func TestMatchScore(t *testing.T) {
	t.Parallel()

	const base = 12.0 / 14.0 // similarity of "abcdefgh" and "abcdefgx"

	t.Run("MID-002/plataforma e ano somam 0,05 cada", func(t *testing.T) {
		t.Parallel()
		game := Candidate{Title: "abcdefgh", Platform: "Windows", Year: 2018}
		cases := []struct {
			name string
			cand Candidate
			want float64
		}{
			{"nada bate", Candidate{Title: "abcdefgx", Platform: "Switch", Year: 2010}, base},
			{"só a plataforma", Candidate{Title: "abcdefgx", Platform: "windows", Year: 2010}, base + 0.05},
			{"só o ano", Candidate{Title: "abcdefgx", Platform: "Switch", Year: 2018}, base + 0.05},
			{"plataforma e ano", Candidate{Title: "abcdefgx", Platform: "Windows", Year: 2018}, base + 0.10},
		}
		for _, tc := range cases {
			if got := MatchScore(game, tc.cand); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("%s: score = %v, want %v", tc.name, got, tc.want)
			}
		}
	})

	t.Run("MID-002/a pontuação não passa de 1", func(t *testing.T) {
		t.Parallel()
		c := Candidate{Title: "Celeste", Platform: "Windows", Year: 2018}
		if got := MatchScore(c, c); got != 1 {
			t.Errorf("score = %v, want 1", got)
		}
	})

	t.Run("PLT-031/MatchScore dá o bônus de plataforma para Windows × Windows", func(t *testing.T) {
		t.Parallel()
		const wantBase = 12.0 / 14.0
		game := Candidate{Title: "abcdefgh", Platform: "Windows"}
		withBonus := MatchScore(game, Candidate{Title: "abcdefgx", Platform: "Windows"})
		without := MatchScore(game, Candidate{Title: "abcdefgx", Platform: "PC"})
		if math.Abs(withBonus-(wantBase+0.05)) > 1e-9 {
			t.Errorf("Windows x Windows = %v, want %v", withBonus, wantBase+0.05)
		}
		if math.Abs(without-wantBase) > 1e-9 {
			t.Errorf("Windows x PC = %v, want %v (no bonus)", without, wantBase)
		}
	})
}

func TestMatchAccepted(t *testing.T) {
	t.Parallel()

	cases := []struct {
		score float64
		want  bool
	}{
		{0.79, false},
		{0.80, true},
		{0.95, true},
	}
	for _, tc := range cases {
		if got := MatchAccepted(tc.score); got != tc.want {
			t.Errorf("MatchAccepted(%v) = %v, want %v", tc.score, got, tc.want)
		}
	}
}
