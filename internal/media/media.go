// Package media downloads the images of the library into data\media\<game id>\ (phase 7, MID-004).
// Files are named by the sha-256 of their content, so the same image is kept once; a thumbnail of
// at most 320 px wide is written next to each one.
package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // PNG is decoded for the thumbnail (registered with image.Decode)
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// MaxImageBytes is the largest image accepted (MID-004): 20 MB.
const MaxImageBytes = 20 << 20

// thumbWidth is the widest thumbnail (MID-004).
const thumbWidth = 320

// ErrUnsupported is returned for content that is not a JPEG or a PNG image. WebP is not accepted:
// the standard library cannot decode it, and the phase does not add a decoder (design §7).
var ErrUnsupported = errors.New("media: formato de imagem não suportado")

// Store writes the media files under root.
type Store struct {
	root   string
	client *http.Client
}

// New returns a Store that writes under root and downloads with client.
func New(root string, client *http.Client) *Store {
	if client == nil {
		client = http.DefaultClient
	}
	return &Store{root: root, client: client}
}

// Stored describes one image on disk. Path and ThumbPath are relative to the root.
type Stored struct {
	Hash      string
	Path      string
	ThumbPath string
}

// Fetch downloads the image at url for the game of gameID. It returns the file, and whether the
// file was written now (false when the same content was already stored for this game). A failed
// download leaves no file behind (MID-004).
func (s *Store) Fetch(ctx context.Context, gameID int64, url string) (Stored, bool, error) {
	data, err := s.download(ctx, url)
	if err != nil {
		return Stored{}, false, err
	}
	ext, err := extOf(data)
	if err != nil {
		return Stored{}, false, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Stored{}, false, fmt.Errorf("media: decodificar imagem: %w", err)
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	dir := filepath.Join(s.root, strconv.FormatInt(gameID, 10))
	out := Stored{
		Hash:      hash,
		Path:      filepath.Join(strconv.FormatInt(gameID, 10), hash+ext),
		ThumbPath: filepath.Join(strconv.FormatInt(gameID, 10), "thumb_"+hash+".jpg"),
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Stored{}, false, fmt.Errorf("media: pasta do jogo: %w", err)
	}
	created := false
	if _, err := os.Stat(filepath.Join(s.root, out.Path)); errors.Is(err, os.ErrNotExist) {
		if err := writeAtomic(filepath.Join(s.root, out.Path), data); err != nil {
			return Stored{}, false, err
		}
		created = true
	}
	if _, err := os.Stat(filepath.Join(s.root, out.ThumbPath)); errors.Is(err, os.ErrNotExist) {
		thumb, err := encodeThumb(img)
		if err != nil {
			return Stored{}, false, err
		}
		if err := writeAtomic(filepath.Join(s.root, out.ThumbPath), thumb); err != nil {
			return Stored{}, false, err
		}
	}
	return out, created, nil
}

// download reads the body of the URL, refusing answers that are not 2xx or larger than the limit.
func (s *Store) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("media: pedido: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("media: baixar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("media: baixar: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("media: ler corpo: %w", err)
	}
	if len(data) > MaxImageBytes {
		return nil, fmt.Errorf("media: imagem maior que 20 MB")
	}
	return data, nil
}

// extOf names the file by what the content is, not by what the server says.
func extOf(data []byte) (string, error) {
	switch http.DetectContentType(data) {
	case "image/png":
		return ".png", nil
	case "image/jpeg":
		return ".jpg", nil
	default:
		return "", ErrUnsupported
	}
}

// encodeThumb scales img down to thumbWidth (keeping the ratio) and encodes it as JPEG. A narrower
// image is encoded at its size.
func encodeThumb(img image.Image) ([]byte, error) {
	dst := downscale(img, thumbWidth)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("media: miniatura: %w", err)
	}
	return buf.Bytes(), nil
}

// downscale averages blocks of img into an image at most w pixels wide.
func downscale(img image.Image, w int) image.Image {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= w {
		return img
	}
	dw, dh := w, sh*w/sw
	if dh < 1 {
		dh = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		y0, y1 := b.Min.Y+y*sh/dh, b.Min.Y+(y+1)*sh/dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := range dw {
			x0, x1 := b.Min.X+x*sw/dw, b.Min.X+(x+1)*sw/dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint32
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := color.RGBA64Model.Convert(img.At(xx, yy)).(color.RGBA64)
					r += uint32(c.R)
					g += uint32(c.G)
					bl += uint32(c.B)
					a += uint32(c.A)
					n++
				}
			}
			out.SetRGBA(x, y, color.RGBA{
				R: uint8(r / n >> 8), G: uint8(g / n >> 8), B: uint8(bl / n >> 8), A: uint8(a / n >> 8),
			})
		}
	}
	return out
}

// writeAtomic writes data to a temporary file next to path and renames it, so a crash never
// leaves a half-written image under the final name.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("media: gravar: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("media: gravar: %w", err)
	}
	return nil
}
