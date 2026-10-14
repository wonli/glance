package desktopui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/xlsx"
	"glance/internal/mcpserver"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testMcpconnectionDocumentIdExpectedRevisionArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Sheet            string `json:"sheet"`
	Cell             string `json:"cell"`
	Value            string `json:"value"`
}

func runDesktopMCPUIStdioSubprocessIfRequested() {
	if os.Getenv("GLANCE_DESKTOP_UI_MCP_STDIO_SUBPROCESS") != "1" {
		return
	}
	handled, exitCode := mcpserver.DispatchCLI([]string{"mcp", "serve", "--stdio"}, os.Stdout, os.Stderr)
	if !handled {
		os.Exit(127)
	}
	os.Exit(exitCode)
}

func TestDesktopMCPStdioEditsTheLiveGioSession(t *testing.T) {
	runDesktopMCPUIStdioSubprocessIfRequested()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	book := xlsx.NewBlank()
	if err := app.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatalf("install workbook: %v", err)
	}
	if err := desktop.startDesktopMCPIPC(); err != nil {
		t.Fatalf("start desktop MCP IPC endpoint: %v", err)
	}
	_, available, err := desktop.desktopMCPIPCInstance()
	if err != nil || !available {
		t.Fatalf("desktop MCP IPC instance available=%v error=%v", available, err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestDesktopMCPStdioEditsTheLiveGioSession$")
	command.Env = append(os.Environ(),
		"GLANCE_DESKTOP_UI_MCP_STDIO_SUBPROCESS=1",
	)
	client := mcp.NewClient(&mcp.Implementation{Name: "desktop-live-session-stdio-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect stdio child to live desktop session: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	documentInfo, err := desktop.applicationRuntime.GetDocument(ctx, app.documentID)
	if err != nil {
		t.Fatalf("get live document: %v", err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "spreadsheet.cell.set", Arguments: testMcpconnectionDocumentIdExpectedRevisionArguments{
		DocumentId: app.documentID, ExpectedRevision: documentInfo.Revision, Sheet: "Sheet1", Cell: "A1", Value: "由桌面 MCP 写入",
	}})
	if err != nil || result.IsError {
		t.Fatalf("MCP spreadsheet edit result=%+v error=%v", result, err)
	}
	updated, err := desktop.applicationRuntime.GetDocument(ctx, app.documentID)
	if err != nil {
		t.Fatalf("get updated live document: %v", err)
	}
	if updated.Revision != documentInfo.Revision+1 || app.wordRevisionAtomic.Load() != updated.Revision {
		t.Fatalf("application/Gio revisions=%d/%d; want %d", updated.Revision, app.wordRevisionAtomic.Load(), documentInfo.Revision+1)
	}
	snapshot, err := desktop.applicationRuntime.Queries().InspectDocument(ctx, application.InspectRequest{
		DocumentID: app.documentID, Sheet: "Sheet1", Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 1, EndColumn: 1},
	})
	if err != nil {
		t.Fatalf("inspect updated live document: %v", err)
	}
	if !strings.Contains(string(snapshot.Content), "由桌面 MCP 写入") {
		t.Fatalf("desktop MCP change is missing from live query: %s", snapshot.Content)
	}
}
