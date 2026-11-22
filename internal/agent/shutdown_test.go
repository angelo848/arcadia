package agent

// Tests for T06 of docs/specs/single-exe/tasks.md: shutdown with a reason and
// drain (EXE-013, EXE-014, AGT-003), agent without IPC (EXE-026, EXE-027) and
// the exit code. Shutdown(ctx, reason) drains with instance.drainTimeout
// (default shutdownDrainTimeout = 500 ms), which the tests override.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/feedback"
	"github.com/arcadia-launcher/arcadia/internal/input"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

const (
	msgIPCFailed    = "IPC indisponível; encerrando o agente"
	msgDrainTimeout = "clientes não fecharam a conexão a tempo"
)

// verifyNoLeaks registers, before any other cleanup of t, a goleak check that
// runs after all of them (cleanups are LIFO).
func verifyNoLeaks(t *testing.T) {
	t.Helper()
	opts := []goleak.Option{goleak.IgnoreCurrent(), goleak.IgnoreTopFunction(lumberjackMillRun)}
	t.Cleanup(func() { goleak.VerifyNone(t, opts...) })
}

// rawMsg is one NDJSON message received by a rawClient.
type rawMsg map[string]json.RawMessage

func (m rawMsg) method() string {
	var s string
	if raw, ok := m["method"]; ok {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}

// rawClient is a bare NDJSON client: a reader goroutine decodes every line
// into msgs and closes msgs at EOF (or any read error).
type rawClient struct {
	conn net.Conn
	msgs chan rawMsg
	mu   sync.Mutex
	all  []rawMsg // every message received, in order
	ids  int
}

func newRawClient(t *testing.T, conn net.Conn) *rawClient {
	t.Helper()
	c := &rawClient{conn: conn, msgs: make(chan rawMsg, 256)}
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer close(c.msgs)
		sc := bufio.NewScanner(conn)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			var m rawMsg
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				m = rawMsg{"_invalid": json.RawMessage(fmt.Sprintf("%q", sc.Text()))}
			}
			c.mu.Lock()
			c.all = append(c.all, m)
			c.mu.Unlock()
			select {
			case c.msgs <- m:
			default: // the test only inspects c.all past this point
			}
		}
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		select {
		case <-readerDone:
		case <-time.After(testTimeout):
			t.Error("raw client reader did not end at cleanup")
		}
	})
	return c
}

func dialRaw(t *testing.T, addr string) *rawClient {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, testTimeout)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	return newRawClient(t, conn)
}

// status sends agent.status and waits for its response, which proves the
// server registered the connection.
func (c *rawClient) status(t *testing.T) {
	t.Helper()
	c.ids++
	id := c.ids
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q}`+"\n", id, ipc.MethodStatus)
	if err := c.conn.SetWriteDeadline(time.Now().Add(testTimeout)); err != nil {
		t.Fatalf("SetWriteDeadline: %v", err)
	}
	if _, err := c.conn.Write([]byte(line)); err != nil {
		t.Fatalf("write agent.status: %v", err)
	}
	want := fmt.Sprint(id)
	timeout := time.After(testTimeout)
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				t.Fatal("connection closed before the agent.status response")
			}
			if string(m["id"]) == want {
				if _, isErr := m["error"]; isErr {
					t.Fatalf("agent.status returned error: %s", m["error"])
				}
				return
			}
		case <-timeout:
			t.Fatalf("no agent.status response within %v", testTimeout)
		}
	}
}

// next returns the next message (heartbeats included); it fails at EOF.
func (c *rawClient) next(t *testing.T) rawMsg {
	t.Helper()
	select {
	case m, ok := <-c.msgs:
		if !ok {
			t.Fatal("connection closed, want a message")
		}
		return m
	case <-time.After(testTimeout):
		t.Fatalf("no message within %v", testTimeout)
	}
	return nil
}

// waitMethod reads until a message with method arrives (skipping heartbeats).
func (c *rawClient) waitMethod(t *testing.T, method string) rawMsg {
	t.Helper()
	for {
		m := c.next(t)
		if m.method() == method {
			return m
		}
		if m.method() != ipc.MethodHeartbeat {
			t.Fatalf("unexpected message while waiting for %s: %v", method, m)
		}
	}
}

// waitEOF waits until the server closed the connection and returns every
// message received.
func (c *rawClient) waitEOF(t *testing.T) []rawMsg {
	t.Helper()
	timeout := time.After(testTimeout)
	for {
		select {
		case _, ok := <-c.msgs:
			if !ok {
				c.mu.Lock()
				defer c.mu.Unlock()
				return append([]rawMsg(nil), c.all...)
			}
		case <-timeout:
			t.Fatalf("server did not close the connection within %v", testTimeout)
		}
	}
}

func countMethod(msgs []rawMsg, method string) int {
	n := 0
	for _, m := range msgs {
		if m.method() == method {
			n++
		}
	}
	return n
}

func shutdownReason(t *testing.T, m rawMsg) string {
	t.Helper()
	var p ipc.ShutdownParams
	if err := json.Unmarshal(m["params"], &p); err != nil {
		t.Fatalf("agent.shutdown params %s: %v", m["params"], err)
	}
	return p.Reason
}

// shutdownAsync runs Shutdown in a goroutine; the result arrives on the channel.
func shutdownAsync(a *instance, ctx context.Context, reason ShutdownReason) <-chan error {
	errc := make(chan error, 1)
	go func() { errc <- a.Shutdown(ctx, reason) }()
	return errc
}

func waitShutdown(t *testing.T, errc <-chan error) error {
	t.Helper()
	select {
	case err := <-errc:
		return err
	case <-time.After(testTimeout):
		t.Fatalf("Shutdown did not return within %v", testTimeout)
	}
	return nil
}

func assertShutdownPending(t *testing.T, errc <-chan error) {
	t.Helper()
	select {
	case err := <-errc:
		t.Fatalf("Shutdown returned (%v) while a client is still connected; want it draining", err)
	default:
	}
}

// assertStoreClosed removes the database files: on Windows an open SQLite file
// cannot be removed.
func assertStoreClosed(t *testing.T, cfg config) {
	t.Helper()
	for _, p := range []string{dbPath(cfg), dbPath(cfg) + "-wal", dbPath(cfg) + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("remove %s after Shutdown: %v (store still open)", filepath.Base(p), err)
		}
	}
}

// findLog returns the log records with level and msg.
func findLog(t *testing.T, cfg config, level, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, rec := range logLines(t, agentLogPath(cfg)) {
		if rec["level"] == level && rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

func TestServeFailed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"EXE-026/nil não é falha", nil, false},
		{"EXE-026/context.Canceled não é falha", context.Canceled, false},
		{"EXE-026/context.Canceled embrulhado não é falha", fmt.Errorf("ipc: serve: %w", context.Canceled), false},
		{"EXE-026/erro de Accept é falha",
			fmt.Errorf("ipc: accept failed 10 times in a row: %w", errors.New("pipe instance unavailable")), true},
		{"EXE-026/net.ErrClosed sem Close é falha", fmt.Errorf("ipc: accept: %w", net.ErrClosed), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := serveFailed(tc.err); got != tc.want {
				t.Errorf("serveFailed(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		shutdownErr error
		ipcFailed   bool
		want        int
	}{
		{"EXE-026/ipcFailed sem erro de shutdown sai com 1", nil, true, 1},
		{"EXE-026/ipcFailed com erro de shutdown sai com 1", errors.New("close store"), true, 1},
		{"AGT-003/sem erro sai com 0", nil, false, 0},
		{"AGT-003/erro de shutdown sai com 1", errors.New("close store"), false, 1},
		{"AGT-003/prazo do shutdown excedido sai com 1", fmt.Errorf("shutdown: %w", context.DeadlineExceeded), false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := exitCode(tc.shutdownErr, tc.ipcFailed); got != tc.want {
				t.Errorf("exitCode(%v, %v) = %d, want %d", tc.shutdownErr, tc.ipcFailed, got, tc.want)
			}
		})
	}
}

func TestShutdownDrain(t *testing.T) {
	t.Run("EXE-013/Shutdown envia agent.shutdown com o reason ao cliente conectado", func(t *testing.T) {
		for _, reason := range []ShutdownReason{ReasonQuit, ReasonSessionEnd, ReasonSignal, ReasonOther} {
			t.Run(string(reason), func(t *testing.T) {
				verifyNoLeaks(t)
				cfg := testConfig(t, "")
				ln := &tcpListen{}
				a := startTestAgent(t, cfg, ln)
				a.drainTimeout = time.Hour // the client closing is what ends the drain

				_, addr := ln.snapshot()
				c := dialRaw(t, addr)
				c.status(t)

				errc := shutdownAsync(a, context.Background(), reason)
				m := c.waitMethod(t, ipc.MethodShutdown)
				if _, hasID := m["id"]; hasID {
					t.Errorf("agent.shutdown has id %s, want a notification without id", m["id"])
				}
				if got := shutdownReason(t, m); got != string(reason) {
					t.Errorf("agent.shutdown reason = %q, want %q", got, reason)
				}

				if err := c.conn.Close(); err != nil {
					t.Fatalf("client close: %v", err)
				}
				if err := waitShutdown(t, errc); err != nil {
					t.Errorf("Shutdown: %v", err)
				}
				if err := a.CloseLog(); err != nil {
					t.Errorf("CloseLog: %v", err)
				}
			})
		}
	})

	t.Run("EXE-014/cliente que fecha libera o Shutdown antes do limite", func(t *testing.T) {
		verifyNoLeaks(t)
		cfg := testConfig(t, "")
		ln := &tcpListen{}
		a := startTestAgent(t, cfg, ln)
		// With a 1 h drain limit and a ctx without deadline, only the client
		// closing can end Shutdown within the test timeout.
		a.drainTimeout = time.Hour

		_, addr := ln.snapshot()
		c := dialRaw(t, addr)
		c.status(t)

		errc := shutdownAsync(a, context.Background(), ReasonQuit)
		c.waitMethod(t, ipc.MethodShutdown)
		assertShutdownPending(t, errc)

		if err := c.conn.Close(); err != nil {
			t.Fatalf("client close: %v", err)
		}
		if err := waitShutdown(t, errc); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
		assertStoreClosed(t, cfg)
		if err := a.CloseLog(); err != nil {
			t.Fatalf("CloseLog: %v", err)
		}
		if recs := findLog(t, cfg, "WARN", msgDrainTimeout); len(recs) != 0 {
			t.Errorf("drain Warn logged although the client closed: %v", recs)
		}
	})

	t.Run("EXE-014/cliente que não fecha: loga Warn com pending=1 e conclui fechando o store", func(t *testing.T) {
		verifyNoLeaks(t)
		cfg := testConfig(t, "")
		ln := &tcpListen{}
		a := startTestAgent(t, cfg, ln)
		a.drainTimeout = 50 * time.Millisecond

		_, addr := ln.snapshot()
		c := dialRaw(t, addr) // reads everything but never closes
		c.status(t)

		sctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		errc := shutdownAsync(a, sctx, ReasonQuit)
		// A drain that times out is not a shutdown error: Warn and continue (design.md).
		if err := waitShutdown(t, errc); err != nil {
			t.Errorf("Shutdown: %v, want nil (drain timeout only logs Warn)", err)
		}

		// Server.Close closed the remaining connection after notifying it.
		msgs := c.waitEOF(t)
		if n := countMethod(msgs, ipc.MethodShutdown); n != 1 {
			t.Errorf("client received %d agent.shutdown, want 1", n)
		}
		assertStoreClosed(t, cfg)

		if err := a.CloseLog(); err != nil {
			t.Fatalf("CloseLog: %v", err)
		}
		recs := findLog(t, cfg, "WARN", msgDrainTimeout)
		if len(recs) != 1 {
			t.Fatalf("%d Warn %q records, want 1", len(recs), msgDrainTimeout)
		}
		if p, ok := recs[0]["pending"].(float64); !ok || p != 1 {
			t.Errorf("pending = %v, want 1", recs[0]["pending"])
		}
		if e, ok := recs[0]["error"].(string); !ok || e == "" {
			t.Errorf("error attr = %v, want a non-empty string", recs[0]["error"])
		}
	})

	t.Run("AGT-003/ordem Drain → Server.Close → Store.Close", func(t *testing.T) {
		verifyNoLeaks(t)
		cfg := testConfig(t, "")
		ln := &tcpListen{}
		a := startTestAgent(t, cfg, ln)
		a.drainTimeout = time.Hour

		_, addr := ln.snapshot()
		c1 := dialRaw(t, addr)
		c1.status(t)

		errc := shutdownAsync(a, context.Background(), ReasonQuit)

		// 1. Drain: the notification arrives while Shutdown is still running…
		c1.waitMethod(t, ipc.MethodShutdown)
		assertShutdownPending(t, errc)
		// …the server still accepts (Server.Close not called yet): a new
		// connection gets agent.shutdown as its first message…
		c2 := dialRaw(t, addr)
		if first := c2.next(t); first.method() != ipc.MethodShutdown {
			t.Errorf("first message on a connection accepted while draining = %v, want %s", first, ipc.MethodShutdown)
		}
		// …and the store is still open.
		if runtime.GOOS == "windows" {
			if err := os.Remove(dbPath(cfg)); err == nil {
				t.Fatal("library.db removed during the drain: store closed before Drain ended")
			}
		}
		assertShutdownPending(t, errc)

		// 2. Clients close: Drain ends, then Server.Close and Store.Close.
		_ = c1.conn.Close()
		_ = c2.conn.Close()
		if err := waitShutdown(t, errc); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
		if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			conn.Close()
			t.Error("dial after Shutdown succeeded, want listener closed by Server.Close")
		}
		assertStoreClosed(t, cfg)
		if err := a.CloseLog(); err != nil {
			t.Errorf("CloseLog: %v", err)
		}
	})

	t.Run("AGT-003/Shutdown idempotente com reason", func(t *testing.T) {
		verifyNoLeaks(t)
		cfg := testConfig(t, "")
		ln := &tcpListen{}
		a := startTestAgent(t, cfg, ln)
		a.drainTimeout = time.Hour

		_, addr := ln.snapshot()
		c := dialRaw(t, addr)
		c.status(t)

		errc := shutdownAsync(a, context.Background(), ReasonQuit)
		m := c.waitMethod(t, ipc.MethodShutdown)
		if got := shutdownReason(t, m); got != string(ReasonQuit) {
			t.Errorf("agent.shutdown reason = %q, want %q", got, ReasonQuit)
		}
		_ = c.conn.Close()
		if err := waitShutdown(t, errc); err != nil {
			t.Fatalf("first Shutdown: %v", err)
		}

		// A second Shutdown with another reason does nothing and returns the
		// same result without waiting (ctx without deadline, 1 h drain limit).
		if err := waitShutdown(t, shutdownAsync(a, context.Background(), ReasonSignal)); err != nil {
			t.Errorf("second Shutdown: %v", err)
		}
		c.mu.Lock()
		n := countMethod(c.all, ipc.MethodShutdown)
		c.mu.Unlock()
		if n != 1 {
			t.Errorf("client received %d agent.shutdown, want 1", n)
		}
		assertStoreClosed(t, cfg)
		if err := a.CloseLog(); err != nil {
			t.Errorf("CloseLog: %v", err)
		}
	})
}

var errAcceptTest = errors.New("accept: cannot create pipe instance (test)")

// failingListener optionally hands out one connection, then fails Accept
// failures times with errAcceptTest (a listener that stays usable), then
// blocks until Close.
type failingListener struct {
	mu       sync.Mutex
	first    net.Conn
	failures int
	accepts  int

	closeOnce sync.Once
	closed    chan struct{}
}

func newFailingListener(first net.Conn, failures int) *failingListener {
	return &failingListener{first: first, failures: failures, closed: make(chan struct{})}
}

func (l *failingListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	l.accepts++
	if c := l.first; c != nil {
		l.first = nil
		l.mu.Unlock()
		return c, nil
	}
	if l.failures > 0 {
		l.failures--
		l.mu.Unlock()
		return nil, errAcceptTest
	}
	l.mu.Unlock()
	<-l.closed
	return nil, net.ErrClosed
}

func (l *failingListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *failingListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

func (l *failingListener) acceptCalls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.accepts
}

// startFailingAgent starts an agent serving on fl with clock and registers the
// cleanup (Shutdown, CloseLog, cancel).
func startFailingAgent(t *testing.T, cfg config, clock *domain.FakeClock, fl *failingListener) *instance {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	a, err := startAgent(ctx, cfg, clock, emptyEnv, func() (net.Listener, error) { return fl, nil })
	if err != nil {
		cancel()
		t.Fatalf("startAgent: %v", err)
	}
	t.Cleanup(func() {
		sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
		defer scancel()
		_ = a.Shutdown(sctx, ReasonOther)
		_ = a.CloseLog()
		cancel()
	})
	return a
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// driveAcceptFailures steps the fake clock through the 9 backoffs between the
// 10 consecutive Accept failures (IPC-018) and waits for IPCFailed. Pending
// waiters at each step: the heartbeat timer and the accept backoff.
func driveAcceptFailures(t *testing.T, clock *domain.FakeClock, a *instance) {
	t.Helper()
	for i := 1; i <= 9; i++ {
		bctx, bcancel := context.WithTimeout(context.Background(), testTimeout)
		err := clock.BlockUntilWaiters(bctx, 2)
		bcancel()
		if err != nil {
			t.Fatalf("backoff %d: BlockUntilWaiters: %v", i, err)
		}
		if isClosed(a.IPCFailed()) {
			t.Fatalf("IPCFailed closed after %d Accept failures, want only after 10", i)
		}
		clock.Advance(5 * time.Second) // >= the longest backoff
	}
	select {
	case <-a.IPCFailed():
	case <-time.After(testTimeout):
		t.Fatalf("IPCFailed not closed within %v after 10 Accept failures", testTimeout)
	}
}

func TestIPCFailed(t *testing.T) {
	t.Run("EXE-026/Serve com 10 falhas de Accept fecha IPCFailed e loga Error IPC indisponível", func(t *testing.T) {
		verifyNoLeaks(t)
		cfg := testConfig(t, "")
		clock := testClock()
		fl := newFailingListener(nil, 10)
		a := startFailingAgent(t, cfg, clock, fl)

		driveAcceptFailures(t, clock, a)
		if n := fl.acceptCalls(); n != 10 {
			t.Errorf("Accept called %d times, want 10", n)
		}

		sctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		if err := a.Shutdown(sctx, ReasonOther); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
		assertStoreClosed(t, cfg)
		if err := a.CloseLog(); err != nil {
			t.Fatalf("CloseLog: %v", err)
		}

		recs := findLog(t, cfg, "ERROR", msgIPCFailed)
		if len(recs) != 1 {
			t.Fatalf("%d Error %q records, want 1", len(recs), msgIPCFailed)
		}
		if e, ok := recs[0]["error"].(string); !ok || e == "" {
			t.Errorf("error attr = %v, want a non-empty string", recs[0]["error"])
		}
		if old := findLog(t, cfg, "ERROR", "ipc: serve ended"); len(old) != 0 {
			t.Errorf("old log %q still written: %v", "ipc: serve ended", old)
		}
	})

	t.Run("EXE-026/Shutdown normal não fecha IPCFailed", func(t *testing.T) {
		t.Run("Server.Close pelo Shutdown", func(t *testing.T) {
			verifyNoLeaks(t)
			cfg := testConfig(t, "")
			a := startTestAgent(t, cfg, &tcpListen{})
			if isClosed(a.IPCFailed()) {
				t.Fatal("IPCFailed closed right after startAgent")
			}
			shutdown(t, a)
			if isClosed(a.IPCFailed()) {
				t.Error("IPCFailed closed by a normal Shutdown")
			}
			if err := a.CloseLog(); err != nil {
				t.Fatalf("CloseLog: %v", err)
			}
			if recs := findLog(t, cfg, "ERROR", msgIPCFailed); len(recs) != 0 {
				t.Errorf("%q logged on a normal Shutdown: %v", msgIPCFailed, recs)
			}
		})
		t.Run("fim do ctx do Serve", func(t *testing.T) {
			verifyNoLeaks(t)
			cfg := testConfig(t, "")
			ln := &tcpListen{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a, err := startAgent(ctx, cfg, testClock(), emptyEnv, ln.listen)
			if err != nil {
				t.Fatalf("startAgent: %v", err)
			}
			cancel() // Serve returns context.Canceled
			// Shutdown waits for the Serve goroutine (wg) before returning.
			sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
			defer scancel()
			if err := a.Shutdown(sctx, ReasonSignal); err != nil {
				t.Errorf("Shutdown: %v", err)
			}
			if isClosed(a.IPCFailed()) {
				t.Error("IPCFailed closed when Serve ended by ctx cancellation")
			}
			if err := a.CloseLog(); err != nil {
				t.Fatalf("CloseLog: %v", err)
			}
			if recs := findLog(t, cfg, "ERROR", msgIPCFailed); len(recs) != 0 {
				t.Errorf("%q logged after ctx cancellation: %v", msgIPCFailed, recs)
			}
		})
	})

	t.Run("EXE-027/Shutdown depois de IPCFailed não espera o drain", func(t *testing.T) {
		verifyNoLeaks(t)
		cfg := testConfig(t, "")
		clock := testClock()
		serverEnd, clientEnd := net.Pipe()
		c := newRawClient(t, clientEnd) // never closes before Shutdown
		fl := newFailingListener(serverEnd, 10)
		a := startFailingAgent(t, cfg, clock, fl)

		driveAcceptFailures(t, clock, a)
		// Serve closed every connection before returning.
		c.waitEOF(t)

		// ctx without deadline and a 1 h drain limit: Shutdown must not wait.
		a.drainTimeout = time.Hour
		if err := waitShutdown(t, shutdownAsync(a, context.Background(), ReasonOther)); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
		c.mu.Lock()
		n := countMethod(c.all, ipc.MethodShutdown)
		c.mu.Unlock()
		if n != 0 {
			t.Errorf("client received %d agent.shutdown after Serve ended, want 0", n)
		}
		assertStoreClosed(t, cfg)
		if err := a.CloseLog(); err != nil {
			t.Errorf("CloseLog: %v", err)
		}
		if recs := findLog(t, cfg, "WARN", msgDrainTimeout); len(recs) != 0 {
			t.Errorf("drain Warn logged after IPCFailed: %v", recs)
		}
	})
}

func TestShutdownWithPausedSession(t *testing.T) {
	t.Run("AGT-013/ordem: BeginShutdown, energia, fontes, roteador, Drain, Server.Close, Tracker.Close, feedback, Store.Close", func(t *testing.T) {
		var (
			mu    sync.Mutex
			order []string
		)
		add := func(step string) {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, step)
		}

		src := newFakeSource("order-src", nil)
		powerWatcher := &fakePowerWatcher{}

		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
					return platformParts{
						Sources: []input.Source{
							&sourceCloseRecorder{
								Source: src,
								onClose: func() {
									add("source")
								},
							},
						},
						Power: func(onSuspend func(at time.Time), onWake func(at time.Time, userPresent bool)) (io.Closer, error) {
							return &closerRecorder{
								Closer: powerWatcher,
								onClose: func() {
									add("power")
								},
							}, nil
						},
						NewOverlay: func() (feedback.Overlay, error) {
							return &overlayCloseRecorder{
								Overlay: feedback.NopOverlay{},
								onClose: func() {
									add("feedback-overlay")
								},
							}, nil
						},
						NewSound: func() (feedback.Sound, error) {
							return &soundCloseRecorder{
								Sound: feedback.NopSound{},
								onClose: func() {
									add("feedback-sound")
								},
							}, nil
						},
					}
				}
			},
		})

		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Shutdown Order Game")
		w.launch(t, c, gameID)

		c.rpcOK(t, ipc.MethodSessionPause, nil)
		_ = c.waitChanges(t, 2)

		sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
		defer scancel()

		if err := w.a.Shutdown(sctx, ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}

		mu.Lock()
		got := make([]string, len(order))
		copy(got, order)
		mu.Unlock()

		pwrIdx := indexOf(got, "power")
		srcIdx := indexOf(got, "source")
		fbkOvIdx := indexOf(got, "feedback-overlay")
		fbkSndIdx := indexOf(got, "feedback-sound")

		if pwrIdx == -1 || srcIdx == -1 || fbkOvIdx == -1 || fbkSndIdx == -1 {
			t.Fatalf("missing shutdown step: %v", got)
		}

		// power and sources must close before feedback
		if pwrIdx > fbkOvIdx || pwrIdx > fbkSndIdx {
			t.Errorf("power (%d) should close before feedback (%d, %d)", pwrIdx, fbkOvIdx, fbkSndIdx)
		}
		if srcIdx > fbkOvIdx || srcIdx > fbkSndIdx {
			t.Errorf("source (%d) should close before feedback (%d, %d)", srcIdx, fbkOvIdx, fbkSndIdx)
		}
	})

	t.Run("AGT-013/sessão Paused encerra com agent-shutdown e a pausa fechada em ended_at", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})

		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Paused Shutdown Game")
		w.launch(t, c, gameID)

		// Advance 35s and pause (active seconds >= 30 so it ends as 'ended', not 'discarded')
		w.clock.Advance(35 * time.Second)
		c.rpcOK(t, ipc.MethodSessionPause, nil)
		_ = c.waitChanges(t, 2)

		// Advance 20s in paused state
		w.clock.Advance(20 * time.Second)
		shutdownAt := w.clock.Now()

		sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
		defer scancel()
		if err := w.a.Shutdown(sctx, ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}

		// Verify session in store
		sessions, _ := sessionsAfterShutdown(t, w.cfg)
		if len(sessions) == 0 {
			t.Fatalf("no session rows after shutdown")
		}
		var (
			sessID int64
			sess   sessionRow
		)
		for id, r := range sessions {
			sessID = id
			sess = r
			break
		}

		if sess.state != string(domain.SessionEnded) {
			t.Errorf("session state = %q; want %q", sess.state, domain.SessionEnded)
		}
		if !sess.endedAt.Valid || sess.endedAt.Int64 != shutdownAt.UnixMilli() {
			t.Errorf("ended_at = %v; want %v", sess.endedAt, shutdownAt.UnixMilli())
		}
		if sess.activeSeconds != 35 {
			t.Errorf("active_seconds = %d; want 35", sess.activeSeconds)
		}
		if !sess.endReason.Valid || sess.endReason.String != string(domain.EndAgentShutdown) {
			t.Errorf("end_reason = %v; want %q", sess.endReason, domain.EndAgentShutdown)
		}

		st := openDB(t, w.cfg)
		defer st.Close()
		pauses, err := st.Pauses(context.Background(), sessID)
		if err != nil || len(pauses) != 1 {
			t.Fatalf("get pauses: %v (len %d)", err, len(pauses))
		}
		if pauses[0].ResumedAt.IsZero() || !pauses[0].ResumedAt.Equal(shutdownAt) {
			t.Errorf("pause resumed_at = %v; want %v", pauses[0].ResumedAt, shutdownAt)
		}
	})

	t.Run("AGT-013/nenhum feedback no encerramento", func(t *testing.T) {
		overlay := &blockingOverlay{}
		sound := &fakeSoundRecorder{}

		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
					return platformParts{
						NewOverlay: func() (feedback.Overlay, error) { return overlay, nil },
						NewSound:   func() (feedback.Sound, error) { return sound, nil },
					}
				}
			},
		})

		c := w.client(t)
		gameID, _ := w.addGame(t, c, "No FBK Shutdown Game")
		w.launch(t, c, gameID)

		c.rpcOK(t, ipc.MethodSessionPause, nil)
		_ = c.waitChanges(t, 2)

		// Record count after pause
		overlay.mu.Lock()
		showsBefore := len(overlay.shows)
		overlay.mu.Unlock()

		sound.mu.Lock()
		cuesBefore := len(sound.cues)
		sound.mu.Unlock()

		sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
		defer scancel()
		if err := w.a.Shutdown(sctx, ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}

		overlay.mu.Lock()
		showsAfter := len(overlay.shows)
		overlay.mu.Unlock()

		sound.mu.Lock()
		cuesAfter := len(sound.cues)
		sound.mu.Unlock()

		if showsAfter != showsBefore {
			t.Errorf("feedback overlay showed during shutdown: before %d, after %d", showsBefore, showsAfter)
		}
		if cuesAfter != cuesBefore {
			t.Errorf("feedback sound played during shutdown: before %d, after %d", cuesBefore, cuesAfter)
		}
	})

	t.Run("AGT-013/Shutdown termina em < 2 s com a sessão em Paused", func(t *testing.T) {
		w := newWiring(t, wiringOpts{})

		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Fast Shutdown Game")
		w.launch(t, c, gameID)

		c.rpcOK(t, ipc.MethodSessionPause, nil)
		_ = c.waitChanges(t, 2)

		start := time.Now()
		sctx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer scancel()

		if err := w.a.Shutdown(sctx, ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		elapsed := time.Since(start)

		if elapsed >= 2*time.Second {
			t.Errorf("Shutdown took %v; want < 2 s", elapsed)
		}
	})

	t.Run("AGT-013/gatilho depois do BeginShutdown é ignorado (PAU-005)", func(t *testing.T) {
		src := newFakeSource("post-shutdown-src", nil)
		var cfgRef config

		w := newWiring(t, wiringOpts{
			mutate: func(cfg *config) {
				cfg.LogLevel = "debug"
				cfgRef = *cfg
				cfg.Platform = func(c domain.Clock, l *slog.Logger) platformParts {
					return platformParts{
						Sources: []input.Source{src},
					}
				}
			},
		})

		c := w.client(t)
		gameID, _ := w.addGame(t, c, "Post Shutdown Trigger Game")
		w.launch(t, c, gameID)

		// Directly call BeginShutdown on tracker to simulate window before router closes
		w.a.tracker.BeginShutdown()

		// Emit trigger post-BeginShutdown
		w.clock.Advance(100 * time.Millisecond)
		src.emit(input.Trigger{Source: domain.PauseHotkey, At: w.clock.Now()})

		// Give router time to process
		time.Sleep(50 * time.Millisecond)

		// Shut down agent completely
		sctx, scancel := context.WithTimeout(context.Background(), testTimeout)
		defer scancel()
		_ = w.a.Shutdown(sctx, ReasonQuit)

		// Check debug log
		var foundDebug bool
		for _, rec := range logLines(t, agentLogPath(cfgRef)) {
			if rec["level"] == "DEBUG" && rec["msg"] == "agente encerrando; gatilho de pausa descartado" {
				foundDebug = true
				break
			}
		}
		if !foundDebug {
			t.Error("expected DEBUG record 'agente encerrando; gatilho de pausa descartado' not found")
		}
	})
}

// alterado em 2026-11-22 (fase 15, T13): TestShutdownTimeToBeat é novo (TTB-028). Os passos "ttb.begin" (logo
// depois do BeginShutdown do serviço, junto do do tracker) e "ttb.close" (logo depois do Close do serviço, antes
// de "store.close") são reportados por a.step.
func TestShutdownTimeToBeat(t *testing.T) {
	newWire := func(t *testing.T) (*ttbWire, *rawClient) {
		t.Helper()
		w := newTTBWire(t)
		w.a.drainTimeout = 50 * time.Millisecond
		return w, w.client(t)
	}
	t.Run("TTB-028/BeginShutdown junto do tracker, Close antes do store, a busca em curso é abandonada sem gravar", func(t *testing.T) {
		w, c := newWire(t)
		var (
			mu    sync.Mutex
			steps []string
		)
		w.a.onShutdownStep = func(s string) {
			mu.Lock()
			defer mu.Unlock()
			steps = append(steps, s)
		}
		id, _ := w.addGame(t, c, "Hades")
		w.hltb.gate()
		w.get(t, c, id, true)
		if term := waitCall(t, w.hltb.entered); term != "Hades" {
			t.Fatalf("term = %q, want Hades", term)
		}

		if err := w.a.Shutdown(contextWithTestTimeout(t), ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		mu.Lock()
		got := append([]string(nil), steps...)
		mu.Unlock()
		begin, closed := indexOf(got, "ttb.begin"), indexOf(got, "ttb.close")
		router, server, store := indexOf(got, "router.close"), indexOf(got, "server.close"), indexOf(got, "store.close")
		if begin == -1 || closed == -1 || router == -1 || server == -1 || store == -1 {
			t.Fatalf("missing shutdown step: %v", got)
		}
		if !(begin < router && begin < server) {
			t.Errorf("ttb.begin (%d) must come with the tracker's BeginShutdown, before router.close (%d) and server.close (%d): %v", begin, router, server, got)
		}
		if !(closed < store) {
			t.Errorf("ttb.close (%d) must come before store.close (%d): %v", closed, store, got)
		}
		msgs := c.waitEOF(t)
		if ids := changedIDs(t, msgs); len(ids) != 0 {
			t.Errorf("timeToBeat.changed after the shutdown began: %v", ids)
		}
		st := openDB(t, w.cfg)
		defer func() { _ = st.Close() }()
		if _, ok, err := st.GetTimeToBeat(t.Context(), id); err != nil || ok {
			t.Errorf("row after an abandoned request: ok=%v err=%v, want none", ok, err)
		}
	})

	t.Run("TTB-028/nenhuma notificação depois do BeginShutdown, mesmo que a busca termine", func(t *testing.T) {
		w, c := newWire(t)
		id, _ := w.addGame(t, c, "Hades")
		w.hltb.gate()
		w.get(t, c, id, true)
		waitCall(t, w.hltb.entered)
		// The request is let go only after the service knows the agent is shutting down.
		w.a.onShutdownStep = func(s string) {
			if s == "ttb.begin" {
				w.hltb.openGate()
			}
		}
		if err := w.a.Shutdown(contextWithTestTimeout(t), ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		if ids := changedIDs(t, c.waitEOF(t)); len(ids) != 0 {
			t.Errorf("timeToBeat.changed after BeginShutdown: %v", ids)
		}
	})

	t.Run("TTB-028/sem trabalho em curso o encerramento é limpo e sem goroutine restante", func(t *testing.T) {
		w, c := newWire(t)
		w.addGame(t, c, "Hades")
		if err := w.a.Shutdown(contextWithTestTimeout(t), ReasonQuit); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
		// newWiring's goleak check (registered before the agent) finds the worker of the service gone.
	})
}

type sourceCloseRecorder struct {
	input.Source
	onClose func()
}

func (r *sourceCloseRecorder) Close() error {
	if r.onClose != nil {
		r.onClose()
	}
	return r.Source.Close()
}

type closerRecorder struct {
	io.Closer
	onClose func()
}

func (r *closerRecorder) Close() error {
	if r.onClose != nil {
		r.onClose()
	}
	return r.Closer.Close()
}

type overlayCloseRecorder struct {
	feedback.Overlay
	onClose func()
}

func (r *overlayCloseRecorder) Close() error {
	if r.onClose != nil {
		r.onClose()
	}
	return r.Overlay.Close()
}

type soundCloseRecorder struct {
	feedback.Sound
	onClose func()
}

func (r *soundCloseRecorder) Close() error {
	if r.onClose != nil {
		r.onClose()
	}
	return r.Sound.Close()
}
