//go:build !windows

package mcpserver

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func listenDesktopIPC(socketDir, _ string) (net.Listener, string, error) {
	address, err := desktopMCPAddress(socketDir)
	if err != nil {
		return nil, "", err
	}
	if len([]byte(address)) > 100 {
		return nil, "", fmt.Errorf("desktop MCP Unix socket path is too long (%d bytes)", len([]byte(address)))
	}
	if conn, err := net.DialTimeout("unix", address, 100*time.Millisecond); err == nil {
		_ = conn.Close()
		return nil, "", fmt.Errorf("desktop MCP endpoint %q is already active", address)
	}
	listener, err := net.Listen("unix", address)
	if err != nil {
		return nil, "", err
	}
	if err := os.Chmod(address, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(address)
		return nil, "", fmt.Errorf("restrict Unix socket permissions: %w", err)
	}
	return listener, address, nil
}

func desktopMCPAddress(socketDir string) (string, error) {
	if socketDir == "" {
		return "", errors.New("desktop MCP socket directory is empty")
	}
	return filepath.Join(socketDir, "glance-mcp.sock"), nil
}

func defaultDesktopMCPSocketDirectory() (string, error) {
	// Use a stable system temp root so GUI and shell launches share the same
	// per-user desktop endpoint even when their TMPDIR values differ.
	return filepath.Join("/tmp", "gm-"+strconv.Itoa(os.Getuid())), nil
}

func ensureDesktopMCPSocketDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create private desktop MCP socket directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("desktop MCP socket path %q is not a directory", path)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("desktop MCP socket directory %q is owned by another user", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("restrict desktop MCP socket directory permissions: %w", err)
	}
	return nil
}

func dialDesktopIPC(address string) (net.Conn, error) {
	return net.Dial("unix", address)
}

func removeStaleDesktopIPC(socketDir, address string) error {
	socketAbs, err := filepath.Abs(socketDir)
	if err != nil {
		return err
	}
	addressAbs, err := filepath.Abs(address)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(socketAbs, addressAbs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("desktop MCP socket path %q is outside its runtime directory", address)
	}
	info, err := os.Lstat(addressAbs)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("desktop MCP IPC path %q is not a Unix socket", address)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("desktop MCP IPC path %q is not owned by the current user", address)
	}
	return os.Remove(addressAbs)
}
