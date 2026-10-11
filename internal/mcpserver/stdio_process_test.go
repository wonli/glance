package mcpserver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"glance/internal/application"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPStdioSubprocessHasIndependentRuntimeAndSavesWord(t *testing.T) {
	if os.Getenv("GLANCE_MCP_STDIO_SUBPROCESS") == "1" {
		handled, exitCode := DispatchCLI([]string{"mcp", "serve", "--stdio"}, os.Stdout, os.Stderr)
		if !handled {
			os.Exit(127)
		}
		os.Exit(exitCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	desktopRuntime, err := application.NewRuntime(ctx, application.RuntimeOptions{})
	if err != nil {
		t.Fatalf("create desktop Runtime: %v", err)
	}
	t.Cleanup(func() { _ = desktopRuntime.Close(context.Background()) })
	desktopDocument, err := desktopRuntime.CreateDocument(ctx, application.FormatDocx)
	if err != nil {
		t.Fatalf("create unsaved desktop document: %v", err)
	}
	desktopEdit, err := desktopRuntime.InvokeCapability(ctx, application.CapabilityRequest{
		RequestID: "desktop-unsaved-edit", DocumentID: desktopDocument.DocumentID,
		CapabilityID: "word.paragraph.append", ExpectedRevision: desktopDocument.Revision,
		Operator:  application.Operator{ID: "desktop-test", Kind: application.OperatorHuman},
		Arguments: []byte(`{"text":"桌面未保存草稿"}`),
	})
	if err != nil {
		t.Fatalf("edit desktop document: %v", err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestMCPStdioSubprocessHasIndependentRuntimeAndSavesWord$")
	command.Env = append(os.Environ(), "GLANCE_MCP_STDIO_SUBPROCESS=1")
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-process-isolation-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect MCP client to stdio subprocess: %v", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = session.Close()
		}
	}()

	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.list", Arguments: map[string]any{}})
	if err != nil || listed.IsError {
		t.Fatalf("list subprocess documents = %+v, %v", listed, err)
	}
	if items, ok := mapValue(t, listed.StructuredContent)["data"].([]any); !ok || len(items) != 0 {
		t.Fatalf("stdio process saw desktop sessions: %#v", listed.StructuredContent)
	}

	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: map[string]any{"format": "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create subprocess Word document = %+v, %v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))

	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "text": "由 stdio MCP 子进程创建并保存",
	}})
	if err != nil || appended.IsError {
		t.Fatalf("append subprocess Word paragraph = %+v, %v", appended, err)
	}
	revision = uint64(numberValue(t, mapValue(t, appended.StructuredContent)["revision"]))
	path := filepath.Join(t.TempDir(), "stdio-report.docx")
	saved, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.save_as", Arguments: map[string]any{
		"document_id": documentID, "expected_revision": revision, "path": path,
	}})
	if err != nil || saved.IsError {
		t.Fatalf("save subprocess Word document = %+v, %v", saved, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stdio subprocess did not create the Word file: %v", err)
	}

	currentDesktop, err := desktopRuntime.GetDocument(ctx, desktopDocument.DocumentID)
	if err != nil {
		t.Fatalf("read desktop session after subprocess save: %v", err)
	}
	if !currentDesktop.Dirty || currentDesktop.Revision != desktopEdit.Revision || currentDesktop.SourcePath != "" {
		t.Fatalf("stdio process changed desktop unsaved session: before=%+v after=%+v", desktopDocument, currentDesktop)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close MCP subprocess session: %v", err)
	}
	closed = true
}
