//go:build windows

package mcpserver

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type desktopNamedPipeAddr string

func (a desktopNamedPipeAddr) Network() string { return "named-pipe" }
func (a desktopNamedPipeAddr) String() string  { return string(a) }

type desktopNamedPipeListener struct {
	name      string
	security  *windows.SECURITY_DESCRIPTOR
	mu        sync.Mutex
	closed    bool
	first     bool
	accepting windows.Handle
	active    *desktopNamedPipeAccept
	acceptErr error
	ready     chan struct{}
	readyOnce sync.Once
}

type desktopNamedPipeAccept struct {
	handle     windows.Handle
	event      windows.Handle
	overlapped windows.Overlapped
	done       chan struct{}
	pending    bool
}

type desktopNamedPipeConn struct {
	file *os.File
	addr desktopNamedPipeAddr
}

func listenDesktopIPC(_ string, instanceID string) (net.Listener, string, error) {
	if !validDesktopMCPInstanceID(instanceID) {
		return nil, "", errors.New("invalid desktop MCP instance ID")
	}
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, "", fmt.Errorf("read current user for named pipe ACL: %w", err)
	}
	userSID := tokenUser.User.Sid.String()
	if userSID == "" {
		return nil, "", errors.New("current Windows user SID is unavailable")
	}
	security, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + userSID + ")")
	if err != nil {
		return nil, "", fmt.Errorf("create current-user named pipe ACL: %w", err)
	}
	name := fmt.Sprintf(`\\.\pipe\GlanceMCP-%s-%s`, compactSID(userSID), instanceID)
	listener := &desktopNamedPipeListener{name: name, security: security, first: true, ready: make(chan struct{})}
	listener.mu.Lock()
	listener.accepting, err = listener.createPipeLocked()
	listener.mu.Unlock()
	if err != nil {
		return nil, "", fmt.Errorf("create initial desktop MCP named pipe: %w", err)
	}
	return listener, name, nil
}

func defaultDesktopMCPSocketDirectory() (string, error) { return "", nil }

func ensureDesktopMCPSocketDirectory(string) error { return nil }

func compactSID(sid string) string {
	var result strings.Builder
	for _, r := range sid {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func dialDesktopIPC(address string) (net.Conn, error) {
	name, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("open desktop MCP named pipe: %w", err)
	}
	file := os.NewFile(uintptr(handle), address)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("wrap desktop MCP named pipe handle")
	}
	return &desktopNamedPipeConn{file: file, addr: desktopNamedPipeAddr(address)}, nil
}

func removeStaleDesktopIPC(_, address string) error {
	if !strings.HasPrefix(strings.ToLower(address), `\\.\pipe\glancemcp-`) {
		return fmt.Errorf("desktop MCP IPC address %q is not a Glance named pipe", address)
	}
	return nil
}

func processExists(pid int) (bool, error) {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return false, nil
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return false, nil
		}
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return true, nil
		}
		return false, err
	}
	_ = windows.CloseHandle(handle)
	return true, nil
}

func (l *desktopNamedPipeListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, net.ErrClosed
	}
	if l.acceptErr != nil {
		err := l.acceptErr
		l.mu.Unlock()
		return nil, err
	}
	handle := l.accepting
	if handle == 0 {
		var err error
		handle, err = l.createPipeLocked()
		if err != nil {
			l.mu.Unlock()
			return nil, fmt.Errorf("create desktop MCP named pipe: %w", err)
		}
		l.accepting = handle
	}
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		l.mu.Unlock()
		return nil, fmt.Errorf("create desktop MCP named pipe event: %w", err)
	}
	accept := &desktopNamedPipeAccept{handle: handle, event: event, done: make(chan struct{})}
	accept.overlapped.HEvent = event
	l.active = accept
	err = windows.ConnectNamedPipe(handle, &accept.overlapped)
	switch {
	case err == nil, errors.Is(err, windows.ERROR_PIPE_CONNECTED):
	case errors.Is(err, windows.ERROR_IO_PENDING):
		accept.pending = true
	default:
		l.active = nil
		if l.accepting == handle {
			l.accepting = 0
		}
		_ = windows.CloseHandle(event)
		_ = windows.CloseHandle(handle)
		l.mu.Unlock()
		return nil, fmt.Errorf("accept desktop MCP named pipe connection: %w", err)
	}
	l.mu.Unlock()

	l.readyOnce.Do(func() { close(l.ready) })
	if accept.pending {
		waitResult, waitErr := windows.WaitForSingleObject(event, windows.INFINITE)
		if waitErr != nil {
			err = waitErr
		} else if waitResult != windows.WAIT_OBJECT_0 {
			err = fmt.Errorf("wait for desktop MCP named pipe connection returned %d", waitResult)
		} else {
			var transferred uint32
			err = windows.GetOverlappedResult(handle, &accept.overlapped, &transferred, false)
		}
	}

	l.mu.Lock()
	closed := l.closed
	if l.active == accept {
		l.active = nil
	}
	if l.accepting == handle {
		l.accepting = 0
	}
	if !closed {
		nextHandle, createErr := l.createPipeLocked()
		if createErr != nil {
			l.acceptErr = fmt.Errorf("create desktop MCP named pipe: %w", createErr)
		} else {
			l.accepting = nextHandle
		}
	}
	_ = windows.CloseHandle(event)
	close(accept.done)
	l.mu.Unlock()
	if err != nil {
		_ = windows.CloseHandle(handle)
		if closed {
			return nil, net.ErrClosed
		}
		return nil, fmt.Errorf("accept desktop MCP named pipe connection: %w", err)
	}
	if closed {
		_ = windows.CloseHandle(handle)
		return nil, net.ErrClosed
	}
	file := os.NewFile(uintptr(handle), l.name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("wrap accepted desktop MCP named pipe")
	}
	return &desktopNamedPipeConn{file: file, addr: desktopNamedPipeAddr(l.name)}, nil
}

func (l *desktopNamedPipeListener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	active := l.active
	handle := l.accepting
	if active != nil {
		// ConnectNamedPipe is issued with OVERLAPPED while holding mu, so a
		// pending operation is fully registered before Close can cancel it.
		if active.pending {
			if err := windows.CancelIoEx(active.handle, &active.overlapped); err != nil &&
				!errors.Is(err, windows.ERROR_NOT_FOUND) &&
				!errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
				l.mu.Unlock()
				return fmt.Errorf("cancel desktop MCP named pipe accept: %w", err)
			}
		}
	} else {
		l.accepting = 0
	}
	l.mu.Unlock()
	if active != nil {
		<-active.done
		return nil
	}
	if handle != 0 {
		if err := windows.CloseHandle(handle); err != nil && !errors.Is(err, windows.ERROR_INVALID_HANDLE) {
			return err
		}
	}
	return nil
}

func (l *desktopNamedPipeListener) Addr() net.Addr {
	return desktopNamedPipeAddr(l.name)
}

func (l *desktopNamedPipeListener) Ready() <-chan struct{} {
	return l.ready
}

func (l *desktopNamedPipeListener) createPipeLocked() (windows.Handle, error) {
	flags := uint32(windows.PIPE_ACCESS_DUPLEX | windows.FILE_FLAG_OVERLAPPED)
	if l.first {
		flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	mode := uint32(windows.PIPE_TYPE_BYTE | windows.PIPE_READMODE_BYTE | windows.PIPE_WAIT | windows.PIPE_REJECT_REMOTE_CLIENTS)
	attributes := &windows.SecurityAttributes{
		Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: l.security,
	}
	name, err := windows.UTF16PtrFromString(l.name)
	if err != nil {
		return 0, err
	}
	handle, err := windows.CreateNamedPipe(name, flags, mode, windows.PIPE_UNLIMITED_INSTANCES, 64*1024, 64*1024, 0, attributes)
	if err != nil {
		return 0, err
	}
	l.first = false
	return handle, nil
}

func (c *desktopNamedPipeConn) Read(data []byte) (int, error)  { return c.file.Read(data) }
func (c *desktopNamedPipeConn) Write(data []byte) (int, error) { return c.file.Write(data) }
func (c *desktopNamedPipeConn) Close() error                   { return c.file.Close() }
func (c *desktopNamedPipeConn) LocalAddr() net.Addr            { return c.addr }
func (c *desktopNamedPipeConn) RemoteAddr() net.Addr           { return c.addr }
func (c *desktopNamedPipeConn) SetDeadline(time.Time) error {
	return errors.New("deadlines are not supported for desktop MCP named pipes")
}
func (c *desktopNamedPipeConn) SetReadDeadline(time.Time) error {
	return errors.New("deadlines are not supported for desktop MCP named pipes")
}
func (c *desktopNamedPipeConn) SetWriteDeadline(time.Time) error {
	return errors.New("deadlines are not supported for desktop MCP named pipes")
}
