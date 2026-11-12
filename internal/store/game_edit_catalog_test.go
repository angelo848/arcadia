package store_test

import (
	"database/sql"
	"fmt"
	"slices"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Tests of Store.UpdateGame for the links and the search index (EDT-004, EDT-007,
// FTS-003, STO-028). The links are developers, publishers, genres and play modes;
// catalog names are created or reused in the same transaction as the game.

// linkDump renders the link rows of one game as raw rows, so that two snapshots
// can be compared for equality (EDT-007, STO-028).
func linkDump(t *testing.T, db *sql.DB, gameID int64) []string {
	t.Helper()
	var out []string
	for _, q := range []string{
		`SELECT 'developer', c.id, c.name FROM game_developer gd JOIN company c ON c.id = gd.company_id WHERE gd.game_id = %[1]d`,
		`SELECT 'publisher', c.id, c.name FROM game_publisher gp JOIN company c ON c.id = gp.company_id WHERE gp.game_id = %[1]d`,
		`SELECT 'genre', g.id, g.name FROM game_genre gg JOIN genre g ON g.id = gg.genre_id WHERE gg.game_id = %[1]d`,
		`SELECT 'play_mode', p.id, p.name FROM game_play_mode gm JOIN play_mode p ON p.id = gm.play_mode_id WHERE gm.game_id = %[1]d`,
	} {
		out = append(out, dumpRows(t, db, fmt.Sprintf(q, gameID))...)
	}
	return out
}

// catalogDump renders the catalog tables (companies, genres, play modes) as raw
// rows, so that a rolled-back edit can be shown to create nothing (EDT-007).
func catalogDump(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var out []string
	for _, q := range []string{
		`SELECT 'company', id, name FROM company ORDER BY id`,
		`SELECT 'genre', id, name FROM genre ORDER BY id`,
		`SELECT 'play_mode', id, name FROM play_mode ORDER BY id`,
	} {
		out = append(out, dumpRows(t, db, q)...)
	}
	return out
}

// indexRowDump renders the game_fts row of id as raw rows (FTS-003, FTS-004).
func indexRowDump(t *testing.T, db *sql.DB, id int64) []string {
	t.Helper()
	return dumpRows(t, db, fmt.Sprintf(
		`SELECT rowid, title, description, developer, publisher FROM game_fts WHERE rowid = %d`, id))
}

func TestUpdateGameLinks(t *testing.T) {
	t.Parallel()

	t.Run("EDT-004/developers [Team Cherry, team cherry] com Team Cherry existente gera uma só empresa com a grafia já gravada", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ids := seedLibrary(t, s,
			libGame{Title: "Hollow Knight", Developers: []string{"Team Cherry"}},
			libGame{Title: "Celeste"})
		cherryID := libLookup(t, s, "company", "Team Cherry")

		d := mustUpdateGame(t, s, ids["Celeste"], store.GamePatch{
			Developers: editPtr([]string{"Team Cherry", "team cherry"}),
		})

		if !slices.Equal(d.Developers, []string{"Team Cherry"}) {
			t.Errorf("Developers = %v, want [Team Cherry]", d.Developers)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM company WHERE name = 'team cherry' COLLATE NOCASE`); n != 1 {
			t.Errorf("companies named team cherry (no case) = %d, want 1", n)
		}
		if got := libLookup(t, s, "company", "Team Cherry"); got != cherryID {
			t.Errorf("company id of Team Cherry = %d, want the existing %d", got, cherryID)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game_developer WHERE game_id = ?`, ids["Celeste"]); n != 1 {
			t.Errorf("developer links of Celeste = %d, want 1", n)
		}
	})

	t.Run("EDT-004/genres e playModes substituem o conjunto; array vazio limpa", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades", Genres: []string{"RPG", "Action"}, Modes: []string{"single"}})["Hades"]

		d := mustUpdateGame(t, s, id, store.GamePatch{
			Genres:    editPtr([]string{"Platformer"}),
			PlayModes: editPtr([]domain.PlayMode{domain.PlayModeCoopOnline, domain.PlayModeMultiLocal}),
		})
		if !slices.Equal(d.Genres, []string{"Platformer"}) {
			t.Errorf("Genres = %v, want [Platformer]", d.Genres)
		}
		if !slices.Equal(d.PlayModes, []string{"coop_online", "multi_local"}) {
			t.Errorf("PlayModes = %v, want [coop_online multi_local]", d.PlayModes)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game_genre WHERE game_id = ?`, id); n != 1 {
			t.Errorf("genre links = %d after replacing, want 1", n)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game_play_mode WHERE game_id = ?`, id); n != 2 {
			t.Errorf("play mode links = %d after replacing, want 2", n)
		}

		d = mustUpdateGame(t, s, id, store.GamePatch{
			Genres:    editPtr([]string{}),
			PlayModes: editPtr([]domain.PlayMode{}),
		})
		if d.Genres == nil || len(d.Genres) != 0 {
			t.Errorf("Genres after empty array = %#v, want []", d.Genres)
		}
		if d.PlayModes == nil || len(d.PlayModes) != 0 {
			t.Errorf("PlayModes after empty array = %#v, want []", d.PlayModes)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game_genre WHERE game_id = ?`, id); n != 0 {
			t.Errorf("genre links = %d after empty array, want 0", n)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game_play_mode WHERE game_id = ?`, id); n != 0 {
			t.Errorf("play mode links = %d after empty array, want 0", n)
		}
	})

	t.Run("EDT-004/nome novo cria a linha de catálogo", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Celeste"})["Celeste"]

		d := mustUpdateGame(t, s, id, store.GamePatch{
			Developers: editPtr([]string{"Nova Empresa"}),
			Publishers: editPtr([]string{"Editora Nova"}),
			Genres:     editPtr([]string{"Novo Genero"}),
		})
		if !slices.Equal(d.Developers, []string{"Nova Empresa"}) {
			t.Errorf("Developers = %v, want [Nova Empresa]", d.Developers)
		}
		if !slices.Equal(d.Publishers, []string{"Editora Nova"}) {
			t.Errorf("Publishers = %v, want [Editora Nova]", d.Publishers)
		}
		if !slices.Equal(d.Genres, []string{"Novo Genero"}) {
			t.Errorf("Genres = %v, want [Novo Genero]", d.Genres)
		}
		// libLookup fails the test when the catalog row does not exist.
		libLookup(t, s, "company", "Nova Empresa")
		libLookup(t, s, "company", "Editora Nova")
		libLookup(t, s, "genre", "Novo Genero")
	})

	t.Run("STO-028/os vínculos são substituídos numa transação", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades", Developers: []string{"Dev Antigo"},
			Publishers: []string{"Editora Antiga"}, Genres: []string{"RPG"}, Modes: []string{"single"}})["Hades"]

		// Publishers and Modes are absent from the patch: they must stay as they are.
		d := mustUpdateGame(t, s, id, store.GamePatch{
			Developers: editPtr([]string{"Dev Novo"}),
			Genres:     editPtr([]string{"Action"}),
		})
		if !slices.Equal(d.Developers, []string{"Dev Novo"}) {
			t.Errorf("Developers = %v, want [Dev Novo]", d.Developers)
		}
		if !slices.Equal(d.Genres, []string{"Action"}) {
			t.Errorf("Genres = %v, want [Action]", d.Genres)
		}
		if !slices.Equal(d.Publishers, []string{"Editora Antiga"}) {
			t.Errorf("Publishers = %v, want the untouched [Editora Antiga]", d.Publishers)
		}
		if !slices.Equal(d.PlayModes, []string{"single"}) {
			t.Errorf("PlayModes = %v, want the untouched [single]", d.PlayModes)
		}
		var devs int64
		if err := s.DB().QueryRowContext(t.Context(),
			`SELECT COUNT(*) FROM game_developer gd JOIN company c ON c.id = gd.company_id
			WHERE gd.game_id = ? AND c.name = 'Dev Antigo'`, id).Scan(&devs); err != nil {
			t.Fatalf("count old developer links: %v", err)
		}
		if devs != 0 {
			t.Errorf("old developer link still present after replacement")
		}
	})
}

func TestUpdateGameIndex(t *testing.T) {
	t.Parallel()

	t.Run("FTS-003/editar descrição e trocar desenvolvedora: o texto novo encontra o jogo, o antigo não, e a desenvolvedora nova encontra", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		id := seedLibrary(t, s, libGame{Title: "Hades", Description: "Sequencia antiga",
			Developers: []string{"Estudio Velho"}})["Hades"]
		if got := matchRowids(t, db, "antiga"); !slices.Equal(got, []int64{id}) {
			t.Fatalf("precondition: MATCH antiga = %v, want [%d]", got, id)
		}

		mustUpdateGame(t, s, id, store.GamePatch{
			Description: editPtr("Metroidvania"),
			Developers:  editPtr([]string{"Estudio Novo"}),
		})

		if got := matchRowids(t, db, "metroidvania"); !slices.Equal(got, []int64{id}) {
			t.Errorf("MATCH metroidvania = %v, want [%d]", got, id)
		}
		if got := matchRowids(t, db, "antiga"); len(got) != 0 {
			t.Errorf("MATCH antiga (old description) = %v, want none", got)
		}
		if got := matchRowids(t, db, "novo"); !slices.Equal(got, []int64{id}) {
			t.Errorf("MATCH novo (new developer) = %v, want [%d]", got, id)
		}
		if got := matchRowids(t, db, "velho"); len(got) != 0 {
			t.Errorf("MATCH velho (old developer) = %v, want none", got)
		}
	})

	t.Run("FTS-003/falha na reindexação (game_fts removida no teste com DROP TABLE) desfaz a edição inteira", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		id := seedLibrary(t, s, libGame{Title: "Hades", Description: "Sequencia antiga",
			Developers: []string{"Estudio Velho"}, Genres: []string{"RPG"}})["Hades"]
		libExec(t, db, `DROP TABLE game_fts`)
		before := readEditRow(t, db, id)
		beforeLinks := linkDump(t, db, id)
		beforeCatalog := catalogDump(t, db)

		_, err := s.UpdateGame(t.Context(), id, store.GamePatch{
			Title:       editPtr("Hades II"),
			Description: editPtr("Metroidvania"),
			Developers:  editPtr([]string{"Estudio Novo"}),
			Genres:      editPtr([]string{"Acao"}),
		}, editAt)
		if err == nil {
			t.Fatalf("UpdateGame without game_fts err = nil, want an error")
		}
		if after := readEditRow(t, db, id); after != before {
			t.Errorf("game row after failed reindex = %+v, want %+v", after, before)
		}
		if got := linkDump(t, db, id); !slices.Equal(got, beforeLinks) {
			t.Errorf("links after failed reindex = %v, want %v", got, beforeLinks)
		}
		if got := catalogDump(t, db); !slices.Equal(got, beforeCatalog) {
			t.Errorf("catalog after failed reindex = %v, want %v", got, beforeCatalog)
		}
	})
}

func TestUpdateGameAtomic(t *testing.T) {
	t.Parallel()

	t.Run("EDT-007/patch com developers novos e uma falha depois não cria empresa nova e não muda linhas", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		id := seedLibrary(t, s, libGame{Title: "Hades", Description: "Original", Developers: []string{"Estudio Velho"}})["Hades"]
		// The company of the patch is created first, then the link insert aborts (a failure after the catalog write).
		libExec(t, db, `CREATE TRIGGER developer_block_insert BEFORE INSERT ON game_developer
			BEGIN SELECT RAISE(ABORT, 'developer insert blocked'); END`)
		before := readEditRow(t, db, id)
		beforeLinks := linkDump(t, db, id)
		beforeCatalog := catalogDump(t, db)
		beforeIndex := indexRowDump(t, db, id)

		_, err := s.UpdateGame(t.Context(), id, store.GamePatch{
			Title:      editPtr("Hades II"),
			Developers: editPtr([]string{"Empresa Nova"}),
		}, editAt)
		if err == nil {
			t.Fatalf("UpdateGame with an aborting link insert err = nil, want an error")
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM company WHERE name = 'Empresa Nova'`); n != 0 {
			t.Errorf("company Empresa Nova created by the failed edit (%d rows), want 0", n)
		}
		if after := readEditRow(t, db, id); after != before {
			t.Errorf("game row after failed edit = %+v, want %+v", after, before)
		}
		if got := linkDump(t, db, id); !slices.Equal(got, beforeLinks) {
			t.Errorf("links after failed edit = %v, want %v", got, beforeLinks)
		}
		if got := catalogDump(t, db); !slices.Equal(got, beforeCatalog) {
			t.Errorf("catalog after failed edit = %v, want %v", got, beforeCatalog)
		}
		if got := indexRowDump(t, db, id); !slices.Equal(got, beforeIndex) {
			t.Errorf("game_fts row after failed edit = %v, want %v", got, beforeIndex)
		}
	})

	t.Run("STO-028/patch sem mudança não reindexa e não grava updated_at", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		id := seedLibrary(t, s, libGame{Title: "Hades", Description: "Roguelike", Developers: []string{"Supergiant"}})["Hades"]
		before := readEditRow(t, db, id)
		// Marks the index row: a real reindex would replace this marker with the title.
		libExec(t, db, `UPDATE game_fts SET title = 'marcador' WHERE rowid = ?`, id)

		mustUpdateGame(t, s, id, store.GamePatch{
			Title:       editPtr("Hades"),
			Description: editPtr("Roguelike"),
			Developers:  editPtr([]string{"Supergiant"}),
		})

		if after := readEditRow(t, db, id); after.UpdatedAt != before.UpdatedAt {
			t.Errorf("updated_at = %d after equal patch, want unchanged %d", after.UpdatedAt, before.UpdatedAt)
		}
		if got := matchRowids(t, db, "marcador"); !slices.Equal(got, []int64{id}) {
			t.Errorf("MATCH marcador = %v, want [%d]: the index was rewritten by an equal patch", got, id)
		}
	})
}
