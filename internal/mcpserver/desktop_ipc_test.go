package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newDesktopIPCTestRuntime(t *testing.T) *application.Runtime {
	t.Helper()
	runtime, err := application.NewRuntime(context.Background(), application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("create application runtime: %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(context.Background()); err != nil {
			t.Errorf("close application runtime: %v", err)
		}
	})
	return runtime
}

func desktopIPCExecutablePath(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return path
}

func newDesktopIPCOptions(t *testing.T) DesktopIPCOptions {
	t.Helper()
	socketDir, err := os.MkdirTemp(os.TempDir(), "g-")
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	if err := os.Chmod(socketDir, 0o700); err != nil {
		t.Fatalf("restrict socket directory permissions: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	return DesktopIPCOptions{RuntimeDir: t.TempDir(), SocketDir: socketDir}
}

func TestDesktopMCPServesOneStableEndpoint(t *testing.T) {
	runtime := newDesktopIPCTestRuntime(t)
	options := newDesktopIPCOptions(t)
	first, err := StartDesktopIPCServer(runtime, desktopIPCExecutablePath(t), options)
	if err != nil {
		t.Fatalf("start desktop IPC server: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	instance := first.Instance()
	address, err := desktopMCPAddressForOptions(options)
	if err != nil {
		t.Fatalf("resolve stable desktop endpoint: %v", err)
	}
	if instance.ID != "" || instance.Address != address || instance.PID != os.Getpid() {
		t.Fatalf("desktop MCP metadata = %+v; want stable address %q, empty ID, and current PID", instance, address)
	}
	second, err := StartDesktopIPCServer(runtime, desktopIPCExecutablePath(t), options)
	if err == nil {
		_ = second.Close()
		t.Fatal("a second server unexpectedly claimed the stable desktop MCP endpoint")
	}
	conn, err := dialDesktopIPC(address)
	if err != nil {
		t.Fatalf("dial stable desktop MCP endpoint: %v", err)
	}
	_ = conn.Close()
}

func TestDesktopMCPDoesNotCreateRegistryDirectory(t *testing.T) {
	runtime := newDesktopIPCTestRuntime(t)
	options := newDesktopIPCOptions(t)
	registryDir := filepath.Join(t.TempDir(), "unused-registry")
	options.RuntimeDir = registryDir
	endpoint, err := StartDesktopIPCServer(runtime, desktopIPCExecutablePath(t), options)
	if err != nil {
		t.Fatalf("start desktop IPC server: %v", err)
	}
	t.Cleanup(func() { _ = endpoint.Close() })
	if _, err := os.Stat(registryDir); !os.IsNotExist(err) {
		t.Fatalf("stable desktop endpoint created an obsolete registry directory; stat error = %v", err)
	}
}

func TestDesktopMCPIPCUsesPrivateSocketAndCleansUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket mode checks are covered by the Windows named-pipe test")
	}
	appRuntime := newDesktopIPCTestRuntime(t)
	options := newDesktopIPCOptions(t)
	endpoint, err := StartDesktopIPCServer(appRuntime, desktopIPCExecutablePath(t), options)
	if err != nil {
		t.Fatalf("start desktop IPC server: %v", err)
	}
	instance := endpoint.Instance()
	t.Cleanup(func() {
		if err := endpoint.Close(); err != nil {
			t.Errorf("close desktop IPC server: %v", err)
		}
	})

	socketDirInfo, err := os.Stat(options.SocketDir)
	if err != nil {
		t.Fatalf("stat socket directory: %v", err)
	}
	if got := socketDirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("socket directory mode = %04o; want 0700", got)
	}
	socketInfo, err := os.Stat(instance.Address)
	if err != nil {
		t.Fatalf("stat Unix socket: %v", err)
	}
	if got := socketInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("Unix socket mode = %04o; want 0600", got)
	}
	if filepath.Base(instance.Address) != "glance-mcp.sock" {
		t.Fatalf("Unix socket name = %q; want stable glance-mcp.sock", filepath.Base(instance.Address))
	}
	if err := endpoint.Close(); err != nil {
		t.Fatalf("close desktop IPC server: %v", err)
	}
	if _, err := os.Stat(instance.Address); !os.IsNotExist(err) {
		t.Errorf("socket %q remains after shutdown; stat error = %v", instance.Address, err)
	}
}

func TestDesktopMCPIPCServesExistingCapabilityCatalog(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	appRuntime := newDesktopIPCTestRuntime(t)
	options := newDesktopIPCOptions(t)
	endpoint, err := StartDesktopIPCServer(appRuntime, desktopIPCExecutablePath(t), options)
	if err != nil {
		t.Fatalf("start desktop IPC server: %v", err)
	}
	t.Cleanup(func() {
		if err := endpoint.Close(); err != nil {
			t.Errorf("close desktop IPC server: %v", err)
		}
	})

	conn, err := dialDesktopIPC(endpoint.Instance().Address)
	if err != nil {
		t.Fatalf("dial desktop IPC server: %v", err)
	}
	defer conn.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "desktop-ipc-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: conn, Writer: conn}, nil)
	if err != nil {
		t.Fatalf("connect MCP client over desktop IPC: %v", err)
	}
	defer session.Close()
	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list MCP tools over desktop IPC: %v", err)
	}
	for _, tool := range result.Tools {
		if tool.Name == "document.list" {
			return
		}
	}
	t.Fatal("desktop IPC MCP session did not expose the existing document.list capability")
}

func TestDesktopMCPInstanceIDIsNotUsedForRouting(t *testing.T) {
	runtime := newDesktopIPCTestRuntime(t)
	options := newDesktopIPCOptions(t)
	executable := desktopIPCExecutablePath(t)
	first, err := StartDesktopIPCServer(runtime, executable, options)
	if err != nil {
		t.Fatalf("start desktop IPC server: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	if first.Instance().ID != "" {
		t.Fatalf("stable desktop IPC unexpectedly published instance ID %q", first.Instance().ID)
	}
	second, err := StartDesktopIPCServer(runtime, executable, options)
	if err == nil {
		_ = second.Close()
		t.Fatal("second desktop process endpoint was allowed in the same socket directory")
	}
	address, err := desktopMCPAddressForOptions(options)
	if err != nil {
		t.Fatalf("resolve stable desktop address: %v", err)
	}
	if first.Instance().Address != address {
		t.Fatalf("desktop endpoint = %q; want stable address %q", first.Instance().Address, address)
	}
}
