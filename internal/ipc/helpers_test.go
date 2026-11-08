package ipc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// guardTimeout only protects the test binary from hanging when the code under
// test misbehaves. It never drives behavior: logical time is always a FakeClock.
const guardTimeout = 5 * time.Second

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// recv waits for a value on ch, failing the test after guardTimeout.
func recv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(guardTimeout):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

// within runs fn in a goroutine and fails the test if it does not return in
// guardTimeout (e.g. a Close that never returns).
func within(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	recv(t, done, what)
}

// blockUntilWaiters waits until clock has at least n pending After calls.
func blockUntilWaiters(t *testing.T, clock *domain.FakeClock, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), guardTimeout)
	defer cancel()
	if err := clock.BlockUntilWaiters(ctx, n); err != nil {
		t.Fatalf("timed out waiting for %d clock waiter(s): %v", n, err)
	}
}

// hasPendingWaiters reports, without blocking, whether clock has at least n
// pending After calls. BlockUntilWaiters checks the count before ctx, so an
// already-cancelled ctx turns it into a synchronous probe. Advance fires and
// removes waiters synchronously, so right after Advance this tells whether a
// waiter fired.
func hasPendingWaiters(clock *domain.FakeClock, n int) bool {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return clock.BlockUntilWaiters(ctx, n) == nil
}

// ---------------------------------------------------------------------------
// In-memory listener (net.Pipe based).

type memAddr struct{}

func (memAddr) Network() string { return "mem" }
func (memAddr) String() string  { return "mem" }

var errListenerClosed = errors.New("test: mem listener closed")

// memListener is a net.Listener whose connections are net.Pipe pairs created by Dial.
type memListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newMemListener() *memListener {
	return &memListener{conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *memListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *memListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *memListener) Addr() net.Addr { return memAddr{} }

// Dial connects to the listener; it blocks until Accept takes the connection.
func (l *memListener) Dial(ctx context.Context) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case l.conns <- server:
		return client, nil
	case <-l.done:
		client.Close()
		server.Close()
		return nil, errListenerClosed
	case <-ctx.Done():
		client.Close()
		server.Close()
		return nil, ctx.Err()
	}
}

// switchDialer dials whichever memListener is current, simulating a server
// that restarts.
type switchDialer struct {
	mu sync.Mutex
	ln *memListener
}

func (d *switchDialer) set(ln *memListener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ln = ln
}

func (d *switchDialer) Dial(ctx context.Context) (net.Conn, error) {
	d.mu.Lock()
	ln := d.ln
	d.mu.Unlock()
	if ln == nil {
		return nil, errListenerClosed
	}
	return ln.Dial(ctx)
}

// ---------------------------------------------------------------------------
// Real server runner.

type serverRun struct {
	srv      *ipc.Server
	ln       net.Listener
	done     chan struct{} // closed when Serve returns
	serveErr error         // valid after done is closed
	stopOnce sync.Once
}

// runServe is a named function so goroutine dumps identify it.
func runServe(r *serverRun) {
	defer close(r.done)
	r.serveErr = r.srv.Serve(context.Background(), r.ln)
}

// startServer runs a real Server on ln and registers a cleanup that closes it
// and waits for Serve to return.
func startServer(t *testing.T, ln net.Listener, clock domain.Clock, handlers map[string]ipc.Handler, log *slog.Logger, opts ...ipc.ServerOption) *serverRun {
	t.Helper()
	if log == nil {
		log = discardLogger()
	}
	r := &serverRun{
		srv:  ipc.NewServer(clock, handlers, log, opts...),
		ln:   ln,
		done: make(chan struct{}),
	}
	go runServe(r)
	t.Cleanup(func() { r.stop(t) })
	return r
}

// stop closes the server and waits for Serve to return (once).
func (r *serverRun) stop(t *testing.T) {
	t.Helper()
	r.stopOnce.Do(func() {
		within(t, "Server.Close", func() {
			if err := r.srv.Close(); err != nil {
				t.Logf("Server.Close: %v", err)
			}
		})
		recv(t, r.done, "Serve to return after Close")
	})
}

// ---------------------------------------------------------------------------
// Raw NDJSON peer (talks to the real Server, or plays a fake server for the Client).

type rawConn struct {
	conn net.Conn
	dec  *ipc.Decoder
}

func newRawConn(t *testing.T, conn net.Conn) *rawConn {
	t.Helper()
	t.Cleanup(func() { conn.Close() })
	return &rawConn{conn: conn, dec: ipc.NewDecoder(conn)}
}

func dialRaw(t *testing.T, ln *memListener) *rawConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), guardTimeout)
	defer cancel()
	conn, err := ln.Dial(ctx)
	if err != nil {
		t.Fatalf("dial mem listener: %v", err)
	}
	return newRawConn(t, conn)
}

// send writes line followed by '\n'.
func (r *rawConn) send(t *testing.T, line string) {
	t.Helper()
	if err := r.conn.SetWriteDeadline(time.Now().Add(guardTimeout)); err != nil {
		t.Fatalf("set write deadline: %v", err)
	}
	if _, err := io.WriteString(r.conn, line+"\n"); err != nil {
		t.Fatalf("write %.80q: %v", line, err)
	}
}

// readLine reads the next line, failing on error or after guardTimeout.
func (r *rawConn) readLine(t *testing.T) []byte {
	t.Helper()
	line, err := r.next()
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	return line
}

func (r *rawConn) next() ([]byte, error) {
	if err := r.conn.SetReadDeadline(time.Now().Add(guardTimeout)); err != nil {
		return nil, fmt.Errorf("set read deadline: %w", err)
	}
	return r.dec.Next()
}

// ping sends agent.ping with the given id and checks the very next line is its
// {"pong":true} response (so nothing else was sent before it).
func (r *rawConn) ping(t *testing.T, id int) {
	t.Helper()
	r.send(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"agent.ping"}`, id))
	assertPong(t, r.readLine(t), id)
}

// assertPong checks line is the success response {"pong":true} for id. The
// line must be a JSON object (a []byte written without json.RawMessage would
// arrive as a base64 JSON string).
func assertPong(t *testing.T, line []byte, id int) {
	t.Helper()
	if len(line) == 0 || line[0] != '{' {
		t.Fatalf("response is not a JSON object (base64?): %.120q", line)
	}
	d := decodeResponse(t, line)
	if d.hasErr {
		t.Fatalf("response to ping id %d is error %d %q", id, d.code, d.msg)
	}
	if want := fmt.Sprint(id); d.id != want {
		t.Fatalf("response id = %s, want %s (line %q)", d.id, want, line)
	}
	if got := compactJSON(t, d.result); got != `{"pong":true}` {
		t.Fatalf("ping result = %s, want {\"pong\":true}", got)
	}
}

// readRequest reads one request sent by the Client.
func (r *rawConn) readRequest(t *testing.T) ipc.Request {
	t.Helper()
	line := r.readLine(t)
	var req ipc.Request
	if err := json.Unmarshal(line, &req); err != nil {
		t.Fatalf("client sent non-JSON request %q: %v", line, err)
	}
	if req.JSONRPC != ipc.Version {
		t.Fatalf("client request jsonrpc = %q, want %q", req.JSONRPC, ipc.Version)
	}
	if len(req.ID) == 0 {
		t.Fatalf("client request has no id: %q", line)
	}
	return req
}

// respond writes a success response with the request's id.
func (r *rawConn) respond(t *testing.T, req ipc.Request, result string) {
	t.Helper()
	r.send(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":%s}`, req.ID, result))
}

// notify writes a server-to-client notification.
func (r *rawConn) notify(t *testing.T, method, params string) {
	t.Helper()
	r.send(t, fmt.Sprintf(`{"jsonrpc":"2.0","method":%q,"params":%s}`, method, params))
}

// readEach reads one line from every conn concurrently (a server broadcasting
// over synchronous net.Pipe conns may write them in any order).
func readEach(t *testing.T, conns []*rawConn) [][]byte {
	t.Helper()
	type res struct {
		line []byte
		err  error
	}
	out := make([]chan res, len(conns))
	var wg sync.WaitGroup
	for i, c := range conns {
		out[i] = make(chan res, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			line, err := c.next()
			out[i] <- res{line, err}
		}()
	}
	wg.Wait()
	lines := make([][]byte, len(conns))
	for i := range conns {
		r := <-out[i]
		if r.err != nil {
			t.Fatalf("conn %d: read line: %v", i, r.err)
		}
		lines[i] = r.line
	}
	return lines
}

// assertHeartbeat checks line is exactly an agent.heartbeat notification (no
// id member) whose params are exactly {"uptimeSeconds":want}.
func assertHeartbeat(t *testing.T, line []byte, want int64) {
	t.Helper()
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(line, &keys); err != nil {
		t.Fatalf("heartbeat is not a JSON object: %v (%q)", err, line)
	}
	if _, ok := keys["id"]; ok {
		t.Fatalf("heartbeat must be a notification without id: %q", line)
	}
	if got := sortedKeys(keys); strings.Join(got, ",") != "jsonrpc,method,params" {
		t.Fatalf("heartbeat members = %v, want [jsonrpc method params] (%q)", got, line)
	}
	if got := compactJSON(t, keys["jsonrpc"]); got != `"2.0"` {
		t.Fatalf("heartbeat jsonrpc = %s", got)
	}
	if got := compactJSON(t, keys["method"]); got != `"agent.heartbeat"` {
		t.Fatalf("heartbeat method = %s", got)
	}
	if got, w := compactJSON(t, keys["params"]), fmt.Sprintf(`{"uptimeSeconds":%d}`, want); got != w {
		t.Fatalf("heartbeat params = %s, want %s", got, w)
	}
}

// exactSizeMessage returns an agent.ping request with the given id padded with
// JSON whitespace to exactly size bytes (without '\n').
func exactSizeMessage(t *testing.T, id, size int) string {
	t.Helper()
	prefix := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"agent.ping"`, id)
	pad := size - len(prefix) - 1
	if pad < 0 {
		t.Fatalf("size %d too small", size)
	}
	msg := prefix + strings.Repeat(" ", pad) + "}"
	if len(msg) != size {
		t.Fatalf("built %d bytes, want %d", len(msg), size)
	}
	return msg
}

// ---------------------------------------------------------------------------
// Fake dialer for the Client: each dial attempt is handed to the test, which
// decides whether it fails or connects to a raw fake server.

var errDialRefused = errors.New("test: dial refused")

type dialResult struct {
	conn net.Conn
	err  error
}

type dialAttempt struct {
	reply chan dialResult
}

type fakeDialer struct {
	attempts chan *dialAttempt
}

func newFakeDialer() *fakeDialer {
	return &fakeDialer{attempts: make(chan *dialAttempt)}
}

func (d *fakeDialer) Dial(ctx context.Context) (net.Conn, error) {
	a := &dialAttempt{reply: make(chan dialResult, 1)}
	select {
	case d.attempts <- a:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case r := <-a.reply:
		return r.conn, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// next waits for the client's next dial attempt.
func (d *fakeDialer) next(t *testing.T) *dialAttempt {
	t.Helper()
	return recv(t, d.attempts, "client dial attempt")
}

// noAttempt fails if a dial attempt is already waiting.
func (d *fakeDialer) noAttempt(t *testing.T, when string) {
	t.Helper()
	select {
	case a := <-d.attempts:
		a.fail()
		t.Fatalf("unexpected dial attempt %s", when)
	default:
	}
}

func (a *dialAttempt) fail() { a.reply <- dialResult{err: errDialRefused} }

// nilConn completes the attempt with (nil, nil), a misbehaving Dialer.
func (a *dialAttempt) nilConn() { a.reply <- dialResult{} }

// awaitBackoffAfterDrop waits until the client, after a dropped connection,
// either dials again (fails the test: the redial must wait 250 ms, IPC-013) or
// registers its backoff wait on clk.
func awaitBackoffAfterDrop(t *testing.T, fd *fakeDialer, clk *domain.FakeClock) {
	t.Helper()
	probeCtx, cancelProbe := context.WithCancel(context.Background())
	defer cancelProbe()
	waiter := make(chan error, 1)
	go func() { waiter <- clk.BlockUntilWaiters(probeCtx, 1) }()
	select {
	case a := <-fd.attempts:
		cancelProbe()
		<-waiter
		a.fail()
		t.Fatal("client redialed immediately after the drop; want a 250ms wait first")
	case err := <-waiter:
		if err != nil {
			t.Fatalf("waiting for backoff: %v", err)
		}
	case <-time.After(guardTimeout):
		cancelProbe()
		<-waiter
		t.Fatal("client neither redialed nor waited after the drop")
	}
}

// connect completes the attempt with a net.Pipe and returns the server side.
func (a *dialAttempt) connect(t *testing.T) *rawConn {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close() })
	a.reply <- dialResult{conn: client}
	return newRawConn(t, server)
}

// newClient creates a Client closed at cleanup.
func newClient(t *testing.T, dial ipc.Dialer, clock domain.Clock) *ipc.Client {
	t.Helper()
	c := ipc.NewClient(dial, clock, discardLogger())
	t.Cleanup(func() {
		within(t, "Client.Close", func() {
			if err := c.Close(); err != nil {
				t.Logf("Client.Close: %v", err)
			}
		})
	})
	return c
}

// callAsync runs Call in a goroutine and returns a channel with its error.
func callAsync(ctx context.Context, c *ipc.Client, method string, result any) <-chan error {
	errc := make(chan error, 1)
	go func() { errc <- c.Call(ctx, method, nil, result) }()
	return errc
}

func guardCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), guardTimeout)
	t.Cleanup(cancel)
	return ctx
}

// ---------------------------------------------------------------------------
// slog capture.

type logStore struct {
	mu    sync.Mutex
	lines []string
}

type captureHandler struct {
	store *logStore
	attrs []slog.Attr
}

func newCaptureLogger() (*slog.Logger, *logStore) {
	s := &logStore{}
	return slog.New(&captureHandler{store: s}), s
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s %s", r.Level, r.Message)
	for _, a := range h.attrs {
		fmt.Fprintf(&b, " %s", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s", a)
		return true
	})
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	h.store.lines = append(h.store.lines, b.String())
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &captureHandler{store: h.store, attrs: append(append([]slog.Attr(nil), h.attrs...), attrs...)}
}

func (h *captureHandler) WithGroup(string) slog.Handler { return h }

func (s *logStore) contains(sub string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

// containsAt reports whether a record at level has a message starting with
// msgPrefix and a formatted line containing sub ("" matches any).
func (s *logStore) containsAt(level slog.Level, msgPrefix, sub string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	head := level.String() + " " + msgPrefix
	for _, l := range s.lines {
		if strings.HasPrefix(l, head) && strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func (s *logStore) all() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.lines...)
}
