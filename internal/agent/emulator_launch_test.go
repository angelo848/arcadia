package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// emulatorGame creates a game of launch type emulator whose ROM is romPath, in a
// store with the given platform emulator (or none). It returns the store-backed fake.
func emulatorGame(t *testing.T, romPath string, platformEmu *store.Emulator) (*recordingGameStore, int64) {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "library.db"), store.OpenOptions{Migrate: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	pc, err := st.AddPlatform(t.Context(), "PC", domain.PlatformCategory("pc"))
	if err != nil {
		t.Fatalf("AddPlatform: %v", err)
	}
	if platformEmu != nil {
		emu, err := st.AddEmulator(t.Context(), platformEmu.Name, platformEmu.ExePath, platformEmu.ArgsTemplate)
		if err != nil {
			t.Fatalf("AddEmulator: %v", err)
		}
		id := emu.ID
		if err := st.SetPlatformEmulator(t.Context(), pc.ID, &id); err != nil {
			t.Fatalf("SetPlatformEmulator: %v", err)
		}
	}
	g, err := st.AddGame(t.Context(), store.NewGame{Title: "Jogo", Exe: romPath, At: testClock().Now()})
	if err != nil {
		t.Fatalf("AddGame: %v", err)
	}
	lt, target := domain.LaunchTypeEmulator, romPath
	if _, err := st.UpdateGame(t.Context(), g.ID, store.GamePatch{LaunchType: &lt, LaunchTarget: &target}, testClock().Now()); err != nil {
		t.Fatalf("UpdateGame: %v", err)
	}
	return &recordingGameStore{inner: st}, g.ID
}

// launchParams is the game.launch params of one game.
func launchParams(t *testing.T, id int64) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]int64{"gameId": id})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestEmulatorLaunch(t *testing.T) {
	t.Parallel()

	t.Run("EMU-004/lança o emulador com os argumentos do modelo", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		emuExe := filepath.Join(dir, "emu", "snes9x.exe")
		if err := os.MkdirAll(filepath.Dir(emuExe), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(emuExe, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		rom := filepath.Join(dir, "roms", "Super Mario.sfc")
		emu := store.Emulator{ID: 1, Name: "SNES9x", ExePath: emuExe, ArgsTemplate: "-f {rom} -d {dir}"}
		gs, id := emulatorGame(t, rom, &emu)
		_, spec, err := prepareLaunch(t.Context(), gs, launchParams(t, id), quietLog())
		if err != nil {
			t.Fatalf("prepareLaunch: %v", err)
		}
		if spec.Exe != emuExe {
			t.Errorf("Exe = %q, want the emulator %q", spec.Exe, emuExe)
		}
		want := domain.BuildArgs("-f {rom} -d {dir}", rom, filepath.Dir(rom))
		if spec.Args != want {
			t.Errorf("Args = %q, want %q", spec.Args, want)
		}
		if spec.WorkDir != filepath.Dir(emuExe) {
			t.Errorf("WorkDir = %q, want the emulator folder", spec.WorkDir)
		}
	})

	t.Run("EMU-005/caminho do emulador inexistente é erro com o caminho", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "sumiu", "emu.exe")
		emu := store.Emulator{ID: 1, Name: "Sumiu", ExePath: missing, ArgsTemplate: "{rom}"}
		gs, id := emulatorGame(t, filepath.Join(t.TempDir(), "g.sfc"), &emu)
		_, _, err := prepareLaunch(t.Context(), gs, launchParams(t, id), quietLog())
		var rpcErr *ipc.Error
		escaped, _ := json.Marshal(missing) // the data is JSON, so the path is escaped in it
		if !errors.As(err, &rpcErr) || !strings.Contains(string(rpcErr.Data), string(escaped[1:len(escaped)-1])) {
			t.Errorf("err = %v, want a launch error naming %q", err, missing)
		}
	})

	t.Run("EMU-005/sem emulador não cria sessão e explica o motivo", func(t *testing.T) {
		t.Parallel()
		gs, id := emulatorGame(t, filepath.Join(t.TempDir(), "g.sfc"), nil)
		_, _, err := prepareLaunch(t.Context(), gs, launchParams(t, id), quietLog())
		var rpcErr *ipc.Error
		if !errors.As(err, &rpcErr) || !strings.Contains(string(rpcErr.Data), "Não há emulador configurado") {
			t.Errorf("err = %v, want the no-emulator message", err)
		}
	})
}
