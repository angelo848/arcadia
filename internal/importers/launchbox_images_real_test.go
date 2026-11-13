package importers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestLaunchBoxImagesRealFolder checks the image lookup against a real LaunchBox installation. It
// runs only when ARCADIA_LAUNCHBOX_ROOT names the LaunchBox folder (the one with Data and Images);
// it reads the Windows games of the library and logs how many images each kind found.
func TestLaunchBoxImagesRealFolder(t *testing.T) {
	root := os.Getenv("ARCADIA_LAUNCHBOX_ROOT")
	if root == "" {
		t.Skip("ARCADIA_LAUNCHBOX_ROOT not set")
	}
	res, err := LaunchBox().Scan(context.Background(), ScanInput{Path: filepath.Join(root, "Data", "Platforms")})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	perKind := map[string]int{}
	withAny, missing := 0, 0
	for _, c := range res.Candidates {
		refs := LaunchBoxImages(root, c.Platform, c.Title)
		if len(refs) > 0 {
			withAny++
		} else if missing < 8 {
			missing++
			t.Logf("no image: %q (%s)", c.Title, c.Platform)
		}
		for _, r := range refs {
			perKind[r.Kind]++
		}
	}
	t.Logf("candidates %d, with images %d, images per kind %v", len(res.Candidates), withAny, perKind)
	if len(res.Candidates) == 0 {
		t.Fatal("no candidates read from the real XML")
	}
	if withAny == 0 {
		t.Error("no game of the real library matched an image: the naming rule does not hold")
	}
}
