package ipc_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// notifyMethod is a notification name used only by these tests; Notify must not
// care about the method.
const notifyMethod = "test.event"

// notifyDropMsg is the IPC-029 Warn message for a notification dropped because
// the connection's outgoing queue was full (tasks.md T05).
const notifyDropMsg = "notificação descartada: fila da conexão cheia"

type notifyParams struct {
	Seq  int    `json:"seq"`
	Name string `json:"name"`
}

// assertNotification checks line is exactly a notification (no id member)
// with the given method and params equal to the JSON encoding of want.
func assertNotification(t *testing.T, line []byte, method string, want any) {
	t.Helper()
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(line, &keys); err != nil {
		t.Fatalf("notification is not a JSON object: %v (%q)", err, line)
	}
	if _, ok := keys["id"]; ok {
		t.Fatalf("%s must be a notification without id: %q", method, line)
	}
	if got := sortedKeys(keys); strings.Join(got, ",") != "jsonrpc,method,params" {
		t.Fatalf("notification members = %v, want [jsonrpc method params] (%q)", got, line)
	}
	if got := compactJSON(t, keys["jsonrpc"]); got != `"2.0"` {
		t.Fatalf("notification jsonrpc = %s", got)
	}
	if got, w := compactJSON(t, keys["method"]), fmt.Sprintf("%q", method); got != w {
		t.Fatalf("notification method = %s, want %s (%q)", got, w, line)
	}
	wantRaw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want params: %v", err)
	}
	if got := compactJSON(t, keys["params"]); got != string(wantRaw) {
		t.Fatalf("notification params = %s, want %s", got, wantRaw)
	}
}

// countLogs counts records at level whose formatted line contains every sub.
func countLogs(logs *logStore, level slog.Level, subs ...string) int {
	n := 0
	for _, l := range logs.all() {
		if !strings.HasPrefix(l, level.String()+" ") {
			continue
		}
		ok := true
		for _, s := range subs {
			if !strings.Contains(l, s) {
				ok = false
				break
			}
		}
		if ok {
			n++
		}
	}
	return n
}

func TestNotify(t *testing.T) {
	t.Parallel()

	t.Run("IPC-029/cada conexão recebe a notificação, sem id", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), nil)
		conns := []*rawConn{dialRaw(t, ln), dialRaw(t, ln), dialRaw(t, ln)}
		for _, c := range conns {
			c.ping(t, 1) // every connection is registered
		}

		want := notifyParams{Seq: 1, Name: "Jogo"}
		within(t, "Notify", func() { r.srv.Notify(notifyMethod, want) })
		for _, line := range readEach(t, conns) {
			assertNotification(t, line, notifyMethod, want)
		}
		// The connections keep working after the notification.
		for _, c := range conns {
			c.ping(t, 2)
		}
	})

	t.Run("IPC-029/fila cheia: só aquela conexão perde, com Warn contendo method e remote", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		clk := domain.NewFakeClock(testStart)
		log, logs := newCaptureLogger()
		r := startServer(t, ln, clk, testHandlers(), log, ipc.WithStartedAt(testStart))
		_, healthy := stuckConn(t, ln, clk, 2)

		want := notifyParams{Seq: 2, Name: "cheia"}
		// Notify never blocks, even with a stuck connection.
		within(t, "Notify with a full queue", func() { r.srv.Notify(notifyMethod, want) })
		for _, line := range readEach(t, healthy) {
			assertNotification(t, line, notifyMethod, want)
		}

		// Exactly one drop (the stuck connection), at Warn, with method and remote
		// (net.Pipe's RemoteAddr is "pipe").
		if n := countLogs(logs, slog.LevelWarn, notifyDropMsg, "method="+notifyMethod, "remote=pipe"); n != 1 {
			t.Fatalf("%d Warn %q logs with method=%s and remote=pipe, want 1; logs: %v",
				n, notifyDropMsg, notifyMethod, logs.all())
		}
		for _, c := range healthy {
			c.ping(t, 3)
		}
	})

	t.Run("IPC-029/antes do Serve não faz nada", func(t *testing.T) {
		t.Parallel()
		log, logs := newCaptureLogger()
		srv := ipc.NewServer(domain.NewFakeClock(testStart), testHandlers(), log)
		within(t, "Notify before Serve", func() {
			srv.Notify(notifyMethod, notifyParams{Seq: 3})
		})
		if n := countLogs(logs, slog.LevelWarn); n != 0 {
			t.Fatalf("Notify before Serve logged %d Warn(s); logs: %v", n, logs.all())
		}

		// Serving afterwards: nothing was kept for later connections.
		ln := newMemListener()
		r := &serverRun{srv: srv, ln: ln, done: make(chan struct{})}
		go runServe(r)
		t.Cleanup(func() { r.stop(t) })
		c := dialRaw(t, ln)
		c.ping(t, 1) // first line is the pong, not the earlier notification
	})

	t.Run("IPC-029/depois do Close não faz nada", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		log, logs := newCaptureLogger()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), log)
		c := dialRaw(t, ln)
		c.ping(t, 1)
		r.stop(t)

		within(t, "Notify after Close", func() {
			r.srv.Notify(notifyMethod, notifyParams{Seq: 4})
			r.srv.Notify(notifyMethod, notifyParams{Seq: 5})
		})
		if n := countLogs(logs, slog.LevelWarn, notifyDropMsg); n != 0 {
			t.Fatalf("Notify after Close logged %d drop(s); logs: %v", n, logs.all())
		}
		assertClosedByServer(t, c, "after Server.Close and Notify")
	})

	t.Run("IPC-029/chamadas concorrentes de várias goroutines", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		log, logs := newCaptureLogger()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), log)
		conns := []*rawConn{dialRaw(t, ln), dialRaw(t, ln)}
		for _, c := range conns {
			c.ping(t, 1)
		}

		// 3 x 5 = 15 notifications fit in every connection's queue (16) even if
		// nobody read, so none may be dropped; the clock is never advanced, so no
		// heartbeat competes for room.
		const senders, perSender = 3, 5
		var wg sync.WaitGroup
		start := make(chan struct{})
		for g := range senders {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for i := range perSender {
					r.srv.Notify(notifyMethod, notifyParams{Seq: g*perSender + i, Name: "concorrente"})
				}
			}()
		}
		close(start)
		within(t, "concurrent Notify calls", wg.Wait)

		for ci, c := range conns {
			seen := make(map[int]bool)
			for range senders * perSender {
				line := c.readLine(t)
				var msg struct {
					Method string       `json:"method"`
					Params notifyParams `json:"params"`
				}
				if err := json.Unmarshal(line, &msg); err != nil {
					t.Fatalf("conn %d: bad line %q: %v", ci, line, err)
				}
				assertNotification(t, line, notifyMethod, msg.Params)
				if seen[msg.Params.Seq] {
					t.Fatalf("conn %d: seq %d received twice", ci, msg.Params.Seq)
				}
				seen[msg.Params.Seq] = true
			}
			if len(seen) != senders*perSender {
				t.Fatalf("conn %d: %d distinct notifications, want %d", ci, len(seen), senders*perSender)
			}
			c.ping(t, 2) // nothing else was queued
		}
		if n := countLogs(logs, slog.LevelWarn, notifyDropMsg); n != 0 {
			t.Fatalf("%d notifications dropped with room in the queues; logs: %v", n, logs.all())
		}
	})

	t.Run("IPC-029/params não serializáveis: loga Error e não envia", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		log, logs := newCaptureLogger()
		r := startServer(t, ln, domain.NewFakeClock(testStart), testHandlers(), log)
		c := dialRaw(t, ln)
		c.ping(t, 1)

		within(t, "Notify with bad params", func() {
			r.srv.Notify(notifyMethod, map[string]any{"bad": make(chan int)})
		})
		if n := countLogs(logs, slog.LevelError, notifyMethod); n != 1 {
			t.Fatalf("%d Error logs mentioning %s for an encoding error, want 1; logs: %v", n, notifyMethod, logs.all())
		}
		c.ping(t, 2) // first line is the pong: nothing was sent
	})

	t.Run("IPC-004/heartbeat descartado continua em Debug", func(t *testing.T) {
		t.Parallel()
		ln := newMemListener()
		clk := domain.NewFakeClock(testStart)
		log, logs := newCaptureLogger()
		startServer(t, ln, clk, testHandlers(), log, ipc.WithStartedAt(testStart))
		stuckConn(t, ln, clk, 1) // heartbeats overflow the stuck connection's queue

		if n := countLogs(logs, slog.LevelDebug, "heartbeat", "remote=pipe"); n == 0 {
			t.Fatalf("no Debug log for a dropped heartbeat; logs: %v", logs.all())
		}
		if n := countLogs(logs, slog.LevelWarn); n != 0 {
			t.Fatalf("dropped heartbeats logged %d Warn(s), want 0 (heartbeat drops stay Debug); logs: %v", n, logs.all())
		}
	})
}
