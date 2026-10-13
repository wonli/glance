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

type testMarkdownpdfDocumentIdExpectedRevisionArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	TargetPath       string `json:"target_path"`
	Orientation      string `json:"orientation"`
}

type testMarkdownpdfPathArguments struct {
	Path string `json:"path"`
}

func TestMCPMarkdownExportPDFCreatesPDFFromLiveDocument(t *testing.T) {
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

	source := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(source, []byte("# MCP Markdown 报告\n\n实时会话通过 MCP 导出。\n"), 0o600); err != nil {
		t.Fatalf("write Markdown source: %v", err)
	}
	opened, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.open", Arguments: testMarkdownpdfPathArguments{Path: source}})
	if err != nil || opened.IsError {
		t.Fatalf("open Markdown tool result = %+v, %v", opened, err)
	}
	openedData := mapValue(t, mapValue(t, opened.StructuredContent)["data"])
	documentID := stringValue(t, openedData["document_id"])
	revision := uint64(numberValue(t, openedData["revision"]))
	target := filepath.Join(t.TempDir(), "mcp-markdown.pdf")
	exported, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "markdown.document.export_pdf", Arguments: testMarkdownpdfDocumentIdExpectedRevisionArguments{DocumentId: documentID, ExpectedRevision: revision, TargetPath: target, Orientation: "portrait"}})
	if err != nil || exported.IsError {
		t.Fatalf("export Markdown PDF tool result = %+v, %v", exported, err)
	}
	data := mapValue(t, mapValue(t, exported.StructuredContent)["data"])
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve exported PDF path: %v", err)
	}
	if stringValue(t, data["path"]) != resolvedTarget || numberValue(t, data["bytes_written"]) <= 0 || numberValue(t, data["page_count"]) < 1 || numberValue(t, data["missing_image_count"]) != 0 {
		t.Fatalf("Markdown PDF export data = %#v", data)
	}
	reader, err := gopdf.Open(target)
	if err != nil {
		t.Fatalf("open MCP-exported PDF: %v", err)
	}
	if reader.Repaired() || int(numberValue(t, data["page_count"])) != reader.NumPages() {
		t.Fatalf("MCP PDF repaired=%v pages=%d; result=%v", reader.Repaired(), reader.NumPages(), data["page_count"])
	}
}
