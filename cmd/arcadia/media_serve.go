package main

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// mediaURLPrefix is the URL path under which the UI reads the images of the library (phase 7).
const mediaURLPrefix = "/media/"

// mediaRoot is the folder of the images: data\media. It is empty until startup resolved the data folder.
func (a *App) mediaRoot() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dataRoot == "" {
		return ""
	}
	return filepath.Join(a.dataRoot, "media")
}

// mediaURL returns the URL of an image path inside the media folder, or "" when it is not there.
func mediaURL(root, abs string) string {
	if root == "" || abs == "" {
		return ""
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ""
	}
	return mediaURLPrefix + filepath.ToSlash(rel)
}

// serveMedia serves the files of the media folder and nothing else: a path that leaves the folder,
// a folder, or a missing file is 404 (phase 7).
func (a *App) serveMedia(w http.ResponseWriter, r *http.Request) {
	root := a.mediaRoot()
	if root == "" || !strings.HasPrefix(r.URL.Path, mediaURLPrefix) {
		http.NotFound(w, r)
		return
	}
	rel := path.Clean("/" + strings.TrimPrefix(r.URL.Path, mediaURLPrefix))
	if rel == "/" || strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	if strings.EqualFold(filepath.Ext(full), ".mp4") {
		// Set explicitly: the system MIME table (registry) must not decide; ServeFile handles Range.
		w.Header().Set("Content-Type", "video/mp4")
	}
	http.ServeFile(w, r, full)
}
