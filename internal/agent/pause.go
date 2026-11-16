package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
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

// pauseTracker is the part of *tracker.Tracker used by the power bridge.
type pauseTracker interface {
	TogglePause(ctx context.Context, reason domain.PauseReason, at time.Time) (tracker.PauseResult, error)
	Suspend(ctx context.Context, at time.Time) error
	Wake(ctx context.Context, at time.Time, userPresent bool) error
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
