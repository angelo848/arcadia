package importers

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ImageRef is one image of a game in the LaunchBox folder, with the media kind it becomes (MID-010).
type ImageRef struct {
	Kind string
	Path string
}

// launchBoxFolders maps the image folders of LaunchBox to the media kinds of the library (AJU-018). The
// kinds are in the output order (AJU-019); inside a kind the folders are in priority order. The region
// subfolders of each folder are read as the same kind.
var launchBoxFolders = []struct {
	kind    string
	folders []string
}{
	{"cover", []string{"Box - Front", "Box - Front - Reconstructed", "Fanart - Box - Front", "Steam Poster", "Epic Games Poster", "GOG Poster", "Origin Poster", "Uplay Thumbnail", "Amazon Poster"}},
	{"screenshot", []string{"Screenshot - Gameplay", "Steam Screenshot", "Epic Games Screenshot", "GOG Screenshot", "Origin Screenshot", "Amazon Screenshot", "Screenshot - High Scores", "Screenshot - Game Over"}},
	{"title", []string{"Screenshot - Game Title"}},
	{"select", []string{"Screenshot - Game Select"}},
	{"background", []string{"Fanart - Background", "Epic Games Background", "Origin Background", "Uplay Background", "Amazon Background"}},
	{"banner", []string{"Banner", "Steam Banner"}},
}

// imageNameMarks are the characters LaunchBox replaces with "_" in an image file name, because they
// are not allowed in a Windows file name: "Black Myth: Wukong" is "Black Myth_ Wukong-01.jpg" and
// "Assassin's Creed" is "Assassin_s Creed-01.jpg".
var imageNameMarks = regexp.MustCompile(`[\\/:*?"<>|']`)

// LaunchBoxImages lists the images of one game from the LaunchBox folder root: root\Images\<platform>\
// <type>\[<region>\]<title>[.<GUID>]-NN.<ext> (AJU-018..AJU-021). platform is the platform name of the
// XML ("Windows" for PC). A missing folder or a name that cannot match gives no images, not an error.
// There is no limit here; the caller decides how many to keep.
func LaunchBoxImages(root, platform, title string) []ImageRef {
	if !plainName(platform) || !plainName(title) {
		return nil
	}
	name := regexp.QuoteMeta(imageNameMarks.ReplaceAllString(title, "_"))
	match := regexp.MustCompile(`(?i)^` + name + `(\.[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})?-\d+\.(jpg|jpeg|png)$`)

	platDir := filepath.Join(root, "Images", platform)
	entries, err := os.ReadDir(platDir)
	if err != nil {
		return nil
	}
	var out []ImageRef
	for _, k := range launchBoxFolders {
		for _, folder := range k.folders {
			for _, e := range entries {
				if !e.IsDir() || !strings.EqualFold(e.Name(), folder) {
					continue
				}
				dir := filepath.Join(platDir, e.Name())
				paths := matchingFiles(dir, match)
				subs, _ := os.ReadDir(dir)
				var regions []string
				for _, s := range subs {
					if s.IsDir() {
						regions = append(regions, s.Name())
					}
				}
				sortNames(regions)
				for _, r := range regions {
					paths = append(paths, matchingFiles(filepath.Join(dir, r), match)...)
				}
				for _, p := range paths {
					out = append(out, ImageRef{Kind: k.kind, Path: p})
				}
			}
		}
	}
	return out
}

// UnmappedLaunchBoxFolders lists the subfolders of Images\<platform> that are not in the table, sorted
// (AJU-022). A missing platform folder, or a platform with a separator or "..", gives nil.
func UnmappedLaunchBoxFolders(root, platform string) []string {
	if !plainName(platform) {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "Images", platform))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || mappedFolder(e.Name()) {
			continue
		}
		out = append(out, e.Name())
	}
	sortNames(out)
	return out
}

func mappedFolder(name string) bool {
	for _, k := range launchBoxFolders {
		for _, f := range k.folders {
			if strings.EqualFold(f, name) {
				return true
			}
		}
	}
	return false
}

// sortNames sorts case-insensitively, breaking ties by the exact name.
func sortNames(s []string) {
	sort.Slice(s, func(i, j int) bool {
		a, b := strings.ToLower(s[i]), strings.ToLower(s[j])
		if a != b {
			return a < b
		}
		return s[i] < s[j]
	})
}

// matchingFiles returns the files directly in dir whose name matches, ordered by name; a missing dir
// gives none.
func matchingFiles(dir string, match *regexp.Regexp) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && match.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sortNames(names)
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = filepath.Join(dir, n)
	}
	return out
}

// plainName rejects a name that is empty or would leave its folder (a separator or "..").
func plainName(s string) bool {
	return strings.TrimSpace(s) != "" && !strings.ContainsAny(s, `\/`) && !strings.Contains(s, "..")
}
