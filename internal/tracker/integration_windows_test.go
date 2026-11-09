//go:build windows

package tracker_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
		// One warm-up launch first, so lazily created runtime/DLL handles
		// (threads, loaded procs) are part of h0.
		cycle(0)
		h0 := processHandleCount(t)
		for i := 1; i <= 20; i++ {
			cycle(i)
		}
		if err := w.tr.Close(realCtx(t)); err != nil {
			t.Fatalf("Tracker.Close: %v", err)
		}
		if h := processHandleCount(t); h > h0+2 {
			t.Errorf("handle count = %d after 20 launches and Close, want <= h0 + 2 = %d", h, h0+2)
		}
	})
}
