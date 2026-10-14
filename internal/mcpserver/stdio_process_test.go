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

type testStdioprocessDocumentIdExpectedRevisionArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Path             string `json:"path"`
}

type testStdioprocessDocumentIdExpectedRevisionTextArguments struct {
	DocumentId       string `json:"document_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Text             string `json:"text"`
}

type testStdioprocessFormatArguments struct {
	Format string `json:"format"`
}

type testStdioprocessEmptyArguments struct {
}

func TestMCPStdioSubprocessHasIndependentRuntimeAndSavesWord(t *testing.T) {
	if os.Getenv("GLANCE_MCP_STDIO_SUBPROCESS") == "1" {
		openPath := os.Getenv("GLANCE_MCP_STDIO_OPEN_PATH")
		handled, exitCode := DispatchCLI([]string{"mcp", "serve", "--stdio", "--open", openPath}, os.Stdout, os.Stderr)
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
	headlessSeedPath := filepath.Join(t.TempDir(), "seed.json")
	if err := os.WriteFile(headlessSeedPath, []byte(`{"headless":true}`), 0o600); err != nil {
		t.Fatalf("create headless seed document: %v", err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestMCPStdioSubprocessHasIndependentRuntimeAndSavesWord$")
	command.Env = append(os.Environ(), "GLANCE_MCP_STDIO_SUBPROCESS=1", "GLANCE_MCP_STDIO_OPEN_PATH="+headlessSeedPath)
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

	listed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.list", Arguments: testStdioprocessEmptyArguments{}})
	if err != nil || listed.IsError {
		t.Fatalf("list subprocess documents = %+v, %v", listed, err)
	}
	if items := rawArray(t, mapValue(t, listed.StructuredContent)["data"]); len(items) != 1 {
		t.Fatalf("headless --open process listed %d documents; want only the explicitly opened seed: %#v", len(items), listed.StructuredContent)
	}

	created, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.create", Arguments: testStdioprocessFormatArguments{Format: "docx"}})
	if err != nil || created.IsError {
		t.Fatalf("create subprocess Word document = %+v, %v", created, err)
	}
	createdData := mapValue(t, mapValue(t, created.StructuredContent)["data"])
	documentID := stringValue(t, createdData["document_id"])
	revision := uint64(numberValue(t, createdData["revision"]))

	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: testStdioprocessDocumentIdExpectedRevisionTextArguments{DocumentId: documentID, ExpectedRevision: revision, Text: "由 stdio MCP 子进程创建并保存"}})
	if err != nil || appended.IsError {
		t.Fatalf("append subprocess Word paragraph = %+v, %v", appended, err)
	}
	revision = uint64(numberValue(t, mapValue(t, appended.StructuredContent)["revision"]))
	path := filepath.Join(t.TempDir(), "stdio-report.docx")
	saved, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "document.save_as", Arguments: testStdioprocessDocumentIdExpectedRevisionArguments{DocumentId: documentID, ExpectedRevision: revision, Path: path}})
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

func runDesktopMCPStdioSubprocessIfRequested() {
	if os.Getenv("GLANCE_MCP_DESKTOP_STDIO_SUBPROCESS") != "1" {
		return
	}
	options := DesktopIPCOptions{
		RuntimeDir: os.Getenv("GLANCE_MCP_DESKTOP_STDIO_RUNTIME_DIR"),
		SocketDir:  os.Getenv("GLANCE_MCP_DESKTOP_STDIO_SOCKET_DIR"),
	}
	args := []string{"mcp", "serve", "--stdio"}
	if instanceID := os.Getenv("GLANCE_MCP_DESKTOP_STDIO_INSTANCE_ID"); instanceID != "" {
		args = append(args, "--instance-id", instanceID)
	}
	handled, exitCode := dispatchCLIWithOptions(args, os.Stdout, os.Stderr, options, "", os.Stdin, os.Stdout)
	if !handled {
		os.Exit(127)
	}
	os.Exit(exitCode)
}

func newDesktopMCPStdioSubprocessCommand(testName string, options DesktopIPCOptions, instanceID string) *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=^"+testName+"$")
	command.Env = append(os.Environ(),
		"GLANCE_MCP_DESKTOP_STDIO_SUBPROCESS=1",
		"GLANCE_MCP_DESKTOP_STDIO_RUNTIME_DIR="+options.RuntimeDir,
		"GLANCE_MCP_DESKTOP_STDIO_SOCKET_DIR="+options.SocketDir,
		"GLANCE_MCP_DESKTOP_STDIO_INSTANCE_ID="+instanceID,
	)
	return command
}

func TestMCPStdioSubprocessTargetsStableDesktopRuntime(t *testing.T) {
	runDesktopMCPStdioSubprocessIfRequested()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	selectedRuntime := newDesktopIPCTestRuntime(t)
	selectedDocument, err := selectedRuntime.CreateDocument(ctx, application.FormatDocx)
	if err != nil {
		t.Fatalf("create selected runtime document: %v", err)
	}
	options := newDesktopIPCOptions(t)
	executable := desktopIPCExecutablePath(t)
	endpoint, err := StartDesktopIPCServer(selectedRuntime, executable, options)
	if err != nil {
		t.Fatalf("start stable desktop endpoint: %v", err)
	}
	t.Cleanup(func() { _ = endpoint.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "desktop-stable-routing-test", Version: "1"}, nil)
	command := newDesktopMCPStdioSubprocessCommand("TestMCPStdioSubprocessTargetsStableDesktopRuntime", options, "ignored-old-id")
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect stdio child to desktop runtime: %v", err)
	}
	defer session.Close()
	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: testStdioprocessDocumentIdExpectedRevisionTextArguments{
		DocumentId: selectedDocument.DocumentID, ExpectedRevision: selectedDocument.Revision, Text: "写入当前 Glance 会话",
	}})
	if err != nil || appended.IsError {
		t.Fatalf("append through current desktop runtime = %+v, %v", appended, err)
	}
	updatedSelected, err := selectedRuntime.GetDocument(ctx, selectedDocument.DocumentID)
	if err != nil {
		t.Fatalf("read selected runtime document: %v", err)
	}
	if updatedSelected.Revision != selectedDocument.Revision+1 || !updatedSelected.Dirty {
		t.Fatalf("desktop runtime was not edited: before=%+v after=%+v", selectedDocument, updatedSelected)
	}
}

func TestMCPStdioSubprocessIgnoresStaleLegacyID(t *testing.T) {
	runDesktopMCPStdioSubprocessIfRequested()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	appRuntime := newDesktopIPCTestRuntime(t)
	document, err := appRuntime.CreateDocument(ctx, application.FormatDocx)
	if err != nil {
		t.Fatalf("create desktop document: %v", err)
	}
	options := newDesktopIPCOptions(t)
	endpoint, err := StartDesktopIPCServer(appRuntime, desktopIPCExecutablePath(t), options)
	if err != nil {
		t.Fatalf("start desktop IPC server: %v", err)
	}
	t.Cleanup(func() { _ = endpoint.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "desktop-stale-id-compatibility-test", Version: "1"}, nil)
	command := newDesktopMCPStdioSubprocessCommand("TestMCPStdioSubprocessIgnoresStaleLegacyID", options, "stale-after-restart")
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect stdio child with ignored stale ID: %v", err)
	}
	defer session.Close()
	appended, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "word.paragraph.append", Arguments: testStdioprocessDocumentIdExpectedRevisionTextArguments{
		DocumentId: document.DocumentID, ExpectedRevision: document.Revision, Text: "重启后唯一实例回退",
	}})
	if err != nil || appended.IsError {
		t.Fatalf("append through stable desktop endpoint = %+v, %v", appended, err)
	}
	updated, err := appRuntime.GetDocument(ctx, document.DocumentID)
	if err != nil {
		t.Fatalf("read desktop document after stale-ID fallback: %v", err)
	}
	if updated.Revision != document.Revision+1 {
		t.Fatalf("stale-ID fallback revision = %d; want %d", updated.Revision, document.Revision+1)
	}
}
