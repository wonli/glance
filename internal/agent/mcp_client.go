package agent

import (
	"context"
	"errors"
	"sync"

	"glance/internal/application"
	"glance/internal/mcpserver"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// InProcessMCPClient connects the built-in Agent to the same registered tools
// exposed by the desktop and stdio MCP servers.
type InProcessMCPClient struct {
	Server    *mcpserver.Server
	Session   *mcp.ClientSession
	close     func() error
	closeOnce sync.Once
	closeErr  error
}

func ConnectMCP(ctx context.Context, runtime *application.Runtime) (*InProcessMCPClient, error) {
	if runtime == nil {
		return nil, errors.New("application runtime is required")
	}
	server, err := mcpserver.NewWithOperator(runtime, application.OperatorBuiltInAgent)
	if err != nil {
		return nil, err
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		return nil, err
	}
	return &InProcessMCPClient{Server: server, Session: session, close: closeSession}, nil
}

func (c *InProcessMCPClient) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.close != nil {
			c.closeErr = c.close()
			c.close = nil
		}
	})
	return c.closeErr
}
