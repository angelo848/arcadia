package ipc_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// sessionZone is a non-UTC zone so the tests verify the UTC conversion.
var sessionZone = time.FixedZone("BRT", -3*60*60)

// sessionStart is T in IPC-031's examples (14:00:00 UTC).
var sessionStart = time.Date(2026, 11, 9, 11, 0, 0, 0, sessionZone)

func runningSnapshot(active time.Duration, asOf time.Time) domain.SessionSnapshot {
	return domain.SessionSnapshot{
		ID:        7,
		GameID:    3,
		Title:     "Hollow Knight",
		State:     domain.SessionRunning,
		StartedAt: sessionStart,
		Active:    active,
		AsOf:      asOf,
	}
}

// sessionWire marshals s and returns its members.
func sessionWire(t *testing.T, s ipc.Session) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal Session: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("Session JSON is not an object: %v (%s)", err, raw)
	}
	return m
}

func TestSessionFromSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("IPC-031/Active 252,4 s dá activeMs 252400 e activeSeconds 252", func(t *testing.T) {
		t.Parallel()
		asOf := sessionStart.Add(252400 * time.Millisecond)
		got := ipc.SessionFromSnapshot(runningSnapshot(252400*time.Millisecond, asOf))
		if got.ActiveMs != 252400 {
			t.Errorf("ActiveMs = %d, want 252400", got.ActiveMs)
		}
		if got.ActiveSeconds != 252 {
			t.Errorf("ActiveSeconds = %d, want 252", got.ActiveSeconds)
		}
		if !got.AsOf.Time.Equal(asOf) {
			t.Errorf("AsOf = %v, want %v", got.AsOf.Time, asOf)
		}
		// Identity fields.
		if got.SessionID != 7 || got.GameID != 3 || got.Title != "Hollow Knight" || got.State != "running" {
			t.Errorf("identity = {%d %d %q %q}, want {7 3 \"Hollow Knight\" \"running\"}",
				got.SessionID, got.GameID, got.Title, got.State)
		}
		if !got.StartedAt.Equal(sessionStart) || got.StartedAt.Location() != time.UTC {
			t.Errorf("StartedAt = %v (%v), want %v in UTC", got.StartedAt, got.StartedAt.Location(), sessionStart.UTC())
		}
		w := sessionWire(t, got)
		if s := compactJSON(t, w["activeMs"]); s != "252400" {
			t.Errorf("activeMs JSON = %s, want 252400", s)
		}
		if s := compactJSON(t, w["activeSeconds"]); s != "252" {
			t.Errorf("activeSeconds JSON = %s, want 252", s)
		}
		if s := compactJSON(t, w["startedAt"]); s != `"2026-11-09T14:00:00Z"` {
			t.Errorf("startedAt JSON = %s, want \"2026-11-09T14:00:00Z\"", s)
		}
	})

	t.Run("IPC-031/activeMs é truncado (252 400,9 ms dá 252400)", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			active      time.Duration
			wantMs      int64
			wantSeconds int64
		}{
			{252400*time.Millisecond + 900*time.Microsecond, 252400, 252},
			{999*time.Millisecond + 999999*time.Nanosecond, 999, 0},
			{1000 * time.Millisecond, 1000, 1},
			{0, 0, 0},
		}
		for _, tc := range cases {
			got := ipc.SessionFromSnapshot(runningSnapshot(tc.active, sessionStart.Add(tc.active)))
			if got.ActiveMs != tc.wantMs || got.ActiveSeconds != tc.wantSeconds {
				t.Errorf("Active %v: activeMs %d activeSeconds %d, want %d %d",
					tc.active, got.ActiveMs, got.ActiveSeconds, tc.wantMs, tc.wantSeconds)
			}
		}
	})

	t.Run("IPC-031/asOf em UTC com milissegundos", func(t *testing.T) {
		t.Parallel()
		// 14:03:12.345678901 UTC given in BRT; Format truncates to .345.
		asOf := time.Date(2026, 11, 9, 11, 3, 12, 345678901, sessionZone)
		got := ipc.SessionFromSnapshot(runningSnapshot(time.Minute, asOf))
		if got.AsOf.Location() != time.UTC {
			t.Errorf("AsOf location = %v, want UTC", got.AsOf.Location())
		}
		w := sessionWire(t, got)
		if s := compactJSON(t, w["asOf"]); s != `"2026-11-09T14:03:12.345Z"` {
			t.Errorf("asOf JSON = %s, want \"2026-11-09T14:03:12.345Z\"", s)
		}
	})

	t.Run("IPC-031/asOf com ms zero sai com .000Z", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			asOf time.Time
			want string
		}{
			{time.Date(2026, 11, 9, 11, 3, 12, 0, sessionZone), `"2026-11-09T14:03:12.000Z"`},
			{time.Date(2026, 11, 9, 14, 3, 12, 300*int(time.Millisecond), time.UTC), `"2026-11-09T14:03:12.300Z"`},
			{time.Date(2026, 11, 9, 14, 3, 12, 999999, time.UTC), `"2026-11-09T14:03:12.000Z"`},
		} {
			w := sessionWire(t, ipc.SessionFromSnapshot(runningSnapshot(time.Minute, tc.asOf)))
			if s := compactJSON(t, w["asOf"]); s != tc.want {
				t.Errorf("asOf %v JSON = %s, want %s", tc.asOf, s, tc.want)
			}
		}
	})

	t.Run("IPC-031/endedAt null em running e grace", func(t *testing.T) {
		t.Parallel()
		for _, st := range []domain.SessionState{domain.SessionRunning, domain.SessionGrace} {
			snap := runningSnapshot(time.Minute, sessionStart.Add(2*time.Minute))
			snap.State = st
			got := ipc.SessionFromSnapshot(snap)
			if got.EndedAt != nil {
				t.Errorf("%s: EndedAt = %v, want nil", st, *got.EndedAt)
			}
			if got.State != string(st) {
				t.Errorf("State = %q, want %q", got.State, st)
			}
			w := sessionWire(t, got)
			raw, ok := w["endedAt"]
			if !ok {
				t.Fatalf("%s: endedAt member missing (must be present as null)", st)
			}
			if s := compactJSON(t, raw); s != "null" {
				t.Errorf("%s: endedAt JSON = %s, want null", st, s)
			}
		}
	})

	t.Run("IPC-031/endedAt preenchido em ended", func(t *testing.T) {
		t.Parallel()
		for _, st := range []domain.SessionState{domain.SessionEnded, domain.SessionDiscarded} {
			ended := sessionStart.Add(10 * time.Minute)
			snap := runningSnapshot(10*time.Minute, ended.Add(domain.SessionGracePeriod))
			snap.State = st
			snap.EndedAt = ended
			got := ipc.SessionFromSnapshot(snap)
			if got.EndedAt == nil {
				t.Fatalf("%s: EndedAt = nil, want %v", st, ended)
			}
			if !got.EndedAt.Equal(ended) || got.EndedAt.Location() != time.UTC {
				t.Errorf("%s: EndedAt = %v (%v), want %v in UTC", st, *got.EndedAt, got.EndedAt.Location(), ended.UTC())
			}
			w := sessionWire(t, got)
			if s := compactJSON(t, w["endedAt"]); s != `"2026-11-09T14:10:00Z"` {
				t.Errorf("%s: endedAt JSON = %s, want \"2026-11-09T14:10:00Z\"", st, s)
			}
			if s := compactJSON(t, w["state"]); s != `"`+string(st)+`"` {
				t.Errorf("state JSON = %s, want %q", s, st)
			}
		}
	})
}

func TestMilliTime(t *testing.T) {
	t.Parallel()

	t.Run("IPC-031/ida e volta preserva o milissegundo", func(t *testing.T) {
		t.Parallel()
		for _, in := range []time.Time{
			time.Date(2026, 11, 9, 14, 3, 12, 345*int(time.Millisecond), time.UTC),
			time.Date(2026, 11, 9, 11, 3, 12, 1*int(time.Millisecond), sessionZone),
			time.Date(2026, 11, 9, 14, 3, 12, 0, time.UTC),
		} {
			raw, err := json.Marshal(ipc.MilliTime{Time: in})
			if err != nil {
				t.Fatalf("marshal %v: %v", in, err)
			}
			var out ipc.MilliTime
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatalf("unmarshal %s: %v", raw, err)
			}
			if !out.Time.Equal(in) {
				t.Errorf("round trip of %v via %s = %v", in, raw, out.Time)
			}
		}
	})

	t.Run("IPC-031/UnmarshalJSON aceita RFC 3339 sem fração e rejeita texto inválido", func(t *testing.T) {
		t.Parallel()
		var out ipc.MilliTime
		if err := json.Unmarshal([]byte(`"2026-11-09T14:03:12Z"`), &out); err != nil {
			t.Fatalf("unmarshal RFC 3339 without fraction: %v", err)
		}
		if want := time.Date(2026, 11, 9, 14, 3, 12, 0, time.UTC); !out.Time.Equal(want) {
			t.Errorf("got %v, want %v", out.Time, want)
		}
		for _, bad := range []string{`"ontem"`, `123`, `"2026-11-09"`} {
			var v ipc.MilliTime
			if err := json.Unmarshal([]byte(bad), &v); err == nil {
				t.Errorf("unmarshal %s: err = nil, want error", bad)
			}
		}
	})
}

func TestGameJSON(t *testing.T) {
	t.Parallel()

	t.Run("IPC-020/firstPlayedAt e lastPlayedAt nulos saem como null", func(t *testing.T) {
		t.Parallel()
		g := ipc.Game{ID: 1, Title: "Hollow Knight", Path: `C:\Games\hk.exe`}
		raw, err := json.Marshal(g)
		if err != nil {
			t.Fatalf("marshal Game: %v", err)
		}
		want := `{"id":1,"title":"Hollow Knight","path":"C:\\Games\\hk.exe","totalPlaySeconds":0,"playCount":0,"firstPlayedAt":null,"lastPlayedAt":null}`
		if got := compactJSON(t, raw); got != want {
			t.Errorf("Game JSON = %s\nwant       %s", got, want)
		}
	})
}
