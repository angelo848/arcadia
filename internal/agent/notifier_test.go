package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// nextMediaChangedID waits up to d for the next media.changed on c (other notifications are
// skipped) and returns its gameId.
func nextMediaChangedID(t *testing.T, c *ipc.Client, d time.Duration) (int64, bool) {
	t.Helper()
	timeout := time.After(d)
	for {
		select {
		case n, ok := <-c.Notifications():
			if !ok {
				return 0, false
			}
			if n.Method != ipc.MethodMediaChanged {
				continue
			}
			var p ipc.MediaChangedParams
			if err := json.Unmarshal(n.Params, &p); err != nil {
				t.Fatalf("media.changed params %s: %v", n.Params, err)
			}
			return p.GameID, true
		case <-timeout:
			return 0, false
		}
	}
}

func TestMediaNotifier(t *testing.T) {
	t.Run("IMG-001/mediaNotifier sem servidor e depois de set(nil) não faz nada e não bloqueia", func(t *testing.T) {
		returns := func(fn func()) bool {
			done := make(chan struct{})
			go func() { defer close(done); fn() }()
			select {
			case <-done:
				return true
			case <-time.After(testTimeout):
				return false
			}
		}

		var n mediaNotifier
		if !returns(func() { n.MediaChanged(1) }) {
			t.Fatal("MediaChanged before set blocked")
		}

		srv := ipc.NewServer(testClock(), map[string]ipc.Handler{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		ctx, cancel := context.WithCancel(context.Background())
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); _ = srv.Serve(ctx, ln) }()
		t.Cleanup(func() {
			cancel()
			_ = srv.Close()
			wg.Wait()
		})
		c := newTCPClient(ln.Addr().String())
		t.Cleanup(func() { _ = c.Close() })
		cctx, ccancel := context.WithTimeout(context.Background(), testTimeout)
		defer ccancel()
		if err := c.Call(cctx, ipc.MethodMediaList, nil, nil); err == nil {
			t.Log("unexpected success on a server without handlers; connection is established anyway")
		}

		n.set(srv)
		if !returns(func() { n.MediaChanged(7) }) {
			t.Fatal("MediaChanged with server blocked")
		}
		if id, ok := nextMediaChangedID(t, c, testTimeout); !ok || id != 7 {
			t.Fatalf("after set(srv): got (%d, %v), want media.changed {gameId: 7}", id, ok)
		}

		n.set(nil)
		if !returns(func() { n.MediaChanged(8) }) {
			t.Fatal("MediaChanged after set(nil) blocked")
		}
		if id, ok := nextMediaChangedID(t, c, 200*time.Millisecond); ok {
			t.Fatalf("after set(nil): got media.changed {gameId: %d}, want nothing", id)
		}
	})
}
