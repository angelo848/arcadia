package ipc_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// backoffSequence is the IPC-013 wait sequence (then 5 s forever).
var backoffSequence = []time.Duration{
	250 * time.Millisecond,
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
	5 * time.Second,
	5 * time.Second,
}

// The client clock is used only by the reconnect loop, so every pending waiter
// on it is a backoff wait.
func TestBackoff(t *testing.T) {
	t.Parallel()

	t.Run("IPC-013/esperas 250ms 500ms 1s 2s 4s 5s 5s", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		newClient(t, fd.Dial, clk)

		fd.next(t).fail() // first attempt is immediate
		// Each wait is exactly d: it has not fired at d-1ns and fires at d.
		for i, d := range backoffSequence {
			blockUntilWaiters(t, clk, 1)
			clk.Advance(d - time.Nanosecond)
			if !hasPendingWaiters(clk, 1) {
				t.Fatalf("wait #%d fired before %v", i+1, d)
			}
			fd.noAttempt(t, fmt.Sprintf("before wait #%d (%v) elapsed", i+1, d))
			clk.Advance(time.Nanosecond)
			fd.next(t).fail()
		}
	})

	t.Run("IPC-013/sem jitter (nada dispara 1ns antes)", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		newClient(t, fd.Dial, clk)

		fd.next(t).fail()
		// Extra 5 s entries: the cap holds with no randomization.
		seq := append(append([]time.Duration(nil), backoffSequence...), 5*time.Second, 5*time.Second, 5*time.Second)
		for i, d := range seq {
			blockUntilWaiters(t, clk, 1)
			clk.Advance(d - time.Nanosecond)
			if !hasPendingWaiters(clk, 1) {
				t.Fatalf("wait #%d fired before %v", i+1, d)
			}
			fd.noAttempt(t, "before the backoff elapsed")
			clk.Advance(time.Nanosecond)
			fd.next(t).fail()
		}
	})

	t.Run("IPC-013/volta a 250ms após conexão e nova queda", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)

		fd.next(t).fail()
		blockUntilWaiters(t, clk, 1)
		clk.Advance(250 * time.Millisecond)
		fd.next(t).fail()
		blockUntilWaiters(t, clk, 1)
		clk.Advance(500 * time.Millisecond)
		peer := fd.next(t).connect(t)

		// Prove the connection works.
		var pong ipc.PingResult
		errc := callAsync(guardCtx(t), c, ipc.MethodPing, &pong)
		peer.respond(t, peer.readRequest(t), `{"pong":true}`)
		if err := recv(t, errc, "Call over the new connection"); err != nil || !pong.Pong {
			t.Fatalf("Call = %+v, %v", pong, err)
		}

		// Drop: the backoff was reset by the connection, so the client waits
		// 250 ms (design.md, revisão da T07: no immediate redial).
		peer.conn.Close()
		awaitBackoffAfterDrop(t, fd, clk)
		clk.Advance(250*time.Millisecond - time.Nanosecond)
		if !hasPendingWaiters(clk, 1) {
			t.Fatal("wait after reconnect+drop was shorter than 250ms")
		}
		fd.noAttempt(t, "before 250ms after the drop")
		clk.Advance(time.Nanosecond)
		fd.next(t).fail()
	})

	t.Run("IPC-013/após queda espera 250ms antes do redial", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)
		peer := fd.next(t).connect(t) // first attempt connects

		var pong ipc.PingResult
		errc := callAsync(guardCtx(t), c, ipc.MethodPing, &pong)
		peer.respond(t, peer.readRequest(t), `{"pong":true}`)
		if err := recv(t, errc, "Call over the connection"); err != nil || !pong.Pong {
			t.Fatalf("Call = %+v, %v", pong, err)
		}

		peer.conn.Close()
		awaitBackoffAfterDrop(t, fd, clk) // no dial before the wait is registered
		clk.Advance(250*time.Millisecond - time.Nanosecond)
		if !hasPendingWaiters(clk, 1) {
			t.Fatal("wait after the drop was shorter than 250ms")
		}
		fd.noAttempt(t, "1ns before 250ms after the drop")
		clk.Advance(time.Nanosecond)
		fd.next(t).fail()
	})
}

func TestDialer(t *testing.T) {
	t.Parallel()

	t.Run("IPC-013/Dialer (nil, nil) é tratado como falha", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)
		fd.next(t).nilConn() // must not panic nor count as a connection

		// Treated as a failed dial: the client waits the first backoff (250 ms).
		blockUntilWaiters(t, clk, 1)

		// No connection was published, so a Call waits and ends with its ctx.
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := recv(t, callAsync(ctx, c, ipc.MethodPing, nil), "Call after a (nil, nil) dial")
		if err == nil || !errors.Is(err, ctx.Err()) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Call error = %v, want errors.Is(err, ctx.Err()) (%v)", err, ctx.Err())
		}

		clk.Advance(250*time.Millisecond - time.Nanosecond)
		if !hasPendingWaiters(clk, 1) {
			t.Fatal("wait after a (nil, nil) dial was shorter than 250ms")
		}
		fd.noAttempt(t, "before 250ms after a (nil, nil) dial")
		clk.Advance(time.Nanosecond)
		fd.next(t).fail()
	})
}

func TestCallWaits(t *testing.T) {
	t.Parallel()

	t.Run("IPC-014/Call enviada quando o servidor sobe antes do prazo", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)
		fd.next(t).fail() // disconnected

		var pong ipc.PingResult
		errc := callAsync(guardCtx(t), c, ipc.MethodPing, &pong)

		// The client is in its backoff wait and has not redialed, so no
		// connection exists while the Call starts. There is no public hook to
		// prove the Call goroutine has already reached its wait (the only clock
		// waiter is the backoff, and Call blocks on internal channels), so the
		// "Call waits before connect" ordering is best effort: the Call is
		// started before the backoff fires and must not have returned yet.
		blockUntilWaiters(t, clk, 1)
		fd.noAttempt(t, "before the backoff elapsed")
		select {
		case err := <-errc:
			t.Fatalf("Call returned before any connection: %v", err)
		default:
		}
		clk.Advance(250 * time.Millisecond)
		peer := fd.next(t).connect(t)
		req := peer.readRequest(t)
		if req.Method != ipc.MethodPing {
			t.Fatalf("sent method %q, want %q", req.Method, ipc.MethodPing)
		}
		peer.respond(t, req, `{"pong":true}`)
		if err := recv(t, errc, "Call"); err != nil || !pong.Pong {
			t.Fatalf("Call = %+v, %v", pong, err)
		}
	})

	t.Run("IPC-014/ctx expira antes de conectar retorna errors.Is ctx.Err()", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)
		fd.next(t).fail() // disconnected; nobody advances the clock

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := recv(t, callAsync(ctx, c, ipc.MethodPing, nil), "Call with expiring ctx")
		if err == nil {
			t.Fatal("Call succeeded without a connection")
		}
		if !errors.Is(err, ctx.Err()) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Call error = %v, want errors.Is(err, ctx.Err()) (%v)", err, ctx.Err())
		}
	})

	t.Run("IPC-014/servidor que não lê: Call retorna errors.Is ctx.Err() no prazo", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		clk := domain.NewFakeClock(testStart)
		c := newClient(t, fd.Dial, clk)
		peer := fd.next(t).connect(t)

		// The connection is up (a call round-trips), so the next Call goes to it.
		var pong ipc.PingResult
		errc := callAsync(guardCtx(t), c, ipc.MethodPing, &pong)
		peer.respond(t, peer.readRequest(t), `{"pong":true}`)
		if err := recv(t, errc, "first Call"); err != nil || !pong.Pong {
			t.Fatalf("first Call = %+v, %v", pong, err)
		}

		// The server stops reading: net.Pipe writes block until read.
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := recv(t, callAsync(ctx, c, ipc.MethodStatus, nil), "Call while the server does not read")
		if err == nil || !errors.Is(err, ctx.Err()) || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Call error = %v, want errors.Is(err, ctx.Err()) (%v)", err, ctx.Err())
		}

		// The connection survives: no backoff wait, no redial.
		if hasPendingWaiters(clk, 1) {
			t.Fatal("client entered backoff: the abandoned Call dropped the connection")
		}
		fd.noAttempt(t, "after a Call gave up while sending")

		// The server reads again: a new Call on the same connection works. The
		// abandoned status request may still arrive first if its write had
		// already started (design.md only forbids sending requests whose caller
		// gave up before they were written), so it is skipped.
		pong = ipc.PingResult{}
		errc = callAsync(guardCtx(t), c, ipc.MethodPing, &pong)
		req := peer.readRequest(t)
		if req.Method == ipc.MethodStatus {
			req = peer.readRequest(t)
		}
		if req.Method != ipc.MethodPing {
			t.Fatalf("server read method %q, want %q", req.Method, ipc.MethodPing)
		}
		peer.respond(t, req, `{"pong":true}`)
		if err := recv(t, errc, "Call after the server reads again"); err != nil || !pong.Pong {
			t.Fatalf("Call after the server reads again = %+v, %v", pong, err)
		}
	})
}

func TestInFlight(t *testing.T) {
	t.Parallel()

	t.Run("IPC-015/queda com chamada pendente retorna ErrConnectionLost", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		c := newClient(t, fd.Dial, domain.NewFakeClock(testStart))
		peer := fd.next(t).connect(t)

		errc := callAsync(guardCtx(t), c, ipc.MethodStatus, nil)
		peer.readRequest(t) // the call is on the wire
		peer.conn.Close()

		err := recv(t, errc, "pending Call after the drop")
		if !errors.Is(err, ipc.ErrConnectionLost) {
			t.Fatalf("Call error = %v, want ErrConnectionLost", err)
		}
	})
}

func TestAfterClose(t *testing.T) {
	t.Parallel()

	t.Run("IPC-016/Call após Close retorna ErrClientClosed", func(t *testing.T) {
		t.Parallel()
		fd := newFakeDialer()
		c := newClient(t, fd.Dial, domain.NewFakeClock(testStart))
		within(t, "Client.Close", func() { _ = c.Close() })

		err := c.Call(guardCtx(t), ipc.MethodPing, nil, nil)
		if !errors.Is(err, ipc.ErrClientClosed) {
			t.Fatalf("Call after Close = %v, want ErrClientClosed", err)
		}
	})
}

func TestNotificationBuffer(t *testing.T) {
	t.Parallel()

	fd := newFakeDialer()
	c := newClient(t, fd.Dial, domain.NewFakeClock(testStart))
	peer := fd.next(t).connect(t)

	// Nobody reads Notifications(): 20 arrive, then a call on the same connection.
	for i := 1; i <= 20; i++ {
		peer.notify(t, ipc.MethodHeartbeat, `{"uptimeSeconds":`+strconv.Itoa(i)+`}`)
	}
	var pong ipc.PingResult
	errc := callAsync(guardCtx(t), c, ipc.MethodPing, &pong)
	peer.respond(t, peer.readRequest(t), `{"pong":true}`)
	callErr := recv(t, errc, "Call after 20 notifications")
	// The response is read after the 20 notifications, so all were handled by now.

	t.Run("IPC-017/canal contém as 16 primeiras", func(t *testing.T) {
		ch := c.Notifications()
		if cap(ch) != ipc.NotificationBuffer {
			t.Errorf("cap(Notifications()) = %d, want %d", cap(ch), ipc.NotificationBuffer)
		}
		var got []int64
	drain:
		for {
			select {
			case n, ok := <-ch:
				if !ok {
					t.Fatal("Notifications() closed while the client is open")
				}
				if n.Method != ipc.MethodHeartbeat {
					t.Fatalf("notification method = %q", n.Method)
				}
				var p ipc.HeartbeatParams
				if err := json.Unmarshal(n.Params, &p); err != nil {
					t.Fatalf("notification params %q: %v", n.Params, err)
				}
				got = append(got, p.UptimeSeconds)
			default:
				break drain
			}
		}
		if len(got) != 16 {
			t.Fatalf("buffered %d notifications (%v), want 16", len(got), got)
		}
		for i, v := range got {
			if v != int64(i+1) {
				t.Fatalf("notifications = %v, want 1..16 in order", got)
			}
		}
	})

	t.Run("IPC-017/Call segue respondida na mesma conexão", func(t *testing.T) {
		if callErr != nil || !pong.Pong {
			t.Fatalf("Call with a full notification buffer = %+v, %v", pong, callErr)
		}
		// Still the same connection: no new dial, and another call works.
		fd.noAttempt(t, "after notifications were dropped")
		pong = ipc.PingResult{}
		errc := callAsync(guardCtx(t), c, ipc.MethodPing, &pong)
		peer.respond(t, peer.readRequest(t), `{"pong":true}`)
		if err := recv(t, errc, "second Call"); err != nil || !pong.Pong {
			t.Fatalf("second Call = %+v, %v", pong, err)
		}
	})
}
