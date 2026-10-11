package mcpserver

import (
	"context"
	"testing"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCloseSessionsTerminatesConnectedMCPClients(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("create Runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("create MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })
	if _, err := session.ListTools(ctx, &mcp.ListToolsParams{}); err != nil {
		t.Fatalf("list tools before server shutdown: %v", err)
	}

	if err := server.CloseSessions(); err != nil {
		t.Fatalf("close MCP server sessions: %v", err)
	}
	active := 0
	for range server.server.Sessions() {
		active++
	}
	if active != 0 {
		t.Fatalf("MCP server retained %d sessions after shutdown", active)
	}
	if _, err := session.ListTools(ctx, &mcp.ListToolsParams{}); err == nil {
		t.Fatal("client session remained usable after server shutdown")
	}
}
