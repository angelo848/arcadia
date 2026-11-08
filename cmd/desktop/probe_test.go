package main

// Tests for the per-call pipe probe of T12 (design.md, "Ajustes da revisão do
// T12"): probePinger dials, sends one request, reads its response and closes.
// The fake server runs on the other end of a net.Pipe.

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// trackedConn records whether Close was called on the client side.
type trackedConn struct {
	net.Conn
	mu     sync.Mutex
	closed bool
}

func (c *trackedConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return c.Conn.Close()
}

func (c *trackedConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// pipeServer is the agent side of a net.Pipe. handle receives the decoded
// request and an encoder for replies; after handle returns the server drains
// the connection until the client closes it (EOF), so done means "client side
// closed".
type pipeServer struct {
	client *trackedConn
	gotReq chan ipc.Request // first request received
	done   chan struct{}    // server goroutine finished
	dials  int
	mu     sync.Mutex
}

func startPipeServer(t *testing.T, handle func(req ipc.Request, enc *ipc.Encoder)) *pipeServer {
	t.Helper()
	cli, srv := net.Pipe()
	s := &pipeServer{
		client: &trackedConn{Conn: cli},
		gotReq: make(chan ipc.Request, 1),
		done:   make(chan struct{}),
	}
	go func() {
		defer close(s.done)
		defer func() { _ = srv.Close() }()
		dec := ipc.NewDecoder(srv)
		enc := ipc.NewEncoder(srv)
		line, err := dec.Next()
		if err != nil {
			return
		}
		var req ipc.Request
		if err := json.Unmarshal(line, &req); err != nil {
			return
		}
		s.gotReq <- req
		handle(req, enc)
		// Drain until the client closes its end.
		for {
			if _, err := dec.Next(); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = s.client.Close()
		select {
		case <-s.done:
		case <-time.After(testTimeout):
			t.Errorf("fake pipe server did not finish within %v", testTimeout)
		}
	})
	return s
}

func (s *pipeServer) dial(ctx context.Context) (net.Conn, error) {
	s.mu.Lock()
	s.dials++
	s.mu.Unlock()
	return s.client, nil
}

// waitDone waits for the server goroutine, which ends only once the client
// side of the pipe was closed.
func (s *pipeServer) waitDone(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(testTimeout):
		t.Fatalf("client side of the pipe was not closed within %v", testTimeout)
	}
}

func respond(t *testing.T, enc *ipc.Encoder, id json.RawMessage, result any) {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Errorf("marshal result: %v", err)
		return
	}
	if err := enc.Encode(ipc.Response{JSONRPC: ipc.Version, ID: id, Result: raw}); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func heartbeat(t *testing.T, enc *ipc.Encoder, uptime int64) {
	t.Helper()
	raw, _ := json.Marshal(ipc.HeartbeatParams{UptimeSeconds: uptime})
	if err := enc.Encode(ipc.Request{JSONRPC: ipc.Version, Method: ipc.MethodHeartbeat, Params: raw}); err != nil {
		t.Errorf("encode heartbeat: %v", err)
	}
}

type callResult struct {
	st  ipc.AgentStatus
	err error
}

func runCall(ctx context.Context, p probePinger) <-chan callResult {
	ch := make(chan callResult, 1)
	go func() {
		var st ipc.AgentStatus
		err := p.Call(ctx, ipc.MethodStatus, nil, &st)
		ch <- callResult{st, err}
	}()
	return ch
}

func waitCall(t *testing.T, ch <-chan callResult) callResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(testTimeout):
		t.Fatalf("probe Call did not return within %v (real time)", testTimeout)
		return callResult{}
	}
}

func TestProbePinger(t *testing.T) {
	t.Parallel()

	t.Run("DSK-001/probe faz uma chamada e fecha a conexão", func(t *testing.T) {
		t.Parallel()
		want := sampleStatus()
		srv := startPipeServer(t, func(req ipc.Request, enc *ipc.Encoder) {
			respond(t, enc, req.ID, want)
		})

		r := waitCall(t, runCall(context.Background(), probePinger{dial: srv.dial}))

		if r.err != nil {
			t.Fatalf("Call err = %v, want nil", r.err)
		}
		assertStatus(t, r.st, want)
		if !srv.client.isClosed() {
			t.Errorf("client conn not closed when Call returned")
		}
		srv.waitDone(t)
		req := <-srv.gotReq
		if req.Method != ipc.MethodStatus {
			t.Errorf("request method = %q, want %q", req.Method, ipc.MethodStatus)
		}
		if req.JSONRPC != ipc.Version {
			t.Errorf("request jsonrpc = %q, want %q", req.JSONRPC, ipc.Version)
		}
		if len(req.ID) == 0 {
			t.Errorf("request has no id, want a call (not a notification)")
		}
		srv.mu.Lock()
		defer srv.mu.Unlock()
		if srv.dials != 1 {
			t.Errorf("dial called %d times, want 1", srv.dials)
		}
	})

	t.Run("DSK-001/probe ignora heartbeat antes da resposta", func(t *testing.T) {
		t.Parallel()
		want := sampleStatus()
		srv := startPipeServer(t, func(req ipc.Request, enc *ipc.Encoder) {
			heartbeat(t, enc, 1)
			heartbeat(t, enc, 2)
			respond(t, enc, req.ID, want)
		})

		r := waitCall(t, runCall(context.Background(), probePinger{dial: srv.dial}))

		if r.err != nil {
			t.Fatalf("Call err = %v, want nil", r.err)
		}
		assertStatus(t, r.st, want)
		if !srv.client.isClosed() {
			t.Errorf("client conn not closed when Call returned")
		}
		srv.waitDone(t)
	})

	t.Run("DSK-001/probe com dial falhando retorna erro na hora", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("pipe not found")
		dial := func(context.Context) (net.Conn, error) { return nil, boom }

		r := waitCall(t, runCall(context.Background(), probePinger{dial: dial}))

		if !errors.Is(r.err, boom) {
			t.Errorf("Call err = %v, want errors.Is(err, dial error)", r.err)
		}
	})

	t.Run("DSK-001/probe com ctx cancelado retorna erro de contexto e fecha a conexão", func(t *testing.T) {
		t.Parallel()
		srv := startPipeServer(t, func(ipc.Request, *ipc.Encoder) {
			// Never answers.
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := runCall(ctx, probePinger{dial: srv.dial})
		select {
		case <-srv.gotReq:
		case <-time.After(testTimeout):
			t.Fatalf("server did not receive the request within %v", testTimeout)
		}
		cancel()
		r := waitCall(t, ch)

		if !errors.Is(r.err, context.Canceled) {
			t.Errorf("Call err = %v, want errors.Is(err, context.Canceled)", r.err)
		}
		if !srv.client.isClosed() {
			t.Errorf("client conn not closed when Call returned")
		}
		srv.waitDone(t)
	})

	t.Run("DSK-001/probe repassa erro JSON-RPC", func(t *testing.T) {
		t.Parallel()
		srv := startPipeServer(t, func(req ipc.Request, enc *ipc.Encoder) {
			resp := ipc.Response{JSONRPC: ipc.Version, ID: req.ID,
				Error: &ipc.Error{Code: ipc.CodeInternalError, Message: "boom"}}
			if err := enc.Encode(resp); err != nil {
				t.Errorf("encode response: %v", err)
			}
		})

		r := waitCall(t, runCall(context.Background(), probePinger{dial: srv.dial}))

		var rpcErr *ipc.Error
		if !errors.As(r.err, &rpcErr) {
			t.Fatalf("Call err = %v, want an *ipc.Error", r.err)
		}
		if rpcErr.Code != ipc.CodeInternalError || rpcErr.Message != "boom" {
			t.Errorf("JSON-RPC error = %+v, want code %d message %q", rpcErr, ipc.CodeInternalError, "boom")
		}
		if !srv.client.isClosed() {
			t.Errorf("client conn not closed when Call returned")
		}
		srv.waitDone(t)
	})
}
