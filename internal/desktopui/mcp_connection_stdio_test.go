package desktopui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/mcpserver"
)

func newDesktopMCPUIOptions(t *testing.T) mcpserver.DesktopIPCOptions {
	t.Helper()
	socketDir, err := os.MkdirTemp(os.TempDir(), "g-")
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	if err := os.Chmod(socketDir, 0o700); err != nil {
		t.Fatalf("restrict socket directory permissions: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	return mcpserver.DesktopIPCOptions{RuntimeDir: t.TempDir(), SocketDir: socketDir}
}

func TestDesktopMCPSettingsGeneratesCurrentRuntimeConfiguration(t *testing.T) {
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	options := newDesktopMCPUIOptions(t)
	desktop.mcpIPCOptions = options
	if err := desktop.startDesktopMCPIPC(); err != nil {
		t.Fatalf("start desktop MCP IPC endpoint: %v", err)
	}
	instance, available, err := desktop.desktopMCPIPCInstance()
	if err != nil || !available {
		t.Fatalf("desktop MCP IPC instance available=%v error=%v", available, err)
	}

	app := newAppWithRuntime(nil, desktop)
	app.openMCPSettingsPage()
	var configuration struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal([]byte(app.mcpConnectionConfig), &configuration); err != nil {
		t.Fatalf("decode generated stdio configuration: %v", err)
	}
	if configuration.Command != instance.ExecutablePath {
		t.Fatalf("stdio command = %q; want actual executable path %q", configuration.Command, instance.ExecutablePath)
	}
	wantArgs := []string{"mcp", "serve", "--stdio"}
	if len(configuration.Args) != len(wantArgs) {
		t.Fatalf("stdio args = %#v; want %#v", configuration.Args, wantArgs)
	}
	for index, want := range wantArgs {
		if configuration.Args[index] != want {
			t.Fatalf("stdio args = %#v; want %#v", configuration.Args, wantArgs)
		}
	}
}

func TestDesktopRuntimeShutdownStopsMCPIPC(t *testing.T) {
	desktop := newDesktopRuntime()
	options := newDesktopMCPUIOptions(t)
	desktop.mcpIPCOptions = options
	if err := desktop.startDesktopMCPIPC(); err != nil {
		t.Fatalf("start desktop MCP IPC endpoint: %v", err)
	}
	_, available, err := desktop.desktopMCPIPCInstance()
	if err != nil || !available {
		t.Fatalf("desktop MCP IPC instance available=%v error=%v", available, err)
	}
	desktop.shutdown()
	_, available, err = desktop.desktopMCPIPCInstance()
	if err != nil || available {
		t.Fatalf("desktop MCP endpoint after shutdown available=%v error=%v", available, err)
	}
}

func TestMCPDesktopConfigurationPreservesExecutablePathWithSpaces(t *testing.T) {
	executablePath := filepath.Join(t.TempDir(), "Glance App", "Glance")
	configuration, err := mcpDesktopStdioConfiguration(executablePath)
	if err != nil {
		t.Fatalf("generate stdio configuration: %v", err)
	}
	var decoded struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal([]byte(configuration), &decoded); err != nil {
		t.Fatalf("decode generated stdio configuration: %v", err)
	}
	if decoded.Command != executablePath {
		t.Fatalf("command = %q; want unsplit path %q", decoded.Command, executablePath)
	}
	wantArgs := []string{"mcp", "serve", "--stdio"}
	if len(decoded.Args) != len(wantArgs) {
		t.Fatalf("args = %#v; want %#v", decoded.Args, wantArgs)
	}
	for index, want := range wantArgs {
		if decoded.Args[index] != want {
			t.Fatalf("args = %#v; want %#v", decoded.Args, wantArgs)
		}
	}
}
