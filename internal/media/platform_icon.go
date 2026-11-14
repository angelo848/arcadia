package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// MaxPlatformIconBytes is the largest platform icon accepted (PLT-012): 2 MB.
const MaxPlatformIconBytes = 2 << 20

// ErrTooLarge is returned for an image larger than the limit (PLT-012).
var ErrTooLarge = errors.New("media: imagem grande demais")

// platformsDirName is the folder of the platform icons under the media root.
const platformsDirName = "platforms"

// ImportPlatformIcon copies the image file src into <root>\platforms\<sha256>.<png|jpg> and returns
// the path relative to the root (PLT-011). The type is detected by the content, not by the name
// (ErrUnsupported), and the file may not exceed MaxPlatformIconBytes (ErrTooLarge). The platforms
// folder is only created once the content has been accepted. No thumbnail is written.
func (s *Store) ImportPlatformIcon(ctx context.Context, src string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("media: ícone da plataforma: %w", err)
	}
	f, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("media: arquivo de origem: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxPlatformIconBytes+1))
	if err != nil {
		return "", fmt.Errorf("media: ler arquivo: %w", err)
	}
	if len(data) > MaxPlatformIconBytes {
		return "", ErrTooLarge
	}
	ext, err := extOf(data)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	rel := filepath.Join(platformsDirName, hex.EncodeToString(sum[:])+ext)
	if err := os.MkdirAll(filepath.Join(s.root, platformsDirName), 0o755); err != nil {
		return "", fmt.Errorf("media: pasta das plataformas: %w", err)
	}
	abs := filepath.Join(s.root, rel)
	if _, err := os.Stat(abs); err == nil {
		// already imported (same content)
	} else if errors.Is(err, os.ErrNotExist) {
		if err := writeAtomic(abs, data); err != nil {
			return "", err
		}
	} else {
		return "", fmt.Errorf("media: verificar ícone existente: %w", err)
	}
	return rel, nil
}

// RemovePlatformIcon deletes the icon file rel (relative to the media root). It refuses, with an
// error, anything that is not a file directly inside <root>\platforms\ (PLT-013): an empty string,
// "..", an absolute path outside that folder, a file at the root. A file already gone is not an
// error.
func (s *Store) RemovePlatformIcon(rel string) error {
	if rel == "" {
		return errors.New("media: caminho do ícone vazio")
	}
	dir := filepath.Join(s.root, platformsDirName)
	target := rel
	if !filepath.IsAbs(target) {
		target = filepath.Join(s.root, target)
	}
	target = filepath.Clean(target)
	if filepath.Dir(target) != dir {
		return fmt.Errorf("media: %q fora da pasta das plataformas", rel)
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("media: apagar ícone: %w", err)
	}
	return nil
}
