package store

import (
	"errors"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestUpdateGameLaunch(t *testing.T) {
	t.Parallel()

	t.Run("EDT-010/tipo, alvo e argumentos são gravados e lidos de volta", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g, err := s.AddGame(t.Context(), NewGame{Title: "Jogo", Exe: `C:\Jogos\j.exe`, At: importAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		lt, target, args := "exe", `C:\Jogos\novo.exe`, "-windowed"
		d, err := s.UpdateGame(t.Context(), g.ID, GamePatch{LaunchType: &lt, LaunchTarget: &target, LaunchArgs: &args}, importAt)
		if err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		if d.LaunchType != "exe" || d.LaunchTarget != `C:\Jogos\novo.exe` || d.LaunchArgs != "-windowed" {
			t.Errorf("detail = %q %q %q", d.LaunchType, d.LaunchTarget, d.LaunchArgs)
		}
	})

	t.Run("EDT-010/editar só o título não apaga o alvo nem os argumentos", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g, err := s.AddGame(t.Context(), NewGame{Title: "Jogo", Exe: `C:\Jogos\j.exe`, At: importAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		args := "-x"
		if _, err := s.UpdateGame(t.Context(), g.ID, GamePatch{LaunchArgs: &args}, importAt); err != nil {
			t.Fatalf("set args: %v", err)
		}
		title := "Outro título"
		d, err := s.UpdateGame(t.Context(), g.ID, GamePatch{Title: &title}, importAt)
		if err != nil {
			t.Fatalf("UpdateGame title: %v", err)
		}
		if d.LaunchTarget != `C:\Jogos\j.exe` || d.LaunchArgs != "-x" {
			t.Errorf("launch changed by a title edit: %q %q", d.LaunchTarget, d.LaunchArgs)
		}
	})

	t.Run("EDT-010/alvo inválido não grava nada", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g, err := s.AddGame(t.Context(), NewGame{Title: "Jogo", Exe: `C:\Jogos\j.exe`, At: importAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		target := "relativo.exe"
		_, err = s.UpdateGame(t.Context(), g.ID, GamePatch{LaunchTarget: &target}, importAt)
		if !errors.Is(err, domain.ErrInvalidField) {
			t.Fatalf("err = %v, want ErrInvalidField", err)
		}
		d, err := s.GameDetail(t.Context(), g.ID)
		if err != nil {
			t.Fatalf("GameDetail: %v", err)
		}
		if d.LaunchTarget != `C:\Jogos\j.exe` {
			t.Errorf("target changed to %q after a rejected edit", d.LaunchTarget)
		}
	})

	t.Run("EDT-010/editar o lançamento não toca no tempo jogado", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g, err := s.AddGame(t.Context(), NewGame{Title: "Jogo", Exe: `C:\Jogos\j.exe`, At: importAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		lt, target := "uri", "steam://rungameid/9"
		if _, err := s.UpdateGame(t.Context(), g.ID, GamePatch{LaunchType: &lt, LaunchTarget: &target}, time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)); err != nil {
			t.Fatalf("UpdateGame: %v", err)
		}
		var total int64
		if err := s.DB().QueryRowContext(t.Context(), `SELECT total_play_seconds FROM game WHERE id = ?`, g.ID).Scan(&total); err != nil {
			t.Fatalf("read total: %v", err)
		}
		if total != 0 {
			t.Errorf("total_play_seconds = %d after a launch edit, want 0", total)
		}
	})
}
