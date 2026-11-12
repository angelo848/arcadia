package domain_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// seqIDs devolve 1..n, ids positivos e distintos.
func seqIDs(n int) []int64 {
	ids := make([]int64, 0, n)
	for i := 1; i <= n; i++ {
		ids = append(ids, int64(i))
	}
	return ids
}

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }
func boolPtr(v bool) *bool    { return &v }

// fullFilter usa todos os membros de FilterSpec com valores válidos.
func fullFilter() domain.FilterSpec {
	return domain.FilterSpec{
		Version:        domain.FilterVersion,
		PlatformIDs:    []int64{1, 2},
		GenreIDs:       []int64{5},
		DeveloperIDs:   []int64{7},
		PublisherIDs:   []int64{8},
		PlayMode:       domain.PlayGroupMulti,
		YearFrom:       intPtr(1998),
		YearTo:         intPtr(2005),
		Statuses:       []domain.GameStatus{domain.StatusPlaying, domain.StatusBacklog},
		Favorite:       boolPtr(true),
		Played:         "played",
		PlaySecondsMin: int64Ptr(3600),
		PlaySecondsMax: int64Ptr(7200),
		Text:           "hollow knight",
	}
}

func TestFilterValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		valid   []domain.FilterSpec
		invalid []domain.FilterSpec
	}{
		{
			name: "FLT-001/{version:1} e filtro cheio válido passam",
			valid: []domain.FilterSpec{
				{Version: 1},
				fullFilter(),
			},
		},
		{
			name: "FLT-001/version ausente, 0 ou 2 é erro",
			invalid: []domain.FilterSpec{
				{},
				{Version: 0},
				{Version: 2},
			},
		},
		{
			name: "FLT-001/id zero ou negativo, 51 ids numa faceta e ids repetidos são erro",
			invalid: []domain.FilterSpec{
				{Version: 1, PlatformIDs: []int64{0}},
				{Version: 1, GenreIDs: []int64{-1}},
				{Version: 1, DeveloperIDs: []int64{1, 0}},
				{Version: 1, PlatformIDs: seqIDs(51)},
				{Version: 1, PublisherIDs: []int64{3, 3}},
			},
		},
		{
			name: "FLT-001/yearFrom > yearTo e playSecondsMin > playSecondsMax são erro",
			invalid: []domain.FilterSpec{
				{Version: 1, YearFrom: intPtr(2005), YearTo: intPtr(2000)},
				{Version: 1, PlaySecondsMin: int64Ptr(10), PlaySecondsMax: int64Ptr(5)},
			},
		},
		{
			name: "FLT-001/text com 201 runas depois de cortar é erro e 200 passa",
			valid: []domain.FilterSpec{
				{Version: 1, Text: runes('a', 200)},
				{Version: 1, Text: "  " + runes('a', 200) + "  "},
			},
			invalid: []domain.FilterSpec{
				{Version: 1, Text: runes('a', 201)},
				{Version: 1, Text: "  " + runes('a', 201) + "  "},
			},
		},
		{
			name: "FLT-005/statuses com valor inválido, repetido ou mais de 5 é erro",
			invalid: []domain.FilterSpec{
				{Version: 1, Statuses: []domain.GameStatus{"Playing"}},
				{Version: 1, Statuses: []domain.GameStatus{"bogus"}},
				{Version: 1, Statuses: []domain.GameStatus{domain.StatusPlaying, domain.StatusPlaying}},
				// Só há cinco status válidos: seis entradas sempre repetem algum, então o erro vale pelos dois motivos.
				{Version: 1, Statuses: []domain.GameStatus{
					domain.StatusPlaying, domain.StatusBacklog, domain.StatusCompleted,
					domain.StatusAbandoned, domain.StatusNotPlayed, domain.StatusPlaying,
				}},
			},
		},
		{
			name: "FLT-006/played fora de any, played e notPlayed é erro",
			invalid: []domain.FilterSpec{
				{Version: 1, Played: "Played"},
				{Version: 1, Played: "notplayed"},
				{Version: 1, Played: "all"},
				{Version: 1, Played: "true"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, f := range tc.valid {
				if err := f.Validate(); err != nil {
					t.Errorf("Validate(%+v) error: %v, want nil", f, err)
				}
			}
			for _, f := range tc.invalid {
				if err := f.Validate(); !errors.Is(err, domain.ErrInvalidFilter) {
					t.Errorf("Validate(%+v) = %v, want ErrInvalidFilter", f, err)
				}
			}
		})
	}
}

// Casos extras de FLT-001..FLT-006 que a tabela acima não cobre pelo nome.
func TestFilterValidateExtra(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		valid   []domain.FilterSpec
		invalid []domain.FilterSpec
	}{
		{
			name: "FLT-001/50 ids numa faceta passam",
			valid: []domain.FilterSpec{
				{Version: 1, PlatformIDs: seqIDs(50)},
			},
		},
		{
			name: "FLT-004/anos fora de 1900..2100 são erro e 1900 a 2100 passam",
			valid: []domain.FilterSpec{
				{Version: 1, YearFrom: intPtr(1900), YearTo: intPtr(2100)},
				{Version: 1, YearFrom: intPtr(1900)},
				{Version: 1, YearTo: intPtr(2100)},
			},
			invalid: []domain.FilterSpec{
				{Version: 1, YearFrom: intPtr(1899)},
				{Version: 1, YearTo: intPtr(2101)},
			},
		},
		{
			name: "FLT-006/segundos negativos são erro",
			valid: []domain.FilterSpec{
				{Version: 1, PlaySecondsMin: int64Ptr(0), PlaySecondsMax: int64Ptr(0)},
			},
			invalid: []domain.FilterSpec{
				{Version: 1, PlaySecondsMin: int64Ptr(-1)},
				{Version: 1, PlaySecondsMax: int64Ptr(-1)},
			},
		},
		{
			name: "FLT-005/cinco statuses distintos passam e lista vazia não filtra",
			valid: []domain.FilterSpec{
				{Version: 1, Statuses: []domain.GameStatus{
					domain.StatusNotPlayed, domain.StatusPlaying, domain.StatusCompleted,
					domain.StatusAbandoned, domain.StatusBacklog,
				}},
				{Version: 1, Statuses: []domain.GameStatus{}},
			},
		},
		{
			name: "FLT-006/any, played e notPlayed passam, e played ausente também",
			valid: []domain.FilterSpec{
				{Version: 1, Played: "any"},
				{Version: 1, Played: "played"},
				{Version: 1, Played: "notPlayed"},
				{Version: 1, Played: ""},
			},
		},
		{
			name: "FLT-003/playMode fora de all, single, multi e coop é erro",
			valid: []domain.FilterSpec{
				{Version: 1, PlayMode: ""},
				{Version: 1, PlayMode: domain.PlayGroupAll},
				{Version: 1, PlayMode: domain.PlayGroupSingle},
				{Version: 1, PlayMode: domain.PlayGroupMulti},
				{Version: 1, PlayMode: domain.PlayGroupCoop},
			},
			invalid: []domain.FilterSpec{
				{Version: 1, PlayMode: "multi_local"},
				{Version: 1, PlayMode: "Multi"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, f := range tc.valid {
				if err := f.Validate(); err != nil {
					t.Errorf("Validate(%+v) error: %v, want nil", f, err)
				}
			}
			for _, f := range tc.invalid {
				if err := f.Validate(); !errors.Is(err, domain.ErrInvalidFilter) {
					t.Errorf("Validate(%+v) = %v, want ErrInvalidFilter", f, err)
				}
			}
		})
	}
}

func TestFilterJSON(t *testing.T) {
	t.Parallel()

	t.Run("FLT-001/FilterSpec completo serializado e lido de volta é igual ao original", func(t *testing.T) {
		t.Parallel()

		orig := fullFilter()
		raw, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var back domain.FilterSpec
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("Unmarshal(%s): %v", raw, err)
		}
		if !reflect.DeepEqual(orig, back) {
			t.Errorf("round trip differs:\n got  %+v\n want %+v\n json %s", back, orig, raw)
		}
	})

	t.Run("FLT-001/nomes JSON do contrato v0.6.0 (platformIds, yearFrom, playSecondsMin, played...)", func(t *testing.T) {
		t.Parallel()

		raw, err := json.Marshal(fullFilter())
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keys); err != nil {
			t.Fatalf("Unmarshal to map: %v", err)
		}
		want := []string{
			"version", "platformIds", "genreIds", "developerIds", "publisherIds", "playMode",
			"yearFrom", "yearTo", "statuses", "favorite", "played", "playSecondsMin", "playSecondsMax", "text",
		}
		for _, k := range want {
			if _, ok := keys[k]; !ok {
				t.Errorf("JSON sem a chave %q: %s", k, raw)
			}
		}
	})

	t.Run("FLT-001/membros ausentes ou null não filtram ao ler", func(t *testing.T) {
		t.Parallel()

		var f domain.FilterSpec
		if err := json.Unmarshal([]byte(`{"version":1,"yearFrom":null,"favorite":null}`), &f); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if f.YearFrom != nil || f.Favorite != nil || f.YearTo != nil {
			t.Errorf("null deveria deixar ponteiros nil: %+v", f)
		}
		if err := f.Validate(); err != nil {
			t.Errorf("Validate(%+v) error: %v", f, err)
		}
	})
}

func TestFilterNormalize(t *testing.T) {
	t.Parallel()

	t.Run("FLT-001/text sem espaços nas pontas; listas ordenadas e sem repetição; PlayMode vazio vira all e Played vazio vira any", func(t *testing.T) {
		t.Parallel()

		in := domain.FilterSpec{
			Version:      1,
			Text:         "  hollow knight  ",
			PlatformIDs:  []int64{3, 1, 3, 2},
			GenreIDs:     []int64{9, 5, 9},
			DeveloperIDs: []int64{4, 4},
			PublisherIDs: []int64{6, 2},
			Statuses:     []domain.GameStatus{domain.StatusBacklog, domain.StatusPlaying, domain.StatusBacklog},
		}
		got := in.Normalize()

		if got.Text != "hollow knight" {
			t.Errorf("Text = %q, want %q", got.Text, "hollow knight")
		}
		if want := []int64{1, 2, 3}; !reflect.DeepEqual(got.PlatformIDs, want) {
			t.Errorf("PlatformIDs = %v, want %v", got.PlatformIDs, want)
		}
		if want := []int64{5, 9}; !reflect.DeepEqual(got.GenreIDs, want) {
			t.Errorf("GenreIDs = %v, want %v", got.GenreIDs, want)
		}
		if want := []int64{4}; !reflect.DeepEqual(got.DeveloperIDs, want) {
			t.Errorf("DeveloperIDs = %v, want %v", got.DeveloperIDs, want)
		}
		if want := []int64{2, 6}; !reflect.DeepEqual(got.PublisherIDs, want) {
			t.Errorf("PublisherIDs = %v, want %v", got.PublisherIDs, want)
		}
		if want := []domain.GameStatus{domain.StatusBacklog, domain.StatusPlaying}; !reflect.DeepEqual(got.Statuses, want) {
			t.Errorf("Statuses = %v, want %v", got.Statuses, want)
		}
		if got.PlayMode != domain.PlayGroupAll {
			t.Errorf("PlayMode = %q, want %q", got.PlayMode, domain.PlayGroupAll)
		}
		if got.Played != "any" {
			t.Errorf("Played = %q, want %q", got.Played, "any")
		}
	})

	t.Run("FLT-001/Normalize é idempotente e o resultado continua válido", func(t *testing.T) {
		t.Parallel()

		once := domain.FilterSpec{Version: 1, PlatformIDs: []int64{2, 1, 2}, Text: " a "}.Normalize()
		twice := once.Normalize()
		if !reflect.DeepEqual(once, twice) {
			t.Errorf("Normalize não é idempotente:\n once  %+v\n twice %+v", once, twice)
		}
		if err := twice.Validate(); err != nil {
			t.Errorf("Validate(normalized) error: %v", err)
		}
	})
}
