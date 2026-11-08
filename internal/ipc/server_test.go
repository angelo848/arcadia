package ipc_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
	"go.uber.org/goleak"
)

// TestServerClient drives the real Server and Client over a loopback TCP
// listener (no external network).
func TestServerClient(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	srvClock := domain.NewFakeClock(testStart)
	startServer(t, ln, srvClock, ipc.AgentHandlers(testInfo(), srvClock), nil, ipc.WithStartedAt(testStart))

	dial := func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
	dialTCPRaw := func(t *testing.T) *rawConn {
		t.Helper()
		conn, err := dial(guardCtx(t))
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return newRawConn(t, conn)
	}

	t.Run("IPC-001/ping e status ponta a ponta", func(t *testing.T) {
		c := newClient(t, dial, domain.NewFakeClock(testStart))

		var pong ipc.PingResult
		if err := c.Call(guardCtx(t), ipc.MethodPing, nil, &pong); err != nil {
			t.Fatalf("Call ping: %v", err)
		}
		if !pong.Pong {
			t.Fatalf("ping result = %+v, want pong=true", pong)
		}

		// The result travels as a JSON object, not as a base64 string.
		var raw json.RawMessage
		if err := c.Call(guardCtx(t), ipc.MethodPing, nil, &raw); err != nil {
			t.Fatalf("Call ping (raw): %v", err)
		}
		if got := compactJSON(t, raw); got != `{"pong":true}` {
			t.Fatalf("raw ping result = %s, want {\"pong\":true}", got)
		}

		// Uptime comes from the server clock (the heartbeat loop is waiting on it).
		blockUntilWaiters(t, srvClock, 1)
		srvClock.Advance(3 * time.Second)

		var st ipc.AgentStatus
		if err := c.Call(guardCtx(t), ipc.MethodStatus, nil, &st); err != nil {
			t.Fatalf("Call status: %v", err)
		}
		info := testInfo()
		if st.Version != info.Version || st.PID != info.PID || st.DataDir != info.DataDir {
			t.Fatalf("status = %+v, want info %+v", st, info)
		}
		if !st.StartedAt.Equal(info.StartedAt) {
			t.Fatalf("status startedAt = %v, want %v", st.StartedAt, info.StartedAt)
		}
		if st.UptimeSeconds != 3 {
			t.Fatalf("status uptimeSeconds = %d, want 3", st.UptimeSeconds)
		}
	})

	t.Run("IPC-010/conexão segue utilizável após batch", func(t *testing.T) {
		r := dialTCPRaw(t)
		for i, batch := range []string{`[]`, `[{"jsonrpc":"2.0","id":1,"method":"agent.ping"}]`} {
			r.send(t, batch)
			d := decodeResponse(t, r.readLine(t))
			if !d.hasErr || d.code != ipc.CodeInvalidRequest || d.id != "null" {
				t.Fatalf("batch %s: got err=%v code=%d id=%s, want -32600 id null", batch, d.hasErr, d.code, d.id)
			}
			r.ping(t, 100+i)
		}
	})

	t.Run("IPC-011/chamada com id após notificação é respondida", func(t *testing.T) {
		r := dialTCPRaw(t)
		r.send(t, `{"jsonrpc":"2.0","method":"agent.ping"}`)
		r.send(t, `{"jsonrpc":"2.0","method":"no.such.method"}`)
		// The next line read must be the response to id 7: notifications got none.
		r.ping(t, 7)
	})

	t.Run("IPC-012/mensagem de 1 MiB exato respondida ponta a ponta", func(t *testing.T) {
		r := dialTCPRaw(t)
		r.send(t, exactSizeMessage(t, 12, ipc.MaxMessageSize))
		assertPong(t, r.readLine(t), 12)
		r.ping(t, 13)
	})
}

func TestServerLimits(t *testing.T) {
	t.Parallel()

	ln := newMemListener()
	srvClock := domain.NewFakeClock(testStart)
	log, logs := newCaptureLogger()
	startServer(t, ln, srvClock, testHandlers(), log)

	a := dialRaw(t, ln)
	b := dialRaw(t, ln)
	a.ping(t, 1)
	b.ping(t, 1)

	t.Run("IPC-003/mensagem acima do limite fecha só aquela conexão", func(t *testing.T) {
		oversized := exactSizeMessage(t, 2, ipc.MaxMessageSize+1)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			// The server may close mid-write; the write error is expected.
			_ = a.conn.SetWriteDeadline(time.Now().Add(guardTimeout))
			_, _ = a.conn.Write([]byte(oversized + "\n"))
		}()
		// A response is optional; the connection must end.
		var err error
		for range 3 {
			if _, err = a.next(); err != nil {
				break
			}
		}
		wg.Wait()
		if err == nil {
			t.Fatal("oversized connection still open after 3 lines")
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			t.Fatalf("server did not close the oversized connection: %v", err)
		}
		// design.md (lacunas da T07): closing for an oversized message logs at Warn.
		// The connection is closed after the log, so it is already recorded.
		if !logs.containsAt(slog.LevelWarn, "", "") {
			t.Fatalf("no Warn log for the oversized message; logs: %v", logs.all())
		}
	})

	t.Run("IPC-003/outro cliente continua respondendo", func(t *testing.T) {
		b.ping(t, 3)
	})

	t.Run("IPC-003/nova conexão é aceita", func(t *testing.T) {
		c := dialRaw(t, ln)
		c.ping(t, 4)
	})
}

func TestHeartbeat(t *testing.T) {
	t.Parallel()

	ln := newMemListener()
	srvClock := domain.NewFakeClock(testStart)
	// startedAt 10 s before the clock start: uptime = 10 + elapsed.
	startServer(t, ln, srvClock, testHandlers(), nil, ipc.WithStartedAt(testStart.Add(-10*time.Second)))

	conns := []*rawConn{dialRaw(t, ln), dialRaw(t, ln)}
	for _, c := range conns {
		c.ping(t, 1) // the server is serving both connections
	}

	t.Run("IPC-004/nada antes de 5 s", func(t *testing.T) {
		blockUntilWaiters(t, srvClock, 1)
		srvClock.Advance(ipc.HeartbeatInterval - time.Nanosecond)
		if !hasPendingWaiters(srvClock, 1) {
			t.Fatal("heartbeat timer fired before 5 s")
		}
		for _, c := range conns {
			c.ping(t, 2) // first line is the ping response, no heartbeat
		}
	})

	t.Run("IPC-004/heartbeat em 5 s com uptimeSeconds", func(t *testing.T) {
		srvClock.Advance(time.Nanosecond)
		for _, line := range readEach(t, conns) {
			assertHeartbeat(t, line, 15)
		}
	})

	t.Run("IPC-004/repete a cada 5 s para todas as conexões", func(t *testing.T) {
		for k := int64(1); k <= 3; k++ {
			blockUntilWaiters(t, srvClock, 1)
			srvClock.Advance(ipc.HeartbeatInterval)
			for _, line := range readEach(t, conns) {
				assertHeartbeat(t, line, 15+5*k)
			}
		}
	})

	t.Run("IPC-004/conexão que não lê não atrasa as outras nem trava o Close", func(t *testing.T) {
		ln := newMemListener()
		clk := domain.NewFakeClock(testStart)
		r := startServer(t, ln, clk, testHandlers(), nil, ipc.WithStartedAt(testStart))

		// The slow connection sends one request and never reads. net.Pipe
		// writes complete only when the server reads, so once send returns the
		// server has registered it; its reply then blocks the server's write.
		slow := dialRaw(t, ln)
		slow.send(t, `{"jsonrpc":"2.0","id":1,"method":"agent.ping"}`)
		normal := []*rawConn{dialRaw(t, ln), dialRaw(t, ln)}
		for _, c := range normal {
			c.ping(t, 1)
		}

		// More heartbeats than any per-connection queue (16) could absorb.
		const beats = 20
		for k := int64(1); k <= beats; k++ {
			blockUntilWaiters(t, clk, 1)
			clk.Advance(ipc.HeartbeatInterval)
			for _, line := range readEach(t, normal) {
				assertHeartbeat(t, line, 5*k)
			}
		}

		r.stop(t) // Server.Close and Serve must return within guardTimeout
	})
}

func TestServerClose(t *testing.T) {
	t.Parallel()

	t.Run("IPC-006/Close antes de Serve faz Serve retornar nil", func(t *testing.T) {
		t.Parallel()
		srv := ipc.NewServer(domain.NewFakeClock(testStart), testHandlers(), discardLogger())
		within(t, "Server.Close before Serve", func() {
			if err := srv.Close(); err != nil {
				t.Errorf("Server.Close before Serve = %v", err)
			}
		})

		ln := newMemListener()
		t.Cleanup(func() { ln.Close() })
		var serveErr error
		within(t, "Serve after Close", func() {
			serveErr = srv.Serve(context.Background(), ln)
		})
		if serveErr != nil {
			t.Fatalf("Serve after Close = %v, want nil (shutdown was requested)", serveErr)
		}
	})
}

// TestServerHandlerError covers the T07 design note: a non-*Error handler error
// is logged by the server and reaches the client only as -32603 "Internal error".
func TestServerHandlerError(t *testing.T) {
	t.Parallel()

	const secret = "boom: secret detail 7f3a"
	handlers := testHandlers()
	handlers["test.fail"] = func(context.Context, json.RawMessage) (any, error) {
		return nil, errors.New(secret)
	}
	log, logs := newCaptureLogger()
	ln := newMemListener()
	startServer(t, ln, domain.NewFakeClock(testStart), handlers, log)

	t.Run("IPC-002/erro genérico de handler é logado e vira -32603 Internal error", func(t *testing.T) {
		r := dialRaw(t, ln)
		r.send(t, `{"jsonrpc":"2.0","id":1,"method":"test.fail"}`)
		line := r.readLine(t)
		d := decodeResponse(t, line)
		if !d.hasErr || d.code != ipc.CodeInternalError || d.msg != "Internal error" || d.id != "1" {
			t.Fatalf("response = %q, want -32603 \"Internal error\" id 1", line)
		}
		if strings.Contains(string(line), "secret detail") {
			t.Fatalf("original error text leaked to the client: %q", line)
		}
		if !logs.contains(secret) {
			t.Fatalf("server did not log the handler error %q; logs: %v", secret, logs.all())
		}
		if !logs.containsAt(slog.LevelError, "ipc: handler failed", secret) {
			t.Fatalf("handler error not logged at Error as \"ipc: handler failed\"; logs: %v", logs.all())
		}
		r.ping(t, 2)
	})
}

func TestReconnect(t *testing.T) {
	t.Parallel()

	t.Run("IPC-005/servidor reinicia e cliente volta a responder", func(t *testing.T) {
		ln1 := newMemListener()
		srv1 := startServer(t, ln1, domain.NewFakeClock(testStart), testHandlers(), nil)
		sd := &switchDialer{ln: ln1}
		cliClock := domain.NewFakeClock(testStart)
		c := newClient(t, sd.Dial, cliClock)

		var pong ipc.PingResult
		if err := c.Call(guardCtx(t), ipc.MethodPing, nil, &pong); err != nil || !pong.Pong {
			t.Fatalf("first ping: %+v, %v", pong, err)
		}

		// Restart: the new server listens before the old one goes away.
		ln2 := newMemListener()
		startServer(t, ln2, domain.NewFakeClock(testStart), testHandlers(), nil)
		sd.set(ln2)
		srv1.stop(t)

		// Fire any backoff wait the client registers.
		pumpCtx, stopPump := context.WithCancel(context.Background())
		var pump sync.WaitGroup
		pump.Add(1)
		go func() {
			defer pump.Done()
			for cliClock.BlockUntilWaiters(pumpCtx, 1) == nil {
				cliClock.Advance(5 * time.Second)
			}
		}()
		defer func() {
			stopPump()
			pump.Wait()
		}()

		var lastErr error
		for attempt := range 10 {
			pong = ipc.PingResult{}
			lastErr = c.Call(guardCtx(t), ipc.MethodPing, nil, &pong)
			if lastErr == nil {
				break
			}
			// A call racing the drop may fail; it must not be the guard ctx expiring.
			if errors.Is(lastErr, context.DeadlineExceeded) {
				t.Fatalf("attempt %d: client did not reconnect: %v", attempt, lastErr)
			}
		}
		if lastErr != nil || !pong.Pong {
			t.Fatalf("ping after restart: %+v, %v", pong, lastErr)
		}
	})
}

// TestShutdown is not parallel: it checks that no goroutine survives Close.
// Goroutines that already existed (e.g. paused parallel tests) are ignored.
func TestShutdown(t *testing.T) {
	ignoreExisting := goleak.IgnoreCurrent()

	t.Run("IPC-006/Server.Close aguarda goroutines", func(t *testing.T) {
		ln := newMemListener()
		srvClock := domain.NewFakeClock(testStart)
		r := startServer(t, ln, srvClock, testHandlers(), nil)
		a := dialRaw(t, ln)
		a.ping(t, 1)
		blockUntilWaiters(t, srvClock, 1) // heartbeat loop running

		within(t, "Server.Close", func() {
			if err := r.srv.Close(); err != nil {
				t.Logf("Server.Close: %v", err)
			}
		})
		recv(t, r.done, "Serve to return after Close")
		if _, err := a.next(); err == nil {
			t.Fatal("connection still open after Server.Close")
		} else if ne := net.Error(nil); errors.As(err, &ne) && ne.Timeout() {
			t.Fatalf("Server.Close did not close the connection: %v", err)
		}
		goleak.VerifyNone(t, ignoreExisting)
	})

	t.Run("IPC-006/Client.Close aguarda goroutines", func(t *testing.T) {
		entered := make(chan struct{})
		exited := make(chan struct{})
		var enterOnce, exitOnce sync.Once
		dial := func(ctx context.Context) (net.Conn, error) {
			defer exitOnce.Do(func() { close(exited) })
			enterOnce.Do(func() { close(entered) })
			<-ctx.Done()
			return nil, ctx.Err()
		}
		c := ipc.NewClient(dial, domain.NewFakeClock(testStart), discardLogger())
		recv(t, entered, "first dial")

		within(t, "Client.Close", func() {
			if err := c.Close(); err != nil {
				t.Logf("Client.Close: %v", err)
			}
		})
		select {
		case <-exited:
		default:
			t.Fatal("Client.Close returned while its goroutine was still inside the Dialer")
		}
		select {
		case _, ok := <-c.Notifications():
			if ok {
				t.Fatal("Notifications() delivered a value instead of being closed")
			}
		default:
			t.Fatal("Notifications() not closed by Close")
		}
		goleak.VerifyNone(t, ignoreExisting)
	})

	t.Run("IPC-006/nenhuma goroutine sobra com conexões abertas", func(t *testing.T) {
		ln := newMemListener()
		srvClock := domain.NewFakeClock(testStart)
		r := startServer(t, ln, srvClock, testHandlers(), nil)

		clients := []*ipc.Client{
			ipc.NewClient(ln.Dial, domain.NewFakeClock(testStart), discardLogger()),
			ipc.NewClient(ln.Dial, domain.NewFakeClock(testStart), discardLogger()),
		}
		for i, c := range clients {
			var pong ipc.PingResult
			if err := c.Call(guardCtx(t), ipc.MethodPing, nil, &pong); err != nil {
				t.Fatalf("client %d ping: %v", i, err)
			}
		}
		raw := dialRaw(t, ln)
		raw.ping(t, 1)

		// One heartbeat in flight to every connection.
		blockUntilWaiters(t, srvClock, 1)
		srvClock.Advance(ipc.HeartbeatInterval)
		assertHeartbeat(t, raw.readLine(t), 5)

		// Server closes with connections still open, then the clients.
		r.stop(t)
		for i, c := range clients {
			within(t, fmt.Sprintf("Client %d Close", i), func() {
				if err := c.Close(); err != nil {
					t.Logf("Client.Close: %v", err)
				}
			})
		}
		raw.conn.Close()
		goleak.VerifyNone(t, ignoreExisting)
	})

	t.Run("IPC-006/Close idempotente", func(t *testing.T) {
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		c := ipc.NewClient(ln.Dial, domain.NewFakeClock(testStart), discardLogger())
		var pong ipc.PingResult
		if err := c.Call(guardCtx(t), ipc.MethodPing, nil, &pong); err != nil {
			t.Fatalf("ping: %v", err)
		}

		for i := range 2 {
			within(t, fmt.Sprintf("Client.Close #%d", i+1), func() { _ = c.Close() })
		}
		for i := range 2 {
			within(t, fmt.Sprintf("Server.Close #%d", i+1), func() { _ = r.srv.Close() })
		}
		r.stop(t) // waits for Serve to return
		if err := c.Call(guardCtx(t), ipc.MethodPing, nil, &pong); !errors.Is(err, ipc.ErrClientClosed) {
			t.Fatalf("Call after double Close = %v, want ErrClientClosed", err)
		}
		goleak.VerifyNone(t, ignoreExisting)
	})
}
