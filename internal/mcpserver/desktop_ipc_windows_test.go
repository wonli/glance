//go:build windows

package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sys/windows"
)

func TestDesktopMCPWindowsNamedPipeUsesCurrentUserACLAndCleansUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	appRuntime := newDesktopIPCTestRuntime(t)
	endpoint, err := StartDesktopIPCServer(appRuntime, desktopIPCExecutablePath(t), DesktopIPCOptions{RuntimeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("start desktop IPC server: %v", err)
	}
	instance := endpoint.Instance()
	if !strings.HasPrefix(instance.Address, `\\.\pipe\GlanceMCP-`) {
		t.Fatalf("named pipe address = %q; want a Glance MCP pipe", instance.Address)
	}
	conn, err := dialDesktopIPC(instance.Address)
	if err != nil {
		_ = endpoint.Close()
		t.Fatalf("dial current-user named pipe: %v", err)
	}
	pipeConn, ok := conn.(*desktopNamedPipeConn)
	if !ok {
		_ = conn.Close()
		_ = endpoint.Close()
		t.Fatalf("dial returned %T; want *desktopNamedPipeConn", conn)
	}
	security, err := windows.GetSecurityInfo(windows.Handle(pipeConn.file.Fd()), windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		_ = conn.Close()
		_ = endpoint.Close()
		t.Fatalf("read named pipe DACL: %v", err)
	}
	dacl, _, err := security.DACL()
	if err != nil {
		_ = conn.Close()
		_ = endpoint.Close()
		t.Fatalf("read named pipe DACL entries: %v", err)
	}
	currentUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		_ = conn.Close()
		_ = endpoint.Close()
		t.Fatalf("read current user SID: %v", err)
	}
	wantSID := currentUser.User.Sid.String()
	allowCount := 0
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			_ = conn.Close()
			_ = endpoint.Close()
			t.Fatalf("read named pipe ACE %d: %v", i, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		allowCount++
		gotSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		if gotSID != wantSID {
			t.Errorf("named pipe grants access to SID %s; want only current user %s", gotSID, wantSID)
		}
	}
	if allowCount != 1 {
		t.Errorf("named pipe DACL has %d allow ACEs; want exactly one current-user ACE", allowCount)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "desktop-ipc-windows-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: conn, Writer: conn}, nil)
	if err != nil {
		_ = conn.Close()
		_ = endpoint.Close()
		t.Fatalf("connect MCP client over named pipe: %v", err)
	}
	tools, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil || len(tools.Tools) == 0 {
		_ = session.Close()
		_ = endpoint.Close()
		toolCount := 0
		if tools != nil {
			toolCount = len(tools.Tools)
		}
		t.Fatalf("list MCP tools over named pipe: tools=%d error=%v", toolCount, err)
	}
	if err := endpoint.Close(); err != nil {
		t.Fatalf("close desktop IPC server: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Logf("close MCP client session after server shutdown: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Logf("close named pipe client: %v", err)
	}
	if _, err := dialDesktopIPC(instance.Address); err == nil {
		t.Fatal("named pipe accepted a connection after desktop shutdown")
	}
}
