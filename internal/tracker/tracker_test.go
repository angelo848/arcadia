package tracker_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// Waiter counts from contracts/session-state-machine.md (FakeClock After is not
// cancellable, so stale timers stay pending until they fire).
const (
	waitRunning       = 2 // heartbeat + periodic check
	waitGrace         = 3 // grace + check + the old heartbeat
	waitBackToRunning = 4 // after Grace -> Running: + stale grace timer + new heartbeat
)

var (
	errDisk   = errors.New("disk I/O error")
	errAccess = errors.New("access is denied")
	errResume = errors.New("resume thread failed")
	errRead   = errors.New("query job information failed")
)

// wantSession compares the fields of a snapshot that the tests fix.
func wantSession(t *testing.T, what string, got domain.SessionSnapshot, id int64, st domain.SessionState,
	active time.Duration, asOf time.Time) {
	t.Helper()
	if got.ID != id || got.GameID != testGameID || got.Title != testTitle {
		t.Errorf("%s: id/gameId/title = %d/%d/%q, want %d/%d/%q", what, got.ID, got.GameID, got.Title, id, testGameID, testTitle)
	}
	if got.State != st {
		t.Errorf("%s: state = %q, want %q", what, got.State, st)
	}
	if !got.StartedAt.Equal(baseT) {
		t.Errorf("%s: startedAt = %v, want %v", what, got.StartedAt, baseT)
	}
	if got.Active != active {
		t.Errorf("%s: active = %v, want %v", what, got.Active, active)
	}
	if !got.AsOf.Equal(asOf) {
		t.Errorf("%s: asOf = %v, want %v", what, got.AsOf, asOf)
	}
}

func wantUpdateCall(t *testing.T, c call, id int64, st domain.SessionState, lastSeen time.Time, active time.Duration) {
	t.Helper()
	if c.id != id || c.state != st || !c.at.Equal(lastSeen) || c.active != active {
		t.Errorf("UpdateSession(id=%d, %q, lastSeen=%v, active=%v), want (id=%d, %q, %v, %v)",
			c.id, c.state, c.at, c.active, id, st, lastSeen, active)
	}
}

func wantEndCall(t *testing.T, c call, id int64, endedAt time.Time, active time.Duration, reason domain.SessionEndReason) {
	t.Helper()
	e := c.end
	if e.ID != id || !e.EndedAt.Equal(endedAt) || e.Active != active || e.Reason != reason {
		t.Errorf("EndSession(id=%d, endedAt=%v, active=%v, reason=%q), want (id=%d, %v, %v, %q)",
			e.ID, e.EndedAt, e.Active, e.Reason, id, endedAt, active, reason)
	}
}

func wantNoCalls(t *testing.T, e *env, ops ...string) {
	t.Helper()
	for _, op := range ops {
		if n := e.rec.count(op); n != 0 {
			t.Errorf("%s called %d time(s), want 0; log %v", op, n, opNames(e.rec.all()))
		}
	}
}

func wantSeq(t *testing.T, e *env, want []string, ops ...string) {
	t.Helper()
	if got := e.rec.seq(ops...); !slices.Equal(got, want) {
		t.Errorf("call order = %v, want %v", got, want)
	}
}

func states(snaps []domain.SessionSnapshot) []domain.SessionState {
	out := make([]domain.SessionState, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, s.State)
	}
	return out
}

// toGraceAfter advances d from Running and empties the job; it returns the
// grace notification.
func toGraceAfter(t *testing.T, e *env, j *fakeJob, waiters int, d time.Duration) domain.SessionSnapshot {
	t.Helper()
	e.advance(t, waiters, d)
	j.empty(t)
	n := e.nextNotify(t)
	if n.State != domain.SessionGrace {
		t.Fatalf("notification after empty job: state %q, want grace", n.State)
	}
	return n
}

func TestTrackerLaunch(t *testing.T) {
	t.Parallel()

	t.Run("SES-001/StartSession acontece antes de Resume", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.launch(t)
		wantSeq(t, e, []string{opPrepare, opStart, opResume}, opPrepare, opStart, opResume)
		if p := e.rec.of(opPrepare); len(p) != 1 || p[0].spec != testSpec {
			t.Errorf("Prepare calls = %+v, want one with %+v", p, testSpec)
		}
		starts := e.rec.of(opStart)
		if len(starts) != 1 || starts[0].gameID != testGameID || !starts[0].at.Equal(baseT) {
			t.Errorf("StartSession calls = %+v, want one (game %d, at %v)", starts, testGameID, baseT)
		}
		wantNoCalls(t, e, opTerminate, opDelete, opEnd)
	})

	t.Run("SES-001/devolve running com startedAt T e activeSeconds 0", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, _ := e.launch(t)
		wantSession(t, "Launch", snap, firstSessionID, domain.SessionRunning, 0, baseT)
		if !snap.EndedAt.IsZero() {
			t.Errorf("Launch: endedAt = %v, want zero", snap.EndedAt)
		}
		cur := e.waitCurrentState(t, domain.SessionRunning)
		if cur.ID != firstSessionID {
			t.Errorf("Current id = %d, want %d", cur.ID, firstSessionID)
		}
	})

	t.Run("IPC-027/notifica running uma vez", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		e.barrier(t, j)
		ns := e.notifications()
		if len(ns) != 1 {
			t.Fatalf("notifications = %v, want exactly one", states(ns))
		}
		wantSession(t, "notification", ns[0], firstSessionID, domain.SessionRunning, 0, baseT)
	})

	t.Run("IPC-031/snapshot devolvido tem Active 0 e AsOf T", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, _ := e.launch(t)
		if snap.Active != 0 || !snap.AsOf.Equal(baseT) {
			t.Errorf("Launch: active=%v asOf=%v, want 0 and %v", snap.Active, snap.AsOf, baseT)
		}
	})
}

func TestTrackerSingleSession(t *testing.T) {
	t.Parallel()

	other := tracker.LaunchSpec{GameID: 9, Title: "Celeste", Exe: `C:\Games\Celeste\Celeste.exe`}

	wantActive := func(t *testing.T, err error, id int64) {
		t.Helper()
		if !errors.Is(err, tracker.ErrSessionActive) {
			t.Fatalf("Launch error = %v, want ErrSessionActive", err)
		}
		var sa *tracker.SessionActiveError
		if !errors.As(err, &sa) {
			t.Fatalf("Launch error = %T %v, want *SessionActiveError", err, err)
		}
		if sa.Session.ID != id || sa.Session.GameID != testGameID || sa.Session.Title != testTitle {
			t.Errorf("SessionActiveError session = %d/%d/%q, want %d/%d/%q",
				sa.Session.ID, sa.Session.GameID, sa.Session.Title, id, testGameID, testTitle)
		}
	}

	t.Run("SES-002/Running: segundo Launch não chama Prepare nem o store e devolve SessionActiveError com id, gameId e title", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, _ := e.launch(t)
		before := len(e.rec.all())
		_, err := e.tryLaunch(t, other)
		wantActive(t, err, snap.ID)
		if got := e.rec.all()[before:]; len(got) != 0 {
			t.Errorf("calls after the refused Launch = %v, want none", opNames(got))
		}
		if n := e.rec.count(opPrepare); n != 1 {
			t.Errorf("Prepare calls = %d, want 1", n)
		}
	})

	t.Run("SES-002/Grace: mesmo resultado", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		e.waitCurrentState(t, domain.SessionGrace)
		before := len(e.rec.all())
		_, err := e.tryLaunch(t, other)
		wantActive(t, err, snap.ID)
		if got := e.rec.all()[before:]; len(got) != 0 {
			t.Errorf("calls after the refused Launch = %v, want none", opNames(got))
		}
	})

	t.Run("SES-002/Launch concorrente durante um Prepare lento recebe SessionActiveError", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		entered, release := e.launcher.block(t)

		type result struct {
			snap domain.SessionSnapshot
			err  error
		}
		ctxA, ctxB := timeoutCtx(t), timeoutCtx(t)
		first := make(chan result, 1)
		go func() {
			s, err := e.tr.Launch(ctxA, testSpec)
			first <- result{s, err}
		}()
		select {
		case <-entered:
		case <-time.After(waitTimeout):
			t.Fatal("first Prepare never started")
		}
		second := make(chan result, 1)
		go func() {
			s, err := e.tr.Launch(ctxB, other)
			second <- result{s, err}
		}()
		release()

		var a, b result
		for range 2 {
			select {
			case a = <-first:
			case b = <-second:
			case <-time.After(waitTimeout):
				t.Fatal("Launch calls did not return")
			}
		}
		if a.err != nil {
			t.Fatalf("first Launch: %v", a.err)
		}
		// The contract accepts sessionId 0 (read in Launching) or the new id
		// (read in Running, the usual case).
		if !errors.Is(b.err, tracker.ErrSessionActive) {
			t.Fatalf("second Launch error = %v, want ErrSessionActive", b.err)
		}
		var sa *tracker.SessionActiveError
		if !errors.As(b.err, &sa) {
			t.Fatalf("second Launch error = %T, want *SessionActiveError", b.err)
		}
		if sa.Session.GameID != testGameID || sa.Session.Title != testTitle ||
			(sa.Session.ID != 0 && sa.Session.ID != a.snap.ID) {
			t.Errorf("SessionActiveError session = %+v, want game %d %q with id 0 or %d", sa.Session, testGameID, testTitle, a.snap.ID)
		}
		if n := e.rec.count(opPrepare); n != 1 {
			t.Errorf("Prepare calls = %d, want 1", n)
		}
		if n := e.rec.count(opStart); n != 1 {
			t.Errorf("StartSession calls = %d, want 1", n)
		}
	})

	t.Run("SES-002/segundo Launch durante um lançamento que depois falha é recusado na hora e não cria processo", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.store.failStart(errDisk, 1) // the first launch fails after Prepare
		entered, release := e.launcher.block(t)

		type result struct {
			snap domain.SessionSnapshot
			err  error
		}
		first := make(chan result, 1)
		ctxA := timeoutCtx(t)
		go func() {
			s, err := e.tr.Launch(ctxA, testSpec)
			first <- result{s, err}
		}()
		select {
		case <-entered:
		case <-time.After(waitTimeout):
			t.Fatal("first Prepare never started")
		}

		// The first launch is held in Prepare: the second one must be refused
		// without waiting for it.
		ctxB, cancelB := context.WithCancel(timeoutCtx(t))
		defer cancelB()
		second := make(chan result, 1)
		go func() {
			s, err := e.tr.Launch(ctxB, other)
			second <- result{s, err}
		}()
		var b result
		select {
		case b = <-second:
		case <-time.After(waitTimeout):
			t.Error("second Launch did not return while the first one was in progress")
			cancelB()
			b = <-second
		}
		release()

		var a result
		select {
		case a = <-first:
		case <-time.After(waitTimeout):
			t.Fatal("first Launch did not return")
		}
		var le *tracker.LaunchError
		if !errors.As(a.err, &le) {
			t.Errorf("first Launch error = %T %v, want *LaunchError", a.err, a.err)
		}
		if !errors.Is(b.err, tracker.ErrSessionActive) {
			t.Fatalf("second Launch error = %v, want ErrSessionActive", b.err)
		}
		var sa *tracker.SessionActiveError
		if !errors.As(b.err, &sa) {
			t.Fatalf("second Launch error = %T, want *SessionActiveError", b.err)
		}
		if sa.Session.ID != 0 || sa.Session.GameID != testGameID || sa.Session.Title != testTitle {
			t.Errorf("SessionActiveError session = %+v, want id 0 (Launching), game %d %q", sa.Session, testGameID, testTitle)
		}
		if n := e.rec.count(opPrepare); n != 1 {
			t.Errorf("Prepare calls = %d, want 1 (log %v)", n, opNames(e.rec.all()))
		}
		if n := e.rec.count(opStart); n != 1 {
			t.Errorf("StartSession calls = %d, want 1 (the failing one)", n)
		}
		wantNoCalls(t, e, opNotify, opUpdate, opEnd)
		e.waitNoCurrent(t)
	})

	t.Run("SES-002/a sessão existente não muda", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		e.advance(t, waitRunning, 10*time.Second)
		_, err := e.tryLaunch(t, other)
		wantActive(t, err, snap.ID)
		e.barrier(t, j)
		cur, ok := e.tr.Current()
		if !ok {
			t.Fatal("Current: no session after a refused Launch")
		}
		wantSession(t, "Current", cur, snap.ID, domain.SessionRunning, 10*time.Second, at(10*time.Second))
		if j.isClosed() {
			t.Error("job closed by a refused Launch")
		}
		if n := len(e.notifications()); n != 1 {
			t.Errorf("notifications = %d, want 1", n)
		}
		wantNoCalls(t, e, opUpdate, opEnd, opDelete, opTerminate)
	})
}

func TestTrackerGrace(t *testing.T) {
	t.Parallel()

	t.Run("SES-003/job vazio em T+100s grava grace, last_seen T+100s e 100 s, e notifica grace", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		n := toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		wantSession(t, "grace notification", n, snap.ID, domain.SessionGrace, 100*time.Second, at(100*time.Second))
		ups := e.rec.waitN(t, opUpdate, 1)
		wantUpdateCall(t, ups[0], snap.ID, domain.SessionGrace, at(100*time.Second), 100*time.Second)
		wantSeq(t, e, []string{opNotify, opUpdate, opNotify}, opUpdate, opNotify)
	})

	t.Run("SES-003/em Grace há 3 waiters", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envConfig{}) // domain defaults
		_, j := e.launch(t)
		e.waiters(t, waitRunning)
		j.empty(t)
		e.nextNotify(t)
		e.waiters(t, waitGrace)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if err := e.clock.BlockUntilWaiters(ctx, waitGrace+1); err == nil {
			t.Errorf("more than %d waiters pending in Grace", waitGrace)
		}
	})

	t.Run("SES-004/15 s − 1 ns não encerra", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		e.advance(t, waitGrace, domain.SessionGracePeriod-time.Nanosecond)
		e.barrier(t, j)
		wantNoCalls(t, e, opEnd)
		if j.isClosed() {
			t.Error("job closed before the grace period expired")
		}
		e.waitCurrentState(t, domain.SessionGrace)
	})

	t.Run("SES-004/15 s encerra com ended_at E e exited, fecha o job, Current vazio e aceita novo Launch", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		e.advance(t, waitGrace, domain.SessionGracePeriod)
		ends := e.rec.waitN(t, opEnd, 1)
		wantEndCall(t, ends[0], snap.ID, at(100*time.Second), 100*time.Second, domain.EndExited)
		e.rec.waitN(t, opJobClose, 1)
		if !j.isClosed() {
			t.Error("job not closed at the end of the session")
		}
		if n := e.nextNotify(t); n.State != domain.SessionEnded {
			t.Errorf("end notification state = %q, want ended", n.State)
		}
		e.waitNoCurrent(t)
		wantNoCalls(t, e, opTerminate)

		snap2, j2 := e.launch(t)
		if snap2.State != domain.SessionRunning || snap2.ID == snap.ID || j2 == j {
			t.Errorf("second Launch = %+v (job %p), want a new running session", snap2, j2)
		}
	})

	t.Run("IPC-031/notificação de fim tem AsOf no vencimento e EndedAt E", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		e.advance(t, waitGrace, domain.SessionGracePeriod)
		n := e.nextNotify(t)
		wantSession(t, "end notification", n, snap.ID, domain.SessionEnded, 100*time.Second, at(115*time.Second))
		if !n.EndedAt.Equal(at(100 * time.Second)) {
			t.Errorf("end notification endedAt = %v, want %v", n.EndedAt, at(100*time.Second))
		}
	})

	t.Run("SES-005/processo novo em E+5s volta a running, grava e notifica", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		e.advance(t, waitGrace, 5*time.Second)
		j.newProcess(t, 5000)
		n := e.nextNotify(t)
		wantSession(t, "running notification", n, snap.ID, domain.SessionRunning, 100*time.Second, at(105*time.Second))
		ups := e.rec.waitN(t, opUpdate, 2)
		wantUpdateCall(t, ups[1], snap.ID, domain.SessionRunning, at(105*time.Second), 100*time.Second)
		wantSeq(t, e, []string{opNotify, opUpdate, opNotify, opUpdate, opNotify}, opUpdate, opNotify)
	})

	t.Run("SES-005/mais 15 s não encerra", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		e.advance(t, waitGrace, 5*time.Second)
		j.newProcess(t, 5000)
		e.nextNotify(t)
		e.advance(t, waitBackToRunning, domain.SessionGracePeriod) // the stale grace timer fires
		e.barrier(t, j)
		wantNoCalls(t, e, opEnd)
		if j.isClosed() {
			t.Error("job closed by a stale grace timer")
		}
		cur := e.waitCurrentState(t, domain.SessionRunning)
		if cur.Active != 115*time.Second || !cur.AsOf.Equal(at(120*time.Second)) {
			t.Errorf("Current active=%v asOf=%v, want 115s at %v", cur.Active, cur.AsOf, at(120*time.Second))
		}
		if n := len(e.notifications()); n != 3 {
			t.Errorf("notifications = %v, want running, grace, running", states(e.notifications()))
		}
	})

	t.Run("SES-006/exemplo de 195 s", func(t *testing.T) {
		t.Parallel()
		st := openTestStore(t)
		g := addTestGame(t, st)
		e := newEnv(t, envConfig{heartbeat: time.Hour, poll: time.Hour, real: st})
		spec := tracker.LaunchSpec{GameID: g.ID, Title: g.Title, Exe: g.LaunchTarget}
		snap, j := e.launchSpec(t, spec)

		e.advance(t, waitRunning, 100*time.Second) // T+100s
		j.empty(t)
		e.nextNotify(t)
		e.advance(t, waitGrace, 5*time.Second) // T+105s
		j.newProcess(t, 5000)
		e.nextNotify(t)
		e.advance(t, waitBackToRunning, 95*time.Second) // T+200s
		j.empty(t)
		if n := e.nextNotify(t); n.State != domain.SessionGrace || n.Active != 195*time.Second {
			t.Errorf("second grace notification = %q/%v, want grace/195s", n.State, n.Active)
		}
		e.advance(t, waitBackToRunning, domain.SessionGracePeriod) // T+215s

		ends := e.rec.waitN(t, opEnd, 1)
		if ends[0].err != nil {
			t.Fatalf("EndSession: %v", ends[0].err)
		}
		if ends[0].end.ID != snap.ID || !ends[0].end.EndedAt.Equal(at(200*time.Second)) ||
			ends[0].end.Active != 195*time.Second || ends[0].end.Reason != domain.EndExited {
			t.Errorf("EndSession = %+v, want id %d, endedAt T+200s, 195s, exited", ends[0].end, snap.ID)
		}
		if ends[0].state != domain.SessionEnded {
			t.Errorf("final state = %q, want ended", ends[0].state)
		}
		if n := e.nextNotify(t); n.State != domain.SessionEnded || n.Active != 195*time.Second {
			t.Errorf("end notification = %q/%v, want ended/195s", n.State, n.Active)
		}
		got, err := st.Game(t.Context(), g.ID)
		if err != nil {
			t.Fatalf("Game: %v", err)
		}
		if got.TotalPlaySeconds != 195 || got.PlayCount != 1 {
			t.Errorf("game total/count = %d/%d, want 195/1", got.TotalPlaySeconds, got.PlayCount)
		}
	})

	t.Run("SES-007/29 s termina discarded e notifica discarded", func(t *testing.T) {
		t.Parallel()
		st := openTestStore(t)
		g := addTestGame(t, st)
		e := newEnv(t, envConfig{heartbeat: time.Hour, poll: time.Hour, real: st})
		spec := tracker.LaunchSpec{GameID: g.ID, Title: g.Title, Exe: g.LaunchTarget}
		snap, j := e.launchSpec(t, spec)
		e.advance(t, waitRunning, 29*time.Second)
		j.empty(t)
		e.nextNotify(t)
		e.advance(t, waitGrace, domain.SessionGracePeriod)
		ends := e.rec.waitN(t, opEnd, 1)
		if ends[0].end.ID != snap.ID || ends[0].end.Active != 29*time.Second || ends[0].end.Reason != domain.EndExited {
			t.Errorf("EndSession = %+v, want id %d, 29s, exited", ends[0].end, snap.ID)
		}
		if ends[0].err != nil || ends[0].state != domain.SessionDiscarded {
			t.Errorf("final state = %q (err %v), want discarded", ends[0].state, ends[0].err)
		}
		n := e.nextNotify(t)
		if n.State != domain.SessionDiscarded || n.Active != 29*time.Second {
			t.Errorf("end notification = %q/%v, want discarded/29s", n.State, n.Active)
		}
		got, err := st.Game(t.Context(), g.ID)
		if err != nil {
			t.Fatalf("Game: %v", err)
		}
		if got.TotalPlaySeconds != 0 || got.PlayCount != 0 || !got.LastPlayedAt.IsZero() {
			t.Errorf("game after discarded session = %d/%d/%v, want unchanged", got.TotalPlaySeconds, got.PlayCount, got.LastPlayedAt)
		}
	})

	t.Run("IPC-027/notificações saem na ordem das transições, uma por transição", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		e.advance(t, waitRunning, 100*time.Second)
		j.empty(t)
		e.nextNotify(t)
		e.advance(t, waitGrace, 5*time.Second)
		j.newProcess(t, 5000)
		e.nextNotify(t)
		e.advance(t, waitBackToRunning, 95*time.Second)
		j.empty(t)
		e.nextNotify(t)
		e.advance(t, waitBackToRunning, domain.SessionGracePeriod)
		e.nextNotify(t)
		if err := e.close(t); err != nil {
			t.Fatalf("Close: %v", err)
		}
		want := []domain.SessionState{domain.SessionRunning, domain.SessionGrace, domain.SessionRunning, domain.SessionGrace, domain.SessionEnded}
		if got := states(e.notifications()); !slices.Equal(got, want) {
			t.Errorf("notification states = %v, want %v", got, want)
		}
		// Each notification follows the write of its transition.
		wantSeq(t, e, []string{
			opNotify,
			opUpdate, opNotify,
			opUpdate, opNotify,
			opUpdate, opNotify,
			opEnd, opNotify,
		}, opUpdate, opEnd, opNotify)
	})
}

func TestTrackerHeartbeat(t *testing.T) {
	t.Parallel()
	cfg := envConfig{poll: time.Hour} // default 30 s heartbeat

	t.Run("SES-009/T+30s grava 30 e T+60s grava 60", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, cfg)
		snap, _ := e.launch(t)
		e.advance(t, waitRunning, 30*time.Second)
		ups := e.rec.waitN(t, opUpdate, 1)
		wantUpdateCall(t, ups[0], snap.ID, domain.SessionRunning, at(30*time.Second), 30*time.Second)
		e.advance(t, waitRunning, 30*time.Second)
		ups = e.rec.waitN(t, opUpdate, 2)
		wantUpdateCall(t, ups[1], snap.ID, domain.SessionRunning, at(60*time.Second), 60*time.Second)
	})

	t.Run("SES-009/30 s − 1 ns não grava", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, cfg)
		snap, j := e.launch(t)
		e.advance(t, waitRunning, domain.SessionHeartbeatInterval-time.Nanosecond)
		e.barrier(t, j)
		wantNoCalls(t, e, opUpdate)
		e.clock.Advance(time.Nanosecond)
		ups := e.rec.waitN(t, opUpdate, 1)
		wantUpdateCall(t, ups[0], snap.ID, domain.SessionRunning, at(30*time.Second), 30*time.Second)
	})

	t.Run("SES-009/em Grace não grava", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envConfig{poll: time.Hour, grace: time.Hour})
		_, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 10*time.Second)
		e.rec.waitN(t, opUpdate, 1) // the grace write
		e.advance(t, waitGrace, 30*time.Second)
		e.barrier(t, j)
		ups := e.rec.of(opUpdate)
		if len(ups) != 1 || ups[0].state != domain.SessionGrace {
			t.Errorf("updates in Grace = %+v, want only the grace write", ups)
		}
	})
}

func TestTrackerPoll(t *testing.T) {
	t.Parallel()
	cfg := envConfig{heartbeat: time.Hour} // default 5 s check and 15 s grace

	t.Run("SES-013/Running com 0 ativos e sem mensagem vai a Grace no tick", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, cfg)
		snap, j := e.launch(t)
		j.setActive(0, nil) // no ACTIVE_PROCESS_ZERO message
		e.advance(t, waitRunning, domain.JobPollInterval)
		n := e.nextNotify(t)
		wantSession(t, "grace notification", n, snap.ID, domain.SessionGrace, 5*time.Second, at(5*time.Second))
		ups := e.rec.waitN(t, opUpdate, 1)
		wantUpdateCall(t, ups[0], snap.ID, domain.SessionGrace, at(5*time.Second), 5*time.Second)
		if e.rec.count(opActive) == 0 {
			t.Error("ActiveProcesses never read")
		}
	})

	t.Run("SES-013/Grace com > 0 volta a Running", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, cfg)
		snap, j := e.launch(t)
		e.waiters(t, waitRunning)
		j.empty(t) // Grace at T
		e.nextNotify(t)
		j.setActive(2, nil) // no NEW_PROCESS message
		e.advance(t, waitGrace, domain.JobPollInterval)
		n := e.nextNotify(t)
		wantSession(t, "running notification", n, snap.ID, domain.SessionRunning, 0, at(5*time.Second))
		ups := e.rec.waitN(t, opUpdate, 2)
		wantUpdateCall(t, ups[1], snap.ID, domain.SessionRunning, at(5*time.Second), 0)
	})

	// The owner is held in a heartbeat write while the check fires at T+15s and
	// ACTIVE_PROCESS_ZERO arrives at T+20s. Go's select then picks either one
	// first; when the empty job is applied first (Grace at T+20s), the late check
	// (fired at T+15s, 1 active process) must not resume at T+15s. Each round is
	// an invariant check (never a false failure); 20 rounds make the bad order
	// almost certain to be exercised (2^-20 to miss it).
	t.Run("SES-013/verificação processada atrasada não faz last_seen nem graceSince voltarem no tempo", func(t *testing.T) {
		t.Parallel()
		const rounds = 20
		lateResumes := 0
		for i := range rounds {
			t.Run(fmt.Sprintf("rodada-%02d", i), func(t *testing.T) {
				e := newEnv(t, envConfig{heartbeat: 10 * time.Second, poll: 15 * time.Second, grace: time.Hour})
				snap, j := e.launch(t)
				g := e.store.blockNextUpdate(t)
				e.advance(t, waitRunning, 10*time.Second) // heartbeat at T+10s
				g.waitEntered(t, "heartbeat UpdateSession")
				e.clock.Advance(5 * time.Second)                           // check fires at T+15s, owner busy
				e.clock.Advance(5 * time.Second)                           // T+20s
				j.send(t, tracker.JobEvent{Kind: tracker.EventActiveZero}) // active stays 1
				g.release()
				e.rec.waitN(t, opActive, 1)
				e.barrier(t, j)

				ups := e.rec.of(opUpdate)
				if len(ups) == 0 {
					t.Fatal("no UpdateSession call")
				}
				wantUpdateCall(t, ups[0], snap.ID, domain.SessionRunning, at(10*time.Second), 10*time.Second)
				for k := 1; k < len(ups); k++ {
					if ups[k].at.Before(ups[k-1].at) {
						t.Errorf("UpdateSession #%d last_seen %v is before #%d %v (log %v)",
							k, ups[k].at, k-1, ups[k-1].at, opNames(e.rec.all()))
					}
					if ups[k].active < ups[k-1].active {
						t.Errorf("UpdateSession #%d active %v is below #%d %v", k, ups[k].active, k-1, ups[k-1].active)
					}
				}
				last := ups[len(ups)-1]
				if !last.at.Equal(at(20*time.Second)) || last.active != 20*time.Second {
					t.Errorf("last UpdateSession = (%q, %v, %v), want last_seen %v and active 20s",
						last.state, last.at, last.active, at(20*time.Second))
				}
				ns := e.notifications()
				for k := 1; k < len(ns); k++ {
					if ns[k].AsOf.Before(ns[k-1].AsOf) {
						t.Errorf("notification %d (%q) AsOf %v is before notification %d (%q) %v",
							k, ns[k].State, ns[k].AsOf, k-1, ns[k-1].State, ns[k-1].AsOf)
					}
				}
				if len(ups) == 3 && ups[1].state == domain.SessionGrace && ups[2].state == domain.SessionRunning {
					lateResumes++
					// Resumed from Grace at T+20s: no active time is added at that instant.
					cur := e.waitCurrentState(t, domain.SessionRunning)
					if cur.Active != 20*time.Second {
						t.Errorf("Current active after the late resume = %v, want 20s", cur.Active)
					}
				}
			})
		}
		t.Logf("late check applied after the empty job in %d of %d rounds", lateResumes, rounds)
	})

	t.Run("SES-013/erro de leitura loga Warn e não muda o estado", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, cfg)
		snap, j := e.launch(t)
		j.setActive(0, errRead)
		e.advance(t, waitRunning, domain.JobPollInterval)
		e.logs.wait(t, slog.LevelWarn, "", nil)
		e.waiters(t, waitRunning) // the check is re-armed
		e.barrier(t, j)
		cur := e.waitCurrentState(t, domain.SessionRunning)
		if cur.ID != snap.ID {
			t.Errorf("Current id = %d, want %d", cur.ID, snap.ID)
		}
		if n := len(e.notifications()); n != 1 {
			t.Errorf("notifications = %v, want only running", states(e.notifications()))
		}
		wantNoCalls(t, e, opUpdate, opEnd)
	})
}

func TestTrackerLaunchFailure(t *testing.T) {
	t.Parallel()

	wantLaunchErr := func(t *testing.T, err, cause error, winErr uint32) {
		t.Helper()
		var le *tracker.LaunchError
		if !errors.As(err, &le) {
			t.Fatalf("Launch error = %T %v, want *LaunchError", err, err)
		}
		if le.WinError != winErr {
			t.Errorf("LaunchError.WinError = %d, want %d", le.WinError, winErr)
		}
		if !errors.Is(err, cause) {
			t.Errorf("Launch error %v does not wrap %v", err, cause)
		}
	}
	wantFailLog := func(t *testing.T, e *env, text string) {
		t.Helper()
		rec := e.logs.wait(t, slog.LevelError, msgLaunchFailed, nil)
		wantAttrs(t, rec, map[string]string{"game_id": fmt.Sprint(testGameID), "path": testSpec.Exe})
		wantAttrContains(t, rec, "error", text)
	}

	t.Run("SES-012/Prepare falha: nada gravado, sem notificação, Idle, Error com game_id, path e error, LaunchError com WinError", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.launcher.failPrepare(&tracker.LaunchError{Err: errAccess, WinError: 5}, 1)
		_, err := e.tryLaunch(t, testSpec)
		wantLaunchErr(t, err, errAccess, 5)
		wantFailLog(t, e, errAccess.Error())
		wantNoCalls(t, e, opStart, opUpdate, opEnd, opDelete, opNotify, opResume, opTerminate)
		if _, ok := e.tr.Current(); ok {
			t.Error("Current reports a session after a failed Prepare")
		}
	})

	t.Run("SES-012/StartSession falha: Terminate e Close do job, LaunchError com WinError 0", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.store.failStart(errDisk, 1)
		_, err := e.tryLaunch(t, testSpec)
		wantLaunchErr(t, err, errDisk, 0)
		wantSeq(t, e, []string{opTerminate, opJobClose}, opResume, opTerminate, opJobClose)
		if !e.launcher.job(t, 0).isClosed() {
			t.Error("job not closed")
		}
		wantFailLog(t, e, errDisk.Error())
		wantNoCalls(t, e, opNotify, opUpdate, opEnd)
		if _, ok := e.tr.Current(); ok {
			t.Error("Current reports a session after a failed StartSession")
		}
	})

	t.Run("SES-012/Resume falha: Terminate, Close e DeleteSession", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.launcher.failNextResume(&tracker.LaunchError{Err: errResume, WinError: 5})
		_, err := e.tryLaunch(t, testSpec)
		wantLaunchErr(t, err, errResume, 5)
		wantSeq(t, e, []string{opResume, opTerminate, opJobClose}, opResume, opTerminate, opJobClose)
		dels := e.rec.of(opDelete)
		if len(dels) != 1 || dels[0].id != firstSessionID {
			t.Errorf("DeleteSession calls = %+v, want one for id %d", dels, firstSessionID)
		}
		wantFailLog(t, e, errResume.Error())
		wantNoCalls(t, e, opNotify, opUpdate, opEnd)
		if _, ok := e.tr.Current(); ok {
			t.Error("Current reports a session after a failed Resume")
		}
	})

	t.Run("SES-012/depois da falha um novo Launch é aceito", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.launcher.failPrepare(&tracker.LaunchError{Err: errAccess, WinError: 5}, 1)
		if _, err := e.tryLaunch(t, testSpec); err == nil {
			t.Fatal("first Launch succeeded, want failure")
		}
		snap, _ := e.launch(t)
		if snap.State != domain.SessionRunning {
			t.Errorf("second Launch state = %q, want running", snap.State)
		}
		if n := e.rec.count(opPrepare); n != 2 {
			t.Errorf("Prepare calls = %d, want 2", n)
		}
	})
}

func TestTrackerWriteFailure(t *testing.T) {
	t.Parallel()

	wantWriteLog := func(t *testing.T, e *env, id int64) {
		t.Helper()
		rec := e.logs.wait(t, slog.LevelError, msgWriteFailed, nil)
		wantAttrs(t, rec, map[string]string{"session_id": fmt.Sprint(id)})
		wantAttrContains(t, rec, "error", errDisk.Error())
	}

	t.Run("SES-014/heartbeat que falha loga Error com session_id e continua Running", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envConfig{poll: time.Hour})
		e.store.failUpdate(errDisk, -1)
		snap, _ := e.launch(t)
		e.advance(t, waitRunning, 30*time.Second)
		wantWriteLog(t, e, snap.ID)
		e.waiters(t, waitRunning) // heartbeat re-armed
		cur := e.waitCurrentState(t, domain.SessionRunning)
		if cur.ID != snap.ID {
			t.Errorf("Current id = %d, want %d", cur.ID, snap.ID)
		}
	})

	t.Run("SES-014/entrada em Grace que falha continua em Grace", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.store.failUpdate(errDisk, -1)
		snap, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		wantWriteLog(t, e, snap.ID)
		cur := e.waitCurrentState(t, domain.SessionGrace)
		if cur.Active != 100*time.Second {
			t.Errorf("Current active = %v, want 100s", cur.Active)
		}
	})

	t.Run("SES-014/próxima escrita tenta de novo", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envConfig{poll: time.Hour})
		e.store.failUpdate(errDisk, 1)
		snap, _ := e.launch(t)
		e.advance(t, waitRunning, 30*time.Second)
		wantWriteLog(t, e, snap.ID)
		e.advance(t, waitRunning, 30*time.Second)
		ups := e.rec.waitN(t, opUpdate, 2)
		if ups[1].err != nil {
			t.Errorf("second write failed: %v", ups[1].err)
		}
		wantUpdateCall(t, ups[1], snap.ID, domain.SessionRunning, at(60*time.Second), 60*time.Second)
	})

	t.Run("SES-014/fim que falha loga Error, fecha o job e volta a Idle", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.store.failEnd(errDisk, -1)
		snap, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		e.advance(t, waitGrace, domain.SessionGracePeriod)
		wantWriteLog(t, e, snap.ID)
		e.rec.waitN(t, opJobClose, 1)
		if !j.isClosed() {
			t.Error("job not closed after a failed end write")
		}
		e.waitNoCurrent(t)
		// design.md (Coordenador): Notify follows the write "com ou sem erro de escrita".
		if n := e.nextNotify(t); n.State != domain.SessionEnded || n.ID != snap.ID {
			t.Errorf("notify after a failed end = id %d state %q, want id %d ended", n.ID, n.State, snap.ID)
		}
		snap2, _ := e.launch(t)
		if snap2.State != domain.SessionRunning {
			t.Errorf("Launch after a failed end = %q, want running", snap2.State)
		}
	})
}

func TestTrackerRootExit(t *testing.T) {
	t.Parallel()

	t.Run("SES-015/saída do processo inicial loga Info com session_id, pid e exit_code", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		j.send(t, tracker.JobEvent{Kind: tracker.EventExitProcess, PID: j.RootPID(), Root: true, ExitCode: 3})
		rec := e.logs.wait(t, slog.LevelInfo, msgRootExited, nil)
		wantAttrs(t, rec, map[string]string{
			"session_id": fmt.Sprint(snap.ID),
			"pid":        fmt.Sprint(j.RootPID()),
			"exit_code":  "3",
		})
	})

	t.Run("SES-015/o estado não muda", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		j.send(t, tracker.JobEvent{Kind: tracker.EventExitProcess, PID: j.RootPID(), Root: true, ExitCode: 3})
		e.logs.wait(t, slog.LevelInfo, msgRootExited, nil)
		cur := e.waitCurrentState(t, domain.SessionRunning)
		if cur.ID != snap.ID {
			t.Errorf("Current id = %d, want %d", cur.ID, snap.ID)
		}
		if j.isClosed() {
			t.Error("job closed on the initial process exit")
		}
		if n := len(e.notifications()); n != 1 {
			t.Errorf("notifications = %v, want only running", states(e.notifications()))
		}
		wantNoCalls(t, e, opUpdate, opEnd, opTerminate)
	})

	t.Run("SES-015/em Grace também loga e continua em Grace", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		j.send(t, tracker.JobEvent{Kind: tracker.EventExitProcess, PID: j.RootPID(), Root: true, ExitCode: 7})
		rec := e.logs.wait(t, slog.LevelInfo, msgRootExited, nil)
		wantAttrs(t, rec, map[string]string{"exit_code": "7"})
		e.waitCurrentState(t, domain.SessionGrace)
		wantNoCalls(t, e, opEnd)
	})
}

func TestTrackerCurrent(t *testing.T) {
	t.Parallel()

	t.Run("IPC-026/sem sessão devolve false", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		if snap, ok := e.tr.Current(); ok {
			t.Errorf("Current = %+v, true; want false", snap)
		}
	})

	t.Run("IPC-031/Running desde T e consulta em T+252,4s: Active 252,4 s e AsOf T+252,4s", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, _ := e.launch(t)
		e.waitCurrentState(t, domain.SessionRunning)
		e.advance(t, waitRunning, 252400*time.Millisecond)
		cur, ok := e.tr.Current()
		if !ok {
			t.Fatal("Current: no session")
		}
		wantSession(t, "Current", cur, snap.ID, domain.SessionRunning, 252400*time.Millisecond, at(252400*time.Millisecond))
	})

	t.Run("IPC-031/Grace desde T+300s e consulta em T+310s: Active 300 s e AsOf T+310s", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 300*time.Second)
		e.waitCurrentState(t, domain.SessionGrace)
		e.advance(t, waitGrace, 10*time.Second)
		cur, ok := e.tr.Current()
		if !ok {
			t.Fatal("Current: no session")
		}
		wantSession(t, "Current", cur, snap.ID, domain.SessionGrace, 300*time.Second, at(310*time.Second))
	})
}

func TestTrackerStop(t *testing.T) {
	t.Parallel()

	t.Run("SES-011/Running: Close em T encerra com ended_at T, agent-shutdown, sem Notify nem Terminate", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		e.advance(t, waitRunning, 40*time.Second)
		if err := e.close(t); err != nil {
			t.Fatalf("Close: %v", err)
		}
		ends := e.rec.of(opEnd)
		if len(ends) != 1 {
			t.Fatalf("EndSession calls = %d, want 1", len(ends))
		}
		wantEndCall(t, ends[0], snap.ID, at(40*time.Second), 40*time.Second, domain.EndAgentShutdown)
		if !j.isClosed() {
			t.Error("job not closed by Close")
		}
		wantNoCalls(t, e, opTerminate)
		if n := len(e.notifications()); n != 1 {
			t.Errorf("notifications = %v, want only the running one", states(e.notifications()))
		}
	})

	t.Run("SES-011/Grace: Close encerra com ended_at E e exited", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		e.advance(t, waitGrace, 10*time.Second)
		if err := e.close(t); err != nil {
			t.Fatalf("Close: %v", err)
		}
		ends := e.rec.of(opEnd)
		if len(ends) != 1 {
			t.Fatalf("EndSession calls = %d, want 1", len(ends))
		}
		wantEndCall(t, ends[0], snap.ID, at(100*time.Second), 100*time.Second, domain.EndExited)
		if !j.isClosed() {
			t.Error("job not closed by Close")
		}
		wantNoCalls(t, e, opTerminate)
		if n := len(e.notifications()); n != 2 {
			t.Errorf("notifications = %v, want running and grace only", states(e.notifications()))
		}
	})

	t.Run("SES-011/Close durante um Prepare lento espera o lançamento e encerra como Running", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		entered, release := e.launcher.block(t)
		type result struct {
			snap domain.SessionSnapshot
			err  error
		}
		launchCtx, closeCtx := timeoutCtx(t), timeoutCtx(t)
		launched := make(chan result, 1)
		go func() {
			s, err := e.tr.Launch(launchCtx, testSpec)
			launched <- result{s, err}
		}()
		select {
		case <-entered:
		case <-time.After(waitTimeout):
			t.Fatal("Prepare never started")
		}
		closed := make(chan error, 1)
		go func() { closed <- e.tr.Close(closeCtx) }()
		// Wait until Close has started the shutdown: from then on every Launch
		// is refused with ErrShuttingDown [AGT-011]. A cancelled context keeps
		// each probe from waiting on the busy owner goroutine.
		probeCtx, cancelProbe := context.WithCancel(context.Background())
		cancelProbe()
		probe := tracker.LaunchSpec{GameID: 9, Title: "Celeste", Exe: `C:\Games\Celeste\Celeste.exe`}
		eventually(t, "Close to start the shutdown", func() bool {
			_, err := e.tr.Launch(probeCtx, probe)
			return errors.Is(err, tracker.ErrShuttingDown)
		})
		// The owner goroutine is still held in Prepare, so Close cannot be done.
		select {
		case err := <-closed:
			t.Fatalf("Close returned (%v) while Prepare was still running", err)
		default:
		}
		release()

		var r result
		select {
		case r = <-launched:
		case <-time.After(waitTimeout):
			t.Fatal("Launch did not return")
		}
		select {
		case err := <-closed:
			if err != nil {
				t.Errorf("Close: %v", err)
			}
		case <-time.After(waitTimeout):
			t.Fatal("Close did not return")
		}
		if r.err != nil {
			t.Fatalf("Launch during Close: %v", r.err)
		}
		if r.snap.State != domain.SessionRunning {
			t.Errorf("Launch state = %q, want running", r.snap.State)
		}
		ends := e.rec.of(opEnd)
		if len(ends) != 1 {
			t.Fatalf("EndSession calls = %d, want 1 (log %v)", len(ends), opNames(e.rec.all()))
		}
		wantEndCall(t, ends[0], r.snap.ID, baseT, 0, domain.EndAgentShutdown)
		wantSeq(t, e, []string{opStart, opResume, opEnd, opJobClose}, opStart, opResume, opEnd, opJobClose)
		wantNoCalls(t, e, opTerminate, opDelete)
	})
}

func TestTrackerShuttingDown(t *testing.T) {
	t.Parallel()

	t.Run("AGT-011/depois de BeginShutdown, Launch devolve ErrShuttingDown sem Prepare", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.tr.BeginShutdown()
		_, err := e.tryLaunch(t, testSpec)
		if !errors.Is(err, tracker.ErrShuttingDown) {
			t.Fatalf("Launch error = %v, want ErrShuttingDown", err)
		}
		wantNoCalls(t, e, opPrepare, opStart, opNotify)
	})

	t.Run("AGT-011/depois de Close, Launch devolve ErrShuttingDown", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		if err := e.close(t); err != nil {
			t.Fatalf("Close: %v", err)
		}
		_, err := e.tryLaunch(t, testSpec)
		if !errors.Is(err, tracker.ErrShuttingDown) {
			t.Fatalf("Launch error = %v, want ErrShuttingDown", err)
		}
		wantNoCalls(t, e, opPrepare, opStart, opNotify)
	})
}

// closeHeldBy starts Close in a goroutine owned by the test, waits until the
// owner goroutine is held at g (reached during the shutdown) and checks that
// Close has not returned; then it releases g and returns Close's result.
func closeHeldBy(t *testing.T, e *env, g *gate, what string) error {
	t.Helper()
	ctx := timeoutCtx(t)
	closed := make(chan error, 1)
	go func() { closed <- e.tr.Close(ctx) }()
	g.waitEntered(t, what)
	// The owner goroutine is blocked in the fake right now, so a Close that
	// waits for it cannot have returned.
	select {
	case err := <-closed:
		g.release()
		t.Fatalf("Close returned (%v) while %s was still running in the owner goroutine", err, what)
	default:
	}
	g.release()
	select {
	case err := <-closed:
		return err
	case <-time.After(waitTimeout):
		t.Fatal("Close did not return after the release")
		return nil
	}
}

// TestTrackerClose is not parallel: goleak inspects every goroutine of the
// process.
func TestTrackerClose(t *testing.T) {
	t.Run("TRK-008/Close sem sessão espera a goroutine", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		e := newEnv(t, quiet)
		if err := e.close(t); err != nil {
			t.Fatalf("Close: %v", err)
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("TRK-008/Close com sessão espera o EndSession da goroutine dona", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		g := e.store.blockNextEnd(t)
		if err := closeHeldBy(t, e, g, "EndSession"); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if n := e.rec.count(opEnd); n != 1 {
			t.Errorf("EndSession calls = %d, want 1", n)
		}
		if !j.isClosed() {
			t.Error("job not closed by Close")
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("TRK-008/Close com sessão espera o Job.Close da goroutine dona", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		g := j.blockNextClose(t)
		if err := closeHeldBy(t, e, g, "Job.Close"); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if !j.isClosed() {
			t.Error("job not closed by Close")
		}
		wantSeq(t, e, []string{opEnd, opJobClose}, opEnd, opJobClose)
		goleak.VerifyNone(t, ignore)
	})

	t.Run("TRK-008/Close com ctx vencido cancela a escrita pendente e ainda espera a goroutine dona", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		h := e.store.holdNextEndUntilCancel(t)

		// The Close ctx is cancelled by the test (deterministic "expiry") once
		// the owner is held in EndSession, which only returns when its ctx ends.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		closed := make(chan error, 1)
		go func() { closed <- e.tr.Close(ctx) }()
		h.waitEntered(t, "EndSession")
		select {
		case err := <-closed:
			h.release()
			t.Fatalf("Close returned (%v) while EndSession was still running in the owner goroutine", err)
		default:
		}
		cancel()

		var err error
		select {
		case err = <-closed:
		case <-time.After(waitTimeout):
			h.release()
			t.Fatal("Close did not return after its ctx ended: it must cancel the pending EndSession")
		}
		select {
		case <-h.returned:
		default:
			t.Error("Close returned before the owner's EndSession returned")
		}
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Errorf("Close error = %v, want the ctx error (context.Canceled)", err)
		}
		ends := e.rec.of(opEnd)
		if len(ends) != 1 || !errors.Is(ends[0].err, context.Canceled) {
			t.Errorf("EndSession calls = %+v, want 1 ended by the cancellation", ends)
		}
		if !j.isClosed() {
			t.Error("job not closed by the owner after the failed EndSession")
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("TRK-008/segundo Close não faz nada", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		e := newEnv(t, quiet)
		e.launch(t)
		if err := e.close(t); err != nil {
			t.Fatalf("first Close: %v", err)
		}
		before := len(e.rec.all())
		if err := e.close(t); err != nil {
			t.Errorf("second Close: %v", err)
		}
		if got := e.rec.all()[before:]; len(got) != 0 {
			t.Errorf("second Close made calls %v, want none", opNames(got))
		}
		if n := e.rec.count(opEnd); n != 1 {
			t.Errorf("EndSession calls = %d, want 1", n)
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("TRK-008/Close sem Start retorna", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		e := newEnv(t, envConfig{heartbeat: time.Hour, poll: time.Hour, noStart: true})
		done := make(chan error, 1)
		ctx := timeoutCtx(t)
		go func() { done <- e.tr.Close(ctx) }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Close without Start: %v", err)
			}
		case <-time.After(waitTimeout):
			t.Fatal("Close without Start did not return")
		}
		goleak.VerifyNone(t, ignore)
	})
}

// Compile-time checks against the T08 signatures.
var (
	_ tracker.SessionStore    = (*store.Store)(nil)
	_ tracker.RecoveryStore   = (*store.Store)(nil)
	_ error                   = (*tracker.SessionActiveError)(nil)
	_ func() tracker.Launcher = tracker.NewLauncher
)
