package main

import (
	"errors"
	"fmt"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// pickGameImageTitle is the title of the dialog that picks an image for a game (EDM-004).
const pickGameImageTitle = "Escolha a imagem"

// msgImageRefused is shown when media.add refuses the file or the kind is full (EDM-002).
const msgImageRefused = "Não foi possível usar esta imagem. Use um arquivo PNG ou JPEG de até 20 MB e confira o limite deste tipo de imagem."

// PickGameImage opens the image dialog; "" means cancelled (EDM-004).
func (a *App) PickGameImage() (string, error) {
	a.mu.Lock()
	wailsCtx := a.wailsCtx
	a.mu.Unlock()
	if wailsCtx == nil || a.platform.openFile == nil {
		return "", userError(errNoDialog, a.logger())
	}
	path, err := a.platform.openFile(wailsCtx, pickGameImageTitle, pickIconFilters)
	if err != nil {
		return "", userError(fmt.Errorf("desktop: pick game image: %w", err), a.logger())
	}
	return path, nil
}

// MediaAdd adds an image file to a game; a cover replaces the current one (EDM-001, EDM-002).
func (a *App) MediaAdd(gameID int64, kind, path string) error {
	var res ipc.MediaAddResult
	err := a.callAgentRaw(ipc.MethodMediaAdd, scrapeTimeout, ipc.MediaAddParams{GameID: gameID, Kind: kind, Path: path}, &res)
	if err == nil {
		return nil
	}
	var rpcErr *ipc.Error
	if errors.As(err, &rpcErr) && rpcErr.Code == ipc.CodeInvalidParams {
		return errors.New(msgImageRefused)
	}
	return userError(err, a.logger())
}

// MediaRemove removes one image of a game (EDM-003).
func (a *App) MediaRemove(mediaID int64) error {
	var res ipc.MediaRemoveResult
	return a.callAgent(ipc.MethodMediaRemove, libraryTimeout, ipc.MediaRemoveParams{MediaID: mediaID}, &res)
}

// SetPlayTime replaces the total play time of a game, in seconds (EDM-005).
func (a *App) SetPlayTime(gameID, seconds int64) (GameDetailView, error) {
	var res ipc.GameDetailResult
	params := ipc.GameSetPlayTimeParams{GameID: gameID, TotalPlaySeconds: seconds}
	if err := a.callAgent(ipc.MethodGameSetPlayTime, libraryTimeout, params, &res); err != nil {
		return GameDetailView{}, err
	}
	return gameDetailView(res.Game), nil
}
