package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// Messages of EMU-005 shown to the user when a game cannot start with its emulator.
const (
	msgNoEmulator       = "Não há emulador configurado para esta plataforma."
	msgEmulatorNotFound = "O emulador não foi encontrado em %s."
)

// emulatorSpec builds the launch of a game that runs in an emulator (EMU-004): the
// emulator's executable, with its arguments where {rom} is the game's launch target
// and {dir} the folder of that ROM. The emulator runs in a Job Object like any exe,
// so tracking, pause and grace do not change (EMU-004).
func emulatorSpec(ctx context.Context, st gameStore, g store.Game, base tracker.LaunchSpec) (tracker.LaunchSpec, error) {
	emu, err := st.ResolveEmulator(ctx, g.ID)
	if err != nil {
		if errors.Is(err, store.ErrNoEmulator) || errors.Is(err, store.ErrEmulatorNotFound) {
			return tracker.LaunchSpec{}, ipc.NewErrorData(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed,
				ipc.LaunchFailedData{Error: msgNoEmulator})
		}
		return tracker.LaunchSpec{}, fmt.Errorf("game.launch: emulator: %w", err)
	}
	if _, err := os.Stat(emu.ExePath); err != nil {
		return tracker.LaunchSpec{}, ipc.NewErrorData(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed,
			ipc.LaunchFailedData{Error: fmt.Sprintf(msgEmulatorNotFound, emu.ExePath)})
	}
	base.Exe = emu.ExePath
	base.Args = domain.BuildArgs(emu.ArgsTemplate, g.LaunchTarget, filepath.Dir(g.LaunchTarget))
	base.WorkDir = filepath.Dir(emu.ExePath)
	return base, nil
}
