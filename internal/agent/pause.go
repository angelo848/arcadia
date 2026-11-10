package agent

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/input"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// triggerQueueSize is the buffer of the channel the input sources write to.
const triggerQueueSize = 8

// toggleTimeout bounds the wait of one TogglePause call for the tracker's
// answer [PAU-003]. It is an I/O bound, measured on the real clock.
const toggleTimeout = time.Second

// PowerHandlerTimeout bounds the wait of the power bridge for the tracker's
// answer to a suspension or a wake-up [PWR-005]. It is an I/O bound, measured on
// the real clock.
const PowerHandlerTimeout = 1500 * time.Millisecond

// pauseTracker is the part of *tracker.Tracker used by the pause router and the
// power bridge.
type pauseTracker interface {
	TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (tracker.PauseResult, error)
	Suspend(ctx context.Context, at time.Time) error
	Wake(ctx context.Context, at time.Time, userPresent bool) error
}

// pauseRouter forwards the triggers of the input sources to the tracker
// [PAU-001..PAU-005]. A single goroutine owns it: it reads in until stop is
// closed.
type pauseRouter struct {
	tr  pauseTracker
	in  <-chan input.Trigger
	log *slog.Logger

	stop      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once

	// Owned by the router goroutine. haveLast is false until the first trigger
	// is forwarded.
	haveLast bool
	lastAt   time.Time
}

// newPauseRouter returns a router that reads triggers from in and applies them
// to tr. Start begins reading.
func newPauseRouter(tr pauseTracker, in <-chan input.Trigger, log *slog.Logger) *pauseRouter {
	return &pauseRouter{tr: tr, in: in, log: log, stop: make(chan struct{})}
}

// Start launches the router goroutine. It is called once.
func (r *pauseRouter) Start() {
	r.wg.Add(1)
	go r.run()
}

// Close stops the router and waits for its goroutine to end. It is idempotent.
func (r *pauseRouter) Close() {
	r.closeOnce.Do(func() { close(r.stop) })
	r.wg.Wait()
}

func (r *pauseRouter) run() {
	defer r.wg.Done()
	for {
		select {
		case <-r.stop:
			return
		case trig := <-r.in:
			r.handle(trig)
		}
	}
}

// handle applies the anti-bounce window and forwards the trigger when it passes
// [PAU-004]. The window is measured from Trigger.At against the last forwarded
// trigger, whatever its source; dropped triggers do not move it.
func (r *pauseRouter) handle(trig input.Trigger) {
	if r.haveLast {
		gap := trig.At.Sub(r.lastAt)
		if gap < 0 {
			gap = -gap
		}
		if gap < domain.PauseToggleDebounce {
			return
		}
	}
	r.haveLast = true
	r.lastAt = trig.At
	r.forward(trig)
}

// forward calls TogglePause and logs the outcome [PAU-001..PAU-003, PAU-005].
func (r *pauseRouter) forward(trig input.Trigger) {
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

// powerBridge passes the suspension and wake-up events of the PC to the tracker
// [PWR-005]. Each call waits at most timeout for the tracker.
type powerBridge struct {
	tr      pauseTracker
	timeout time.Duration // PowerHandlerTimeout; testes encurtam
	log     *slog.Logger
}

// OnSuspend tells the tracker that the PC is going to sleep at the given
// instant.
func (b *powerBridge) OnSuspend(at time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	err := b.tr.Suspend(ctx, at)
	switch {
	case err == nil:
	case errors.Is(err, context.DeadlineExceeded):
		b.log.Warn("tracker não respondeu à suspensão a tempo", "timeout", b.timeout.String())
	case errors.Is(err, tracker.ErrShuttingDown):
		b.log.Debug("agente encerrando; suspensão não registrada")
	default:
		b.log.Warn("falha ao registrar suspensão", "error", err.Error())
	}
}

// OnWake tells the tracker that the PC woke up at the given instant.
func (b *powerBridge) OnWake(at time.Time, userPresent bool) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	if err := b.tr.Wake(ctx, at, userPresent); err != nil {
		b.log.Debug("retomada do PC não registrada", "error", err.Error())
	}
}
