package main

// Tests for T27 (docs/specs/phase-13-modo-console/tasks.md): the Video* bindings [CON-027, VID-005..VID-008].
//
// Assumptions: every Video* binding uses libraryTimeout; errors pass through userError (UI-008);
// VideoQueue always sends gameIds and returns a non-nil Skipped; VideoQueueMissing omits platformId
// when 0; VideoStatus maps absent quota counts to -1 and an absent pausedUntil to "".

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

type videoBinding struct {
	name   string
	method string
	call   func(a *App) error
}

var videoBindings = []videoBinding{
	{"VideoQueue", ipc.MethodVideoQueue, func(a *App) error { _, err := a.VideoQueue([]int64{1}); return err }},
	{"VideoQueueMissing", ipc.MethodVideoQueueMissing, func(a *App) error { _, err := a.VideoQueueMissing(0); return err }},
	{"VideoCancelPending", ipc.MethodVideoCancelPending, func(a *App) error { _, err := a.VideoCancelPending(); return err }},
	{"VideoStatus", ipc.MethodVideoStatus, func(a *App) error { _, err := a.VideoStatus(); return err }},
}

func TestVideoBindings(t *testing.T) {
	t.Parallel()

	t.Run("CON-027/o binding expõe os quatro Video*", func(t *testing.T) {
		t.Parallel()
		typ := reflect.TypeOf(&App{})
		for _, b := range videoBindings {
			if _, ok := typ.MethodByName(b.name); !ok {
				t.Errorf("*App has no method %s", b.name)
			}
		}
	})

	t.Run("VID-005/VideoQueue envia gameIds com prazo de libraryTimeout e devolve queued e skipped", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodVideoQueue: map[string]any{
			"queued":  2,
			"skipped": []any{map[string]any{"gameId": 9, "reason": "has-video"}, map[string]any{"gameId": 10, "reason": "platform-unmapped"}},
		}}}
		a := newBindingApp(p, &gameEmitLog{})

		got, err := a.VideoQueue([]int64{3, 9, 10})
		if err != nil {
			t.Fatalf("VideoQueue err = %v", err)
		}
		m := wireMap(t, singleCall(t, p, ipc.MethodVideoQueue, libraryTimeout))
		if !reflect.DeepEqual(m["gameIds"], []any{float64(3), float64(9), float64(10)}) {
			t.Errorf("params = %v, want gameIds [3 9 10]", m)
		}
		want := VideoQueueView{Queued: 2, Skipped: []VideoSkipView{{GameID: 9, Reason: "has-video"}, {GameID: 10, Reason: "platform-unmapped"}}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("view = %+v, want %+v", got, want)
		}
		raw, _ := json.Marshal(got)
		for _, k := range []string{`"queued":2`, `"skipped":[`, `"gameId":9`, `"reason":"has-video"`} {
			if !strings.Contains(string(raw), k) {
				t.Errorf("JSON %s has no %s", raw, k)
			}
		}
	})

	t.Run("VID-005/VideoQueue sem pulados devolve Skipped vazio, nunca nil", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodVideoQueue: map[string]any{"queued": 1, "skipped": nil}}}
		a := newBindingApp(p, &gameEmitLog{})
		got, err := a.VideoQueue([]int64{1})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Skipped == nil {
			t.Error("Skipped = nil, want empty slice")
		}
		raw, _ := json.Marshal(got)
		if !strings.Contains(string(raw), `"skipped":[]`) {
			t.Errorf("JSON %s, want skipped as []", raw)
		}
	})

	t.Run("VID-006/VideoQueueMissing envia platformId e devolve queued e skipped como números", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodVideoQueueMissing: map[string]any{"queued": 5, "skipped": 3}}}
		a := newBindingApp(p, &gameEmitLog{})
		got, err := a.VideoQueueMissing(4)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		m := wireMap(t, singleCall(t, p, ipc.MethodVideoQueueMissing, libraryTimeout))
		if m["platformId"] != float64(4) {
			t.Errorf("params = %v, want platformId 4", m)
		}
		if got != (VideoQueueMissingView{Queued: 5, Skipped: 3}) {
			t.Errorf("view = %+v", got)
		}
		raw, _ := json.Marshal(got)
		if !strings.Contains(string(raw), `"queued":5`) || !strings.Contains(string(raw), `"skipped":3`) {
			t.Errorf("JSON = %s", raw)
		}
	})

	t.Run("VID-006/VideoQueueMissing(0) omite platformId (biblioteca toda)", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodVideoQueueMissing: map[string]any{"queued": 0, "skipped": 0}}}
		a := newBindingApp(p, &gameEmitLog{})
		if _, err := a.VideoQueueMissing(0); err != nil {
			t.Fatalf("err = %v", err)
		}
		c := singleCall(t, p, ipc.MethodVideoQueueMissing, libraryTimeout)
		if len(c.params) > 0 && string(c.params) != "null" {
			if _, ok := wireMap(t, c)["platformId"]; ok {
				t.Errorf("params = %s, want platformId omitted", c.params)
			}
		}
	})

	t.Run("VID-007/VideoCancelPending não envia params e devolve cancelled", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodVideoCancelPending: map[string]any{"cancelled": 7}}}
		a := newBindingApp(p, &gameEmitLog{})
		n, err := a.VideoCancelPending()
		if err != nil || n != 7 {
			t.Fatalf("VideoCancelPending = %d, %v, want 7, nil", n, err)
		}
		c := singleCall(t, p, ipc.MethodVideoCancelPending, libraryTimeout)
		if s := string(c.params); s != "" && s != "null" && s != "{}" {
			t.Errorf("params = %s, want none", s)
		}
	})

	t.Run("VID-008/VideoStatus completo vira VideoStatusView em camelCase", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodVideoStatus: map[string]any{"status": map[string]any{
			"pending": 4, "running": 1, "failed": 2, "lane": "paused-quota",
			"pausedUntil": "2026-11-16T00:00:00Z", "requestsToday": 20000, "maxRequestsPerDay": 20000,
		}}}}
		a := newBindingApp(p, &gameEmitLog{})
		got, err := a.VideoStatus()
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		singleCall(t, p, ipc.MethodVideoStatus, libraryTimeout)
		want := VideoStatusView{Pending: 4, Running: 1, Failed: 2, Lane: "paused-quota",
			PausedUntil: "2026-11-16T00:00:00Z", RequestsToday: 20000, MaxRequestsPerDay: 20000}
		if got != want {
			t.Errorf("view = %+v, want %+v", got, want)
		}
		raw, _ := json.Marshal(got)
		for _, k := range []string{`"pending":4`, `"running":1`, `"failed":2`, `"lane":"paused-quota"`,
			`"pausedUntil":"2026-11-16T00:00:00Z"`, `"requestsToday":20000`, `"maxRequestsPerDay":20000`} {
			if !strings.Contains(string(raw), k) {
				t.Errorf("JSON %s has no %s", raw, k)
			}
		}
	})

	t.Run("VID-008/VideoStatus sem cota nem pausa: PausedUntil vazio e contagens -1", func(t *testing.T) {
		t.Parallel()
		p := &gamePinger{results: map[string]any{ipc.MethodVideoStatus: map[string]any{"status": map[string]any{
			"pending": 0, "running": 0, "failed": 0, "lane": "no-credentials",
		}}}}
		a := newBindingApp(p, &gameEmitLog{})
		got, err := a.VideoStatus()
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.PausedUntil != "" || got.RequestsToday != -1 || got.MaxRequestsPerDay != -1 || got.Lane != "no-credentials" {
			t.Errorf("view = %+v, want PausedUntil \"\", counts -1, lane no-credentials", got)
		}
	})

	for _, b := range videoBindings {
		t.Run("CON-027/UI-008 "+b.name+" com conexão perdida mostra txtNotResponding", func(t *testing.T) {
			t.Parallel()
			a := newBindingApp(&gamePinger{errs: map[string]error{b.method: ipc.ErrConnectionLost}}, &gameEmitLog{})
			if err := b.call(a); err == nil || err.Error() != txtNotResponding {
				t.Errorf("err = %v, want %q", err, txtNotResponding)
			}
		})
		t.Run("CON-027/UI-008 "+b.name+" com erro desconhecido mostra txtUnexpected", func(t *testing.T) {
			t.Parallel()
			a := newBindingApp(&gamePinger{errs: map[string]error{b.method: ipc.NewError(ipc.CodeInternalError, "boom")}}, &gameEmitLog{})
			if err := b.call(a); err == nil || err.Error() != txtUnexpected {
				t.Errorf("err = %v, want %q", err, txtUnexpected)
			}
		})
	}

	t.Run("VID-005/UI-008 VideoQueue e VideoQueueMissing sem credenciais mostram msgProviderNoKey", func(t *testing.T) {
		t.Parallel()
		for _, b := range videoBindings[:2] {
			a := newBindingApp(&gamePinger{errs: map[string]error{
				b.method: ipc.NewError(ipc.CodeProviderNoKey, ipc.MsgProviderNoKey),
			}}, &gameEmitLog{})
			if err := b.call(a); err == nil || err.Error() != msgProviderNoKey {
				t.Errorf("%s err = %v, want %q", b.name, err, msgProviderNoKey)
			}
		}
	})
}
