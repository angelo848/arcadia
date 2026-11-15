package agent

// Tests for T23 of docs/specs/phase-12-metadados-unificados/tasks.md: the batch notifier
// (metadata.batchChanged, MET-054, IPC-017) and the progress coalescer (batch-state-machine.md §3).
// The coalescer takes its times as arguments; the test clock is a domain.FakeClock, never time.Sleep.

import (
	"context"
	"encoding/json"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// nextBatchChangedRaw waits up to d for the next metadata.batchChanged on c (other notifications
// are skipped) and returns its raw params.
func nextBatchChangedRaw(t *testing.T, c *ipc.Client, d time.Duration) (json.RawMessage, bool) {
	t.Helper()
	timeout := time.After(d)
	for {
		select {
		case n, ok := <-c.Notifications():
			if !ok {
				return nil, false
			}
			if n.Method != ipc.MethodMetadataBatchChanged {
				continue
			}
			return n.Params, true
		case <-timeout:
			return nil, false
		}
	}
}

// nextBatchChanged is nextBatchChangedRaw decoded into the typed params.
func nextBatchChanged(t *testing.T, c *ipc.Client, d time.Duration) (ipc.MetadataBatchChangedParams, bool) {
	t.Helper()
	raw, ok := nextBatchChangedRaw(t, c, d)
	if !ok {
		return ipc.MetadataBatchChangedParams{}, false
	}
	var p ipc.MetadataBatchChangedParams
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("metadata.batchChanged params %s: %v", raw, err)
	}
	return p, true
}

func TestBatchNotifier(t *testing.T) {
	t.Run("MET-054/batchNotifier sem servidor e depois de set(nil) não faz nada nem bloqueia", func(t *testing.T) {
		returns := func(fn func()) bool {
			done := make(chan struct{})
			go func() { defer close(done); fn() }()
			select {
			case <-done:
				return true
			case <-time.After(testTimeout):
				return false
			}
		}
		p := ipc.MetadataBatchChangedParams{BatchID: 1, State: ipc.BatchStateSearching, Done: 1, Total: 2}

		var n batchNotifier
		if !returns(func() { n.BatchChanged(p) }) {
			t.Fatal("BatchChanged before set blocked")
		}

		srv := ipc.NewServer(testClock(), map[string]ipc.Handler{}, quietLog())
		ctx, cancel := context.WithCancel(context.Background())
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); _ = srv.Serve(ctx, ln) }()
		t.Cleanup(func() {
			cancel()
			_ = srv.Close()
			wg.Wait()
		})
		c := newTCPClient(ln.Addr().String())
		t.Cleanup(func() { _ = c.Close() })
		cctx, ccancel := context.WithTimeout(context.Background(), testTimeout)
		defer ccancel()
		_ = c.Call(cctx, ipc.MethodMetadataBatchStatus, nil, nil) // establishes the connection

		n.set(srv)
		if !returns(func() { n.BatchChanged(p) }) {
			t.Fatal("BatchChanged with server blocked")
		}
		if got, ok := nextBatchChanged(t, c, testTimeout); !ok || got != p {
			t.Fatalf("after set(srv): got (%+v, %v), want %+v", got, ok, p)
		}

		n.set(nil)
		if !returns(func() { n.BatchChanged(p) }) {
			t.Fatal("BatchChanged after set(nil) blocked")
		}
		if got, ok := nextBatchChanged(t, c, 200*time.Millisecond); ok {
			t.Fatalf("after set(nil): got metadata.batchChanged %+v, want nothing", got)
		}
	})

	t.Run("MET-054/dois clientes IPC reais recebem a notificação com batchId, state, done, total, failed", func(t *testing.T) {
		srv := ipc.NewServer(testClock(), map[string]ipc.Handler{}, quietLog())
		ctx, cancel := context.WithCancel(context.Background())
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); _ = srv.Serve(ctx, ln) }()
		t.Cleanup(func() {
			cancel()
			_ = srv.Close()
			wg.Wait()
		})
		c1, c2 := newTCPClient(ln.Addr().String()), newTCPClient(ln.Addr().String())
		t.Cleanup(func() { _ = c1.Close(); _ = c2.Close() })
		for _, c := range []*ipc.Client{c1, c2} {
			cctx, ccancel := context.WithTimeout(context.Background(), testTimeout)
			_ = c.Call(cctx, ipc.MethodMetadataBatchStatus, nil, nil) // establishes the connection
			ccancel()
		}

		var n batchNotifier
		n.set(srv)
		want := ipc.MetadataBatchChangedParams{BatchID: 1763217600000, State: ipc.BatchStateSearching, Done: 12, Total: 80, Failed: 1}
		n.BatchChanged(want)

		for i, c := range []*ipc.Client{c1, c2} {
			raw, ok := nextBatchChangedRaw(t, c, testTimeout)
			if !ok {
				t.Fatalf("client %d: no metadata.batchChanged within %v", i+1, testTimeout)
			}
			var got ipc.MetadataBatchChangedParams
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("client %d: unmarshal %s: %v", i+1, raw, err)
			}
			if got != want {
				t.Errorf("client %d: got %+v, want %+v", i+1, got, want)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatalf("client %d: fields: %v", i+1, err)
			}
			if len(fields) != 5 {
				t.Errorf("client %d: params %s have %d keys, want exactly batchId, state, done, total, failed", i+1, raw, len(fields))
			}
			for _, k := range []string{"batchId", "state", "done", "total", "failed"} {
				if _, ok := fields[k]; !ok {
					t.Errorf("client %d: params %s lack %q", i+1, raw, k)
				}
			}
		}
	})

	t.Run("MET-054/fila cheia (16) descarta sem bloquear (IPC-017)", func(t *testing.T) {
		srv := ipc.NewServer(testClock(), map[string]ipc.Handler{}, quietLog())
		ctx, cancel := context.WithCancel(context.Background())
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); _ = srv.Serve(ctx, ln) }()
		t.Cleanup(func() {
			cancel()
			_ = srv.Close()
			wg.Wait()
		})
		c := newTCPClient(ln.Addr().String()) // connected, never reads its notifications
		t.Cleanup(func() { _ = c.Close() })
		cctx, ccancel := context.WithTimeout(context.Background(), testTimeout)
		_ = c.Call(cctx, ipc.MethodMetadataBatchStatus, nil, nil)
		ccancel()

		var n batchNotifier
		n.set(srv)
		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := 0; i < 100; i++ {
				n.BatchChanged(ipc.MetadataBatchChangedParams{BatchID: 1, State: ipc.BatchStateSearching, Done: i, Total: 100})
			}
		}()
		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Fatal("BatchChanged blocked on a full queue")
		}
	})
}

// bnWindow is the progress window of batch-state-machine.md §3 and the contract (progressWindowMs 500).
const bnWindow = 500 * time.Millisecond

// bnStep is one call of a coalescer scenario, at an offset from the scenario start.
type bnStep struct {
	at   time.Duration
	op   string // "counters", "state" or "due"
	want bool   // counters: sendNow; due: due; state: unused
}

func TestProgressCoalescer(t *testing.T) {
	t.Run("MET-054/primeira mudança de contador sai na hora", func(t *testing.T) {
		clock := domain.NewFakeClock(time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC))
		c := newProgressCoalescer(bnWindow)
		if !c.counters(clock.Now()) {
			t.Fatal("first counter change: sendNow = false, want true")
		}
		if _, ok := c.next(); ok {
			t.Error("next() after an immediate send reports a pending window, want none")
		}
	})

	t.Run("MET-054/mudanças dentro da janela de 500 ms viram uma no fim", func(t *testing.T) {
		clock := domain.NewFakeClock(time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC))
		c := newProgressCoalescer(bnWindow)
		start := clock.Now()
		if !c.counters(start) {
			t.Fatal("first counter change: sendNow = false, want true")
		}
		for _, off := range []time.Duration{10, 100, 300, 490} {
			if c.counters(start.Add(off * time.Millisecond)) {
				t.Errorf("counter at +%dms inside the window: sendNow = true, want false (folded)", off)
			}
		}
		if c.due(start.Add(499 * time.Millisecond)) {
			t.Error("due at +499ms: true, want false (window still open)")
		}
		if !c.due(start.Add(bnWindow)) {
			t.Error("due at window end: false, want true (one folded notice)")
		}
		if c.due(start.Add(bnWindow)) {
			t.Error("due a second time: true, want false (the pending notice was sent)")
		}
	})

	t.Run("MET-054/next() dá o fim da janela só com pendência", func(t *testing.T) {
		start := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
		c := newProgressCoalescer(bnWindow)
		if _, ok := c.next(); ok {
			t.Fatal("next() on a new coalescer reports a timer, want none")
		}
		c.counters(start)
		if _, ok := c.next(); ok {
			t.Fatal("next() with no pending change reports a timer, want none (no timer without pending)")
		}
		c.counters(start.Add(100 * time.Millisecond))
		end, ok := c.next()
		if !ok || !end.Equal(start.Add(bnWindow)) {
			t.Fatalf("next() = (%v, %v), want (%v, true)", end, ok, start.Add(bnWindow))
		}
	})

	t.Run("MET-054/depois da janela, a próxima sai na hora de novo", func(t *testing.T) {
		start := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
		c := newProgressCoalescer(bnWindow)
		c.counters(start)
		if !c.counters(start.Add(bnWindow + 100*time.Millisecond)) {
			t.Error("counter after the window with no pending change: sendNow = false, want true")
		}
	})

	t.Run("MET-054/sem pendência, due() é false em qualquer instante (nenhum temporizador)", func(t *testing.T) {
		start := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
		c := newProgressCoalescer(bnWindow)
		for _, off := range []time.Duration{0, bnWindow, 10 * bnWindow} {
			if c.due(start.Add(off)) {
				t.Errorf("due at +%v with nothing pending: true, want false", off)
			}
		}
	})

	t.Run("MET-054/o agrupador não cria goroutine", func(t *testing.T) {
		// Counted before and after, with no test running in parallel: the coalescer must not add one.
		start := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
		before := runtime.NumGoroutine()
		c := newProgressCoalescer(bnWindow)
		c.counters(start)
		c.counters(start.Add(10 * time.Millisecond))
		c.state(start.Add(20 * time.Millisecond))
		c.due(start.Add(bnWindow))
		if after := runtime.NumGoroutine(); after > before {
			t.Errorf("goroutines went from %d to %d; the coalescer must not start any", before, after)
		}
	})

	t.Run("MET-054/cenários em tabela (como IMG-003)", func(t *testing.T) {
		cases := []struct {
			name  string
			steps []bnStep
		}{
			{"sem contadores, due nunca dispara", []bnStep{
				{0, "due", false}, {500 * time.Millisecond, "due", false}}},
			{"primeira na hora; as seguintes numa só no fim da janela", []bnStep{
				{0, "counters", true}, {100 * time.Millisecond, "counters", false}, {300 * time.Millisecond, "counters", false},
				{499 * time.Millisecond, "due", false}, {500 * time.Millisecond, "due", true}, {500 * time.Millisecond, "due", false}}},
			{"depois da janela, sem pendência, sai na hora", []bnStep{
				{0, "counters", true}, {600 * time.Millisecond, "counters", true}}},
			{"o fim da janela reabre a janela: a seguinte fica pendente até o novo fim", []bnStep{
				{0, "counters", true}, {100 * time.Millisecond, "counters", false}, {500 * time.Millisecond, "due", true},
				{700 * time.Millisecond, "counters", false}, {999 * time.Millisecond, "due", false}, {1000 * time.Millisecond, "due", true}}},
			{"estado sai na hora e reabre a janela a partir dele", []bnStep{
				{0, "counters", true}, {100 * time.Millisecond, "state", false}, {200 * time.Millisecond, "counters", false},
				{599 * time.Millisecond, "due", false}, {600 * time.Millisecond, "due", true}}},
			{"estado reabre a janela e descarta pendente de contador", []bnStep{
				{0, "counters", true}, {100 * time.Millisecond, "counters", false}, {200 * time.Millisecond, "state", false},
				{700 * time.Millisecond, "due", false}, {800 * time.Millisecond, "counters", true}}},
			{"rajada de contadores vira uma notificação por janela", []bnStep{
				{0, "counters", true}, {10 * time.Millisecond, "counters", false}, {50 * time.Millisecond, "counters", false},
				{200 * time.Millisecond, "counters", false}, {490 * time.Millisecond, "counters", false},
				{499 * time.Millisecond, "due", false}, {500 * time.Millisecond, "due", true},
				{510 * time.Millisecond, "counters", false}, {1000 * time.Millisecond, "due", true}}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				start := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
				c := newProgressCoalescer(bnWindow)
				for i, st := range tc.steps {
					now := start.Add(st.at)
					switch st.op {
					case "counters":
						if got := c.counters(now); got != st.want {
							t.Errorf("step %d counters at +%v: sendNow = %v, want %v", i, st.at, got, st.want)
						}
					case "state":
						c.state(now)
					case "due":
						if got := c.due(now); got != st.want {
							t.Errorf("step %d due at +%v: %v, want %v", i, st.at, got, st.want)
						}
					default:
						t.Fatalf("bad op %q", st.op)
					}
				}
			})
		}
	})
}
