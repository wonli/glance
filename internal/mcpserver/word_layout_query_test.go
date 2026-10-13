package mcpserver

import (
	"context"
	"testing"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testWordlayoutDocumentIdExpectedRevisionArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
}

func TestMCPWordPageSetupQueryReturnsSectionData(t *testing.T) {
	ctx := context.Background()
	runtime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	doc, err := runtime.CreateDocument(ctx, application.FormatDocx)
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	server, err := New(runtime)
	if err != nil {
		t.Fatalf("New MCP server: %v", err)
	}
	session, closeSession, err := server.ConnectInMemory(ctx)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() { _ = closeSession() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.page_setup.get", Arguments: testWordlayoutDocumentIdExpectedRevisionArguments{DocumentId: doc.DocumentID, ExpectedRevision: doc.Revision}})
	if err != nil || result.IsError {
		t.Fatalf("query Word page setup = %+v, %v", result, err)
	}
	data := mapValue(t, mapValue(t, result.StructuredContent)["data"])
	sections := rawArray(t, data["sections"])
	if len(sections) == 0 {
		t.Fatalf("Word page setup sections = %#v", data["sections"])
	}
	setup := mapValue(t, mapValue(t, sections[0])["page_setup"])
	if _, ok := setup["width_points"]; !ok {
		t.Fatalf("page setup has no width in points: %#v", setup)
	}
	if got := uint64(numberValue(t, mapValue(t, result.StructuredContent)["revision"])); got != doc.Revision {
		t.Fatalf("page setup query revision = %d, want unchanged %d", got, doc.Revision)
	}
}
