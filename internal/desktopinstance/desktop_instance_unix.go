//go:build !windows

package desktopinstance

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const unixSocketPathLimit = 100

type unixDesktopLock struct {
	file *os.File
}

func ensureRuntimeDirectory() (string, error) {
	// Keep the lock and activation endpoint stable across launch environments;
	// TMPDIR can differ between Finder, shells, and individually launched apps.
	path := filepath.Join("/tmp", "gm-"+strconv.Itoa(os.Getuid()))
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("create private desktop runtime directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect private desktop runtime directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("desktop runtime path %q is not a directory", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() {
		return "", fmt.Errorf("desktop runtime directory %q is not owned by the current user", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return "", fmt.Errorf("restrict desktop runtime directory permissions: %w", err)
	}
	return path, nil
}

func acquireDesktopLock(runtimeDir string) (interface{ Close() error }, bool, error) {
	path := filepath.Join(runtimeDir, "glance-desktop.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open desktop instance lock: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, false, fmt.Errorf("inspect desktop instance lock: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, false, errors.New("desktop instance lock is not a regular file")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Getuid() {
		_ = file.Close()
		return nil, false, errors.New("desktop instance lock is not owned by the current user")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, false, fmt.Errorf("restrict desktop instance lock permissions: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("acquire desktop instance lock: %w", err)
	}
	return &unixDesktopLock{file: file}, true, nil
}

func (l *unixDesktopLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	return errors.Join(unlockErr, closeErr)
}

func startActivationServer(runtimeDir string, requests chan<- struct{}) (func() error, error) {
	if err := removeStaleDesktopMCPSocket(runtimeDir); err != nil {
		return nil, err
	}
	path := filepath.Join(runtimeDir, "glance-activate.sock")
	if len([]byte(path)) > unixSocketPathLimit {
		return nil, fmt.Errorf("desktop activation socket path is too long (%d bytes)", len([]byte(path)))
	}
	if err := removeOwnedStaleSocket(path, runtimeDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on desktop activation socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = removeOwnedStaleSocket(path, runtimeDir)
		return nil, fmt.Errorf("restrict desktop activation socket permissions: %w", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			handleActivationConnection(conn, requests)
		}
	}()
	return func() error {
		closeErr := listener.Close()
		<-done
		removeErr := removeOwnedStaleSocket(path, runtimeDir)
		if errors.Is(closeErr, net.ErrClosed) {
			closeErr = nil
		}
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		return errors.Join(closeErr, removeErr)
	}, nil
}

func removeStaleDesktopMCPSocket(runtimeDir string) error {
	path := filepath.Join(runtimeDir, "glance-mcp.sock")
	if conn, err := net.DialTimeout("unix", path, 100*time.Millisecond); err == nil {
		_ = conn.Close()
		return errors.New("a previous Glance process still owns the desktop MCP socket; close it before starting this release")
	}
	if err := removeOwnedStaleSocket(path, runtimeDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale desktop MCP socket: %w", err)
	}
	return nil
}

func handleActivationConnection(conn net.Conn, requests chan<- struct{}) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if err := readActivationFrame(conn); err != nil {
		return
	}
	select {
	case requests <- struct{}{}:
	default:
	}
	_, _ = conn.Write([]byte{0xAC})
}

func sendActivationRequest(runtimeDir string) error {
	path := filepath.Join(runtimeDir, "glance-activate.sock")
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	if err := writeActivationFrame(conn); err != nil {
		return err
	}
	var ack [1]byte
	if _, err := io.ReadFull(conn, ack[:]); err != nil {
		return err
	}
	if ack[0] != 0xAC {
		return errors.New("desktop activation endpoint returned an invalid acknowledgement")
	}
	return nil
}

func removeOwnedStaleSocket(path, runtimeDir string) error {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	dirAbs, err := filepath.Abs(runtimeDir)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(dirAbs, pathAbs)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("desktop activation socket %q is outside its private runtime directory", path)
	}
	info, err := os.Lstat(pathAbs)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("desktop activation path %q is not a Unix socket", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("desktop activation socket %q is not owned by the current user", path)
	}
	return os.Remove(pathAbs)
}
