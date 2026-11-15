package importers

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// titleVector is one row of docs/specs/phase-11-importacao-v2/contracts/import-rules.md, section 2.
type titleVector struct {
	n          int // vector number in import-rules.md §2
	in         string
	fromFolder bool
	want       string
}

// titleVectors are the 40 vectors of import-rules.md §2 (the stem is the input).
var titleVectors = []titleVector{
	{1, "Super Mario World (U) [!]", false, "Super Mario World"},
	{2, "Legend of Zelda, The - Majora's Mask (U) [!]", false, "The Legend of Zelda - Majora's Mask"},
	{3, "Legend of Zelda, The - Ocarina of Time (USA)_redux_patched", false, "The Legend of Zelda - Ocarina of Time redux patched"},
	{4, "Final Fantasy VIII (USA) (Disc 1)", false, "Final Fantasy VIII"},
	{5, "Resident Evil 2 - Dual Shock Ver. (USA) (Disc 1)", false, "Resident Evil 2 - Dual Shock Ver."},
	{6, "Dr. Mario (World)", false, "Dr. Mario"},
	{7, "Super.Mario.World.USA", false, "Super Mario World USA"},
	{8, "Sonic_The_Hedgehog_2_(W)_(REV01)", false, "Sonic The Hedgehog 2"},
	{9, "Pokemon - Red Version (UE) [S][!]", false, "Pokemon - Red Version"},
	{10, "Game [b1]", false, "Game"},
	{11, "Game v1.2 (USA)", false, "Game v1.2"},
	{12, "Game.v1.2.(USA)", false, "Game v1.2"},
	{13, "Super Mario Bros. 3 (U)", false, "Super Mario Bros. 3"},
	{14, "Hello, A World", false, "Hello, A World"},
	{15, "Zelda, The", false, "The Zelda"},
	{16, "Legend of Dragoon, The (USA) (Disc 1)", false, "The Legend of Dragoon"},
	{17, "Bug's Life, A (USA)", false, "A Bug's Life"},
	{18, "Astérix & Obélix (F)", false, "Astérix & Obélix"},
	{19, "(Disc 1)", false, "(Disc 1)"},
	{20, "Game - (USA)", false, "Game"},
	{21, "Game (USA", false, "Game (USA"},
	{22, "Game ((USA))", false, "Game"},
	{23, "A  B\tC", false, "A B C"},
	{24, "Final Fantasy VII (USA) (Disc 1) (v1.1)", false, "Final Fantasy VII"},
	{25, "007 - The World is Not Enough (U) [!]", false, "007 - The World is Not Enough"},
	{26, "Mr. Driller", false, "Mr. Driller"},
	{27, "Smith, Mr.", false, "Smith, Mr."},
	{28, "The Legend of Zelda", false, "The Legend of Zelda"},
	{29, strings.Repeat("é", 250), false, strings.Repeat("é", 200)},
	{30, "31. Spyro - Year of the Dragon (USA)", true, "Spyro - Year of the Dragon"},
	{31, "31. Spyro - Year of the Dragon (USA)", false, "31. Spyro - Year of the Dragon"},
	{32, "1942", true, "1942"},
	{33, "2.5D Game", true, "2.5D Game"},
	{34, "10. 4 Game", true, "10. 4 Game"},
	{35, "roeteekhwuIT,.uD9k_", false, "roeteekhwuIT, uD9k"},
	{36, "Foo, A - Bar, The", false, "A Foo - Bar, The"},
	{37, "The Foo, A", false, "The Foo, A"},
	{38, "Foo , The - Bar", false, "The Foo - Bar"},
	{39, "Super.Mario.World (USA)", false, "Super Mario World"},
	{40, "x.y" + strings.Repeat("a", 200) + " z", false, "x y" + strings.Repeat("a", 197)},
}

// vectorByN returns the vector numbered n of import-rules.md §2.
func vectorByN(t *testing.T, n int) titleVector {
	t.Helper()
	for _, v := range titleVectors {
		if v.n == n {
			return v
		}
	}
	t.Fatalf("no title vector %d", n)
	return titleVector{}
}

func TestSplitExt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, in, stem, ext string
	}{
		{"IMV-001/Mr. Driller.zip", "Mr. Driller.zip", "Mr. Driller", "zip"},
		{"IMV-001/Dr. Mario sem extensão", "Dr. Mario", "Dr. Mario", ""},
		{"IMV-001/Game 2.0 sem extensão", "Game 2.0", "Game 2.0", ""},
		{"IMV-001/extensão em maiúsculas vira minúscula", "Banjo-Kazooie (U) [!].Z64", "Banjo-Kazooie (U) [!]", "z64"},
		{"IMV-001/extensão com 6 caracteres não é extensão", "Jogo.abcdef", "Jogo.abcdef", ""},
		{"IMV-001/só dígitos não é extensão", "Jogo.123", "Jogo.123", ""},
		{"IMV-001/extensão de 5 caracteres é extensão", "Jogo.abc12", "Jogo", "abc12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stem, ext := SplitExt(tc.in)
			if stem != tc.stem || ext != tc.ext {
				t.Errorf("SplitExt(%q) = (%q, %q), want (%q, %q)", tc.in, stem, ext, tc.stem, tc.ext)
			}
		})
	}
}

func TestNormalizeTitle(t *testing.T) {
	t.Parallel()

	// Groups of vectors by rule. Each subtest name starts with the rule ID.
	groups := []struct {
		name string
		nums []int
	}{
		{"IMV-002/grupos múltiplos", []int{1, 9}},
		{"IMV-002/aninhados", []int{22}},
		{"IMV-002/sem par fica", []int{21}},
		{"IMV-002/tudo marcação mantém o nome", []int{19}},
		{"IMV-003/_ vira espaço", []int{8}},
		{"IMV-003/ponto só sem espaços", []int{7, 12}},
		{"IMV-003/ponto entre dígitos", []int{11, 12}},
		{"IMV-003/Dr. Mario e Super Mario Bros. 3 mantêm ponto", []int{6, 13, 26}},
		{"IMV-004/artigo no fim", []int{15, 16, 17}},
		{"IMV-004/antes de \" - \"", []int{2, 3}},
		{"IMV-004/no meio da frase não move", []int{14, 27}},
		{"IMV-005/espaços e tabulação", []int{23}},
		{"IMV-005/traço solto", []int{20}},
		{"IMV-005/250 runas com acento viram 200", []int{29}},
		{"IMV-006/prefixo de pasta", []int{30}},
		{"IMV-006/mesmo texto de arquivo não perde o prefixo", []int{31}},
		{"IMV-006/1942", []int{32}},
		{"IMV-006/2.5D", []int{33}},
		{"IMV-006/10. 4 Game", []int{34}},
		{"IMV-003/005/_ final some na aparagem e ponto vira espaço", []int{35}},
		{"IMV-004/036 só a primeira ocorrência move", []int{36}},
		{"IMV-004/037 título que já começa com artigo não move", []int{37}},
		{"IMV-004/038 espaço antes da vírgula não sobra", []int{38}},
		{"IMV-003/039 pontos viram espaço depois de tirar o grupo", []int{39}},
		{"IMV-005/040 corte em 200 runas antes de decidir o espaço", []int{40}},
	}
	for _, g := range groups {
		t.Run(g.name, func(t *testing.T) {
			t.Parallel()
			for _, n := range g.nums {
				v := vectorByN(t, n)
				if got := NormalizeTitle(v.in, v.fromFolder); got != v.want {
					t.Errorf("vector %d: NormalizeTitle(%q, %v) = %q, want %q", v.n, v.in, v.fromFolder, got, v.want)
				}
			}
		})
	}

	t.Run("IMV-004/antes de ': '", func(t *testing.T) {
		t.Parallel()
		in := "Zelda, The: Majora's Mask"
		want := "The Zelda: Majora's Mask"
		if got := NormalizeTitle(in, false); got != want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", in, got, want)
		}
	})

	for _, tc := range []struct{ name, in, want string }{
		{"IMV-004/artigos estrangeiros/Le", "Petit Prince, Le", "Le Petit Prince"},
		{"IMV-004/artigos estrangeiros/El", "Caballero, El - Aventura", "El Caballero - Aventura"},
		{"IMV-004/artigos estrangeiros/Das", "Spiel, Das", "Das Spiel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeTitle(tc.in, false); got != tc.want {
				t.Errorf("NormalizeTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("IMV-004/só o primeiro", func(t *testing.T) {
		t.Parallel()
		in := "Hello, The - Mundo, The"
		want := "The Hello - Mundo, The"
		if got := NormalizeTitle(in, false); got != want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", in, got, want)
		}
	})

	t.Run("IMV-005/nome vazio não vira título", func(t *testing.T) {
		t.Parallel()
		for _, in := range []string{"", "   "} {
			if got := NormalizeTitle(in, false); got != "" {
				t.Errorf("NormalizeTitle(%q) = %q, want empty", in, got)
			}
		}
	})
}

func TestNormalizeTitleIdempotent(t *testing.T) {
	t.Parallel()

	t.Run("IMV-008/vetores 1 a 29 e 35 a 40 são idempotentes", func(t *testing.T) {
		t.Parallel()
		for _, v := range titleVectors {
			if v.n > 29 && v.n < 35 {
				continue
			}
			once := NormalizeTitle(v.in, false)
			twice := NormalizeTitle(once, false)
			if twice != once {
				t.Errorf("vector %d: normalizing twice gave %q, once gave %q", v.n, twice, once)
			}
		}
	})

	t.Run("IMV-008/propriedade sobre conjunto de nomes com semente fixa", func(t *testing.T) {
		t.Parallel()
		const alphabet = "()[]_.-, abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		r := rand.New(rand.NewPCG(20261115, 11))
		for i := range 2000 {
			n := 1 + r.IntN(24)
			b := make([]byte, n)
			for j := range b {
				b[j] = alphabet[r.IntN(len(alphabet))]
			}
			name := string(b)
			once := NormalizeTitle(name, false)
			if again := NormalizeTitle(name, false); again != once {
				t.Fatalf("name #%d %q: not deterministic: %q then %q", i, name, once, again)
			}
			if twice := NormalizeTitle(once, false); twice != once {
				t.Fatalf("name #%d %q: not idempotent: %q then %q", i, name, once, twice)
			}
		}
	})
}

func TestDiscNumber(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, in string
		want     int
	}{
		{"IMV-021/(Disc 2)", "Jogo (Disc 2)", 2},
		{"IMV-021/(Disk 2)", "Jogo (Disk 2)", 2},
		{"IMV-021/(CD 2)", "Jogo (CD 2)", 2},
		{"IMV-021/(Disc 2 of 3)", "Jogo (Disc 2 of 3)", 2},
		{"IMV-021/[Disc 2]", "Jogo [Disc 2]", 2},
		{"IMV-021/sem marcador é 0", "Jogo (USA)", 0},
		{"IMV-021/N de 1 a 99", "Jogo (Disc 1)", 1},
		{"IMV-021/N de 1 a 99/99", "Jogo (Disc 99)", 99},
		{"IMV-021/100 não casa", "Jogo (Disc 100)", 0},
		{"IMV-021/primeiro marcador vence", "Jogo (Disc 2) (Disc 1)", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := DiscNumber(tc.in); got != tc.want {
				t.Errorf("DiscNumber(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestTitleKey(t *testing.T) {
	t.Parallel()

	t.Run("IMV-021/chave ignora marcador de disco e maiúsculas", func(t *testing.T) {
		t.Parallel()
		a := titleKey("Final Fantasy VIII (USA) (Disc 1)")
		b := titleKey("FINAL FANTASY VIII (usa) (Disc 2)")
		if a != "final fantasy viii" {
			t.Errorf("titleKey(disc 1) = %q, want %q", a, "final fantasy viii")
		}
		if a != b {
			t.Errorf("titleKey of two discs differ: %q and %q", a, b)
		}
	})
}
