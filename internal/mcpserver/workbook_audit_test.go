package mcpserver

import (
	"context"
	"testing"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testWorkbookauditStartRowStartColumnArguments struct {
	StartRow    int `json:"start_row"`
	StartColumn int `json:"start_column"`
	EndRow      int `json:"end_row"`
	EndColumn   int `json:"end_column"`
}

type testWorkbookauditStartRowStartColumnEndRowArguments struct {
	StartRow    int `json:"start_row"`
	StartColumn int `json:"start_column"`
	EndRow      int `json:"end_row"`
	EndColumn   int `json:"end_column"`
}

type testWorkbookauditDocumentIdExpectedRevisionArguments struct {
	DocumentId       string                                              `json:"document_id"`
	ExpectedRevision uint64                                              `json:"expected_revision"`
	Sheet            string                                              `json:"sheet"`
	Range            testWorkbookauditStartRowStartColumnEndRowArguments `json:"range"`
	RequiredHeaders  []string                                            `json:"required_headers"`
	KeyColumns       []int                                               `json:"key_columns"`
}

type testWorkbookauditDocumentIdExpectedRevisionSheetArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Sheet            string `json:"sheet"`
	Cell             string `json:"cell"`
	Value            string `json:"value"`
}

type testWorkbookauditFormatArguments struct {
	Format string `json:"format"`
}

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
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: testWorkbookauditFormatArguments{Format: "xlsx"}})
	if err != nil || created.IsError {
		t.Fatalf("create workbook = %+v, %v", created, err)
	}
	doc := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, doc["document_id"])
	revision := uint64(numberValue(t, doc["revision"]))
	for _, item := range []struct{ cell, value string }{{"A1", "ID"}, {"B1", "ID"}, {"A2", "1"}, {"B2", "#REF!"}} {
		set, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "spreadsheet.cell.set", Arguments: testWorkbookauditDocumentIdExpectedRevisionSheetArguments{DocumentId: documentID, ExpectedRevision: revision, Sheet: "Sheet1", Cell: item.cell, Value: item.value}})
		if err != nil || set.IsError {
			t.Fatalf("set audit fixture %s = %+v, %v", item.cell, set, err)
		}
		revision = uint64(numberValue(t, mapValue(t, set.StructuredContent)["revision"]))
	}
	audit, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "workbook.audit", Arguments: testWorkbookauditDocumentIdExpectedRevisionArguments{DocumentId: documentID, ExpectedRevision: revision, Sheet: "Sheet1", Range: testWorkbookauditStartRowStartColumnEndRowArguments{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 2}, RequiredHeaders: []string{"ID", "姓名"}, KeyColumns: []int{1}}})
	if err != nil || audit.IsError {
		t.Fatalf("workbook.audit = %+v, %v", audit, err)
	}
	data := mapValue(t, mapValue(t, audit.StructuredContent)["data"])
	findings := rawArray(t, data["findings"])
	if len(findings) == 0 {
		t.Fatalf("workbook.audit returned no structured findings: %#v", data)
	}
	if got := uint64(numberValue(t, mapValue(t, audit.StructuredContent)["revision"])); got != revision {
		t.Fatalf("read-only workbook audit revision = %d, want %d", got, revision)
	}
}
