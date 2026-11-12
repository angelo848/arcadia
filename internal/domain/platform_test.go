package domain_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

func TestParsePlatformCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		inputs  []string
		want    []domain.PlatformCategory
		wantErr bool
	}{
		{
			name:   "DOM-015/pc, console, arcade e handheld são aceitos",
			inputs: []string{"pc", "console", "arcade", "handheld"},
			want: []domain.PlatformCategory{
				domain.CategoryPC, domain.CategoryConsole, domain.CategoryArcade, domain.CategoryHandheld,
			},
		},
		{
			name:    "DOM-015/outro valor é erro",
			inputs:  []string{"", "PC", "desktop", "mobile"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for i, in := range tc.inputs {
				got, err := domain.ParsePlatformCategory(in)
				if tc.wantErr {
					if err == nil {
						t.Errorf("ParsePlatformCategory(%q) = %q, want error", in, got)
					}
					continue
				}
				if err != nil {
					t.Fatalf("ParsePlatformCategory(%q) error: %v", in, err)
				}
				if got != tc.want[i] {
					t.Errorf("ParsePlatformCategory(%q) = %q, want %q", in, got, tc.want[i])
				}
			}
		})
	}
}
