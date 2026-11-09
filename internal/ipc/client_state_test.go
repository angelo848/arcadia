package ipc_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// noSignal fails if StateChanged already holds a signal (or is closed).
func noSignal(t *testing.T, c *ipc.Client, when string) {
	t.Helper()
	select {
	case _, ok := <-c.StateChanged():
		if !ok {
			t.Fatalf("StateChanged() closed %s", when)
		}
		t.Fatalf("unexpected StateChanged signal %s", when)
	default:
	}
}

// awaitSignal waits for one StateChanged signal (guarded by guardTimeout).
func awaitSignal(t *testing.T, c *ipc.Client, what string) {
	t.Helper()
	select {
	case _, ok := <-c.StateChanged():
		if !ok {
			t.Fatalf("StateChanged() closed while waiting for %s", what)
		}
	case <-time.After(guardTimeout):
		t.Fatalf("timed out waiting for StateChanged signal: %s", what)
	}
}

func TestClientState(t *testing.T) {
	t.Parallel()

	t.Run("EXE-019/Connected falso antes do primeiro dial", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)

		if cap(c.StateChanged()) != 1 {
			t.Fatalf("cap(StateChanged()) = %d, want 1", cap(c.StateChanged()))
		}

		// The first dial is in progress and has not completed.
		a := fd.next(t)
		if c.Connected() {
			t.Fatal("Connected() = true while the first dial has not completed")
		}
		noSignal(t, c, "before any connection")

		// A failed dial is not a state change: still disconnected, no signal.
		a.fail()
		blockUntilWaiters(t, clk, 1) // client is in its backoff wait
		if c.Connected() {
			t.Fatal("Connected() = true after a failed dial")
		}
		noSignal(t, c, "after a failed dial")
	})

	t.Run("EXE-019/conectar sinaliza StateChanged e Connected fica verdadeiro", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)

		// First dial fails, then the backoff fires and the second one connects.
		fd.next(t).fail()
		blockUntilWaiters(t, clk, 1)
		noSignal(t, c, "after a failed dial")
		clk.Advance(250 * time.Millisecond)
		peer := fd.next(t).connect(t)

		awaitSignal(t, c, "connect")
		if !c.Connected() {
			t.Fatal("Connected() = false after the connect signal")
		}

		// The connection is really usable.
		var pong ipc.PingResult
		errc := callAsync(guardCtx(t), c, ipc.MethodPing, &pong)
		peer.respond(t, peer.readRequest(t), `{"pong":true}`)
		if err := recv(t, errc, "Call over the connection"); err != nil || !pong.Pong {
			t.Fatalf("Call = %+v, %v", pong, err)
		}
		if !c.Connected() {
			t.Fatal("Connected() = false while the connection is up")
		}
		noSignal(t, c, "while the connection stays up")
	})

	t.Run("EXE-019/queda sinaliza StateChanged e Connected fica falso", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)
		peer := fd.next(t).connect(t)
		awaitSignal(t, c, "connect")
		if !c.Connected() {
			t.Fatal("Connected() = false after the connect signal")
		}

		peer.conn.Close()
		awaitSignal(t, c, "drop")
		if c.Connected() {
			t.Fatal("Connected() = true after the drop signal")
		}

		// Still disconnected while the client waits its backoff.
		awaitBackoffAfterDrop(t, fd, clk)
		if c.Connected() {
			t.Fatal("Connected() = true during the backoff after the drop")
		}
		noSignal(t, c, "during the backoff after the drop")
	})

	t.Run("EXE-019/sinais seguidos se acumulam em um sem bloquear o cliente", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)

		// Nobody reads StateChanged: connect, drop, connect, drop (4 signals).
		peer := fd.next(t).connect(t)
		peer.conn.Close()
		awaitBackoffAfterDrop(t, fd, clk) // the client was not blocked by the signals
		clk.Advance(250 * time.Millisecond)
		peer = fd.next(t).connect(t)
		peer.conn.Close()
		// The drop signal is sent before the backoff wait, so once the wait is
		// registered every signal of this cycle has been emitted.
		awaitBackoffAfterDrop(t, fd, clk)

		// The client keeps going: the backoff fires and it dials again.
		clk.Advance(250 * time.Millisecond)
		a := fd.next(t)

		// Exactly one coalesced signal is pending.
		select {
		case _, ok := <-c.StateChanged():
			if !ok {
				t.Fatal("StateChanged() closed while the client is open")
			}
		default:
			t.Fatal("no StateChanged signal after 4 state changes")
		}
		noSignal(t, c, "after reading the coalesced signal")
		if c.Connected() {
			t.Fatal("Connected() = true while the third dial is in progress")
		}
		a.fail()
	})

	t.Run("EXE-019/notificação lida antes do EOF já está em Notifications ao observar a queda", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)
		peer := fd.next(t).connect(t)
		awaitSignal(t, c, "connect")

		// net.Pipe: notify returns once the client has read the bytes; the
		// connection is closed right after, so EOF follows the notification.
		peer.notify(t, ipc.MethodShutdown, `{"reason":"quit"}`)
		peer.conn.Close()

		awaitSignal(t, c, "drop")
		if c.Connected() {
			t.Fatal("Connected() = true after the drop signal")
		}
		select {
		case n, ok := <-c.Notifications():
			if !ok {
				t.Fatal("Notifications() closed while the client is open")
			}
			if n.Method != ipc.MethodShutdown {
				t.Fatalf("notification method = %q, want %q", n.Method, ipc.MethodShutdown)
			}
			var p ipc.ShutdownParams
			if err := json.Unmarshal(n.Params, &p); err != nil {
				t.Fatalf("notification params %q: %v", n.Params, err)
			}
			if p.Reason != "quit" {
				t.Fatalf("notification reason = %q, want %q", p.Reason, "quit")
			}
		default:
			t.Fatal("notification read before EOF not in Notifications() when the drop was signalled")
		}
	})

	t.Run("EXE-019/StateChanged é fechado por Close", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)
		fd.next(t).connect(t)
		awaitSignal(t, c, "connect")

		within(t, "Client.Close", func() { _ = c.Close() })

		// Close returned, so the channel is already closed; at most one
		// buffered signal (the drop caused by Close) may precede that.
		ch := c.StateChanged()
		for i := 0; ; i++ {
			select {
			case _, ok := <-ch:
				if !ok {
					if c.Connected() {
						t.Fatal("Connected() = true after Close")
					}
					return
				}
				if i >= 1 {
					t.Fatal("StateChanged() delivered more than one buffered signal after Close")
				}
			default:
				t.Fatal("StateChanged() not closed by Close")
			}
		}
	})
}
