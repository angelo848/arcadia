package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// connQueueSize is the capacity of each connection's outgoing queue. Responses
// wait for room; heartbeats are dropped when the queue is full, so a slow
// connection never delays the others.
const connQueueSize = 16

// Backoff of Accept errors that leave the listener usable (anything other than
// net.ErrClosed): the wait starts at acceptBackoffMin, doubles after each
// consecutive failure up to acceptBackoffMax, and resets after a successful
// Accept. At the maxAcceptFailures-th consecutive failure Serve gives up.
const (
	acceptBackoffMin  = 100 * time.Millisecond
	acceptBackoffMax  = 5 * time.Second
	maxAcceptFailures = 10
)

var errServerServed = errors.New("ipc: Serve already called")

// serverConfig holds the values set by ServerOption.
type serverConfig struct {
	startedAt    time.Time
	hasStartedAt bool
}

// ServerOption configures a Server.
type ServerOption func(*serverConfig)

// WithStartedAt sets the instant the uptime of agent.heartbeat is measured
// from. Without it NewServer uses clock.Now().
func WithStartedAt(t time.Time) ServerOption {
	return func(c *serverConfig) {
		c.startedAt = t
		c.hasStartedAt = true
	}
}

// Server serves JSON-RPC 2.0 over NDJSON on the connections of a net.Listener
// and broadcasts agent.heartbeat to all of them every HeartbeatInterval.
type Server struct {
	clock     domain.Clock
	handlers  map[string]Handler
	log       *slog.Logger
	startedAt time.Time

	mu        sync.Mutex
	ln        net.Listener
	conns     map[*serverConn]struct{}
	cancel    context.CancelFunc // cancels the serve context; nil before Serve
	serveDone chan struct{}      // closed when Serve returns; nil before Serve
	closed    bool               // no new connections; set by Close or when Serve ends
	closeCall bool               // Close was called
	served    bool               // Serve was called; a Server serves at most once

	// wg tracks the heartbeat loop and the reader/writer goroutines of every connection.
	wg sync.WaitGroup
}

// NewServer returns a Server dispatching to handlers. Handler errors that are
// not *Error are logged before Dispatch turns them into -32603.
func NewServer(clock domain.Clock, handlers map[string]Handler, log *slog.Logger, opts ...ServerOption) *Server {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	var cfg serverConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	startedAt := cfg.startedAt
	if !cfg.hasStartedAt {
		startedAt = clock.Now()
	}
	wrapped := make(map[string]Handler, len(handlers))
	for method, h := range handlers {
		wrapped[method] = logHandlerErrors(log, method, h)
	}
	return &Server{
		clock:     clock,
		handlers:  wrapped,
		log:       log,
		startedAt: startedAt,
		conns:     make(map[*serverConn]struct{}),
	}
}

// logHandlerErrors wraps h so that errors which are not *Error (sent to the
// client only as "Internal error") are logged with their original text.
func logHandlerErrors(log *slog.Logger, method string, h Handler) Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		result, err := h(ctx, params)
		if err != nil {
			var rpcErr *Error
			if !errors.As(err, &rpcErr) || rpcErr == nil {
				log.Error("ipc: handler failed", "method", method, "error", err.Error())
			}
		}
		return result, err
	}
}

// StartedAt returns the instant uptime is measured from.
func (s *Server) StartedAt() time.Time { return s.startedAt }

// Serve accepts connections on ln until Close is called or ctx is done, then
// closes every connection and waits for all server goroutines. It returns nil
// after Close (also when Close came before Serve, in which case ln is just
// closed), ctx.Err() when ctx ended it, or the accept error otherwise. A Server
// serves once: a second Serve closes ln and returns an error.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.mu.Lock()
	if s.served {
		s.mu.Unlock()
		_ = ln.Close()
		return errServerServed
	}
	s.served = true
	if s.closed {
		// Close came before the first Serve: shutdown was requested, not an error.
		s.mu.Unlock()
		_ = ln.Close()
		return nil
	}
	s.ln = ln
	s.cancel = cancel
	s.serveDone = make(chan struct{})
	serveDone := s.serveDone
	s.wg.Add(1)
	go s.heartbeatLoop(serveCtx)
	s.mu.Unlock()
	defer close(serveDone)

	// Closing the listener when ctx ends unblocks Accept.
	stopDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(stopDone)
		_ = ln.Close()
	})

	err := s.acceptLoop(serveCtx, ln)

	if !stop() {
		<-stopDone
	}
	s.shutdown()
	s.wg.Wait()

	s.mu.Lock()
	byClose := s.closeCall
	s.mu.Unlock()
	switch {
	case byClose:
		return nil
	case ctx.Err() != nil:
		return ctx.Err()
	default:
		return err
	}
}

func (s *Server) acceptLoop(ctx context.Context, ln net.Listener) error {
	failures := 0
	backoff := acceptBackoffMin
	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.isClosed() || ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, net.ErrClosed) {
				return fmt.Errorf("ipc: accept: %w", err)
			}
			// The listener is still usable (e.g. go-winio failed to create the
			// next pipe instance): log and retry with backoff, up to a limit.
			failures++
			if failures >= maxAcceptFailures {
				s.log.Error("ipc: accept failed repeatedly, giving up", "error", err.Error(), "failures", failures)
				return fmt.Errorf("ipc: accept failed %d times in a row: %w", failures, err)
			}
			s.log.Warn("ipc: accept failed, retrying", "error", err.Error(), "failures", failures, "retry_in", backoff.String())
			select {
			case <-ctx.Done():
				return nil
			case <-s.clock.After(backoff):
			}
			backoff = min(2*backoff, acceptBackoffMax)
			continue
		}
		failures = 0
		backoff = acceptBackoffMin
		s.startConn(ctx, conn)
	}
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// startConn registers conn and starts its reader and writer goroutines.
func (s *Server) startConn(ctx context.Context, conn net.Conn) {
	c := &serverConn{
		conn: conn,
		enc:  NewEncoder(conn),
		out:  make(chan any, connQueueSize),
		done: make(chan struct{}),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		_ = conn.Close()
		return
	}
	s.conns[c] = struct{}{}
	s.wg.Add(2)
	go s.readLoop(ctx, c)
	go s.writeLoop(c)
}

// Close stops accepting, closes the listener and every connection, and returns
// after Serve and all server goroutines have finished. It is idempotent.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closeCall = true
	s.mu.Unlock()
	s.shutdown()
	s.mu.Lock()
	serveDone := s.serveDone
	s.mu.Unlock()
	if serveDone != nil {
		<-serveDone
	}
	s.wg.Wait()
	return nil
}

// shutdown marks the server closed, cancels the serve context and closes the
// listener and all connections. It does not wait.
func (s *Server) shutdown() {
	s.mu.Lock()
	s.closed = true
	ln, cancel := s.ln, s.cancel
	conns := make([]*serverConn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if ln != nil {
		_ = ln.Close()
	}
	for _, c := range conns {
		c.close()
	}
}

// heartbeatLoop sends agent.heartbeat to every open connection every
// HeartbeatInterval until ctx is done.
func (s *Server) heartbeatLoop(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-s.clock.After(HeartbeatInterval):
			s.broadcastHeartbeat(now)
		}
	}
}

func (s *Server) broadcastHeartbeat(now time.Time) {
	params, err := json.Marshal(HeartbeatParams{UptimeSeconds: uptimeSeconds(s.startedAt, now)})
	if err != nil {
		s.log.Error("ipc: encode heartbeat", "error", err.Error())
		return
	}
	msg := Request{JSONRPC: Version, Method: MethodHeartbeat, Params: params}

	s.mu.Lock()
	conns := make([]*serverConn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	for _, c := range conns {
		select {
		case c.out <- msg:
		default:
			s.log.Debug("ipc: heartbeat dropped, connection queue full", "remote", c.conn.RemoteAddr().String())
		}
	}
}

// readLoop reads and dispatches messages until the connection ends, then
// closes it and unregisters it.
func (s *Server) readLoop(ctx context.Context, c *serverConn) {
	defer s.wg.Done()
	defer func() {
		c.close()
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
	}()

	dec := NewDecoder(c.conn)
	for {
		msg, err := dec.Next()
		if err != nil {
			switch {
			case errors.Is(err, ErrMessageTooLarge):
				s.log.Warn("ipc: message too large, closing connection", "remote", c.conn.RemoteAddr().String(), "limit", MaxMessageSize)
			case errors.Is(err, io.EOF), errors.Is(err, net.ErrClosed):
				s.log.Debug("ipc: connection closed", "remote", c.conn.RemoteAddr().String())
			default:
				s.log.Debug("ipc: connection read failed", "remote", c.conn.RemoteAddr().String(), "error", err.Error())
			}
			return
		}
		resp, ok := Dispatch(ctx, s.handlers, msg)
		if !ok {
			continue
		}
		select {
		case c.out <- json.RawMessage(resp):
		case <-c.done:
			return
		}
	}
}

// writeLoop is the only writer of the connection.
func (s *Server) writeLoop(c *serverConn) {
	defer s.wg.Done()
	for {
		select {
		case <-c.done:
			return
		case msg := <-c.out:
			if err := c.enc.Encode(msg); err != nil {
				s.log.Debug("ipc: connection write failed", "remote", c.conn.RemoteAddr().String(), "error", err.Error())
				c.close()
				return
			}
		}
	}
}

// serverConn is one accepted connection. Its reader and writer goroutines end
// when done is closed (by close) or the connection fails.
type serverConn struct {
	conn      net.Conn
	enc       *Encoder
	out       chan any // messages for writeLoop; never closed
	done      chan struct{}
	closeOnce sync.Once
}

// close closes the connection and signals both goroutines. It is idempotent.
func (c *serverConn) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}
