package ipc

import (
	"context"
	"encoding/json"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// AgentInfo is the static identity of the running agent reported by agent.status.
type AgentInfo struct {
	Version   string
	PID       int
	StartedAt time.Time
	DataDir   string
}

// AgentHandlers returns the handlers for agent.ping and agent.status. Uptime is
// computed from clock on every call relative to info.StartedAt.
func AgentHandlers(info AgentInfo, clock domain.Clock) map[string]Handler {
	return map[string]Handler{
		MethodPing: noParams(func(context.Context) (any, error) {
			return PingResult{Pong: true}, nil
		}),
		MethodStatus: noParams(func(context.Context) (any, error) {
			return AgentStatus{
				Version:       info.Version,
				PID:           info.PID,
				StartedAt:     info.StartedAt.UTC(),
				DataDir:       info.DataDir,
				UptimeSeconds: uptimeSeconds(info.StartedAt, clock.Now()),
			}, nil
		}),
	}
}

// noParams wraps a handler for a method without params: any non-empty params
// (the dispatcher already normalized empty ones to nil) yield -32602.
func noParams(fn func(ctx context.Context) (any, error)) Handler {
	return func(ctx context.Context, params json.RawMessage) (any, error) {
		if len(params) != 0 {
			return nil, NewError(CodeInvalidParams, msgInvalidParams)
		}
		return fn(ctx)
	}
}

// uptimeSeconds returns floor(now - startedAt) in whole seconds, never negative.
func uptimeSeconds(startedAt, now time.Time) int64 {
	d := now.Sub(startedAt)
	if d < 0 {
		return 0
	}
	return int64(d / time.Second)
}
