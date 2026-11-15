//go:build windows

package tracker_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
)

// launcherFlags is the launcher pattern: the initial process starts the game
// and exits at once; the child lives until the until-file.
var launcherFlags = []string{"--depth", "1", "--parent-run", "0", "--run", "forever"}

func TestTrackerIntegration(t *testing.T) {
	t.Parallel()

	// launcherExited launches launcherFlags and waits until the initial process
	// exited (report and SES-015 log) while the child is alive.
	launcherExited := func(t *testing.T, w *winEnv, sc *scenario) (child reportLine) {
		t.Helper()
		w.launch(t, sc, launcherFlags...)
		lines := sc.waitReport(t, "child start and launcher exit", func(ls []reportLine) bool {
			return len(linesOf(ls, "start", 0)) == 1 && len(linesOf(ls, "exit", 1)) == 1
		})
		root := linesOf(lines, "start", 1)
		if len(root) != 1 {
			t.Fatalf("start lines = %+v, want one with depth 1", lines)
		}
		w.waitRootExit(t, root[0].PID)
		return linesOf(lines, "start", 0)[0]
	}

	t.Run("TRK-002/lançador sai e a sessão continua Running", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		child := launcherExited(t, w, sc)
		if !isRunning(t, openProcess(t, child.PID)) {
			t.Fatalf("child %d is not alive", child.PID)
		}
		if ns := w.notifications(); len(ns) != 1 {
			t.Errorf("notifications = %+v, want only the running one", ns)
		}
		w.wantCurrent(t, domain.SessionRunning)
	})

	t.Run("TRK-002/criar F faz a sessão ir a Grace", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		child := launcherExited(t, w, sc)
		h := openProcess(t, child.PID)
		sc.createUntil(t)
		n := w.nextNotify(t)
		if n.State != domain.SessionGrace {
			t.Fatalf("notification after F = %q, want grace", n.State)
		}
		waitProcessExit(t, h, "child")
		w.wantCurrent(t, domain.SessionGrace)
	})

	t.Run("TRK-003/árvore de 7 processos continua Running até a última folha sair", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		w.launch(t, sc, "--depth", "2", "--fanout", "2", "--parent-run", "0", "--run", "forever")
		lines := sc.waitReport(t, "7 starts and the 3 intermediate exits", func(ls []reportLine) bool {
			return len(linesOf(ls, "start", -1)) == 7 && len(linesOf(ls, "exit", 2))+len(linesOf(ls, "exit", 1)) == 3
		})
		root := linesOf(lines, "start", 2)
		if len(root) != 1 {
			t.Fatalf("start lines = %+v, want one with depth 2", lines)
		}
		w.waitRootExit(t, root[0].PID)
		var leaves []windows.Handle
		for _, l := range linesOf(lines, "start", 0) {
			leaves = append(leaves, openProcess(t, l.PID))
		}
		if len(leaves) != 4 {
			t.Fatalf("leaves = %d, want 4", len(leaves))
		}
		for i, h := range leaves {
			if !isRunning(t, h) {
				t.Fatalf("leaf %d is not alive", i)
			}
		}
		if ns := w.notifications(); len(ns) != 1 {
			t.Fatalf("notifications with the 4 leaves alive = %+v, want only the running one", ns)
		}
		w.wantCurrent(t, domain.SessionRunning)

		sc.createUntil(t)
		for i, h := range leaves {
			waitProcessExit(t, h, fmt.Sprintf("leaf %d", i))
		}
		if n := w.nextNotify(t); n.State != domain.SessionGrace {
			t.Fatalf("notification after the last leaf = %q, want grace", n.State)
		}
	})

	t.Run("TRK-005/cwd = D, args de launch_args sem alteração e exe com espaços entre aspas", func(t *testing.T) {
		t.Parallel()
		if !strings.Contains(fakeGameExe, " ") {
			t.Fatalf("fake-game path %q has no space; TestMain must build it into a path with spaces", fakeGameExe)
		}
		sc := newScenario(t)
		workDir := filepath.Join(t.TempDir(), "work dir")
		if err := os.Mkdir(workDir, 0o755); err != nil {
			t.Fatal(err)
		}
		w := newWinEnv(t)
		argv := sc.argv("--run", "0", "--", "a", "b c", `C:\x y\z.rom`)
		w.launchArgv(t, sc, argv, workDir)
		lines := sc.waitCounts(t, 1, 0)
		st := linesOf(lines, "start", 0)
		if len(st) != 1 {
			t.Fatalf("start lines = %+v, want 1", lines)
		}
		if !strings.EqualFold(filepath.Clean(st[0].Cwd), filepath.Clean(workDir)) {
			t.Errorf("cwd = %q, want %q", st[0].Cwd, workDir)
		}
		// An unquoted exe path would be split by the command-line parser and its
		// pieces would show up in args.
		if !slices.Equal(st[0].Args, argv) {
			t.Errorf("args = %q, want %q", st[0].Args, argv)
		}
	})

	t.Run("TRK-006/filho com breakaway é criado fora do job e a sessão vai a Grace quando o inicial sai", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		w.launch(t, sc, "--depth", "1", "--breakaway", "--parent-run", "2s", "--run", "forever")
		lines := sc.waitReport(t, "breakaway child start", func(ls []reportLine) bool {
			return len(linesOf(ls, "start", 0)) == 1 || len(linesOf(ls, "exit", 1)) == 1
		})
		children := linesOf(lines, "start", 0)
		if len(children) != 1 {
			t.Fatalf("breakaway child was not created; report %+v (test process in a job: %v)", lines, testerInJob(t))
		}
		child := openProcess(t, children[0].PID)
		if !testerInJob(t) && inAnyJob(t, child) {
			t.Errorf("breakaway child %d is in a job, want outside", children[0].PID)
		}

		n := w.nextNotify(t)
		if n.State != domain.SessionGrace {
			t.Fatalf("notification after the initial process exits = %q, want grace", n.State)
		}
		if !isRunning(t, child) {
			t.Errorf("breakaway child %d exited, want alive while the session is in Grace", children[0].PID)
		}
		lines, err := readReport(sc.report)
		if err != nil {
			t.Fatal(err)
		}
		exits := linesOf(lines, "exit", 1)
		if len(exits) != 1 || exits[0].Code == nil || *exits[0].Code != 0 || exits[0].Error != "" {
			t.Errorf("initial process exit lines = %+v, want one with code 0 and no error", exits)
		}
	})

	t.Run("SES-015/--exit-code 7 loga exit_code 7", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		snap := w.launch(t, sc, "--run", "0", "--exit-code", "7")
		lines := sc.waitCounts(t, 1, 1)
		st := linesOf(lines, "start", 0)
		if len(st) != 1 {
			t.Fatalf("start lines = %+v, want 1", lines)
		}
		rec := w.waitRootExit(t, st[0].PID)
		wantAttrs(t, rec, map[string]string{
			"session_id": fmt.Sprint(snap.ID),
			"pid":        fmt.Sprint(st[0].PID),
			"exit_code":  "7",
		})
	})
}

func TestTrackerCloseKeepsGame(t *testing.T) {
	t.Parallel()

	t.Run("TRK-004/depois de Close todos os processos continuam vivos (GetExitCodeProcess = STILL_ACTIVE)", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		w.launch(t, sc, "--depth", "1", "--fanout", "2", "--parent-run", "forever", "--run", "forever")
		lines := sc.waitCounts(t, 3, 0)
		var procs []windows.Handle
		for _, l := range linesOf(lines, "start", -1) {
			procs = append(procs, openProcess(t, l.PID))
		}

		if err := w.tr.Close(realCtx(t)); err != nil {
			t.Fatalf("Tracker.Close: %v", err)
		}
		for i, h := range procs {
			// A short bounded wait: a kill on job close would signal the process.
			if ev, _ := windows.WaitForSingleObject(h, 300); ev != uint32(windows.WAIT_TIMEOUT) {
				t.Errorf("process %d was signalled after Close (wait = %#x)", i, ev)
			}
			if code := exitCodeOf(t, h); code != stillLive {
				t.Errorf("process %d exit code = %d after Close, want STILL_ACTIVE", i, code)
			}
		}
	})
}

// TestIntegrationPause covers TRK-009: pausing never touches the game.
func TestIntegrationPause(t *testing.T) {
	t.Parallel()

	pauseFlags := []string{"--depth", "1", "--parent-run", "forever", "--run", "forever"}

	// start launches the two-process game and returns a handle to each.
	start := func(t *testing.T, w *winEnv, sc *scenario) []windows.Handle {
		t.Helper()
		w.launch(t, sc, pauseFlags...)
		lines := sc.waitCounts(t, 2, 0)
		var procs []windows.Handle
		for _, l := range linesOf(lines, "start", -1) {
			procs = append(procs, openProcess(t, l.PID))
		}
		if len(procs) != 2 {
			t.Fatalf("processes = %d, want 2", len(procs))
		}
		return procs
	}
	wantAliveInJob := func(t *testing.T, when string, procs []windows.Handle) {
		t.Helper()
		for i, h := range procs {
			if !isRunning(t, h) || exitCodeOf(t, h) != stillLive {
				t.Errorf("%s: process %d is not alive", when, i)
			}
			// Inside a terminal/IDE job every child is "in a job"; the check
			// is only meaningful when the test process is not in one.
			if !testerInJob(t) && !inAnyJob(t, h) {
				t.Errorf("%s: process %d is outside the job", when, i)
			}
		}
	}

	t.Run("TRK-009/pausa e retomada mantêm os dois processos vivos e dentro do job (IsProcessInJob)", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		procs := start(t, w, sc)
		wantAliveInJob(t, "before the pause", procs)

		w.clock.Advance(10 * time.Second)
		if _, err := w.tr.Pause(realCtx(t), w.clock.Now()); err != nil {
			t.Fatalf("Pause: %v", err)
		}
		if n := w.nextNotify(t); n.State != domain.SessionPaused {
			t.Fatalf("notification after Pause = %q, want paused", n.State)
		}
		wantAliveInJob(t, "during the pause", procs)

		w.clock.Advance(10 * time.Second)
		if _, err := w.tr.Resume(realCtx(t), w.clock.Now()); err != nil {
			t.Fatalf("Resume: %v", err)
		}
		if n := w.nextNotify(t); n.State != domain.SessionRunning {
			t.Fatalf("notification after Resume = %q, want running", n.State)
		}
		wantAliveInJob(t, "after the pause", procs)
		w.wantCurrent(t, domain.SessionRunning)
	})

	t.Run("TRK-009/F criado durante a pausa e processos saindo: a sessão vai a Grace e termina sem perder evento do job", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		procs := start(t, w, sc)

		w.clock.Advance(10 * time.Second)
		if _, err := w.tr.Pause(realCtx(t), w.clock.Now()); err != nil {
			t.Fatalf("Pause: %v", err)
		}
		if n := w.nextNotify(t); n.State != domain.SessionPaused {
			t.Fatalf("notification after Pause = %q, want paused", n.State)
		}
		sc.createUntil(t)
		for i, h := range procs {
			waitProcessExit(t, h, fmt.Sprintf("process %d", i))
		}
		if n := w.nextNotify(t); n.State != domain.SessionGrace {
			t.Fatalf("notification after the job emptied while paused = %q, want grace", n.State)
		}
		end := w.expireGrace(t)
		if end.State != domain.SessionEnded && end.State != domain.SessionDiscarded {
			t.Errorf("final notification = %q, want ended or discarded", end.State)
		}
		if _, ok := w.tr.Current(); ok {
			t.Error("Current reports a session after the grace period")
		}
	})
}

// processesInOwnJob returns the PIDs of the job the test process runs in
// (QueryInformationJobObject with a NULL job handle); ok is false when it
// cannot be read.
func processesInOwnJob() (pids map[uint32]bool, ok bool) {
	const jobObjectBasicProcessIdList = 3
	buf := make([]uint64, 2+1024) // 2 DWORDs + ULONG_PTR list; 8-byte aligned
	var ret uint32
	err := windows.QueryInformationJobObject(0, jobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(&buf[0])), uint32(len(buf)*8), &ret)
	if err != nil && !errors.Is(err, windows.ERROR_MORE_DATA) {
		return nil, false
	}
	n := *(*uint32)(unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0])) + 4)) // NumberOfProcessIdsInList
	pids = map[uint32]bool{}
	for i := uint32(0); i < n && int(1+i) < len(buf); i++ {
		pids[uint32(buf[1+i])] = true
	}
	return pids, true
}

// TestIntegrationLaunchUntracked covers TRK-010: a game launched without
// tracking is a plain process, in no job of ours and with no session.
func TestIntegrationLaunchUntracked(t *testing.T) {
	t.Parallel()

	// untracked launches fake-game detached and makes sure that, before the
	// scenario cleanup (which runs after this one) removes the TempDir, the
	// process is released by F and gone: it is in no job, so nothing else
	// would wait for it.
	untracked := func(t *testing.T, w *winEnv, sc *scenario, argv []string, workDir string) (uint32, windows.Handle) {
		t.Helper()
		pid, err := w.tr.LaunchUntracked(realCtx(t), w.spec(argv, workDir))
		if err != nil {
			t.Fatalf("LaunchUntracked: %v", err)
		}
		sc.expect(argv)
		h := openProcess(t, int(pid))
		t.Cleanup(func() {
			sc.createUntil(t)
			if ev, _ := windows.WaitForSingleObject(h, uint32(realWait/time.Millisecond)); ev != windows.WAIT_OBJECT_0 {
				t.Errorf("cleanup: launched pid %d still running", pid)
			}
		})
		return pid, h
	}

	t.Run("TRK-010/processo roda com cwd = pasta de trabalho e os argumentos intactos", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		workDir := filepath.Join(t.TempDir(), "work dir")
		if err := os.Mkdir(workDir, 0o755); err != nil {
			t.Fatal(err)
		}
		argv := sc.argv("--run", "forever", "--", "a", "b c", `C:\x y\z.rom`)
		pid, _ := untracked(t, w, sc, argv, workDir)
		lines := sc.waitCounts(t, 1, 0)
		st := linesOf(lines, "start", 0)
		if len(st) != 1 {
			t.Fatalf("start lines = %+v, want 1", lines)
		}
		if uint32(st[0].PID) != pid {
			t.Errorf("reported pid = %d, LaunchUntracked returned %d", st[0].PID, pid)
		}
		if !strings.EqualFold(filepath.Clean(st[0].Cwd), filepath.Clean(workDir)) {
			t.Errorf("cwd = %q, want %q", st[0].Cwd, workDir)
		}
		if !slices.Equal(st[0].Args, argv) {
			t.Errorf("args = %q, want %q", st[0].Args, argv)
		}
	})

	t.Run("CON-011/o filho está sempre no job guardado: IsProcessInJob(filho) é verdadeiro, e sem KILL_ON_JOB_CLOSE ele sobrevive ao Close do job", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		_, h := untracked(t, w, sc, sc.argv("--run", "forever"), sc.dir)
		sc.waitCounts(t, 1, 0)

		if !inAnyJob(t, h) {
			t.Error("IsProcessInJob(child) = false, want true (the guarded job)")
		}
		// The flags (BREAKAWAY_OK without KILL_ON_JOB_CLOSE) cannot be read
		// without the job handle, which the tracker owns; the absence of
		// KILL_ON_JOB_CLOSE is checked by its effect: Close releases the job
		// and the game keeps running.
		if err := w.tr.Close(realCtx(t)); err != nil {
			t.Fatalf("Tracker.Close: %v", err)
		}
		if ev, _ := windows.WaitForSingleObject(h, 300); ev != uint32(windows.WAIT_TIMEOUT) {
			t.Errorf("child was signalled after the job was closed (wait = %#x)", ev)
		}
	})

	t.Run("TRK-010/nenhuma linha de play_session e o processo continua vivo depois de Tracker.Close; termina quando F existe", func(t *testing.T) {
		t.Parallel()
		sc := newScenario(t)
		w := newWinEnv(t)
		_, h := untracked(t, w, sc, sc.argv("--run", "forever"), sc.dir)
		sc.waitCounts(t, 1, 0)

		var rows int
		if err := w.st.DB().QueryRow(`SELECT COUNT(*) FROM play_session`).Scan(&rows); err != nil || rows != 0 {
			t.Errorf("play_session rows = %d (err %v), want 0", rows, err)
		}
		if ns := w.notifications(); len(ns) != 0 {
			t.Errorf("notifications = %+v, want none", ns)
		}
		if err := w.tr.Close(realCtx(t)); err != nil {
			t.Fatalf("Tracker.Close: %v", err)
		}
		if ev, _ := windows.WaitForSingleObject(h, 300); ev != uint32(windows.WAIT_TIMEOUT) {
			t.Errorf("process was signalled after Close (wait = %#x)", ev)
		}
		if code := exitCodeOf(t, h); code != stillLive {
			t.Errorf("exit code after Close = %d, want STILL_ACTIVE", code)
		}
		sc.createUntil(t)
		waitProcessExit(t, h, "launched process")
		if err := w.st.DB().QueryRow(`SELECT COUNT(*) FROM play_session`).Scan(&rows); err != nil || rows != 0 {
			t.Errorf("play_session rows at the end = %d (err %v), want 0", rows, err)
		}
	})
}

// TestLaunchDetachedHandles is not parallel: it measures the handles of the
// whole process.
func TestLaunchDetachedHandles(t *testing.T) {
	t.Run("TRK-011/20 lançamentos de fake-game --run 0 e um Close deixam GetProcessHandleCount <= h0 + 2", func(t *testing.T) {
		sc := newScenario(t)
		w := newWinEnv(t)
		argv := sc.argv("--run", "0")
		launched := 0
		launch := func(i int) {
			t.Helper()
			pid, err := w.tr.LaunchUntracked(realCtx(t), w.spec(argv, sc.dir))
			if err != nil {
				t.Fatalf("launch %d: LaunchUntracked: %v", i, err)
			}
			sc.expect(argv)
			launched++
			// The process is short-lived; wait for it so they do not pile up.
			// The "exit" line is written before the process ends, so wait on
			// the process handle too: the guarded job must be empty (CON-011).
			sc.waitCounts(t, launched, launched)
			if ph, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid); err == nil {
				waitProcessExit(t, ph, "launched process")
				_ = windows.CloseHandle(ph)
			}
		}
		// Warm-up launches first (see TestHandleCount): lazily created
		// runtime/DLL handles must be part of h0.
		for i := -9; i <= 0; i++ {
			launch(i)
		}
		h0 := settledHandleCount(t)
		threads0 := osThreadsCreated()
		for i := 1; i <= 20; i++ {
			launch(i)
		}
		if err := w.tr.Close(realCtx(t)); err != nil {
			t.Fatalf("Tracker.Close: %v", err)
		}
		waitHandleCountAtMost(t, h0, 2, threads0, "after 20 launches and Close")
	})
}

// TestHandleCount is not parallel: it measures the handles of the whole
// process.
func TestHandleCount(t *testing.T) {
	t.Run("TRK-007/20 lançamentos de --run 0 com carência vencida e Close terminam com ≤ h0 + 2 handles", func(t *testing.T) {
		sc := newScenario(t)
		w := newWinEnv(t)
		cycle := func(i int) {
			t.Helper()
			w.launch(t, sc, "--run", "0")
			if n := w.nextNotify(t); n.State != domain.SessionGrace {
				t.Fatalf("launch %d: notification = %q, want grace", i, n.State)
			}
			if n := w.expireGrace(t); n.State != domain.SessionDiscarded {
				t.Fatalf("launch %d: end notification = %q, want discarded", i, n.State)
			}
		}
		// Warm-up launches first: the Go runtime grows its OS thread pool (a
		// few handles per thread) in bounded steps during the first cycles, so
		// lazily created runtime/DLL handles must be part of h0.
		for i := -9; i <= 0; i++ {
			cycle(i)
		}
		h0 := settledHandleCount(t)
		threads0 := osThreadsCreated()
		for i := 1; i <= 20; i++ {
			cycle(i)
		}
		if err := w.tr.Close(realCtx(t)); err != nil {
			t.Fatalf("Tracker.Close: %v", err)
		}
		waitHandleCountAtMost(t, h0, 2, threads0, "after 20 launches and Close")
	})
}
