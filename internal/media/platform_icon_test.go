package media_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/media"
)

func jpegIconBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for x := 0; x < 32; x++ {
		for y := 0; y < 32; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	return buf.Bytes()
}

func writeIconSrc(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

// TestPlatformIcon checks the copy and removal of platform icons (PLT-011..PLT-013).
func TestPlatformIcon(t *testing.T) {
	t.Parallel()

	t.Run("PLT-011/PNG copiado para platforms\\<sha256>.png e caminho relativo devolvido", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		body := pngBytes(t, 64, 64)
		src := writeIconSrc(t, "icone.png", body)
		rel, err := media.New(root, nil).ImportPlatformIcon(t.Context(), src)
		if err != nil {
			t.Fatalf("ImportPlatformIcon: %v", err)
		}
		sum := sha256.Sum256(body)
		want := filepath.Join("platforms", hex.EncodeToString(sum[:])+".png")
		if rel != want {
			t.Errorf("rel = %q, want %q", rel, want)
		}
		stored, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil || !bytes.Equal(stored, body) {
			t.Errorf("stored file differs from the source: %v", err)
		}
	})

	t.Run("PLT-011/JPEG vira .jpg", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		body := jpegIconBytes(t)
		src := writeIconSrc(t, "icone.jpeg", body)
		rel, err := media.New(root, nil).ImportPlatformIcon(t.Context(), src)
		if err != nil {
			t.Fatalf("ImportPlatformIcon: %v", err)
		}
		if filepath.Ext(rel) != ".jpg" || filepath.Dir(rel) != "platforms" {
			t.Errorf("rel = %q, want platforms\\<sha256>.jpg", rel)
		}
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("copy missing: %v", err)
		}
	})

	t.Run("PLT-011/apagar o original não afeta a cópia", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		body := pngBytes(t, 16, 16)
		src := writeIconSrc(t, "icone.png", body)
		rel, err := media.New(root, nil).ImportPlatformIcon(t.Context(), src)
		if err != nil {
			t.Fatalf("ImportPlatformIcon: %v", err)
		}
		if err := os.Remove(src); err != nil {
			t.Fatalf("remove source: %v", err)
		}
		stored, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil || !bytes.Equal(stored, body) {
			t.Errorf("copy lost after the source was deleted: %v", err)
		}
	})

	t.Run("PLT-012/acima de 2 MB é ErrTooLarge e nada é criado", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		// A valid PNG followed by padding, so only the size can reject it.
		body := append(pngBytes(t, 16, 16), make([]byte, media.MaxPlatformIconBytes)...)
		src := writeIconSrc(t, "grande.png", body)
		_, err := media.New(root, nil).ImportPlatformIcon(t.Context(), src)
		if !errors.Is(err, media.ErrTooLarge) {
			t.Errorf("err = %v, want ErrTooLarge", err)
		}
		if _, err := os.Stat(filepath.Join(root, "platforms")); !os.IsNotExist(err) {
			t.Errorf("platforms dir must not exist, stat err = %v", err)
		}
	})

	t.Run("PLT-012/texto com extensão .png é ErrUnsupported e nada é criado", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		src := writeIconSrc(t, "falso.png", []byte("texto puro"))
		_, err := media.New(root, nil).ImportPlatformIcon(t.Context(), src)
		if !errors.Is(err, media.ErrUnsupported) {
			t.Errorf("err = %v, want ErrUnsupported", err)
		}
		if _, err := os.Stat(filepath.Join(root, "platforms")); !os.IsNotExist(err) {
			t.Errorf("platforms dir must not exist, stat err = %v", err)
		}
	})

	t.Run("PLT-013/RemovePlatformIcon apaga dentro de platforms\\", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		st := media.New(root, nil)
		src := writeIconSrc(t, "icone.png", pngBytes(t, 16, 16))
		rel, err := st.ImportPlatformIcon(t.Context(), src)
		if err != nil {
			t.Fatalf("ImportPlatformIcon: %v", err)
		}
		if err := st.RemovePlatformIcon(rel); err != nil {
			t.Fatalf("RemovePlatformIcon: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Errorf("file still exists, stat err = %v", err)
		}
	})

	t.Run("PLT-013/RemovePlatformIcon recusa ..\\ e caminho absoluto fora de platforms\\", func(t *testing.T) {
		t.Parallel()
		base := t.TempDir()
		root := filepath.Join(base, "media")
		if err := os.MkdirAll(filepath.Join(root, "platforms"), 0o755); err != nil {
			t.Fatal(err)
		}
		victim := filepath.Join(base, "segredo.png")
		sibling := filepath.Join(root, "outra.png")
		for _, p := range []string{victim, sibling} {
			if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		st := media.New(root, nil)
		cases := map[string]string{
			"dotdot":            filepath.Join("platforms", "..", "..", "segredo.png"),
			"dotdot direto":     filepath.Join("..", "segredo.png"),
			"absoluto fora":     victim,
			"fora de platforms": "outra.png",
			"vazio":             "",
		}
		for name, rel := range cases {
			if err := st.RemovePlatformIcon(rel); err == nil {
				t.Errorf("%s: RemovePlatformIcon(%q) = nil, want error", name, rel)
			}
		}
		for _, p := range []string{victim, sibling} {
			if _, err := os.Stat(p); err != nil {
				t.Errorf("%s was removed: %v", strings.TrimPrefix(p, base), err)
			}
		}
	})
}
