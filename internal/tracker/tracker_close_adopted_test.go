package tracker_test

import (
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

type fakeWinCloser struct {
	mu   sync.Mutex
	n    int
	sets [][]uint32
}

func (c *fakeWinCloser) PostClose(pids map[uint32]struct{}) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sets = append(c.sets, slices.Sorted(maps.Keys(pids)))
	return c.n, nil
}

func (c *fakeWinCloser) last() []uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sets) == 0 {
		return nil
	}
	return c.sets[len(c.sets)-1]
}

type fakeKiller struct {
	mu    sync.Mutex
	calls map[uint32]string
	errs  map[uint32]error
}

func (k *fakeKiller) Kill(pid uint32, want string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.calls == nil {
		k.calls = map[uint32]string{}
	}
	k.calls[pid] = want
	return k.errs[pid]
}

func (k *fakeKiller) pids() []uint32 {
	k.mu.Lock()
	defer k.mu.Unlock()
	return slices.Sorted(maps.Keys(k.calls))
}

var (
	helperProc = tracker.ProcInfo{PID: 4243, Path: `C:\Games\CS2\helper.exe`}
	otherProc  = tracker.ProcInfo{PID: 9, Path: `D:\Other\launcher.exe`}
)

// adoptedRunning launches the URI game and adopts gameProc (+ extra); it
// returns once the session is running.
func adoptedRunning(t *testing.T, closer *fakeWinCloser, killer *fakeKiller, extra ...tracker.ProcInfo) (*env, *fakeLister) {
	t.Helper()
	lister := &fakeLister{}
	lister.set(explorerProc)
	e := newEnv(t, envConfig{
		heartbeat: time.Hour, poll: time.Hour, procs: lister, watch: watchEvery,
		rules: gameRules, closer: closer, killer: killer,
	})
	if _, err := e.tryLaunch(t, uriSpec); err != nil {
		t.Fatalf("Launch(uri): %v", err)
	}
	lister.set(append([]tracker.ProcInfo{explorerProc, gameProc, otherProc}, extra...)...)
	tick(t, e, 1)
	if got := stateSeq(t, e, 1); got[0] != domain.SessionRunning {
		t.Fatalf("state = %q, want running", got[0])
	}
	return e, lister
}

func TestTrackerCloseAdopted(t *testing.T) {
	t.Parallel()

	t.Run("CON-008/sessão adotada: o conjunto vazio de janelas mata na hora só quem casa, conferindo o caminho", func(t *testing.T) {
		t.Parallel()
		closer, killer := &fakeWinCloser{n: 0}, &fakeKiller{}
		e, _ := adoptedRunning(t, closer, killer, helperProc)
		wantClosed(t, waitClose(t, closeAsync(t, e, e.clock.Now())))
		if got := closer.last(); !slices.Equal(got, []uint32{4242, 4243}) {
			t.Errorf("RequestClose pids = %v, want [4242 4243] (explorer and the unrelated process excluded)", got)
		}
		if got := killer.pids(); !slices.Equal(got, []uint32{4242, 4243}) {
			t.Errorf("Kill pids = %v, want [4242 4243]", got)
		}
		if killer.calls[4242] != gameProc.Path || killer.calls[4243] != helperProc.Path {
			t.Errorf("Kill image checks = %v, want the matched paths", killer.calls)
		}
		e.rec.waitN(t, opEnd, 1)
		if e.rec.of(opEnd)[0].end.Reason != domain.EndClosedByHotkey {
			t.Errorf("end reason = %q, want closed-by-hotkey", e.rec.of(opEnd)[0].end.Reason)
		}
	})

	t.Run("CON-008/o prazo mata só quem ainda casa e ainda está vivo", func(t *testing.T) {
		t.Parallel()
		closer, killer := &fakeWinCloser{n: 1}, &fakeKiller{}
		e, lister := adoptedRunning(t, closer, killer, helperProc)
		ch := closeAsync(t, e, e.clock.Now())
		e.rec.waitN(t, opNotify, 1)
		lister.set(explorerProc, gameProc, otherProc) // the helper exits during the wait
		e.waiters(t, 5)
		e.clock.Advance(closeWait)
		wantClosed(t, waitClose(t, ch))
		if got := killer.pids(); !slices.Equal(got, []uint32{4242}) {
			t.Errorf("Kill pids = %v, want [4242] only", got)
		}
	})

	t.Run("CON-008/saída durante a espera termina sem Kill", func(t *testing.T) {
		t.Parallel()
		closer, killer := &fakeWinCloser{n: 1}, &fakeKiller{}
		e, lister := adoptedRunning(t, closer, killer)
		ch := closeAsync(t, e, e.clock.Now())
		e.waiters(t, 5)
		lister.set(explorerProc, otherProc) // the game exits
		tick(t, e, 2)
		wantClosed(t, waitClose(t, ch))
		if got := killer.pids(); len(got) != 0 {
			t.Errorf("Kill pids = %v, want none", got)
		}
	})

	t.Run("CON-013/só parte dos processos adotados morreu: failed, a sessão segue e o watcher encerra", func(t *testing.T) {
		t.Parallel()
		closer := &fakeWinCloser{n: 1}
		killer := &fakeKiller{errs: map[uint32]error{4243: errKill}}
		e, lister := adoptedRunning(t, closer, killer, helperProc)
		ch := closeAsync(t, e, e.clock.Now())
		e.waiters(t, 5)
		e.clock.Advance(closeWait)
		wantFailed(t, waitClose(t, ch))
		wantNoCalls(t, e, opEnd)
		e.waitCurrentState(t, domain.SessionRunning)
		lister.set(explorerProc, otherProc) // the rest exits by itself
		tick(t, e, 2)
		if got := stateSeq(t, e, 1); got[0] != domain.SessionGrace {
			t.Errorf("state after the watcher sees the end = %q, want grace", got[0])
		}
	})
}
