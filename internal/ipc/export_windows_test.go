package ipc

import "golang.org/x/sys/windows"

// Test-only exports of the IPC-019 seam for the external tests (package ipc_test).

// RealPipeServerSID is realPipeServerSID: the string SID of the user owning the
// server process of the pipe handle h.
func RealPipeServerSID(h windows.Handle) (string, error) { return realPipeServerSID(h) }

// SetPipeServerSID replaces the pipeServerSID seam used by DialPipe and returns
// a function that restores the previous one. Callers must not run in parallel
// with anything that dials a pipe.
func SetPipeServerSID(f func(h windows.Handle) (string, error)) (restore func()) {
	old := pipeServerSID
	pipeServerSID = f
	return func() { pipeServerSID = old }
}
