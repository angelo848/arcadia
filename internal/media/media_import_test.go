package media_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/media"
)

// TestImport checks the copy of a local image into the media folder (MID-010).
func TestImport(t *testing.T) {
	t.Parallel()

	t.Run("MID-010/copia a imagem local para data/media com hash e miniatura", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		src := filepath.Join(t.TempDir(), "Starfield-01.png")
		body := pngBytes(t, 640, 360)
		if err := os.WriteFile(src, body, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		st := media.New(root, nil)
		got, created, err := st.Import(t.Context(), 7, src)
		if err != nil || !created {
			t.Fatalf("Import = %+v, %v, %v", got, created, err)
		}
		stored, err := os.ReadFile(filepath.Join(root, got.Path))
		if err != nil || !bytes.Equal(stored, body) {
			t.Errorf("stored file differs from the source: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, got.ThumbPath)); err != nil {
			t.Errorf("thumbnail missing: %v", err)
		}
	})

	t.Run("MID-010/arquivo que não é imagem é recusado", func(t *testing.T) {
		t.Parallel()
		src := filepath.Join(t.TempDir(), "notas.jpg")
		if err := os.WriteFile(src, []byte("texto puro"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, _, err := media.New(t.TempDir(), nil).Import(t.Context(), 7, src); !errors.Is(err, media.ErrUnsupported) {
			t.Errorf("err = %v, want ErrUnsupported", err)
		}
	})

	t.Run("MID-010/arquivo inexistente devolve erro", func(t *testing.T) {
		t.Parallel()
		if _, _, err := media.New(t.TempDir(), nil).Import(t.Context(), 7, filepath.Join(t.TempDir(), "x.png")); err == nil {
			t.Error("err = nil for a missing file")
		}
	})
}
