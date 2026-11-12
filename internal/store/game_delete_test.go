package store_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Tests of Store.DeleteGame (STO-029, DEL-001, DEL-004, DEL-006, FTS-004). The
// serialization with the tracker and the agent's error mapping are tested in the
// tracker and agent packages, not here.

// delOwnedQueries count the rows that belong to one game. Each takes the game id.
// Pauses are counted through their session, which is the only way to reach them.
var delOwnedQueries = []struct{ label, query string }{
	{"game", `SELECT COUNT(*) FROM game WHERE id = ?`},
	{"play_session", `SELECT COUNT(*) FROM play_session WHERE game_id = ?`},
	{"pause_interval", `SELECT COUNT(*) FROM pause_interval p JOIN play_session s ON s.id = p.session_id WHERE s.game_id = ?`},
	{"game_genre", `SELECT COUNT(*) FROM game_genre WHERE game_id = ?`},
	{"game_developer", `SELECT COUNT(*) FROM game_developer WHERE game_id = ?`},
	{"game_publisher", `SELECT COUNT(*) FROM game_publisher WHERE game_id = ?`},
	{"game_play_mode", `SELECT COUNT(*) FROM game_play_mode WHERE game_id = ?`},
	{"additional_app", `SELECT COUNT(*) FROM additional_app WHERE game_id = ?`},
	{"tracking_rule", `SELECT COUNT(*) FROM tracking_rule WHERE game_id = ?`},
	{"media_asset", `SELECT COUNT(*) FROM media_asset WHERE game_id = ?`},
	{"game_fts", `SELECT COUNT(*) FROM game_fts WHERE rowid = ?`},
}

// delCounts returns how many rows of each kind still belong to gameID.
func delCounts(t *testing.T, s *store.Store, gameID int64) map[string]int64 {
	t.Helper()
	out := make(map[string]int64, len(delOwnedQueries))
	for _, q := range delOwnedQueries {
		out[q.label] = countRows(t, s.DB(), q.query, gameID)
	}
	return out
}

func TestDeleteGame(t *testing.T) {
	t.Parallel()

	t.Run("DEL-001/remove o jogo, as sessões, as pausas, os vínculos, additional_app, tracking_rule e media_asset", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		id := seedLibrary(t, s, libGame{Title: "Lanterna Sumida", Genres: []string{"RPG"}, Modes: []string{"single"},
			Developers: []string{"Estudio Um"}, Publishers: []string{"Editora Um"}})["Lanterna Sumida"]
		sid := libInsertSession(t, db, libSessionRow{GameID: id, Started: libAt(10, 10), Ended: libAt(10, 11), Active: 3600, State: "ended"})
		libInsertPause(t, db, sid, libAt(10, 10)+1_200_000, libAt(10, 10)+1_800_000)
		libExec(t, db, `INSERT INTO additional_app (game_id, name, target) VALUES (?, 'Mapa', 'C:\Games\mapa.exe')`, id)
		libExec(t, db, `INSERT INTO tracking_rule (game_id, kind, value) VALUES (?, 'process', 'lanterna.exe')`, id)
		libExec(t, db, `INSERT INTO media_asset (game_id, kind, path) VALUES (?, 'cover', 'C:\img\capa.png')`, id)

		before := delCounts(t, s, id)
		for label, n := range before {
			if n != 1 {
				t.Fatalf("precondition: %s rows of the game = %d, want 1", label, n)
			}
		}

		if err := s.DeleteGame(t.Context(), id); err != nil {
			t.Fatalf("DeleteGame(%d): %v", id, err)
		}
		for label, n := range delCounts(t, s, id) {
			if n != 0 {
				t.Errorf("%s rows of the removed game = %d, want 0", label, n)
			}
		}
	})

	t.Run("DEL-001/outros jogos, a plataforma, gêneros, empresas e modos continuam como estavam", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		ids := seedLibrary(t, s,
			libGame{Title: "Lanterna Sumida", Platform: "Switch", Genres: []string{"RPG"}, Modes: []string{"single"},
				Developers: []string{"Estudio Um"}, Publishers: []string{"Editora Um"}},
			libGame{Title: "Hades", Platform: "Switch", Genres: []string{"RPG"}, Modes: []string{"coop_local"},
				Developers: []string{"Estudio Um"}, Publishers: []string{"Editora Dois"}})
		keep, gone := ids["Hades"], ids["Lanterna Sumida"]
		libInsertSession(t, db, libSessionRow{GameID: keep, Started: libAt(10, 10), Ended: libAt(10, 11), Active: 3600, State: "ended"})

		beforeKeep := readEditRow(t, db, keep)
		beforeLinks := linkDump(t, db, keep)
		beforeSessions := editChildRows(t, db, keep)
		beforeIndex := indexRowDump(t, db, keep)
		beforeCatalog := catalogDump(t, db)
		beforePlatforms := dumpRows(t, db, `SELECT id, name, category FROM platform ORDER BY id`)

		if err := s.DeleteGame(t.Context(), gone); err != nil {
			t.Fatalf("DeleteGame(%d): %v", gone, err)
		}

		if after := readEditRow(t, db, keep); after != beforeKeep {
			t.Errorf("kept game row = %+v, want %+v", after, beforeKeep)
		}
		if got := linkDump(t, db, keep); !slices.Equal(got, beforeLinks) {
			t.Errorf("kept game links = %v, want %v", got, beforeLinks)
		}
		if got := editChildRows(t, db, keep); !slices.Equal(got, beforeSessions) {
			t.Errorf("kept game sessions and pauses = %v, want %v", got, beforeSessions)
		}
		if got := indexRowDump(t, db, keep); !slices.Equal(got, beforeIndex) {
			t.Errorf("kept game_fts row = %v, want %v", got, beforeIndex)
		}
		if got := catalogDump(t, db); !slices.Equal(got, beforeCatalog) {
			t.Errorf("catalog = %v, want %v", got, beforeCatalog)
		}
		if got := dumpRows(t, db, `SELECT id, name, category FROM platform ORDER BY id`); !slices.Equal(got, beforePlatforms) {
			t.Errorf("platforms = %v, want %v", got, beforePlatforms)
		}
	})

	t.Run("STO-029/jogo com sessão aberta devolve ErrGameInUse e nada muda", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		id := seedLibrary(t, s, libGame{Title: "Lanterna Sumida", Description: "Mapa secreto"})["Lanterna Sumida"]
		sid := libInsertSession(t, db, libSessionRow{GameID: id, Started: libAt(10, 10), Active: 60, State: "running"})
		libInsertPause(t, db, sid, libAt(10, 10)+1_200_000, libAt(10, 10)+1_800_000)

		before := readEditRow(t, db, id)
		beforeSessions := editChildRows(t, db, id)
		beforeIndex := indexRowDump(t, db, id)

		err := s.DeleteGame(t.Context(), id)
		if !errors.Is(err, store.ErrGameInUse) {
			t.Fatalf("DeleteGame with an open session err = %v, want ErrGameInUse", err)
		}
		if after := readEditRow(t, db, id); after != before {
			t.Errorf("game row = %+v after refused delete, want %+v", after, before)
		}
		if got := editChildRows(t, db, id); !slices.Equal(got, beforeSessions) {
			t.Errorf("sessions and pauses = %v after refused delete, want %v", got, beforeSessions)
		}
		if got := indexRowDump(t, db, id); !slices.Equal(got, beforeIndex) {
			t.Errorf("game_fts row = %v after refused delete, want %v", got, beforeIndex)
		}
	})

	t.Run("STO-029/id inexistente devolve ErrGameNotFound", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)

		if err := s.DeleteGame(t.Context(), 999); !errors.Is(err, store.ErrGameNotFound) {
			t.Fatalf("DeleteGame(999) err = %v, want ErrGameNotFound", err)
		}
	})

	t.Run("DEL-004/gameId inexistente devolve ErrGameNotFound e não altera outros jogos", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		id := seedLibrary(t, s, libGame{Title: "Lanterna Sumida", Description: "Mapa secreto"})["Lanterna Sumida"]
		before := readEditRow(t, db, id)
		beforeIndex := indexRowDump(t, db, id)

		if err := s.DeleteGame(t.Context(), id+1000); !errors.Is(err, store.ErrGameNotFound) {
			t.Fatalf("DeleteGame(missing id) err = %v, want ErrGameNotFound", err)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM game`); n != 1 {
			t.Errorf("games after refused delete = %d, want 1", n)
		}
		if after := readEditRow(t, db, id); after != before {
			t.Errorf("other game row = %+v, want %+v", after, before)
		}
		if got := indexRowDump(t, db, id); !slices.Equal(got, beforeIndex) {
			t.Errorf("other game_fts row = %v, want %v", got, beforeIndex)
		}
	})
}

func TestDeleteGameAtomic(t *testing.T) {
	t.Parallel()

	t.Run("DEL-006/trigger BEFORE DELETE ON game que aborta mantém jogo, sessões, pausas e linha de game_fts", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		id := seedLibrary(t, s, libGame{Title: "Lanterna Sumida", Description: "Mapa secreto", Genres: []string{"RPG"}})["Lanterna Sumida"]
		sid := libInsertSession(t, db, libSessionRow{GameID: id, Started: libAt(10, 10), Ended: libAt(10, 11), Active: 3600, State: "ended"})
		libInsertPause(t, db, sid, libAt(10, 10)+1_200_000, libAt(10, 10)+1_800_000)
		libExec(t, db, `CREATE TRIGGER game_block_delete BEFORE DELETE ON game
			BEGIN SELECT RAISE(ABORT, 'game delete blocked'); END`)

		before := readEditRow(t, db, id)
		beforeLinks := linkDump(t, db, id)
		beforeSessions := editChildRows(t, db, id)
		beforeIndex := indexRowDump(t, db, id)
		if len(beforeIndex) == 0 {
			t.Fatalf("precondition: game_fts has no row for the game")
		}

		if err := s.DeleteGame(t.Context(), id); err == nil {
			t.Fatalf("DeleteGame with an aborting trigger err = nil, want an error")
		}
		if after := readEditRow(t, db, id); after != before {
			t.Errorf("game row = %+v after aborted delete, want %+v", after, before)
		}
		if got := linkDump(t, db, id); !slices.Equal(got, beforeLinks) {
			t.Errorf("links = %v after aborted delete, want %v", got, beforeLinks)
		}
		if got := editChildRows(t, db, id); !slices.Equal(got, beforeSessions) {
			t.Errorf("sessions and pauses = %v after aborted delete, want %v", got, beforeSessions)
		}
		if got := indexRowDump(t, db, id); !slices.Equal(got, beforeIndex) {
			t.Errorf("game_fts row = %v after aborted delete, want %v", got, beforeIndex)
		}
	})

	t.Run("FTS-004/após remover, o título do jogo não aparece no índice", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		id := seedLibrary(t, s, libGame{Title: "Lanterna Sumida"})["Lanterna Sumida"]
		if got := matchRowids(t, db, "Sumida"); !slices.Equal(got, []int64{id}) {
			t.Fatalf("precondition: MATCH Sumida = %v, want [%d]", got, id)
		}

		if err := s.DeleteGame(t.Context(), id); err != nil {
			t.Fatalf("DeleteGame(%d): %v", id, err)
		}
		if got := matchRowids(t, db, "Sumida"); len(got) != 0 {
			t.Errorf("MATCH Sumida after removal = %v, want none", got)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM game_fts WHERE rowid = ?`, id); n != 0 {
			t.Errorf("game_fts rows with the removed id = %d, want 0", n)
		}
	})

	t.Run("FTS-004/um jogo novo com o mesmo id reaproveitado não aparece pelo texto do removido", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		id := seedLibrary(t, s, libGame{Title: "Lanterna Sumida", Description: "Mapa secreto"})["Lanterna Sumida"]
		if err := s.DeleteGame(t.Context(), id); err != nil {
			t.Fatalf("DeleteGame(%d): %v", id, err)
		}

		// The new game is inserted without touching game_fts, so a stale index row
		// of the removed game would still match its text. SQLite reuses the rowid
		// of the empty table, which is the removed id.
		libExec(t, db, `INSERT INTO game (platform_id, title, launch_type, created_at, updated_at)
			VALUES (?, 'Relogio Novo', 'exe', 1700000000000, 1700000000000)`, libPlatform(t, db, "PC"))
		var newID int64
		if err := db.QueryRowContext(t.Context(), `SELECT id FROM game WHERE title = 'Relogio Novo'`).Scan(&newID); err != nil {
			t.Fatalf("id of the new game: %v", err)
		}
		if newID != id {
			t.Fatalf("new game id = %d, removed id = %d: the rowid was not reused, the case cannot show the leak", newID, id)
		}

		for _, text := range []string{"Sumida", "secreto"} {
			if got := matchRowids(t, db, text); len(got) != 0 {
				t.Errorf("MATCH %q finds %v after the removed game's id was reused, want none", text, got)
			}
		}
	})
}
