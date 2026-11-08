//go:build ignore

// gen-icon writes the placeholder tray icon cmd/agent/assets/icon.ico: a
// minimal 16x16, 32-bit, BMP-based ICO (no external tools, no downloads).
//
//	go run scripts/gen-icon.go cmd/agent/assets/icon.ico
package main

import (
	"bytes"
	"encoding/binary"
	"log/slog"
	"os"
	"path/filepath"
)

const size = 16

func main() {
	if len(os.Args) != 2 {
		slog.Error("usage: go run scripts/gen-icon.go <out.ico>")
		os.Exit(2)
	}
	if err := os.MkdirAll(filepath.Dir(os.Args[1]), 0o755); err != nil {
		slog.Error("create icon dir", "error", err)
		os.Exit(1)
	}
	if err := os.WriteFile(os.Args[1], buildICO(), 0o644); err != nil {
		slog.Error("write icon", "error", err)
		os.Exit(1)
	}
}

func put(b *bytes.Buffer, v any) {
	// Writes to a bytes.Buffer cannot fail for fixed-size values.
	_ = binary.Write(b, binary.LittleEndian, v)
}

func buildICO() []byte {
	var img bytes.Buffer
	// BITMAPINFOHEADER; the height is doubled to cover the XOR and AND masks.
	put(&img, uint32(40))
	put(&img, int32(size))
	put(&img, int32(2*size))
	put(&img, uint16(1))  // planes
	put(&img, uint16(32)) // bits per pixel
	put(&img, uint32(0))  // BI_RGB
	put(&img, uint32(size*size*4+size*4))
	put(&img, int32(0))
	put(&img, int32(0))
	put(&img, uint32(0))
	put(&img, uint32(0))
	// XOR bitmap, BGRA, bottom-up: purple square with a light center block.
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px := []byte{0xC0, 0x30, 0x70, 0xFF}
			if x >= 5 && x <= 10 && y >= 4 && y <= 11 {
				px = []byte{0xFF, 0xE0, 0xF0, 0xFF}
			}
			img.Write(px)
		}
	}
	// AND mask: all opaque; each 16-bit row padded to 4 bytes.
	img.Write(make([]byte, size*4))

	var out bytes.Buffer
	put(&out, uint16(0))                // reserved
	put(&out, uint16(1))                // type: icon
	put(&out, uint16(1))                // image count
	out.Write([]byte{size, size, 0, 0}) // width, height, palette colors, reserved
	put(&out, uint16(1))                // planes
	put(&out, uint16(32))               // bits per pixel
	put(&out, uint32(img.Len()))        // image size
	put(&out, uint32(6+16))             // image offset
	out.Write(img.Bytes())
	return out.Bytes()
}
