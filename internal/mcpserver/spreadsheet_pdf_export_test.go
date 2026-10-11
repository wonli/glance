package mcpserver

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"glance/internal/application"
)

func TestMCPSpecifiesSpreadsheetPDFRangeAndExportsLiveSession(t *testing.T) {
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
		t.Fatalf("connect in-memory MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: map[string]any{"format": "xlsx"}})
	if err != nil || created.IsError {
		t.Fatalf("create XLSX tool result = %+v, %v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))
	updated, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "spreadsheet.cell.set", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision,
		"sheet": "Sheet1", "cell": "A1", "value": "工资统计",
	}})
	if err != nil || updated.IsError {
		t.Fatalf("set spreadsheet source cell = %+v, %v", updated, err)
	}
	updatedDocument, err := runtime.GetDocument(ctx, documentID)
	if err != nil {
		t.Fatalf("GetDocument after cell update: %v", err)
	}
	revision = updatedDocument.Revision
	target := filepath.Join(t.TempDir(), "payroll.pdf")
	exported, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "spreadsheet.document.export_pdf", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision,
		"sheet": "Sheet1", "bounds": map[string]any{"start_row": 1, "start_column": 1, "end_row": 5, "end_column": 4},
		"target_path": target, "orientation": "landscape",
	}})
	if err != nil || exported.IsError {
		t.Fatalf("export spreadsheet PDF tool result = %+v, %v", exported, err)
	}
	data := mapValue(t, mapValue(t, exported.StructuredContent)["data"])
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve exported PDF path: %v", err)
	}
	if stringValue(t, data["path"]) != resolvedTarget || numberValue(t, data["bytes_written"]) <= 0 || numberValue(t, data["page_count"]) < 1 {
		t.Fatalf("spreadsheet PDF export data = %#v", data)
	}
	reader, err := gopdf.Open(target)
	if err != nil {
		t.Fatalf("open MCP-exported PDF: %v", err)
	}
	if reader.Repaired() || int(numberValue(t, data["page_count"])) != reader.NumPages() {
		t.Fatalf("MCP PDF repaired=%v pages=%d; result=%v", reader.Repaired(), reader.NumPages(), data["page_count"])
	}
}
