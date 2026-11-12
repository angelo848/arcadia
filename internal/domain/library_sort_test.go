package domain_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestParseSort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inputs  []string
		want    []domain.Sort
		wantErr bool
	}{
		{
			name:   "LIB-001/title, lastPlayed, totalPlayed e year são aceitos",
			inputs: []string{"title", "lastPlayed", "totalPlayed", "year"},
			want:   []domain.Sort{domain.SortTitle, domain.SortLastPlayed, domain.SortTotalPlayed, domain.SortYear},
		},
		{
			name:    "LIB-001/vazio, Title e idle são erro",
			inputs:  []string{"", "Title", "idle"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for i, in := range tc.inputs {
				got, err := domain.ParseSort(in)
				if tc.wantErr {
					if err == nil {
						t.Errorf("ParseSort(%q) = %q, want error", in, got)
					}
					continue
				}
				if err != nil {
					t.Fatalf("ParseSort(%q) error: %v", in, err)
				}
				if got != tc.want[i] {
					t.Errorf("ParseSort(%q) = %q, want %q", in, got, tc.want[i])
				}
			}
		})
	}
}

func TestDefaultSort(t *testing.T) {
	t.Parallel()

	t.Run("LIB-001/DefaultSort é title", func(t *testing.T) {
		t.Parallel()

		if domain.DefaultSort != domain.SortTitle {
			t.Errorf("DefaultSort = %q, want %q", domain.DefaultSort, domain.SortTitle)
		}
	})
}
