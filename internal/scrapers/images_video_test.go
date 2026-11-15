package scrapers_test

import (
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/scrapers"
)

func TestSelectImagesIgnoresVideo(t *testing.T) {
	t.Parallel()

	t.Run("VID-002/scrapers.SelectImages ignora vídeo (MET-025)", func(t *testing.T) {
		t.Parallel()
		offers := []scrapers.Offer{
			offer("igdb", "video", "v1", "v2"),
			offer("steamgriddb", "video", "v3"),
			offer("steamgriddb", scrapers.KindCover, "c1"),
		}
		for _, windows := range []bool{true, false} {
			got, _ := scrapers.SelectImages(offers, map[string]int{}, map[string]int{}, windows)
			for _, p := range got {
				if p.Kind == "video" {
					t.Errorf("windows=%v: SelectImages picked a video: %+v", windows, p)
				}
			}
		}
	})
}
