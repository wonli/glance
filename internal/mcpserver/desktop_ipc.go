package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"glance/internal/application"
)

// DesktopMCPInstance is retained as runtime metadata for existing desktop UI
// callers. Its ID is empty because the desktop MCP endpoint is now per-user.
type DesktopMCPInstance struct {
	ID             string    `json:"id,omitempty"`
	PID            int       `json:"pid"`
	ExecutablePath string    `json:"executable_path"`
	Address        string    `json:"address"`
	StartedAt      time.Time `json:"started_at"`
}

// DesktopIPCOptions allows tests to isolate the stable endpoint. RuntimeDir is
// a legacy fallback directory when SocketDir is empty; it is no longer a
// registry location.
type DesktopIPCOptions struct {
	RuntimeDir string
	SocketDir  string
}

// DesktopIPCServer serves MCP sessions against one existing desktop Runtime.
type DesktopIPCServer struct {
	instance    DesktopMCPInstance
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

// StartDesktopIPCServer serves the current desktop Runtime on the stable
// current-user endpoint. executablePath is retained for runtime display and
// generated settings JSON; it does not select an endpoint.
func StartDesktopIPCServer(runtime *application.Runtime, executablePath string, opts DesktopIPCOptions) (*DesktopIPCServer, error) {
	if runtime == nil {
		return nil, errors.New("application runtime is required")
	}
	if executablePath != "" {
		absolute, err := filepath.Abs(executablePath)
		if err != nil {
			return nil, fmt.Errorf("resolve Glance executable path: %w", err)
		}
		executablePath = absolute
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
	instanceServer, err := New(runtime)
	if err != nil {
		return nil, fmt.Errorf("create desktop MCP server: %w", err)
	}
	listener, address, err := listenDesktopIPC(socketDir, "")
	if err != nil {
		_ = instanceServer.CloseSessions()
		return nil, fmt.Errorf("listen for desktop MCP IPC: %w", err)
	}
	endpoint := &DesktopIPCServer{
		instance: DesktopMCPInstance{
			PID: os.Getpid(), ExecutablePath: executablePath, Address: address, StartedAt: time.Now().UTC(),
		},
		socketDir: socketDir, listener: listener, mcp: instanceServer,
		acceptDone: make(chan struct{}), activeConns: make(map[net.Conn]struct{}),
	}
	go endpoint.acceptLoop()
	if readyListener, ok := listener.(interface{ Ready() <-chan struct{} }); ok {
		<-readyListener.Ready()
	}
	return endpoint, nil
}

// Instance returns process metadata and the stable endpoint address. Its ID is
// intentionally empty and must not be used to route a connection.
func (s *DesktopIPCServer) Instance() DesktopMCPInstance {
	if s == nil {
		return DesktopMCPInstance{}
	}
	return s.instance
}

// Close stops accepting sessions, terminates active MCP connections, and
// removes the Unix endpoint created by this server.
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
		if s.socketDir != "" && s.instance.Address != "" {
			if err := removeStaleDesktopIPC(s.socketDir, s.instance.Address); err != nil && !errors.Is(err, os.ErrNotExist) {
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

func desktopMCPSocketDirectory(opts DesktopIPCOptions) (string, error) {
	directory := opts.SocketDir
	if directory == "" {
		directory = opts.RuntimeDir
	}
	if directory != "" {
		return filepath.Abs(directory)
	}
	return defaultDesktopMCPSocketDirectory()
}

func desktopMCPAddressForOptions(opts DesktopIPCOptions) (string, error) {
	socketDir, err := desktopMCPSocketDirectory(opts)
	if err != nil {
		return "", err
	}
	if socketDir != "" {
		if err := ensureDesktopMCPSocketDirectory(socketDir); err != nil {
			return "", err
		}
	}
	return desktopMCPAddress(socketDir)
}
