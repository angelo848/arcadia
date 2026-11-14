package importers

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// guidName matches a LaunchBox file name with the game ID: Title.<GUID>-NN.ext [AJU-021].
var guidName = regexp.MustCompile(`\.[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}-\d+\.`)

// TestLaunchBoxImagesRealFolder checks the image lookup against a real LaunchBox installation. It
// runs only when ARCADIA_LAUNCHBOX_ROOT names the LaunchBox folder (the one with Data and Images).
// It logs the images found per folder (AJU-018), the folders not mapped per platform (AJU-022) and
// how many files carry a GUID in the name (AJU-021).
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
	perFolder := map[string]int{}
	platforms := map[string]bool{}
	withAny, missing, withGUID := 0, 0, 0
	for _, c := range res.Candidates {
		platforms[c.Platform] = true
		refs := LaunchBoxImages(root, c.Platform, c.Title)
		if len(refs) > 0 {
			withAny++
		} else if missing < 8 {
			missing++
			t.Logf("no image: %q (%s)", c.Title, c.Platform)
		}
		for _, r := range refs {
			perKind[r.Kind]++
			if rel, err := filepath.Rel(filepath.Join(root, "Images", c.Platform), r.Path); err == nil {
				perFolder[c.Platform+`\`+filepath.Dir(rel)]++
			}
			if guidName.MatchString(filepath.Base(r.Path)) {
				withGUID++
			}
		}
	}
	names := make([]string, 0, len(platforms))
	for p := range platforms {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		t.Logf("AJU-022 unmapped folders of %s: %v", p, UnmappedLaunchBoxFolders(root, p))
	}
	t.Logf("AJU-018 images per folder: %v", perFolder)
	t.Logf("AJU-021 files with GUID in the name: %d", withGUID)
	t.Logf("candidates %d, with images %d, images per kind %v", len(res.Candidates), withAny, perKind)
	if len(res.Candidates) == 0 {
		t.Fatal("no candidates read from the real XML")
	}
	if withAny == 0 {
		t.Error("no game of the real library matched an image: the naming rule does not hold")
	}
}
