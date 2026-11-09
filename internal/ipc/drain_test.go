package ipc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// shortDrain is the real-time deadline used where a client never closes its
// connection. Drain is bounded only by its ctx (tasks.md, decision 1), so a
// short real deadline is the input under test, not a wait for behavior.
const shortDrain = 50 * time.Millisecond

// drainAsync runs Drain with agent.shutdown/reason in a goroutine and returns
// a channel with its result. Every caller receives from the channel, and
// Server.Close (cleanup) makes a pending Drain return, since it closes every
// connection.
func drainAsync(ctx context.Context, srv *ipc.Server, reason string) <-chan error {
	errc := make(chan error, 1)
	go func() {
		errc <- srv.Drain(ctx, ipc.MethodShutdown, ipc.ShutdownParams{Reason: reason})
	}()
	return errc
}

// assertShutdown checks line is exactly an agent.shutdown notification (no id
// member) whose params are exactly {"reason":reason}.
func assertShutdown(t *testing.T, line []byte, reason string) {
	t.Helper()
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(line, &keys); err != nil {
		t.Fatalf("shutdown notification is not a JSON object: %v (%q)", err, line)
	}
	if _, ok := keys["id"]; ok {
		t.Fatalf("agent.shutdown must be a notification without id: %q", line)
	}
	if got := sortedKeys(keys); strings.Join(got, ",") != "jsonrpc,method,params" {
		t.Fatalf("shutdown members = %v, want [jsonrpc method params] (%q)", got, line)
	}
	if got := compactJSON(t, keys["jsonrpc"]); got != `"2.0"` {
		t.Fatalf("shutdown jsonrpc = %s", got)
	}
	if got := compactJSON(t, keys["method"]); got != `"agent.shutdown"` {
		t.Fatalf("shutdown method = %s, want \"agent.shutdown\" (%q)", got, line)
	}
	want, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		t.Fatalf("marshal want params: %v", err)
	}
	if got := compactJSON(t, keys["params"]); got != string(want) {
		t.Fatalf("shutdown params = %s, want %s", got, want)
	}
}

// assertDrainError checks err is a *DrainError for a ctx that ended with
// wantCtxErr and wantPending connections still open.
func assertDrainError(t *testing.T, err error, wantCtxErr error, wantPending int) {
	t.Helper()
	if err == nil {
		t.Fatalf("Drain = nil, want an error wrapping %v", wantCtxErr)
	}
	if !errors.Is(err, wantCtxErr) {
		t.Fatalf("Drain = %v, want errors.Is(err, %v)", err, wantCtxErr)
	}
	var de *ipc.DrainError
	if !errors.As(err, &de) || de == nil {
		t.Fatalf("Drain = %v (%T), want a *ipc.DrainError", err, err)
	}
	if de.Pending != wantPending {
		t.Fatalf("DrainError.Pending = %d, want %d", de.Pending, wantPending)
	}
	if !errors.Is(de.Err, wantCtxErr) {
		t.Fatalf("DrainError.Err = %v, want %v", de.Err, wantCtxErr)
	}
	if de.Error() == "" {
		t.Fatal("DrainError.Error() is empty")
	}
}

// assertClosedByServer checks the next read on c fails because the server
// closed the connection (not because the guard deadline expired).
func assertClosedByServer(t *testing.T, c *rawConn, when string) {
	t.Helper()
	line, err := c.next()
	if err == nil {
		t.Fatalf("connection still open %s; read %q", when, line)
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		t.Fatalf("server did not close the connection %s: %v", when, err)
	}
}

// stuckBeats is how many heartbeats stuckConn broadcasts: more than one
// message held by the blocked writer plus a full queue (16) can absorb.
const stuckBeats = 20

// stuckConn dials a connection that sends one request and never reads, plus
// healthy connections that read everything, on a server started with
// WithStartedAt(testStart) and clock clk. It then broadcasts stuckBeats
// heartbeats. net.Pipe writes complete only when the peer reads, so the stuck
// connection's writer blocks on its first message and its outgoing queue ends
// full (heartbeats are dropped only when it is full, and nothing else is ever
// taken from it). On return the last broadcast has finished: the heartbeat loop
// is waiting on clk again.
func stuckConn(t *testing.T, ln *memListener, clk *domain.FakeClock, healthy int) (*rawConn, []*rawConn) {
	t.Helper()
	slow := dialRaw(t, ln)
	// Once send returns the server read the request, so the connection is registered.
	slow.send(t, `{"jsonrpc":"2.0","id":1,"method":"agent.ping"}`)
	conns := make([]*rawConn, healthy)
	for i := range conns {
		conns[i] = dialRaw(t, ln)
		conns[i].ping(t, 1)
	}
	for k := int64(1); k <= stuckBeats; k++ {
		blockUntilWaiters(t, clk, 1)
		clk.Advance(ipc.HeartbeatInterval)
		for _, line := range readEach(t, conns) {
			assertHeartbeat(t, line, 5*k)
		}
	}
	blockUntilWaiters(t, clk, 1) // the last broadcastHeartbeat returned
	return slow, conns
}

// probePending calls Drain with an already cancelled ctx, which only reports
// the connections still open without waiting (the server is already draining,
// so nothing is sent). It returns 0 for nil.
func probePending(t *testing.T, srv *ipc.Server) int {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var err error
	within(t, "Drain with a cancelled ctx", func() {
		err = srv.Drain(ctx, ipc.MethodShutdown, ipc.ShutdownParams{Reason: "probe"})
	})
	if err == nil {
		return 0
	}
	var de *ipc.DrainError
	if !errors.As(err, &de) || de == nil {
		t.Fatalf("Drain with a cancelled ctx = %v (%T), want nil or *ipc.DrainError", err, err)
	}
	return de.Pending
}

// awaitPending waits until the server reports want connections still open
// (e.g. after a client closed and the server has not unregistered it yet).
func awaitPending(t *testing.T, srv *ipc.Server, want int) {
	t.Helper()
	deadline := time.Now().Add(guardTimeout)
	for {
		got := probePending(t, srv)
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server reports %d open connection(s), want %d", got, want)
		}
		runtime.Gosched()
	}
}

// notReturned fails if errc already holds a result.
func notReturned(t *testing.T, errc <-chan error, when string) {
	t.Helper()
	select {
	case err := <-errc:
		t.Fatalf("Drain returned %v %s; want it still waiting", err, when)
	default:
	}
}

func TestDrain(t *testing.T) {
	t.Parallel()

	t.Run("EXE-013/cada conexão recebe agent.shutdown com params.reason", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		conns := []*rawConn{dialRaw(t, ln), dialRaw(t, ln), dialRaw(t, ln)}
		for _, c := range conns {
			c.ping(t, 1) // registered and served
		}

		errc := drainAsync(guardCtx(t), r.srv, "quit")
		for _, line := range readEach(t, conns) {
			assertShutdown(t, line, "quit")
		}
		for _, c := range conns {
			c.conn.Close()
		}
		if err := recv(t, errc, "Drain to return after every client closed"); err != nil {
			t.Fatalf("Drain = %v, want nil", err)
		}
	})

	t.Run("EXE-013/a notificação não tem id", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		c := dialRaw(t, ln)
		c.ping(t, 1)

		errc := drainAsync(guardCtx(t), r.srv, "signal")
		line := c.readLine(t)
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(line, &keys); err != nil {
			t.Fatalf("notification is not a JSON object: %v (%q)", err, line)
		}
		if _, ok := keys["id"]; ok {
			t.Fatalf("agent.shutdown has an id member: %q", line)
		}
		assertShutdown(t, line, "signal")
		c.conn.Close()
		if err := recv(t, errc, "Drain to return"); err != nil {
			t.Fatalf("Drain = %v, want nil", err)
		}
	})

	t.Run("EXE-013/a notificação chega antes de o servidor fechar a conexão", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		c := dialRaw(t, ln)
		c.ping(t, 1)

		// The client reads nothing until Drain gave up: the notification must
		// still be there to read, and only Close ends the connection.
		ctx, cancel := context.WithTimeout(context.Background(), shortDrain)
		defer cancel()
		err := recv(t, drainAsync(ctx, r.srv, "quit"), "Drain to return at its deadline")
		assertDrainError(t, err, context.DeadlineExceeded, 1)

		assertShutdown(t, c.readLine(t), "quit")
		r.stop(t)
		assertClosedByServer(t, c, "after Server.Close")
	})

	t.Run("EXE-014/todos os clientes fecham: Drain retorna nil antes do prazo", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		conns := []*rawConn{dialRaw(t, ln), dialRaw(t, ln)}
		for _, c := range conns {
			c.ping(t, 1)
		}

		ctx, cancel := context.WithTimeout(context.Background(), guardTimeout)
		defer cancel()
		errc := drainAsync(ctx, r.srv, "quit")
		for _, line := range readEach(t, conns) {
			assertShutdown(t, line, "quit")
		}
		for _, c := range conns {
			c.conn.Close()
		}
		err := recv(t, errc, "Drain to return after every client closed")
		if err != nil {
			t.Fatalf("Drain = %v, want nil", err)
		}
		if ctx.Err() != nil {
			t.Fatalf("Drain returned only after its ctx ended (%v); want it to return as soon as the clients closed", ctx.Err())
		}
		serveNotReturned(t, r, "after Drain")
	})

	t.Run("EXE-014/cliente que não fecha: Drain retorna errors.Is DeadlineExceeded e DrainError.Pending = 1", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		c := dialRaw(t, ln)
		c.ping(t, 1)

		ctx, cancel := context.WithTimeout(context.Background(), shortDrain)
		defer cancel()
		errc := drainAsync(ctx, r.srv, "quit")
		assertShutdown(t, c.readLine(t), "quit") // read, but never closed
		err := recv(t, errc, "Drain to return at its deadline")
		if ctx.Err() == nil {
			t.Fatalf("Drain returned %v before its ctx ended, with a client still open", err)
		}
		assertDrainError(t, err, context.DeadlineExceeded, 1)
	})

	t.Run("EXE-014/DrainError.Pending conta as conexões ainda abertas", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		conns := []*rawConn{dialRaw(t, ln), dialRaw(t, ln)}
		for _, c := range conns {
			c.ping(t, 1)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errc := drainAsync(ctx, r.srv, "other")
		for _, line := range readEach(t, conns) {
			assertShutdown(t, line, "other")
		}
		cancel() // ended by hand, with both connections open
		err := recv(t, errc, "Drain to return after ctx cancel")
		assertDrainError(t, err, context.Canceled, 2)
	})

	t.Run("EXE-014/Drain não fecha conexões nem para o accept", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		a := dialRaw(t, ln)
		a.ping(t, 1)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errc := drainAsync(ctx, r.srv, "quit")
		assertShutdown(t, a.readLine(t), "quit")

		// While draining: the existing connection is served and a new one is accepted.
		a.ping(t, 2)
		b := dialRaw(t, ln)
		assertShutdown(t, b.readLine(t), "quit")
		b.ping(t, 1)

		cancel()
		err := recv(t, errc, "Drain to return after ctx cancel")
		assertDrainError(t, err, context.Canceled, 2)

		// After Drain returned: nothing was closed and accept goes on.
		a.ping(t, 3)
		b.ping(t, 2)
		// The server stays in drain after Drain returned with an error (EXE-015).
		c := dialRaw(t, ln)
		assertShutdown(t, c.readLine(t), "quit")
		c.ping(t, 1)
		serveNotReturned(t, r, "after Drain returned")
	})

	t.Run("EXE-014/sem conexões abertas retorna nil na hora", func(t *testing.T) {
		t.Parallel()
		ln := newScriptListener() // Accept blocks: no connection ever
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		ln.nextAccept(t, 1) // Serve is in its accept loop

		var err error
		// ctx without deadline: only the guard of within stops a Drain that waits.
		within(t, "Drain with no open connection", func() {
			err = r.srv.Drain(context.Background(), ipc.MethodShutdown, ipc.ShutdownParams{Reason: "quit"})
		})
		if err != nil {
			t.Fatalf("Drain with no open connection = %v, want nil", err)
		}
		serveNotReturned(t, r, "after Drain")
	})

	t.Run("EXE-014/Close depois do Drain fecha as conexões restantes", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		conns := []*rawConn{dialRaw(t, ln), dialRaw(t, ln)}
		for _, c := range conns {
			c.ping(t, 1)
		}

		ctx, cancel := context.WithTimeout(context.Background(), shortDrain)
		defer cancel()
		errc := drainAsync(ctx, r.srv, "quit")
		for _, line := range readEach(t, conns) {
			assertShutdown(t, line, "quit")
		}
		err := recv(t, errc, "Drain to return at its deadline")
		assertDrainError(t, err, context.DeadlineExceeded, 2)

		r.stop(t) // Close and Serve return within guardTimeout
		if r.serveErr != nil {
			t.Fatalf("Serve after Drain + Close = %v, want nil", r.serveErr)
		}
		for _, c := range conns {
			assertClosedByServer(t, c, "after Drain + Server.Close")
		}
	})

	t.Run("EXE-013/conexão com fila cheia não impede as outras de receber agent.shutdown", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		clk := domain.NewFakeClock(testStart)
		r := startServer(t, ln, clk, testHandlers(), nil, ipc.WithStartedAt(testStart))
		_, healthy := stuckConn(t, ln, clk, 1)
		good := healthy[0]

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errc := drainAsync(ctx, r.srv, "quit")
		// The healthy connection is notified although the stuck one has no room.
		assertShutdown(t, good.readLine(t), "quit")
		good.conn.Close()
		awaitPending(t, r.srv, 1) // only the stuck connection is left
		notReturned(t, errc, "with the stuck connection still open")

		cancel() // the short ctx of the drain ends
		err := recv(t, errc, "Drain to return after ctx cancel")
		assertDrainError(t, err, context.Canceled, 1)
	})

	t.Run("EXE-014/Close durante Drain faz Drain retornar nil", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		c := dialRaw(t, ln)
		c.ping(t, 1)

		errc := drainAsync(guardCtx(t), r.srv, "quit")
		assertShutdown(t, c.readLine(t), "quit") // read, but never closed
		notReturned(t, errc, "before Close, with a client open")

		r.stop(t)
		if err := recv(t, errc, "Drain to return after Server.Close"); err != nil {
			t.Fatalf("Drain interrupted by Close = %v, want nil", err)
		}
		assertClosedByServer(t, c, "after Server.Close")
	})

	t.Run("EXE-014/Close com Drain esperando fila cheia faz Drain retornar nil", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		clk := domain.NewFakeClock(testStart)
		r := startServer(t, ln, clk, testHandlers(), nil, ipc.WithStartedAt(testStart))
		stuckConn(t, ln, clk, 0)

		// Drain waits for room in the stuck connection's queue (second pass).
		errc := drainAsync(guardCtx(t), r.srv, "quit")
		notReturned(t, errc, "before Close, with the queue full")

		r.stop(t)
		if err := recv(t, errc, "Drain to return after Server.Close"); err != nil {
			t.Fatalf("Drain interrupted by Close = %v, want nil", err)
		}
	})
}

func TestDrainRepeated(t *testing.T) {
	t.Parallel()

	t.Run("EXE-013/Drains concorrentes enviam a notificação uma vez só e ambos retornam", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		conns := []*rawConn{dialRaw(t, ln), dialRaw(t, ln)}
		for _, c := range conns {
			c.ping(t, 1)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		reasons := []string{"quit", "signal", "session-end"}
		errcs := make([]<-chan error, len(reasons))
		for i, reason := range reasons {
			errcs[i] = drainAsync(ctx, r.srv, reason)
		}
		lines := readEach(t, conns)
		var first string
		for _, reason := range reasons {
			if bytes.Contains(lines[0], []byte(fmt.Sprintf(`"reason":%q`, reason))) {
				first = reason
			}
		}
		if first == "" {
			t.Fatalf("first line is not an agent.shutdown of one of the drains: %q", lines[0])
		}
		for _, line := range lines {
			assertShutdown(t, line, first) // every connection got the same, first, notification
		}

		cancel()
		for i, errc := range errcs {
			err := recv(t, errc, fmt.Sprintf("Drain %d to return after ctx cancel", i))
			assertDrainError(t, err, context.Canceled, 2)
		}
		// Every Drain returned, so anything they queued precedes the pong.
		for _, c := range conns {
			c.ping(t, 2)
		}
	})

	t.Run("EXE-013/Drains concorrentes retornam nil quando os clientes fecham", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		conns := []*rawConn{dialRaw(t, ln), dialRaw(t, ln)}
		for _, c := range conns {
			c.ping(t, 1)
		}

		ctx := guardCtx(t)
		errcs := []<-chan error{drainAsync(ctx, r.srv, "quit"), drainAsync(ctx, r.srv, "quit")}
		for _, line := range readEach(t, conns) {
			assertShutdown(t, line, "quit")
		}
		for _, c := range conns {
			c.conn.Close()
		}
		for i, errc := range errcs {
			if err := recv(t, errc, fmt.Sprintf("Drain %d to return", i)); err != nil {
				t.Fatalf("Drain %d = %v, want nil", i, err)
			}
		}
	})

	t.Run("EXE-015/servidor continua em drain depois de Drain retornar", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		old := dialRaw(t, ln)
		old.ping(t, 1)

		errc := drainAsync(guardCtx(t), r.srv, "session-end")
		assertShutdown(t, old.readLine(t), "session-end")
		old.conn.Close()
		if err := recv(t, errc, "Drain to return after the client closed"); err != nil {
			t.Fatalf("Drain = %v, want nil", err)
		}

		// Drain returned nil, but a connection opened afterwards still gets the
		// notification as its first message, with the first reason.
		late := dialRaw(t, ln)
		assertShutdown(t, late.readLine(t), "session-end")
		late.ping(t, 1)

		// A later Drain keeps the first notification and sends nothing new.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		errc = drainAsync(ctx, r.srv, "other")
		later := dialRaw(t, ln)
		assertShutdown(t, later.readLine(t), "session-end")
		cancel()
		assertDrainError(t, recv(t, errc, "second Drain to return"), context.Canceled, 2)
		late.ping(t, 2) // no second agent.shutdown before the pong
		later.ping(t, 1)
		serveNotReturned(t, r, "after Drain returned")
	})

	t.Run("EXE-027/Drain antes de Serve retorna nil e não entra em drain", func(t *testing.T) {
		t.Parallel()
		srv := ipc.NewServer(domain.NewFakeClock(testStart), testHandlers(), discardLogger())
		var err error
		within(t, "Drain before Serve", func() {
			err = srv.Drain(context.Background(), ipc.MethodShutdown, ipc.ShutdownParams{Reason: "quit"})
		})
		if err != nil {
			t.Fatalf("Drain before Serve = %v, want nil", err)
		}

		ln := newMemListener()
		r := &serverRun{srv: srv, ln: ln, done: make(chan struct{})}
		go runServe(r)
		t.Cleanup(func() { r.stop(t) })

		c := dialRaw(t, ln)
		c.ping(t, 1) // first line is the pong: no agent.shutdown was queued

		// The early Drain did not take the notification: a Drain while serving sends its own.
		errc := drainAsync(guardCtx(t), srv, "signal")
		assertShutdown(t, c.readLine(t), "signal")
		c.conn.Close()
		if err := recv(t, errc, "Drain to return after the client closed"); err != nil {
			t.Fatalf("Drain while serving = %v, want nil", err)
		}
	})
}

func TestDrainNewConn(t *testing.T) {
	t.Parallel()

	t.Run("EXE-015/conexão aceita durante o drain recebe agent.shutdown como primeira mensagem, com o mesmo reason", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		old := dialRaw(t, ln)
		old.ping(t, 1)

		errc := drainAsync(guardCtx(t), r.srv, "session-end")
		// The old connection got the notification, so the server is draining.
		assertShutdown(t, old.readLine(t), "session-end")

		late := dialRaw(t, ln)
		assertShutdown(t, late.readLine(t), "session-end") // first line, before anything else
		late.ping(t, 1)                                    // and the connection is served

		// Drain waits for the late connection too.
		old.conn.Close()
		late.ping(t, 2)
		late.conn.Close()
		if err := recv(t, errc, "Drain to return after both clients closed"); err != nil {
			t.Fatalf("Drain = %v, want nil", err)
		}
	})
}

// recordConn records every byte the server writes to the connection.
type recordConn struct {
	net.Conn
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *recordConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.mu.Lock()
	c.buf.Write(p[:n])
	c.mu.Unlock()
	return n, err
}

func (c *recordConn) written() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// endedServer is a Server whose Serve has one served, recorded connection.
type endedServer struct {
	srv      *ipc.Server
	ln       *scriptListener
	cancel   context.CancelFunc
	done     chan struct{} // closed when Serve returns
	serveErr error         // valid after done is closed
	client   *rawConn
	rec      *recordConn
}

func startRecordedServer(t *testing.T) *endedServer {
	t.Helper()
	client, server := net.Pipe()
	rec := &recordConn{Conn: server}
	ln := newScriptListener(acceptResult{conn: rec})
	ctx, cancel := context.WithCancel(context.Background())
	e := &endedServer{
		srv:    ipc.NewServer(domain.NewFakeClock(testStart), testHandlers(), discardLogger()),
		ln:     ln,
		cancel: cancel,
		done:   make(chan struct{}),
		client: newRawConn(t, client),
		rec:    rec,
	}
	go func() {
		defer close(e.done)
		e.serveErr = e.srv.Serve(ctx, ln)
	}()
	t.Cleanup(func() {
		cancel()
		within(t, "Server.Close", func() { _ = e.srv.Close() })
		recv(t, e.done, "Serve to return at cleanup")
	})
	ln.nextAccept(t, 1) // hands out the recorded connection
	e.client.ping(t, 1)
	ln.nextAccept(t, 2) // Serve waits for the next connection
	return e
}

// serveEndings are the three ways Serve returns (EXE-027).
var serveEndings = []struct {
	name    string
	end     func(t *testing.T, e *endedServer)
	wantErr func(err error) bool
}{
	{
		name: "erro de Accept",
		end:  func(t *testing.T, e *endedServer) { e.ln.push(acceptResult{err: net.ErrClosed}) },
		wantErr: func(err error) bool {
			return err != nil && errors.Is(err, net.ErrClosed)
		},
	},
	{
		name: "Close",
		end: func(t *testing.T, e *endedServer) {
			within(t, "Server.Close", func() { _ = e.srv.Close() })
		},
		wantErr: func(err error) bool { return err == nil },
	},
	{
		name:    "fim do ctx",
		end:     func(t *testing.T, e *endedServer) { e.cancel() },
		wantErr: func(err error) bool { return errors.Is(err, context.Canceled) },
	},
}

// drainAfterEnd ends Serve with ending, then checks Drain with a ctx without
// deadline returns nil at once (the guard of within fails the test otherwise).
func drainAfterEnd(t *testing.T, ending int) *endedServer {
	t.Helper()
	se := serveEndings[ending]
	e := startRecordedServer(t)
	se.end(t, e)
	recv(t, e.done, "Serve to return ("+se.name+")")
	if !se.wantErr(e.serveErr) {
		t.Fatalf("Serve ended by %s returned %v", se.name, e.serveErr)
	}

	var err error
	within(t, "Drain after Serve returned ("+se.name+")", func() {
		err = e.srv.Drain(context.Background(), ipc.MethodShutdown, ipc.ShutdownParams{Reason: "other"})
	})
	if err != nil {
		t.Fatalf("Drain after Serve ended by %s = %v, want nil", se.name, err)
	}
	return e
}

func TestDrainAfterServe(t *testing.T) {
	t.Parallel()

	t.Run("EXE-027/Serve encerrado por erro de Accept: Drain retorna nil sem esperar", func(t *testing.T) {
		t.Parallel()
		drainAfterEnd(t, 0)
	})

	t.Run("EXE-027/após Close: Drain retorna nil", func(t *testing.T) {
		t.Parallel()
		drainAfterEnd(t, 1)
	})

	t.Run("EXE-027/após fim do ctx do Serve: Drain retorna nil", func(t *testing.T) {
		t.Parallel()
		drainAfterEnd(t, 2)
	})

	t.Run("EXE-027/nada é enviado", func(t *testing.T) {
		t.Parallel()
		for i, se := range serveEndings {
			t.Run(se.name, func(t *testing.T) {
				t.Parallel()
				e := drainAfterEnd(t, i)
				assertClosedByServer(t, e.client, "after Serve returned and Drain")
				got := e.rec.written()
				if !strings.Contains(got, `"pong":true`) {
					t.Fatalf("recorder missed the ping response; written %q", got)
				}
				if strings.Contains(got, ipc.MethodShutdown) {
					t.Fatalf("Drain after Serve returned sent agent.shutdown: %q", got)
				}
			})
		}
	})
}
