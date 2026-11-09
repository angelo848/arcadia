//go:build windows

package tracker_test

import (
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/arcadia-launcher/arcadia/internal/tracker"
)

// treeFlags: initial process plus one child, both alive until the until-file.
var treeFlags = []string{"--depth", "1", "--parent-run", "forever", "--run", "forever"}

// prepare runs the real Launcher.Prepare for fake-game in sc.dir and closes
// the job in t.Cleanup (before the scenario cleanup stops the processes).
// testJob is a prepared job plus what resume needs to register its tree in
// the scenario.
type testJob struct {
	tracker.Job
	sc   *scenario
	argv []string
}

func prepare(t *testing.T, sc *scenario, flags ...string) *testJob {
	t.Helper()
	argv := sc.argv(flags...)
	job, err := tracker.NewLauncher().Prepare(tracker.LaunchSpec{
		GameID:  1,
		Title:   "fake game",
		Exe:     fakeGameExe,
		Args:    joinArgs(argv),
		WorkDir: sc.dir,
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if job == nil {
		t.Fatal("Prepare returned a nil job without error")
	}
	t.Cleanup(func() {
		if err := job.Close(); err != nil {
			t.Errorf("cleanup Job.Close: %v", err)
		}
	})
	return &testJob{Job: job, sc: sc, argv: argv}
}

// resume resumes the initial process; from then on the scenario cleanup waits
// for the whole tree.
func resume(t *testing.T, job *testJob) {
	t.Helper()
	if err := job.Resume(); err != nil {
		_ = job.Terminate()
		t.Fatalf("Resume: %v", err)
	}
	job.sc.expect(job.argv)
}

// collectNewProcess reads job events until n EventNewProcess arrived and
// returns their PIDs.
func collectNewProcess(t *testing.T, job tracker.Job, n int) []uint32 {
	t.Helper()
	var pids []uint32
	timeout := time.After(realWait)
	for len(pids) < n {
		select {
		case ev, ok := <-job.Events():
			if !ok {
				t.Fatalf("job events closed after %d new process event(s), want %d", len(pids), n)
			}
			if ev.Kind == tracker.EventNewProcess {
				pids = append(pids, ev.PID)
			}
		case <-timeout:
			t.Fatalf("timeout waiting for %d EventNewProcess; got %v", n, pids)
		}
	}
	return pids
}

// rootAndChild waits for the two start lines of treeFlags and returns them.
func rootAndChild(t *testing.T, sc *scenario) (root, child reportLine) {
	t.Helper()
	lines := sc.waitCounts(t, 2, 0)
	roots, children := linesOf(lines, "start", 1), linesOf(lines, "start", 0)
	if len(roots) != 1 || len(children) != 1 {
		t.Fatalf("start lines = %+v, want one with depth 1 and one with depth 0", lines)
	}
	return roots[0], children[0]
}

func TestJobTree(t *testing.T) {
	t.Parallel()

	t.Run("TRK-001/processo inicial e filho estão no job (IsProcessInJob)", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		job := prepare(t, sc, treeFlags...)
		resume(t, job)
		root, child := rootAndChild(t, sc)
		if uint32(root.PID) != job.RootPID() {
			t.Errorf("initial process pid = %d, want RootPID %d", root.PID, job.RootPID())
		}
		if child.PPID != root.PID {
			t.Errorf("child ppid = %d, want %d", child.PPID, root.PID)
		}
		if testerInJob(t) {
			t.Log("test process runs inside a job: IsProcessInJob(NULL) is only a weak check; ActiveProcesses decides")
		}
		for _, l := range []reportLine{root, child} {
			if !inAnyJob(t, openProcess(t, l.PID)) {
				t.Errorf("pid %d (depth %d): IsProcessInJob = false, want true", l.PID, l.Depth)
			}
		}
		n, err := job.ActiveProcesses()
		if err != nil {
			t.Fatalf("ActiveProcesses: %v", err)
		}
		if n != 2 {
			t.Errorf("ActiveProcesses = %d, want 2 (initial process and child in the job)", n)
		}
	})

	t.Run("TRK-001/job emite 2 EventNewProcess", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		job := prepare(t, sc, treeFlags...)
		resume(t, job)
		pids := collectNewProcess(t, job, 2)
		root, child := rootAndChild(t, sc)
		want := []uint32{uint32(root.PID), uint32(child.PID)}
		slices.Sort(want)
		slices.Sort(pids)
		if !slices.Equal(pids, want) {
			t.Errorf("EventNewProcess pids = %v, want %v (initial process and child)", pids, want)
		}
		// No further process may be reported while the tree is unchanged.
		for {
			select {
			case ev := <-job.Events():
				if ev.Kind == tracker.EventNewProcess {
					t.Errorf("unexpected extra EventNewProcess %+v", ev)
				}
				continue
			default:
			}
			break
		}
	})
}

func TestLaunchFailureWindows(t *testing.T) {
	t.Parallel()

	t.Run("SES-012/exe inexistente devolve LaunchError com WinError 2 e não deixa processo", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "no such game.exe")

		// Launcher: no job (nothing was created), Windows code ERROR_FILE_NOT_FOUND.
		job, err := tracker.NewLauncher().Prepare(tracker.LaunchSpec{GameID: 1, Title: "missing", Exe: missing})
		if job != nil {
			_ = job.Close()
			t.Errorf("Prepare returned a job for a missing exe")
		}
		var le *tracker.LaunchError
		if !errors.As(err, &le) {
			t.Fatalf("Prepare error = %v (%T), want *tracker.LaunchError", err, err)
		}
		if le.WinError != 2 {
			t.Errorf("Prepare WinError = %d, want 2", le.WinError)
		}

		// Tracker: same error, nothing recorded, no notification, no session.
		w := newWinEnv(t)
		_, err = w.tr.Launch(realCtx(t), tracker.LaunchSpec{GameID: w.game.ID, Title: w.game.Title, Exe: missing})
		le = nil
		if !errors.As(err, &le) {
			t.Fatalf("Launch error = %v (%T), want *tracker.LaunchError", err, err)
		}
		if le.WinError != 2 {
			t.Errorf("Launch WinError = %d, want 2", le.WinError)
		}
		open, err := w.st.OpenSessions(realCtx(t))
		if err != nil {
			t.Fatalf("OpenSessions: %v", err)
		}
		if len(open) != 0 {
			t.Errorf("open sessions = %+v, want none", open)
		}
		if _, ok := w.tr.Current(); ok {
			t.Error("Current reports a session after a failed launch")
		}
		if ns := w.notifications(); len(ns) != 0 {
			t.Errorf("notifications = %+v, want none", ns)
		}
		rec := w.waitLog(t, slog.LevelError, msgLaunchFailed, nil)
		wantAttrs(t, rec, map[string]string{"path": missing})
	})

	t.Run("SES-012/Terminate antes do Resume encerra o processo suspenso", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		job := prepare(t, sc, "--run", "forever")
		h := openProcess(t, int(job.RootPID()))
		if !isRunning(t, h) {
			t.Fatal("suspended initial process is not alive before Terminate")
		}
		if err := job.Terminate(); err != nil {
			t.Fatalf("Terminate: %v", err)
		}
		waitProcessExit(t, h, "terminated initial process")
		if code := exitCodeOf(t, h); code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		lines, err := readReport(sc.report)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) != 0 {
			t.Errorf("report = %+v, want empty (the process never ran)", lines)
		}
	})
}

func TestActiveProcesses(t *testing.T) {
	t.Parallel()

	t.Run("SES-013/ActiveProcesses conta os vivos e chega a 0", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		job := prepare(t, sc, treeFlags...)
		if n, err := job.ActiveProcesses(); err != nil || n != 1 {
			t.Errorf("ActiveProcesses before Resume = %d, %v, want 1 (suspended initial process)", n, err)
		}
		resume(t, job)
		rootAndChild(t, sc)
		if n, err := job.ActiveProcesses(); err != nil || n != 2 {
			t.Errorf("ActiveProcesses with the tree alive = %d, %v, want 2", n, err)
		}

		sc.createUntil(t)
		deadline := time.After(realWait)
		tick := time.NewTicker(pollEvery)
		defer tick.Stop()
		for {
			n, err := job.ActiveProcesses()
			if err != nil {
				t.Fatalf("ActiveProcesses: %v", err)
			}
			if n == 0 {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("ActiveProcesses = %d after %v with the until-file, want 0", n, realWait)
			case <-tick.C:
			}
		}
		sc.waitCounts(t, 2, 2)
	})
}

// TestJobGoroutines is not parallel: goleak inspects every goroutine of the
// process.
func TestJobGoroutines(t *testing.T) {
	t.Run("TRK-008/Close do job espera a leitora da IOCP", func(t *testing.T) {
		ignore := goleak.IgnoreCurrent()
		sc := newScenario(t)
		job := prepare(t, sc, "--run", "forever")
		resume(t, job)
		collectNewProcess(t, job, 1) // the reader is running
		// NEW_PROCESS is posted at AssignProcessToJobObject, before Resume: wait
		// until fake-game really runs before closing.
		sc.waitCounts(t, 1, 0)

		if err := job.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		timeout := time.After(realWait)
	drain:
		for {
			select {
			case _, ok := <-job.Events():
				if !ok {
					break drain
				}
			case <-timeout:
				t.Fatal("Events not closed after Close")
			}
		}
		goleak.VerifyNone(t, ignore)

		if err := job.Close(); err != nil {
			t.Errorf("second Close: %v", err)
		}
	})
}
