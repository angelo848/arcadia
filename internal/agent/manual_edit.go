package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// manualEditStore is what the manual edits need from the store.
type manualEditStore interface {
	GameDetail(ctx context.Context, id int64) (store.GameDetail, error)
	ListMedia(ctx context.Context, gameID int64) ([]store.MediaRow, error)
	AddMedia(ctx context.Context, m store.MediaRow) (bool, error)
	RemoveMedia(ctx context.Context, id int64) (store.MediaRow, bool, error)
	SetTotalPlaySeconds(ctx context.Context, id, seconds int64, at time.Time) (store.GameDetail, error)
}

// manualEditFiles is what the manual edits need from the media folder.
type manualEditFiles interface {
	Import(ctx context.Context, gameID int64, src string) (media.Stored, bool, error)
	RemoveFiles(gameID int64, rels ...string) error
}

// manualEdit serves media.add, media.remove and game.setPlayTime (phase 14, EDM-001..EDM-005).
type manualEdit struct {
	st    manualEditStore
	files manualEditFiles
	notes interface{ MediaChanged(gameID int64) }
	clock domain.Clock
	log   *slog.Logger
}

func manualEditHandlers(m manualEdit) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodMediaAdd:        m.mediaAdd,
		ipc.MethodMediaRemove:     m.mediaRemove,
		ipc.MethodGameSetPlayTime: m.setPlayTime,
	}
}

func errBadEdit() error { return ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams) }

func (m manualEdit) internal(what string, err error) error {
	m.log.Error("edição manual: "+what, "error", err.Error())
	return ipc.NewError(ipc.CodeInternalError, ipc.MsgInternalError)
}

// manualKind reports whether the user may add images of kind by hand (EDM-001).
func manualKind(kind string) bool {
	return kind == "cover" || kind == "screenshot" || kind == "background"
}

// mediaAdd copies an image file into the media of a game. A cover replaces the current one; the
// other kinds are refused when the game is at the limit of the kind (EDM-001, EDM-002).
func (m manualEdit) mediaAdd(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.MediaAddParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if p.GameID < 1 || !manualKind(p.Kind) || !filepath.IsAbs(p.Path) {
		return nil, errBadEdit()
	}
	if _, err := m.st.GameDetail(ctx, p.GameID); errors.Is(err, store.ErrGameNotFound) {
		return nil, ipc.NewError(ipc.CodeGameNotFound, ipc.MsgGameNotFound)
	} else if err != nil {
		return nil, m.internal("ler jogo", err)
	}
	rows, err := m.st.ListMedia(ctx, p.GameID)
	if err != nil {
		return nil, m.internal("listar mídia", err)
	}
	var sameKind []store.MediaRow
	for _, r := range rows {
		if r.Kind == p.Kind {
			sameKind = append(sameKind, r)
		}
	}
	replace := p.Kind == "cover"
	if !replace && len(sameKind) >= domain.MediaKindLimit(p.Kind) {
		return nil, errBadEdit()
	}
	stored, _, err := m.files.Import(ctx, p.GameID, p.Path)
	if err != nil {
		m.log.Warn("edição manual: imagem recusada", "game", p.GameID, "error", err.Error())
		return nil, errBadEdit()
	}
	for _, r := range rows {
		if r.Hash == stored.Hash {
			// The game already has this very image: nothing to add, and its files stay.
			return ipc.MediaAddResult{Added: false}, nil
		}
	}
	if replace {
		for _, r := range sameKind {
			if err := m.remove(ctx, r.ID); err != nil {
				return nil, err
			}
		}
	}
	added, err := m.st.AddMedia(ctx, store.MediaRow{
		GameID: p.GameID, Kind: p.Kind, Provider: ipc.ProviderUser, SourceURL: p.Path,
		Hash: stored.Hash, Path: stored.Path, ThumbPath: stored.ThumbPath, CreatedAt: m.clock.Now(),
	})
	if err != nil {
		return nil, m.internal("gravar mídia", err)
	}
	m.notes.MediaChanged(p.GameID)
	return ipc.MediaAddResult{Added: added}, nil
}

// remove deletes one media row and its files. A file that cannot be deleted is only logged: the
// row is already gone and the orphan sweep collects the file later.
func (m manualEdit) remove(ctx context.Context, id int64) error {
	row, found, err := m.st.RemoveMedia(ctx, id)
	if err != nil {
		return m.internal("apagar mídia", err)
	}
	if !found {
		return nil
	}
	if err := m.files.RemoveFiles(row.GameID, row.Path, row.ThumbPath); err != nil {
		m.log.Warn("edição manual: arquivo de mídia não apagado", "game", row.GameID, "error", err.Error())
	}
	m.notes.MediaChanged(row.GameID)
	return nil
}

// mediaRemove deletes one image of a game, row and files (EDM-003).
func (m manualEdit) mediaRemove(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.MediaRemoveParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if p.MediaID < 1 {
		return nil, errBadEdit()
	}
	if err := m.remove(ctx, p.MediaID); err != nil {
		return nil, err
	}
	return ipc.MediaRemoveResult{Removed: true}, nil
}

// setPlayTime replaces the total play time of a game (EDM-005).
func (m manualEdit) setPlayTime(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.GameSetPlayTimeParams
	if e := decodeOrInvalid(params, &p); e != nil {
		return nil, e
	}
	if p.GameID < 1 || p.TotalPlaySeconds < 0 || p.TotalPlaySeconds > ipc.MaxPlaySeconds {
		return nil, errBadEdit()
	}
	d, err := m.st.SetTotalPlaySeconds(ctx, p.GameID, p.TotalPlaySeconds, m.clock.Now())
	if errors.Is(err, store.ErrGameNotFound) {
		return nil, ipc.NewError(ipc.CodeGameNotFound, ipc.MsgGameNotFound)
	}
	if err != nil {
		return nil, m.internal("gravar tempo jogado", err)
	}
	return ipc.GameDetailResult{Game: gameDetailToIPC(d)}, nil
}
