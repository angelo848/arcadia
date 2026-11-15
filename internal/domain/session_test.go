package domain_test

import (
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestParseSessionState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inputs  []string
		want    []domain.SessionState
		wantErr bool
	}{
		{
			name:   "DOM-010/running, paused, grace, ended e discarded são aceitos",
			inputs: []string{"running", "paused", "grace", "ended", "discarded"},
			want: []domain.SessionState{
				domain.SessionRunning, domain.SessionPaused, domain.SessionGrace, domain.SessionEnded, domain.SessionDiscarded,
			},
		},
		{
			name:    "DOM-010/vazio é erro",
			inputs:  []string{""},
			wantErr: true,
		},
		{
			name:    "DOM-010/Paused com maiúscula é erro",
			inputs:  []string{"Paused", "PAUSED"},
			wantErr: true,
		},
		{
			name:    "DOM-007/Running com maiúscula é erro",
			inputs:  []string{"Running", "GRACE", "Ended"},
			wantErr: true,
		},
		{
			name:    "DOM-007/valor desconhecido continua erro",
			inputs:  []string{" running", "running ", "paused ", "exited"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for i, in := range tc.inputs {
				got, err := domain.ParseSessionState(in)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("ParseSessionState(%q) = %q, nil; want error", in, got)
					}
					continue
				}
				if err != nil {
					t.Fatalf("ParseSessionState(%q) error = %v, want nil", in, err)
				}
				if got != tc.want[i] {
					t.Fatalf("ParseSessionState(%q) = %q, want %q", in, got, tc.want[i])
				}
			}
		})
	}
}

func TestSessionStateValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  domain.SessionState
		want string
	}{
		{name: "DOM-007/SessionRunning é running", got: domain.SessionRunning, want: "running"},
		{name: "DOM-007/SessionGrace é grace", got: domain.SessionGrace, want: "grace"},
		{name: "DOM-007/SessionEnded é ended", got: domain.SessionEnded, want: "ended"},
		{name: "DOM-007/SessionDiscarded é discarded", got: domain.SessionDiscarded, want: "discarded"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if string(tc.got) != tc.want {
				t.Fatalf("constant = %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestParseSessionEndReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inputs  []string
		want    []domain.SessionEndReason
		wantErr bool
	}{
		{
			name:   "DOM-007/exited, agent-shutdown e recovered são aceitos",
			inputs: []string{"exited", "agent-shutdown", "recovered"},
			want:   []domain.SessionEndReason{domain.EndExited, domain.EndAgentShutdown, domain.EndRecovered},
		},
		{
			// alterado em 2026-11-15 (fase 13)
			name:   "CON-016/ParseSessionEndReason aceita exited, agent-shutdown, recovered, client-no-launch e closed-by-hotkey",
			inputs: []string{"exited", "agent-shutdown", "recovered", "client-no-launch", "closed-by-hotkey"},
			want: []domain.SessionEndReason{domain.EndExited, domain.EndAgentShutdown, domain.EndRecovered,
				domain.EndClientNoLaunch, domain.EndClosedByHotkey},
		},
		{
			name:    "CON-016/rejeita vazio, closed_by_hotkey, Closed-By-Hotkey e outro valor",
			inputs:  []string{"", "closed_by_hotkey", "Closed-By-Hotkey", "closed-by-hotkey ", "closed"},
			wantErr: true,
		},
		{
			name:    "DOM-007/valor desconhecido é erro",
			inputs:  []string{"crashed", "Exited", "agent_shutdown", "ended", " recovered"},
			wantErr: true,
		},
		{
			name:    "DOM-007/vazio é erro",
			inputs:  []string{""},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for i, in := range tc.inputs {
				got, err := domain.ParseSessionEndReason(in)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("ParseSessionEndReason(%q) = %q, nil; want error", in, got)
					}
					continue
				}
				if err != nil {
					t.Fatalf("ParseSessionEndReason(%q) error = %v, want nil", in, err)
				}
				if got != tc.want[i] {
					t.Fatalf("ParseSessionEndReason(%q) = %q, want %q", in, got, tc.want[i])
				}
				if string(got) != in {
					t.Fatalf("string(ParseSessionEndReason(%q)) = %q, want round-trip", in, got)
				}
			}
		})
	}
}

func TestOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		active time.Duration
		want   domain.SessionState
	}{
		{name: "DOM-008/0 é discarded", active: 0, want: domain.SessionDiscarded},
		{name: "DOM-008/29,999 s é discarded", active: 29999 * time.Millisecond, want: domain.SessionDiscarded},
		{name: "DOM-008/30 s é ended", active: 30 * time.Second, want: domain.SessionEnded},
		{name: "DOM-008/2 h é ended", active: 2 * time.Hour, want: domain.SessionEnded},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.Outcome(tc.active); got != tc.want {
				t.Fatalf("Outcome(%v) = %q, want %q", tc.active, got, tc.want)
			}
		})
	}
}

func TestSessionConstants(t *testing.T) {
	t.Parallel()

	t.Run("DOM-008/MinCountedSession é 30 s, carência 15 s, heartbeat 30 s, verificação 5 s", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			got  time.Duration
			want time.Duration
		}{
			{name: "MinCountedSession", got: domain.MinCountedSession, want: 30 * time.Second},
			{name: "SessionGracePeriod", got: domain.SessionGracePeriod, want: 15 * time.Second},
			{name: "SessionHeartbeatInterval", got: domain.SessionHeartbeatInterval, want: 30 * time.Second},
			{name: "JobPollInterval", got: domain.JobPollInterval, want: 5 * time.Second},
		}
		for _, tc := range tests {
			if tc.got != tc.want {
				t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
			}
		}
	})
}
