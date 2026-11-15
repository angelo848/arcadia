package store_test

import (
	"database/sql"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/store/dbgen"
)

// versionSevenFile builds a database at goose version 7 (goose UpTo(7)) and runs
// seed on it, if non-nil, before closing it. It returns the path. Opening it
// with store.Open(Migrate) then applies migration 00008.
func versionSevenFile(t *testing.T, seed func(t *testing.T, db *sql.DB)) string {
	t.Helper()
	path := dbPath(t)
	db := openRaw(t, path)
	defer db.Close()
	ctx := t.Context()

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.UpTo(ctx, 7); err != nil {
		t.Fatalf("goose UpTo(7): %v", err)
	}
	if v, err := provider.GetDBVersion(ctx); err != nil || v != 7 {
		t.Fatalf("precondition: goose version = %d (err=%v), want 7", v, err)
	}
	if seed != nil {
		seed(t, db)
	}
	return path
}

// m8Row is one row of the platform table.
type m8Row struct {
	ID         int64
	Name       string
	Category   string
	Icon       sql.NullString
	EmulatorID sql.NullInt64
	SeedKey    sql.NullString
}

func m8Platforms(t *testing.T, db *sql.DB) []m8Row {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT id, name, category, icon, emulator_id, seed_key FROM platform ORDER BY id`)
	if err != nil {
		t.Fatalf("read platforms: %v", err)
	}
	defer rows.Close()
	var out []m8Row
	for rows.Next() {
		var r m8Row
		if err := rows.Scan(&r.ID, &r.Name, &r.Category, &r.Icon, &r.EmulatorID, &r.SeedKey); err != nil {
			t.Fatalf("scan platform: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func m8ByID(rows []m8Row, id int64) (m8Row, bool) {
	for _, r := range rows {
		if r.ID == id {
			return r, true
		}
	}
	return m8Row{}, false
}

func m8ByKey(rows []m8Row, key string) (m8Row, bool) {
	for _, r := range rows {
		if r.SeedKey.Valid && r.SeedKey.String == key {
			return r, true
		}
	}
	return m8Row{}, false
}

func m8Count(rows []m8Row, name string) int {
	n := 0
	for _, r := range rows {
		if strings.EqualFold(r.Name, name) {
			n++
		}
	}
	return n
}

// m8Emulator inserts an emulator and returns its id.
func m8Emulator(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.ExecContext(t.Context(),
		`INSERT INTO emulator (name, exe_path, args_template) VALUES (?, 'C:\Emus\x.exe', '')`, name)
	if err != nil {
		t.Fatalf("insert emulator %q: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("emulator LastInsertId: %v", err)
	}
	return id
}

// m8Platform inserts a version-7 platform (no seed_key column yet). emu is nil
// or an emulator id; icon may be "".
func m8Platform(t *testing.T, db *sql.DB, name, category string, emu any, icon string) int64 {
	t.Helper()
	var ic any
	if icon != "" {
		ic = icon
	}
	res, err := db.ExecContext(t.Context(),
		`INSERT INTO platform (name, category, icon, emulator_id) VALUES (?, ?, ?, ?)`, name, category, ic, emu)
	if err != nil {
		t.Fatalf("insert platform %q: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("platform LastInsertId: %v", err)
	}
	return id
}

// m8SeedPresent checks that every domain.SeedPlatforms entry exists exactly once,
// by key, with the canonical name and category, and returns them by key.
func m8SeedPresent(t *testing.T, rows []m8Row) map[string]m8Row {
	t.Helper()
	out := make(map[string]m8Row, len(domain.SeedPlatforms))
	for _, sp := range domain.SeedPlatforms {
		n := 0
		for _, r := range rows {
			if r.SeedKey.Valid && r.SeedKey.String == sp.Key {
				n++
				out[sp.Key] = r
			}
		}
		if n != 1 {
			t.Errorf("seed key %q appears %d times, want 1", sp.Key, n)
			continue
		}
		got := out[sp.Key]
		if got.Name != sp.Name || got.Category != string(sp.Category) {
			t.Errorf("seed %q = (%q, %q), want (%q, %q)", sp.Key, got.Name, got.Category, sp.Name, sp.Category)
		}
		if c := m8Count(rows, sp.Name); c != 1 {
			t.Errorf("platforms named %q = %d, want 1", sp.Name, c)
		}
	}
	return out
}

func m8NullInt(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: true} }

func m8GameIDs(t *testing.T, db *sql.DB, platformID int64) []int64 {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT id FROM game WHERE platform_id = ? ORDER BY id`, platformID)
	if err != nil {
		t.Fatalf("games of platform %d: %v", platformID, err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// m8PCAndWindows builds a v7 database with PC (lower id) and Windows, each with
// two games, and migrates it. pcEmu/winEmu say whether each gets an emulator.
type m8PCWin struct {
	pcID, winID, pcEmu, winEmu int64
	pcGames, winGames          []int64
}

func m8PCWindowsFile(t *testing.T, pcHasEmu, winHasEmu bool) (string, *m8PCWin) {
	t.Helper()
	st := &m8PCWin{}
	path := versionSevenFile(t, func(t *testing.T, db *sql.DB) {
		var pcE, winE any
		if pcHasEmu {
			st.pcEmu = m8Emulator(t, db, "PCEmu")
			pcE = st.pcEmu
		}
		if winHasEmu {
			st.winEmu = m8Emulator(t, db, "WinEmu")
			winE = st.winEmu
		}
		st.pcID = m8Platform(t, db, "PC", "pc", pcE, "")
		st.winID = m8Platform(t, db, "Windows", "pc", winE, "")
		st.pcGames = []int64{mustInsertGame(t, db, st.pcID, "PC A", ""), mustInsertGame(t, db, st.pcID, "PC B", "")}
		st.winGames = []int64{mustInsertGame(t, db, st.winID, "Win A", ""), mustInsertGame(t, db, st.winID, "Win B", "")}
	})
	return path, st
}

func TestMigration8(t *testing.T) {
	t.Parallel()

	t.Run("PLT-001/banco vazio fica com as 13 plataformas iguais a domain.SeedPlatforms", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		requireGooseVersion(t, s, 9) // alterado em 2026-11-15 (fase 13): migração 00009

		rows := m8Platforms(t, s.DB())
		if len(rows) != len(domain.SeedPlatforms) || len(rows) != 13 {
			t.Fatalf("platform rows = %d, want %d (the seed)", len(rows), len(domain.SeedPlatforms))
		}
		m8SeedPresent(t, rows)
	})

	t.Run("PLT-001/seed sem ícone e sem emulador padrão", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		for _, r := range m8Platforms(t, s.DB()) {
			if r.Icon.Valid {
				t.Errorf("platform %q icon = %q, want NULL", r.Name, r.Icon.String)
			}
			if r.EmulatorID.Valid {
				t.Errorf("platform %q emulator_id = %d, want NULL", r.Name, r.EmulatorID.Int64)
			}
		}
	})

	t.Run("PLT-002/PC vira Windows mantendo id, jogos, sessões, totais e emulador", func(t *testing.T) {
		t.Parallel()
		var pcID, emuID int64
		var games []int64
		var before [][]string
		path := versionSevenFile(t, func(t *testing.T, db *sql.DB) {
			emuID = m8Emulator(t, db, "RetroArch")
			pcID = m8Platform(t, db, "PC", "pc", emuID, "")
			g1 := mustInsertGame(t, db, pcID, "Celeste", "2018-01-25")
			g2 := mustInsertGame(t, db, pcID, "Hades", "")
			games = []int64{g1, g2}
			if _, err := db.ExecContext(t.Context(),
				`UPDATE game SET total_play_seconds = 3600, play_count = 3 WHERE id = ?`, g1); err != nil {
				t.Fatalf("set totals: %v", err)
			}
			sid := insertOpenSession(t, db, g1)
			if _, err := db.ExecContext(t.Context(), `INSERT INTO pause_interval (session_id, paused_at, resumed_at, reason)
				VALUES (?, 1700000100000, 1700000200000, 'hotkey')`, sid); err != nil {
				t.Fatalf("insert pause: %v", err)
			}
			for _, q := range libraryDumpQueries {
				before = append(before, dumpRows(t, db, q))
			}
		})

		s := openStore(t, path, true)
		rows := m8Platforms(t, s.DB())
		got, ok := m8ByID(rows, pcID)
		if !ok {
			t.Fatalf("platform id %d disappeared", pcID)
		}
		if got.Name != "Windows" || got.Category != "pc" || got.SeedKey.String != "windows" || !got.SeedKey.Valid {
			t.Errorf("platform %d = (%q, %q, %v), want (Windows, pc, windows)", pcID, got.Name, got.Category, got.SeedKey)
		}
		if got.EmulatorID != m8NullInt(emuID) {
			t.Errorf("emulator_id = %v, want %d", got.EmulatorID, emuID)
		}
		if c := m8Count(rows, "PC"); c != 0 {
			t.Errorf("platforms named PC = %d, want 0", c)
		}
		if len(rows) != 13 {
			t.Errorf("platform rows = %d, want 13", len(rows))
		}
		m8SeedPresent(t, rows)
		if ids := m8GameIDs(t, s.DB(), pcID); !slices.Equal(ids, games) {
			t.Errorf("games of platform %d = %v, want %v", pcID, ids, games)
		}
		for i, q := range libraryDumpQueries {
			if after := dumpRows(t, s.DB(), q); !slices.Equal(after, before[i]) {
				t.Errorf("rows changed for %s:\n before %v\n after  %v", q, before[i], after)
			}
		}
	})

	t.Run("PLT-003/PC e Windows: jogos vão para Windows, PC some, total de jogos igual, Windows mantém o emulador", func(t *testing.T) {
		t.Parallel()
		path, st := m8PCWindowsFile(t, true, true)
		s := openStore(t, path, true)
		rows := m8Platforms(t, s.DB())

		if _, ok := m8ByID(rows, st.pcID); ok {
			t.Errorf("platform PC (id %d) still exists", st.pcID)
		}
		if c := m8Count(rows, "PC"); c != 0 {
			t.Errorf("platforms named PC = %d, want 0", c)
		}
		w, ok := m8ByID(rows, st.winID)
		if !ok {
			t.Fatalf("platform Windows (id %d) disappeared", st.winID)
		}
		if !w.SeedKey.Valid || w.SeedKey.String != "windows" {
			t.Errorf("Windows seed_key = %v, want windows", w.SeedKey)
		}
		if w.EmulatorID != m8NullInt(st.winEmu) {
			t.Errorf("Windows emulator_id = %v, want its own %d", w.EmulatorID, st.winEmu)
		}
		want := append(slices.Clone(st.winGames), st.pcGames...)
		slices.Sort(want)
		if ids := m8GameIDs(t, s.DB(), st.winID); !slices.Equal(ids, want) {
			t.Errorf("games of Windows = %v, want %v", ids, want)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game`); n != 4 {
			t.Errorf("total games = %d, want 4", n)
		}
		if len(rows) != 13 {
			t.Errorf("platform rows = %d, want 13", len(rows))
		}
		m8SeedPresent(t, rows)
	})

	t.Run("PLT-003/Windows sem emulador herda o de PC", func(t *testing.T) {
		t.Parallel()
		path, st := m8PCWindowsFile(t, true, false)
		s := openStore(t, path, true)
		w, ok := m8ByID(m8Platforms(t, s.DB()), st.winID)
		if !ok {
			t.Fatalf("platform Windows (id %d) disappeared", st.winID)
		}
		if w.EmulatorID != m8NullInt(st.pcEmu) {
			t.Errorf("Windows emulator_id = %v, want PC's %d", w.EmulatorID, st.pcEmu)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game WHERE platform_id = ?`, st.winID); n != 4 {
			t.Errorf("games in Windows = %d, want 4", n)
		}
	})

	t.Run("PLT-004/Sony PlayStation 2 adotada: id, jogos e emulador mantidos, nome e categoria do seed, sem duplicata", func(t *testing.T) {
		t.Parallel()
		var psID, emuID int64
		var games []int64
		path := versionSevenFile(t, func(t *testing.T, db *sql.DB) {
			emuID = m8Emulator(t, db, "PCSX2")
			psID = m8Platform(t, db, "Sony PlayStation 2", "pc", emuID, `platforms\abc.png`)
			games = []int64{mustInsertGame(t, db, psID, "Okami", ""), mustInsertGame(t, db, psID, "Ico", "")}
		})

		s := openStore(t, path, true)
		rows := m8Platforms(t, s.DB())
		got, ok := m8ByID(rows, psID)
		if !ok {
			t.Fatalf("platform id %d disappeared", psID)
		}
		if got.Name != "Sony Playstation 2" || got.Category != "console" ||
			!got.SeedKey.Valid || got.SeedKey.String != "ps2" {
			t.Errorf("platform %d = (%q, %q, %v), want (Sony Playstation 2, console, ps2)",
				psID, got.Name, got.Category, got.SeedKey)
		}
		if got.EmulatorID != m8NullInt(emuID) {
			t.Errorf("emulator_id = %v, want %d", got.EmulatorID, emuID)
		}
		if !got.Icon.Valid || got.Icon.String != `platforms\abc.png` {
			t.Errorf("icon = %v, want kept", got.Icon)
		}
		if c := m8Count(rows, "Sony Playstation 2"); c != 1 {
			t.Errorf("platforms named Sony Playstation 2 = %d, want 1", c)
		}
		if ids := m8GameIDs(t, s.DB(), psID); !slices.Equal(ids, games) {
			t.Errorf("games = %v, want %v", ids, games)
		}
		// 12 other seed rows + the adopted one + the new Windows.
		if len(rows) != 13 {
			t.Errorf("platform rows = %d, want 13", len(rows))
		}
		m8SeedPresent(t, rows)
	})

	// genesisFile builds Sega Genesis (a), SEGA GENESIS (b), sega genesis (c).
	type genesis struct {
		a, b, c       int64
		ga, gb, gc    []int64
		updatedBefore []string
	}
	genesisFile := func(t *testing.T, ea, eb, ec bool) (string, *genesis) {
		t.Helper()
		g := &genesis{}
		path := versionSevenFile(t, func(t *testing.T, db *sql.DB) {
			emu := func(on bool, name string) any {
				if !on {
					return nil
				}
				return m8Emulator(t, db, name)
			}
			g.a = m8Platform(t, db, "Sega Genesis", "console", emu(ea, "EmuA"), "")
			g.b = m8Platform(t, db, "SEGA GENESIS", "console", emu(eb, "EmuB"), "")
			g.c = m8Platform(t, db, "sega genesis", "console", emu(ec, "EmuC"), "")
			g.ga = []int64{mustInsertGame(t, db, g.a, "Sonic", "")}
			g.gb = []int64{mustInsertGame(t, db, g.b, "Streets of Rage", ""), mustInsertGame(t, db, g.b, "Gunstar", "")}
			g.gc = []int64{mustInsertGame(t, db, g.c, "Toejam", "")}
			if _, err := db.ExecContext(t.Context(),
				`UPDATE game SET updated_at = 1700000000000 + id * 1000`); err != nil {
				t.Fatalf("set updated_at: %v", err)
			}
			g.updatedBefore = dumpRows(t, db, `SELECT id, updated_at FROM game ORDER BY id`)
		})
		return path, g
	}

	t.Run("PLT-005/Sega Genesis e SEGA GENESIS: fica o menor id com os jogos de todas", func(t *testing.T) {
		t.Parallel()
		path, g := genesisFile(t, true, true, true)
		s := openStore(t, path, true)
		rows := m8Platforms(t, s.DB())

		if c := m8Count(rows, "sega genesis"); c != 1 {
			t.Fatalf("platforms named sega genesis = %d, want 1", c)
		}
		a, ok := m8ByID(rows, g.a)
		if !ok {
			t.Fatalf("lowest id %d was removed", g.a)
		}
		if _, ok := m8ByID(rows, g.b); ok {
			t.Errorf("platform %d still exists", g.b)
		}
		if _, ok := m8ByID(rows, g.c); ok {
			t.Errorf("platform %d still exists", g.c)
		}
		if a.Name != "Sega Genesis" {
			t.Errorf("name = %q, want the one of the lowest id (Sega Genesis)", a.Name)
		}
		if a.SeedKey.Valid {
			t.Errorf("seed_key = %v, want NULL (not a seed platform)", a.SeedKey)
		}
		want := slices.Concat(g.ga, g.gb, g.gc)
		slices.Sort(want)
		if ids := m8GameIDs(t, s.DB(), g.a); !slices.Equal(ids, want) {
			t.Errorf("games of %d = %v, want %v", g.a, ids, want)
		}
		if a.EmulatorID.Valid == false {
			t.Errorf("emulator_id of the survivor is NULL, want its own")
		}
		if len(rows) != 14 { // 13 seed + Sega Genesis
			t.Errorf("platform rows = %d, want 14", len(rows))
		}
	})

	t.Run("PLT-005/menor id sem emulador herda o da primeira outra por id", func(t *testing.T) {
		t.Parallel()
		var eb, ec int64
		path, g := genesisFile(t, false, true, true)
		db := openRaw(t, path)
		if err := db.QueryRowContext(t.Context(), `SELECT emulator_id FROM platform WHERE id = ?`, g.b).Scan(&eb); err != nil {
			t.Fatalf("read emulator of b: %v", err)
		}
		if err := db.QueryRowContext(t.Context(), `SELECT emulator_id FROM platform WHERE id = ?`, g.c).Scan(&ec); err != nil {
			t.Fatalf("read emulator of c: %v", err)
		}
		_ = db.Close()
		if eb == ec {
			t.Fatalf("precondition: emulators of b and c are equal (%d)", eb)
		}

		s := openStore(t, path, true)
		a, ok := m8ByID(m8Platforms(t, s.DB()), g.a)
		if !ok {
			t.Fatalf("lowest id %d was removed", g.a)
		}
		if a.EmulatorID != m8NullInt(eb) {
			t.Errorf("emulator_id = %v, want the one of the first other by id (%d), not %d", a.EmulatorID, eb, ec)
		}
	})

	t.Run("PLT-005/updated_at dos jogos movidos não muda", func(t *testing.T) {
		t.Parallel()
		path, g := genesisFile(t, false, false, false)
		s := openStore(t, path, true)
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game WHERE platform_id = ?`, g.a); n != 4 {
			t.Fatalf("games in the survivor = %d, want 4", n)
		}
		after := dumpRows(t, s.DB(), `SELECT id, updated_at FROM game ORDER BY id`)
		if !slices.Equal(after, g.updatedBefore) {
			t.Errorf("updated_at changed:\n before %v\n after  %v", g.updatedBefore, after)
		}
	})

	t.Run("PLT-006/InsertPlatform do dbgen com o mesmo nome em outra grafia é recusado", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ctx := t.Context()
		before := countRows(t, s.DB(), `SELECT COUNT(*) FROM platform`)

		if _, err := s.Queries().InsertPlatform(ctx, dbgen.InsertPlatformParams{Name: "Amiga", Category: "pc"}); err != nil {
			t.Fatalf("InsertPlatform(Amiga): %v", err)
		}
		for _, name := range []string{"AMIGA", "amiga", "WINDOWS", "nintendo 64"} {
			if _, err := s.Queries().InsertPlatform(ctx, dbgen.InsertPlatformParams{Name: name, Category: "pc"}); err == nil {
				t.Errorf("InsertPlatform(%q) succeeded, want a unique violation", name)
			}
		}
		if after := countRows(t, s.DB(), `SELECT COUNT(*) FROM platform`); after != before+1 {
			t.Errorf("platform rows = %d, want %d (only Amiga added)", after, before+1)
		}
	})

	t.Run("PLT-006/segundo seed_key igual é recusado pelo índice parcial", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ctx := t.Context()

		if _, err := s.DB().ExecContext(ctx,
			`INSERT INTO platform (name, category, seed_key) VALUES ('Outra', 'pc', 'windows')`); err == nil {
			t.Errorf("second seed_key 'windows' accepted, want a unique violation")
		}
		// NULL keys are not constrained by the partial index.
		for _, name := range []string{"Livre 1", "Livre 2"} {
			if _, err := s.DB().ExecContext(ctx,
				`INSERT INTO platform (name, category, seed_key) VALUES (?, 'pc', NULL)`, name); err != nil {
				t.Errorf("insert %q with NULL seed_key: %v", name, err)
			}
		}
	})

	t.Run("PLT-007/plataforma do seed excluída não volta ao reabrir o banco", func(t *testing.T) {
		t.Parallel()
		path := dbPath(t)
		s, err := store.Open(t.Context(), path, store.OpenOptions{Migrate: true})
		if err != nil {
			t.Fatalf("store.Open: %v", err)
		}
		res, err := s.DB().ExecContext(t.Context(), `DELETE FROM platform WHERE seed_key = '3ds'`)
		if err != nil {
			_ = s.Close()
			t.Fatalf("delete 3ds: %v", err)
		}
		if n, _ := res.RowsAffected(); n != 1 {
			_ = s.Close()
			t.Fatalf("rows deleted = %d, want 1", n)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("Store.Close: %v", err)
		}

		s2 := openStore(t, path, true)
		rows := m8Platforms(t, s2.DB())
		if len(rows) != 12 {
			t.Errorf("platform rows after reopening = %d, want 12", len(rows))
		}
		if _, ok := m8ByKey(rows, "3ds"); ok {
			t.Errorf("seed platform 3ds came back")
		}
		if c := m8Count(rows, "Nintendo 3DS"); c != 0 {
			t.Errorf("platforms named Nintendo 3DS = %d, want 0", c)
		}
	})

	t.Run("PLT-008/00008 roda em transação (arquivo sem NO TRANSACTION)", func(t *testing.T) {
		t.Parallel()
		data, err := os.ReadFile("migrations/00008_platforms.sql")
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		src := strings.ToUpper(string(data))
		if strings.Contains(src, "NO TRANSACTION") {
			t.Errorf("00008_platforms.sql contains NO TRANSACTION; it must run in a goose transaction")
		}
		if !strings.Contains(src, "+GOOSE UP") {
			t.Errorf("00008_platforms.sql has no +goose Up section")
		}
	})
}

func TestMigration8TrimsNames(t *testing.T) {
	t.Parallel()

	t.Run("PLT-005/PLT-006/nome com espaços nas pontas é aparado e AddPlatform não duplica", func(t *testing.T) {
		t.Parallel()
		var id int64
		path := versionSevenFile(t, func(t *testing.T, db *sql.DB) {
			id = m8Platform(t, db, " Sega Genesis ", "console", nil, "")
		})
		s := openStore(t, path, true)
		got, ok := m8ByID(m8Platforms(t, s.DB()), id)
		if !ok {
			t.Fatalf("platform %d disappeared", id)
		}
		if got.Name != "Sega Genesis" {
			t.Errorf("name = %q, want %q", got.Name, "Sega Genesis")
		}
		p, created, err := s.AddPlatform(t.Context(), "sega genesis", domain.CategoryConsole)
		if err != nil {
			t.Fatalf("AddPlatform: %v", err)
		}
		if created || p.ID != id {
			t.Errorf("AddPlatform = (id %d, created %v), want the existing id %d with created=false", p.ID, created, id)
		}
		if c := m8Count(m8Platforms(t, s.DB()), "Sega Genesis"); c != 1 {
			t.Errorf("platforms named Sega Genesis = %d, want 1", c)
		}
	})
}

func TestPlatformGameCount(t *testing.T) {
	t.Parallel()

	t.Run("PLT-014/GameCount conta jogos ocultos e ordem por nome sem maiúsculas", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ctx := t.Context()

		amiga := insertPlatform(t, s, "amiga")
		atari := insertPlatform(t, s, "Atari 2600")
		zx := insertPlatform(t, s, "ZX Spectrum")
		for _, title := range []string{"A1", "A2", "A3"} {
			mustInsertGame(t, s.DB(), amiga, title, "")
		}
		hidden := mustInsertGame(t, s.DB(), amiga, "A4 hidden", "")
		if _, err := s.DB().ExecContext(ctx, `UPDATE game SET hidden = 1 WHERE id = ?`, hidden); err != nil {
			t.Fatalf("hide game: %v", err)
		}
		mustInsertGame(t, s.DB(), zx, "Z1", "")

		check := func(t *testing.T, label string, ps []store.Platform) {
			t.Helper()
			names := make([]string, len(ps))
			for i, p := range ps {
				names[i] = p.Name
			}
			sorted := slices.Clone(names)
			slices.SortStableFunc(sorted, func(x, y string) int {
				return strings.Compare(strings.ToLower(x), strings.ToLower(y))
			})
			if !slices.Equal(names, sorted) {
				t.Errorf("%s: order = %v, want by name ignoring case %v", label, names, sorted)
			}
			want := map[int64]int64{amiga: 4, atari: 0, zx: 1}
			seen := 0
			for _, p := range ps {
				if w, ok := want[p.ID]; ok {
					seen++
					if p.GameCount != w {
						t.Errorf("%s: GameCount of %q = %d, want %d", label, p.Name, p.GameCount, w)
					}
				} else if p.GameCount != 0 {
					t.Errorf("%s: GameCount of %q = %d, want 0", label, p.Name, p.GameCount)
				}
			}
			if seen != len(want) {
				t.Errorf("%s: found %d of the %d test platforms", label, seen, len(want))
			}
			if len(ps) != 13+3 {
				t.Errorf("%s: platforms = %d, want 16 (seed + 3)", label, len(ps))
			}
		}

		ps, err := s.Platforms(ctx)
		if err != nil {
			t.Fatalf("Platforms: %v", err)
		}
		check(t, "Platforms", ps)

		cat, err := s.Catalog(ctx)
		if err != nil {
			t.Fatalf("Catalog: %v", err)
		}
		check(t, "Catalog", cat.Platforms)

		// SeedKey and Icon are read too.
		w, ok := func() (store.Platform, bool) {
			for _, p := range ps {
				if p.Name == "Windows" {
					return p, true
				}
			}
			return store.Platform{}, false
		}()
		if !ok {
			t.Fatalf("Windows not in Platforms")
		}
		if w.SeedKey != "windows" || w.Icon != "" {
			t.Errorf("Windows = (SeedKey %q, Icon %q), want (windows, empty)", w.SeedKey, w.Icon)
		}
	})
}
