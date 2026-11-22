package agent

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/feedback"
	"github.com/arcadia-launcher/arcadia/internal/input"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// closeTimeout bounds the wait of one CloseByHotkey call [CON-004, CON-021]. It equals
// x-console.closeHandlerTimeoutMs of the IPC contract.
const closeTimeout = 6 * time.Second

// routerTracker is the part of *tracker.Tracker used by the trigger router.
type routerTracker interface {
	TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (tracker.PauseResult, error)
	CloseByHotkey(ctx context.Context, at time.Time) (tracker.CloseResult, error)
	Current() (domain.SessionSnapshot, bool)
}

// feedbackSender is the part of the feedback coordinator used by the router.
type feedbackSender interface{ Send(feedback.Event) }

// closeSink receives the game.closedByHotkey notification [CON-019].
type closeSink interface{ GameClosedByHotkey(gameID int64) }

// triggerRouter forwards the triggers of the input sources to the tracker, by action
// [PAU-001..PAU-005, CON-004]. A single goroutine owns it.
type triggerRouter struct {
	tr   routerTracker
	in   <-chan input.Trigger
	log  *slog.Logger
	fb   feedbackSender
	sink closeSink

	opener       bigBoxOpener
	shuttingDown func() bool

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	// Owned by the router goroutine; one window per action.
	havePause, haveClose bool
	lastPause, lastClose time.Time
	haveBigBox           bool
	lastBigBox           time.Time
}

func newTriggerRouter(tr routerTracker, in <-chan input.Trigger, log *slog.Logger, fb feedbackSender, sink closeSink, opener bigBoxOpener, shuttingDown func() bool) *triggerRouter {
	ctx, cancel := context.WithCancel(context.Background())
	if shuttingDown == nil {
		shuttingDown = func() bool { return false }
	}
	return &triggerRouter{tr: tr, in: in, log: log, fb: fb, sink: sink, opener: opener, shuttingDown: shuttingDown, ctx: ctx, cancel: cancel}
}

// Start launches the router goroutine. It is called once.
func (r *triggerRouter) Start() {
	r.wg.Add(1)
	go r.run()
}

// Close cancels pending calls, stops the router and waits for its goroutine. Idempotent.
func (r *triggerRouter) Close() {
	r.closeOnce.Do(r.cancel)
	r.wg.Wait()
}

func (r *triggerRouter) run() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		case trig := <-r.in:
			if r.ctx.Err() != nil {
				return
			}
			r.handle(trig)
		}
	}
}

func gapOK(have bool, last, at time.Time, window time.Duration) bool {
	if !have {
		return true
	}
	gap := at.Sub(last)
	if gap < 0 {
		gap = -gap
	}
	return gap >= window
}

// forwardBigBox opens the Big Box unless a game session is open (ATB-004..ATB-007).
func (r *triggerRouter) forwardBigBox() {
	if r.shuttingDown() || r.ctx.Err() != nil {
		r.log.Debug("agente encerrando; gatilho do Big Box descartado")
		return
	}
	// Current reports only a live session; any snapshot it returns blocks the Big Box.
	if _, ok := r.tr.Current(); ok {
		r.log.Debug("Big Box ignorado: jogo em execução")
		r.fb.Send(feedback.EventRefused)
		return
	}
	if err := r.opener.OpenBigBox(); err != nil {
		path := ""
		if p, ok := r.opener.(interface{ Path() string }); ok {
			path = p.Path()
		}
		r.log.Warn("falha ao abrir o Big Box", "path", path, "error", err.Error())
		r.fb.Send(feedback.EventRefused)
		return
	}
	r.log.Info("Big Box pedido pelo controle")
}

func (r *triggerRouter) handle(trig input.Trigger) {
	if trig.Action == input.ActionBigBox {
		if !gapOK(r.haveBigBox, r.lastBigBox, trig.At, domain.BigBoxDebounce) {
			return
		}
		r.haveBigBox, r.lastBigBox = true, trig.At
		r.forwardBigBox()
		return
	}
	if trig.Action == input.ActionClose {
		if !gapOK(r.haveClose, r.lastClose, trig.At, domain.CloseDebounce) {
			return
		}
		r.haveClose, r.lastClose = true, trig.At
		r.forwardClose(trig)
		return
	}
	// PAU-004: the window is measured against the last forwarded pause trigger.
	if !gapOK(r.havePause, r.lastPause, trig.At, domain.PauseToggleDebounce) {
		return
	}
	r.havePause, r.lastPause = true, trig.At
	r.forwardPause(trig)
}

// forwardPause calls TogglePause and logs the outcome [PAU-001..PAU-003, PAU-005].
func (r *triggerRouter) forwardPause(trig input.Trigger) {
	ctx, cancel := context.WithTimeout(context.Background(), toggleTimeout)
	defer cancel()
	res, err := r.tr.TogglePause(ctx, trig.Source, trig.At)
	switch {
	case errors.Is(err, tracker.ErrShuttingDown):
		r.log.Debug("agente encerrando; gatilho de pausa descartado", "source", string(trig.Source))
	case err != nil:
		r.log.Warn("falha ao alternar pausa", "source", string(trig.Source), "error", err.Error())
	case res == tracker.PauseIgnored:
		r.log.Debug("gatilho de pausa ignorado", "source", string(trig.Source))
	}
}

// forwardClose calls CloseByHotkey and turns the outcome into sound and notification
// [CON-012, CON-014, CON-015, CON-019, CON-021].
func (r *triggerRouter) forwardClose(trig input.Trigger) {
	ctx, cancel := context.WithTimeout(r.ctx, closeTimeout)
	defer cancel()
	res, err := r.tr.CloseByHotkey(ctx, trig.At)
	switch {
	case errors.Is(err, tracker.ErrShuttingDown):
		r.log.Debug("agente encerrando; gatilho de fechar descartado")
		return
	case errors.Is(err, context.DeadlineExceeded):
		r.log.Warn("tracker não respondeu ao fechar a tempo", "timeout", closeTimeout.String())
		return
	case errors.Is(err, context.Canceled):
		r.log.Debug("fechar cancelado pelo encerramento do roteador")
		return
	case err != nil:
		r.log.Warn("falha ao fechar o jogo", "error", err.Error())
		return
	}
	switch res.Kind {
	case tracker.CloseClosed:
		r.sink.GameClosedByHotkey(res.GameID)
	case tracker.CloseRefused:
		if res.Reason == tracker.CloseReasonClosing {
			r.log.Debug("fechar ignorado", "reason", res.Reason)
			return
		}
		r.fb.Send(feedback.EventRefused)
	case tracker.CloseFailed:
		r.fb.Send(feedback.EventRefused)
	}
}
