package tracker_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// Fixed instants: the machine is pure, so no clock is involved.
var baseT = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

const (
	testGameID    int64 = 5
	testSessionID int64 = 7
	testTitle           = "Hollow Knight"
)

func at(d time.Duration) time.Time { return baseT.Add(d) }

func newMachine() *tracker.Machine {
	return tracker.NewMachine(domain.SessionGracePeriod, domain.SessionHeartbeatInterval, domain.JobPollInterval)
}

func effKinds(effs []tracker.Effect) []tracker.EffectKind {
	out := make([]tracker.EffectKind, 0, len(effs))
	for _, e := range effs {
		out = append(out, e.Kind)
	}
	return out
}

// wantOrder asserts the exact sequence of effect kinds.
func wantOrder(t *testing.T, effs []tracker.Effect, want ...tracker.EffectKind) {
	t.Helper()
	if got := effKinds(effs); !slices.Equal(got, want) {
		t.Fatalf("effect kinds = %v, want %v (in order)", got, want)
	}
}

// wantKinds asserts the multiset of effect kinds (order not checked).
func wantKinds(t *testing.T, effs []tracker.Effect, want ...tracker.EffectKind) {
	t.Helper()
	got := effKinds(effs)
	g := slices.Clone(got)
	w := slices.Clone(want)
	slices.Sort(g)
	slices.Sort(w)
	if !slices.Equal(g, w) {
		t.Fatalf("effect kinds = %v, want (any order) %v", got, want)
	}
}

func indexOf(effs []tracker.Effect, k tracker.EffectKind) int {
	for i, e := range effs {
		if e.Kind == k {
			return i
		}
	}
	return -1
}

// wantBefore asserts that effect a comes before effect b (e.g. the write before the notification).
func wantBefore(t *testing.T, effs []tracker.Effect, a, b tracker.EffectKind) {
	t.Helper()
	ia, ib := indexOf(effs, a), indexOf(effs, b)
	if ia < 0 || ib < 0 || ia > ib {
		t.Fatalf("effect %d must come before %d; got %v", a, b, effKinds(effs))
	}
}

// one returns the single effect of kind k.
func one(t *testing.T, effs []tracker.Effect, k tracker.EffectKind) tracker.Effect {
	t.Helper()
	var found []tracker.Effect
	for _, e := range effs {
		if e.Kind == k {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one effect of kind %d, got %d in %v", k, len(found), effKinds(effs))
	}
	return found[0]
}

func none(t *testing.T, effs []tracker.Effect, kinds ...tracker.EffectKind) {
	t.Helper()
	for _, k := range kinds {
		if indexOf(effs, k) >= 0 {
			t.Fatalf("unexpected effect of kind %d in %v", k, effKinds(effs))
		}
	}
}

func wantState(t *testing.T, m *tracker.Machine, want tracker.State) {
	t.Helper()
	if got := m.State(); got != want {
		t.Fatalf("State() = %d, want %d", got, want)
	}
}

func wantUpdate(t *testing.T, e tracker.Effect, st domain.SessionState, lastSeen time.Time, active time.Duration) {
	t.Helper()
	if e.State != st || !e.At.Equal(lastSeen) || e.Active != active {
		t.Fatalf("UpdateSession = {State:%q At:%v Active:%v}, want {State:%q At:%v Active:%v}",
			e.State, e.At, e.Active, st, lastSeen, active)
	}
}

func wantEnd(t *testing.T, e tracker.Effect, st domain.SessionState, endedAt time.Time, active time.Duration, reason domain.SessionEndReason) {
	t.Helper()
	if e.State != st || !e.At.Equal(endedAt) || e.Active != active || e.Reason != reason {
		t.Fatalf("EndSession = {State:%q At:%v Active:%v Reason:%q}, want {State:%q At:%v Active:%v Reason:%q}",
			e.State, e.At, e.Active, e.Reason, st, endedAt, active, reason)
	}
}

func wantArm(t *testing.T, e tracker.Effect, delay time.Duration) {
	t.Helper()
	if e.Delay != delay {
		t.Fatalf("Arm effect kind %d Delay = %v, want %v", e.Kind, e.Delay, delay)
	}
}

func wantSnap(t *testing.T, got domain.SessionSnapshot, want domain.SessionSnapshot) {
	t.Helper()
	if got.ID != want.ID || got.GameID != want.GameID || got.Title != want.Title || got.State != want.State ||
		!got.StartedAt.Equal(want.StartedAt) || !got.EndedAt.Equal(want.EndedAt) ||
		got.Active != want.Active || !got.AsOf.Equal(want.AsOf) {
		t.Fatalf("snapshot = %+v, want %+v", got, want)
	}
}

func snap(st domain.SessionState, active time.Duration, asOf, endedAt time.Time) domain.SessionSnapshot {
	return domain.SessionSnapshot{
		ID: testSessionID, GameID: testGameID, Title: testTitle, State: st,
		StartedAt: baseT, EndedAt: endedAt, Active: active, AsOf: asOf,
	}
}

// running is a machine in Running since baseT, with the generations of its timers.
type running struct {
	m       *tracker.Machine
	hbGen   uint64
	pollGen uint64
}

func newRunning(t *testing.T) running {
	t.Helper()
	m := newMachine()
	if !m.Begin(testGameID, testTitle) {
		t.Fatal("Begin in Idle returned false")
	}
	m.Prepared(baseT)
	m.Inserted(testSessionID)
	effs := m.Resumed()
	wantState(t, m, tracker.StateRunning)
	return running{
		m:       m,
		hbGen:   one(t, effs, tracker.EffArmHeartbeat).Gen,
		pollGen: one(t, effs, tracker.EffArmPoll).Gen,
	}
}

// toGrace moves a Running machine to Grace via TrackedEmpty and returns the grace generation.
func toGrace(t *testing.T, m *tracker.Machine, now time.Time) uint64 {
	t.Helper()
	effs := m.TrackedEmpty(now)
	wantState(t, m, tracker.StateGrace)
	return one(t, effs, tracker.EffArmGrace).Gen
}

func TestMachineLaunch(t *testing.T) {
	t.Parallel()

	t.Run("SES-001/Prepared emite StartSession com At = t", func(t *testing.T) {
		t.Parallel()
		m := newMachine()
		wantState(t, m, tracker.StateIdle)
		if !m.Begin(testGameID, testTitle) {
			t.Fatal("Begin in Idle returned false")
		}
		wantState(t, m, tracker.StateLaunching)
		effs := m.Prepared(baseT)
		wantOrder(t, effs, tracker.EffStartSession)
		if !effs[0].At.Equal(baseT) {
			t.Fatalf("StartSession At = %v, want %v", effs[0].At, baseT)
		}
		wantState(t, m, tracker.StateLaunching)
		gameID, sessionID, title, ok := m.Pending()
		if !ok || gameID != testGameID || sessionID != 0 || title != testTitle {
			t.Fatalf("Pending() = (%d, %d, %q, %v), want (%d, 0, %q, true)", gameID, sessionID, title, ok, testGameID, testTitle)
		}
	})

	t.Run("SES-001/Inserted emite ResumeJob", func(t *testing.T) {
		t.Parallel()
		m := newMachine()
		m.Begin(testGameID, testTitle)
		m.Prepared(baseT)
		effs := m.Inserted(testSessionID)
		wantOrder(t, effs, tracker.EffResumeJob)
		wantState(t, m, tracker.StateLaunching)
		_, sessionID, _, ok := m.Pending()
		if !ok || sessionID != testSessionID {
			t.Fatalf("Pending() sessionID = %d ok = %v, want %d true", sessionID, ok, testSessionID)
		}
	})

	t.Run("SES-001/Resumed vai a Running com startedAt = t, arma heartbeat e verificação e notifica running com Active 0 e AsOf t", func(t *testing.T) {
		t.Parallel()
		m := newMachine()
		m.Begin(testGameID, testTitle)
		m.Prepared(baseT)
		m.Inserted(testSessionID)
		effs := m.Resumed()
		wantOrder(t, effs, tracker.EffArmHeartbeat, tracker.EffArmPoll, tracker.EffNotify)
		wantState(t, m, tracker.StateRunning)
		wantArm(t, effs[0], domain.SessionHeartbeatInterval)
		wantArm(t, effs[1], domain.JobPollInterval)
		n := effs[2]
		if n.State != domain.SessionRunning {
			t.Fatalf("Notify State = %q, want running", n.State)
		}
		wantSnap(t, n.Session, snap(domain.SessionRunning, 0, baseT, time.Time{}))
	})

	t.Run("SES-012/PrepareFailed volta a Idle sem efeitos", func(t *testing.T) {
		t.Parallel()
		m := newMachine()
		m.Begin(testGameID, testTitle)
		if effs := m.PrepareFailed(); len(effs) != 0 {
			t.Fatalf("PrepareFailed effects = %v, want none", effKinds(effs))
		}
		wantState(t, m, tracker.StateIdle)
		if _, ok := m.Snapshot(baseT); ok {
			t.Fatal("Snapshot ok after PrepareFailed")
		}
		if !m.Begin(testGameID, testTitle) {
			t.Fatal("Begin after PrepareFailed returned false")
		}
	})

	t.Run("SES-012/InsertFailed emite TerminateJob e CloseJob", func(t *testing.T) {
		t.Parallel()
		m := newMachine()
		m.Begin(testGameID, testTitle)
		m.Prepared(baseT)
		effs := m.InsertFailed()
		wantOrder(t, effs, tracker.EffTerminateJob, tracker.EffCloseJob)
		wantState(t, m, tracker.StateIdle)
		if !m.Begin(testGameID, testTitle) {
			t.Fatal("Begin after InsertFailed returned false")
		}
	})

	t.Run("SES-012/ResumeFailed emite TerminateJob, CloseJob e DeleteSession", func(t *testing.T) {
		t.Parallel()
		m := newMachine()
		m.Begin(testGameID, testTitle)
		m.Prepared(baseT)
		m.Inserted(testSessionID)
		effs := m.ResumeFailed()
		wantOrder(t, effs, tracker.EffTerminateJob, tracker.EffCloseJob, tracker.EffDeleteSession)
		none(t, effs, tracker.EffNotify)
		wantState(t, m, tracker.StateIdle)
		if _, _, _, ok := m.Pending(); ok {
			t.Fatal("Pending ok after ResumeFailed")
		}
		if !m.Begin(testGameID, testTitle) {
			t.Fatal("Begin after ResumeFailed returned false")
		}
	})

	t.Run("SES-012/Begin fora de Idle devolve false", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			setup func(t *testing.T) *tracker.Machine
			want  tracker.State
		}{
			{"Launching antes do insert", func(t *testing.T) *tracker.Machine {
				m := newMachine()
				m.Begin(testGameID, testTitle)
				return m
			}, tracker.StateLaunching},
			{"Launching depois do insert", func(t *testing.T) *tracker.Machine {
				m := newMachine()
				m.Begin(testGameID, testTitle)
				m.Prepared(baseT)
				m.Inserted(testSessionID)
				return m
			}, tracker.StateLaunching},
			{"Running", func(t *testing.T) *tracker.Machine { return newRunning(t).m }, tracker.StateRunning},
			{"Grace", func(t *testing.T) *tracker.Machine {
				r := newRunning(t)
				toGrace(t, r.m, at(100*time.Second))
				return r.m
			}, tracker.StateGrace},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				m := tc.setup(t)
				if m.Begin(99, "Other") {
					t.Fatal("Begin returned true outside Idle")
				}
				wantState(t, m, tc.want)
				if gameID, _, title, ok := m.Pending(); !ok || gameID != testGameID || title != testTitle {
					t.Fatalf("Pending() = (%d, %q, %v), want existing session (%d, %q)", gameID, title, ok, testGameID, testTitle)
				}
			})
		}
	})
}

func TestMachineGrace(t *testing.T) {
	t.Parallel()

	t.Run("SES-003/TrackedEmpty em T+100s grava grace, last_seen T+100s, Active 100 s, arma carência e notifica", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		e := at(100 * time.Second)
		effs := r.m.TrackedEmpty(e)
		wantKinds(t, effs, tracker.EffUpdateSession, tracker.EffArmGrace, tracker.EffNotify)
		wantBefore(t, effs, tracker.EffUpdateSession, tracker.EffNotify)
		wantState(t, r.m, tracker.StateGrace)
		wantUpdate(t, one(t, effs, tracker.EffUpdateSession), domain.SessionGrace, e, 100*time.Second)
		wantArm(t, one(t, effs, tracker.EffArmGrace), domain.SessionGracePeriod)
		n := one(t, effs, tracker.EffNotify)
		if n.State != domain.SessionGrace {
			t.Fatalf("Notify State = %q, want grace", n.State)
		}
		wantSnap(t, n.Session, snap(domain.SessionGrace, 100*time.Second, e, time.Time{}))
	})

	t.Run("SES-004/GraceTimeout da geração atual emite EndSession(ended_at = E, exited), CloseJob e Notify ended", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		e := at(100 * time.Second)
		g := toGrace(t, r.m, e)
		due := e.Add(domain.SessionGracePeriod)
		effs := r.m.GraceTimeout(due, g)
		wantKinds(t, effs, tracker.EffEndSession, tracker.EffCloseJob, tracker.EffNotify)
		wantBefore(t, effs, tracker.EffEndSession, tracker.EffNotify)
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionEnded, e, 100*time.Second, domain.EndExited)
		n := one(t, effs, tracker.EffNotify)
		if n.State != domain.SessionEnded {
			t.Fatalf("Notify State = %q, want ended", n.State)
		}
		// IPC-031: AsOf is the instant the grace period expired; EndedAt stays E.
		wantSnap(t, n.Session, snap(domain.SessionEnded, 100*time.Second, due, e))
		wantState(t, r.m, tracker.StateIdle)
		if _, ok := r.m.Snapshot(due); ok {
			t.Fatal("Snapshot ok after the session ended")
		}
		if _, _, _, ok := r.m.Pending(); ok {
			t.Fatal("Pending ok after the session ended")
		}
		if !r.m.Begin(testGameID, testTitle) {
			t.Fatal("Begin after the session ended returned false")
		}
	})

	t.Run("SES-004/GraceTimeout de geração antiga é ignorado", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		g1 := toGrace(t, r.m, at(100*time.Second))
		r.m.ProcessAdded(at(105 * time.Second))
		g2 := toGrace(t, r.m, at(200*time.Second))
		if g1 == g2 {
			t.Fatalf("grace generation reused: %d", g1)
		}
		if effs := r.m.GraceTimeout(at(115*time.Second), g1); len(effs) != 0 {
			t.Fatalf("stale GraceTimeout effects = %v, want none", effKinds(effs))
		}
		wantState(t, r.m, tracker.StateGrace)
		// The current generation still ends the session.
		effs := r.m.GraceTimeout(at(215*time.Second), g2)
		one(t, effs, tracker.EffEndSession)
		wantState(t, r.m, tracker.StateIdle)
	})

	t.Run("SES-005/ProcessAdded em Grace volta a Running com last_seen E+5s e notifica", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		e := at(100 * time.Second)
		toGrace(t, r.m, e)
		back := e.Add(5 * time.Second)
		effs := r.m.ProcessAdded(back)
		wantKinds(t, effs, tracker.EffUpdateSession, tracker.EffArmHeartbeat, tracker.EffNotify)
		wantBefore(t, effs, tracker.EffUpdateSession, tracker.EffNotify)
		wantState(t, r.m, tracker.StateRunning)
		wantUpdate(t, one(t, effs, tracker.EffUpdateSession), domain.SessionRunning, back, 100*time.Second)
		wantArm(t, one(t, effs, tracker.EffArmHeartbeat), domain.SessionHeartbeatInterval)
		n := one(t, effs, tracker.EffNotify)
		if n.State != domain.SessionRunning {
			t.Fatalf("Notify State = %q, want running", n.State)
		}
		wantSnap(t, n.Session, snap(domain.SessionRunning, 100*time.Second, back, time.Time{}))
		// The grace interval does not count: 10 s after coming back, active = 100 + 10.
		s, ok := r.m.Snapshot(back.Add(10 * time.Second))
		if !ok || s.Active != 110*time.Second {
			t.Fatalf("Snapshot Active = %v ok = %v, want 110s true", s.Active, ok)
		}
	})

	t.Run("SES-005/carência antiga depois da volta é ignorada", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		e := at(100 * time.Second)
		g := toGrace(t, r.m, e)
		r.m.ProcessAdded(e.Add(5 * time.Second))
		if effs := r.m.GraceTimeout(e.Add(domain.SessionGracePeriod), g); len(effs) != 0 {
			t.Fatalf("stale GraceTimeout effects = %v, want none", effKinds(effs))
		}
		wantState(t, r.m, tracker.StateRunning)
		if effs := r.m.GraceTimeout(e.Add(5*time.Second+domain.SessionGracePeriod), g); len(effs) != 0 {
			t.Fatalf("stale GraceTimeout effects = %v, want none", effKinds(effs))
		}
		wantState(t, r.m, tracker.StateRunning)
	})

	t.Run("SES-006/sequência T+100, T+105, T+200, T+215 dá 195 s, ended_at T+200s e ended", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toGrace(t, r.m, at(100*time.Second))
		r.m.ProcessAdded(at(105 * time.Second))
		g := toGrace(t, r.m, at(200*time.Second))
		effs := r.m.GraceTimeout(at(215*time.Second), g)
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionEnded, at(200*time.Second), 195*time.Second, domain.EndExited)
		n := one(t, effs, tracker.EffNotify)
		wantSnap(t, n.Session, snap(domain.SessionEnded, 195*time.Second, at(215*time.Second), at(200*time.Second)))
		wantState(t, r.m, tracker.StateIdle)
	})

	t.Run("SES-006/tempo ativo nunca é negativo com o relógio voltando", func(t *testing.T) {
		t.Parallel()
		before := at(-10 * time.Second)

		t.Run("Snapshot antes de runningSince", func(t *testing.T) {
			r := newRunning(t)
			s, ok := r.m.Snapshot(before)
			if !ok || s.Active < 0 {
				t.Fatalf("Snapshot Active = %v ok = %v, want >= 0 true", s.Active, ok)
			}
		})
		t.Run("Heartbeat antes de runningSince", func(t *testing.T) {
			r := newRunning(t)
			effs := r.m.Heartbeat(before, r.hbGen)
			if u := one(t, effs, tracker.EffUpdateSession); u.Active < 0 {
				t.Fatalf("UpdateSession Active = %v, want >= 0", u.Active)
			}
		})
		t.Run("TrackedEmpty antes de runningSince", func(t *testing.T) {
			r := newRunning(t)
			effs := r.m.TrackedEmpty(before)
			if u := one(t, effs, tracker.EffUpdateSession); u.Active < 0 {
				t.Fatalf("UpdateSession Active = %v, want >= 0", u.Active)
			}
			if n := one(t, effs, tracker.EffNotify); n.Session.Active < 0 {
				t.Fatalf("Notify Active = %v, want >= 0", n.Session.Active)
			}
			s, _ := r.m.Snapshot(before)
			if s.Active < 0 {
				t.Fatalf("Grace Snapshot Active = %v, want >= 0", s.Active)
			}
		})
		t.Run("Stop antes de runningSince", func(t *testing.T) {
			r := newRunning(t)
			effs := r.m.Stop(before)
			e := one(t, effs, tracker.EffEndSession)
			if e.Active < 0 {
				t.Fatalf("EndSession Active = %v, want >= 0", e.Active)
			}
			if e.State != domain.SessionDiscarded {
				t.Fatalf("EndSession State = %q, want discarded", e.State)
			}
		})
		t.Run("intervalo negativo não desconta o acumulado", func(t *testing.T) {
			r := newRunning(t)
			toGrace(t, r.m, at(100*time.Second))
			r.m.ProcessAdded(at(105 * time.Second)) // runningSince = T+105, accumulated = 100 s
			s, _ := r.m.Snapshot(at(50 * time.Second))
			if s.Active < 100*time.Second {
				t.Fatalf("Snapshot Active = %v, want >= 100s", s.Active)
			}
			effs := r.m.TrackedEmpty(at(50 * time.Second))
			if u := one(t, effs, tracker.EffUpdateSession); u.Active < 100*time.Second {
				t.Fatalf("UpdateSession Active = %v, want >= 100s", u.Active)
			}
		})
	})

	t.Run("SES-007/vazio em T+29s dá discarded com 29 s", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		e := at(29 * time.Second)
		g := toGrace(t, r.m, e)
		due := e.Add(domain.SessionGracePeriod)
		effs := r.m.GraceTimeout(due, g)
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionDiscarded, e, 29*time.Second, domain.EndExited)
		n := one(t, effs, tracker.EffNotify)
		if n.State != domain.SessionDiscarded {
			t.Fatalf("Notify State = %q, want discarded", n.State)
		}
		wantSnap(t, n.Session, snap(domain.SessionDiscarded, 29*time.Second, due, e))
		wantState(t, r.m, tracker.StateIdle)
	})
}

func TestMachineHeartbeat(t *testing.T) {
	t.Parallel()

	t.Run("SES-009/Heartbeat em Running grava running, last_seen t e o ativo, e rearma", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		gen := r.hbGen
		for _, s := range []time.Duration{30, 60} {
			now := at(s * time.Second)
			effs := r.m.Heartbeat(now, gen)
			wantKinds(t, effs, tracker.EffUpdateSession, tracker.EffArmHeartbeat)
			wantUpdate(t, one(t, effs, tracker.EffUpdateSession), domain.SessionRunning, now, s*time.Second)
			arm := one(t, effs, tracker.EffArmHeartbeat)
			wantArm(t, arm, domain.SessionHeartbeatInterval)
			gen = arm.Gen
			wantState(t, r.m, tracker.StateRunning)
		}
	})

	t.Run("SES-009/Heartbeat em Grace não grava", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toGrace(t, r.m, at(10*time.Second))
		if effs := r.m.Heartbeat(at(30*time.Second), r.hbGen); len(effs) != 0 {
			t.Fatalf("Heartbeat in Grace effects = %v, want none", effKinds(effs))
		}
		wantState(t, r.m, tracker.StateGrace)
	})

	t.Run("SES-009/Heartbeat de geração antiga é ignorado", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			stale func(t *testing.T, r running) uint64
		}{
			{"depois de um heartbeat", func(t *testing.T, r running) uint64 {
				r.m.Heartbeat(at(30*time.Second), r.hbGen)
				return r.hbGen
			}},
			{"depois de Grace e volta a Running", func(t *testing.T, r running) uint64 {
				toGrace(t, r.m, at(10*time.Second))
				r.m.ProcessAdded(at(20 * time.Second))
				return r.hbGen
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := newRunning(t)
				g := tc.stale(t, r)
				if effs := r.m.Heartbeat(at(45*time.Second), g); len(effs) != 0 {
					t.Fatalf("stale Heartbeat effects = %v, want none", effKinds(effs))
				}
				wantState(t, r.m, tracker.StateRunning)
			})
		}
	})
}

func TestMachinePoll(t *testing.T) {
	t.Parallel()
	errRead := errors.New("query job failed")

	t.Run("SES-013/Running com 0 ativos vai a Grace em t", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		now := at(40 * time.Second)
		effs := r.m.Poll(now, r.pollGen, 0, nil)
		// Same as TrackedEmpty(t), plus the poll re-armed (Grace waiters: grace + poll + old heartbeat).
		wantKinds(t, effs, tracker.EffUpdateSession, tracker.EffArmGrace, tracker.EffNotify, tracker.EffArmPoll)
		wantBefore(t, effs, tracker.EffUpdateSession, tracker.EffNotify)
		wantState(t, r.m, tracker.StateGrace)
		wantUpdate(t, one(t, effs, tracker.EffUpdateSession), domain.SessionGrace, now, 40*time.Second)
		wantArm(t, one(t, effs, tracker.EffArmGrace), domain.SessionGracePeriod)
		wantArm(t, one(t, effs, tracker.EffArmPoll), domain.JobPollInterval)
		wantSnap(t, one(t, effs, tracker.EffNotify).Session, snap(domain.SessionGrace, 40*time.Second, now, time.Time{}))
	})

	t.Run("SES-013/Grace com > 0 ativos volta a Running em t", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		effs := r.m.Poll(at(40*time.Second), r.pollGen, 0, nil)
		pollGen := one(t, effs, tracker.EffArmPoll).Gen
		now := at(45 * time.Second)
		effs = r.m.Poll(now, pollGen, 3, nil)
		wantKinds(t, effs, tracker.EffUpdateSession, tracker.EffArmHeartbeat, tracker.EffNotify, tracker.EffArmPoll)
		wantBefore(t, effs, tracker.EffUpdateSession, tracker.EffNotify)
		wantState(t, r.m, tracker.StateRunning)
		wantUpdate(t, one(t, effs, tracker.EffUpdateSession), domain.SessionRunning, now, 40*time.Second)
		wantArm(t, one(t, effs, tracker.EffArmPoll), domain.JobPollInterval)
		wantSnap(t, one(t, effs, tracker.EffNotify).Session, snap(domain.SessionRunning, 40*time.Second, now, time.Time{}))
	})

	t.Run("SES-013/erro não muda o estado e rearma", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			setup  func(t *testing.T) (*tracker.Machine, uint64)
			active int
			want   tracker.State
		}{
			{"Running com erro e 0 ativos", func(t *testing.T) (*tracker.Machine, uint64) {
				r := newRunning(t)
				return r.m, r.pollGen
			}, 0, tracker.StateRunning},
			{"Grace com erro e > 0 ativos", func(t *testing.T) (*tracker.Machine, uint64) {
				r := newRunning(t)
				toGrace(t, r.m, at(10*time.Second))
				return r.m, r.pollGen
			}, 2, tracker.StateGrace},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				m, gen := tc.setup(t)
				effs := m.Poll(at(20*time.Second), gen, tc.active, errRead)
				wantOrder(t, effs, tracker.EffArmPoll)
				wantArm(t, effs[0], domain.JobPollInterval)
				wantState(t, m, tc.want)
			})
		}
	})

	t.Run("SES-013/Running com > 0 rearma", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		effs := r.m.Poll(at(5*time.Second), r.pollGen, 1, nil)
		wantOrder(t, effs, tracker.EffArmPoll)
		wantArm(t, effs[0], domain.JobPollInterval)
		wantState(t, r.m, tracker.StateRunning)
		// The re-armed generation is honored by the next tick.
		effs = r.m.Poll(at(10*time.Second), effs[0].Gen, 0, nil)
		one(t, effs, tracker.EffUpdateSession)
		wantState(t, r.m, tracker.StateGrace)
	})

	t.Run("SES-013/Grace com 0 ativos rearma", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toGrace(t, r.m, at(10*time.Second))
		effs := r.m.Poll(at(15*time.Second), r.pollGen, 0, nil)
		wantOrder(t, effs, tracker.EffArmPoll)
		wantState(t, r.m, tracker.StateGrace)
	})
}

func TestMachineStop(t *testing.T) {
	t.Parallel()

	t.Run("SES-011/Running: EndSession(ended_at = T, ativo até T, agent-shutdown) e CloseJob, sem Notify nem Terminate", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		stop := at(100 * time.Second)
		effs := r.m.Stop(stop)
		wantOrder(t, effs, tracker.EffEndSession, tracker.EffCloseJob)
		none(t, effs, tracker.EffNotify, tracker.EffTerminateJob)
		wantEnd(t, effs[0], domain.SessionEnded, stop, 100*time.Second, domain.EndAgentShutdown)
		wantState(t, r.m, tracker.StateIdle)
	})

	t.Run("SES-011/Grace: EndSession(ended_at = E, exited)", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		e := at(100 * time.Second)
		toGrace(t, r.m, e)
		effs := r.m.Stop(e.Add(7 * time.Second))
		wantOrder(t, effs, tracker.EffEndSession, tracker.EffCloseJob)
		none(t, effs, tracker.EffNotify, tracker.EffTerminateJob)
		wantEnd(t, effs[0], domain.SessionEnded, e, 100*time.Second, domain.EndExited)
		wantState(t, r.m, tracker.StateIdle)
	})

	t.Run("SES-011/Idle não emite nada", func(t *testing.T) {
		t.Parallel()
		m := newMachine()
		if effs := m.Stop(baseT); len(effs) != 0 {
			t.Fatalf("Stop in Idle effects = %v, want none", effKinds(effs))
		}
		wantState(t, m, tracker.StateIdle)
	})
}

func TestMachineSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("IPC-031/Running: Active = acumulado + (now − runningSince), AsOf = now", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			setup  func(t *testing.T, m *tracker.Machine)
			now    time.Time
			active time.Duration
		}{
			{"primeiro intervalo", func(*testing.T, *tracker.Machine) {}, at(252400 * time.Millisecond), 252400 * time.Millisecond},
			{"depois de uma carência", func(t *testing.T, m *tracker.Machine) {
				toGrace(t, m, at(100*time.Second))
				m.ProcessAdded(at(105 * time.Second))
			}, at(130 * time.Second), 125 * time.Second},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := newRunning(t)
				tc.setup(t, r.m)
				s, ok := r.m.Snapshot(tc.now)
				if !ok {
					t.Fatal("Snapshot ok = false in Running")
				}
				wantSnap(t, s, snap(domain.SessionRunning, tc.active, tc.now, time.Time{}))
			})
		}
	})

	t.Run("IPC-031/Grace: Active = acumulado", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toGrace(t, r.m, at(300*time.Second))
		now := at(310 * time.Second)
		s, ok := r.m.Snapshot(now)
		if !ok {
			t.Fatal("Snapshot ok = false in Grace")
		}
		wantSnap(t, s, snap(domain.SessionGrace, 300*time.Second, now, time.Time{}))
	})

	t.Run("IPC-026/Idle e Launching: ok = false", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			setup func(m *tracker.Machine)
		}{
			{"Idle", func(*tracker.Machine) {}},
			{"Launching", func(m *tracker.Machine) { m.Begin(testGameID, testTitle) }},
			{"Launching depois de Prepared", func(m *tracker.Machine) {
				m.Begin(testGameID, testTitle)
				m.Prepared(baseT)
			}},
			{"Launching depois de Inserted", func(m *tracker.Machine) {
				m.Begin(testGameID, testTitle)
				m.Prepared(baseT)
				m.Inserted(testSessionID)
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				m := newMachine()
				tc.setup(m)
				if _, ok := m.Snapshot(at(time.Second)); ok {
					t.Fatal("Snapshot ok = true, want false")
				}
			})
		}
	})
}

func TestMachineStaleTimers(t *testing.T) {
	t.Parallel()

	t.Run("SES-013/Poll de geração antiga é ignorado", func(t *testing.T) {
		t.Parallel()

		t.Run("em Running, geração anterior ao rearme", func(t *testing.T) {
			r := newRunning(t)
			effs := r.m.Poll(at(5*time.Second), r.pollGen, 1, nil)
			fresh := one(t, effs, tracker.EffArmPoll).Gen
			if fresh == r.pollGen {
				t.Fatalf("poll generation reused: %d", fresh)
			}
			for _, active := range []int{0, 1} {
				if effs := r.m.Poll(at(10*time.Second), r.pollGen, active, nil); len(effs) != 0 {
					t.Fatalf("stale Poll (active=%d) effects = %v, want none", active, effKinds(effs))
				}
				wantState(t, r.m, tracker.StateRunning)
			}
			// The fresh generation is still honored.
			effs = r.m.Poll(at(10*time.Second), fresh, 0, nil)
			one(t, effs, tracker.EffUpdateSession)
			wantState(t, r.m, tracker.StateGrace)
		})

		t.Run("em Grace, geração anterior ao rearme", func(t *testing.T) {
			r := newRunning(t)
			toGrace(t, r.m, at(10*time.Second))
			effs := r.m.Poll(at(15*time.Second), r.pollGen, 0, nil)
			one(t, effs, tracker.EffArmPoll)
			if effs := r.m.Poll(at(20*time.Second), r.pollGen, 3, nil); len(effs) != 0 {
				t.Fatalf("stale Poll effects = %v, want none", effKinds(effs))
			}
			wantState(t, r.m, tracker.StateGrace)
		})

		t.Run("geração de sessão encerrada disparando numa sessão nova", func(t *testing.T) {
			r := newRunning(t)
			oldPoll, oldHB := r.pollGen, r.hbGen
			g := toGrace(t, r.m, at(100*time.Second))
			r.m.GraceTimeout(at(115*time.Second), g)
			wantState(t, r.m, tracker.StateIdle)

			// New session on the same machine.
			if !r.m.Begin(testGameID, testTitle) {
				t.Fatal("Begin after the session ended returned false")
			}
			r.m.Prepared(at(200 * time.Second))
			r.m.Inserted(testSessionID + 1)
			effs := r.m.Resumed()
			wantState(t, r.m, tracker.StateRunning)
			newPoll := one(t, effs, tracker.EffArmPoll).Gen
			newHB := one(t, effs, tracker.EffArmHeartbeat).Gen
			if newPoll == oldPoll || newHB == oldHB || newPoll == oldHB || newHB == oldPoll {
				t.Fatalf("generations reused across sessions: old poll %d hb %d, new poll %d hb %d", oldPoll, oldHB, newPoll, newHB)
			}

			if effs := r.m.Poll(at(205*time.Second), oldPoll, 0, nil); len(effs) != 0 {
				t.Fatalf("stale Poll from ended session effects = %v, want none", effKinds(effs))
			}
			wantState(t, r.m, tracker.StateRunning)
			if effs := r.m.Heartbeat(at(205*time.Second), oldHB); len(effs) != 0 {
				t.Fatalf("stale Heartbeat from ended session effects = %v, want none", effKinds(effs))
			}
			wantState(t, r.m, tracker.StateRunning)

			// Grace in the new session: the old grace generation must not end it.
			toGrace(t, r.m, at(210*time.Second))
			if effs := r.m.GraceTimeout(at(225*time.Second), g); len(effs) != 0 {
				t.Fatalf("stale GraceTimeout from ended session effects = %v, want none", effKinds(effs))
			}
			wantState(t, r.m, tracker.StateGrace)
		})
	})

	t.Run("SES-004/depois do fim (Idle), timers da sessão encerrada não emitem nada", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			end  func(t *testing.T, r running) (grace uint64)
		}{
			{"fim por GraceTimeout", func(t *testing.T, r running) uint64 {
				g := toGrace(t, r.m, at(100*time.Second))
				r.m.GraceTimeout(at(115*time.Second), g)
				return g
			}},
			{"fim por Stop em Running", func(t *testing.T, r running) uint64 {
				r.m.Stop(at(100 * time.Second))
				return 0
			}},
			{"fim por Stop em Grace", func(t *testing.T, r running) uint64 {
				g := toGrace(t, r.m, at(100*time.Second))
				r.m.Stop(at(105 * time.Second))
				return g
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := newRunning(t)
				g := tc.end(t, r)
				wantState(t, r.m, tracker.StateIdle)
				now := at(200 * time.Second)
				t.Run("SES-009/Heartbeat", func(t *testing.T) {
					if effs := r.m.Heartbeat(now, r.hbGen); len(effs) != 0 {
						t.Fatalf("Heartbeat after end effects = %v, want none", effKinds(effs))
					}
				})
				t.Run("SES-013/Poll", func(t *testing.T) {
					for _, active := range []int{0, 2} {
						if effs := r.m.Poll(now, r.pollGen, active, nil); len(effs) != 0 {
							t.Fatalf("Poll (active=%d) after end effects = %v, want none", active, effKinds(effs))
						}
					}
				})
				if g != 0 {
					t.Run("SES-004/GraceTimeout", func(t *testing.T) {
						if effs := r.m.GraceTimeout(now, g); len(effs) != 0 {
							t.Fatalf("GraceTimeout after end effects = %v, want none", effKinds(effs))
						}
					})
				}
				wantState(t, r.m, tracker.StateIdle)
				if _, ok := r.m.Snapshot(now); ok {
					t.Fatal("Snapshot ok after the session ended")
				}
			})
		}
	})
}

func TestMachineIgnoredRows(t *testing.T) {
	t.Parallel()

	t.Run("SES-005/Running × ProcessAdded não emite nada", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		if effs := r.m.ProcessAdded(at(10 * time.Second)); len(effs) != 0 {
			t.Fatalf("ProcessAdded in Running effects = %v, want none", effKinds(effs))
		}
		wantState(t, r.m, tracker.StateRunning)
		// Timers and active time are untouched.
		s, ok := r.m.Snapshot(at(20 * time.Second))
		if !ok || s.Active != 20*time.Second {
			t.Fatalf("Snapshot Active = %v ok = %v, want 20s true", s.Active, ok)
		}
		if effs := r.m.Heartbeat(at(30*time.Second), r.hbGen); len(effs) == 0 {
			t.Fatal("heartbeat generation invalidated by an ignored ProcessAdded")
		}
	})

	t.Run("SES-003/Grace × TrackedEmpty não emite nada", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		e := at(100 * time.Second)
		g := toGrace(t, r.m, e)
		if effs := r.m.TrackedEmpty(at(105 * time.Second)); len(effs) != 0 {
			t.Fatalf("TrackedEmpty in Grace effects = %v, want none", effKinds(effs))
		}
		wantState(t, r.m, tracker.StateGrace)
		// The original grace generation and graceSince still hold.
		effs := r.m.GraceTimeout(e.Add(domain.SessionGracePeriod), g)
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionEnded, e, 100*time.Second, domain.EndExited)
		wantState(t, r.m, tracker.StateIdle)
	})
}
