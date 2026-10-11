package desktopui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/agent"
	"glance/internal/agent/provider"
	"glance/internal/application"
	"glance/internal/document"
)

func TestBuiltInAgentAuditsWorkbookCreatesAndSavesReportIntoDesktopWorkspace(t *testing.T) {
	ctx := context.Background()
	runtime := newDesktopRuntime()
	t.Cleanup(runtime.shutdown)
	app := newAppWithRuntime(nil, runtime)
	runtime.attachWindow(1, app)

	workbook, err := runtime.applicationRuntime.CreateDocument(ctx, application.FormatXLSX)
	if err != nil {
		t.Fatalf("create payroll workbook: %v", err)
	}
	workbookRevision := workbook.Revision
	for _, cell := range []struct{ address, value string }{
		{"A1", "员工"}, {"B1", "工资"}, {"A2", "王小明"}, {"B2", "5000"}, {"A3", "李华"}, {"B3", "6000"},
	} {
		arguments, err := json.Marshal(map[string]string{"sheet": "Sheet1", "cell": cell.address, "value": cell.value})
		if err != nil {
			t.Fatalf("encode payroll fixture cell %s: %v", cell.address, err)
		}
		result, err := runtime.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
			RequestID: "agent-workflow-fixture-" + cell.address, DocumentID: workbook.DocumentID,
			CommandID: "spreadsheet.cell.set", ExpectedRevision: workbookRevision,
			Operator:  application.Operator{ID: "workflow-fixture", Kind: application.OperatorHuman},
			Arguments: arguments,
		})
		if err != nil {
			t.Fatalf("seed payroll cell %s: %v", cell.address, err)
		}
		workbookRevision = result.Revision
	}
	app.processAgentApplicationEvents()
	if app.documentSessionState.documentID != workbook.DocumentID {
		t.Fatalf("desktop did not attach seeded workbook: active=%q, want %q", app.documentSessionState.documentID, workbook.DocumentID)
	}

	reportPath := filepath.Join(t.TempDir(), "工资日报.docx")
	model := &payrollReportWorkflowProvider{
		t: t, workbookID: workbook.DocumentID, workbookRevision: workbookRevision, reportPath: reportPath,
	}
	client, err := agent.ConnectMCP(ctx, runtime.applicationRuntime)
	if err != nil {
		t.Fatalf("ConnectMCP: %v", err)
	}
	host, err := agent.NewHost(ctx, model, client.Session, agent.HostOptions{Model: "workflow-test", MaxToolRounds: 6, MaxToolCalls: 6})
	if err != nil {
		_ = client.Close()
		t.Fatalf("NewHost: %v", err)
	}
	runtime.agentMu.Lock()
	runtime.agentClient, runtime.agentHost = client, host
	runtime.agentMu.Unlock()
	t.Cleanup(func() {
		app.shutdownOnce.Do(func() { close(app.shutdown) })
		app.closeAgentSubscription()
		app.stopAndWaitManagedBackground()
		app.closeWorkspace()
	})
	turnCompleted := make(chan struct{}, 1)
	stopObserver := host.Subscribe(ctx, "window-1", func(event agent.AgentEvent) {
		if event.Type == agent.EventTurnCompleted {
			select {
			case turnCompleted <- struct{}{}:
			default:
			}
		}
	})
	t.Cleanup(stopObserver)

	app.agentUI.chatInput.SetText("检查工资表并生成今天的工资日报，保存为新 Word 文件。")
	app.sendAgentChatMessage()
	select {
	case <-turnCompleted:
	case <-time.After(5 * time.Second):
		t.Fatal("built-in Agent workflow did not complete")
	}
	app.processAgentChatEvents()
	app.processAgentApplicationEvents()

	if model.calls != 5 || model.reportDocumentID == "" {
		t.Fatalf("workflow provider calls=%d report document=%q", model.calls, model.reportDocumentID)
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("Agent report file was not created: %v", err)
	}
	info, err := runtime.applicationRuntime.GetDocument(ctx, model.reportDocumentID)
	if err != nil || info.SourcePath != canonicalDocumentPath(reportPath) || info.Dirty {
		t.Fatalf("saved report state = %+v, error=%v", info, err)
	}
	snapshot, err := runtime.applicationRuntime.Queries().InspectDocument(ctx, application.InspectRequest{
		DocumentID: model.reportDocumentID, Start: 0, Limit: 20,
	})
	if err != nil || !strings.Contains(string(snapshot.Content), model.reportText) {
		t.Fatalf("saved report content is missing; error=%v content=%s", err, snapshot.Content)
	}
	meta, ok := app.tabMeta[model.reportDocumentID]
	if !ok || meta.Title != filepath.Base(reportPath) || canonicalDocumentPath(meta.SourcePath) != canonicalDocumentPath(reportPath) {
		t.Fatalf("Agent-created report was not reflected in desktop workspace: %+v, found=%v", meta, ok)
	}
	var finalChat string
	for _, line := range app.agentUI.chatLines {
		if line.role == "assistant" {
			finalChat += line.text
		}
	}
	if !strings.Contains(finalChat, reportPath) {
		t.Fatalf("chat did not report the saved report path %q: %s", reportPath, finalChat)
	}
}

type payrollReportWorkflowProvider struct {
	t                *testing.T
	workbookID       string
	workbookRevision uint64
	reportPath       string
	reportText       string
	reportDocumentID string
	reportRevision   uint64
	calls            int
}

func (p *payrollReportWorkflowProvider) Stream(_ context.Context, request provider.ModelRequest, emit func(provider.ModelEvent) error) error {
	call := p.calls
	p.calls++
	if call == 0 {
		p.reportText = "工资表日报：Sheet1 A1:B3 共检查 6 个单元格，包含员工和工资列，未发现异常。"
		return emit(provider.ModelEvent{ToolCallDelta: workflowToolCall(0, "audit-payroll", "workbook.audit", map[string]any{
			"document_id": p.workbookID, "expected_revision": p.workbookRevision,
			"sheet": "Sheet1", "range": document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 3, EndColumn: 2},
		})})
	}
	lastTool, err := lastWorkflowToolResult(request)
	if err != nil {
		return err
	}
	switch call {
	case 1:
		var result struct {
			Data struct {
				CellsScanned  int `json:"cells_scanned"`
				FindingsTotal int `json:"findings_total"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(lastTool), &result); err != nil {
			return err
		}
		if result.Data.CellsScanned != 6 || result.Data.FindingsTotal != 0 {
			p.t.Errorf("Agent audit result = %+v", result.Data)
		}
		return emit(provider.ModelEvent{ToolCallDelta: workflowToolCall(0, "create-report", "document.create", map[string]any{"format": "docx"})})
	case 2:
		var result struct {
			Data application.DocumentInfo `json:"data"`
		}
		if err := json.Unmarshal([]byte(lastTool), &result); err != nil {
			return err
		}
		p.reportDocumentID = result.Data.DocumentID
		p.reportRevision = result.Data.Revision
		if p.reportDocumentID == "" {
			p.t.Error("document.create returned no report document ID")
		}
		return emit(provider.ModelEvent{ToolCallDelta: workflowToolCall(0, "append-report", "word.paragraph.append", map[string]any{
			"document_id": p.reportDocumentID, "expected_revision": p.reportRevision, "text": p.reportText,
		})})
	case 3:
		var result struct {
			DocumentID string `json:"document_id"`
			Revision   uint64 `json:"revision"`
			Data       struct {
				Text string `json:"text"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(lastTool), &result); err != nil {
			return err
		}
		if result.DocumentID != p.reportDocumentID || result.Revision <= p.reportRevision || result.Data.Text != p.reportText {
			p.t.Errorf("word.paragraph.append result = %+v", result)
		}
		p.reportRevision = result.Revision
		return emit(provider.ModelEvent{ToolCallDelta: workflowToolCall(0, "save-report", "document.save_as", map[string]any{
			"document_id": p.reportDocumentID, "expected_revision": p.reportRevision, "path": p.reportPath,
		})})
	case 4:
		var result struct {
			Data application.DocumentInfo `json:"data"`
		}
		if err := json.Unmarshal([]byte(lastTool), &result); err != nil {
			return err
		}
		if result.Data.SourcePath != canonicalDocumentPath(p.reportPath) || result.Data.Dirty {
			p.t.Errorf("document.save_as result = %+v", result.Data)
		}
		if err := emit(provider.ModelEvent{TextDelta: "已检查工资表并生成日报，文件已保存到：" + p.reportPath}); err != nil {
			return err
		}
		return emit(provider.ModelEvent{Done: true})
	default:
		return context.Canceled
	}
}

func workflowToolCall(index int, id, name string, arguments map[string]any) *provider.ToolCallDelta {
	encoded, _ := json.Marshal(arguments)
	return &provider.ToolCallDelta{Index: index, ID: id, Type: "function", Name: name, ArgumentsDelta: string(encoded)}
}

func lastWorkflowToolResult(request provider.ModelRequest) (string, error) {
	if len(request.Messages) == 0 {
		return "", context.Canceled
	}
	last := request.Messages[len(request.Messages)-1]
	if last.Role != "tool" {
		return "", context.Canceled
	}
	return last.Content, nil
}
