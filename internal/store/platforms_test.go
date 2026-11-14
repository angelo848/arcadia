package store_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// Tests of the platform operations of phase 9 (PLT-013, PLT-014, PLT-016..PLT-021,
// PLT-025..PLT-027) against a real migrated store (13 seed platforms).

// platSeedID returns the id of the platform with the seed key, failing when absent.
func platSeedID(t *testing.T, s *store.Store, key string) int64 {
	t.Helper()
	return countRows(t, s.DB(), `SELECT id FROM platform WHERE seed_key = ?`, key)
}

// platGame inserts a game of the platform straight into the table and returns its id.
func platGame(t *testing.T, s *store.Store, platformID int64, title string, hidden bool) int64 {
	t.Helper()
	id, err := insertGame(s.DB(), t.Context(), platformID, title, "")
	if err != nil {
		t.Fatalf("insert game %q: %v", title, err)
	}
	if hidden {
		libExec(t, s.DB(), `UPDATE game SET hidden = 1 WHERE id = ?`, id)
	}
	return id
}

// platAdd adds a platform and requires created == true.
func platAdd(t *testing.T, s *store.Store, name string, cat domain.PlatformCategory) store.Platform {
	t.Helper()
	p, created, err := s.AddPlatform(t.Context(), name, cat)
	if err != nil {
		t.Fatalf("AddPlatform(%q): %v", name, err)
	}
	if !created {
		t.Fatalf("AddPlatform(%q): created = false, want a new platform", name)
	}
	return p
}

// platDelete deletes a platform without games and requires success.
func platDelete(t *testing.T, s *store.Store, id int64) {
	t.Helper()
	if _, err := s.DeletePlatform(t.Context(), id); err != nil {
		t.Fatalf("DeletePlatform(%d): %v", id, err)
	}
}

func platByID(t *testing.T, s *store.Store, id int64) store.Platform {
	t.Helper()
	p, err := s.PlatformByID(t.Context(), id)
	if err != nil {
		t.Fatalf("PlatformByID(%d): %v", id, err)
	}
	return p
}

func platCount(t *testing.T, s *store.Store) int64 {
	t.Helper()
	return countRows(t, s.DB(), `SELECT COUNT(*) FROM platform`)
}

func TestPlatforms(t *testing.T) {
	t.Parallel()

	t.Run("PLT-025/mesmo nome em outra grafia acha a plataforma", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		made := platAdd(t, s, "Mega Drive", domain.CategoryConsole)

		got, found, err := s.ResolvePlatform(t.Context(), "  MEGA drive ")
		if err != nil || !found {
			t.Fatalf("ResolvePlatform = %+v, %v, %v; want found", got, found, err)
		}
		if got.ID != made.ID || got.Name != "Mega Drive" {
			t.Errorf("platform = %+v, want %+v", got, made)
		}
	})

	t.Run("PLT-025/nome canônico acha a plataforma do seed renomeada", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := platSeedID(t, s, "switch")
		if _, err := s.UpdatePlatform(t.Context(), id, "Meu Switch", domain.CategoryConsole); err != nil {
			t.Fatalf("UpdatePlatform: %v", err)
		}

		got, found, err := s.ResolvePlatform(t.Context(), "nintendo switch")
		if err != nil || !found {
			t.Fatalf("ResolvePlatform = %+v, %v, %v; want found", got, found, err)
		}
		if got.ID != id || got.SeedKey != "switch" || got.Name != "Meu Switch" {
			t.Errorf("platform = %+v, want the renamed seed platform %d", got, id)
		}
	})

	t.Run("PLT-025/nome canônico com chave sem plataforma não cria (ResolvePlatform)", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		platDelete(t, s, platSeedID(t, s, "n64"))
		before := platCount(t, s)

		_, found, err := s.ResolvePlatform(t.Context(), "Nintendo 64")
		if err != nil {
			t.Fatalf("ResolvePlatform: %v", err)
		}
		if found {
			t.Errorf("found = true, want false (the seed platform was deleted)")
		}
		if after := platCount(t, s); after != before {
			t.Errorf("platforms = %d, want %d: ResolvePlatform never creates", after, before)
		}
	})

	t.Run("PLT-025/nome inexistente devolve false", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		before := platCount(t, s)
		p, found, err := s.ResolvePlatform(t.Context(), "Atari 2600")
		if err != nil || found || p.ID != 0 {
			t.Errorf("ResolvePlatform = %+v, %v, %v; want zero, false, nil", p, found, err)
		}
		if after := platCount(t, s); after != before {
			t.Errorf("platforms = %d, want %d", after, before)
		}
	})

	t.Run("PLT-016/AddPlatform de nome existente devolve created=false e ignora a categoria", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		first := platAdd(t, s, "Mega Drive", domain.CategoryConsole)
		before := platCount(t, s)

		again, created, err := s.AddPlatform(t.Context(), "mega DRIVE", domain.CategoryArcade)
		if err != nil {
			t.Fatalf("AddPlatform: %v", err)
		}
		if created {
			t.Errorf("created = true, want false")
		}
		if again.ID != first.ID || again.Name != "Mega Drive" || again.Category != domain.CategoryConsole {
			t.Errorf("platform = %+v, want the existing %+v (category ignored)", again, first)
		}

		// A seed platform found by its canonical name behaves the same way.
		seed, created, err := s.AddPlatform(t.Context(), "nintendo 64", domain.CategoryArcade)
		if err != nil {
			t.Fatalf("AddPlatform(nintendo 64): %v", err)
		}
		if created || seed.SeedKey != "n64" || seed.Category != domain.CategoryConsole {
			t.Errorf("seed platform = %+v created=%v, want the existing n64 console, created=false", seed, created)
		}
		if after := platCount(t, s); after != before {
			t.Errorf("platforms = %d, want %d", after, before)
		}
	})

	t.Run("PLT-017/AddPlatform com nome canônico de chave livre grava seed_key e categoria do seed", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		platDelete(t, s, platSeedID(t, s, "n64"))

		p, created, err := s.AddPlatform(t.Context(), "nintendo 64", domain.CategoryArcade)
		if err != nil {
			t.Fatalf("AddPlatform: %v", err)
		}
		if !created {
			t.Errorf("created = false, want true")
		}
		if p.SeedKey != "n64" {
			t.Errorf("SeedKey = %q, want n64", p.SeedKey)
		}
		if p.Category != domain.CategoryConsole {
			t.Errorf("Category = %q, want console (the seed one, not the sent arcade)", p.Category)
		}
		if !strings.EqualFold(p.Name, "Nintendo 64") {
			t.Errorf("Name = %q, want Nintendo 64", p.Name)
		}
		if got := platSeedID(t, s, "n64"); got != p.ID {
			t.Errorf("row with seed_key n64 = %d, want %d", got, p.ID)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM platform WHERE seed_key = 'n64'`); n != 1 {
			t.Errorf("platforms with key n64 = %d, want 1", n)
		}
	})

	t.Run("PLT-018/renomear mantém jogos, seed_key, ícone e emulador", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := platSeedID(t, s, "windows")
		game := platGame(t, s, id, "Hades", false)
		icon := `platforms\abc.png`
		if _, _, err := s.SetPlatformIcon(t.Context(), id, icon); err != nil {
			t.Fatalf("SetPlatformIcon: %v", err)
		}
		emu, err := s.AddEmulator(t.Context(), "Emu", filepath.Join(t.TempDir(), "e.exe"), "{rom}")
		if err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		emuID := emu.ID
		if err := s.SetPlatformEmulator(t.Context(), id, &emuID); err != nil {
			t.Fatalf("SetPlatformEmulator: %v", err)
		}

		got, err := s.UpdatePlatform(t.Context(), id, "  PC Gamer ", domain.CategoryPC)
		if err != nil {
			t.Fatalf("UpdatePlatform: %v", err)
		}
		if got.ID != id || got.Name != "PC Gamer" {
			t.Errorf("platform = %+v, want id %d named PC Gamer (trimmed)", got, id)
		}
		now := platByID(t, s, id)
		if now.Name != "PC Gamer" || now.SeedKey != "windows" || now.Icon != icon || now.GameCount != 1 ||
			now.EmulatorID == nil || *now.EmulatorID != emuID {
			t.Errorf("stored platform = %+v, want PC Gamer, key windows, icon %q, 1 game, emulator %d", now, icon, emuID)
		}
		if r := readGameRow(t, s.DB(), game); r.PlatformID != id {
			t.Errorf("game.platform_id = %d, want %d", r.PlatformID, id)
		}
	})

	t.Run("PLT-018/mudar só as maiúsculas do próprio nome é aceito", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		p := platAdd(t, s, "Mega Drive", domain.CategoryConsole)

		got, err := s.UpdatePlatform(t.Context(), p.ID, "MEGA DRIVE", domain.CategoryConsole)
		if err != nil {
			t.Fatalf("UpdatePlatform: %v", err)
		}
		if got.Name != "MEGA DRIVE" || platByID(t, s, p.ID).Name != "MEGA DRIVE" {
			t.Errorf("name = %q, want MEGA DRIVE", got.Name)
		}
	})

	t.Run("PLT-018/nome de outra plataforma devolve ErrPlatformNameTaken e nada muda", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		platAdd(t, s, "Mega Drive", domain.CategoryConsole)
		other := platAdd(t, s, "Dreamcast", domain.CategoryConsole)

		for _, name := range []string{"mega drive", "MEGA DRIVE", "Nintendo Switch", "nintendo switch"} {
			_, err := s.UpdatePlatform(t.Context(), other.ID, name, domain.CategoryArcade)
			if !errors.Is(err, store.ErrPlatformNameTaken) {
				t.Errorf("UpdatePlatform(%q) err = %v, want ErrPlatformNameTaken", name, err)
			}
		}
		if now := platByID(t, s, other.ID); now.Name != "Dreamcast" || now.Category != domain.CategoryConsole {
			t.Errorf("platform = %+v, want unchanged Dreamcast / console", now)
		}
	})

	t.Run("PLT-018/renomear para nome canônico de chave livre não atribui a chave", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		platDelete(t, s, platSeedID(t, s, "n64"))
		p := platAdd(t, s, "Meu Console", domain.CategoryConsole)

		got, err := s.UpdatePlatform(t.Context(), p.ID, "Nintendo 64", domain.CategoryConsole)
		if err != nil {
			t.Fatalf("UpdatePlatform: %v", err)
		}
		if got.SeedKey != "" || platByID(t, s, p.ID).SeedKey != "" {
			t.Errorf("SeedKey = %q, want empty: only the creation assigns a key", got.SeedKey)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM platform WHERE seed_key = 'n64'`); n != 0 {
			t.Errorf("platforms with key n64 = %d, want 0", n)
		}
	})

	t.Run("PLT-018/id inexistente é ErrPlatformNotFound e nome inválido é recusado sem mudar nada", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		p := platAdd(t, s, "Mega Drive", domain.CategoryConsole)

		if _, err := s.UpdatePlatform(t.Context(), 999999, "X", domain.CategoryPC); !errors.Is(err, store.ErrPlatformNotFound) {
			t.Errorf("UpdatePlatform(missing) err = %v, want ErrPlatformNotFound", err)
		}
		for _, name := range []string{"", "   ", strings.Repeat("é", domain.MaxNameRunes+1)} {
			_, err := s.UpdatePlatform(t.Context(), p.ID, name, domain.CategoryConsole)
			if err == nil || errors.Is(err, store.ErrPlatformNameTaken) || errors.Is(err, store.ErrPlatformNotFound) {
				t.Errorf("UpdatePlatform(%q) err = %v, want a name validation error", name, err)
			}
		}
		if now := platByID(t, s, p.ID); now.Name != "Mega Drive" {
			t.Errorf("Name = %q, want unchanged", now.Name)
		}
	})

	t.Run("PLT-019/UpdatePlatform grava a categoria", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		p := platAdd(t, s, "Mega Drive", domain.CategoryConsole)

		for _, cat := range []domain.PlatformCategory{domain.CategoryPC, domain.CategoryArcade, domain.CategoryHandheld, domain.CategoryConsole} {
			got, err := s.UpdatePlatform(t.Context(), p.ID, "Mega Drive", cat)
			if err != nil {
				t.Fatalf("UpdatePlatform(%q): %v", cat, err)
			}
			if got.Category != cat || platByID(t, s, p.ID).Category != cat {
				t.Errorf("Category = %q, want %q", got.Category, cat)
			}
		}
		if _, err := s.UpdatePlatform(t.Context(), p.ID, "Mega Drive", domain.PlatformCategory("laptop")); err == nil {
			t.Errorf("UpdatePlatform(laptop) err = nil, want an invalid category error")
		}
		if got := platByID(t, s, p.ID).Category; got != domain.CategoryConsole {
			t.Errorf("Category = %q, want unchanged console", got)
		}
	})

	t.Run("PLT-013/SetPlatformIcon devolve o ícone anterior; PlatformIconInUse com uma e com duas plataformas", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		a := platAdd(t, s, "Mega Drive", domain.CategoryConsole)
		b := platAdd(t, s, "Dreamcast", domain.CategoryConsole)
		const iconA, iconB = `platforms\a.png`, `platforms\b.png`

		p, prev, err := s.SetPlatformIcon(t.Context(), a.ID, iconA)
		if err != nil || prev != "" || p.Icon != iconA {
			t.Fatalf("SetPlatformIcon(a) = %+v, %q, %v; want icon %q, previous empty", p, prev, err, iconA)
		}
		p, prev, err = s.SetPlatformIcon(t.Context(), a.ID, iconB)
		if err != nil || prev != iconA || p.Icon != iconB {
			t.Fatalf("SetPlatformIcon(b) = %+v, %q, %v; want icon %q, previous %q", p, prev, err, iconB, iconA)
		}
		if used, err := s.PlatformIconInUse(t.Context(), iconA); err != nil || used {
			t.Errorf("PlatformIconInUse(a) = %v, %v; want false, nil", used, err)
		}
		if used, err := s.PlatformIconInUse(t.Context(), iconB); err != nil || !used {
			t.Errorf("PlatformIconInUse(b) with one platform = %v, %v; want true, nil", used, err)
		}

		if _, _, err := s.SetPlatformIcon(t.Context(), b.ID, iconB); err != nil {
			t.Fatalf("SetPlatformIcon(second platform): %v", err)
		}
		if _, _, err := s.SetPlatformIcon(t.Context(), a.ID, ""); err != nil {
			t.Fatalf("SetPlatformIcon(clear): %v", err)
		}
		if got := platByID(t, s, a.ID).Icon; got != "" {
			t.Errorf("Icon after clearing = %q, want empty", got)
		}
		if used, _ := s.PlatformIconInUse(t.Context(), iconB); !used {
			t.Errorf("PlatformIconInUse(b) with the second platform = false, want true")
		}
		if _, _, err := s.SetPlatformIcon(t.Context(), b.ID, ""); err != nil {
			t.Fatalf("SetPlatformIcon(clear second): %v", err)
		}
		if used, _ := s.PlatformIconInUse(t.Context(), iconB); used {
			t.Errorf("PlatformIconInUse(b) with no platform = true, want false")
		}
		if _, _, err := s.SetPlatformIcon(t.Context(), 999999, iconA); !errors.Is(err, store.ErrPlatformNotFound) {
			t.Errorf("SetPlatformIcon(missing) err = %v, want ErrPlatformNotFound", err)
		}
	})

	t.Run("PLT-020/DeletePlatform sem jogos apaga e devolve o ícone; emuladores ficam", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		p := platAdd(t, s, "Mega Drive", domain.CategoryConsole)
		const icon = `platforms\a.png`
		if _, _, err := s.SetPlatformIcon(t.Context(), p.ID, icon); err != nil {
			t.Fatalf("SetPlatformIcon: %v", err)
		}
		emu, err := s.AddEmulator(t.Context(), "Emu", filepath.Join(t.TempDir(), "e.exe"), "{rom}")
		if err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		emuID := emu.ID
		if err := s.SetPlatformEmulator(t.Context(), p.ID, &emuID); err != nil {
			t.Fatalf("SetPlatformEmulator: %v", err)
		}
		before := platCount(t, s)

		got, err := s.DeletePlatform(t.Context(), p.ID)
		if err != nil {
			t.Fatalf("DeletePlatform: %v", err)
		}
		if got != icon {
			t.Errorf("icon = %q, want %q", got, icon)
		}
		if _, err := s.PlatformByID(t.Context(), p.ID); !errors.Is(err, store.ErrPlatformNotFound) {
			t.Errorf("PlatformByID after delete err = %v, want ErrPlatformNotFound", err)
		}
		if after := platCount(t, s); after != before-1 {
			t.Errorf("platforms = %d, want %d", after, before-1)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM emulator WHERE id = ?`, emuID); n != 1 {
			t.Errorf("emulator rows = %d, want 1 (emulators stay)", n)
		}

		// Without an icon the result is empty; a missing id is ErrPlatformNotFound.
		q := platAdd(t, s, "Dreamcast", domain.CategoryConsole)
		if got, err := s.DeletePlatform(t.Context(), q.ID); err != nil || got != "" {
			t.Errorf("DeletePlatform(no icon) = %q, %v; want empty, nil", got, err)
		}
		if _, err := s.DeletePlatform(t.Context(), 999999); !errors.Is(err, store.ErrPlatformNotFound) {
			t.Errorf("DeletePlatform(missing) err = %v, want ErrPlatformNotFound", err)
		}
	})

	t.Run("PLT-021/DeletePlatform com jogo oculto devolve PlatformInUseError{GameCount: N} e nada é apagado", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		p := platAdd(t, s, "Mega Drive", domain.CategoryConsole)
		const icon = `platforms\a.png`
		if _, _, err := s.SetPlatformIcon(t.Context(), p.ID, icon); err != nil {
			t.Fatalf("SetPlatformIcon: %v", err)
		}
		platGame(t, s, p.ID, "Sonic", false)
		platGame(t, s, p.ID, "Streets of Rage", true)
		before := platCount(t, s)

		got, err := s.DeletePlatform(t.Context(), p.ID)
		if got != "" {
			t.Errorf("icon = %q, want empty on failure", got)
		}
		if !errors.Is(err, store.ErrPlatformInUse) {
			t.Fatalf("err = %v, want errors.Is ErrPlatformInUse", err)
		}
		var inUse *store.PlatformInUseError
		if !errors.As(err, &inUse) {
			t.Fatalf("err = %v, want *store.PlatformInUseError", err)
		}
		if inUse.PlatformID != p.ID || inUse.GameCount != 2 {
			t.Errorf("PlatformInUseError = %+v, want PlatformID %d and GameCount 2 (hidden games count)", *inUse, p.ID)
		}
		if after := platCount(t, s); after != before {
			t.Errorf("platforms = %d, want %d", after, before)
		}
		if now := platByID(t, s, p.ID); now.Icon != icon || now.GameCount != 2 {
			t.Errorf("platform = %+v, want unchanged (icon %q, 2 games)", now, icon)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM game WHERE platform_id = ?`, p.ID); n != 2 {
			t.Errorf("games = %d, want 2", n)
		}
	})

	t.Run("PLT-027/AddGame vai para a plataforma de chave windows renomeada", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		win := platSeedID(t, s, "windows")
		if _, err := s.UpdatePlatform(t.Context(), win, "PC", domain.CategoryPC); err != nil {
			t.Fatalf("UpdatePlatform: %v", err)
		}
		before := platCount(t, s)

		g := mustAddGame(t, s, store.NewGame{Title: "Hades", Exe: `C:\Games\Hades\Hades.exe`, At: addAt})
		if g.PlatformID != win || g.Platform != "PC" {
			t.Errorf("game platform = %d %q, want %d PC", g.PlatformID, g.Platform, win)
		}
		if r := readGameRow(t, s.DB(), g.ID); r.PlatformID != win {
			t.Errorf("game.platform_id = %d, want %d", r.PlatformID, win)
		}
		if after := platCount(t, s); after != before {
			t.Errorf("platforms = %d, want %d (none created)", after, before)
		}
	})

	t.Run("PLT-027/AddGame recria Windows (pc, windows) quando ela foi excluída", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		platDelete(t, s, platSeedID(t, s, "windows"))

		g := mustAddGame(t, s, store.NewGame{Title: "Hades", Exe: `C:\Games\Hades\Hades.exe`, At: addAt})
		var name, cat string
		if err := s.DB().QueryRowContext(t.Context(), `SELECT name, category FROM platform WHERE seed_key = 'windows'`).Scan(&name, &cat); err != nil {
			t.Fatalf("platform of key windows: %v", err)
		}
		if name != "Windows" || cat != "pc" {
			t.Errorf("recreated platform = %q / %q, want Windows / pc", name, cat)
		}
		if r := readGameRow(t, s.DB(), g.ID); r.PlatformID != platSeedID(t, s, "windows") {
			t.Errorf("game.platform_id = %d, want the recreated platform", r.PlatformID)
		}
	})

	t.Run("PLT-027/AddGame grava a chave numa Windows sem chave inserida à mão", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		platDelete(t, s, platSeedID(t, s, "windows"))
		libExec(t, s.DB(), `INSERT INTO platform (name, category) VALUES ('Windows', 'pc')`)
		manual := libLookup(t, s, "platform", "Windows")

		g := mustAddGame(t, s, store.NewGame{Title: "Hades", Exe: `C:\Games\Hades\Hades.exe`, At: addAt})
		if g.PlatformID != manual {
			t.Errorf("game platform = %d, want the hand-made Windows %d", g.PlatformID, manual)
		}
		if got := platByID(t, s, manual).SeedKey; got != "windows" {
			t.Errorf("SeedKey = %q, want windows (the key is written on it)", got)
		}
		if n := countRows(t, s.DB(), `SELECT COUNT(*) FROM platform WHERE name = 'Windows' COLLATE NOCASE`); n != 1 {
			t.Errorf("platforms named Windows = %d, want 1", n)
		}
	})

	t.Run("PLT-027/AddGame vai para a de chave windows mesmo existindo outra chamada Windows", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		win := platSeedID(t, s, "windows")
		if _, err := s.UpdatePlatform(t.Context(), win, "PC", domain.CategoryPC); err != nil {
			t.Fatalf("UpdatePlatform: %v", err)
		}
		libExec(t, s.DB(), `INSERT INTO platform (name, category) VALUES ('Windows', 'pc')`)
		impostor := libLookup(t, s, "platform", "Windows")

		g := mustAddGame(t, s, store.NewGame{Title: "Hades", Exe: `C:\Games\Hades\Hades.exe`, At: addAt})
		if g.PlatformID != win {
			t.Errorf("game platform = %d, want the one of key windows (%d), not the one named Windows (%d)", g.PlatformID, win, impostor)
		}
		if now := platByID(t, s, impostor); now.SeedKey != "" || now.GameCount != 0 {
			t.Errorf("platform named Windows = %+v, want no key and no games", now)
		}
	})

	t.Run("PLT-026/EnsureWindowsPlatformID devolve a de chave windows renomeada mesmo existindo outra chamada Windows", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		win := platSeedID(t, s, "windows")
		if _, err := s.UpdatePlatform(t.Context(), win, "PC", domain.CategoryPC); err != nil {
			t.Fatalf("UpdatePlatform: %v", err)
		}
		libExec(t, s.DB(), `INSERT INTO platform (name, category) VALUES ('Windows', 'pc')`)
		before := platCount(t, s)

		got, err := s.EnsureWindowsPlatformID(t.Context())
		if err != nil {
			t.Fatalf("EnsureWindowsPlatformID: %v", err)
		}
		if got != win {
			t.Errorf("id = %d, want %d (key windows)", got, win)
		}
		if after := platCount(t, s); after != before {
			t.Errorf("platforms = %d, want %d", after, before)
		}
	})

	t.Run("PLT-026/EnsureWindowsPlatformID recria Windows (pc, windows) quando ela foi excluída", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		platDelete(t, s, platSeedID(t, s, "windows"))

		id, err := s.EnsureWindowsPlatformID(t.Context())
		if err != nil {
			t.Fatalf("EnsureWindowsPlatformID: %v", err)
		}
		p := platByID(t, s, id)
		if p.Name != "Windows" || p.Category != domain.CategoryPC || p.SeedKey != "windows" {
			t.Errorf("platform = %+v, want Windows / pc / windows", p)
		}
		again, err := s.EnsureWindowsPlatformID(t.Context())
		if err != nil || again != id {
			t.Errorf("second call = %d, %v; want %d, nil", again, err, id)
		}
	})

	t.Run("PLT-026/WindowsPlatformID não cria quando a chave windows não existe", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		win := platSeedID(t, s, "windows")
		if id, found, err := s.WindowsPlatformID(t.Context()); err != nil || !found || id != win {
			t.Errorf("WindowsPlatformID = %d, %v, %v; want %d, true, nil", id, found, err, win)
		}

		platDelete(t, s, win)
		before := platCount(t, s)
		id, found, err := s.WindowsPlatformID(t.Context())
		if err != nil || found || id != 0 {
			t.Errorf("WindowsPlatformID after delete = %d, %v, %v; want 0, false, nil", id, found, err)
		}
		if after := platCount(t, s); after != before {
			t.Errorf("platforms = %d, want %d: it never creates", after, before)
		}
	})

	t.Run("PLT-014/PlatformByID traz GameCount", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		p := platAdd(t, s, "Mega Drive", domain.CategoryConsole)
		if got := platByID(t, s, p.ID); got.GameCount != 0 {
			t.Errorf("GameCount = %d, want 0", got.GameCount)
		}
		platGame(t, s, p.ID, "Sonic", false)
		platGame(t, s, p.ID, "Streets of Rage", true)

		got := platByID(t, s, p.ID)
		if got.GameCount != 2 || got.Name != "Mega Drive" || got.Category != domain.CategoryConsole {
			t.Errorf("platform = %+v, want Mega Drive / console with GameCount 2 (hidden included)", got)
		}
		if _, err := s.PlatformByID(t.Context(), 999999); !errors.Is(err, store.ErrPlatformNotFound) {
			t.Errorf("PlatformByID(missing) err = %v, want ErrPlatformNotFound", err)
		}
	})
}
