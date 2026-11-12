package store

// White-box tests: ftsQuery is unexported (design.md, FTS-005).

import (
	"slices"
	"testing"
)

func TestFTSQuery(t *testing.T) {
	t.Parallel()

	t.Run("FTS-005/palavras viram \"palavra\"* e são unidas por espaço", func(t *testing.T) {
		t.Parallel()
		got, ok := ftsQuery("Hollow Knight")
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if want := `"Hollow"* "Knight"*`; got != want {
			t.Errorf("ftsQuery(%q) = %q, want %q", "Hollow Knight", got, want)
		}
	})

	t.Run("FTS-005/aspas, dois-pontos, asterisco, parênteses, hífen e circunflexo não geram erro de sintaxe", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			in, want string
		}{
			{in: `a"b`, want: `"a"* "b"*`},
			{in: `title:x`, want: `"title"* "x"*`},
			{in: `foo*`, want: `"foo"*`},
			{in: `(bar)`, want: `"bar"*`},
			{in: `well-known`, want: `"well"* "known"*`},
			{in: `^x`, want: `"x"*`},
		}
		for _, c := range cases {
			got, ok := ftsQuery(c.in)
			if !ok {
				t.Errorf("ftsQuery(%q) ok = false, want true", c.in)
				continue
			}
			if got != c.want {
				t.Errorf("ftsQuery(%q) = %q, want %q", c.in, got, c.want)
			}
		}
	})

	t.Run("FTS-005/AND, OR e NOT digitados viram texto e não mudam o sentido", func(t *testing.T) {
		t.Parallel()
		got, ok := ftsQuery("cats AND dogs OR NOT fish")
		if !ok {
			t.Fatal("ok = false, want true")
		}
		want := `"cats"* "AND"* "dogs"* "OR"* "NOT"* "fish"*`
		if got != want {
			t.Errorf("ftsQuery = %q, want %q", got, want)
		}
	})

	t.Run("FTS-005/só pontuação devolve ok = false", func(t *testing.T) {
		t.Parallel()
		for _, in := range []string{"", "   ", `!!! "" ()-*^:`, "...---"} {
			if got, ok := ftsQuery(in); ok {
				t.Errorf("ftsQuery(%q) = %q, ok = true, want ok = false", in, got)
			}
		}
	})
}

func TestFTSQueryMatch(t *testing.T) {
	t.Parallel()

	t.Run("FTS-005/pokemon encontra o jogo com título Pokémon", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g, err := s.AddGame(t.Context(), NewGame{Title: "Pokémon", Exe: `C:\Games\pokemon.exe`, At: ftsAddAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		query, ok := ftsQuery("pokemon")
		if !ok {
			t.Fatal("ftsQuery(pokemon) ok = false, want true")
		}
		if got := ftsMatchRowids(t, s, query); !slices.Equal(got, []int64{g.ID}) {
			t.Errorf("MATCH %q rowids = %v, want [%d]", query, got, g.ID)
		}
	})
}
