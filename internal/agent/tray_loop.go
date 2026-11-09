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

// trayLoop is the portable tray event loop. Before the menu exists (ready has
// not delivered yet) ctx being done or a signal on sessionEnd calls
// quitBeforeReady (which must end the systray message loop without a tray
// window) and returns; if ready does not deliver within trayReadyTimeout it logs
// an Error once and keeps waiting. After ready delivers, "Abrir Arcadia" calls
// onOpen synchronously, and "Sair", ctx done or sessionEnd call quit and
// return. A close of stop (the systray loop ended on its own) returns without
// calling anything. A nil sessionEnd never fires.
func trayLoop(ctx context.Context, clock domain.Clock, log *slog.Logger,
	ready <-chan trayItems, sessionEnd <-chan struct{}, stop <-chan struct{},
	quit, quitBeforeReady, onOpen func()) {
	var it trayItems
	timeout := clock.After(trayReadyTimeout)
	timedOut := false
waitReady:
	for {
		select {
		case it = <-ready:
			if timedOut {
				log.Info("bandeja inicializada após o prazo")
			}
			break waitReady
		case <-timeout:
			timeout = nil
			timedOut = true
			log.Error("bandeja não inicializou", "timeout", trayReadyTimeout.String())
		case <-ctx.Done():
			trayQuitEarly(log, ready, "sinal de encerramento", quit, quitBeforeReady)
			return
		case <-sessionEnd:
			trayQuitEarly(log, ready, "fim de sessão", quit, quitBeforeReady)
			return
		case <-stop:
			return
		}
	}

	for {
		select {
		case <-it.open:
			onOpen()
		case <-it.quit:
			quit()
			return
		case <-ctx.Done():
			quit()
			return
		case <-sessionEnd:
			quit()
			return
		case <-stop:
			return
		}
	}
}

// trayQuitEarly ends the tray when a quit condition arrives while waiting for
// ready. If the menu became ready at the same moment, the regular quit is used
// so the library removes its icon; otherwise quitBeforeReady is called.
func trayQuitEarly(log *slog.Logger, ready <-chan trayItems, reason string, quit, quitBeforeReady func()) {
	select {
	case <-ready:
		quit()
		return
	default:
	}
	log.Info("encerrando antes de a bandeja inicializar", "reason", reason)
	quitBeforeReady()
}
