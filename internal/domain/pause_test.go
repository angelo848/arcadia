package domain_test

import (
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestCloseDebounce(t *testing.T) {
	t.Parallel()

	t.Run("CON-004/CloseDebounce é 300 ms e é independente de PauseToggleDebounce", func(t *testing.T) {
		t.Parallel()
		if domain.CloseDebounce != 300*time.Millisecond {
			t.Fatalf("CloseDebounce = %v, want 300ms", domain.CloseDebounce)
		}
		// Distinct named constants: changing one must not be an alias of the other.
		// Both are typed durations, so this only documents the independent values.
		if domain.PauseToggleDebounce != 300*time.Millisecond {
			t.Fatalf("PauseToggleDebounce = %v, want 300ms (unchanged)", domain.PauseToggleDebounce)
		}
	})
}

func TestParsePauseReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inputs  []string
		want    []domain.PauseReason
		wantErr bool
	}{
		{
			name:   "DOM-011/hotkey, gamepad, ui e sleep são aceitos",
			inputs: []string{"hotkey", "gamepad", "ui", "sleep"},
			want: []domain.PauseReason{
				domain.PauseHotkey, domain.PauseGamepad, domain.PauseUI, domain.PauseSleep,
			},
		},
		{
			name:    "DOM-011/vazio, Hotkey, idle e minimized são erro",
			inputs:  []string{"", "Hotkey", "idle", "minimized", "UI", " sleep", "gamepad "},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for i, in := range tc.inputs {
				got, err := domain.ParsePauseReason(in)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("ParsePauseReason(%q) = %q, nil; want error", in, got)
					}
					continue
				}
				if err != nil {
					t.Fatalf("ParsePauseReason(%q) error = %v, want nil", in, err)
				}
				if got != tc.want[i] {
					t.Fatalf("ParsePauseReason(%q) = %q, want %q", in, got, tc.want[i])
				}
				if string(got) != in {
					t.Fatalf("string(ParsePauseReason(%q)) = %q, want round-trip", in, got)
				}
			}
		})
	}
}

func TestPauseReasonUserInitiated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reason domain.PauseReason
		want   bool
	}{
		{name: "DOM-011/hotkey, gamepad e ui são do usuário e sleep não/hotkey", reason: domain.PauseHotkey, want: true},
		{name: "DOM-011/hotkey, gamepad e ui são do usuário e sleep não/gamepad", reason: domain.PauseGamepad, want: true},
		{name: "DOM-011/hotkey, gamepad e ui são do usuário e sleep não/ui", reason: domain.PauseUI, want: true},
		{name: "DOM-011/hotkey, gamepad e ui são do usuário e sleep não/sleep", reason: domain.PauseSleep, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.reason.UserInitiated(); got != tc.want {
				t.Fatalf("%q.UserInitiated() = %v, want %v", tc.reason, got, tc.want)
			}
		})
	}
}

func TestPauseConstants(t *testing.T) {
	t.Parallel()

	t.Run("DOM-010/SessionPaused é paused", func(t *testing.T) {
		t.Parallel()
		if string(domain.SessionPaused) != "paused" {
			t.Fatalf("SessionPaused = %q, want paused", domain.SessionPaused)
		}
	})

	t.Run("DOM-011/PauseToggleDebounce é 300 ms", func(t *testing.T) {
		t.Parallel()
		if domain.PauseToggleDebounce != 300*time.Millisecond {
			t.Fatalf("PauseToggleDebounce = %v, want 300ms", domain.PauseToggleDebounce)
		}
	})
}
