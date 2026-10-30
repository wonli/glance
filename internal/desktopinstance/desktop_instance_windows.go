//go:build windows

package desktopinstance

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsDesktopLock struct {
	release   chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

type mutexAcquireResult struct {
	lock     *windowsDesktopLock
	acquired bool
	err      error
}

func ensureRuntimeDirectory() (string, error) { return "", nil }

func acquireDesktopLock(_ string) (interface{ Close() error }, bool, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, false, fmt.Errorf("read current user for desktop mutex ACL: %w", err)
	}
	userSID := tokenUser.User.Sid.String()
	if userSID == "" {
		return nil, false, errors.New("current Windows user SID is unavailable")
	}
	security, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + userSID + ")")
	if err != nil {
		return nil, false, fmt.Errorf("create current-user desktop mutex ACL: %w", err)
	}
	name, err := windows.UTF16PtrFromString(`Global\GlanceDesktop-` + compactSID(userSID))
	if err != nil {
		return nil, false, err
	}
	attributes := &windows.SecurityAttributes{
		Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: security,
	}
	result := make(chan mutexAcquireResult, 1)
	go func() {
		runtime.LockOSThread()
		handle, createErr := windows.CreateMutex(attributes, false, name)
		if handle == 0 || (createErr != nil && createErr != windows.ERROR_ALREADY_EXISTS) {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
			runtime.UnlockOSThread()
			result <- mutexAcquireResult{err: fmt.Errorf("create per-user desktop mutex: %w", createErr)}
			return
		}
		waitResult, waitErr := windows.WaitForSingleObject(handle, 0)
		if waitErr != nil {
			_ = windows.CloseHandle(handle)
			runtime.UnlockOSThread()
			result <- mutexAcquireResult{err: fmt.Errorf("check per-user desktop mutex: %w", waitErr)}
			return
		}
		if waitResult != windows.WAIT_OBJECT_0 && waitResult != windows.WAIT_ABANDONED {
			_ = windows.CloseHandle(handle)
			runtime.UnlockOSThread()
			result <- mutexAcquireResult{acquired: false}
			return
		}
		lock := &windowsDesktopLock{release: make(chan struct{}), done: make(chan struct{})}
		result <- mutexAcquireResult{lock: lock, acquired: true}
		<-lock.release
		lock.closeErr = errors.Join(windows.ReleaseMutex(handle), windows.CloseHandle(handle))
		runtime.UnlockOSThread()
		close(lock.done)
	}()
	acquired := <-result
	return acquired.lock, acquired.acquired, acquired.err
}

func (l *windowsDesktopLock) Close() error {
	if l == nil {
		return nil
	}
	l.closeOnce.Do(func() { close(l.release) })
	<-l.done
	return l.closeErr
}

var cancelSynchronousIO = windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelSynchronousIo")

type activationPipeServer struct {
	name      string
	security  *windows.SECURITY_DESCRIPTOR
	mu        sync.Mutex
	closed    bool
	active    windows.Handle
	connected bool
	done      chan struct{}
}

func startActivationServer(_ string, requests chan<- struct{}) (func() error, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("read current user for activation pipe ACL: %w", err)
	}
	userSID := tokenUser.User.Sid.String()
	if userSID == "" {
		return nil, errors.New("current Windows user SID is unavailable")
	}
	security, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + userSID + ")")
	if err != nil {
		return nil, fmt.Errorf("create current-user activation pipe ACL: %w", err)
	}
	server := &activationPipeServer{
		name: `\\.\pipe\GlanceActivate-` + compactSID(userSID), security: security, done: make(chan struct{}),
	}
	first, err := server.createPipe(true)
	if err != nil {
		return nil, fmt.Errorf("create initial desktop activation pipe: %w", err)
	}
	server.active = first
	threadReady := make(chan error, 1)
	var thread windows.Handle
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		var err error
		thread, err = windows.OpenThread(windows.THREAD_TERMINATE, false, windows.GetCurrentThreadId())
		threadReady <- err
		if err != nil {
			_ = windows.CloseHandle(first)
			close(server.done)
			return
		}
		server.acceptLoop(first, requests)
	}()
	if err := <-threadReady; err != nil {
		return nil, fmt.Errorf("open activation server thread: %w", err)
	}
	return func() error {
		server.mu.Lock()
		server.closed = true
		server.mu.Unlock()
		// Closing a handle from another thread does not reliably interrupt a
		// synchronous ConnectNamedPipe or ReadFile. Cancel the actual server
		// thread's I/O until it exits, covering the race before a call starts.
		ticker := time.NewTicker(time.Millisecond * 10)
		defer ticker.Stop()
		defer windows.CloseHandle(thread)
		for {
			cancelSynchronousIO.Call(uintptr(thread))
			select {
			case <-server.done:
				return nil
			case <-ticker.C:
			}
		}
	}, nil
}

func (s *activationPipeServer) createPipe(first bool) (windows.Handle, error) {
	flags := uint32(windows.PIPE_ACCESS_DUPLEX)
	if first {
		flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	mode := uint32(windows.PIPE_TYPE_BYTE | windows.PIPE_READMODE_BYTE | windows.PIPE_WAIT | windows.PIPE_REJECT_REMOTE_CLIENTS)
	attributes := &windows.SecurityAttributes{
		Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: s.security,
	}
	name, err := windows.UTF16PtrFromString(s.name)
	if err != nil {
		return 0, err
	}
	return windows.CreateNamedPipe(name, flags, mode, windows.PIPE_UNLIMITED_INSTANCES, 4096, 4096, 0, attributes)
}

func (s *activationPipeServer) acceptLoop(handle windows.Handle, requests chan<- struct{}) {
	defer close(s.done)
	first := false
	for {
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed {
			_ = windows.CloseHandle(handle)
			return
		}
		err := windows.ConnectNamedPipe(handle, nil)
		if err != nil && err != windows.ERROR_PIPE_CONNECTED {
			s.mu.Lock()
			closed := s.closed
			s.active = 0
			s.mu.Unlock()
			_ = windows.CloseHandle(handle)
			if closed {
				return
			}
			var createErr error
			handle, createErr = s.createPipe(first)
			if createErr != nil {
				return
			}
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				_ = windows.CloseHandle(handle)
				return
			}
			s.active = handle
			s.mu.Unlock()
			continue
		}
		first = false
		s.mu.Lock()
		if s.closed {
			s.active = 0
			s.mu.Unlock()
			_ = windows.CloseHandle(handle)
			return
		}
		s.connected = true
		s.mu.Unlock()
		serveActivationPipe(handle, requests)

		s.mu.Lock()
		closed = s.closed
		s.active = 0
		s.connected = false
		s.mu.Unlock()
		if closed {
			return
		}
		var createErr error
		handle, createErr = s.createPipe(first)
		if createErr != nil {
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = windows.CloseHandle(handle)
			return
		}
		s.active = handle
		s.mu.Unlock()
	}
}

func serveActivationPipe(handle windows.Handle, requests chan<- struct{}) {
	file := os.NewFile(uintptr(handle), "glance-activation-pipe")
	if file == nil {
		_ = windows.CloseHandle(handle)
		return
	}
	defer file.Close()
	if err := readActivationFrame(file); err != nil {
		return
	}
	select {
	case requests <- struct{}{}:
	default:
	}
	_, _ = file.Write([]byte{0xAC})
}

func sendActivationRequest(_ string) error {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("read current user for activation pipe: %w", err)
	}
	userSID := tokenUser.User.Sid.String()
	if userSID == "" {
		return errors.New("current Windows user SID is unavailable")
	}
	address := `\\.\pipe\GlanceActivate-` + compactSID(userSID)
	name, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return fmt.Errorf("open desktop activation pipe: %w", err)
	}
	file := os.NewFile(uintptr(handle), address)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("wrap desktop activation pipe")
	}
	defer file.Close()
	if err := writeActivationFrame(file); err != nil {
		return err
	}
	var ack [1]byte
	if _, err := io.ReadFull(file, ack[:]); err != nil {
		return err
	}
	if ack[0] != 0xAC {
		return errors.New("desktop activation pipe returned an invalid acknowledgement")
	}
	return nil
}

func compactSID(sid string) string {
	var result strings.Builder
	for _, r := range sid {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		}
	}
	return result.String()
}
