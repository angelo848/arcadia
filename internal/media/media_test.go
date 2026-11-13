package media_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/media"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png: %v", err)
	}
	return buf.Bytes()
}

func serve(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetch(t *testing.T) {
	t.Parallel()

	t.Run("MID-004/grava em data/media/<id> com o hash no nome", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		body := pngBytes(t, 64, 64)
		srv := serve(t, body)
		st := media.New(root, srv.Client())

		got, created, err := st.Fetch(context.Background(), 7, srv.URL+"/cover.png")
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if !created {
			t.Error("created = false for a new file")
		}
		if !strings.HasPrefix(got.Path, "7"+string(filepath.Separator)) || !strings.HasSuffix(got.Path, ".png") {
			t.Errorf("path = %q, want 7/<hash>.png", got.Path)
		}
		if len(got.Hash) != 64 {
			t.Errorf("hash = %q, want a sha-256 hex", got.Hash)
		}
		stored, err := os.ReadFile(filepath.Join(root, got.Path))
		if err != nil || !bytes.Equal(stored, body) {
			t.Errorf("stored file differs from the download: %v", err)
		}
	})

	t.Run("MID-004/a mesma imagem não é gravada de novo", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		srv := serve(t, pngBytes(t, 32, 32))
		st := media.New(root, srv.Client())
		first, _, err := st.Fetch(context.Background(), 7, srv.URL+"/a.png")
		if err != nil {
			t.Fatalf("first: %v", err)
		}
		second, created, err := st.Fetch(context.Background(), 7, srv.URL+"/b.png")
		if err != nil {
			t.Fatalf("second: %v", err)
		}
		if created || second.Path != first.Path || second.Hash != first.Hash {
			t.Errorf("second = %+v created %v, want the first file again", second, created)
		}
	})

	t.Run("MID-004/imagem maior que 20 MB é recusada e nada fica gravado", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		big := make([]byte, media.MaxImageBytes+1)
		copy(big, pngBytes(t, 8, 8))
		srv := serve(t, big)
		st := media.New(root, srv.Client())
		if _, _, err := st.Fetch(context.Background(), 7, srv.URL+"/big.png"); err == nil {
			t.Fatal("Fetch err = nil for an image over the limit")
		}
		if entries, _ := os.ReadDir(filepath.Join(root, "7")); len(entries) != 0 {
			t.Errorf("files after refused download = %d, want 0", len(entries))
		}
	})

	t.Run("MID-004/falha de download devolve erro sem arquivo parcial", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)
		st := media.New(root, srv.Client())
		_, _, err := st.Fetch(context.Background(), 7, srv.URL+"/missing.png")
		if err == nil || errors.Is(err, media.ErrUnsupported) {
			t.Errorf("err = %v, want a download error", err)
		}
		if entries, _ := os.ReadDir(filepath.Join(root, "7")); len(entries) != 0 {
			t.Errorf("files after failed download = %d, want 0", len(entries))
		}
	})

	t.Run("MID-004/formato que não é imagem é recusado", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<html>não é imagem</html>"))
		}))
		t.Cleanup(srv.Close)
		st := media.New(t.TempDir(), srv.Client())
		if _, _, err := st.Fetch(context.Background(), 7, srv.URL+"/x"); !errors.Is(err, media.ErrUnsupported) {
			t.Errorf("err = %v, want ErrUnsupported", err)
		}
	})

	t.Run("MID-004/miniatura tem no máximo 320 px de largura e é JPEG", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		srv := serve(t, pngBytes(t, 1000, 500))
		st := media.New(root, srv.Client())
		got, _, err := st.Fetch(context.Background(), 7, srv.URL+"/wide.png")
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		f, err := os.Open(filepath.Join(root, got.ThumbPath))
		if err != nil {
			t.Fatalf("open thumb: %v", err)
		}
		defer f.Close()
		cfg, err := jpeg.DecodeConfig(f)
		if err != nil {
			t.Fatalf("thumb is not a JPEG: %v", err)
		}
		if cfg.Width != 320 || cfg.Height != 160 {
			t.Errorf("thumb size = %dx%d, want 320x160", cfg.Width, cfg.Height)
		}
	})
}
