package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fakeGameExe is the fake-game binary compiled once by TestMain.
var fakeGameExe string

func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	dir, err := os.MkdirTemp("", "fake-game-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdir temp: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	goBin, err := exec.LookPath("go")
	if err != nil {
		goBin = filepath.Join(runtime.GOROOT(), "bin", "go.exe")
	}
	fakeGameExe = filepath.Join(dir, "fake-game.exe")
	build := exec.Command(goBin, "build", "-o", fakeGameExe, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "go build fake-game: %v\n%s\n", err, out)
		return 1
	}
	return m.Run()
}

const (
	realWait  = 10 * time.Second // bounded real wait for process events
	pollEvery = 20 * time.Millisecond
)

// scenario owns a temp dir with a report and an until-file; its cleanup creates the
// until-file and waits for every reported process to write its exit line, so no
// fake-game outlives the test (and TempDir removal does not hit open files).
type scenario struct {
	dir    string
	report string
	until  string
}

func newScenario(t *testing.T) *scenario {
	t.Helper()
	dir := t.TempDir()
	s := &scenario{
		dir:    dir,
		report: filepath.Join(dir, "report.jsonl"),
		until:  filepath.Join(dir, "stop"),
	}
	t.Cleanup(func() { s.stop(t) })
	return s
}

func (s *scenario) createUntil(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(s.until, nil, 0o644); err != nil {
		t.Fatalf("create until-file: %v", err)
	}
}

func (s *scenario) stop(t *testing.T) {
	_ = os.WriteFile(s.until, nil, 0o644)
	deadline := time.Now().Add(realWait)
	for {
		lines, _ := readReport(s.report)
		starts, exits := countEvents(lines)
		if exits >= starts {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("cleanup: %d start lines but only %d exit lines after %v", starts, exits, realWait)
			return
		}
		<-time.After(pollEvery)
	}
}

// readReport parses the complete lines of a JSON Lines report; a missing file is no lines.
func readReport(path string) ([]reportLine, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Ignore a trailing partial line still being written.
	if i := bytes.LastIndexByte(data, '\n'); i >= 0 {
		data = data[:i+1]
	} else {
		return nil, nil
	}
	var lines []reportLine
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		var l reportLine
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("report line %q: %w", raw, err)
		}
		lines = append(lines, l)
	}
	return lines, sc.Err()
}

func countEvents(lines []reportLine) (starts, exits int) {
	for _, l := range lines {
		switch l.Event {
		case "start":
			starts++
		case "exit":
			exits++
		}
	}
	return starts, exits
}

func filterEvent(lines []reportLine, event string) []reportLine {
	var out []reportLine
	for _, l := range lines {
		if l.Event == event {
			out = append(out, l)
		}
	}
	return out
}

// waitReport polls the report until cond holds or realWait passes.
func waitReport(t *testing.T, path string, what string, cond func([]reportLine) bool) []reportLine {
	t.Helper()
	deadline := time.After(realWait)
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		lines, err := readReport(path)
		if err != nil {
			t.Fatalf("read report: %v", err)
		}
		if cond(lines) {
			return lines
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %s; report: %+v", what, lines)
		case <-tick.C:
		}
	}
}

// runFake runs fake-game to completion (bounded) and returns its exit code.
func runFake(t *testing.T, dir string, args ...string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), realWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, fakeGameExe, args...)
	cmd.Dir = dir
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("fake-game %q did not exit within %v", args, realWait)
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("run fake-game %q: %v", args, err)
	}
	return cmd.ProcessState.ExitCode()
}

// openProcess opens a waitable handle to pid; the handle is closed in t.Cleanup.
func openProcess(t *testing.T, pid int) windows.Handle {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		t.Fatalf("OpenProcess(%d): %v", pid, err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(h) })
	return h
}

func exitCodeOf(t *testing.T, h windows.Handle) uint32 {
	t.Helper()
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		t.Fatalf("GetExitCodeProcess: %v", err)
	}
	return code
}

func TestFakeGame(t *testing.T) {
	t.Parallel()

	t.Run("FKG-001/--run 300ms --exit-code 3 sai com 3 depois de ≥ 300 ms, sem filhos", func(t *testing.T) {
		t.Parallel()
		s := newScenario(t)
		start := time.Now()
		code := runFake(t, s.dir, "--run", "300ms", "--exit-code", "3", "--max-life", "30s", "--report", s.report)
		elapsed := time.Since(start)
		if code != 3 {
			t.Errorf("exit code = %d, want 3", code)
		}
		if elapsed < 300*time.Millisecond {
			t.Errorf("elapsed = %v, want >= 300ms", elapsed)
		}
		lines, err := readReport(s.report)
		if err != nil {
			t.Fatal(err)
		}
		if starts := filterEvent(lines, "start"); len(starts) != 1 || starts[0].Depth != 0 {
			t.Errorf("start lines = %+v, want exactly one with depth 0 (no children)", starts)
		}
	})

	t.Run("FKG-002/depth 2 fanout 2: 7 starts (1 com depth 2, 2 com depth 1, 4 com depth 0) e ppid = pid do pai", func(t *testing.T) {
		t.Parallel()
		s := newScenario(t)
		ctx, cancel := context.WithTimeout(context.Background(), realWait)
		defer cancel()
		cmd := exec.CommandContext(ctx, fakeGameExe, "--depth", "2", "--fanout", "2", "--run", "0",
			"--max-life", "30s", "--until-file", s.until, "--report", s.report)
		cmd.Dir = s.dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("run root: %v", err)
		}
		rootPID := cmd.ProcessState.Pid()

		lines := waitReport(t, s.report, "7 start and 7 exit lines", func(ls []reportLine) bool {
			st, ex := countEvents(ls)
			return st >= 7 && ex >= 7
		})
		starts := filterEvent(lines, "start")
		if len(starts) != 7 {
			t.Fatalf("start lines = %d, want 7: %+v", len(starts), starts)
		}
		byPID := map[int]reportLine{}
		perDepth := map[int]int{}
		for _, l := range starts {
			byPID[l.PID] = l
			perDepth[l.Depth]++
		}
		if want := map[int]int{2: 1, 1: 2, 0: 4}; !reflect.DeepEqual(perDepth, want) {
			t.Errorf("starts per depth = %v, want %v", perDepth, want)
		}
		root, ok := byPID[rootPID]
		if !ok || root.Depth != 2 {
			t.Fatalf("root pid %d: start line %+v (found=%v), want depth 2", rootPID, root, ok)
		}
		if root.PPID != os.Getpid() {
			t.Errorf("root ppid = %d, want test pid %d", root.PPID, os.Getpid())
		}
		children := map[int]int{}
		for _, l := range starts {
			if l.PID == rootPID {
				continue
			}
			parent, ok := byPID[l.PPID]
			if !ok {
				t.Errorf("pid %d (depth %d): ppid %d is not a reported fake-game", l.PID, l.Depth, l.PPID)
				continue
			}
			if parent.Depth != l.Depth+1 {
				t.Errorf("pid %d depth %d has parent %d with depth %d, want %d", l.PID, l.Depth, parent.PID, parent.Depth, l.Depth+1)
			}
			children[l.PPID]++
		}
		for pid, l := range byPID {
			if l.Depth > 0 && children[pid] != 2 {
				t.Errorf("pid %d (depth %d) has %d children, want 2", pid, l.Depth, children[pid])
			}
		}
	})

	t.Run("FKG-003/padrão lançador: o inicial sai com --exit-code e o filho continua até F", func(t *testing.T) {
		t.Parallel()
		s := newScenario(t)
		code := runFake(t, s.dir, "--depth", "1", "--parent-run", "0", "--run", "forever", "--exit-code", "5",
			"--until-file", s.until, "--max-life", "30s", "--report", s.report)
		if code != 5 {
			t.Errorf("launcher exit code = %d, want 5", code)
		}
		lines := waitReport(t, s.report, "child start line", func(ls []reportLine) bool {
			for _, l := range ls {
				if l.Event == "start" && l.Depth == 0 {
					return true
				}
			}
			return false
		})
		var childPID int
		for _, l := range lines {
			if l.Event == "start" && l.Depth == 0 {
				childPID = l.PID
			}
		}
		h := openProcess(t, childPID)
		if ev, _ := windows.WaitForSingleObject(h, 0); ev != uint32(windows.WAIT_TIMEOUT) {
			t.Fatalf("child %d is not running after launcher exited (wait = %#x)", childPID, ev)
		}

		s.createUntil(t)
		if ev, err := windows.WaitForSingleObject(h, uint32(realWait/time.Millisecond)); ev != windows.WAIT_OBJECT_0 {
			t.Fatalf("child %d did not exit after F was created (wait = %#x, %v)", childPID, ev, err)
		}
		if got := exitCodeOf(t, h); got != 5 {
			t.Errorf("child exit code = %d, want 5", got)
		}
	})

	t.Run("FKG-004/criar F encerra a árvore em até 500 ms", func(t *testing.T) {
		t.Parallel()
		s := newScenario(t)
		ctx, cancel := context.WithTimeout(context.Background(), realWait)
		defer cancel()
		cmd := exec.CommandContext(ctx, fakeGameExe, "--depth", "1", "--fanout", "2", "--parent-run", "forever",
			"--run", "forever", "--until-file", s.until, "--max-life", "30s", "--report", s.report)
		cmd.Dir = s.dir
		if err := cmd.Start(); err != nil {
			t.Fatalf("start: %v", err)
		}
		waitErr := make(chan error, 1)
		go func() { waitErr <- cmd.Wait() }()
		t.Cleanup(func() {
			s.stop(t)
			<-waitErr
		})

		lines := waitReport(t, s.report, "3 start lines", func(ls []reportLine) bool {
			st, _ := countEvents(ls)
			return st >= 3
		})
		var handles []windows.Handle
		for _, l := range filterEvent(lines, "start") {
			handles = append(handles, openProcess(t, l.PID))
		}

		s.createUntil(t)
		created := time.Now()
		for i, h := range handles {
			if ev, err := windows.WaitForSingleObject(h, uint32(realWait/time.Millisecond)); ev != windows.WAIT_OBJECT_0 {
				t.Fatalf("process %d did not exit (wait = %#x, %v)", i, ev, err)
			}
		}
		if elapsed := time.Since(created); elapsed > 500*time.Millisecond {
			t.Errorf("tree exited %v after F was created, want <= 500ms", elapsed)
		}
	})

	t.Run("FKG-005/--max-life 1s encerra --run forever", func(t *testing.T) {
		t.Parallel()
		s := newScenario(t)
		start := time.Now()
		code := runFake(t, s.dir, "--run", "forever", "--max-life", "1s", "--until-file", s.until, "--report", s.report)
		elapsed := time.Since(start)
		if code != 0 {
			t.Errorf("exit code = %d, want 0 (normal exit)", code)
		}
		if elapsed < time.Second || elapsed > 5*time.Second {
			t.Errorf("elapsed = %v, want between 1s and 5s", elapsed)
		}
		lines, err := readReport(s.report)
		if err != nil {
			t.Fatal(err)
		}
		if exits := filterEvent(lines, "exit"); len(exits) != 1 || exits[0].Code == nil || *exits[0].Code != 0 {
			t.Errorf("exit lines = %+v, want one with code 0", exits)
		}
	})

	t.Run("FKG-006/linhas start e exit com pid, ppid, depth, cwd, args e code", func(t *testing.T) {
		t.Parallel()
		s := newScenario(t)
		args := []string{"--run", "0", "--exit-code", "7", "--max-life", "30s", "--report", s.report}
		ctx, cancel := context.WithTimeout(context.Background(), realWait)
		defer cancel()
		cmd := exec.CommandContext(ctx, fakeGameExe, args...)
		cmd.Dir = s.dir
		if err := cmd.Run(); cmd.ProcessState == nil {
			t.Fatalf("run fake-game: %v", err)
		}
		if got := cmd.ProcessState.ExitCode(); got != 7 {
			t.Fatalf("exit code = %d, want 7", got)
		}
		pid := cmd.ProcessState.Pid()

		raw, err := os.ReadFile(s.report)
		if err != nil {
			t.Fatalf("read report: %v", err)
		}
		rawLines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
		if len(rawLines) != 2 {
			t.Fatalf("report has %d lines, want 2:\n%s", len(rawLines), raw)
		}
		lines, err := readReport(s.report)
		if err != nil {
			t.Fatal(err)
		}
		st, ex := lines[0], lines[1]
		if st.Event != "start" || ex.Event != "exit" {
			t.Fatalf("events = %q, %q, want start, exit", st.Event, ex.Event)
		}
		if st.PID != pid || st.PPID != os.Getpid() || st.Depth != 0 {
			t.Errorf("start pid/ppid/depth = %d/%d/%d, want %d/%d/0", st.PID, st.PPID, st.Depth, pid, os.Getpid())
		}
		if !strings.EqualFold(filepath.Clean(st.Cwd), filepath.Clean(s.dir)) {
			t.Errorf("start cwd = %q, want %q", st.Cwd, s.dir)
		}
		if !reflect.DeepEqual(st.Args, args) {
			t.Errorf("start args = %q, want %q", st.Args, args)
		}
		if st.Code != nil {
			t.Errorf("start line has code %d, want none", *st.Code)
		}
		if ex.PID != pid || ex.Depth != 0 {
			t.Errorf("exit pid/depth = %d/%d, want %d/0", ex.PID, ex.Depth, pid)
		}
		if ex.Code == nil || *ex.Code != 7 {
			t.Errorf("exit code field = %v, want 7", ex.Code)
		}
		if ex.Error != "" {
			t.Errorf("exit error = %q, want empty", ex.Error)
		}
	})

	t.Run("FKG-006/argumentos depois de -- aparecem em args", func(t *testing.T) {
		t.Parallel()
		s := newScenario(t)
		args := []string{"--run", "0", "--max-life", "30s", "--report", s.report, "--", "a", "b c", `C:\x y\z.rom`}
		if code := runFake(t, s.dir, args...); code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		lines, err := readReport(s.report)
		if err != nil {
			t.Fatal(err)
		}
		starts := filterEvent(lines, "start")
		if len(starts) != 1 {
			t.Fatalf("start lines = %+v, want 1", starts)
		}
		if !reflect.DeepEqual(starts[0].Args, args) {
			t.Errorf("args = %q, want %q", starts[0].Args, args)
		}
	})

	t.Run("FKG-007/flag inválida sai com 2 sem criar relatório", func(t *testing.T) {
		t.Parallel()
		s := newScenario(t)
		invalid := [][]string{
			{"--report", s.report, "--bogus"},
			{"--report", s.report, "--depth", "-1"},
			{"--report", s.report, "--depth", "1", "--fanout", "2", "--run", "abc"},
			{"--report", s.report, "--depth", "1", "--exit-code", "256"},
			{"--report", s.report, "--max-life", "0"},
		}
		for _, args := range invalid {
			if code := runFake(t, s.dir, args...); code != 2 {
				t.Errorf("fake-game %q exit code = %d, want 2", args, code)
			}
			if _, err := os.Stat(s.report); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("fake-game %q: report exists (stat err = %v), want not created", args, err)
			}
		}
	})

	t.Run("FKG-008/--breakaway dentro de um job sem BREAKAWAY_OK sai com 4 e grava exit com error", func(t *testing.T) {
		t.Parallel()
		s := newScenario(t)

		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			t.Fatalf("CreateJobObject: %v", err)
		}
		defer windows.CloseHandle(job)
		// KILL_ON_JOB_CLOSE (no BREAKAWAY_OK): anything left in the job dies when the handle closes.
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			t.Fatalf("SetInformationJobObject: %v", err)
		}

		argv := []string{fakeGameExe, "--depth", "1", "--breakaway", "--parent-run", "0", "--run", "forever",
			"--until-file", s.until, "--max-life", "30s", "--report", s.report}
		cmdLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(argv))
		if err != nil {
			t.Fatal(err)
		}
		dir, err := windows.UTF16PtrFromString(s.dir)
		if err != nil {
			t.Fatal(err)
		}
		var si windows.StartupInfo
		si.Cb = uint32(unsafe.Sizeof(si))
		var pi windows.ProcessInformation
		if err := windows.CreateProcess(nil, cmdLine, nil, nil, false, windows.CREATE_SUSPENDED, nil, dir, &si, &pi); err != nil {
			t.Fatalf("CreateProcess: %v", err)
		}
		defer windows.CloseHandle(pi.Process)
		if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
			_ = windows.TerminateProcess(pi.Process, 1)
			_ = windows.CloseHandle(pi.Thread)
			t.Fatalf("AssignProcessToJobObject: %v", err)
		}
		_, resumeErr := windows.ResumeThread(pi.Thread)
		_ = windows.CloseHandle(pi.Thread)
		if resumeErr != nil {
			_ = windows.TerminateProcess(pi.Process, 1)
			t.Fatalf("ResumeThread: %v", resumeErr)
		}

		if ev, err := windows.WaitForSingleObject(pi.Process, uint32(realWait/time.Millisecond)); ev != windows.WAIT_OBJECT_0 {
			_ = windows.TerminateProcess(pi.Process, 1)
			t.Fatalf("fake-game did not exit (wait = %#x, %v)", ev, err)
		}
		if code := exitCodeOf(t, pi.Process); code != 4 {
			t.Errorf("exit code = %d, want 4", code)
		}

		lines, err := readReport(s.report)
		if err != nil {
			t.Fatal(err)
		}
		if starts := filterEvent(lines, "start"); len(starts) != 1 || starts[0].Depth != 1 {
			t.Errorf("start lines = %+v, want only the parent (depth 1)", starts)
		}
		exits := filterEvent(lines, "exit")
		if len(exits) != 1 {
			t.Fatalf("exit lines = %+v, want 1", exits)
		}
		ex := exits[0]
		if ex.PID != int(pi.ProcessId) || ex.Depth != 1 {
			t.Errorf("exit pid/depth = %d/%d, want %d/1", ex.PID, ex.Depth, pi.ProcessId)
		}
		if ex.Code == nil || *ex.Code != 4 {
			t.Errorf("exit code field = %v, want 4", ex.Code)
		}
		if ex.Error == "" {
			t.Errorf("exit error is empty, want the child creation error")
		}
	})
}
