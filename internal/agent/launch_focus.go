package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// launchFocus brings the window of a launched game to the foreground. The agent has no window and is
// never the foreground process, so a game it starts would otherwise open behind the other windows.
type launchFocus struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	log    *slog.Logger
	run    func(ctx context.Context, log *slog.Logger) // focusLaunchedWindow; tests replace it
}

func newLaunchFocus(parent context.Context, log *slog.Logger) *launchFocus {
	ctx, cancel := context.WithCancel(parent)
	return &launchFocus{ctx: ctx, cancel: cancel, log: log, run: focusLaunchedWindow}
}

// wrap runs h and, when the launch succeeded, focuses the new window in the background.
func (f *launchFocus) wrap(h ipc.Handler) ipc.Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		res, err := h(ctx, params)
		if err == nil && f.ctx.Err() == nil {
			f.wg.Add(1)
			go func() {
				defer f.wg.Done()
				f.run(f.ctx, f.log)
			}()
		}
		return res, err
	}
}

// Stop cancels the pending focus attempts and waits for them.
func (f *launchFocus) Stop() {
	f.cancel()
	f.wg.Wait()
}
