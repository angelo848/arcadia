package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// probeRequestID is the JSON-RPC id of the single request a probe sends.
const probeRequestID = "1"

// dialFunc opens one connection to the agent.
type dialFunc func(ctx context.Context) (net.Conn, error)

// probePinger makes each Call on a fresh, short-lived connection: dial, send
// one request, read its response, close. A missing pipe fails at once, so the
// caller decides how to wait (DSK-001) instead of the ipc.Client backoff.
type probePinger struct {
	dial dialFunc
}

// Call implements pinger. The connection is always closed before returning;
// ctx being done closes it early and aborts any pending read or write.
func (p probePinger) Call(ctx context.Context, method string, params, result any) error {
	conn, err := p.dial(ctx)
	if err != nil {
		return fmt.Errorf("probe %s: %w", method, err)
	}
	if conn == nil {
		return fmt.Errorf("probe %s: dial returned no connection", method)
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if err := oneCall(conn, method, params, result); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("probe %s: %w", method, ctxErr)
		}
		return fmt.Errorf("probe %s: %w", method, err)
	}
	return nil
}

// oneCall writes one request on conn and waits for the matching response,
// skipping notifications (such as agent.heartbeat) received meanwhile.
func oneCall(conn net.Conn, method string, params, result any) error {
	req := ipc.Request{JSONRPC: ipc.Version, ID: json.RawMessage(probeRequestID), Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("encode params: %w", err)
		}
		req.Params = raw
	}
	if err := ipc.NewEncoder(conn).Encode(req); err != nil {
		return err
	}
	dec := ipc.NewDecoder(conn)
	for {
		line, err := dec.Next()
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}
		var resp ipc.Response
		if err := json.Unmarshal(line, &resp); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		if string(resp.ID) != probeRequestID {
			continue // a notification (no id) or an unrelated message
		}
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("decode result: %w", err)
			}
		}
		return nil
	}
}
