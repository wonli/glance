package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"glance/internal/application"
)

func TestMCPWordEditorReviewReturnsStructuredFindings(t *testing.T) {
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
	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: map[string]any{"format": "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create Word tool result = %+v, %v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))
	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "text": "重复，，标点（未闭合",
	}})
	if err != nil || appended.IsError {
		t.Fatalf("append Word tool result = %+v, %v", appended, err)
	}
	revision = uint64(numberValue(t, mapValue(t, appended.StructuredContent)["revision"]))
	reviewed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.review.editor.analyze", Arguments: map[string]any{
		"document_id": documentID, "start_block": 0, "max_blocks": 100,
	}})
	if err != nil || reviewed.IsError {
		t.Fatalf("review Word Editor tool result = %+v, %v", reviewed, err)
	}
	data := mapValue(t, mapValue(t, reviewed.StructuredContent)["data"])
	findings, ok := data["findings"].([]any)
	if !ok || len(findings) != 2 {
		t.Fatalf("Word review findings = %#v", data["findings"])
	}
	if stringValue(t, mapValue(t, findings[0])["code"]) != "repeated_punctuation" || stringValue(t, mapValue(t, findings[1])["code"]) != "unbalanced_delimiters" {
		t.Fatalf("Word review findings = %#v", findings)
	}
	first := mapValue(t, findings[0])
	if stringValue(t, first["block_id"]) == "" || stringValue(t, first["paragraph_id"]) == "" || stringValue(t, first["run_id"]) == "" {
		t.Fatalf("Word review did not return stable targets: %#v", first)
	}
}
