package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

var (
	// ErrConnectionLost is returned by Call when the connection drops while the
	// call is waiting for its response.
	ErrConnectionLost = errors.New("ipc: connection lost")
	// ErrClientClosed is returned by Call after Close.
	ErrClientClosed = errors.New("ipc: client closed")

	errNilConn = errors.New("ipc: dialer returned no connection and no error")
)

// Reconnect backoff (IPC-013): starts at backoffMin, doubles after every wait,
// capped at backoffMax, no jitter, reset after a successful connection.
const (
	backoffMin = 250 * time.Millisecond
	backoffMax = 5 * time.Second
)

// Dialer opens a connection to the agent. It must return promptly once ctx is done.
type Dialer func(ctx context.Context) (net.Conn, error)

// Client is a JSON-RPC 2.0 client that keeps one connection to the agent,
// reconnecting with backoff when it drops. It is safe for concurrent use.
type Client struct {
	dial  Dialer
	clock domain.Clock
	log   *slog.Logger

	ctx    context.Context // cancelled by Close
	cancel context.CancelFunc

	notifications chan Notification
	state         chan struct{} // buffer 1; coalesced connect/drop signals
	nextID        atomic.Uint64

	mu        sync.Mutex
	conn      *clientConn   // current connection; nil while disconnected
	connReady chan struct{} // closed when conn becomes available; replaced on each drop
	closed    bool

	closeOnce sync.Once
	done      chan struct{} // closed when the connection goroutine exits
}

// NewClient starts the goroutine that owns the dial -> read -> drop -> backoff
// cycle. The first dial is immediate. The clock is used only for backoff waits.
func NewClient(dial Dialer, clock domain.Clock, log *slog.Logger) *Client {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		dial:          dial,
		clock:         clock,
		log:           log,
		ctx:           ctx,
		cancel:        cancel,
		notifications: make(chan Notification, NotificationBuffer),
		state:         make(chan struct{}, 1),
		connReady:     make(chan struct{}),
		done:          make(chan struct{}),
	}
	go c.run()
	return c
}

// Notifications returns the channel of server notifications. Its buffer holds
// NotificationBuffer values; when full, new notifications are dropped. It is
// closed by Close.
func (c *Client) Notifications() <-chan Notification { return c.notifications }

// Connected reports whether a connection to the agent is currently established.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}

// StateChanged returns a channel that receives a value after the client
// connects or loses its connection (EXE-019). It has a buffer of 1 and signals
// are coalesced: consecutive changes with no reader leave a single pending
// signal, so readers must check Connected. A failed dial is not a change.
// Notifications read before a drop are already in Notifications when the drop
// is signalled. The channel is closed by Close.
func (c *Client) StateChanged() <-chan struct{} { return c.state }

// signalState emits a coalesced, non-blocking state signal. Only the run
// goroutine (via serve) calls it, so it is the only sender on c.state.
func (c *Client) signalState() {
	select {
	case c.state <- struct{}{}:
	default:
	}
}

// Close stops reconnecting, closes the connection and returns after the
// connection goroutine has exited. It is idempotent.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		cc := c.conn
		c.mu.Unlock()
		c.cancel()
		if cc != nil {
			_ = cc.conn.Close()
		}
	})
	<-c.done
	return nil
}

// Call sends method with params and decodes the result into result (if not
// nil). Without a connection it waits for the next one until ctx is done.
// A JSON-RPC error response is returned as the received *Error.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	var rawParams json.RawMessage
	if params != nil {
		p, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("ipc: call %s: encode params: %w", method, err)
		}
		rawParams = p
	}
	for {
		cc, err := c.waitConn(ctx, method)
		if err != nil {
			return err
		}
		res, sent, err := c.callOn(ctx, cc, method, rawParams)
		if err != nil {
			if !sent && errors.Is(err, ErrConnectionLost) {
				continue // dropped before the request was written: wait for the next connection
			}
			return err
		}
		if res.err != nil {
			return res.err
		}
		if result != nil && len(res.result) > 0 {
			if err := json.Unmarshal(res.result, result); err != nil {
				return fmt.Errorf("ipc: call %s: decode result: %w", method, err)
			}
		}
		return nil
	}
}

// waitConn returns the current connection, waiting for one if disconnected.
func (c *Client) waitConn(ctx context.Context, method string) (*clientConn, error) {
	for {
		c.mu.Lock()
		closed, cc, ready := c.closed, c.conn, c.connReady
		c.mu.Unlock()
		if closed {
			return nil, ErrClientClosed
		}
		if cc != nil {
			return cc, nil
		}
		select {
		case <-ready:
		case <-c.ctx.Done():
			return nil, ErrClientClosed
		case <-ctx.Done():
			return nil, fmt.Errorf("ipc: call %s: waiting for connection: %w", method, ctx.Err())
		}
	}
}

// callOn sends one request on cc and waits for its response. sent reports
// whether the request was handed to the connection.
func (c *Client) callOn(ctx context.Context, cc *clientConn, method string, params json.RawMessage) (callResult, bool, error) {
	id := c.nextID.Add(1)
	ch := make(chan callResult, 1)
	if !cc.addPending(id, ch) {
		return callResult{}, false, ErrConnectionLost
	}
	defer cc.removePending(id)

	req := Request{JSONRPC: Version, ID: json.RawMessage(strconv.FormatUint(id, 10)), Method: method, Params: params}
	// Hand the request to the connection's writer. The handoff is unbuffered,
	// so giving up here means the request was never written.
	select {
	case cc.out <- outgoing{id: id, req: req}:
	case <-cc.lost:
		return callResult{}, false, ErrConnectionLost
	case <-ctx.Done():
		return callResult{}, false, fmt.Errorf("ipc: call %s: sending request: %w", method, ctx.Err())
	}

	select {
	case res := <-ch:
		return res, true, nil
	case <-cc.lost:
		select {
		case res := <-ch: // the response won the race with the drop
			return res, true, nil
		default:
		}
		if c.ctx.Err() != nil {
			return callResult{}, true, ErrClientClosed
		}
		return callResult{}, true, fmt.Errorf("ipc: call %s: %w", method, ErrConnectionLost)
	case <-ctx.Done():
		return callResult{}, true, fmt.Errorf("ipc: call %s: %w", method, ctx.Err())
	}
}

// run owns the connection cycle; it exits when Close cancels c.ctx.
func (c *Client) run() {
	defer close(c.done)
	defer close(c.notifications) // run is the only sender
	defer close(c.state)         // run is the only sender
	backoff := backoffMin
	for {
		conn, err := c.dial(c.ctx)
		if err == nil && conn == nil {
			err = errNilConn // a misbehaving Dialer counts as a failed dial
		}
		if c.ctx.Err() != nil {
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		if err != nil {
			level := slog.LevelDebug
			if errors.Is(err, ErrUntrustedServer) {
				level = slog.LevelWarn // possible pipe squatting
			}
			c.log.Log(c.ctx, level, "ipc: dial failed", "error", err.Error(), "retry_in", backoff.String())
		} else {
			backoff = backoffMin
			c.serve(conn)
			if c.ctx.Err() != nil {
				return
			}
		}
		select {
		case <-c.ctx.Done():
			return
		case <-c.clock.After(backoff):
		}
		backoff = min(backoff*2, backoffMax)
	}
}

// serve publishes conn as the current connection, starts its writer goroutine
// and reads from it until it fails. Then it closes conn, fails the pending
// calls with ErrConnectionLost and waits for the writer to exit.
func (c *Client) serve(conn net.Conn) {
	cc := &clientConn{
		conn:    conn,
		enc:     NewEncoder(conn),
		out:     make(chan outgoing),
		pending: make(map[uint64]chan callResult),
		lost:    make(chan struct{}),
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = conn.Close()
		return
	}
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		c.writeLoop(cc)
	}()
	c.conn = cc
	close(c.connReady)
	c.mu.Unlock()
	c.signalState()
	c.log.Debug("ipc: connected")

	c.readLoop(cc)

	c.mu.Lock()
	c.conn = nil
	c.connReady = make(chan struct{})
	c.mu.Unlock()
	c.signalState()  // readLoop has returned, so notifications read before EOF are already delivered
	_ = conn.Close() // unblocks a writer stuck in Write
	cc.markLost()    // stops an idle writer and fails pending calls
	writer.Wait()
}

// writeLoop is the only writer of cc. It exits when cc is lost or a write
// fails (it then closes the connection so readLoop ends and serve cleans up).
// Requests whose caller gave up before they were taken here are never sent;
// ones abandoned while queued behind another write are skipped.
func (c *Client) writeLoop(cc *clientConn) {
	for {
		select {
		case <-cc.lost:
			return
		case o := <-cc.out:
			if !cc.isPending(o.id) {
				continue // caller gave up before the request was written
			}
			if err := cc.enc.Encode(o.req); err != nil {
				c.log.Debug("ipc: connection write failed", "error", err.Error())
				_ = cc.conn.Close()
				return
			}
		}
	}
}

// clientIncoming is any message the server may send.
type clientIncoming struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
}

func (c *Client) readLoop(cc *clientConn) {
	dec := NewDecoder(cc.conn)
	for {
		line, err := dec.Next()
		if err != nil {
			switch {
			case errors.Is(err, io.EOF), errors.Is(err, net.ErrClosed):
				c.log.Debug("ipc: connection closed")
			case errors.Is(err, ErrMessageTooLarge):
				c.log.Warn("ipc: message too large, dropping connection", "limit", MaxMessageSize)
			default:
				c.log.Debug("ipc: connection read failed", "error", err.Error())
			}
			return
		}
		var msg clientIncoming
		if err := json.Unmarshal(line, &msg); err != nil {
			c.log.Warn("ipc: invalid message from server", "error", err.Error())
			continue
		}
		if msg.Method != "" {
			if len(msg.ID) != 0 {
				c.log.Debug("ipc: ignoring server request", "method", msg.Method)
				continue
			}
			c.deliver(Notification{Method: msg.Method, Params: msg.Params})
			continue
		}
		id, err := strconv.ParseUint(string(msg.ID), 10, 64)
		if err != nil {
			c.log.Warn("ipc: response with unknown id", "id", string(msg.ID))
			continue
		}
		res := callResult{result: msg.Result}
		if msg.Error != nil {
			res.err = msg.Error
		}
		if !cc.resolve(id, res) {
			c.log.Warn("ipc: response with unknown id", "id", string(msg.ID))
		}
	}
}

// deliver hands n to Notifications without blocking; when the buffer is full n is dropped.
func (c *Client) deliver(n Notification) {
	select {
	case c.notifications <- n:
	default:
		c.log.Debug("ipc: notification dropped, buffer full", "method", n.Method)
	}
}

type callResult struct {
	result json.RawMessage
	err    *Error
}

// outgoing is a request handed to the connection's writer.
type outgoing struct {
	id  uint64
	req Request
}

// clientConn is one established connection and its pending calls.
type clientConn struct {
	conn net.Conn
	enc  *Encoder      // used only by writeLoop
	out  chan outgoing // unbuffered handoff from callOn to writeLoop; never closed

	mu      sync.Mutex
	pending map[uint64]chan callResult
	isLost  bool
	lost    chan struct{} // closed when the connection drops
}

func (cc *clientConn) addPending(id uint64, ch chan callResult) bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if cc.isLost {
		return false
	}
	cc.pending[id] = ch
	return true
}

func (cc *clientConn) removePending(id uint64) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	delete(cc.pending, id)
}

func (cc *clientConn) isPending(id uint64) bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	_, ok := cc.pending[id]
	return ok
}

// resolve delivers res to the call waiting for id and reports whether there
// was one (the id may be unknown or its caller may have given up).
func (cc *clientConn) resolve(id uint64, res callResult) bool {
	cc.mu.Lock()
	ch, ok := cc.pending[id]
	delete(cc.pending, id)
	cc.mu.Unlock()
	if ok {
		ch <- res // buffered 1, one response per id
	}
	return ok
}

func (cc *clientConn) markLost() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if !cc.isLost {
		cc.isLost = true
		close(cc.lost)
	}
}
