package ipc_test

import (
	"slices"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// TestMediaContract checks the JSON members of the phase 7 types against the schemas of the
// contract (MID-001..MID-006): a member the contract does not name, or one it names and Go
// does not send, fails here.
func TestMediaContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		schema string
		value  any
	}{
		{"ProviderKeyStatus", "ProviderKeyStatus", ipc.ProviderKeyStatus{Provider: "igdb"}},
		{"ProviderKeyStatusResult", "ProviderKeyStatusResult", ipc.ProviderKeyStatusResult{}},
		{"ScrapeCandidate", "ScrapeCandidate", ipc.ScrapeCandidate{}},
		{"ScrapeSearchResult", "ScrapeSearchResult", ipc.ScrapeSearchResult{}},
		{"ScrapeApplyResult", "ScrapeApplyResult", ipc.ScrapeApplyResult{}},
		{"MediaItem", "MediaItem", ipc.MediaItem{}},
		{"MediaListResult", "MediaListResult", ipc.MediaListResult{}},
		{"MediaQueueResult", "MediaQueueResult", ipc.MediaQueueResult{}},
		{"MediaCancelResult", "MediaCancelResult", ipc.MediaCancelResult{}},
	}
	for _, tc := range cases {
		t.Run("MID-001/"+tc.name+" tem as chaves do contrato", func(t *testing.T) {
			t.Parallel()
			want := schemaProps(t, tc.schema)
			got := jsonKeys(t, tc.value)
			if !slices.Equal(got, want) {
				t.Errorf("json keys = %v, contract = %v", got, want)
			}
		})
	}
}
