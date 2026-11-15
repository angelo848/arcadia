//go:build windows

package tracker

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/winapi"
)

// Job object notification codes (winnt.h), delivered as the byte count of a
// completion packet. x/sys/windows does not declare them.
const (
	jobMsgActiveProcessZero   = 4
	jobMsgNewProcess          = 6
	jobMsgExitProcess         = 7
	jobMsgAbnormalExitProcess = 8
)

// Completion keys of the job's port: jobKey marks job notifications, stopKey
// is posted by Close to stop the reader.
const (
	jobKey  uintptr = 1
	stopKey uintptr = 2
)

// eventsBuffer is the capacity of the Events channel.
const eventsBuffer = 64

// jobAssociateCompletionPort is JOBOBJECT_ASSOCIATE_COMPLETION_PORT.
type jobAssociateCompletionPort struct {
	CompletionKey  uintptr
	CompletionPort windows.Handle
}

// jobBasicAccounting is JOBOBJECT_BASIC_ACCOUNTING_INFORMATION.
type jobBasicAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

// NewLauncher returns the Windows Launcher: the game starts suspended inside a
// new Job Object (with an IO completion port for its notifications) and only
// runs after Resume, so every descendant is born inside the job.
func NewLauncher() Launcher { return winLauncher{} }

type winLauncher struct{}

// Prepare creates the job and its port, creates the process suspended and
// assigns it to the job. On error everything it created is closed (and a
// created process is terminated), and the error is a *LaunchError.
func (winLauncher) Prepare(spec LaunchSpec) (Job, error) {
	j, err := prepareJob(spec)
	if err != nil {
		le := &LaunchError{Err: err}
		var errno windows.Errno
		if errors.As(err, &errno) {
			le.WinError = uint32(errno)
		}
		return nil, le
	}
	return j, nil
}

// LaunchDetached starts the game with the same command line, working directory
// and error mode as Prepare, inside a Job Object kept only so that it can be
// closed (CON-011); see launchDetached. On error everything created is closed
// (a created process is terminated) and the error is a *LaunchError [SES-034].
func (winLauncher) LaunchDetached(spec LaunchSpec) (DetachedJob, uint32, error) {
	j, pid, err := launchDetached(spec)
	if err != nil {
		le := &LaunchError{Err: err}
		var errno windows.Errno
		if errors.As(err, &errno) {
			le.WinError = uint32(errno)
		}
		return nil, 0, le
	}
	return j, pid, nil
}

// launchDetached creates the process suspended, creates a job with
// JOB_OBJECT_LIMIT_BREAKAWAY_OK only (no KILL_ON_JOB_CLOSE, ADR 0005; no
// SILENT_BREAKAWAY_OK, ADR 0016), assigns the process to it, resumes it and
// closes hThread and hProcess. The job handle is the only one kept.
func launchDetached(spec LaunchSpec) (_ *winDetachedJob, _ uint32, err error) {
	appName, cmdLine, cwd, err := launchStrings(spec)
	if err != nil {
		return nil, 0, err
	}
	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_DEFAULT_ERROR_MODE)
	if err := windows.CreateProcess(appName, &cmdLine[0], nil, nil, false, flags, nil, cwd, &si, &pi); err != nil {
		return nil, 0, fmt.Errorf("create process %q: %w", spec.Exe, err)
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)
	resumed := false
	defer func() {
		if err != nil && !resumed {
			// The process never ran game code: kill it before releasing it.
			_ = windows.TerminateProcess(pi.Process, 1)
		}
	}()

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create job object: %w", err)
	}
	defer func() {
		if err != nil {
			_ = windows.CloseHandle(job)
		}
	}()
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, 0, fmt.Errorf("set job limits: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return nil, 0, fmt.Errorf("assign process to job (is the agent inside a job that forbids nesting?): %w", err)
		}
		return nil, 0, fmt.Errorf("assign process to job: %w", err)
	}
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		return nil, 0, fmt.Errorf("resume thread: %w", err)
	}
	resumed = true
	return &winDetachedJob{job: job, closer: newWindowCloser()}, pi.ProcessId, nil
}

// launchStrings builds the application name, command line and working
// directory shared by Prepare and LaunchDetached [TRK-005].
func launchStrings(spec LaunchSpec) (appName *uint16, cmdLine []uint16, cwd *uint16, err error) {
	appName, err = windows.UTF16PtrFromString(spec.Exe)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("exe path %q: %w", spec.Exe, err)
	}
	line := quoteExe(spec.Exe)
	if spec.Args != "" {
		line += " " + spec.Args
	}
	cmdLine, err = windows.UTF16FromString(line)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("command line: %w", err)
	}
	workDir := spec.WorkDir
	if workDir == "" {
		workDir = filepath.Dir(spec.Exe)
	}
	cwd, err = windows.UTF16PtrFromString(workDir)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("working dir %q: %w", workDir, err)
	}
	return appName, cmdLine, cwd, nil
}

func prepareJob(spec LaunchSpec) (_ *winJob, err error) {
	appName, cmdLine, cwd, err := launchStrings(spec)
	if err != nil {
		return nil, err
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}
	defer func() {
		if err != nil {
			_ = windows.CloseHandle(job)
		}
	}()

	// BREAKAWAY_OK only (ADR 0016): no KILL_ON_JOB_CLOSE (ADR 0005, closing the
	// agent never kills the game) and no SILENT_BREAKAWAY_OK.
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, fmt.Errorf("set job limits: %w", err)
	}

	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		return nil, fmt.Errorf("create completion port: %w", err)
	}
	defer func() {
		if err != nil {
			_ = windows.CloseHandle(port)
		}
	}()
	assoc := jobAssociateCompletionPort{CompletionKey: jobKey, CompletionPort: port}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectAssociateCompletionPortInformation,
		uintptr(unsafe.Pointer(&assoc)), uint32(unsafe.Sizeof(assoc))); err != nil {
		return nil, fmt.Errorf("associate completion port: %w", err)
	}

	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_DEFAULT_ERROR_MODE)
	if err := windows.CreateProcess(appName, &cmdLine[0], nil, nil, false, flags, nil, cwd, &si, &pi); err != nil {
		return nil, fmt.Errorf("create process %q: %w", spec.Exe, err)
	}
	if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		// The process never ran game code: kill it before releasing it.
		_ = windows.TerminateProcess(pi.Process, 1)
		_ = windows.CloseHandle(pi.Thread)
		_ = windows.CloseHandle(pi.Process)
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return nil, fmt.Errorf("assign process to job (is the agent inside a job that forbids nesting?): %w", err)
		}
		return nil, fmt.Errorf("assign process to job: %w", err)
	}

	j := &winJob{
		job:     job,
		port:    port,
		process: pi.Process,
		thread:  pi.Thread,
		rootPID: pi.ProcessId,
		closer:  newWindowCloser(),
		events:  make(chan JobEvent, eventsBuffer),
		stop:    make(chan struct{}),
	}
	j.wg.Add(1)
	go j.read()
	return j, nil
}

// quoteExe always wraps the executable path in quotes (paths may contain
// spaces); EscapeArg only quotes when needed. Duplicate of the unexported
// helper in internal/winapi (design decision 11).
func quoteExe(exe string) string {
	q := windows.EscapeArg(exe)
	if strings.HasPrefix(q, `"`) {
		return q
	}
	return `"` + q + `"`
}

// winJob owns the job, its completion port and the initial process and thread
// handles. The reader goroutine is owned by the job and waited on in Close.
type winJob struct {
	job, port windows.Handle
	process   windows.Handle // initial process; read by the reader, closed after it exits
	rootPID   uint32
	closer    windowCloser // posts WM_CLOSE for RequestClose (CON-030)

	events chan JobEvent
	stop   chan struct{}
	wg     sync.WaitGroup

	mu             sync.Mutex
	thread         windows.Handle // initial thread; 0 after Resume or Close
	resumed        bool           // Resume succeeded; Terminate is refused from then on
	threadCloseErr error          // CloseHandle(thread) failure in Resume, reported by Close
	closed         bool

	closeOnce sync.Once
	closeErr  error
}

func (j *winJob) RootPID() uint32 { return j.rootPID }

func (j *winJob) Events() <-chan JobEvent { return j.events }

// Resume resumes the initial thread and closes its handle.
func (j *winJob) Resume() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return errors.New("resume: job closed")
	}
	if j.thread == 0 {
		return errors.New("resume: already resumed")
	}
	if _, err := windows.ResumeThread(j.thread); err != nil {
		return fmt.Errorf("resume thread: %w", err)
	}
	// The game is running now: a failure to close our thread handle must not
	// look like a failed resume (the coordinator would terminate the game), so
	// it is kept for Close to report.
	j.resumed = true
	if err := windows.CloseHandle(j.thread); err != nil {
		j.threadCloseErr = fmt.Errorf("close thread handle: %w", err)
	}
	j.thread = 0
	return nil
}

// Terminate kills the suspended initial process with exit code 1. It is only
// used for a failed launch, before Resume.
func (j *winJob) Terminate() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return errors.New("terminate: job closed")
	}
	if j.resumed {
		return errors.New("terminate: refused, the game was already resumed")
	}
	if err := windows.TerminateProcess(j.process, 1); err != nil {
		return fmt.Errorf("terminate process: %w", err)
	}
	return nil
}

// ActiveProcesses queries JobObjectBasicAccountingInformation.ActiveProcesses.
func (j *winJob) ActiveProcesses() (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return 0, errors.New("active processes: job closed")
	}
	var info jobBasicAccounting
	if err := windows.QueryInformationJobObject(j.job, windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return 0, fmt.Errorf("query job accounting: %w", err)
	}
	return int(info.ActiveProcesses), nil
}

// RequestClose posts WM_CLOSE to the visible, unowned top-level windows of the
// processes in the job (JobObjectBasicProcessIdList) and returns how many
// windows got it (CON-030). A process that broke away (ADR 0016) is not in the
// job and is never asked.
func (j *winJob) RequestClose() (int, error) {
	j.mu.Lock()
	if j.closed {
		j.mu.Unlock()
		return 0, errors.New("request close: job closed")
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

// Kill terminates every process in the job with exit code 1 (CON-030). It is
// valid after Resume and never closes the job; Terminate stays reserved for a
// failed launch. If TerminateJobObject is refused, each process of the job is
// terminated on its own; the error is reported only if one survives.
func (j *winJob) Kill() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return errors.New("kill: job closed")
	}
	return killJobTree(j.job)
}

// killJobTree terminates every process of job with exit code 1, falling back
// to one process at a time when TerminateJobObject is refused (shared by
// winJob and winDetachedJob, CON-030, CON-010). The caller holds the lock that
// keeps job open.
func killJobTree(job windows.Handle) error {
	jobErr := windows.TerminateJobObject(job, 1)
	if jobErr == nil {
		return nil
	}
	pids, err := jobPIDs(job)
	if err != nil {
		return fmt.Errorf("kill: terminate job: %w", errors.Join(jobErr, err))
	}
	var errs []error
	for _, pid := range pids {
		if err := terminatePID(pid); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("kill: terminate job: %w", errors.Join(append([]error{jobErr}, errs...)...))
	}
	return nil
}

// terminatePID terminates a process of the job with exit code 1. A process
// that already exited is not an error.
func terminatePID(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil // gone
		}
		return fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	if err := windows.TerminateProcess(h, 1); err != nil {
		if ev, _ := windows.WaitForSingleObject(h, 0); ev == windows.WAIT_OBJECT_0 {
			return nil // it exited meanwhile
		}
		return fmt.Errorf("terminate process %d: %w", pid, err)
	}
	return nil
}

// Close stops the reader, waits for it and closes every handle. Closing the
// job handle never kills its processes (no KILL_ON_JOB_CLOSE). Idempotent.
func (j *winJob) Close() error {
	j.closeOnce.Do(func() {
		close(j.stop)
		var errs []error
		readerDone := false
		if err := windows.PostQueuedCompletionStatus(j.port, 0, stopKey, nil); err != nil {
			// The reader only leaves on stopKey; without it Close would hang.
			// This cannot happen with a valid port, so report it loudly.
			errs = append(errs, fmt.Errorf("post stop to completion port: %w", err))
		} else {
			j.wg.Wait()
			readerDone = true
		}

		j.mu.Lock()
		j.closed = true
		if j.threadCloseErr != nil {
			errs = append(errs, j.threadCloseErr)
		}
		if j.thread != 0 {
			if err := windows.CloseHandle(j.thread); err != nil {
				errs = append(errs, fmt.Errorf("close thread handle: %w", err))
			}
			j.thread = 0
		}
		j.mu.Unlock()

		if readerDone {
			// The reader has exited: the port and the process handle are ours.
			for _, h := range []struct {
				name string
				h    windows.Handle
			}{{"completion port", j.port}, {"process handle", j.process}} {
				if err := windows.CloseHandle(h.h); err != nil {
					errs = append(errs, fmt.Errorf("close %s: %w", h.name, err))
				}
			}
		}
		j.mu.Lock()
		if err := windows.CloseHandle(j.job); err != nil {
			errs = append(errs, fmt.Errorf("close job handle: %w", err))
		}
		j.mu.Unlock()
		j.closeErr = errors.Join(errs...)
	})
	return j.closeErr
}

// read is the reader goroutine: it turns completion packets into JobEvents
// until it sees stopKey, then closes events.
func (j *winJob) read() {
	defer j.wg.Done()
	defer close(j.events)
	for {
		var msg uint32
		var key uintptr
		var ov *windows.Overlapped
		if err := windows.GetQueuedCompletionStatus(j.port, &msg, &key, &ov, windows.INFINITE); err != nil {
			if ov == nil {
				// The port itself failed (only possible if it was closed, which
				// Close does after this goroutine exits): stop reading.
				return
			}
			continue
		}
		if key == stopKey {
			return
		}
		if key != jobKey {
			continue
		}
		// For job notifications lpOverlapped carries the PID, not a pointer.
		pid := uint32(uintptr(unsafe.Pointer(ov)))
		var ev JobEvent
		switch msg {
		case jobMsgNewProcess:
			ev = JobEvent{Kind: EventNewProcess, PID: pid, Root: pid == j.rootPID}
		case jobMsgExitProcess, jobMsgAbnormalExitProcess:
			ev = JobEvent{Kind: EventExitProcess, PID: pid}
			if pid == j.rootPID {
				ev.Root = true
				var code uint32
				if err := windows.GetExitCodeProcess(j.process, &code); err == nil {
					ev.ExitCode = code
				}
			}
		case jobMsgActiveProcessZero:
			ev = JobEvent{Kind: EventActiveZero}
		default:
			continue
		}
		select {
		case j.events <- ev:
		case <-j.stop:
			return
		}
	}
}

// OpenURI opens uri through the shell (RUL-003). Failures are *LaunchError.
func (winLauncher) OpenURI(uri string) error {
	if err := winapi.ShellOpen(uri); err != nil {
		return &LaunchError{Err: err}
	}
	return nil
}
