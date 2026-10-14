package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"glance/internal/application"
)

// DesktopMCPInstance identifies one live Glance process and its private MCP
// IPC endpoint. Its ID, not its PID or executable path, selects a process.
type DesktopMCPInstance struct {
	ID             string    `json:"id"`
	PID            int       `json:"pid"`
	ExecutablePath string    `json:"executable_path"`
	Address        string    `json:"address"`
	StartedAt      time.Time `json:"started_at"`
}

// DesktopIPCOptions overrides the registry directory for isolated tests.
// Production callers should leave RuntimeDir empty.
type DesktopIPCOptions struct {
	RuntimeDir string
	SocketDir  string
}

// DesktopIPCServer serves MCP sessions against one existing desktop Runtime.
type DesktopIPCServer struct {
	instance    DesktopMCPInstance
	runtimeDir  string
	socketDir   string
	listener    net.Listener
	mcp         *Server
	acceptDone  chan struct{}
	connMu      sync.Mutex
	activeConns map[net.Conn]struct{}
	closing     bool
	connections sync.WaitGroup
	closeOnce   sync.Once
	closeErr    error
}

// StartDesktopIPCServer publishes one per-process IPC endpoint after binding
// and starting its accept loop. It never constructs another application Runtime.
func StartDesktopIPCServer(runtime *application.Runtime, executablePath string, opts DesktopIPCOptions) (*DesktopIPCServer, error) {
	if runtime == nil {
		return nil, errors.New("application runtime is required")
	}
	executablePath, err := canonicalExecutablePath(executablePath)
	if err != nil {
		return nil, fmt.Errorf("resolve Glance executable path: %w", err)
	}
	runtimeDir, err := desktopMCPRuntimeDirectory(opts)
	if err != nil {
		return nil, err
	}
	if err := ensureDesktopMCPRuntimeDirectory(runtimeDir); err != nil {
		return nil, err
	}
	socketDir, err := desktopMCPSocketDirectory(opts)
	if err != nil {
		return nil, err
	}
	if socketDir != "" {
		if err := ensureDesktopMCPSocketDirectory(socketDir); err != nil {
			return nil, err
		}
	}
	instanceID, err := newDesktopMCPInstanceID()
	if err != nil {
		return nil, fmt.Errorf("create desktop MCP instance ID: %w", err)
	}
	instanceServer, err := New(runtime)
	if err != nil {
		return nil, fmt.Errorf("create desktop MCP server: %w", err)
	}
	listener, address, err := listenDesktopIPC(socketDir, instanceID)
	if err != nil {
		return nil, fmt.Errorf("listen for desktop MCP IPC: %w", err)
	}
	instance := DesktopMCPInstance{
		ID: instanceID, PID: os.Getpid(), ExecutablePath: executablePath,
		Address: address, StartedAt: time.Now().UTC(),
	}
	endpoint := &DesktopIPCServer{
		instance: instance, runtimeDir: runtimeDir, socketDir: socketDir, listener: listener,
		mcp: instanceServer, acceptDone: make(chan struct{}), activeConns: make(map[net.Conn]struct{}),
	}
	go endpoint.acceptLoop()
	if readyListener, ok := listener.(interface{ Ready() <-chan struct{} }); ok {
		<-readyListener.Ready()
	}
	if err := writeDesktopMCPInstance(runtimeDir, instance); err != nil {
		_ = endpoint.Close()
		return nil, fmt.Errorf("publish desktop MCP instance: %w", err)
	}
	return endpoint, nil
}

// Instance returns the process identity published in the registry.
func (s *DesktopIPCServer) Instance() DesktopMCPInstance {
	if s == nil {
		return DesktopMCPInstance{}
	}
	return s.instance
}

// Close stops accepting connections, terminates active MCP sessions, and
// removes the endpoint and its registry entry.
func (s *DesktopIPCServer) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		var closeErrors []error
		s.connMu.Lock()
		s.closing = true
		s.connMu.Unlock()
		if s.listener != nil {
			if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				closeErrors = append(closeErrors, err)
			}
		}
		if s.acceptDone != nil {
			<-s.acceptDone
		}
		s.connMu.Lock()
		for conn := range s.activeConns {
			if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				closeErrors = append(closeErrors, err)
			}
		}
		s.connMu.Unlock()
		if s.mcp != nil {
			if err := s.mcp.CloseSessions(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		s.connections.Wait()
		if s.runtimeDir != "" {
			if err := removeDesktopMCPInstance(s.runtimeDir, s.instance.ID); err != nil && !os.IsNotExist(err) {
				closeErrors = append(closeErrors, err)
			}
			if err := removeStaleDesktopIPC(s.socketDir, s.instance.Address); err != nil && !os.IsNotExist(err) {
				closeErrors = append(closeErrors, err)
			}
		}
		s.closeErr = errors.Join(closeErrors...)
	})
	return s.closeErr
}

func (s *DesktopIPCServer) acceptLoop() {
	defer close(s.acceptDone)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.connMu.Lock()
		if s.closing {
			s.connMu.Unlock()
			_ = conn.Close()
			continue
		}
		s.activeConns[conn] = struct{}{}
		s.connMu.Unlock()
		s.connections.Add(1)
		go func(conn net.Conn) {
			defer s.connections.Done()
			defer func() {
				_ = conn.Close()
				s.connMu.Lock()
				delete(s.activeConns, conn)
				s.connMu.Unlock()
			}()
			_ = s.mcp.ServeConn(context.Background(), conn)
		}(conn)
	}
}

// ListDesktopMCPInstances returns live registrations sorted by instance ID.
// Stale records and their Unix sockets are removed only after process liveness
// has been checked.
func ListDesktopMCPInstances(opts DesktopIPCOptions) ([]DesktopMCPInstance, error) {
	runtimeDir, err := desktopMCPRuntimeDirectory(opts)
	if err != nil {
		return nil, err
	}
	if err := validateDesktopMCPRuntimeDirectory(runtimeDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	socketDir, err := desktopMCPSocketDirectory(opts)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return nil, fmt.Errorf("read desktop MCP registry: %w", err)
	}
	instances := make([]DesktopMCPInstance, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(runtimeDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read desktop MCP record %q: %w", entry.Name(), err)
		}
		var instance DesktopMCPInstance
		if err := json.Unmarshal(data, &instance); err != nil || !validDesktopMCPInstanceID(instance.ID) || entry.Name() != instance.ID+".json" || instance.PID <= 0 || instance.Address == "" || instance.ExecutablePath == "" {
			continue
		}
		alive, err := processExists(instance.PID)
		if err != nil {
			return nil, fmt.Errorf("check process %d for desktop MCP record %q: %w", instance.PID, instance.ID, err)
		}
		if !alive {
			if err := removeStaleDesktopIPC(socketDir, instance.Address); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("remove stale desktop MCP IPC for %q: %w", instance.ID, err)
			}
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("remove stale desktop MCP record %q: %w", instance.ID, err)
			}
			continue
		}
		instances = append(instances, instance)
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
	return instances, nil
}

// ResolveDesktopMCPInstance validates a specific ID or uses the executable
// path only when it identifies exactly one live instance. A stale ID may use
// the same unique-path fallback.
func ResolveDesktopMCPInstance(instanceID, executablePath string, opts DesktopIPCOptions) (DesktopMCPInstance, error) {
	executablePath, err := canonicalExecutablePath(executablePath)
	if err != nil {
		return DesktopMCPInstance{}, fmt.Errorf("resolve Glance executable path: %w", err)
	}
	instances, err := ListDesktopMCPInstances(opts)
	if err != nil {
		return DesktopMCPInstance{}, err
	}
	if instanceID != "" {
		for _, instance := range instances {
			if instance.ID != instanceID {
				continue
			}
			if instance.ExecutablePath != executablePath {
				return DesktopMCPInstance{}, fmt.Errorf("desktop MCP instance %q belongs to a different executable", instanceID)
			}
			return instance, nil
		}
	}
	var matches []DesktopMCPInstance
	for _, instance := range instances {
		if instance.ExecutablePath == executablePath {
			matches = append(matches, instance)
		}
	}
	switch len(matches) {
	case 0:
		return DesktopMCPInstance{}, fmt.Errorf("no running Glance desktop instance matches executable %q", executablePath)
	case 1:
		return matches[0], nil
	default:
		return DesktopMCPInstance{}, fmt.Errorf("ambiguous desktop MCP target: %d running Glance instances match this executable; copy the configuration again from Settings → MCP", len(matches))
	}
}

func desktopMCPRuntimeDirectory(opts DesktopIPCOptions) (string, error) {
	if strings.TrimSpace(opts.RuntimeDir) != "" {
		return filepath.Abs(opts.RuntimeDir)
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve per-user cache directory for desktop MCP: %w", err)
	}
	return filepath.Join(cacheDir, "glance", "mcp-instances"), nil
}

func desktopMCPSocketDirectory(opts DesktopIPCOptions) (string, error) {
	if strings.TrimSpace(opts.SocketDir) != "" {
		return filepath.Abs(opts.SocketDir)
	}
	return defaultDesktopMCPSocketDirectory()
}

func ensureDesktopMCPRuntimeDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create private desktop MCP runtime directory: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("restrict desktop MCP runtime directory permissions: %w", err)
	}
	return validateDesktopMCPRuntimeDirectory(path)
}

func validateDesktopMCPRuntimeDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("desktop MCP runtime path %q is not a private directory", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("restrict desktop MCP runtime directory permissions: %w", err)
		}
	}
	return nil
}

func newDesktopMCPInstanceID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

func validDesktopMCPInstanceID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func canonicalExecutablePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("executable path is empty")
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func writeDesktopMCPInstance(runtimeDir string, instance DesktopMCPInstance) error {
	data, err := json.Marshal(instance)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(runtimeDir, ".desktop-mcp-instance-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, filepath.Join(runtimeDir, instance.ID+".json"))
}

func removeDesktopMCPInstance(runtimeDir, instanceID string) error {
	if !validDesktopMCPInstanceID(instanceID) {
		return errors.New("invalid desktop MCP instance ID")
	}
	return os.Remove(filepath.Join(runtimeDir, instanceID+".json"))
}
