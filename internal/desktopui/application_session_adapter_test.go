package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/application"
	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/format/jsondoc"
	"glance/internal/format/markdowndoc"
	"glance/internal/format/xlsx"
	"glance/internal/workspace"
)

func TestDesktopRuntimeRegistersLiveSessionInApplicationRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.json")
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	shared := newSharedDocumentSession(path)
	shared.documentID = workspaceSessionID(path)
	shared.jsonDoc = jsondoc.NewBlank()
	if registered := desktop.registerSession(shared); registered != shared {
		t.Fatal("desktop runtime replaced the live session")
	}

	info, err := desktop.applicationRuntime.GetDocument(context.Background(), shared.documentID)
	if err != nil {
		t.Fatalf("application runtime did not receive live session: %v", err)
	}
	if info.Format != application.FormatJSON || info.SourcePath == "" || !info.Dirty {
		t.Fatalf("unexpected shared session metadata: %+v", info)
	}
	openedSession, openedFromDisk, err := desktop.openLiveDocument(path)
	if err != nil {
		t.Fatalf("application runtime open: %v", err)
	}
	if openedSession != shared || openedFromDisk {
		t.Fatalf("open did not reuse the existing desktop session: session=%p openedFromDisk=%v", openedSession, openedFromDisk)
	}
	if !desktop.publishSessionRevision(shared.documentID, 6) {
		t.Fatal("desktop runtime did not publish the session revision")
	}
	updated, err := desktop.applicationRuntime.GetDocument(context.Background(), shared.documentID)
	if err != nil {
		t.Fatalf("application runtime get after revision: %v", err)
	}
	if updated.Revision != 6 {
		t.Fatalf("application revision = %d, want 6", updated.Revision)
	}

	desktop.removeSession(shared.documentID)
	if _, err := desktop.applicationRuntime.GetDocument(context.Background(), shared.documentID); err != application.ErrDocumentNotFound {
		t.Fatalf("closed desktop session remains indexed: %v", err)
	}
}

func TestRibbonAndAgentCommandsShareDesktopDocumentSession(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	book := xlsx.NewBlank()
	if err := app.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatalf("install workbook: %v", err)
	}
	app.selectedCell = "A1"
	app.cellEditor.SetText("手工编辑")
	if err := app.applyCellEdit(); err != nil {
		t.Fatalf("apply Ribbon cell edit: %v", err)
	}

	info, err := desktop.applicationRuntime.GetDocument(ctx, app.documentID)
	if err != nil {
		t.Fatalf("get application document: %v", err)
	}
	result, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-cell-edit", DocumentID: app.documentID, CommandID: "spreadsheet.cell.set",
		ExpectedRevision: info.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorExternalAgent},
		Arguments: json.RawMessage(`{"sheet":"Sheet1","cell":"A2","value":"Agent 写入"}`),
	})
	if err != nil {
		t.Fatalf("execute Agent command: %v", err)
	}
	if result.Revision != info.Revision+1 || app.wordRevisionAtomic.Load() != result.Revision {
		t.Fatalf("application/desktop revisions = %d/%d; want %d", result.Revision, app.wordRevisionAtomic.Load(), result.Revision)
	}
	query, err := desktop.applicationRuntime.Queries().InspectDocument(ctx, application.InspectRequest{
		DocumentID: app.documentID, Sheet: "Sheet1", Range: &document.CellRange{StartRow: 1, StartColumn: 1, EndRow: 2, EndColumn: 1},
	})
	if err != nil {
		t.Fatalf("inspect shared document: %v", err)
	}
	var content struct {
		Rows [][]struct {
			Value string `json:"value"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(query.Content, &content); err != nil {
		t.Fatalf("unmarshal shared query: %v", err)
	}
	if len(content.Rows) != 2 || content.Rows[0][0].Value != "手工编辑" || content.Rows[1][0].Value != "Agent 写入" {
		t.Fatalf("Ribbon and Agent did not update one live session: %s", query.Content)
	}
}

func TestWordPDFExportRejectsUncommittedDesktopDraft(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create Word document: %v", err)
	}
	t.Cleanup(func() { _ = word.Close() })
	_, runID, err := word.AppendBodyTextParagraph("已提交正文")
	if err != nil {
		t.Fatalf("append committed Word text: %v", err)
	}
	source := filepath.Join(t.TempDir(), "shared.docx")
	shared := newSharedDocumentSession(source)
	shared.documentID = workspaceSessionID(source)
	shared.word = word
	shared.wordBlocks = word.WordBlocks()
	shared.syncWordTextState(shared.wordBlocks, true)
	shared.captureWordTextDraft(runID, "尚未提交的正文草稿")
	if registered := desktop.registerSession(shared); registered != shared {
		t.Fatal("desktop runtime replaced the live Word session")
	}
	info, err := desktop.applicationRuntime.GetDocument(ctx, shared.documentID)
	if err != nil {
		t.Fatalf("read registered Word session: %v", err)
	}
	target := filepath.Join(t.TempDir(), "draft.pdf")
	arguments, err := json.Marshal(map[string]string{"target_path": target})
	if err != nil {
		t.Fatalf("encode export arguments: %v", err)
	}
	_, err = desktop.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
		RequestID: "export-word-with-draft", DocumentID: info.DocumentID,
		CapabilityID: "word.document.export_pdf", ExpectedRevision: info.Revision,
		Operator:  application.Operator{ID: "agent", Kind: application.OperatorExternalAgent},
		Arguments: arguments,
	})
	if !errors.Is(err, application.ErrPendingEditorDraft) {
		t.Fatalf("PDF export with pending draft error = %v; want ErrPendingEditorDraft", err)
	}
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("PDF target exists after rejected draft export: stat error=%v", statErr)
	}
}

func TestMarkdownPDFExportRejectsUncommittedDesktopDraft(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	markdown := markdowndoc.NewBlank()
	if err := markdown.SetText("# 已提交正文"); err != nil {
		t.Fatalf("set Markdown source: %v", err)
	}
	if err := app.installDocument(loadedDocument{markdownDoc: markdown}); err != nil {
		t.Fatalf("install Markdown document: %v", err)
	}
	view := app.ensureMarkdownView()
	view.editor.SetText("# 尚未提交的编辑草稿")
	app.observeMarkdownDraft()
	info, err := desktop.applicationRuntime.GetDocument(ctx, app.documentID)
	if err != nil {
		t.Fatalf("read registered Markdown session: %v", err)
	}
	target := filepath.Join(t.TempDir(), "draft.pdf")
	arguments, err := json.Marshal(map[string]string{"target_path": target})
	if err != nil {
		t.Fatalf("encode export arguments: %v", err)
	}
	_, err = desktop.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
		RequestID: "export-markdown-with-draft", DocumentID: info.DocumentID,
		CapabilityID: "markdown.document.export_pdf", ExpectedRevision: info.Revision,
		Operator:  application.Operator{ID: "agent", Kind: application.OperatorExternalAgent},
		Arguments: arguments,
	})
	if !errors.Is(err, application.ErrPendingEditorDraft) {
		t.Fatalf("PDF export with pending Markdown draft error = %v; want ErrPendingEditorDraft", err)
	}
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("PDF target exists after rejected draft export: stat error=%v", statErr)
	}
}

func TestSpreadsheetRecalculateRibbonUsesRegisteredCapability(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	book := xlsx.NewBlank()
	sheet := book.SheetNames()[0]
	if err := book.SetCellText(sheet, "A1", "1"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText(sheet, "A2", "2"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellFormula(sheet, "B1", "=SUM(A1:A2)"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellText(sheet, "A1", "5"); err != nil {
		t.Fatal(err)
	}
	book.SetDirty(false)
	if err := app.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatalf("install workbook: %v", err)
	}
	state := app.documentSessionState
	app.recalculateSpreadsheet()
	select {
	case result := <-app.formatResults:
		if result.err != nil {
			t.Fatalf("Ribbon recalculation: %v", result.err)
		}
		if result.operation != "重新计算公式" {
			t.Fatalf("Ribbon recalculation operation = %q", result.operation)
		}
		app.processSpreadsheetFormatResult()
	case <-time.After(3 * time.Second):
		t.Fatal("Ribbon recalculation did not publish the registered capability result")
	}
	value, err := book.Value(sheet, "B1")
	if err != nil || value != "7" {
		t.Fatalf("Ribbon recalculation value = %q, %v; want 7", value, err)
	}
	info, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
	if err != nil || !info.Dirty || info.Revision != state.wordRevisionAtomic.Load() {
		t.Fatalf("desktop document after Ribbon recalculation = %+v, revision=%d, error=%v", info, state.wordRevisionAtomic.Load(), err)
	}
}

func TestAgentSpreadsheetCellCommandsPreserveTypedValuesInLiveSession(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	book := xlsx.NewBlank()
	if err := app.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatalf("install workbook: %v", err)
	}
	state := app.documentSessionState
	info, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
	if err != nil {
		t.Fatalf("get live workbook: %v", err)
	}
	operator := application.Operator{ID: "live-typed-cell-test", Kind: application.OperatorExternalAgent}
	marshal := func(value any) json.RawMessage {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal command arguments: %v", err)
		}
		return data
	}
	requestID := 0
	invoke := func(commandID string, arguments any) application.CommandResult {
		t.Helper()
		requestID++
		current, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
		if err != nil {
			t.Fatalf("get current live workbook: %v", err)
		}
		result, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
			RequestID: fmt.Sprintf("live-typed-cell-%d", requestID), DocumentID: state.documentID,
			CommandID: commandID, ExpectedRevision: current.Revision, Operator: operator,
			Arguments: marshal(arguments),
		})
		if err != nil {
			t.Fatalf("execute %s: %v", commandID, err)
		}
		return result
	}
	first := invoke("spreadsheet.cell.set", map[string]any{"sheet": "Sheet1", "cell": "A1", "value": "00123"})
	number := invoke("spreadsheet.cell.set", map[string]any{"sheet": "Sheet1", "cell": "B1", "value": 123.5})
	boolean := invoke("spreadsheet.cell.set", map[string]any{"sheet": "Sheet1", "cell": "C1", "value": true})
	if first.Revision != info.Revision+1 || number.Revision != first.Revision+1 || boolean.Revision != number.Revision+1 {
		t.Fatalf("live session revisions: initial=%d text=%d number=%d boolean=%d", info.Revision, first.Revision, number.Revision, boolean.Revision)
	}
	for cell, want := range map[string]string{"A1": "00123", "B1": "123.5", "C1": "TRUE"} {
		if got, err := book.Value("Sheet1", cell); err != nil || got != want {
			t.Fatalf("live cell %s=%q error=%v; want %q", cell, got, err, want)
		}
	}
	noop := invoke("spreadsheet.cell.set", map[string]any{"sheet": "Sheet1", "cell": "A1", "value": "00123"})
	var noopResult struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(noop.Data, &noopResult); err != nil || noopResult.Changed || noop.Revision != boolean.Revision {
		t.Fatalf("live typed no-op result=%s revision=%d error=%v", noop.Data, noop.Revision, err)
	}

	batch := invoke("spreadsheet.cells.set", map[string]any{"sheet": "Sheet1", "edits": []map[string]any{
		{"row": 2, "column": 1, "value": "00045"},
		{"row": 2, "column": 2, "value": 45.25},
		{"row": 2, "column": 3, "value": false},
	}})
	var batchResult struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(batch.Data, &batchResult); err != nil || !batchResult.Changed {
		t.Fatalf("live typed batch result=%s error=%v", batch.Data, err)
	}
	for cell, want := range map[string]string{"A2": "00045", "B2": "45.25", "C2": "FALSE"} {
		if got, err := book.Value("Sheet1", cell); err != nil || got != want {
			t.Fatalf("live batch cell %s=%q error=%v; want %q", cell, got, err, want)
		}
	}
}

func TestSpreadsheetShareRibbonUsesRegisteredCopyCapability(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	book := xlsx.NewBlank()
	if err := book.SetCellText("Sheet1", "A1", "Agent 导出的副本"); err != nil {
		t.Fatal(err)
	}
	if err := app.installDocument(loadedDocument{book: book, sheets: book.SheetNames()}); err != nil {
		t.Fatalf("install workbook: %v", err)
	}
	state := app.documentSessionState
	before, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
	if err != nil {
		t.Fatalf("read live workbook before export: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "shared-copy.xlsx")
	app.spreadsheetShareCopyRequest = &spreadsheetShareCopyPickerRequest{state: state, historyState: book.HistoryState()}
	app.startSpreadsheetShareCopyFile(app.spreadsheetShareCopyRequest, destination)
	select {
	case result := <-app.spreadsheetShareCopyResults:
		if result.err != nil {
			t.Fatalf("registered Ribbon copy: %v", result.err)
		}
		app.applySpreadsheetShareCopyResult(result)
	case <-time.After(3 * time.Second):
		t.Fatal("registered Ribbon copy did not complete")
	}
	after, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
	if err != nil {
		t.Fatalf("read live workbook after export: %v", err)
	}
	if after.SourcePath != before.SourcePath || after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("copy changed live workbook session: before=%+v after=%+v", before, after)
	}
	copyBook, err := xlsx.Open(destination)
	if err != nil {
		t.Fatalf("open exported copy: %v", err)
	}
	defer copyBook.Close()
	if value, err := copyBook.Value("Sheet1", "A1"); err != nil || value != "Agent 导出的副本" {
		t.Fatalf("exported copy A1 = %q, %v", value, err)
	}

	existing := filepath.Join(t.TempDir(), "keep.xlsx")
	if err := os.WriteFile(existing, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.spreadsheetShareCopyRequest = &spreadsheetShareCopyPickerRequest{state: state, historyState: book.HistoryState()}
	app.startSpreadsheetShareCopyFile(app.spreadsheetShareCopyRequest, existing)
	select {
	case result := <-app.spreadsheetShareCopyResults:
		if !errors.Is(result.err, application.ErrTargetExists) {
			t.Fatalf("overwrite attempt error = %v; want ErrTargetExists", result.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("overwrite attempt did not complete")
	}
	contents, err := os.ReadFile(existing)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("existing target changed: contents=%q error=%v", contents, err)
	}

	app.cellEdit.selectCell("A1", "Agent 导出的副本")
	if !app.cellEdit.begin() {
		t.Fatal("could not begin spreadsheet cell draft")
	}
	app.cellEdit.setDraft("未提交的单元格草稿")
	draftDestination := filepath.Join(t.TempDir(), "pending-draft.xlsx")
	info, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
	if err != nil {
		t.Fatalf("read live workbook with pending draft: %v", err)
	}
	arguments, err := json.Marshal(map[string]string{"path": draftDestination})
	if err != nil {
		t.Fatal(err)
	}
	_, err = desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "copy-pending-spreadsheet-draft", DocumentID: state.documentID, CommandID: "document.export_copy",
		ExpectedRevision: info.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorExternalAgent},
		Arguments: arguments,
	})
	if !errors.Is(err, application.ErrPendingEditorDraft) {
		t.Fatalf("Agent copy with pending cell draft error = %v; want ErrPendingEditorDraft", err)
	}
	if _, err := os.Stat(draftDestination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending cell draft copy target exists: %v", err)
	}
	app.cellEdit.cancel()
	secondView := newDocumentSessionView(state.sharedDocumentSession, 99)
	secondView.cellEdit.selectCell("A1", "Agent 导出的副本")
	if !secondView.cellEdit.begin() {
		t.Fatal("could not begin second-window spreadsheet draft")
	}
	stillPendingPath := filepath.Join(t.TempDir(), "still-pending.xlsx")
	stillPendingArgs, err := json.Marshal(map[string]string{"path": stillPendingPath})
	if err != nil {
		t.Fatal(err)
	}
	_, err = desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "copy-second-window-spreadsheet-draft", DocumentID: state.documentID, CommandID: "document.export_copy",
		ExpectedRevision: info.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorExternalAgent},
		Arguments: stillPendingArgs,
	})
	if !errors.Is(err, application.ErrPendingEditorDraft) {
		t.Fatalf("Agent copy with another window draft error = %v; want ErrPendingEditorDraft", err)
	}
	secondView.cellEdit.cancel()
	retryPath := filepath.Join(t.TempDir(), "after-cancel.xlsx")
	retryArgs, err := json.Marshal(map[string]string{"path": retryPath})
	if err != nil {
		t.Fatal(err)
	}
	_, err = desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "copy-after-spreadsheet-draft-cancel", DocumentID: state.documentID, CommandID: "document.export_copy",
		ExpectedRevision: info.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorExternalAgent},
		Arguments: retryArgs,
	})
	if err != nil {
		t.Fatalf("copy after canceling cell draft: %v", err)
	}
}

func TestWordShareRibbonUsesRegisteredCopyCapabilityAndRejectsPendingDraft(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.installDocument(loadedDocument{word: word}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	state := app.documentSessionState
	before, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
	if err != nil {
		t.Fatalf("read live Word document before export: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "shared-copy.docx")
	request := &wordShareCopyPickerRequest{state: state, revision: state.wordRevisionAtomic.Load(), draftEpoch: state.wordDraftEpoch.Load()}
	app.startWordShareCopyFile(request, destination)
	select {
	case result := <-app.wordShareCopyResults:
		if result.err != nil {
			t.Fatalf("registered Word copy: %v", result.err)
		}
		app.applyWordShareCopyResult(result)
	case <-time.After(3 * time.Second):
		t.Fatal("registered Word copy did not complete")
	}
	after, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
	if err != nil {
		t.Fatalf("read live Word document after export: %v", err)
	}
	if after.SourcePath != before.SourcePath || after.Revision != before.Revision || after.Dirty != before.Dirty {
		t.Fatalf("copy changed live Word session: before=%+v after=%+v", before, after)
	}
	copyDoc, err := docx.Open(destination)
	if err != nil {
		t.Fatalf("open exported DOCX copy: %v", err)
	}
	_ = copyDoc.Close()

	state.captureWordTextDraft("draft-run", "未提交草稿")
	draftPath := filepath.Join(t.TempDir(), "draft-copy.docx")
	request = &wordShareCopyPickerRequest{state: state, revision: state.wordRevisionAtomic.Load(), draftEpoch: state.wordDraftEpoch.Load()}
	app.startWordShareCopyFile(request, draftPath)
	if app.status != "文档在选择位置期间已更改，未导出共享副本；请重试" {
		t.Fatalf("Ribbon status with pending draft = %q", app.status)
	}
	info, err := desktop.applicationRuntime.GetDocument(ctx, state.documentID)
	if err != nil {
		t.Fatalf("read live Word document with draft: %v", err)
	}
	arguments, err := json.Marshal(map[string]string{"path": draftPath})
	if err != nil {
		t.Fatal(err)
	}
	_, err = desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "copy-pending-word-draft", DocumentID: state.documentID, CommandID: "document.export_copy",
		ExpectedRevision: info.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorExternalAgent},
		Arguments: arguments,
	})
	if !errors.Is(err, application.ErrPendingEditorDraft) {
		t.Fatalf("Agent copy with pending draft error = %v; want ErrPendingEditorDraft", err)
	}
	if _, err := os.Stat(draftPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending draft copy target exists: %v", err)
	}
}

func TestAgentWordParagraphAppendUpdatesDesktopSessionAndRevision(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create blank Word document: %v", err)
	}
	if err := app.installDocument(loadedDocument{word: word}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	info, err := desktop.applicationRuntime.GetDocument(ctx, app.documentID)
	if err != nil {
		t.Fatalf("get application document: %v", err)
	}
	result, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-report-paragraph", DocumentID: app.documentID, CommandID: "word.paragraph.append",
		ExpectedRevision: info.Revision,
		Operator:         application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments:        json.RawMessage(`{"text":"本周完成项目总结"}`),
	})
	if err != nil {
		t.Fatalf("append Agent report paragraph: %v", err)
	}
	var appended struct {
		ParagraphID string `json:"paragraph_id"`
		RunID       string `json:"run_id"`
	}
	if err := json.Unmarshal(result.Data, &appended); err != nil || appended.RunID == "" || appended.ParagraphID == "" {
		t.Fatalf("decode appended Word identities: %v, data=%s", err, result.Data)
	}
	marshalArgs := func(values map[string]any) json.RawMessage {
		arguments, err := json.Marshal(values)
		if err != nil {
			t.Fatalf("marshal Word command arguments: %v", err)
		}
		return arguments
	}
	alignmentArguments, err := json.Marshal(map[string]string{"paragraph_id": appended.ParagraphID, "alignment": "center"})
	if err != nil {
		t.Fatalf("marshal paragraph alignment arguments: %v", err)
	}
	aligned, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-report-alignment", DocumentID: app.documentID, CommandID: "word.paragraph.alignment.set",
		ExpectedRevision: result.Revision,
		Operator:         application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments:        alignmentArguments,
	})
	if err != nil {
		t.Fatalf("set Agent report alignment: %v", err)
	}
	styleArguments, err := json.Marshal(map[string]any{"run_id": appended.RunID, "bold": true})
	if err != nil {
		t.Fatalf("marshal Word style arguments: %v", err)
	}
	styled, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-report-style", DocumentID: app.documentID, CommandID: "word.text_run.style.set",
		ExpectedRevision: aligned.Revision,
		Operator:         application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments:        styleArguments,
	})
	if err != nil {
		t.Fatalf("set Agent report style: %v", err)
	}
	family, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-report-font-family", DocumentID: app.documentID, CommandID: "word.text_run.font_family.set",
		ExpectedRevision: styled.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"run_id": appended.RunID, "font_family": "Arial"}),
	})
	if err != nil {
		t.Fatalf("set Agent report font family: %v", err)
	}
	sized, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-report-font-size", DocumentID: app.documentID, CommandID: "word.text_run.font_size.set",
		ExpectedRevision: family.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"run_id": appended.RunID, "font_size_points": 16}),
	})
	if err != nil {
		t.Fatalf("set Agent report font size: %v", err)
	}
	listed, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-report-list", DocumentID: app.documentID, CommandID: "word.paragraph.list.set",
		ExpectedRevision: sized.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"paragraph_id": appended.ParagraphID, "list_type": "bullet"}),
	})
	if err != nil {
		t.Fatalf("set Agent report list: %v", err)
	}
	spacing, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-report-spacing", DocumentID: app.documentID, CommandID: "word.paragraph.spacing.set",
		ExpectedRevision: listed.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"paragraph_id": appended.ParagraphID, "before_points": 12, "after_points": 8}),
	})
	if err != nil {
		t.Fatalf("set Agent report paragraph spacing: %v", err)
	}
	ranged, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-report-range-replace", DocumentID: app.documentID, CommandID: "word.text_range.replace",
		ExpectedRevision: spacing.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"run_id": appended.RunID, "start": 2, "end": 4, "replacement": "交付"}),
	})
	if err != nil {
		t.Fatalf("replace Agent report text range: %v", err)
	}
	paged, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-report-page-break", DocumentID: app.documentID, CommandID: "word.page_break.insert",
		ExpectedRevision: ranged.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"run_id": appended.RunID, "start": 8, "end": 8}),
	})
	if err != nil {
		t.Fatalf("insert Agent report page break: %v", err)
	}
	if app.wordRevisionAtomic.Load() != paged.Revision {
		t.Fatalf("desktop revision = %d, Agent command revision = %d", app.wordRevisionAtomic.Load(), paged.Revision)
	}
	if run, ok := selectedWordRun(word.WordBlocks(), appended.RunID); !ok || run.Text != "本周交付项目总结" || !run.Style.Bold || run.Style.FontFamily != "Arial" || run.Style.FontSize != 16 {
		t.Fatalf("Agent style was not applied to the live Word document: %+v, exists=%v", run, ok)
	}
	paragraph, ok := selectedWordParagraph(word.WordBlocks(), appended.ParagraphID)
	if !ok || paragraph.ListKind != document.WordListBullet || paragraph.Layout.BeforePT != 12 || paragraph.Layout.AfterPT != 8 {
		t.Fatalf("Agent paragraph formatting was not applied to the live Word document: %+v, exists=%v", paragraph, ok)
	}
	query, err := desktop.applicationRuntime.Queries().InspectDocument(ctx, application.InspectRequest{DocumentID: app.documentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect desktop Word session: %v", err)
	}
	if !strings.Contains(string(query.Content), "本周交付项目总结") || !query.Info.Dirty {
		t.Fatalf("Agent result is not visible in the live desktop session: content=%s info=%+v", query.Content, query.Info)
	}
	search, err := desktop.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
		RequestID: "agent-search-live-report", DocumentID: app.documentID, CapabilityID: "word.text.search",
		Operator:  application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"query": "交付项目", "limit": 10}),
	})
	if err != nil {
		t.Fatalf("search live desktop Word session: %v", err)
	}
	var searchResult struct {
		Matches []struct {
			RunID string `json:"run_id"`
			Start int    `json:"start"`
			End   int    `json:"end"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(search.Data, &searchResult); err != nil || len(searchResult.Matches) != 1 || searchResult.Matches[0].RunID != appended.RunID || searchResult.Matches[0].Start != 2 || searchResult.Matches[0].End != 6 {
		t.Fatalf("search live desktop Word result = %+v, error=%v, data=%s", searchResult, err, search.Data)
	}
	table, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-insert-live-table", DocumentID: app.documentID, CommandID: "word.table.insert",
		ExpectedRevision: paged.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"after_paragraph_id": appended.ParagraphID, "rows": 1, "columns": 2}),
	})
	if err != nil {
		t.Fatalf("insert table in live desktop Word session: %v", err)
	}
	var tableResult struct {
		TableID string `json:"table_id"`
	}
	if err := json.Unmarshal(table.Data, &tableResult); err != nil || tableResult.TableID == "" {
		t.Fatalf("decode live table result: %v, data=%s", err, table.Data)
	}
	var liveTable bool
	for _, block := range word.WordBlocks() {
		if block.Table != nil && block.ID == tableResult.TableID && len(block.Table.Rows) == 1 && len(block.Table.Rows[0].Cells) == 2 {
			liveTable = true
		}
	}
	if !liveTable || app.wordRevisionAtomic.Load() != table.Revision {
		t.Fatalf("Agent table was not attached to the live Word session: table=%v revision=%d result=%+v", liveTable, app.wordRevisionAtomic.Load(), table)
	}
	textBox, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-insert-live-textbox", DocumentID: app.documentID, CommandID: "word.text_box.insert",
		ExpectedRevision: table.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"after_paragraph_id": appended.ParagraphID, "text": "本周重点：完成工资核算"}),
	})
	if err != nil {
		t.Fatalf("insert text box in live desktop Word session: %v", err)
	}
	query, err = desktop.applicationRuntime.Queries().InspectDocument(ctx, application.InspectRequest{DocumentID: app.documentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect live desktop Word text box: %v", err)
	}
	if !strings.Contains(string(query.Content), "本周重点：完成工资核算") || app.wordRevisionAtomic.Load() != textBox.Revision {
		t.Fatalf("Agent text box is not visible in the live desktop session: revision=%d result=%+v content=%s", app.wordRevisionAtomic.Load(), textBox, query.Content)
	}
	pageRevision := textBox.Revision
	for _, capability := range []struct {
		id   string
		args map[string]any
	}{
		{"word.header.set", map[string]any{"text": "桌面实时页眉"}},
		{"word.footer.set", map[string]any{"text": "桌面实时页脚"}},
		{"word.page_number.set", map[string]any{"alignment": "center"}},
		{"word.page_setup.orientation.set", map[string]any{"orientation": "landscape"}},
		{"word.page_setup.margins.set", map[string]any{"top_points": 54, "right_points": 60, "bottom_points": 66, "left_points": 48}},
		{"word.page_setup.size.set", map[string]any{"width_points": 792, "height_points": 612}},
		{"word.page_setup.columns.set", map[string]any{"columns": 2}},
		{"word.page_setup.line_numbers.set", map[string]any{"mode": "continuous"}},
		{"word.page_setup.borders.set", map[string]any{"border_type": "dashed"}},
	} {
		result, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
			RequestID: "agent-live-" + capability.id, DocumentID: app.documentID, CommandID: capability.id,
			ExpectedRevision: pageRevision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
			Arguments: marshalArgs(capability.args),
		})
		if err != nil {
			t.Fatalf("execute %s in live desktop Word session: %v", capability.id, err)
		}
		if result.Revision != pageRevision+1 || app.wordRevisionAtomic.Load() != result.Revision {
			t.Fatalf("%s did not publish its live revision: app=%d result=%+v", capability.id, app.wordRevisionAtomic.Load(), result)
		}
		pageRevision = result.Revision
		if capability.id == "word.header.set" || capability.id == "word.footer.set" || capability.id == "word.page_number.set" {
			repeated, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
				RequestID: "agent-live-noop-" + capability.id, DocumentID: app.documentID, CommandID: capability.id,
				ExpectedRevision: pageRevision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
				Arguments: marshalArgs(capability.args),
			})
			if err != nil {
				t.Fatalf("repeat %s in live desktop Word session: %v", capability.id, err)
			}
			var data struct {
				Changed bool `json:"changed"`
			}
			if err := json.Unmarshal(repeated.Data, &data); err != nil || data.Changed || repeated.Revision != pageRevision || app.wordRevisionAtomic.Load() != pageRevision {
				t.Fatalf("repeat %s changed live desktop revision: result=%+v data=%+v revision=%d error=%v", capability.id, repeated, data, app.wordRevisionAtomic.Load(), err)
			}
		}
	}
	for _, capability := range []struct {
		id   string
		args map[string]any
	}{
		{"word.page_setup.orientation.set", map[string]any{"orientation": "landscape"}},
		{"word.page_setup.margins.set", map[string]any{"top_points": 54, "right_points": 60, "bottom_points": 66, "left_points": 48}},
		{"word.page_setup.size.set", map[string]any{"width_points": 792, "height_points": 612}},
		{"word.page_setup.columns.set", map[string]any{"columns": 2}},
		{"word.page_setup.line_numbers.set", map[string]any{"mode": "continuous"}},
		{"word.page_setup.borders.set", map[string]any{"border_type": "dashed"}},
	} {
		result, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
			RequestID: "agent-live-noop-" + capability.id, DocumentID: app.documentID, CommandID: capability.id,
			ExpectedRevision: pageRevision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
			Arguments: marshalArgs(capability.args),
		})
		if err != nil {
			t.Fatalf("repeat %s in live desktop Word session: %v", capability.id, err)
		}
		var data struct {
			Changed bool `json:"changed"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil || data.Changed || result.Revision != pageRevision || app.wordRevisionAtomic.Load() != pageRevision {
			t.Fatalf("repeat %s changed live desktop revision: result=%+v data=%+v revision=%d error=%v", capability.id, result, data, app.wordRevisionAtomic.Load(), err)
		}
	}
	language, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-live-language", DocumentID: app.documentID, CommandID: "word.proofing.language.set",
		ExpectedRevision: pageRevision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"run_id": appended.RunID, "language_code": "zh-CN"}),
	})
	if err != nil {
		t.Fatalf("set proofing language in live desktop Word session: %v", err)
	}
	var languageData struct {
		Changed bool `json:"changed"`
	}
	if err := json.Unmarshal(language.Data, &languageData); err != nil || !languageData.Changed || language.Revision != pageRevision+1 {
		t.Fatalf("initial live proofing language result=%s revision=%d error=%v", language.Data, language.Revision, err)
	}
	languageNoop, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-live-language-noop", DocumentID: app.documentID, CommandID: "word.proofing.language.set",
		ExpectedRevision: language.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"run_id": appended.RunID, "language_code": "zh-CN"}),
	})
	var languageNoopData struct {
		Changed bool `json:"changed"`
	}
	if err != nil || json.Unmarshal(languageNoop.Data, &languageNoopData) != nil || languageNoopData.Changed || languageNoop.Revision != language.Revision || app.wordRevisionAtomic.Load() != language.Revision {
		t.Fatalf("repeat live proofing language changed revision: data=%s result_revision=%d desktop_revision=%d error=%v", languageNoop.Data, languageNoop.Revision, app.wordRevisionAtomic.Load(), err)
	}
	pageRevision = language.Revision
	statistics, err := desktop.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
		RequestID: "agent-live-word-statistics", DocumentID: app.documentID, CapabilityID: "word.text.statistics",
		Operator:  application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"start_block": 0, "max_blocks": 100}),
	})
	if err != nil || statistics.Revision != pageRevision {
		t.Fatalf("read live desktop Word statistics: revision=%d want=%d error=%v", statistics.Revision, pageRevision, err)
	}
	properties, err := desktop.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
		RequestID: "agent-live-word-properties", DocumentID: app.documentID, CapabilityID: "word.document.properties.get",
		Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent}, Arguments: marshalArgs(map[string]any{}),
	})
	if err != nil {
		t.Fatalf("read live desktop Word properties: %v", err)
	}
	if properties.Revision != pageRevision {
		t.Fatalf("read Word properties changed revision from %d to %d", pageRevision, properties.Revision)
	}
	updatedProperties, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-live-word-properties-set", DocumentID: app.documentID, CommandID: "word.document.properties.set",
		ExpectedRevision: pageRevision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"title": "桌面实时 Agent 文档", "creator": "Glance Agent"}),
	})
	if err != nil {
		t.Fatalf("update live desktop Word properties: %v", err)
	}
	background, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-live-word-background", DocumentID: app.documentID, CommandID: "word.page.background.set",
		ExpectedRevision: updatedProperties.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"color": "#E3F2FD"}),
	})
	if err != nil {
		t.Fatalf("set live desktop Word page background: %v", err)
	}
	if app.wordRevisionAtomic.Load() != background.Revision {
		t.Fatalf("desktop revision = %d, Word page background revision = %d", app.wordRevisionAtomic.Load(), background.Revision)
	}
	wordProperties, err := word.GetWordDocumentProperties()
	if err != nil || wordProperties.Title != "桌面实时 Agent 文档" || wordProperties.Creator != "Glance Agent" {
		t.Fatalf("metadata command did not update the live document: %+v, error=%v", wordProperties, err)
	}
	wordBackground, err := word.GetPageBackgroundColor()
	if err != nil || wordBackground != "E3F2FD" {
		t.Fatalf("page background command did not update the live document: %q, error=%v", wordBackground, err)
	}
	imagePath := filepath.Join(t.TempDir(), "agent-image.png")
	if err := os.WriteFile(imagePath, tinyToolbarPNG(t), 0o600); err != nil {
		t.Fatalf("write Agent image fixture: %v", err)
	}
	imageInserted, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-live-word-image-insert", DocumentID: app.documentID, CommandID: "word.image.insert",
		ExpectedRevision: background.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"paragraph_id": appended.ParagraphID, "after_inline_id": appended.RunID, "path": imagePath}),
	})
	if err != nil {
		t.Fatalf("insert image into live desktop Word session: %v", err)
	}
	var imageResult struct {
		ImageID string `json:"image_id"`
	}
	if err := json.Unmarshal(imageInserted.Data, &imageResult); err != nil || imageResult.ImageID == "" {
		t.Fatalf("decode inserted desktop image: %v, data=%s", err, imageInserted.Data)
	}
	imageRotated, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-live-word-image-rotate", DocumentID: app.documentID, CommandID: "word.image.rotate",
		ExpectedRevision: imageInserted.Revision, Operator: application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent},
		Arguments: marshalArgs(map[string]any{"image_id": imageResult.ImageID, "delta_degrees": -90}),
	})
	if err != nil || app.wordRevisionAtomic.Load() != imageRotated.Revision {
		t.Fatalf("rotate image in live desktop Word session: result=%+v app revision=%d error=%v", imageRotated, app.wordRevisionAtomic.Load(), err)
	}
	if image := findWordImage(word.WordBlocks(), imageResult.ImageID); image == nil || image.RotationDegrees != 270 {
		t.Fatalf("Agent image rotation is not visible in the live desktop Word session: %+v", image)
	}
}

func TestAgentWordTextRangeClipboardUsesLiveDesktopSession(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create blank Word document: %v", err)
	}
	if err := app.installDocument(loadedDocument{word: word}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	info, err := desktop.applicationRuntime.GetDocument(ctx, app.documentID)
	if err != nil {
		t.Fatalf("get application document: %v", err)
	}
	operator := application.Operator{ID: "agent", Kind: application.OperatorBuiltInAgent}
	appended, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-clipboard-append", DocumentID: app.documentID, CommandID: "word.paragraph.append",
		ExpectedRevision: info.Revision, Operator: operator, Arguments: json.RawMessage(`{"text":"ABC🙂DEF"}`),
	})
	if err != nil {
		t.Fatalf("append source text: %v", err)
	}
	var target struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(appended.Data, &target); err != nil || target.RunID == "" {
		t.Fatalf("decode appended run ID: %v, data=%s", err, appended.Data)
	}
	arguments, err := json.Marshal(map[string]any{"run_id": target.RunID, "start": 3, "end": 4})
	if err != nil {
		t.Fatalf("marshal range arguments: %v", err)
	}
	copied, err := desktop.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
		RequestID: "agent-clipboard-copy", DocumentID: app.documentID, CapabilityID: "word.text_range.copy",
		ExpectedRevision: appended.Revision, Operator: operator, Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("copy desktop text range: %v", err)
	}
	var copyData struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(copied.Data, &copyData); err != nil || copyData.Text != "🙂" || copied.Revision != appended.Revision {
		t.Fatalf("desktop copy = %+v, revision=%d, error=%v", copyData, copied.Revision, err)
	}
	cutArguments, err := json.Marshal(map[string]any{"run_id": target.RunID, "start": 3, "end": 4})
	if err != nil {
		t.Fatalf("marshal cut arguments: %v", err)
	}
	cut, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-clipboard-cut", DocumentID: app.documentID, CommandID: "word.text_range.cut",
		ExpectedRevision: appended.Revision, Operator: operator, Arguments: cutArguments,
	})
	if err != nil {
		t.Fatalf("cut desktop text range: %v", err)
	}
	if cut.Revision != appended.Revision+1 || app.wordRevisionAtomic.Load() != cut.Revision {
		t.Fatalf("desktop cut revision/application revision = %d/%d; want %d", cut.Revision, app.wordRevisionAtomic.Load(), cut.Revision)
	}
	if run, ok := selectedWordRun(word.WordBlocks(), target.RunID); !ok || run.Text != "ABCDEF" {
		t.Fatalf("desktop cut text = %+v, exists=%v; want ABCDEF", run, ok)
	}
	targetParagraph, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-format-painter-target", DocumentID: app.documentID, CommandID: "word.paragraph.append",
		ExpectedRevision: cut.Revision, Operator: operator, Arguments: json.RawMessage(`{"text":"Target"}`),
	})
	if err != nil {
		t.Fatalf("append format painter target: %v", err)
	}
	var paintTarget struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(targetParagraph.Data, &paintTarget); err != nil || paintTarget.RunID == "" {
		t.Fatalf("decode format painter target run: %v, data=%s", err, targetParagraph.Data)
	}
	marshalAgentArguments := func(value any) json.RawMessage {
		arguments, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal Agent command arguments: %v", err)
		}
		return arguments
	}
	styled, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-format-painter-source", DocumentID: app.documentID, CommandID: "word.text_run.style.set",
		ExpectedRevision: targetParagraph.Revision, Operator: operator,
		Arguments: marshalAgentArguments(map[string]any{"run_id": target.RunID, "bold": true}),
	})
	if err != nil {
		t.Fatalf("style format painter source: %v", err)
	}
	painted, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-format-painter-apply", DocumentID: app.documentID, CommandID: "word.text_format.paint",
		ExpectedRevision: styled.Revision, Operator: operator,
		Arguments: marshalAgentArguments(map[string]string{"source_run_id": target.RunID, "target_run_id": paintTarget.RunID}),
	})
	if err != nil {
		t.Fatalf("apply format painter to live desktop session: %v", err)
	}
	if app.wordRevisionAtomic.Load() != painted.Revision {
		t.Fatalf("desktop format painter revision = %d; app revision = %d", painted.Revision, app.wordRevisionAtomic.Load())
	}
	if run, ok := selectedWordRun(word.WordBlocks(), paintTarget.RunID); !ok || run.Text != "Target" || !run.Style.Bold {
		t.Fatalf("desktop format painter target = %+v, exists=%v; want retained text and bold style", run, ok)
	}
	watermarked, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-watermark-set", DocumentID: app.documentID, CommandID: "word.watermark.set",
		ExpectedRevision: painted.Revision, Operator: operator, Arguments: json.RawMessage(`{"text":"机密"}`),
	})
	if err != nil {
		t.Fatalf("set watermark in live desktop session: %v", err)
	}
	watermark, err := desktop.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
		RequestID: "agent-watermark-get", DocumentID: app.documentID, CapabilityID: "word.watermark.get",
		ExpectedRevision: watermarked.Revision, Operator: operator, Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("get watermark from live desktop session: %v", err)
	}
	var watermarkData struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(watermark.Data, &watermarkData); err != nil || watermarkData.Text != "机密" || app.wordRevisionAtomic.Load() != watermarked.Revision {
		t.Fatalf("desktop watermark = %+v, error=%v, desktop revision=%d; want 机密 at revision %d", watermarkData, err, app.wordRevisionAtomic.Load(), watermarked.Revision)
	}
	tracking, err := desktop.applicationRuntime.Commands().Execute(ctx, application.CommandRequest{
		RequestID: "agent-track-revisions-enable", DocumentID: app.documentID, CommandID: "word.review.track.set",
		ExpectedRevision: watermarked.Revision, Operator: operator, Arguments: json.RawMessage(`{"enabled":true}`),
	})
	if err != nil {
		t.Fatalf("enable revision tracking in live desktop session: %v", err)
	}
	trackingState, err := desktop.applicationRuntime.InvokeCapability(ctx, application.CapabilityRequest{
		RequestID: "agent-track-revisions-get", DocumentID: app.documentID, CapabilityID: "word.review.track.get",
		ExpectedRevision: tracking.Revision, Operator: operator, Arguments: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("read revision tracking in live desktop session: %v", err)
	}
	var trackingData struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(trackingState.Data, &trackingData); err != nil || !trackingData.Enabled || app.wordRevisionAtomic.Load() != tracking.Revision {
		t.Fatalf("desktop revision tracking = %+v, error=%v, desktop revision=%d; want enabled at revision %d", trackingData, err, app.wordRevisionAtomic.Load(), tracking.Revision)
	}
}

func TestRibbonWordSpacingPresetUsesApplicationCommandBus(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	app := newAppWithRuntime(nil, desktop)
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("create Word document: %v", err)
	}
	if err := app.installDocument(loadedDocument{word: word}); err != nil {
		t.Fatalf("install Word document: %v", err)
	}
	events := make(chan application.Event, 1)
	cancelEvents := desktop.applicationRuntime.On(ctx, application.EventFilter{
		Types: []string{application.EventDocumentChanged}, DocumentID: app.documentID,
	}, func(event application.Event) { events <- event })
	defer cancelEvents()

	app.executeWordToolbarAction("design-compact")
	select {
	case event := <-events:
		if event.CommandID != "word.paragraph.spacing_all.set" || event.Source != application.OperatorHuman {
			t.Fatalf("Ribbon spacing event = %+v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Ribbon spacing preset did not publish a Command Bus event")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil || !result.applicationCommand {
			t.Fatalf("Ribbon spacing result = %+v", result)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("Ribbon spacing preset did not publish its UI result")
	}
	if app.wordRevisionAtomic.Load() != 2 {
		t.Fatalf("Word session revision = %d; want 2", app.wordRevisionAtomic.Load())
	}
	paragraphs := allWordParagraphs(word.WordBlocks())
	if len(paragraphs) == 0 || paragraphs[0].Layout.AfterPT != 4 {
		t.Fatalf("Ribbon spacing preset did not update live Word session: %+v", paragraphs)
	}
}

func TestApplicationRuntimeOpenAndCreateUseDesktopSessions(t *testing.T) {
	ctx := context.Background()
	desktop := newDesktopRuntime()
	t.Cleanup(desktop.shutdown)
	path := filepath.Join(t.TempDir(), "opened.json")
	if err := os.WriteFile(path, []byte("{\"ready\":true}\n"), 0o600); err != nil {
		t.Fatalf("write JSON fixture: %v", err)
	}
	opened, openedFromDisk, err := desktop.openLiveDocument(path)
	if err != nil {
		t.Fatalf("openLiveDocument: %v", err)
	}
	if !openedFromDisk || opened == nil || opened.jsonDoc == nil {
		t.Fatalf("unexpected opened session: session=%p openedFromDisk=%v", opened, openedFromDisk)
	}
	appInfo, err := desktop.applicationRuntime.GetDocument(ctx, opened.documentID)
	if err != nil {
		t.Fatalf("application runtime did not index opened session: %v", err)
	}
	if appInfo.SourcePath == "" || appInfo.Format != application.FormatJSON {
		t.Fatalf("unexpected opened application metadata: %+v", appInfo)
	}
	if err := opened.jsonDoc.SetText(`{"ready":false,"live":"unsaved"}`); err != nil {
		t.Fatalf("update live JSON draft: %v", err)
	}
	query, err := desktop.applicationRuntime.Queries().InspectDocument(ctx, application.InspectRequest{DocumentID: opened.documentID, Limit: 10})
	if err != nil {
		t.Fatalf("inspect live desktop document: %v", err)
	}
	if !strings.Contains(string(query.Content), "unsaved") || !query.Info.Dirty {
		t.Fatalf("MCP query did not see the live unsaved draft: content=%s info=%+v", query.Content, query.Info)
	}
	var queriedContent map[string]any
	if err := json.Unmarshal(query.Content, &queriedContent); err != nil {
		t.Fatalf("unmarshal live query: %v", err)
	}
	if queriedContent["kind"] != "object" {
		t.Fatalf("unexpected live query DTO: %s", query.Content)
	}
	created, err := desktop.applicationRuntime.CreateDocument(ctx, application.FormatJSON)
	if err != nil {
		t.Fatalf("application runtime create: %v", err)
	}
	if created.DocumentID == "" || !created.Dirty {
		t.Fatalf("unexpected created application metadata: %+v", created)
	}
	createdSession, ok := desktop.session(created.DocumentID)
	if !ok || createdSession.jsonDoc == nil {
		t.Fatal("application create did not register a desktop live session")
	}
	target := filepath.Join(t.TempDir(), "created.json")
	if _, err := desktop.applicationRuntime.SaveDocumentAs(ctx, created.DocumentID, target); err != nil {
		t.Fatalf("save created application session: %v", err)
	}
	if err := desktop.applicationRuntime.CloseDocument(ctx, created.DocumentID); err != nil {
		t.Fatalf("close created application session: %v", err)
	}
}

func TestWorkspaceSnapshotRestoreAttachesApplicationSession(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "workspace.db")
	store, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatalf("open first workspace: %v", err)
	}
	collection, err := workspace.OpenCollection(store)
	if err != nil {
		_ = store.Close()
		t.Fatalf("open first collection: %v", err)
	}
	firstRuntime := newDesktopRuntime()
	firstApp := newAppWithRuntime(nil, firstRuntime)
	firstApp.workspaceStore, firstApp.collection = store, collection
	if err := firstApp.installDocument(loadedDocument{jsonDoc: jsondoc.NewBlank()}); err != nil {
		firstRuntime.shutdown()
		_ = store.Close()
		t.Fatalf("install draft: %v", err)
	}
	documentID := firstApp.documentSessionState.documentID
	if _, err := firstRuntime.applicationRuntime.GetDocument(ctx, documentID); err != nil {
		firstRuntime.shutdown()
		_ = store.Close()
		t.Fatalf("application runtime did not index source session: %v", err)
	}
	firstRuntime.shutdown()
	if err := store.Close(); err != nil {
		t.Fatalf("close first workspace: %v", err)
	}

	secondStore, err := workspace.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen workspace: %v", err)
	}
	secondCollection, err := workspace.OpenCollection(secondStore)
	if err != nil {
		_ = secondStore.Close()
		t.Fatalf("reopen collection: %v", err)
	}
	secondRuntime := newDesktopRuntime()
	secondApp := newAppWithRuntime(nil, secondRuntime)
	secondApp.workspaceStore, secondApp.collection = secondStore, secondCollection
	secondApp.initializeWorkspace()
	t.Cleanup(secondRuntime.shutdown)
	info, err := secondRuntime.applicationRuntime.GetDocument(ctx, documentID)
	if err != nil {
		t.Fatalf("restored workspace session is absent from application runtime: %v", err)
	}
	if info.DocumentID != documentID || info.Format != application.FormatJSON {
		t.Fatalf("unexpected restored application session: %+v", info)
	}
}
