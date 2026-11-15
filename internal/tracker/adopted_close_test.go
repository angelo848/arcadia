package tracker

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// scriptedLister is a concurrency-safe snapshot source the test rewrites.
type scriptedLister struct {
	mu    sync.Mutex
	procs []ProcInfo
}

func (l *scriptedLister) set(p ...ProcInfo) {
	l.mu.Lock()
	l.procs = p
	l.mu.Unlock()
}

func (l *scriptedLister) List() ([]ProcInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.procs), nil
}

type recordedCloser struct {
	mu   sync.Mutex
	sets []map[uint32]struct{}
	n    int
	err  error
}

func (c *recordedCloser) PostClose(pids map[uint32]struct{}) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sets = append(c.sets, maps.Clone(pids))
	return c.n, c.err
}

func (c *recordedCloser) lastPIDs() []uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sets) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(c.sets[len(c.sets)-1]))
}

type killCall struct {
	pid  uint32
	want string
}

type recordedKiller struct {
	mu    sync.Mutex
	calls []killCall
	errs  map[uint32]error
}

func (k *recordedKiller) Kill(pid uint32, wantImage string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.calls = append(k.calls, killCall{pid, wantImage})
	return k.errs[pid]
}

func (k *recordedKiller) pids() []uint32 {
	k.mu.Lock()
	defer k.mu.Unlock()
	var out []uint32
	for _, c := range k.calls {
		out = append(out, c.pid)
	}
	slices.Sort(out)
	return out
}

const (
	gamePath1 = `C:\Games\X\g.exe`
	gamePath2 = `C:\Games\X\helper\g.exe`
	storePath = `C:\Program Files\Store\store.exe`
)

type adoptedEnv struct {
	clock  *domain.FakeClock
	lister *scriptedLister
	job    *adoptedJob
	closer *recordedCloser
	killer *recordedKiller
}

// newAdoptedEnv adopts g.exe processes 10 and 11 (a store client, pid 12, runs
// too but matches no rule) and consumes the two NewProcess events.
func newAdoptedEnv(t *testing.T) *adoptedEnv {
	t.Helper()
	e := &adoptedEnv{
		clock:  domain.NewFakeClock(time.Date(2026, time.November, 12, 18, 0, 0, 0, time.UTC)),
		lister: &scriptedLister{},
		closer: &recordedCloser{n: 2},
		killer: &recordedKiller{errs: map[uint32]error{}},
	}
	j, err := newAdoptedJob(e.lister, []Rule{{Kind: RuleExeName, Value: "g.exe"}}, e.clock, time.Second, nil)
	if err != nil {
		t.Fatalf("newAdoptedJob: %v", err)
	}
	t.Cleanup(func() { _ = j.Close() })
	e.job = j
	j.setClosers(e.closer, e.killer)
	e.lister.set(
		ProcInfo{PID: 10, Path: gamePath1},
		ProcInfo{PID: 11, Path: gamePath2},
		ProcInfo{PID: 12, Path: storePath},
	)
	e.tick(t)
	e.wantEvent(t, EventNewProcess)
	e.wantEvent(t, EventNewProcess)
	return e
}

func (e *adoptedEnv) tick(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := e.clock.BlockUntilWaiters(ctx, 1); err != nil {
		t.Fatalf("BlockUntilWaiters: %v", err)
	}
	e.clock.Advance(time.Second)
}

func (e *adoptedEnv) wantEvent(t *testing.T, kind JobEventKind) JobEvent {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-e.job.Events():
			if !ok {
				t.Fatalf("events closed while waiting for kind %v", kind)
			}
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("timeout waiting for event kind %v", kind)
		}
	}
}

func TestAdoptedJobClose(t *testing.T) {
	t.Parallel()

	t.Run("CON-008/o conjunto a fechar é o dos adotados vivos que ainda casam com uma regra pelo caminho (snapshot novo)", func(t *testing.T) {
		t.Parallel()
		e := newAdoptedEnv(t)
		// Fresh snapshot, no clock tick: pid 11 left, pid 13 is new and matches
		// but was never adopted (not in live).
		e.lister.set(
			ProcInfo{PID: 10, Path: gamePath1},
			ProcInfo{PID: 12, Path: storePath},
			ProcInfo{PID: 13, Path: gamePath1},
		)
		n, err := e.job.RequestClose()
		if err != nil || n != 2 {
			t.Fatalf("RequestClose = (%d, %v), want (2, nil) from the closer", n, err)
		}
		if got := e.closer.lastPIDs(); !slices.Equal(got, []uint32{10}) {
			t.Errorf("closer set = %v, want [10]", got)
		}
	})

	t.Run("CON-008/processo fora das regras e o cliente da loja nunca recebem WM_CLOSE nem morrem", func(t *testing.T) {
		t.Parallel()
		e := newAdoptedEnv(t)
		if _, err := e.job.RequestClose(); err != nil {
			t.Fatalf("RequestClose: %v", err)
		}
		if got := e.closer.lastPIDs(); slices.Contains(got, 12) {
			t.Errorf("closer set %v contains the store client 12", got)
		}
		if err := e.job.Kill(); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		if got := e.killer.pids(); !slices.Equal(got, []uint32{10, 11}) {
			t.Errorf("killed = %v, want [10 11]", got)
		}
	})

	t.Run("CON-008/PID reaproveitado (caminho diferente do snapshot) não é morto", func(t *testing.T) {
		t.Parallel()
		e := newAdoptedEnv(t)
		// pid 10 was reused by an unrelated program: it no longer matches.
		e.lister.set(
			ProcInfo{PID: 10, Path: `C:\Windows\notepad.exe`},
			ProcInfo{PID: 11, Path: gamePath2},
		)
		if err := e.job.Kill(); err != nil {
			t.Fatalf("Kill: %v", err)
		}
		if got := e.killer.pids(); !slices.Equal(got, []uint32{11}) {
			t.Errorf("killed = %v, want [11]", got)
		}
		e.killer.mu.Lock()
		defer e.killer.mu.Unlock()
		if e.killer.calls[0].want != gamePath2 {
			t.Errorf("wantImage = %q, want the snapshot path %q", e.killer.calls[0].want, gamePath2)
		}
	})

	t.Run("CON-008/conjunto adotado vazio: RequestClose 0 e Kill nil", func(t *testing.T) {
		t.Parallel()
		e := newAdoptedEnv(t)
		e.lister.set(ProcInfo{PID: 12, Path: storePath})
		n, err := e.job.RequestClose()
		if err != nil || n != 0 {
			t.Errorf("RequestClose = (%d, %v), want (0, nil)", n, err)
		}
		if err := e.job.Kill(); err != nil {
			t.Errorf("Kill = %v, want nil", err)
		}
		if got := e.killer.pids(); len(got) != 0 {
			t.Errorf("killed = %v, want none", got)
		}
	})

	t.Run("CON-008/saída durante a espera: o aviso de nenhum adotado vivo (EventActiveZero) chega pelo canal", func(t *testing.T) {
		t.Parallel()
		e := newAdoptedEnv(t)
		if _, err := e.job.RequestClose(); err != nil {
			t.Fatalf("RequestClose: %v", err)
		}
		e.lister.set(ProcInfo{PID: 12, Path: storePath})
		e.tick(t)
		e.wantEvent(t, EventActiveZero)
		if n, _ := e.job.ActiveProcesses(); n != 0 {
			t.Errorf("ActiveProcesses = %d, want 0", n)
		}
	})

	t.Run("CON-008/erro de Kill de um processo não impede o dos outros e vira erro juntado", func(t *testing.T) {
		t.Parallel()
		e := newAdoptedEnv(t)
		boom := errors.New("access denied")
		e.killer.errs[10] = boom
		err := e.job.Kill()
		if !errors.Is(err, boom) {
			t.Errorf("Kill = %v, want an error wrapping the failure of pid 10", err)
		}
		if got := e.killer.pids(); !slices.Equal(got, []uint32{10, 11}) {
			t.Errorf("killer called for %v, want both [10 11]", got)
		}
	})
}
