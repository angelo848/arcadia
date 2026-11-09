package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// trayReadyTimeout is how long the tray loop waits for the systray onReady
// before logging that the tray did not initialize (the agent keeps serving IPC).
const trayReadyTimeout = 30 * time.Second

// trayItems carries the menu click channels from onReady to the tray loop.
type trayItems struct {
	open <-chan struct{}
	quit <-chan struct{}
}

// trayLoopDeps are the channels and callbacks of trayLoop.
type trayLoopDeps struct {
	Ready                 <-chan trayItems
	SessionEnd, IPCFailed <-chan struct{} // nil never fires
	Stop                  <-chan struct{}
	Quit, QuitBeforeReady func()
	OnOpen                func()
	OnToggle              func() bool // T07
	SetChecked            func(bool)  // T07
}

// trayLoop is the portable tray event loop; it returns why the agent is
// shutting down (EXE-016). Before the menu exists (Ready has not delivered
// yet) ctx being done, a signal on SessionEnd or a close of IPCFailed calls
// QuitBeforeReady (which must end the systray message loop without a tray
// window) and returns; if Ready does not deliver within trayReadyTimeout it
// logs an Error once and keeps waiting. After Ready delivers, "Abrir Arcadia"
// calls OnOpen synchronously, and "Sair", ctx done, SessionEnd or IPCFailed
// call Quit and return. A close of Stop (the systray loop ended on its own)
// returns ReasonOther without calling anything.
//
// Reasons: "Sair" -> quit; SessionEnd -> session-end; ctx (Ctrl+C/SIGTERM) ->
// signal; Stop and IPCFailed (EXE-026) -> other.
func trayLoop(ctx context.Context, clock domain.Clock, log *slog.Logger, d trayLoopDeps) ShutdownReason {
	var it trayItems
	timeout := clock.After(trayReadyTimeout)
	timedOut := false
waitReady:
	for {
		select {
		case it = <-d.Ready:
			if timedOut {
				log.Info("bandeja inicializada após o prazo")
			}
			break waitReady
		case <-timeout:
			timeout = nil
			timedOut = true
			log.Error("bandeja não inicializou", "timeout", trayReadyTimeout.String())
		case <-ctx.Done():
			trayQuitEarly(log, d, "sinal de encerramento")
			return ReasonSignal
		case <-d.SessionEnd:
			trayQuitEarly(log, d, "fim de sessão")
			return ReasonSessionEnd
		case <-d.IPCFailed:
			trayQuitEarly(log, d, "IPC indisponível")
			return ReasonOther
		case <-d.Stop:
			return ReasonOther
		}
	}

	for {
		select {
		case <-it.open:
			d.OnOpen()
		case <-it.quit:
			d.Quit()
			return ReasonQuit
		case <-ctx.Done():
			d.Quit()
			return ReasonSignal
		case <-d.SessionEnd:
			d.Quit()
			return ReasonSessionEnd
		case <-d.IPCFailed:
			d.Quit()
			return ReasonOther
		case <-d.Stop:
			return ReasonOther
		}
	}
}

// trayQuitEarly ends the tray when a quit condition arrives while waiting for
// Ready. If the menu became ready at the same moment, the regular Quit is used
// so the library removes its icon; otherwise QuitBeforeReady is called.
func trayQuitEarly(log *slog.Logger, d trayLoopDeps, reason string) {
	select {
	case <-d.Ready:
		d.Quit()
		return
	default:
	}
	log.Info("encerrando antes de a bandeja inicializar", "reason", reason)
	d.QuitBeforeReady()
}
