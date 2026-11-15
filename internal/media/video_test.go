package media_test

// Tests of Store.FetchVideo (phase 13, VID-013, VID-014, VID-025): streaming to a temporary file,
// the 20 MiB ceiling, MP4 only (ftyp at bytes 4..8), SHA-256 of the content as the file name.
// Assumed signature (tasks.md T19):
//
//	const MaxVideoBytes = 20 << 20
//	type PermanentError struct{ Msg string }
//	func (s *Store) FetchVideo(ctx, gameID int64, r io.Reader, size int64) (Stored, bool, error)

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/media"
)

const (
	msgTooBig      = "vídeo maior que 20 MB"
	msgUnsupported = "formato de vídeo não suportado"
)

// mp4Head is a minimal valid beginning of an MP4: box size, "ftyp", major brand.
var mp4Head = []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 2, 0, 'i', 's', 'o', 'm', 'm', 'p', '4', '1'}

// zeros reads an endless run of zero bytes and counts what was read.
type zeros struct{ n int64 }

func (z *zeros) Read(p []byte) (int, error) {
	clear(p)
	z.n += int64(len(p))
	return len(p), nil
}

// mp4Of returns the bytes of an MP4-looking payload of exactly n bytes.
func mp4Of(n int) []byte {
	b := make([]byte, n)
	copy(b, mp4Head)
	return b
}

// mp4Stream streams head followed by zeros, n bytes in all, without holding them in memory.
func mp4Stream(n int64) io.Reader {
	return io.LimitReader(io.MultiReader(bytes.NewReader(mp4Head), &zeros{}), n)
}

// touched fails the test when anything reads from it.
type touched struct{ t *testing.T }

func (r touched) Read([]byte) (int, error) {
	r.t.Helper()
	r.t.Error("the reader was touched, but the announced size is above the ceiling")
	return 0, io.EOF
}

// filesUnder lists every regular file below root (relative paths).
func filesUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func wantPermanent(t *testing.T, err error, msg string) {
	t.Helper()
	var pe *media.PermanentError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %v (%T), want *media.PermanentError", err, err)
	}
	if pe.Msg != msg || pe.Error() != msg {
		t.Errorf("PermanentError = {Msg: %q, Error(): %q}, want %q", pe.Msg, pe.Error(), msg)
	}
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestFetchVideo(t *testing.T) {
	t.Parallel()

	t.Run("VID-013/MaxVideoBytes é 20 MiB", func(t *testing.T) {
		t.Parallel()
		if media.MaxVideoBytes != 20971520 {
			t.Errorf("MaxVideoBytes = %d, want 20971520", media.MaxVideoBytes)
		}
	})

	t.Run("VID-013/tamanho informado maior que o teto: nada é lido (leitor que falha se tocado) e nenhuma linha", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		_, added, err := media.New(root, nil).FetchVideo(t.Context(), 5, touched{t}, media.MaxVideoBytes+1)
		wantPermanent(t, err, msgTooBig)
		if added {
			t.Error("added = true on a refused video")
		}
		if f := filesUnder(t, root); len(f) != 0 {
			t.Errorf("files left: %v", f)
		}
	})

	t.Run("VID-013/corpo passa do teto durante a leitura: o download para e o temporário é apagado (pasta sem resto)", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		z := &zeros{}
		body := io.MultiReader(bytes.NewReader(mp4Head), z) // endless
		_, added, err := media.New(root, nil).FetchVideo(t.Context(), 5, body, 0)
		wantPermanent(t, err, msgTooBig)
		if added {
			t.Error("added = true on a refused video")
		}
		if z.n > media.MaxVideoBytes+1 {
			t.Errorf("read %d bytes past the head, want the download to stop at MaxVideoBytes+1 = %d in all", z.n, media.MaxVideoBytes+1)
		}
		if f := filesUnder(t, root); len(f) != 0 {
			t.Errorf("files left: %v", f)
		}
	})

	t.Run("VID-013/exatamente 20 MiB é aceito", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		got, added, err := media.New(root, nil).FetchVideo(t.Context(), 5, mp4Stream(media.MaxVideoBytes), media.MaxVideoBytes)
		if err != nil {
			t.Fatalf("FetchVideo(20 MiB): %v", err)
		}
		if !added {
			t.Error("added = false, want true")
		}
		info, err := os.Stat(filepath.Join(root, got.Path))
		if err != nil {
			t.Fatalf("stat stored file: %v", err)
		}
		if info.Size() != media.MaxVideoBytes {
			t.Errorf("stored size = %d, want %d", info.Size(), media.MaxVideoBytes)
		}
	})

	t.Run("VID-013/os três casos são *PermanentError com a mensagem do contrato", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			r    io.Reader
			size int64
			msg  string
		}{
			{"tamanho informado", bytes.NewReader(nil), media.MaxVideoBytes + 1, msgTooBig},
			{"corpo acima do teto", io.MultiReader(bytes.NewReader(mp4Head), &zeros{}), 0, msgTooBig},
			{"não é MP4", bytes.NewReader([]byte("<html>not a video at all</html>")), 31, msgUnsupported},
		}
		for _, c := range cases {
			_, _, err := media.New(t.TempDir(), nil).FetchVideo(t.Context(), 1, c.r, c.size)
			t.Logf("%s: %v", c.name, err)
			wantPermanent(t, err, c.msg)
		}
	})

	t.Run("VID-014/aceita só MP4 (ftyp nos bytes 4..8); outro conteúdo é permanente \"formato de vídeo não suportado\" e não deixa temporário", func(t *testing.T) {
		t.Parallel()
		bad := map[string][]byte{
			"html":           []byte("<!DOCTYPE html><html></html>"),
			"png":            append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 64)...),
			"vazio":          nil,
			"curto":          []byte{0, 0, 0, 0x18, 'f', 't', 'y'},
			"ftyp no início": append([]byte("ftyp"), make([]byte, 32)...),
			"webm":           append([]byte{0x1a, 0x45, 0xdf, 0xa3}, make([]byte, 64)...),
		}
		for name, data := range bad {
			root := t.TempDir()
			_, added, err := media.New(root, nil).FetchVideo(t.Context(), 9, bytes.NewReader(data), int64(len(data)))
			if added {
				t.Errorf("%s: added = true", name)
			}
			var pe *media.PermanentError
			if !errors.As(err, &pe) || pe.Msg != msgUnsupported {
				t.Errorf("%s: error = %v, want PermanentError %q", name, err, msgUnsupported)
			}
			if f := filesUnder(t, root); len(f) != 0 {
				t.Errorf("%s: files left: %v", name, f)
			}
		}
		// A real MP4 is accepted.
		if _, _, err := media.New(t.TempDir(), nil).FetchVideo(t.Context(), 9, bytes.NewReader(mp4Of(1024)), 1024); err != nil {
			t.Errorf("valid MP4 refused: %v", err)
		}
	})

	t.Run("VID-014/arquivo gravado em <gameID>/<sha256>.mp4 com o hash calculado no caminho", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		data := mp4Of(4096)
		data[100], data[2000] = 7, 9
		got, added, err := media.New(root, nil).FetchVideo(t.Context(), 42, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("FetchVideo: %v", err)
		}
		if !added {
			t.Error("added = false, want true")
		}
		h := sum(data)
		if got.Hash != h {
			t.Errorf("Hash = %q, want %q", got.Hash, h)
		}
		wantPath := filepath.Join(strconv.Itoa(42), h+".mp4")
		if got.Path != wantPath {
			t.Errorf("Path = %q, want %q", got.Path, wantPath)
		}
		onDisk, err := os.ReadFile(filepath.Join(root, got.Path))
		if err != nil {
			t.Fatalf("read stored file: %v", err)
		}
		if !bytes.Equal(onDisk, data) {
			t.Error("stored content differs from the body")
		}
		if f := filesUnder(t, root); len(f) != 1 {
			t.Errorf("files = %v, want only the .mp4", f)
		}
	})

	t.Run("VID-014/conteúdo igual já gravado devolve added=false e não duplica arquivo nem deixa temporário", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		st := media.New(root, nil)
		data := mp4Of(2048)
		first, added, err := st.FetchVideo(t.Context(), 3, bytes.NewReader(data), int64(len(data)))
		if err != nil || !added {
			t.Fatalf("first FetchVideo = (added %v, err %v)", added, err)
		}
		second, added, err := st.FetchVideo(t.Context(), 3, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("second FetchVideo: %v", err)
		}
		if added {
			t.Error("added = true for content already stored")
		}
		if second.Hash != first.Hash || second.Path != first.Path {
			t.Errorf("second = %+v, want the same file as %+v", second, first)
		}
		if f := filesUnder(t, root); len(f) != 1 {
			t.Errorf("files = %v, want exactly one", f)
		}
	})

	t.Run("VID-014/thumb_path vazio", func(t *testing.T) {
		t.Parallel()
		got, _, err := media.New(t.TempDir(), nil).FetchVideo(t.Context(), 1, bytes.NewReader(mp4Of(512)), 512)
		if err != nil {
			t.Fatalf("FetchVideo: %v", err)
		}
		if got.ThumbPath != "" {
			t.Errorf("ThumbPath = %q, want empty", got.ThumbPath)
		}
	})

	t.Run("VID-014/contexto cancelado interrompe e apaga o temporário", func(t *testing.T) {
		t.Parallel()
		// Already canceled.
		root := t.TempDir()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, added, err := media.New(root, nil).FetchVideo(ctx, 1, mp4Stream(1<<20), 1<<20)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("canceled before: error = %v, want context.Canceled", err)
		}
		if added {
			t.Error("added = true after cancelation")
		}
		if f := filesUnder(t, root); len(f) != 0 {
			t.Errorf("files left after cancelation: %v", f)
		}

		// Canceled while the body is being read.
		root2 := t.TempDir()
		ctx2, cancel2 := context.WithCancel(t.Context())
		defer cancel2()
		reads := 0
		body := readerFunc(func(p []byte) (int, error) {
			reads++
			if reads == 3 {
				cancel2()
			}
			clear(p)
			if reads == 1 {
				copy(p, mp4Head)
			}
			return len(p), nil
		})
		_, _, err = media.New(root2, nil).FetchVideo(ctx2, 1, body, 0)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("canceled during: error = %v, want context.Canceled", err)
		}
		if f := filesUnder(t, root2); len(f) != 0 {
			t.Errorf("files left after cancelation during the read: %v", f)
		}
	})

	t.Run("VID-014/erro de leitura do corpo apaga o temporário", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		boom := errors.New("conexão caiu")
		body := io.MultiReader(bytes.NewReader(mp4Of(4096)), readerFunc(func([]byte) (int, error) { return 0, boom }))
		_, _, err := media.New(root, nil).FetchVideo(t.Context(), 1, body, 0)
		if !errors.Is(err, boom) {
			t.Errorf("error = %v, want the read error wrapped", err)
		}
		if f := filesUnder(t, root); len(f) != 0 {
			t.Errorf("files left: %v", f)
		}
	})

	t.Run("VID-025/RemoveGame apaga a pasta do jogo com o .mp4", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		st := media.New(root, nil)
		got, _, err := st.FetchVideo(t.Context(), 11, bytes.NewReader(mp4Of(1024)), 1024)
		if err != nil {
			t.Fatalf("FetchVideo: %v", err)
		}
		if !rmExists(filepath.Join(root, got.Path)) {
			t.Fatal("precondition: the .mp4 was not written")
		}
		if err := st.RemoveGame(11); err != nil {
			t.Fatalf("RemoveGame(11): %v", err)
		}
		if rmExists(filepath.Join(root, "11")) {
			t.Error("folder of game 11 still exists after RemoveGame")
		}
	})
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// TestFetchVideoStreamsFromHTTP is sequential on purpose: it measures the bytes allocated by the
// process, so no parallel test may run beside it.
func TestFetchVideoStreamsFromHTTP(t *testing.T) {
	t.Run("VID-014/leitura via httptest com corpo de 15 MiB usa memória limitada", func(t *testing.T) {
		const total = 15 << 20
		// The first chunk carries the MP4 head; the later ones are zeros.
		first := mp4Of(64 << 10)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(total))
			rest := make([]byte, 64<<10)
			for sent := 0; sent < total; sent += len(rest) {
				b := rest
				if sent == 0 {
					b = first
				}
				if _, err := w.Write(b); err != nil {
					return
				}
			}
		}))
		defer srv.Close()

		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()

		root := t.TempDir()
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		got, added, err := media.New(root, nil).FetchVideo(t.Context(), 8, resp.Body, resp.ContentLength)
		runtime.ReadMemStats(&after)
		if err != nil {
			t.Fatalf("FetchVideo over HTTP: %v", err)
		}
		if !added {
			t.Error("added = false, want true")
		}
		info, err := os.Stat(filepath.Join(root, got.Path))
		if err != nil || info.Size() != total {
			t.Fatalf("stored file = (%v, %v), want %d bytes", info, err, total)
		}
		// Reading the whole body into memory would allocate at least 15 MiB; streaming needs a few buffers.
		if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 8<<20 {
			t.Errorf("allocated %d bytes while fetching a %d-byte body; want a bounded amount (< 8 MiB)", alloc, total)
		}
	})
}
