package store_test

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

func TestListGamesSort(t *testing.T) {
	t.Parallel()

	t.Run("LIB-001/title com b(1), A(2), a(3), C(4) dá A, a, b, C", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "b"}, libGame{Title: "A"}, libGame{Title: "a"}, libGame{Title: "C"},
		)

		page := libRun(t, s, libQuery(libFilter(), domain.SortTitle))
		requireTitles(t, page.Games, "A", "a", "b", "C")
	})

	t.Run("LIB-001/lastPlayed põe nunca jogados por último, com desempate por título e id", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Zeta", LastPlayed: libAt(3, 10)},
			libGame{Title: "Delta", LastPlayed: libAt(3, 10)},
			libGame{Title: "Alpha", LastPlayed: libAt(2, 10)},
			libGame{Title: "Beta"},
			libGame{Title: "gamma"},
		)

		page := libRun(t, s, libQuery(libFilter(), domain.SortLastPlayed))
		requireTitles(t, page.Games, "Delta", "Zeta", "Alpha", "Beta", "gamma")
	})

	t.Run("LIB-001/totalPlayed é decrescente; year é crescente com sem ano por último", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Antigo", Release: "1995-03-11", Plays: 1, Seconds: 100},
			libGame{Title: "Dez", Release: "2001", Plays: 1, Seconds: 500},
			libGame{Title: "Dois", Release: "2001-05-01", Plays: 1, Seconds: 500},
			libGame{Title: "Novo", Release: "2010", Plays: 1, Seconds: 50},
			libGame{Title: "SemAno"},
		)

		byTotal := libRun(t, s, libQuery(libFilter(), domain.SortTotalPlayed))
		requireTitles(t, byTotal.Games, "Dez", "Dois", "Antigo", "Novo", "SemAno")

		byYear := libRun(t, s, libQuery(libFilter(), domain.SortYear))
		requireTitles(t, byYear.Games, "Antigo", "Dez", "Dois", "Novo", "SemAno")
	})

	t.Run("LIB-001/busca por texto usa o mesmo sort, sem ranking", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Alpha", Description: "metroidvania", Plays: 1, Seconds: 10},
			libGame{Title: "Beta", Description: "metroidvania", Plays: 1, Seconds: 999},
			libGame{Title: "Gama", Description: "platformer", Plays: 1, Seconds: 5000},
		)
		f := domain.FilterSpec{Version: 1, Text: "metroidvania"}

		requireTitles(t, libRun(t, s, libQuery(f, domain.SortTotalPlayed)).Games, "Beta", "Alpha")
		requireTitles(t, libRun(t, s, libQuery(f, domain.SortTitle)).Games, "Alpha", "Beta")
	})

	t.Run("GAM-005/ordem padrão é a de antes: título sem diferenciar maiúsculas e depois id", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "b"}, libGame{Title: "A"}, libGame{Title: "a"}, libGame{Title: "C"},
		)

		page := libRun(t, s, libQuery(libFilter(), domain.DefaultSort))
		requireTitles(t, page.Games, "A", "a", "b", "C")
		wantIDs := []int64{2, 3, 1, 4}
		for i, g := range page.Games {
			if g.ID != wantIDs[i] {
				t.Errorf("game %d id = %d, want %d (ties broken by id)", i, g.ID, wantIDs[i])
			}
		}
	})
}

func TestListGamesPage(t *testing.T) {
	t.Parallel()

	t.Run("LIB-002/450 jogos com limit 200 dão páginas de 200, 200 e 50 sem repetição nem falta", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		// Titles repeat (100 distinct, 4 or 5 games each) so the id tie-break is exercised.
		games := make([]libGame, 0, 450)
		for i := range 450 {
			games = append(games, libGame{Title: fmt.Sprintf("Jogo %03d", i%100)})
		}
		ids := seedLibraryIDs(t, s, games...)

		type entry struct {
			title string
			id    int64
		}
		want := make([]entry, 0, len(games))
		for i, g := range games {
			want = append(want, entry{g.Title, ids[i]})
		}
		slices.SortFunc(want, func(a, b entry) int {
			if c := strings.Compare(a.title, b.title); c != 0 {
				return c
			}
			return cmp.Compare(a.id, b.id)
		})

		var got []int64
		for i, off := range []int{0, 200, 400} {
			page := libRun(t, s, store.Query{Filter: libFilter(), Sort: domain.SortTitle, Offset: off, Limit: 200})
			if wantLen := []int{200, 200, 50}[i]; len(page.Games) != wantLen {
				t.Errorf("page at offset %d has %d games, want %d", off, len(page.Games), wantLen)
			}
			if page.Total != 450 {
				t.Errorf("Total at offset %d = %d, want 450", off, page.Total)
			}
			for _, g := range page.Games {
				got = append(got, g.ID)
			}
		}
		wantIDs := make([]int64, 0, len(want))
		for _, e := range want {
			wantIDs = append(wantIDs, e.id)
		}
		if !slices.Equal(got, wantIDs) {
			t.Errorf("concatenated pages differ from the full ordered list (%d vs %d ids)", len(got), len(wantIDs))
		}
	})

	t.Run("LIB-002/offset além do fim dá games [] e total correto", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s, libGame{Title: "Um"}, libGame{Title: "Dois"}, libGame{Title: "Tres"})

		page := libRun(t, s, store.Query{Filter: libFilter(), Sort: domain.SortTitle, Offset: 10, Limit: 200})
		if page.Games == nil || len(page.Games) != 0 {
			t.Errorf("Games = %#v, want a non-nil empty slice", page.Games)
		}
		if page.Total != 3 {
			t.Errorf("Total = %d, want 3", page.Total)
		}
	})

	t.Run("LIB-002/limit 0 ou 501 e offset negativo são erro (no store)", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s, libGame{Title: "Um"}, libGame{Title: "Dois"}, libGame{Title: "Tres"})

		for _, q := range []store.Query{
			{Filter: libFilter(), Sort: domain.SortTitle, Limit: 0},
			{Filter: libFilter(), Sort: domain.SortTitle, Limit: 501},
			{Filter: libFilter(), Sort: domain.SortTitle, Offset: -1, Limit: 200},
		} {
			if _, err := s.ListGames(t.Context(), q); err == nil {
				t.Errorf("ListGames(%+v) err = nil, want an error", q)
			}
		}
	})

	t.Run("LIB-002/limit 1 e 500 são aceitos (limites da faixa)", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s, libGame{Title: "Um"}, libGame{Title: "Dois"}, libGame{Title: "Tres"})

		one := libRun(t, s, store.Query{Filter: libFilter(), Sort: domain.SortTitle, Limit: 1})
		requireTitles(t, one.Games, "Dois")
		if one.Total != 3 {
			t.Errorf("Total with limit 1 = %d, want 3", one.Total)
		}
		most := libRun(t, s, store.Query{Filter: libFilter(), Sort: domain.SortTitle, Limit: 500})
		requireTitles(t, most.Games, "Dois", "Tres", "Um")
	})

	t.Run("STO-025/Total é a contagem do filtro, não da página", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "P1", Status: "backlog"},
			libGame{Title: "P2", Status: "backlog"},
			libGame{Title: "P3", Status: "backlog"},
			libGame{Title: "P4", Status: "backlog"},
			libGame{Title: "P5", Status: "backlog"},
			libGame{Title: "Outro", Status: "completed"},
		)
		f := domain.FilterSpec{Version: 1, Statuses: []domain.GameStatus{domain.StatusBacklog}}

		page := libRun(t, s, store.Query{Filter: f, Sort: domain.SortTitle, Offset: 0, Limit: 2})
		requireTitles(t, page.Games, "P1", "P2")
		if page.Total != 5 {
			t.Errorf("Total = %d, want 5 (filter count, not page size %d)", page.Total, len(page.Games))
		}
	})
}

func TestListGamesEmpty(t *testing.T) {
	t.Parallel()

	t.Run("LIB-003/filtro sem resultado e biblioteca vazia dão games [] (não nil) e total 0", func(t *testing.T) {
		t.Parallel()
		empty := openStore(t, dbPath(t), true)
		populated := openStore(t, dbPath(t), true)
		seedLibrary(t, populated, libGame{Title: "Um"})

		cases := []struct {
			name string
			s    *store.Store
			f    domain.FilterSpec
		}{
			{"biblioteca vazia sem filtro", empty, libFilter()},
			{"biblioteca vazia com favorite true", empty, domain.FilterSpec{Version: 1, Favorite: boolPtr(true)}},
			{"filtro sem resultado", populated, domain.FilterSpec{Version: 1, Favorite: boolPtr(true)}},
		}
		for _, c := range cases {
			page := libRun(t, c.s, libQuery(c.f, domain.SortTitle))
			if page.Games == nil || len(page.Games) != 0 {
				t.Errorf("%s: Games = %#v, want a non-nil empty slice", c.name, page.Games)
			}
			if page.Total != 0 {
				t.Errorf("%s: Total = %d, want 0", c.name, page.Total)
			}
		}
	})
}
