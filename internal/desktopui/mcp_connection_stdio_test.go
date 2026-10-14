package desktopui

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

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

func TestDesktopMCPSettingsSelectsOnlyLiveInstanceAndGeneratesStdioConfiguration(t *testing.T) {
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
	if len(app.mcpInstances) != 1 {
		t.Fatalf("Settings → MCP listed %d instances; want 1: %+v", len(app.mcpInstances), app.mcpInstances)
	}
	if app.mcpSelectedInstanceID != instance.ID {
		t.Fatalf("single live instance selection = %q; want %q", app.mcpSelectedInstanceID, instance.ID)
	}
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
	wantArgs := []string{"mcp", "serve", "--stdio", "--instance-id", instance.ID}
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
	instance, available, err := desktop.desktopMCPIPCInstance()
	if err != nil || !available {
		t.Fatalf("desktop MCP IPC instance available=%v error=%v", available, err)
	}
	desktop.shutdown()
	instances, err := mcpserver.ListDesktopMCPInstances(options)
	if err != nil {
		t.Fatalf("list instances after shutdown: %v", err)
	}
	for _, remaining := range instances {
		if remaining.ID == instance.ID {
			t.Fatalf("MCP instance remained registered after shutdown: %+v", remaining)
		}
	}
}

func TestDesktopMCPSettingsRequiresSelectionForMultipleInstances(t *testing.T) {
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	options := newDesktopMCPUIOptions(t)
	desktop.mcpIPCOptions = options
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	first, err := mcpserver.StartDesktopIPCServer(desktop.applicationRuntime, executable, options)
	if err != nil {
		t.Fatalf("start first same-path desktop instance: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := mcpserver.StartDesktopIPCServer(desktop.applicationRuntime, executable, options)
	if err != nil {
		t.Fatalf("start second same-path desktop instance: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	app := newAppWithRuntime(nil, desktop)
	app.openMCPSettingsPage()
	if len(app.mcpInstances) != 2 {
		t.Fatalf("Settings → MCP listed %d same-path instances; want 2", len(app.mcpInstances))
	}
	if app.mcpSelectedInstanceID != "" || app.mcpConnectionConfig != "" {
		t.Fatalf("multiple instances must require a click before config is generated: selected=%q config=%q", app.mcpSelectedInstanceID, app.mcpConnectionConfig)
	}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1000, 700))}
	app.layoutMCPSettingsPage(gtx)
	row := app.mcpInstanceButtons[second.Instance().ID]
	if row == nil {
		t.Fatal("second live instance has no stable picker row")
	}
	row.Click()
	ops.Reset()
	app.layoutMCPSettingsPage(gtx)
	if app.mcpInstanceButtons[second.Instance().ID] != row {
		t.Fatal("instance picker replaced its persistent clickable between frames")
	}
	if app.mcpSelectedInstanceID != second.Instance().ID {
		t.Fatalf("selected instance = %q; want %q", app.mcpSelectedInstanceID, second.Instance().ID)
	}
	var configuration struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal([]byte(app.mcpConnectionConfig), &configuration); err != nil {
		t.Fatalf("decode selected stdio configuration: %v", err)
	}
	if configuration.Command != second.Instance().ExecutablePath || configuration.Args[len(configuration.Args)-1] != second.Instance().ID {
		t.Fatalf("selected target config = %+v; want instance %+v", configuration, second.Instance())
	}
}

func TestMCPDesktopConfigurationPreservesExecutablePathWithSpaces(t *testing.T) {
	instance := mcpserver.DesktopMCPInstance{ID: "instance-with-spaces", ExecutablePath: filepath.Join(t.TempDir(), "Glance App", "Glance")}
	configuration, err := mcpDesktopStdioConfiguration(instance)
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
	if decoded.Command != instance.ExecutablePath {
		t.Fatalf("command = %q; want unsplit path %q", decoded.Command, instance.ExecutablePath)
	}
	if len(decoded.Args) != 5 || decoded.Args[4] != instance.ID {
		t.Fatalf("args = %#v; want instance ID as one argument", decoded.Args)
	}
}
