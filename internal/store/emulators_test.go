package store

import (
	"errors"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestEmulators(t *testing.T) {
	t.Parallel()

	t.Run("EMU-001/nome duplicado sem diferença de maiúsculas é recusado", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		if _, err := s.AddEmulator(t.Context(), "SNES9x", `C:\Emu\snes9x.exe`, "{rom}"); err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		_, err := s.AddEmulator(t.Context(), "snes9x", `C:\Emu\outro.exe`, "{rom}")
		if !errors.Is(err, ErrEmulatorNameTaken) {
			t.Fatalf("err = %v, want ErrEmulatorNameTaken", err)
		}
	})

	t.Run("EMU-001/caminho relativo do emulador é recusado", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		_, err := s.AddEmulator(t.Context(), "Emu", `snes9x.exe`, "")
		if !errors.Is(err, domain.ErrInvalidField) {
			t.Fatalf("err = %v, want ErrInvalidField", err)
		}
	})

	t.Run("EMU-001/marcador desconhecido no modelo é recusado", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		_, err := s.AddEmulator(t.Context(), "Emu", `C:\Emu\e.exe`, "-x {foo}")
		if !errors.Is(err, domain.ErrInvalidField) {
			t.Fatalf("err = %v, want ErrInvalidField", err)
		}
	})

	t.Run("EMU-002/apagar emulador zera as referências sem apagar jogos", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		emu, err := s.AddEmulator(t.Context(), "Emu", `C:\Emu\e.exe`, "{rom}")
		if err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		id := emu.ID
		if err := s.SetPlatformEmulator(t.Context(), pc, &id); err != nil {
			t.Fatalf("SetPlatformEmulator: %v", err)
		}
		g, err := s.AddGame(t.Context(), NewGame{Title: "Jogo", Exe: `C:\j.exe`, At: importAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		if err := s.SetGameEmulator(t.Context(), g.ID, &id, importAt); err != nil {
			t.Fatalf("SetGameEmulator: %v", err)
		}
		if err := s.DeleteEmulator(t.Context(), id); err != nil {
			t.Fatalf("DeleteEmulator: %v", err)
		}
		if n := count(t, s, `SELECT COUNT(*) FROM platform WHERE id = ? AND emulator_id IS NULL`, pc); n != 1 {
			t.Errorf("platform emulator not cleared")
		}
		if n := count(t, s, `SELECT COUNT(*) FROM game WHERE id = ? AND emulator_id IS NULL`, g.ID); n != 1 {
			t.Errorf("game emulator not cleared")
		}
		if n := count(t, s, `SELECT COUNT(*) FROM game`); n != 1 {
			t.Errorf("games = %d, want 1 (no game deleted)", n)
		}
	})

	t.Run("EMU-002/platform.setEmulator aponta só para emulador existente", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		bad := int64(999)
		if err := s.SetPlatformEmulator(t.Context(), pc, &bad); !errors.Is(err, ErrEmulatorNotFound) {
			t.Errorf("err = %v, want ErrEmulatorNotFound", err)
		}
	})

	t.Run("EMU-002/jogo com emulador próprio ganha do padrão da plataforma", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		pc := importPC(t, s)
		platformEmu, err := s.AddEmulator(t.Context(), "Padrão", `C:\Emu\padrao.exe`, "{rom}")
		if err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		gameEmu, err := s.AddEmulator(t.Context(), "Próprio", `C:\Emu\proprio.exe`, "-x {rom}")
		if err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		pid, gid := platformEmu.ID, gameEmu.ID
		if err := s.SetPlatformEmulator(t.Context(), pc, &pid); err != nil {
			t.Fatalf("SetPlatformEmulator: %v", err)
		}
		g, err := s.AddGame(t.Context(), NewGame{Title: "Jogo", Exe: `C:\j.exe`, At: importAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		got, err := s.ResolveEmulator(t.Context(), g.ID)
		if err != nil || got.ID != platformEmu.ID {
			t.Fatalf("resolve without override = %+v, %v; want the platform default", got, err)
		}
		if err := s.SetGameEmulator(t.Context(), g.ID, &gid, importAt); err != nil {
			t.Fatalf("SetGameEmulator: %v", err)
		}
		got, err = s.ResolveEmulator(t.Context(), g.ID)
		if err != nil || got.ID != gameEmu.ID {
			t.Errorf("resolve with override = %+v, %v; want the game's emulator", got, err)
		}
	})

	t.Run("EMU-005/sem emulador nenhum devolve ErrNoEmulator", func(t *testing.T) {
		t.Parallel()
		s := ftsOpen(t)
		g, err := s.AddGame(t.Context(), NewGame{Title: "Jogo", Exe: `C:\j.exe`, At: importAt})
		if err != nil {
			t.Fatalf("AddGame: %v", err)
		}
		if _, err := s.ResolveEmulator(t.Context(), g.ID); !errors.Is(err, ErrNoEmulator) {
			t.Errorf("err = %v, want ErrNoEmulator", err)
		}
	})
}
