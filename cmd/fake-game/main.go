//go:build windows

// Command fake-game simulates games and launchers for the tracker integration tests.
// See docs/specs/phase-3-tracker/contracts/fake-game.md. It is a test helper, not product
// code, so it uses real time.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	exitOther      = 1
	exitBadFlags   = 2
	exitSpawnError = 4

	untilPollEvery = 50 * time.Millisecond
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	opts, err := parseOptions(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake-game: %v\n", err)
		return exitBadFlags
	}

	rep, err := openReport(opts.Report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake-game: %v\n", err)
		return exitOther
	}
	if rep != nil {
		defer rep.Close()
	}

	cwd, _ := os.Getwd()
	writeLine(rep, reportLine{
		Event: "start",
		PID:   os.Getpid(),
		PPID:  os.Getppid(),
		Depth: opts.Depth,
		Cwd:   cwd,
		Args:  args,
	})

	if opts.Depth > 0 {
		cargs := childArgs(args, opts.Depth-1)
		for i := 0; i < opts.Fanout; i++ {
			if err := spawnChild(cargs, opts.Breakaway); err != nil {
				code := exitSpawnError
				writeLine(rep, reportLine{
					Event: "exit",
					PID:   os.Getpid(),
					Depth: opts.Depth,
					Code:  &code,
					Error: err.Error(),
				})
				fmt.Fprintf(os.Stderr, "fake-game: %v\n", err)
				return exitSpawnError
			}
		}
	}

	life := opts.Run
	if opts.Depth > 0 {
		life = opts.ParentRun
	}
	live(life, opts.MaxLife, opts.UntilFile)

	code := opts.ExitCode
	writeLine(rep, reportLine{
		Event: "exit",
		PID:   os.Getpid(),
		Depth: opts.Depth,
		Code:  &code,
	})
	return code
}

// live blocks until the lifetime ends, the until-file exists or maxLife passes.
func live(life lifetime, maxLife time.Duration, untilFile string) {
	if !life.Forever && life.D <= 0 {
		return
	}
	limit := maxLife
	if !life.Forever && life.D < limit {
		limit = life.D
	}
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	tick := time.NewTicker(untilPollEvery)
	defer tick.Stop()
	for {
		if untilFile != "" {
			if _, err := os.Stat(untilFile); err == nil {
				return
			}
		}
		select {
		case <-deadline.C:
			return
		case <-tick.C:
		}
	}
}

func openReport(path string) (*os.File, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open report: %w", err)
	}
	return f, nil
}

// writeLine appends one JSON line with a single Write call (atomic with O_APPEND).
func writeLine(f *os.File, l reportLine) {
	if f == nil {
		return
	}
	b, err := json.Marshal(l)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake-game: marshal report line: %v\n", err)
		return
	}
	b = append(b, '\n')
	if _, err := f.Write(b); err != nil {
		fmt.Fprintf(os.Stderr, "fake-game: write report: %v\n", err)
	}
}

// spawnChild starts this executable with args, without waiting for it; the process and
// thread handles are closed right away.
func spawnChild(args []string, breakaway bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("create child: executable path: %w", err)
	}
	argv := append([]string{exe}, args...)
	cmdLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(argv))
	if err != nil {
		return fmt.Errorf("create child: command line: %w", err)
	}
	appName, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return fmt.Errorf("create child: executable path: %w", err)
	}
	var flags uint32
	if breakaway {
		flags |= windows.CREATE_BREAKAWAY_FROM_JOB
	}
	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(appName, cmdLine, nil, nil, false, flags, nil, nil, &si, &pi); err != nil {
		return fmt.Errorf("create child: %w", err)
	}
	_ = windows.CloseHandle(pi.Thread)
	_ = windows.CloseHandle(pi.Process)
	return nil
}
