package main

import (
	"fmt"
)

// pickGameTitle is the title of the executable dialog of a game's page (EDT-010).
const pickGameTitle = "Escolha o executável do jogo"

// PickGameExecutable opens the Explorer dialog for the target of a game (an executable, or a ROM of an emulator game) and returns its path;
// "" means the dialog was cancelled. It only picks the path: the game is saved by UpdateGame (EDT-010).
func (a *App) PickGameExecutable() (string, error) {
	a.mu.Lock()
	wailsCtx := a.wailsCtx
	a.mu.Unlock()
	if wailsCtx == nil || a.platform.openFile == nil {
		return "", userError(errNoDialog, a.logger())
	}
	path, err := a.platform.openFile(wailsCtx, pickGameTitle, pickAnyFilters)
	if err != nil {
		return "", userError(fmt.Errorf("desktop: pick game exe: %w", err), a.logger())
	}
	return path, nil
}
