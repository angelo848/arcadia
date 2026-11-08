//go:build windows

package ipc

import (
	"context"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// CurrentUserSID returns the string SID ("S-1-5-21-...") of the user that owns
// the current process token.
func CurrentUserSID() (string, error) {
	// GetCurrentProcessToken is a pseudo-handle: it must not be closed.
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("ipc: read process token user: %w", err)
	}
	return tu.User.Sid.String(), nil
}

// ListenPipe creates the first instance of the named pipe name, readable and
// writable only by the current user (protected DACL D:P(A;;GA;;;<SID>)), in
// byte mode, rejecting remote clients.
//
// First-instance semantics: go-winio v0.6.4 creates the first instance with
// NtCreateNamedPipeFile and disposition FILE_CREATE (the native equivalent of
// FILE_FLAG_FIRST_PIPE_INSTANCE), so ListenPipe fails if any instance of name
// already exists, whoever created it.
func ListenPipe(name string) (net.Listener, error) {
	sid, err := CurrentUserSID()
	if err != nil {
		return nil, err
	}
	ln, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;" + sid + ")",
		MessageMode:        false,
	})
	if err != nil {
		return nil, fmt.Errorf("ipc: listen on pipe %s: %w", name, err)
	}
	return ln, nil
}

// pipeServerSID returns the string SID of the user owning the server process
// of the pipe handle h. It is a variable so tests can replace it.
var pipeServerSID = realPipeServerSID

// realPipeServerSID resolves the server process of the pipe handle h and
// returns the string SID of its token user. It does not close h.
func realPipeServerSID(h windows.Handle) (string, error) {
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(h, &pid); err != nil {
		return "", fmt.Errorf("get pipe server process id: %w", err)
	}
	hp, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", fmt.Errorf("open server process %d: %w", pid, err)
	}
	defer windows.CloseHandle(hp)
	var tok windows.Token
	if err := windows.OpenProcessToken(hp, windows.TOKEN_QUERY, &tok); err != nil {
		return "", fmt.Errorf("open token of server process %d: %w", pid, err)
	}
	defer tok.Close()
	tu, err := tok.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("read token user of server process %d: %w", pid, err)
	}
	return tu.User.Sid.String(), nil
}

// DialPipe connects to the named pipe name, retrying while the pipe is busy
// until ctx is done, and verifies that the server process belongs to the
// current user (IPC-019). On a failed verification the connection is closed
// and the error wraps ErrUntrustedServer.
func DialPipe(ctx context.Context, name string) (net.Conn, error) {
	conn, err := winio.DialPipeContext(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("ipc: dial pipe %s: %w", name, err)
	}
	if cause := verifyPipeServer(conn); cause != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ipc: dial pipe %s: %w: %v", name, ErrUntrustedServer, cause)
	}
	return conn, nil
}

// verifyPipeServer returns nil when the server end of conn is owned by the
// current user, or the reason it is not (or cannot be verified).
func verifyPipeServer(conn net.Conn) error {
	fder, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return fmt.Errorf("connection %T does not expose its handle", conn)
	}
	// The handle still belongs to conn: it is not closed here.
	got, err := pipeServerSID(windows.Handle(fder.Fd()))
	if err != nil {
		return err
	}
	want, err := CurrentUserSID()
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("server owned by %s, want %s", got, want)
	}
	return nil
}

// PipeDialer returns a Dialer that connects to the named pipe name.
func PipeDialer(name string) Dialer {
	return func(ctx context.Context) (net.Conn, error) {
		return DialPipe(ctx, name)
	}
}
