package domain_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestParseGameStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inputs  []string
		want    []domain.GameStatus
		wantErr bool
	}{
		{
			name:   "DOM-012/not_played, playing, completed, abandoned e backlog são aceitos",
			inputs: []string{"not_played", "playing", "completed", "abandoned", "backlog"},
			want: []domain.GameStatus{
				domain.StatusNotPlayed, domain.StatusPlaying, domain.StatusCompleted,
				domain.StatusAbandoned, domain.StatusBacklog,
			},
		},
		{
			name:    "DOM-012/vazio, Playing e valor desconhecido são erro",
			inputs:  []string{"", "Playing", "unknown"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for i, in := range tc.inputs {
				got, err := domain.ParseGameStatus(in)
				if tc.wantErr {
					if err == nil {
						t.Errorf("ParseGameStatus(%q) = %q, want error", in, got)
					}
					continue
				}
				if err != nil {
					t.Fatalf("ParseGameStatus(%q) error: %v", in, err)
				}
				if got != tc.want[i] {
					t.Errorf("ParseGameStatus(%q) = %q, want %q", in, got, tc.want[i])
				}
			}
		})
	}
}

func TestDefaultNewGameStatus(t *testing.T) {
	t.Parallel()

	t.Run("DOM-012/padrão de jogo novo é not_played", func(t *testing.T) {
		t.Parallel()

		if domain.StatusNotPlayed != "not_played" {
			t.Errorf("StatusNotPlayed = %q, want %q", domain.StatusNotPlayed, "not_played")
		}
	})
}
