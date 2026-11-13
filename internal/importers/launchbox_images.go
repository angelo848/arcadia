package importers

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// maxImagesPerKind caps the images taken for one game and one kind (MID-010). The LaunchBox folders
// can hold dozens of screenshots for a game; five per kind keeps the first ones only.
const maxImagesPerKind = 5

// ImageRef is one image of a game in the LaunchBox folder, with the media kind it becomes (MID-010).
type ImageRef struct {
	Kind string
	Path string
}

// launchBoxKinds maps the image folders of LaunchBox to the media kinds of the library (MID-010), in
// the order they are read. The region subfolders of each folder are read as the same kind.
var launchBoxKinds = []struct {
	folder, kind string
}{
	{"Banner", "banner"},
	{"Fanart - Background", "background"},
	{"Screenshot - Game Title", "title"},
	{"Screenshot - Gameplay", "screenshot"},
	{"Screenshot - Game Select", "select"},
}

// imageNameMarks are the characters LaunchBox replaces with "_" in an image file name, because they
// are not allowed in a Windows file name: "Black Myth: Wukong" is "Black Myth_ Wukong-01.jpg" and
// "Assassin's Creed" is "Assassin_s Creed-01.jpg".
var imageNameMarks = regexp.MustCompile(`[\\/:*?"<>|']`)

// LaunchBoxImages lists the images of one game from the LaunchBox folder root: root\Images\<platform>\
// <type>\[<region>\]<title>-NN.<ext> (MID-010). platform is the platform name of the XML ("Windows" for
// PC). A missing folder or a name that cannot match gives no images, not an error.
func LaunchBoxImages(root, platform, title string) []ImageRef {
	if !plainName(platform) || !plainName(title) {
		return nil
	}
	name := regexp.QuoteMeta(imageNameMarks.ReplaceAllString(title, "_"))
	match := regexp.MustCompile(`(?i)^` + name + `-\d+\.(jpg|jpeg|png)$`)

	var out []ImageRef
	for _, k := range launchBoxKinds {
		dir := filepath.Join(root, "Images", platform, k.folder)
		var files []string
		files = append(files, matchingFiles(dir, match)...)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				files = append(files, matchingFiles(filepath.Join(dir, e.Name()), match)...)
			}
		}
		sort.Strings(files)
		if len(files) > maxImagesPerKind {
			files = files[:maxImagesPerKind]
		}
		for _, f := range files {
			out = append(out, ImageRef{Kind: k.kind, Path: f})
		}
	}
	return out
}

// matchingFiles returns the files directly in dir whose name matches; a missing dir gives none.
func matchingFiles(dir string, match *regexp.Regexp) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && match.MatchString(e.Name()) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

// plainName rejects a name that is empty or would leave its folder (a separator or "..").
func plainName(s string) bool {
	return strings.TrimSpace(s) != "" && !strings.ContainsAny(s, `\/`) && !strings.Contains(s, "..")
}
