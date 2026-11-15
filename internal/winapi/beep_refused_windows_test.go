//go:build windows

package winapi

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/feedback"
)

func TestBeeperRefused(t *testing.T) {
	t.Run("CON-015/CueRefused = 300 Hz por 100 ms e 200 Hz por 200 ms (função de tom injetada)", func(t *testing.T) {
		f := newFakeBeep(false)
		b := setupBeeper(t, f)
		b.Play(feedback.CueRefused)
		if got := f.next(t); got != (tone{300, 100}) {
			t.Fatalf("first tone = %+v, want {300 100}", got)
		}
		if got := f.next(t); got != (tone{200, 200}) {
			t.Fatalf("second tone = %+v, want {200 200}", got)
		}
		select {
		case tn := <-f.calls:
			t.Fatalf("extra tone: %+v", tn)
		case <-time.After(100 * time.Millisecond):
		}
	})

	t.Run("CON-015/Close entre os dois tons não toca o segundo", func(t *testing.T) {
		f := newFakeBeep(true)
		b := setupBeeper(t, f)
		b.Play(feedback.CueRefused)
		if got := f.next(t); got != (tone{300, 100}) { // blocked inside the first tone
			t.Fatalf("first tone = %+v", got)
		}
		closed := make(chan error, 1)
		go func() { closed <- b.Close() }()
		// Give the Close goroutine the chance to raise stop before the tone returns
		// (stop is not observable from outside; see report).
		for i := 0; i < 200; i++ {
			runtime.Gosched()
		}
		f.open()
		select {
		case err := <-closed:
			if err != nil {
				t.Fatalf("Close: %v", err)
			}
		case <-time.After(beepWait):
			t.Fatal("Close did not return")
		}
		if got := f.recorded(); len(got) != 1 {
			t.Fatalf("tones = %+v, want only the first one", got)
		}
	})

	t.Run("CON-015/x-console.refusedCue do contrato é igual aos dois tons", func(t *testing.T) {
		path := filepath.Join("..", "..", "docs", "specs", "phase-13-modo-console", "contracts", "ipc-v0.15.0-delta.md")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read contract delta: %v", err)
		}
		m := regexp.MustCompile(`"refusedCue":\s*\[\[(\d+),\s*(\d+)\],\s*\[(\d+),\s*(\d+)\]\]`).FindStringSubmatch(string(raw))
		if m == nil {
			t.Fatal("x-console.refusedCue not found in the contract delta")
		}
		var n [4]uint32
		for i := range n {
			v, _ := strconv.ParseUint(m[i+1], 10, 32)
			n[i] = uint32(v)
		}
		f := newFakeBeep(false)
		b := setupBeeper(t, f)
		b.Play(feedback.CueRefused)
		first, second := f.next(t), f.next(t)
		if first != (tone{n[0], n[1]}) || second != (tone{n[2], n[3]}) {
			t.Errorf("tones = %+v, %+v; contract = %v", first, second, n)
		}
	})

	t.Run("CON-015/os sinais de pausa e retomada não mudam (SND-001)", func(t *testing.T) {
		f := newFakeBeep(false)
		b := setupBeeper(t, f)
		b.Play(feedback.CuePaused)
		if a, c := f.next(t), f.next(t); a != (tone{660, 90}) || c != (tone{440, 120}) {
			t.Fatalf("paused tones = %+v, %+v", a, c)
		}
		b.Play(feedback.CueResumed)
		if a, c := f.next(t), f.next(t); a != (tone{440, 90}) || c != (tone{660, 120}) {
			t.Fatalf("resumed tones = %+v, %+v", a, c)
		}
	})
}
