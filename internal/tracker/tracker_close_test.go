package tracker_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// Contract: contracts/session-close-state-machine.md. Closing adds one pending
// waiter (CloseTimeout) to the ones of the origin state (FakeClock After is not
// cancellable). Assumed API (T11):
//
//	func (t *Tracker) CloseByHotkey(ctx context.Context, at time.Time) (CloseResult, error)
//	type CloseResult struct{ Kind CloseKind; GameID int64; Untracked bool; Reason string }
const (
	waitClosing = waitRunning + 1
	closeWait   = 3 * time.Second

	// detachedPollEvery mirrors x-console.closeDetachedPollMs (CON-030).
	detachedPollEvery = 250 * time.Millisecond
)

var errKill = errors.New("access is denied (kill)")

type closeOut struct {
	res tracker.CloseResult
	err error
}

// closeAsync calls CloseByHotkey in a goroutine owned by the test; the call
// only returns when the close is over (the owner goroutine never blocks).
func closeAsync(t *testing.T, e *env, when time.Time) <-chan closeOut {
	t.Helper()
	ctx := timeoutCtx(t)
	ch := make(chan closeOut, 1)
	go func() {
		res, err := e.tr.CloseByHotkey(ctx, when)
		ch <- closeOut{res, err}
	}()
	return ch
}

func waitClose(t *testing.T, ch <-chan closeOut) closeOut {
	t.Helper()
	select {
	case o := <-ch:
		return o
	case <-time.After(waitTimeout):
		t.Fatal("CloseByHotkey did not return")
		return closeOut{}
	}
}

// noClose asserts the call did not return yet (short real-time window; the
// behaviour itself is driven by the FakeClock).
func noClose(t *testing.T, ch <-chan closeOut, what string) {
	t.Helper()
	select {
	case o := <-ch:
		t.Fatalf("%s: CloseByHotkey returned early: %+v", what, o)
	case <-time.After(30 * time.Millisecond):
	}
}

func wantClosed(t *testing.T, o closeOut) {
	t.Helper()
	if o.err != nil || o.res.Kind != tracker.CloseClosed || o.res.GameID != testGameID || o.res.Untracked {
		t.Fatalf("CloseByHotkey = %+v, %v; want closed{gameId=%d}", o.res, o.err, testGameID)
	}
}

func wantRefused(t *testing.T, o closeOut, reason string) {
	t.Helper()
	if o.err != nil || o.res.Kind != tracker.CloseRefused || o.res.Reason != reason {
		t.Fatalf("CloseByHotkey = %+v, %v; want refused{%s}", o.res, o.err, reason)
	}
}

func wantFailed(t *testing.T, o closeOut) {
	t.Helper()
	if o.err != nil || o.res.Kind != tracker.CloseFailed {
		t.Fatalf("CloseByHotkey = %+v, %v; want failed", o.res, o.err)
	}
}

// notifyAfter returns the next notification beyond the first `seen`.
func notifyAfter(t *testing.T, e *env, seen int) domain.SessionSnapshot {
	t.Helper()
	return e.rec.waitN(t, opNotify, seen+1)[seen].snap
}

func TestTrackerCloseByHotkey(t *testing.T) {
	t.Parallel()

	t.Run("CON-005/Running: a árvore sai em 1 s durante a espera, sem Kill, ended em T+600s com closed-by-hotkey", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		j.setCloseResult(2, nil)
		e.advance(t, waitRunning, 600*time.Second)
		ch := closeAsync(t, e, at(600*time.Second))
		e.rec.waitN(t, opRequestClose, 1)
		e.advance(t, waitClosing, time.Second)
		noClose(t, ch, "tree still alive")
		j.empty(t)
		wantClosed(t, waitClose(t, ch))

		wantNoCalls(t, e, opKill, opFeedback)
		ends := e.rec.waitN(t, opEnd, 1)
		wantEndCall(t, ends[0], snap.ID, at(600*time.Second), 600*time.Second, domain.EndClosedByHotkey)
		if ends[0].state != domain.SessionEnded {
			t.Errorf("end state = %q, want ended", ends[0].state)
		}
		n := notifyAfter(t, e, 1)
		wantSession(t, "end notification", n, snap.ID, domain.SessionEnded, 600*time.Second, at(601*time.Second))
		if !n.EndedAt.Equal(at(600 * time.Second)) {
			t.Errorf("endedAt = %v, want %v", n.EndedAt, at(600*time.Second))
		}
		if !j.isClosed() {
			t.Error("job not closed after the close")
		}
		e.waitNoCurrent(t)
	})

	t.Run("CON-005/a árvore não sai: Kill em at+3s e os mesmos valores", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		j.setCloseResult(1, nil)
		e.advance(t, waitRunning, 600*time.Second)
		ch := closeAsync(t, e, at(600*time.Second))
		e.advance(t, waitClosing, closeWait)
		wantClosed(t, waitClose(t, ch))

		if n := e.rec.count(opKill); n != 1 {
			t.Errorf("Kill calls = %d, want 1", n)
		}
		wantSeq(t, e, []string{opRequestClose, opKill, opEnd, opJobClose}, opRequestClose, opKill, opEnd, opJobClose)
		wantEndCall(t, e.rec.of(opEnd)[0], snap.ID, at(600*time.Second), 600*time.Second, domain.EndClosedByHotkey)
		n := notifyAfter(t, e, 1)
		wantSession(t, "end notification", n, snap.ID, domain.SessionEnded, 600*time.Second, at(603*time.Second))
		if !n.EndedAt.Equal(at(600 * time.Second)) {
			t.Errorf("endedAt = %v, want %v", n.EndedAt, at(600*time.Second))
		}
	})

	t.Run("CON-005/RequestClose devolve 0: Kill imediato e fim na hora", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		j.setCloseResult(0, nil)
		e.advance(t, waitRunning, 600*time.Second)
		wantClosed(t, waitClose(t, closeAsync(t, e, at(600*time.Second))))
		wantSeq(t, e, []string{opRequestClose, opKill, opEnd}, opRequestClose, opKill, opEnd)
		wantEndCall(t, e.rec.of(opEnd)[0], snap.ID, at(600*time.Second), 600*time.Second, domain.EndClosedByHotkey)
	})

	t.Run("CON-005/RequestClose com erro: Kill imediato", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		j.setCloseResult(0, errKill)
		e.advance(t, waitRunning, 600*time.Second)
		wantClosed(t, waitClose(t, closeAsync(t, e, at(600*time.Second))))
		wantSeq(t, e, []string{opRequestClose, opKill, opEnd}, opRequestClose, opKill, opEnd)
	})

	t.Run("CON-005/20 s de tempo ativo termina discarded", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		j.setCloseResult(0, nil)
		e.advance(t, waitRunning, 20*time.Second)
		wantClosed(t, waitClose(t, closeAsync(t, e, at(20*time.Second))))
		ends := e.rec.waitN(t, opEnd, 1)
		wantEndCall(t, ends[0], snap.ID, at(20*time.Second), 20*time.Second, domain.EndClosedByHotkey)
		if ends[0].state != domain.SessionDiscarded {
			t.Errorf("end state = %q, want discarded", ends[0].state)
		}
		if n := notifyAfter(t, e, 1); n.State != domain.SessionDiscarded {
			t.Errorf("notification state = %q, want discarded", n.State)
		}
	})

	t.Run("CON-005/at anterior ao startedAt: refused no-game, nada gravado", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		e.advance(t, waitRunning, 10*time.Second)
		wantRefused(t, waitClose(t, closeAsync(t, e, at(-time.Second))), tracker.CloseReasonNoGame)
		wantNoCalls(t, e, opRequestClose, opKill, opEnd)
		if j.isClosed() {
			t.Error("job closed by a refused close")
		}
		e.waitCurrentState(t, domain.SessionRunning)
	})

	t.Run("CON-005/o dono continua atendendo durante os 3 s (evento do job e Current)", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		j.setCloseResult(1, nil)
		e.advance(t, waitRunning, 600*time.Second)
		ch := closeAsync(t, e, at(600*time.Second))
		e.waiters(t, waitClosing)
		j.newProcess(t, 5000) // ProcessAdded in Closing: Debug, the deadline continues
		e.clock.Advance(time.Second)
		cur, ok := e.tr.Current()
		if !ok || cur.State != domain.SessionRunning || cur.Active != 600*time.Second {
			t.Errorf("Current during Closing = %+v, %v; want running with 600 s (frozen at the trigger)", cur, ok)
		}
		noClose(t, ch, "1 s into the wait")
		e.clock.Advance(closeWait - time.Second)
		wantClosed(t, waitClose(t, ch))
		wantEndCall(t, e.rec.of(opEnd)[0], snap.ID, at(600*time.Second), 600*time.Second, domain.EndClosedByHotkey)
	})

	t.Run("CON-006/Paused: pausa fechada em at, active 100 s, sem feedback extra", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := pausedAt(t, e, 100*time.Second)
		j.setCloseResult(0, nil)
		e.advance(t, waitPaused, 40*time.Second)
		seen := len(e.notifications())
		wantClosed(t, waitClose(t, closeAsync(t, e, at(140*time.Second))))
		ends := e.rec.waitN(t, opEnd, 1)
		wantEndCall(t, ends[0], snap.ID, at(140*time.Second), 100*time.Second, domain.EndClosedByHotkey)
		if n := notifyAfter(t, e, seen); n.State != domain.SessionEnded || n.Active != 100*time.Second || !n.EndedAt.Equal(at(140*time.Second)) {
			t.Errorf("end notification = %+v, want ended, 100 s, endedAt T+140s", n)
		}
		if got := e.feedbackKinds(); len(got) != 1 || got[0] != tracker.FeedbackPaused {
			t.Errorf("feedback = %v, want only the one of the pause", got)
		}
	})

	t.Run("CON-007/Grace: nenhum RequestClose, Kill chamado, ended_at=graceSince, asOf=at, a carência pendente não grava depois", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		e.advance(t, waitGrace, 5*time.Second)
		wantClosed(t, waitClose(t, closeAsync(t, e, at(105*time.Second))))
		wantNoCalls(t, e, opRequestClose)
		if n := e.rec.count(opKill); n != 1 {
			t.Errorf("Kill calls = %d, want 1", n)
		}
		ends := e.rec.waitN(t, opEnd, 1)
		wantEndCall(t, ends[0], snap.ID, at(100*time.Second), 100*time.Second, domain.EndClosedByHotkey)
		n := notifyAfter(t, e, 2)
		wantSession(t, "end notification", n, snap.ID, domain.SessionEnded, 100*time.Second, at(105*time.Second))
		if !n.EndedAt.Equal(at(100 * time.Second)) {
			t.Errorf("endedAt = %v, want graceSince %v", n.EndedAt, at(100*time.Second))
		}
		e.clock.Advance(domain.SessionGracePeriod) // the old grace timer fires
		e.waitNoCurrent(t)
		if c := e.rec.count(opEnd); c != 1 {
			t.Errorf("EndSession calls after the old grace timer = %d, want 1", c)
		}
	})

	t.Run("CON-007/Grace com Kill que falha: failed e a carência segue", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		toGraceAfter(t, e, j, waitRunning, 100*time.Second)
		j.failKill(errKill, 1)
		wantFailed(t, waitClose(t, closeAsync(t, e, at(100*time.Second))))
		wantNoCalls(t, e, opEnd, opRequestClose)
		e.advance(t, waitGrace, domain.SessionGracePeriod)
		ends := e.rec.waitN(t, opEnd, 1)
		wantEndCall(t, ends[0], snap.ID, at(100*time.Second), 100*time.Second, domain.EndExited)
	})

	t.Run("CON-009/Launching de URI sem adoção: refused not-adopted, nada gravado, o prazo de 60 s segue", func(t *testing.T) {
		t.Parallel()
		lister := &fakeLister{}
		lister.set(explorerProc)
		e := newAdoptEnv(t, lister)
		if _, err := e.tryLaunch(t, uriSpec); err != nil {
			t.Fatalf("Launch(uri): %v", err)
		}
		wantRefused(t, waitClose(t, closeAsync(t, e, at(time.Second))), tracker.CloseReasonNotAdopted)
		wantNoCalls(t, e, opRequestClose, opKill, opEnd)
		e.advance(t, 1, watchEvery)
		e.waiters(t, 1)
		e.clock.Advance(domain.SessionAdoptTimeout)
		if got := stateSeq(t, e, 1); got[0] != domain.SessionDiscarded {
			t.Errorf("notification = %q, want discarded (the adoption deadline still applies)", got[0])
		}
	})

	t.Run("CON-012/Idle sem detached: refused no-game com Debug", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		wantRefused(t, waitClose(t, closeAsync(t, e, baseT)), tracker.CloseReasonNoGame)
		e.logs.wait(t, slog.LevelDebug, "fechar ignorado", nil)
		wantNoCalls(t, e, opRequestClose, opKill, opEnd)
	})

	t.Run("CON-013/Kill falha na espera: failed, Warn, Running continua contando de at, nada gravado ou notificado, o heartbeat seguinte grava", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		j.setCloseResult(1, nil)
		j.failKill(errKill, 1)
		e.advance(t, waitRunning, 600*time.Second)
		ch := closeAsync(t, e, at(600*time.Second))
		e.advance(t, waitClosing, closeWait)
		wantFailed(t, waitClose(t, ch))

		rec := e.logs.wait(t, slog.LevelWarn, "falha ao fechar o jogo", nil)
		wantAttrs(t, rec, map[string]string{"session_id": "100"})
		wantAttrContains(t, rec, "error", "access is denied")
		wantNoCalls(t, e, opEnd, opUpdate, opJobClose)
		if got := len(e.notifications()); got != 1 {
			t.Errorf("notifications = %d, want only the running one", got)
		}
		cur, _ := e.waitCurrent(t, "running again from the trigger", func(s domain.SessionSnapshot, ok bool) bool {
			return ok && s.State == domain.SessionRunning && s.Active == 603*time.Second
		})
		if cur.ID != snap.ID {
			t.Errorf("Current id = %d, want %d", cur.ID, snap.ID)
		}
		e.waiters(t, 3)
		e.clock.Advance(time.Hour)
		ups := e.rec.waitN(t, opUpdate, 1)
		last := ups[len(ups)-1]
		wantUpdateCall(t, last, snap.ID, domain.SessionRunning, at(603*time.Second+time.Hour), 603*time.Second+time.Hour)
	})

	t.Run("CON-013/Paused com Kill que falha: continua pausada com o mesmo intervalo", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j := pausedAt(t, e, 100*time.Second)
		j.setCloseResult(0, nil)
		j.failKill(errKill, 1)
		e.advance(t, waitPaused, 40*time.Second)
		wantFailed(t, waitClose(t, closeAsync(t, e, at(140*time.Second))))
		wantNoCalls(t, e, opEnd, opResumeP)
		if n := e.rec.count(opPause); n != 1 {
			t.Errorf("PauseSession calls = %d, want the original one only", n)
		}
		cur := e.waitCurrentState(t, domain.SessionPaused)
		if cur.Active != 100*time.Second {
			t.Errorf("paused active = %v, want 100 s", cur.Active)
		}
	})

	t.Run("CON-014/closed não chama o feedback do tracker", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		j.setCloseResult(0, nil)
		e.advance(t, waitRunning, 600*time.Second)
		wantClosed(t, waitClose(t, closeAsync(t, e, at(600*time.Second))))
		e.rec.waitN(t, opEnd, 1)
		wantNoCalls(t, e, opFeedback)
	})

	t.Run("CON-021/depois de BeginShutdown: ErrShuttingDown, sem RequestClose nem Kill", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.launch(t)
		e.tr.BeginShutdown()
		o := waitClose(t, closeAsync(t, e, at(time.Minute)))
		if !errors.Is(o.err, tracker.ErrShuttingDown) {
			t.Errorf("CloseByHotkey err = %v, want ErrShuttingDown", o.err)
		}
		wantNoCalls(t, e, opRequestClose, opKill)
	})

	t.Run("CON-030/espera de 3 s medida só pelo Clock: 3 s − 1 ns não mata, 3 s mata", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		j.setCloseResult(1, nil)
		e.advance(t, waitRunning, 600*time.Second)
		ch := closeAsync(t, e, at(600*time.Second))
		e.advance(t, waitClosing, closeWait-time.Nanosecond)
		noClose(t, ch, "3 s − 1 ns")
		wantNoCalls(t, e, opKill)
		e.clock.Advance(time.Nanosecond)
		wantClosed(t, waitClose(t, ch))
		if n := e.rec.count(opKill); n != 1 {
			t.Errorf("Kill calls = %d, want 1", n)
		}
	})

	t.Run("CON-030/disparo de geração antiga ignorado", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		_, j1 := e.launch(t)
		j1.setCloseResult(1, nil)
		e.advance(t, waitRunning, 600*time.Second)
		ch := closeAsync(t, e, at(600*time.Second)) // timer #1 fires at T+603s
		e.waiters(t, waitClosing)
		e.clock.Advance(time.Second)
		j1.empty(t) // the tree leaves: the close ends, timer #1 stays pending
		wantClosed(t, waitClose(t, ch))
		e.waitNoCurrent(t)
		if n := e.nextNotify(t); n.State != domain.SessionEnded { // required by the contract
			t.Fatalf("notification after the close = %q, want ended", n.State)
		}

		_, j2 := e.launch(t) // started at T+601s
		j2.setCloseResult(1, nil)
		e.advance(t, 5, time.Second) // T+602s; pending: old timers + timer #1
		ch2 := closeAsync(t, e, at(602*time.Second))
		e.rec.waitN(t, opRequestClose, 2)
		e.waiters(t, 6)
		e.clock.Advance(time.Second) // T+603s: the stale timer #1 fires
		noClose(t, ch2, "stale CloseTimeout")
		wantNoCalls(t, e, opKill)
		e.clock.Advance(closeWait - time.Second) // T+605s: timer #2
		wantClosed(t, waitClose(t, ch2))
		if n := e.rec.count(opKill); n != 1 {
			t.Errorf("Kill calls = %d, want 1 (only the current generation)", n)
		}
	})

	t.Run("CON-031/Launch e LaunchUntracked durante a espera: SessionActive, nada criado; depois do fim voltam", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		j.setCloseResult(1, nil)
		e.advance(t, waitRunning, 600*time.Second)
		ch := closeAsync(t, e, at(600*time.Second))
		e.waiters(t, waitClosing)

		_, err := e.tr.Launch(timeoutCtx(t), testSpec)
		var active *tracker.SessionActiveError
		if !errors.As(err, &active) || active.Session.ID != snap.ID || active.Session.GameID != testGameID {
			t.Errorf("Launch during Closing err = %v, want SessionActiveError for session %d", err, snap.ID)
		}
		if _, err := e.tr.LaunchUntracked(timeoutCtx(t), testSpec); !errors.As(err, &active) {
			t.Errorf("LaunchUntracked during Closing err = %v, want SessionActiveError", err)
		}
		if n := e.launcher.jobCount(); n != 1 {
			t.Errorf("jobs created = %d, want 1", n)
		}
		wantNoCalls(t, e, opDetached)

		e.clock.Advance(closeWait)
		wantClosed(t, waitClose(t, ch))
		e.waitNoCurrent(t)
		e.nextNotify(t) // the ended notification
		if s, _ := e.launch(t); s.State != domain.SessionRunning {
			t.Errorf("Launch after the close = %+v, want running", s)
		}
	})

	t.Run("CON-032/pausa, suspensão e heartbeat ignorados, Current congelado, nenhum session.changed, segundo gatilho refused closing", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, envConfig{heartbeat: 2 * time.Second, poll: time.Hour})
		_, j := e.launch(t)
		j.setCloseResult(1, nil)
		e.advance(t, waitRunning, time.Second)
		ch := closeAsync(t, e, at(time.Second))
		e.waiters(t, waitClosing)

		res, err := e.tr.TogglePause(timeoutCtx(t), domain.PauseHotkey, at(time.Second))
		if err != nil {
			t.Errorf("TogglePause during Closing: %v", err)
		}
		_ = res
		if err := e.tr.Suspend(timeoutCtx(t), at(time.Second)); err != nil {
			t.Errorf("Suspend during Closing: %v", err)
		}
		if err := e.tr.Wake(timeoutCtx(t), at(time.Second), true); err != nil {
			t.Errorf("Wake during Closing: %v", err)
		}
		for name, fn := range map[string]func() (domain.SessionSnapshot, error){
			"Pause":  func() (domain.SessionSnapshot, error) { return e.tr.Pause(timeoutCtx(t), at(time.Second)) },
			"Resume": func() (domain.SessionSnapshot, error) { return e.tr.Resume(timeoutCtx(t), at(time.Second)) },
		} {
			s, err := fn()
			if err != nil || s.State != domain.SessionRunning || s.Active != time.Second {
				t.Errorf("%s during Closing = %+v, %v; want the current running session at 1 s", name, s, err)
			}
		}
		wantRefused(t, waitClose(t, closeAsync(t, e, at(time.Second))), tracker.CloseReasonClosing)

		e.clock.Advance(time.Second) // T+2s: the heartbeat fires during Closing
		cur, _ := e.tr.Current()
		if cur.State != domain.SessionRunning || cur.Active != time.Second {
			t.Errorf("Current during Closing = %+v, want running frozen at 1 s", cur)
		}
		wantNoCalls(t, e, opPause, opResumeP, opFeedback, opUpdate, opKill)
		if got := len(e.notifications()); got != 1 {
			t.Errorf("notifications before the end = %d, want only the running one", got)
		}
		e.clock.Advance(2 * time.Second) // T+4s: deadline
		wantClosed(t, waitClose(t, ch))
		wantNoCalls(t, e, opUpdate)
	})

	t.Run("CON-033/Close do tracker durante a espera: nada morto, ended_at=at, agent-shutdown, sem notificação, o pendente responde ErrShuttingDown", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		snap, j := e.launch(t)
		j.setCloseResult(1, nil)
		e.advance(t, waitRunning, 600*time.Second)
		ch := closeAsync(t, e, at(600*time.Second))
		e.waiters(t, waitClosing)
		if err := e.close(t); err != nil {
			t.Fatalf("Close: %v", err)
		}
		o := waitClose(t, ch)
		if !errors.Is(o.err, tracker.ErrShuttingDown) {
			t.Errorf("pending CloseByHotkey err = %v, want ErrShuttingDown", o.err)
		}
		wantNoCalls(t, e, opKill)
		ends := e.rec.waitN(t, opEnd, 1)
		wantEndCall(t, ends[0], snap.ID, at(600*time.Second), 600*time.Second, domain.EndAgentShutdown)
		if got := len(e.notifications()); got != 1 {
			t.Errorf("notifications = %d, want only the running one (no end notification at shutdown)", got)
		}
		if !j.isClosed() {
			t.Error("job not closed at shutdown")
		}
	})
}

// TestTrackerCloseGoroutines is not parallel (goleak inspects every goroutine
// of the process).
func TestTrackerCloseGoroutines(t *testing.T) {
	t.Run("CON-030/nenhuma goroutine vazada depois de fechar com espera e com Kill, e o job fecha", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		j.setCloseResult(1, nil)
		e.advance(t, waitRunning, 600*time.Second)
		ch := closeAsync(t, e, at(600*time.Second))
		e.advance(t, waitClosing, closeWait)
		wantClosed(t, waitClose(t, ch))
		if !j.isClosed() {
			t.Error("job handle not closed")
		}
		if err := e.close(t); err != nil {
			t.Errorf("Close: %v", err)
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("CON-033/nenhuma goroutine vazada com Close durante a espera", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		e := newEnv(t, quiet)
		_, j := e.launch(t)
		j.setCloseResult(1, nil)
		e.advance(t, waitRunning, 600*time.Second)
		ch := closeAsync(t, e, at(600*time.Second))
		e.waiters(t, waitClosing)
		if err := e.close(t); err != nil {
			t.Errorf("Close: %v", err)
		}
		waitClose(t, ch)
		if !j.isClosed() {
			t.Error("job handle not closed")
		}
		goleak.VerifyNone(t, ignore)
	})
}
