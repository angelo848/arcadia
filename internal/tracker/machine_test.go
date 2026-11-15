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

// ---- Phase 4: pause (docs/specs/phase-4-pause, contract session-state-machine.md) ----

var allPauseReasons = []domain.PauseReason{domain.PauseHotkey, domain.PauseGamepad, domain.PauseUI, domain.PauseSleep}

// pausedBy moves a Running machine (started at baseT) to Paused at `when` through the path matching reason.
func pausedBy(t *testing.T, r running, when time.Time, reason domain.PauseReason) []tracker.Effect {
	t.Helper()
	var effs []tracker.Effect
	switch reason {
	case domain.PauseUI:
		effs = r.m.Pause(when)
	case domain.PauseSleep:
		effs = r.m.Suspend(when)
	default:
		effs = r.m.TogglePause(when, reason)
	}
	wantState(t, r.m, tracker.StatePaused)
	wantReason(t, r.m, reason)
	return effs
}

func feedbacks(effs []tracker.Effect) []tracker.FeedbackKind {
	var out []tracker.FeedbackKind
	for _, e := range effs {
		if e.Kind == tracker.EffFeedback {
			out = append(out, e.Feedback)
		}
	}
	return out
}

func wantFeedback(t *testing.T, effs []tracker.Effect, want ...tracker.FeedbackKind) {
	t.Helper()
	if got := feedbacks(effs); !slices.Equal(got, want) {
		t.Fatalf("feedback = %v, want %v (effects %v)", got, want, effKinds(effs))
	}
}

func wantNoEffects(t *testing.T, effs []tracker.Effect) {
	t.Helper()
	if len(effs) != 0 {
		t.Fatalf("effects = %v, want none", effKinds(effs))
	}
}

func wantReason(t *testing.T, m *tracker.Machine, want domain.PauseReason) {
	t.Helper()
	if got, ok := m.PausedReason(); !ok || got != want {
		t.Fatalf("PausedReason() = (%q, %v), want (%q, true)", got, ok, want)
	}
}

func wantPauseStart(t *testing.T, e tracker.Effect, reason domain.PauseReason, when time.Time, active time.Duration) {
	t.Helper()
	if e.Pause != reason || e.State != domain.SessionPaused || !e.At.Equal(when) || e.Active != active {
		t.Fatalf("PauseSession = {Pause:%q State:%q At:%v Active:%v}, want {%q paused %v %v}",
			e.Pause, e.State, e.At, e.Active, reason, when, active)
	}
}

func wantPauseEnd(t *testing.T, e tracker.Effect, st domain.SessionState, when time.Time, active time.Duration) {
	t.Helper()
	if e.State != st || !e.At.Equal(when) || e.Active != active {
		t.Fatalf("ResumePause = {State:%q At:%v Active:%v}, want {%q %v %v}", e.State, e.At, e.Active, st, when, active)
	}
}

func wantNotify(t *testing.T, e tracker.Effect, st domain.SessionState, active time.Duration, asOf time.Time) {
	t.Helper()
	if e.State != st {
		t.Fatalf("Notify State = %q, want %q", e.State, st)
	}
	wantSnap(t, e.Session, snap(st, active, asOf, time.Time{}))
}

func TestMachinePause(t *testing.T) {
	t.Parallel()
	t100 := at(100 * time.Second)

	t.Run("SES-016/Running + hotkey em T+100s: Paused, PauseSession(hotkey, active 100 s), notificação paused com activeMs 100000 e asOf T+100s, sem arme de timer", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		effs := pausedBy(t, r, t100, domain.PauseHotkey)
		wantKinds(t, effs, tracker.EffPauseSession, tracker.EffNotify, tracker.EffFeedback)
		wantBefore(t, effs, tracker.EffPauseSession, tracker.EffNotify)
		wantBefore(t, effs, tracker.EffNotify, tracker.EffFeedback)
		none(t, effs, tracker.EffArmHeartbeat, tracker.EffArmPoll, tracker.EffArmGrace, tracker.EffTerminateJob, tracker.EffCloseJob)
		wantPauseStart(t, one(t, effs, tracker.EffPauseSession), domain.PauseHotkey, t100, 100*time.Second)
		wantNotify(t, one(t, effs, tracker.EffNotify), domain.SessionPaused, 100*time.Second, t100)
	})

	t.Run("SES-016/Snapshot em T+130s: paused com activeMs 100000", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseHotkey)
		s, ok := r.m.Snapshot(at(130 * time.Second))
		if !ok {
			t.Fatal("Snapshot ok = false in Paused")
		}
		wantSnap(t, s, snap(domain.SessionPaused, 100*time.Second, at(130*time.Second), time.Time{}))
	})

	t.Run("SES-017/Paused + gatilho em T+160s: Running, ResumePause(running, active 100 s), notificação running; Snapshot em T+170s dá 110000", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseHotkey)
		t160 := at(160 * time.Second)
		effs := r.m.TogglePause(t160, domain.PauseGamepad)
		wantKinds(t, effs, tracker.EffResumePause, tracker.EffNotify, tracker.EffFeedback)
		wantBefore(t, effs, tracker.EffResumePause, tracker.EffNotify)
		none(t, effs, tracker.EffArmHeartbeat, tracker.EffArmPoll, tracker.EffArmGrace)
		wantState(t, r.m, tracker.StateRunning)
		wantPauseEnd(t, one(t, effs, tracker.EffResumePause), domain.SessionRunning, t160, 100*time.Second)
		wantNotify(t, one(t, effs, tracker.EffNotify), domain.SessionRunning, 100*time.Second, t160)
		if _, ok := r.m.PausedReason(); ok {
			t.Fatal("PausedReason ok = true in Running")
		}
		s, ok := r.m.Snapshot(at(170 * time.Second))
		if !ok || s.Active != 110*time.Second || s.State != domain.SessionRunning {
			t.Fatalf("Snapshot = %+v ok = %v, want running 110s", s, ok)
		}
	})

	t.Run("SES-018/Idle, Launching e Grace ignoram TogglePause, Suspend e Wake sem efeitos", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			setup func(t *testing.T) *tracker.Machine
			want  tracker.State
		}{
			{"Idle", func(*testing.T) *tracker.Machine { return newMachine() }, tracker.StateIdle},
			{"Launching", func(*testing.T) *tracker.Machine {
				m := newMachine()
				m.Begin(testGameID, testTitle)
				return m
			}, tracker.StateLaunching},
			{"Launching depois do insert", func(*testing.T) *tracker.Machine {
				m := newMachine()
				m.Begin(testGameID, testTitle)
				m.Prepared(baseT)
				m.Inserted(testSessionID)
				return m
			}, tracker.StateLaunching},
			{"Grace", func(t *testing.T) *tracker.Machine {
				r := newRunning(t)
				toGrace(t, r.m, t100)
				return r.m
			}, tracker.StateGrace},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				m := tc.setup(t)
				now := at(110 * time.Second)
				wantNoEffects(t, m.TogglePause(now, domain.PauseHotkey))
				wantNoEffects(t, m.TogglePause(now, domain.PauseGamepad))
				wantNoEffects(t, m.Suspend(now))
				wantNoEffects(t, m.Wake(now, true))
				wantNoEffects(t, m.Wake(now, false))
				wantState(t, m, tc.want)
				if _, ok := m.PausedReason(); ok {
					t.Fatal("PausedReason ok = true outside Paused")
				}
			})
		}
	})

	t.Run("SES-018/gatilho com instante anterior a startedAt é ignorado", func(t *testing.T) {
		t.Parallel()
		before := at(-time.Second)
		r := newRunning(t)
		wantNoEffects(t, r.m.TogglePause(before, domain.PauseHotkey))
		wantNoEffects(t, r.m.Suspend(before))
		wantState(t, r.m, tracker.StateRunning)
		s, ok := r.m.Snapshot(at(20 * time.Second))
		if !ok || s.Active != 20*time.Second || s.State != domain.SessionRunning {
			t.Fatalf("Snapshot = %+v ok = %v, want running 20s", s, ok)
		}
		// startedAt itself is accepted (t >= startedAt).
		one(t, r.m.TogglePause(baseT, domain.PauseHotkey), tracker.EffPauseSession)
		wantState(t, r.m, tracker.StatePaused)
	})

	t.Run("SES-019/cadeia pausa em 100, retoma em 160, pausa em 200, retoma em 210, job vazio em 300, carência vence em 315: End com 230 s, ended_at T+300s", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		r.m.TogglePause(at(100*time.Second), domain.PauseHotkey)
		r.m.TogglePause(at(160*time.Second), domain.PauseHotkey)
		r.m.TogglePause(at(200*time.Second), domain.PauseHotkey)
		effs := r.m.TogglePause(at(210*time.Second), domain.PauseHotkey)
		wantPauseEnd(t, one(t, effs, tracker.EffResumePause), domain.SessionRunning, at(210*time.Second), 140*time.Second)
		g := toGrace(t, r.m, at(300*time.Second))
		effs = r.m.GraceTimeout(at(315*time.Second), g)
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionEnded, at(300*time.Second), 230*time.Second, domain.EndExited)
		wantSnap(t, one(t, effs, tracker.EffNotify).Session, snap(domain.SessionEnded, 230*time.Second, at(315*time.Second), at(300*time.Second)))
		wantState(t, r.m, tracker.StateIdle)
	})

	t.Run("SES-020/Heartbeat em Paused grava paused com active 100 s e rearma; geração obsoleta é ignorada", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseHotkey)
		// Pausing does not re-arm: the running heartbeat generation is still the current one.
		now := at(120 * time.Second)
		effs := r.m.Heartbeat(now, r.hbGen)
		wantKinds(t, effs, tracker.EffUpdateSession, tracker.EffArmHeartbeat)
		wantUpdate(t, one(t, effs, tracker.EffUpdateSession), domain.SessionPaused, now, 100*time.Second)
		arm := one(t, effs, tracker.EffArmHeartbeat)
		wantArm(t, arm, domain.SessionHeartbeatInterval)
		wantState(t, r.m, tracker.StatePaused)
		// The old generation is now stale.
		wantNoEffects(t, r.m.Heartbeat(at(125*time.Second), r.hbGen))
		now = at(150 * time.Second)
		effs = r.m.Heartbeat(now, arm.Gen)
		wantUpdate(t, one(t, effs, tracker.EffUpdateSession), domain.SessionPaused, now, 100*time.Second)
		none(t, effs, tracker.EffPauseSession, tracker.EffResumePause)
		wantState(t, r.m, tracker.StatePaused)
	})

	t.Run("SES-021/TrackedEmpty em Paused: ResumePause(grace, active 100 s), carência armada, notificação grace, sem feedback", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseHotkey)
		t400 := at(400 * time.Second)
		effs := r.m.TrackedEmpty(t400)
		wantKinds(t, effs, tracker.EffResumePause, tracker.EffArmGrace, tracker.EffNotify)
		wantBefore(t, effs, tracker.EffResumePause, tracker.EffNotify)
		none(t, effs, tracker.EffFeedback, tracker.EffUpdateSession)
		wantState(t, r.m, tracker.StateGrace)
		wantPauseEnd(t, one(t, effs, tracker.EffResumePause), domain.SessionGrace, t400, 100*time.Second)
		wantArm(t, one(t, effs, tracker.EffArmGrace), domain.SessionGracePeriod)
		wantNotify(t, one(t, effs, tracker.EffNotify), domain.SessionGrace, 100*time.Second, t400)
	})

	t.Run("SES-021/carência vence em T+415s: End exited em ended_at T+400s com 100 s", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseHotkey)
		g := one(t, r.m.TrackedEmpty(at(400*time.Second)), tracker.EffArmGrace).Gen
		effs := r.m.GraceTimeout(at(415*time.Second), g)
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionEnded, at(400*time.Second), 100*time.Second, domain.EndExited)
		none(t, effs, tracker.EffFeedback)
		wantState(t, r.m, tracker.StateIdle)
	})

	t.Run("SES-021/ProcessAdded na carência volta a Running e não reabre a pausa", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseHotkey)
		r.m.TrackedEmpty(at(400 * time.Second))
		back := at(405 * time.Second)
		effs := r.m.ProcessAdded(back)
		wantKinds(t, effs, tracker.EffUpdateSession, tracker.EffArmHeartbeat, tracker.EffNotify)
		none(t, effs, tracker.EffPauseSession, tracker.EffResumePause, tracker.EffFeedback)
		wantUpdate(t, one(t, effs, tracker.EffUpdateSession), domain.SessionRunning, back, 100*time.Second)
		wantState(t, r.m, tracker.StateRunning)
		if _, ok := r.m.PausedReason(); ok {
			t.Fatal("pause reopened after ProcessAdded in Grace")
		}
	})

	t.Run("SES-021/ProcessAdded em Paused é ignorado", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseHotkey)
		wantNoEffects(t, r.m.ProcessAdded(at(110*time.Second)))
		wantState(t, r.m, tracker.StatePaused)
		wantReason(t, r.m, domain.PauseHotkey)
	})

	t.Run("SES-022/Poll em Paused com 0 ativos vai a Grace e rearma", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseGamepad)
		now := at(250 * time.Second)
		effs := r.m.Poll(now, r.pollGen, 0, nil)
		wantKinds(t, effs, tracker.EffResumePause, tracker.EffArmGrace, tracker.EffNotify, tracker.EffArmPoll)
		none(t, effs, tracker.EffFeedback)
		wantState(t, r.m, tracker.StateGrace)
		wantPauseEnd(t, one(t, effs, tracker.EffResumePause), domain.SessionGrace, now, 100*time.Second)
		wantArm(t, one(t, effs, tracker.EffArmPoll), domain.JobPollInterval)
		wantNotify(t, one(t, effs, tracker.EffNotify), domain.SessionGrace, 100*time.Second, now)
	})

	t.Run("SES-022/Poll em Paused com ativos > 0 ou com erro continua Paused e rearma", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			active int
			err    error
		}{
			{"ativos > 0", 2, nil},
			{"erro com 0 ativos", 0, errors.New("query job failed")},
			{"erro com ativos > 0", 3, errors.New("query job failed")},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := newRunning(t)
				pausedBy(t, r, t100, domain.PauseHotkey)
				effs := r.m.Poll(at(250*time.Second), r.pollGen, tc.active, tc.err)
				wantOrder(t, effs, tracker.EffArmPoll)
				wantArm(t, effs[0], domain.JobPollInterval)
				wantState(t, r.m, tracker.StatePaused)
				wantReason(t, r.m, domain.PauseHotkey)
			})
		}
	})

	t.Run("SES-023/pausada em T+20s e carência em T+615s: End discarded com 20 s", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, at(20*time.Second), domain.PauseHotkey)
		g := one(t, r.m.TrackedEmpty(at(600*time.Second)), tracker.EffArmGrace).Gen
		effs := r.m.GraceTimeout(at(615*time.Second), g)
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionDiscarded, at(600*time.Second), 20*time.Second, domain.EndExited)
		if n := one(t, effs, tracker.EffNotify); n.State != domain.SessionDiscarded {
			t.Fatalf("Notify State = %q, want discarded", n.State)
		}
		wantState(t, r.m, tracker.StateIdle)
	})

	t.Run("SES-023/29,999 s descartada e 30 s encerrada, com pausas no meio", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			active time.Duration
			state  domain.SessionState
		}{
			{"29,999 s", 29999 * time.Millisecond, domain.SessionDiscarded},
			{"30 s", 30 * time.Second, domain.SessionEnded},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := newRunning(t)
				// 10 s running, paused for 6 min of wall clock, then the rest of the active time.
				r.m.TogglePause(at(10*time.Second), domain.PauseHotkey)
				resume := at(370 * time.Second)
				r.m.TogglePause(resume, domain.PauseHotkey)
				empty := resume.Add(tc.active - 10*time.Second)
				g := toGrace(t, r.m, empty)
				effs := r.m.GraceTimeout(empty.Add(domain.SessionGracePeriod), g)
				wantEnd(t, one(t, effs, tracker.EffEndSession), tc.state, empty, tc.active, domain.EndExited)
			})
		}
	})

	t.Run("SES-024/Stop em Paused: End agent-shutdown com ended_at = now e 100 s, EffCloseJob e sem notificação", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseHotkey)
		stop := at(500 * time.Second)
		effs := r.m.Stop(stop)
		wantOrder(t, effs, tracker.EffEndSession, tracker.EffCloseJob)
		none(t, effs, tracker.EffNotify, tracker.EffTerminateJob, tracker.EffFeedback)
		wantEnd(t, effs[0], domain.SessionEnded, stop, 100*time.Second, domain.EndAgentShutdown)
		wantState(t, r.m, tracker.StateIdle)
		if _, ok := r.m.PausedReason(); ok {
			t.Fatal("PausedReason ok = true after Stop")
		}
	})

	t.Run("SES-026/Suspend em Running: Paused(sleep), PauseSession, notificação paused e nenhum EffFeedback", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		effs := pausedBy(t, r, t100, domain.PauseSleep)
		wantOrder(t, effs, tracker.EffPauseSession, tracker.EffNotify)
		none(t, effs, tracker.EffFeedback, tracker.EffTerminateJob, tracker.EffCloseJob)
		wantPauseStart(t, effs[0], domain.PauseSleep, t100, 100*time.Second)
		wantNotify(t, effs[1], domain.SessionPaused, 100*time.Second, t100)
	})

	t.Run("SES-027/Suspend em Paused(hotkey), Paused(sleep) e Grace é ignorado e o motivo é mantido", func(t *testing.T) {
		t.Parallel()
		for _, reason := range []domain.PauseReason{domain.PauseHotkey, domain.PauseSleep} {
			t.Run(string(reason), func(t *testing.T) {
				r := newRunning(t)
				pausedBy(t, r, t100, reason)
				wantNoEffects(t, r.m.Suspend(at(110*time.Second)))
				wantState(t, r.m, tracker.StatePaused)
				wantReason(t, r.m, reason)
			})
		}
		t.Run("Grace", func(t *testing.T) {
			r := newRunning(t)
			toGrace(t, r.m, t100)
			wantNoEffects(t, r.m.Suspend(at(105*time.Second)))
			wantState(t, r.m, tracker.StateGrace)
		})
	})

	t.Run("SES-028/Wake(userPresent) em Paused(sleep): Running, ResumePause, notificação running e EffFeedback(resumed); active continua 100 s", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseSleep)
		wake := at(28900 * time.Second)
		effs := r.m.Wake(wake, true)
		wantKinds(t, effs, tracker.EffResumePause, tracker.EffNotify, tracker.EffFeedback)
		wantBefore(t, effs, tracker.EffResumePause, tracker.EffNotify)
		wantFeedback(t, effs, tracker.FeedbackResumed)
		wantState(t, r.m, tracker.StateRunning)
		wantPauseEnd(t, one(t, effs, tracker.EffResumePause), domain.SessionRunning, wake, 100*time.Second)
		wantNotify(t, one(t, effs, tracker.EffNotify), domain.SessionRunning, 100*time.Second, wake)
		s, ok := r.m.Snapshot(at(28910 * time.Second))
		if !ok || s.Active != 110*time.Second {
			t.Fatalf("Snapshot Active = %v ok = %v, want 110s true", s.Active, ok)
		}
	})

	t.Run("SES-029/Wake sem usuário presente em Paused(sleep) é ignorado; depois gatilho retoma", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseSleep)
		wantNoEffects(t, r.m.Wake(at(28900*time.Second), false))
		wantState(t, r.m, tracker.StatePaused)
		wantReason(t, r.m, domain.PauseSleep)
		effs := r.m.TogglePause(at(28950*time.Second), domain.PauseHotkey)
		wantKinds(t, effs, tracker.EffResumePause, tracker.EffNotify, tracker.EffFeedback)
		wantState(t, r.m, tracker.StateRunning)
	})

	t.Run("SES-029/Wake em Running é ignorado", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		wantNoEffects(t, r.m.Wake(t100, true))
		wantNoEffects(t, r.m.Wake(t100, false))
		wantState(t, r.m, tracker.StateRunning)
	})

	t.Run("SES-030/Paused(hotkey) e Paused(gamepad): Suspend e depois Wake(userPresent) mantêm a pausa sem efeitos", func(t *testing.T) {
		t.Parallel()
		for _, reason := range []domain.PauseReason{domain.PauseHotkey, domain.PauseGamepad} {
			t.Run(string(reason), func(t *testing.T) {
				r := newRunning(t)
				pausedBy(t, r, t100, reason)
				wantNoEffects(t, r.m.Suspend(at(110*time.Second)))
				wantNoEffects(t, r.m.Wake(at(28900*time.Second), true))
				wantState(t, r.m, tracker.StatePaused)
				wantReason(t, r.m, reason)
				s, _ := r.m.Snapshot(at(29000 * time.Second))
				if s.State != domain.SessionPaused || s.Active != 100*time.Second {
					t.Fatalf("Snapshot = %+v, want paused 100s", s)
				}
			})
		}
	})

	t.Run("SES-035/Pause em Running: Paused(ui), efeitos de SES-016 e EffFeedback(paused)", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		effs := pausedBy(t, r, t100, domain.PauseUI)
		wantKinds(t, effs, tracker.EffPauseSession, tracker.EffNotify, tracker.EffFeedback)
		wantFeedback(t, effs, tracker.FeedbackPaused)
		none(t, effs, tracker.EffArmHeartbeat, tracker.EffArmPoll, tracker.EffArmGrace)
		wantPauseStart(t, one(t, effs, tracker.EffPauseSession), domain.PauseUI, t100, 100*time.Second)
		wantNotify(t, one(t, effs, tracker.EffNotify), domain.SessionPaused, 100*time.Second, t100)
	})

	t.Run("SES-035/Pause em Paused (qualquer motivo) não emite efeitos e mantém o motivo", func(t *testing.T) {
		t.Parallel()
		for _, reason := range allPauseReasons {
			t.Run(string(reason), func(t *testing.T) {
				r := newRunning(t)
				pausedBy(t, r, t100, reason)
				wantNoEffects(t, r.m.Pause(at(120*time.Second)))
				wantState(t, r.m, tracker.StatePaused)
				wantReason(t, r.m, reason)
			})
		}
	})

	t.Run("SES-036/Resume em Paused (hotkey, gamepad, ui e sleep) volta a Running com EffFeedback(resumed)", func(t *testing.T) {
		t.Parallel()
		for _, reason := range allPauseReasons {
			t.Run(string(reason), func(t *testing.T) {
				r := newRunning(t)
				pausedBy(t, r, t100, reason)
				t160 := at(160 * time.Second)
				effs := r.m.Resume(t160)
				wantKinds(t, effs, tracker.EffResumePause, tracker.EffNotify, tracker.EffFeedback)
				wantBefore(t, effs, tracker.EffResumePause, tracker.EffNotify)
				wantFeedback(t, effs, tracker.FeedbackResumed)
				wantState(t, r.m, tracker.StateRunning)
				wantPauseEnd(t, one(t, effs, tracker.EffResumePause), domain.SessionRunning, t160, 100*time.Second)
				wantNotify(t, one(t, effs, tracker.EffNotify), domain.SessionRunning, 100*time.Second, t160)
			})
		}
	})

	t.Run("SES-036/Resume em Running é idempotente e não emite efeitos", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		wantNoEffects(t, r.m.Resume(t100))
		wantState(t, r.m, tracker.StateRunning)
		s, _ := r.m.Snapshot(at(110 * time.Second))
		if s.Active != 110*time.Second {
			t.Fatalf("Active = %v, want 110s (untouched)", s.Active)
		}
	})

	t.Run("SES-037/NotPausable devolve no-session em Idle, grace em Grace e ok=false em Running e Paused; Pause e Resume não emitem efeitos", func(t *testing.T) {
		t.Parallel()
		// ok = true means "not pausable" and carries the reason.
		cases := []struct {
			name    string
			setup   func(t *testing.T) *tracker.Machine
			reason  string
			ok      bool
			refused bool // Pause/Resume must emit nothing
		}{
			{"Idle", func(*testing.T) *tracker.Machine { return newMachine() }, "no-session", true, true},
			{"Launching", func(*testing.T) *tracker.Machine {
				m := newMachine()
				m.Begin(testGameID, testTitle)
				return m
			}, "no-session", true, true},
			{"Grace", func(t *testing.T) *tracker.Machine {
				r := newRunning(t)
				toGrace(t, r.m, t100)
				return r.m
			}, "grace", true, true},
			{"Running", func(t *testing.T) *tracker.Machine { return newRunning(t).m }, "", false, false},
			{"Paused", func(t *testing.T) *tracker.Machine {
				r := newRunning(t)
				pausedBy(t, r, t100, domain.PauseHotkey)
				return r.m
			}, "", false, false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				m := tc.setup(t)
				before := m.State()
				reason, ok := m.NotPausable()
				if ok != tc.ok || (tc.ok && reason != tc.reason) {
					t.Fatalf("NotPausable() = (%q, %v), want (%q, %v)", reason, ok, tc.reason, tc.ok)
				}
				if tc.refused {
					wantNoEffects(t, m.Pause(at(120*time.Second)))
					wantNoEffects(t, m.Resume(at(130*time.Second)))
					wantState(t, m, before)
				}
			})
		}
	})

	t.Run("SES-022/Poll com geração obsoleta em Paused não emite efeitos e não muda o estado", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseHotkey)
		for _, active := range []int{0, 2} {
			wantNoEffects(t, r.m.Poll(at(250*time.Second), r.pollGen+100, active, nil))
			wantNoEffects(t, r.m.Poll(at(250*time.Second), 0, active, nil))
		}
		wantState(t, r.m, tracker.StatePaused)
		wantReason(t, r.m, domain.PauseHotkey)
		// The current generation is still accepted afterwards.
		wantOrder(t, r.m.Poll(at(260*time.Second), r.pollGen, 1, nil), tracker.EffArmPoll)
	})

	t.Run("SES-020/Paused para Grace por TrackedEmpty ou Poll: Heartbeat com a geração antiga é ignorado, também após ProcessAdded", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			toGr func(r running) // Paused -> Grace
		}{
			{"TrackedEmpty", func(r running) { r.m.TrackedEmpty(at(400 * time.Second)) }},
			{"Poll com 0 ativos", func(r running) { r.m.Poll(at(400*time.Second), r.pollGen, 0, nil) }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := newRunning(t)
				pausedBy(t, r, t100, domain.PauseHotkey)
				tc.toGr(r)
				wantState(t, r.m, tracker.StateGrace)
				wantNoEffects(t, r.m.Heartbeat(at(410*time.Second), r.hbGen))

				back := r.m.ProcessAdded(at(420 * time.Second))
				wantState(t, r.m, tracker.StateRunning)
				newGen := one(t, back, tracker.EffArmHeartbeat).Gen
				if newGen == r.hbGen {
					t.Fatalf("heartbeat generation %d reused after Paused->Grace->Running", newGen)
				}
				wantNoEffects(t, r.m.Heartbeat(at(430*time.Second), r.hbGen))
				one(t, r.m.Heartbeat(at(430*time.Second), newGen), tracker.EffUpdateSession)
			})
		}
	})

	t.Run("SES-017/SES-028/Resume, Wake e TogglePause a partir de Paused não invalidam as gerações de heartbeat e poll", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			reason domain.PauseReason
			resume func(m *tracker.Machine, now time.Time) []tracker.Effect
		}{
			{"Resume", domain.PauseUI, func(m *tracker.Machine, now time.Time) []tracker.Effect { return m.Resume(now) }},
			{"Wake", domain.PauseSleep, func(m *tracker.Machine, now time.Time) []tracker.Effect { return m.Wake(now, true) }},
			{"TogglePause", domain.PauseHotkey, func(m *tracker.Machine, now time.Time) []tracker.Effect {
				return m.TogglePause(now, domain.PauseHotkey)
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := newRunning(t)
				pausedBy(t, r, t100, tc.reason)
				effs := tc.resume(r.m, at(160*time.Second))
				none(t, effs, tracker.EffArmHeartbeat, tracker.EffArmPoll)
				wantState(t, r.m, tracker.StateRunning)

				hb := r.m.Heartbeat(at(170*time.Second), r.hbGen)
				wantUpdate(t, one(t, hb, tracker.EffUpdateSession), domain.SessionRunning, at(170*time.Second), 110*time.Second)
				one(t, hb, tracker.EffArmHeartbeat)
				pl := r.m.Poll(at(171*time.Second), r.pollGen, 1, nil)
				wantOrder(t, pl, tracker.EffArmPoll)
			})
		}
	})

	t.Run("SES-021/Heartbeat e Poll na Grace vinda de Paused mantêm o tempo ativo congelado em 100 s", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, t100, domain.PauseHotkey)
		r.m.TrackedEmpty(at(400 * time.Second))
		wantState(t, r.m, tracker.StateGrace)

		wantNoEffects(t, r.m.Heartbeat(at(405*time.Second), r.hbGen))
		s, ok := r.m.Snapshot(at(406 * time.Second))
		if !ok || s.State != domain.SessionGrace || s.Active != 100*time.Second {
			t.Fatalf("Snapshot after Heartbeat = %+v ok = %v, want grace 100s", s, ok)
		}

		// Poll with 0 active in Grace: only re-arms, active stays frozen.
		pollGen := one(t, r.m.Poll(at(407*time.Second), r.pollGen, 0, nil), tracker.EffArmPoll).Gen
		wantState(t, r.m, tracker.StateGrace)
		s, _ = r.m.Snapshot(at(408 * time.Second))
		if s.Active != 100*time.Second {
			t.Fatalf("Active after Poll(0) in Grace = %v, want 100s", s.Active)
		}

		// Poll with active > 0 leaves Grace with the same 100 s carried over.
		back := at(410 * time.Second)
		effs := r.m.Poll(back, pollGen, 2, nil)
		wantUpdate(t, one(t, effs, tracker.EffUpdateSession), domain.SessionRunning, back, 100*time.Second)
		wantState(t, r.m, tracker.StateRunning)
	})
}

func TestMachineFeedback(t *testing.T) {
	t.Parallel()
	t100 := at(100 * time.Second)

	t.Run("FBK-001/pausa por hotkey, gamepad e ui emite FeedbackPaused", func(t *testing.T) {
		t.Parallel()
		for _, reason := range []domain.PauseReason{domain.PauseHotkey, domain.PauseGamepad, domain.PauseUI} {
			t.Run(string(reason), func(t *testing.T) {
				r := newRunning(t)
				wantFeedback(t, pausedBy(t, r, t100, reason), tracker.FeedbackPaused)
			})
		}
	})

	t.Run("FBK-001/retomada por gatilho, Resume e Wake com usuário emite FeedbackResumed", func(t *testing.T) {
		t.Parallel()
		later := at(160 * time.Second)
		cases := []struct {
			name   string
			pause  domain.PauseReason
			resume func(m *tracker.Machine) []tracker.Effect
		}{
			{"gatilho hotkey", domain.PauseHotkey, func(m *tracker.Machine) []tracker.Effect { return m.TogglePause(later, domain.PauseHotkey) }},
			{"gatilho gamepad", domain.PauseGamepad, func(m *tracker.Machine) []tracker.Effect { return m.TogglePause(later, domain.PauseGamepad) }},
			{"gatilho sobre pausa sleep", domain.PauseSleep, func(m *tracker.Machine) []tracker.Effect { return m.TogglePause(later, domain.PauseHotkey) }},
			{"Resume sobre pausa ui", domain.PauseUI, func(m *tracker.Machine) []tracker.Effect { return m.Resume(later) }},
			{"Resume sobre pausa sleep", domain.PauseSleep, func(m *tracker.Machine) []tracker.Effect { return m.Resume(later) }},
			{"Wake com usuário sobre pausa sleep", domain.PauseSleep, func(m *tracker.Machine) []tracker.Effect { return m.Wake(later, true) }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				r := newRunning(t)
				pausedBy(t, r, t100, tc.pause)
				wantFeedback(t, tc.resume(r.m), tracker.FeedbackResumed)
			})
		}
	})

	t.Run("FBK-001/pausa por sleep não emite feedback", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		wantFeedback(t, pausedBy(t, r, t100, domain.PauseSleep))
	})

	t.Run("FBK-001/Paused para Grace não emite feedback", func(t *testing.T) {
		t.Parallel()
		t.Run("TrackedEmpty", func(t *testing.T) {
			r := newRunning(t)
			pausedBy(t, r, t100, domain.PauseHotkey)
			wantFeedback(t, r.m.TrackedEmpty(at(200*time.Second)))
		})
		t.Run("Poll", func(t *testing.T) {
			r := newRunning(t)
			pausedBy(t, r, t100, domain.PauseUI)
			wantFeedback(t, r.m.Poll(at(200*time.Second), r.pollGen, 0, nil))
		})
	})

	t.Run("FBK-001/fim de sessão não emite feedback", func(t *testing.T) {
		t.Parallel()
		t.Run("Stop em Running", func(t *testing.T) {
			r := newRunning(t)
			wantFeedback(t, r.m.Stop(t100))
		})
		t.Run("Stop em Paused", func(t *testing.T) {
			r := newRunning(t)
			pausedBy(t, r, t100, domain.PauseHotkey)
			wantFeedback(t, r.m.Stop(at(200*time.Second)))
		})
		t.Run("GraceTimeout depois de Paused", func(t *testing.T) {
			r := newRunning(t)
			pausedBy(t, r, t100, domain.PauseHotkey)
			g := one(t, r.m.TrackedEmpty(at(200*time.Second)), tracker.EffArmGrace).Gen
			wantFeedback(t, r.m.GraceTimeout(at(215*time.Second), g))
		})
	})

	t.Run("FBK-001/gatilho, evento ou pedido ignorado ou idempotente não emite feedback", func(t *testing.T) {
		t.Parallel()
		t.Run("Idle e Grace", func(t *testing.T) {
			grace := newRunning(t)
			toGrace(t, grace.m, t100)
			for name, m := range map[string]*tracker.Machine{"Idle": newMachine(), "Grace": grace.m} {
				now := at(110 * time.Second)
				for _, effs := range [][]tracker.Effect{
					m.TogglePause(now, domain.PauseHotkey), m.Suspend(now), m.Wake(now, true), m.Pause(now), m.Resume(now),
				} {
					if fb := feedbacks(effs); len(fb) != 0 {
						t.Fatalf("%s: feedback = %v, want none", name, fb)
					}
				}
			}
		})
		t.Run("gatilho antes de startedAt", func(t *testing.T) {
			r := newRunning(t)
			wantFeedback(t, r.m.TogglePause(at(-time.Second), domain.PauseHotkey))
		})
		t.Run("Pause em Paused", func(t *testing.T) {
			r := newRunning(t)
			pausedBy(t, r, t100, domain.PauseHotkey)
			wantFeedback(t, r.m.Pause(at(110*time.Second)))
		})
		t.Run("Resume em Running", func(t *testing.T) {
			r := newRunning(t)
			wantFeedback(t, r.m.Resume(t100))
		})
		t.Run("Suspend e Wake ignorados em Paused", func(t *testing.T) {
			r := newRunning(t)
			pausedBy(t, r, t100, domain.PauseHotkey)
			wantFeedback(t, r.m.Suspend(at(110*time.Second)))
			wantFeedback(t, r.m.Wake(at(120*time.Second), true))
			wantFeedback(t, r.m.Wake(at(130*time.Second), false))
			s := newRunning(t)
			pausedBy(t, s, t100, domain.PauseSleep)
			wantFeedback(t, s.m.Wake(at(120*time.Second), false))
		})
		t.Run("Wake em Running", func(t *testing.T) {
			r := newRunning(t)
			wantFeedback(t, r.m.Wake(t100, true))
		})
	})
}

// ---- Phase 13: close by controller (docs/specs/phase-13-modo-console, contract session-close-state-machine.md) ----

// toClosing moves a Running machine (started at baseT) to Closing at `when`; the trigger must emit no effects.
func toClosing(t *testing.T, r running, when time.Time) {
	t.Helper()
	wantNoEffects(t, r.m.CloseRequested(when))
	wantState(t, r.m, tracker.StateClosing)
}

func wantClosingAt(t *testing.T, m *tracker.Machine, when time.Time, from tracker.State) {
	t.Helper()
	got, gotFrom, ok := m.ClosingAt()
	if !ok || !got.Equal(when) || gotFrom != from {
		t.Fatalf("ClosingAt() = (%v, %d, %v), want (%v, %d, true)", got, gotFrom, ok, when, from)
	}
}

func TestMachineCloseByHotkey(t *testing.T) {
	t.Parallel()
	t600 := at(600 * time.Second)

	t.Run("CON-005/Running com 600 s: CloseRequested entra em Closing sem efeitos e CloseDone grava ended_at=T+600s, active 600 s, closed-by-hotkey, ended", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toClosing(t, r, t600)
		wantClosingAt(t, r.m, t600, tracker.StateRunning)
		effs := r.m.CloseDone(at(602 * time.Second))
		wantKinds(t, effs, tracker.EffEndSession, tracker.EffCloseJob, tracker.EffNotify)
		wantBefore(t, effs, tracker.EffEndSession, tracker.EffNotify)
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionEnded, t600, 600*time.Second, domain.EndClosedByHotkey)
		none(t, effs, tracker.EffTerminateJob, tracker.EffFeedback)
		wantState(t, r.m, tracker.StateIdle)
		if _, _, ok := r.m.ClosingAt(); ok {
			t.Fatal("ClosingAt ok after CloseDone")
		}
		if !r.m.Begin(testGameID, testTitle) {
			t.Fatal("Begin after CloseDone returned false")
		}
	})

	t.Run("CON-005/o instante de CloseDone não muda os valores gravados (T+602s e T+603s), só o AsOf", func(t *testing.T) {
		t.Parallel()
		for _, now := range []time.Time{at(602 * time.Second), at(603 * time.Second)} {
			r := newRunning(t)
			toClosing(t, r, t600)
			effs := r.m.CloseDone(now)
			wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionEnded, t600, 600*time.Second, domain.EndClosedByHotkey)
			n := one(t, effs, tracker.EffNotify)
			if n.State != domain.SessionEnded {
				t.Fatalf("Notify State = %q, want ended", n.State)
			}
			wantSnap(t, n.Session, snap(domain.SessionEnded, 600*time.Second, now, t600))
		}
	})

	t.Run("CON-005/20 s de tempo ativo termina discarded", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toClosing(t, r, at(20*time.Second))
		effs := r.m.CloseDone(at(23 * time.Second))
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionDiscarded, at(20*time.Second), 20*time.Second, domain.EndClosedByHotkey)
		n := one(t, effs, tracker.EffNotify)
		wantSnap(t, n.Session, snap(domain.SessionDiscarded, 20*time.Second, at(23*time.Second), at(20*time.Second)))
	})

	t.Run("CON-005/CloseRequested fora de Running, Paused e Grace é nil", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			setup func(t *testing.T) *tracker.Machine
			want  tracker.State
		}{
			{"Idle", func(*testing.T) *tracker.Machine { return newMachine() }, tracker.StateIdle},
			{"Launching", func(*testing.T) *tracker.Machine {
				m := newMachine()
				m.Begin(testGameID, testTitle)
				return m
			}, tracker.StateLaunching},
			{"Launching depois do insert", func(*testing.T) *tracker.Machine {
				m := newMachine()
				m.Begin(testGameID, testTitle)
				m.Prepared(baseT)
				m.Inserted(testSessionID)
				return m
			}, tracker.StateLaunching},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				m := tc.setup(t)
				wantNoEffects(t, m.CloseRequested(t600))
				wantState(t, m, tc.want)
				if _, _, ok := m.ClosingAt(); ok {
					t.Fatal("ClosingAt ok outside Closing")
				}
			})
		}
	})

	t.Run("CON-012/CloseRequested antes de startedAt em Running ou Paused é ignorado", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		wantNoEffects(t, r.m.CloseRequested(at(-time.Second)))
		wantState(t, r.m, tracker.StateRunning)
		p := newRunning(t)
		pausedBy(t, p, at(100*time.Second), domain.PauseHotkey)
		wantNoEffects(t, p.m.CloseRequested(at(-time.Second)))
		wantState(t, p.m, tracker.StatePaused)
	})

	t.Run("CON-006/Paused com 100 s: tempo congelado, EndSession em closeAt com active 100 s", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, at(100*time.Second), domain.PauseHotkey)
		closeAt := at(130 * time.Second)
		toClosing(t, r, closeAt)
		wantClosingAt(t, r.m, closeAt, tracker.StatePaused)
		effs := r.m.CloseDone(at(133 * time.Second))
		wantKinds(t, effs, tracker.EffEndSession, tracker.EffCloseJob, tracker.EffNotify)
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionEnded, closeAt, 100*time.Second, domain.EndClosedByHotkey)
		wantState(t, r.m, tracker.StateIdle)
	})

	t.Run("CON-007/Grace: CloseRequested devolve EndSession em graceSince, CloseJob e Notify com AsOf=at, e o GraceTimeout antigo vira obsoleto", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		e := at(100 * time.Second)
		g := toGrace(t, r.m, e)
		now := at(105 * time.Second)
		effs := r.m.CloseRequested(now)
		wantKinds(t, effs, tracker.EffEndSession, tracker.EffCloseJob, tracker.EffNotify)
		wantBefore(t, effs, tracker.EffEndSession, tracker.EffNotify)
		wantEnd(t, one(t, effs, tracker.EffEndSession), domain.SessionEnded, e, 100*time.Second, domain.EndClosedByHotkey)
		none(t, effs, tracker.EffTerminateJob, tracker.EffFeedback)
		wantSnap(t, one(t, effs, tracker.EffNotify).Session, snap(domain.SessionEnded, 100*time.Second, now, e))
		wantState(t, r.m, tracker.StateIdle)
		wantNoEffects(t, r.m.GraceTimeout(e.Add(domain.SessionGracePeriod), g))
		wantState(t, r.m, tracker.StateIdle)
	})

	t.Run("CON-013/CloseUndone em Running: runningSince=closeAt, o tempo da espera passa a contar, devolve armHeartbeat e armPoll", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toClosing(t, r, t600)
		effs := r.m.CloseUndone()
		wantKinds(t, effs, tracker.EffArmHeartbeat, tracker.EffArmPoll)
		wantArm(t, one(t, effs, tracker.EffArmHeartbeat), domain.SessionHeartbeatInterval)
		wantArm(t, one(t, effs, tracker.EffArmPoll), domain.JobPollInterval)
		hb, poll := one(t, effs, tracker.EffArmHeartbeat).Gen, one(t, effs, tracker.EffArmPoll).Gen
		if hb == r.hbGen || poll == r.pollGen {
			t.Fatalf("generations not renewed: hb %d (old %d), poll %d (old %d)", hb, r.hbGen, poll, r.pollGen)
		}
		wantState(t, r.m, tracker.StateRunning)
		if _, _, ok := r.m.ClosingAt(); ok {
			t.Fatal("ClosingAt ok after CloseUndone")
		}
		s, ok := r.m.Snapshot(at(610 * time.Second))
		if !ok || s.State != domain.SessionRunning || s.Active != 610*time.Second {
			t.Fatalf("Snapshot = %+v ok = %v, want running 610s (no gap)", s, ok)
		}
		// The new generations are honored; the old ones are not.
		wantNoEffects(t, r.m.Heartbeat(at(620*time.Second), r.hbGen))
		one(t, r.m.Heartbeat(at(620*time.Second), hb), tracker.EffUpdateSession)
	})

	t.Run("CON-013/CloseUndone em Paused não muda nada", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		pausedBy(t, r, at(100*time.Second), domain.PauseHotkey)
		toClosing(t, r, at(130*time.Second))
		r.m.CloseUndone()
		wantState(t, r.m, tracker.StatePaused)
		wantReason(t, r.m, domain.PauseHotkey)
		s, ok := r.m.Snapshot(at(200 * time.Second))
		if !ok || s.State != domain.SessionPaused || s.Active != 100*time.Second {
			t.Fatalf("Snapshot = %+v ok = %v, want paused 100s", s, ok)
		}
		// The open pause interval still ends normally.
		effs := r.m.TogglePause(at(160*time.Second), domain.PauseHotkey)
		wantPauseEnd(t, one(t, effs, tracker.EffResumePause), domain.SessionRunning, at(160*time.Second), 100*time.Second)
	})

	t.Run("CON-014/nenhum caminho de fechamento emite EffFeedback", func(t *testing.T) {
		t.Parallel()
		var all [][]tracker.Effect
		r := newRunning(t)
		all = append(all, r.m.CloseRequested(t600), r.m.CloseDone(at(602*time.Second)))
		p := newRunning(t)
		pausedBy(t, p, at(100*time.Second), domain.PauseHotkey)
		all = append(all, p.m.CloseRequested(at(130*time.Second)), p.m.CloseDone(at(133*time.Second)))
		g := newRunning(t)
		toGrace(t, g.m, at(100*time.Second))
		all = append(all, g.m.CloseRequested(at(105*time.Second)))
		u := newRunning(t)
		all = append(all, u.m.CloseRequested(t600), u.m.CloseUndone())
		s := newRunning(t)
		all = append(all, s.m.CloseRequested(t600), s.m.Stop(at(601*time.Second)))
		for i, effs := range all {
			if fb := feedbacks(effs); len(fb) != 0 {
				t.Fatalf("path %d: feedback = %v, want none", i, fb)
			}
		}
	})

	t.Run("CON-032/Snapshot em Closing é o estado de origem com o tempo de closeAt", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toClosing(t, r, t600)
		now := at(602 * time.Second)
		s, ok := r.m.Snapshot(now)
		if !ok {
			t.Fatal("Snapshot ok = false in Closing")
		}
		wantSnap(t, s, snap(domain.SessionRunning, 600*time.Second, now, time.Time{}))

		p := newRunning(t)
		pausedBy(t, p, at(100*time.Second), domain.PauseHotkey)
		toClosing(t, p, at(130*time.Second))
		s, ok = p.m.Snapshot(now)
		if !ok {
			t.Fatal("Snapshot ok = false in Closing (from Paused)")
		}
		wantSnap(t, s, snap(domain.SessionPaused, 100*time.Second, now, time.Time{}))
	})

	t.Run("CON-032/TogglePause, Pause, Resume, Suspend, Wake e Heartbeat em Closing não geram efeitos", func(t *testing.T) {
		t.Parallel()
		for _, from := range []string{"Running", "Paused"} {
			t.Run(from, func(t *testing.T) {
				r := newRunning(t)
				closeAt, origin := t600, tracker.StateRunning
				if from == "Paused" {
					pausedBy(t, r, at(100*time.Second), domain.PauseHotkey)
					closeAt, origin = at(130*time.Second), tracker.StatePaused
				}
				toClosing(t, r, closeAt)
				now := closeAt.Add(time.Second)
				wantNoEffects(t, r.m.TogglePause(now, domain.PauseHotkey))
				wantNoEffects(t, r.m.TogglePause(now, domain.PauseGamepad))
				wantNoEffects(t, r.m.Pause(now))
				wantNoEffects(t, r.m.Resume(now))
				wantNoEffects(t, r.m.Suspend(now))
				wantNoEffects(t, r.m.Wake(now, true))
				wantNoEffects(t, r.m.Wake(now, false))
				wantNoEffects(t, r.m.Heartbeat(now, r.hbGen))
				wantState(t, r.m, tracker.StateClosing)
				wantClosingAt(t, r.m, closeAt, origin)
			})
		}
	})

	t.Run("CON-032/Poll em Closing só rearma", func(t *testing.T) {
		t.Parallel()
		for _, active := range []int{0, 2} {
			r := newRunning(t)
			toClosing(t, r, t600)
			effs := r.m.Poll(at(601*time.Second), r.pollGen, active, nil)
			wantOrder(t, effs, tracker.EffArmPoll)
			wantArm(t, effs[0], domain.JobPollInterval)
			wantState(t, r.m, tracker.StateClosing)
		}
	})

	t.Run("CON-032/segundo CloseRequested em Closing não muda nada (closeAt e origem ficam)", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toClosing(t, r, t600)
		wantNoEffects(t, r.m.CloseRequested(at(601*time.Second)))
		wantState(t, r.m, tracker.StateClosing)
		wantClosingAt(t, r.m, t600, tracker.StateRunning)
	})

	t.Run("CON-032/Begin em Closing devolve false", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toClosing(t, r, t600)
		if r.m.Begin(99, "Other") {
			t.Fatal("Begin returned true in Closing")
		}
		wantState(t, r.m, tracker.StateClosing)
	})

	t.Run("CON-033/Stop em Closing: ended_at=closeAt, agent-shutdown, CloseJob e nenhum Notify", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toClosing(t, r, t600)
		effs := r.m.Stop(at(601 * time.Second))
		wantOrder(t, effs, tracker.EffEndSession, tracker.EffCloseJob)
		none(t, effs, tracker.EffNotify, tracker.EffTerminateJob)
		wantEnd(t, effs[0], domain.SessionEnded, t600, 600*time.Second, domain.EndAgentShutdown)
		wantState(t, r.m, tracker.StateIdle)

		p := newRunning(t)
		pausedBy(t, p, at(100*time.Second), domain.PauseHotkey)
		toClosing(t, p, at(130*time.Second))
		effs = p.m.Stop(at(131 * time.Second))
		wantOrder(t, effs, tracker.EffEndSession, tracker.EffCloseJob)
		wantEnd(t, effs[0], domain.SessionEnded, at(130*time.Second), 100*time.Second, domain.EndAgentShutdown)
		wantState(t, p.m, tracker.StateIdle)
	})

	t.Run("CON-005/depois do fim por fechamento, timers da sessão encerrada não emitem nada", func(t *testing.T) {
		t.Parallel()
		r := newRunning(t)
		toClosing(t, r, t600)
		r.m.CloseDone(at(602 * time.Second))
		now := at(700 * time.Second)
		wantNoEffects(t, r.m.Heartbeat(now, r.hbGen))
		wantNoEffects(t, r.m.Poll(now, r.pollGen, 0, nil))
		if _, ok := r.m.Snapshot(now); ok {
			t.Fatal("Snapshot ok after the session ended")
		}
	})
}
