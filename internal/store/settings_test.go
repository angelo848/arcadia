package store_test

import (
	"database/sql"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/arcadia-launcher/arcadia/internal/store"
)

// columnDetail is one row of PRAGMA table_info with the flags AST-001 checks.
type columnDetail struct {
	Name    string
	Type    string
	NotNull bool
	PK      int // 1-based position in the primary key, 0 when not part of it
}

func tableColumnDetails(t *testing.T, db *sql.DB, table string) []columnDetail {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT name, type, "notnull", pk FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatalf("pragma_table_info(%s): %v", table, err)
	}
	defer rows.Close()
	var cols []columnDetail
	for rows.Next() {
		var (
			c       columnDetail
			notNull int
		)
		if err := rows.Scan(&c.Name, &c.Type, &notNull, &c.PK); err != nil {
			t.Fatalf("scan pragma_table_info(%s): %v", table, err)
		}
		c.NotNull = notNull != 0
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return cols
}

// versionOneFile builds a database at goose version 1 by applying only the
// first migration from the source directory (goose UpTo(1)), then inserts a
// platform and a game so the 1 -> 2 migration can be checked for data loss.
func versionOneFile(t *testing.T) (path string, gameID int64) {
	t.Helper()
	path = dbPath(t)
	db := openRaw(t, path)
	defer db.Close()

	provider, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS("migrations"))
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.UpTo(t.Context(), 1); err != nil {
		t.Fatalf("goose UpTo(1): %v", err)
	}
	if v, err := provider.GetDBVersion(t.Context()); err != nil || v != 1 {
		t.Fatalf("precondition: goose version = %d (err=%v), want 1", v, err)
	}
	if slices.Contains(tableNames(t, db), "app_setting") {
		t.Fatal("precondition: app_setting must not exist at version 1")
	}

	res, err := db.ExecContext(t.Context(),
		`INSERT INTO platform (name, category) VALUES ('Nintendo 64', 'console')`)
	if err != nil {
		t.Fatalf("insert platform: %v", err)
	}
	pid, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("platform LastInsertId: %v", err)
	}
	gameID = mustInsertGame(t, db, pid, "Ocarina of Time", "1998-11-21")
	return path, gameID
}

// --- AST-001 (schema) -------------------------------------------------------

func TestAppSettingSchema(t *testing.T) {
	t.Parallel()

	t.Run("AST-001/tabela app_setting com key TEXT PK, value TEXT NOT NULL, updated_at INTEGER NOT NULL", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		db := s.DB()
		if !slices.Contains(tableNames(t, db), "app_setting") {
			t.Fatalf("table app_setting missing; tables = %v", tableNames(t, db))
		}

		want := map[string]struct {
			typ     string
			notNull bool
			pk      int
		}{
			"key":        {"TEXT", false, 1}, // PRIMARY KEY; NOT NULL is not required by the DDL
			"value":      {"TEXT", true, 0},
			"updated_at": {"INTEGER", true, 0},
		}
		cols := tableColumnDetails(t, db, "app_setting")
		if len(cols) != len(want) {
			t.Errorf("app_setting has %d columns %+v, want exactly key, value, updated_at", len(cols), cols)
		}
		for _, c := range cols {
			w, ok := want[c.Name]
			if !ok {
				t.Errorf("unexpected column app_setting.%s", c.Name)
				continue
			}
			if !strings.EqualFold(c.Type, w.typ) {
				t.Errorf("app_setting.%s type = %q, want %s", c.Name, c.Type, w.typ)
			}
			if w.notNull && !c.NotNull {
				t.Errorf("app_setting.%s is nullable, want NOT NULL", c.Name)
			}
			if c.PK != w.pk {
				t.Errorf("app_setting.%s pk position = %d, want %d", c.Name, c.PK, w.pk)
			}
			delete(want, c.Name)
		}
		for name := range want {
			t.Errorf("column app_setting.%s missing", name)
		}
	})

	t.Run("AST-001/banco novo fica em SchemaVersion", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		v, err := s.CurrentVersion(t.Context())
		if err != nil {
			t.Fatalf("CurrentVersion: %v", err)
		}
		if v != store.SchemaVersion {
			t.Errorf("CurrentVersion = %d, want %d", v, store.SchemaVersion)
		}
	})

	t.Run("AST-001/banco na versão 1 migra para SchemaVersion mantendo os dados", func(t *testing.T) {
		t.Parallel()
		path, gameID := versionOneFile(t)

		s := openStore(t, path, true)
		ctx := t.Context()

		v, err := s.CurrentVersion(ctx)
		if err != nil {
			t.Fatalf("CurrentVersion: %v", err)
		}
		if v != store.SchemaVersion {
			t.Errorf("CurrentVersion = %d, want %d", v, store.SchemaVersion)
		}
		if !slices.Contains(tableNames(t, s.DB()), "app_setting") {
			t.Errorf("app_setting missing after migrating from version 1")
		}

		platforms, err := s.Queries().ListPlatforms(ctx)
		if err != nil {
			t.Fatalf("ListPlatforms: %v", err)
		}
		if countNamed(platforms, "Nintendo 64") != 1 {
			t.Errorf("ListPlatforms = %+v, want one platform named Nintendo 64", platforms)
		}
		var title string
		if err := s.DB().QueryRowContext(ctx, `SELECT title FROM game WHERE id = ?`, gameID).Scan(&title); err != nil {
			t.Fatalf("read game %d: %v", gameID, err)
		}
		if title != "Ocarina of Time" {
			t.Errorf("game title = %q, want Ocarina of Time", title)
		}
		// The play_mode seeds of migration 1 must survive as well.
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM play_mode`); n != 5 {
			t.Errorf("play_mode rows = %d, want 5", n)
		}
	})
}

// --- AST-001 (API) ----------------------------------------------------------

func TestSettings(t *testing.T) {
	t.Parallel()

	// Fixed instants (no time.Now): sub-millisecond nanos and a non-UTC zone
	// make sure updated_at is UnixMilli of the instant, not of a reformatted value.
	brt := time.FixedZone("BRT", -3*60*60)
	t1 := time.Date(2026, time.November, 8, 10, 30, 15, 123_456_789, brt)
	t2 := time.Date(2026, time.November, 9, 22, 5, 0, 987_000_001, time.UTC)

	t.Run("AST-001/chave inexistente retorna ErrSettingNotFound", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ctx := t.Context()

		cases := []struct {
			name  string
			setup func(t *testing.T)
		}{
			{"tabela vazia", func(*testing.T) {}},
			{"outra chave gravada", func(t *testing.T) {
				if err := s.SetSetting(ctx, "other", "x", t1); err != nil {
					t.Fatalf("SetSetting(other): %v", err)
				}
			}},
		}
		for _, c := range cases {
			c.setup(t)
			got, err := s.Setting(ctx, "autostart")
			if !errors.Is(err, store.ErrSettingNotFound) {
				t.Errorf("%s: Setting(autostart) err = %v, want errors.Is(err, store.ErrSettingNotFound)", c.name, err)
			}
			if got != "" {
				t.Errorf("%s: Setting(autostart) value = %q, want \"\"", c.name, got)
			}
		}
	})

	t.Run("AST-001/gravar duas vezes devolve o último valor", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ctx := t.Context()

		writes := []struct {
			value string
			at    time.Time
		}{
			{"true", t1},
			{"false", t2},
		}
		for i, w := range writes {
			if err := s.SetSetting(ctx, "autostart", w.value, w.at); err != nil {
				t.Fatalf("SetSetting #%d(%q): %v", i+1, w.value, err)
			}
			got, err := s.Setting(ctx, "autostart")
			if err != nil {
				t.Fatalf("Setting after write #%d: %v", i+1, err)
			}
			if got != w.value {
				t.Errorf("Setting after write #%d = %q, want %q", i+1, got, w.value)
			}
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM app_setting WHERE key = 'autostart'`); n != 1 {
			t.Errorf("app_setting rows for autostart = %d, want 1 (upsert)", n)
		}
	})

	t.Run("AST-001/updated_at é UnixMilli do instante recebido", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ctx := t.Context()

		readUpdatedAt := func(t *testing.T) int64 {
			t.Helper()
			var ms int64
			if err := s.DB().QueryRowContext(ctx,
				`SELECT updated_at FROM app_setting WHERE key = 'autostart'`).Scan(&ms); err != nil {
				t.Fatalf("read updated_at: %v", err)
			}
			return ms
		}

		for i, at := range []time.Time{t1, t2} {
			if err := s.SetSetting(ctx, "autostart", "true", at); err != nil {
				t.Fatalf("SetSetting #%d: %v", i+1, err)
			}
			if got, want := readUpdatedAt(t), at.UnixMilli(); got != want {
				t.Errorf("updated_at after write #%d = %d, want %d (UnixMilli of %v)", i+1, got, want, at)
			}
		}
	})
}
