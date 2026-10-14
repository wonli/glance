package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestDesktopMCPRegistryKeepsDistinctLiveInstances(t *testing.T) {
	runtime := newDesktopIPCTestRuntime(t)
	options := newDesktopIPCOptions(t)
	executable := desktopIPCExecutablePath(t)
	first, err := StartDesktopIPCServer(runtime, executable, options)
	if err != nil {
		t.Fatalf("start first desktop IPC server: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := StartDesktopIPCServer(runtime, executable, options)
	if err != nil {
		t.Fatalf("start second desktop IPC server: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	firstInstance, secondInstance := first.Instance(), second.Instance()
	if firstInstance.ID == "" || secondInstance.ID == "" || firstInstance.ID == secondInstance.ID {
		t.Fatalf("instance IDs are not unique: first=%q second=%q", firstInstance.ID, secondInstance.ID)
	}
	if firstInstance.Address == "" || secondInstance.Address == "" || firstInstance.Address == secondInstance.Address {
		t.Fatalf("IPC addresses are not unique: first=%q second=%q", firstInstance.Address, secondInstance.Address)
	}
	if firstInstance.PID != os.Getpid() || secondInstance.PID != os.Getpid() {
		t.Fatalf("instance PIDs = %d and %d; want %d", firstInstance.PID, secondInstance.PID, os.Getpid())
	}
	if firstInstance.ExecutablePath != secondInstance.ExecutablePath {
		t.Fatalf("canonical executable paths differ: %q and %q", firstInstance.ExecutablePath, secondInstance.ExecutablePath)
	}
	instances, err := ListDesktopMCPInstances(options)
	if err != nil {
		t.Fatalf("list desktop MCP instances: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("listed %d instances; want 2: %+v", len(instances), instances)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first desktop IPC server: %v", err)
	}
	instances, err = ListDesktopMCPInstances(options)
	if err != nil {
		t.Fatalf("list instances after close: %v", err)
	}
	if len(instances) != 1 || instances[0].ID != secondInstance.ID {
		t.Fatalf("instances after close = %+v; want only %q", instances, secondInstance.ID)
	}
}

func TestDesktopMCPInstanceResolutionRejectsAmbiguousPath(t *testing.T) {
	runtime := newDesktopIPCTestRuntime(t)
	options := newDesktopIPCOptions(t)
	executable := desktopIPCExecutablePath(t)
	first, err := StartDesktopIPCServer(runtime, executable, options)
	if err != nil {
		t.Fatalf("start first desktop IPC server: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := StartDesktopIPCServer(runtime, executable, options)
	if err != nil {
		t.Fatalf("start second desktop IPC server: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	selected, err := ResolveDesktopMCPInstance(second.Instance().ID, executable, options)
	if err != nil || selected.ID != second.Instance().ID {
		t.Fatalf("resolve explicit instance = %+v, %v; want %q", selected, err, second.Instance().ID)
	}
	if _, err := ResolveDesktopMCPInstance("", executable, options); err == nil || !strings.Contains(strings.ToLower(err.Error()), "ambiguous") {
		t.Fatalf("resolve without ID among same-path instances error = %v; want ambiguity", err)
	}
	if _, err := ResolveDesktopMCPInstance("stale-instance-id", executable, options); err == nil || !strings.Contains(strings.ToLower(err.Error()), "ambiguous") {
		t.Fatalf("resolve stale ID among same-path instances error = %v; want ambiguity", err)
	}
}

func TestDesktopMCPRegistryPrunesOnlyDeadProcesses(t *testing.T) {
	options := newDesktopIPCOptions(t)
	dir := options.RuntimeDir
	executable := desktopIPCExecutablePath(t)
	dead := DesktopMCPInstance{
		ID: "dead-process-instance", PID: 1 << 30, ExecutablePath: executable,
		Address: filepath.Join(options.SocketDir, "dead-process.sock"), StartedAt: time.Now().UTC(),
	}
	liveWithoutEndpoint := DesktopMCPInstance{
		ID: "live-process-instance", PID: os.Getpid(), ExecutablePath: executable,
		Address: filepath.Join(options.SocketDir, "temporarily-unavailable.sock"), StartedAt: time.Now().UTC(),
	}
	for _, instance := range []DesktopMCPInstance{dead, liveWithoutEndpoint} {
		data, err := json.Marshal(instance)
		if err != nil {
			t.Fatalf("encode instance %q: %v", instance.ID, err)
		}
		if err := os.WriteFile(filepath.Join(dir, instance.ID+".json"), data, 0o600); err != nil {
			t.Fatalf("write instance %q: %v", instance.ID, err)
		}
	}

	instances, err := ListDesktopMCPInstances(options)
	if err != nil {
		t.Fatalf("list desktop MCP instances: %v", err)
	}
	if len(instances) != 1 || instances[0].ID != liveWithoutEndpoint.ID {
		t.Fatalf("instances after stale cleanup = %+v; want live process only", instances)
	}
	if _, err := os.Stat(filepath.Join(dir, dead.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("dead process record still exists, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, liveWithoutEndpoint.ID+".json")); err != nil {
		t.Fatalf("live process record was removed: %v", err)
	}
}

func TestDesktopMCPIPCUsesPrivateSocketAndCleansUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket mode checks are covered by the Windows named-pipe test")
	}
	appRuntime := newDesktopIPCTestRuntime(t)
	options := newDesktopIPCOptions(t)
	dir := options.RuntimeDir
	endpoint, err := StartDesktopIPCServer(appRuntime, desktopIPCExecutablePath(t), options)
	if err != nil {
		t.Fatalf("start desktop IPC server: %v", err)
	}
	instance := endpoint.Instance()
	defer func() {
		if err := endpoint.Close(); err != nil {
			t.Errorf("close desktop IPC server: %v", err)
		}
	}()

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat runtime directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("runtime directory mode = %04o; want 0700", got)
	}
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
	if info, err := os.Stat(filepath.Join(dir, instance.ID+".json")); err != nil {
		t.Fatalf("stat registry record: %v", err)
	} else if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("registry record mode = %04o; want 0600", got)
	}

	if err := endpoint.Close(); err != nil {
		t.Fatalf("close desktop IPC server: %v", err)
	}
	for _, path := range []string{instance.Address, filepath.Join(dir, instance.ID+".json")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("path %q remains after shutdown; stat error = %v", path, err)
		}
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

func TestDesktopMCPRegistrySortsInstancesByID(t *testing.T) {
	runtime := newDesktopIPCTestRuntime(t)
	options := newDesktopIPCOptions(t)
	executable := desktopIPCExecutablePath(t)
	var endpoints []*DesktopIPCServer
	for range 3 {
		endpoint, err := StartDesktopIPCServer(runtime, executable, options)
		if err != nil {
			t.Fatalf("start desktop IPC server: %v", err)
		}
		endpoints = append(endpoints, endpoint)
		t.Cleanup(func() { _ = endpoint.Close() })
	}
	instances, err := ListDesktopMCPInstances(options)
	if err != nil {
		t.Fatalf("list desktop MCP instances: %v", err)
	}
	if len(instances) != len(endpoints) {
		t.Fatalf("listed %d instances; want %d", len(instances), len(endpoints))
	}
	for i := 1; i < len(instances); i++ {
		if instances[i-1].ID > instances[i].ID {
			t.Fatalf("instances are not sorted by ID: %+v", instances)
		}
	}
}
