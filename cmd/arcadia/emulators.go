package main

import (
	"fmt"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// EmulatorView is an emulator as the UI sees it (EMU-001).
type EmulatorView struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ExePath      string `json:"exePath"`
	ArgsTemplate string `json:"argsTemplate"`
}

// EmulatorListView is the result of EmulatorList.
type EmulatorListView struct {
	Emulators []EmulatorView `json:"emulators"`
}

func emulatorView(e ipc.Emulator) EmulatorView {
	return EmulatorView{ID: e.ID, Name: e.Name, ExePath: e.ExePath, ArgsTemplate: e.ArgsTemplate}
}

// EmulatorList returns the emulators, ordered by name (EMU-001).
func (a *App) EmulatorList() (EmulatorListView, error) {
	var res ipc.EmulatorListResult
	if err := a.callAgent(ipc.MethodEmulatorList, libraryTimeout, nil, &res); err != nil {
		return EmulatorListView{}, err
	}
	out := EmulatorListView{Emulators: make([]EmulatorView, 0, len(res.Emulators))}
	for _, e := range res.Emulators {
		out.Emulators = append(out.Emulators, emulatorView(e))
	}
	return out, nil
}

// EmulatorAdd stores a new emulator (EMU-001).
func (a *App) EmulatorAdd(name, exePath, argsTemplate string) (EmulatorView, error) {
	var res ipc.EmulatorResult
	if err := a.callAgent(ipc.MethodEmulatorAdd, libraryTimeout,
		ipc.EmulatorAddParams{Name: name, ExePath: exePath, ArgsTemplate: argsTemplate}, &res); err != nil {
		return EmulatorView{}, err
	}
	return emulatorView(res.Emulator), nil
}

// EmulatorUpdate changes an emulator (EMU-001).
func (a *App) EmulatorUpdate(id int64, name, exePath, argsTemplate string) (EmulatorView, error) {
	var res ipc.EmulatorResult
	if err := a.callAgent(ipc.MethodEmulatorUpdate, libraryTimeout,
		ipc.EmulatorUpdateParams{ID: id, Name: name, ExePath: exePath, ArgsTemplate: argsTemplate}, &res); err != nil {
		return EmulatorView{}, err
	}
	return emulatorView(res.Emulator), nil
}

// EmulatorDelete removes an emulator; the platforms and games that used it keep
// their rows without it (EMU-002).
func (a *App) EmulatorDelete(id int64) error {
	var res ipc.EmulatorDeletedResult
	return a.callAgent(ipc.MethodEmulatorDelete, libraryTimeout, ipc.EmulatorDeleteParams{ID: id}, &res)
}

// PlatformSetEmulator sets the default emulator of a platform; 0 clears it (EMU-002).
func (a *App) PlatformSetEmulator(platformID, emulatorID int64) (PlatformView, error) {
	// ipc.Optional has no JSON encoding, so a typed struct would send its Go fields. The
	// params are a map: emulatorId is null to clear the default, or the id (the contract).
	var emu any
	if emulatorID > 0 {
		emu = emulatorID
	}
	params := map[string]any{"platformId": platformID, "emulatorId": emu}
	var res ipc.PlatformResult
	if err := a.callAgent(ipc.MethodPlatformSetEmulator, libraryTimeout, params, &res); err != nil {
		return PlatformView{}, err
	}
	return platformView(res.Platform), nil
}

// pickEmulatorTitle is the title of the emulator executable dialog (EMU-006).
const pickEmulatorTitle = "Escolha o executável do emulador"

// PickEmulatorExe opens the executable dialog for an emulator; "" means cancelled.
// It only picks the path: the emulator is stored by EmulatorAdd or EmulatorUpdate (EMU-006).
func (a *App) PickEmulatorExe() (string, error) {
	a.mu.Lock()
	wailsCtx := a.wailsCtx
	a.mu.Unlock()
	if wailsCtx == nil || a.platform.openFile == nil {
		return "", userError(errNoDialog, a.logger())
	}
	path, err := a.platform.openFile(wailsCtx, pickEmulatorTitle, pickExeFilters)
	if err != nil {
		return "", userError(fmt.Errorf("desktop: pick emulator exe: %w", err), a.logger())
	}
	return path, nil
}
