package store_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// requireStrings checks a list field: it must be non-nil and equal to want.
func requireStrings(t *testing.T, field string, got, want []string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = nil, want a non-nil slice", field)
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", field, got, want)
	}
}

func TestGameDetail(t *testing.T) {
	t.Parallel()

	t.Run("STO-026/detalhe traz os campos de Game, descrição, data de conclusão, vínculos e modos", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		ids := seedLibrary(t, s, libGame{
			Title: "Hollow Knight", Release: "2017-02-24", Status: "completed", Favorite: true,
			Seconds: 3600, Plays: 2, FirstPlayed: libAt(1, 20), LastPlayed: libAt(9, 21), Completed: libAt(12, 0),
			Description: "Metroidvania", Genres: []string{"Ação"}, Modes: []string{"single"},
			Developers: []string{"Team Cherry"}, Publishers: []string{"Team Cherry"},
		})
		id := ids["Hollow Knight"]

		d, err := s.GameDetail(t.Context(), id)
		if err != nil {
			t.Fatalf("GameDetail(%d): %v", id, err)
		}
		if d.ID != id || d.Title != "Hollow Knight" {
			t.Errorf("ID/Title = %d/%q, want %d/Hollow Knight", d.ID, d.Title, id)
		}
		if d.LaunchType != "exe" || d.LaunchTarget != `C:\Games\Hollow Knight.exe` || d.LaunchArgs != "" || d.WorkingDir != "" {
			t.Errorf("launch = %q %q %q %q, want exe, C:\\Games\\Hollow Knight.exe, '', ''",
				d.LaunchType, d.LaunchTarget, d.LaunchArgs, d.WorkingDir)
		}
		if d.TotalPlaySeconds != 3600 || d.PlayCount != 2 {
			t.Errorf("TotalPlaySeconds/PlayCount = %d/%d, want 3600/2", d.TotalPlaySeconds, d.PlayCount)
		}
		if !d.FirstPlayedAt.Equal(time.UnixMilli(libAt(1, 20))) || !d.LastPlayedAt.Equal(time.UnixMilli(libAt(9, 21))) {
			t.Errorf("FirstPlayedAt/LastPlayedAt = %v/%v, want the stored instants", d.FirstPlayedAt, d.LastPlayedAt)
		}
		if want := libLookup(t, s, "platform", "PC"); d.PlatformID != want || d.Platform != "PC" {
			t.Errorf("PlatformID/Platform = %d/%q, want %d/PC", d.PlatformID, d.Platform, want)
		}
		if d.Status != domain.StatusCompleted {
			t.Errorf("Status = %q, want %q", d.Status, domain.StatusCompleted)
		}
		if !d.Favorite {
			t.Errorf("Favorite = false, want true")
		}
		if d.ReleaseDate != "2017-02-24" {
			t.Errorf("ReleaseDate = %q, want 2017-02-24", d.ReleaseDate)
		}
		if d.Description != "Metroidvania" {
			t.Errorf("Description = %q, want Metroidvania", d.Description)
		}
		if want := time.UnixMilli(libAt(12, 0)); !d.CompletedAt.Equal(want) {
			t.Errorf("CompletedAt = %v, want %v", d.CompletedAt, want)
		}
		requireStrings(t, "Developers", d.Developers, []string{"Team Cherry"})
		requireStrings(t, "Publishers", d.Publishers, []string{"Team Cherry"})
		requireStrings(t, "Genres", d.Genres, []string{"Ação"})
		requireStrings(t, "PlayModes", d.PlayModes, []string{"single"})
	})

	t.Run("STO-026/listas sem nada são [] e nunca nil", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{Title: "Vazio"})["Vazio"]

		d, err := s.GameDetail(t.Context(), id)
		if err != nil {
			t.Fatalf("GameDetail(%d): %v", id, err)
		}
		requireStrings(t, "Developers", d.Developers, []string{})
		requireStrings(t, "Publishers", d.Publishers, []string{})
		requireStrings(t, "Genres", d.Genres, []string{})
		requireStrings(t, "PlayModes", d.PlayModes, []string{})
		if !d.CompletedAt.IsZero() {
			t.Errorf("CompletedAt = %v, want zero (NULL)", d.CompletedAt)
		}
		if !d.FirstPlayedAt.IsZero() || !d.LastPlayedAt.IsZero() {
			t.Errorf("FirstPlayedAt/LastPlayedAt = %v/%v, want zero", d.FirstPlayedAt, d.LastPlayedAt)
		}
		if d.ReleaseDate != "" {
			t.Errorf("ReleaseDate = %q, want empty", d.ReleaseDate)
		}
	})

	t.Run("STO-026/nomes de gênero, desenvolvedora, editora e modo em ordem alfabética sem diferenciar maiúsculas", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		id := seedLibrary(t, s, libGame{
			Title:      "Ordem",
			Genres:     []string{"Beta", "alpha", "Gamma"},
			Developers: []string{"Zeta Studio", "alpha Team"},
			Publishers: []string{"bravo", "Alpha Pub"},
			Modes:      []string{"single", "coop_online", "multi_local"},
		})["Ordem"]

		d, err := s.GameDetail(t.Context(), id)
		if err != nil {
			t.Fatalf("GameDetail(%d): %v", id, err)
		}
		requireStrings(t, "Genres", d.Genres, []string{"alpha", "Beta", "Gamma"})
		requireStrings(t, "Developers", d.Developers, []string{"alpha Team", "Zeta Studio"})
		requireStrings(t, "Publishers", d.Publishers, []string{"Alpha Pub", "bravo"})
		requireStrings(t, "PlayModes", d.PlayModes, []string{"coop_online", "multi_local", "single"})
	})

	t.Run("STO-026/id inexistente devolve erro que embrulha ErrGameNotFound", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		seedLibrary(t, s, libGame{Title: "Existe"})
		for _, id := range []int64{999, -1, 0} {
			_, err := s.GameDetail(t.Context(), id)
			if !errors.Is(err, store.ErrGameNotFound) {
				t.Errorf("GameDetail(%d) err = %v, want errors.Is(err, store.ErrGameNotFound)", id, err)
			}
		}
	})
}

// Added 2026-11-25 (fase 16): GameDetail.SessionPlaySeconds (SED-034).
func TestGameDetailSessionPlaySeconds(t *testing.T) {
	t.Parallel()

	seed := func(t *testing.T) (*store.Store, int64) {
		t.Helper()
		s := openStore(t, dbPath(t), true)
		return s, seedLibrary(t, s, libGame{Title: "Hades", Seconds: 7200})["Hades"]
	}

	t.Run("SED-034/GameDetail traz SessionPlaySeconds = soma de active_seconds das sessões ended", func(t *testing.T) {
		t.Parallel()
		s, g := seed(t)
		libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(1, 10), Ended: libAt(1, 11), Active: 3600, State: "ended"})
		libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(2, 10), Ended: libAt(2, 11), Active: 1800, State: "ended"})
		d, err := s.GameDetail(t.Context(), g)
		if err != nil {
			t.Fatalf("GameDetail: %v", err)
		}
		if d.SessionPlaySeconds != 5400 {
			t.Errorf("SessionPlaySeconds = %d, want 5400", d.SessionPlaySeconds)
		}
	})

	t.Run("SED-034/descartada e aberta não entram na soma", func(t *testing.T) {
		t.Parallel()
		s, g := seed(t)
		libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(1, 10), Ended: libAt(1, 11), Active: 100, State: "ended"})
		libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(2, 10), Ended: libAt(2, 11), Active: 20, State: "discarded"})
		libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(3, 10), Active: 999, State: "running"})
		d, err := s.GameDetail(t.Context(), g)
		if err != nil {
			t.Fatalf("GameDetail: %v", err)
		}
		if d.SessionPlaySeconds != 100 {
			t.Errorf("SessionPlaySeconds = %d, want 100", d.SessionPlaySeconds)
		}
	})

	t.Run("SED-034/jogo sem sessões tem 0", func(t *testing.T) {
		t.Parallel()
		s, g := seed(t)
		d, err := s.GameDetail(t.Context(), g)
		if err != nil {
			t.Fatalf("GameDetail: %v", err)
		}
		if d.SessionPlaySeconds != 0 {
			t.Errorf("SessionPlaySeconds = %d, want 0", d.SessionPlaySeconds)
		}
	})

	t.Run("SED-034/total ajustado à mão (SetTotalPlaySeconds) não muda SessionPlaySeconds", func(t *testing.T) {
		t.Parallel()
		s, g := seed(t)
		libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(1, 10), Ended: libAt(1, 11), Active: 3600, State: "ended"})
		d, err := s.SetTotalPlaySeconds(t.Context(), g, 50000, time.UnixMilli(libAt(5, 0)))
		if err != nil {
			t.Fatalf("SetTotalPlaySeconds: %v", err)
		}
		if d.TotalPlaySeconds != 50000 || d.SessionPlaySeconds != 3600 {
			t.Errorf("Total/SessionPlaySeconds = %d/%d, want 50000/3600", d.TotalPlaySeconds, d.SessionPlaySeconds)
		}
		again, err := s.GameDetail(t.Context(), g)
		if err != nil {
			t.Fatalf("GameDetail: %v", err)
		}
		if again.SessionPlaySeconds != 3600 {
			t.Errorf("SessionPlaySeconds = %d, want 3600", again.SessionPlaySeconds)
		}
	})

	t.Run("SED-034/UpdateGame e SetTotalPlaySeconds também devolvem SessionPlaySeconds", func(t *testing.T) {
		t.Parallel()
		s, g := seed(t)
		libInsertSession(t, s.DB(), libSessionRow{GameID: g, Started: libAt(1, 10), Ended: libAt(1, 11), Active: 777, State: "ended"})
		title := "Hades II"
		d, err := s.UpdateGame(t.Context(), g, store.GamePatch{Title: &title}, time.UnixMilli(libAt(5, 0)))
		if err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		if d.SessionPlaySeconds != 777 {
			t.Errorf("UpdateGame SessionPlaySeconds = %d, want 777", d.SessionPlaySeconds)
		}
		d, err = s.SetTotalPlaySeconds(t.Context(), g, 10, time.UnixMilli(libAt(5, 1)))
		if err != nil {
			t.Fatalf("SetTotalPlaySeconds: %v", err)
		}
		if d.SessionPlaySeconds != 777 {
			t.Errorf("SetTotalPlaySeconds SessionPlaySeconds = %d, want 777", d.SessionPlaySeconds)
		}
	})
}

func TestAddGameFields(t *testing.T) {
	t.Parallel()

	t.Run("GAM-001/PLT-027/AddGame devolve Platform Windows, Status not_played, Favorite false e ReleaseDate vazio", func(t *testing.T) {
		t.Parallel()
		s := openStore(t, dbPath(t), true)
		g := mustAddGame(t, s, store.NewGame{Title: "Hades", Exe: `C:\Games\Hades\Hades.exe`, WorkingDir: `C:\Games\Hades`, At: addAt})

		if g.Platform != "Windows" {
			t.Errorf("Platform = %q, want Windows", g.Platform)
		}
		if want := libLookup(t, s, "platform", "Windows"); g.PlatformID != want {
			t.Errorf("PlatformID = %d, want %d (Windows)", g.PlatformID, want)
		}
		if g.Status != domain.StatusNotPlayed {
			t.Errorf("Status = %q, want %q", g.Status, domain.StatusNotPlayed)
		}
		if g.Favorite {
			t.Errorf("Favorite = true, want false")
		}
		if g.ReleaseDate != "" {
			t.Errorf("ReleaseDate = %q, want empty", g.ReleaseDate)
		}
	})
}
