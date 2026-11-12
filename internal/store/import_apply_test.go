package store

import (
	"errors"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// importPC returns the id of the PC platform, creating it when needed.
func importPC(t *testing.T, s *Store) int64 {
	t.Helper()
	id, err := s.EnsurePlatformID(t.Context(), "PC", domain.PlatformCategory("pc"))
	if err != nil {
		t.Fatalf("EnsurePlatformID: %v", err)
	}
	return id
}

// count runs a COUNT query with args and returns the value.
func count(t *testing.T, s *Store, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := s.DB().QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

var importAt = time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)

func TestApplyImport(t *testing.T) {
	t.Parallel()

	t.Run("IMP-006/mesmo launch_target não duplica", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		g := ImportGame{PlatformID: pc, Title: "Counter-Strike 2", LaunchType: "uri", LaunchTarget: "steam://rungameid/730"}
		if _, err := s.ApplyImport(t.Context(), g, importAt); err != nil {
			t.Fatalf("first ApplyImport: %v", err)
		}
		other := g
		other.Title = "Outro nome"
		_, err := s.ApplyImport(t.Context(), other, importAt)
		if !errors.Is(err, ErrDuplicate) {
			t.Fatalf("second ApplyImport err = %v, want ErrDuplicate", err)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM game`); n != 1 {
			t.Errorf("games = %d, want 1", n)
		}
	})

	t.Run("IMP-006/mesmo título e plataforma sem diferença de maiúsculas não duplica", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		if _, err := s.ApplyImport(t.Context(), ImportGame{PlatformID: pc, Title: "ABZU", LaunchType: "exe", LaunchTarget: `D:\a\ABZU.exe`}, importAt); err != nil {
			t.Fatalf("first: %v", err)
		}
		_, err := s.ApplyImport(t.Context(), ImportGame{PlatformID: pc, Title: "abzu", LaunchType: "exe", LaunchTarget: `E:\b\abzu.exe`}, importAt)
		if !errors.Is(err, ErrDuplicate) {
			t.Fatalf("err = %v, want ErrDuplicate", err)
		}
	})

	t.Run("IMP-007/tempo vira sessão source import com ended_at na última partida", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		last := time.Date(2025, time.January, 22, 3, 0, 8, 0, time.UTC)
		id, err := s.ApplyImport(t.Context(), ImportGame{
			PlatformID: pc, Title: "Counter-Strike 2", LaunchType: "uri", LaunchTarget: "steam://rungameid/730",
			Played: &ImportPlayed{Seconds: 33060, LastPlay: last},
		}, importAt)
		if err != nil {
			t.Fatalf("ApplyImport: %v", err)
		}
		var source string
		var endedAt, startedAt int64
		if err := s.DB().QueryRowContext(t.Context(),
			`SELECT source, ended_at, started_at FROM play_session WHERE game_id = ?`, id).Scan(&source, &endedAt, &startedAt); err != nil {
			t.Fatalf("read session: %v", err)
		}
		if source != "import" || endedAt != TimeToMillis(last) || startedAt != TimeToMillis(last.Add(-33060*time.Second)) {
			t.Errorf("session source=%q ended=%d started=%d", source, endedAt, startedAt)
		}
		var total int64
		if err := s.DB().QueryRowContext(t.Context(), `SELECT total_play_seconds FROM game WHERE id = ?`, id).Scan(&total); err != nil {
			t.Fatalf("read total: %v", err)
		}
		if total != 33060 {
			t.Errorf("total_play_seconds = %d, want 33060", total)
		}
	})

	t.Run("IMP-009/apply não cria sessão rastreada", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		if _, err := s.ApplyImport(t.Context(), ImportGame{
			PlatformID: pc, Title: "Jogo", LaunchType: "exe", LaunchTarget: `C:\j.exe`,
			Played: &ImportPlayed{Seconds: 60, LastPlay: importAt},
		}, importAt); err != nil {
			t.Fatalf("ApplyImport: %v", err)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM play_session WHERE source = 'tracked'`); n != 0 {
			t.Errorf("tracked sessions = %d, want 0", n)
		}
	})

	t.Run("RUL-001/install_dir gera regra install_dir com learned=0", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		id, err := s.ApplyImport(t.Context(), ImportGame{
			PlatformID: pc, Title: "Jogo Steam", LaunchType: "uri", LaunchTarget: "steam://rungameid/1",
			InstallDir: `C:\Steam\common\Jogo`,
		}, importAt)
		if err != nil {
			t.Fatalf("ApplyImport: %v", err)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM tracking_rule WHERE game_id = ? AND kind = 'install_dir' AND value = ? AND learned = 0`, id, `C:\Steam\common\Jogo`); n != 1 {
			t.Errorf("install_dir rules = %d, want 1 with learned=0", n)
		}
	})

	t.Run("RUL-001/exe gera regra exe_path e uri não gera regra de exe", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		exeID, err := s.ApplyImport(t.Context(), ImportGame{PlatformID: pc, Title: "Avulso", LaunchType: "exe", LaunchTarget: `D:\a.exe`}, importAt)
		if err != nil {
			t.Fatalf("exe: %v", err)
		}
		uriID, err := s.ApplyImport(t.Context(), ImportGame{PlatformID: pc, Title: "Epic", LaunchType: "uri", LaunchTarget: "com.epicgames.launcher://apps/x"}, importAt)
		if err != nil {
			t.Fatalf("uri: %v", err)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM tracking_rule WHERE game_id = ? AND kind = 'exe_path'`, exeID); n != 1 {
			t.Errorf("exe rules = %d, want 1", n)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM tracking_rule WHERE game_id = ?`, uriID); n != 0 {
			t.Errorf("uri rules = %d, want 0", n)
		}
	})

	t.Run("IMP-001/vínculos e índice de busca são gravados com o jogo", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		year := 2025
		id, err := s.ApplyImport(t.Context(), ImportGame{
			PlatformID: pc, Title: "GTA V", LaunchType: "uri", LaunchTarget: "uri://gta",
			Year: &year, Developers: []string{"Rockstar North"}, Genres: []string{"Racing", "Shooter"},
			PlayModes: []domain.PlayMode{domain.PlayModeSingle},
		}, importAt)
		if err != nil {
			t.Fatalf("ApplyImport: %v", err)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM game_genre WHERE game_id = ?`, id); n != 2 {
			t.Errorf("genres = %d, want 2", n)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM game_play_mode WHERE game_id = ?`, id); n != 1 {
			t.Errorf("play modes = %d, want 1", n)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM game WHERE id = ? AND release_date = '2025'`, id); n != 1 {
			t.Errorf("release_date not the year 2025")
		}
		if len(ftsMatchRowids(t, s, "Rockstar")) != 1 {
			t.Errorf("search index does not find the developer")
		}
	})

	t.Run("IMP-008/candidato inválido não grava nada e não afeta os demais", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		if _, err := s.ApplyImport(t.Context(), ImportGame{PlatformID: pc, Title: "Ruim", LaunchType: "xbox", LaunchTarget: "x"}, importAt); err == nil {
			t.Fatal("invalid launch type accepted")
		}
		if _, err := s.ApplyImport(t.Context(), ImportGame{PlatformID: pc, Title: "Bom", LaunchType: "exe", LaunchTarget: `C:\bom.exe`}, importAt); err != nil {
			t.Fatalf("good game after a failure: %v", err)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM game`); n != 1 {
			t.Errorf("games = %d, want only the good one", n)
		}
	})

	t.Run("RUL-006/regras importadas ficam com learned=0 (a heurística fica para depois)", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		id, err := s.ApplyImport(t.Context(), ImportGame{
			PlatformID: pc, Title: "Pasta", LaunchType: "exe", LaunchTarget: `D:\Pasta\p.exe`,
			InstallDir: `D:\Pasta`,
		}, importAt)
		if err != nil {
			t.Fatalf("ApplyImport: %v", err)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM tracking_rule WHERE game_id = ? AND learned <> 0`, id); n != 0 {
			t.Errorf("learned rules = %d, want 0", n)
		}
	})
}
