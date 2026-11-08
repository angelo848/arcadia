package ipc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrMessageTooLarge is returned by Decoder.Next when a line exceeds MaxMessageSize.
var ErrMessageTooLarge = errors.New("ipc: message exceeds 1 MiB")

// readerBufferSize is the bufio buffer size; lines longer than it are assembled
// incrementally, never holding more than MaxMessageSize+1 bytes of a line.
const readerBufferSize = 64 << 10

// Decoder reads NDJSON lines from a stream.
type Decoder struct {
	r   *bufio.Reader
	err error // sticky terminal error
}

// NewDecoder returns a Decoder reading from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReaderSize(r, readerBufferSize)}
}

// Next returns the next non-empty line without its trailing "\n" (and "\r", if
// any). The last line may lack a '\n'; io.EOF is returned on the following call.
// A line longer than MaxMessageSize yields ErrMessageTooLarge, after which the
// Decoder is unusable and the connection should be closed.
func (d *Decoder) Next() ([]byte, error) {
	for {
		if d.err != nil {
			return nil, d.err
		}
		line, err := d.readLine()
		if err != nil {
			d.err = err
			if errors.Is(err, io.EOF) && len(line) > 0 {
				return line, nil
			}
			return nil, err
		}
		if len(line) == 0 {
			continue // empty lines are ignored
		}
		return line, nil
	}
}

// readLine reads one line, stripping "\n" and a trailing "\r". On EOF it returns
// any pending data together with io.EOF.
func (d *Decoder) readLine() ([]byte, error) {
	var acc []byte
	for {
		chunk, err := d.r.ReadSlice('\n')
		content := chunk
		if err == nil {
			content = chunk[:len(chunk)-1] // drop '\n'
		}
		// Content may hold one byte beyond the limit: a '\r' that finish strips.
		if len(acc)+len(content) > MaxMessageSize+1 {
			return nil, ErrMessageTooLarge
		}
		acc = append(acc, content...)
		switch {
		case err == nil:
			return d.finish(acc)
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			line, ferr := d.finish(acc)
			if ferr != nil {
				return nil, ferr
			}
			return line, io.EOF
		default:
			return nil, fmt.Errorf("ipc: read message: %w", err)
		}
	}
}

func (d *Decoder) finish(line []byte) ([]byte, error) {
	line = bytes.TrimSuffix(line, []byte{'\r'})
	if len(line) > MaxMessageSize {
		return nil, ErrMessageTooLarge
	}
	return line, nil
}

// Encoder writes values as NDJSON lines. It is safe for concurrent use: each
// Encode issues a single Write under a mutex.
type Encoder struct {
	mu sync.Mutex
	w  io.Writer
}

// NewEncoder returns an Encoder writing to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes v as JSON followed by '\n'.
func (e *Encoder) Encode(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("ipc: encode message: %w", err)
	}
	data = append(data, '\n')
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.w.Write(data); err != nil {
		return fmt.Errorf("ipc: write message: %w", err)
	}
	return nil
}
