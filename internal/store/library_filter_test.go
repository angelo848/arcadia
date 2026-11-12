package store_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

func TestListGamesFilter(t *testing.T) {
	t.Parallel()

	t.Run("FLT-002/platformIds [1,2] com genreIds [5] dá [A, B]; platformIds [1] com genreIds [5,6] dá [A, C]", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "A", Platform: "PC", Genres: []string{"RPG"}},
			libGame{Title: "B", Platform: "Switch", Genres: []string{"RPG"}},
			libGame{Title: "C", Platform: "PC", Genres: []string{"Ação"}},
		)
		pc := libLookup(t, s, "platform", "PC")
		sw := libLookup(t, s, "platform", "Switch")
		rpg := libLookup(t, s, "genre", "RPG")
		acao := libLookup(t, s, "genre", "Ação")

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1,
			PlatformIDs: []int64{pc, sw}, GenreIDs: []int64{rpg}}), "A", "B")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1,
			PlatformIDs: []int64{pc}, GenreIDs: []int64{rpg, acao}}), "A", "C")
	})

	t.Run("FLT-002/developerIds e publisherIds seguem a mesma regra", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "A", Developers: []string{"Dev D1"}, Publishers: []string{"Pub X"}},
			libGame{Title: "B", Developers: []string{"Dev D2"}, Publishers: []string{"Pub X"}},
			libGame{Title: "C", Developers: []string{"Dev D1"}, Publishers: []string{"Pub Y"}},
		)
		d1 := libLookup(t, s, "company", "Dev D1")
		d2 := libLookup(t, s, "company", "Dev D2")
		x := libLookup(t, s, "company", "Pub X")
		y := libLookup(t, s, "company", "Pub Y")

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1,
			DeveloperIDs: []int64{d1, d2}, PublisherIDs: []int64{x}}), "A", "B")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1,
			DeveloperIDs: []int64{d1}, PublisherIDs: []int64{x, y}}), "A", "C")
	})

	t.Run("FLT-003/playMode multi dá [M]; single dá [S]; coop dá []; all ou ausente dá [M, S, N]", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "M", Modes: []string{"multi_local"}},
			libGame{Title: "S", Modes: []string{"single"}},
			libGame{Title: "N"},
		)

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, PlayMode: domain.PlayGroupMulti}), "M")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, PlayMode: domain.PlayGroupSingle}), "S")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, PlayMode: domain.PlayGroupCoop}))
		// Results are in title order, so the set [M, S, N] comes out as M, N, S.
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, PlayMode: domain.PlayGroupAll}), "M", "N", "S")
		requireTitles(t, libSearch(t, s, libFilter()), "M", "N", "S")
	})

	t.Run("FLT-004/yearFrom 2000 dá [2005]; yearFrom 1998 e yearTo 2005 dá [1998, 2005]", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Ano1998", Release: "1998-06-12"},
			libGame{Title: "Ano2005", Release: "2005"},
			libGame{Title: "SemAno"},
		)

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, YearFrom: intPtr(2000)}), "Ano2005")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, YearFrom: intPtr(1998), YearTo: intPtr(2005)}),
			"Ano1998", "Ano2005")
	})

	t.Run("FLT-004/com limite, jogo sem ano fica fora; sem limite, jogo sem ano entra", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Ano1998", Release: "1998-06-12"},
			libGame{Title: "Ano2005", Release: "2005"},
			libGame{Title: "SemAno"},
		)

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, YearFrom: intPtr(1900)}), "Ano1998", "Ano2005")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, YearTo: intPtr(2100)}), "Ano1998", "Ano2005")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, YearFrom: nil, YearTo: nil}),
			"Ano1998", "Ano2005", "SemAno")
	})

	t.Run("FLT-004/ano vale só com YYYY, YYYY-MM ou YYYY-MM-DD: 1998abc não tem ano", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Ano2001", Release: "2001-05"},
			libGame{Title: "Estranho", Release: "1998abc"},
		)

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, YearFrom: intPtr(1900), YearTo: intPtr(2100)}), "Ano2001")
		requireTitles(t, libSearch(t, s, libFilter()), "Ano2001", "Estranho")
	})

	t.Run("FLT-005/statuses é any-of e lista vazia não filtra", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Jogando", Status: "playing"},
			libGame{Title: "Zerado", Status: "completed"},
			libGame{Title: "Fila", Status: "backlog"},
		)

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1,
			Statuses: []domain.GameStatus{domain.StatusPlaying, domain.StatusBacklog}}), "Fila", "Jogando")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1,
			Statuses: []domain.GameStatus{}}), "Fila", "Jogando", "Zerado")
	})

	t.Run("FLT-005/favorite true só favoritos, false só não favoritos, null não filtra", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Fav", Favorite: true},
			libGame{Title: "NaoFav"},
		)

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, Favorite: boolPtr(true)}), "Fav")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, Favorite: boolPtr(false)}), "NaoFav")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, Favorite: nil}), "Fav", "NaoFav")
	})

	t.Run("FLT-006/played dá só jogos com play_count > 0; notPlayed só com 0; any não filtra", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Jogado", Plays: 1, Seconds: 60},
			libGame{Title: "Nunca"},
		)

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, Played: "played"}), "Jogado")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, Played: "notPlayed"}), "Nunca")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, Played: "any"}), "Jogado", "Nunca")
	})

	t.Run("FLT-006/faixa de segundos é inclusiva; jogo com 0 s fica fora com playSecondsMin 1", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Zero"},
			libGame{Title: "Meia", Plays: 1, Seconds: 1800},
			libGame{Title: "Hora", Plays: 1, Seconds: 3600},
			libGame{Title: "Duas", Plays: 1, Seconds: 7200},
			libGame{Title: "Tres", Plays: 1, Seconds: 10800},
		)

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1,
			PlaySecondsMin: int64Ptr(3600), PlaySecondsMax: int64Ptr(7200)}), "Duas", "Hora")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1,
			PlaySecondsMin: int64Ptr(1)}), "Duas", "Hora", "Meia", "Tres")
	})

	t.Run("FLT-007/text combina com os outros filtros; text só com espaços não filtra", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Hollow Knight", Description: "Metroidvania", Developers: []string{"Team Cherry"}},
			libGame{Title: "Ori", Platform: "Switch", Description: "Metroidvania"},
			libGame{Title: "Celeste", Description: "platformer"},
		)
		pc := libLookup(t, s, "platform", "PC")

		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1,
			Text: "metroidvania", PlatformIDs: []int64{pc}}), "Hollow Knight")
		requireTitles(t, libSearch(t, s, domain.FilterSpec{Version: 1, Text: "   "}), "Celeste", "Hollow Knight", "Ori")
	})

	t.Run("FLT-008/filtro vazio devolve a biblioteca inteira e total = número de jogos; com favorite true, total = 1 e página e total batem", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Um"},
			libGame{Title: "Dois", Favorite: true},
			libGame{Title: "Tres"},
		)

		all := libRun(t, s, libQuery(libFilter(), domain.SortTitle))
		requireTitles(t, all.Games, "Dois", "Tres", "Um")
		if all.Total != 3 {
			t.Errorf("Total = %d, want 3", all.Total)
		}

		fav := libRun(t, s, libQuery(domain.FilterSpec{Version: 1, Favorite: boolPtr(true)}, domain.SortTitle))
		requireTitles(t, fav.Games, "Dois")
		if fav.Total != 1 || int64(len(fav.Games)) != fav.Total {
			t.Errorf("favorite page len/Total = %d/%d, want 1/1", len(fav.Games), fav.Total)
		}
	})

	t.Run("STO-025/Total usa o mesmo WHERE da página", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "F1", Favorite: true},
			libGame{Title: "F2", Favorite: true},
			libGame{Title: "N1"},
			libGame{Title: "N2"},
		)
		f := domain.FilterSpec{Version: 1, Favorite: boolPtr(true)}

		first := libRun(t, s, store.Query{Filter: f, Sort: domain.SortTitle, Offset: 0, Limit: 1})
		second := libRun(t, s, store.Query{Filter: f, Sort: domain.SortTitle, Offset: 1, Limit: 1})
		requireTitles(t, first.Games, "F1")
		requireTitles(t, second.Games, "F2")
		if first.Total != 2 || second.Total != 2 {
			t.Errorf("Total = %d and %d, want 2 on both pages (same WHERE as the page)", first.Total, second.Total)
		}
	})

	t.Run("STO-025/página traz platform, status, favorite e ReleaseDate", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s, libGame{Title: "Pagina", Platform: "Switch", Status: "playing", Favorite: true, Release: "1998-06-12"})

		page := libRun(t, s, libQuery(libFilter(), domain.SortTitle))
		if len(page.Games) != 1 {
			t.Fatalf("Games = %+v, want 1 game", page.Games)
		}
		g := page.Games[0]
		if g.Platform != "Switch" || g.PlatformID != libLookup(t, s, "platform", "Switch") {
			t.Errorf("Platform/PlatformID = %q/%d, want Switch/its id", g.Platform, g.PlatformID)
		}
		if g.Status != domain.StatusPlaying || !g.Favorite || g.ReleaseDate != "1998-06-12" {
			t.Errorf("Status/Favorite/ReleaseDate = %q/%v/%q, want playing/true/1998-06-12",
				g.Status, g.Favorite, g.ReleaseDate)
		}
	})

	t.Run("FLT-001/filtro inválido (version diferente de 1) é erro no store e não devolve página", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s, libGame{Title: "Qualquer"})
		for _, f := range []domain.FilterSpec{
			{Version: 0},
			{Version: 2},
			{Version: 1, Statuses: []domain.GameStatus{"lixo"}},
		} {
			if _, err := s.ListGames(t.Context(), libQuery(f, domain.SortTitle)); err == nil {
				t.Errorf("ListGames(%+v) err = nil, want an error", f)
			}
		}
	})
}
