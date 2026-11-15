package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestMediaChangeCoalescer(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	const window = 500 * time.Millisecond
	const g, h int64 = 7, 3

	t.Run("IMG-003/primeira gravação envia na hora e abre janela de 500 ms", func(t *testing.T) {
		t.Parallel()
		c := newMediaChangeCoalescer(window)
		if !c.changed(g, at(0)) {
			t.Fatal("first change must send now")
		}
		if _, ok := c.next(); ok {
			t.Fatal("no pending change expected after an immediate send")
		}
		if c.changed(g, at(100)) {
			t.Fatal("change inside the window must not send now")
		}
		if got, ok := c.next(); !ok || !got.Equal(at(500)) {
			t.Fatalf("next = %v, %v; want %v, true", got, ok, at(500))
		}
	})

	t.Run("IMG-003/gravações em 0, 100 e 200 ms: avisos em 0 e 500 ms, nada em 1000 ms", func(t *testing.T) {
		t.Parallel()
		c := newMediaChangeCoalescer(window)
		if !c.changed(g, at(0)) {
			t.Fatal("change at 0 must send now")
		}
		if c.changed(g, at(100)) || c.changed(g, at(200)) {
			t.Fatal("changes at 100 and 200 must not send now")
		}
		if got := c.due(at(499)); len(got) != 0 {
			t.Fatalf("due(499) = %v; want empty", got)
		}
		if got := c.due(at(500)); !reflect.DeepEqual(got, []int64{g}) {
			t.Fatalf("due(500) = %v; want [%d]", got, g)
		}
		if got := c.due(at(1000)); len(got) != 0 {
			t.Fatalf("due(1000) = %v; want empty", got)
		}
	})

	t.Run("IMG-003/gravação em 1200 ms depois da janela vazia envia na hora", func(t *testing.T) {
		t.Parallel()
		c := newMediaChangeCoalescer(window)
		c.changed(g, at(0))
		c.changed(g, at(100))
		c.due(at(500))
		c.due(at(1000))
		if !c.changed(g, at(1200)) {
			t.Fatal("change after an empty window must send now")
		}
	})

	t.Run("IMG-003/G e H em t = 0: um aviso imediato de cada", func(t *testing.T) {
		t.Parallel()
		c := newMediaChangeCoalescer(window)
		if !c.changed(g, at(0)) || !c.changed(h, at(0)) {
			t.Fatal("each game must send now at t=0")
		}
		if got := c.due(at(0)); len(got) != 0 {
			t.Fatalf("due(0) = %v; want empty", got)
		}
	})

	t.Run("IMG-003/janelas de G e H independentes", func(t *testing.T) {
		t.Parallel()
		c := newMediaChangeCoalescer(window)
		c.changed(g, at(0))
		c.changed(h, at(300))
		c.changed(g, at(350))
		c.changed(h, at(400))
		if got := c.due(at(500)); !reflect.DeepEqual(got, []int64{g}) {
			t.Fatalf("due(500) = %v; want [%d]", got, g)
		}
		if got := c.due(at(800)); !reflect.DeepEqual(got, []int64{h}) {
			t.Fatalf("due(800) = %v; want [%d]", got, h)
		}
	})

	t.Run("IMG-003/due antes do fim da janela não envia; due exatamente no fim envia", func(t *testing.T) {
		t.Parallel()
		c := newMediaChangeCoalescer(window)
		c.changed(g, at(0))
		c.changed(g, at(10))
		if got := c.due(at(499)); len(got) != 0 {
			t.Fatalf("due(499) = %v; want empty", got)
		}
		if got := c.due(at(500)); !reflect.DeepEqual(got, []int64{g}) {
			t.Fatalf("due(500) = %v; want [%d]", got, g)
		}
	})

	t.Run("IMG-003/due devolve ids ordenados", func(t *testing.T) {
		t.Parallel()
		c := newMediaChangeCoalescer(window)
		c.changed(g, at(0))
		c.changed(h, at(0))
		c.changed(g, at(10))
		c.changed(h, at(20))
		if got := c.due(at(500)); !reflect.DeepEqual(got, []int64{h, g}) {
			t.Fatalf("due(500) = %v; want [%d %d]", got, h, g)
		}
	})

	t.Run("IMG-003/aviso final abre nova janela: gravação 100 ms depois fica pendente", func(t *testing.T) {
		t.Parallel()
		c := newMediaChangeCoalescer(window)
		c.changed(g, at(0))
		c.changed(g, at(100))
		if got := c.due(at(500)); !reflect.DeepEqual(got, []int64{g}) {
			t.Fatalf("due(500) = %v; want [%d]", got, g)
		}
		if c.changed(g, at(600)) {
			t.Fatal("change 100 ms after the final notice must stay pending")
		}
		if got, ok := c.next(); !ok || !got.Equal(at(1000)) {
			t.Fatalf("next = %v, %v; want %v, true", got, ok, at(1000))
		}
	})

	t.Run("IMG-003/next é false sem pendência e é o menor fim entre os pendentes", func(t *testing.T) {
		t.Parallel()
		c := newMediaChangeCoalescer(window)
		if _, ok := c.next(); ok {
			t.Fatal("next must be false on an empty coalescer")
		}
		c.changed(g, at(0))
		c.changed(h, at(100))
		if _, ok := c.next(); ok {
			t.Fatal("next must be false without pending changes")
		}
		c.changed(h, at(150)) // ends at 600
		c.changed(g, at(200)) // ends at 500
		if got, ok := c.next(); !ok || !got.Equal(at(500)) {
			t.Fatalf("next = %v, %v; want %v, true", got, ok, at(500))
		}
	})

	t.Run("IMG-003/janelas vencidas sem pendência saem dos mapas", func(t *testing.T) {
		t.Parallel()
		c := newMediaChangeCoalescer(window)
		c.changed(g, at(0))
		c.changed(h, at(0))
		c.due(at(500))
		if len(c.until) != 0 || len(c.pending) != 0 {
			t.Fatalf("maps not empty: until=%v pending=%v", c.until, c.pending)
		}
	})

	t.Run("IMG-004/mediaChangedWindow é igual a x-media.mediaChangedWindowMs do contrato v0.12.0", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join("..", "..", "docs", "specs", "phase-10-2-imagens-ao-vivo", "contracts", "ipc.openrpc.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read contract: %v", err)
		}
		var doc struct {
			Info struct {
				XMedia struct {
					WindowMs *int64 `json:"mediaChangedWindowMs"`
				} `json:"x-media"`
			} `json:"info"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("parse contract: %v", err)
		}
		if doc.Info.XMedia.WindowMs == nil {
			t.Fatal("x-media.mediaChangedWindowMs missing from the contract")
		}
		want := time.Duration(*doc.Info.XMedia.WindowMs) * time.Millisecond
		if mediaChangedWindow != want {
			t.Fatalf("mediaChangedWindow = %v; contract says %v", mediaChangedWindow, want)
		}
	})
}
