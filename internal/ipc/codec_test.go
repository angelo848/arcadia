package ipc_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

func TestDecoder(t *testing.T) {
	t.Parallel()

	t.Run("IPC-012/exatamente 1.048.576 bytes aceito", func(t *testing.T) {
		t.Parallel()
		line := bytes.Repeat([]byte{'a'}, ipc.MaxMessageSize)
		line[0], line[len(line)-1] = '<', '>' // detect truncation at either end
		stream := append(append([]byte{}, line...), '\n')

		dec := ipc.NewDecoder(bytes.NewReader(stream))
		got, err := dec.Next()
		if err != nil {
			t.Fatalf("Next() error = %v, want nil for a %d-byte line", err, ipc.MaxMessageSize)
		}
		if len(got) != ipc.MaxMessageSize {
			t.Fatalf("len(Next()) = %d, want %d", len(got), ipc.MaxMessageSize)
		}
		if !bytes.Equal(got, line) {
			t.Fatal("Next() content differs from the line written")
		}
		if _, err := dec.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next() at end = %v, want io.EOF", err)
		}
	})

	t.Run("IPC-012/linha no limite seguida de outra mensagem", func(t *testing.T) {
		t.Parallel()
		big := bytes.Repeat([]byte{'b'}, ipc.MaxMessageSize)
		stream := append(append(append([]byte{}, big...), '\n'), []byte("{\"after\":1}\n")...)

		dec := ipc.NewDecoder(bytes.NewReader(stream))
		got, err := dec.Next()
		if err != nil || !bytes.Equal(got, big) {
			t.Fatalf("first Next() = (%d bytes, %v), want the %d-byte line", len(got), err, ipc.MaxMessageSize)
		}
		got, err = dec.Next()
		if err != nil || string(got) != `{"after":1}` {
			t.Fatalf("second Next() = (%q, %v), want {\"after\":1}", got, err)
		}
	})

	t.Run("IPC-003/1.048.577 bytes retorna ErrMessageTooLarge", func(t *testing.T) {
		t.Parallel()
		stream := append(bytes.Repeat([]byte{'c'}, ipc.MaxMessageSize+1), '\n')
		dec := ipc.NewDecoder(bytes.NewReader(stream))
		got, err := dec.Next()
		if !errors.Is(err, ipc.ErrMessageTooLarge) {
			t.Fatalf("Next() error = %v (len %d), want ErrMessageTooLarge", err, len(got))
		}
	})

	t.Run("IPC-003/muito acima do limite sem \\n retorna ErrMessageTooLarge", func(t *testing.T) {
		t.Parallel()
		stream := bytes.Repeat([]byte{'d'}, 3*ipc.MaxMessageSize)
		dec := ipc.NewDecoder(bytes.NewReader(stream))
		if _, err := dec.Next(); !errors.Is(err, ipc.ErrMessageTooLarge) {
			t.Fatalf("Next() error = %v, want ErrMessageTooLarge", err)
		}
	})

	t.Run("IPC-012/várias mensagens no stream", func(t *testing.T) {
		t.Parallel()
		msgs := []string{
			`{"jsonrpc":"2.0","id":1,"method":"agent.ping"}`,
			`[]`,
			`not json at all`,
			`{"jsonrpc":"2.0","id":"x","method":"agent.status","params":{}}`,
		}
		stream := strings.Join(msgs, "\n") + "\n"

		dec := ipc.NewDecoder(strings.NewReader(stream))
		for i, want := range msgs {
			got, err := dec.Next()
			if err != nil {
				t.Fatalf("Next() #%d error = %v", i, err)
			}
			if bytes.ContainsRune(got, '\n') {
				t.Errorf("Next() #%d = %q contains '\\n'", i, got)
			}
			if string(got) != want {
				t.Errorf("Next() #%d = %q, want %q", i, got, want)
			}
		}
		if _, err := dec.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next() after last message = %v, want io.EOF", err)
		}
	})

	t.Run("IPC-012/última linha sem \\n é devolvida e io.EOF vem na chamada seguinte", func(t *testing.T) {
		t.Parallel()
		dec := ipc.NewDecoder(strings.NewReader("{\"a\":1}\n{\"b\":2}"))
		got, err := dec.Next()
		if err != nil || string(got) != `{"a":1}` {
			t.Fatalf("first Next() = (%q, %v), want {\"a\":1}", got, err)
		}
		got, err = dec.Next()
		if err != nil {
			t.Fatalf("second Next() error = %v, want nil (last line without '\\n' must be returned)", err)
		}
		if string(got) != `{"b":2}` {
			t.Fatalf("second Next() = %q, want {\"b\":2}", got)
		}
		if _, err := dec.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("third Next() = %v, want io.EOF", err)
		}
	})

	t.Run("IPC-012/stream de uma linha só sem \\n", func(t *testing.T) {
		t.Parallel()
		dec := ipc.NewDecoder(strings.NewReader(`{"only":true}`))
		got, err := dec.Next()
		if err != nil || string(got) != `{"only":true}` {
			t.Fatalf("Next() = (%q, %v), want {\"only\":true}", got, err)
		}
		if _, err := dec.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next() at end = %v, want io.EOF", err)
		}
	})

	t.Run("IPC-012/última linha sem \\n de exatamente MaxMessageSize é aceita", func(t *testing.T) {
		t.Parallel()
		line := bytes.Repeat([]byte{'e'}, ipc.MaxMessageSize)
		dec := ipc.NewDecoder(bytes.NewReader(line))
		got, err := dec.Next()
		if err != nil || !bytes.Equal(got, line) {
			t.Fatalf("Next() = (%d bytes, %v), want the %d-byte line", len(got), err, ipc.MaxMessageSize)
		}
		if _, err := dec.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next() at end = %v, want io.EOF", err)
		}
	})

	t.Run("IPC-012/CRLF: linha sai sem \\r", func(t *testing.T) {
		t.Parallel()
		dec := ipc.NewDecoder(strings.NewReader("{\"a\":1}\r\n{\"b\":2}\r\n"))
		for i, want := range []string{`{"a":1}`, `{"b":2}`} {
			got, err := dec.Next()
			if err != nil {
				t.Fatalf("Next() #%d error = %v", i, err)
			}
			if string(got) != want {
				t.Errorf("Next() #%d = %q, want %q (no trailing '\\r')", i, got, want)
			}
		}
		if _, err := dec.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next() at end = %v, want io.EOF", err)
		}
	})

	t.Run("IPC-012/linhas vazias e só \\r são puladas", func(t *testing.T) {
		t.Parallel()
		stream := "\n\r\n{\"a\":1}\n\n\n\r\n{\"b\":2}\r\n\r\n\n"
		dec := ipc.NewDecoder(strings.NewReader(stream))
		for i, want := range []string{`{"a":1}`, `{"b":2}`} {
			got, err := dec.Next()
			if err != nil {
				t.Fatalf("Next() #%d error = %v", i, err)
			}
			if string(got) != want {
				t.Errorf("Next() #%d = %q, want %q (empty lines are skipped)", i, got, want)
			}
		}
		if got, err := dec.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next() at end = (%q, %v), want io.EOF (trailing empty lines skipped)", got, err)
		}
	})

	t.Run("IPC-012/stream só com linhas vazias retorna io.EOF", func(t *testing.T) {
		t.Parallel()
		dec := ipc.NewDecoder(strings.NewReader("\n\r\n\n\r"))
		if got, err := dec.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next() = (%q, %v), want io.EOF", got, err)
		}
	})

	t.Run("IPC-012/linha de MaxMessageSize bytes + \\r\\n é aceita", func(t *testing.T) {
		t.Parallel()
		line := bytes.Repeat([]byte{'f'}, ipc.MaxMessageSize)
		line[0], line[len(line)-1] = '<', '>'
		stream := append(append([]byte{}, line...), '\r', '\n')
		stream = append(stream, []byte("{\"after\":1}\r\n")...)

		dec := ipc.NewDecoder(bytes.NewReader(stream))
		got, err := dec.Next()
		if err != nil {
			t.Fatalf("Next() error = %v, want nil (limit applies to the line without \\r\\n)", err)
		}
		if !bytes.Equal(got, line) {
			t.Fatalf("Next() = %d bytes, want the %d-byte line without '\\r'", len(got), ipc.MaxMessageSize)
		}
		got, err = dec.Next()
		if err != nil || string(got) != `{"after":1}` {
			t.Fatalf("second Next() = (%q, %v), want {\"after\":1}", got, err)
		}
	})

	t.Run("IPC-003/MaxMessageSize+1 bytes + \\r\\n retorna ErrMessageTooLarge", func(t *testing.T) {
		t.Parallel()
		stream := append(bytes.Repeat([]byte{'g'}, ipc.MaxMessageSize+1), '\r', '\n')
		dec := ipc.NewDecoder(bytes.NewReader(stream))
		if _, err := dec.Next(); !errors.Is(err, ipc.ErrMessageTooLarge) {
			t.Fatalf("Next() error = %v, want ErrMessageTooLarge", err)
		}
	})

	t.Run("IPC-012/stream vazio retorna io.EOF", func(t *testing.T) {
		t.Parallel()
		dec := ipc.NewDecoder(strings.NewReader(""))
		if _, err := dec.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next() = %v, want io.EOF", err)
		}
	})
}

// errReaderRanAway is returned by infiniteReader once it has served far more than
// a bounded Decoder could need, so an unbounded implementation fails instead of hanging.
var errReaderRanAway = errors.New("test: decoder read more than 4 MiB without stopping")

// infiniteReader yields an endless line without '\n' and counts the bytes served.
type infiniteReader struct {
	n int64
}

const infiniteReaderCap = 4 << 20

func (r *infiniteReader) Read(p []byte) (int, error) {
	if r.n > infiniteReaderCap {
		return 0, errReaderRanAway
	}
	for i := range p {
		p[i] = 'x'
	}
	r.n += int64(len(p))
	return len(p), nil
}

func TestDecoderBounds(t *testing.T) {
	t.Parallel()

	t.Run("IPC-003/reader infinito sem \\n: ErrMessageTooLarge com leitura limitada", func(t *testing.T) {
		t.Parallel()
		r := &infiniteReader{}
		dec := ipc.NewDecoder(r)
		_, err := dec.Next()
		if errors.Is(err, errReaderRanAway) {
			t.Fatalf("Next() kept reading past %d bytes (read %d): no size limit enforced", infiniteReaderCap, r.n)
		}
		if !errors.Is(err, ipc.ErrMessageTooLarge) {
			t.Fatalf("Next() error = %v, want ErrMessageTooLarge", err)
		}
		const maxRead = ipc.MaxMessageSize + 1 + 64*1024 // limit + 1 + bufio buffer
		if r.n > maxRead {
			t.Errorf("bytes read = %d, want <= %d (memory must stay bounded)", r.n, maxRead)
		}
	})

	t.Run("IPC-003/erro preso: Next seguinte repete ErrMessageTooLarge mesmo com linha válida depois", func(t *testing.T) {
		t.Parallel()
		stream := append(bytes.Repeat([]byte{'h'}, ipc.MaxMessageSize+1), '\n')
		stream = append(stream, []byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"agent.ping\"}\n")...)

		dec := ipc.NewDecoder(bytes.NewReader(stream))
		if _, err := dec.Next(); !errors.Is(err, ipc.ErrMessageTooLarge) {
			t.Fatalf("first Next() error = %v, want ErrMessageTooLarge", err)
		}
		for i := 2; i <= 3; i++ {
			got, err := dec.Next()
			if !errors.Is(err, ipc.ErrMessageTooLarge) {
				t.Fatalf("Next() #%d = (%.40q, %v), want ErrMessageTooLarge again (sticky error)", i, got, err)
			}
			if got != nil {
				t.Errorf("Next() #%d returned %d bytes along with the error, want nil", i, len(got))
			}
		}
	})
}

// lockedBuffer serializes each Write call, but nothing more: interleaving across
// separate Write calls is still possible unless the Encoder holds its own lock.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte{}, b.buf.Bytes()...)
}

// splitWriter forwards each Write in small chunks, making a non-serialized Encoder
// very likely to interleave lines.
type splitWriter struct{ w io.Writer }

func (s splitWriter) Write(p []byte) (int, error) {
	n := 0
	for len(p) > 0 {
		k := min(7, len(p))
		m, err := s.w.Write(p[:k])
		n += m
		if err != nil {
			return n, err
		}
		p = p[k:]
	}
	return n, nil
}

func TestEncoder(t *testing.T) {
	t.Parallel()

	t.Run("IPC-012/Encode grava JSON seguido de \\n", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		enc := ipc.NewEncoder(&buf)
		if err := enc.Encode(ipc.PingResult{Pong: true}); err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if err := enc.Encode(map[string]int{"n": 2}); err != nil {
			t.Fatalf("Encode: %v", err)
		}
		want := "{\"pong\":true}\n{\"n\":2}\n"
		if got := buf.String(); got != want {
			t.Fatalf("encoded = %q, want %q", got, want)
		}
	})

	t.Run("IPC-012/Encode e Decoder fazem ida e volta", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		enc := ipc.NewEncoder(&buf)
		in := ipc.HeartbeatParams{UptimeSeconds: 42}
		if err := enc.Encode(in); err != nil {
			t.Fatalf("Encode: %v", err)
		}
		line, err := ipc.NewDecoder(&buf).Next()
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		var out ipc.HeartbeatParams
		if err := json.Unmarshal(line, &out); err != nil {
			t.Fatalf("Unmarshal %q: %v", line, err)
		}
		if out != in {
			t.Fatalf("round trip = %+v, want %+v", out, in)
		}
	})

	t.Run("IPC-012/Encode concorrente não intercala linhas", func(t *testing.T) {
		t.Parallel()
		const writers = 32
		const perWriter = 20
		type msg struct {
			Writer  int    `json:"writer"`
			Seq     int    `json:"seq"`
			Payload string `json:"payload"`
		}

		out := &lockedBuffer{}
		enc := ipc.NewEncoder(splitWriter{w: out})

		var wg sync.WaitGroup
		errs := make(chan error, writers*perWriter)
		for w := range writers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for s := range perWriter {
					payload := strings.Repeat(fmt.Sprintf("%02d", w), 200)
					if err := enc.Encode(msg{Writer: w, Seq: s, Payload: payload}); err != nil {
						errs <- err
						return
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("concurrent Encode: %v", err)
		}

		seen := make(map[[2]int]bool)
		dec := ipc.NewDecoder(bytes.NewReader(out.Bytes()))
		for {
			line, err := dec.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			var m msg
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatalf("interleaved or corrupt line %.80q: %v", line, err)
			}
			if want := strings.Repeat(fmt.Sprintf("%02d", m.Writer), 200); m.Payload != want {
				t.Fatalf("line from writer %d has a foreign payload", m.Writer)
			}
			key := [2]int{m.Writer, m.Seq}
			if seen[key] {
				t.Fatalf("duplicate line writer=%d seq=%d", m.Writer, m.Seq)
			}
			seen[key] = true
		}
		if len(seen) != writers*perWriter {
			t.Fatalf("decoded %d lines, want %d", len(seen), writers*perWriter)
		}
	})
}
