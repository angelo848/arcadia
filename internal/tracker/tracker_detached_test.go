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

// T12: a game launched without counting time lives in a Job Object kept by the
// tracker (detachedJob) only so that it can be closed. Contract:
// contracts/session-close-state-machine.md (untracked rows and DetachedPoll).
// Assumed API:
//
//	type DetachedJob interface { ActiveProcesses() (int, error); RequestClose() (int, error); Kill() error; Close() error }
//	Launcher.LaunchDetached(spec LaunchSpec) (DetachedJob, uint32, error)
//
// While an untracked close waits, two FakeClock waiters are pending: the 3 s
// deadline and the 250 ms DetachedPoll (re-armed while ActiveProcesses > 0).
// Idle with a kept job and no close in progress arms no waiter. A second
// LaunchUntracked is refused while ActiveProcesses() > 0 (checked at launch).
const waitDetached = 2

func launchUntracked(t *testing.T, e *env) *fakeDetachedJob {
	t.Helper()
	n := len(e.launcher.detached)
	if _, err := e.tr.LaunchUntracked(timeoutCtx(t), testSpec); err != nil {
		t.Fatalf("LaunchUntracked: %v", err)
	}
	return e.launcher.detachedJob(t, n)
}

func wantClosedUntracked(t *testing.T, o closeOut) {
	t.Helper()
	if o.err != nil || o.res.Kind != tracker.CloseClosed || o.res.GameID != testGameID || !o.res.Untracked {
		t.Fatalf("CloseByHotkey = %+v, %v; want closed{gameId=%d, untracked}", o.res, o.err, testGameID)
	}
}

func wantNoSessionWrites(t *testing.T, e *env) {
	t.Helper()
	wantNoCalls(t, e, opStart, opUpdate, opEnd, opDelete, opNotify, opPrepare, opFeedback)
}

func wantActiveZeroID(t *testing.T, err error) {
	t.Helper()
	var sa *tracker.SessionActiveError
	if !errors.As(err, &sa) || !errors.Is(err, tracker.ErrSessionActive) {
		t.Fatalf("err = %v, want *SessionActiveError", err)
	}
	if sa.Session.ID != 0 || sa.Session.GameID != testGameID || sa.Session.Title != testTitle {
		t.Errorf("SessionActiveError session = %+v, want id 0, game %d, %q", sa.Session, testGameID, testTitle)
	}
}

func TestTrackerDetachedClose(t *testing.T) {
	t.Parallel()

	t.Run("CON-010/detached com processos: RequestClose, espera, Kill no prazo, Close do job, closed untracked, nada gravado nem notificado", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setCloseResult(2, nil)
		ch := closeAsync(t, e, baseT)
		e.rec.waitN(t, opDRequestClose, 1)
		e.waiters(t, waitDetached)
		e.clock.Advance(closeWait - time.Nanosecond)
		noClose(t, ch, "3 s - 1 ns")
		wantNoCalls(t, e, opDKill)
		e.clock.Advance(time.Nanosecond)
		wantClosedUntracked(t, waitClose(t, ch))

		if n := e.rec.count(opDKill); n != 1 {
			t.Errorf("Kill calls = %d, want 1", n)
		}
		if n := d.closeCalls(); n != 1 {
			t.Errorf("job Close calls = %d, want 1", n)
		}
		wantSeq(t, e, []string{opDRequestClose, opDKill, opDClose}, opDRequestClose, opDKill, opDClose)
		wantNoSessionWrites(t, e)
		if _, ok := e.tr.Current(); ok {
			t.Error("Current reports a session")
		}
	})

	t.Run("CON-010/a conferência a cada 250 ms vê ActiveProcesses=0 e fecha sem Kill", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setCloseResult(1, nil)
		ch := closeAsync(t, e, baseT)
		e.advance(t, waitDetached, detachedPollEvery) // still alive: re-armed
		noClose(t, ch, "tree alive at 250 ms")
		d.setActive(0)
		e.advance(t, waitDetached, detachedPollEvery)
		wantClosedUntracked(t, waitClose(t, ch))
		wantNoCalls(t, e, opDKill)
		if n := d.closeCalls(); n != 1 {
			t.Errorf("job Close calls = %d, want 1", n)
		}
		wantNoSessionWrites(t, e)
	})

	t.Run("CON-010/ActiveProcesses=0 de início: refused no-game, o job é fechado e esquecido", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setActive(0)
		wantRefused(t, waitClose(t, closeAsync(t, e, baseT)), tracker.CloseReasonNoGame)
		wantNoCalls(t, e, opDRequestClose, opDKill)
		if n := d.closeCalls(); n != 1 {
			t.Errorf("job Close calls = %d, want 1", n)
		}
		wantRefused(t, waitClose(t, closeAsync(t, e, baseT)), tracker.CloseReasonNoGame)
		if n := d.closeCalls(); n != 1 {
			t.Errorf("job Close calls after the second trigger = %d, want still 1 (forgotten)", n)
		}
		wantNoSessionWrites(t, e)
	})

	t.Run("CON-010/RequestClose devolve 0: Kill na hora, sem espera", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setCloseResult(0, nil)
		wantClosedUntracked(t, waitClose(t, closeAsync(t, e, baseT)))
		wantSeq(t, e, []string{opDRequestClose, opDKill, opDClose}, opDRequestClose, opDKill, opDClose)
		wantNoSessionWrites(t, e)
	})

	t.Run("CON-010/RequestClose com erro: Kill na hora", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setCloseResult(0, errKill)
		wantClosedUntracked(t, waitClose(t, closeAsync(t, e, baseT)))
		wantSeq(t, e, []string{opDRequestClose, opDKill, opDClose}, opDRequestClose, opDKill, opDClose)
	})

	t.Run("CON-013/Kill falha no detached: failed, Warn, o job continua guardado e um novo gatilho tenta de novo", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setCloseResult(1, nil)
		d.failKill(errKill, 1)
		ch := closeAsync(t, e, baseT)
		e.advance(t, waitDetached, closeWait)
		wantFailed(t, waitClose(t, ch))
		rec := e.logs.wait(t, slog.LevelWarn, "falha ao fechar o jogo", nil)
		wantAttrContains(t, rec, "error", "access is denied")
		if n := d.closeCalls(); n != 0 {
			t.Errorf("job Close calls = %d, want 0 (still kept)", n)
		}
		wantNoSessionWrites(t, e)

		d.setActive(1)
		ch2 := closeAsync(t, e, baseT)
		e.rec.waitN(t, opDRequestClose, 2)
		e.advance(t, waitDetached, closeWait)
		wantClosedUntracked(t, waitClose(t, ch2))
	})

	t.Run("CON-030/segundo CloseByHotkey durante a espera do detached: refused closing", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setCloseResult(1, nil)
		ch := closeAsync(t, e, baseT)
		e.waiters(t, waitDetached)
		wantRefused(t, waitClose(t, closeAsync(t, e, baseT)), tracker.CloseReasonClosing)
		e.clock.Advance(closeWait)
		wantClosedUntracked(t, waitClose(t, ch))
		if n := e.rec.count(opDRequestClose); n != 1 {
			t.Errorf("RequestClose calls = %d, want 1", n)
		}
	})

	t.Run("CON-031/Launch e LaunchUntracked durante o fechamento do detached: SessionActive com sessionId 0, gameId e título", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setCloseResult(1, nil)
		ch := closeAsync(t, e, baseT)
		e.waiters(t, waitDetached)

		_, err := e.tr.Launch(timeoutCtx(t), testSpec)
		wantActiveZeroID(t, err)
		_, err = e.tr.LaunchUntracked(timeoutCtx(t), testSpec)
		wantActiveZeroID(t, err)
		if n := e.rec.count(opDetached); n != 1 {
			t.Errorf("LaunchDetached calls = %d, want 1", n)
		}
		wantNoCalls(t, e, opPrepare, opStart)

		e.clock.Advance(closeWait)
		wantClosedUntracked(t, waitClose(t, ch))
		// after the end both launches are accepted again
		if _, err := e.tr.LaunchUntracked(timeoutCtx(t), testSpec); err != nil {
			t.Errorf("LaunchUntracked after the close: %v", err)
		}
	})

	t.Run("CON-033/Close do tracker durante a espera do detached: sem Kill, job fechado, o pendente responde ErrShuttingDown", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setCloseResult(1, nil)
		ch := closeAsync(t, e, baseT)
		e.waiters(t, waitDetached)
		if err := e.close(t); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if o := waitClose(t, ch); !errors.Is(o.err, tracker.ErrShuttingDown) {
			t.Errorf("pending CloseByHotkey err = %v, want ErrShuttingDown", o.err)
		}
		wantNoCalls(t, e, opDKill)
		if n := d.closeCalls(); n != 1 {
			t.Errorf("job Close calls = %d, want 1", n)
		}
		wantNoSessionWrites(t, e)
	})
}

func TestTrackerDetachedKeep(t *testing.T) {
	t.Parallel()

	t.Run("CON-011/segundo LaunchUntracked com o job vivo é recusado: SessionActive id 0, um jogo por vez", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		_, err := e.tr.LaunchUntracked(timeoutCtx(t), testSpec)
		wantActiveZeroID(t, err)
		if n := e.rec.count(opDetached); n != 1 {
			t.Errorf("LaunchDetached calls = %d, want 1", n)
		}
		wantNoCalls(t, e, opDKill)
		if n := d.closeCalls(); n != 0 {
			t.Errorf("job Close calls = %d, want 0", n)
		}
	})

	t.Run("CON-011/árvore encerrada: um novo LaunchUntracked é aceito e fecha o job anterior sem matar", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d1 := launchUntracked(t, e)
		d1.setActive(0)
		d2 := launchUntracked(t, e)
		if n := d1.closeCalls(); n != 1 {
			t.Errorf("first job Close calls = %d, want 1", n)
		}
		wantNoCalls(t, e, opDKill)
		if n := d2.closeCalls(); n != 0 {
			t.Errorf("second job Close calls = %d, want 0 (kept)", n)
		}
		// the kept one is the last: the hotkey acts on it
		d2.setCloseResult(0, nil)
		wantClosedUntracked(t, waitClose(t, closeAsync(t, e, baseT)))
	})

	t.Run("CON-011/Tracker.Close fecha o job guardado sem matar, uma vez", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		if err := e.close(t); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if n := d.closeCalls(); n != 1 {
			t.Errorf("job Close calls = %d, want 1", n)
		}
		wantNoCalls(t, e, opDKill, opDRequestClose)
	})

	t.Run("CON-011/BeginShutdown não mata nem fecha o job; depois dele CloseByHotkey devolve ErrShuttingDown", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		e.tr.BeginShutdown()
		o := waitClose(t, closeAsync(t, e, baseT))
		if !errors.Is(o.err, tracker.ErrShuttingDown) {
			t.Errorf("CloseByHotkey err = %v, want ErrShuttingDown", o.err)
		}
		wantNoCalls(t, e, opDKill, opDRequestClose)
		if n := d.closeCalls(); n != 0 {
			t.Errorf("job Close calls = %d, want 0 until Close", n)
		}
	})

	t.Run("CON-011/falha ao criar o job: LaunchError e nada guardado", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		e.launcher.failDetached(&tracker.LaunchError{Err: errAccess, WinError: 5}, 1)
		_, err := e.tr.LaunchUntracked(timeoutCtx(t), testSpec)
		var le *tracker.LaunchError
		if !errors.As(err, &le) {
			t.Fatalf("LaunchUntracked err = %v, want *LaunchError", err)
		}
		wantRefused(t, waitClose(t, closeAsync(t, e, baseT)), tracker.CloseReasonNoGame)
		wantNoSessionWrites(t, e)
	})

	t.Run("CON-011/o contrato SES-032 continua: nenhuma sessão e nenhuma notificação", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		launchUntracked(t, e)
		if _, ok := e.tr.Current(); ok {
			t.Error("Current reports a session")
		}
		wantNoSessionWrites(t, e)
		if n := e.launcher.jobCount(); n != 0 {
			t.Errorf("tracked jobs created = %d, want 0", n)
		}
	})

	t.Run("CON-021/com sessão aberta o gatilho fecha só a sessão, nunca o detached recusado", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setActive(0) // so that a tracked launch is accepted; the old job is released
		_, j := e.launch(t)
		j.setCloseResult(0, nil)
		e.advance(t, waitRunning, 600*time.Second)
		wantClosed(t, waitClose(t, closeAsync(t, e, at(600*time.Second))))
		wantNoCalls(t, e, opDKill)
	})
}

// TestTrackerDetachedGoroutines is not parallel (goleak inspects the process).
func TestTrackerDetachedGoroutines(t *testing.T) {
	t.Run("CON-010/nenhuma goroutine vazada depois de fechar o detached com espera e Kill", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		d.setCloseResult(1, nil)
		ch := closeAsync(t, e, baseT)
		e.waiters(t, waitDetached)
		e.clock.Advance(closeWait)
		wantClosedUntracked(t, waitClose(t, ch))
		if err := e.close(t); err != nil {
			t.Errorf("Close: %v", err)
		}
		goleak.VerifyNone(t, ignore)
	})

	t.Run("CON-011/nenhuma goroutine vazada com o detached guardado e Close", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		e := newEnv(t, quiet)
		d := launchUntracked(t, e)
		if err := e.close(t); err != nil {
			t.Errorf("Close: %v", err)
		}
		if n := d.closeCalls(); n != 1 {
			t.Errorf("job Close calls = %d, want 1", n)
		}
		goleak.VerifyNone(t, ignore)
	})
}

var _ = domain.SessionRunning
