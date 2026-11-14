package main

import (
	"errors"
	"fmt"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// PLT-012 texts of an invalid platform icon.
const (
	msgIconNotAbsolute = "Informe o caminho completo do arquivo."
	msgIconNotFound    = "Arquivo não encontrado."
	msgIconNotAFile    = "O caminho não é um arquivo."
	msgIconTooLarge    = "O arquivo passa de 2 MB."
	msgIconUnsupported = "O arquivo não é uma imagem PNG ou JPEG."
)

// msgPlatformInUseFmt is the PLT-021 text of a platform.delete refused with -32013.
const msgPlatformInUseFmt = "Esta plataforma tem %d jogo(s). Mova-os para outra plataforma ou exclua-os antes."

// pickPlatformIconTitle is the title of the platform icon dialog (PLT-023).
const pickPlatformIconTitle = "Escolha a imagem do ícone"

// pickIconFilters is the filter of the platform icon dialog (PLT-023).
var pickIconFilters = []fileFilter{{DisplayName: "Imagens (*.png;*.jpg;*.jpeg)", Pattern: "*.png;*.jpg;*.jpeg"}}

// PlatformUpdate renames a platform and sets its category (PLT-018, PLT-019).
func (a *App) PlatformUpdate(id int64, name, category string) (PlatformView, error) {
	var res ipc.PlatformResult
	if err := a.callAgent(ipc.MethodPlatformUpdate, libraryTimeout,
		ipc.PlatformUpdateParams{ID: id, Name: name, Category: category}, &res); err != nil {
		return PlatformView{}, err
	}
	return a.platformView(res.Platform), nil
}

// PlatformSetIcon sets the custom icon of a platform from an image file; a nil path goes back to the
// bundled icon (PLT-012, PLT-013).
func (a *App) PlatformSetIcon(id int64, path *string) (PlatformView, error) {
	// ipc.Optional has no JSON encoding: the params are a map, path is null to restore the default.
	var p any
	if path != nil {
		p = *path
	}
	params := map[string]any{"platformId": id, "path": p}
	var res ipc.PlatformResult
	err := a.callAgentRaw(ipc.MethodPlatformSetIcon, libraryTimeout, params, &res)
	if err != nil {
		var rpcErr *ipc.Error
		if errors.As(err, &rpcErr) && rpcErr.Code == ipc.CodeInvalidParams {
			var d ipc.PlatformIconErrorData
			if decodeData(rpcErr.Data, &d) {
				switch d.Reason {
				case ipc.IconReasonNotAbsolute:
					return PlatformView{}, errors.New(msgIconNotAbsolute)
				case ipc.IconReasonNotFound:
					return PlatformView{}, errors.New(msgIconNotFound)
				case ipc.IconReasonNotAFile:
					return PlatformView{}, errors.New(msgIconNotAFile)
				case ipc.IconReasonTooLarge:
					return PlatformView{}, errors.New(msgIconTooLarge)
				case ipc.IconReasonUnsupported:
					return PlatformView{}, errors.New(msgIconUnsupported)
				}
			}
		}
		return PlatformView{}, userError(err, a.logger())
	}
	return a.platformView(res.Platform), nil
}

// PlatformDelete removes a platform without games (PLT-020); with games it is refused (PLT-021).
func (a *App) PlatformDelete(id int64) error {
	var res ipc.PlatformDeletedResult
	err := a.callAgentRaw(ipc.MethodPlatformDelete, libraryTimeout, ipc.PlatformDeleteParams{ID: id}, &res)
	if err == nil {
		return nil
	}
	var rpcErr *ipc.Error
	if errors.As(err, &rpcErr) && rpcErr.Code == ipc.CodePlatformInUse {
		var d ipc.PlatformInUseData
		if decodeData(rpcErr.Data, &d) {
			return fmt.Errorf(msgPlatformInUseFmt, d.GameCount)
		}
	}
	return userError(err, a.logger())
}

// PickPlatformIcon opens the image dialog for a platform icon; "" means cancelled. It only picks the
// path: the icon is stored by PlatformSetIcon (PLT-023).
func (a *App) PickPlatformIcon() (string, error) {
	a.mu.Lock()
	wailsCtx := a.wailsCtx
	a.mu.Unlock()
	if wailsCtx == nil || a.platform.openFile == nil {
		return "", userError(errNoDialog, a.logger())
	}
	path, err := a.platform.openFile(wailsCtx, pickPlatformIconTitle, pickIconFilters)
	if err != nil {
		return "", userError(fmt.Errorf("desktop: pick platform icon: %w", err), a.logger())
	}
	return path, nil
}
