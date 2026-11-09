package ipc_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
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

var errTransientAccept = errors.New("test: transient accept failure 4c1d")

// acceptBackoffs is the IPC-018 sequence of waits between consecutive failed
// Accepts: doubles from 100 ms, capped at 5 s, no jitter. The 10th consecutive
// failure ends Serve without waiting, so there are 9 waits.
var acceptBackoffs = []time.Duration{
	100 * time.Millisecond,
	200 * time.Millisecond,
	400 * time.Millisecond,
	800 * time.Millisecond,
	1600 * time.Millisecond,
	3200 * time.Millisecond,
	5000 * time.Millisecond,
	5000 * time.Millisecond,
	5000 * time.Millisecond,
}

// maxAcceptFailures is the IPC-018 number of consecutive Accept failures after
// which Serve gives up.
const maxAcceptFailures = 10

// acceptResult is one programmed outcome of scriptListener.Accept.
type acceptResult struct {
	conn net.Conn
	err  error
}

// failures returns one acceptResult per error, in order.
func failures(errs ...error) []acceptResult {
	out := make([]acceptResult, len(errs))
	for i, err := range errs {
		out[i] = acceptResult{err: err}
	}
	return out
}

// scriptListener is a net.Listener whose Accept hands out programmed results
// in order and blocks while the queue is empty, until push or Close. Every
// Accept call announces its 1-based number on called when it starts.
type scriptListener struct {
	mu     sync.Mutex
	queue  []acceptResult
	calls  int
	wake   chan struct{} // cap 1: the queue changed
	called chan int      // buffered: number of each Accept call
	done   chan struct{}
	once   sync.Once
}

func newScriptListener(results ...acceptResult) *scriptListener {
	return &scriptListener{
		queue:  results,
		wake:   make(chan struct{}, 1),
		called: make(chan int, 64),
		done:   make(chan struct{}),
	}
}

func (l *scriptListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	l.calls++
	n := l.calls
	l.mu.Unlock()
	select {
	case l.called <- n:
	default:
	}
	for {
		select {
		case <-l.done:
			return nil, net.ErrClosed
		default:
		}
		l.mu.Lock()
		if len(l.queue) > 0 {
			r := l.queue[0]
			l.queue = l.queue[1:]
			l.mu.Unlock()
			return r.conn, r.err
		}
		l.mu.Unlock()
		select {
		case <-l.wake:
		case <-l.done:
			return nil, net.ErrClosed
		}
	}
}

func (l *scriptListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *scriptListener) Addr() net.Addr { return memAddr{} }

// push appends results to the queue and wakes a blocked Accept.
func (l *scriptListener) push(results ...acceptResult) {
	l.mu.Lock()
	l.queue = append(l.queue, results...)
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *scriptListener) acceptCalls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

// nextAccept waits for the server's next Accept call and checks its number.
func (l *scriptListener) nextAccept(t *testing.T, want int) {
	t.Helper()
	if n := recv(t, l.called, fmt.Sprintf("Accept call #%d", want)); n != want {
		t.Fatalf("Accept call #%d, want #%d", n, want)
	}
}

// expectNoAccept checks, right after an Advance, that the server did not call
// Accept again. The heartbeat waiter may have fired in that Advance and is
// re-registered asynchronously, so it waits until the clock has its 2 waiters
// (heartbeat + backoff) back. If the backoff had fired instead, the server
// calls Accept before registering any new wait, so the call is already on
// l.called by then (or the server blocks in Accept and the call shows up
// without the waiters coming back).
func (l *scriptListener) expectNoAccept(t *testing.T, clk *domain.FakeClock, when string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), guardTimeout)
	defer cancel()
	waited := make(chan error, 1)
	go func() { waited <- clk.BlockUntilWaiters(ctx, 2) }()
	select {
	case n := <-l.called:
		cancel()
		<-waited
		t.Fatalf("Accept call #%d happened %s", n, when)
	case err := <-waited:
		if err != nil {
			t.Fatalf("waiting for heartbeat + backoff waiters %s: %v", when, err)
		}
		select {
		case n := <-l.called:
			t.Fatalf("Accept call #%d happened %s", n, when)
		default:
		}
	}
}

// stepBackoff is called with Accept call #call having just failed. It waits for
// the backoff wait d to be registered and fires it, checking that the next
// Accept (#call+1) happens. With strict it first advances d-1ns and checks
// that nothing fired (no jitter, no shorter wait).
func (l *scriptListener) stepBackoff(t *testing.T, clk *domain.FakeClock, call int, d time.Duration, strict bool) {
	t.Helper()
	blockUntilWaiters(t, clk, 2) // heartbeat (5 s) + accept backoff
	if strict {
		clk.Advance(d - time.Nanosecond)
		l.expectNoAccept(t, clk, fmt.Sprintf("1ns before the %v backoff after failure #%d", d, call))
		clk.Advance(time.Nanosecond)
	} else {
		clk.Advance(d)
	}
	l.nextAccept(t, call+1)
}

// recordingClock is a FakeClock that records the duration of every After call,
// so a test can see which waits the server asked for.
type recordingClock struct {
	*domain.FakeClock
	mu     sync.Mutex
	afters []time.Duration
}

func (c *recordingClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.afters = append(c.afters, d)
	c.mu.Unlock()
	return c.FakeClock.After(d)
}

// take returns the durations recorded since the previous take.
func (c *recordingClock) take() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.afters
	c.afters = nil
	return out
}

// serveNotReturned fails if Serve has already returned.
func serveNotReturned(t *testing.T, r *serverRun, when string) {
	t.Helper()
	select {
	case <-r.done:
		t.Fatalf("Serve returned %s: %v", when, r.serveErr)
	default:
	}
}

// TestServerAcceptError covers IPC-018: an Accept error other than
// net.ErrClosed is logged at Warn and Accept is retried after a backoff on the
// server clock (100 ms doubling to 5 s, no jitter, reset by a successful
// Accept); the 10th consecutive failure ends Serve with that error, logged at
// Error; Close or the end of ctx during the wait shut down normally.
//
// Waiter count: the heartbeat loop always has one pending After(5 s) on the
// same clock, so "backoff wait registered" means 2 waiters. Long sequences do
// fire the heartbeat (no connections, so it sends nothing); helpers always wait
// for both waiters to be back before advancing.
func TestServerAcceptError(t *testing.T) {
	t.Parallel()

	t.Run("IPC-018/erro transitório de Accept é logado em Warn e o accept continua", func(t *testing.T) {
		t.Parallel()
		ln := newScriptListener(failures(errTransientAccept)...)
		clk := domain.NewFakeClock(testStart)
		log, logs := newCaptureLogger()
		r := startServer(t, ln, clk, testHandlers(), log)

		ln.nextAccept(t, 1)
		blockUntilWaiters(t, clk, 2) // heartbeat (5 s) + accept backoff (100 ms)

		if !logs.containsAt(slog.LevelWarn, "", errTransientAccept.Error()) {
			t.Fatalf("transient Accept error not logged at Warn with its text; logs: %v", logs.all())
		}
		serveNotReturned(t, r, "after a transient Accept error")

		clk.Advance(acceptBackoffs[0])
		ln.nextAccept(t, 2)

		// The retried Accept takes a connection, which is served.
		client, server := net.Pipe()
		c := newRawConn(t, client)
		ln.push(acceptResult{conn: server})
		c.ping(t, 1)
		serveNotReturned(t, r, "after the retried Accept succeeded")
	})

	t.Run("IPC-018/Close durante a espera após erro de Accept faz Serve retornar nil", func(t *testing.T) {
		t.Parallel()
		ln := newScriptListener(failures(errTransientAccept)...)
		clk := domain.NewFakeClock(testStart)
		r := startServer(t, ln, clk, testHandlers(), nil)

		ln.nextAccept(t, 1)
		blockUntilWaiters(t, clk, 2) // server is inside the 100 ms backoff wait

		r.stop(t) // Server.Close and Serve must return within guardTimeout
		if r.serveErr != nil {
			t.Fatalf("Serve after Close during the backoff wait = %v, want nil", r.serveErr)
		}
	})

	t.Run("IPC-018/fim do ctx durante a espera faz Serve retornar ctx.Err()", func(t *testing.T) {
		t.Parallel()
		ln := newScriptListener(failures(errTransientAccept)...)
		clk := domain.NewFakeClock(testStart)
		srv := ipc.NewServer(clk, testHandlers(), discardLogger())
		ctx, cancel := context.WithCancel(context.Background())
		served := make(chan error, 1)
		go func() { served <- srv.Serve(ctx, ln) }()
		t.Cleanup(func() {
			cancel()
			within(t, "Server.Close", func() { _ = srv.Close() })
		})

		ln.nextAccept(t, 1)
		blockUntilWaiters(t, clk, 2) // server is inside the 100 ms backoff wait

		cancel()
		err := recv(t, served, "Serve to return after ctx cancel during the backoff wait")
		if !errors.Is(err, context.Canceled) || err != ctx.Err() {
			t.Fatalf("Serve after ctx cancel during the backoff wait = %v, want ctx.Err() (%v)", err, ctx.Err())
		}
		if n := ln.acceptCalls(); n != 1 {
			t.Fatalf("Accept called %d times, want 1 (ctx ended during the first wait)", n)
		}
	})

	t.Run("IPC-018/esperas 100 200 400 800 1600 3200 5000 5000 5000 ms", func(t *testing.T) {
		t.Parallel()
		// 9 failures, then Accept blocks (empty queue): the 10th call never fails.
		errs := make([]error, len(acceptBackoffs))
		for i := range errs {
			errs[i] = fmt.Errorf("test: accept failure #%d", i+1)
		}
		ln := newScriptListener(failures(errs...)...)
		clk := &recordingClock{FakeClock: domain.NewFakeClock(testStart)}
		r := startServer(t, ln, clk, testHandlers(), nil)

		ln.nextAccept(t, 1)
		for i, d := range acceptBackoffs {
			blockUntilWaiters(t, clk.FakeClock, 2) // heartbeat + backoff after failure #i+1
			// Since the previous step the server asked for exactly one backoff
			// wait, d; anything else must be the heartbeat re-arming.
			var backoffs []time.Duration
			for _, got := range clk.take() {
				if got != ipc.HeartbeatInterval || d == ipc.HeartbeatInterval {
					backoffs = append(backoffs, got)
				}
			}
			wrong := !slices.Contains(backoffs, d)
			if d != ipc.HeartbeatInterval {
				wrong = !slices.Equal(backoffs, []time.Duration{d})
			}
			if wrong {
				t.Fatalf("after Accept failure #%d the server waited %v, want %v (sequence %v)", i+1, backoffs, d, acceptBackoffs)
			}
			clk.Advance(d) // exactly d fires it
			ln.nextAccept(t, i+2)
		}
		serveNotReturned(t, r, "with the 10th Accept still pending")
	})

	t.Run("IPC-018/sem jitter (nada dispara 1ns antes)", func(t *testing.T) {
		t.Parallel()
		errs := make([]error, len(acceptBackoffs))
		for i := range errs {
			errs[i] = fmt.Errorf("test: accept failure #%d", i+1)
		}
		ln := newScriptListener(failures(errs...)...)
		clk := domain.NewFakeClock(testStart)
		r := startServer(t, ln, clk, testHandlers(), nil)

		ln.nextAccept(t, 1)
		for i, d := range acceptBackoffs {
			// d-1ns fires nothing; the last nanosecond fires the wait.
			ln.stepBackoff(t, clk, i+1, d, true)
		}
		serveNotReturned(t, r, "with the 10th Accept still pending")
	})

	t.Run("IPC-018/Accept bem-sucedido zera o backoff para 100 ms", func(t *testing.T) {
		t.Parallel()
		client, server := net.Pipe()
		c := newRawConn(t, client)
		results := failures(errTransientAccept, errTransientAccept, errTransientAccept)
		results = append(results, acceptResult{conn: server})
		results = append(results, failures(errTransientAccept)...)
		ln := newScriptListener(results...)
		clk := domain.NewFakeClock(testStart)
		r := startServer(t, ln, clk, testHandlers(), nil)

		ln.nextAccept(t, 1)
		ln.stepBackoff(t, clk, 1, acceptBackoffs[0], true) // 100 ms
		ln.stepBackoff(t, clk, 2, acceptBackoffs[1], true) // 200 ms
		ln.stepBackoff(t, clk, 3, acceptBackoffs[2], true) // 400 ms; call #4 accepts the conn
		ln.nextAccept(t, 5)                                // fails again
		// The failure after a successful Accept waits 100 ms again, not 800 ms.
		ln.stepBackoff(t, clk, 5, acceptBackoffs[0], true)

		c.ping(t, 1) // the connection accepted between the failures is served
		serveNotReturned(t, r, "after the backoff reset")
	})

	t.Run("IPC-018/10ª falha seguida: Serve retorna errors.Is último erro e loga Error", func(t *testing.T) {
		t.Parallel()
		errs := make([]error, maxAcceptFailures)
		for i := range errs {
			errs[i] = fmt.Errorf("test: accept failure #%d 9b2e", i+1)
		}
		ln := newScriptListener(failures(errs...)...)
		clk := domain.NewFakeClock(testStart)
		log, logs := newCaptureLogger()
		r := startServer(t, ln, clk, testHandlers(), log)

		ln.nextAccept(t, 1)
		for i, d := range acceptBackoffs {
			ln.stepBackoff(t, clk, i+1, d, false)
		}
		// The 10th failure ends Serve without any further wait or Advance.
		recv(t, r.done, "Serve to return after the 10th consecutive Accept failure")

		last := errs[maxAcceptFailures-1]
		if r.serveErr == nil || !errors.Is(r.serveErr, last) {
			t.Fatalf("Serve = %v, want an error with errors.Is(err, %q)", r.serveErr, last)
		}
		if errors.Is(r.serveErr, net.ErrClosed) {
			t.Fatalf("Serve = %v, want the last Accept error, not net.ErrClosed", r.serveErr)
		}
		if n := ln.acceptCalls(); n != maxAcceptFailures {
			t.Fatalf("Accept called %d times, want %d", n, maxAcceptFailures)
		}
		if !logs.containsAt(slog.LevelError, "", last.Error()) {
			t.Fatalf("giving up not logged at Error with the last Accept error; logs: %v", logs.all())
		}
		warns := 0
		for _, l := range logs.all() {
			if strings.HasPrefix(l, slog.LevelWarn.String()+" ") && strings.Contains(l, "9b2e") {
				warns++
			}
		}
		if warns != len(acceptBackoffs) {
			t.Fatalf("%d Warn logs for Accept failures, want %d (one per retried failure); logs: %v",
				warns, len(acceptBackoffs), logs.all())
		}
	})

	t.Run("IPC-018/net.ErrClosed sem Close continua encerrando Serve com erro", func(t *testing.T) {
		t.Parallel()
		ln := newScriptListener(failures(net.ErrClosed)...)
		clk := domain.NewFakeClock(testStart)
		r := startServer(t, ln, clk, testHandlers(), nil)

		ln.nextAccept(t, 1)
		// No Close, no ctx end, no Advance: Serve ends on its own.
		recv(t, r.done, "Serve to return after Accept returned net.ErrClosed")
		if r.serveErr == nil || !errors.Is(r.serveErr, net.ErrClosed) {
			t.Fatalf("Serve = %v, want an error with errors.Is(err, net.ErrClosed)", r.serveErr)
		}
		if n := ln.acceptCalls(); n != 1 {
			t.Fatalf("Accept called %d times, want 1 (net.ErrClosed is not retried)", n)
		}
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

	t.Run("IPC-006/Drain não deixa goroutines", func(t *testing.T) {
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		conns := []*rawConn{dialRaw(t, ln), dialRaw(t, ln)}
		for _, c := range conns {
			c.ping(t, 1)
		}

		// Drain that ends because every client closed.
		errc := drainAsync(guardCtx(t), r.srv, "quit")
		for _, line := range readEach(t, conns) {
			assertShutdown(t, line, "quit")
		}
		for _, c := range conns {
			c.conn.Close()
		}
		if err := recv(t, errc, "Drain to return after the clients closed"); err != nil {
			t.Fatalf("Drain = %v, want nil", err)
		}

		// Drain that ends by ctx, with a connection left for Close.
		left := dialRaw(t, ln)
		assertShutdown(t, left.readLine(t), "quit") // still in drain after Drain returned (EXE-015)
		left.ping(t, 1)
		ctx, cancel := context.WithCancel(context.Background())
		errc = drainAsync(ctx, r.srv, "quit")
		cancel()
		if err := recv(t, errc, "Drain to return after ctx cancel"); !errors.Is(err, context.Canceled) {
			t.Fatalf("Drain with a connection open and ctx cancelled = %v, want errors.Is Canceled", err)
		}

		r.stop(t)
		left.conn.Close()
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
