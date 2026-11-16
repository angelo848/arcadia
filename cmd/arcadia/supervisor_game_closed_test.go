package main

// Tests for T15: the supervisor forwarding of game.closedByHotkey as the Wails event
// game:closedByHotkey [CON-020]. Same harness and rules as TestSupervisorMediaChanged.

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

const msgGameClosedInvalid = "game.closedByHotkey com parâmetros inválidos"

func TestSupervisorGameClosedByHotkey(t *testing.T) {
	t.Parallel()

	t.Run("CON-020/evento válido é emitido com GameClosedView{GameID}", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		h.client.send(t, ipc.Notification{Method: ipc.MethodGameClosedByHotkey, Params: json.RawMessage(`{"gameId":42}`)})
		h.client.finishNotes()
		h.waitDone(t)

		got := collectEmits(h)
		if len(got) != 1 || got[0].event != gameClosedEvent {
			t.Fatalf("emitted %#v, want exactly one %s", got, gameClosedEvent)
		}
		if v, ok := got[0].data.(GameClosedView); !ok || v.GameID != 42 {
			t.Errorf("data = %#v, want GameClosedView{GameID: 42}", got[0].data)
		}
		if n := h.logs.countLevel(slog.LevelWarn); n != 0 {
			t.Errorf("Warn records = %d, want 0; log:\n%s", n, h.logs.dump())
		}
	})

	t.Run("CON-020/params ausentes, JSON inválido, gameId ausente, não inteiro, 0 e negativo: um Warn cada e nada emitido", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		bad := []json.RawMessage{
			nil,
			json.RawMessage(`{bad`),
			json.RawMessage(`{}`),
			json.RawMessage(`{"gameId":null}`),
			json.RawMessage(`{"gameId":0}`),
			json.RawMessage(`{"gameId":-3}`),
			json.RawMessage(`{"gameId":1.5}`),
			json.RawMessage(`{"gameId":"7"}`),
		}
		for _, p := range bad {
			h.client.send(t, ipc.Notification{Method: ipc.MethodGameClosedByHotkey, Params: p})
		}
		h.client.finishNotes()
		h.waitDone(t)

		if got := collectEmits(h); len(got) != 0 {
			t.Errorf("emitted %#v, want nothing", got)
		}
		if n := len(h.logs.find(0, slog.LevelWarn, msgGameClosedInvalid)); n != len(bad) {
			t.Errorf("%q logged %d times, want %d; log:\n%s", msgGameClosedInvalid, n, len(bad), h.logs.dump())
		}
	})

	t.Run("CON-020/o repasse não chama o agente, não chama Quit, não relança e o supervisor continua conectado", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)
		before := len(h.client.methodCalls())

		h.client.send(t, ipc.Notification{Method: ipc.MethodGameClosedByHotkey, Params: json.RawMessage(`{"gameId":7}`)})
		select {
		case e := <-h.emits:
			if e.event != gameClosedEvent {
				t.Fatalf("event = %q, want %q", e.event, gameClosedEvent)
			}
		case <-time.After(testTimeout):
			t.Fatal("game:closedByHotkey not emitted")
		}
		if h.isDone() || !h.client.Connected() {
			t.Error("supervisor stopped or client disconnected")
		}
		h.client.finishNotes()
		h.waitDone(t)

		if after := len(h.client.methodCalls()); after != before {
			t.Errorf("agent methods called after the notification: %v", h.client.methodCalls())
		}
		if n := h.quitN.Load(); n != 0 {
			t.Errorf("Quit called %d times", n)
		}
		if n := h.ens.n.Load(); n != 1 {
			t.Errorf("Ensure called %d times, want 1", n)
		}
	})

	t.Run("CON-020/media.changed continua repassado ao lado de game.closedByHotkey", func(t *testing.T) {
		t.Parallel()
		h := newSupHarness(t, supOptions{})
		h.start(t)
		h.connectOK(t)

		h.client.send(t, ipc.Notification{Method: ipc.MethodGameClosedByHotkey, Params: json.RawMessage(`{"gameId":1}`)})
		h.client.send(t, ipc.Notification{Method: ipc.MethodMediaChanged, Params: json.RawMessage(`{"gameId":2}`)})
		h.client.finishNotes()
		h.waitDone(t)

		got := collectEmits(h)
		if len(got) != 2 || got[0].event != gameClosedEvent || got[1].event != mediaChangedEvent {
			t.Fatalf("emitted %#v, want game:closedByHotkey then media:changed", got)
		}
	})
}
