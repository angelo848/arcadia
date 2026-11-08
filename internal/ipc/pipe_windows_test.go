package ipc_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/arcadia-launcher/arcadia/internal/domain"
	"github.com/arcadia-launcher/arcadia/internal/ipc"
)

// fileAllAccess is FILE_ALL_ACCESS (STANDARD_RIGHTS_REQUIRED | SYNCHRONIZE | 0x1FF),
// the specific rights GENERIC_ALL maps to for file/pipe objects through the
// file generic mapping. The object manager may store the ACE either with the
// generic bit as written in the SDDL ("GA") or already mapped, so both are
// accepted as "full control".
const fileAllAccess windows.ACCESS_MASK = 0x000F0000 | 0x00100000 | 0x1FF

// tokenUserSID reads the current user's SID straight from the process token,
// independently of the code under test.
func tokenUserSID(t *testing.T) *windows.SID {
	t.Helper()
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("GetTokenUser: %v", err)
	}
	sid, err := tu.User.Sid.Copy()
	if err != nil {
		t.Fatalf("copy SID: %v", err)
	}
	return sid
}

// testPipeName returns a pipe name unique to this test: PipeName(sid)+"-test-<rand>".
func testPipeName(t *testing.T) string {
	t.Helper()
	sid, err := ipc.CurrentUserSID()
	if err != nil {
		t.Fatalf("CurrentUserSID: %v", err)
	}
	return fmt.Sprintf("%s-test-%016x", ipc.PipeName(sid), rand.Uint64())
}

// listenPipe calls ListenPipe and closes the listener at cleanup.
func listenPipe(t *testing.T, name string) net.Listener {
	t.Helper()
	ln, err := ipc.ListenPipe(name)
	if err != nil {
		t.Fatalf("ListenPipe(%q): %v", name, err)
	}
	if ln == nil {
		t.Fatalf("ListenPipe(%q) returned a nil listener without error", name)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

// acceptPending runs an owned goroutine that keeps an Accept pending on ln:
// every accepted conn is closed at once, and the loop ends when Accept fails
// (listener closed). At cleanup it closes ln and waits for the goroutine.
func acceptPending(t *testing.T, ln net.Listener) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	t.Cleanup(func() {
		ln.Close() // listenPipe's later Close is a no-op (go-winio closeOnce)
		within(t, "accept loop to exit after listener Close", wg.Wait)
	})
}

// listenExistsErrno is the Windows code observed when ListenPipe hits a name
// that already exists, even when the existing pipe belongs to the current user
// and process. go-winio creates the first instance with
// NtCreateNamedPipeFile(FILE_CREATE); design.md expected ERROR_ALREADY_EXISTS
// (STATUS_OBJECT_NAME_COLLISION), but the code observed on Windows 11 build
// 26200 with go-winio v0.6.4 is ERROR_ACCESS_DENIED (5), so that is what is
// pinned here. The agent (T10) must treat this code as "name already exists".
const listenExistsErrno = windows.ERROR_ACCESS_DENIED

// assertSecondListenFails listens on a fresh name, serves it, checks that a
// second ListenPipe on the same name fails preserving the *os.PathError
// (Op "open") and windows.Errno chain (IPC-007, IPC-019), and that the first
// listener still serves a ping afterwards.
func assertSecondListenFails(t *testing.T) {
	t.Helper()
	name := testPipeName(t)
	ln := listenPipe(t, name)
	srvClock := domain.NewFakeClock(testStart)
	startServer(t, ln, srvClock, ipc.AgentHandlers(testInfo(), srvClock), nil, ipc.WithStartedAt(testStart))

	ln2, err := ipc.ListenPipe(name)
	if err == nil {
		if ln2 != nil {
			ln2.Close()
		}
		t.Fatalf("second ListenPipe(%q) succeeded, want an error (first instance already owned)", name)
	}
	if ln2 != nil {
		ln2.Close()
		t.Fatalf("second ListenPipe(%q) returned a non-nil listener together with error %v", name, err)
	}

	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("second ListenPipe error %v (%T) does not wrap *os.PathError", err, err)
	}
	if pathErr.Op != "open" {
		t.Fatalf("os.PathError.Op = %q, want \"open\" (err %v)", pathErr.Op, err)
	}
	var errno windows.Errno
	if !errors.As(err, &errno) {
		t.Fatalf("second ListenPipe error %v does not wrap windows.Errno", err)
	}
	if errno != listenExistsErrno {
		t.Fatalf("windows.Errno = %d (%v), want %d (%v)", uint32(errno), errno, uint32(listenExistsErrno), listenExistsErrno)
	}

	// The failed second listen must not disturb the first one.
	conn, err := ipc.DialPipe(guardCtx(t), name)
	if err != nil {
		t.Fatalf("DialPipe(%q) after the failed second listen: %v", name, err)
	}
	newRawConn(t, conn).ping(t, 1)
}

// TestPipe covers IPC-007 against the real Windows named pipe (go-winio).
//
// goleak is intentionally not used here: go-winio starts a process-wide IO
// completion goroutine on first use that never exits, and these subtests run
// in parallel with other tests of the package. Clean shutdown is still
// asserted: Client.Close and Server.Close must return and Serve must exit
// (startServer/newClient cleanups with guard timeouts).
func TestPipe(t *testing.T) {
	t.Parallel()

	t.Run("IPC-007/nome segue arcadia-agent-<SID>", func(t *testing.T) {
		t.Parallel()
		sid, err := ipc.CurrentUserSID()
		if err != nil {
			t.Fatalf("CurrentUserSID: %v", err)
		}
		want := tokenUserSID(t).String()
		if sid != want {
			t.Fatalf("CurrentUserSID() = %q, want token user SID %q", sid, want)
		}
		if !strings.HasPrefix(sid, "S-1-") {
			t.Fatalf("CurrentUserSID() = %q, want a string SID (S-1-...)", sid)
		}
		if got, wantName := ipc.PipeName(sid), `\\.\pipe\arcadia-agent-`+want; got != wantName {
			t.Fatalf("PipeName(%q) = %q, want %q", sid, got, wantName)
		}
	})

	t.Run("IPC-007/usuário atual conecta e faz ping via Server", func(t *testing.T) {
		t.Parallel()
		name := testPipeName(t)
		ln := listenPipe(t, name)
		srvClock := domain.NewFakeClock(testStart)
		startServer(t, ln, srvClock, ipc.AgentHandlers(testInfo(), srvClock), nil, ipc.WithStartedAt(testStart))

		c := newClient(t, ipc.PipeDialer(name), domain.NewFakeClock(testStart))
		var pong ipc.PingResult
		if err := c.Call(guardCtx(t), ipc.MethodPing, nil, &pong); err != nil {
			t.Fatalf("Call ping over pipe %q: %v", name, err)
		}
		if !pong.Pong {
			t.Fatalf("ping result = %+v, want pong=true", pong)
		}

		// DialPipe on its own also reaches the server as the current user.
		conn, err := ipc.DialPipe(guardCtx(t), name)
		if err != nil {
			t.Fatalf("DialPipe(%q): %v", name, err)
		}
		newRawConn(t, conn).ping(t, 77) // newRawConn closes conn at cleanup
	})

	t.Run("IPC-007/segunda escuta no mesmo nome falha", func(t *testing.T) {
		t.Parallel()
		assertSecondListenFails(t)
	})

	t.Run("IPC-007/DACL protegida só com o SID atual", func(t *testing.T) {
		t.Parallel()
		name := testPipeName(t)
		ln := listenPipe(t, name)

		// go-winio v0.6.4 creates the first instance without accepting clients:
		// a connectable instance exists only while an Accept is pending
		// (design.md, T08 note). Keep one pending for the whole subtest.
		acceptPending(t, ln)

		// Opening the pipe by name consumes a server instance, so reading the
		// DACL by name (GetNamedSecurityInfo) would race the accept loop
		// re-arming its next instance. Instead DialPipe, which retries while the
		// pipe is busy, connects once the Accept is pending, and the DACL of the
		// pipe object is read through the client handle: GENERIC_READ includes
		// READ_CONTROL.
		conn, err := ipc.DialPipe(guardCtx(t), name)
		if err != nil {
			t.Fatalf("DialPipe(%q): %v", name, err)
		}
		defer conn.Close()
		fder, ok := conn.(interface{ Fd() uintptr })
		if !ok {
			t.Fatalf("DialPipe returned %T, which does not expose Fd()", conn)
		}
		sd, err := windows.GetSecurityInfo(windows.Handle(fder.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatalf("GetSecurityInfo(pipe %q): %v", name, err)
		}
		if sd == nil {
			t.Fatalf("pipe %q has no security descriptor", name)
		}

		control, _, err := sd.Control()
		if err != nil {
			t.Fatalf("security descriptor control: %v", err)
		}
		if control&windows.SE_DACL_PRESENT == 0 {
			t.Fatalf("DACL not present (control=%#x, sddl=%s)", control, sd)
		}
		if control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatalf("DACL not protected (control=%#x, sddl=%s)", control, sd)
		}

		dacl, _, err := sd.DACL()
		if err != nil {
			t.Fatalf("read DACL: %v", err)
		}
		if dacl == nil {
			t.Fatalf("NULL DACL (everyone allowed), want exactly one ACE (sddl=%s)", sd)
		}
		if dacl.AceCount != 1 {
			t.Fatalf("DACL has %d ACEs, want exactly 1 (sddl=%s)", dacl.AceCount, sd)
		}

		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, 0, &ace); err != nil {
			t.Fatalf("GetAce(0): %v", err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatalf("ACE type = %d, want ACCESS_ALLOWED_ACE_TYPE (sddl=%s)", ace.Header.AceType, sd)
		}
		if ace.Mask != windows.GENERIC_ALL && ace.Mask != fileAllAccess {
			t.Fatalf("ACE mask = %#x, want GENERIC_ALL (%#x) or FILE_ALL_ACCESS (%#x) (sddl=%s)",
				ace.Mask, windows.GENERIC_ALL, fileAllAccess, sd)
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if want := tokenUserSID(t); !aceSID.Equals(want) {
			t.Fatalf("ACE SID = %s, want current user %s (sddl=%s)", aceSID, want, sd)
		}
	})
}

// TestListenPipeExists covers the IPC-019 part of ListenPipe: on an existing
// name the error keeps the chain down to go-winio's *os.PathError and the
// windows.Errno, so the agent (T10) can tell "name already exists".
func TestListenPipeExists(t *testing.T) {
	t.Parallel()

	t.Run("IPC-019/ListenPipe em nome existente preserva os.PathError e windows.Errno", func(t *testing.T) {
		t.Parallel()
		assertSecondListenFails(t)
	})
}

// acceptInto runs an owned goroutine that accepts on ln and hands every
// accepted connection to the returned channel. At cleanup it closes ln, waits
// for the goroutine and closes connections nobody took.
func acceptInto(t *testing.T, ln net.Listener) <-chan net.Conn {
	t.Helper()
	conns := make(chan net.Conn, 16)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			select {
			case conns <- c:
			default:
				c.Close()
			}
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		within(t, "accept loop to exit after listener Close", wg.Wait)
		for {
			select {
			case c := <-conns:
				c.Close()
			default:
				return
			}
		}
	})
	return conns
}

// nextServerConn takes the next accepted connection (closed at cleanup).
func nextServerConn(t *testing.T, conns <-chan net.Conn) net.Conn {
	t.Helper()
	c := recv(t, conns, "server side to accept a connection")
	t.Cleanup(func() { c.Close() })
	return c
}

// rawPipeDial connects to name with go-winio directly, bypassing DialPipe and
// its owner check. The connection is closed at cleanup.
func rawPipeDial(t *testing.T, name string) net.Conn {
	t.Helper()
	conn, err := winio.DialPipeContext(guardCtx(t), name)
	if err != nil {
		t.Fatalf("winio.DialPipeContext(%q): %v", name, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// writeAsync writes data on conn from an owned goroutine (a pipe write may
// block until the peer reads). At cleanup conn is closed and the goroutine
// awaited.
func writeAsync(t *testing.T, conn net.Conn, data string) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.WriteString(conn, data)
	}()
	t.Cleanup(func() {
		conn.Close()
		within(t, "pipe write to return", wg.Wait)
	})
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// flushMarker is written by the flush connection of assertRejectedConnsClosed.
const flushMarker = "x"

// assertRejectedConnsClosed checks that every connection the server side
// accepted from rejected dials (before this call) reads EOF/error with no data
// and returns how many there were. It ends by dialing a raw "flush" connection
// that writes flushMarker: accepted connections are read in order until the
// flush one shows up. A rejected connection the server never saw (go-winio
// retries silently when ConnectNamedPipe reports ERROR_NO_DATA because the
// client already left) is also fine: nothing is left open. A rejected
// connection still open makes its read time out and fails the test.
func assertRejectedConnsClosed(t *testing.T, name string, conns <-chan net.Conn) int {
	t.Helper()
	flush := rawPipeDial(t, name)
	writeAsync(t, flush, flushMarker)

	rejected := 0
	for {
		c := nextServerConn(t, conns)
		if err := c.SetReadDeadline(time.Now().Add(guardTimeout)); err != nil {
			t.Fatalf("set read deadline on the server side: %v", err)
		}
		buf := make([]byte, 64)
		n, err := c.Read(buf)
		switch {
		case n > 0 && string(buf[:n]) == flushMarker:
			return rejected
		case n > 0:
			t.Fatalf("server read %q from a rejected connection, want EOF with no data", buf[:n])
		case err == nil:
			t.Fatal("server read neither data nor EOF from a rejected connection")
		case isTimeout(err):
			t.Fatalf("rejected connection still open on the server side (read timed out): %v", err)
		}
		rejected++ // EOF / broken pipe: the client closed it
	}
}

// otherUserSID is a SID that is never the test user: LocalSystem.
const otherUserSID = "S-1-5-18"

// TestDialPipeOwner covers IPC-019: DialPipe accepts a pipe served by the
// current user and rejects (ErrUntrustedServer, connection closed) a server
// owned by someone else or whose owner cannot be read.
//
// Not parallel, on purpose: the subtests that fake the server owner swap the
// package-level pipeServerSID seam (via ipc.SetPipeServerSID, restored in
// t.Cleanup) and no other test may dial a pipe meanwhile. A top-level test
// without t.Parallel runs, with all its non-parallel subtests, before any
// parallel test of the package (TestPipe, TestListenPipeExists, ...) is
// resumed, so the swap is never observed by them, also under -shuffle.
// Every subtest's goroutines (servers, clients, accept loops) end in its own
// cleanups, which run before the restore.
func TestDialPipeOwner(t *testing.T) {
	t.Run("IPC-019/servidor do mesmo usuário é aceito e responde ping", func(t *testing.T) {
		name := testPipeName(t)
		ln := listenPipe(t, name)
		srvClock := domain.NewFakeClock(testStart)
		startServer(t, ln, srvClock, ipc.AgentHandlers(testInfo(), srvClock), nil, ipc.WithStartedAt(testStart))

		conn, err := ipc.DialPipe(guardCtx(t), name)
		if err != nil {
			t.Fatalf("DialPipe(%q) to a pipe of the current user: %v", name, err)
		}
		if conn == nil {
			t.Fatalf("DialPipe(%q) returned a nil connection without error", name)
		}
		newRawConn(t, conn).ping(t, 1)
	})

	t.Run("IPC-019/realPipeServerSID do próprio pipe devolve CurrentUserSID", func(t *testing.T) {
		name := testPipeName(t)
		conns := acceptInto(t, listenPipe(t, name))
		conn := rawPipeDial(t, name)
		server := nextServerConn(t, conns)

		fder, ok := conn.(interface{ Fd() uintptr })
		if !ok {
			t.Fatalf("go-winio pipe connection %T does not expose Fd()", conn)
		}
		got, err := ipc.RealPipeServerSID(windows.Handle(fder.Fd()))
		if err != nil {
			t.Fatalf("realPipeServerSID(own pipe): %v", err)
		}
		want, err := ipc.CurrentUserSID()
		if err != nil {
			t.Fatalf("CurrentUserSID: %v", err)
		}
		if got != want {
			t.Fatalf("realPipeServerSID(own pipe) = %q, want CurrentUserSID %q", got, want)
		}
		if tok := tokenUserSID(t).String(); got != tok {
			t.Fatalf("realPipeServerSID(own pipe) = %q, want token user SID %q", got, tok)
		}

		// The handle still belongs to the connection: it was not closed.
		writeAsync(t, conn, "ok")
		if err := server.SetReadDeadline(time.Now().Add(guardTimeout)); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		buf := make([]byte, 2)
		if _, err := io.ReadFull(server, buf); err != nil || string(buf) != "ok" {
			t.Fatalf("server read %q, %v after realPipeServerSID; want \"ok\" (client handle must stay open)", buf, err)
		}
	})

	rejectCases := []struct {
		name  string
		owner func(windows.Handle) (string, error)
		cause string // text the error must carry ("" = not checked)
	}{
		{
			name:  "IPC-019/SID diferente retorna ErrUntrustedServer e fecha a conexão",
			owner: func(windows.Handle) (string, error) { return otherUserSID, nil },
		},
		{
			name: "IPC-019/falha ao ler o SID do servidor retorna ErrUntrustedServer e fecha a conexão",
			owner: func(windows.Handle) (string, error) {
				return "", errors.New("test: cannot read pipe server SID 51aa")
			},
			cause: "51aa",
		},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			t.Cleanup(ipc.SetPipeServerSID(func(h windows.Handle) (string, error) {
				calls.Add(1)
				return tc.owner(h)
			}))
			name := testPipeName(t)
			conns := acceptInto(t, listenPipe(t, name))

			conn, err := ipc.DialPipe(guardCtx(t), name)
			if conn != nil {
				conn.Close()
				t.Fatalf("DialPipe(%q) returned a connection (err %v), want none", name, err)
			}
			if !errors.Is(err, ipc.ErrUntrustedServer) {
				t.Fatalf("DialPipe(%q) = %v, want errors.Is(err, ErrUntrustedServer)", name, err)
			}
			if tc.cause != "" && !strings.Contains(err.Error(), tc.cause) {
				t.Fatalf("DialPipe error %q does not carry the cause %q", err, tc.cause)
			}
			if n := calls.Load(); n != 1 {
				t.Fatalf("pipeServerSID called %d times by one DialPipe, want 1", n)
			}

			if n := assertRejectedConnsClosed(t, name, conns); n > 1 {
				t.Fatalf("server accepted %d rejected connections from one DialPipe, want at most 1", n)
			} else if n == 0 {
				t.Log("the rejected connection never reached Accept (go-winio absorbed ERROR_NO_DATA)")
			}
		})
	}

	t.Run("IPC-019/Client com PipeDialer trata ErrUntrustedServer como falha de dial e não envia Call", func(t *testing.T) {
		var calls atomic.Int32
		t.Cleanup(ipc.SetPipeServerSID(func(windows.Handle) (string, error) {
			calls.Add(1)
			return otherUserSID, nil
		}))
		name := testPipeName(t)
		conns := acceptInto(t, listenPipe(t, name))

		clk := domain.NewFakeClock(testStart)
		log, logs := newCaptureLogger()
		c := ipc.NewClient(ipc.PipeDialer(name), clk, log)
		var closeOnce sync.Once
		closeClient := func() {
			closeOnce.Do(func() {
				within(t, "Client.Close", func() {
					if err := c.Close(); err != nil {
						t.Logf("Client.Close: %v", err)
					}
				})
			})
		}
		t.Cleanup(closeClient)

		callCtx, cancelCall := context.WithCancel(context.Background())
		t.Cleanup(cancelCall)
		var pong ipc.PingResult
		callErr := callAsync(callCtx, c, ipc.MethodPing, &pong)

		// A failed dial is followed by the IPC-013 backoff: 250 ms, then 500 ms.
		blockUntilWaiters(t, clk, 1)
		clk.Advance(250 * time.Millisecond)
		blockUntilWaiters(t, clk, 1)
		if n := calls.Load(); n < 2 {
			t.Fatalf("pipeServerSID called %d times after two dial attempts, want >= 2", n)
		}

		// design.md: ErrUntrustedServer is logged at Warn (possible attack), not Debug.
		if !logs.containsAt(slog.LevelWarn, "", ipc.ErrUntrustedServer.Error()) {
			t.Fatalf("ErrUntrustedServer dial failure not logged at Warn; logs: %v", logs.all())
		}

		select {
		case err := <-callErr:
			t.Fatalf("Call returned %v (result %+v) with only untrusted servers, want it waiting for a connection", err, pong)
		default:
		}
		cancelCall()
		if err := recv(t, callErr, "Call to return after its ctx was cancelled"); !errors.Is(err, context.Canceled) {
			t.Fatalf("Call = %v, want errors.Is(err, context.Canceled)", err)
		}
		closeClient()

		// Nothing reached the untrusted server: every connection it accepted
		// reads EOF with no data.
		n := assertRejectedConnsClosed(t, name, conns)
		t.Logf("server side saw %d rejected connection(s)", n)
	})
}
