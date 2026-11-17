package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// locationStore is what game.location needs from the store.
type locationStore interface {
	Game(ctx context.Context, id int64) (store.Game, error)
	TrackingRules(ctx context.Context, gameID int64) ([]store.TrackingRule, error)
}

// locationHandlers serves game.location: the file of an exe or emulator game (its ROM), or the install
// folder of a URI game. A path that no longer exists is not returned.
func locationHandlers(st locationStore, stat func(string) (os.FileInfo, error), log *slog.Logger) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodGameLocation: func(ctx context.Context, params json.RawMessage) (any, error) {
			var p ipc.GameLocationParams
			if params == nil {
				return nil, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
			}
			if rpcErr := ipc.DecodeParams(params, &p); rpcErr != nil {
				return nil, rpcErr
			}
			g, err := st.Game(ctx, p.GameID)
			if err != nil {
				if errors.Is(err, store.ErrGameNotFound) {
					return nil, ipc.NewErrorData(ipc.CodeGameNotFound, ipc.MsgGameNotFound, ipc.GameNotFoundData{GameID: p.GameID})
				}
				log.Error("falha ao ler o jogo", "game_id", p.GameID, "error", err.Error())
				return nil, fmt.Errorf("game.location: %w", err)
			}
			if g.LaunchType == domain.LaunchTypeURI {
				rules, err := st.TrackingRules(ctx, g.ID)
				if err != nil {
					return nil, fmt.Errorf("game.location: %w", err)
				}
				for _, r := range rules {
					if r.Kind != "install_dir" {
						continue
					}
					if fi, err := stat(r.Value); err == nil && fi.IsDir() {
						return ipc.GameLocationResult{Path: r.Value, Kind: ipc.LocationDir}, nil
					}
				}
				return ipc.GameLocationResult{}, nil
			}
			if _, err := stat(g.LaunchTarget); err != nil {
				return ipc.GameLocationResult{}, nil
			}
			return ipc.GameLocationResult{Path: g.LaunchTarget, Kind: ipc.LocationFile}, nil
		},
	}
}
