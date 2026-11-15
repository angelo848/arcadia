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
	"strconv"
)

// MaxVideoBytes is the largest video accepted (VID-013): 20 MB.
const MaxVideoBytes = 20 << 20

const (
	msgVideoTooBig      = "vídeo maior que 20 MB"
	msgVideoUnsupported = "formato de vídeo não suportado"
)

// PermanentError is a refusal the download queue must not retry (VID-013, VID-014).
type PermanentError struct{ Msg string }

func (e *PermanentError) Error() string { return e.Msg }

// FetchVideo streams the MP4 read from r (size is the announced length, or <= 0 when unknown) into
// <root>/<gameID>/<sha256>.mp4. It returns the file and whether it was written now. On any error
// no file is left under the root.
func (s *Store) FetchVideo(ctx context.Context, gameID int64, r io.Reader, size int64) (Stored, bool, error) {
	if size > MaxVideoBytes {
		return Stored{}, false, &PermanentError{Msg: msgVideoTooBig}
	}
	if err := ctx.Err(); err != nil {
		return Stored{}, false, fmt.Errorf("media: vídeo: %w", err)
	}
	id := strconv.FormatInt(gameID, 10)
	dir := filepath.Join(s.root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Stored{}, false, fmt.Errorf("media: pasta do jogo: %w", err)
	}
	f, err := os.CreateTemp(dir, "video-*.tmp")
	if err != nil {
		return Stored{}, false, fmt.Errorf("media: temporário: %w", err)
	}
	tmp := f.Name()
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
		_ = os.Remove(tmp) // no-op after a successful rename
	}()

	lr := io.LimitReader(r, MaxVideoBytes+1)
	h := sha256.New()
	var total int64
	var head [8]byte
	n, err := io.ReadFull(lr, head[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return Stored{}, false, fmt.Errorf("media: ler vídeo: %w", err)
	}
	if n < len(head) || string(head[4:8]) != "ftyp" {
		return Stored{}, false, &PermanentError{Msg: msgVideoUnsupported}
	}
	w := io.MultiWriter(f, h)
	if _, err := w.Write(head[:]); err != nil {
		return Stored{}, false, fmt.Errorf("media: gravar: %w", err)
	}
	total = int64(n)
	buf := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return Stored{}, false, fmt.Errorf("media: vídeo: %w", err)
		}
		m, rerr := lr.Read(buf)
		if m > 0 {
			total += int64(m)
			if total > MaxVideoBytes {
				return Stored{}, false, &PermanentError{Msg: msgVideoTooBig}
			}
			if _, err := w.Write(buf[:m]); err != nil {
				return Stored{}, false, fmt.Errorf("media: gravar: %w", err)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return Stored{}, false, fmt.Errorf("media: ler vídeo: %w", rerr)
		}
	}
	closed = true
	if err := f.Close(); err != nil {
		return Stored{}, false, fmt.Errorf("media: gravar: %w", err)
	}

	hash := hex.EncodeToString(h.Sum(nil))
	out := Stored{Hash: hash, Path: filepath.Join(id, hash+".mp4")}
	final := filepath.Join(s.root, out.Path)
	if _, err := os.Stat(final); err == nil {
		return out, false, nil
	}
	if err := os.Rename(tmp, final); err != nil {
		return Stored{}, false, fmt.Errorf("media: gravar: %w", err)
	}
	return out, true, nil
}
