package desktopui

import (
	"image"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"glance/internal/mcpserver"
	"glance/internal/workspace"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestDesktopMCPDefaultPortIsStable(t *testing.T) {
	if mcpserver.DefaultDesktopMCPPort != 50628 {
		t.Fatalf("DefaultDesktopMCPPort = %d; want 50628", mcpserver.DefaultDesktopMCPPort)
	}
	desktop := newDesktopRuntime()
	port := desktop.loadConfiguredMCPPort()
	if port != 50628 {
		t.Fatalf("loadConfiguredMCPPort() = %d; want 50628", port)
	}
}

func TestDesktopMCPPortPersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatalf("open workspace store: %v", err)
	}
	defer store.Close()

	desktop := newDesktopRuntime()
	desktop.workspaceStore = store

	// 初始状态应为默认端口
	if got := desktop.loadConfiguredMCPPort(); got != 50628 {
		t.Fatalf("initial loadConfiguredMCPPort = %d; want 50628", got)
	}

	// 保存有效端口
	testPort := 52345
	if err := desktop.saveConfiguredMCPPort(testPort); err != nil {
		t.Fatalf("saveConfiguredMCPPort(%d): %v", testPort, err)
	}

	// 验证从 store 读取
	val, found, err := store.Preference(workspace.PreferenceMCPServerPort)
	if err != nil || !found || val != strconv.Itoa(testPort) {
		t.Fatalf("Preference = (%q, %v, %v); want (%q, true, nil)", val, found, err, strconv.Itoa(testPort))
	}
	if got := desktop.loadConfiguredMCPPort(); got != testPort {
		t.Fatalf("loadConfiguredMCPPort = %d; want %d", got, testPort)
	}

	// 模拟应用重启：用全新的 desktopRuntime 加载同一个 store
	reopened := newDesktopRuntime()
	reopened.workspaceStore = store
	if got := reopened.loadConfiguredMCPPort(); got != testPort {
		t.Fatalf("reopened loadConfiguredMCPPort = %d; want %d", got, testPort)
	}

	// 校验非法端口
	for _, invalid := range []int{0, 80, 1023, 65536, 100000, -1} {
		if err := desktop.saveConfiguredMCPPort(invalid); err == nil {
			t.Fatalf("expected error for invalid port %d, got nil", invalid)
		}
	}
}

func TestDesktopMCPRestartServiceUpdatesPortAndEndpoint(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatalf("open workspace store: %v", err)
	}
	defer store.Close()

	desktop := newDesktopRuntime()
	desktop.workspaceStore = store
	t.Cleanup(desktop.shutdown)

	// 获取两个可用的空闲端口用于测试
	l1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port1 := l1.Addr().(*net.TCPAddr).Port
	_ = l1.Close()

	l2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port2 := l2.Addr().(*net.TCPAddr).Port
	_ = l2.Close()

	// 启动第一个端口
	if err := desktop.startDesktopMCPEndpointWithPort(port1); err != nil {
		t.Fatalf("start on port %d: %v", port1, err)
	}
	info1, avail1, err := desktop.desktopMCPConnectionInfo()
	if err != nil || !avail1 {
		t.Fatalf("info1: avail=%v, err=%v", avail1, err)
	}
	if !strings.Contains(info1.URL, strconv.Itoa(port1)) {
		t.Fatalf("info1 URL %q does not contain port %d", info1.URL, port1)
	}

	// 热重启切换到第二个端口
	if err := desktop.restartDesktopMCPEndpoint(port2); err != nil {
		t.Fatalf("restart to port %d: %v", port2, err)
	}
	info2, avail2, err := desktop.desktopMCPConnectionInfo()
	if err != nil || !avail2 {
		t.Fatalf("info2: avail=%v, err=%v", avail2, err)
	}
	if !strings.Contains(info2.URL, strconv.Itoa(port2)) {
		t.Fatalf("info2 URL %q does not contain port %d", info2.URL, port2)
	}

	// 验证持久化已更新
	if saved := desktop.loadConfiguredMCPPort(); saved != port2 {
		t.Fatalf("persisted port = %d; want %d", saved, port2)
	}

	// 验证旧端口已被释放且不可访问
	if _, err := http.Get(info1.URL); err == nil {
		t.Fatalf("old port %d still accepted connection", port1)
	}
}

func TestDesktopMCPRestartPortConflictDoesNotDisruptExistingEndpoint(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatalf("open workspace store: %v", err)
	}
	defer store.Close()

	desktop := newDesktopRuntime()
	desktop.workspaceStore = store
	t.Cleanup(desktop.shutdown)

	l1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port1 := l1.Addr().(*net.TCPAddr).Port
	_ = l1.Close()

	if err := desktop.startDesktopMCPEndpointWithPort(port1); err != nil {
		t.Fatalf("start on port %d: %v", port1, err)
	}

	// 占用另一个端口
	occupier, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupier.Close()
	conflictPort := occupier.Addr().(*net.TCPAddr).Port

	// 尝试切换到被占用的端口，应该报错
	err = desktop.restartDesktopMCPEndpoint(conflictPort)
	if err == nil {
		t.Fatalf("expected error when restarting to conflicting port %d, got nil", conflictPort)
	}
	if !strings.Contains(err.Error(), "占用") && !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("error message %q should mention conflict/in use", err.Error())
	}

	// 验证原服务依然正常
	info, avail, err := desktop.desktopMCPConnectionInfo()
	if err != nil || !avail || !strings.Contains(info.URL, strconv.Itoa(port1)) {
		t.Fatalf("original endpoint disrupted: %+v, avail=%v, err=%v", info, avail, err)
	}
	// 验证持久化配置未被错误覆盖
	if saved := desktop.loadConfiguredMCPPort(); saved == conflictPort {
		t.Fatalf("persisted port was erroneously overwritten with conflict port %d", conflictPort)
	}
}

func TestDesktopMCPSettingsUIPortConfigAndSave(t *testing.T) {
	t.Setenv("GLANCE_MCP_PORT", "0")
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatalf("open workspace store: %v", err)
	}
	defer store.Close()

	desktop := newDesktopRuntime()
	desktop.workspaceStore = store
	t.Cleanup(desktop.shutdown)

	if err := desktop.startDesktopMCPEndpoint(); err != nil {
		t.Fatalf("start desktop MCP endpoint: %v", err)
	}

	app := newAppWithRuntime(nil, desktop)
	app.openMCPSettingsPage()

	currentPort := desktop.currentDesktopMCPPort()
	if app.mcpPortInput.Text() != strconv.Itoa(currentPort) {
		t.Fatalf("port input initial text = %q; want %d", app.mcpPortInput.Text(), currentPort)
	}

	// 查找一个可用的新端口
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	newPort := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	// 模拟用户在输入框输入新端口
	app.mcpPortInput.SetText(strconv.Itoa(newPort))

	// 模拟点击“保存端口”按钮
	app.handleMCPSavePort()

	if !app.mcpPortSaveSuccess {
		t.Fatalf("handleMCPSavePort did not set mcpPortSaveSuccess; error: %q", app.mcpPortError)
	}
	if app.mcpPortError != "" {
		t.Fatalf("unexpected mcpPortError: %q", app.mcpPortError)
	}

	// 验证 URL 和配置已更新为新端口
	if !strings.Contains(app.mcpConnectionURL, strconv.Itoa(newPort)) {
		t.Fatalf("mcpConnectionURL = %q; want port %d", app.mcpConnectionURL, newPort)
	}
	if !strings.Contains(app.mcpConnectionConfig, strconv.Itoa(newPort)) {
		t.Fatalf("mcpConnectionConfig = %q; want port %d", app.mcpConnectionConfig, newPort)
	}

	// 验证持久化已更新
	val, found, err := store.Preference(workspace.PreferenceMCPServerPort)
	if err != nil || !found || val != strconv.Itoa(newPort) {
		t.Fatalf("persisted port preference = (%q, %v, %v); want %d", val, found, err, newPort)
	}
	t.Setenv("GLANCE_MCP_PORT", "")
	if saved := desktop.loadConfiguredMCPPort(); saved != newPort {
		t.Fatalf("loadConfiguredMCPPort = %d; want %d", saved, newPort)
	}

	// 验证输入非法端口时的提示
	app.mcpPortInput.SetText("invalid-port")
	app.handleMCPSavePort()
	if app.mcpPortSaveSuccess {
		t.Fatal("expected failure on invalid port")
	}
	if app.mcpPortError == "" {
		t.Fatal("expected error message on invalid port")
	}
}

func TestDesktopMCPSettingsLayoutRendersPortSection(t *testing.T) {
	t.Setenv("GLANCE_MCP_PORT", "0")
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	if err := desktop.startDesktopMCPEndpoint(); err != nil {
		t.Fatalf("start: %v", err)
	}
	app := newAppWithRuntime(nil, desktop)
	app.openMCPSettingsPage()

	var ops op.Ops
	gtx := layout.Context{
		Ops: &ops,
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}

	dims := app.layoutMCPSettingsPage(gtx)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("layoutMCPSettingsPage returned non-positive dims: %+v", dims)
	}

	btnDims := app.layoutMCPSavePortButton(gtx)
	if btnDims.Size.X != 89 || btnDims.Size.Y != 40 {
		t.Fatalf("layoutMCPSavePortButton dims = %+v; want (89, 40) matching Pen Node rDt8Z", btnDims)
	}
}
