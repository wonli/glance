package desktopui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/xlsx"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testMcpconnectionDocumentIdExpectedRevisionArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Sheet            string `json:"sheet"`
	Cell             string `json:"cell"`
	Value            string `json:"value"`
}

func TestDesktopMCPHTTPEditsTheLiveGioSession(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	book := xlsx.NewBlank()
	if err := app.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatalf("install workbook: %v", err)
	}
	if err := desktop.startDesktopMCPEndpoint(); err != nil {
		t.Fatalf("start desktop MCP endpoint: %v", err)
	}
	info, available, err := desktop.desktopMCPConnectionInfo()
	if err != nil || !available {
		t.Fatalf("desktop MCP connection info available=%v error=%v", available, err)
	}
	if err := desktop.startDesktopMCPEndpoint(); err != nil {
		t.Fatalf("repeat desktop MCP endpoint start: %v", err)
	}
	secondInfo, secondAvailable, secondErr := desktop.desktopMCPConnectionInfo()
	if secondErr != nil || !secondAvailable || secondInfo.URL != info.URL || secondInfo.Token != info.Token {
		t.Fatalf("repeat start changed endpoint: first=%+v second=%+v available=%v error=%v", info, secondInfo, secondAvailable, secondErr)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "desktop-live-session-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   info.URL,
		HTTPClient: &http.Client{Transport: desktopMCPAuthTransport{token: info.Token, base: http.DefaultTransport}},
	}, nil)
	if err != nil {
		t.Fatalf("connect to desktop MCP: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	documentInfo, err := desktop.applicationRuntime.GetDocument(ctx, app.documentID)
	if err != nil {
		t.Fatalf("get live document: %v", err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "spreadsheet.cell.set", Arguments: testMcpconnectionDocumentIdExpectedRevisionArguments{DocumentId: app.documentID, ExpectedRevision: documentInfo.Revision, Sheet: "Sheet1", Cell: "A1", Value: "由桌面 MCP 写入"}})
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

func TestDesktopRuntimeShutdownStopsMCPHTTPEndpoint(t *testing.T) {
	desktop := newDesktopRuntime()
	if err := desktop.startDesktopMCPEndpoint(); err != nil {
		t.Fatalf("start desktop MCP endpoint: %v", err)
	}
	info, available, err := desktop.desktopMCPConnectionInfo()
	if err != nil || !available {
		t.Fatalf("desktop MCP connection info available=%v error=%v", available, err)
	}
	desktop.shutdown()
	if _, err := http.Get(info.URL); err == nil {
		t.Fatal("MCP endpoint still accepted connections after desktop runtime shutdown")
	}
	desktop.shutdown()
}

func TestMCPConnectionDialogShowsEphemeralClientConfiguration(t *testing.T) {
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	if err := desktop.startDesktopMCPEndpoint(); err != nil {
		t.Fatalf("start desktop MCP endpoint: %v", err)
	}
	app := newAppWithRuntime(nil, desktop)
	app.showMCPConnectionDialog()
	if !app.modal.Visible || app.modal.Title != "MCP 连接" || app.modal.CustomLayout == nil {
		t.Fatalf("MCP connection dialog is incomplete: visible=%v title=%q custom=%v", app.modal.Visible, app.modal.Title, app.modal.CustomLayout != nil)
	}
	info, available, err := desktop.desktopMCPConnectionInfo()
	if err != nil || !available {
		t.Fatalf("desktop MCP info available=%v error=%v", available, err)
	}
	var configuration struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal([]byte(app.mcpConnectionConfig), &configuration); err != nil {
		t.Fatalf("decode displayed MCP connection config: %v", err)
	}
	if configuration.URL != info.URL || configuration.Headers["Authorization"] != "Bearer "+info.Token {
		t.Fatalf("displayed MCP config does not match endpoint info: %+v", configuration)
	}
}

type desktopMCPAuthTransport struct {
	token string
	base  http.RoundTripper
}

func (t desktopMCPAuthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}
