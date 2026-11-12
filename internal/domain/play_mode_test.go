package domain_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestParsePlayMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inputs  []string
		want    []domain.PlayMode
		wantErr bool
	}{
		{
			name:   "DOM-014/single, multi_local, multi_online, coop_local e coop_online são aceitos",
			inputs: []string{"single", "multi_local", "multi_online", "coop_local", "coop_online"},
			want: []domain.PlayMode{
				domain.PlayModeSingle, domain.PlayModeMultiLocal, domain.PlayModeMultiOnline,
				domain.PlayModeCoopLocal, domain.PlayModeCoopOnline,
			},
		},
		{
			name:    "DOM-014/nome fora da lista e maiúscula são erro",
			inputs:  []string{"Single", "SINGLE", "multi", "coop", "all", ""},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for i, in := range tc.inputs {
				got, err := domain.ParsePlayMode(in)
				if tc.wantErr {
					if err == nil {
						t.Errorf("ParsePlayMode(%q) = %q, want error", in, got)
					}
					continue
				}
				if err != nil {
					t.Fatalf("ParsePlayMode(%q) error: %v", in, err)
				}
				if got != tc.want[i] {
					t.Errorf("ParsePlayMode(%q) = %q, want %q", in, got, tc.want[i])
				}
			}
		})
	}
}

func TestParsePlayModeGroup(t *testing.T) {
	t.Parallel()

	t.Run("DOM-014/all, single, multi e coop são aceitos e vazio é erro", func(t *testing.T) {
		t.Parallel()

		for _, in := range []string{"all", "single", "multi", "coop"} {
			if _, err := domain.ParsePlayModeGroup(in); err != nil {
				t.Errorf("ParsePlayModeGroup(%q) error: %v", in, err)
			}
		}
		if _, err := domain.ParsePlayModeGroup(""); err == nil {
			t.Error(`ParsePlayModeGroup("") = nil error, want error`)
		}
	})

	t.Run("DOM-014/multi tem multi_local e multi_online; coop tem coop_local e coop_online; single só single", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			group string
			want  []domain.PlayMode
		}{
			{"multi", []domain.PlayMode{domain.PlayModeMultiLocal, domain.PlayModeMultiOnline}},
			{"coop", []domain.PlayMode{domain.PlayModeCoopLocal, domain.PlayModeCoopOnline}},
			{"single", []domain.PlayMode{domain.PlayModeSingle}},
		}
		for _, tc := range tests {
			group, err := domain.ParsePlayModeGroup(tc.group)
			if err != nil {
				t.Fatalf("ParsePlayModeGroup(%q) error: %v", tc.group, err)
			}
			if got := group.Modes(); !sameModes(got, tc.want) {
				t.Errorf("%s.Modes() = %v, want %v", tc.group, got, tc.want)
			}
		}
	})

	t.Run("DOM-014/all devolve nil (qualquer modo, inclusive jogo sem modo)", func(t *testing.T) {
		t.Parallel()

		group, err := domain.ParsePlayModeGroup("all")
		if err != nil {
			t.Fatalf("ParsePlayModeGroup(all) error: %v", err)
		}
		if got := group.Modes(); got != nil {
			t.Errorf("all.Modes() = %v, want nil", got)
		}
		if got := domain.PlayGroupAll.Modes(); got != nil {
			t.Errorf("PlayGroupAll.Modes() = %v, want nil", got)
		}
	})
}

// sameModes compara como conjuntos: a ordem dos modos dentro de um grupo não faz parte da regra.
func sameModes(got, want []domain.PlayMode) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[domain.PlayMode]bool, len(got))
	for _, m := range got {
		seen[m] = true
	}
	for _, m := range want {
		if !seen[m] {
			return false
		}
	}
	return true
}
