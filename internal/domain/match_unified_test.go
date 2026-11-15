package domain

import (
	"reflect"
	"testing"
)

// Casos da T03 (phase-12-metadados-unificados): termo de busca, chave de comparação e marcas.
// Vetores de contracts/match-rules.md §2.1 e §5.1. Cada t.Run começa pelo ID do requisito.

func TestSearchTerm(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		title string
		want  string
	}{
		{"MET-004/Legend of Zelda - Majora's Mask (USA)", "Legend of Zelda - Majora's Mask (USA)", "Legend of Zelda: Majora's Mask"},
		{"MET-004/Final Fantasy VII™", "Final Fantasy VII™", "Final Fantasy VII"},
		{"MET-004/Spider-Man mantém hífen", "Spider-Man", "Spider-Man"},
		{"MET-004/Game [!] (Disc 1)", "Game [!] (Disc 1)", "Game"},
		{"MET-004/Foo  -  Bar", "Foo  -  Bar", "Foo: Bar"},
		{"MET-004/A - B - C troca só a primeira", "A - B - C", "A: B - C"},
		{"MET-004/(Demo) vira vazio", "(Demo)", ""},
		{"MET-004/® e © removidos", "Tetris® © Edition", "Tetris Edition"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := SearchTerm(tc.title); got != tc.want {
				t.Fatalf("SearchTerm(%q) = %q, want %q", tc.title, got, tc.want)
			}
		})
	}

	t.Run("MET-004/o título original não é alterado", func(t *testing.T) {
		t.Parallel()
		title := "Legend of Zelda - Majora's Mask (USA)"
		first := SearchTerm(title)
		second := SearchTerm(title)
		if title != "Legend of Zelda - Majora's Mask (USA)" {
			t.Fatalf("título de entrada alterado para %q", title)
		}
		if first != second {
			t.Fatalf("função não é pura: %q != %q", first, second)
		}
	})
}

func TestMatchKey(t *testing.T) {
	t.Parallel()

	t.Run("MET-006/Super Mario World igual", func(t *testing.T) {
		t.Parallel()
		if got := MatchKey("Super Mario World"); got != "super mario world" {
			t.Fatalf("MatchKey = %q, want %q", got, "super mario world")
		}
	})

	t.Run("MET-006/Zelda - Majora's Mask e Zelda: Majora's Mask têm a mesma chave", func(t *testing.T) {
		t.Parallel()
		a := MatchKey("Zelda - Majora's Mask")
		b := MatchKey("Zelda: Majora's Mask")
		if a != b {
			t.Fatalf("chaves diferentes: %q x %q", a, b)
		}
	})

	t.Run("MET-006/Mario & Luigi e Mario and Luigi", func(t *testing.T) {
		t.Parallel()
		a := MatchKey("Mario & Luigi")
		b := MatchKey("Mario and Luigi")
		if a != "mario luigi" || b != "mario luigi" {
			t.Fatalf("MatchKey = %q e %q, want ambas %q", a, b, "mario luigi")
		}
	})

	t.Run("MET-006/Final Fantasy VII e Final Fantasy 7", func(t *testing.T) {
		t.Parallel()
		a := MatchKey("Final Fantasy VII")
		b := MatchKey("Final Fantasy 7")
		if a != "final fantasy 7" || b != "final fantasy 7" {
			t.Fatalf("MatchKey = %q e %q, want ambas %q", a, b, "final fantasy 7")
		}
	})

	t.Run("MET-006/romanos ii a xx viram número; i, v e x sozinhos não", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			in, want string
		}{
			{"Rocky II", "rocky 2"},
			{"Rocky iii", "rocky 3"},
			{"Rocky IV", "rocky 4"},
			{"Rocky VI", "rocky 6"},
			{"Rocky VII", "rocky 7"},
			{"Rocky VIII", "rocky 8"},
			{"Rocky IX", "rocky 9"},
			{"Rocky XI", "rocky 11"},
			{"Rocky XII", "rocky 12"},
			{"Rocky XIII", "rocky 13"},
			{"Rocky XIV", "rocky 14"},
			{"Rocky XV", "rocky 15"},
			{"Rocky XVI", "rocky 16"},
			{"Rocky XVII", "rocky 17"},
			{"Rocky XVIII", "rocky 18"},
			{"Rocky XIX", "rocky 19"},
			{"Rocky XX", "rocky 20"},
			{"Mega Man X", "mega man x"},
			{"Final Fantasy V", "final fantasy v"},
			{"Rocky I", "rocky i"},
		}
		for _, tc := range cases {
			if got := MatchKey(tc.in); got != tc.want {
				t.Errorf("MatchKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	})

	t.Run("MET-006/the, a, an somem", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			in, want string
		}{
			{"The Cat", "cat"},
			{"A Hat in Time", "hat in time"},
			{"An Ocean", "ocean"},
			{"The a an and Thing", "thing"},
		}
		for _, tc := range cases {
			if got := MatchKey(tc.in); got != tc.want {
				t.Errorf("MatchKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	})

	t.Run("MET-006/acentos e palavras de edição", func(t *testing.T) {
		t.Parallel()
		if a, b := MatchKey("Pokémon Red"), MatchKey("Pokemon Red"); a != b {
			t.Errorf("acento muda a chave: %q x %q", a, b)
		}
		if a, b := MatchKey("Cuphead Remastered"), MatchKey("Cuphead"); a != b {
			t.Errorf("palavra de edição não sai: %q x %q", a, b)
		}
		if a, b := MatchKey("Pokemon Red Edition"), MatchKey("Pokemon Red"); a != b {
			t.Errorf("'Edition' não sai: %q x %q", a, b)
		}
	})
}

func TestMarks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		key  string
		want []string
	}{
		{"MET-007/mega man x → [x]", "mega man x", []string{"x"}},
		{"MET-007/final fantasy 7 → [7]", "final fantasy 7", []string{"7"}},
		{"MET-007/link s awakening → []", "link s awakening", []string{}},
		{"MET-007/don t → []", "don t", []string{}},
		{"MET-007/ordem de aparição preservada", "mega man x 2", []string{"x", "2"}},
		{"MET-007/final fantasy v → [v]", "final fantasy v", []string{"v"}},
		{"MET-007/chave vazia → []", "", []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Marks(tc.key)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Marks(%q) = %#v, want %#v", tc.key, got, tc.want)
			}
		})
	}
}
