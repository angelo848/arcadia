package store_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Tests of the catalog read and the platform creation (STO-030, EDT-008, IPC-048
// at store level). The catalog is read by library.catalog and the platform list
// by platform.list; the agent tests cover the wire format.

// idName is an expected genre or company item: its database id and its name.
type idName struct {
	ID   int64
	Name string
}

// platformsByName sorts the platforms by name (binary order), for comparisons
// that do not depend on the order returned by the store.
func platformsByName(ps []store.Platform) []store.Platform {
	out := slices.Clone(ps)
	slices.SortFunc(out, func(a, b store.Platform) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func TestCatalog(t *testing.T) {
	t.Parallel()

	t.Run("STO-030/Catalog tem plataformas com id, name e category, gêneros e empresas em ordem NOCASE, e os cinco modos", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s,
			libGame{Title: "Hades", Genres: []string{"rpg", "Action"}, Developers: []string{"team cherry", "Capcom"}},
			libGame{Title: "Zelda", Genres: []string{"Zelda"}, Developers: []string{"bungie"}})
		libExec(t, s.DB(), `INSERT INTO platform (name, category) VALUES ('Switch', 'console')`)

		c, err := s.Catalog(t.Context())
		if err != nil {
			t.Fatalf("Catalog: %v", err)
		}

		wantPlatforms := []store.Platform{
			{ID: libLookup(t, s, "platform", "PC"), Name: "PC", Category: domain.CategoryPC},
			{ID: libLookup(t, s, "platform", "Switch"), Name: "Switch", Category: domain.CategoryConsole},
		}
		if got := platformsByName(c.Platforms); !slices.Equal(got, wantPlatforms) {
			t.Errorf("Platforms = %+v, want %+v", got, wantPlatforms)
		}
		// Genres and companies are {id, name} items: the ids are the database ids
		// the FilterSpec filters by (FLT-002, FLT-003).
		wantGenres := []idName{
			{libLookup(t, s, "genre", "Action"), "Action"},
			{libLookup(t, s, "genre", "rpg"), "rpg"},
			{libLookup(t, s, "genre", "Zelda"), "Zelda"},
		}
		var gotGenres []idName
		for _, g := range c.Genres {
			gotGenres = append(gotGenres, idName{g.ID, g.Name})
		}
		if !slices.Equal(gotGenres, wantGenres) {
			t.Errorf("Genres = %+v, want %+v (NOCASE order)", gotGenres, wantGenres)
		}
		wantCompanies := []idName{
			{libLookup(t, s, "company", "bungie"), "bungie"},
			{libLookup(t, s, "company", "Capcom"), "Capcom"},
			{libLookup(t, s, "company", "team cherry"), "team cherry"},
		}
		var gotCompanies []idName
		for _, co := range c.Companies {
			gotCompanies = append(gotCompanies, idName{co.ID, co.Name})
		}
		if !slices.Equal(gotCompanies, wantCompanies) {
			t.Errorf("Companies = %+v, want %+v (NOCASE order)", gotCompanies, wantCompanies)
		}
		wantModes := []domain.PlayMode{
			domain.PlayModeCoopLocal, domain.PlayModeCoopOnline, domain.PlayModeMultiLocal,
			domain.PlayModeMultiOnline, domain.PlayModeSingle,
		}
		gotModes := slices.Clone(c.PlayModes)
		slices.Sort(gotModes)
		if !slices.Equal(gotModes, wantModes) {
			t.Errorf("PlayModes = %v, want the five play modes %v", c.PlayModes, wantModes)
		}
	})

	t.Run("STO-030/nenhum campo é nulo em banco sem gênero nem empresa", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)

		c, err := s.Catalog(t.Context())
		if err != nil {
			t.Fatalf("Catalog: %v", err)
		}
		if c.Platforms == nil {
			t.Errorf("Platforms is nil, want a non-nil list")
		}
		if c.Genres == nil {
			t.Errorf("Genres is nil, want a non-nil list")
		}
		if c.Companies == nil {
			t.Errorf("Companies is nil, want a non-nil list")
		}
		if c.PlayModes == nil || len(c.PlayModes) != 5 {
			t.Errorf("PlayModes = %v, want the five modes", c.PlayModes)
		}
		if len(c.Genres) != 0 || len(c.Companies) != 0 {
			t.Errorf("Genres %+v and Companies %+v, want both empty", c.Genres, c.Companies)
		}
	})
}

func TestAddPlatform(t *testing.T) {
	t.Parallel()

	t.Run("EDT-008/nome novo cria a plataforma e devolve ela", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)

		p, err := s.AddPlatform(t.Context(), "Mega Drive", domain.CategoryConsole)
		if err != nil {
			t.Fatalf("AddPlatform: %v", err)
		}
		if p.ID == 0 {
			t.Errorf("ID = 0, want the id of the new row")
		}
		if p.Name != "Mega Drive" || p.Category != domain.CategoryConsole {
			t.Errorf("platform = %+v, want Mega Drive / console", p)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM platform WHERE id = ? AND name = 'Mega Drive' AND category = 'console'`, p.ID); n != 1 {
			t.Errorf("platform rows matching the returned platform = %d, want 1", n)
		}
	})

	t.Run("EDT-008/mesmo nome em outra grafia devolve a existente e ignora a categoria enviada", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		first, err := s.AddPlatform(t.Context(), "Switch", domain.CategoryConsole)
		if err != nil {
			t.Fatalf("AddPlatform(Switch): %v", err)
		}

		again, err := s.AddPlatform(t.Context(), "SWITCH", domain.CategoryPC)
		if err != nil {
			t.Fatalf("AddPlatform(SWITCH): %v", err)
		}
		if again.ID != first.ID {
			t.Errorf("ID = %d, want the existing %d", again.ID, first.ID)
		}
		if again.Name != "Switch" {
			t.Errorf("Name = %q, want the existing spelling Switch", again.Name)
		}
		if again.Category != domain.CategoryConsole {
			t.Errorf("Category = %q, want the existing console (the sent pc is ignored)", again.Category)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM platform WHERE name = 'switch' COLLATE NOCASE`); n != 1 {
			t.Errorf("platforms named switch (no case) = %d, want 1", n)
		}
	})
}

func TestPlatforms(t *testing.T) {
	t.Parallel()

	t.Run("IPC-048/Platforms em ordem de nome sem diferenciar maiúsculas; lista vazia não é nil", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)

		empty, err := s.Platforms(t.Context())
		if err != nil {
			t.Fatalf("Platforms on an empty database: %v", err)
		}
		if empty == nil || len(empty) != 0 {
			t.Errorf("Platforms on an empty database = %#v, want an empty non-nil list", empty)
		}

		// Binary order would put Amiga and PC before atari and switch.
		for _, p := range []struct {
			name string
			cat  domain.PlatformCategory
		}{
			{"switch", domain.CategoryConsole},
			{"PC", domain.CategoryPC},
			{"atari", domain.CategoryArcade},
			{"Amiga", domain.CategoryArcade},
		} {
			if _, err := s.AddPlatform(t.Context(), p.name, p.cat); err != nil {
				t.Fatalf("AddPlatform(%q): %v", p.name, err)
			}
		}

		ps, err := s.Platforms(t.Context())
		if err != nil {
			t.Fatalf("Platforms: %v", err)
		}
		var names []string
		for _, p := range ps {
			names = append(names, p.Name)
		}
		if want := []string{"Amiga", "atari", "PC", "switch"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v (NOCASE order)", names, want)
		}
	})
}
