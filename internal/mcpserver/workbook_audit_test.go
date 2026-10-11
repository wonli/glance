package mcpserver

import (
	"context"
	"testing"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPWorkbookAuditRunsThroughCapabilityCatalog(t *testing.T) {
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
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: map[string]any{"format": "xlsx"}})
	if err != nil || created.IsError {
		t.Fatalf("create workbook = %+v, %v", created, err)
	}
	doc := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, doc["document_id"])
	revision := uint64(numberValue(t, doc["revision"]))
	for _, item := range []struct{ cell, value string }{{"A1", "ID"}, {"B1", "ID"}, {"A2", "1"}, {"B2", "#REF!"}} {
		set, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "spreadsheet.cell.set", Arguments: map[string]any{
			"document_id": documentID, "expected_revision": revision,
			"sheet": "Sheet1", "cell": item.cell, "value": item.value,
		}})
		if err != nil || set.IsError {
			t.Fatalf("set audit fixture %s = %+v, %v", item.cell, set, err)
		}
		revision = uint64(numberValue(t, mapValue(t, set.StructuredContent)["revision"]))
	}
	audit, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "workbook.audit", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision,
		"sheet": "Sheet1", "range": map[string]any{
			"start_row": 1, "start_column": 1, "end_row": 2, "end_column": 2,
		}, "required_headers": []string{"ID", "姓名"}, "key_columns": []int{1},
	}})
	if err != nil || audit.IsError {
		t.Fatalf("workbook.audit = %+v, %v", audit, err)
	}
	data := mapValue(t, mapValue(t, audit.StructuredContent)["data"])
	findings, ok := data["findings"].([]any)
	if !ok || len(findings) == 0 {
		t.Fatalf("workbook.audit returned no structured findings: %#v", data)
	}
	if got := uint64(numberValue(t, mapValue(t, audit.StructuredContent)["revision"])); got != revision {
		t.Fatalf("read-only workbook audit revision = %d, want %d", got, revision)
	}
}
