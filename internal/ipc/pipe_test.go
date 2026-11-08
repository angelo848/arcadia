package ipc_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// TestPipeName covers the portable part of IPC-007: the pipe name format.
func TestPipeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sid  string
		want string
	}{
		{
			name: "IPC-007/nome segue arcadia-agent-<SID>",
			sid:  "S-1-5-21-1004336348-1177238915-682003330-1001",
			want: `\\.\pipe\arcadia-agent-S-1-5-21-1004336348-1177238915-682003330-1001`,
		},
		{
			name: "IPC-007/nome segue arcadia-agent-<SID> (SID curto)",
			sid:  "S-1-5-18",
			want: `\\.\pipe\arcadia-agent-S-1-5-18`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ipc.PipeName(tt.sid); got != tt.want {
				t.Fatalf("PipeName(%q) = %q, want %q", tt.sid, got, tt.want)
			}
		})
	}
}
