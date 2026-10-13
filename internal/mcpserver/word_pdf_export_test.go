package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/SalvioniDigitalSolutions/gopdf"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"glance/internal/application"
)

type testWordpdfDocumentIdExpectedRevisionArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	TargetPath       string `json:"target_path"`
	Orientation      string `json:"orientation"`
}

type testWordpdfFormatArguments struct {
	Format string `json:"format"`
}

func TestMCPWordExportPDFCreatesPDFFromLiveDocument(t *testing.T) {
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

	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: testWordpdfFormatArguments{Format: "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create Word tool result = %+v, %v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))
	path := filepath.Join(t.TempDir(), "mcp-export.pdf")
	exported, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.document.export_pdf", Arguments: testWordpdfDocumentIdExpectedRevisionArguments{DocumentId: documentID, ExpectedRevision: revision, TargetPath: path, Orientation: "document"}})
	if err != nil || exported.IsError {
		t.Fatalf("export Word PDF tool result = %+v, %v", exported, err)
	}
	data := mapValue(t, mapValue(t, exported.StructuredContent)["data"])
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve MCP PDF path: %v", err)
	}
	if stringValue(t, data["path"]) != resolvedPath || numberValue(t, data["bytes_written"]) <= 0 || numberValue(t, data["page_count"]) < 1 {
		t.Fatalf("Word PDF export data = %#v", data)
	}
	if _, err := os.Stat(stringValue(t, data["path"])); err != nil {
		t.Fatalf("stat MCP PDF result path: %v", err)
	}
	reader, err := gopdf.Open(path)
	if err != nil {
		t.Fatalf("open MCP-exported PDF: %v", err)
	}
	if reader.Repaired() || int(numberValue(t, data["page_count"])) != reader.NumPages() {
		t.Fatalf("MCP PDF repaired=%v pages=%d; result=%v", reader.Repaired(), reader.NumPages(), data["page_count"])
	}
}
