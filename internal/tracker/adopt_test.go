package tracker_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/store"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// Processes of the adoption tests: the shell and the game share no folder with
// the game install dir except the game itself (RUL-002).
var (
	explorerProc = tracker.ProcInfo{PID: 100, Path: `C:\Windows\explorer.exe`}
	gameProc     = tracker.ProcInfo{PID: 4242, Path: `C:\Games\CS2\cs2.exe`}
	gameRules    = []store.TrackingRule{{Kind: tracker.RuleInstallDir, Value: `C:\Games\CS2`}}
	watchEvery   = time.Second
)

// newAdoptEnv builds a tracker whose watcher reads lister every watchEvery.
func newAdoptEnv(t *testing.T, lister *fakeLister) *env {
	t.Helper()
	return newEnv(t, envConfig{
		heartbeat: time.Hour,
		poll:      time.Hour,
		procs:     lister,
		watch:     watchEvery,
		rules:     gameRules,
	})
}

// stateSeq collects the next n notification states of e.
func stateSeq(t *testing.T, e *env, n int) []domain.SessionState {
	t.Helper()
	out := make([]domain.SessionState, 0, n)
	for range n {
		out = append(out, e.nextNotify(t).State)
	}
	return out
}

func TestTrackerAdoption(t *testing.T) {
	t.Parallel()

	t.Run("RUL-002/processo novo que casa com install_dir é adotado e a sessão fica running", func(t *testing.T) {
		t.Parallel()
		lister := &fakeLister{}
		lister.set(explorerProc)
		e := newAdoptEnv(t, lister)
		if _, err := e.tryLaunch(t, uriSpec); err != nil {
			t.Fatalf("Launch(uri): %v", err)
		}
		lister.set(explorerProc, gameProc)
		tick(t, e, 1)
		if got := stateSeq(t, e, 1); got[0] != domain.SessionRunning {
			t.Errorf("first notification = %q, want running", got[0])
		}
		if cur, ok := e.tr.Current(); !ok || cur.State != domain.SessionRunning {
			t.Errorf("Current = %+v, %v; want a running session", cur, ok)
		}
	})

	t.Run("RUL-002/processo que já existia antes do lançamento não é o jogo", func(t *testing.T) {
		t.Parallel()
		lister := &fakeLister{}
		lister.set(explorerProc, gameProc) // the game already runs before Launch
		e := newAdoptEnv(t, lister)
		if _, err := e.tryLaunch(t, uriSpec); err != nil {
			t.Fatalf("Launch(uri): %v", err)
		}
		e.advance(t, 1, watchEvery)
		// Past the adoption timeout, the only notification is the discard.
		e.waiters(t, 1)
		e.clock.Advance(domain.SessionAdoptTimeout)
		if got := stateSeq(t, e, 1); got[0] != domain.SessionDiscarded {
			t.Errorf("first notification = %q, want discarded (the baseline process must not be adopted)", got[0])
		}
	})

	t.Run("RUL-002/processo fora das regras não é adotado e a sessão é descartada no prazo", func(t *testing.T) {
		t.Parallel()
		lister := &fakeLister{}
		lister.set(explorerProc)
		e := newAdoptEnv(t, lister)
		if _, err := e.tryLaunch(t, uriSpec); err != nil {
			t.Fatalf("Launch(uri): %v", err)
		}
		lister.set(explorerProc, tracker.ProcInfo{PID: 9, Path: `D:\Other\launcher.exe`})
		e.advance(t, 1, watchEvery)
		e.waiters(t, 1)
		e.clock.Advance(domain.SessionAdoptTimeout)
		if got := stateSeq(t, e, 1); got[0] != domain.SessionDiscarded {
			t.Errorf("first notification = %q, want discarded", got[0])
		}
	})

	t.Run("RUL-004/saída do jogo entra em carência e o processo voltando a casar retoma a sessão", func(t *testing.T) {
		t.Parallel()
		lister := &fakeLister{}
		lister.set(explorerProc)
		e := newAdoptEnv(t, lister)
		if _, err := e.tryLaunch(t, uriSpec); err != nil {
			t.Fatalf("Launch(uri): %v", err)
		}
		lister.set(explorerProc, gameProc)
		tick(t, e, 1)
		lister.set(explorerProc) // the game exits: the job is empty
		tick(t, e, 2)
		lister.set(explorerProc, gameProc) // it comes back within the grace period
		tick(t, e, 3)
		got := stateSeq(t, e, 3)
		want := []domain.SessionState{domain.SessionRunning, domain.SessionGrace, domain.SessionRunning}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("states = %v, want %v", got, want)
			}
		}
	})
}

// tick moves the clock one watch period and waits until the notifications reach
// want. The watcher re-arms its timer after each snapshot, so a tick can land
// before the timer exists: tick repeats (a few periods at most) until the snapshot
// is taken (RUL-002).
func tick(t *testing.T, e *env, want int) {
	t.Helper()
	for range 8 {
		e.clock.Advance(watchEvery)
		deadline := time.Now().Add(20 * time.Millisecond)
		for time.Now().Before(deadline) {
			if e.rec.count(opNotify) >= want {
				return
			}
			runtime.Gosched()
		}
	}
	t.Fatalf("no notification %d after the watch ticks", want)
}
