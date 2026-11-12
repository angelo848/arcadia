package store_test

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Tests of Store.UpdateGame for the simple fields (STO-028 partial, EDT-001,
// EDT-002, EDT-003, EDT-005, EDT-006, EDT-007, EDT-009). Links (developers,
// publishers, genres, play modes) are accepted by the patch but are written
// by a later task, so these tests leave them out.

// editAt is the instant W given to UpdateGame. It is fixed (no clock) and not
// UTC, so the stored value must be UnixMilli of the instant itself (ADR 0009).
var editAt = time.Date(2026, time.November, 12, 9, 30, 0, 0, time.FixedZone("BRT", -3*60*60))

// editDone is the UTC midnight of a day, what EDT-001 stores for completedAt.
var editDone = time.Date(2026, time.November, 12, 0, 0, 0, 0, time.UTC)

// editPtr returns a pointer to a copy of v. Nested calls build the double
// pointers of GamePatch (editPtr(editPtr(1998)) is a **int).
func editPtr[T any](v T) *T { return &v }

// editRow is the raw game row, read bypassing the store API.
type editRow struct {
	PlatformID                 int64
	Title, Status, Description string
	Favorite                   int64
	Release                    sql.NullString
	Completed                  sql.NullInt64
	Total, Plays               int64
	First, Last                sql.NullInt64
	UpdatedAt                  int64
}

func readEditRow(t *testing.T, db *sql.DB, id int64) editRow {
	t.Helper()
	var r editRow
	err := db.QueryRowContext(t.Context(), `SELECT platform_id, title, status, description, favorite, release_date,
		completed_at, total_play_seconds, play_count, first_played_at, last_played_at, updated_at
		FROM game WHERE id = ?`, id).Scan(&r.PlatformID, &r.Title, &r.Status, &r.Description, &r.Favorite,
		&r.Release, &r.Completed, &r.Total, &r.Plays, &r.First, &r.Last, &r.UpdatedAt)
	if err != nil {
		t.Fatalf("read game %d: %v", id, err)
	}
	return r
}

// editChildRows renders the sessions and the pauses of the game as raw rows, so
// that two snapshots can be compared (EDT-005, EDT-006).
func editChildRows(t *testing.T, db *sql.DB, gameID int64) []string {
	t.Helper()
	out := dumpRows(t, db, fmt.Sprintf(`SELECT id, game_id, started_at, ended_at, last_seen_at, active_seconds,
		state, source, end_reason FROM play_session WHERE game_id = %d ORDER BY id`, gameID))
	return append(out, dumpRows(t, db, fmt.Sprintf(`SELECT p.id, p.session_id, p.paused_at, p.resumed_at, p.reason
		FROM pause_interval p JOIN play_session s ON s.id = p.session_id WHERE s.game_id = %d ORDER BY p.id`, gameID))...)
}

// mustUpdateGame calls UpdateGame with editAt and fails the test on error.
func mustUpdateGame(t *testing.T, s *store.Store, id int64, p store.GamePatch) store.GameDetail {
	t.Helper()
	d, err := s.UpdateGame(t.Context(), id, p, editAt)
	if err != nil {
		t.Fatalf("UpdateGame(%d, %+v): %v", id, p, err)
	}
	return d
}

func TestUpdateGameFields(t *testing.T) {
	t.Parallel()

	t.Run("EDT-001/title, status, favorite, description e completedAt são gravados", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades", Status: "playing"})["Hades"]

		d := mustUpdateGame(t, s, id, store.GamePatch{
			Title: editPtr("Hades II"), Status: editPtr(domain.StatusCompleted), Favorite: editPtr(true),
			Description: editPtr("Roguelite"), CompletedAt: editPtr(editPtr(editDone)),
		})

		if d.Title != "Hades II" || d.Status != domain.StatusCompleted || !d.Favorite ||
			d.Description != "Roguelite" || !d.CompletedAt.Equal(editDone) {
			t.Errorf("returned detail = title %q status %q favorite %v description %q completed %v",
				d.Title, d.Status, d.Favorite, d.Description, d.CompletedAt)
		}
		r := readEditRow(t, s.DB(), id)
		if r.Title != "Hades II" || r.Status != string(domain.StatusCompleted) || r.Favorite != 1 ||
			r.Description != "Roguelite" || !r.Completed.Valid || r.Completed.Int64 != editDone.UnixMilli() {
			t.Errorf("row = %+v, want the patched values", r)
		}
	})

	t.Run("EDT-001/completedAt grava meia-noite UTC do dia; CompletedAt nulo apaga", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Celeste"})["Celeste"]

		mustUpdateGame(t, s, id, store.GamePatch{CompletedAt: editPtr(editPtr(editDone))})
		r := readEditRow(t, s.DB(), id)
		if !r.Completed.Valid || r.Completed.Int64 != time.Date(2026, time.November, 12, 0, 0, 0, 0, time.UTC).UnixMilli() {
			t.Fatalf("completed_at = %+v, want midnight UTC of 2026-11-12", r.Completed)
		}

		d := mustUpdateGame(t, s, id, store.GamePatch{CompletedAt: editPtr((*time.Time)(nil))})
		if r := readEditRow(t, s.DB(), id); r.Completed.Valid {
			t.Errorf("completed_at = %d after null, want NULL", r.Completed.Int64)
		}
		if !d.CompletedAt.IsZero() {
			t.Errorf("returned CompletedAt = %v after null, want zero", d.CompletedAt)
		}
	})

	t.Run("EDT-002/platformId existente troca a plataforma e mantém sessões, total e contagem", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades", Seconds: 3600, Plays: 2,
			FirstPlayed: libAt(1, 20), LastPlayed: libAt(9, 21)})["Hades"]
		libInsertSession(t, s.DB(), libSessionRow{GameID: id, Started: libAt(9, 20), Ended: libAt(9, 21),
			Active: 3600, State: "ended", EndReason: "exited"})
		switchID := libPlatform(t, s.DB(), "Switch")
		before := readEditRow(t, s.DB(), id)
		childBefore := editChildRows(t, s.DB(), id)

		d := mustUpdateGame(t, s, id, store.GamePatch{PlatformID: editPtr(switchID)})

		r := readEditRow(t, s.DB(), id)
		if r.PlatformID != switchID {
			t.Errorf("platform_id = %d, want %d (Switch)", r.PlatformID, switchID)
		}
		if d.PlatformID != switchID || d.Platform != "Switch" {
			t.Errorf("returned PlatformID/Platform = %d/%q, want %d/Switch", d.PlatformID, d.Platform, switchID)
		}
		if r.Total != before.Total || r.Plays != before.Plays || r.First != before.First || r.Last != before.Last {
			t.Errorf("total/plays/first/last = %d/%d/%+v/%+v, want %d/%d/%+v/%+v",
				r.Total, r.Plays, r.First, r.Last, before.Total, before.Plays, before.First, before.Last)
		}
		if got := editChildRows(t, s.DB(), id); !reflect.DeepEqual(got, childBefore) {
			t.Errorf("sessions and pauses = %v, want %v", got, childBefore)
		}
	})

	t.Run("EDT-002/platformId inexistente devolve ErrPlatformNotFound e nada muda", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades", Seconds: 60, Plays: 1})["Hades"]
		before := readEditRow(t, s.DB(), id)

		_, err := s.UpdateGame(t.Context(), id, store.GamePatch{
			PlatformID: editPtr(int64(999999)), Title: editPtr("Outro"),
		}, editAt)
		if !errors.Is(err, store.ErrPlatformNotFound) {
			t.Fatalf("UpdateGame err = %v, want errors.Is(err, store.ErrPlatformNotFound)", err)
		}
		if after := readEditRow(t, s.DB(), id); after != before {
			t.Errorf("row after failed edit = %+v, want %+v", after, before)
		}
	})

	t.Run("EDT-003/year 1998 com release_date 1998-06-12 preserva a data; year nulo zera release_date", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Crash", Release: "1998-06-12"})["Crash"]

		d := mustUpdateGame(t, s, id, store.GamePatch{Year: editPtr(editPtr(1998))})
		if r := readEditRow(t, s.DB(), id); r.Release.String != "1998-06-12" || !r.Release.Valid {
			t.Errorf("release_date = %+v, want 1998-06-12 kept", r.Release)
		}
		if d.ReleaseDate != "1998-06-12" {
			t.Errorf("returned ReleaseDate = %q, want 1998-06-12", d.ReleaseDate)
		}

		d = mustUpdateGame(t, s, id, store.GamePatch{Year: editPtr((*int)(nil))})
		if r := readEditRow(t, s.DB(), id); r.Release.Valid {
			t.Errorf("release_date = %q after year null, want NULL", r.Release.String)
		}
		if d.ReleaseDate != "" {
			t.Errorf("returned ReleaseDate = %q after year null, want empty", d.ReleaseDate)
		}
	})

	t.Run("EDT-005/total, contagem, primeira e última vez, sessões e pausas iguais antes e depois", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades", Seconds: 5400, Plays: 3,
			FirstPlayed: libAt(1, 20), LastPlayed: libAt(9, 21)})["Hades"]
		sess := libInsertSession(t, s.DB(), libSessionRow{GameID: id, Started: libAt(9, 20), Ended: libAt(9, 21),
			Active: 5400, State: "ended", EndReason: "exited"})
		libInsertPause(t, s.DB(), sess, libAt(9, 20)+1000, libAt(9, 20)+2000)
		before := readEditRow(t, s.DB(), id)
		childBefore := editChildRows(t, s.DB(), id)

		mustUpdateGame(t, s, id, store.GamePatch{
			Title: editPtr("Hades II"), Status: editPtr(domain.StatusAbandoned), Favorite: editPtr(true),
			Description: editPtr("texto"), CompletedAt: editPtr(editPtr(editDone)), Year: editPtr(editPtr(2001)),
		})

		after := readEditRow(t, s.DB(), id)
		if after.Total != before.Total || after.Plays != before.Plays ||
			after.First != before.First || after.Last != before.Last {
			t.Errorf("total/plays/first/last changed: before %d/%d/%+v/%+v, after %d/%d/%+v/%+v",
				before.Total, before.Plays, before.First, before.Last, after.Total, after.Plays, after.First, after.Last)
		}
		if got := editChildRows(t, s.DB(), id); !reflect.DeepEqual(got, childBefore) {
			t.Errorf("sessions and pauses = %v, want %v", got, childBefore)
		}
	})

	t.Run("EDT-005/patch igual ao atual não muda updated_at; patch com mudança muda", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Igual"})["Igual"]
		fixtureUpdated := readEditRow(t, s.DB(), id).UpdatedAt

		mustUpdateGame(t, s, id, store.GamePatch{
			Title: editPtr("Igual"), Status: editPtr(domain.StatusNotPlayed),
			Favorite: editPtr(false), Description: editPtr(""),
		})
		if got := readEditRow(t, s.DB(), id).UpdatedAt; got != fixtureUpdated {
			t.Errorf("updated_at = %d after equal patch, want unchanged %d", got, fixtureUpdated)
		}

		mustUpdateGame(t, s, id, store.GamePatch{Favorite: editPtr(true)})
		if got := readEditRow(t, s.DB(), id).UpdatedAt; got != editAt.UnixMilli() {
			t.Errorf("updated_at = %d after a change, want %d (W)", got, editAt.UnixMilli())
		}
	})

	t.Run("EDT-007/trigger BEFORE UPDATE ON game que aborta faz UpdateGame devolver erro e nada muda", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades", Seconds: 60, Plays: 1})["Hades"]
		libExec(t, s.DB(), `CREATE TRIGGER game_block_update BEFORE UPDATE ON game
			BEGIN SELECT RAISE(ABORT, 'game update blocked'); END`)
		before := readEditRow(t, s.DB(), id)

		_, err := s.UpdateGame(t.Context(), id, store.GamePatch{
			Title: editPtr("Novo"), Status: editPtr(domain.StatusCompleted),
		}, editAt)
		if err == nil {
			t.Fatalf("UpdateGame with an aborting trigger err = nil, want an error")
		}
		if after := readEditRow(t, s.DB(), id); after != before {
			t.Errorf("row after aborted edit = %+v, want %+v", after, before)
		}
	})

	t.Run("STO-028/UpdateGame devolve o GameDetail já atualizado", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades", Release: "2017-02-24", Genres: []string{"RPG"},
			Modes: []string{"single"}, Developers: []string{"Supergiant Games"}})["Hades"]

		d := mustUpdateGame(t, s, id, store.GamePatch{
			Title: editPtr("Hades II"), Status: editPtr(domain.StatusPlaying), Favorite: editPtr(true),
			Description: editPtr("Sequência"), CompletedAt: editPtr(editPtr(editDone)), Year: editPtr(editPtr(2024)),
		})

		if d.ID != id || d.Title != "Hades II" || d.Status != domain.StatusPlaying || !d.Favorite ||
			d.Description != "Sequência" || !d.CompletedAt.Equal(editDone) {
			t.Errorf("returned detail = %+v, want the patched fields", d)
		}
		if d.ReleaseDate != "2024" {
			t.Errorf("ReleaseDate = %q, want 2024 (WithYear over 2017-02-24)", d.ReleaseDate)
		}
		requireStrings(t, "Genres", d.Genres, []string{"RPG"})
		requireStrings(t, "PlayModes", d.PlayModes, []string{"single"})
		requireStrings(t, "Developers", d.Developers, []string{"Supergiant Games"})
	})

	t.Run("STO-028/id inexistente devolve ErrGameNotFound", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s, libGame{Title: "Existe"})
		before := countRows(t, s.DB(), `SELECT COUNT(*) FROM game`)
		for _, id := range []int64{999, 0, -1} {
			_, err := s.UpdateGame(t.Context(), id, store.GamePatch{Title: editPtr("Fantasma")}, editAt)
			if !errors.Is(err, store.ErrGameNotFound) {
				t.Errorf("UpdateGame(%d) err = %v, want errors.Is(err, store.ErrGameNotFound)", id, err)
			}
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game`); n != before {
			t.Errorf("game rows = %d, want %d (no insert)", n, before)
		}
	})

	t.Run("EDT-006/editar com sessão Running não muda a sessão", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades"})["Hades"]
		libInsertSession(t, s.DB(), libSessionRow{GameID: id, Started: libAt(9, 20), Active: 120, State: "running"})
		before := editChildRows(t, s.DB(), id)

		mustUpdateGame(t, s, id, store.GamePatch{Title: editPtr("Hades II")})
		if got := editChildRows(t, s.DB(), id); !reflect.DeepEqual(got, before) {
			t.Errorf("session after edit = %v, want %v", got, before)
		}
	})

	t.Run("EDT-006/editar com sessão Grace não muda a sessão", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades"})["Hades"]
		libInsertSession(t, s.DB(), libSessionRow{GameID: id, Started: libAt(9, 20), Active: 120, State: "grace"})
		before := editChildRows(t, s.DB(), id)

		mustUpdateGame(t, s, id, store.GamePatch{Favorite: editPtr(true)})
		if got := editChildRows(t, s.DB(), id); !reflect.DeepEqual(got, before) {
			t.Errorf("session after edit = %v, want %v", got, before)
		}
	})

	t.Run("EDT-006/editar com sessão Paused não muda a sessão nem a pausa aberta", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades"})["Hades"]
		sess := libInsertSession(t, s.DB(), libSessionRow{GameID: id, Started: libAt(9, 20), Active: 120, State: "paused"})
		libInsertPause(t, s.DB(), sess, libAt(9, 21), 0)
		before := editChildRows(t, s.DB(), id)

		mustUpdateGame(t, s, id, store.GamePatch{Description: editPtr("texto")})
		if got := editChildRows(t, s.DB(), id); !reflect.DeepEqual(got, before) {
			t.Errorf("session and open pause after edit = %v, want %v", got, before)
		}
	})

	t.Run("EDT-009/patch vazio devolve o jogo sem alteração e não muda updated_at", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Hades", Release: "2020", Genres: []string{"RPG"}})["Hades"]
		before, err := s.GameDetail(t.Context(), id)
		if err != nil {
			t.Fatalf("GameDetail(%d): %v", id, err)
		}
		rowBefore := readEditRow(t, s.DB(), id)

		d, err := s.UpdateGame(t.Context(), id, store.GamePatch{}, editAt)
		if err != nil {
			t.Fatalf("UpdateGame with an empty patch: %v", err)
		}
		if !reflect.DeepEqual(d, before) {
			t.Errorf("returned detail = %+v, want the game unchanged %+v", d, before)
		}
		if after := readEditRow(t, s.DB(), id); after != rowBefore {
			t.Errorf("row after empty patch = %+v, want %+v", after, rowBefore)
		}
	})
}
