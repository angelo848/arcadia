package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/media"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// msgPlatformNotFound is the text of the -32001 of the platform methods.
const msgPlatformNotFound = "Plataforma não encontrada."

// platformStore is the part of the store used by the platform handlers.
type platformStore interface {
	Platforms(ctx context.Context) ([]store.Platform, error)
	PlatformByID(ctx context.Context, id int64) (store.Platform, error)
	AddPlatform(ctx context.Context, name string, category domain.PlatformCategory) (store.Platform, bool, error)
	UpdatePlatform(ctx context.Context, id int64, name string, category domain.PlatformCategory) (store.Platform, error)
	SetPlatformIcon(ctx context.Context, id int64, icon string) (store.Platform, string, error)
	PlatformIconInUse(ctx context.Context, icon string) (bool, error)
	DeletePlatform(ctx context.Context, id int64) (string, error)
	SetPlatformEmulator(ctx context.Context, platformID int64, emulatorID *int64) error
	Catalog(ctx context.Context) (store.Catalog, error)
}

// platformIconFiles is the part of *media.Store used for the platform icons.
type platformIconFiles interface {
	ImportPlatformIcon(ctx context.Context, src string) (string, error)
	RemovePlatformIcon(rel string) error
}

// platformService serves the platform methods [PLT-011..PLT-024]. iconMu serializes
// the icon changes and deletions so the "in use" check and the file removal are atomic.
type platformService struct {
	st        platformStore
	files     platformIconFiles
	mediaRoot string
	stat      func(string) (fs.FileInfo, error)
	log       *slog.Logger
	iconMu    sync.Mutex
}

// newPlatformService builds the service; stat is os.Stat in production.
func newPlatformService(st platformStore, files platformIconFiles, mediaRoot string,
	stat func(string) (fs.FileInfo, error), log *slog.Logger) *platformService {
	return &platformService{st: st, files: files, mediaRoot: mediaRoot, stat: stat, log: log}
}

// platformHandlers returns the handlers of platform.list, platform.add, library.catalog,
// platform.update, platform.setIcon, platform.delete and platform.setEmulator.
func platformHandlers(s *platformService) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodPlatformList:        ipc.NoParams(s.list),
		ipc.MethodLibraryCatalog:      ipc.NoParams(s.catalog),
		ipc.MethodPlatformAdd:         s.add,
		ipc.MethodPlatformUpdate:      s.update,
		ipc.MethodPlatformSetIcon:     s.setIcon,
		ipc.MethodPlatformDelete:      s.delete,
		ipc.MethodPlatformSetEmulator: s.setEmulator,
	}
}

func platformInvalid() *ipc.Error { return ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams) }

func platformNotFoundError() *ipc.Error {
	return ipc.NewError(codeEmulatorNotFound, msgPlatformNotFound)
}

// fail maps a store error of a platform method to the contract; anything else is internal.
func (s *platformService) fail(method string, err error) error {
	switch {
	case errors.Is(err, store.ErrPlatformNotFound):
		return platformNotFoundError()
	case errors.Is(err, store.ErrEmulatorNotFound):
		return ipc.NewError(codeEmulatorNotFound, msgEmulatorIDNotFound)
	case errors.Is(err, store.ErrPlatformNameTaken), errors.Is(err, domain.ErrInvalidField):
		return platformInvalid()
	}
	s.log.Error("falha em método de plataforma", "method", method, "error", err.Error())
	return fmt.Errorf("%s: %w", method, err)
}

func (s *platformService) list(ctx context.Context) (any, error) {
	ps, err := s.st.Platforms(ctx)
	if err != nil {
		return nil, s.fail("platform.list", err)
	}
	return ipc.PlatformListResult{Platforms: s.toIPCPlatforms(ps)}, nil
}

func (s *platformService) catalog(ctx context.Context) (any, error) {
	cat, err := s.st.Catalog(ctx)
	if err != nil {
		return nil, s.fail("library.catalog", err)
	}
	modes := make([]string, 0, len(cat.PlayModes))
	for _, m := range cat.PlayModes {
		modes = append(modes, string(m))
	}
	return ipc.Catalog{
		Platforms: s.toIPCPlatforms(cat.Platforms),
		Genres:    toIPCCatalogItems(cat.Genres),
		Companies: toIPCCatalogItems(cat.Companies),
		PlayModes: modes,
	}, nil
}

func (s *platformService) add(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.PlatformAddParams
	if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	name, err := domain.NormalizeName(p.Name)
	if err != nil {
		return nil, platformInvalid()
	}
	category, err := domain.ParsePlatformCategory(p.Category)
	if err != nil {
		return nil, platformInvalid()
	}
	pl, created, err := s.st.AddPlatform(ctx, name, category)
	if err != nil {
		return nil, s.fail("platform.add", err)
	}
	return ipc.PlatformAddResult{Platform: toIPCPlatform(pl, s.mediaRoot), Created: created}, nil
}

func (s *platformService) update(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.PlatformUpdateParams
	if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if p.ID < 1 { // a missing id decodes as 0
		return nil, platformInvalid()
	}
	name, err := domain.NormalizeName(p.Name)
	if err != nil {
		return nil, platformInvalid()
	}
	category, err := domain.ParsePlatformCategory(p.Category)
	if err != nil {
		return nil, platformInvalid()
	}
	pl, err := s.st.UpdatePlatform(ctx, p.ID, name, category)
	if err != nil {
		return nil, s.fail("platform.update", err)
	}
	return ipc.PlatformResult{Platform: toIPCPlatform(pl, s.mediaRoot)}, nil
}

// iconInvalid is the -32602 of an unacceptable icon file, with data.reason [PLT-012].
func iconInvalid(reason string) *ipc.Error {
	return ipc.NewErrorData(ipc.CodeInvalidParams, ipc.MsgInvalidParams, ipc.PlatformIconErrorData{Reason: reason})
}

func (s *platformService) setIcon(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.PlatformSetIconParams
	if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if p.PlatformID < 1 || !p.Path.Present {
		return nil, platformInvalid()
	}
	s.iconMu.Lock()
	defer s.iconMu.Unlock()

	rel := ""
	if p.Path.Present && !p.Path.Null {
		src := p.Path.Value
		if !filepath.IsAbs(src) {
			return nil, iconInvalid(ipc.IconReasonNotAbsolute)
		}
		info, err := s.stat(src)
		switch {
		case err != nil:
			return nil, iconInvalid(ipc.IconReasonNotFound)
		case !info.Mode().IsRegular():
			return nil, iconInvalid(ipc.IconReasonNotAFile)
		case info.Size() > media.MaxPlatformIconBytes:
			return nil, iconInvalid(ipc.IconReasonTooLarge)
		}
		rel, err = s.files.ImportPlatformIcon(ctx, src)
		switch {
		case errors.Is(err, media.ErrTooLarge):
			return nil, iconInvalid(ipc.IconReasonTooLarge)
		case errors.Is(err, media.ErrUnsupported):
			return nil, iconInvalid(ipc.IconReasonUnsupported)
		case err != nil:
			return nil, s.fail("platform.setIcon", err)
		}
	}

	pl, previous, err := s.st.SetPlatformIcon(ctx, p.PlatformID, rel)
	if err != nil {
		if rel != "" {
			s.removeIfUnused(ctx, rel)
		}
		return nil, s.fail("platform.setIcon", err)
	}
	if previous != "" && previous != rel {
		s.removeIfUnused(ctx, previous)
	}
	return ipc.PlatformResult{Platform: toIPCPlatform(pl, s.mediaRoot)}, nil
}

// removeIfUnused deletes the icon file when no platform uses it; failures are only logged.
// The caller holds iconMu.
func (s *platformService) removeIfUnused(ctx context.Context, rel string) {
	inUse, err := s.st.PlatformIconInUse(ctx, rel)
	if err != nil {
		s.log.Warn("falha ao verificar o uso do ícone da plataforma", "icon", rel, "error", err.Error())
		return
	}
	if inUse {
		return
	}
	if err := s.files.RemovePlatformIcon(rel); err != nil {
		s.log.Warn("falha ao apagar o ícone da plataforma", "icon", rel, "error", err.Error())
	}
}

func (s *platformService) delete(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.PlatformDeleteParams
	if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if p.ID < 1 {
		return nil, platformInvalid()
	}
	s.iconMu.Lock()
	defer s.iconMu.Unlock()
	icon, err := s.st.DeletePlatform(ctx, p.ID)
	if err != nil {
		var inUse *store.PlatformInUseError
		if errors.As(err, &inUse) {
			return nil, ipc.NewErrorData(ipc.CodePlatformInUse, ipc.MsgPlatformInUse,
				ipc.PlatformInUseData{PlatformID: inUse.PlatformID, GameCount: inUse.GameCount})
		}
		return nil, s.fail("platform.delete", err)
	}
	if icon != "" {
		s.removeIfUnused(ctx, icon)
	}
	return ipc.PlatformDeletedResult{ID: p.ID}, nil
}

func (s *platformService) setEmulator(ctx context.Context, params json.RawMessage) (any, error) {
	var p ipc.PlatformSetEmulatorParams
	if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if p.PlatformID < 1 {
		return nil, platformInvalid()
	}
	var emu *int64
	if p.EmulatorID.Present && !p.EmulatorID.Null {
		v := p.EmulatorID.Value
		emu = &v
	}
	if err := s.st.SetPlatformEmulator(ctx, p.PlatformID, emu); err != nil {
		return nil, s.fail("platform.setEmulator", err)
	}
	pl, err := s.st.PlatformByID(ctx, p.PlatformID)
	if err != nil {
		return nil, s.fail("platform.setEmulator", err)
	}
	return ipc.PlatformResult{Platform: toIPCPlatform(pl, s.mediaRoot)}, nil
}

// toIPCPlatform converts a store platform to its wire form: the icon becomes an absolute
// path under mediaRoot and an empty seed key or icon becomes null [PLT-014].
func toIPCPlatform(p store.Platform, mediaRoot string) ipc.Platform {
	out := ipc.Platform{
		ID: p.ID, Name: p.Name, Category: string(p.Category), EmulatorID: p.EmulatorID, GameCount: p.GameCount,
	}
	if p.SeedKey != "" {
		k := p.SeedKey
		out.SeedKey = &k
	}
	if p.Icon != "" {
		ip := filepath.Join(mediaRoot, p.Icon)
		out.IconPath = &ip
	}
	return out
}

func (s *platformService) toIPCPlatforms(ps []store.Platform) []ipc.Platform {
	out := make([]ipc.Platform, 0, len(ps))
	for _, p := range ps {
		out = append(out, toIPCPlatform(p, s.mediaRoot))
	}
	return out
}
