package store_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// addAt is a fixed instant (no time.Now) with sub-millisecond nanos and a
// non-UTC zone, so created_at must be UnixMilli of the instant itself.
var addAt = time.Date(2026, time.November, 9, 14, 20, 30, 456_789_123, time.FixedZone("BRT", -3*60*60))

func mustAddGame(t *testing.T, s *store.Store, g store.NewGame) store.Game {
	t.Helper()
	got, err := s.AddGame(t.Context(), g)
	if err != nil {
		t.Fatalf("AddGame(%+v): %v", g, err)
	}
	return got
}

// gameRow is the raw game row as stored, read bypassing the store API.
type gameRow struct {
	PlatformID                                      int64
	Title, LaunchType, LaunchTarget, LaunchArgs, WD string
	TotalPlaySeconds, PlayCount                     int64
	FirstPlayedAt, LastPlayedAt                     sql.NullInt64
	CreatedAt, UpdatedAt                            int64
}

func readGameRow(t *testing.T, db *sql.DB, id int64) gameRow {
	t.Helper()
	var r gameRow
	if err := db.QueryRowContext(t.Context(), `SELECT platform_id, title, launch_type, launch_target, launch_args,
		working_dir, total_play_seconds, play_count, first_played_at, last_played_at, created_at, updated_at
		FROM game WHERE id = ?`, id).Scan(&r.PlatformID, &r.Title, &r.LaunchType, &r.LaunchTarget, &r.LaunchArgs,
		&r.WD, &r.TotalPlaySeconds, &r.PlayCount, &r.FirstPlayedAt, &r.LastPlayedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
		t.Fatalf("read game %d: %v", id, err)
	}
	return r
}

type platformRow struct {
	ID             int64
	Name, Category string
}

func platformRows(t *testing.T, db *sql.DB) []platformRow {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT id, name, category FROM platform ORDER BY id`)
	if err != nil {
		t.Fatalf("select platform: %v", err)
	}
	defer rows.Close()
	var out []platformRow
	for rows.Next() {
		var p platformRow
		if err := rows.Scan(&p.ID, &p.Name, &p.Category); err != nil {
			t.Fatalf("scan platform: %v", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func sameGame(a, b store.Game) bool {
	return a.ID == b.ID && a.Title == b.Title && a.LaunchType == b.LaunchType &&
		a.LaunchTarget == b.LaunchTarget && a.LaunchArgs == b.LaunchArgs && a.WorkingDir == b.WorkingDir &&
		a.TotalPlaySeconds == b.TotalPlaySeconds && a.PlayCount == b.PlayCount &&
		a.FirstPlayedAt.Equal(b.FirstPlayedAt) && a.LastPlayedAt.Equal(b.LastPlayedAt)
}

func TestAddGame(t *testing.T) {
	t.Parallel()

	t.Run("GAM-001/linha com title, launch_type exe, launch_target limpo, working_dir, launch_args vazio, totais 0 e datas nulas", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		dir := filepath.Join(t.TempDir(), "Hollow Knight")
		sep := string(filepath.Separator)
		// Unclean path: "<dir>/sub/../hollow_knight.exe" must be stored as filepath.Clean of it.
		raw := dir + sep + "sub" + sep + ".." + sep + "hollow_knight.exe"
		wantTarget := filepath.Clean(raw)
		if wantTarget == raw {
			t.Fatalf("precondition: %q should not already be clean", raw)
		}

		g := mustAddGame(t, s, store.NewGame{Title: "Hollow Knight", Exe: raw, WorkingDir: dir, At: addAt})

		if g.ID <= 0 {
			t.Errorf("returned ID = %d, want > 0", g.ID)
		}
		want := store.Game{
			ID: g.ID, Title: "Hollow Knight", LaunchType: "exe", LaunchTarget: wantTarget,
			LaunchArgs: "", WorkingDir: dir,
		}
		if !sameGame(g, want) {
			t.Errorf("returned Game = %+v, want %+v", g, want)
		}
		if !g.FirstPlayedAt.IsZero() || !g.LastPlayedAt.IsZero() {
			t.Errorf("returned FirstPlayedAt/LastPlayedAt = %v/%v, want zero", g.FirstPlayedAt, g.LastPlayedAt)
		}

		r := readGameRow(t, s.DB(), g.ID)
		if r.Title != "Hollow Knight" || r.LaunchType != "exe" || r.LaunchTarget != wantTarget ||
			r.LaunchArgs != "" || r.WD != dir || r.TotalPlaySeconds != 0 || r.PlayCount != 0 {
			t.Errorf("row = %+v, want title Hollow Knight, launch_type exe, launch_target %q, launch_args '', working_dir %q, totals 0",
				r, wantTarget, dir)
		}
		if r.FirstPlayedAt.Valid || r.LastPlayedAt.Valid {
			t.Errorf("first_played_at/last_played_at = %v/%v, want NULL", r.FirstPlayedAt, r.LastPlayedAt)
		}
	})

	t.Run("GAM-001/created_at e updated_at são UnixMilli de At", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := mustAddGame(t, s, store.NewGame{Title: "Celeste", Exe: `C:\Games\Celeste\Celeste.exe`, WorkingDir: `C:\Games\Celeste`, At: addAt})
		r := readGameRow(t, s.DB(), g.ID)
		if want := addAt.UnixMilli(); r.CreatedAt != want || r.UpdatedAt != want {
			t.Errorf("created_at/updated_at = %d/%d, want both %d (UnixMilli of %v)", r.CreatedAt, r.UpdatedAt, want, addAt)
		}
	})

	t.Run("GAM-001/plataforma PC (categoria pc) criada na primeira adição", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		if ps := platformRows(t, s.DB()); len(ps) != 0 {
			t.Fatalf("precondition: platforms = %+v, want none", ps)
		}
		g := mustAddGame(t, s, store.NewGame{Title: "Hades", Exe: `C:\Games\Hades\Hades.exe`, WorkingDir: `C:\Games\Hades`, At: addAt})
		ps := platformRows(t, s.DB())
		if len(ps) != 1 || ps[0].Name != "PC" || ps[0].Category != "pc" {
			t.Fatalf("platforms = %+v, want exactly one {Name:PC Category:pc}", ps)
		}
		if r := readGameRow(t, s.DB(), g.ID); r.PlatformID != ps[0].ID {
			t.Errorf("game.platform_id = %d, want %d (PC)", r.PlatformID, ps[0].ID)
		}
	})

	t.Run("GAM-001/segunda adição reutiliza a plataforma", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g1 := mustAddGame(t, s, store.NewGame{Title: "Hades", Exe: `C:\Games\Hades\Hades.exe`, WorkingDir: `C:\Games\Hades`, At: addAt})
		g2 := mustAddGame(t, s, store.NewGame{Title: "Celeste", Exe: `C:\Games\Celeste\Celeste.exe`, WorkingDir: `C:\Games\Celeste`, At: addAt.Add(time.Minute)})
		ps := platformRows(t, s.DB())
		if len(ps) != 1 || ps[0].Name != "PC" || ps[0].Category != "pc" {
			t.Fatalf("platforms = %+v, want exactly one {Name:PC Category:pc}", ps)
		}
		r1, r2 := readGameRow(t, s.DB(), g1.ID), readGameRow(t, s.DB(), g2.ID)
		if r1.PlatformID != ps[0].ID || r2.PlatformID != ps[0].ID {
			t.Errorf("platform_id = %d and %d, want both %d", r1.PlatformID, r2.PlatformID, ps[0].ID)
		}
	})

	t.Run("GAM-001/mesmo exe duas vezes cria dois ids", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ng := store.NewGame{Title: "Hades", Exe: `C:\Games\Hades\Hades.exe`, WorkingDir: `C:\Games\Hades`, At: addAt}
		g1 := mustAddGame(t, s, ng)
		g2, err := s.AddGame(t.Context(), ng)
		if err != nil {
			t.Fatalf("second AddGame of the same exe: %v, want success (decision P2)", err)
		}
		if g1.ID == g2.ID {
			t.Errorf("both additions returned ID %d, want distinct ids", g1.ID)
		}
		games := libSearch(t, s, libFilter())
		if len(games) != 2 {
			t.Errorf("Games = %+v, want 2 games", games)
		}
	})

	t.Run("GAM-001/mesmo caminho com outra caixa cria outro jogo", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g1 := mustAddGame(t, s, store.NewGame{Title: "HK", Exe: `C:\Games\HK\hollow_knight.exe`, WorkingDir: `C:\Games\HK`, At: addAt})
		g2 := mustAddGame(t, s, store.NewGame{Title: "HK", Exe: `c:\games\hk\HOLLOW_KNIGHT.EXE`, WorkingDir: `c:\games\hk`, At: addAt})
		if g1.ID == g2.ID {
			t.Errorf("both additions returned ID %d, want distinct ids", g1.ID)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game`); n != 2 {
			t.Errorf("game rows = %d, want 2", n)
		}
		r2 := readGameRow(t, s.DB(), g2.ID)
		if !strings.Contains(r2.LaunchTarget, "HOLLOW_KNIGHT.EXE") {
			t.Errorf("second launch_target = %q, want the case as given", r2.LaunchTarget)
		}
	})
}

func TestGames(t *testing.T) {
	t.Parallel()

	t.Run("GAM-005/b, A, a e C (ids 1..4) saem na ordem A(2), a(3), b(1), C(4)", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		for i, title := range []string{"b", "A", "a", "C"} {
			g := mustAddGame(t, s, store.NewGame{Title: title, Exe: `C:\Games\` + title + `.exe`, WorkingDir: `C:\Games`, At: addAt})
			if g.ID != int64(i+1) {
				t.Fatalf("precondition: %q got id %d, want %d", title, g.ID, i+1)
			}
		}
		games := libSearch(t, s, libFilter())
		type item struct {
			Title string
			ID    int64
		}
		got := make([]item, 0, len(games))
		for _, g := range games {
			got = append(got, item{g.Title, g.ID})
		}
		want := []item{{"A", 2}, {"a", 3}, {"b", 1}, {"C", 4}}
		if !slices.Equal(got, want) {
			t.Errorf("Games order = %v, want %v", got, want)
		}
	})

	t.Run("GAM-005/biblioteca vazia devolve slice vazio, não nil", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		games := libRun(t, s, libQuery(libFilter(), domain.SortTitle)).Games
		if games == nil || len(games) != 0 {
			t.Errorf("Games = %#v, want a non-nil empty slice", games)
		}
	})
}

func TestGame(t *testing.T) {
	t.Parallel()

	t.Run("GAM-001/devolve o jogo pelo id", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		mustAddGame(t, s, store.NewGame{Title: "Hades", Exe: `C:\Games\Hades\Hades.exe`, WorkingDir: `C:\Games\Hades`, At: addAt})
		added := mustAddGame(t, s, store.NewGame{Title: "Celeste", Exe: `C:\Games\Celeste\Celeste.exe`, WorkingDir: `C:\Games\Celeste`, At: addAt})
		got, err := s.Game(t.Context(), added.ID)
		if err != nil {
			t.Fatalf("Game(%d): %v", added.ID, err)
		}
		if !sameGame(got, added) {
			t.Errorf("Game(%d) = %+v, want %+v", added.ID, got, added)
		}
	})

	t.Run("GAM-001/id inexistente: errors.Is ErrGameNotFound", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := mustAddGame(t, s, store.NewGame{Title: "Hades", Exe: `C:\Games\Hades\Hades.exe`, WorkingDir: `C:\Games\Hades`, At: addAt})
		for _, id := range []int64{g.ID + 1, 0, -1} {
			_, err := s.Game(t.Context(), id)
			if !errors.Is(err, store.ErrGameNotFound) {
				t.Errorf("Game(%d) err = %v, want errors.Is(err, store.ErrGameNotFound)", id, err)
			}
		}
	})
}
