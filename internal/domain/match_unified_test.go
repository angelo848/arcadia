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

// Casos da T04 (phase-12-metadados-unificados): mesmo jogo, pontuação e confiança.
// Vetores de contracts/match-rules.md §2.3, §2.4 e §3. Pontuação em milésimos (int).
// Onde o contrato dá duas casas (Dice calculado à mão), a comparação usa tolerância de 5 milésimos.
// Cada t.Run começa pelo ID do requisito.

const scoreTolerance = 5 // milésimos

func withinThousandths(got, want, tol int) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func TestSameGame(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a, b Member
		want bool
	}{
		{
			"MET-005/mesma chave, provedores distintos",
			Member{Provider: "igdb", Title: "Hades", Year: 2020},
			Member{Provider: "steamgriddb", Title: "Hades", Year: 2020},
			true,
		},
		{
			"MET-005/Dice 0,90 com marcas iguais junta",
			Member{Provider: "igdb", Title: "Hollow Knight", Year: 2017},
			Member{Provider: "steamstore", Title: "Hollow Knights", Year: 2017},
			true,
		},
		{
			"MET-005/Sonic e Sonic Mania não juntam",
			Member{Provider: "igdb", Title: "Sonic", Year: 1991},
			Member{Provider: "steamstore", Title: "Sonic Mania", Year: 1991},
			false,
		},
		{
			"MET-005/mesmo provedor nunca junta",
			Member{Provider: "igdb", Title: "Hades", Year: 2020},
			Member{Provider: "igdb", Title: "Hades", Year: 2020},
			false,
		},
		{
			"MET-005/diferença de 6 anos não junta",
			Member{Provider: "igdb", Title: "Hades", Year: 2020},
			Member{Provider: "steamgriddb", Title: "Hades", Year: 2026},
			false,
		},
		{
			"MET-005/diferença de 1 ano junta",
			Member{Provider: "igdb", Title: "Hades", Year: 2020},
			Member{Provider: "steamgriddb", Title: "Hades", Year: 2021},
			true,
		},
		{
			"MET-005/ano 0 junta",
			Member{Provider: "igdb", Title: "Hades", Year: 2020},
			Member{Provider: "steamgriddb", Title: "Hades", Year: 0},
			true,
		},
		{
			"MET-005/marcas diferentes não juntam",
			Member{Provider: "igdb", Title: "Resident Evil", Year: 1996},
			Member{Provider: "steamgriddb", Title: "Resident Evil 2", Year: 1996},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := SameGame(tc.a, tc.b); got != tc.want {
				t.Fatalf("SameGame(%q, %q) = %v, want %v", tc.a.Title, tc.b.Title, got, tc.want)
			}
		})
	}

	t.Run("MET-005/a comparação é simétrica", func(t *testing.T) {
		t.Parallel()
		a := Member{Provider: "igdb", Title: "Hollow Knight", Year: 2017}
		b := Member{Provider: "steamstore", Title: "Hollow Knights", Year: 2017}
		if SameGame(a, b) != SameGame(b, a) {
			t.Fatalf("SameGame não é simétrica para %q e %q", a.Title, b.Title)
		}
	})
}

func TestScoreGroup(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		q          Query
		members    []Member
		want       int // milésimos
		tol        int
		wantCapped bool
	}{
		{
			name:    "MET-006/Super Mario World 1,00",
			q:       Query{Title: "Super Mario World"},
			members: []Member{{Provider: "igdb", Title: "Super Mario World"}},
			want:    1000,
		},
		{
			name:    "MET-006/Zelda Majora 1,00",
			q:       Query{Title: "The Legend of Zelda - Majora's Mask"},
			members: []Member{{Provider: "igdb", Title: "The Legend of Zelda: Majora's Mask"}},
			want:    1000,
		},
		{
			name:    "MET-006/Mario & Luigi 1,00",
			q:       Query{Title: "Mario & Luigi"},
			members: []Member{{Provider: "igdb", Title: "Mario and Luigi"}},
			want:    1000,
		},
		{
			name:    "MET-006/FF VII e FF 7 1,00",
			q:       Query{Title: "Final Fantasy VII"},
			members: []Member{{Provider: "igdb", Title: "Final Fantasy 7"}},
			want:    1000,
		},
		{
			name:    "MET-006/Link's Awakening ≈ 0,90 (tolerância 0,005)",
			q:       Query{Title: "Link's Awakening"},
			members: []Member{{Provider: "igdb", Title: "Links Awakening"}},
			want:    900,
			tol:     scoreTolerance,
		},
		{
			name:       "MET-007/Resident Evil × Resident Evil 2 = 0,79",
			q:          Query{Title: "Resident Evil"},
			members:    []Member{{Provider: "igdb", Title: "Resident Evil 2"}},
			want:       790,
			wantCapped: true,
		},
		{
			name:       "MET-007/Final Fantasy VII × VIII = 0,79",
			q:          Query{Title: "Final Fantasy VII"},
			members:    []Member{{Provider: "igdb", Title: "Final Fantasy VIII"}},
			want:       790,
			wantCapped: true,
		},
		{
			name:       "MET-007/Mega Man X × Mega Man = 0,79",
			q:          Query{Title: "Mega Man X"},
			members:    []Member{{Provider: "igdb", Title: "Mega Man"}},
			want:       790,
			wantCapped: true,
		},
		{
			name:       "MET-007/Final Fantasy V × 5 = 0,79",
			q:          Query{Title: "Final Fantasy V"},
			members:    []Member{{Provider: "igdb", Title: "Final Fantasy 5"}},
			want:       790,
			wantCapped: true,
		},
		{
			name:    "MET-006/Pokemon Red Version ≈ 0,71",
			q:       Query{Title: "Pokemon Red"},
			members: []Member{{Provider: "igdb", Title: "Pokemon Red Version"}},
			want:    710,
			tol:     scoreTolerance,
		},
		{
			name: "MET-006/Pokemon Red com plataforma, ano e 2 provedores = 0,83",
			q:    Query{Title: "Pokemon Red", Year: 1998, LibraryKey: "gb"},
			members: []Member{
				{Provider: "igdb", Title: "Pokemon Red Version", Year: 1998, PlatformKeys: []string{"gb"}, PlatformConfirmed: true},
				{Provider: "steamgriddb", Title: "Pokemon Red Version", Year: 1998},
			},
			want: 830,
			tol:  scoreTolerance,
		},
		{
			name:    "MET-006/Sonic ≈ 0,47",
			q:       Query{Title: "Sonic"},
			members: []Member{{Provider: "igdb", Title: "Sonic the Hedgehog"}},
			want:    470,
			tol:     scoreTolerance,
		},
		{
			name: "MET-006/Doom em dois provedores satura em 1,00",
			q:    Query{Title: "Doom"},
			members: []Member{
				{Provider: "igdb", Title: "DOOM"},
				{Provider: "steamgriddb", Title: "DOOM"},
			},
			want: 1000,
		},
		{
			name: "MET-006/bônus de provedor extra limitado a 0,04",
			q:    Query{Title: "Pokemon Red"},
			members: []Member{
				{Provider: "igdb", Title: "Pokemon Red Version"},
				{Provider: "steamstore", Title: "Pokemon Red Version"},
				{Provider: "steamgriddb", Title: "Pokemon Red Version"},
			},
			want: 750,
			tol:  scoreTolerance,
		},
		{
			name:       "MET-007/plataforma não confirmada limita a 0,79",
			q:          Query{Title: "Hades", LibraryKey: "gba"},
			members:    []Member{{Provider: "igdb", Title: "Hades", PlatformKeys: []string{"gba"}, PlatformConfirmed: false}},
			want:       790,
			wantCapped: true,
		},
		{
			name:       "MET-007/membros com plataforma que não inclui a da biblioteca limitam",
			q:          Query{Title: "Hades", LibraryKey: "snes"},
			members:    []Member{{Provider: "igdb", Title: "Hades", PlatformKeys: []string{"gba"}, PlatformConfirmed: true}},
			want:       790,
			wantCapped: true,
		},
		{
			name:    "MET-007/membro só do SteamGridDB (sem plataforma) não trava",
			q:       Query{Title: "Hades", LibraryKey: "snes"},
			members: []Member{{Provider: "steamgriddb", Title: "Hades"}},
			want:    1000,
		},
		{
			name:    "MET-007/biblioteca sem chave de seed nunca trava nem confere",
			q:       Query{Title: "Pokemon Red", Year: 1998, LibraryKey: ""},
			members: []Member{{Provider: "igdb", Title: "Pokemon Red Version", Year: 1998, PlatformKeys: []string{"gba"}, PlatformConfirmed: true}},
			want:    764, // 714 de similaridade + 50 de ano; sem bônus de plataforma e sem trava
			tol:     scoreTolerance,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ScoreGroup(tc.q, tc.members)
			if !withinThousandths(got.Score, tc.want, tc.tol) {
				t.Fatalf("ScoreGroup(%q) Score = %d, want %d (tolerância %d)", tc.q.Title, got.Score, tc.want, tc.tol)
			}
			if got.Capped != tc.wantCapped {
				t.Fatalf("ScoreGroup(%q) Capped = %v, want %v", tc.q.Title, got.Capped, tc.wantCapped)
			}
		})
	}

	t.Run("MET-006/Score e Sim são inteiros em milésimos", func(t *testing.T) {
		t.Parallel()
		got := ScoreGroup(Query{Title: "Super Mario World"}, []Member{{Provider: "igdb", Title: "Super Mario World"}})
		if got.Score != 1000 || got.Sim != 1000 {
			t.Fatalf("Score = %d, Sim = %d; want 1000 e 1000", got.Score, got.Sim)
		}
	})
}

func scored(score, sim int, capped bool) *Scored {
	return &Scored{Score: score, Sim: sim, Capped: capped}
}

func TestClassifyMatch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		first, second *Scored
		want          Confidence
	}{
		{"MET-009/lista vazia = none (primeiro ausente)", nil, nil, ConfidenceNone},
		{"MET-009/850 sim 850 sem segundo = match", scored(850, 850, false), nil, ConfidenceMatch},
		{"MET-009/849 = doubt", scored(849, 849, false), nil, ConfidenceDoubt},
		{"MET-009/900 com sim 780 = doubt (título fraco, bônus)", scored(900, 780, false), nil, ConfidenceDoubt},
		{"MET-009/850 com sim 800 = match (piso de similaridade)", scored(850, 800, false), nil, ConfidenceMatch},
		{"MET-009/850 com sim 799 = doubt", scored(850, 799, false), nil, ConfidenceDoubt},
		{"MET-009/850 com trava não é match", scored(850, 850, true), nil, ConfidenceDoubt},
		{"MET-009/vantagem exata 50 = match (comparação em milésimos: 950 − 900 ≥ 50)", scored(950, 950, false), scored(900, 900, false), ConfidenceMatch},
		{"MET-009/vantagem 49 = doubt", scored(950, 950, false), scored(901, 901, false), ConfidenceDoubt},
		{"MET-009/trava 790 = doubt", scored(790, 920, true), nil, ConfidenceDoubt},
		{"MET-009/600 = doubt", scored(600, 600, false), nil, ConfidenceDoubt},
		{"MET-009/599 = none", scored(599, 599, false), nil, ConfidenceNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyMatch(tc.first, tc.second); got != tc.want {
				t.Fatalf("ClassifyMatch = %q, want %q", got, tc.want)
			}
		})
	}
}
