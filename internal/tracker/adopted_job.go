package tracker

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// ErrNotOwned is returned by adoptedJob.Terminate: the agent did not start the
// adopted process and must not kill it (RUL-005).
var ErrNotOwned = errors.New("tracker: adopted process is not owned by the agent")

// adoptedJob follows the game processes of a URI launch that the agent did not
// create (RUL-002). It implements Job without a Job Object: a watcher goroutine,
// owned by the job, takes a process snapshot every interval and reports the
// matching processes as job events. The processes running before the launch are
// the baseline and are never the game (RUL-002). Close stops the watcher and waits
// for it; the event channel is closed by the watcher on the way out.
type adoptedJob struct {
	lister   ProcessLister
	rules    []Rule
	clock    domain.Clock
	interval time.Duration
	baseline map[uint32]bool

	out       chan JobEvent
	stop      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once

	mu   sync.Mutex
	live map[uint32]bool // matching processes seen in the last snapshot
	root uint32          // first matching process, 0 until one is seen
}

// newAdoptedJob takes the baseline snapshot and starts the watcher. A failing
// baseline is an error: without it, processes from before the launch could match.
func newAdoptedJob(lister ProcessLister, rules []Rule, clock domain.Clock, interval time.Duration) (*adoptedJob, error) {
	procs, err := lister.List()
	if err != nil {
		return nil, fmt.Errorf("tracker: baseline snapshot: %w", err)
	}
	base := make(map[uint32]bool, len(procs))
	for _, p := range procs {
		base[p.PID] = true
	}
	j := &adoptedJob{
		lister:   lister,
		rules:    rules,
		clock:    clock,
		interval: interval,
		baseline: base,
		out:      make(chan JobEvent, 16),
		stop:     make(chan struct{}),
		live:     map[uint32]bool{},
	}
	j.wg.Add(1)
	go j.run()
	return j, nil
}

// run is the watcher loop. It returns when stop is closed, closing out on the way.
func (j *adoptedJob) run() {
	defer j.wg.Done()
	defer close(j.out)
	for {
		select {
		case <-j.stop:
			return
		case <-j.clock.After(j.interval):
		}
		if !j.scan() {
			return
		}
	}
}

// scan takes one snapshot and reports the changes. It returns false when stop
// closed while an event was waiting to be delivered.
func (j *adoptedJob) scan() bool {
	procs, err := j.lister.List()
	if err != nil {
		return true // a failed snapshot changes nothing; the next one will tell
	}
	now := map[uint32]bool{}
	for _, p := range procs {
		if !j.baseline[p.PID] && matchesAny(p, j.rules) {
			now[p.PID] = true
		}
	}

	j.mu.Lock()
	prev := j.live
	j.live = now
	var events []JobEvent
	for _, pid := range slices.Sorted(maps.Keys(now)) {
		if !prev[pid] {
			if j.root == 0 {
				j.root = pid
			}
			events = append(events, JobEvent{Kind: EventNewProcess, PID: pid, Root: pid == j.root})
		}
	}
	for _, pid := range slices.Sorted(maps.Keys(prev)) {
		if !now[pid] {
			events = append(events, JobEvent{Kind: EventExitProcess, PID: pid, Root: pid == j.root})
		}
	}
	if len(prev) > 0 && len(now) == 0 {
		events = append(events, JobEvent{Kind: EventActiveZero})
	}
	j.mu.Unlock()

	for _, ev := range events {
		select {
		case j.out <- ev:
		case <-j.stop:
			return false
		}
	}
	return true
}

// RootPID returns the first matching process, or 0 before one is seen.
func (j *adoptedJob) RootPID() uint32 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.root
}

// Resume does nothing: the adopted process is already running.
func (j *adoptedJob) Resume() error { return nil }

// Terminate refuses: the agent does not own the adopted process (RUL-005).
func (j *adoptedJob) Terminate() error { return ErrNotOwned }

// Events returns the watcher notifications.
func (j *adoptedJob) Events() <-chan JobEvent { return j.out }

// ActiveProcesses returns the number of matching processes in the last snapshot.
func (j *adoptedJob) ActiveProcesses() (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.live), nil
}

// Close stops the watcher and waits for it. It never kills processes and is
// idempotent.
func (j *adoptedJob) Close() error {
	j.closeOnce.Do(func() { close(j.stop) })
	j.wg.Wait()
	return nil
}
