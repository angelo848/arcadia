//go:build windows

package tracker

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// winDetachedJob is the Job Object of a game launched without counting time
// (CON-010, CON-011). It owns only the job handle: no completion port, no
// process handle and no goroutine. The job has no KILL_ON_JOB_CLOSE, so Close
// never kills the game (ADR 0005).
type winDetachedJob struct {
	closer windowCloser // posts WM_CLOSE for RequestClose (CON-030)

	mu     sync.Mutex
	job    windows.Handle
	closed bool
}

// ActiveProcesses queries JobObjectBasicAccountingInformation.ActiveProcesses.
func (j *winDetachedJob) ActiveProcesses() (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return 0, errors.New("active processes: detached job closed")
	}
	var info jobBasicAccounting
	if err := windows.QueryInformationJobObject(j.job, windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return 0, fmt.Errorf("query job accounting: %w", err)
	}
	return int(info.ActiveProcesses), nil
}

// RequestClose posts WM_CLOSE to the visible, unowned top-level windows of the
// processes in the job and returns how many windows got it (CON-030).
func (j *winDetachedJob) RequestClose() (int, error) {
	j.mu.Lock()
	if j.closed {
		j.mu.Unlock()
		return 0, errors.New("request close: detached job closed")
	}
	pids, err := jobPIDs(j.job)
	j.mu.Unlock()
	if err != nil {
		return 0, fmt.Errorf("request close: %w", err)
	}
	set := make(map[uint32]struct{}, len(pids))
	for _, p := range pids {
		set[p] = struct{}{}
	}
	return j.closer.PostClose(set)
}

// Kill terminates every process in the job with exit code 1 (CON-010); it never
// closes the job.
func (j *winDetachedJob) Kill() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return errors.New("kill: detached job closed")
	}
	return killJobTree(j.job)
}

// Close releases the job handle without killing its processes. Idempotent.
func (j *winDetachedJob) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	if err := windows.CloseHandle(j.job); err != nil {
		return fmt.Errorf("close detached job handle: %w", err)
	}
	return nil
}
