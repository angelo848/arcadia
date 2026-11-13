package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestServeMedia checks what the UI can read from the media folder (MID-004).
func TestServeMedia(t *testing.T) {
	t.Parallel()

	data := t.TempDir()
	media := filepath.Join(data, "media")
	if err := os.MkdirAll(filepath.Join(media, "7"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(media, "7", "h.png"), []byte("png"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(data, "library.db"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}
	a := &App{dataRoot: data}

	cases := []struct {
		name string
		url  string
		want int
	}{
		{"MID-004/imagem da pasta de mídia é servida", "/media/7/h.png", http.StatusOK},
		{"MID-004/caminho que sai da pasta de mídia é recusado", "/media/../library.db", http.StatusNotFound},
		{"MID-004/caminho codificado com .. é recusado", "/media/7/..%2F..%2Flibrary.db", http.StatusNotFound},
		{"MID-004/pasta não é servida", "/media/7", http.StatusNotFound},
		{"MID-004/arquivo que não existe é 404", "/media/7/nao.png", http.StatusNotFound},
		{"MID-004/outro caminho não é da mídia", "/library.db", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			a.serveMedia(rec, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if rec.Code != tc.want {
				t.Errorf("%s: status %d, want %d", tc.url, rec.Code, tc.want)
			}
		})
	}
}

// TestMediaURL checks the URL the UI gets for an image path (MID-006).
func TestMediaURL(t *testing.T) {
	t.Parallel()
	root := filepath.Join("C:", "Arcadia", "data", "media")
	if got := mediaURL(root, filepath.Join(root, "7", "h.png")); got != "/media/7/h.png" {
		t.Errorf("mediaURL = %q, want /media/7/h.png", got)
	}
	if got := mediaURL(root, filepath.Join(root, "..", "library.db")); got != "" {
		t.Errorf("mediaURL outside the folder = %q, want empty", got)
	}
	if got := mediaURL("", filepath.Join(root, "7", "h.png")); got != "" {
		t.Errorf("mediaURL without a root = %q, want empty", got)
	}
}
