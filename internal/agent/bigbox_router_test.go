package agent

// Tests for T23 of docs/specs/phase-15-big-box-v2/tasks.md: the trigger router with the third action,
// input.ActionBigBox [ATB-003..ATB-007, ATB-015]. The tracker is the pauseFake (its Current() comes from
// the embedded fakeTracker); the opener is openerFake; the "nothing happened" checks run after flush.

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/feedback"
	"github.com/arcadia-launcher/arcadia/internal/input"
)

// openerFake is a bigBoxOpener that counts calls.
type openerFake struct {
	mu    sync.Mutex
	n     int
	err   error
	calls chan struct{}
}

func (o *openerFake) OpenBigBox() error {
	o.mu.Lock()
	o.n++
	err := o.err
	if o.calls == nil {
		o.calls = make(chan struct{}, 64)
	}
	ch := o.calls
	o.mu.Unlock()
	ch <- struct{}{}
	return err
}

func (o *openerFake) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.n
}

func (o *openerFake) setErr(err error) {
	o.mu.Lock()
	o.err = err
	o.mu.Unlock()
}

// waitOpen waits for n OpenBigBox calls.
func (rig *pauseRig) waitOpen(t *testing.T, n int) {
	t.Helper()
	rig.opener.mu.Lock()
	if rig.opener.calls == nil {
		rig.opener.calls = make(chan struct{}, 64)
	}
	ch := rig.opener.calls
	rig.opener.mu.Unlock()
	for i := 0; i < n; i++ {
		select {
		case <-ch:
		case <-time.After(pauseWait):
			t.Fatalf("OpenBigBox calls = %d, want %d (timeout)", i, n)
		}
	}
}

func bigBoxTrig(at time.Time) input.Trigger {
	return input.Trigger{Source: domain.PauseGamepad, Action: input.ActionBigBox, At: at}
}

func (rig *pauseRig) setSession(state domain.SessionState) {
	f := rig.tr.fakeTracker
	f.mu.Lock()
	f.current = domain.SessionSnapshot{ID: 5, GameID: 42, State: state}
	f.hasCurrent = true
	f.mu.Unlock()
}

// wantOnlyCurrent fails when the tracker saw a TogglePause or CloseByHotkey.
func (rig *pauseRig) wantOnlyCurrent(t *testing.T) {
	t.Helper()
	toggles := 0
	for _, c := range rig.tr.recorded() {
		switch c.method {
		case "TogglePause":
			toggles++
		case "CloseByHotkey":
			t.Errorf("unexpected CloseByHotkey call %+v", c)
		}
	}
	// The only TogglePause calls are the sentinels sent by flush.
	if toggles != rig.sentinels {
		t.Errorf("TogglePause calls = %d, want %d (flush sentinels only)", toggles, rig.sentinels)
	}
}

func TestBigBoxRouter(t *testing.T) {
	t.Run("ATB-003/BigBoxDebounce é 300 ms", func(t *testing.T) {
		if domain.BigBoxDebounce != 300*time.Millisecond {
			t.Fatalf("BigBoxDebounce = %v, want 300 ms", domain.BigBoxDebounce)
		}
	})

	t.Run("ATB-003/menos de 300 ms entre os At viram um só, sem som; 300 ms ou mais viram dois", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- bigBoxTrig(pauseT0)
		rig.in <- bigBoxTrig(pauseT0.Add(299 * time.Millisecond)) // dropped
		rig.in <- bigBoxTrig(pauseT0.Add(300 * time.Millisecond))
		rig.waitOpen(t, 2)
		rig.flush(t)
		if n := rig.opener.count(); n != 2 {
			t.Errorf("OpenBigBox calls = %d, want 2", n)
		}
		rig.wantNoFeedback(t)
	})

	t.Run("ATB-003/as janelas de pausa e de fechar não contam o gatilho do Big Box e vice-versa", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- pauseTrig(domain.PauseGamepad, pauseT0)
		rig.in <- closeTrig(pauseT0)
		rig.in <- bigBoxTrig(pauseT0) // same instant: own window
		rig.nextToggles(t, 1)
		rig.nextCloses(t, 1)
		rig.waitOpen(t, 1)
		// And the Big Box window does not swallow a pause/close right after it.
		rig.in <- bigBoxTrig(pauseT0.Add(time.Second))
		rig.in <- pauseTrig(domain.PauseHotkey, pauseT0.Add(time.Second))
		rig.in <- closeTrig(pauseT0.Add(time.Second))
		rig.waitOpen(t, 1)
		rig.nextToggles(t, 1)
		rig.nextCloses(t, 1)
	})

	t.Run("ATB-003/o roteador só chama Current: nenhuma chamada a TogglePause nem CloseByHotkey", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- bigBoxTrig(pauseT0)
		rig.waitOpen(t, 1)
		rig.flush(t)
		rig.wantOnlyCurrent(t)
		if len(rig.tr.closes) != 0 {
			t.Error("CloseByHotkey reached")
		}
	})

	t.Run("ATB-004/sem sessão o abridor é chamado uma vez, Info e nenhum som", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.in <- bigBoxTrig(pauseT0)
		rig.waitOpen(t, 1)
		rig.flush(t)
		if n := rig.opener.count(); n != 1 {
			t.Errorf("OpenBigBox calls = %d, want 1", n)
		}
		if n := countLog(rig.log.lines(t), "INFO", "Big Box pedido pelo controle"); n != 1 {
			t.Errorf("Info \"Big Box pedido pelo controle\" = %d, want 1; log = %+v", n, rig.log.lines(t))
		}
		rig.wantNoFeedback(t)
	})

	t.Run("ATB-005/sessão aberta em qualquer estado: abridor nunca chamado, Debug e EventRefused", func(t *testing.T) {
		// Launching and closing are not persisted states: the snapshot of a launching/closing
		// session is one of these (see report); every state the tracker can report is covered.
		for _, st := range []domain.SessionState{domain.SessionRunning, domain.SessionPaused, domain.SessionGrace, domain.SessionEnded} {
			t.Run("ATB-005/"+string(st), func(t *testing.T) {
				rig := startPauseRig(t, newPauseFake())
				rig.setSession(st)
				rig.in <- bigBoxTrig(pauseT0)
				rig.waitRefused(t)
				rig.flush(t)
				if n := rig.opener.count(); n != 0 {
					t.Errorf("OpenBigBox calls = %d, want 0", n)
				}
				if n := countLog(rig.log.lines(t), "DEBUG", "Big Box ignorado: jogo em execução"); n != 1 {
					t.Errorf("Debug \"Big Box ignorado: jogo em execução\" = %d, want 1; log = %+v", n, rig.log.lines(t))
				}
				if got := rig.fb.events(); len(got) != 1 || got[0] != feedback.EventRefused {
					t.Errorf("feedback = %v, want one EventRefused", got)
				}
			})
		}
	})

	t.Run("ATB-005/jogo sem contar tempo vivo e nenhuma sessão: segue ATB-004", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake()) // Current() = (zero, false); an untracked game has no session
		rig.in <- bigBoxTrig(pauseT0)
		rig.waitOpen(t, 1)
		rig.wantNoFeedback(t)
	})

	t.Run("ATB-006/spawn que falha: Warn com path e error, EventRefused, e o gatilho seguinte tenta de novo", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.opener.setErr(errors.New("CreateProcess: access denied"))
		rig.in <- bigBoxTrig(pauseT0)
		rig.waitOpen(t, 1)
		rig.waitRefused(t)
		rig.in <- bigBoxTrig(pauseT0.Add(time.Second))
		rig.waitOpen(t, 1)
		rig.flush(t)
		if n := countLog(rig.log.lines(t), "WARN", "falha ao abrir o Big Box"); n != 2 {
			t.Errorf("Warn \"falha ao abrir o Big Box\" = %d, want 2; log = %+v", n, rig.log.lines(t))
		}
		if n := rig.opener.count(); n != 2 {
			t.Errorf("OpenBigBox calls = %d, want 2", n)
		}
	})

	t.Run("ATB-007/shuttingDown true: Debug, sem processo e sem som", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.down.Store(true)
		rig.in <- bigBoxTrig(pauseT0)
		rig.flush(t)
		if n := rig.opener.count(); n != 0 {
			t.Errorf("OpenBigBox calls = %d, want 0", n)
		}
		if n := countLog(rig.log.lines(t), "DEBUG", "agente encerrando; gatilho do Big Box descartado"); n != 1 {
			t.Errorf("Debug record = %d, want 1; log = %+v", n, rig.log.lines(t))
		}
		rig.wantNoFeedback(t)
	})

	t.Run("ATB-007/ctx do roteador cancelado: sem processo e sem som", func(t *testing.T) {
		rig := startPauseRig(t, newPauseFake())
		rig.r.Close()
		rig.in <- bigBoxTrig(pauseT0)
		if n := rig.opener.count(); n != 0 {
			t.Errorf("OpenBigBox calls = %d, want 0", n)
		}
		rig.wantNoFeedback(t)
	})

	t.Run("ATB-015/nenhuma sessão é criada, pausada, retomada ou encerrada pelo caminho do Big Box", func(t *testing.T) {
		tr := newPauseFake()
		rig := startPauseRig(t, tr)
		rig.setSession(domain.SessionRunning)
		rig.in <- bigBoxTrig(pauseT0)
		rig.in <- bigBoxTrig(pauseT0.Add(time.Second))
		rig.waitRefused(t)
		rig.waitRefused(t)
		rig.flush(t)
		rig.wantOnlyCurrent(t)
		if pa, re := tr.uiInstants(); len(pa)+len(re) != 0 {
			t.Errorf("Pause/Resume calls = %v/%v, want none", pa, re)
		}
		if l := tr.launches(); len(l) != 0 {
			t.Errorf("Launch calls = %v, want none", l)
		}
	})
}
