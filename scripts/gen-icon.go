//go:build ignore

// gen-icon turns the master picture of the application icon (a square PNG with
// transparency, rendered from scripts/icons/arcadia.svg) into the icon files of
// the executable and of the tray. No external tools, no downloads.
//
//	go run scripts/gen-icon.go <master.png> <out.ico|out.png>...
//
// A .ico output holds the sizes in icoSizes (32-bit BMP entries; the 256 one is
// PNG, as Windows expects). A .png output is the master itself, re-encoded.
// Usual outputs: internal/agent/assets/icon.ico, cmd/arcadia/build/windows/icon.ico
// and cmd/arcadia/build/appicon.png.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
)

var icoSizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}

func main() {
	if len(os.Args) < 3 {
		slog.Error("usage: go run scripts/gen-icon.go <master.png> <out.ico|out.png>...")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		slog.Error("generate icons", "error", err)
		os.Exit(1)
	}
}

func run(masterPath string, outs []string) error {
	master, err := readMaster(masterPath)
	if err != nil {
		return err
	}
	for _, out := range outs {
		var data []byte
		switch strings.ToLower(filepath.Ext(out)) {
		case ".ico":
			data, err = buildICO(master)
		case ".png":
			data, err = encodePNG(master)
		default:
			err = fmt.Errorf("unknown output type: %s", out)
		}
		if err != nil {
			return fmt.Errorf("build %s: %w", out, err)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("create dir of %s: %w", out, err)
		}
		if err := os.WriteFile(out, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
	}
	return nil
}

func readMaster(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open master: %w", err)
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode master: %w", err)
	}
	b := src.Bounds()
	if b.Dx() != b.Dy() || b.Dx() < 256 {
		return nil, fmt.Errorf("master must be square and at least 256 px, got %dx%d", b.Dx(), b.Dy())
	}
	img := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(img, img.Bounds(), src, b.Min, draw.Src)
	return img, nil
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// resize shrinks the square master to size x size by averaging the area each
// target pixel covers, with the colours weighted by their alpha.
func resize(src *image.NRGBA, size int) *image.NRGBA {
	n := src.Bounds().Dx()
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := float64(n) / float64(size)
	for y := 0; y < size; y++ {
		y0, y1 := float64(y)*scale, float64(y+1)*scale
		for x := 0; x < size; x++ {
			x0, x1 := float64(x)*scale, float64(x+1)*scale
			var r, g, b, a, area float64
			for sy := int(y0); sy < n && float64(sy) < y1; sy++ {
				wy := math.Min(y1, float64(sy+1)) - math.Max(y0, float64(sy))
				for sx := int(x0); sx < n && float64(sx) < x1; sx++ {
					w := wy * (math.Min(x1, float64(sx+1)) - math.Max(x0, float64(sx)))
					p := src.NRGBAAt(sx, sy)
					pa := float64(p.A) * w
					r += float64(p.R) * pa
					g += float64(p.G) * pa
					b += float64(p.B) * pa
					a += pa
					area += w
				}
			}
			if a == 0 {
				continue
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o] = uint8(math.Round(r / a))
			dst.Pix[o+1] = uint8(math.Round(g / a))
			dst.Pix[o+2] = uint8(math.Round(b / a))
			dst.Pix[o+3] = uint8(math.Round(a / area))
		}
	}
	return dst
}

func put(b *bytes.Buffer, v any) {
	// Writes to a bytes.Buffer cannot fail for fixed-size values.
	_ = binary.Write(b, binary.LittleEndian, v)
}

// bmpEntry encodes one icon image as a 32-bit DIB followed by its AND mask.
func bmpEntry(img *image.NRGBA) []byte {
	size := img.Bounds().Dx()
	maskRow := ((size + 31) / 32) * 4
	var b bytes.Buffer
	// BITMAPINFOHEADER; the height is doubled to cover the XOR and AND masks.
	put(&b, uint32(40))
	put(&b, int32(size))
	put(&b, int32(2*size))
	put(&b, uint16(1))  // planes
	put(&b, uint16(32)) // bits per pixel
	put(&b, uint32(0))  // BI_RGB
	put(&b, uint32(size*size*4+size*maskRow))
	put(&b, int32(0))
	put(&b, int32(0))
	put(&b, uint32(0))
	put(&b, uint32(0))
	// XOR bitmap: BGRA, bottom-up.
	for y := size - 1; y >= 0; y-- {
		for x := 0; x < size; x++ {
			p := img.NRGBAAt(x, y)
			b.Write([]byte{p.B, p.G, p.R, p.A})
		}
	}
	// AND mask: a set bit marks a fully transparent pixel.
	for y := size - 1; y >= 0; y-- {
		row := make([]byte, maskRow)
		for x := 0; x < size; x++ {
			if img.NRGBAAt(x, y).A == 0 {
				row[x/8] |= 0x80 >> (x % 8)
			}
		}
		b.Write(row)
	}
	return b.Bytes()
}

func buildICO(master *image.NRGBA) ([]byte, error) {
	images := make([][]byte, 0, len(icoSizes))
	for _, size := range icoSizes {
		img := resize(master, size)
		if size < 256 {
			images = append(images, bmpEntry(img))
			continue
		}
		data, err := encodePNG(img)
		if err != nil {
			return nil, err
		}
		images = append(images, data)
	}

	var out bytes.Buffer
	put(&out, uint16(0)) // reserved
	put(&out, uint16(1)) // type: icon
	put(&out, uint16(len(images)))
	offset := 6 + 16*len(images)
	for i, size := range icoSizes {
		out.Write([]byte{byte(size), byte(size), 0, 0}) // width, height (0 means 256), palette colors, reserved
		put(&out, uint16(1))                            // planes
		put(&out, uint16(32))                           // bits per pixel
		put(&out, uint32(len(images[i])))
		put(&out, uint32(offset))
		offset += len(images[i])
	}
	for _, data := range images {
		out.Write(data)
	}
	return out.Bytes(), nil
}
