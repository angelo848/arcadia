package ipc

import "errors"

// pipePrefix is the common prefix of the agent's named pipe (IPC-007).
const pipePrefix = `\\.\pipe\arcadia-agent-`

// ErrUntrustedServer is returned by DialPipe when the process serving the pipe
// is not owned by the current user, or its owner cannot be verified (IPC-019).
var ErrUntrustedServer = errors.New("ipc: untrusted pipe server")

// PipeName returns the agent's named pipe path for the user identified by sid
// (a string SID such as "S-1-5-21-..."): `\\.\pipe\arcadia-agent-<sid>`.
func PipeName(sid string) string {
	return pipePrefix + sid
}
