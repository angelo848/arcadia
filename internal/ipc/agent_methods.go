package ipc

import (
	"context"
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
		MethodPing: NoParams(func(context.Context) (any, error) {
			return PingResult{Pong: true}, nil
		}),
		MethodStatus: NoParams(func(context.Context) (any, error) {
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

// uptimeSeconds returns floor(now - startedAt) in whole seconds, never negative.
func uptimeSeconds(startedAt, now time.Time) int64 {
	d := now.Sub(startedAt)
	if d < 0 {
		return 0
	}
	return int64(d / time.Second)
}
