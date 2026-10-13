package mcpserver

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/workspace"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testWorkspacerecentFormatQueryArguments struct {
	Format string `json:"format"`
	Query  string `json:"query"`
	Limit  int    `json:"limit"`
}

func TestMCPWorkspaceRecentListReturnsFilteredPage(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{WorkspaceDBPath: filepath.Join(t.TempDir(), "workspace.sqlite")})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	openedAt := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	if err := runtime.WorkspaceStore().SaveRecents([]workspace.Recent{
		{Path: "/docs/工资表.xlsx", Kind: "xlsx", OpenedAt: openedAt},
		{Path: "/docs/本周周报.docx", Kind: "docx", OpenedAt: openedAt.Add(time.Minute)},
	}); err != nil {
		t.Fatalf("SaveRecents: %v", err)
	}
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("ConnectInMemory: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "workspace.recent.list", Arguments: testWorkspacerecentFormatQueryArguments{Format: "docx", Query: "周报", Limit: 1}})
	if err != nil || result.IsError {
		t.Fatalf("workspace.recent.list result = %+v, %v", result, err)
	}
	data := mapValue(t, mapValue(t, result.StructuredContent)["data"])
	items := rawArray(t, data["items"])
	if len(items) != 1 {
		t.Fatalf("recent list data = %#v", data)
	}
	item := mapValue(t, items[0])
	if stringValue(t, item["path"]) != "/docs/本周周报.docx" || stringValue(t, item["format"]) != "docx" || boolValue(t, data["has_more"]) {
		t.Fatalf("recent item/result = %#v / %#v", item, data)
	}
}

type testWorkspacerecentEmptyArguments struct {
}

func TestMCPWorkspaceRecentListExplainsMissingWorkspace(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("ConnectInMemory: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "workspace.recent.list", Arguments: testWorkspacerecentEmptyArguments{}})
	if err != nil {
		t.Fatalf("workspace.recent.list call: %v", err)
	}
	if !result.IsError || stringValue(t, mapValue(t, result.StructuredContent)["error"]) != "workspace_unavailable" {
		t.Fatalf("missing workspace result = %+v", result)
	}
}

type testWorkspacerecentDocumentIdArguments struct {
	DocumentId string `json:"document_id"`
}

func TestMCPWordVersionListReturnsMetadataForSavedDocument(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{WorkspaceDBPath: filepath.Join(t.TempDir(), "workspace.sqlite")})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, application.FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	path := filepath.Join(t.TempDir(), "report.docx")
	doc, err = runtime.SaveDocumentAs(ctx, doc.DocumentID, path)
	if err != nil {
		t.Fatalf("SaveDocumentAs: %v", err)
	}
	version, added, err := runtime.WorkspaceStore().AddWordVersion(workspace.WordVersionInput{
		PathKey: doc.SourcePath, DisplayPath: path, Source: workspace.WordVersionManualSave,
		CapturedAt: time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC), Snapshot: []byte("safe metadata only"),
	})
	if err != nil || !added {
		t.Fatalf("AddWordVersion = %+v, added=%v, err=%v", version, added, err)
	}
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("ConnectInMemory: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.version.list", Arguments: testWorkspacerecentDocumentIdArguments{DocumentId: doc.DocumentID}})
	if err != nil || result.IsError {
		t.Fatalf("word.version.list result = %+v, %v", result, err)
	}
	data := mapValue(t, mapValue(t, result.StructuredContent)["data"])
	versions := rawArray(t, data["versions"])
	if len(versions) != 1 {
		t.Fatalf("version metadata = %#v", data)
	}
	metadata := mapValue(t, versions[0])
	if numberValue(t, metadata["id"]) != float64(version.ID) || stringValue(t, metadata["source"]) != string(workspace.WordVersionManualSave) || numberValue(t, metadata["size_bytes"]) != float64(len("safe metadata only")) {
		t.Fatalf("version metadata = %#v", metadata)
	}
}
