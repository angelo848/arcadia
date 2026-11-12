package main

// Tests for T12 (docs/specs/phase-3-tracker/tasks.md): userError, the JSON-RPC
// error -> Portuguese text table of DSK-011.
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - userError finds the *ipc.Error with errors.As (it may be wrapped) and reads
//     Data as SessionRef (-32002), LaunchFailedData (-32003) or
//     InvalidExecutableData (-32004).
//   - Connection errors are recognised with errors.Is (they may be wrapped).
//   - "Logs Error" means exactly one record at slog.LevelError per call; the
//     message text is not fixed by the spec and is not checked.
//   - «title» / «error» in DSK-011 only mark the inserted value; the guillemets
//     are not part of the text (DSK-011, alterado em 2026-11-09, testes da T12).
//   - -32002..-32004 with missing or invalid data still do not log Error (the
//     codes are excluded from Error logging by DSK-011).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// DSK-011 texts.
const (
	txtGameNotFound  = "Jogo não encontrado. Atualize a lista."
	txtFileNotFound  = "Arquivo não encontrado."
	txtNotAFile      = "O caminho escolhido não é um arquivo."
	txtNotExe        = "Escolha um arquivo .exe."
	txtInvalidData   = "Dados inválidos."
	txtNotResponding = "O agente não está respondendo. Tente de novo."
	txtUnexpected    = "Erro inesperado. Veja o log."
	sessionActiveFmt = "Já há um jogo em andamento: %s. Feche-o antes de iniciar outro."
	launchFailedFmt  = "Não foi possível iniciar o jogo: %s"
	txtSessionActive = "Já há um jogo em andamento. Feche-o antes de iniciar outro."
	txtLaunchFailed  = "Não foi possível iniciar o jogo."
)

// rawDataErr is an *ipc.Error whose data member is raw ("" = absent).
func rawDataErr(code int, msg string, raw string) error {
	e := ipc.NewError(code, msg)
	if raw != "" {
		e.Data = json.RawMessage(raw)
	}
	return e
}

func sessionActiveErr(title string) error {
	return ipc.NewErrorData(ipc.CodeSessionActive, ipc.MsgSessionActive,
		ipc.SessionRef{SessionID: 9, GameID: 3, Title: title})
}

func launchFailedErr(msg string) error {
	return ipc.NewErrorData(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed, ipc.LaunchFailedData{Error: msg})
}

func invalidExeErr(reason string) error {
	return ipc.NewErrorData(ipc.CodeInvalidExecutable, ipc.MsgInvalidExecutable,
		ipc.InvalidExecutableData{Reason: reason})
}

func gameNotFoundErr() error {
	return ipc.NewErrorData(ipc.CodeGameNotFound, ipc.MsgGameNotFound, ipc.GameNotFoundData{GameID: 3})
}

// userText runs userError with a recording logger and returns the text and the
// number of Error records.
func userText(t *testing.T, err error) (string, int) {
	t.Helper()
	logs := newRecLog()
	got := userError(err, logs.logger())
	if got == nil {
		t.Fatalf("userError(%v) = nil, want an error", err)
	}
	return got.Error(), logs.countLevel(slog.LevelError)
}

func wantText(t *testing.T, err error, want ...string) {
	t.Helper()
	got, _ := userText(t, err)
	for _, w := range want {
		if got == w {
			return
		}
	}
	t.Errorf("userError(%v) = %q, want one of %q", err, got, want)
}

func TestUserError(t *testing.T) {
	t.Parallel()

	t.Run("DSK-011/-32001 Jogo não encontrado. Atualize a lista.", func(t *testing.T) {
		t.Parallel()
		wantText(t, gameNotFoundErr(), txtGameNotFound)
		wantText(t, fmt.Errorf("desktop: launch: %w", gameNotFoundErr()), txtGameNotFound)
	})

	t.Run("DSK-011/-32002 com title", func(t *testing.T) {
		t.Parallel()
		wantText(t, sessionActiveErr("Hollow Knight"),
			"Já há um jogo em andamento: Hollow Knight. Feche-o antes de iniciar outro.")
		wantText(t, sessionActiveErr("Celeste"), fmt.Sprintf(sessionActiveFmt, "Celeste"))
	})

	t.Run("DSK-011/-32003 com error", func(t *testing.T) {
		t.Parallel()
		const msg = "Acesso negado. (740)"
		wantText(t, launchFailedErr(msg), fmt.Sprintf(launchFailedFmt, msg))
		wantText(t, launchFailedErr(msg), "Não foi possível iniciar o jogo: Acesso negado. (740)")
	})

	t.Run("DSK-011/-32002 com data ausente ou inválido", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{"", `"x"`, `42`, `{"title":7}`, `null`} {
			wantText(t, rawDataErr(ipc.CodeSessionActive, ipc.MsgSessionActive, raw), txtSessionActive)
		}
	})

	t.Run("DSK-011/-32003 com data ausente ou inválido", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{"", `"x"`, `42`, `{"error":7}`, `null`} {
			wantText(t, rawDataErr(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed, raw), txtLaunchFailed)
		}
	})

	t.Run("DSK-011/-32004 com motivo desconhecido ou data inválido dá Dados inválidos.", func(t *testing.T) {
		t.Parallel()
		wantText(t, invalidExeErr("weird"), txtInvalidData)
		wantText(t, invalidExeErr(""), txtInvalidData)
		for _, raw := range []string{"", `"x"`, `{"reason":7}`, `null`} {
			wantText(t, rawDataErr(ipc.CodeInvalidExecutable, ipc.MsgInvalidExecutable, raw), txtInvalidData)
		}
	})

	t.Run("DSK-011/-32004 not-absolute e not-found dão Arquivo não encontrado.", func(t *testing.T) {
		t.Parallel()
		wantText(t, invalidExeErr(ipc.ReasonNotAbsolute), txtFileNotFound)
		wantText(t, invalidExeErr(ipc.ReasonNotFound), txtFileNotFound)
	})

	t.Run("DSK-011/-32004 not-a-file", func(t *testing.T) {
		t.Parallel()
		wantText(t, invalidExeErr(ipc.ReasonNotAFile), txtNotAFile)
	})

	t.Run("DSK-011/-32004 not-exe", func(t *testing.T) {
		t.Parallel()
		wantText(t, invalidExeErr(ipc.ReasonNotExe), txtNotExe)
	})

	t.Run("DSK-011/-32602 Dados inválidos.", func(t *testing.T) {
		t.Parallel()
		wantText(t, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams), txtInvalidData)
	})

	t.Run("DSK-011/ErrConnectionLost, ErrClientClosed, DeadlineExceeded e errNoClient dão O agente não está respondendo.", func(t *testing.T) {
		t.Parallel()
		for _, err := range []error{
			ipc.ErrConnectionLost,
			fmt.Errorf("ipc: call game.list: %w", ipc.ErrConnectionLost),
			ipc.ErrClientClosed,
			fmt.Errorf("ipc: call game.list: %w", ipc.ErrClientClosed),
			context.DeadlineExceeded,
			fmt.Errorf("ipc: call game.launch: waiting for connection: %w", context.DeadlineExceeded),
			errNoClient,
		} {
			wantText(t, err, txtNotResponding)
		}
	})

	t.Run("DSK-011/context.Canceled dá O agente não está respondendo.", func(t *testing.T) {
		t.Parallel()
		for _, err := range []error{
			context.Canceled,
			fmt.Errorf("ipc: call game.list: %w", context.Canceled),
		} {
			wantText(t, err, txtNotResponding)
		}
	})

	t.Run("DSK-011/outro erro dá Erro inesperado. Veja o log.", func(t *testing.T) {
		t.Parallel()
		for _, err := range []error{
			errors.New("boom"),
			ipc.NewError(ipc.CodeMethodNotFound, "Method not found"), // old agent (design.md)
			ipc.NewError(ipc.CodeInternalError, "Internal error"),    // launch deadline / stat error
			ipc.NewError(ipc.CodeInvalidRequest, "Invalid Request"),
			ipc.NewError(-32099, "unknown"),
		} {
			wantText(t, err, txtUnexpected)
		}
	})

	t.Run("DSK-011/-32001, -32602 e erro inesperado logam Error", func(t *testing.T) {
		t.Parallel()
		for _, err := range []error{
			gameNotFoundErr(),
			ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams),
			ipc.NewError(ipc.CodeInternalError, "Internal error"),
			ipc.NewError(ipc.CodeMethodNotFound, "Method not found"),
			errors.New("boom"),
		} {
			if _, n := userText(t, err); n != 1 {
				t.Errorf("userError(%v) logged %d Error records, want 1", err, n)
			}
		}
	})

	t.Run("DSK-011/-32002..-32004 e erros de conexão não logam Error", func(t *testing.T) {
		t.Parallel()
		for _, err := range []error{
			sessionActiveErr("Celeste"),
			launchFailedErr("boom"),
			invalidExeErr(ipc.ReasonNotFound),
			invalidExeErr(ipc.ReasonNotExe),
			invalidExeErr("weird"),
			rawDataErr(ipc.CodeSessionActive, ipc.MsgSessionActive, ""),
			rawDataErr(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed, `"x"`),
			rawDataErr(ipc.CodeInvalidExecutable, ipc.MsgInvalidExecutable, ""),
			context.Canceled,
			ipc.ErrConnectionLost,
			ipc.ErrClientClosed,
			fmt.Errorf("ipc: call: %w", context.DeadlineExceeded),
			errNoClient,
		} {
			if _, n := userText(t, err); n != 0 {
				t.Errorf("userError(%v) logged %d Error records, want 0", err, n)
			}
		}
	})
}

// T21 (docs/specs/phase-4-pause/tasks.md): DSK-015 adds -32005 (session not
// pausable) to the DSK-011 table.
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - userError reads Data as SessionNotPausableData (-32005); a missing or
//     invalid data member, or an unknown reason, gives the generic pause text.
//   - The log level of -32005 is not fixed by the spec and is not checked.
const (
	txtNothingToPause   = "Não há jogo em andamento para pausar."
	txtGraceNoPause     = "O jogo está encerrando. Não dá para pausar agora."
	txtPauseUnavailable = "Não foi possível pausar agora."
)

// notPausableErr is an -32005 error whose data carries reason.
func notPausableErr(reason string) error {
	return ipc.NewErrorData(ipc.CodeSessionNotPausable, ipc.MsgSessionNotPausable,
		ipc.SessionNotPausableData{Reason: reason})
}

func TestUserErrorNotPausable(t *testing.T) {
	t.Parallel()

	t.Run("DSK-015/-32005 reason no-session → Não há jogo em andamento para pausar.", func(t *testing.T) {
		t.Parallel()
		wantText(t, notPausableErr(ipc.ReasonNoSession), txtNothingToPause)
		wantText(t, fmt.Errorf("desktop: session.pause: %w", notPausableErr(ipc.ReasonNoSession)), txtNothingToPause)
	})

	t.Run("DSK-015/-32005 reason grace → O jogo está encerrando. Não dá para pausar agora.", func(t *testing.T) {
		t.Parallel()
		wantText(t, notPausableErr(ipc.ReasonGrace), txtGraceNoPause)
	})

	t.Run("DSK-015/-32005 sem data ou motivo desconhecido → Não foi possível pausar agora.", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{"", `"x"`, `null`, `{}`, `{"reason":7}`} {
			wantText(t, rawDataErr(ipc.CodeSessionNotPausable, ipc.MsgSessionNotPausable, raw), txtPauseUnavailable)
		}
		wantText(t, notPausableErr("weird"), txtPauseUnavailable)
		wantText(t, notPausableErr(""), txtPauseUnavailable)
	})

	t.Run("DSK-015/as demais mensagens de DSK-011 continuam", func(t *testing.T) {
		t.Parallel()
		wantText(t, gameNotFoundErr(), txtGameNotFound)
		wantText(t, sessionActiveErr("Celeste"), fmt.Sprintf(sessionActiveFmt, "Celeste"))
		wantText(t, launchFailedErr("boom"), "Não foi possível iniciar o jogo: boom")
		wantText(t, invalidExeErr(ipc.ReasonNotExe), txtNotExe)
		wantText(t, ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams), txtInvalidData)
		wantText(t, ipc.ErrConnectionLost, txtNotResponding)
		wantText(t, errors.New("boom"), txtUnexpected)
	})
}

// T24 (docs/specs/phase-5-library/tasks.md): DSK-018 gives DeleteGame its own
// text for -32002 (refused removal, no title); the other codes keep DSK-011.
//
// Assumptions beyond tasks.md (the implementer must follow them):
//   - The DSK-018 text is used for every -32002 of game.delete, with or without
//     a data member, and it never contains the game title.
//   - -32002 is not logged at Error (DSK-011 excludes the codes -32002..-32004
//     from Error logging; DSK-018 does not change that).
//   - userError keeps the DSK-011 text of -32002 for the other bindings.
const txtRemoveSessionActive = "Não é possível remover: o jogo está em uma sessão ativa. Feche a sessão antes de remover."

func TestDeleteMessage(t *testing.T) {
	t.Parallel()

	t.Run("DSK-018/-32002 em DeleteGame mostra exatamente a mensagem de DSK-018, sem o título", func(t *testing.T) {
		t.Parallel()
		for _, agentErr := range []error{
			sessionActiveErr("Hollow Knight"),
			fmt.Errorf("desktop: game.delete: %w", sessionActiveErr("Hollow Knight")),
			rawDataErr(ipc.CodeSessionActive, ipc.MsgSessionActive, ""),
			rawDataErr(ipc.CodeSessionActive, ipc.MsgSessionActive, `"x"`),
		} {
			logs := newRecLog()
			p := &gamePinger{errs: map[string]error{ipc.MethodGameDelete: agentErr}}
			a := &App{client: p, log: logs.logger()}

			err := a.DeleteGame(3)

			wantErrText(t, err, txtRemoveSessionActive)
			if err != nil && strings.Contains(err.Error(), "Hollow Knight") {
				t.Errorf("DeleteGame err = %q, must not repeat the title", err)
			}
			if n := logs.countLevel(slog.LevelError); n != 0 {
				t.Errorf("DeleteGame(%v) logged %d Error records, want 0; log:\n%s", agentErr, n, logs.dump())
			}
		}
	})

	t.Run("DSK-018/demais códigos em DeleteGame seguem DSK-011", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			agentErr error
			want     string
		}{
			{gameNotFoundErr(), txtGameNotFound},
			{ipc.NewError(ipc.CodeInvalidParams, ipc.MsgInvalidParams), txtInvalidData},
			{ipc.ErrConnectionLost, txtNotResponding},
			{errors.New("boom"), txtUnexpected},
		}
		for _, tc := range cases {
			p := &gamePinger{errs: map[string]error{ipc.MethodGameDelete: tc.agentErr}}
			a := &App{client: p, log: discardLog()}

			wantErrText(t, a.DeleteGame(3), tc.want)
		}
	})
}

// TestElevationRequired covers ADM-003: a launch that fails with
// ERROR_ELEVATION_REQUIRED (740) tells the user to run the Arcadia as administrator.
func TestElevationRequired(t *testing.T) {
	t.Run("ADM-003/erro 740 pede que o Arcadia rode como administrador", func(t *testing.T) {
		raw := `{"error":"tracker: create process: requires elevation","winError":740}`
		wantText(t, rawDataErr(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed, raw),
			"Não foi possível iniciar o jogo: o Arcadia precisa estar como administrador. Feche-o e abra de novo com a elevação.")
	})

	t.Run("ADM-003/outro winError mantém a mensagem comum com o detalhe", func(t *testing.T) {
		raw := `{"error":"tracker: create process: not found","winError":2}`
		wantText(t, rawDataErr(ipc.CodeLaunchFailed, ipc.MsgLaunchFailed, raw),
			"Não foi possível iniciar o jogo: tracker: create process: not found")
	})
}
