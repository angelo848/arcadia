package scrapers

import (
	"slices"
	"testing"
)

func TestMerge(t *testing.T) {
	t.Parallel()

	t.Run("MID-003/prioridade do IGDB antes da Steam Store", func(t *testing.T) {
		t.Parallel()
		got := Merge(map[string]Metadata{
			"igdb":       {Description: "Descrição do IGDB", Year: 2018, Genres: []string{"Platform"}},
			"steamstore": {Description: "Descrição da Steam", Year: 2017, Genres: []string{"Indie"}, Developers: []string{"Dev"}},
		})
		if got.Description != "Descrição do IGDB" || got.Year != 2018 || !slices.Equal(got.Genres, []string{"Platform"}) {
			t.Errorf("Merge = %+v, want the IGDB values", got)
		}
		if !slices.Equal(got.Developers, []string{"Dev"}) {
			t.Errorf("developers = %v, want the Steam value (IGDB had none)", got.Developers)
		}
	})

	t.Run("MID-003/campo sem valor em nenhum provedor fica vazio", func(t *testing.T) {
		t.Parallel()
		got := Merge(map[string]Metadata{"igdb": {Description: "x"}})
		if got.Year != 0 || len(got.Publishers) != 0 || got.Modes != nil && len(got.Modes) != 0 {
			t.Errorf("Merge = %+v, want empty year and lists", got)
		}
	})
}
