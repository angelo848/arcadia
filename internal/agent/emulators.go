package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"github.com/arcadia-launcher/arcadia/internal/store"
)

// codeEmulatorNotFound is the -32001 of the contract for an unknown emulator or
// platform id (EMU-001, EMU-002).
const codeEmulatorNotFound = -32001

// msgEmulatorIDNotFound is the text of that error.
const msgEmulatorIDNotFound = "Emulador não encontrado."

// emulatorStore is the part of the store the emulator handlers use.
type emulatorStore interface {
	Emulators(ctx context.Context) ([]store.Emulator, error)
	AddEmulator(ctx context.Context, name, exePath, argsTemplate string) (store.Emulator, error)
	UpdateEmulator(ctx context.Context, id int64, name, exePath, argsTemplate string) (store.Emulator, error)
	DeleteEmulator(ctx context.Context, id int64) error
	SetPlatformEmulator(ctx context.Context, platformID int64, emulatorID *int64) error
}

// emulatorHandlers returns the handlers of the emulator methods (contract v0.8.0).
func emulatorHandlers(st emulatorStore, log *slog.Logger) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		ipc.MethodEmulatorList: ipc.NoParams(func(ctx context.Context) (any, error) {
			return handleEmulatorList(ctx, st, log)
		}),
		ipc.MethodEmulatorAdd: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleEmulatorAdd(ctx, st, log, params)
		},
		ipc.MethodEmulatorUpdate: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleEmulatorUpdate(ctx, st, log, params)
		},
		ipc.MethodEmulatorDelete: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handleEmulatorDelete(ctx, st, log, params)
		},
		ipc.MethodPlatformSetEmulator: func(ctx context.Context, params json.RawMessage) (any, error) {
			return handlePlatformSetEmulator(ctx, st, log, params)
		},
	}
}

func handleEmulatorList(ctx context.Context, st emulatorStore, log *slog.Logger) (any, error) {
	list, err := st.Emulators(ctx)
	if err != nil {
		log.Error("falha ao listar os emuladores", "error", err.Error())
		return nil, fmt.Errorf("emulator.list: %w", err)
	}
	out := ipc.EmulatorListResult{Emulators: make([]ipc.Emulator, 0, len(list))}
	for _, e := range list {
		out.Emulators = append(out.Emulators, toIPCEmulator(e))
	}
	return out, nil
}

func handleEmulatorAdd(ctx context.Context, st emulatorStore, log *slog.Logger, params json.RawMessage) (any, error) {
	var p ipc.EmulatorAddParams
	if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	e, err := st.AddEmulator(ctx, p.Name, p.ExePath, p.ArgsTemplate)
	if err != nil {
		return nil, emulatorError("emulator.add", err, log)
	}
	return ipc.EmulatorResult{Emulator: toIPCEmulator(e)}, nil
}

func handleEmulatorUpdate(ctx context.Context, st emulatorStore, log *slog.Logger, params json.RawMessage) (any, error) {
	var p ipc.EmulatorUpdateParams
	if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	e, err := st.UpdateEmulator(ctx, p.ID, p.Name, p.ExePath, p.ArgsTemplate)
	if err != nil {
		return nil, emulatorError("emulator.update", err, log)
	}
	return ipc.EmulatorResult{Emulator: toIPCEmulator(e)}, nil
}

func handleEmulatorDelete(ctx context.Context, st emulatorStore, log *slog.Logger, params json.RawMessage) (any, error) {
	var p ipc.EmulatorDeleteParams
	if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	if err := st.DeleteEmulator(ctx, p.ID); err != nil {
		return nil, emulatorError("emulator.delete", err, log)
	}
	return ipc.EmulatorDeletedResult{ID: p.ID}, nil
}

func handlePlatformSetEmulator(ctx context.Context, st emulatorStore, log *slog.Logger, params json.RawMessage) (any, error) {
	var p ipc.PlatformSetEmulatorParams
	if rpcErr := decodeOrInvalid(params, &p); rpcErr != nil {
		return nil, rpcErr
	}
	var emu *int64
	if p.EmulatorID.Present && !p.EmulatorID.Null {
		v := p.EmulatorID.Value
		emu = &v
	}
	if err := st.SetPlatformEmulator(ctx, p.PlatformID, emu); err != nil {
		return nil, emulatorError("platform.setEmulator", err, log)
	}
	return ipc.PlatformResult{Platform: ipc.Platform{ID: p.PlatformID, EmulatorID: emu}}, nil
}

// decodeOrInvalid decodes the params of a method, or returns -32602 when they are absent
// or not exactly the contract's object (IPC-041).
func decodeOrInvalid(params json.RawMessage, v any) *ipc.Error {
	if params == nil {
		return ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	}
	if rpcErr := ipc.DecodeParams(params, v); rpcErr != nil {
		return rpcErr
	}
	return nil
}

// emulatorError maps a store error to the contract: a name taken or an invalid field
// is -32602, an unknown id is -32001, anything else is an internal error logged at Error.
func emulatorError(method string, err error, log *slog.Logger) error {
	switch {
	case errors.Is(err, store.ErrEmulatorNameTaken), errors.Is(err, domain.ErrInvalidField):
		return ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams)
	case errors.Is(err, store.ErrEmulatorNotFound), errors.Is(err, store.ErrPlatformNotFound), errors.Is(err, store.ErrGameNotFound):
		return ipc.NewError(codeEmulatorNotFound, msgEmulatorIDNotFound)
	}
	log.Error("falha em método de emulador", "method", method, "error", err.Error())
	return fmt.Errorf("%s: %w", method, err)
}

func toIPCEmulator(e store.Emulator) ipc.Emulator {
	return ipc.Emulator{ID: e.ID, Name: e.Name, ExePath: e.ExePath, ArgsTemplate: e.ArgsTemplate}
}
