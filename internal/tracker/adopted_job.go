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
	lister    ProcessLister
	rules     []Rule
	clock     domain.Clock
	interval  time.Duration
	baseline  map[uint32]bool
	steam     func() (running, present bool) // Steam signal of the app, nil = none (STM-001)
	steamSeen bool                           // Steam reported the app running at least once

	out       chan JobEvent
	stop      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once

	mu     sync.Mutex
	live   map[uint32]bool // matching processes seen in the last snapshot
	root   uint32          // first matching process, 0 until one is seen
	closer windowCloser    // RequestClose (CON-008, CON-030)
	killer processKiller   // Kill (CON-008)
}

// newAdoptedJob takes the baseline snapshot and starts the watcher. A failing
// baseline is an error: without it, processes from before the launch could match.
func newAdoptedJob(lister ProcessLister, rules []Rule, clock domain.Clock, interval time.Duration,
	steam func() (running, present bool)) (*adoptedJob, error) {
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
		steam:    steam,
		out:      make(chan JobEvent, 16),
		stop:     make(chan struct{}),
		live:     map[uint32]bool{},
		closer:   newWindowCloser(),
		killer:   newProcessKiller(),
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
	stopped := j.steamStopped()
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
	// The Steam stop comes first: it moves the session to a zero grace before the
	// exits can arm the 15 s grace of ActiveZero (STM-001).
	if stopped {
		events = append(events, JobEvent{Kind: EventSteamStopped})
	}
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

// setClosers replaces the window closer and the process killer (tests).
func (j *adoptedJob) setClosers(c windowCloser, k processKiller) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.closer, j.killer = c, k
}

// targets takes a fresh snapshot and returns the adopted processes still alive
// that still match a rule by their path, with that path (CON-008). A process
// the watcher never adopted (baseline, store client, newcomer) is never a
// target; a recycled PID no longer matches.
func (j *adoptedJob) targets() (map[uint32]string, windowCloser, processKiller, error) {
	procs, err := j.lister.List()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("tracker: process snapshot: %w", err)
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	set := map[uint32]string{}
	for _, p := range procs {
		if j.live[p.PID] && matchesAny(p, j.rules) {
			set[p.PID] = p.Path
		}
	}
	return set, j.closer, j.killer, nil
}

// RequestClose posts WM_CLOSE to the windows of the adopted processes still
// alive and matching (CON-008, CON-030). An empty set posts nothing.
func (j *adoptedJob) RequestClose() (int, error) {
	set, closer, _, err := j.targets()
	if err != nil {
		return 0, err
	}
	if len(set) == 0 {
		return 0, nil
	}
	pids := make(map[uint32]struct{}, len(set))
	for pid := range set {
		pids[pid] = struct{}{}
	}
	return closer.PostClose(pids)
}

// Kill terminates the adopted processes still alive and matching, each one only
// if its image is still the snapshot path (CON-008). A failure does not stop
// the others; the errors are joined.
func (j *adoptedJob) Kill() error {
	set, _, killer, err := j.targets()
	if err != nil {
		return err
	}
	var errs []error
	for _, pid := range slices.Sorted(maps.Keys(set)) {
		if err := killer.Kill(pid, set[pid]); err != nil {
			errs = append(errs, fmt.Errorf("kill adopted process %d: %w", pid, err))
		}
	}
	return errors.Join(errs...)
}

// Close stops the watcher and waits for it. It never kills processes and is
// idempotent.
func (j *adoptedJob) Close() error {
	j.closeOnce.Do(func() { close(j.stop) })
	j.wg.Wait()
	return nil
}

// steamStopped reports whether Steam went from running to stopped for the app
// (STM-001). A missing key or an error is no signal; the first running reading only
// arms the detection, so an app that Steam never ran does not count as stopped.
func (j *adoptedJob) steamStopped() bool {
	if j.steam == nil {
		return false
	}
	running, present := j.steam()
	if !present {
		return false
	}
	if running {
		j.steamSeen = true
		return false
	}
	if j.steamSeen {
		j.steamSeen = false
		return true
	}
	return false
}
