package store_test

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// Tests of Store.GameIDs (DEL-010): the ids of the games in the library, read only. The order of the
// result is not part of the contract, so the tests compare sorted copies.

func TestGameIDs(t *testing.T) {
	t.Parallel()

	t.Run("DEL-010/devolve os ids de todos os jogos, sem repetição", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ids := seedLibrary(t, s, libGame{Title: "Lanterna Sumida"}, libGame{Title: "Hades"}, libGame{Title: "Celeste"})
		want := []int64{ids["Lanterna Sumida"], ids["Hades"], ids["Celeste"]}

		got, err := s.GameIDs(t.Context())
		if err != nil {
			t.Fatalf("GameIDs: %v", err)
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("GameIDs = %v, want %v", got, want)
		}
	})

	t.Run("DEL-010/biblioteca vazia devolve lista vazia e nil", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)

		got, err := s.GameIDs(t.Context())
		if err != nil {
			t.Fatalf("GameIDs on an empty library: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("GameIDs on an empty library = %v, want none", got)
		}
	})

	t.Run("DEL-010/jogo removido não aparece na lista", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ids := seedLibrary(t, s, libGame{Title: "Hades"}, libGame{Title: "Celeste"})
		if err := s.DeleteGame(t.Context(), ids["Hades"]); err != nil {
			t.Fatalf("DeleteGame: %v", err)
		}

		got, err := s.GameIDs(t.Context())
		if err != nil {
			t.Fatalf("GameIDs: %v", err)
		}
		if want := []int64{ids["Celeste"]}; !slices.Equal(got, want) {
			t.Errorf("GameIDs after removing Hades = %v, want %v", got, want)
		}
	})

	t.Run("DEL-010/só leitura: as linhas dos jogos e do índice não mudam", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s, libGame{Title: "Hades", Description: "Mapa"}, libGame{Title: "Celeste"})
		gameRows := countRows(t, s.DB(), `SELECT COUNT(*) FROM game`)
		indexRows := countRows(t, s.DB(), `SELECT COUNT(*) FROM game_fts`)

		if _, err := s.GameIDs(t.Context()); err != nil {
			t.Fatalf("GameIDs: %v", err)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game`); n != gameRows {
			t.Errorf("game rows after GameIDs = %d, want %d", n, gameRows)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game_fts`); n != indexRows {
			t.Errorf("game_fts rows after GameIDs = %d, want %d", n, indexRows)
		}
	})

	t.Run("DEL-010/ctx cancelado devolve o erro do contexto", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s, libGame{Title: "Hades"})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := s.GameIDs(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("GameIDs with a cancelled ctx err = %v, want context.Canceled", err)
		}
	})
}
