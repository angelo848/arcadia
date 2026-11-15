package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// errNoClient is a binding call before startup created the IPC client.
var errNoClient = errors.New("desktop: agent client not ready")

// DSK-011 texts shown to the user.
const (
	msgGameNotFound      = "Jogo não encontrado. Atualize a lista."
	msgSessionActive     = "Já há um jogo em andamento. Feche-o antes de iniciar outro."
	msgSessionActiveFmt  = "Já há um jogo em andamento: %s. Feche-o antes de iniciar outro."
	msgLaunchFailed      = "Não foi possível iniciar o jogo."
	msgLaunchFailedFmt   = "Não foi possível iniciar o jogo: %s"
	msgFileNotFound      = "Arquivo não encontrado."
	msgNotAFile          = "O caminho escolhido não é um arquivo."
	msgNotExe            = "Escolha um arquivo .exe."
	msgInvalidData       = "Dados inválidos."
	msgAgentNotResponds  = "O agente não está respondendo. Tente de novo."
	msgUnexpectedFailure = "Erro inesperado. Veja o log."
	// Metadata and media (phase 7, contract v0.9.0).
	msgProviderNoKey       = "A chave deste provedor não está cadastrada. Cadastre-a nas configurações de provedores."
	msgProviderUnavailable = "O provedor não respondeu agora. Tente de novo."
	msgMediaJobNotFound    = "Esse download já não está na fila."
	// Unified metadata search (phase 12, contract v0.14.0).
	msgBatchNotFound   = "Essa busca de metadados não existe mais."
	msgBatchBusy       = "Já existe uma busca de metadados em andamento."
	msgBatchWrongState = "A busca de metadados não está nessa etapa."
)

// ADM-003: a game that needs more elevation than the agent has fails with
// ERROR_ELEVATION_REQUIRED (740). The text tells the user what to do.
const (
	winErrorElevationRequired = 740
	msgElevationRequired      = "Não foi possível iniciar o jogo: o Arcadia precisa estar como administrador. Feche-o e abra de novo com a elevação."
)

// msgRemoveSessionActive is the DSK-018 text of a game.delete refused with -32002
// (open session). It never names the game.
const msgRemoveSessionActive = "Não é possível remover: o jogo está em uma sessão ativa. Feche a sessão antes de remover."

// DSK-015 texts for -32005 (session not pausable).
const (
	msgNothingToPause   = "Não há jogo em andamento para pausar."
	msgGraceNoPause     = "O jogo está encerrando. Não dá para pausar agora."
	msgPauseUnavailable = "Não foi possível pausar agora."
)

// userError converts a binding error into the Portuguese text of DSK-011.
// Errors other than -32002..-32004 and connection errors are logged at Error.
func userError(err error, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	if isConnectionError(err) {
		return errors.New(msgAgentNotResponds)
	}
	var rpcErr *ipc.Error
	if errors.As(err, &rpcErr) {
		switch rpcErr.Code {
		case ipc.CodeSessionActive:
			var d ipc.SessionRef
			if decodeData(rpcErr.Data, &d) && d.Title != "" {
				return fmt.Errorf(msgSessionActiveFmt, d.Title)
			}
			return errors.New(msgSessionActive)
		case ipc.CodeLaunchFailed:
			var d ipc.LaunchFailedData
			if decodeData(rpcErr.Data, &d) && d.WinError != nil && *d.WinError == winErrorElevationRequired {
				return errors.New(msgElevationRequired)
			}
			if decodeData(rpcErr.Data, &d) && d.Error != "" {
				return fmt.Errorf(msgLaunchFailedFmt, d.Error)
			}
			return errors.New(msgLaunchFailed)
		case ipc.CodeInvalidExecutable:
			var d ipc.InvalidExecutableData
			if !decodeData(rpcErr.Data, &d) {
				return errors.New(msgInvalidData)
			}
			switch d.Reason {
			case ipc.ReasonNotAbsolute, ipc.ReasonNotFound:
				return errors.New(msgFileNotFound)
			case ipc.ReasonNotAFile:
				return errors.New(msgNotAFile)
			case ipc.ReasonNotExe:
				return errors.New(msgNotExe)
			default:
				return errors.New(msgInvalidData)
			}
		case ipc.CodeSessionNotPausable:
			// Expected state of the session, not a failure: logged at Debug only.
			log.Debug("sessão não pausável", "error", err.Error())
			var d ipc.SessionNotPausableData
			if decodeData(rpcErr.Data, &d) {
				switch d.Reason {
				case ipc.ReasonNoSession:
					return errors.New(msgNothingToPause)
				case ipc.ReasonGrace:
					return errors.New(msgGraceNoPause)
				}
			}
			return errors.New(msgPauseUnavailable)
		case ipc.CodeGameNotFound:
			logBindingError(log, err)
			return errors.New(msgGameNotFound)
		case ipc.CodeProviderNoKey:
			return errors.New(msgProviderNoKey)
		case ipc.CodeProviderUnavailable:
			return errors.New(msgProviderUnavailable)
		case ipc.CodeMediaJobNotFound:
			return errors.New(msgMediaJobNotFound)
		case ipc.CodeBatchNotFound:
			return errors.New(msgBatchNotFound)
		case ipc.CodeBatchBusy:
			return errors.New(msgBatchBusy)
		case ipc.CodeBatchWrongState:
			return errors.New(msgBatchWrongState)
		case ipc.CodeInvalidParams:
			logBindingError(log, err)
			return errors.New(msgInvalidData)
		}
	}
	logBindingError(log, err)
	return errors.New(msgUnexpectedFailure)
}

// isConnectionError reports the errors shown as "agent not responding":
// no client, connection lost, client closed, deadline or cancellation.
func isConnectionError(err error) bool {
	return errors.Is(err, errNoClient) ||
		errors.Is(err, ipc.ErrConnectionLost) ||
		errors.Is(err, ipc.ErrClientClosed) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled)
}

// decodeData decodes a JSON-RPC error data member into v; absent or invalid
// data yields false.
func decodeData(raw json.RawMessage, v any) bool {
	if len(raw) == 0 {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

func logBindingError(log *slog.Logger, err error) {
	log.Error("falha na chamada ao agente", "error", err.Error())
}
